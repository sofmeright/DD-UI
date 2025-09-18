// src/api/utils/docker_action.go
package utils

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
)

func PerformContainerAction(ctx context.Context, hostProvider HostProvider, dockerProvider DockerClientProvider, hostName, ctr, action string) error {
	h, err := hostProvider.GetHostByName(ctx, hostName)
	if err != nil {
		return err
	}
	cli, err := dockerProvider.DockerClientForHost(HostRow{Name: h.Name, Addr: h.Addr, Vars: h.Vars})
	if err != nil {
		return err
	}
	defer cli.Close()

	// Default graceful timeout (seconds) for stop/restart
	sec := 10
	to := &sec

	switch action {
	case "start", "play":
		return cli.ContainerStart(ctx, ctr, container.StartOptions{})
	case "stop":
		return cli.ContainerStop(ctx, ctr, container.StopOptions{Timeout: to})
	case "kill":
		// default to SIGKILL
		return cli.ContainerKill(ctx, ctr, "KILL")
	case "restart":
		return cli.ContainerRestart(ctx, ctr, container.StopOptions{Timeout: to})
	case "pause":
		return cli.ContainerPause(ctx, ctr)
	case "unpause", "resume":
		return cli.ContainerUnpause(ctx, ctr)
	case "remove":
		return cli.ContainerRemove(ctx, ctr, container.RemoveOptions{
			Force:         true,
			RemoveVolumes: false,
		})
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

func OneShotStats(ctx context.Context, hostProvider HostProvider, dockerProvider DockerClientProvider, hostName, ctr string) (string, error) {
	h, err := hostProvider.GetHostByName(ctx, hostName)
	if err != nil {
		return "", err
	}
	cli, err := dockerProvider.DockerClientForHost(HostRow{Name: h.Name, Addr: h.Addr, Vars: h.Vars})
	if err != nil {
		return "", err
	}
	defer cli.Close()

	// Use non-streaming stats (read once)
	resp, err := cli.ContainerStats(ctx, ctr, false)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Return raw JSON body to the caller (UI shows as text)
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	for {
		n, er := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if er != nil {
			break
		}
	}
	// common.DebugLog("stats: %s len=%d", ctr, len(buf)) // Comment out - needs to be injected
	return string(buf), nil
}
