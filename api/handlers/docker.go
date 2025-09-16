package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"dd-ui/common"
	"dd-ui/database"
	"dd-ui/services"
	"dd-ui/utils"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// setupDockerRoutes sets up all Docker operations related routes
// This organizes the Docker management functionality from web.go into logical groups:
// - Container operations (list, logs, inspect, actions, stats)
// - Image operations (list, delete)
// - Network operations (list, delete) 
// - Volume operations (list, delete)
// - WebSocket container exec
func SetupDockerRoutes(router chi.Router) {
	// Container operations
	router.Route("/containers", func(r chi.Router) {
		r.Route("/hosts/{hostname}", func(r chi.Router) {
			r.Get("/", handleContainersList)
			r.Route("/{ctr}", func(r chi.Router) {
				r.Get("/", handleContainerGet)
				r.Get("/logs", handleContainerLogs)
				r.Get("/logs/stream", handleContainerLogsStream)
				r.Get("/inspect", handleContainerInspect)
				r.Get("/stats", handleContainerStats)
				r.Post("/action", handleContainerAction)
				r.Post("/enhanced-action", handleContainerEnhancedAction)
			})
		})
	})
	
	// Image operations
	router.Route("/images", func(r chi.Router) {
		r.Route("/hosts/{hostname}", func(r chi.Router) {
			r.Get("/", handleImagesList)
			r.Post("/delete", handleImagesDelete)
		})
	})
	
	// Network operations  
	router.Route("/networks", func(r chi.Router) {
		r.Route("/hosts/{hostname}", func(r chi.Router) {
			r.Get("/", handleNetworksList)
			r.Post("/delete", handleNetworksDelete)
		})
	})
	
	// Volume operations
	router.Route("/volumes", func(r chi.Router) {
		r.Route("/hosts/{hostname}", func(r chi.Router) {
			r.Get("/", handleVolumesList)
			r.Post("/delete", handleVolumesDelete)
		})
	})
	
	// WebSocket container exec
	router.Get("/ws/hosts/{name}/containers/{ctr}/exec", handleContainerExec)
}

// -------- Container Handlers --------

// handleContainersList lists all containers for a specific host
func handleContainersList(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	containers, err := database.ListContainersByHost(r.Context(), hostname)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list containers: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"containers": containers})
}

// handleContainerGet returns details for a single container
func handleContainerGet(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	containerName := chi.URLParam(r, "ctr")
	container, err := database.GetContainerByHostAndName(r.Context(), hostname, containerName)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get container: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"container": container})
}

// handleContainerLogsStream streams container logs via Server-Sent Events
func handleContainerLogsStream(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	ctr := chi.URLParam(r, "ctr")

	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cli, err := services.DockerClientForHost(h)
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

	fl, ok := utils.WriteSSEHeader(w)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	stdout := utils.NewSSELineWriter(w, fl, "stdout")
	stderr := utils.NewSSELineWriter(w, fl, "stderr")

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
	for {
		select {
		case <-done:
			return
		case <-tick.C:
			_, _ = stdout.Write([]byte{})
		case <-r.Context().Done():
			return
		}
	}
}

// handleContainerLogs returns container logs as plain text
func handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	ctr := chi.URLParam(r, "ctr")
	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cli, err := services.DockerClientForHost(h)
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
}

