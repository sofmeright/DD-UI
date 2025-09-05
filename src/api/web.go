// src/api/web.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	// CORS – permissive for now; tighten later
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// -------- API
	r.Route("/api", func(api chi.Router) {
		api.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, Health{Status: "ok", StartedAt: startedAt, Edition: "Community"})
		})

		// Session probe MUST be public
		api.Get("/session", SessionHandler)

		// Everything below requires auth
		api.Group(func(priv chi.Router) {
			priv.Use(RequireAuth)

			// List hosts with optional filters:
			//   /api/hosts?owner=alice&q=anch&limit=50&offset=0
			priv.Get("/hosts", func(w http.ResponseWriter, r *http.Request) {
				items, err := ListHosts(r.Context())
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				owner := strings.TrimSpace(r.URL.Query().Get("owner"))
				q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
				limit := clamp(parseIntDefault(r.URL.Query().Get("limit"), 200), 1, 1000)
				offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

				// in-process filter/paginate (DB-level can come later)
				filtered := make([]HostRow, 0, len(items))
				for _, h := range items {
					if owner != "" && !strings.EqualFold(h.Owner, owner) {
						continue
					}
					if q != "" {
						if !strings.Contains(strings.ToLower(h.Name), q) &&
							!strings.Contains(strings.ToLower(h.Addr), q) {
							continue
						}
					}
					filtered = append(filtered, h)
				}
				lo := offset
				if lo > len(filtered) {
					lo = len(filtered)
				}
				hi := lo + limit
				if hi > len(filtered) {
					hi = len(filtered)
				}
				page := filtered[lo:hi]

				writeJSON(w, http.StatusOK, map[string]any{
					"items":  page,
					"total":  len(filtered),
					"limit":  limit,
					"offset": offset,
				})
			})

			// List containers by host
			priv.Get("/hosts/{name}/containers", func(w http.ResponseWriter, r *http.Request) {
				name := chi.URLParam(r, "name")
				items, err := listContainersByHost(r.Context(), name)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			// Trigger on-demand scan for a single host
			priv.Post("/scan/host/{name}", func(w http.ResponseWriter, r *http.Request) {
				name := chi.URLParam(r, "name")
				to := parseDurationDefault(r.URL.Query().Get("timeout"), 45*time.Second)
				ctx, cancel := context.WithTimeout(r.Context(), to)
				defer cancel()

				n, err := ScanHostContainers(ctx, name)
				if err != nil {
					if errors.Is(err, ErrSkipScan) {
						writeJSON(w, http.StatusOK, map[string]any{
							"host":   name,
							"saved":  0,
							"status": "skipped",
						})
						return
					}
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"host":   name,
					"saved":  n,
					"status": "ok",
				})
			})

			// Trigger scan for all known hosts (sequential, simple summary) + IaC scan (non-fatal)
			//   POST /api/scan/all?timeout=30s
			priv.Post("/scan/all", func(w http.ResponseWriter, r *http.Request) {
				// Always try IaC scan as part of Sync; log-only on error
				if _, _, err := ScanIacLocal(r.Context()); err != nil {
					log.Printf("iac: sync scan failed: %v", err)
				}

				hostRows, err := ListHosts(r.Context())
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				perHostTO := parseDurationDefault(r.URL.Query().Get("timeout"), 30*time.Second)

				type result struct {
					Host    string `json:"host"`
					Saved   int    `json:"saved,omitempty"`
					Err     string `json:"err,omitempty"`
					Skipped bool   `json:"skipped,omitempty"`
					Reason  string `json:"reason,omitempty"`
				}

				var (
					results []result
					total   int
					scanned int
					skipped int
					failed  int
				)

				for _, h := range hostRows {
					// Pre-filter: if this host would use a local unix sock but isn't the designated local host, skip it.
					url, _ := dockerURLFor(h)
					if isUnixSock(url) && !localHostAllowed(h) {
						results = append(results, result{
							Host:    h.Name,
							Skipped: true,
							Reason:  "local docker.sock only allowed for the designated local host",
						})
						skipped++
						continue
					}

					ctx, cancel := context.WithTimeout(r.Context(), perHostTO)
					n, err := ScanHostContainers(ctx, h.Name)
					cancel()

					if err != nil {
						if errors.Is(err, ErrSkipScan) {
							results = append(results, result{Host: h.Name, Skipped: true})
							skipped++
							continue
						}
						results = append(results, result{Host: h.Name, Err: err.Error()})
						failed++
						continue
					}

					total += n
					scanned++
					results = append(results, result{Host: h.Name, Saved: n})
				}

				writeJSON(w, http.StatusOK, map[string]any{
					"hosts_total": len(hostRows),
					"scanned":     scanned,
					"skipped":     skipped,
					"errors":      failed,
					"saved":       total,
					"results":     results,
					"status":      "ok",
				})
			})

			// POST /api/inventory/reload  (optional body: {"path":"/new/path"})
			priv.Post("/inventory/reload", func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Path string `json:"path"` }
				_ = json.NewDecoder(r.Body).Decode(&body)

				var err error
				if strings.TrimSpace(body.Path) != "" {
					err = ReloadInventoryWithPath(body.Path)
				} else {
					err = ReloadInventory()
				}
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"status": "reloaded"})
			})

			// IaC: force scan (local)
			priv.Post("/iac/scan", func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
				defer cancel()
				stacks, services, err := ScanIacLocal(ctx)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"status":   "ok",
					"stacks":   stacks,
					"services": services,
				})
			})

			// IaC: fetch desired stacks/services for a host (host + groups)
			priv.Get("/hosts/{name}/iac", func(w http.ResponseWriter, r *http.Request) {
				name := chi.URLParam(r, "name")
				items, err := listIacStacksForHost(r.Context(), name)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"stacks": items})
			})

			// List all IaC files recorded for a stack
			priv.Get("/iac/stacks/{id}/files", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, _ := strconv.ParseInt(idStr, 10, 64)
				rows, err := db.Query(r.Context(), `
				SELECT role, rel_path, sops, sha256_hex, size_bytes, updated_at
				FROM iac_stack_files
				WHERE stack_id=$1
				ORDER BY role, rel_path
				`, id)
				if err != nil { http.Error(w, err.Error(), 500); return }
				defer rows.Close()
				type fileRow struct {
					Role string `json:"role"`
					Rel  string `json:"rel_path"`
					Sops bool   `json:"sops"`
					Sha  string `json:"sha256_hex"`
					Size int64  `json:"size_bytes"`
					UpdatedAt time.Time `json:"updated_at"`
				}
				var out []fileRow
				for rows.Next() {
					var fr fileRow
					if err := rows.Scan(&fr.Role, &fr.Rel, &fr.Sops, &fr.Sha, &fr.Size, &fr.UpdatedAt); err != nil { http.Error(w, err.Error(), 500); return }
					out = append(out, fr)
				}
				writeJSON(w, 200, map[string]any{"files": out})
			})

			// Read a file (optionally decrypt SOPS)
			priv.Get("/iac/stacks/{id}/file", func(w http.ResponseWriter, r *http.Request) {
				id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
				rel := strings.TrimSpace(r.URL.Query().Get("path"))
				decrypt := r.URL.Query().Get("decrypt") == "1"

				sp, err := getStackPaths(r.Context(), id)
				if err != nil { http.Error(w, err.Error(), 400); return }
				abs, err := safeJoin(filepath.Join(sp.Root, sp.Rel), rel)
				if err != nil { http.Error(w, "invalid path", 400); return }

				var b []byte
				if decrypt {
					// gated by env; default allowed=false
					if !strings.EqualFold(env("DDUI_SOPS_DECRYPT_ENABLE", "0"), "1") {
						http.Error(w, "SOPS decrypt disabled by server", 403); return
					}
					b, err = sopsDecryptFile(r.Context(), abs)
				} else {
					b, err = os.ReadFile(abs)
				}
				if err != nil { http.Error(w, err.Error(), 400); return }

				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(200)
				_, _ = w.Write(b)
			})

			// Save a file (edit/create). Disabled unless DDUI_IAC_WRITE=1
			priv.Post("/iac/stacks/{id}/file", func(w http.ResponseWriter, r *http.Request) {
				if !strings.EqualFold(env("DDUI_IAC_WRITE", "0"), "1") {
					http.Error(w, "editing disabled by server", 403); return
				}
				id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
				rel := strings.TrimSpace(r.URL.Query().Get("path"))
				sp, err := getStackPaths(r.Context(), id)
				if err != nil { http.Error(w, err.Error(), 400); return }
				abs, err := safeJoin(filepath.Join(sp.Root, sp.Rel), rel)
				if err != nil { http.Error(w, "invalid path", 400); return }
				b, _ := io.ReadAll(r.Body)

				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil { http.Error(w, err.Error(), 500); return }
				if err := os.WriteFile(abs, b, 0o644); err != nil { http.Error(w, err.Error(), 500); return }

				// refresh file table for this stack (cheap path: update hash)
				sum, sz := sha256File(abs)
				role := detectRoleByName(rel) // compose|env|script|other
				sops := looksSops(b)
				_ = upsertIacFile(r.Context(), id, role, rel, sops, sum, sz)

				writeJSON(w, 200, map[string]any{"status":"saved"})
			})
		})
	})

	// Legacy alias
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, Health{Status: "ok", StartedAt: startedAt, Edition: "Community"})
	})

	// -------- Auth endpoints (must come BEFORE SPA fallback)
	r.Get("/login", LoginHandler)
	r.Get("/auth/login", LoginHandler) // alias
	r.Get("/auth/callback", CallbackHandler)
	r.Post("/logout", LogoutHandler)
	r.Post("/auth/logout", LogoutHandler) // alias

	// -------- Static SPA (Vite)
	uiRoot := env("DDUI_UI_DIR", "/home/ddui/ui/dist")
	fs := http.FileServer(http.Dir(uiRoot))

	// Serve built assets directly
	r.Get("/assets/*", func(w http.ResponseWriter, req *http.Request) {
		fs.ServeHTTP(w, req)
	})

	// SPA fallback (last)
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api") || strings.HasPrefix(req.URL.Path, "/auth") {
			http.NotFound(w, req)
			return
		}
		path := filepath.Join(uiRoot, filepath.Clean(strings.TrimPrefix(req.URL.Path, "/")))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, req, path)
			return
		}
		http.ServeFile(w, req, filepath.Join(uiRoot, "index.html"))
	})

	return r
}

func respondJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	// discourage caching of API responses
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func parseDurationDefault(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

// best-effort role guess
func detectRoleByName(rel string) string {
    name := strings.ToLower(filepath.Base(rel))
    if strings.HasSuffix(name, ".env") || strings.Contains(name, ".env.") { return "env" }
    if strings.Contains(name, "compose") && (strings.HasSuffix(name,".yml") || strings.HasSuffix(name,".yaml")) { return "compose" }
    if strings.HasSuffix(name, ".sh") { return "script" }
    return "other"
}

func sopsDecryptFile(ctx context.Context, abs string) ([]byte, error) {
    // ephemeral decrypt; no persist
    cmd := exec.CommandContext(ctx, "sops", "-d", abs)
    // Optionally set env for age key path (tmpfs)
    // cmd.Env = append(os.Environ(), "SOPS_AGE_KEY_FILE=/dev/shm/age.key")
    out, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("sops decrypt failed: %v: %s", err, string(out))
    }
    return out, nil
}