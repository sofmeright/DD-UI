package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dd-ui/common"
	"dd-ui/database"
)

// SyncMode represents the synchronization mode
type SyncMode string

const (
	SyncModeOff  SyncMode = "off"
	SyncModePush SyncMode = "push"  // Local → Git (atomic)
	SyncModePull SyncMode = "pull"  // Git → Local (atomic)
	SyncModeSync SyncMode = "sync"  // Smart bi-directional
)

// GitSyncService manages Git repository synchronization
type GitSyncService struct {
	mu          sync.RWMutex
	config      *database.GitSyncConfig
	syncPath    string    // /data
	gitPath     string    // /data/.git
	workPath    string    // /data/.git-work (git working directory)
	isRunning   bool
	isCloning   bool      // Track if currently cloning to prevent interruption
	lastError   error
	stopChan    chan struct{}
	syncTicker  *time.Ticker
}

var (
	gitSync     *GitSyncService
	gitSyncOnce sync.Once
)

// GetGitSync returns the singleton GitSyncService instance
func GetGitSync() *GitSyncService {
	gitSyncOnce.Do(func() {
		// Derive sync path from DD_UI_IAC_ROOT environment variable
		syncPath := strings.TrimSpace(common.Env("DD_UI_IAC_ROOT", "/data"))
		gitSync = &GitSyncService{
			syncPath: syncPath,
			gitPath:  filepath.Join(syncPath, ".git"),
			workPath: filepath.Join(syncPath, ".git-work"), // Separate git working directory for atomic operations
			stopChan: make(chan struct{}),
		}
		common.InfoLog("GitSync: Using sync path from DD_UI_IAC_ROOT: %s", syncPath)
	})
	return gitSync
}

// Initialize loads config from database and starts sync if enabled
func (g *GitSyncService) Initialize(ctx context.Context) error {
	config, err := database.GetGitSyncConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading git config: %w", err)
	}

	g.mu.Lock()
	g.config = config
	g.mu.Unlock()

	if config.SyncEnabled && config.RepoURL != "" {
		return g.Start(ctx)
	}

	return nil
}

// Start begins the Git synchronization process
func (g *GitSyncService) Start(ctx context.Context) error {
	g.mu.Lock()
	
	if g.isRunning {
		g.mu.Unlock()
		return nil
	}
	
	// Set running flag early
	g.isRunning = true
	g.mu.Unlock()

	common.InfoLog("Start: Beginning Git sync initialization for %s", g.config.RepoURL)
	
	// Initialize repository without holding lock
	if err := g.initRepository(ctx); err != nil {
		g.mu.Lock()
		g.isRunning = false
		g.lastError = err
		g.mu.Unlock()
		common.ErrorLog("Start: Failed to initialize repository: %v", err)
		return fmt.Errorf("initializing repository: %w", err)
	}

	// Start auto-pull if enabled
	if g.config.AutoPull && g.config.PullIntervalMins > 0 {
		g.startAutoPull(ctx)
	}

	common.InfoLog("Start: Git sync successfully started for %s", g.config.RepoURL)
	return nil
}

// Stop halts the Git synchronization
func (g *GitSyncService) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.isRunning {
		return
	}

	// Don't stop if we're in the middle of cloning
	if g.isCloning {
		common.WarnLog("Stop: Cannot stop Git sync while clone operation is in progress")
		return
	}

	close(g.stopChan)
	if g.syncTicker != nil {
		g.syncTicker.Stop()
	}

	g.isRunning = false
	g.stopChan = make(chan struct{}) // Reset for next start
	common.InfoLog("Git sync stopped")
}

// initRepository initializes or clones the Git repository
func (g *GitSyncService) initRepository(ctx context.Context) error {
	// Check if .git directory exists
	if _, err := os.Stat(g.gitPath); os.IsNotExist(err) {
		// No .git directory - need to initialize
		if _, err := os.Stat(g.syncPath); err == nil {
			// Directory exists - initialize git in existing directory
			common.InfoLog("Init: Initializing Git in existing /data directory")
			return g.initInExistingDirectory(ctx)
		} else {
			// Directory doesn't exist - create and initialize
			if err := os.MkdirAll(g.syncPath, 0755); err != nil {
				return fmt.Errorf("failed to create data directory: %w", err)
			}
			return g.initInExistingDirectory(ctx)
		}
	}

	// Repository exists, ensure it's configured correctly
	if err := g.configureRepository(ctx); err != nil {
		return err
	}
	
	// Ensure working directory exists and is initialized
	return g.ensureWorkingDirectory(ctx)
}

// initInExistingDirectory initializes git in an existing directory and sets up selective sync
func (g *GitSyncService) initInExistingDirectory(ctx context.Context) error {
	common.InfoLog("InitExisting: Initializing Git repository in %s", g.syncPath)
	
	// Set cloning flag (we're initializing, which is similar)
	g.mu.Lock()
	g.isCloning = true
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		g.isCloning = false
		g.mu.Unlock()
	}()

	// Initialize git repository
	cmd := exec.CommandContext(ctx, "git", "init")
	cmd.Dir = g.syncPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init failed: %w\n%s", err, output)
	}
	common.InfoLog("InitExisting: Git repository initialized")

	// Configure repository
	if err := g.configureRepository(ctx); err != nil {
		return err
	}

	// Create .gitignore for selective tracking
	if err := g.createGitIgnore(ctx); err != nil {
		return err
	}

	// Add remote
	remoteCmd := exec.CommandContext(ctx, "git", "remote", "add", "origin", g.config.RepoURL)
	remoteCmd.Dir = g.syncPath
	if output, err := remoteCmd.CombinedOutput(); err != nil {
		// Remote might already exist, try setting URL
		setURLCmd := exec.CommandContext(ctx, "git", "remote", "set-url", "origin", g.config.RepoURL)
		setURLCmd.Dir = g.syncPath
		if output2, err2 := setURLCmd.CombinedOutput(); err2 != nil {
			return fmt.Errorf("failed to set remote: %w\n%s\n%s", err, output, output2)
		}
	}
	common.InfoLog("InitExisting: Remote 'origin' configured to %s", g.config.RepoURL)

	// Fetch from remote (don't merge yet)
	var fetchCmd *exec.Cmd
	var cleanup func()

	if g.config.SSHKey != "" {
		sshCmd, cleanupFunc := g.setupSSHCommand(ctx)
		if sshCmd != "" {
			cleanup = cleanupFunc
			fetchCmd = exec.CommandContext(ctx, "git", "fetch", "origin", g.config.Branch)
			fetchCmd.Dir = g.syncPath
			fetchCmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
		}
	}
	
	if fetchCmd == nil {
		fetchCmd = g.buildGitCommand(ctx, "fetch", "origin", g.config.Branch)
	}

	if cleanup != nil {
		defer cleanup()
	}

	common.InfoLog("InitExisting: Fetching from remote repository...")
	if fetchOutput, err := fetchCmd.CombinedOutput(); err != nil {
		common.WarnLog("InitExisting: Initial fetch failed (repo might be empty): %v\n%s", err, fetchOutput)
		// This is okay - repo might be empty
	}

	// Check if remote branch exists
	listCmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", "origin", g.config.Branch)
	listCmd.Dir = g.syncPath
	listOutput, _ := listCmd.Output()
	
	if len(listOutput) > 0 {
		// Remote branch exists - set up tracking and merge
		common.InfoLog("InitExisting: Remote branch %s exists, setting up tracking", g.config.Branch)
		
		// Create local branch tracking remote
		branchCmd := exec.CommandContext(ctx, "git", "checkout", "-b", g.config.Branch, fmt.Sprintf("origin/%s", g.config.Branch))
		branchCmd.Dir = g.syncPath
		if _, err := branchCmd.CombinedOutput(); err != nil {
			// Branch might already exist
			checkoutCmd := exec.CommandContext(ctx, "git", "checkout", g.config.Branch)
			checkoutCmd.Dir = g.syncPath
			checkoutCmd.CombinedOutput()
			
			// Set upstream
			upstreamCmd := exec.CommandContext(ctx, "git", "branch", "--set-upstream-to", fmt.Sprintf("origin/%s", g.config.Branch), g.config.Branch)
			upstreamCmd.Dir = g.syncPath
			upstreamCmd.CombinedOutput()
		}
		
		// Try to merge remote changes (will handle conflicts later)
		g.mergeRemoteChanges(ctx)
	} else {
		// No remote branch - create initial commit
		common.InfoLog("InitExisting: No remote branch found, will create initial commit")
		g.createInitialCommit(ctx)
	}

	// Ensure working directory exists
	if err := g.ensureWorkingDirectory(ctx); err != nil {
		return fmt.Errorf("failed to ensure working directory: %w", err)
	}

	return nil
}

