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

func makeRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(cors.AllowAll().Handler)

	// API
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, Health{
			Status:    "ok",
			StartedAt: startedAt,
			Edition:   "Community",
		})
	})

	// Static UI (SPA)
	uiRoot := "/home/ddui/ui/dist"
	fs := http.FileServer(http.Dir(uiRoot))

	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		// Serve actual file if it exists
		path := filepath.Join(uiRoot, strings.TrimPrefix(req.URL.Path, "/"))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, req)
			return
		}
		// SPA fallback
		http.ServeFile(w, req, filepath.Join(uiRoot, "index.html"))
	})

	return r
}

func respondJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
