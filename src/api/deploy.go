// src/api/deploy.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// ctxManualKey marks a deploy as "manual", which bypasses Auto DevOps gating.
type ctxManualKey struct{}

// deployStack stages a mirror of the stack into a scope-aware builds dir,
// auto-decrypts any SOPS-protected env files into that stage (same names),
// then runs `docker compose up -d` with the staged compose files.
// Originals are never modified and plaintext only lives in the stage dir.
//
// IMPORTANT: Non-manual invocations are **gated** by shouldAutoApply(ctx, stackID).
// Manual invocations bypass Auto DevOps (still require files to exist).
func deployStack(ctx context.Context, stackID int64) error {
	// Auto-DevOps gate (unless manual override)
	if man, _ := ctx.Value(ctxManualKey{}).(bool); !man {
		allowed, aerr := shouldAutoApply(ctx, stackID)
		if aerr != nil {
			return aerr
		}
		if !allowed {
			log.Printf("deploy: stack %d skipped (auto_devops disabled by effective policy)", stackID)
			return nil
		}
	}

	// Working dir for compose (stack root on disk)
	root, err := getRepoRootForStack(ctx, stackID)
	if err != nil {
		return err
	}
	var rel string
	_ = db.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	if strings.TrimSpace(rel) == "" {
		return errors.New("deploy: stack has no rel_path")
	}

	// Stage files for compose
	stageDir, stagedComposes, cleanup, derr := stageStackForCompose(ctx, stackID)
	if derr != nil {
		return derr
	}
	defer func() { if cleanup != nil { cleanup() } }()

	// Nothing to deploy? No-op (kept for clarity)
	if len(stagedComposes) == 0 {
		log.Printf("deploy: stack %d: no compose files tracked; skipping", stackID)
		return nil
	}

	// docker compose -f <files...> up -d --remove-orphans
	args := []string{"compose"}
	for _, f := range stagedComposes {
		args = append(args, "-f", f)
	}
	args = append(args, "up", "-d", "--remove-orphans")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("deploy: docker compose failed: %v\n%s", err, string(out))
		return fmt.Errorf("docker compose up failed: %v\n%s", err, string(out))
	}

	log.Printf("deploy: stack %d deployed (compose=%d, stage=%s, repoRoot=%s)", stackID, len(stagedComposes), stageDir, root)
	return nil
}
