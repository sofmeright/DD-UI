// src/api/deploy.go
// src/api/deploy.go
package services

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"dd-ui/common"
	"dd-ui/database"
	"dd-ui/utils"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// funcStackStager adapts a function to the StackStager interface
type funcStackStager struct {
	stageFunc func(ctx context.Context, stackID int64) (string, []string, func(), error)
}

func (f *funcStackStager) StageStackForCompose(ctx context.Context, stackID int64) (string, interface{}, func(), error) {
	dir, composes, cleanup, err := f.stageFunc(ctx, stackID)
	return dir, composes, cleanup, err
}

// getHostForStack retrieves the host information for a given stack ID
func getHostForStack(ctx context.Context, stackID int64) (database.HostRow, error) {
	var host database.HostRow
	err := common.DB.QueryRow(ctx, `
		SELECT h.id, h.name, h.addr, h.vars
		FROM iac_stacks s
		JOIN hosts h ON (s.scope_kind='host' AND s.scope_name=h.name)
		WHERE s.id = $1
	`, stackID).Scan(&host.ID, &host.Name, &host.Addr, &host.Vars)
	return host, err
}

// CtxManualKey marks a deploy as "manual", which bypasses Auto DevOps gating.
type CtxManualKey struct{}

// CtxForceKey marks a deploy as "forced", which bypasses configuration unchanged checks.
type CtxForceKey struct{}

