package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

var sshEnvMu sync.Mutex

func isUnixSock(url string) bool {
	return strings.HasPrefix(url, "unix://")
}

func localHostAllowed(h HostRow) bool {
	// 1) per-host opt-in
	if v := strings.ToLower(strings.TrimSpace(h.Vars["docker_local"])); v == "true" || v == "1" || v == "yes" {
		return true
	}
	// 2) env-mapped inventory name
	if lh := strings.TrimSpace(env("DDUI_LOCAL_HOST", "")); lh != "" && strings.EqualFold(lh, h.Name) {
		return true
	}
	// 3) obvious localhost addresses
	switch strings.ToLower(strings.TrimSpace(h.Addr)) {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}

func dockerURLFor(h HostRow) (string, string) {
	if v := h.Vars["docker_host"]; v != "" {
		return v, h.Vars["docker_ssh_cmd"]
	}

	kind := env("DDUI_SCAN_KIND", "local") // ssh|tcp|local
	switch kind {
	case "local":
		// Only valid for the one “local” host (we’ll enforce in ScanHostContainers)
		sock := env("DDUI_LOCAL_SOCK", "/var/run/docker.sock")
		return "unix://" + sock, ""
	case "tcp":
		host := h.Addr
		if host == "" { host = h.Name }
		port := env("DDUI_DOCKER_TCP_PORT", "2375")
		return fmt.Sprintf("tcp://%s:%s", host, port), ""
	default: // ssh
		user := h.Vars["ansible_user"]
		if user == "" {
			user = env("DDUI_SSH_USER", "root")
		}
		addr := h.Addr
		if addr == "" { addr = h.Name }
		return fmt.Sprintf("ssh://%s@%s", user, addr), os.Getenv("DOCKER_SSH_CMD")
	}
}

func withSSHEnv(cmd string, fn func() error) error {
	// DOCKER_SSH_CMD is read by docker's ssh connhelper at dial time.
	sshEnvMu.Lock()
	defer sshEnvMu.Unlock()

	prev, had := os.LookupEnv("DOCKER_SSH_CMD")
	if cmd != "" {
		_ = os.Setenv("DOCKER_SSH_CMD", cmd)
	}
	defer func() {
		if had {
			_ = os.Setenv("DOCKER_SSH_CMD", prev)
		} else {
			_ = os.Unsetenv("DOCKER_SSH_CMD")
		}
	}()
	return fn()
}

func dockerClientFor(ctx context.Context, h HostRow) (*client.Client, func(), error) {
	url, sshCmd := dockerURLFor(h)
	var cli *client.Client
	err := withSSHEnv(sshCmd, func() error {
		var err error
		cli, err = client.NewClientWithOpts(
			client.WithHost(url),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			return err
		}
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, err = cli.Ping(pctx)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = cli.Close() }
	return cli, cleanup, nil
}

// ScanHostContainers connects to a host's Docker and persists containers.
func ScanHostContainers(ctx context.Context, hostName string) (int, error) {
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return 0, err
	}

	// Resolve docker URL first
	url, sshCmd := dockerURLFor(h)

	// HARD GUARD: local-sock may only be used for the explicitly “local” host
	if isUnixSock(url) && !localHostAllowed(h) {
		msg := fmt.Errorf("refusing local docker.sock for non-local host %q (set DDUI_LOCAL_HOST=%s or hosts.%s.vars.docker_local=true)", h.Name, h.Name, h.Name)
		scanLog(ctx, h.ID, "warn", "skip local sock for non-local host", map[string]any{"url": url, "reason": msg.Error()})
		return 0, msg
	}

	// (optional) debug log of the URL we’re about to dial
	if strings.EqualFold(env("DDUI_SCAN_DEBUG", ""), "true") {
		log.Printf("scan: host=%s docker_url=%s", h.Name, url)
	}

	cli, done, err := dockerClientForURL(ctx, url, sshCmd)
	if err != nil {
		scanLog(ctx, h.ID, "error", "docker connect failed", map[string]any{"error": err.Error(), "url": url})
		return 0, err
	}
	defer done()

	args := filters.NewArgs() // All containers
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		scanLog(ctx, h.ID, "error", "container list failed", map[string]any{"error": err.Error()})
		return 0, err
	}
	count := 0
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
		}

		// infer stack/compose project name
		project := c.Labels["com.docker.compose.project"]
		if project == "" {
			project = c.Labels["com.docker.stack.namespace"]
		}

		var stackIDPtr *int64
		if project != "" {
			sid, err := ensureStack(ctx, h.ID, project, h.Owner)
			if err != nil {
				scanLog(ctx, h.ID, "warn", "ensure stack failed", map[string]any{"project": project, "error": err.Error()})
			} else {
				// make a pointer for upsertContainer; nil means “no stack”
				stackID := sid
				stackIDPtr = &stackID
			}
		}

		// ports as a generic map for UI
		ports := map[string]any{"ports": c.Ports}

		if err := upsertContainer(
			ctx,
			h.ID,
			stackIDPtr,                 // <— pointer (or nil)
			c.ID,
			trimSlash(name),
			c.Image,
			c.State,
			c.Status,
			h.Owner,
			ports,
			c.Labels,
		); err != nil {
			scanLog(ctx, h.ID, "error", "upsert container failed", map[string]any{"name": name, "id": c.ID, "error": err.Error()})
			continue
		}


		scanLog(ctx, h.ID, "info", "container discovered",
			map[string]any{"name": name, "image": c.Image, "state": c.State, "project": project})
		count++
	}
	scanLog(ctx, h.ID, "info", "scan complete", map[string]any{"containers": count})
	return count, nil
}

func trimSlash(s string) string {
	for len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	return s
}
