// src/api/docker_inspect.go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type InspectOut struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	State         string            `json:"state"`
	Health        string            `json:"health,omitempty"`
	Created       string            `json:"created"`
	Cmd           []string          `json:"cmd,omitempty"`
	Entrypoint    []string          `json:"entrypoint,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	RestartPolicy string            `json:"restart_policy,omitempty"`
	Ports         []PortBindingOut  `json:"ports,omitempty"`
	Volumes       []VolumeOut       `json:"volumes,omitempty"`
	Networks      []string          `json:"networks,omitempty"`
}

type PortBindingOut struct {
	Published string `json:"published,omitempty"`
	Target    string `json:"target,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
}

type VolumeOut struct {
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
	Mode   string `json:"mode,omitempty"`
	RW     bool   `json:"rw"`
}

func dockerClientForHost(h HostRow) (*client.Client, error) {
	url, err := dockerURLFor(h)
	if err != nil {
		return nil, err
	}
	return client.NewClientWithOpts(
		client.WithHost(url),
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
}

func inspectContainerByHost(ctx context.Context, hostName, container string) (*InspectOut, error) {
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, err
	}
	cli, err := dockerClientForHost(h)
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	info, err := cli.ContainerInspect(ctx, container)
	if err != nil {
		return nil, err
	}

	out := &InspectOut{
		ID:         info.ID,
		Name:       strings.TrimPrefix(info.Name, "/"),
		Image:      info.Config.Image,
		State:      "",
		Health:     "",
		Created:    info.Created,
		Cmd:        []string(info.Config.Cmd),
		Entrypoint: []string(info.Config.Entrypoint),
		Env:        toEnvMap(info.Config.Env),
		Labels:     info.Config.Labels,
	}

	if info.State != nil {
		out.State = info.State.Status
		if info.State.Health != nil {
			out.Health = info.State.Health.Status
		}
	}

	if info.HostConfig != nil {
		out.RestartPolicy = info.HostConfig.RestartPolicy.Name
	}

	// Ports: HostConfig.PortBindings
	var ports []PortBindingOut
	if info.HostConfig != nil && info.HostConfig.PortBindings != nil {
		for port, bindings := range info.HostConfig.PortBindings {
			target := string(port)
			proto := ""
			if p, err := nat.ParsePort(target); err == nil {
				proto = string(p.Proto())
				target = fmt.Sprintf("%d/%s", p.Int(), p.Proto())
			}
			if len(bindings) == 0 {
				ports = append(ports, PortBindingOut{Target: target, Protocol: proto})
			}
			for _, b := range bindings {
				ports = append(ports, PortBindingOut{
					Published: b.HostIPPort(),
					Target:    target,
					Protocol:  proto,
				})
			}
		}
	}
	out.Ports = ports

	// Volumes / Mounts
	var vols []VolumeOut
	for _, m := range info.Mounts {
		vols = append(vols, VolumeOut{
			Source: m.Source,
			Target: m.Destination,
			Mode:   m.Mode,
			RW:     m.RW,
		})
	}
	out.Volumes = vols

	// Networks
	if info.NetworkSettings != nil {
		nets := make([]string, 0, len(info.NetworkSettings.Networks))
		for name := range info.NetworkSettings.Networks {
			nets = append(nets, name)
		}
		out.Networks = nets
	}

	return out, nil
}

func toEnvMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, e := range env {
		kv := strings.SplitN(e, "=", 2)
		if len(kv) == 2 {
			out[kv[0]] = kv[1]
		} else if kv[0] != "" {
			out[kv[0]] = ""
		}
	}
	return out
}
