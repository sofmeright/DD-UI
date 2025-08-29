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
	oidcProv     *oidc.Provider
	oidcVerifier *oidc.IDTokenVerifier
	oauthCfg     *oauth2.Config
	store        *sessions.CookieStore
	cfg          AuthConfig
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
}

const (
	sessionName  = "ddui_sess"
	oauthTmpName = "ddui_oauth"
	cookieMaxAge = 7 * 24 * 3600
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
	}
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return errors.New("OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET, OIDC_REDIRECT_URL are required")
	}

	ctx := context.Background()
	oidcProv, err = oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return err
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

func scopes(s string) []string {
	var out []string
	for _, p := range strings.Fields(s) {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func randHex(n int) string {
	b := make([]byte, n/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	state := randHex(32)
	nonce := randHex(32)

	tmp, _ := store.Get(r, oauthTmpName)
	tmp.Values["state"] = state
	tmp.Values["nonce"] = nonce
	tmp.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Domain:   cfg.CookieDomain,
	}
	_ = tmp.Save(r, w)

	url := oauthCfg.AuthCodeURL(state, oidc.Nonce(nonce))
	http.Redirect(w, r, url, http.StatusFound)
}

func CallbackHandler(w http.ResponseWriter, r *http.Request) {
	// expire the temporary oauth cookie
	tmp.Options.MaxAge = -1
	_ = tmp.Save(r, w)
	tmp.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Domain:   cfg.CookieDomain,
	}
	_ = tmp.Save(r, w)

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

	sess, _ := store.Get(r, sessionName)
	sess.Values["user"] = u
	sess.Values["exp"] = time.Now().Add(7 * 24 * time.Hour).Unix()
	_ = sess.Save(r, w)

	log.Printf("auth: login ok sub=%s email=%s", u.Sub, u.Email)

	http.Redirect(w, r, "/", http.StatusFound)
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := store.Get(r, sessionName)
	sess.Options.MaxAge = -1
	_ = sess.Save(r, w)
	w.WriteHeader(http.StatusNoContent)
}

func SessionHandler(w http.ResponseWriter, r *http.Request) {
    sess, _ := store.Get(r, sessionName)

    // try to decode user and expiry
    u, ok := sess.Values["user"].(User)
    exp, _ := sess.Values["exp"].(int64)

    if !ok || exp == 0 || time.Now().Unix() > exp {
        // signed out
        writeJSON(w, http.StatusOK, map[string]any{
            "user": nil,
        })
        return
    }

    writeJSON(w, http.StatusOK, map[string]any{
        "user": u,
    })
}

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
