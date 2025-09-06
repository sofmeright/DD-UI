// deploy.go
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// deployStack tries to deploy a stack by running `docker compose up -d`
// if it finds a compose file under the stack's directory. If none is
// found, it logs and returns nil (no-op) so callers don't fail.
func deployStack(ctx context.Context, stackID int64) error {
	root, err := getRepoRootForStack(ctx, stackID)
	if err != nil {
		return err
	}

	var rel string
	_ = db.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	if strings.TrimSpace(rel) == "" {
		return errors.New("deploy: stack has no rel_path")
	}

	dir, err := joinUnder(root, rel)
	if err != nil {
		return err
	}

	// common compose filenames to try
	candidates := []string{
		"docker-compose.yaml",
		"docker-compose.yml",
		"compose.yaml",
		"compose.yml",
		filepath.Join("docker-compose", "docker-compose.yaml"),
		filepath.Join("docker-compose", "docker-compose.yml"),
	}

	var composePath string
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			composePath = p
			break
		}
	}

	if composePath == "" {
		log.Printf("deploy: stack %d: no compose file found under %s; skipping", stackID, dir)
		return nil // no-op is not an error
	}

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "up", "-d")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("deploy: docker compose failed: %v\n%s", err, string(out))
		return err
	}

	log.Printf("deploy: stack %d deployed via %s", stackID, composePath)
	return nil
}
