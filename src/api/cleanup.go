package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/google/uuid"
)

// CleanupOptions holds configuration for cleanup operations
type CleanupOptions struct {
	DryRun           bool                       `json:"dry_run"`
	Force            bool                       `json:"force"`
	ExcludeFilters   map[string][]string        `json:"exclude_filters"`
	ConfirmationToken string                    `json:"confirmation_token"`
}

// CleanupResult holds the result of a cleanup operation
type CleanupResult struct {
	SpaceReclaimed string            `json:"space_reclaimed"`
	ItemsRemoved   map[string]int    `json:"items_removed"`
	Errors         []string          `json:"errors"`
	Status         string            `json:"status"`
}

// CleanupJob represents a cleanup job in the database
type CleanupJob struct {
	ID          string                 `json:"id"`
	Operation   string                 `json:"operation"`
	Scope       string                 `json:"scope"`
	Target      string                 `json:"target"`
	Status      string                 `json:"status"`
	DryRun      bool                   `json:"dry_run"`
	Force       bool                   `json:"force"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Progress    map[string]interface{} `json:"progress"`
	Results     map[string]interface{} `json:"results"`
	Owner       string                 `json:"owner"`
}

// SpacePreview holds information about disk space that can be freed
type SpacePreview struct {
	Operation     string            `json:"operation"`
	EstimatedSize string            `json:"estimated_size"`
	EstimatedBytes int64            `json:"estimated_bytes"`
	ItemCount     map[string]int    `json:"item_count"`
	Details       []string          `json:"details"`
	Status        string            `json:"status"`
	Error         string            `json:"error,omitempty"`
}

// performSystemPrune executes docker system prune on a specific host
func performSystemPrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	debugLog("Starting system prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		debugLog("DRY RUN: System prune on %s - no actual cleanup performed", hostName)
		result.Status = "dry_run"
		result.SpaceReclaimed = "0B (dry run)"
		return result, nil
	}

	// Use SSH command to run docker system prune
	cmd := "docker system prune -af"
	output, err := runDockerCommand(host, cmd)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("System prune failed: %v", err))
		result.Status = "failed"
		return result, nil
	}

	// Parse output for space reclaimed
	spaceReclaimed := parseDockerPruneOutput(output)
	result.SpaceReclaimed = spaceReclaimed
	result.ItemsRemoved["system"] = 1

	debugLog("System prune completed on %s: %s reclaimed", hostName, result.SpaceReclaimed)
	return result, nil
}

// performImagePrune executes docker image prune on a specific host
func performImagePrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	debugLog("Starting image prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		debugLog("DRY RUN: Image prune on %s - no actual cleanup performed", hostName)
		result.Status = "dry_run"
		result.SpaceReclaimed = "0B (dry run)"
		return result, nil
	}

	cmd := "docker image prune -af"
	output, err := runDockerCommand(host, cmd)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Image prune failed: %v", err))
		result.Status = "failed"
		return result, nil
	}

	spaceReclaimed := parseDockerPruneOutput(output)
	result.SpaceReclaimed = spaceReclaimed
	result.ItemsRemoved["images"] = 1

	debugLog("Image prune completed on %s: %s reclaimed", hostName, result.SpaceReclaimed)
	return result, nil
}

// performContainerPrune executes docker container prune on a specific host
func performContainerPrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	debugLog("Starting container prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		debugLog("DRY RUN: Container prune on %s - no actual cleanup performed", hostName)
		result.Status = "dry_run"
		result.SpaceReclaimed = "0B (dry run)"
		return result, nil
	}

	cmd := "docker container prune -f"
	output, err := runDockerCommand(host, cmd)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Container prune failed: %v", err))
		result.Status = "failed"
		return result, nil
	}

	spaceReclaimed := parseDockerPruneOutput(output)
	result.SpaceReclaimed = spaceReclaimed
	result.ItemsRemoved["containers"] = 1

	debugLog("Container prune completed on %s: %s reclaimed", hostName, result.SpaceReclaimed)
	return result, nil
}

// performVolumePrune executes docker volume prune on a specific host
func performVolumePrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	debugLog("Starting volume prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		debugLog("DRY RUN: Volume prune on %s - no actual cleanup performed", hostName)
		result.Status = "dry_run"
		result.SpaceReclaimed = "0B (dry run)"
		return result, nil
	}

	cmd := "docker volume prune -f"
	output, err := runDockerCommand(host, cmd)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Volume prune failed: %v", err))
		result.Status = "failed"
		return result, nil
	}

	spaceReclaimed := parseDockerPruneOutput(output)
	result.SpaceReclaimed = spaceReclaimed
	result.ItemsRemoved["volumes"] = 1

	debugLog("Volume prune completed on %s: %s reclaimed", hostName, result.SpaceReclaimed)
	return result, nil
}

// performNetworkPrune executes docker network prune on a specific host
func performNetworkPrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	debugLog("Starting network prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		debugLog("DRY RUN: Network prune on %s - no actual cleanup performed", hostName)
		result.Status = "dry_run"
		result.SpaceReclaimed = "0B (dry run)"
		return result, nil
	}

	cmd := "docker network prune -f"
	_, err = runDockerCommand(host, cmd)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Network prune failed: %v", err))
		result.Status = "failed"
		return result, nil
	}

	result.SpaceReclaimed = "0B" // Networks don't report space reclaimed
	result.ItemsRemoved["networks"] = 1

	debugLog("Network prune completed on %s", hostName)
	return result, nil
}

// performBuildCachePrune executes docker buildx prune on a specific host
func performBuildCachePrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	debugLog("Starting build cache prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		debugLog("DRY RUN: Build cache prune on %s - no actual cleanup performed", hostName)
		result.Status = "dry_run"
		result.SpaceReclaimed = "0B (dry run)"
		return result, nil
	}

	// Try buildx first, fallback to builder
	cmd := "docker buildx prune -af 2>/dev/null || docker builder prune -af"
	output, err := runDockerCommand(host, cmd)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Build cache prune failed: %v", err))
		result.Status = "failed"
		return result, nil
	}

	spaceReclaimed := parseDockerPruneOutput(output)
	result.SpaceReclaimed = spaceReclaimed
	result.ItemsRemoved["build_cache"] = 1

	debugLog("Build cache prune completed on %s: %s reclaimed", hostName, spaceReclaimed)
	return result, nil
}

// runDockerCommand executes a Docker command using the appropriate method (local Docker client or SSH)
func runDockerCommand(host HostRow, command string) (string, error) {
	url, _ := dockerURLFor(host)
	
	// For local Docker socket access, use direct execution
	if strings.HasPrefix(url, "unix://") && localHostAllowed(host) {
		// Local execution via Docker socket
		cmd := exec.Command("sh", "-c", command)
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	// For remote hosts or non-allowed local access, use SSH
	if host.Addr != "" && host.Addr != "localhost" && host.Addr != "127.0.0.1" {
		sshCmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", host.Addr, command)
		output, err := sshCmd.CombinedOutput()
		return string(output), err
	}

	// Fallback to local execution
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// parseDockerPruneOutput parses Docker command output to extract space reclaimed
func parseDockerPruneOutput(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for patterns like "Total reclaimed space: 2.1GB"
		if strings.Contains(line, "Total reclaimed space:") || strings.Contains(line, "Total:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[len(parts)-1]
			}
		}
		// Alternative pattern "Space reclaimed: 123MB"
		if strings.Contains(line, "Space reclaimed:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return parts[2]
			}
		}
	}
	// If no space info found, return "unknown"
	if strings.Contains(output, "deleted") || strings.Contains(output, "removed") {
		return "unknown"
	}
	return "0B"
}

// createCleanupJob creates a new cleanup job in the database
func createCleanupJob(ctx context.Context, operation, scope, target, owner string, options CleanupOptions) (*CleanupJob, error) {
	job := &CleanupJob{
		ID:        uuid.New().String(),
		Operation: operation,
		Scope:     scope,
		Target:    target,
		Status:    "queued",
		DryRun:    options.DryRun,
		Force:     options.Force,
		CreatedAt: time.Now(),
		Progress:  make(map[string]interface{}),
		Results:   make(map[string]interface{}),
		Owner:     owner,
	}

	excludeFiltersJSON, _ := json.Marshal(options.ExcludeFilters)

	_, err := db.Exec(ctx, `
		INSERT INTO cleanup_jobs (id, operation, scope, target, status, dry_run, force, exclude_filters, created_at, progress, results, owner)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, job.ID, job.Operation, job.Scope, job.Target, job.Status, job.DryRun, job.Force, string(excludeFiltersJSON), job.CreatedAt, "{}", "{}", job.Owner)

	if err != nil {
		return nil, fmt.Errorf("failed to create cleanup job: %w", err)
	}

	debugLog("Created cleanup job %s: %s %s %s", job.ID, operation, scope, target)
	return job, nil
}

