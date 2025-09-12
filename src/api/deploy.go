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

// deployStack: stage -> (optional: compute config-hash) -> docker compose up -d
// (-p = EXACT stack name) -> stamp -> associate via label(sanitized).
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

	// Resolve raw project name (as user typed) + label form for lookups
	rawProjectName, err := fetchStackName(ctx, stackID)
	if err != nil || strings.TrimSpace(rawProjectName) == "" {
		return errors.New("deploy: could not resolve stack name")
	}
	labelProject := composeProjectLabelFromStack(rawProjectName)

	// Working dir and rel path
	root, err := getRepoRootForStack(ctx, stackID)
	if err != nil {
		return err
	}
	var rel string
	_ = db.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	if strings.TrimSpace(rel) == "" {
		return errors.New("deploy: stack has no rel_path")
	}

	// Stage (SOPS decrypts into tmpfs and is cleaned afterwards)
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

	// Precompute rendered config-hash (best effort; used as stamp metadata)
	renderedCfgHash := computeRenderedConfigHash(ctx, stageDir, rawProjectName, stagedComposes)

	// Build a deployment stamp (content bytes = concatenated staged compose files).
	var allComposeContent []byte
	for _, f := range stagedComposes {
		b, rerr := os.ReadFile(f)
		if rerr != nil {
			return fmt.Errorf("failed to read staged compose file %s: %v", f, rerr)
		}
		allComposeContent = append(allComposeContent, b...)
	}

	var meta map[string]string
	if renderedCfgHash != "" {
		meta = map[string]string{"rendered_config_hash": renderedCfgHash}
	}
	stamp, serr := CreateDeploymentStamp(ctx, stackID, "compose", "", allComposeContent, meta)
	if serr != nil {
		log.Printf("deploy: failed to create deployment stamp: %v", serr)
	}

	// docker compose -p <RAW stack name> -f ... up -d --remove-orphans
	args := []string{"compose", "-p", rawProjectName}
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

	// Mark success and associate by Compose label (sanitized form).
	if stamp != nil {
		if uerr := UpdateDeploymentStampStatus(ctx, stamp.ID, "success"); uerr != nil {
			log.Printf("deploy: failed to update deployment stamp status: %v", uerr)
		}
		go func(label string, stampID int64, depHash string) {
			backoff := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 5 * time.Second}
			for i := 0; i < len(backoff); i++ {
				if i > 0 {
					time.Sleep(backoff[i])
				}
				if err := associateByProjectInspect(context.Background(), label, stampID, depHash); err == nil {
					return
				}
			}
			if err := associateByProjectInspect(context.Background(), label, stampID, depHash); err != nil {
				log.Printf("deploy: association (inspect) still failing for project=%s: %v", label, err)
			}
		}(labelProject, stamp.ID, stamp.DeploymentHash)
	}

	log.Printf("deploy: stack %d deployed (compose=%d, stage=%s, repoRoot=%s, stamp=%v)",
		stackID, len(stagedComposes), stageDir, root, stamp != nil)

	return nil
}

// associateByProjectInspect stamps all containers with the given Compose project label value.
func associateByProjectInspect(ctx context.Context, projectLabel string, stampID int64, deploymentHash string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()

	flt := filters.NewArgs()
	flt.Add("label", "com.docker.compose.project="+projectLabel)

	list, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: flt})
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return fmt.Errorf("no containers yet for compose project=%s", projectLabel)
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
