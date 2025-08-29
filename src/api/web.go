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

	h := func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, Health{
			Status:    "ok",
			StartedAt: startedAt,
			Edition:   "Community",
		})
	}
	r.Get("/healthz", h)
	r.Get("/api/healthz", h)

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