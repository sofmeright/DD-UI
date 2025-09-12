// src/api/scan_docker.go
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// ===== helpers =====

var (
	ErrSkipScan = errors.New("skip scan") // sentinel for intentional skip
	sshEnvMu    sync.Mutex
)

func isUnixSock(url string) bool { return strings.HasPrefix(url, "unix://") }

func localHostAllowed(h HostRow) bool {
	// 1) per-host opt-in (inventory var)
	if v := strings.ToLower(strings.TrimSpace(h.Vars["docker_local"])); v == "true" || v == "1" || v == "yes" {
		return true
	}
	// 2) env mapping
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
	// per-host override
	if v := h.Vars["docker_host"]; v != "" {
		return v, h.Vars["docker_ssh_cmd"]
	}
	kind := env("DOCKER_CONNECTION_METHOD", "ssh") // ssh|tcp|local
	switch kind {
	case "local":
		sock := env("DOCKER_SOCK_PATH", "/var/run/docker.sock")
		return "unix://" + sock, ""
	case "tcp":
		host := h.Addr
		if host == "" {
			host = h.Name
		}
		port := env("DOCKER_TCP_PORT", "2375")
		return fmt.Sprintf("tcp://%s:%s", host, port), ""
	default: // ssh
		user := h.Vars["ansible_user"]
		if user == "" {
			user = env("SSH_USER", "root")
		}
		addr := h.Addr
		if addr == "" {
			addr = h.Name
		}
		
		// Build SSH command for Docker CLI SSH support
		sshCmd := "ssh"
		if keyFile := env("SSH_KEY_FILE", ""); keyFile != "" {
			sshCmd += " -i " + keyFile
		}
		if env("SSH_STRICT_HOST_KEY", "true") == "false" {
			sshCmd += " -o StrictHostKeyChecking=no"
		}
		if port := env("SSH_PORT", ""); port != "" && port != "22" {
			sshCmd += " -p " + port
		}
		
		// Return SSH URL format that Docker CLI expects
		return fmt.Sprintf("ssh://%s@%s", user, addr), sshCmd
	}
}

