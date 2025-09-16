package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"dd-ui/common"
	"dd-ui/database"
	"dd-ui/middleware"
	"dd-ui/services"
	"github.com/go-chi/chi/v5"
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

// getSessionUser extracts the user from the session
func getSessionUser(r *http.Request) string {
	sess, _ := common.Store.Get(r, common.SessionName)
	u, ok := sess.Values["user"].(middleware.User)
	if !ok {
		return "anonymous"
	}
	if u.Email != "" {
		return u.Email
	}
	if u.Name != "" {
		return u.Name
	}
	return "anonymous"
}

// handleCleanupSystemPrune handles POST /api/cleanup/hosts/{hostname}/system
func handleCleanupSystemPrune(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	if hostname == "" {
		http.Error(w, "hostname is required", http.StatusBadRequest)
		return
	}

	var options CleanupOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Require confirmation for non-dry-run operations
	if !options.DryRun && options.ConfirmationToken == "" {
		http.Error(w, "confirmation token required for destructive operations", http.StatusBadRequest)
		return
	}

	owner := getSessionUser(r)
	job, err := createCleanupJob(r.Context(), "system_prune", "single_host", hostname, owner, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create cleanup job: %v", err), http.StatusInternalServerError)
		return
	}

	// Start the cleanup operation in a goroutine
	go executeCleanupJob(job.ID, "system", hostname, options)

	common.RespondJSON(w, job)
}

// handleCleanupImagePrune handles POST /api/cleanup/hosts/{hostname}/images
func handleCleanupImagePrune(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	if hostname == "" {
		http.Error(w, "hostname is required", http.StatusBadRequest)
		return
	}

	var options CleanupOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Require confirmation for non-dry-run operations
	if !options.DryRun && options.ConfirmationToken == "" {
		http.Error(w, "confirmation token required for destructive operations", http.StatusBadRequest)
		return
	}

	owner := getSessionUser(r)
	job, err := createCleanupJob(r.Context(), "image_prune", "single_host", hostname, owner, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create cleanup job: %v", err), http.StatusInternalServerError)
		return
	}

	// Start the cleanup operation in a goroutine
	go executeCleanupJob(job.ID, "image", hostname, options)

	common.RespondJSON(w, job)
}

// handleCleanupContainerPrune handles POST /api/cleanup/hosts/{hostname}/containers
func handleCleanupContainerPrune(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	if hostname == "" {
		http.Error(w, "hostname is required", http.StatusBadRequest)
		return
	}

	var options CleanupOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Require confirmation for non-dry-run operations
	if !options.DryRun && options.ConfirmationToken == "" {
		http.Error(w, "confirmation token required for destructive operations", http.StatusBadRequest)
		return
	}

	owner := getSessionUser(r)
	job, err := createCleanupJob(r.Context(), "container_prune", "single_host", hostname, owner, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create cleanup job: %v", err), http.StatusInternalServerError)
		return
	}

	// Start the cleanup operation in a goroutine
	go executeCleanupJob(job.ID, "container", hostname, options)

	common.RespondJSON(w, job)
}

// handleCleanupVolumePrune handles POST /api/cleanup/hosts/{hostname}/volumes
func handleCleanupVolumePrune(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	if hostname == "" {
		http.Error(w, "hostname is required", http.StatusBadRequest)
		return
	}

	var options CleanupOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Require confirmation for non-dry-run operations
	if !options.DryRun && options.ConfirmationToken == "" {
		http.Error(w, "confirmation token required for destructive operations", http.StatusBadRequest)
		return
	}

	owner := getSessionUser(r)
	job, err := createCleanupJob(r.Context(), "volume_prune", "single_host", hostname, owner, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create cleanup job: %v", err), http.StatusInternalServerError)
		return
	}

	// Start the cleanup operation in a goroutine
	go executeCleanupJob(job.ID, "volume", hostname, options)

	common.RespondJSON(w, job)
}

// handleCleanupNetworkPrune handles POST /api/cleanup/hosts/{hostname}/networks
func handleCleanupNetworkPrune(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	if hostname == "" {
		http.Error(w, "hostname is required", http.StatusBadRequest)
		return
	}

	var options CleanupOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Require confirmation for non-dry-run operations
	if !options.DryRun && options.ConfirmationToken == "" {
		http.Error(w, "confirmation token required for destructive operations", http.StatusBadRequest)
		return
	}

	owner := getSessionUser(r)
	job, err := createCleanupJob(r.Context(), "network_prune", "single_host", hostname, owner, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create cleanup job: %v", err), http.StatusInternalServerError)
		return
	}

	// Start the cleanup operation in a goroutine
	go executeCleanupJob(job.ID, "network", hostname, options)

	common.RespondJSON(w, job)
}

