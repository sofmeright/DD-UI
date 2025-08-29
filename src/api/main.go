package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
)

type Health struct {
	Status  string `json:"status"`
	Edition string `json:"edition"`
}

func main() {
	addr := env("BIND_ADDR", ":8080")
	uiDir := env("UI_DIST", "./ui/dist")

	r := chi.NewRouter()

	r.Get("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, Health{
			Status:  "ok",
			Edition: env("DDUI_EDITION", "Community"),
		})
	})

	fs := http.FileServer(http.Dir(uiDir))
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(uiDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(uiDir, "index.html"))
	}))

	s := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("DDUI API up on %s (serving UI from %s)", addr, uiDir)
	log.Fatal(s.ListenAndServe())
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}