// createGitIgnore creates a .gitignore file for selective tracking
func (g *GitSyncService) createGitIgnore(ctx context.Context) error {
	gitignorePath := filepath.Join(g.syncPath, ".gitignore")
	
	// Check if .gitignore already exists
	if _, err := os.Stat(gitignorePath); err == nil {
		common.InfoLog("InitExisting: .gitignore already exists")
		return nil
	}

	gitignoreContent := `# DD-UI Git Sync - Selective Tracking
# Only track inventory and docker-compose files

# Ignore everything by default
/*

# But track these specifically
!inventory
!docker-compose/
!docker-compose/**
!README.md
!.gitignore

# Ignore backup files
*.backup
*.bak
*~
.*.swp

# Ignore DD-UI runtime files
.git-sync-state
.dd-ui-cache/
`

	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}
	
	common.InfoLog("InitExisting: Created .gitignore for selective tracking")
	return nil
}

// mergeRemoteChanges attempts to merge remote changes with local
func (g *GitSyncService) mergeRemoteChanges(ctx context.Context) error {
	// Check for initial setup conflict (both sides have files)
	if conflict, msg := g.detectInitialSetupConflict(ctx); conflict {
		common.WarnLog("Initial setup conflict detected: %s", msg)
		return fmt.Errorf("initial setup conflict: %s", msg)
	}
	
	// First, add any local changes to index
	addCmd := exec.CommandContext(ctx, "git", "add", "inventory", "docker-compose")
	addCmd.Dir = g.syncPath
	addCmd.CombinedOutput() // Ignore errors - files might not exist

	// Check if we have local changes
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = g.syncPath
	statusOutput, _ := statusCmd.Output()
	
	if len(statusOutput) > 0 {
		// Commit local changes first with configured author
		authorName := g.config.CommitAuthorName
		if authorName == "" {
			authorName = "DD-UI Bot"
		}
		authorEmail := g.config.CommitAuthorEmail
		if authorEmail == "" {
			authorEmail = "ddui@localhost"
		}
		
		commitCmd := exec.CommandContext(ctx, "git",
			"-c", fmt.Sprintf("user.name=%s", authorName),
			"-c", fmt.Sprintf("user.email=%s", authorEmail),
			"commit", "-m", "Local changes before merge")
		commitCmd.Dir = g.syncPath
		commitCmd.CombinedOutput()
	}

	// Try to merge remote
	var mergeCmd *exec.Cmd
	var cleanup func()

	if g.config.SSHKey != "" {
		sshCmd, cleanupFunc := g.setupSSHCommand(ctx)
		if sshCmd != "" {
			cleanup = cleanupFunc
			mergeCmd = exec.CommandContext(ctx, "git", "pull", "--no-rebase", "origin", g.config.Branch)
			mergeCmd.Dir = g.syncPath
			mergeCmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
		}
	}
	
	if mergeCmd == nil {
		mergeCmd = g.buildGitCommand(ctx, "pull", "--no-rebase", "origin", g.config.Branch)
	}

	if cleanup != nil {
		defer cleanup()
	}

	if output, err := mergeCmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "CONFLICT") {
			common.WarnLog("InitExisting: Merge conflicts detected, will need manual resolution")
			g.handleConflicts(ctx, string(output))
		}
		return fmt.Errorf("merge failed: %w\n%s", err, output)
	}

	return nil
}

// createInitialCommit creates the first commit with existing files
func (g *GitSyncService) createInitialCommit(ctx context.Context) error {
	// Add inventory and docker-compose files
	addCmd := exec.CommandContext(ctx, "git", "add", ".gitignore")
	addCmd.Dir = g.syncPath
	addCmd.CombinedOutput()

	// Add inventory if it exists
	if _, err := os.Stat(filepath.Join(g.syncPath, "inventory")); err == nil {
		addInvCmd := exec.CommandContext(ctx, "git", "add", "inventory")
		addInvCmd.Dir = g.syncPath
		addInvCmd.CombinedOutput()
	}

	// Add docker-compose directory if it exists
	if _, err := os.Stat(filepath.Join(g.syncPath, "docker-compose")); err == nil {
		addDCCmd := exec.CommandContext(ctx, "git", "add", "docker-compose")
		addDCCmd.Dir = g.syncPath
		addDCCmd.CombinedOutput()
	}

	// Create initial commit with configured author
	authorName := g.config.CommitAuthorName
	if authorName == "" {
		authorName = "DD-UI Bot"
	}
	authorEmail := g.config.CommitAuthorEmail
	if authorEmail == "" {
		authorEmail = "ddui@localhost"
	}
	
	commitCmd := exec.CommandContext(ctx, "git",
		"-c", fmt.Sprintf("user.name=%s", authorName),
		"-c", fmt.Sprintf("user.email=%s", authorEmail),
		"commit", "-m", "Initial DD-UI sync commit")
	commitCmd.Dir = g.syncPath
	if output, err := commitCmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			common.InfoLog("InitExisting: No files to commit initially")
			return nil
		}
		return fmt.Errorf("initial commit failed: %w\n%s", err, output)
	}

	common.InfoLog("InitExisting: Created initial commit")
	return nil
}