func withSSHEnv(cmd string, fn func() error) error {
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

func dockerClientForURL(ctx context.Context, url, sshCmd string) (*client.Client, func(), error) {
	var cli *client.Client
	
	// Handle SSH connections with proper SSH transport
	if strings.HasPrefix(url, "ssh://") {
		debugLog("SSH connection detected, using SSH transport")
		
		// Parse SSH URL to extract user and host
		user, host, err := parseSSHURL(url)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid SSH URL: %v", err)
		}
		
		// Get SSH key file from environment
		keyFile := env("SSH_KEY_FILE", "")
		if keyFile == "" {
			return nil, nil, fmt.Errorf("SSH_KEY_FILE not configured")
		}
		
		// Create Docker client with SSH transport
		cli, cleanup, err := createSSHDockerClient(user, host, keyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create SSH Docker client: %v", err)
		}
		
		// Test connection
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		_, err = cli.Ping(pctx)
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("SSH Docker connection test failed: %v", err)
		}
		
		debugLog("SSH Docker client created successfully for %s@%s", user, host)
		return cli, cleanup, nil
	}
	
	err := withSSHEnv(sshCmd, func() error {
		var err error
		
		// Set DOCKER_HOST environment variable
		prevHost := os.Getenv("DOCKER_HOST")
		if url != "" {
			os.Setenv("DOCKER_HOST", url)
		}
		defer func() {
			if prevHost != "" {
				os.Setenv("DOCKER_HOST", prevHost)
			} else if url != "" {
				os.Unsetenv("DOCKER_HOST")
			}
		}()
		
		cli, err = client.NewClientWithOpts(
			client.FromEnv,
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
	return cli, func() { _ = cli.Close() }, nil
}

// ===== small utils =====

func flattenPorts(pm nat.PortMap) []map[string]any {
	out := make([]map[string]any, 0, len(pm))
	for port, binds := range pm {
		privateStr := port.Port()
		private, _ := strconv.Atoi(privateStr)
		typ := string(port.Proto())
		if len(binds) == 0 {
			out = append(out, map[string]any{
				"IP": "", "PublicPort": 0, "PrivatePort": private, "Type": typ,
			})
			continue
		}
		for _, b := range binds {
			pub, _ := strconv.Atoi(b.HostPort)
			out = append(out, map[string]any{
				"IP": b.HostIP, "PublicPort": pub, "PrivatePort": private, "Type": typ,
			})
		}
	}
	return out
}

// ===== main scan =====

func ScanHostContainers(ctx context.Context, hostName string) (int, error) {
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return 0, err
	}
	url, sshCmd := dockerURLFor(h)

	// refuse scanning many hosts through a single local sock
	if isUnixSock(url) && !localHostAllowed(h) {
		scanLog(ctx, h.ID, "info", "skip local sock for non-local host",
			map[string]any{"url": url, "host": h.Name})
		return 0, ErrSkipScan
	}
	if strings.EqualFold(env("DDUI_SCAN_DOCKER_DEBUG", "false"), "true") {
		infoLog("scan: host=%s docker_url=%s", h.Name, url)
	}

	cli, done, err := dockerClientForURL(ctx, url, sshCmd)
	if err != nil {
		scanLog(ctx, h.ID, "error", "docker connect failed", map[string]any{"error": err.Error(), "url": url})
		return 0, err
	}
	defer done()

	list, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: filters.NewArgs()})
	if err != nil {
		scanLog(ctx, h.ID, "error", "container list failed", map[string]any{"error": err.Error()})
		return 0, err
	}

	seen := make([]string, 0, len(list))
	saved := 0

	for _, c := range list {
		seen = append(seen, c.ID)

		ci, err := cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			scanLog(ctx, h.ID, "warn", "inspect failed", map[string]any{"id": c.ID, "error": err.Error()})
			continue
		}

		labels := map[string]string{}
		if ci.Config != nil && ci.Config.Labels != nil {
			labels = ci.Config.Labels
		}

		project := labels["com.docker.compose.project"]
		if project == "" {
			project = labels["com.docker.stack.namespace"]
		}
		var stackIDPtr *int64
		if project != "" {
			if sid, err := ensureStack(ctx, h.ID, project, h.Owner); err == nil {
				stackID := sid
				stackIDPtr = &stackID
			} else {
				scanLog(ctx, h.ID, "warn", "ensure stack failed", map[string]any{"project": project, "error": err.Error()})
			}
		}

		// ports/IP/env/networks/mounts/created
		var portsOut []map[string]any
		ip := ""
		if ci.NetworkSettings != nil {
			portsOut = flattenPorts(ci.NetworkSettings.Ports)
			if ci.NetworkSettings.IPAddress != "" {
				ip = ci.NetworkSettings.IPAddress
			} else if ci.NetworkSettings.Networks != nil {
				for _, ep := range ci.NetworkSettings.Networks {
					if ep != nil && ep.IPAddress != "" {
						ip = ep.IPAddress
						break
					}
				}
			}
		}
		var envOut []string
		if ci.Config != nil && ci.Config.Env != nil {
			envOut = ci.Config.Env
		}
		var networksOut any = map[string]any{}
		if ci.NetworkSettings != nil && ci.NetworkSettings.Networks != nil {
			networksOut = ci.NetworkSettings.Networks
		}
		mountsOut := any(ci.Mounts)

		var createdPtr *time.Time
		if ci.Created != "" {
			if t, err := time.Parse(time.RFC3339Nano, ci.Created); err == nil {
				createdPtr = &t
			}
		}
		if createdPtr == nil && c.Created > 0 {
			t := time.Unix(c.Created, 0).UTC()
			createdPtr = &t
		}

		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		if err := upsertContainer(
			ctx, h.ID, stackIDPtr, c.ID, name, c.Image, c.State, c.Status, h.Owner,
			createdPtr, ip, portsOut, labels, envOut, networksOut, mountsOut,
		); err != nil {
			scanLog(ctx, h.ID, "error", "upsert container failed", map[string]any{"name": name, "id": c.ID, "error": err.Error()})
			continue
		}

		saved++
		scanLog(ctx, h.ID, "info", "container discovered",
			map[string]any{"name": name, "image": c.Image, "state": c.State, "project": project})
	}

	// prune gone containers
	if pruned, err := pruneMissingContainers(ctx, h.ID, seen); err == nil && pruned > 0 {
		scanLog(ctx, h.ID, "info", "pruned missing containers", map[string]any{"count": pruned})
	}

	scanLog(ctx, h.ID, "info", "scan complete", map[string]any{"containers": saved})
	return saved, nil
}
