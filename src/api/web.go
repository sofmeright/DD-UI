// src/api/web.go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
)

type Health struct {
	Status    string    `json:"status"`
	StartedAt time.Time `json:"startedAt"`
	Edition   string    `json:"edition"`
}

func makeRouter() http.Handler {
	r := chi.NewRouter()

	// CORS – locked down for credentials
	uiOrigin := strings.TrimSpace(env("DDUI_UI_ORIGIN", ""))
	allowedOrigins := []string{}
	if uiOrigin != "" {
		allowedOrigins = append(allowedOrigins, uiOrigin)
	}
	// dev helpers
	allowedOrigins = append(allowedOrigins,
		"http://localhost:5173",
		"http://127.0.0.1:5173",
	)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins, // no "*"
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "X-Confirm-Reveal"},
		AllowCredentials: true,
		MaxAge:           600,
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

			/* ---------- GitOps Automation Configuration (Hierarchical) ---------- */

			// GET global auto-deployment setting (environment fallback)
			priv.Get("/gitops/global", func(w http.ResponseWriter, r *http.Request) {
				val, src := getGlobalDevopsApply(r.Context())
				writeJSON(w, http.StatusOK, map[string]any{
					"auto_deploy": val,
					"source":      src, // "db" or "env"
				})
			})

			// PATCH global: { "auto_deploy": true|false } or { "auto_deploy": null } to clear to ENV
			priv.Patch("/gitops/global", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					AutoDeploy *bool `json:"auto_deploy"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if err := setGlobalDevopsApply(r.Context(), body.AutoDeploy); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				val, src := getGlobalDevopsApply(r.Context())
				writeJSON(w, http.StatusOK, map[string]any{"auto_deploy": val, "source": src, "status": "ok"})
			})

			// GET host auto-deployment override + effective setting
			priv.Get("/gitops/hosts/{name}", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				override, _ := getHostDevopsOverride(r.Context(), host)
				global, _ := getAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					global = &d
				}
				effective := *global
				if override != nil {
					effective = *override
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"override":   override,  // null means inherit from global
					"effective":  effective, // actual value used
					"inherits_from": "global",
				})
			})

			// PATCH host auto-deployment: { "auto_deploy": true|false|null }
			priv.Patch("/gitops/hosts/{name}", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				var body struct {
					AutoDeploy *bool `json:"auto_deploy"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if err := setHostDevopsOverride(r.Context(), host, body.AutoDeploy); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				override, _ := getHostDevopsOverride(r.Context(), host)
				global, _ := getAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					global = &d
				}
				effective := *global
				if override != nil {
					effective = *override
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"override": override, 
					"effective": effective, 
					"inherits_from": "global",
					"status": "ok",
				})
			})

			// GET group auto-deployment override + effective setting
			priv.Get("/gitops/groups/{name}", func(w http.ResponseWriter, r *http.Request) {
				group := chi.URLParam(r, "name")
				override, _ := getGroupDevopsOverride(r.Context(), group)
				global, _ := getAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					global = &d
				}
				effective := *global
				if override != nil {
					effective = *override
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"override":   override,  // null means inherit from global
					"effective":  effective, // actual value used
					"inherits_from": "global",
				})
			})

			// PATCH group auto-deployment: { "auto_deploy": true|false|null }
			priv.Patch("/gitops/groups/{name}", func(w http.ResponseWriter, r *http.Request) {
				group := chi.URLParam(r, "name")
				var body struct {
					AutoDeploy *bool `json:"auto_deploy"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if err := setGroupDevopsOverride(r.Context(), group, body.AutoDeploy); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				override, _ := getGroupDevopsOverride(r.Context(), group)
				global, _ := getAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					global = &d
				}
				effective := *global
				if override != nil {
					effective = *override
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"override": override, 
					"effective": effective, 
					"inherits_from": "global",
					"status": "ok",
				})
			})

			/* Stack-specific GitOps Configuration */
			// GET /api/gitops/hosts/{name}/stacks/{stackname}
			priv.Get("/gitops/hosts/{name}/stacks/{stackname}", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				stackName := chi.URLParam(r, "stackname")
				override, err := getStackDevopsOverride(r.Context(), "host", host, stackName)
				if err != nil {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				
				// Determine effective value via hierarchy
				hostOverride, _ := getHostDevopsOverride(r.Context(), host)
				global, _ := getAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					global = &d
				}
				
				effective := *global
				inheritsFrom := "global"
				if hostOverride != nil {
					effective = *hostOverride
					inheritsFrom = "host"
				}
				if override != nil {
					effective = *override
					inheritsFrom = "stack"
				}
				
				writeJSON(w, http.StatusOK, map[string]any{
					"override": override, 
					"effective": effective, 
					"inherits_from": inheritsFrom,
					"status": "ok",
				})
			})

			// PATCH /api/gitops/hosts/{name}/stacks/{stackname}
			priv.Patch("/gitops/hosts/{name}/stacks/{stackname}", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				stackName := chi.URLParam(r, "stackname")
				var body struct {
					AutoDeploy *bool `json:"auto_deploy"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if err := setStackDevopsOverride(r.Context(), "host", host, stackName, body.AutoDeploy); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				
				// Return updated configuration
				override, _ := getStackDevopsOverride(r.Context(), "host", host, stackName)
				hostOverride, _ := getHostDevopsOverride(r.Context(), host)
				global, _ := getAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					global = &d
				}
				
				effective := *global
				inheritsFrom := "global"
				if hostOverride != nil {
					effective = *hostOverride
					inheritsFrom = "host"
				}
				if override != nil {
					effective = *override
					inheritsFrom = "stack"
				}
				
				writeJSON(w, http.StatusOK, map[string]any{
					"override": override, 
					"effective": effective, 
					"inherits_from": inheritsFrom,
					"status": "ok",
				})
			})

			// GET /api/gitops/groups/{name}/stacks/{stackname}
			priv.Get("/gitops/groups/{name}/stacks/{stackname}", func(w http.ResponseWriter, r *http.Request) {
				group := chi.URLParam(r, "name")
				stackName := chi.URLParam(r, "stackname")
				override, err := getStackDevopsOverride(r.Context(), "group", group, stackName)
				if err != nil {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				
				// Determine effective value via hierarchy
				groupOverride, _ := getGroupDevopsOverride(r.Context(), group)
				global, _ := getAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					global = &d
				}
				
				effective := *global
				inheritsFrom := "global"
				if groupOverride != nil {
					effective = *groupOverride
					inheritsFrom = "group"
				}
				if override != nil {
					effective = *override
					inheritsFrom = "stack"
				}
				
				writeJSON(w, http.StatusOK, map[string]any{
					"override": override, 
					"effective": effective, 
					"inherits_from": inheritsFrom,
					"status": "ok",
				})
			})

			// PATCH /api/gitops/groups/{name}/stacks/{stackname}
			priv.Patch("/gitops/groups/{name}/stacks/{stackname}", func(w http.ResponseWriter, r *http.Request) {
				group := chi.URLParam(r, "name")
				stackName := chi.URLParam(r, "stackname")
				var body struct {
					AutoDeploy *bool `json:"auto_deploy"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if err := setStackDevopsOverride(r.Context(), "group", group, stackName, body.AutoDeploy); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				
				// Return updated configuration
				override, _ := getStackDevopsOverride(r.Context(), "group", group, stackName)
				groupOverride, _ := getGroupDevopsOverride(r.Context(), group)
				global, _ := getAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					global = &d
				}
				
				effective := *global
				inheritsFrom := "global"
				if groupOverride != nil {
					effective = *groupOverride
					inheritsFrom = "group"
				}
				if override != nil {
					effective = *override
					inheritsFrom = "stack"
				}
				
				writeJSON(w, http.StatusOK, map[string]any{
					"override": override, 
					"effective": effective, 
					"inherits_from": inheritsFrom,
					"status": "ok",
				})
			})

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

			// Logs (simple, non-follow)
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

			// -------- Live logs (SSE) --------
			priv.Get("/hosts/{name}/containers/{ctr}/logs/stream", func(w http.ResponseWriter, r *http.Request) {
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

				inspect, err := cli.ContainerInspect(r.Context(), ctr)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				opts := container.LogsOptions{
					ShowStdout: true,
					ShowStderr: true,
					Follow:     true,
					Timestamps: false,
					Details:    false,
				}
				if s := strings.TrimSpace(r.URL.Query().Get("since")); s != "" {
					opts.Since = s
				}
				if t := strings.TrimSpace(r.URL.Query().Get("tail")); t != "" {
					opts.Tail = t
				}

				rc, err := cli.ContainerLogs(r.Context(), ctr, opts)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				defer rc.Close()

				fl, ok := writeSSEHeader(w)
				if !ok {
					http.Error(w, "stream unsupported", http.StatusInternalServerError)
					return
				}

				stdout := &sseLineWriter{w: w, fl: fl, stream: "stdout"}
				stderr := &sseLineWriter{w: w, fl: fl, stream: "stderr"}

				// Keep-alive tick
				tick := time.NewTicker(15 * time.Second)
				defer tick.Stop()

				done := make(chan struct{})
				go func() {
					defer close(done)
					if inspect.Config != nil && inspect.Config.Tty {
						// No multiplexing when TTY true
						sc := bufio.NewScanner(rc)
						for sc.Scan() {
							_, _ = stdout.Write(append(sc.Bytes(), '\n'))
						}
					} else {
						// Demux Docker log multiplexing
						_, _ = stdcopy.StdCopy(stdout, stderr, rc)
					}
				}()

				// pump until client disconnects
				notify := r.Context().Done()
				for {
					select {
					case <-notify:
						return
					case <-done:
						return
					case <-tick.C:
						_, _ = w.Write([]byte(": keep-alive\n\n"))
						fl.Flush()
					}
				}
			})

			// -------- Interactive console (WebSocket) --------
			priv.Get("/ws/hosts/{name}/containers/{ctr}/exec", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				ctr := chi.URLParam(r, "ctr")

				debugLog("Console: WebSocket connection requested for host=%s container=%s", host, ctr)

				conn, err := wsUpgrader.Upgrade(w, r, nil)
				if err != nil {
					debugLog("Console: WebSocket upgrade failed for host=%s: %v", host, err)
					return
				}
				defer conn.Close()

				h, err := GetHostByName(r.Context(), host)
				if err != nil {
					debugLog("Console: Failed to get host %s: %v", host, err)
					_ = conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
					return
				}

				// Hybrid approach: use local Docker client for local host, SSH exec for remote hosts
				if localHostAllowed(h) {
					debugLog("Console: Using local Docker client for host %s (local host optimization)", host)
					cli, err := dockerClientForHost(h)
					if err != nil {
						debugLog("Console: Failed to create Docker client for host %s: %v", host, err)
						_ = conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
						return
					}
					defer cli.Close()
					debugLog("Console: Docker client created successfully for host %s", host)
					
					// Use existing Docker client approach for local host
					handleLocalConsole(conn, cli, host, ctr, r)
				} else {
					debugLog("Console: Using SSH exec for remote host %s", host)
					// Use direct SSH exec approach for remote hosts
					handleRemoteConsole(conn, h, host, ctr, r)
				}

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

				stored, _ := getImageTagMap(r.Context(), host)

				type row struct {
					Repo    string `json:"repo"`
					Tag     string `json:"tag"`
					ID      string `json:"id"`
					Size    string `json:"size"`
					Created string `json:"created"`
				}
				var items []row
				seen := make(map[string]struct{}, len(list))

				for _, im := range list {
					id := im.ID
					seen[id] = struct{}{}
					repo := "<none>"
					tag := "none"

					if len(im.RepoTags) > 0 && im.RepoTags[0] != "<none>:<none>" {
						parts := strings.SplitN(im.RepoTags[0], ":", 2)
						repo = parts[0]
						if len(parts) == 2 {
							tag = parts[1]
						}
					} else if prev, ok := stored[id]; ok {
						if strings.TrimSpace(prev[0]) != "" {
							repo = prev[0]
						}
						if strings.TrimSpace(prev[1]) != "" {
							tag = prev[1]
						}
					}

					_ = upsertImageTag(r.Context(), host, id, repo, tag)

					items = append(items, row{
						Repo:    repo,
						Tag:     tag,
						ID:      id,
						Size:    humanSize(im.Size),
						Created: time.Unix(im.Created, 0).Format(time.RFC3339),
					})
				}

				_ = cleanupImageTags(r.Context(), host, seen)

				writeJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			// Bulk delete images
			priv.Post("/hosts/{name}/images/delete", func(w http.ResponseWriter, r *http.Request) {
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

				var body struct {
					IDs   []string `json:"ids"`
					Force bool     `json:"force"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if len(body.IDs) == 0 {
					http.Error(w, "ids required", http.StatusBadRequest)
					return
				}

				type res struct {
					ID  string `json:"id"`
					Ok  bool   `json:"ok"`
					Err string `json:"err,omitempty"`
				}
				out := make([]res, 0, len(body.IDs))

				for _, id := range body.IDs {
					_, err := cli.ImageRemove(r.Context(), id, image.RemoveOptions{
						Force:         body.Force,
						PruneChildren: true,
					})
					if err != nil {
						out = append(out, res{ID: id, Ok: false, Err: err.Error()})
						continue
					}
					out = append(out, res{ID: id, Ok: true})
				}

				_, _ = db.Exec(r.Context(),
					`DELETE FROM image_tags WHERE host_name=$1 AND image_id = ANY($2::text[])`,
					host, body.IDs,
				)

				writeJSON(w, http.StatusOK, map[string]any{"results": out})
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

			// Delete networks
			priv.Post("/hosts/{name}/networks/delete", func(w http.ResponseWriter, r *http.Request) {
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

				var body struct {
					Names []string `json:"names"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if len(body.Names) == 0 {
					http.Error(w, "names required", http.StatusBadRequest)
					return
				}

				type res struct {
					Name string `json:"name"`
					Ok   bool   `json:"ok"`
					Err  string `json:"err,omitempty"`
				}
				out := make([]res, 0, len(body.Names))

				for _, n := range body.Names {
					if err := cli.NetworkRemove(r.Context(), n); err != nil {
						out = append(out, res{Name: n, Ok: false, Err: err.Error()})
						continue
					}
					out = append(out, res{Name: n, Ok: true})
				}

				writeJSON(w, http.StatusOK, map[string]any{"results": out})
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

			// Delete volumes
			priv.Post("/hosts/{name}/volumes/delete", func(w http.ResponseWriter, r *http.Request) {
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

				var body struct {
					Names []string `json:"names"`
					Force bool     `json:"force"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if len(body.Names) == 0 {
					http.Error(w, "names required", http.StatusBadRequest)
					return
				}

				type res struct {
					Name string `json:"name"`
					Ok   bool   `json:"ok"`
					Err  string `json:"err,omitempty"`
				}
				out := make([]res, 0, len(body.Names))

				for _, v := range body.Names {
					if err := cli.VolumeRemove(r.Context(), v, body.Force); err != nil {
						out = append(out, res{Name: v, Ok: false, Err: err.Error()})
						continue
					}
					out = append(out, res{Name: v, Ok: true})
				}

				writeJSON(w, http.StatusOK, map[string]any{"results": out})
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

			// Trigger scan for all known hosts (inventory only)
			priv.Post("/scan/all", func(w http.ResponseWriter, r *http.Request) {
				// IaC scan (non-fatal)
				if _, _, err := ScanIacLocal(r.Context()); err != nil {
					errorLog("iac: sync scan failed: %v", err)
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

			// POST /api/inventory/reload
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

			// Desired stacks/services for a host (host + groups) - now using enhanced logic for drift detection
			priv.Get("/hosts/{name}/iac", func(w http.ResponseWriter, r *http.Request) {
				name := chi.URLParam(r, "name")
				debugLog("Basic-IAC request for host: %s (using enhanced logic)", name)
				items, err := listEnhancedIacStacksForHost(r.Context(), name)
				if err != nil {
					debugLog("Basic-IAC (enhanced) failed for host %s: %v", name, err)
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				debugLog("Basic-IAC (enhanced) returning %d stacks for host %s", len(items), name)
				for i, stack := range items {
					debugLog("Response stack[%d]: %s has %d services, %d rendered_services, compose=%v", i, stack.Name, len(stack.Services), len(stack.RenderedServices), stack.Compose != "")
					if len(stack.RenderedServices) > 0 {
						debugLog("  Stack %s using RENDERED services (decrypted):", stack.Name)
						for j, rs := range stack.RenderedServices {
							debugLog("    RenderedService[%d]: %s -> image=%s, container=%s", j, rs.ServiceName, rs.Image, rs.ContainerName)
						}
					}
					if len(stack.Services) > 0 {
						debugLog("  Stack %s raw services (may be encrypted):", stack.Name)
						for j, svc := range stack.Services {
							debugLog("    RawService[%d]: %s -> image=%s, container=%s", j, svc.ServiceName, svc.Image, svc.ContainerName)
						}
					}
				}
				writeJSON(w, http.StatusOK, map[string]any{"stacks": items})
			})

			// Enhanced stacks/services with deployment stamp-based drift detection
			priv.Get("/hosts/{name}/enhanced-iac", func(w http.ResponseWriter, r *http.Request) {
				name := chi.URLParam(r, "name")
				debugLog("Enhanced-IAC request for host: %s", name)
				items, err := listEnhancedIacStacksForHost(r.Context(), name)
				if err != nil {
					debugLog("Enhanced-IAC failed for host %s: %v", name, err)
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				debugLog("Enhanced-IAC returning %d stacks for host %s", len(items), name)
				for i, stack := range items {
					debugLog("Enhanced stack[%d]: %s has %d services, %d rendered_services", i, stack.Name, len(stack.Services), len(stack.RenderedServices))
					for j, rs := range stack.RenderedServices {
						debugLog("  Enhanced RenderedService[%d]: %s -> image=%s, container=%s", j, rs.ServiceName, rs.Image, rs.ContainerName)
					}
					for j, svc := range stack.Services {
						debugLog("  Enhanced RawService[%d]: %s -> image=%s, container=%s", j, svc.ServiceName, svc.Image, svc.ContainerName)
					}
				}
				writeJSON(w, http.StatusOK, map[string]any{"stacks": items})
			})

			// Direct SSH command execution on hosts
			priv.Post("/hosts/{name}/ssh", func(w http.ResponseWriter, r *http.Request) {
				hostName := chi.URLParam(r, "name")
				var body struct {
					Command string   `json:"command"`
					Args    []string `json:"args,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "invalid JSON", http.StatusBadRequest)
					return
				}
				if strings.TrimSpace(body.Command) == "" {
					http.Error(w, "command is required", http.StatusBadRequest)
					return
				}

				op := SSHDirectOperation{
					HostName: hostName,
					Command:  body.Command,
					Args:     body.Args,
				}

				output, err := ExecuteSSHDirectOperation(r.Context(), op)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"success": false,
						"error":   err.Error(),
					})
					return
				}

				writeJSON(w, http.StatusOK, map[string]any{
					"success": true,
					"output":  output,
				})
			})

			// Enhanced container operations with deployment stamp awareness
			priv.Post("/hosts/{name}/containers/{ctr}/enhanced-action", func(w http.ResponseWriter, r *http.Request) {
				hostName := chi.URLParam(r, "name")
				containerID := chi.URLParam(r, "ctr")
				var body struct {
					Action string `json:"action"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "invalid JSON", http.StatusBadRequest)
					return
				}

				var result *ContainerOpResult
				switch strings.ToLower(body.Action) {
				case "start":
					result = StartContainer(r.Context(), hostName, containerID)
				case "stop":
					result = StopContainer(r.Context(), hostName, containerID)
				case "restart":
					result = RestartContainer(r.Context(), hostName, containerID)
				case "logs":
					result = GetContainerLogs(r.Context(), hostName, containerID)
				default:
					http.Error(w, "unsupported action", http.StatusBadRequest)
					return
				}

				if result.Success {
					writeJSON(w, http.StatusOK, result)
				} else {
					writeJSON(w, http.StatusBadRequest, result)
				}
			})

			// Create a new stack in the local IaC repo
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

			// Get a single IaC stack (returns effective auto devops)
			priv.Get("/iac/stacks/{id}", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, _ := strconv.ParseInt(idStr, 10, 64)

				var row struct {
					ID         int64   `json:"id"`
					RepoID     int64   `json:"repo_id"`
					ScopeKind  string  `json:"scope_kind"`
					ScopeName  string  `json:"scope_name"`
					StackName  string  `json:"stack_name"`
					RelPath    string  `json:"rel_path"`
					IacEnabled bool    `json:"iac_enabled"`
					DeployKind string  `json:"deploy_kind"`
					SopsStatus string  `json:"sops_status"`
					Override   *bool   `json:"auto_apply_override"`
					UpdatedAt  string  `json:"updated_at"`
					Effective  bool    `json:"effective_auto_devops"`
				}
				var updatedAt time.Time
				err := db.QueryRow(r.Context(), `
					SELECT id, repo_id, scope_kind::text, scope_name, stack_name, rel_path,
						iac_enabled, deploy_kind::text, sops_status::text, auto_apply_override, updated_at
					FROM iac_stacks
					WHERE id=$1
				`, id).Scan(
					&row.ID, &row.RepoID, &row.ScopeKind, &row.ScopeName, &row.StackName, &row.RelPath,
					&row.IacEnabled, &row.DeployKind, &row.SopsStatus, &row.Override, &updatedAt,
				)
				if err != nil {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				row.UpdatedAt = updatedAt.Format(time.RFC3339)
				eff, _ := shouldAutoApply(r.Context(), id)
				row.Effective = eff
				writeJSON(w, http.StatusOK, map[string]any{"stack": row})
			})

			// Patch IaC stack (no implicit override writes)
			priv.Patch("/iac/stacks/{id}", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, _ := strconv.ParseInt(idStr, 10, 64)

				var body struct {
					IacEnabled        *bool `json:"iac_enabled,omitempty"`
					AutoDevOps        *bool `json:"auto_devops,omitempty"`          // explicit override set/clear (when provided)
					AutoDevOpsInherit *bool `json:"auto_devops_inherit,omitempty"`  // when true, clear override (set NULL)
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}

				// Only update iac_enabled if caller asked
				if body.IacEnabled != nil {
					if _, err := db.Exec(r.Context(), `UPDATE iac_stacks SET iac_enabled=$1 WHERE id=$2`, *body.IacEnabled, id); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
				}

				// Clear explicit override if requested
				if body.AutoDevOpsInherit != nil && *body.AutoDevOpsInherit {
					if _, err := db.Exec(r.Context(), `UPDATE iac_stacks SET auto_apply_override=NULL WHERE id=$1`, id); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
				}

				// Set explicit override when provided
				if body.AutoDevOps != nil {
					if _, err := db.Exec(r.Context(), `UPDATE iac_stacks SET auto_apply_override=$1 WHERE id=$2`, *body.AutoDevOps, id); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
				}

				eff, _ := shouldAutoApply(r.Context(), id)
				writeJSON(w, http.StatusOK, map[string]any{
					"id":                    id,
					"iac_enabled":           body.IacEnabled,
					"auto_devops":           body.AutoDevOps,
					"auto_devops_inherit":   body.AutoDevOpsInherit,
					"effective_auto_devops": eff,
					"status":                "ok",
				})
			})

			// Delete a stack (optionally delete files too)
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

			// Read file content for a stack file (with optional decrypt)
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
					if !envBool("DDUI_ALLOW_SOPS_DECRYPT", "false") {
						http.Error(w, "decrypt disabled on server", http.StatusForbidden)
						return
					}
					if strings.ToLower(r.Header.Get("X-Confirm-Reveal")) != "yes" {
						http.Error(w, "confirmation required", http.StatusForbidden)
						return
					}
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

				// Optional SOPS auto-encrypt on save (opt-in)
				shouldSops := body.Sops || strings.HasSuffix(strings.ToLower(body.Path), "_private.env") || strings.HasSuffix(strings.ToLower(body.Path), "_secret.env")
				if shouldSops && (os.Getenv("SOPS_AGE_KEY") != "" || os.Getenv("SOPS_AGE_KEY_FILE") != "" || os.Getenv("SOPS_AGE_RECIPIENTS") != "") {
					args := []string{"-e", "-i"}
					if strings.HasSuffix(strings.ToLower(body.Path), ".env") {
						args = []string{"-e", "-i", "--input-type", "dotenv"}
					}
					if rec := strings.TrimSpace(os.Getenv("SOPS_AGE_RECIPIENTS")); rec != "" {
						for _, rcp := range strings.Fields(rec) {
							if rcp != "" {
								args = append(args, "--age", rcp)
							}
						}
					}
					args = append(args, full)

					ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
					defer cancel()
					out, err := exec.CommandContext(ctx, "sops", args...).CombinedOutput()
					if err != nil {
						errorLog("sops: encrypt failed: %v out=%s", err, string(out))
					} else {
						debugLog("sops: encrypted %s", full)
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

			// Deploy endpoint
			// - Manual deploys: **default** (for UI). Pass ?auto=1 for background/auto callers.
			priv.Post("/iac/stacks/{id}/deploy", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, err := strconv.ParseInt(idStr, 10, 64)
				if err != nil || id <= 0 {
					http.Error(w, "bad id", http.StatusBadRequest)
					return
				}

				// must have something to deploy
				ok, derr := stackHasContent(r.Context(), id)
				if derr != nil {
					http.Error(w, derr.Error(), http.StatusInternalServerError)
					return
				}
				if !ok {
					http.Error(w, "stack has no files", http.StatusBadRequest)
					return
				}

				// Manual by default (UI-friendly). Auto callers pass ?auto=1 explicitly.
				manual := !isTrueish(r.URL.Query().Get("auto"))

				// Gate only if NOT manual
				if !manual {
					allowed, aerr := shouldAutoApply(r.Context(), id)
					if aerr != nil {
						http.Error(w, aerr.Error(), http.StatusBadRequest)
						return
					}
					if !allowed {
						writeJSON(w, http.StatusAccepted, map[string]any{
							"status":  "skipped",
							"reason":  "auto_devops_disabled",
							"stackID": id,
						})
						return
					}
				}

				// async deploy with timeout + logging; mark manual flag in context
				go func(stackID int64, manual bool) {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					ctx = context.WithValue(ctx, ctxManualKey{}, manual)
					if err := deployStack(ctx, stackID); err != nil {
						errorLog("deploy: stack %d failed: %v", stackID, err)
						return
					}
					infoLog("deploy: stack %d ok", stackID)
				}(id, manual)

				writeJSON(w, http.StatusAccepted, map[string]any{
					"status":  "queued",
					"id":      id,
					"manual":  manual,
					"allowed": true,
				})
			})

			// Streaming deploy endpoint (compatible with existing frontend)
			priv.Get("/iac/stacks/{id}/deploy-stream", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, err := strconv.ParseInt(idStr, 10, 64)
				if err != nil || id <= 0 {
					http.Error(w, "bad id", http.StatusBadRequest)
					return
				}

				// Check if stack has content
				ok, derr := stackHasContent(r.Context(), id)
				if derr != nil {
					http.Error(w, derr.Error(), http.StatusInternalServerError)
					return
				}
				if !ok {
					http.Error(w, "stack has no deployable content", http.StatusBadRequest)
					return
				}

				// Set up Server-Sent Events
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")
				w.Header().Set("Access-Control-Allow-Origin", "*")
				
				// Create event channel
				eventChan := make(chan map[string]interface{}, 100)
				
				// Start deployment in background
				ctx := context.WithValue(r.Context(), ctxManualKey{}, true)
				// Check for force parameter to bypass config unchanged checks
				if r.URL.Query().Get("force") == "true" {
					ctx = context.WithValue(ctx, ctxForceKey{}, true)
				}
				go func() {
					if err := deployStackWithStream(ctx, id, eventChan); err != nil {
						errorLog("deploy-stream: stack %d failed: %v", id, err)
					}
				}()

				// Stream events to client
				flusher, ok := w.(http.Flusher)
				if !ok {
					http.Error(w, "Streaming not supported", http.StatusInternalServerError)
					return
				}

				for event := range eventChan {
					eventJSON, _ := json.Marshal(event)
					fmt.Fprintf(w, "data: %s\n\n", eventJSON)
					flusher.Flush()

					// Break on completion or error
					if eventType, ok := event["type"].(string); ok && (eventType == "complete" || eventType == "error") {
						break
					}
				}
			})

			// Alternative streaming deploy endpoint using scope/stack name
			priv.Get("/scopes/{scope}/stacks/{stackname}/deploy-stream", func(w http.ResponseWriter, r *http.Request) {
				scope := chi.URLParam(r, "scope")
				stackName := chi.URLParam(r, "stackname")

				// Find the stack ID
				var stackID int64
				err := db.QueryRow(r.Context(), `
					SELECT id FROM iac_stacks 
					WHERE scope_name = $1 AND stack_name = $2
				`, scope, stackName).Scan(&stackID)
				if err != nil {
					http.Error(w, "Stack not found", http.StatusNotFound)
					return
				}

				// Check if stack has content
				ok, derr := stackHasContent(r.Context(), stackID)
				if derr != nil {
					http.Error(w, derr.Error(), http.StatusInternalServerError)
					return
				}
				if !ok {
					http.Error(w, "stack has no deployable content", http.StatusBadRequest)
					return
				}

				// Set up Server-Sent Events
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")
				w.Header().Set("Access-Control-Allow-Origin", "*")
				
				// Create event channel
				eventChan := make(chan map[string]interface{}, 100)
				
				// Start deployment in background
				ctx := context.WithValue(r.Context(), ctxManualKey{}, true)
				// Check for force parameter to bypass config unchanged checks
				if r.URL.Query().Get("force") == "true" {
					ctx = context.WithValue(ctx, ctxForceKey{}, true)
				}
				go func() {
					if err := deployStackWithStream(ctx, stackID, eventChan); err != nil {
						errorLog("deploy-stream: stack %d failed: %v", stackID, err)
					}
				}()

				// Stream events to client
				flusher, ok := w.(http.Flusher)
				if !ok {
					http.Error(w, "Streaming not supported", http.StatusInternalServerError)
					return
				}

				for event := range eventChan {
					eventJSON, _ := json.Marshal(event)
					fmt.Fprintf(w, "data: %s\n\n", eventJSON)
					flusher.Flush()

					// Break on completion
					if eventType, ok := event["type"].(string); ok && eventType == "complete" {
						break
					}
				}
			})

			// Check if configuration has changed endpoint
			priv.Post("/iac/stacks/{id}/deploy-check", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, err := strconv.ParseInt(idStr, 10, 64)
				if err != nil || id <= 0 {
					http.Error(w, "bad id", http.StatusBadRequest)
					return
				}

				// Get the current configuration
				_, err = getRepoRootForStack(r.Context(), id)
				if err != nil {
					http.Error(w, "failed to get repo root", http.StatusInternalServerError)
					return
				}
				var rel string
				_ = db.QueryRow(r.Context(), `SELECT rel_path FROM iac_stacks WHERE id=$1`, id).Scan(&rel)
				if strings.TrimSpace(rel) == "" {
					http.Error(w, "stack has no rel_path", http.StatusBadRequest)
					return
				}

				// Stage (SOPS decrypts into tmpfs and is cleaned afterwards)
				_, stagedComposes, cleanup, derr := stageStackForCompose(r.Context(), id)
				if derr != nil {
					http.Error(w, fmt.Sprintf("failed to stage stack: %v", derr), http.StatusInternalServerError)
					return
				}
				defer func() {
					if cleanup != nil {
						cleanup()
					}
				}()

				if len(stagedComposes) == 0 {
					respondJSON(w, map[string]interface{}{
						"config_unchanged": false,
					})
					return
				}

				var allComposeContent []byte
				for _, f := range stagedComposes {
					b, rerr := os.ReadFile(f)
					if rerr != nil {
						http.Error(w, fmt.Sprintf("failed to read compose file: %v", rerr), http.StatusInternalServerError)
						return
					}
					allComposeContent = append(allComposeContent, b...)
					allComposeContent = append(allComposeContent, '\n')
				}

				// Check if this configuration exists
				existingStamp, checkErr := CheckDeploymentStampExists(r.Context(), id, allComposeContent)
				if checkErr == nil && existingStamp != nil {
					respondJSON(w, map[string]interface{}{
						"config_unchanged":    true,
						"last_deploy_time":    existingStamp.DeploymentTimestamp.Format("2006-01-02 15:04:05"),
						"last_deploy_status":  existingStamp.DeploymentStatus,
						"existing_stamp_id":   existingStamp.ID,
					})
					return
				}

				respondJSON(w, map[string]interface{}{
					"config_unchanged": false,
				})
			})

			// Confirmation endpoint for deploying unchanged configuration
			priv.Get("/iac/stacks/{id}/deploy-force", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, err := strconv.ParseInt(idStr, 10, 64)
				if err != nil || id <= 0 {
					http.Error(w, "bad id", http.StatusBadRequest)
					return
				}

				// Set up Server-Sent Events
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")
				w.Header().Set("Access-Control-Allow-Origin", "*")

				flusher, ok := w.(http.Flusher)
				if !ok {
					http.Error(w, "streaming not supported", http.StatusInternalServerError)
					return
				}

				eventChan := make(chan map[string]interface{}, 100)

				// Start forced deployment in background
				ctx := context.WithValue(r.Context(), ctxManualKey{}, true)
				ctx = context.WithValue(ctx, ctxForceKey{}, true)
				go func() {
					if err := deployStackWithStream(ctx, id, eventChan); err != nil {
						errorLog("deploy-force: stack %d failed: %v", id, err)
					}
				}()

				// Stream events to client
				for event := range eventChan {
					eventJSON, _ := json.Marshal(event)
					fmt.Fprintf(w, "data: %s\n\n", eventJSON)
					flusher.Flush()

					// Break on completion
					if eventType, ok := event["type"].(string); ok && eventType == "complete" {
						break
					}
				}
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
	uiRoot := env("DDUI_UI_DIR", "/app/ui/dist")
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

// --- Image repo/tag persistence helpers ---

func upsertImageTag(ctx context.Context, host, id, repo, tag string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO image_tags (host_name, image_id, repo, tag)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (host_name, image_id)
		DO UPDATE SET repo=EXCLUDED.repo, tag=EXCLUDED.tag, last_seen=now();
	`, host, id, repo, tag)
	return err
}

func getImageTagMap(ctx context.Context, host string) (map[string][2]string, error) {
	rows, err := db.Query(ctx, `SELECT image_id, repo, tag FROM image_tags WHERE host_name=$1`, host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][2]string)
	for rows.Next() {
		var id, repo, tag string
		if err := rows.Scan(&id, &repo, &tag); err != nil {
			return nil, err
		}
		out[id] = [2]string{repo, tag}
	}
	return out, nil
}

func cleanupImageTags(ctx context.Context, host string, keepIDs map[string]struct{}) error {
	ids := make([]string, 0, len(keepIDs))
	for id := range keepIDs {
		ids = append(ids, id)
	}
	_, err := db.Exec(ctx, `
		DELETE FROM image_tags t
		WHERE t.host_name = $1
		  AND NOT EXISTS (
		    SELECT 1
		    FROM UNNEST($2::text[]) AS u(id)
		    WHERE u.id = t.image_id
		  );
	`, host, ids)
	return err
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

/* ---------------- DevOps Apply helpers ---------------- */

func isTrueish(s string) bool {
	if s == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	}
	return false
}

// app_settings: simple KV store
func getAppSetting(ctx context.Context, key string) (string, bool) {
	var v string
	err := db.QueryRow(ctx, `SELECT value FROM app_settings WHERE key=$1`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}
func setAppSetting(ctx context.Context, key, value string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO app_settings (key, value) VALUES ($1,$2)
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()
	`, key, value)
	return err
}
func delAppSetting(ctx context.Context, key string) error {
	_, err := db.Exec(ctx, `DELETE FROM app_settings WHERE key=$1`, key)
	return err
}
func getAppSettingBool(ctx context.Context, key string) (*bool, bool) {
	if s, ok := getAppSetting(ctx, key); ok {
		b := isTrueish(s)
		return &b, true
	}
	return nil, false
}

// Global DevOps Apply (Auto DevOps) – DB override with ENV fallback
func getGlobalDevopsApply(ctx context.Context) (bool, string) {
	if b, ok := getAppSettingBool(ctx, "devops_apply"); ok && b != nil {
		return *b, "db"
	}
	return envBool("DDUI_DEVOPS_APPLY", "false"), "env"
}
func setGlobalDevopsApply(ctx context.Context, v *bool) error {
	if v == nil {
		return delAppSetting(ctx, "devops_apply")
	}
	if *v {
		return setAppSetting(ctx, "devops_apply", "true")
	}
	return setAppSetting(ctx, "devops_apply", "false")
}

// host_settings: per-host overrides
func getHostDevopsOverride(ctx context.Context, host string) (*bool, error) {
	var val *bool
	err := db.QueryRow(ctx, `SELECT auto_apply_override FROM host_settings WHERE host_name=$1`, host).Scan(&val)
	if err != nil {
		return nil, nil // treat as absent
	}
	return val, nil
}
func setHostDevopsOverride(ctx context.Context, host string, v *bool) error {
	if v == nil {
		_, err := db.Exec(ctx, `DELETE FROM host_settings WHERE host_name=$1`, host)
		return err
	}
	_, err := db.Exec(ctx, `
		INSERT INTO host_settings (host_name, auto_apply_override)
		VALUES ($1,$2)
		ON CONFLICT (host_name) DO UPDATE SET auto_apply_override=EXCLUDED.auto_apply_override, updated_at=now()
	`, host, *v)
	return err
}

// group_settings: per-group overrides
func getGroupDevopsOverride(ctx context.Context, group string) (*bool, error) {
	var val *bool
	err := db.QueryRow(ctx, `SELECT auto_apply_override FROM group_settings WHERE group_name=$1`, group).Scan(&val)
	if err != nil {
		return nil, nil // treat as absent
	}
	return val, nil
}
func setGroupDevopsOverride(ctx context.Context, group string, v *bool) error {
	if v == nil {
		_, err := db.Exec(ctx, `DELETE FROM group_settings WHERE group_name=$1`, group)
		return err
	}
	_, err := db.Exec(ctx, `
		INSERT INTO group_settings (group_name, auto_apply_override)
		VALUES ($1,$2)
		ON CONFLICT (group_name) DO UPDATE SET auto_apply_override=EXCLUDED.auto_apply_override, updated_at=now()
	`, group, *v)
	return err
}

/* ---------- SSE + WS helpers ---------- */

type sseLineWriter struct {
	mu     sync.Mutex
	w      http.ResponseWriter
	fl     http.Flusher
	stream string // "stdout" | "stderr"
	buf    []byte
}

func (s *sseLineWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	for {
		i := -1
		for j, b := range s.buf {
			if b == '\n' {
				i = j
				break
			}
		}
		if i == -1 {
			break
		}
		line := string(s.buf[:i])
		s.buf = s.buf[i+1:]
		_, _ = s.w.Write([]byte("event: " + s.stream + "\n"))
		_, _ = s.w.Write([]byte("data: " + line + "\n\n"))
		s.fl.Flush()
	}
	return len(p), nil
}

func writeSSEHeader(w http.ResponseWriter) (http.Flusher, bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	// Disable proxy buffering (nginx)
	w.Header().Set("X-Accel-Buffering", "no")
	fl, ok := w.(http.Flusher)
	return fl, ok
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		// allow same-origin and configured UI origin
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		ui := strings.TrimSpace(env("DDUI_UI_ORIGIN", ""))
		if origin == "" || origin == ui {
			return true
		}
		// dev helpers
		if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
			return true
		}
		return false
	},
}

// handleLocalConsole handles console connections for local hosts using Docker client
func handleLocalConsole(conn *websocket.Conn, cli *client.Client, host, ctr string, r *http.Request) {
	// Choose command: prefer explicit ?cmd, else ?shell, else auto
	rawCmd := strings.TrimSpace(r.URL.Query().Get("cmd"))
	shell := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("shell")))

	var candidates [][]string
	if rawCmd != "" {
		candidates = [][]string{strings.Fields(rawCmd)}
	} else {
		switch shell {
		case "bash":
			candidates = [][]string{{"/bin/bash"}, {"/usr/bin/bash"}}
		case "ash":
			candidates = [][]string{{"/bin/ash"}}
		case "dash":
			candidates = [][]string{{"/bin/dash"}}
		case "sh":
			candidates = [][]string{{"/bin/sh"}, {"sh"}}
		default: // auto
			candidates = [][]string{
				{"/bin/bash"}, {"/usr/bin/bash"},
				{"/bin/ash"},
				{"/bin/dash"},
				{"/bin/sh"}, {"sh"},
			}
		}
	}

	type runner struct {
		id  string
		att types.HijackedResponse
	}
	var chosen *runner

	tryCtx, cancelTry := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancelTry()

	for _, cmd := range candidates {
		debugLog("Console: Trying shell command %v on host=%s container=%s", cmd, host, ctr)
		created, cerr := cli.ContainerExecCreate(tryCtx, ctr, types.ExecConfig{
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Tty:          true,
			Cmd:          cmd,
			Env:          []string{},
			WorkingDir:   "",
		})
		if cerr != nil || created.ID == "" {
			debugLog("Console: Shell command %v failed on host=%s: %v", cmd, host, cerr)
			continue
		}

		att, aerr := cli.ContainerExecAttach(tryCtx, created.ID, types.ExecStartCheck{Tty: true})
		if aerr != nil {
			errorLog("Console: Shell attach failed for %v on host=%s container=%s: %v", cmd, host, ctr, aerr)
			continue
		}

		// Inspect quickly: if it already exited, treat as not available.
		time.Sleep(150 * time.Millisecond) // tiny grace for startup
		ins, ierr := cli.ContainerExecInspect(tryCtx, created.ID)
		if ierr != nil {
			errorLog("Console: Shell inspect failed for %v on host=%s container=%s: %v", cmd, host, ctr, ierr)
			att.Close()
			continue
		}
		if ins.Running {
			debugLog("Console: Successfully started shell %v on host=%s container=%s", cmd, host, ctr)
			chosen = &runner{id: created.ID, att: att}
			break
		}
		// Not running (probably ENOENT / 127) — close & try next
		debugLog("Console: Shell %v exited on host=%s container=%s (exit_code=%d)", cmd, host, ctr, ins.ExitCode)
		att.Close()
	}

	if chosen == nil {
		debugLog("Console: No supported shell found on host=%s container=%s", host, ctr)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: no supported shell found (tried bash, ash, dash, sh)"))
		return
	}
	debugLog("Console: Console session established for host=%s container=%s", host, ctr)
	defer chosen.att.Close()

	// WS -> container stdin (handles resize control messages)
	go func(execID string) {
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				// half-close write if possible
				type closer interface{ CloseWrite() error }
				if cw, ok := chosen.att.Conn.(closer); ok {
					_ = cw.CloseWrite()
				} else {
					_ = chosen.att.Conn.Close()
				}
				return
			}
			if mt != websocket.TextMessage && mt != websocket.BinaryMessage {
				continue
			}

			// Optional resize: {"type":"resize","cols":80,"rows":24}
			if len(data) > 10 && data[0] == '{' {
				var msg struct {
					Type string `json:"type"`
					Cols int    `json:"cols"`
					Rows int    `json:"rows"`
				}
				if err := json.Unmarshal(data, &msg); err == nil && strings.EqualFold(msg.Type, "resize") {
					_ = cli.ContainerExecResize(context.Background(), execID, container.ResizeOptions{
						Width:  uint(msg.Cols),
						Height: uint(msg.Rows),
					})
					continue
				}
			}

			_, _ = chosen.att.Conn.Write(data)
		}
	}(chosen.id)

	// Container stdout/err -> WS (binary)
	buf := make([]byte, 32*1024)
	for {
		n, err := chosen.att.Reader.Read(buf)
		if n > 0 {
			_ = conn.WriteMessage(websocket.BinaryMessage, buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// handleRemoteConsole handles console connections for remote hosts using direct SSH exec
func handleRemoteConsole(conn *websocket.Conn, h HostRow, host, ctr string, r *http.Request) {
	// Choose command: prefer explicit ?cmd, else ?shell, else auto
	rawCmd := strings.TrimSpace(r.URL.Query().Get("cmd"))
	shell := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("shell")))

	var candidates [][]string
	if rawCmd != "" {
		candidates = [][]string{strings.Fields(rawCmd)}
	} else {
		switch shell {
		case "bash":
			candidates = [][]string{{"/bin/bash"}, {"/usr/bin/bash"}}
		case "ash":
			candidates = [][]string{{"/bin/ash"}}
		case "dash":
			candidates = [][]string{{"/bin/dash"}}
		case "sh":
			candidates = [][]string{{"/bin/sh"}, {"sh"}}
		default: // auto
			candidates = [][]string{
				{"/bin/bash"}, {"/usr/bin/bash"},
				{"/bin/ash"},
				{"/bin/dash"},
				{"/bin/sh"}, {"sh"},
			}
		}
	}

	// Get SSH configuration
	user := h.Vars["ansible_user"]
	if user == "" {
		user = env("SSH_USER", "root")
	}
	addr := h.Addr
	if addr == "" {
		addr = h.Name
	}
	keyFile := env("SSH_KEY_FILE", "")
	if keyFile == "" {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: SSH_KEY_FILE not configured"))
		return
	}

	// Try to find a working shell
	var chosenShell []string
	for _, cmd := range candidates {
		debugLog("Console: Testing shell %v on remote host=%s container=%s", cmd, host, ctr)
		
		// Test if the shell exists in the container via SSH + docker exec
		testCmd := fmt.Sprintf("docker exec %s %s -c 'echo shell_test' 2>/dev/null", ctr, strings.Join(cmd, " "))
		sshCmd := []string{
			"ssh", "-i", keyFile, "-o", "StrictHostKeyChecking=no", 
			"-o", "ConnectTimeout=10", fmt.Sprintf("%s@%s", user, addr), testCmd,
		}
		
		testCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		testResult := exec.CommandContext(testCtx, sshCmd[0], sshCmd[1:]...)
		output, err := testResult.CombinedOutput()
		cancel()
		
		if err == nil && strings.Contains(string(output), "shell_test") {
			debugLog("Console: Found working shell %v on remote host=%s container=%s", cmd, host, ctr)
			chosenShell = cmd
			break
		}
		debugLog("Console: Shell %v not available on remote host=%s container=%s: %v", cmd, host, ctr, err)
	}

	if chosenShell == nil {
		debugLog("Console: No supported shell found on remote host=%s container=%s", host, ctr)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: no supported shell found (tried bash, ash, dash, sh)"))
		return
	}

	// Start the interactive shell via SSH + docker exec
	dockerExecCmd := fmt.Sprintf("docker exec -it %s %s", ctr, strings.Join(chosenShell, " "))
	sshCmd := []string{
		"ssh", "-i", keyFile, "-o", "StrictHostKeyChecking=no",
		"-t", "-t", // Force TTY allocation
		fmt.Sprintf("%s@%s", user, addr), dockerExecCmd,
	}

	debugLog("Console: Starting remote shell via: %v", sshCmd)
	
	cmd := exec.CommandContext(r.Context(), sshCmd[0], sshCmd[1:]...)
	
	// Create pipes for stdin/stdout/stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: failed to create stdin pipe"))
		return
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: failed to create stdout pipe"))
		return
	}
	defer stdout.Close()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: failed to create stderr pipe"))
		return
	}
	defer stderr.Close()

	// Start the SSH command
	if err := cmd.Start(); err != nil {
		errorLog("Console: Failed to start remote shell on host=%s container=%s: %v", host, ctr, err)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: failed to start remote shell"))
		return
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
	}()

	debugLog("Console: Remote console session established for host=%s container=%s", host, ctr)

	// WebSocket -> SSH stdin
	go func() {
		defer stdin.Close()
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.TextMessage && mt != websocket.BinaryMessage {
				continue
			}

			// Handle resize messages (note: SSH doesn't support resize the same way)
			if len(data) > 10 && data[0] == '{' {
				var msg struct {
					Type string `json:"type"`
					Cols int    `json:"cols"`
					Rows int    `json:"rows"`
				}
				if err := json.Unmarshal(data, &msg); err == nil && strings.EqualFold(msg.Type, "resize") {
					// For SSH, we can't easily resize the remote TTY, so we'll skip this
					continue
				}
			}

			_, err = stdin.Write(data)
			if err != nil {
				return
			}
		}
	}()

	// SSH stdout -> WebSocket
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				_ = conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// SSH stderr -> WebSocket
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				_ = conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for the SSH command to finish
	cmd.Wait()
}