// cloneRepository clones the configured repository (legacy - kept for empty directory case)
func (g *GitSyncService) cloneRepository(ctx context.Context) error {
	common.InfoLog("Clone: Starting clone of %s to %s", g.config.RepoURL, g.syncPath)
	common.InfoLog("Clone: This may take a while for large repositories...")

	// Set cloning flag
	g.mu.Lock()
	g.isCloning = true
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		g.isCloning = false
		g.mu.Unlock()
	}()

	var cloneCmd *exec.Cmd
	var cleanup func()

	// Handle SSH key authentication specially for clone
	if g.config.SSHKey != "" {
		sshCmd, cleanupFunc := g.setupSSHCommand(ctx)
		if sshCmd != "" {
			cleanup = cleanupFunc
			cloneCmd = exec.CommandContext(ctx, "git", "clone", 
				"--branch", g.config.Branch,
				g.config.RepoURL, g.syncPath)
			cloneCmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
		}
	}
	
	// Fallback to standard method if SSH setup failed or not using SSH
	if cloneCmd == nil {
		cloneCmd = g.buildGitCommand(ctx, "clone", 
			"--branch", g.config.Branch,
			g.config.RepoURL, g.syncPath)
	}

	// Ensure cleanup happens
	if cleanup != nil {
		defer cleanup()
	}

	common.InfoLog("Clone: Executing git clone command (this may take several minutes for large repos)")
	startTime := time.Now()
	output, err := cloneCmd.CombinedOutput()
	cloneTime := time.Since(startTime)
	common.InfoLog("Clone: Operation completed in %v", cloneTime)
	if err != nil {
		g.logOperation(ctx, "clone", "failed", "", "", nil, string(output))
		common.ErrorLog("Git clone failed: %v\nOutput: %s", err, string(output))
		// Provide more helpful error messages
		if strings.Contains(string(output), "Permission denied") || strings.Contains(string(output), "publickey") {
			return fmt.Errorf("SSH authentication failed. Please check your SSH key is correct and has access to the repository: %w", err)
		} else if strings.Contains(string(output), "Could not resolve hostname") {
			return fmt.Errorf("Failed to resolve repository hostname. Please check the repository URL: %w", err)
		} else if strings.Contains(string(output), "Repository not found") || strings.Contains(string(output), "does not exist") {
			return fmt.Errorf("Repository not found. Please check the repository URL and access permissions: %w", err)
		}
		return fmt.Errorf("git clone failed: %w\nOutput: %s", err, output)
	}

	common.InfoLog("Clone: Repository successfully cloned in %v", cloneTime)
	g.logOperation(ctx, "clone", "success", "", "", nil, fmt.Sprintf("Repository cloned successfully in %v", cloneTime))
	return g.configureRepository(ctx)
}

// configureRepository sets up Git configuration
func (g *GitSyncService) configureRepository(ctx context.Context) error {
	// Set user name and email
	cmds := [][]string{
		{"config", "user.name", g.config.CommitAuthorName},
		{"config", "user.email", g.config.CommitAuthorEmail},
		{"config", "pull.rebase", "false"}, // Use merge strategy
	}

	for _, args := range cmds {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = g.syncPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
		}
	}

	// For token auth, update the remote URL
	// For SSH auth, we don't modify the URL - we rely on GIT_SSH_COMMAND
	if g.config.AuthToken != "" {
		remoteURL := g.buildAuthenticatedURL()
		cmd := exec.CommandContext(ctx, "git", "remote", "set-url", "origin", remoteURL)
		cmd.Dir = g.syncPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("setting remote URL: %w", err)
		}
	}

	// Log current remote configuration for debugging
	cmd := exec.CommandContext(ctx, "git", "remote", "-v")
	cmd.Dir = g.syncPath
	if output, err := cmd.Output(); err == nil {
		common.DebugLog("Git remote configuration:\n%s", string(output))
	}

	return nil
}

// buildGitCommand creates a Git command with proper authentication
func (g *GitSyncService) buildGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.syncPath
	cmd.Env = os.Environ()

	// Add authentication via environment variables
	if g.config.AuthToken != "" {
		// For HTTPS with token
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_ASKPASS=/bin/echo"))
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_USERNAME=token"))
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_PASSWORD=%s", g.config.AuthToken))
	} else if g.config.SSHKey != "" {
		// For SSH with key - create a unique temp file for this command
		if sshCmd, cleanup := g.setupSSHCommand(ctx); sshCmd != "" {
			cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
			// Register cleanup to run after command completes
			if cleanup != nil {
				defer cleanup()
			}
		}
	}

	return cmd
}

// setupSSHCommand prepares SSH authentication for Git operations
// Returns the GIT_SSH_COMMAND value and a cleanup function
func (g *GitSyncService) setupSSHCommand(ctx context.Context) (string, func()) {
	if g.config.SSHKey == "" {
		return "", nil
	}

	// Create a unique temporary file for the SSH key
	tmpFile, err := os.CreateTemp("/tmp", "ddui_git_ssh_key_*.pem")
	if err != nil {
		common.ErrorLog("Failed to create temp SSH key file: %v", err)
		return "", nil
	}

	// Write the SSH key to the file
	sshKeyContent := g.config.SSHKey
	// Ensure the key ends with a newline
	if !strings.HasSuffix(sshKeyContent, "\n") {
		sshKeyContent += "\n"
	}

	if _, err := tmpFile.WriteString(sshKeyContent); err != nil {
		common.ErrorLog("Failed to write SSH key: %v", err)
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", nil
	}
	tmpFile.Close()

	// Set proper permissions
	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		common.ErrorLog("Failed to set SSH key permissions: %v", err)
		os.Remove(tmpFile.Name())
		return "", nil
	}

	// Create cleanup function
	cleanup := func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			common.DebugLog("Failed to remove temp SSH key: %v", err)
		}
	}

	// Build SSH command with proper options
	// -o IdentitiesOnly=yes ensures only the specified key is used
	// -o StrictHostKeyChecking=no bypasses host key verification (needed for automated systems)
	// -o UserKnownHostsFile=/dev/null prevents host key storage
	// -o LogLevel=ERROR reduces verbosity
	sshCmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes -o LogLevel=ERROR", tmpFile.Name())
	
	common.InfoLog("Git SSH authentication configured using provided SSH key")
	common.DebugLog("SSH key file created at: %s (will be cleaned up after operation)", tmpFile.Name())
	return sshCmd, cleanup
}

// buildAuthenticatedURL creates a Git URL with embedded authentication
func (g *GitSyncService) buildAuthenticatedURL() string {
	if g.config.AuthToken == "" {
		return g.config.RepoURL
	}

	// For GitHub/GitLab HTTPS URLs, embed the token
	if strings.HasPrefix(g.config.RepoURL, "https://") {
		parts := strings.SplitN(g.config.RepoURL, "://", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("https://token:%s@%s", g.config.AuthToken, parts[1])
		}
	}

	return g.config.RepoURL
}

