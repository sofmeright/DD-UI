package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// getSessionUser extracts the user from the session
func getSessionUser(r *http.Request) string {
	sess, _ := store.Get(r, sessionName)
	u, ok := sess.Values["user"].(User)
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

	respondJSON(w, job)
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

	respondJSON(w, job)
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

	respondJSON(w, job)
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

	respondJSON(w, job)
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

	respondJSON(w, job)
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

	respondJSON(w, job)
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

	respondJSON(w, job)
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

	respondJSON(w, job)
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

	respondJSON(w, preview)
}

// handleCleanupAllHostsPreview handles GET /api/cleanup/preview/{operation}/all-hosts
func handleCleanupAllHostsPreview(w http.ResponseWriter, r *http.Request) {
	operation := chi.URLParam(r, "operation")
	if operation == "" {
		http.Error(w, "operation is required", http.StatusBadRequest)
		return
	}

	// Get all hosts
	hosts, err := ListHosts(r.Context())
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
	
	respondJSON(w, allPreview)
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
		debugLog("Cleanup job %s failed: %v", jobID, err)
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

	debugLog("Cleanup job %s completed successfully", jobID)
}

// executeCleanupJobAllHosts executes a cleanup job on all hosts
func executeCleanupJobAllHosts(jobID, operation string, options CleanupOptions) {
	ctx := context.Background()

	// Update job status to running
	updateJobStatus(ctx, jobID, "running")

	// Get all hosts
	hosts, err := ListHosts(ctx)
	if err != nil {
		debugLog("Failed to get hosts for cleanup job %s: %v", jobID, err)
		updateJobStatus(ctx, jobID, "failed")
		return
	}

	if len(hosts) == 0 {
		debugLog("No hosts found for cleanup job %s", jobID)
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
		go func(host HostRow) {
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

	debugLog("All-hosts cleanup job %s completed", jobID)
}