// handleCleanupBuildCachePrune handles POST /api/cleanup/hosts/{hostname}/build-cache
func handleCleanupBuildCachePrune(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	if hostname == "" {
		http.Error(w, "hostname is required", http.StatusBadRequest)
		return
	}

	var options CleanupOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Require confirmation for non-dry-run operations
	if !options.DryRun && options.ConfirmationToken == "" {
		http.Error(w, "confirmation token required for destructive operations", http.StatusBadRequest)
		return
	}

	owner := getSessionUser(r)
	job, err := createCleanupJob(r.Context(), "build_cache_prune", "single_host", hostname, owner, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create cleanup job: %v", err), http.StatusInternalServerError)
		return
	}

	// Start the cleanup operation in a goroutine
	go executeCleanupJob(job.ID, "build-cache", hostname, options)

	common.RespondJSON(w, job)
}

// handleCleanupAllHostsSystem handles POST /api/cleanup/all-hosts/system
func handleCleanupAllHostsSystem(w http.ResponseWriter, r *http.Request) {
	var options CleanupOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Require confirmation for non-dry-run operations
	if !options.DryRun && options.ConfirmationToken == "" {
		http.Error(w, "confirmation token required for destructive operations", http.StatusBadRequest)
		return
	}

	owner := getSessionUser(r)
	job, err := createCleanupJob(r.Context(), "system_prune", "all_hosts", "all", owner, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create cleanup job: %v", err), http.StatusInternalServerError)
		return
	}

	// Start the cleanup operation in a goroutine
	go executeCleanupJobAllHosts(job.ID, "system", options)

	common.RespondJSON(w, job)
}

// handleGetCleanupJob handles GET /api/cleanup/jobs/{jobId}
func handleGetCleanupJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	if jobID == "" {
		http.Error(w, "job ID is required", http.StatusBadRequest)
		return
	}

	job, err := getCleanupJob(r.Context(), jobID)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	common.RespondJSON(w, job)
}

// handleCleanupSpacePreview handles GET /api/cleanup/preview/{operation}/{hostname}
func handleCleanupSpacePreview(w http.ResponseWriter, r *http.Request) {
	operation := chi.URLParam(r, "operation")
	hostname := chi.URLParam(r, "hostname")

	if operation == "" || hostname == "" {
		http.Error(w, "operation and hostname are required", http.StatusBadRequest)
		return
	}

	preview, err := getSpacePreview(r.Context(), hostname, operation)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get space preview: %v", err), http.StatusInternalServerError)
		return
	}

	common.RespondJSON(w, preview)
}

// handleCleanupAllHostsPreview handles GET /api/cleanup/preview/{operation}/all-hosts
func handleCleanupAllHostsPreview(w http.ResponseWriter, r *http.Request) {
	operation := chi.URLParam(r, "operation")
	if operation == "" {
		http.Error(w, "operation is required", http.StatusBadRequest)
		return
	}

	// Get all hosts
	hosts, err := database.ListHosts(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get hosts: %v", err), http.StatusInternalServerError)
		return
	}

	type AllHostsPreview struct {
		Operation      string                    `json:"operation"`
		TotalBytes     int64                     `json:"total_bytes"`
		TotalSize      string                    `json:"total_size"`
		HostPreviews   map[string]*SpacePreview  `json:"host_previews"`
		TotalItemCount map[string]int            `json:"total_item_count"`
	}

	allPreview := &AllHostsPreview{
		Operation:      operation,
		HostPreviews:   make(map[string]*SpacePreview),
		TotalItemCount: make(map[string]int),
	}

	totalBytes := int64(0)

	for _, host := range hosts {
		preview, err := getSpacePreview(r.Context(), host.Name, operation)
		if err != nil {
			// Create error preview for failed hosts
			preview = &SpacePreview{
				Operation: operation,
				Status:    "error",
				Error:     err.Error(),
			}
		}
		
		allPreview.HostPreviews[host.Name] = preview
		totalBytes += preview.EstimatedBytes
		
		// Sum up item counts
		for itemType, count := range preview.ItemCount {
			allPreview.TotalItemCount[itemType] += count
		}
	}

	allPreview.TotalBytes = totalBytes
	allPreview.TotalSize = humanizeBytes(totalBytes)
	
	common.RespondJSON(w, allPreview)
}

// handleCleanupJobStream handles GET /api/cleanup/jobs/{jobId}/stream
func handleCleanupJobStream(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	if jobID == "" {
		http.Error(w, "job ID is required", http.StatusBadRequest)
		return
	}

	// Set up SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			job, err := getCleanupJob(ctx, jobID)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: job not found\n\n")
				if flusher != nil {
					flusher.Flush()
				}
				return
			}

			data, _ := json.Marshal(job)
			fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}

			// Stop streaming if job is complete
			if job.Status == "completed" || job.Status == "failed" {
				fmt.Fprintf(w, "event: complete\ndata: %s\n\n", data)
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
		}
	}
}

