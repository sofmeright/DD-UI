// docker_client_helper.go
package services

import (
	"context"
	"dd-ui/database"
	"github.com/docker/docker/client"
)

// DockerClientForHost creates a Docker client for a specific host
// This is the single-parameter version used throughout the codebase
func DockerClientForHost(h database.HostRow) (*client.Client, error) {
	url, sshCmd := DockerURLFor(h)
	cli, _, err := DockerClientForURL(context.Background(), url, sshCmd)
	return cli, err
}