// updateJobProgress updates the progress of a cleanup job
func updateJobProgress(ctx context.Context, jobID string, progress map[string]interface{}) error {
	progressJSON, _ := json.Marshal(progress)
	
	_, err := db.Exec(ctx, `
		UPDATE cleanup_jobs 
		SET progress = $1
		WHERE id = $2
	`, string(progressJSON), jobID)

	return err
}

// updateJobStatus updates the status of a cleanup job
func updateJobStatus(ctx context.Context, jobID, status string) error {
	now := time.Now()
	var query string
	var args []interface{}

	switch status {
	case "running":
		query = "UPDATE cleanup_jobs SET status = $1, started_at = $2 WHERE id = $3"
		args = []interface{}{status, now, jobID}
	case "completed", "failed":
		query = "UPDATE cleanup_jobs SET status = $1, completed_at = $2 WHERE id = $3"
		args = []interface{}{status, now, jobID}
	default:
		query = "UPDATE cleanup_jobs SET status = $1 WHERE id = $2"
		args = []interface{}{status, jobID}
	}

	_, err := db.Exec(ctx, query, args...)
	return err
}

// updateJobResults updates the results of a cleanup job
func updateJobResults(ctx context.Context, jobID string, results map[string]interface{}) error {
	resultsJSON, _ := json.Marshal(results)
	
	_, err := db.Exec(ctx, `
		UPDATE cleanup_jobs 
		SET results = $1
		WHERE id = $2
	`, string(resultsJSON), jobID)

	return err
}

