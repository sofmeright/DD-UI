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
	"time"
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
// 
// Creates deployment stamps for tracking and drift detection.
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

	// Create deployment stamp for tracking
	deploymentMethod := "compose"
	deploymentUser := "" // TODO: Extract from context when available
	
	// Read staged compose files to generate deployment hash
	var allComposeContent []byte
	for _, composeFile := range stagedComposes {
		content, err := os.ReadFile(composeFile)
		if err != nil {
			return fmt.Errorf("failed to read staged compose file %s: %v", composeFile, err)
		}
		allComposeContent = append(allComposeContent, content...)
	}

	stamp, err := CreateDeploymentStamp(ctx, stackID, deploymentMethod, deploymentUser, allComposeContent, nil)
	if err != nil {
		log.Printf("deploy: failed to create deployment stamp: %v", err)
		// Continue deployment even if stamp creation fails
	}

	// docker compose -f <files...> up -d --remove-orphans
	args := []string{"compose"}
	for _, f := range stagedComposes {
		args = append(args, "-f", f)
	}
	
	// Add deployment stamp labels to all containers
	if stamp != nil {
		args = append(args, "--label", fmt.Sprintf("ddui.deployment.stamp_id=%d", stamp.ID))
		args = append(args, "--label", fmt.Sprintf("ddui.deployment.hash=%s", stamp.DeploymentHash))
		args = append(args, "--label", fmt.Sprintf("ddui.deployment.timestamp=%s", stamp.DeploymentTimestamp.Format("2006-01-02T15:04:05Z")))
		args = append(args, "--label", fmt.Sprintf("ddui.stack.id=%d", stackID))
	}
	
	args = append(args, "up", "-d", "--remove-orphans")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		// Mark deployment as failed if we have a stamp
		if stamp != nil {
			_ = UpdateDeploymentStampStatus(ctx, stamp.ID, "failed")
		}
		log.Printf("deploy: docker compose failed: %v\n%s", err, string(out))
		return fmt.Errorf("docker compose up failed: %v\n%s", err, string(out))
	}

	// Mark deployment as successful
	if stamp != nil {
		if err := UpdateDeploymentStampStatus(ctx, stamp.ID, "success"); err != nil {
			log.Printf("deploy: failed to update deployment stamp status: %v", err)
		}
		
		// Associate containers with the deployment stamp
		go func() {
			// Small delay to allow containers to be created
			time.Sleep(2 * time.Second)
			associateContainersWithStamp(context.Background(), stackID, stamp.ID, stamp.DeploymentHash)
		}()
	}

	log.Printf("deploy: stack %d deployed (compose=%d, stage=%s, repoRoot=%s, stamp=%v)", 
		stackID, len(stagedComposes), stageDir, root, stamp != nil)
	return nil
}

// associateContainersWithStamp finds containers deployed by this stack and associates them with the deployment stamp
func associateContainersWithStamp(ctx context.Context, stackID int64, stampID int64, deploymentHash string) {
	// Query containers that belong to this stack and were recently created
	rows, err := db.Query(ctx, `
		SELECT container_id 
		FROM containers 
		WHERE stack_id = $1 
		  AND (deployment_stamp_id IS NULL OR deployment_stamp_id != $2)
		  AND created_ts > now() - interval '5 minutes'
	`, stackID, stampID)
	if err != nil {
		log.Printf("deploy: failed to query containers for stamp association: %v", err)
		return
	}
	defer rows.Close()

	var containerIDs []string
	for rows.Next() {
		var containerID string
		if err := rows.Scan(&containerID); err != nil {
			continue
		}
		containerIDs = append(containerIDs, containerID)
	}

	// Associate each container with the deployment stamp
	for _, containerID := range containerIDs {
		if err := AssociateContainerWithStamp(ctx, containerID, stampID, deploymentHash); err != nil {
			log.Printf("deploy: failed to associate container %s with stamp %d: %v", containerID, stampID, err)
		} else {
			log.Printf("deploy: associated container %s with deployment stamp %d", containerID, stampID)
		}
	}
}
