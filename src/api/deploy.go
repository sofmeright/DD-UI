// src/api/deploy.go
// src/api/deploy.go
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// getHostForStack retrieves the host information for a given stack ID
func getHostForStack(ctx context.Context, stackID int64) (HostRow, error) {
	var host HostRow
	err := db.QueryRow(ctx, `
		SELECT h.id, h.name, h.addr, h.vars
		FROM iac_stacks s
		JOIN hosts h ON (s.scope_kind='host' AND s.scope_name=h.name)
		WHERE s.id = $1
	`, stackID).Scan(&host.ID, &host.Name, &host.Addr, &host.Vars)
	return host, err
}

// ctxManualKey marks a deploy as "manual", which bypasses Auto DevOps gating.
type ctxManualKey struct{}

// ctxForceKey marks a deploy as "forced", which bypasses configuration unchanged checks.
type ctxForceKey struct{}

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
			infoLog("deploy: stack %d skipped (auto_devops disabled by effective policy)", stackID)
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
		infoLog("deploy: stack %d: no compose files tracked; skipping", stackID)
		return nil
	}

	// Precompute rendered config-hash + bundle hash (best effort; for stamping/drift).
	renderedCfgHash := computeRenderedConfigHash(ctx, stageDir, rawProjectName, stagedComposes)
	bundleHash, _ := ComputeCurrentBundleHash(ctx, stackID)

	// Build a deployment stamp (content bytes = concatenated staged compose files).
	// We also pass metadata (string map).
	meta := map[string]string{
		"rendered_config_hash": renderedCfgHash,
		"bundle_hash":          bundleHash,
	}
	var allComposeContent []byte
	for _, f := range stagedComposes {
		b, rerr := os.ReadFile(f)
		if rerr != nil {
			return fmt.Errorf("failed to read staged compose file %s: %v", f, rerr)
		}
		allComposeContent = append(allComposeContent, b...)
		allComposeContent = append(allComposeContent, '\n')
	}
	stamp, serr := CreateDeploymentStamp(ctx, stackID, "compose", "", allComposeContent, meta)
	if serr != nil {
		errorLog("deploy: failed to create deployment stamp: %v", serr)
		// If stamp creation fails due to unique constraint, try to find the existing one
		if existingStamp, findErr := CheckDeploymentStampExists(ctx, stackID, allComposeContent); findErr == nil && existingStamp != nil {
			debugLog("deploy: reusing existing deployment stamp %d", existingStamp.ID)
			stamp = existingStamp
		} else {
			debugLog("deploy: could not find existing stamp either: %v", findErr)
			return serr
		}
	}
	
	if stamp == nil || stamp.ID == 0 {
		errorLog("deploy: CRITICAL - stamp is nil or has ID 0 after creation (stamp=%v)", stamp)
		return fmt.Errorf("deployment stamp creation failed - invalid stamp ID")
	}
	debugLog("deploy: created/found stamp with ID %d for stack %d", stamp.ID, stackID)

	// docker compose -p <RAW stack name> -f ... up -d --remove-orphans
	args := []string{"compose", "-p", rawProjectName}
	for _, f := range stagedComposes {
		args = append(args, "-f", f)
	}
	args = append(args, "up", "-d", "--remove-orphans")

	// Get the host info to set up proper Docker connection
	var dockerEnv []string
	if host, herr := getHostForStack(ctx, stackID); herr == nil {
		dockerURL, sshCmd := dockerURLFor(host)
		dockerEnv = append(os.Environ(), "DOCKER_HOST="+dockerURL)
		if sshCmd != "" {
			dockerEnv = append(dockerEnv, "DOCKER_SSH_CMD="+sshCmd)
		}
		debugLog("deploy: using Docker host %s for stack %d", dockerURL, stackID)
	} else {
		errorLog("deploy: failed to get host for stack %d, using default Docker connection: %v", stackID, herr)
		dockerEnv = os.Environ()
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	cmd.Env = dockerEnv

	out, err := cmd.CombinedOutput()
	if err != nil {
		if stamp != nil {
			_ = UpdateDeploymentStampStatus(ctx, stamp.ID, "failed")
		}
		errorLog("deploy: docker compose failed: %v\n----\n%s\n----", err, string(out))
		return fmt.Errorf("docker compose up failed: %v\n%s", err, string(out))
	}

	// Mark success and associate by Compose label (sanitized form).
	if stamp != nil {
		if uerr := UpdateDeploymentStampStatus(ctx, stamp.ID, "success"); uerr != nil {
			errorLog("deploy: failed to update deployment stamp status: %v", uerr)
		}
		
		// Update drift cache after successful deployment
		if host, herr := getHostForStack(ctx, stackID); herr == nil {
			dockerURL, sshCmd := dockerURLFor(host)
			if dcli, done, derr := dockerClientForURL(ctx, dockerURL, sshCmd); derr == nil {
				if cerr := onSuccessfulDeployment(ctx, stackID, rawProjectName, dcli); cerr != nil {
					errorLog("deploy: failed to update drift cache: %v", cerr)
				}
				if done != nil {
					done()
				}
			}
		}
		go func(label string, stampID int64, depHash string) {
          // depHash is the stamp.DeploymentHash (content hash)
			backoff := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 5 * time.Second}
			for i := 0; i < len(backoff); i++ {
				if i > 0 {
					time.Sleep(backoff[i])
				}
				if err := associateByProjectInspect(context.Background(), label, stampID, depHash, stackID); err == nil {
					return
				}
			}
			if err := associateByProjectInspect(context.Background(), label, stampID, depHash, stackID); err != nil {
				errorLog("deploy: association (inspect) still failing for project=%s: %v", label, err)
			}
		}(labelProject, stamp.ID, stamp.DeploymentHash)
	}

	infoLog("deploy: stack %d deployed (compose=%d, stage=%s, repoRoot=%s, stamp=%v)",
		stackID, len(stagedComposes), stageDir, root, stamp != nil)

	return nil
}