// deployStack: stage -> (optional: compute config-hash) -> docker compose up -d
// (-p = EXACT stack name) -> stamp -> associate via label(sanitized).
func DeployStack(ctx context.Context, stackID int64) error {
	// Auto-DevOps gate (unless manual)
	if man, _ := ctx.Value(CtxManualKey{}).(bool); !man {
		allowed, aerr := ShouldAutoApply(ctx, stackID)
		if aerr != nil {
			return aerr
		}
		if !allowed {
			common.InfoLog("deploy: stack %d skipped (auto_devops disabled by effective policy)", stackID)
			return nil
		}
	}

	// Resolve raw project name (as user typed) + label form for lookups
	rawProjectName, err := utils.FetchStackName(ctx, common.DB, stackID)
	if err != nil || strings.TrimSpace(rawProjectName) == "" {
		return errors.New("deploy: could not resolve stack name")
	}
	labelProject := utils.ComposeProjectLabelFromStack(rawProjectName)

	// Working dir and rel path
	root, err := GetRepoRootForStack(ctx, stackID)
	if err != nil {
		return err
	}
	var rel string
	_ = common.DB.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	if strings.TrimSpace(rel) == "" {
		return errors.New("deploy: stack has no rel_path")
	}

	// Stage (SOPS decrypts into tmpfs and is cleaned afterwards)
	stageDir, stagedComposes, cleanup, derr := StageStackForCompose(ctx, stackID)
	if derr != nil {
		return derr
	}
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if len(stagedComposes) == 0 {
		common.InfoLog("deploy: stack %d: no compose files tracked; skipping", stackID)
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
	stamp, serr := database.CreateDeploymentStamp(ctx, stackID, "compose", "", allComposeContent, meta)
	if serr != nil {
		common.InfoLog("deploy: failed to create deployment stamp: %v", serr)
		// If stamp creation fails due to unique constraint, try to find the existing one
		if existingStamp, findErr := database.CheckDeploymentStampExists(ctx, stackID, allComposeContent); findErr == nil && existingStamp != nil {
			common.InfoLog("deploy: reusing existing deployment stamp %d", existingStamp.ID)
			stamp = existingStamp
		} else {
			common.ErrorLog("deploy: could not find existing stamp either: %v", findErr)
			return serr
		}
	}
	
	if stamp == nil || stamp.ID == 0 {
		common.ErrorLog("deploy: CRITICAL - stamp is nil or has ID 0 after creation (stamp=%v)", stamp)
		return fmt.Errorf("deployment stamp creation failed - invalid stamp ID")
	}
	common.DebugLog("deploy: created/found stamp with ID %d for stack %d", stamp.ID, stackID)

	// docker compose -p <RAW stack name> -f ... up -d --remove-orphans
	args := []string{"compose", "-p", rawProjectName}
	for _, f := range stagedComposes {
		args = append(args, "-f", f)
	}
	args = append(args, "up", "-d", "--remove-orphans")

	// Get the host info to set up proper Docker connection
	var dockerEnv []string
	if host, herr := getHostForStack(ctx, stackID); herr == nil {
		dockerURL, _ := DockerURLFor(host)
		
		// Set up SSH config for Docker CLI instead of using DOCKER_SSH_CMD
		if err := setupSSHConfigForDocker(host); err != nil {
			common.ErrorLog("deploy: failed to setup SSH config for Docker CLI: %v", err)
			return err
		}
		
		dockerEnv = append(os.Environ(), "DOCKER_HOST="+dockerURL)
		common.DebugLog("deploy: using Docker host %s for stack %d", dockerURL, stackID)
	} else {
		common.ErrorLog("deploy: failed to get host for stack %d, using default Docker connection: %v", stackID, herr)
		dockerEnv = os.Environ()
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stageDir
	cmd.Env = dockerEnv

	out, err := cmd.CombinedOutput()
	if err != nil {
		if stamp != nil {
			_ = database.UpdateDeploymentStampStatus(ctx, stamp.ID, "failed")
		}
		common.ErrorLog("deploy: docker compose failed: %v\n----\n%s\n----", err, string(out))
		return fmt.Errorf("docker compose up failed: %v\n%s", err, string(out))
	}

	// Mark success and associate by Compose label (sanitized form).
	if stamp != nil {
		if uerr := database.UpdateDeploymentStampStatus(ctx, stamp.ID, "success"); uerr != nil {
			common.ErrorLog("deploy: failed to update deployment stamp status: %v", uerr)
		}
		
		// Update drift cache after successful deployment
		if host, herr := getHostForStack(ctx, stackID); herr == nil {
			dockerURL, sshCmd := DockerURLFor(host)
			if dcli, done, derr := DockerClientForURL(ctx, dockerURL, sshCmd); derr == nil {
				if cerr := utils.OnSuccessfulDeploymentWithDeps(ctx, common.DB, &funcStackStager{stageFunc: StageStackForCompose}, stackID, rawProjectName, dcli); cerr != nil {
					common.ErrorLog("deploy: failed to update drift cache: %v", cerr)
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
				common.ErrorLog("deploy: association (inspect) still failing for project=%s: %v", label, err)
			}
		}(labelProject, stamp.ID, stamp.DeploymentHash)
	}

	common.InfoLog("deploy: stack %d deployed (compose=%d, stage=%s, repoRoot=%s, stamp=%v)",
		stackID, len(stagedComposes), stageDir, root, stamp != nil)

	return nil
}

// DeployStackWithStream performs deployment while streaming docker compose output
func DeployStackWithStream(ctx context.Context, stackID int64, eventChannel chan<- map[string]interface{}) error {
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
	if man, _ := ctx.Value(CtxManualKey{}).(bool); !man {
		allowed, aerr := ShouldAutoApply(ctx, stackID)
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
	rawProjectName, err := utils.FetchStackName(ctx, common.DB, stackID)
	if err != nil || strings.TrimSpace(rawProjectName) == "" {
		sendEvent("error", "Could not resolve stack name", nil)
		return errors.New("deploy: could not resolve stack name")
	}
	labelProject := utils.ComposeProjectLabelFromStack(rawProjectName)

	sendEvent("info", fmt.Sprintf("Starting deployment of stack: %s", rawProjectName), nil)

	// Working dir and rel path
	_, err = GetRepoRootForStack(ctx, stackID)
	if err != nil {
		sendEvent("error", fmt.Sprintf("Failed to get repo root: %v", err), nil)
		return err
	}
	var rel string
	_ = common.DB.QueryRow(ctx, `SELECT rel_path FROM iac_stacks WHERE id=$1`, stackID).Scan(&rel)
	if strings.TrimSpace(rel) == "" {
		sendEvent("error", "Stack has no rel_path", nil)
		return errors.New("deploy: stack has no rel_path")
	}

	// Stage (SOPS decrypts into tmpfs and is cleaned afterwards)
	sendEvent("info", "Staging stack files and decrypting secrets...", nil)
	stageDir, stagedComposes, cleanup, derr := StageStackForCompose(ctx, stackID)
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
	if forced, _ := ctx.Value(CtxForceKey{}).(bool); !forced {
		existingStamp, checkErr := database.CheckDeploymentStampExists(ctx, stackID, allComposeContent)
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

	stamp, serr := database.CreateDeploymentStamp(ctx, stackID, "compose", "", allComposeContent, meta)
	if serr != nil {
		common.InfoLog("deploy: failed to create deployment stamp: %v", serr)
		// If stamp creation fails due to unique constraint, try to find the existing one
		if existingStamp, findErr := database.CheckDeploymentStampExists(ctx, stackID, allComposeContent); findErr == nil && existingStamp != nil {
			common.InfoLog("deploy: reusing existing deployment stamp %d", existingStamp.ID)
			stamp = existingStamp
			sendEvent("info", "Reusing existing deployment stamp", nil)
		} else {
			common.ErrorLog("deploy: could not find existing stamp either: %v", findErr)
			sendEvent("error", fmt.Sprintf("Failed to create deployment stamp: %v", serr), nil)
			return serr
		}
	}
	
	if stamp == nil || stamp.ID == 0 {
		common.ErrorLog("deploy: CRITICAL - stamp is nil or has ID 0 after creation (stamp=%v)", stamp)
		sendEvent("error", "Deployment stamp creation failed - invalid stamp ID", nil)
		return fmt.Errorf("deployment stamp creation failed - invalid stamp ID")
	}
	common.DebugLog("deploy: created/found stamp with ID %d for stack %d", stamp.ID, stackID)

	// docker compose command
	args := []string{"compose", "-p", rawProjectName}
	for _, f := range stagedComposes {
		args = append(args, "-f", f)
	}
	args = append(args, "up", "-d", "--remove-orphans")

	// Get the host info to set up proper Docker connection
	var dockerEnv []string
	if host, herr := getHostForStack(ctx, stackID); herr == nil {
		dockerURL, _ := DockerURLFor(host)
		
		// Set up SSH config for Docker CLI instead of using DOCKER_SSH_CMD
		if err := setupSSHConfigForDocker(host); err != nil {
			common.ErrorLog("deploy: failed to setup SSH config for Docker CLI: %v", err)
			sendEvent("error", fmt.Sprintf("Failed to setup SSH config: %v", err), nil)
			return err
		}
		
		dockerEnv = append(os.Environ(), "DOCKER_HOST="+dockerURL)
		common.DebugLog("deploy: using Docker host %s for stack %d", dockerURL, stackID)
		sendEvent("info", fmt.Sprintf("Using Docker host: %s", dockerURL), nil)
	} else {
		common.ErrorLog("deploy: failed to get host for stack %d, using default Docker connection: %v", stackID, herr)
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
			_ = database.UpdateDeploymentStampStatus(ctx, stamp.ID, "failed")
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
			_ = database.UpdateDeploymentStampStatus(ctx, stamp.ID, "failed")
		}
		sendEvent("error", fmt.Sprintf("Docker compose failed: %v", cmdErr), nil)
		return cmdErr
	}

	sendEvent("success", "Docker compose completed successfully", nil)

	// Post-deployment tasks
	if stamp != nil {
		if uerr := database.UpdateDeploymentStampStatus(ctx, stamp.ID, "success"); uerr != nil {
			common.ErrorLog("deploy: failed to update deployment stamp status: %v", uerr)
		}
		
		if host, herr := getHostForStack(ctx, stackID); herr == nil {
			dockerURL, sshCmd := DockerURLFor(host)
			if dcli, done, derr := DockerClientForURL(ctx, dockerURL, sshCmd); derr == nil {
				if cerr := utils.OnSuccessfulDeploymentWithDeps(ctx, common.DB, &funcStackStager{stageFunc: StageStackForCompose}, stackID, rawProjectName, dcli); cerr != nil {
					common.ErrorLog("deploy: failed to update drift cache: %v", cerr)
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
				common.ErrorLog("deploy: association (inspect) still failing for project=%s: %v", label, err)
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
		dockerURL, sshCmd := DockerURLFor(host)
		if c, d, derr := DockerClientForURL(ctx, dockerURL, sshCmd); derr == nil {
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
		if e := database.AssociateContainerWithStamp(ctx, c.ID, stampID, deploymentHash); e != nil {
			assocErrs++
			common.ErrorLog("deploy: failed to associate container %s with stamp %d: %v", c.ID, stampID, e)
		}
	}
	if assocErrs > 0 {
		return fmt.Errorf("associated %d/%d containers (some failed)", len(list)-assocErrs, len(list))
	}
	return nil
}

// setupSSHConfigForDocker creates SSH config for Docker CLI to use proper authentication
func setupSSHConfigForDocker(host database.HostRow) error {
	// Get SSH configuration
	user := host.Vars["ansible_user"]
	if user == "" {
		user = common.Env("SSH_USER", "root")
	}
	addr := host.Addr
	if addr == "" {
		addr = host.Name
	}
	keyFile := common.Env("SSH_KEY_FILE", "")
	if keyFile == "" {
		return fmt.Errorf("SSH_KEY_FILE not configured")
	}
	
	// Create ~/.ssh directory if it doesn't exist
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/root" // fallback for root user
	}
	sshDir := homeDir + "/.ssh"
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create SSH directory: %v", err)
	}
	
	// Create SSH config file
	configPath := sshDir + "/config"
	
	// Check if config already has this host
	configContent := fmt.Sprintf(`Host %s
    User %s
    IdentityFile %s
    StrictHostKeyChecking no
    ConnectTimeout 30
    UserKnownHostsFile /dev/null

`, addr, user, keyFile)
	
	// Read existing config
	existingConfig := ""
	if data, err := os.ReadFile(configPath); err == nil {
		existingConfig = string(data)
	}
	
	// Check if this host config already exists
	if strings.Contains(existingConfig, fmt.Sprintf("Host %s", addr)) {
		common.DebugLog("SSH config for host %s already exists", addr)
		return nil
	}
	
	// Append new host config
	file, err := os.OpenFile(configPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open SSH config file: %v", err)
	}
	defer file.Close()
	
	if _, err := file.WriteString(configContent); err != nil {
		return fmt.Errorf("failed to write SSH config: %v", err)
	}
	
	common.DebugLog("SSH config added for host %s (%s@%s)", host.Name, user, addr)
	return nil
}