// atomicPull performs an atomic pull operation (Git → Local)
// Preserves local changes in git history before overwriting with remote
func (g *GitSyncService) atomicPull(ctx context.Context, force bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	common.InfoLog("AtomicPull: Starting pull from remote")
	startTime := time.Now()
	
	// Ensure working directory exists
	if err := g.ensureWorkingDirectory(ctx); err != nil {
		return fmt.Errorf("failed to ensure working directory: %w", err)
	}

	// Setup SSH if needed
	var cleanup func()
	var gitEnv []string
	if g.config.SSHKey != "" {
		sshCmd, cleanupFunc := g.setupSSHCommand(ctx)
		if sshCmd != "" {
			cleanup = cleanupFunc
			// Use command-specific environment instead of global
			gitEnv = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
		}
	}
	if cleanup != nil {
		defer cleanup()
	}
	
	// If no SSH key, use current environment
	if gitEnv == nil {
		gitEnv = os.Environ()
	}

	// Step 1: Pull latest from remote
	common.InfoLog("AtomicPull: Pulling latest from remote")
	pullCmd := exec.CommandContext(ctx, "git", "pull", "origin", g.config.Branch)
	pullCmd.Dir = g.workPath
	pullCmd.Env = gitEnv
	if _, err := pullCmd.CombinedOutput(); err != nil {
		// If pull fails, try to reset and pull again
		common.WarnLog("Pull failed, resetting and retrying: %v", err)
		
		// Reset to match remote
		resetCmd := exec.CommandContext(ctx, "git", "reset", "--hard", fmt.Sprintf("origin/%s", g.config.Branch))
		resetCmd.Dir = g.workPath
		resetCmd.Env = gitEnv
		resetCmd.CombinedOutput()
		
		// Try pull again
		pullCmd2 := exec.CommandContext(ctx, "git", "pull", "origin", g.config.Branch)
		pullCmd2.Dir = g.workPath
		pullCmd2.Env = gitEnv
		if output, err := pullCmd2.CombinedOutput(); err != nil {
			return fmt.Errorf("git pull failed after reset: %w\n%s", err, output)
		}
	}

	// Step 2: Check if local files differ from what's in the repo
	// First save the current HEAD commit hash
	headHashCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	headHashCmd.Dir = g.workPath
	headHashBytes, _ := headHashCmd.Output()
	originalHead := strings.TrimSpace(string(headHashBytes))
	
	// Copy local files into the working directory to see if they differ
	common.InfoLog("AtomicPull: Checking for local changes to preserve")
	if err := g.copyFromDataToGit(); err != nil {
		common.WarnLog("Failed to copy local files for comparison: %v", err)
	}
	
	// Check if there are differences
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = g.workPath
	statusOutput, _ := statusCmd.Output()
	
	if len(statusOutput) > 0 {
		// There are local changes that differ from remote - preserve them in history
		common.InfoLog("AtomicPull: Local files differ from remote, preserving in history")
		
		// Stage all changes
		addCmd := exec.CommandContext(ctx, "git", "add", "-A")
		addCmd.Dir = g.workPath
		addCmd.CombinedOutput()
		
		// Commit with preservation message
		authorName := g.config.CommitAuthorName
		if authorName == "" {
			authorName = "DD-UI Bot"
		}
		authorEmail := g.config.CommitAuthorEmail
		if authorEmail == "" {
			authorEmail = "ddui@localhost"
		}
		
		commitMsg := fmt.Sprintf("info: DD-UI ignored these local changes [%s]", time.Now().Format("2006-01-02 15:04:05"))
		commitCmd := exec.CommandContext(ctx, "git",
			"-c", fmt.Sprintf("user.name=%s", authorName),
			"-c", fmt.Sprintf("user.email=%s", authorEmail),
			"commit", "-m", commitMsg)
		commitCmd.Dir = g.workPath
		commitCmd.Env = gitEnv
		if output, err := commitCmd.CombinedOutput(); err != nil {
			if !strings.Contains(string(output), "nothing to commit") {
				common.WarnLog("Failed to preserve local changes: %v\n%s", err, output)
			}
		} else {
			common.InfoLog("AtomicPull: Local changes preserved in commit")
			
			// Push the preservation commit - this is critical for audit trail
			pushCmd := exec.CommandContext(ctx, "git", "push", "origin", g.config.Branch)
			pushCmd.Dir = g.workPath
			pushCmd.Env = gitEnv
			if pushOutput, err := pushCmd.CombinedOutput(); err != nil {
				// If we can't push the preservation commit, we should fail
				// This ensures we never lose track of what was overwritten
				return fmt.Errorf("failed to push preservation commit (local changes would be lost): %w\n%s", err, pushOutput)
			}
			common.InfoLog("AtomicPull: Preservation commit pushed successfully")
		}
	}
	
	// Step 3: Reset back to the original HEAD (the commit before we preserved local changes)
	// This gives us the clean remote state without our local changes
	common.InfoLog("AtomicPull: Resetting to clean remote state")
	resetToCmd := exec.CommandContext(ctx, "git", "reset", "--hard", originalHead)
	resetToCmd.Dir = g.workPath
	resetToCmd.CombinedOutput()
	
	// Step 4: Now copy the clean remote state to local /data
	common.InfoLog("AtomicPull: Applying remote files to /data")
	if err := g.copyFromGitToData(); err != nil {
		return fmt.Errorf("failed to sync from git to data: %w", err)
	}

	// Step 5: Update last sync hash
	if hash, err := g.getCurrentCommit(ctx); err == nil {
		database.UpdateLastSyncHash(ctx, hash)
	}

	duration := time.Since(startTime).Milliseconds()
	common.InfoLog("AtomicPull: Completed in %dms", duration)
	g.logOperationWithDuration(ctx, "pull", "success", "", "", nil, "Atomic pull completed", "system", int(duration))

	return nil
}

// Pull is the public interface for pulling changes
func (g *GitSyncService) Pull(ctx context.Context, initiatedBy string) error {
	// Use force setting from config
	force := g.config != nil && g.config.ForceOnConflict
	return g.atomicPull(ctx, force)
}

