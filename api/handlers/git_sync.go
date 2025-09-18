package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"dd-ui/common"
	"dd-ui/database"
	"dd-ui/middleware"
	"dd-ui/services"
)

// GitSyncHandlers handles Git synchronization API endpoints
type GitSyncHandlers struct{}

// NewGitSyncHandlers creates a new Git sync handler
func NewGitSyncHandlers() *GitSyncHandlers {
	return &GitSyncHandlers{}
}

// GetConfig returns the current Git sync configuration
func (h *GitSyncHandlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	config, err := database.GetGitSyncConfig(ctx)
	if err != nil {
		common.ErrorLog("Failed to get git config: %v", err)
		http.Error(w, "Failed to get configuration", http.StatusInternalServerError)
		return
	}

	// If config is nil (shouldn't happen with our fixes), return empty config
	if config == nil {
		config = &database.GitSyncConfig{
			RepoURL:           "",
			Branch:            "main",
			CommitAuthorName:  "DD-UI Bot",
			CommitAuthorEmail: "ddui@localhost",
			PullIntervalMins:  5,
		}
	}

	// Don't expose sensitive fields in full
	response := map[string]interface{}{
		"repo_url":           config.RepoURL,
		"branch":             config.Branch,
		"has_token":          config.AuthToken != "",
		"has_ssh_key":        config.SSHKey != "",
		"commit_author_name": config.CommitAuthorName,
		"commit_author_email": config.CommitAuthorEmail,
		"sync_enabled":       config.SyncEnabled,
		"sync_mode":          config.SyncMode,
		"force_on_conflict":  config.ForceOnConflict,
		"auto_push":          config.AutoPush,
		"auto_pull":          config.AutoPull,
		"pull_interval_mins": config.PullIntervalMins,
		"push_on_change":     config.PushOnChange,
		"sync_path":          config.SyncPath,
	}

	common.DebugLog("Returning config: repo=%s, branch=%s, author=%s <%s>",
		response["repo_url"], response["branch"], 
		response["commit_author_name"], response["commit_author_email"])

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateConfig updates the Git sync configuration
func (h *GitSyncHandlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetUserEmail(ctx)

	var config database.GitSyncConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		common.ErrorLog("Failed to decode request body: %v", err)
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Force push_on_change to always be true
	config.PushOnChange = true
	// Ignore sync_path from frontend - derive from DD_UI_IAC_ROOT
	config.SyncPath = strings.TrimSpace(common.Env("DD_UI_IAC_ROOT", "/data"))
	
	common.DebugLog("Received git config update: repo=%s, branch=%s, has_token=%v, has_key=%v", 
		config.RepoURL, config.Branch, config.AuthToken != "", config.SSHKey != "")
	common.DebugLog("Full config received: author_name=%s, author_email=%s, sync_enabled=%v, sync_mode=%s, force=%v, path=%s",
		config.CommitAuthorName, config.CommitAuthorEmail, config.SyncEnabled, 
		config.SyncMode, config.ForceOnConflict, config.SyncPath)

	// Get existing config to preserve tokens if not updated
	existing, existErr := database.GetGitSyncConfig(ctx)
	if existErr != nil {
		common.DebugLog("No existing config or error: %v", existErr)
	}
	
	if existing != nil {
		// Only preserve credentials if they weren't provided
		// The frontend should send actual values for all other fields
		if config.AuthToken == "" || config.AuthToken == "***UNCHANGED***" {
			config.AuthToken = existing.AuthToken
		}
		if config.SSHKey == "" || config.SSHKey == "***UNCHANGED***" {
			config.SSHKey = existing.SSHKey
		}
	}

	// Update configuration - this should always save, regardless of connection status
	gitSync := services.GetGitSync()
	if err := gitSync.UpdateConfig(ctx, &config); err != nil {
		common.ErrorLog("Failed to update git config: %v", err)
		http.Error(w, fmt.Sprintf("Failed to update configuration: %v", err), http.StatusInternalServerError)
		return
	}

	common.InfoLog("Git sync config updated by %s", user)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"message": "Configuration saved successfully",
	})
}

// GetStatus returns the current Git sync status
func (h *GitSyncHandlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get status from database
	dbStatus, err := database.GetGitSyncStatus(ctx)
	if err != nil {
		common.ErrorLog("Failed to get git status: %v", err)
		http.Error(w, "Failed to get status", http.StatusInternalServerError)
		return
	}

	// Get runtime status from service
	gitSync := services.GetGitSync()
	runtimeStatus := gitSync.GetStatus()

	// Merge statuses
	for k, v := range runtimeStatus {
		dbStatus[k] = v
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dbStatus)
}