// executeCleanupJob executes a single-host cleanup job
func executeCleanupJob(jobID, operation, hostname string, options CleanupOptions) {
	ctx := context.Background()

	// Update job status to running
	updateJobStatus(ctx, jobID, "running")

	var result *CleanupResult
	var err error

	switch operation {
	case "system":
		result, err = performSystemPrune(ctx, hostname, options)
	case "image":
		result, err = performImagePrune(ctx, hostname, options)
	case "container":
		result, err = performContainerPrune(ctx, hostname, options)
	case "volume":
		result, err = performVolumePrune(ctx, hostname, options)
	case "network":
		result, err = performNetworkPrune(ctx, hostname, options)
	case "build-cache":
		result, err = performBuildCachePrune(ctx, hostname, options)
	default:
		err = fmt.Errorf("unknown operation: %s", operation)
	}

	if err != nil {
		common.DebugLog("Cleanup job %s failed: %v", jobID, err)
		updateJobStatus(ctx, jobID, "failed")
		updateJobResults(ctx, jobID, map[string]interface{}{
			hostname: map[string]interface{}{
				"status": "failed",
				"error":  err.Error(),
			},
		})
		return
	}

	// Update job with results
	updateJobStatus(ctx, jobID, "completed")
	updateJobResults(ctx, jobID, map[string]interface{}{
		hostname: map[string]interface{}{
			"status":          result.Status,
			"space_reclaimed": result.SpaceReclaimed,
			"items_removed":   result.ItemsRemoved,
			"errors":          result.Errors,
		},
	})

	common.DebugLog("Cleanup job %s completed successfully", jobID)
}