// atomicPush performs an atomic push operation (Local → Git)
// Always pulls first to preserve history, then pushes local state
func (g *GitSyncService) atomicPush(ctx context.Context, force bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	common.InfoLog("AtomicPush: Starting push to remote")
	common.InfoLog("AtomicPush: Working directory: %s", g.workPath)
	common.InfoLog("AtomicPush: Data directory: %s", g.syncPath)
	
	// Ensure working directory exists before attempting push
	if err := g.ensureWorkingDirectory(ctx); err != nil {
		return fmt.Errorf("failed to ensure working directory: %w", err)
	}
	
	startTime := time.Now()

	// Setup SSH if needed
	var cleanup func()
	var gitEnv []string
	if g.config.SSHKey != "" {
		sshCmd, cleanupFunc := g.setupSSHCommand(ctx)
		if sshCmd != "" {
			cleanup = cleanupFunc
			gitEnv = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
		}
	}
	if cleanup != nil {
		defer cleanup()
	}
	
	// If no SSH key, use current environment
	if gitEnv == nil {
		gitEnv = os.Environ()
	}

	// Step 1: Pull latest from remote to ensure we have current state
	common.InfoLog("AtomicPush: Pulling latest from remote first")
	pullCmd := exec.CommandContext(ctx, "git", "pull", "origin", g.config.Branch)
	pullCmd.Dir = g.workPath
	pullCmd.Env = gitEnv
	if pullOutput, err := pullCmd.CombinedOutput(); err != nil {
		// If pull fails due to conflicts, we'll overwrite with our local state
		if !strings.Contains(string(pullOutput), "CONFLICT") {
			common.WarnLog("Pull failed (non-conflict): %v\n%s", err, pullOutput)
		} else {
			common.InfoLog("Pull had conflicts, will overwrite with local state")
			// Reset to clean state
			resetCmd := exec.CommandContext(ctx, "git", "reset", "--hard", "HEAD")
			resetCmd.Dir = g.workPath
			resetCmd.CombinedOutput()
		}
	}

	// Step 2: Copy from /data to git working directory using rsync
	common.InfoLog("AtomicPush: Syncing files from %s to %s", g.syncPath, g.workPath)
	if err := g.copyFromDataToGit(); err != nil {
		return fmt.Errorf("failed to sync from data to git: %w", err)
	}
	common.InfoLog("AtomicPush: Sync completed")

	// Step 3: Stage all changes (including deletions)
	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = g.workPath
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w\n%s", err, output)
	}

	// Step 4: Create commit with configured author
	commitMsg := fmt.Sprintf("State push: %s", time.Now().Format("2006-01-02 15:04:05"))
	
	// Use configured author or defaults
	authorName := g.config.CommitAuthorName
	if authorName == "" {
		authorName = "DD-UI Bot"
	}
	authorEmail := g.config.CommitAuthorEmail
	if authorEmail == "" {
		authorEmail = "ddui@localhost"
	}
	
	commitCmd := exec.CommandContext(ctx, "git", 
		"-c", fmt.Sprintf("user.name=%s", authorName),
		"-c", fmt.Sprintf("user.email=%s", authorEmail),
		"commit", "-m", commitMsg)
	commitCmd.Dir = g.workPath
	if output, err := commitCmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			common.InfoLog("AtomicPush: No changes to push - nothing to commit")
			common.InfoLog("AtomicPush: This means no inventory or docker-compose files were found, or they haven't changed")
			return nil
		}
		return fmt.Errorf("git commit failed: %w\n%s", err, output)
	}
	
	common.InfoLog("AtomicPush: Commit successful, preparing to push...")

	// Step 5: Push to remote (never force since we pulled first)
	common.InfoLog("AtomicPush: Executing git push to %s branch %s", g.config.RepoURL, g.config.Branch)
	pushCmd := exec.CommandContext(ctx, "git", "push", "origin", g.config.Branch)
	pushCmd.Dir = g.workPath
	pushCmd.Env = gitEnv
	if output, err := pushCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %w\n%s", err, output)
	}
	common.InfoLog("AtomicPush: Push completed successfully")

	// Step 6: Update last sync hash
	if hash, err := g.getCurrentCommit(ctx); err == nil {
		database.UpdateLastSyncHash(ctx, hash)
	}

	duration := time.Since(startTime).Milliseconds()
	common.InfoLog("AtomicPush: Completed in %dms", duration)
	g.logOperationWithDuration(ctx, "push", "success", "", "", nil, "Atomic push completed", "system", int(duration))

	return nil
}

// Push is the public interface for pushing changes  
func (g *GitSyncService) Push(ctx context.Context, message string, initiatedBy string) error {
	// Use force setting from config
	force := g.config != nil && g.config.ForceOnConflict
	return g.atomicPush(ctx, force)
}

// detectConflict checks if both local and remote have changes since last sync
func (g *GitSyncService) detectConflict(ctx context.Context) (bool, string) {
	// Get current remote commit
	lsRemoteCmd := exec.CommandContext(ctx, "git", "ls-remote", "origin", g.config.Branch)
	lsRemoteCmd.Dir = g.workPath
	
	// Setup SSH if needed for ls-remote
	if g.config.SSHKey != "" {
		sshCmd, cleanup := g.setupSSHCommand(ctx)
		if cleanup != nil {
			defer cleanup()
		}
		if sshCmd != "" {
			lsRemoteCmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
		}
	}
	
	output, err := lsRemoteCmd.CombinedOutput()
	if err != nil {
		return false, ""
	}
	
	parts := strings.Fields(string(output))
	if len(parts) == 0 {
		return false, ""
	}
	remoteHash := parts[0]
	
	// Check if remote changed since last sync
	remoteChanged := g.config.LastSyncHash != "" && g.config.LastSyncHash != remoteHash
	
	// Check if local files changed
	localChanged := g.hasLocalChanges()
	
	if localChanged && remoteChanged {
		return true, "both local and remote have changes"
	}
	
	return false, ""
}

