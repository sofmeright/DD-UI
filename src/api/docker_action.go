// src/api/docker_actions.go
package main

import (
	"context"
	"errors"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

func doContainerAction(ctx context.Context, host, ctr, action string) error {
	h, err := GetHostByName(ctx, host)
	if err != nil {
		return err
	}
	cli, err := dockerClientForHost(h)
	if err != nil {
		return err
	}
	defer cli.Close()

	switch action {
	case "start", "play":
		return cli.ContainerStart(ctx, ctr, types.ContainerStartOptions{})
	case "stop":
		timeout := 10 * time.Second
		return cli.ContainerStop(ctx, ctr, container.StopOptions{Timeout: &timeout})
	case "kill":
		sig := "KILL"
		return cli.ContainerKill(ctx, ctr, sig)
	case "restart":
		timeout := 10 * time.Second
		return cli.ContainerRestart(ctx, ctr, &timeout)
	case "pause":
		return cli.ContainerPause(ctx, ctr)
	case "resume", "unpause":
		return cli.ContainerUnpause(ctx, ctr)
	case "remove", "rm":
		return cli.ContainerRemove(ctx, ctr, types.ContainerRemoveOptions{Force: false, RemoveVolumes: false})
	default:
		return errors.New("unknown action")
	}
}