// getCleanupJob retrieves a cleanup job from the database
func getCleanupJob(ctx context.Context, jobID string) (*CleanupJob, error) {
	job := &CleanupJob{}
	var excludeFiltersJSON, progressJSON, resultsJSON string

	err := db.QueryRow(ctx, `
		SELECT id, operation, scope, target, status, dry_run, force, 
			   created_at, started_at, completed_at, exclude_filters, progress, results, owner
		FROM cleanup_jobs 
		WHERE id = $1
	`, jobID).Scan(&job.ID, &job.Operation, &job.Scope, &job.Target, &job.Status, &job.DryRun, &job.Force,
		&job.CreatedAt, &job.StartedAt, &job.CompletedAt, &excludeFiltersJSON, &progressJSON, &resultsJSON, &job.Owner)

	if err != nil {
		return nil, err
	}

	// Parse JSON fields
	json.Unmarshal([]byte(progressJSON), &job.Progress)
	json.Unmarshal([]byte(resultsJSON), &job.Results)

	return job, nil
}


// humanizeBytes converts bytes to human readable format
func humanizeBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// parseByteSize parses human readable byte sizes like "10GB", "500MB"
func parseByteSize(size string) int64 {
	size = strings.TrimSpace(strings.ToUpper(size))
	if size == "0" || size == "" {
		return 0
	}
	
	multipliers := map[string]int64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}
	
	for suffix, mult := range multipliers {
		if strings.HasSuffix(size, suffix) {
			numStr := strings.TrimSuffix(size, suffix)
			if num, err := strconv.ParseFloat(numStr, 64); err == nil {
				return int64(num * float64(mult))
			}
		}
	}
	
	// Try parsing as raw number (bytes)
	if num, err := strconv.ParseInt(size, 10, 64); err == nil {
		return num
	}
	
	errorLog("Invalid size format: %s, defaulting to 10GB", size)
	return 10 * 1024 * 1024 * 1024 // Default 10GB
}