// ensureWorkingDirectory ensures the git working directory exists and is properly initialized
func (g *GitSyncService) ensureWorkingDirectory(ctx context.Context) error {
	// Check if working directory exists and is a valid git repo
	if info, err := os.Stat(filepath.Join(g.workPath, ".git")); err != nil || !info.IsDir() {
		common.InfoLog("Working directory doesn't exist or not a git repo, cloning from %s", g.config.RepoURL)
		
		// Remove any existing directory first
		os.RemoveAll(g.workPath)
		
		// Setup authentication for clone
		var cloneCmd *exec.Cmd
		var cleanup func()
		
		if g.config.SSHKey != "" {
			// SSH authentication
			sshCmd, cleanupFunc := g.setupSSHCommand(ctx)
			if sshCmd != "" {
				cleanup = cleanupFunc
				// Try with specified branch first, fallback to default if it fails
				cloneCmd = exec.CommandContext(ctx, "git", "clone", g.config.RepoURL, g.workPath, "--branch", g.config.Branch)
				cloneCmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
			}
		} else if g.config.AuthToken != "" {
			// HTTPS with token
			authURL := g.buildAuthenticatedURL()
			cloneCmd = exec.CommandContext(ctx, "git", "clone", authURL, g.workPath, "--branch", g.config.Branch)
		} else {
			// No authentication (public repo)
			cloneCmd = exec.CommandContext(ctx, "git", "clone", g.config.RepoURL, g.workPath, "--branch", g.config.Branch)
		}
		
		if cleanup != nil {
			defer cleanup()
		}
		
		// Try to clone the remote repository
		if output, err := cloneCmd.CombinedOutput(); err != nil {
			// If clone fails because branch doesn't exist, try without branch specification
			if strings.Contains(string(output), "not found in upstream") || 
			   strings.Contains(string(output), "Remote branch") && strings.Contains(string(output), "not found") ||
			   strings.Contains(string(output), "couldn't find remote ref") {
				common.WarnLog("Branch %s not found, cloning default branch", g.config.Branch)
				
				// Clone without specifying branch (gets default)
				var fallbackCmd *exec.Cmd
				if g.config.SSHKey != "" && cleanup != nil {
					sshCmd, _ := g.setupSSHCommand(ctx)
					fallbackCmd = exec.CommandContext(ctx, "git", "clone", g.config.RepoURL, g.workPath)
					fallbackCmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
				} else if g.config.AuthToken != "" {
					authURL := g.buildAuthenticatedURL()
					fallbackCmd = exec.CommandContext(ctx, "git", "clone", authURL, g.workPath)
				} else {
					fallbackCmd = exec.CommandContext(ctx, "git", "clone", g.config.RepoURL, g.workPath)
				}
				
				if fallbackOutput, fallbackErr := fallbackCmd.CombinedOutput(); fallbackErr != nil {
					// If fallback also fails, check if repo is empty
					if strings.Contains(string(fallbackOutput), "empty repository") {
						// Continue to empty repo initialization below
						output = fallbackOutput
						err = fallbackErr
					} else {
						return fmt.Errorf("failed to clone repository: %w\n%s", fallbackErr, fallbackOutput)
					}
				} else {
					// Successfully cloned default branch - detect which branch we're on
					branchCmd := exec.CommandContext(ctx, "git", "branch", "--show-current")
					branchCmd.Dir = g.workPath
					if branchBytes, _ := branchCmd.Output(); len(branchBytes) > 0 {
						actualBranch := strings.TrimSpace(string(branchBytes))
						if actualBranch != g.config.Branch {
							common.InfoLog("Repository uses branch '%s', updating config", actualBranch)
							g.config.Branch = actualBranch
							// Note: We should persist this to database but that requires a DB update
						}
					}
					common.InfoLog("Successfully cloned repository")
					return nil
				}
			}
			
			// If clone fails because repo is empty, initialize it
			if strings.Contains(string(output), "empty repository") {
				common.InfoLog("Remote repository is empty, initializing new repo structure")
				
				// Create directory
				if err := os.MkdirAll(g.workPath, 0755); err != nil {
					return fmt.Errorf("failed to create work directory: %w", err)
				}
				
				// Initialize git
				initCmd := exec.CommandContext(ctx, "git", "init")
				initCmd.Dir = g.workPath
				if output, err := initCmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to init work directory: %w\n%s", err, output)
				}
				
				// Set branch name
				branchCmd := exec.CommandContext(ctx, "git", "checkout", "-b", g.config.Branch)
				branchCmd.Dir = g.workPath
				branchCmd.CombinedOutput()
				
				// Add origin
				remoteCmd := exec.CommandContext(ctx, "git", "remote", "add", "origin", g.config.RepoURL)
				remoteCmd.Dir = g.workPath
				remoteCmd.CombinedOutput()
				
				// Create initial README
				readmePath := filepath.Join(g.workPath, "README.md")
				readmeContent := fmt.Sprintf("# DD-UI Git Sync Repository\n\nThis repository is managed by DD-UI for syncing inventory and docker-compose configurations.\n\nInitialized: %s\n", time.Now().Format(time.RFC3339))
				os.WriteFile(readmePath, []byte(readmeContent), 0644)
				
				// Initial commit
				addCmd := exec.CommandContext(ctx, "git", "add", ".")
				addCmd.Dir = g.workPath
				addCmd.CombinedOutput()
				
				authorName := g.config.CommitAuthorName
				if authorName == "" {
					authorName = "DD-UI Bot"
				}
				authorEmail := g.config.CommitAuthorEmail
				if authorEmail == "" {
					authorEmail = "ddui@localhost"
				}
				
				commitCmd := exec.CommandContext(ctx, "git",
					"-c", fmt.Sprintf("user.name=%s", authorName),
					"-c", fmt.Sprintf("user.email=%s", authorEmail),
					"commit", "-m", "Initial repository setup by DD-UI")
				commitCmd.Dir = g.workPath
				commitCmd.CombinedOutput()
				
				common.InfoLog("Created initial repository structure")
			} else {
				return fmt.Errorf("failed to clone repository: %w\n%s", err, output)
			}
			
			// Fetch
			var gitEnv []string
			if g.config.SSHKey != "" {
				sshCmd, cleanup := g.setupSSHCommand(ctx)
				if cleanup != nil {
					defer cleanup()
				}
				if sshCmd != "" {
					gitEnv = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
				}
			} else {
				gitEnv = os.Environ()
			}
			
			fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin")
			fetchCmd.Dir = g.workPath
			fetchCmd.Env = gitEnv
			fetchCmd.CombinedOutput() // Ignore errors - might be empty repo
			
			// Checkout branch
			checkoutCmd := exec.CommandContext(ctx, "git", "checkout", "-b", g.config.Branch)
			checkoutCmd.Dir = g.workPath
			checkoutCmd.CombinedOutput()
		}
		
		common.InfoLog("Git working directory ready at %s", g.workPath)
	}
	
	return nil
}

// detectInitialSetupConflict checks if both local and remote have files during initial setup
func (g *GitSyncService) detectInitialSetupConflict(ctx context.Context) (bool, string) {
	// Check if local has docker-compose or inventory files
	hasLocalFiles := false
	
	// Check for local docker-compose directory
	if info, err := os.Stat(filepath.Join(g.syncPath, "docker-compose")); err == nil && info.IsDir() {
		// Check if directory has any files
		entries, _ := os.ReadDir(filepath.Join(g.syncPath, "docker-compose"))
		if len(entries) > 0 {
			hasLocalFiles = true
		}
	}
	
	// Check for local inventory file
	if _, err := os.Stat(filepath.Join(g.syncPath, "inventory")); err == nil {
		hasLocalFiles = true
	}
	
	if !hasLocalFiles {
		// No local files, safe to pull from remote
		return false, ""
	}
	
	// Check if remote has files
	hasRemoteFiles := false
	
	// Use git ls-tree to check remote contents
	lsTreeCmd := exec.CommandContext(ctx, "git", "ls-tree", "-r", "--name-only", fmt.Sprintf("origin/%s", g.config.Branch))
	lsTreeCmd.Dir = g.syncPath
	if output, err := lsTreeCmd.CombinedOutput(); err == nil && len(output) > 0 {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "docker-compose/") || line == "inventory" {
				hasRemoteFiles = true
				break
			}
		}
	}
	
	if hasLocalFiles && hasRemoteFiles {
		return true, "both local (/data) and remote repository contain files. Please choose Push to overwrite remote with local, Pull to overwrite local with remote, or manually resolve before enabling sync"
	}
	
	return false, ""
}

// hasLocalChanges checks if local files have been modified
func (g *GitSyncService) hasLocalChanges() bool {
	// Compare file timestamps or checksums
	// For now, simple implementation - check if files exist and are recent
	inventoryPath := filepath.Join(g.syncPath, "inventory")
	dockerComposePath := filepath.Join(g.syncPath, "docker-compose")
	
	if info, err := os.Stat(inventoryPath); err == nil {
		if time.Since(info.ModTime()) < 24*time.Hour {
			return true
		}
	}
	
	if info, err := os.Stat(dockerComposePath); err == nil {
		if time.Since(info.ModTime()) < 24*time.Hour {
			return true
		}
	}
	
	return false
}

// backupCurrentState creates a backup of current state before destructive operations
func (g *GitSyncService) backupCurrentState(backupPath string) error {
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return err
	}
	
	// Backup inventory
	if _, err := os.Stat(filepath.Join(g.syncPath, "inventory")); err == nil {
		src := filepath.Join(g.syncPath, "inventory")
		dst := filepath.Join(backupPath, "inventory")
		if err := g.copyFile(src, dst); err != nil {
			common.WarnLog("Failed to backup inventory: %v", err)
		}
	}
	
	// Backup docker-compose directory
	if _, err := os.Stat(filepath.Join(g.syncPath, "docker-compose")); err == nil {
		src := filepath.Join(g.syncPath, "docker-compose")
		dst := filepath.Join(backupPath, "docker-compose")
		if err := g.copyDir(src, dst); err != nil {
			common.WarnLog("Failed to backup docker-compose: %v", err)
		}
	}
	
	common.InfoLog("Backup created at %s", backupPath)
	return nil
}

