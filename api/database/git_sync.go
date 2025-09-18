package database

import (
	"context"
	"strings"
	"time"
	"dd-ui/common"
)

// GitSyncConfig holds the Git synchronization configuration
type GitSyncConfig struct {
	ID                 int    `json:"id"`
	RepoURL           string `json:"repo_url"`
	Branch            string `json:"branch"`
	AuthToken         string `json:"auth_token"`
	SSHKey            string `json:"ssh_key"`
	CommitAuthorName  string `json:"commit_author_name"`
	CommitAuthorEmail string `json:"commit_author_email"`
	SyncEnabled       bool   `json:"sync_enabled"`
	SyncMode          string `json:"sync_mode"` // 'off', 'push', 'pull', 'sync'
	ForceOnConflict   bool   `json:"force_on_conflict"`
	LastSyncHash      string `json:"last_sync_hash"`
	AutoPush          bool   `json:"auto_push"` // Deprecated
	AutoPull          bool   `json:"auto_pull"` // Deprecated
	PullIntervalMins  int    `json:"pull_interval_mins"`
	PushOnChange      bool   `json:"push_on_change"` // Deprecated
	SyncPath          string `json:"sync_path"`
}

// GitSyncLogEntry represents a git sync operation log
type GitSyncLogEntry struct {
	Operation    string
	Status       string
	CommitBefore string
	CommitAfter  string
	FilesChanged []string
	Additions    int
	Deletions    int
	Message      string
	InitiatedBy  string
	DurationMs   int
}

// GetGitSyncConfig retrieves the Git sync configuration
func GetGitSyncConfig(ctx context.Context) (*GitSyncConfig, error) {
	query := `
		SELECT id, repo_url, branch, auth_token, ssh_key,
		       commit_author_name, commit_author_email,
		       sync_enabled, COALESCE(sync_mode, 'off'), 
		       COALESCE(force_on_conflict, false),
		       COALESCE(last_sync_hash, ''),
		       auto_push, auto_pull,
		       pull_interval_minutes, push_on_change, sync_path
		FROM git_sync_config
		LIMIT 1
	`

	var config GitSyncConfig
	err := common.DB.QueryRow(ctx, query).Scan(
		&config.ID,
		&config.RepoURL,
		&config.Branch,
		&config.AuthToken,
		&config.SSHKey,
		&config.CommitAuthorName,
		&config.CommitAuthorEmail,
		&config.SyncEnabled,
		&config.SyncMode,
		&config.ForceOnConflict,
		&config.LastSyncHash,
		&config.AutoPush,
		&config.AutoPull,
		&config.PullIntervalMins,
		&config.PushOnChange,
		&config.SyncPath,
	)

	if err != nil {
		// If no config exists, return default
		// pgx returns ErrNoRows or similar error messages
		if strings.Contains(err.Error(), "no rows") || strings.Contains(err.Error(), "ErrNoRows") {
			return &GitSyncConfig{
				RepoURL:           "",
				Branch:            "main",
				CommitAuthorName:  "DD-UI Bot",
				CommitAuthorEmail: "ddui@localhost",
				SyncPath:          "/data",
				PullIntervalMins:  5,
				SyncEnabled:       false,
				SyncMode:          "off",
				ForceOnConflict:   false,
				LastSyncHash:      "",
				AutoPush:          false,
				AutoPull:          false,
			}, nil
		}
		return nil, err
	}

	// Decrypt token if SOPS is available
	if config.AuthToken != "" {
		decrypted, err := common.DecryptIfNeeded(config.AuthToken)
		if err == nil {
			config.AuthToken = decrypted
		}
	}

	// Decrypt SSH key if SOPS is available
	if config.SSHKey != "" {
		decrypted, err := common.DecryptIfNeeded(config.SSHKey)
		if err == nil {
			config.SSHKey = decrypted
		}
	}

	return &config, nil
}

