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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
)

// ctxManualKey marks a deploy as "manual", which bypasses Auto DevOps gating.
type ctxManualKey struct{}

func composeProjectName(stackID int64) string { return fmt.Sprintf("ddui-%d", stackID) }

// deployStack stages a mirror of the stack into a scope-aware builds dir,
// auto-decrypts any SOPS-protected env files into that stage (same names),
// then runs `docker compose -p ddui-<stackID> -f ... up -d --remove-orphans`.
//
// IMPORTANT: Non-manual invocations are **gated** by shouldAutoApply(ctx, stackID).
// Manual invocations bypass Auto DevOps (still require files to exist).
//
// Creates deployment stamps for tracking (hash is over staged compose bundle).
func deployStack(ctx context.Context, stackID int64) error {
	// Auto-DevOps gate (unless manual override)
	if man, _ := ctx.Value(ctxManualKey{}).(bool); !man {
		allowed, aerr := shouldAutoApply(ctx, stackID)
		if aerr != nil {
			return aerr
		}
		if !allowed {
			log.Printf("deploy: stack %d skipped (auto_devops says no change to apply)", stackID)
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
	// Delay cleanup so any background association/ps can still see the path.
	{
		cfn := cleanup
		defer func() {
			if cfn == nil {
				return
			}
			go func() {
				time.Sleep(60 * time.Second)
				cfn()
			}()
		}()
	}

	// Nothing to deploy?
	if len(stagedComposes) == 0 {
		log.Printf("deploy: stack %d: no compose files tracked; skipping", stackID)
		return nil
	}

	// Create deployment stamp (hash over staged compose files)
	deploymentMethod := "compose"
	deploymentUser := "" // TODO: fill from auth context if available

	var allComposeContent []byte
	for _, composeFile := range stagedComposes {
		content, rerr := os.ReadFile(composeFile)
		if rerr != nil {
			return fmt.Errorf("failed to read staged compose file %s: %w", composeFile, rerr)
		}
		allComposeContent = append(allComposeContent, content...)
	}

	stamp, err := CreateDeploymentStamp(ctx, stackID, deploymentMethod, deploymentUser, allComposeContent, nil)
	if err != nil {
		log.Printf("deploy: failed to create deployment stamp: %v", err)
		// Proceed even if stamping fails
	}

	// docker compose -p ddui-<stackID> -f <files...> up -d --remove-orphans
	project := composeProjectName(stackID)
	args := []string{"compose", "-p", project}
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

	// Mark deployment as successful
	if stamp != nil {
		if err := UpdateDeploymentStampStatus(ctx, stamp.ID, "success"); err != nil {
			log.Printf("deploy: failed to update deployment stamp status: %v", err)
		}
		// Associate containers with the deployment stamp based on Compose labels, not names.
		go func(sid, stid int64, hash string) {
			// small grace for container creation
			time.Sleep(2 * time.Second)
			associateContainersWithStamp(context.Background(), sid, stid, hash)
		}(stackID, stamp.ID, stamp.DeploymentHash)
	}

	log.Printf("deploy: stack %d deployed (compose=%d, stage=%s, repoRoot=%s, stamp=%v)",
		stackID, len(stagedComposes), stageDir, root, stamp != nil)
	return nil
}

// associateContainersWithStamp finds containers deployed by this stack (via Compose project label)
// and associates them with the deployment stamp. This works whether or not container_name is set.
func associateContainersWithStamp(ctx context.Context, stackID int64, stampID int64, deploymentHash string) {
	// Resolve host from stack scope (host-scoped only)
	var scopeKind, scopeName string
	if err := db.QueryRow(ctx, `SELECT scope_kind, scope_name FROM iac_stacks WHERE id=$1`, stackID).Scan(&scopeKind, &scopeName); err != nil {
		log.Printf("deploy: stamp assoc: load scope failed: %v", err)
		return
	}
	if strings.ToLower(scopeKind) != "host" {
		log.Printf("deploy: stamp assoc: stack %d not host-scoped (scope=%s); skipping", stackID, scopeKind)
		return
	}

	h, err := GetHostByName(ctx, scopeName)
	if err != nil {
		log.Printf("deploy: stamp assoc: get host %s: %v", scopeName, err)
		return
	}
	cli, err := dockerClientForHost(h)
	if err != nil {
		log.Printf("deploy: stamp assoc: docker client: %v", err)
		return
	}
	defer cli.Close()

	f := filters.NewArgs()
	f.Add("label", "com.docker.compose.project="+composeProjectName(stackID))
	list, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true, Filters: f})
	if err != nil {
		log.Printf("deploy: stamp assoc: list containers: %v", err)
		return
	}
	if len(list) == 0 {
		log.Printf("deploy: stamp assoc: no containers found for project=%s", composeProjectName(stackID))
	}

	for _, c := range list {
		if err := AssociateContainerWithStamp(ctx, c.ID, stampID, deploymentHash); err != nil {
			log.Printf("deploy: failed to associate container %s with stamp %d: %v", c.ID, stampID, err)
		} else {
			log.Printf("deploy: associated container %s with deployment stamp %d", c.ID, stampID)
		}
	}
}