// clearGitWorkingDirectory removes inventory and docker-compose from git working directory
func (g *GitSyncService) clearGitWorkingDirectory() error {
	// Remove inventory
	inventoryPath := filepath.Join(g.workPath, "inventory")
	if err := os.Remove(inventoryPath); err != nil && !os.IsNotExist(err) {
		common.WarnLog("Failed to remove git inventory: %v", err)
	}
	
	// Remove docker-compose directory
	dockerComposePath := filepath.Join(g.workPath, "docker-compose")
	if err := os.RemoveAll(dockerComposePath); err != nil {
		common.WarnLog("Failed to remove git docker-compose: %v", err)
	}
	
	return nil
}

// copyFromDataToGit copies inventory and docker-compose from /data to git working directory using rsync
func (g *GitSyncService) copyFromDataToGit() error {
	// Use rsync for inventory if it exists
	srcInventory := filepath.Join(g.syncPath, "inventory")
	if _, err := os.Stat(srcInventory); err == nil {
		common.InfoLog("CopyFromDataToGit: Syncing inventory file with rsync")
		dstInventory := filepath.Join(g.workPath, "inventory")
		
		// rsync -avz --delete for exact mirror
		rsyncCmd := exec.Command("rsync", "-avz", srcInventory, dstInventory)
		if _, err := rsyncCmd.CombinedOutput(); err != nil {
			// Fallback to cp if rsync not available
			common.WarnLog("rsync failed, falling back to cp: %v", err)
			cpCmd := exec.Command("cp", "-f", srcInventory, dstInventory)
			if cpOutput, err := cpCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to copy inventory: %w\n%s", err, cpOutput)
			}
		}
		common.InfoLog("CopyFromDataToGit: Inventory synced to %s", dstInventory)
	} else {
		// Remove inventory from git if it doesn't exist locally
		dstInventory := filepath.Join(g.workPath, "inventory")
		if _, err := os.Stat(dstInventory); err == nil {
			common.InfoLog("CopyFromDataToGit: Removing inventory from git (not in /data)")
			os.Remove(dstInventory)
		}
	}
	
	// Use rsync for docker-compose directory
	srcDockerCompose := filepath.Join(g.syncPath, "docker-compose")
	dstDockerCompose := filepath.Join(g.workPath, "docker-compose")
	
	if _, err := os.Stat(srcDockerCompose); err == nil {
		common.InfoLog("CopyFromDataToGit: Syncing docker-compose directory with rsync")
		
		// Ensure destination exists
		os.MkdirAll(dstDockerCompose, 0755)
		
		// rsync -avz --delete for exact mirror (trailing slash important!)
		rsyncCmd := exec.Command("rsync", "-avz", "--delete", srcDockerCompose+"/", dstDockerCompose+"/")
		if _, err := rsyncCmd.CombinedOutput(); err != nil {
			// Fallback to cp if rsync not available
			common.WarnLog("rsync failed, falling back to cp: %v", err)
			
			// Remove destination first for clean copy
			os.RemoveAll(dstDockerCompose)
			cpCmd := exec.Command("cp", "-r", srcDockerCompose, dstDockerCompose)
			if cpOutput, err := cpCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to copy docker-compose: %w\n%s", err, cpOutput)
			}
		}
		common.InfoLog("CopyFromDataToGit: Docker-compose directory synced to %s", dstDockerCompose)
	} else {
		// Remove docker-compose from git if it doesn't exist locally
		if _, err := os.Stat(dstDockerCompose); err == nil {
			common.InfoLog("CopyFromDataToGit: Removing docker-compose from git (not in /data)")
			os.RemoveAll(dstDockerCompose)
		}
	}
	
	return nil
}

// copyFromGitToData copies inventory and docker-compose from git working directory to /data using rsync
func (g *GitSyncService) copyFromGitToData() error {
	// Use rsync for inventory if it exists in git
	srcInventory := filepath.Join(g.workPath, "inventory")
	dstInventory := filepath.Join(g.syncPath, "inventory")
	
	if _, err := os.Stat(srcInventory); err == nil {
		common.InfoLog("CopyFromGitToData: Syncing inventory file from git")
		
		// rsync -avz for exact copy
		rsyncCmd := exec.Command("rsync", "-avz", srcInventory, dstInventory)
		if _, err := rsyncCmd.CombinedOutput(); err != nil {
			// Fallback to cp if rsync not available
			common.WarnLog("rsync failed, falling back to cp: %v", err)
			cpCmd := exec.Command("cp", "-f", srcInventory, dstInventory)
			if cpOutput, err := cpCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to copy inventory: %w\n%s", err, cpOutput)
			}
		}
		common.InfoLog("CopyFromGitToData: Inventory synced to %s", dstInventory)
	} else {
		// Remove inventory from /data if it doesn't exist in git
		if _, err := os.Stat(dstInventory); err == nil {
			common.InfoLog("CopyFromGitToData: Removing inventory from /data (not in git)")
			os.Remove(dstInventory)
		}
	}
	
	// Use rsync for docker-compose directory
	srcDockerCompose := filepath.Join(g.workPath, "docker-compose")
	dstDockerCompose := filepath.Join(g.syncPath, "docker-compose")
	
	if _, err := os.Stat(srcDockerCompose); err == nil {
		common.InfoLog("CopyFromGitToData: Syncing docker-compose directory from git")
		
		// Ensure destination exists
		os.MkdirAll(dstDockerCompose, 0755)
		
		// rsync -avz --delete for exact mirror (trailing slash important!)
		rsyncCmd := exec.Command("rsync", "-avz", "--delete", srcDockerCompose+"/", dstDockerCompose+"/")
		if _, err := rsyncCmd.CombinedOutput(); err != nil {
			// Fallback to cp if rsync not available
			common.WarnLog("rsync failed, falling back to cp: %v", err)
			
			// Remove destination first for clean copy
			os.RemoveAll(dstDockerCompose)
			cpCmd := exec.Command("cp", "-r", srcDockerCompose, dstDockerCompose)
			if cpOutput, err := cpCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to copy docker-compose: %w\n%s", err, cpOutput)
			}
		}
		common.InfoLog("CopyFromGitToData: Docker-compose directory synced to %s", dstDockerCompose)
	} else {
		// Remove docker-compose from /data if it doesn't exist in git
		if _, err := os.Stat(dstDockerCompose); err == nil {
			common.InfoLog("CopyFromGitToData: Removing docker-compose from /data (not in git)")
			os.RemoveAll(dstDockerCompose)
		}
	}
	
	return nil
}

// copyFile copies a single file
func (g *GitSyncService) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()
	
	_, err = io.Copy(destFile, sourceFile)
	return err
}

// copyDir recursively copies a directory
func (g *GitSyncService) copyDir(src, dst string) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	
	// Walk source directory
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Calculate destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)
		
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		
		return g.copyFile(path, dstPath)
	})
}

