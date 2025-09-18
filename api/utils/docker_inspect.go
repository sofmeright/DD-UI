// src/api/utils/docker_inspect.go
package utils

import (
	"context"
	"strings"

	"github.com/docker/docker/client"
)

// DockerClientProvider interface for getting Docker clients
type DockerClientProvider interface {
	DockerClientForHost(h HostRow) (*client.Client, error)
	DockerURLFor(h HostRow) (string, string)
	DockerClientForURL(ctx context.Context, url, sshCmd string) (*client.Client, func(), error)
}

// HostRow represents a host record
type HostRow struct {
	Name string
	Addr string
	Vars map[string]string
}

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

func DockerClientForHost(provider DockerClientProvider, h HostRow) (*client.Client, error) {
	return provider.DockerClientForHost(h)
}

func InspectContainerByHost(ctx context.Context, hostProvider HostProvider, dockerProvider DockerClientProvider, hostName, container string) (*InspectOut, error) {
	h, err := hostProvider.GetHostByName(ctx, hostName)
	if err != nil {
		return nil, err
	}
	cli, err := dockerProvider.DockerClientForHost(HostRow{Name: h.Name, Addr: h.Addr, Vars: h.Vars})
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
		Image:      "",
		State:      "",
		Health:     "",
		Created:    info.Created,
		Cmd:        nil,
		Entrypoint: nil,
		Env:        map[string]string{},
		Labels:     map[string]string{},
	}

	// Config
	if info.Config != nil {
		out.Image = info.Config.Image
		out.Cmd = append([]string{}, info.Config.Cmd...)
		out.Entrypoint = append([]string{}, info.Config.Entrypoint...)
		out.Env = ToEnvMap(info.Config.Env)
		if info.Config.Labels != nil {
			out.Labels = info.Config.Labels
		}
	}

	// State / Health
	if info.State != nil {
		out.State = info.State.Status
		if info.State.Health != nil {
			out.Health = info.State.Health.Status
		}
	}

	// Restart policy
	if info.HostConfig != nil {
		out.RestartPolicy = string(info.HostConfig.RestartPolicy.Name)
	}

	// Ports
	if info.HostConfig != nil && info.HostConfig.PortBindings != nil {
		for port, bindings := range info.HostConfig.PortBindings {
			parts := strings.SplitN(string(port), "/", 2)
			target := parts[0]
			proto := ""
			if len(parts) == 2 {
				proto = parts[1]
			}
			if len(bindings) == 0 {
				out.Ports = append(out.Ports, PortBindingOut{
					Target:   target,
					Protocol: proto,
				})
				continue
			}
			for _, b := range bindings {
				pub := b.HostPort
				if b.HostIP != "" && b.HostIP != "0.0.0.0" {
					pub = b.HostIP + ":" + b.HostPort
				}
				out.Ports = append(out.Ports, PortBindingOut{
					Published: pub,
					Target:    target,
					Protocol:  proto,
				})
			}
		}
	}

	// Volumes / mounts
	for _, m := range info.Mounts {
		out.Volumes = append(out.Volumes, VolumeOut{
			Source: m.Source,
			Target: m.Destination,
			Mode:   m.Mode,
			RW:     m.RW,
		})
	}

	// Networks
	if info.NetworkSettings != nil && info.NetworkSettings.Networks != nil {
		for name := range info.NetworkSettings.Networks {
			out.Networks = append(out.Networks, name)
		}
	}

	return out, nil
}

func ToEnvMap(env []string) map[string]string {
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