// getSpacePreview analyzes how much space can be freed by a cleanup operation
func getSpacePreview(ctx context.Context, hostName string, operation string) (*SpacePreview, error) {
	debugLog("Getting space preview for %s operation on host %s", operation, hostName)

	host, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host %s not found: %v", hostName, err)
	}

	preview := &SpacePreview{
		Operation:  operation,
		ItemCount:  make(map[string]int),
		Details:    []string{},
		Status:     "success",
	}

	switch operation {
	case "system":
		return getSystemSpacePreview(host, preview)
	case "images":
		return getImageSpacePreview(host, preview)
	case "containers":
		return getContainerSpacePreview(host, preview)
	case "volumes":
		return getVolumeSpacePreview(host, preview)
	case "networks":
		return getNetworkSpacePreview(host, preview)
	case "build-cache":
		return getBuildCacheSpacePreview(host, preview)
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

// getSystemSpacePreview gets space preview for system prune
func getSystemSpacePreview(host HostRow, preview *SpacePreview) (*SpacePreview, error) {
	ctx := context.Background()
	cli, err := dockerClientForHost(host)
	if err != nil {
		preview.Status = "error"
		preview.Error = fmt.Sprintf("Failed to connect to Docker: %v", err)
		return preview, nil
	}
	defer cli.Close()

	totalBytes := int64(0)

	// Get dangling images
	images, err := cli.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("dangling", "true")),
	})
	if err == nil {
		imageBytes := int64(0)
		for _, image := range images {
			imageBytes += image.Size
		}
		totalBytes += imageBytes
		if len(images) > 0 {
			preview.Details = append(preview.Details, fmt.Sprintf("Dangling images: %d (%s)", len(images), humanizeBytes(imageBytes)))
			preview.ItemCount["images"] = len(images)
		}
	}

	// Get stopped containers
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("status", "exited")),
	})
	if err == nil {
		containerBytes := int64(0)
		for _, container := range containers {
			containerBytes += container.SizeRw
		}
		totalBytes += containerBytes
		if len(containers) > 0 {
			preview.Details = append(preview.Details, fmt.Sprintf("Stopped containers: %d (%s)", len(containers), humanizeBytes(containerBytes)))
			preview.ItemCount["containers"] = len(containers)
		}
	}

	// Get unused volumes
	vl, err := cli.VolumeList(ctx, volume.ListOptions{Filters: filters.NewArgs()})
	if err == nil {
		unusedCount := 0
		for range vl.Volumes {
			// For system prune estimate, assume all volumes are candidates
			// (Docker system prune only removes unused volumes)
			unusedCount++
		}
		if unusedCount > 0 {
			preview.Details = append(preview.Details, fmt.Sprintf("Unused volumes: %d", unusedCount))
			preview.ItemCount["volumes"] = unusedCount
		}
	}

	// Get unused networks
	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{})
	if err == nil {
		customNetworks := 0
		for _, network := range networks {
			// Skip system networks
			if network.Name != "bridge" && network.Name != "host" && network.Name != "none" {
				customNetworks++
			}
		}
		if customNetworks > 0 {
			preview.Details = append(preview.Details, fmt.Sprintf("Unused networks: %d", customNetworks))
			preview.ItemCount["networks"] = customNetworks
		}
	}

	preview.EstimatedBytes = totalBytes
	preview.EstimatedSize = humanizeBytes(totalBytes)
	return preview, nil
}

// getImageSpacePreview gets space preview for image cleanup
func getImageSpacePreview(host HostRow, preview *SpacePreview) (*SpacePreview, error) {
	ctx := context.Background()
	cli, err := dockerClientForHost(host)
	if err != nil {
		preview.Status = "error"
		preview.Error = fmt.Sprintf("Failed to connect to Docker: %v", err)
		return preview, nil
	}
	defer cli.Close()

	// Get dangling images using Docker API
	images, err := cli.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("dangling", "true")),
	})
	if err != nil {
		preview.Status = "error"
		preview.Error = fmt.Sprintf("Failed to list images: %v", err)
		return preview, nil
	}

	totalBytes := int64(0)
	imageCount := len(images)
	for _, image := range images {
		totalBytes += image.Size
	}

	preview.EstimatedBytes = totalBytes
	preview.EstimatedSize = humanizeBytes(totalBytes)
	preview.ItemCount["unused_images"] = imageCount
	preview.Details = append(preview.Details, fmt.Sprintf("%d unused images", imageCount))
	return preview, nil
}