// UpdateGitSyncConfig updates the Git sync configuration
func UpdateGitSyncConfig(ctx context.Context, config *GitSyncConfig) error {
	common.DebugLog("UpdateGitSyncConfig called with: repo=%s, branch=%s, author=%s <%s>, sync=%v, auto_push=%v, auto_pull=%v",
		config.RepoURL, config.Branch, config.CommitAuthorName, config.CommitAuthorEmail,
		config.SyncEnabled, config.AutoPush, config.AutoPull)
	
	// Encrypt sensitive fields if SOPS is available
	authToken := config.AuthToken
	sshKey := config.SSHKey

	if authToken != "" {
		encrypted, err := common.EncryptIfAvailable(authToken)
		if err == nil {
			authToken = encrypted
		}
	}

	if sshKey != "" {
		encrypted, err := common.EncryptIfAvailable(sshKey)
		if err == nil {
			sshKey = encrypted
		}
	}

	// First check if a config exists
	var existingID int
	err := common.DB.QueryRow(ctx, "SELECT id FROM git_sync_config LIMIT 1").Scan(&existingID)
	
	if err != nil && !strings.Contains(err.Error(), "no rows") {
		// Real error
		return err
	}
	
	var query string
	if existingID > 0 {
		// Update existing
		query = `
			UPDATE git_sync_config SET
				repo_url = $1,
				branch = $2,
				auth_token = $3,
				ssh_key = $4,
				commit_author_name = $5,
				commit_author_email = $6,
				sync_enabled = $7,
				sync_mode = $8,
				force_on_conflict = $9,
				auto_push = $10,
				auto_pull = $11,
				pull_interval_minutes = $12,
				push_on_change = $13,
				sync_path = $14,
				updated_at = NOW()
			WHERE id = $15`
	} else {
		// Insert new
		query = `
			INSERT INTO git_sync_config (
				repo_url, branch, auth_token, ssh_key,
				commit_author_name, commit_author_email,
				sync_enabled, sync_mode, force_on_conflict,
				auto_push, auto_pull,
				pull_interval_minutes, push_on_change, sync_path
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`
	}

	// Execute the query with appropriate parameters
	if existingID > 0 {
		// Update with ID parameter
		_, err = common.DB.Exec(ctx, query,
			config.RepoURL,
			config.Branch,
			authToken,
			sshKey,
			config.CommitAuthorName,
			config.CommitAuthorEmail,
			config.SyncEnabled,
			config.SyncMode,
			config.ForceOnConflict,
			config.AutoPush,
			config.AutoPull,
			config.PullIntervalMins,
			config.PushOnChange,
			config.SyncPath,
			existingID, // Add ID for WHERE clause
		)
	} else {
		// Insert without ID parameter
		_, err = common.DB.Exec(ctx, query,
			config.RepoURL,
			config.Branch,
			authToken,
			sshKey,
			config.CommitAuthorName,
			config.CommitAuthorEmail,
			config.SyncEnabled,
			config.SyncMode,
			config.ForceOnConflict,
			config.AutoPush,
			config.AutoPull,
			config.PullIntervalMins,
			config.PushOnChange,
			config.SyncPath,
		)
	}

	if err != nil {
		common.ErrorLog("Database error updating git config: %v", err)
	}

	return err
}

// LogGitSyncOperation logs a Git sync operation
func LogGitSyncOperation(ctx context.Context, entry GitSyncLogEntry) error {
	query := `
		INSERT INTO git_sync_log (
			operation, status, commit_before, commit_after,
			files_changed, additions, deletions, message,
			initiated_by, duration_ms, completed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW()
		)
	`

	_, err := common.DB.Exec(ctx, query,
		entry.Operation,
		entry.Status,
		entry.CommitBefore,
		entry.CommitAfter,
		entry.FilesChanged,
		entry.Additions,
		entry.Deletions,
		entry.Message,
		entry.InitiatedBy,
		entry.DurationMs,
	)

	return err
}

