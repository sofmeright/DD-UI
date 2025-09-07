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
	"net/url"
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
	oidcProv           *oidc.Provider
	oidcVerifier       *oidc.IDTokenVerifier
	oauthCfg           *oauth2.Config
	store              *sessions.CookieStore
	cfg                AuthConfig
	endSessionEndpoint string // discovered from .well-known
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

	PostLogoutRedirectURL string // used for RP-initiated logout
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

// Support "@/path" style and raw values.
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

// Read from ENV var or *_FILE path env var. Also accepts "@/path" in the value.
func envOrFile(valueKey, fileKey string) (string, error) {
	// 1) Direct value (supports "@/path" shorthand)
	if raw := os.Getenv(valueKey); raw != "" {
		return readSecretMaybeFile(raw)
	}
	// 2) File path via *_FILE
	if fp := strings.TrimSpace(os.Getenv(fileKey)); fp != "" {
		b, err := os.ReadFile(fp)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	return "", nil
}

func InitAuthFromEnv() error {
	var err error

	// ---- OIDC client credentials (allow *_FILE and "@/path")
	clientID, err := envOrFile("OIDC_CLIENT_ID", "OIDC_CLIENT_ID_FILE")
	if err != nil {
		return err
	}
	clientSecret, err := envOrFile("OIDC_CLIENT_SECRET", "OIDC_CLIENT_SECRET_FILE")
	if err != nil {
		return err
	}

	// ---- Session secret (renamed: DDUI_SESSION_SECRET / DDUI_SESSION_SECRET_FILE)
	// Compatibility with old SESSION_SECRET is intentionally dropped.
	sec, err := envOrFile("DDUI_SESSION_SECRET", "DDUI_SESSION_SECRET_FILE")
	if err != nil {
		return err
	}
	if sec == "" {
		sec = randHex(64) // generate one if not provided
	}

	redirect := env("OIDC_REDIRECT_URL", "")

	// Derive SecureCookies if COOKIE_SECURE is unset.
	// - If redirect URI is https, use secure cookies.
	// - If it's http (dev), allow non-secure cookies so the session works locally.
	secureStr := strings.TrimSpace(env("COOKIE_SECURE", ""))
	var secure bool
	if secureStr == "" {
		secure = strings.HasPrefix(strings.ToLower(redirect), "https://")
	} else {
		switch strings.ToLower(secureStr) {
		case "1", "true", "yes", "on":
			secure = true
		default:
			secure = false
		}
	}

	cfg = AuthConfig{
		Issuer:                env("OIDC_ISSUER_URL", ""),
		ClientID:              clientID,
		ClientSecret:          clientSecret,
		RedirectURL:           redirect,
		Scopes:                scopes(env("OIDC_SCOPES", "openid email profile")),
		SessionSecret:         []byte(sec),
		AllowedDomain:         strings.ToLower(env("OIDC_ALLOWED_EMAIL_DOMAIN", "")),
		SecureCookies:         secure,
		CookieDomain:          env("COOKIE_DOMAIN", ""),
		PostLogoutRedirectURL: env("OIDC_POST_LOGOUT_REDIRECT_URL", ""),
	}

	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return errors.New("OIDC_ISSUER_URL, OIDC_CLIENT_ID{/_FILE}, OIDC_CLIENT_SECRET{/_FILE}, OIDC_REDIRECT_URL are required")
	}

	// ---- OIDC wiring
	ctx := context.Background()
	oidcProv, err = oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return err
	}

	// Try to discover end_session_endpoint (not all providers expose it)
	var disc struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := oidcProv.Claims(&disc); err == nil {
		endSessionEndpoint = strings.TrimSpace(disc.EndSessionEndpoint)
	}
	if endSessionEndpoint == "" {
		log.Printf("auth: no end_session_endpoint found in discovery; RP-logout will fall back to local clear")
	}

	oidcVerifier = oidcProv.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	oauthCfg = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     oidcProv.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
	}

	// ---- Session cookie store
	// Use SameSite=None for broadest compatibility when API/UI run on different origins;
	// fall back to Lax for non-secure local dev.
	sameSite := http.SameSiteLaxMode
	if cfg.SecureCookies {
		sameSite = http.SameSiteNoneMode
	}
	store = sessions.NewCookieStore(cfg.SessionSecret)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   cookieMaxAge,
		HttpOnly: true,
		Secure:   cfg.SecureCookies,
		SameSite: sameSite,
		Domain:   cfg.CookieDomain,
	}

	return nil
}