// getContainerSpacePreview gets space preview for container cleanup
func getContainerSpacePreview(host HostRow, preview *SpacePreview) (*SpacePreview, error) {
	ctx := context.Background()
	cli, err := dockerClientForHost(host)
	if err != nil {
		preview.Status = "error"
		preview.Error = fmt.Sprintf("Failed to connect to Docker: %v", err)
		return preview, nil
	}
	defer cli.Close()

	// Get stopped containers using Docker API
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("status", "exited"),
			filters.Arg("status", "created"),
			filters.Arg("status", "dead"),
		),
	})
	if err != nil {
		preview.Status = "error"
		preview.Error = fmt.Sprintf("Failed to list containers: %v", err)
		return preview, nil
	}

	totalBytes := int64(0)
	containerCount := len(containers)
	for _, container := range containers {
		totalBytes += container.SizeRw // Size of writable layer
	}

	preview.EstimatedBytes = totalBytes
	preview.EstimatedSize = humanizeBytes(totalBytes)
	preview.ItemCount["stopped_containers"] = containerCount
	preview.Details = append(preview.Details, fmt.Sprintf("%d stopped containers", containerCount))
	return preview, nil
}

// getVolumeSpacePreview gets space preview for volume cleanup
func getVolumeSpacePreview(host HostRow, preview *SpacePreview) (*SpacePreview, error) {
	ctx := context.Background()
	cli, err := dockerClientForHost(host)
	if err != nil {
		preview.Status = "error"
		preview.Error = fmt.Sprintf("Failed to connect to Docker: %v", err)
		return preview, nil
	}
	defer cli.Close()

	// Get dangling volumes using Docker API
	volumes, err := cli.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(filters.Arg("dangling", "true")),
	})
	if err != nil {
		preview.Status = "error"
		preview.Error = fmt.Sprintf("Failed to list volumes: %v", err)
		return preview, nil
	}

	volumeCount := len(volumes.Volumes)
	// Docker API doesn't provide volume size directly, so estimate
	estimatedBytes := int64(volumeCount * 100 * 1024 * 1024) // 100MB per volume average

	preview.EstimatedBytes = estimatedBytes
	preview.EstimatedSize = humanizeBytes(estimatedBytes)
	preview.ItemCount["unused_volumes"] = volumeCount
	preview.Details = append(preview.Details, fmt.Sprintf("%d unused volumes (estimated)", volumeCount))
	return preview, nil
}

// getNetworkSpacePreview gets space preview for network cleanup
func getNetworkSpacePreview(host HostRow, preview *SpacePreview) (*SpacePreview, error) {
	ctx := context.Background()
	cli, err := dockerClientForHost(host)
	if err != nil {
		preview.Status = "error"
		preview.Error = fmt.Sprintf("Failed to connect to Docker: %v", err)
		return preview, nil
	}
	defer cli.Close()

	// Get all networks
	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		preview.Status = "error"
		preview.Error = fmt.Sprintf("Failed to list networks: %v", err)
		return preview, nil
	}

	// Get all containers to check which networks are in use
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		preview.Status = "error"
		preview.Error = fmt.Sprintf("Failed to list containers: %v", err)
		return preview, nil
	}

	// Build set of networks in use
	networksInUse := make(map[string]bool)
	for _, container := range containers {
		if container.NetworkSettings != nil {
			for networkName := range container.NetworkSettings.Networks {
				networksInUse[networkName] = true
			}
		}
	}

	// Count unused networks (excluding default networks)
	networkCount := 0
	for _, network := range networks {
		// Skip default networks
		if network.Name == "bridge" || network.Name == "host" || network.Name == "none" {
			continue
		}
		// Skip networks that are in use
		if !networksInUse[network.Name] {
			networkCount++
		}
	}

	// Networks don't take significant disk space, but we'll show count
	preview.EstimatedBytes = 0
	preview.EstimatedSize = "0B"
	preview.ItemCount["unused_networks"] = networkCount
	preview.Details = append(preview.Details, fmt.Sprintf("%d unused networks", networkCount))
	return preview, nil
}

// getBuildCacheSpacePreview gets space preview for build cache cleanup
func getBuildCacheSpacePreview(host HostRow, preview *SpacePreview) (*SpacePreview, error) {
	// Build cache space estimation - use system df for now
	// Note: This could be improved with direct API calls when Docker adds build cache API
	cacheCount := 0
	estimatedBytes := int64(0) // Default to 0 for build cache since we can't easily estimate

	preview.EstimatedBytes = estimatedBytes
	preview.EstimatedSize = humanizeBytes(estimatedBytes)
	preview.ItemCount["build_cache_items"] = cacheCount
	preview.Details = append(preview.Details, "Build cache cleanup available")
	return preview, nil
}