// GetGitSyncLogs retrieves recent Git sync logs
func GetGitSyncLogs(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT operation, status, commit_after, files_changed, 
		       message, initiated_by, duration_ms, started_at, completed_at
		FROM git_sync_log
		ORDER BY started_at DESC
		LIMIT $1
	`

	rows, err := common.DB.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var (
			operation    string
			status       string
			commitAfter  *string
			filesChanged []string
			message      *string
			initiatedBy  string
			durationMs   *int
			startedAt    time.Time
			completedAt  *time.Time
		)

		err := rows.Scan(&operation, &status, &commitAfter, &filesChanged,
			&message, &initiatedBy, &durationMs, &startedAt, &completedAt)
		if err != nil {
			continue
		}

		log := map[string]interface{}{
			"operation":     operation,
			"status":        status,
			"files_changed": len(filesChanged),
			"initiated_by":  initiatedBy,
			"started_at":    startedAt,
		}

		if commitAfter != nil {
			log["commit"] = (*commitAfter)[:8] // Short hash
		}
		if message != nil {
			log["message"] = *message
		}
		if durationMs != nil {
			log["duration_ms"] = *durationMs
		}
		if completedAt != nil {
			log["completed_at"] = *completedAt
		}

		logs = append(logs, log)
	}

	return logs, nil
}

// UpdateLastSyncHash updates the last sync commit hash for conflict detection
func UpdateLastSyncHash(ctx context.Context, hash string) error {
	query := `UPDATE git_sync_config SET last_sync_hash = $1, updated_at = NOW()`
	_, err := common.DB.Exec(ctx, query, hash)
	return err
}

// UpdateGitSyncTimestamp updates the last sync timestamp
func UpdateGitSyncTimestamp(ctx context.Context, operation string) error {
	var query string
	switch operation {
	case "pull":
		query = `UPDATE git_sync_config SET last_pull_at = NOW() WHERE id = 1`
	case "push":
		query = `UPDATE git_sync_config SET last_push_at = NOW() WHERE id = 1`
	default:
		return nil
	}

	_, err := common.DB.Exec(ctx, query)
	return err
}

// RecordGitConflict records a Git merge conflict
func RecordGitConflict(ctx context.Context, filePath, conflictType, details string) error {
	query := `
		INSERT INTO git_sync_conflicts (
			file_path, conflict_type, remote_content, resolved
		) VALUES (
			$1, $2, $3, FALSE
		)
	`

	_, err := common.DB.Exec(ctx, query, filePath, conflictType, details)
	return err
}

// GetUnresolvedConflicts retrieves unresolved Git conflicts
func GetUnresolvedConflicts(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT id, file_path, conflict_type, created_at
		FROM git_sync_conflicts
		WHERE resolved = FALSE
		ORDER BY created_at DESC
	`

	rows, err := common.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conflicts []map[string]interface{}
	for rows.Next() {
		var (
			id           int
			filePath     string
			conflictType string
			createdAt    time.Time
		)

		err := rows.Scan(&id, &filePath, &conflictType, &createdAt)
		if err != nil {
			continue
		}

		conflicts = append(conflicts, map[string]interface{}{
			"id":            id,
			"file_path":     filePath,
			"conflict_type": conflictType,
			"created_at":    createdAt,
		})
	}

	return conflicts, nil
}

// ResolveGitConflict marks a conflict as resolved
func ResolveGitConflict(ctx context.Context, conflictID int, resolutionType, resolvedBy string) error {
	query := `
		UPDATE git_sync_conflicts
		SET resolved = TRUE,
		    resolution_type = $2,
		    resolved_by = $3,
		    resolved_at = NOW()
		WHERE id = $1
	`

	_, err := common.DB.Exec(ctx, query, conflictID, resolutionType, resolvedBy)
	return err
}

// GetGitSyncStatus retrieves the current sync status
func GetGitSyncStatus(ctx context.Context) (map[string]interface{}, error) {
	query := `
		SELECT sync_enabled, last_pull_at, last_push_at, 
		       last_commit_hash, last_sync_status, last_sync_message
		FROM git_sync_config
		LIMIT 1
	`

	var (
		syncEnabled    bool
		lastPullAt     *time.Time
		lastPushAt     *time.Time
		lastCommitHash *string
		lastSyncStatus *string
		lastSyncMsg    *string
	)

	err := common.DB.QueryRow(ctx, query).Scan(
		&syncEnabled,
		&lastPullAt,
		&lastPushAt,
		&lastCommitHash,
		&lastSyncStatus,
		&lastSyncMsg,
	)

	if err != nil && err.Error() != "no rows in result set" {
		return nil, err
	}

	status := map[string]interface{}{
		"sync_enabled": syncEnabled,
	}

	if lastPullAt != nil {
		status["last_pull_at"] = *lastPullAt
	}
	if lastPushAt != nil {
		status["last_push_at"] = *lastPushAt
	}
	if lastCommitHash != nil {
		status["last_commit"] = (*lastCommitHash)[:8]
	}
	if lastSyncStatus != nil {
		status["last_status"] = *lastSyncStatus
	}
	if lastSyncMsg != nil {
		status["last_message"] = *lastSyncMsg
	}

	// Get conflict count
	var conflictCount int
	countQuery := `SELECT COUNT(*) FROM git_sync_conflicts WHERE resolved = FALSE`
	common.DB.QueryRow(ctx, countQuery).Scan(&conflictCount)
	status["unresolved_conflicts"] = conflictCount

	return status, nil
}