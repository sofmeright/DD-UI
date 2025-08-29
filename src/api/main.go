package main

import (
	"context"
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

type ScanRequest struct {
	Root   string `json:"root"`
	Filter string `json:"filter"`
}

type Stack struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Path        string `json:"path"`
	Status      string `json:"status"`
	Containers  int    `json:"containers"`
	Owner       string `json:"owner"`
	CreatedAt   string `json:"createdAt"`
	ModifiedAt  string `json:"modifiedAt"`
	SOPS        bool   `json:"sops"`
	PullPolicy  string `json:"pullPolicy"`
}

type Host struct {
	Host     string  `json:"host"`
	Groups   []string`json:"groups"`
	LastSync string  `json:"lastSync"`
	Stacks   []Stack `json:"stacks"`
}

type Manifest struct {
	Hosts []Host `json:"hosts"`
}

func main() {
	addr := getenv("BIND_ADDR", ":8080")
	uiDir := getenv("UI_DIST", "./ui/dist")

	r := chi.NewRouter()

	r.Get("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, Health{Status: "ok", Edition: getenv("DDUI_EDITION", "Community")})
	})

	r.Post("/api/scan", func(w http.ResponseWriter, r *http.Request) {
		var req ScanRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		root := req.Root
		if root == "" { root = getenv("DDUI_SCAN_ROOT", "/opt/docker/ant-parade/docker-compose") }

		_ = root // replace with real scan; stubbed for now:
		now := time.Now().Format(time.RFC3339)
		writeJSON(w, Manifest{
			Hosts: []Host{
				{
					Host:     "anchorage",
					Groups:   []string{"docker","clustered"},
					LastSync: now,
					Stacks: []Stack{
						{Name:"grafana",Type:"compose",Path:filepath.Join(root,"anchorage/grafana"),Status:"drift",Containers:1,Owner:"ops",CreatedAt:now,ModifiedAt:now,SOPS:true,PullPolicy:"always"},
					},
				},
			},
		})
	})

	r.Get("/api/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok { http.Error(w, "stream unsupported", http.StatusInternalServerError); return }

		ctx := r.Context()
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			case ts := <-t.C:
				i++
				_, _ = w.Write([]byte("event: log\n"))
				_, _ = w.Write([]byte("data: " + `{"line":` + jsonInt(i) + `,"msg":"tick","ts":"` + ts.Format(time.RFC3339) + `"}` + "\n\n"))
				flusher.Flush()
			}
		}
	})

	fs := http.FileServer(http.Dir(uiDir))
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(uiDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r); return
		}
		http.ServeFile(w, r, filepath.Join(uiDir, "index.html"))
	}))

	s := &http.Server{ Addr: addr, Handler: r }
	log.Printf("DDUI listening on %s (UI %s)", addr, uiDir)
	log.Fatal(s.ListenAndServe())
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" { return v }
	return def
}

func jsonInt(i int) string { return json.Number(int64(i)).String() }