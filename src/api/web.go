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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
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
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Authorization", "X-Confirm-Reveal"},
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

			/* ---------- Runtime / Inventory ---------- */

			// Hosts listing with filters
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

			// Inspect a single container by host (for the stack compare page)
			priv.Get("/hosts/{name}/containers/{ctr}/inspect", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				ctr := chi.URLParam(r, "ctr")
				out, err := inspectContainerByHost(r.Context(), host, ctr)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, out)
			})

			// Container quick actions: play/stop/kill/restart/pause/unpause/remove
			priv.Post("/hosts/{name}/containers/{ctr}/action", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				ctr := chi.URLParam(r, "ctr")
				var body struct {
					Action  string `json:"action"`
					Timeout string `json:"timeout,omitempty"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if strings.TrimSpace(body.Action) == "" {
					http.Error(w, "missing action", http.StatusBadRequest)
					return
				}
				if err := performContainerAction(r.Context(), host, ctr, strings.ToLower(body.Action)); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			})

			// Logs (simple, non-follow; UI may show as text)
			priv.Get("/hosts/{name}/containers/{ctr}/logs", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				ctr := chi.URLParam(r, "ctr")
				h, err := GetHostByName(r.Context(), host)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				cli, err := dockerClientForHost(h)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				defer cli.Close()

				tail := strings.TrimSpace(r.URL.Query().Get("tail"))
				opts := container.LogsOptions{
					ShowStdout: true,
					ShowStderr: true,
					Timestamps: false,
					Follow:     false,
					Details:    false,
				}
				if tail != "" {
					opts.Tail = tail
				}
				rc, err := cli.ContainerLogs(r.Context(), ctr, opts)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				defer rc.Close()

				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				_, _ = io.Copy(w, rc)
			})

			// Stats (one-shot JSON passthrough)
			priv.Get("/hosts/{name}/containers/{ctr}/stats", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				ctr := chi.URLParam(r, "ctr")
				txt, err := oneShotStats(r.Context(), host, ctr)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				_, _ = w.Write([]byte(txt))
			})

			// Images
			priv.Get("/hosts/{name}/images", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				h, err := GetHostByName(r.Context(), host)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				cli, err := dockerClientForHost(h)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				defer cli.Close()

				list, err := cli.ImageList(r.Context(), image.ListOptions{All: true})
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				type row struct {
					Repo    string `json:"repo"`
					Tag     string `json:"tag"`
					ID      string `json:"id"`
					Size    string `json:"size"`
					Created string `json:"created"`
				}
				var items []row
				for _, im := range list {
					repo := "<none>"
					tag := "<none>"
					if len(im.RepoTags) > 0 {
						parts := strings.SplitN(im.RepoTags[0], ":", 2)
						repo = parts[0]
						if len(parts) == 2 {
							tag = parts[1]
						}
					}
					items = append(items, row{
						Repo:    repo,
						Tag:     tag,
						ID:      im.ID,
						Size:    humanSize(im.Size),
						Created: time.Unix(im.Created, 0).Format(time.RFC3339),
					})
				}
				writeJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			// Networks
			priv.Get("/hosts/{name}/networks", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				h, err := GetHostByName(r.Context(), host)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				cli, err := dockerClientForHost(h)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				defer cli.Close()

				nets, err := cli.NetworkList(r.Context(), types.NetworkListOptions{Filters: filters.NewArgs()})
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				type row struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Driver string `json:"driver"`
					Scope  string `json:"scope"`
				}
				var items []row
				for _, n := range nets {
					items = append(items, row{ID: n.ID, Name: n.Name, Driver: n.Driver, Scope: n.Scope})
				}
				writeJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			// Volumes
			priv.Get("/hosts/{name}/volumes", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				h, err := GetHostByName(r.Context(), host)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				cli, err := dockerClientForHost(h)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				defer cli.Close()

				vl, err := cli.VolumeList(r.Context(), volume.ListOptions{Filters: filters.NewArgs()})
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				type row struct {
					Name       string `json:"name"`
					Driver     string `json:"driver"`
					Mountpoint string `json:"mountpoint"`
					Created    string `json:"created"`
				}
				var items []row
				for _, v := range vl.Volumes {
					created := v.CreatedAt
					items = append(items, row{v.Name, v.Driver, v.Mountpoint, created})
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

			/* ---------- IaC ---------- */

			// Force IaC scan (local)
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

			// Desired stacks/services for a host (host + groups)
			priv.Get("/hosts/{name}/iac", func(w http.ResponseWriter, r *http.Request) {
				name := chi.URLParam(r, "name")
				items, err := listIacStacksForHost(r.Context(), name)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"stacks": items})
			})

			// Create a new stack in the local IaC repo
			// POST /api/iac/stacks  { scope_kind:"host"|"group", scope_name:"hostA", stack_name:"myapp" }
			priv.Post("/iac/stacks", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					ScopeKind string `json:"scope_kind"`
					ScopeName string `json:"scope_name"`
					StackName string `json:"stack_name"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if body.ScopeKind == "" || body.ScopeName == "" || body.StackName == "" {
					http.Error(w, "scope_kind, scope_name, stack_name required", http.StatusBadRequest)
					return
				}

				root := strings.TrimSpace(env(iacDefaultRootEnv, iacDefaultRoot))
				dirname := strings.TrimSpace(env(iacDirNameEnv, iacDefaultDirName))
				rel := filepath.ToSlash(filepath.Join(dirname, body.ScopeName, body.StackName))
				full, err := joinUnder(root, rel)
				if err != nil {
					http.Error(w, "invalid path", http.StatusBadRequest)
					return
				}
				if err := os.MkdirAll(full, 0o755); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				repoID, err := upsertIacRepoLocal(r.Context(), root)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				id, err := upsertIacStack(r.Context(), repoID, body.ScopeKind, body.ScopeName, body.StackName, rel, "", "unmanaged", "", "none", true)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"id": id, "rel_path": rel})
			})

			// Delete a stack (optionally delete files too)
			// DELETE /api/iac/stacks/{id}?delete_files=1
			priv.Delete("/iac/stacks/{id}", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, _ := strconv.ParseInt(idStr, 10, 64)
				deleteFiles := r.URL.Query().Get("delete_files") == "1" || r.URL.Query().Get("delete_files") == "true"

				root, err := getRepoRootForStack(r.Context(), id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				var rel string
				_ = db.QueryRow(r.Context(), `SELECT rel_path FROM iac_stacks WHERE id=$1`, id).Scan(&rel)

				if _, err := db.Exec(r.Context(), `DELETE FROM iac_stacks WHERE id=$1`, id); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if deleteFiles && rel != "" {
					if full, err := joinUnder(root, rel); err == nil {
						_ = os.RemoveAll(full) // best effort
					}
				}
				writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
			})

			// ===== IaC Editor APIs =====

			// List files tracked for a stack
			priv.Get("/iac/stacks/{id}/files", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, _ := strconv.ParseInt(idStr, 10, 64)
				items, err := listFilesForStack(r.Context(), id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"files": items})
			})

			// Read file content for a stack file
			//   GET /api/iac/stacks/{id}/file?path=rel/path&decrypt=1
			priv.Get("/iac/stacks/{id}/file", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, _ := strconv.ParseInt(idStr, 10, 64)
				rel := strings.TrimSpace(r.URL.Query().Get("path"))
				if rel == "" {
					http.Error(w, "missing path", http.StatusBadRequest)
					return
				}
				root, err := getRepoRootForStack(r.Context(), id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				full, err := joinUnder(root, rel)
				if err != nil {
					http.Error(w, "invalid path", http.StatusBadRequest)
					return
				}
				decrypt := r.URL.Query().Get("decrypt") == "1" || r.URL.Query().Get("decrypt") == "true"

				var data []byte
				if decrypt {
					// require explicit confirmation + server-side allow
					if strings.ToLower(os.Getenv("DDUI_ALLOW_SOPS_DECRYPT")) != "1" {
						http.Error(w, "decrypt disabled on server", http.StatusForbidden)
						return
					}
					if strings.ToLower(r.Header.Get("X-Confirm-Reveal")) != "yes" {
						http.Error(w, "confirmation required", http.StatusForbidden)
						return
					}
					// run `sops -d`
					ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
					defer cancel()
					cmd := exec.CommandContext(ctx, "sops", "-d", full)
					out, err := cmd.CombinedOutput()
					if err != nil {
						http.Error(w, "sops decrypt failed: "+string(out), http.StatusNotImplemented)
						return
					}
					data = out
				} else {
					b, err := os.ReadFile(full)
					if err != nil {
						http.Error(w, err.Error(), http.StatusNotFound)
						return
					}
					data = b
				}

				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				_, _ = w.Write(data)
			})

			// Create/update file content
			//   POST /api/iac/stacks/{id}/file  { "path": "docker-compose/host/stack/x.env", "content":"..." }
			priv.Post("/iac/stacks/{id}/file", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, _ := strconv.ParseInt(idStr, 10, 64)
				var body struct {
					Path    string `json:"path"`
					Content string `json:"content"`
					Sops    bool   `json:"sops,omitempty"`
					Role    string `json:"role,omitempty"` // compose|env|script|other
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if strings.TrimSpace(body.Path) == "" {
					http.Error(w, "path required", http.StatusBadRequest)
					return
				}
				root, err := getRepoRootForStack(r.Context(), id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				full, err := joinUnder(root, body.Path)
				if err != nil {
					http.Error(w, "invalid path", http.StatusBadRequest)
					return
				}
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if err := os.WriteFile(full, []byte(body.Content), 0o644); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				// auto-role
				if body.Role == "" {
					low := strings.ToLower(full)
					switch {
					case strings.HasSuffix(low, ".yml"), strings.HasSuffix(low, ".yaml"):
						body.Role = "compose"
					case strings.HasSuffix(low, ".sh"):
						body.Role = "script"
					case strings.Contains(low, ".env") || strings.HasSuffix(low, ".env"):
						body.Role = "env"
					default:
						body.Role = "other"
					}
				}

				// SOPS auto-encrypt if requested or filename matches
				shouldSops := body.Sops || strings.HasSuffix(strings.ToLower(body.Path), "_private.env") || strings.HasSuffix(strings.ToLower(body.Path), "_secret.env")
				if shouldSops && (os.Getenv("SOPS_AGE_KEY") != "" || os.Getenv("SOPS_AGE_KEY_FILE") != "") {
					args := []string{"-e", "-i"}
					// hint for dotenv
					if strings.HasSuffix(strings.ToLower(body.Path), ".env") {
						args = []string{"-e", "-i", "--input-type", "dotenv"}
					}
					args = append(args, full)
					ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
					defer cancel()
					out, err := exec.CommandContext(ctx, "sops", args...).CombinedOutput()
					if err != nil {
						log.Printf("sops: encrypt failed: %v out=%s", err, string(out))
					} else {
						log.Printf("sops: encrypted %s", full)
						body.Sops = true
					}
				}

				sum, sz := sha256File(full)
				relFromRoot := filepath.ToSlash(strings.TrimPrefix(full, strings.TrimSuffix(root, "/")+"/"))
				if err := upsertIacFile(r.Context(), id, body.Role, relFromRoot, body.Sops, sum, sz); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"status": "saved", "size": sz, "sha256": sum, "sops": body.Sops})
			})

			// Delete a file from a stack
			//   DELETE /api/iac/stacks/{id}/file?path=rel/path
			priv.Delete("/iac/stacks/{id}/file", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, _ := strconv.ParseInt(idStr, 10, 64)
				rel := strings.TrimSpace(r.URL.Query().Get("path"))
				if rel == "" {
					http.Error(w, "missing path", http.StatusBadRequest)
					return
				}
				root, err := getRepoRootForStack(r.Context(), id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				full, err := joinUnder(root, rel)
				if err != nil {
					http.Error(w, "invalid path", http.StatusBadRequest)
					return
				}
				_ = os.Remove(full) // best effort
				_ = deleteIacFileRow(r.Context(), id, rel)
				writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
			})

			// Deploy endpoint (MVP placeholder – returns 202)
			priv.Post("/iac/stacks/{id}/deploy", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				log.Printf("deploy: requested stack id=%s", idStr)
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{"status":"queued"}`))
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

// safe path join under repo root
func joinUnder(root, rel string) (string, error) {
	clean := filepath.Clean("/" + rel) // force absolute-clean then strip
	clean = strings.TrimPrefix(clean, "/")
	full := filepath.Join(root, clean)
	r, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(r, "..") {
		return "", errors.New("outside root")
	}
	return full, nil
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatInt(b, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strings.TrimSuffix(strings.TrimSpace(
		strconv.FormatFloat(float64(b)/float64(div), 'f', 1, 64)), ".0") + " " + string("KMGTPE"[exp]) + "B"
}