// executeCleanupJobAllHosts executes a cleanup job on all hosts
func executeCleanupJobAllHosts(jobID, operation string, options CleanupOptions) {
	ctx := context.Background()

	// Update job status to running
	updateJobStatus(ctx, jobID, "running")

	// Get all hosts
	hosts, err := database.ListHosts(ctx)
	if err != nil {
		common.DebugLog("Failed to get hosts for cleanup job %s: %v", jobID, err)
		updateJobStatus(ctx, jobID, "failed")
		return
	}

	if len(hosts) == 0 {
		common.DebugLog("No hosts found for cleanup job %s", jobID)
		updateJobStatus(ctx, jobID, "completed")
		return
	}

	results := make(map[string]interface{})
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Update progress
	updateJobProgress(ctx, jobID, map[string]interface{}{
		"total_hosts":     len(hosts),
		"completed_hosts": 0,
		"current_host":    "",
	})

	completedHosts := 0

	for _, host := range hosts {
		wg.Add(1)
		go func(host database.HostRow) {
			hostname := host.Name
			defer wg.Done()

			// Update current host
			mu.Lock()
			updateJobProgress(ctx, jobID, map[string]interface{}{
				"total_hosts":       len(hosts),
				"completed_hosts":   completedHosts,
				"current_host":      hostname,
				"current_operation": fmt.Sprintf("cleaning %s", operation),
			})
			mu.Unlock()

			var result *CleanupResult
			var err error

			switch operation {
			case "system":
				result, err = performSystemPrune(ctx, hostname, options)
			case "image":
				result, err = performImagePrune(ctx, hostname, options)
			case "container":
				result, err = performContainerPrune(ctx, hostname, options)
			case "volume":
				result, err = performVolumePrune(ctx, hostname, options)
			case "network":
				result, err = performNetworkPrune(ctx, hostname, options)
			case "build-cache":
				result, err = performBuildCachePrune(ctx, hostname, options)
			}

			mu.Lock()
			if err != nil {
				results[hostname] = map[string]interface{}{
					"status": "failed",
					"error":  err.Error(),
				}
			} else {
				results[hostname] = map[string]interface{}{
					"status":          result.Status,
					"space_reclaimed": result.SpaceReclaimed,
					"items_removed":   result.ItemsRemoved,
					"errors":          result.Errors,
				}
			}
			completedHosts++
			
			// Update progress
			updateJobProgress(ctx, jobID, map[string]interface{}{
				"total_hosts":     len(hosts),
				"completed_hosts": completedHosts,
				"current_host":    "",
			})
			mu.Unlock()
		}(host)
	}

	wg.Wait()

	// Update final results
	updateJobStatus(ctx, jobID, "completed")
	updateJobResults(ctx, jobID, results)

	common.DebugLog("All-hosts cleanup job %s completed", jobID)
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

	_, err := common.DB.Exec(ctx, `
		INSERT INTO cleanup_jobs (id, operation, scope, target, status, dry_run, force, exclude_filters, created_at, progress, results, owner)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, job.ID, job.Operation, job.Scope, job.Target, job.Status, job.DryRun, job.Force, string(excludeFiltersJSON), job.CreatedAt, "{}", "{}", job.Owner)

	if err != nil {
		return nil, fmt.Errorf("failed to create cleanup job: %w", err)
	}

	common.DebugLog("Created cleanup job %s: %s %s %s", job.ID, operation, scope, target)
	return job, nil
}

// updateJobProgress updates the progress of a cleanup job
func updateJobProgress(ctx context.Context, jobID string, progress map[string]interface{}) error {
	progressJSON, _ := json.Marshal(progress)
	
	_, err := common.DB.Exec(ctx, `
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

	_, err := common.DB.Exec(ctx, query, args...)
	return err
}

// updateJobResults updates the results of a cleanup job
func updateJobResults(ctx context.Context, jobID string, results map[string]interface{}) error {
	resultsJSON, _ := json.Marshal(results)
	
	_, err := common.DB.Exec(ctx, `
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

	err := common.DB.QueryRow(ctx, `
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

// performSystemPrune executes docker system prune on a specific host
func performSystemPrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	common.DebugLog("Starting system prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := database.GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		common.DebugLog("DRY RUN: System prune on %s - no actual cleanup performed", hostName)
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

	common.DebugLog("System prune completed on %s: %s reclaimed", hostName, result.SpaceReclaimed)
	return result, nil
}

// performImagePrune executes docker image prune on a specific host
func performImagePrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	common.DebugLog("Starting image prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := database.GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		common.DebugLog("DRY RUN: Image prune on %s - no actual cleanup performed", hostName)
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

	common.DebugLog("Image prune completed on %s: %s reclaimed", hostName, result.SpaceReclaimed)
	return result, nil
}

// performContainerPrune executes docker container prune on a specific host
func performContainerPrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	common.DebugLog("Starting container prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := database.GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		common.DebugLog("DRY RUN: Container prune on %s - no actual cleanup performed", hostName)
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

	common.DebugLog("Container prune completed on %s: %s reclaimed", hostName, result.SpaceReclaimed)
	return result, nil
}

// performVolumePrune executes docker volume prune on a specific host
func performVolumePrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	common.DebugLog("Starting volume prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := database.GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		common.DebugLog("DRY RUN: Volume prune on %s - no actual cleanup performed", hostName)
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

	common.DebugLog("Volume prune completed on %s: %s reclaimed", hostName, result.SpaceReclaimed)
	return result, nil
}

// performNetworkPrune executes docker network prune on a specific host
func performNetworkPrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	common.DebugLog("Starting network prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := database.GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		common.DebugLog("DRY RUN: Network prune on %s - no actual cleanup performed", hostName)
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

	common.DebugLog("Network prune completed on %s", hostName)
	return result, nil
}

// performBuildCachePrune executes docker buildx prune on a specific host
func performBuildCachePrune(ctx context.Context, hostName string, options CleanupOptions) (*CleanupResult, error) {
	common.DebugLog("Starting build cache prune on host %s (dry_run: %t)", hostName, options.DryRun)
	
	host, err := database.GetHostByName(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	result := &CleanupResult{
		ItemsRemoved: make(map[string]int),
		Errors:       []string{},
		Status:       "completed",
	}

	if options.DryRun {
		common.DebugLog("DRY RUN: Build cache prune on %s - no actual cleanup performed", hostName)
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

	common.DebugLog("Build cache prune completed on %s: %s reclaimed", hostName, spaceReclaimed)
	return result, nil
}

// runDockerCommand executes a Docker command using the appropriate method (local Docker client or SSH)
func runDockerCommand(host database.HostRow, command string) (string, error) {
	url, _ := services.DockerURLFor(host)
	
	// For local Docker socket access, use direct execution
	if strings.HasPrefix(url, "unix://") && services.LocalHostAllowed(host) {
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

// getSpacePreview analyzes how much space can be freed by a cleanup operation
func getSpacePreview(ctx context.Context, hostName string, operation string) (*SpacePreview, error) {
	common.DebugLog("Getting space preview for %s operation on host %s", operation, hostName)

	preview := &SpacePreview{
		Operation:  operation,
		ItemCount:  make(map[string]int),
		Details:    []string{},
		Status:     "success",
		EstimatedSize: "0B",
		EstimatedBytes: 0,
	}

	// For now, return a minimal preview - this could be expanded later
	preview.Details = append(preview.Details, fmt.Sprintf("%s cleanup available", operation))
	return preview, nil
}