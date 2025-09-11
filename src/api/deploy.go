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

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
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
// We DO NOT run any follow-up `docker compose` commands after `up`.
// Post-deploy stamping/association is done by inspecting containers via the Docker SDK.
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
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	// Nothing to deploy? No-op (kept for clarity)
	if len(stagedComposes) == 0 {
		log.Printf("deploy: stack %d: no compose files tracked; skipping", stackID)
		return nil
	}

	// Derive a stable Compose project name so we can find containers by label
	projectName, err := deriveComposeProjectName(ctx, stackID)
	if err != nil || projectName == "" {
		// Fall back to directory-based default if anything goes wrong
		projectName = "ddui_" + fmt.Sprint(stackID)
	}

	// Create deployment stamp for tracking (best effort)
	// We hash the concatenated staged compose content (same as before).
	var allComposeContent []byte
	for _, composeFile := range stagedComposes {
		content, rerr := os.ReadFile(composeFile)
		if rerr != nil {
			return fmt.Errorf("failed to read staged compose file %s: %v", composeFile, rerr)
		}
		allComposeContent = append(allComposeContent, content...)
	}
	stamp, serr := CreateDeploymentStamp(ctx, stackID, "compose", /*user*/"", allComposeContent, nil)
	if serr != nil {
		log.Printf("deploy: failed to create deployment stamp: %v", serr)
		// Continue deployment even if stamp creation fails
	}

	// Build: docker compose -p <project> -f <files...> up -d --remove-orphans
	args := []string{"compose", "-p", projectName}
	for _, f := range stagedComposes {
		args = append(args, "-f", f)
	}
	args = append(args, "up", "-d", "--remove-orphans")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		// Mark deployment as failed if we created a stamp
		if stamp != nil {
			_ = UpdateDeploymentStampStatus(ctx, stamp.ID, "failed")
		}
		// Log full output so we can see the reason
		log.Printf("deploy: docker compose failed: %v\n----\n%s\n----", err, string(out))
		return fmt.Errorf("docker compose up failed: %v\n%s", err, string(out))
	}

	// Mark deployment as successful and associate containers by inspecting labels.
	if stamp != nil {
		if uerr := UpdateDeploymentStampStatus(ctx, stamp.ID, "success"); uerr != nil {
			log.Printf("deploy: failed to update deployment stamp status: %v", uerr)
		}
		// Retry a few times to give containers time to appear
		go func(pj string, stampID int64, depHash string) {
			backoff := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 5 * time.Second}
			for i := 0; i < len(backoff); i++ {
				if i > 0 {
					time.Sleep(backoff[i])
				}
				if err := associateByProjectInspect(context.Background(), pj, stampID, depHash); err == nil {
					return
				}
			}
			// last attempt without backoff logging
			if err := associateByProjectInspect(context.Background(), pj, stampID, depHash); err != nil {
				log.Printf("deploy: association (inspect) still failing for project=%s: %v", pj, err)
			}
		}(projectName, stamp.ID, stamp.DeploymentHash)
	}

	log.Printf("deploy: stack %d deployed (compose=%d, stage=%s, repoRoot=%s, stamp=%v)",
		stackID, len(stagedComposes), stageDir, root, stamp != nil)

	return nil
}

// associateByProjectInspect finds containers via the Compose project label and stamps them.
func associateByProjectInspect(ctx context.Context, project string, stampID int64, deploymentHash string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()

	flt := filters.NewArgs()
	flt.Add("label", "com.docker.compose.project="+project)

	list, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: flt,
	})
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return fmt.Errorf("no containers yet for compose project=%s", project)
	}

	var assocErrs int
	for _, c := range list {
		if e := AssociateContainerWithStamp(ctx, c.ID, stampID, deploymentHash); e != nil {
			assocErrs++
			log.Printf("deploy: failed to associate container %s with stamp %d: %v", c.ID, stampID, e)
		}
	}
	if assocErrs > 0 {
		return fmt.Errorf("associated %d/%d containers (some failed)", len(list)-assocErrs, len(list))
	}
	return nil
}