func scopes(s string) []string { return strings.Fields(s) }
func randHex(n int) string     { b := make([]byte, n/2); _, _ = rand.Read(b); return hex.EncodeToString(b) }

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if oauthCfg == nil || oidcProv == nil {
		http.Error(w, "auth not initialized", http.StatusInternalServerError)
		return
	}

	// CSRF + replay protection
	state := randHex(32)
	nonce := randHex(32)

	// Short-lived temp cookie just for OAuth flow data
	tmp, _ := store.Get(r, oauthTmpName)
	tmp.Values["state"] = state
	tmp.Values["nonce"] = nonce
	tmp.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode, // safe for top-level redirects
		Domain:   cfg.CookieDomain,
	}
	_ = tmp.Save(r, w)

	// Build OP authorize URL with nonce
	authURL := oauthCfg.AuthCodeURL(state, oidc.Nonce(nonce))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func CallbackHandler(w http.ResponseWriter, r *http.Request) {
	// Read and immediately expire the temporary oauth cookie
	tmp, _ := store.Get(r, oauthTmpName)
	wantState, _ := tmp.Values["state"].(string)
	nonce, _ := tmp.Values["nonce"].(string)
	tmp.Options.MaxAge = -1
	_ = tmp.Save(r, w)

	// CSRF protection: state must match
	if r.URL.Query().Get("state") != wantState || wantState == "" {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	tok, err := oauthCfg.Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "code exchange failed", http.StatusBadGateway)
		return
	}

	// --- raw id_token for RP-logout ---
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token", http.StatusBadGateway)
		return
	}

	idt, err := oidcVerifier.Verify(ctx, rawID)
	if err != nil {
		http.Error(w, "id token verify failed", http.StatusUnauthorized)
		return
	}
	if idt.Nonce != nonce {
		http.Error(w, "nonce mismatch", http.StatusBadRequest)
		return
	}

	// --- claims ---
	var claims struct {
		Sub    string `json:"sub"`
		Email  string `json:"email"`
		Name   string `json:"name"`
		Pic    string `json:"picture"`
		HD     string `json:"hd"`
		Domain string `json:"domain"`
		Exp    int64  `json:"exp"`
	}
	if err := idt.Claims(&claims); err != nil {
		http.Error(w, "claims parse failed", http.StatusBadGateway)
		return
	}

	if cfg.AllowedDomain != "" {
		d := strings.ToLower(domainForClaims(claims.Email, claims.HD, claims.Domain))
		if d != cfg.AllowedDomain {
			http.Error(w, "domain not allowed", http.StatusForbidden)
			return
		}
	}

	u := User{
		Sub:   claims.Sub,
		Email: strings.ToLower(claims.Email),
		Name:  claims.Name,
		Pic:   claims.Pic,
	}

	// Save user + id_token for logout (id_token_hint)
	sess, _ := store.Get(r, sessionName)
	sess.Values["user"] = u
	sess.Values["exp"] = time.Now().Add(7 * 24 * time.Hour).Unix()
	sess.Values["id_token"] = rawID
	_ = sess.Save(r, w)

	log.Printf("auth: login ok sub=%s email=%s", u.Sub, u.Email)

	http.Redirect(w, r, "/", http.StatusFound)
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// read id_token for RP-initiated logout BEFORE clearing
	sess, _ := store.Get(r, sessionName)
	rawID, _ := sess.Values["id_token"].(string)

	// expire main session
	for k := range sess.Values {
		delete(sess.Values, k)
	}
	// match store options; SameSite mirrors InitAuthFromEnv choice
	sameSite := http.SameSiteLaxMode
	if cfg.SecureCookies {
		sameSite = http.SameSiteNoneMode
	}
	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.SecureCookies,
		SameSite: sameSite,
		Domain:   cfg.CookieDomain,
	}
	_ = sess.Save(r, w)

	// expire oauth temp cookie too
	tmp, _ := store.Get(r, oauthTmpName)
	tmp.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Domain:   cfg.CookieDomain,
	}
	_ = tmp.Save(r, w)

	// If discovery had an end_session_endpoint and we have an id_token, do RP-initiated logout.
	if endSessionEndpoint != "" && strings.TrimSpace(rawID) != "" {
		u, _ := url.Parse(endSessionEndpoint)
		q := u.Query()
		q.Set("id_token_hint", rawID)
		if cfg.PostLogoutRedirectURL != "" {
			q.Set("post_logout_redirect_uri", cfg.PostLogoutRedirectURL)
		}
		// Some IdPs expect client_id too
		if cfg.ClientID != "" {
			q.Set("client_id", cfg.ClientID)
		}
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusSeeOther) // 303
		return
	}

	// Fallback: JSON callers get 204, otherwise go home
	if r.Header.Get("Accept") == "application/json" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
