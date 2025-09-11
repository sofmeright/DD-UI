// src/api/deploy.go
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ctxManualKey marks a deploy as "manual", which bypasses Auto DevOps gating.
type ctxManualKey struct{}

// deployStack stages a mirror of the stack into a scope-aware builds dir,
// auto-decrypts any SOPS-protected env files into that stage (same names),
// then runs `docker compose up -d` with the staged compose files.
// Originals are never modified and plaintext only lives in the stage dir.
//
// IMPORTANT: Non-manual invocations are **gated** by shouldAutoDeployNow(ctx, stackID).
// Manual invocations bypass Auto DevOps (still require files to exist).
// Also records a deployment stamp keyed to the IaC bundle hash and associates
// containers by querying `docker compose ps -q` (no global --label needed).
func deployStack(ctx context.Context, stackID int64) error {
	// Auto-DevOps gate (unless manual override)
	if man, _ := ctx.Value(ctxManualKey{}).(bool); !man {
		allowed, aerr := shouldAutoDeployNow(ctx, stackID)
		if aerr != nil {
			return aerr
		}
		if !allowed {
			log.Printf("deploy: stack %d skipped (policy or no bundle changes)", stackID)
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

	// IMPORTANT: do NOT clean the stage immediately.
	// Some post-deploy code (compose ps / association) still needs this directory.
	// Defer a delayed cleanup so background tasks have time to finish.
	{
		cfn := cleanup // capture
		// When deployStack returns, schedule cleanup after a grace period.
		defer func() {
			if cfn == nil {
				return
			}
			go func() {
				// Give enough time for compose ps retries / association to run.
				// 45–90s is typically plenty; adjust if your watcher needs more.
				time.Sleep(60 * time.Second)
				cfn()
			}()
		}()
	}

	// Nothing to deploy? No-op (kept for clarity)
	if len(stagedComposes) == 0 {
		log.Printf("deploy: stack %d: no compose files tracked; skipping", stackID)
		return nil
	}

	// Compute desired bundle hash (compose/env/scripts from DB)
	bundleHash, err := ComputeCurrentBundleHash(ctx, stackID)
	if err != nil {
		return fmt.Errorf("failed to compute bundle hash: %w", err)
	}

	// Create deployment stamp (pending)
	deploymentMethod := "compose"
	deploymentUser := "" // TODO: Extract from context when available
	stamp, err := CreateDeploymentStampWithHash(ctx, stackID, deploymentMethod, deploymentUser, bundleHash, nil)
	if err != nil {
		log.Printf("deploy: failed to create deployment stamp: %v", err)
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
		if stamp != nil {
			_ = UpdateDeploymentStampStatus(ctx, stamp.ID, "failed")
		}
		log.Printf("deploy: docker compose failed: %v\n%s", err, string(out))
		return fmt.Errorf("docker compose up failed: %v\n%s", err, string(out))
	}

	// Mark deployment as successful and associate containers
	if stamp != nil {
		if err := UpdateDeploymentStampStatus(ctx, stamp.ID, "success"); err != nil {
			log.Printf("deploy: failed to update deployment stamp status: %v", err)
		}

		// Associate containers via `docker compose ps -q` with retry (no labels needed)
		go func(stampID int64, depHash string) {
			const (
				maxAttempts = 10
				delay       = 1 * time.Second
			)
			for attempt := 1; attempt <= maxAttempts; attempt++ {
				ids, perr := composeProjectContainerIDs(stageDir, stagedComposes)
				if perr != nil {
					log.Printf("deploy: compose ps failed (attempt %d/%d): %v", attempt, maxAttempts, perr)
				} else if len(ids) > 0 {
					updated, uerr := AssociateContainersWithStampIDs(context.Background(), ids, stampID, depHash)
					if uerr != nil {
						log.Printf("deploy: association update failed: %v", uerr)
					} else if updated > 0 {
						log.Printf("deploy: associated %d containers with stamp %d", updated, stampID)
						return
					}
				}
				time.Sleep(delay)
			}
			log.Printf("deploy: association retries exhausted for stamp %d", stampID)
		}(stamp.ID, stamp.DeploymentHash)
	}

	log.Printf("deploy: stack %d deployed (compose=%d, stage=%s, repoRoot=%s, stamp=%v)",
		stackID, len(stagedComposes), stageDir, root, stamp != nil)
	return nil
}

// composeProjectContainerIDs returns container IDs for the staged compose project.
func composeProjectContainerIDs(dir string, files []string) ([]string, error) {
	args := []string{"compose"}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	args = append(args, "ps", "-q")

	cmd := exec.Command("docker", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var ids []string
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			ids = append(ids, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return ids, nil
}