// Sync performs smart bi-directional sync (no force option)
func (g *GitSyncService) Sync(ctx context.Context, initiatedBy string) error {
	common.InfoLog("SmartSync: Checking for changes")
	
	// Detect if there's a conflict
	if conflict, details := g.detectConflict(ctx); conflict {
		return fmt.Errorf("conflict detected: %s. Use Push or Pull with force to resolve", details)
	}
	
	// Check what changed
	localChanged := g.hasLocalChanges()
	remoteChanged := false
	
	// Check remote for changes
	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", g.config.Branch)
	fetchCmd.Dir = g.workPath
	fetchCmd.CombinedOutput() // Ignore errors, just try to fetch
	
	lsRemoteCmd := exec.CommandContext(ctx, "git", "ls-remote", "origin", g.config.Branch)
	lsRemoteCmd.Dir = g.workPath
	if output, err := lsRemoteCmd.CombinedOutput(); err == nil {
		parts := strings.Fields(string(output))
		if len(parts) > 0 {
			remoteHash := parts[0]
			remoteChanged = g.config.LastSyncHash != "" && g.config.LastSyncHash != remoteHash
		}
	}
	
	// Smart sync decision
	if localChanged && !remoteChanged {
		common.InfoLog("SmartSync: Local changes detected, pushing")
		return g.atomicPush(ctx, false)
	} else if remoteChanged && !localChanged {
		common.InfoLog("SmartSync: Remote changes detected, pulling")
		return g.atomicPull(ctx, false)
	} else if localChanged && remoteChanged {
		// This shouldn't happen as detectConflict should have caught it
		return fmt.Errorf("conflict: both sides have changes")
	} else {
		common.InfoLog("SmartSync: No changes detected")
		return nil
	}
}

// startAutoPull starts the automatic pull timer
func (g *GitSyncService) startAutoPull(ctx context.Context) {
	interval := time.Duration(g.config.PullIntervalMins) * time.Minute
	g.syncTicker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-g.syncTicker.C:
				if err := g.Pull(ctx, "system"); err != nil {
					common.ErrorLog("Auto-pull failed: %v", err)
				}
			case <-g.stopChan:
				return
			}
		}
	}()

	common.InfoLog("Auto-pull enabled with %d minute interval", g.config.PullIntervalMins)
}

// Helper functions

func (g *GitSyncService) getCurrentCommit(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = g.syncPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (g *GitSyncService) getChangedFiles(ctx context.Context, beforeHash, afterHash string) []string {
	if beforeHash == "" || afterHash == "" || beforeHash == afterHash {
		return nil
	}

	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", beforeHash, afterHash)
	cmd.Dir = g.syncPath
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var result []string
	for _, f := range files {
		if f != "" {
			result = append(result, f)
		}
	}
	return result
}

func (g *GitSyncService) inventoryChanged(files []string) bool {
	for _, file := range files {
		if strings.Contains(file, "inventory") {
			return true
		}
	}
	return false
}

func (g *GitSyncService) generateCommitMessage(files []string) string {
	// Analyze changes to create meaningful commit message
	var stacks, hosts, inventory int
	for _, file := range files {
		switch {
		case strings.Contains(file, "inventory"):
			inventory++
		case strings.Contains(file, "docker-compose"):
			stacks++
		case strings.Contains(file, "hosts/"):
			hosts++
		}
	}

	parts := []string{}
	if inventory > 0 {
		parts = append(parts, fmt.Sprintf("%d inventory", inventory))
	}
	if stacks > 0 {
		parts = append(parts, fmt.Sprintf("%d stacks", stacks))
	}
	if hosts > 0 {
		parts = append(parts, fmt.Sprintf("%d hosts", hosts))
	}

	if len(parts) == 0 {
		return fmt.Sprintf("Update %d files", len(files))
	}

	return fmt.Sprintf("Update %s", strings.Join(parts, ", "))
}

func (g *GitSyncService) handleConflicts(ctx context.Context, output string) {
	// Parse conflict information and store for resolution
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "CONFLICT") {
			// Extract file path from conflict message
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, "/") || strings.HasSuffix(part, ".yml") || strings.HasSuffix(part, ".yaml") {
					database.RecordGitConflict(ctx, part, "merge", "")
				}
			}
		}
	}
}

func (g *GitSyncService) logOperation(ctx context.Context, operation, status, beforeHash, afterHash string, files []string, message string) {
	g.logOperationWithDuration(ctx, operation, status, beforeHash, afterHash, files, message, "system", 0)
}

func (g *GitSyncService) logOperationWithDuration(ctx context.Context, operation, status, beforeHash, afterHash string, 
	files []string, message, initiatedBy string, duration int) {
	
	if err := database.LogGitSyncOperation(ctx, database.GitSyncLogEntry{
		Operation:     operation,
		Status:        status,
		CommitBefore:  beforeHash,
		CommitAfter:   afterHash,
		FilesChanged:  files,
		Message:       message,
		InitiatedBy:   initiatedBy,
		DurationMs:    duration,
	}); err != nil {
		common.ErrorLog("Failed to log git operation: %v", err)
	}
}


// GetStatus returns the current sync status
func (g *GitSyncService) GetStatus() map[string]interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()

	status := map[string]interface{}{
		"enabled":    g.config != nil && g.config.SyncEnabled,
		"running":    g.isRunning,
		"cloning":    g.isCloning,
		"repo_url":   "",
		"branch":     "",
		"auto_pull":  false,
		"auto_push":  false,
		"last_error": "",
	}

	if g.config != nil {
		status["repo_url"] = g.config.RepoURL
		status["branch"] = g.config.Branch
		status["auto_pull"] = g.config.AutoPull
		status["auto_push"] = g.config.AutoPush
	}

	if g.lastError != nil {
		status["last_error"] = g.lastError.Error()
	}

	return status
}

// UpdateConfig updates the Git sync configuration
func (g *GitSyncService) UpdateConfig(ctx context.Context, config *database.GitSyncConfig) error {
	common.InfoLog("UpdateConfig: Updating Git sync configuration")
	
	// Check if we're in the middle of cloning
	g.mu.RLock()
	if g.isCloning {
		g.mu.RUnlock()
		common.WarnLog("UpdateConfig: Clone operation in progress, please wait...")
		return fmt.Errorf("clone operation in progress, please wait and try again")
	}
	g.mu.RUnlock()
	
	// Update configuration in database first (doesn't need lock)
	if err := database.UpdateGitSyncConfig(ctx, config); err != nil {
		common.ErrorLog("UpdateConfig: Failed to save config to database: %v", err)
		return err
	}
	common.InfoLog("UpdateConfig: Config saved to database")

	// Quick lock to update config and stop if needed
	g.mu.Lock()
	needsStop := g.isRunning && !g.isCloning
	if needsStop {
		common.InfoLog("UpdateConfig: Stopping existing sync")
		// Stop without lock - we already have it
		if g.isRunning && !g.isCloning {
			close(g.stopChan)
			if g.syncTicker != nil {
				g.syncTicker.Stop()
			}
			g.isRunning = false
			g.stopChan = make(chan struct{}) // Reset for next start
			common.InfoLog("Git sync stopped")
		}
	}
	g.config = config
	g.mu.Unlock()

	// Start sync outside of lock to prevent blocking
	if config.SyncEnabled && config.RepoURL != "" {
		common.InfoLog("UpdateConfig: Starting Git sync for %s", config.RepoURL)
		// Start in background to prevent blocking the HTTP response
		// Use background context so it doesn't get canceled when HTTP request completes
		go func() {
			bgCtx := context.Background()
			if err := g.Start(bgCtx); err != nil {
				common.ErrorLog("UpdateConfig: Failed to start Git sync: %v", err)
				g.mu.Lock()
				g.lastError = err
				g.mu.Unlock()
			} else {
				common.InfoLog("UpdateConfig: Git sync started successfully")
			}
		}()
		return nil
	}

	return nil
}