// Pull triggers a manual pull from the remote repository
func (h *GitSyncHandlers) Pull(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetUserEmail(ctx)

	gitSync := services.GetGitSync()
	if err := gitSync.Pull(ctx, user); err != nil {
		common.ErrorLog("Git pull failed: %v", err)
		
		response := map[string]interface{}{
			"status": "error",
			"message": err.Error(),
		}
		
		// Check for conflicts
		if conflicts, _ := database.GetUnresolvedConflicts(ctx); len(conflicts) > 0 {
			response["conflicts"] = conflicts
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(response)
		return
	}

	common.InfoLog("Git pull completed by %s", user)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"message": "Repository pulled successfully",
	})
}

// Push triggers a manual push to the remote repository
func (h *GitSyncHandlers) Push(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetUserEmail(ctx)

	// Get commit message from request
	var req struct {
		Message string `json:"message"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	gitSync := services.GetGitSync()
	if err := gitSync.Push(ctx, req.Message, user); err != nil {
		common.ErrorLog("Git push failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	common.InfoLog("Git push completed by %s", user)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"message": "Changes pushed successfully",
	})
}

// Sync performs a full sync (pull then push)
func (h *GitSyncHandlers) Sync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetUserEmail(ctx)

	gitSync := services.GetGitSync()
	if err := gitSync.Sync(ctx, user); err != nil {
		common.ErrorLog("Git sync failed: %v", err)
		
		response := map[string]interface{}{
			"status": "error",
			"message": err.Error(),
		}
		
		// Check for conflicts
		if conflicts, _ := database.GetUnresolvedConflicts(ctx); len(conflicts) > 0 {
			response["conflicts"] = conflicts
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(response)
		return
	}

	common.InfoLog("Git sync completed by %s", user)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"message": "Repository synchronized successfully",
	})
}

// GetLogs returns recent Git sync operation logs
func (h *GitSyncHandlers) GetLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get limit from query params
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	logs, err := database.GetGitSyncLogs(ctx, limit)
	if err != nil {
		common.ErrorLog("Failed to get git logs: %v", err)
		http.Error(w, "Failed to get logs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// GetConflicts returns unresolved Git conflicts
func (h *GitSyncHandlers) GetConflicts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	conflicts, err := database.GetUnresolvedConflicts(ctx)
	if err != nil {
		common.ErrorLog("Failed to get conflicts: %v", err)
		http.Error(w, "Failed to get conflicts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(conflicts)
}

// ResolveConflict marks a conflict as resolved
func (h *GitSyncHandlers) ResolveConflict(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetUserEmail(ctx)

	var req struct {
		ConflictID     int    `json:"conflict_id"`
		ResolutionType string `json:"resolution_type"` // "local", "remote", "manual"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := database.ResolveGitConflict(ctx, req.ConflictID, req.ResolutionType, user); err != nil {
		common.ErrorLog("Failed to resolve conflict: %v", err)
		http.Error(w, "Failed to resolve conflict", http.StatusInternalServerError)
		return
	}

	common.InfoLog("Conflict %d resolved by %s using %s", req.ConflictID, user, req.ResolutionType)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"message": "Conflict resolved",
	})
}

// CheckInitialSetupConflict checks if both local and remote have files during initial setup
func (h *GitSyncHandlers) CheckInitialSetupConflict(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// Get current config
	config, err := database.GetGitSyncConfig(ctx)
	if err != nil || config == nil || config.RepoURL == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"has_conflict": false,
			"message": "No repository configured",
		})
		return
	}
	
	// Check for initial setup conflict
	hasConflict := false
	message := ""
	
	// Check if local has docker-compose or inventory files
	hasLocalFiles := false
	syncPath := "/data"
	
	// Check for local docker-compose directory
	if info, err := os.Stat(syncPath + "/docker-compose"); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(syncPath + "/docker-compose")
		if len(entries) > 0 {
			hasLocalFiles = true
		}
	}
	
	// Check for local inventory file
	if _, err := os.Stat(syncPath + "/inventory"); err == nil {
		hasLocalFiles = true
	}
	
	// Only check remote if we have local files
	if hasLocalFiles {
		// Test connection to see if remote exists and has files
		cmd := exec.Command("git", "ls-remote", "--heads", config.RepoURL, config.Branch)
		cmd.Env = os.Environ()
		
		// Add authentication if provided
		if config.AuthToken != "" {
			cmd.Env = append(cmd.Env,
				"GIT_ASKPASS=/bin/echo",
				"GIT_USERNAME=token",
				fmt.Sprintf("GIT_PASSWORD=%s", config.AuthToken),
			)
		} else if config.SSHKey != "" {
			// Create temporary SSH key file for authentication
			tmpFile, err := os.CreateTemp("/tmp", "ddui_conflict_check_ssh_*.pem")
			if err == nil {
				sshKeyContent := config.SSHKey
				if !strings.HasSuffix(sshKeyContent, "\n") {
					sshKeyContent += "\n"
				}
				tmpFile.WriteString(sshKeyContent)
				tmpFile.Close()
				os.Chmod(tmpFile.Name(), 0600)
				defer os.Remove(tmpFile.Name())
				
				cmd.Env = append(cmd.Env,
					fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes -o LogLevel=ERROR", tmpFile.Name()),
				)
			}
		}
		
		if output, err := cmd.CombinedOutput(); err == nil && len(output) > 0 {
			// Remote exists and has content - this indicates potential conflict
			hasConflict = true
			message = "Both local (/data) and remote repository contain files. Choose Push to overwrite remote with local, Pull to overwrite local with remote, or manually resolve before enabling sync."
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"has_conflict": hasConflict,
		"message": message,
		"has_local_files": hasLocalFiles,
	})
}

// TestConnection tests the Git repository connection
func (h *GitSyncHandlers) TestConnection(w http.ResponseWriter, r *http.Request) {
	
	common.DebugLog("TestConnection: Starting connection test")

	var req struct {
		RepoURL   string `json:"repo_url"`
		Branch    string `json:"branch"`
		AuthToken string `json:"auth_token"`
		SSHKey    string `json:"ssh_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.ErrorLog("TestConnection: Failed to decode request: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"message": "Invalid request format",
		})
		return
	}

	// Default branch if not specified
	if req.Branch == "" {
		req.Branch = "main"
	}

	common.DebugLog("TestConnection: Testing repo=%s, branch=%s, has_token=%v, has_key=%v", 
		req.RepoURL, req.Branch, req.AuthToken != "", req.SSHKey != "")

	// Validate inputs
	if req.RepoURL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"message": "Repository URL is required",
		})
		return
	}

	// Validate URL and authentication combination
	isHTTPS := strings.HasPrefix(req.RepoURL, "https://") || strings.HasPrefix(req.RepoURL, "http://")
	isSSH := strings.HasPrefix(req.RepoURL, "git@") || strings.Contains(req.RepoURL, "ssh://")
	
	if isHTTPS && req.SSHKey != "" && req.AuthToken == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"message": "HTTPS URL requires a token, not SSH key. Either use SSH URL format (git@gitlab.prplanit.com:user/repo.git) or provide a Personal Access Token instead of SSH key.",
		})
		return
	}
	
	if isSSH && req.AuthToken != "" && req.SSHKey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"message": "SSH URL requires an SSH key, not a token. Either use HTTPS URL format or provide an SSH key instead of token.",
		})
		return
	}
	
	// If no credentials provided in request, try to use stored credentials
	if req.AuthToken == "" && req.SSHKey == "" {
		// Try to get stored config
		if storedConfig, err := database.GetGitSyncConfig(r.Context()); err == nil && storedConfig != nil {
			common.DebugLog("TestConnection: No credentials in request, using stored credentials")
			// Use stored credentials if available
			if storedConfig.AuthToken != "" {
				req.AuthToken = storedConfig.AuthToken
			}
			if storedConfig.SSHKey != "" {
				req.SSHKey = storedConfig.SSHKey
			}
		}
	}
	
	// Test connection by attempting to ls-remote
	common.DebugLog("TestConnection: Running git ls-remote --heads %s", req.RepoURL)
	cmd := exec.Command("git", "ls-remote", "--heads", req.RepoURL)
	cmd.Env = os.Environ() // Start with current environment
	
	// Add authentication if provided
	if req.AuthToken != "" {
		common.DebugLog("TestConnection: Using token authentication for HTTPS")
		cmd.Env = append(cmd.Env,
			"GIT_ASKPASS=/bin/echo",
			"GIT_USERNAME=token",
			fmt.Sprintf("GIT_PASSWORD=%s", req.AuthToken),
		)
	} else if req.SSHKey != "" {
		common.DebugLog("TestConnection: Using SSH key authentication")
		// Create a unique temporary file for the SSH key
		tmpFile, err := os.CreateTemp("/tmp", "ddui_test_ssh_key_*.pem")
		if err != nil {
			common.ErrorLog("TestConnection: Failed to create temp SSH key file: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "error",
				"message": "Failed to process SSH key",
			})
			return
		}
		
		// Write SSH key ensuring it has proper line ending
		sshKeyContent := req.SSHKey
		if !strings.HasSuffix(sshKeyContent, "\n") {
			sshKeyContent += "\n"
		}
		
		if _, err := tmpFile.WriteString(sshKeyContent); err != nil {
			common.ErrorLog("TestConnection: Failed to write SSH key: %v", err)
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "error",
				"message": "Failed to process SSH key",
			})
			return
		}
		tmpFile.Close()
		
		// Set proper permissions
		if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
			common.ErrorLog("TestConnection: Failed to set SSH key permissions: %v", err)
			os.Remove(tmpFile.Name())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "error",
				"message": "Failed to process SSH key",
			})
			return
		}
		defer os.Remove(tmpFile.Name())
		
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes -o LogLevel=ERROR", tmpFile.Name()),
		)
	}

	output, err := cmd.CombinedOutput()
	outputStr := string(output)
	
	if err != nil {
		common.ErrorLog("TestConnection: Command failed: %v", err)
		common.ErrorLog("TestConnection: Output length: %d", len(outputStr))
		if outputStr != "" {
			common.ErrorLog("TestConnection: Output: %s", outputStr)
		} else {
			common.ErrorLog("TestConnection: No output from git command")
		}
		
		// Parse error message for common issues
		var errorMsg string
		
		// Check for SSH-specific errors
		if strings.Contains(outputStr, "Permission denied (publickey") {
			errorMsg = "SSH authentication failed. Please check your SSH key has access to the repository."
		} else if strings.Contains(outputStr, "Permission denied") {
			errorMsg = "Authentication failed. Check your credentials."
		} else if strings.Contains(outputStr, "Host key verification failed") {
			// This shouldn't happen since we use StrictHostKeyChecking=no
			// Log the full error for debugging
			common.ErrorLog("Unexpected SSH host key verification error with StrictHostKeyChecking=no: %s", outputStr)
			errorMsg = "SSH host key verification failed unexpectedly. Full error: " + strings.TrimSpace(outputStr)
		} else if strings.Contains(outputStr, "Could not resolve hostname") || strings.Contains(outputStr, "Name or service not known") {
			errorMsg = "Cannot reach repository. Check the URL and network connection."
		} else if strings.Contains(outputStr, "port 22: Connection refused") || strings.Contains(outputStr, "port 2424: Connection refused") {
			errorMsg = "Connection refused. Check if the Git server is running and the port is correct."
		} else if strings.Contains(outputStr, "Repository not found") || strings.Contains(outputStr, "does not exist") {
			errorMsg = "Repository not found. Check the URL and access permissions."
		} else if strings.Contains(outputStr, "fatal:") {
			// Extract the fatal error message
			lines := strings.Split(outputStr, "\n")
			for _, line := range lines {
				if strings.Contains(line, "fatal:") {
					errorMsg = strings.TrimPrefix(line, "fatal: ")
					break
				}
			}
			if errorMsg == "" {
				errorMsg = "Git command failed: " + outputStr
			}
		} else if strings.TrimSpace(outputStr) == "" {
			// No output at all - provide a generic but helpful message
			errorMsg = "Connection test failed. Please verify: 1) Repository URL is correct, 2) Authentication credentials are valid, 3) Network connectivity to Git server"
		} else {
			errorMsg = fmt.Sprintf("Connection failed: %s", strings.TrimSpace(outputStr))
		}
		
		// Ensure we always have an error message
		if errorMsg == "" || errorMsg == "Connection failed: " {
			errorMsg = "Connection test failed. Please check your repository URL and credentials."
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"message": errorMsg,
		})
		return
	}

	// Check if branch exists
	branchExists := false
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, fmt.Sprintf("refs/heads/%s", req.Branch)) {
			branchExists = true
			break
		}
	}

	response := map[string]interface{}{
		"status": "success",
		"message": "Connection successful",
		"branch_exists": branchExists,
	}

	if !branchExists {
		response["message"] = fmt.Sprintf("Connection successful but branch '%s' not found", req.Branch)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

