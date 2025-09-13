// src/api/docker_actions.go
package main

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
)

func performContainerAction(ctx context.Context, hostName, ctr, action string) error {
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return err
	}
	cli, err := dockerClientForHost(h)
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

func oneShotStats(ctx context.Context, hostName, ctr string) (string, error) {
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return "", err
	}
	cli, err := dockerClientForHost(h)
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
	debugLog("stats: %s len=%d", ctr, len(buf))
	return string(buf), nil
}
