// src/api/auth.go
package main

import (
	"context"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"   // <-- add
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
)

func init() {
	gob.Register(User{}) // ensure gorilla/sessions can (de)serialize User
}

type User struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Pic   string `json:"picture,omitempty"`
}

var (
	oidcProv            *oidc.Provider
	oidcVerifier        *oidc.IDTokenVerifier
	oauthCfg            *oauth2.Config
	store               *sessions.CookieStore
	cfg                 AuthConfig
	endSessionEndpoint  string // discovered from .well-known
)

type AuthConfig struct {
	Issuer        string
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	Scopes        []string
	SessionSecret []byte
	AllowedDomain string
	SecureCookies bool
	CookieDomain  string

	PostLogoutRedirectURL string // <-- add
}

const (
	sessionName  = "ddui_sess"
	oauthTmpName = "ddui_oauth"
	cookieMaxAge = 7 * 24 * 3600 // 7d
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func readSecretMaybeFile(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if strings.HasPrefix(v, "@/") || strings.HasPrefix(v, "@./") || strings.HasPrefix(v, "@/run/") {
		path := strings.TrimPrefix(v, "@")
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	return v, nil
}

func InitAuthFromEnv() error {
	var err error
	cs, err := readSecretMaybeFile(env("OIDC_CLIENT_SECRET", ""))
	if err != nil {
		return err
	}

	sec, err := readSecretMaybeFile(env("SESSION_SECRET", ""))
	if err != nil {
		return err
	}
	if sec == "" {
		sec = randHex(64)
	}

	cfg = AuthConfig{
		Issuer:        env("OIDC_ISSUER_URL", ""),
		ClientID:      env("OIDC_CLIENT_ID", ""),
		ClientSecret:  cs,
		RedirectURL:   env("OIDC_REDIRECT_URL", ""),
		Scopes:        scopes(env("OIDC_SCOPES", "openid email profile")),
		SessionSecret: []byte(sec),
		AllowedDomain: strings.ToLower(env("OIDC_ALLOWED_EMAIL_DOMAIN", "")),
		SecureCookies: strings.ToLower(env("COOKIE_SECURE", "true")) == "true",
		CookieDomain:  env("COOKIE_DOMAIN", ""),

		PostLogoutRedirectURL: env("OIDC_POST_LOGOUT_REDIRECT_URL", ""), // <-- add
	}
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return errors.New("OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET, OIDC_REDIRECT_URL are required")
	}

	ctx := context.Background()
	oidcProv, err = oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return err
	}
	// discover end_session_endpoint
	var disc struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := oidcProv.Claims(&disc); err == nil {
		endSessionEndpoint = strings.TrimSpace(disc.EndSessionEndpoint)
	}
	if endSessionEndpoint == "" {
		log.Printf("auth: no end_session_endpoint found in discovery; RP-logout will fall back to /login")
	}

	oidcVerifier = oidcProv.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	oauthCfg = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     oidcProv.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
	}

	store = sessions.NewCookieStore(cfg.SessionSecret)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   cookieMaxAge,
		HttpOnly: true,
		Secure:   cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Domain:   cfg.CookieDomain,
	}
	return nil
}

func scopes(s string) []string { /* unchanged */ return strings.Fields(s) }
func randHex(n int) string     { /* unchanged */ b := make([]byte, n/2); _, _ = rand.Read(b); return hex.EncodeToString(b) }

func LoginHandler(w http.ResponseWriter, r *http.Request) { /* unchanged */ }

func CallbackHandler(w http.ResponseWriter, r *http.Request) {
	// ... unchanged up to verifying claims ...

	// Build our app user
	u := User{
		Sub:   claims.Sub,
		Email: strings.ToLower(claims.Email),
		Name:  claims.Name,
		Pic:   claims.Pic,
	}

	// Persist user + id_token in the session for logout (id_token_hint)
	sess, _ := store.Get(r, sessionName)
	sess.Values["user"] = u
	sess.Values["exp"] = time.Now().Add(7 * 24 * time.Hour).Unix()
	sess.Values["id_token"] = rawID // <-- save raw ID token
	_ = sess.Save(r, w)

	log.Printf("auth: login ok sub=%s email=%s", u.Sub, u.Email)
	http.Redirect(w, r, "/", http.StatusFound)
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := store.Get(r, sessionName)
	// get id_token before clearing (optional, recommended)
	var idTokenHint string
	if v, ok := sess.Values["id_token"].(string); ok {
		idTokenHint = v
	}
	// expire session cookie
	sess.Options.MaxAge = -1
	_ = sess.Save(r, w)

	// If we know end_session + have a post-logout redirect, go to the IdP
	if endSessionEndpoint != "" && cfg.PostLogoutRedirectURL != "" {
		u, _ := url.Parse(endSessionEndpoint)
		q := u.Query()
		q.Set("post_logout_redirect_uri", cfg.PostLogoutRedirectURL)
		if idTokenHint != "" {
			q.Set("id_token_hint", idTokenHint)
		}
		// optional, some IdPs like it
		q.Set("client_id", cfg.ClientID)
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
		return
	}

	// Fallback: just land at /login
	http.Redirect(w, r, "/login", http.StatusFound)
}

func SessionHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := store.Get(r, sessionName)
	u, ok := sess.Values["user"].(User)
	exp, _ := sess.Values["exp"].(int64)

	if !ok || exp == 0 || time.Now().Unix() > exp {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

// --- auth helpers/middleware ---

type ctxKey string

const userKey ctxKey = "ddui.user"

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, _ := store.Get(r, sessionName)
		u, ok := sess.Values["user"].(User)
		exp, _ := sess.Values["exp"].(int64)
		if !ok || time.Now().Unix() > exp {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CurrentUser(ctx context.Context) User {
	if v := ctx.Value(userKey); v != nil {
		if u, ok := v.(User); ok {
			return u
		}
	}
	return User{}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func domainForClaims(email, hd, dom string) string {
	if hd != "" {
		return hd
	}
	if dom != "" {
		return dom
	}
	i := strings.LastIndex(email, "@")
	if i > 0 {
		return email[i+1:]
	}
	return ""
}
