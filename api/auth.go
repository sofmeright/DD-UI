// src/api/auth.go
package main

import (
	"context"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"dd-ui/common"
	"dd-ui/middleware"
	"github.com/alexedwards/scs/v2"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func init() {
	gob.Register(middleware.User{}) // ensure scs can (de)serialize User
	gob.Register(map[string]interface{}{}) // for storing oauth temp data
}

var (
	oidcProv           *oidc.Provider
	oidcVerifier       *oidc.IDTokenVerifier
	oauthCfg           *oauth2.Config
	sessionManager     *scs.SessionManager
	cfg                AuthConfig
	endSessionEndpoint string // discovered from .well-known
)

// ---- server-side id_token store (per-session) ----

type idTokenEntry struct {
	token string
	exp   time.Time
}
type idTokenStore struct {
	mu sync.RWMutex
	m  map[string]idTokenEntry // sid -> entry
}

func (s *idTokenStore) put(sid, token string, exp time.Time) {
	if sid == "" || token == "" {
		return
	}
	s.mu.Lock()
	if s.m == nil {
		s.m = make(map[string]idTokenEntry)
	}
	s.m[sid] = idTokenEntry{token: token, exp: exp}
	s.mu.Unlock()
}
func (s *idTokenStore) pop(sid string) string {
	if sid == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ent, ok := s.m[sid]
	if ok {
		delete(s.m, sid)
		if time.Now().Before(ent.exp) {
			return ent.token
		}
	}
	return ""
}
func (s *idTokenStore) sweep() {
	now := time.Now()
	s.mu.Lock()
	for k, v := range s.m {
		if now.After(v.exp) {
			delete(s.m, k)
		}
	}
	s.mu.Unlock()
}

var idtStore idTokenStore

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

func InitAuthFromEnv() (*scs.SessionManager, error) {
	var err error

	// ---- OIDC client credentials (allow *_FILE and "@/path")
	clientID, err := envOrFile("OIDC_CLIENT_ID", "OIDC_CLIENT_ID_FILE")
	if err != nil {
		return nil, err
	}
	clientSecret, err := envOrFile("OIDC_CLIENT_SECRET", "OIDC_CLIENT_SECRET_FILE")
	if err != nil {
		return nil, err
	}

	// ---- Session secret (renamed: DDUI_SESSION_SECRET / DDUI_SESSION_SECRET_FILE)
	// Compatibility with old SESSION_SECRET is intentionally dropped.
	sec, err := envOrFile("DDUI_SESSION_SECRET", "DDUI_SESSION_SECRET_FILE")
	if err != nil {
		return nil, err
	}
	if sec == "" {
		sec = randHex(64) // generate one if not provided
	}

	redirect := env("OIDC_REDIRECT_URL", "")

	// Derive SecureCookies if COOKIE_SECURE is unset.
	secureStr := strings.TrimSpace(env("DDUI_COOKIE_SECURE", ""))
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
		CookieDomain:          env("DDUI_COOKIE_DOMAIN", ""),
		PostLogoutRedirectURL: env("OIDC_POST_LOGOUT_REDIRECT_URL", ""),
	}

	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return nil, errors.New("OIDC_ISSUER_URL, OIDC_CLIENT_ID{/_FILE}, OIDC_CLIENT_SECRET{/_FILE}, OIDC_REDIRECT_URL are required")
	}

	// ---- OIDC wiring
	ctx := context.Background()
	oidcProv, err = oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, err
	}

	// Try to discover end_session_endpoint (not all providers expose it)
	var disc struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := oidcProv.Claims(&disc); err == nil {
		endSessionEndpoint = strings.TrimSpace(disc.EndSessionEndpoint)
	}
	if endSessionEndpoint == "" {
		infoLog("auth: no end_session_endpoint found in discovery; RP-logout will fall back to local clear")
	}

	oidcVerifier = oidcProv.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	oauthCfg = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     oidcProv.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
	}

	// ---- Session manager setup
	sessionManager = scs.New()
	sessionManager.Lifetime = time.Duration(cookieMaxAge) * time.Second
	sessionManager.Cookie.Name = sessionName
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.Secure = cfg.SecureCookies
	sessionManager.Cookie.Path = "/"
	sessionManager.Cookie.Domain = cfg.CookieDomain
	if cfg.SecureCookies {
		sessionManager.Cookie.SameSite = http.SameSiteNoneMode
	} else {
		sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	}
	
	// Also initialize the global SessionManager in common package so handlers can use it
	common.SessionManager = sessionManager

	// start background sweeper for server-side id_tokens
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			idtStore.sweep()
		}
	}()

	return sessionManager, nil
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

	// Store OAuth flow data in session (will auto-expire)
	oauthData := map[string]interface{}{
		"state": state,
		"nonce": nonce,
	}
	sessionManager.Put(r.Context(), "oauth_temp", oauthData)

	// Build OP authorize URL with nonce
	authURL := oauthCfg.AuthCodeURL(state, oidc.Nonce(nonce))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func CallbackHandler(w http.ResponseWriter, r *http.Request) {
	// Read and remove the temporary oauth data
	oauthData, _ := sessionManager.Pop(r.Context(), "oauth_temp").(map[string]interface{})
	wantState, _ := oauthData["state"].(string)
	nonce, _ := oauthData["nonce"].(string)

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

	// --- raw id_token for verification + server-side storage only ---
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

	u := middleware.User{
		Sub:   claims.Sub,
		Email: strings.ToLower(claims.Email),
		Name:  claims.Name,
		Pic:   claims.Pic,
	}

	// Save minimal session + sid; store id_token server-side keyed by sid
	sid := sessionManager.GetString(r.Context(), "sid")
	if strings.TrimSpace(sid) == "" {
		sid = randHex(32)
		sessionManager.Put(r.Context(), "sid", sid)
	}
	sessionManager.Put(r.Context(), "user", u)
	sessionManager.Put(r.Context(), "exp", time.Now().Add(7 * 24 * time.Hour).Unix())

	// expiry = min(session 7d, token exp if present)
	exp := time.Now().Add(7 * 24 * time.Hour)
	if claims.Exp > 0 {
		if te := time.Unix(claims.Exp, 0); te.Before(exp) {
			exp = te
		}
	}
	idtStore.put(sid, rawID, exp)

	infoLog("auth: login ok sub=%s email=%s", u.Sub, u.Email)

	http.Redirect(w, r, "/", http.StatusFound)
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// retrieve sid/id_token for RP-initiated logout BEFORE clearing
	sid := sessionManager.GetString(r.Context(), "sid")
	rawID := idtStore.pop(sid) // empty if absent/expired

	// destroy the session
	err := sessionManager.Destroy(r.Context())
	if err != nil {
		errorLog("auth: failed to destroy session: %v", err)
	}

	// OAuth temp data is automatically cleaned up with session destruction

	// If discovery had an end_session_endpoint and we have an id_token, do RP-initiated logout.
	if endSessionEndpoint != "" && strings.TrimSpace(rawID) != "" {
		u, _ := url.Parse(endSessionEndpoint)
		q := u.Query()
		q.Set("id_token_hint", rawID)
		if cfg.PostLogoutRedirectURL != "" {
			q.Set("post_logout_redirect_uri", cfg.PostLogoutRedirectURL)
		}
		// Some IdPs (and specs) allow/expect client_id too — harmless if ignored
		if cfg.ClientID != "" {
			q.Set("client_id", cfg.ClientID)
		}
		u.RawQuery = q.Encode()
		infoLog("auth: rp-logout redirecting to OP end_session_endpoint")
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
	u, ok := sessionManager.Get(r.Context(), "user").(middleware.User)
	exp := sessionManager.GetInt64(r.Context(), "exp")

	if !ok || exp == 0 || time.Now().Unix() > exp {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

// --- auth helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false) // Don't escape HTML characters like & in YAML anchors
	_ = encoder.Encode(v)
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
