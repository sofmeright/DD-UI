package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

type Health struct {
	Status    string    `json:"status"`
	StartedAt time.Time `json:"startedAt"`
	Edition   string    `json:"edition"`
}

// ---- DDUI internal label constants (kept small & neutral) ----
const (
	dduiLabelManaged = "ddui.managed"
	dduiLabelUID     = "ddui.uid"
	dduiLabelSpec    = "ddui.spec_digest"
	dduiLabelStackID = "ddui.stack_id"
	dduiLabelService = "ddui.service"
)

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

			/* ---------- Global DevOps Apply (Auto DevOps) ---------- */

			// GET current global value (DB override or ENV fallback) + source
			priv.Get("/devops/apply", func(w http.ResponseWriter, r *http.Request) {
				val, src := getGlobalDevopsApply(r.Context())
				writeJSON(w, http.StatusOK, map[string]any{
					"value":  val,
					"source": src, // "db" or "env"
				})
			})

			// PATCH global: { "value": true|false } or { "value": null } to clear to ENV
			priv.Patch("/devops/apply", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Value *bool `json:"value"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if err := setGlobalDevopsApply(r.Context(), body.Value); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				val, src := getGlobalDevopsApply(r.Context())
				writeJSON(w, http.StatusOK, map[string]any{"value": val, "source": src, "status": "ok"})
			})

			/* ---------- Host-level DevOps Apply override ---------- */

			// GET host override + effective (host override if set, else global)
			priv.Get("/hosts/{name}/devops/apply", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				ov, _ := getHostDevopsOverride(r.Context(), host)
				glob, _ := getAppSettingBool(r.Context(), "devops_apply")
				if glob == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					glob = &d
				}
				eff := *glob
				if ov != nil {
					eff = *ov
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"override": ov,   // null means inherit
					"effective": eff, // bool
				})
			})

			// PATCH host override: { "value": true|false } or { "value": null } to clear
			priv.Patch("/hosts/{name}/devops/apply", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				var body struct {
					Value *bool `json:"value"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if err := setHostDevopsOverride(r.Context(), host, body.Value); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				ov, _ := getHostDevopsOverride(r.Context(), host)
				glob, _ := getAppSettingBool(r.Context(), "devops_apply")
				if glob == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					glob = &d
				}
				eff := *glob
				if ov != nil {
					eff = *ov
				}
				writeJSON(w, http.StatusOK, map[string]any{"override": ov, "effective": eff, "status": "ok"})
			})

			/* ---------- Group-level DevOps Apply override ---------- */

			// GET group override + effective (group override if set, else global)
			priv.Get("/groups/{name}/devops/apply", func(w http.ResponseWriter, r *http.Request) {
				group := chi.URLParam(r, "name")
				ov, _ := getGroupDevopsOverride(r.Context(), group)
				glob, _ := getAppSettingBool(r.Context(), "devops_apply")
				if glob == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					glob = &d
				}
				eff := *glob
				if ov != nil {
					eff = *ov
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"override": ov,   // null means inherit
					"effective": eff, // bool
				})
			})

			// PATCH group override: { "value": true|false } or { "value": null } to clear
			priv.Patch("/groups/{name}/devops/apply", func(w http.ResponseWriter, r *http.Request) {
				group := chi.URLParam(r, "name")
				var body struct {
					Value *bool `json:"value"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if err := setGroupDevopsOverride(r.Context(), group, body.Value); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				ov, _ := getGroupDevopsOverride(r.Context(), group)
				glob, _ := getAppSettingBool(r.Context(), "devops_apply")
				if glob == nil {
					d := envBool("DDUI_DEVOPS_APPLY", "false")
					glob = &d
				}
				eff := *glob
				if ov != nil {
					eff = *ov
				}
				writeJSON(w, http.StatusOK, map[string]any{"override": ov, "effective": eff, "status": "ok"})
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

				conn, err := wsUpgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()

				h, err := GetHostByName(r.Context(), host)
				if err != nil {
					_ = conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
					return
				}
				cli, err := dockerClientForHost(h)
				if err != nil {
					_ = conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
					return
				}
				defer cli.Close()

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
						continue
					}

					att, aerr := cli.ContainerExecAttach(tryCtx, created.ID, types.ExecStartCheck{Tty: true})
					if aerr != nil {
						continue
					}

					// Inspect quickly: if it already exited, treat as not available.
					time.Sleep(150 * time.Millisecond) // tiny grace for startup
					ins, ierr := cli.ContainerExecInspect(tryCtx, created.ID)
					if ierr == nil && ins.Running {
						chosen = &runner{id: created.ID, att: att}
						break
					}
					// Not running (probably ENOENT / 127) — close & try next
					att.Close()
				}

				if chosen == nil {
					_ = conn.WriteMessage(websocket.TextMessage, []byte("error: no supported shell found (tried bash, ash, dash, sh)"))
					return
				}
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
					// New:
					FilesLatestEdit string `json:"files_latest_edit"`
					FilesDigest     string `json:"files_digest"`
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

				// compute latest edit + digest for determinism
				latest, digest, _ := filesLatestAndDigest(r.Context(), id)
				if !latest.IsZero() {
					row.FilesLatestEdit = latest.Format(time.RFC3339)
				} else {
					row.FilesLatestEdit = ""
				}
				row.FilesDigest = digest

				writeJSON(w, http.StatusOK, map[string]any{"stack": row})
			})

			// Patch IaC stack (decoupled + compat)
			priv.Patch("/iac/stacks/{id}", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				id, _ := strconv.ParseInt(idStr, 10, 64)

				var body struct {
					IacEnabled        *bool `json:"iac_enabled,omitempty"`
					AutoDevOps        *bool `json:"auto_devops,omitempty"`
					AutoDevOpsInherit *bool `json:"auto_devops_inherit,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}

				// Update iac_enabled only (decoupled)
				var touchedIac bool
				if body.IacEnabled != nil {
					if _, err := db.Exec(r.Context(), `UPDATE iac_stacks SET iac_enabled=$1 WHERE id=$2`, *body.IacEnabled, id); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					touchedIac = true
				}

				// Clear override if requested
				if body.AutoDevOpsInherit != nil && *body.AutoDevOpsInherit {
					if _, err := db.Exec(r.Context(), `UPDATE iac_stacks SET auto_apply_override=NULL WHERE id=$1`, id); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
				}

				// Set explicit auto devops override
				if body.AutoDevOps != nil {
					if _, err := db.Exec(r.Context(), `UPDATE iac_stacks SET auto_apply_override=$1 WHERE id=$2`, *body.AutoDevOps, id); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
				}

				// Compatibility: if only iac_enabled was set, mirror into auto_apply_override
				if touchedIac && body.AutoDevOps == nil && (body.AutoDevOpsInherit == nil || !*body.AutoDevOpsInherit) {
					_, _ = db.Exec(r.Context(), `UPDATE iac_stacks SET auto_apply_override=$1 WHERE id=$2`, *body.IacEnabled, id)
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
						log.Printf("deploy: stack %d failed: %v", stackID, err)
						return
					}
					log.Printf("deploy: stack %d ok", stackID)
				}(id, manual)

				writeJSON(w, http.StatusAccepted, map[string]any{
					"status":  "queued",
					"id":      id,
					"manual":  manual,
					"allowed": true,
				})
			})

			/* ---------- NEW: Desired Spec with SOPS-safe decryption & interpolation ---------- */

			priv.Get("/iac/stacks/{id}/desired", func(w http.ResponseWriter, r *http.Request) {
				idStr := chi.URLParam(r, "id")
				stackID, _ := strconv.ParseInt(idStr, 10, 64)
				spec, err := buildDesiredSpec(r.Context(), stackID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, http.StatusOK, spec)
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

// shouldAutoApply resolves effective Auto DevOps policy:
// global -> (host|group) override -> stack override
func shouldAutoApply(ctx context.Context, stackID int64) (bool, error) {
	// Base: global
	global, _ := getGlobalDevopsApply(ctx)

	// Fetch stack scope + stack override
	var scopeKind, scopeName string
	var stackOv *bool
	err := db.QueryRow(ctx, `
		SELECT scope_kind::text, scope_name, auto_apply_override
		FROM iac_stacks WHERE id=$1
	`, stackID).Scan(&scopeKind, &scopeName, &stackOv)
	if err != nil {
		return false, errors.New("stack not found")
	}

	eff := global

	switch strings.ToLower(scopeKind) {
	case "host":
		if hov, _ := getHostDevopsOverride(ctx, scopeName); hov != nil {
			eff = *hov
		}
	case "group":
		if gov, _ := getGroupDevopsOverride(ctx, scopeName); gov != nil {
			eff = *gov
		}
	}

	// Stack override wins if set
	if stackOv != nil {
		eff = *stackOv
	}

	return eff, nil
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

/* ---------------------- Desired Spec builder (SOPS-aware) ---------------------- */

type desiredSpec struct {
	Project         string                        `json:"project"`
	FilesDigest     string                        `json:"files_digest"`
	FilesLatestEdit string                        `json:"files_latest_edit"`
	Services        map[string]desiredServiceSpec `json:"services"`
}

type desiredServiceSpec struct {
	Service         string   `json:"service"`
	Image           string   `json:"image"`                        // canonical repo:tag (digest stripped, tag defaults to :latest)
	ImageAlternates []string `json:"image_alternates,omitempty"`   // other acceptable string forms (for tolerant drift compare)
	ContainerName   string   `json:"container_name,omitempty"`
	ExpectedNames   []string `json:"expected_names,omitempty"` // if container_name empty
	Ports           []string `json:"ports,omitempty"`
	Volumes         []string `json:"volumes,omitempty"`
	EnvKeys         []string `json:"env_keys,omitempty"` // keys only; no values returned
}

// Build desired spec for a stack by reading compose yaml(s) + .env, SOPS-decrypting where needed.
// Does NOT reveal secret values; only returns trivial fields and env KEYS.
func buildDesiredSpec(ctx context.Context, stackID int64) (*desiredSpec, error) {
	// Load stack basic info
	var scopeKind, scopeName, stackName, relPath string
	if err := db.QueryRow(ctx, `SELECT scope_kind::text, scope_name, stack_name, rel_path FROM iac_stacks WHERE id=$1`, stackID).
		Scan(&scopeKind, &scopeName, &stackName, &relPath); err != nil {
		return nil, errors.New("stack not found")
	}

	root, err := getRepoRootForStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	baseDir, err := joinUnder(root, relPath)
	if err != nil {
		return nil, err
	}

	// Collect compose-role files
	rows, err := db.Query(ctx, `SELECT rel_path, sops, sha256, updated_at FROM iac_files WHERE stack_id=$1 AND role='compose' ORDER BY rel_path`, stackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type frow struct {
		rel   string
		sops  bool
		sha   string
		mtime time.Time
	}
	var compFiles []frow
	for rows.Next() {
		var rel, sha string
		var sops bool
		var mt time.Time
		if err := rows.Scan(&rel, &sops, &sha, &mt); err != nil {
			return nil, err
		}
		compFiles = append(compFiles, frow{rel: rel, sops: sops, sha: sha, mtime: mt})
	}

	latest, digest, _ := filesLatestAndDigest(ctx, stackID)

	// Build env (Compose interpolation uses process env + .env in project dir)
	envMap := map[string]string{}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			envMap[kv[:i]] = kv[i+1:]
		}
	}
	// `.env` lives alongside the main compose — try both yaml dirs and stack base
	envCandidates := []string{
		filepath.Join(baseDir, ".env"),
	}
	// Add per-compose directory .env candidates uniquely
	seenEnv := map[string]struct{}{}
	for _, cf := range compFiles {
		full, err := joinUnder(root, cf.rel)
		if err == nil {
			dir := filepath.Dir(full)
			p := filepath.Join(dir, ".env")
			if _, ok := seenEnv[p]; !ok {
				envCandidates = append(envCandidates, p)
				seenEnv[p] = struct{}{}
			}
		}
	}
	for _, p := range envCandidates {
		if b, err := readPossiblySopsFile(ctx, p, true /* dotenv */); err == nil && len(b) > 0 {
			mergeDotenv(envMap, string(b))
		}
	}

	// Parse compose and gather services
	services := map[string]desiredServiceSpec{}
	for _, cf := range compFiles {
		full, err := joinUnder(root, cf.rel)
		if err != nil {
			continue
		}
		cnt, err := readPossiblySopsFile(ctx, full, false)
		if err != nil {
			continue
		}
		// Interpolation happens before YAML parse in Compose; emulate for the fields we extract.
		var y any
		if err := yaml.Unmarshal(cnt, &y); err != nil {
			continue
		}
		m, _ := y.(map[string]any)
		svcs, _ := m["services"].(map[string]any)
		for svcName, raw := range svcs {
			svcMap, _ := raw.(map[string]any)
			if svcMap == nil {
				continue
			}
			// Build service-local env for interpolation
			svcEnv := envFromService(svcMap["environment"])
			envSvc := mergeEnvs(envMap, svcEnv)

			// image (resolve + canonicalize)
			rawImage := strOf(svcMap["image"])
			resolvedImage := resolveVars(rawImage, envSvc)
			image := canonicalImage(resolvedImage)
			imgAlts := imageAlternates(rawImage, image)

			// container_name (optional)
			cn := resolveVars(strOf(svcMap["container_name"]), envSvc)

			// ports
			var ports []string
			if lst, ok := asList(svcMap["ports"]); ok {
				for _, v := range lst {
					ports = append(ports, resolveVars(v, envSvc))
				}
			}

			// volumes
			var vols []string
			if lst, ok := asList(svcMap["volumes"]); ok {
				for _, v := range lst {
					vols = append(vols, resolveVars(v, envSvc))
				}
			}

			// environment keys only
			envKeys := envKeysOnly(svcMap["environment"])

			// Expected container names (compose defaults)
			expected := expectedNamesFor(stackName, svcName, cn)

			// Merge (later files override earlier)
			services[svcName] = desiredServiceSpec{
				Service:         svcName,
				Image:           image,
				ImageAlternates: imgAlts,
				ContainerName:   cn,
				ExpectedNames:   expected,
				Ports:           ports,
				Volumes:         vols,
				EnvKeys:         envKeys,
			}
		}
	}

	return &desiredSpec{
		Project:         stackName,
		FilesDigest:     digest,
		FilesLatestEdit: func() string { if latest.IsZero() { return "" }; return latest.Format(time.RFC3339) }(),
		Services:        services,
	}, nil
}

/* ---------------------- helpers for desired spec ---------------------- */

// computeServiceSpecDigest creates a stable hash of what "matters" for drift.
// It includes project, files digest, service name, canonical image, container_name,
// and sorted lists of ports/volumes/env keys.
func computeServiceSpecDigest(project, filesDigest, svcName string, s desiredServiceSpec) string {
	h := sha256.New()
	write := func(parts ...string) {
		for _, p := range parts {
			io.WriteString(h, p)
			io.WriteString(h, "\n")
		}
	}
	write("v1", project, filesDigest, svcName, s.Image, s.ContainerName)
	if len(s.Ports) > 0 {
		cp := append([]string(nil), s.Ports...)
		sort.Strings(cp)
		write(cp...)
	}
	if len(s.Volumes) > 0 {
		cv := append([]string(nil), s.Volumes...)
		sort.Strings(cv)
		write(cv...)
	}
	if len(s.EnvKeys) > 0 {
		ck := append([]string(nil), s.EnvKeys...)
		sort.Strings(ck)
		write(ck...)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Merge env maps (b overwrites a)
func mergeEnvs(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// Extract service-level env key->value (mapping or list with KEY=VAL). Only string values are included.
func envFromService(v any) map[string]string {
	out := map[string]string{}
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if s, ok := vv.(string); ok {
				out[k] = s
			}
		}
	case []any:
		for _, it := range t {
			if s, ok := it.(string); ok {
				if i := strings.IndexByte(s, '='); i > 0 {
					out[s[:i]] = s[i+1:]
				}
			}
		}
	}
	return out
}

// filesLatestAndDigest returns max(updated_at) across stack files and a combined sha256 of each file's stored sha256.
func filesLatestAndDigest(ctx context.Context, stackID int64) (time.Time, string, error) {
	rows, err := db.Query(ctx, `SELECT sha256, updated_at FROM iac_files WHERE stack_id=$1 ORDER BY updated_at DESC`, stackID)
	if err != nil {
		return time.Time{}, "", err
	}
	defer rows.Close()
	var latest time.Time
	h := sha256.New()
	for rows.Next() {
		var sha string
		var mt time.Time
		if err := rows.Scan(&sha, &mt); err != nil {
			return time.Time{}, "", err
		}
		if mt.After(latest) {
			latest = mt
		}
		h.Write([]byte(sha))
	}
	sum := hex.EncodeToString(h.Sum(nil))
	return latest, sum, nil
}

// readPossiblySopsFile tries sops -d first; if that fails, falls back to plain read.
// For dotenv files, we add "--input-type dotenv" to keep sops happy when encrypting.
func readPossiblySopsFile(ctx context.Context, fullPath string, isDotenv bool) ([]byte, error) {
	// If file missing plain, return error
	if _, err := os.Stat(fullPath); err != nil {
		return nil, err
	}
	// Try sops -d if binary is present
	if pathInPath("sops") {
		args := []string{"-d", fullPath}
		if isDotenv {
			args = []string{"-d", "--input-type", "dotenv", fullPath}
		}
		ctxTO, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctxTO, "sops", args...).CombinedOutput()
		if err == nil && len(out) > 0 && !looksLikeBinary(out) {
			return out, nil
		}
	}
	// Fallback to plain
	return os.ReadFile(fullPath)
}

func looksLikeBinary(b []byte) bool {
	// treat as text if it has no NULs and is mostly printable
	if bytes.IndexByte(b, 0) >= 0 {
		return true
	}
	return false
}

func pathInPath(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

func mergeDotenv(dst map[string]string, content string) {
	lines := strings.Split(content, "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		// allow 'export KEY=VAL'
		if strings.HasPrefix(ln, "export ") {
			ln = strings.TrimSpace(strings.TrimPrefix(ln, "export "))
		}
		// KEY=VAL (allow quotes)
		if i := strings.IndexByte(ln, '='); i > 0 {
			k := strings.TrimSpace(ln[:i])
			v := strings.TrimSpace(ln[i+1:])
			v = strings.Trim(v, `"'`)
			if k != "" {
				dst[k] = v
			}
		}
	}
}

var varRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?)-([^}]*))?\}`)

// resolveVars supports ${VAR}, ${VAR-default}, ${VAR:-default} semantics.
func resolveVars(s string, env map[string]string) string {
	if s == "" {
		return s
	}
	return varRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := varRe.FindStringSubmatch(m)
		if len(sub) < 4 {
			return ""
		}
		key := sub[1]
		op := sub[2] // "" or ":" for ":-"
		def := sub[3]
		val, ok := env[key]
		if !ok {
			// not set: use default if provided
			if def != "" {
				return def
			}
			return ""
		}
		// ":" means treat empty as unset
		if op == ":" && val == "" && def != "" {
			return def
		}
		return val
	})
}

// canonicalImage normalizes to "repo:tag" (drop @digest, default :latest).
// It correctly handles registries with ports, e.g. "localhost:5000/repo" -> "localhost:5000/repo:latest".
func canonicalImage(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// strip digest
	if i := strings.IndexByte(s, '@'); i > 0 {
		s = s[:i]
	}
	// If the part after the last slash lacks a ':', append :latest
	lastSlash := strings.LastIndexByte(s, '/')
	namePart := s
	if lastSlash >= 0 {
		namePart = s[lastSlash+1:]
	}
	if !strings.Contains(namePart, ":") {
		return s + ":latest"
	}
	return s
}

// de-dup while preserving order
func uniq(ss []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// imageAlternates returns a small tolerant set of strings that should be considered equal to the canonical image.
func imageAlternates(raw, canon string) []string {
	alts := []string{canon}
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		// strip digest then apply canonicalization again on the raw base
		base := strings.SplitN(trimmed, "@", 2)[0]
		alts = append(alts, canonicalImage(base))
	}
	return uniq(alts)
}

// expectedNamesFor returns acceptable container names (explicit + compose defaults)
func expectedNamesFor(project, service, explicit string) []string {
	project = strings.TrimSpace(project)
	service = strings.TrimSpace(service)
	out := []string{}
	if explicit != "" {
		out = append(out, explicit)
	}
	if project != "" && service != "" {
		// underscore and hyphen forms
		out = append(out,
			project+"_"+service+"_1",
			project+"-"+service+"-1",
		)
		// tolerate swapped separators in project/service too
		p2 := strings.ReplaceAll(project, "-", "_")
		s2 := strings.ReplaceAll(service, "-", "_")
		out = append(out, p2+"_"+s2+"_1")
		p3 := strings.ReplaceAll(project, "_", "-")
		s3 := strings.ReplaceAll(service, "_", "-")
		out = append(out, p3+"-"+s3+"-1")
	}
	return uniq(out)
}

func strOf(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func asList(v any) ([]string, bool) {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, it := range t {
			if s, ok := it.(string); ok {
				out = append(out, s)
			} else if m, ok := it.(map[string]any); ok {
				// compact mapping like {"target": "80", "published": "8080", "protocol":"tcp"} -> string form if we can
				if s := stringifyMapPortOrVolume(m); s != "" {
					out = append(out, s)
				}
			}
		}
		return out, true
	case []string:
		return t, true
	case string:
		return []string{t}, true
	default:
		return nil, false
	}
}

func stringifyMapPortOrVolume(m map[string]any) string {
	// very light best-effort; ports: published:target/proto
	if tgt := strOf(m["target"]); tgt != "" {
		pub := strOf(m["published"])
		proto := strOf(m["protocol"])
		s := ""
		if pub != "" {
			s = pub + ":"
		}
		s += tgt
		if proto != "" {
			s += "/" + proto
		}
		return s
	}
	// volumes "source:target[:mode]"
	if src := strOf(m["source"]); src != "" {
		tgt := strOf(m["target"])
		mode := strOf(m["type"])
		s := src + ":" + tgt
		if mode != "" {
			s += ":" + mode
		}
		return s
	}
	return ""
}

// env_keys from service.environment (mapping or list)
func envKeysOnly(v any) []string {
	keys := []string{}
	switch t := v.(type) {
	case map[string]any:
		for k := range t {
			keys = append(keys, k)
		}
	case []any:
		for _, it := range t {
			if s, ok := it.(string); ok {
				if i := strings.IndexByte(s, '='); i > 0 {
					keys = append(keys, s[:i])
				} else if s != "" {
					keys = append(keys, s)
				}
			}
		}
	}
	sort.Strings(keys)
	return keys
}
