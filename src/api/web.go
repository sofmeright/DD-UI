// src/api/web.go
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

var startedAt = time.Now().UTC()

type Health struct {
	Status    string    `json:"status"`
	StartedAt time.Time `json:"startedAt"`
	Edition   string    `json:"edition"`
}

func makeRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(cors.AllowAll().Handler)

	// health (both paths for compatibility)
	h := func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, Health{Status: "ok", StartedAt: startedAt, Edition: "Community"})
	}
	r.Get("/healthz", h)
	r.Get("/api/healthz", h)

	// auth endpoints
	r.Get("/login", LoginHandler)
	r.Get("/auth/callback", CallbackHandler) // OIDC_REDIRECT_URL must point to this path
	r.Post("/logout", LogoutHandler)

	// session: 401 if not signed in, user JSON if signed in
	r.With(RequireAuth).Get("/api/session", SessionHandler)

	// SPA
	uiRoot := "/home/ddui/ui/dist"
	fs := http.FileServer(http.Dir(uiRoot))

	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		path := filepath.Join(uiRoot, strings.TrimPrefix(req.URL.Path, "/"))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, req)
			return
		}
		http.ServeFile(w, req, filepath.Join(uiRoot, "index.html"))
	})

	return r
}

func respondJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
