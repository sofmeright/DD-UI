package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

func main() {
	addr := os.Getenv("DDUI_BIND")
	if addr == "" { addr = ":8080" }

	if err := InitAuthFromEnv(); err != nil {
		log.Fatalf("OIDC setup failed: %v", err)
	}
	log.Printf("DDUI API on %s (ui=/home/ddui/ui/dist)", addr)

	if err := http.ListenAndServe(addr, makeRouter()); err != nil {
		log.Fatal(err)
	}
	
	if err := InitAuthFromEnv(); err != nil {
		log.Fatalf("auth init: %v", err)
	}

	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"edition": "Community",
			"now":     time.Now().UTC(),
		})
	})

	// Auth routes
	r.Get("/auth/login", LoginHandler)
	r.Get("/auth/callback", CallbackHandler)
	r.Post("/auth/logout", LogoutHandler)

	// Session probe for the UI
	r.Get("/api/session", RequireAuth(http.HandlerFunc(SessionHandler)).ServeHTTP)

	// Example protected API route shell (no business logic yet)
	r.Get("/api/me", RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := CurrentUser(r.Context())
		writeJSON(w, http.StatusOK, u)
	})).ServeHTTP)

	// Static SPA (built UI)
	uiDir := env("DDUI_UI_DIR", "/home/ddui/ui/dist")
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/auth") {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(uiDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		http.ServeFile(w, r, filepath.Join(uiDir, "index.html"))
	})

	addr := env("DDUI_LISTEN", ":8080")
	log.Printf("DDUI API on %s (ui=%s)", addr, uiDir)
	log.Fatal(http.ListenAndServe(addr, r))
}