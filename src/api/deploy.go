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

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// ctxManualKey marks a deploy as "manual", which bypasses Auto DevOps gating.
type ctxManualKey struct{}

// deployStack: stage -> docker compose up -d (project = stack name) -> stamp -> associate via labels.
func deployStack(ctx context.Context, stackID int64) error {
	// Auto-DevOps gate (unless manual)
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

	// Resolve root and rel path
	root, err := getRepoRootForStack(ctx, stackID)
	if err != nil {
		return err
	}
	var rel string
	_ = db.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	if strings.TrimSpace(rel) == "" {
		return errors.New("deploy: stack has no rel_path")
	}

	// Stage compose files (and decrypt .env if applicable)
	stageDir, stagedComposes, cleanup, derr := stageStackForCompose(ctx, stackID)
	if derr != nil {
		return derr
	}
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if len(stagedComposes) == 0 {
		log.Printf("deploy: stack %d: no compose files tracked; skipping", stackID)
		return nil
	}

	// Derive Compose project label (normalized) from scope+stack
	projectName := deriveComposeProjectName(ctx, stackID)
	if strings.TrimSpace(projectName) == "" {
		projectName = fmt.Sprintf("ddui_%d", stackID)
	}

	// Build a deployment stamp (best effort) from staged compose content.
	var allComposeContent []byte
	for _, f := range stagedComposes {
		b, rerr := os.ReadFile(f)
		if rerr != nil {
			return fmt.Errorf("failed to read staged compose file %s: %v", f, rerr)
		}
		allComposeContent = append(allComposeContent, b...)
	}
	stamp, serr := CreateDeploymentStamp(ctx, stackID, "compose", "", allComposeContent, nil)
	if serr != nil {
		log.Printf("deploy: failed to create deployment stamp: %v", serr)
	}

	// docker compose -p <project> -f ... up -d --remove-orphans
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
		if stamp != nil {
			_ = UpdateDeploymentStampStatus(ctx, stamp.ID, "failed")
		}
		log.Printf("deploy: docker compose failed: %v\n----\n%s\n----", err, string(out))
		return fmt.Errorf("docker compose up failed: %v\n%s", err, string(out))
	}

	// Mark success and associate containers by project label.
	if stamp != nil {
		if uerr := UpdateDeploymentStampStatus(ctx, stamp.ID, "success"); uerr != nil {
			log.Printf("deploy: failed to update deployment stamp status: %v", uerr)
		}
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
			if err := associateByProjectInspect(context.Background(), pj, stamp.ID, stamp.DeploymentHash); err != nil {
				log.Printf("deploy: association (inspect) still failing for project=%s: %v", pj, err)
			}
		}(projectName, stamp.ID, stamp.DeploymentHash)
	}

	log.Printf("deploy: stack %d deployed (compose=%d, stage=%s, repoRoot=%s, stamp=%v)",
		stackID, len(stagedComposes), stageDir, root, stamp != nil)

	return nil
}

// associateByProjectInspect stamps all containers with the given Compose project.
func associateByProjectInspect(ctx context.Context, project string, stampID int64, deploymentHash string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()

	flt := filters.NewArgs()
	flt.Add("label", "com.docker.compose.project="+project)

	list, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: flt})
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
