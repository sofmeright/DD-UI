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

type Health struct {
	Status    string    `json:"status"`
	StartedAt time.Time `json:"startedAt"`
	Edition   string    `json:"edition"`
}

var startedAt = time.Now()

func makeRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(cors.AllowAll().Handler)

	// --- Public API
	r.Route("/api", func(api chi.Router) {
		api.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, Health{Status: "ok", StartedAt: startedAt, Edition: "Community"})
		})
		api.Get("/session", SessionHandler) // no auth; returns 200 with user or empty user
	})

	// --- Auth endpoints (server-handled, not SPA)
	r.Get("/login", LoginHandler)
	r.Get("/auth/callback", CallbackHandler)
	r.Post("/logout", LogoutHandler)

	// --- Static SPA (Vite build)
	uiRoot := "/home/ddui/ui/dist"
	fs := http.FileServer(http.Dir(uiRoot))

	// serve built assets directly
	r.Get("/assets/*", func(w http.ResponseWriter, req *http.Request) {
		fs.ServeHTTP(w, req)
	})

	// SPA fallback
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