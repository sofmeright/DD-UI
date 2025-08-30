package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

var sshEnvMu sync.Mutex

func dockerURLFor(h HostRow) (string, string) {
	// Prefer explicit var from inventory: docker_host: ssh://user@host or tcp://...
	if v := h.Vars["docker_host"]; v != "" {
		return v, h.Vars["docker_ssh_cmd"]
	}

	kind := env("DDUI_SCAN_KIND", "ssh") // "ssh" | "tcp" | "local"
	switch kind {
	case "local":
		return "unix:///var/run/docker.sock", ""
	case "tcp":
		// requires dockerd to listen on TCP (optionally with TLS)
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
	cli, done, err := dockerClientFor(ctx, h)
	if err != nil {
		scanLog(ctx, h.ID, "error", "docker connect failed", map[string]any{"error": err.Error()})
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
