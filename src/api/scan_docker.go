// src/api/scan_docker.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// ... ErrSkipScan, guards, dockerURLFor, withSSHEnv, dockerClientForURL unchanged ...

// flatten nat.PortMap -> []{IP,PublicPort,PrivatePort,Type} for UI
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

func firstIPFromNetworks(nws map[string]*types.NetworkSettings) string {
	// NB: in inspect it's map[string]*types.EndpointSettings (older) / types.NetworkSettings? Adjust:
	// We only need IP string robustly.
	for _, v := range nws {
		if v != nil {
			// docker/types has .IPAddress directly for endpoint settings
			if ip := v.IPAddress; ip != "" {
				return ip
			}
		}
	}
	return ""
}

// ScanHostContainers connects to Docker for a host, persists containers, and prunes deletions.
func ScanHostContainers(ctx context.Context, hostName string) (int, error) {
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return 0, err
	}
	url, sshCmd := dockerURLFor(h)

	if isUnixSock(url) && !localHostAllowed(h) {
		scanLog(ctx, h.ID, "info", "skip local sock for non-local host",
			map[string]any{"url": url, "host": h.Name})
		return 0, ErrSkipScan
	}
	if strings.EqualFold(env("DDUI_SCAN_DEBUG", "false"), "true") {
		log.Printf("scan: host=%s docker_url=%s", h.Name, url)
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

		// Inspect for richer data
		ci, err := cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			scanLog(ctx, h.ID, "warn", "inspect failed", map[string]any{"id": c.ID, "error": err.Error()})
			continue
		}

		labels := map[string]string{}
		if ci.Config != nil && ci.Config.Labels != nil {
			labels = ci.Config.Labels
		}

		// project/stack
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

		// ports from inspect
		var portsOut []map[string]any
		if ci.NetworkSettings != nil {
			portsOut = flattenPorts(ci.NetworkSettings.Ports)
		}

		// IP address
		ip := ""
		if ci.NetworkSettings != nil {
			if ci.NetworkSettings.IPAddress != "" {
				ip = ci.NetworkSettings.IPAddress
			} else if ci.NetworkSettings.Networks != nil {
				// older/newer SDKs: iterate endpoint settings
				for _, ep := range ci.NetworkSettings.Networks {
					if ep != nil && ep.IPAddress != "" {
						ip = ep.IPAddress
						break
					}
				}
			}
		}

		// env as original docker []"KEY=VAL"
		var envOut []string
		if ci.Config != nil && ci.Config.Env != nil {
			envOut = ci.Config.Env
		}

		// networks+mounts raw for completeness
		var networksOut any = map[string]any{}
		if ci.NetworkSettings != nil && ci.NetworkSettings.Networks != nil {
			networksOut = ci.NetworkSettings.Networks
		}
		mountsOut := any(ci.Mounts)

		// created timestamp
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

		// container name
		name := ""
		if len(c.Names) > 0 { name = strings.TrimPrefix(c.Names[0], "/") }

		// upsert
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

	// prune stale rows
	if pruned, err := pruneMissingContainers(ctx, h.ID, seen); err == nil && pruned > 0 {
		scanLog(ctx, h.ID, "info", "pruned missing containers", map[string]any{"count": pruned})
	}

	scanLog(ctx, h.ID, "info", "scan complete", map[string]any{"containers": saved})
	return saved, nil
}