// handleContainerInspect returns detailed container inspection data
func handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	ctr := chi.URLParam(r, "ctr")
	out, err := database.GetContainerByHostAndName(r.Context(), hostname, ctr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleContainerAction performs basic container actions (start, stop, restart, pause, unpause, remove)
func handleContainerAction(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	ctr := chi.URLParam(r, "ctr")
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cli, err := services.DockerClientForHost(h)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer cli.Close()

	switch body.Action {
	case "start":
		err = cli.ContainerStart(r.Context(), ctr, container.StartOptions{})
	case "stop":
		timeout := 10
		err = cli.ContainerStop(r.Context(), ctr, container.StopOptions{Timeout: &timeout})
	case "restart":
		timeout := 10
		err = cli.ContainerRestart(r.Context(), ctr, container.StopOptions{Timeout: &timeout})
	case "pause":
		err = cli.ContainerPause(r.Context(), ctr)
	case "unpause":
		err = cli.ContainerUnpause(r.Context(), ctr)
	case "remove":
		err = cli.ContainerRemove(r.Context(), ctr, container.RemoveOptions{Force: true})
	default:
		http.Error(w, "unsupported action", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleContainerStats returns container statistics
func handleContainerStats(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	ctr := chi.URLParam(r, "ctr")
	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cli, err := services.DockerClientForHost(h)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer cli.Close()

	stats, err := cli.ContainerStats(r.Context(), ctr, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer stats.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, stats.Body)
}

// handleContainerEnhancedAction performs enhanced container actions with deployment awareness
func handleContainerEnhancedAction(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	containerID := chi.URLParam(r, "ctr")
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Find the host and get deployment stamps
	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cli, err := services.DockerClientForHost(h)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer cli.Close()

	// Get container details to find the stack
	inspect, err := cli.ContainerInspect(r.Context(), containerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Look for deployment stamp in labels
	var stampID int64
	if stampLabel, ok := inspect.Config.Labels["ddui.deployment.stamp"]; ok {
		stampID, _ = strconv.ParseInt(stampLabel, 10, 64)
	}

	// Execute the action with deployment awareness
	switch body.Action {
	case "start":
		err = cli.ContainerStart(r.Context(), containerID, container.StartOptions{})
	case "stop":
		timeout := 10
		err = cli.ContainerStop(r.Context(), containerID, container.StopOptions{Timeout: &timeout})
	case "restart":
		timeout := 10
		err = cli.ContainerRestart(r.Context(), containerID, container.StopOptions{Timeout: &timeout})
	default:
		http.Error(w, "unsupported enhanced action", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"container_id":  containerID,
		"action":        body.Action,
		"stamp_id":      stampID,
		"enhanced":      true,
	})
}

// handleContainerExec handles WebSocket container exec sessions
func handleContainerExec(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "name")
	ctr := chi.URLParam(r, "ctr")

	common.DebugLog("Console: WebSocket connection requested for host=%s container=%s", host, ctr)

	conn, err := utils.WSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		common.DebugLog("Console: WebSocket upgrade failed for host=%s: %v", host, err)
		return
	}
	defer conn.Close()

	h, err := database.GetHostByName(r.Context(), host)
	if err != nil {
		common.DebugLog("Console: Failed to get host %s: %v", host, err)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
		return
	}

	// Hybrid approach: use local Docker client for local host, SSH exec for remote hosts
	if services.LocalHostAllowed(h) {
		common.DebugLog("Console: Using local Docker client for host %s (local host optimization)", host)
		cli, err := services.DockerClientForHost(h)
		if err != nil {
			common.DebugLog("Console: Failed to create Docker client for host %s: %v", host, err)
			_ = conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
			return
		}
		defer cli.Close()
		common.DebugLog("Console: Docker client created successfully for host %s", host)
		
		// Use existing Docker client approach for local host
		handleLocalConsole(conn, cli, host, ctr, r)
	} else {
		common.DebugLog("Console: Using SSH exec for remote host %s", host)
		// Use direct SSH exec approach for remote hosts
		handleRemoteConsole(conn, h, host, ctr, r)
	}
}

// -------- Image Handlers --------

// handleImagesList lists all images for a specific host
func handleImagesList(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cli, err := services.DockerClientForHost(h)
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

	stored, _ := services.GetImageTagMap(r.Context(), hostname)

	// Get containers to determine image usage
	containers, _ := cli.ContainerList(r.Context(), container.ListOptions{All: true})
	usedImages := make(map[string]bool)
	for _, c := range containers {
		usedImages[c.ImageID] = true
		// Only track by actual image SHA, not by tag name
	}

	type row struct {
		Repo     string `json:"repo"`
		Tag      string `json:"tag"`
		ID       string `json:"id"`
		Size     string `json:"size"`
		Created  string `json:"created"`
		Orphaned bool   `json:"orphaned"`
		Usage    string `json:"usage"`
	}
	var items []row
	seen := make(map[string]struct{}, len(list))

	for _, im := range list {
		id := im.ID
		seen[id] = struct{}{}
		repo := "<none>"
		tag := "none"
		orphaned := false

		if len(im.RepoTags) > 0 && im.RepoTags[0] != "<none>:<none>" {
			parts := strings.SplitN(im.RepoTags[0], ":", 2)
			repo = parts[0]
			if len(parts) == 2 {
				tag = parts[1]
			}
		} else if prev, ok := stored[id]; ok {
			if strings.TrimSpace(prev[0]) != "" {
				repo = prev[0]
				orphaned = true // Mark as orphaned since we're using cached data
			}
			if strings.TrimSpace(prev[1]) != "" {
				tag = prev[1]
				orphaned = true // Mark as orphaned since we're using cached data
			}
		}

		_ = services.UpsertImageTag(r.Context(), hostname, id, repo, tag)

		// Determine usage status
		usage := "Unused"
		if usedImages[id] {
			usage = "Live"
		}
		// Only check by image SHA, not by tag name

		items = append(items, row{
			Repo:     repo,
			Tag:      tag,
			Orphaned: orphaned,
			Usage:    usage,
			ID:      id,
			Size:    services.HumanSize(im.Size),
			Created: time.Unix(im.Created, 0).Format(time.RFC3339),
		})
	}

	_ = services.CleanupImageTags(r.Context(), hostname, seen)

	writeJSON(w, http.StatusOK, map[string]any{"images": items})
}

// handleImagesDelete deletes specified images from a host
func handleImagesDelete(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cli, err := services.DockerClientForHost(h)
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
	_, _ = common.DB.Exec(r.Context(),
		`DELETE FROM image_tags WHERE host_name=$1 AND image_id = ANY($2::text[])`,
		hostname, body.IDs,
	)
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

// -------- Network Handlers --------

// handleNetworksList lists all networks for a specific host
func handleNetworksList(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cli, err := services.DockerClientForHost(h)
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
	writeJSON(w, http.StatusOK, map[string]any{"networks": items})
}

// handleNetworksDelete deletes specified networks from a host
func handleNetworksDelete(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cli, err := services.DockerClientForHost(h)
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
		err := cli.NetworkRemove(r.Context(), n)
		if err != nil {
			out = append(out, res{Name: n, Ok: false, Err: err.Error()})
			continue
		}
		out = append(out, res{Name: n, Ok: true})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

// -------- Volume Handlers --------

// handleVolumesList lists all volumes for a specific host
func handleVolumesList(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cli, err := services.DockerClientForHost(h)
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
	writeJSON(w, http.StatusOK, map[string]any{"volumes": items})
}

// handleVolumesDelete deletes specified volumes from a host
func handleVolumesDelete(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	h, err := database.GetHostByName(r.Context(), hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cli, err := services.DockerClientForHost(h)
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

	for _, n := range body.Names {
		err := cli.VolumeRemove(r.Context(), n, body.Force)
		if err != nil {
			out = append(out, res{Name: n, Ok: false, Err: err.Error()})
			continue
		}
		out = append(out, res{Name: n, Ok: true})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

// -------- Console Handling Functions --------

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
		common.DebugLog("Console: Trying shell command %v on host=%s container=%s", cmd, host, ctr)
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
			common.DebugLog("Console: Shell command %v failed on host=%s: %v", cmd, host, cerr)
			continue
		}

		att, aerr := cli.ContainerExecAttach(tryCtx, created.ID, types.ExecStartCheck{Tty: true})
		if aerr != nil {
			common.ErrorLog("Console: Shell attach failed for %v on host=%s container=%s: %v", cmd, host, ctr, aerr)
			continue
		}

		// Inspect quickly: if it already exited, treat as not available.
		time.Sleep(150 * time.Millisecond) // tiny grace for startup
		ins, ierr := cli.ContainerExecInspect(tryCtx, created.ID)
		if ierr != nil {
			common.ErrorLog("Console: Shell inspect failed for %v on host=%s container=%s: %v", cmd, host, ctr, ierr)
			att.Close()
			continue
		}
		if ins.Running {
			common.DebugLog("Console: Successfully started shell %v on host=%s container=%s", cmd, host, ctr)
			chosen = &runner{id: created.ID, att: att}
			break
		}
		// Not running (probably ENOENT / 127) — close & try next
		common.DebugLog("Console: Shell %v exited on host=%s container=%s (exit_code=%d)", cmd, host, ctr, ins.ExitCode)
		att.Close()
	}

	if chosen == nil {
		common.DebugLog("Console: No supported shell found on host=%s container=%s", host, ctr)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: no supported shell found (tried bash, ash, dash, sh)"))
		return
	}
	common.DebugLog("Console: Console session established for host=%s container=%s", host, ctr)
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
func handleRemoteConsole(conn *websocket.Conn, h database.HostRow, host, ctr string, r *http.Request) {
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
		user = common.Env("SSH_USER", "root")
	}
	addr := h.Addr
	if addr == "" {
		addr = h.Name
	}
	keyFile := common.Env("SSH_KEY_FILE", "")
	if keyFile == "" {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: SSH_KEY_FILE not configured"))
		return
	}

	// Try to find a working shell
	var chosenShell []string
	for _, cmd := range candidates {
		common.DebugLog("Console: Testing shell %v on remote host=%s container=%s", cmd, host, ctr)
		
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
			common.DebugLog("Console: Found working shell %v on remote host=%s container=%s", cmd, host, ctr)
			chosenShell = cmd
			break
		}
		common.DebugLog("Console: Shell %v not available on remote host=%s container=%s: %v", cmd, host, ctr, err)
	}

	if chosenShell == nil {
		common.DebugLog("Console: No supported shell found on remote host=%s container=%s", host, ctr)
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

	common.DebugLog("Console: Starting remote shell via: %v", sshCmd)
	
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
		common.ErrorLog("Console: Failed to start remote shell on host=%s container=%s: %v", host, ctr, err)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: failed to start remote shell"))
		return
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
	}()

	common.DebugLog("Console: Remote console session established for host=%s container=%s", host, ctr)

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