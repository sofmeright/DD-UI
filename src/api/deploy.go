package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// deployStack tries to deploy a stack by running `docker compose up -d`.
// It now auto-decrypts SOPS-protected compose/env files into temp files
// (never overwriting your repo) whenever AGE keys are present.
func deployStack(ctx context.Context, stackID int64) error {
	// Figure out the working dir for compose (stack root on disk)
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

	// Collect compose & env files, transparently decrypting to temp files if needed.
	composes, envs, decCleanup, derr := prepareStackFilesForCompose(ctx, stackID)
	if derr != nil {
		return derr
	}
	defer func() { if decCleanup != nil { decCleanup() } }()

	// If nothing to deploy, keep previous "no-op" behavior.
	if len(composes) == 0 {
		log.Printf("deploy: stack %d: no compose files tracked; skipping", stackID)
		return nil
	}

	// Build: docker compose -f <compose...> --env-file <env...> up -d --remove-orphans
	args := []string{"compose"}
	for _, f := range composes {
		args = append(args, "-f", f)
	}
	for _, e := range envs {
		args = append(args, "--env-file", e)
	}
	args = append(args, "up", "-d", "--remove-orphans")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("deploy: docker compose failed: %v\n%s", err, string(out))
		return fmt.Errorf("docker compose up failed: %v\n%s", err, string(out))
	}

	log.Printf("deploy: stack %d deployed (files=%d, envs=%d)", stackID, len(composes), len(envs))
	return nil
}