// deployStackWithStream performs deployment while streaming docker compose output
func deployStackWithStream(ctx context.Context, stackID int64, eventChannel chan<- map[string]interface{}) error {
	defer close(eventChannel)
	
	sendEvent := func(eventType, message string, data map[string]interface{}) {
		event := map[string]interface{}{
			"type": eventType,
			"message": message,
		}
		if data != nil {
			for k, v := range data {
				event[k] = v
			}
		}
		select {
		case eventChannel <- event:
		case <-ctx.Done():
			return
		}
	}
	
	// Auto-DevOps gate (unless manual)
	if man, _ := ctx.Value(ctxManualKey{}).(bool); !man {
		allowed, aerr := shouldAutoApply(ctx, stackID)
		if aerr != nil {
			sendEvent("error", fmt.Sprintf("Auto-DevOps check failed: %v", aerr), nil)
			return aerr
		}
		if !allowed {
			sendEvent("skipped", "Auto-DevOps disabled by effective policy", nil)
			return nil
		}
	}

	// Resolve raw project name (as user typed) + label form for lookups
	rawProjectName, err := fetchStackName(ctx, stackID)
	if err != nil || strings.TrimSpace(rawProjectName) == "" {
		sendEvent("error", "Could not resolve stack name", nil)
		return errors.New("deploy: could not resolve stack name")
	}
	labelProject := composeProjectLabelFromStack(rawProjectName)

	sendEvent("info", fmt.Sprintf("Starting deployment of stack: %s", rawProjectName), nil)

	// Working dir and rel path
	_, err = getRepoRootForStack(ctx, stackID)
	if err != nil {
		sendEvent("error", fmt.Sprintf("Failed to get repo root: %v", err), nil)
		return err
	}
	var rel string
	_ = db.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	if strings.TrimSpace(rel) == "" {
		sendEvent("error", "Stack has no rel_path", nil)
		return errors.New("deploy: stack has no rel_path")
	}

	// Stage (SOPS decrypts into tmpfs and is cleaned afterwards)
	sendEvent("info", "Staging stack files and decrypting secrets...", nil)
	stageDir, stagedComposes, cleanup, derr := stageStackForCompose(ctx, stackID)
	if derr != nil {
		sendEvent("error", fmt.Sprintf("Failed to stage stack: %v", derr), nil)
		return derr
	}
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if len(stagedComposes) == 0 {
		sendEvent("info", "No compose files found; skipping deployment", nil)
		return nil
	}

	// Precompute rendered config-hash + bundle hash (best effort; for stamping/drift).
	renderedCfgHash := computeRenderedConfigHash(ctx, stageDir, rawProjectName, stagedComposes)
	bundleHash, _ := ComputeCurrentBundleHash(ctx, stackID)

	// Build a deployment stamp - first check if configuration has changed
	meta := map[string]string{
		"rendered_config_hash": renderedCfgHash,
		"bundle_hash":          bundleHash,
	}
	var allComposeContent []byte
	for _, f := range stagedComposes {
		b, rerr := os.ReadFile(f)
		if rerr != nil {
			sendEvent("error", fmt.Sprintf("Failed to read staged compose file: %v", rerr), nil)
			return fmt.Errorf("failed to read staged compose file %s: %v", f, rerr)
		}
		allComposeContent = append(allComposeContent, b...)
		allComposeContent = append(allComposeContent, '\n')
	}

	// Check if this exact configuration has been deployed before (unless forced)
	if forced, _ := ctx.Value(ctxForceKey{}).(bool); !forced {
		existingStamp, checkErr := CheckDeploymentStampExists(ctx, stackID, allComposeContent)
		if checkErr == nil && existingStamp != nil {
			// Configuration unchanged - ask user for confirmation
			sendEvent("config_unchanged", 
				fmt.Sprintf("Configuration unchanged since %s. Deploy anyway?", 
					existingStamp.DeploymentTimestamp.Format("2006-01-02 15:04:05")), 
				map[string]interface{}{
					"existing_stamp_id": existingStamp.ID,
					"last_deploy_time": existingStamp.DeploymentTimestamp,
					"last_deploy_status": existingStamp.DeploymentStatus,
				})
			// Wait for user confirmation before continuing
			return nil
		}
	}

	stamp, serr := CreateDeploymentStamp(ctx, stackID, "compose", "", allComposeContent, meta)
	if serr != nil {
		errorLog("deploy: failed to create deployment stamp: %v", serr)
		// If stamp creation fails due to unique constraint, try to find the existing one
		if existingStamp, findErr := CheckDeploymentStampExists(ctx, stackID, allComposeContent); findErr == nil && existingStamp != nil {
			debugLog("deploy: reusing existing deployment stamp %d", existingStamp.ID)
			stamp = existingStamp
			sendEvent("info", "Reusing existing deployment stamp", nil)
		} else {
			debugLog("deploy: could not find existing stamp either: %v", findErr)
			sendEvent("error", fmt.Sprintf("Failed to create deployment stamp: %v", serr), nil)
			return serr
		}
	}
	
	if stamp == nil || stamp.ID == 0 {
		errorLog("deploy: CRITICAL - stamp is nil or has ID 0 after creation (stamp=%v)", stamp)
		sendEvent("error", "Deployment stamp creation failed - invalid stamp ID", nil)
		return fmt.Errorf("deployment stamp creation failed - invalid stamp ID")
	}
	debugLog("deploy: created/found stamp with ID %d for stack %d", stamp.ID, stackID)

	// docker compose command
	args := []string{"compose", "-p", rawProjectName}
	for _, f := range stagedComposes {
		args = append(args, "-f", f)
	}
	args = append(args, "up", "-d", "--remove-orphans")

	// Get the host info to set up proper Docker connection
	var dockerEnv []string
	if host, herr := getHostForStack(ctx, stackID); herr == nil {
		dockerURL, sshCmd := dockerURLFor(host)
		dockerEnv = append(os.Environ(), "DOCKER_HOST="+dockerURL)
		if sshCmd != "" {
			dockerEnv = append(dockerEnv, "DOCKER_SSH_CMD="+sshCmd)
		}
		debugLog("deploy: using Docker host %s for stack %d", dockerURL, stackID)
		sendEvent("info", fmt.Sprintf("Using Docker host: %s", dockerURL), nil)
	} else {
		errorLog("deploy: failed to get host for stack %d, using default Docker connection: %v", stackID, herr)
		dockerEnv = os.Environ()
		sendEvent("info", "Using default Docker connection", nil)
	}

	sendEvent("info", fmt.Sprintf("Running: docker %s", strings.Join(args, " ")), nil)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	cmd.Env = dockerEnv

	// Create pipes for streaming output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sendEvent("error", fmt.Sprintf("Failed to create stdout pipe: %v", err), nil)
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		sendEvent("error", fmt.Sprintf("Failed to create stderr pipe: %v", err), nil)
		return err
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		sendEvent("error", fmt.Sprintf("Failed to start docker compose: %v", err), nil)
		if stamp != nil {
			_ = UpdateDeploymentStampStatus(ctx, stamp.ID, "failed")
		}
		return err
	}

	// Stream output in real-time
	done := make(chan error, 2)
	
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				sendEvent("stdout", line, nil)
			}
		}
		done <- scanner.Err()
	}()
	
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				sendEvent("stderr", line, nil)
			}
		}
		done <- scanner.Err()
	}()

	// Wait for completion
	cmdErr := cmd.Wait()
	<-done
	<-done

	if cmdErr != nil {
		if stamp != nil {
			_ = UpdateDeploymentStampStatus(ctx, stamp.ID, "failed")
		}
		sendEvent("error", fmt.Sprintf("Docker compose failed: %v", cmdErr), nil)
		return cmdErr
	}

	sendEvent("success", "Docker compose completed successfully", nil)

	// Post-deployment tasks
	if stamp != nil {
		if uerr := UpdateDeploymentStampStatus(ctx, stamp.ID, "success"); uerr != nil {
			errorLog("deploy: failed to update deployment stamp status: %v", uerr)
		}
		
		if host, herr := getHostForStack(ctx, stackID); herr == nil {
			dockerURL, sshCmd := dockerURLFor(host)
			if dcli, done, derr := dockerClientForURL(ctx, dockerURL, sshCmd); derr == nil {
				if cerr := onSuccessfulDeployment(ctx, stackID, rawProjectName, dcli); cerr != nil {
					errorLog("deploy: failed to update drift cache: %v", cerr)
				}
				if done != nil {
					done()
				}
			}
		}
		go func(label string, stampID int64, depHash string) {
			backoff := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 5 * time.Second}
			for i := 0; i < len(backoff); i++ {
				if i > 0 {
					time.Sleep(backoff[i])
				}
				if err := associateByProjectInspect(context.Background(), label, stampID, depHash, stackID); err == nil {
					return
				}
			}
			if err := associateByProjectInspect(context.Background(), label, stampID, depHash, stackID); err != nil {
				errorLog("deploy: association (inspect) still failing for project=%s: %v", label, err)
			}
		}(labelProject, stamp.ID, stamp.DeploymentHash)
	}

	sendEvent("complete", fmt.Sprintf("Deployment of stack %s completed successfully", rawProjectName), map[string]interface{}{
		"success": true,
		"stackID": stackID,
	})

	return nil
}

// associateByProjectInspect stamps all containers with the given Compose project label value.
func associateByProjectInspect(ctx context.Context, projectLabel string, stampID int64, deploymentHash string, stackID int64) error {
	var cli *client.Client
	var done func()
	if host, herr := getHostForStack(ctx, stackID); herr == nil {
		dockerURL, sshCmd := dockerURLFor(host)
		if c, d, derr := dockerClientForURL(ctx, dockerURL, sshCmd); derr == nil {
			cli = c
			done = d
		} else {
			return derr
		}
	} else {
		return herr
	}
	defer func() {
		if done != nil {
			done()
		}
	}()

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
			errorLog("deploy: failed to associate container %s with stamp %d: %v", c.ID, stampID, e)
		}
	}
	if assocErrs > 0 {
		return fmt.Errorf("associated %d/%d containers (some failed)", len(list)-assocErrs, len(list))
	}
	return nil
}
