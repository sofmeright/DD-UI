package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type DeploymentStamp struct {
	ID                  int64     `json:"id"`
	StackID             int64     `json:"stack_id"`
	DeploymentHash      string    `json:"deployment_hash"`
	DeploymentTimestamp time.Time `json:"deployment_timestamp"`
	DeploymentMethod    string    `json:"deployment_method"`
	DeploymentUser      string    `json:"deployment_user,omitempty"`
	DeploymentEnvHash   string    `json:"deployment_env_hash,omitempty"`
	DeploymentStatus    string    `json:"deployment_status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// CreateDeploymentStamp keeps the original API (hash from config bytes).
func CreateDeploymentStamp(ctx context.Context, stackID int64, method, user string, config []byte, envVars map[string]string) (*DeploymentStamp, error) {
	deploymentHash := generateDeploymentHash(config)

	envHash := ""
	if len(envVars) > 0 {
		envHash = generateEnvHash(envVars)
	}

	var stamp DeploymentStamp
	err := db.QueryRow(ctx, `
		INSERT INTO deployment_stamps 
			(host_id, stack_id, deployment_hash, deployment_method, deployment_user, deployment_env_hash, deployment_status)
		SELECT h.id, s.id, $2, $3, $4, $5, 'pending'
		FROM iac_stacks s
		LEFT JOIN hosts h ON (s.scope_kind='host' AND s.scope_name=h.name)
		WHERE s.id = $1
		RETURNING id, stack_id, deployment_hash, deployment_timestamp, deployment_method, 
		          COALESCE(deployment_user, ''), COALESCE(deployment_env_hash, ''), deployment_status, 
		          created_at, updated_at
	`, stackID, deploymentHash, method, user, envHash).Scan(
		&stamp.ID, &stamp.StackID, &stamp.DeploymentHash, &stamp.DeploymentTimestamp,
		&stamp.DeploymentMethod, &stamp.DeploymentUser, &stamp.DeploymentEnvHash,
		&stamp.DeploymentStatus, &stamp.CreatedAt, &stamp.UpdatedAt,
	)
	return &stamp, err
}

// CreateDeploymentStampWithHash is used by deploy.go when we already computed the bundle hash.
func CreateDeploymentStampWithHash(ctx context.Context, stackID int64, method, user, deploymentHash string, envVars map[string]string) (*DeploymentStamp, error) {
	envHash := ""
	if len(envVars) > 0 {
		envHash = generateEnvHash(envVars)
	}

	var stamp DeploymentStamp
	err := db.QueryRow(ctx, `
		INSERT INTO deployment_stamps 
			(host_id, stack_id, deployment_hash, deployment_method, deployment_user, deployment_env_hash, deployment_status)
		SELECT h.id, s.id, $2, $3, $4, $5, 'pending'
		FROM iac_stacks s
		LEFT JOIN hosts h ON (s.scope_kind='host' AND s.scope_name=h.name)
		WHERE s.id = $1
		RETURNING id, stack_id, deployment_hash, deployment_timestamp, deployment_method, 
		          COALESCE(deployment_user, ''), COALESCE(deployment_env_hash, ''), deployment_status, 
		          created_at, updated_at
	`, stackID, deploymentHash, method, user, envHash).Scan(
		&stamp.ID, &stamp.StackID, &stamp.DeploymentHash, &stamp.DeploymentTimestamp,
		&stamp.DeploymentMethod, &stamp.DeploymentUser, &stamp.DeploymentEnvHash,
		&stamp.DeploymentStatus, &stamp.CreatedAt, &stamp.UpdatedAt,
	)
	return &stamp, err
}

// CheckDeploymentStampExists checks if a deployment stamp already exists for the given config
func CheckDeploymentStampExists(ctx context.Context, stackID int64, config []byte) (*DeploymentStamp, error) {
	deploymentHash := generateDeploymentHash(config)
	
	var stamp DeploymentStamp
	err := db.QueryRow(ctx, `
		SELECT id, stack_id, deployment_hash, deployment_timestamp, deployment_method, 
		       COALESCE(deployment_user, ''), COALESCE(deployment_env_hash, ''), deployment_status, 
		       created_at, updated_at
		FROM deployment_stamps
		WHERE stack_id = $1 AND deployment_hash = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, stackID, deploymentHash).Scan(
		&stamp.ID, &stamp.StackID, &stamp.DeploymentHash, &stamp.DeploymentTimestamp,
		&stamp.DeploymentMethod, &stamp.DeploymentUser, &stamp.DeploymentEnvHash,
		&stamp.DeploymentStatus, &stamp.CreatedAt, &stamp.UpdatedAt,
	)
	
	if err != nil {
		return nil, err
	}
	return &stamp, nil
}

// UpdateDeploymentStampStatus updates the status of a deployment stamp.
func UpdateDeploymentStampStatus(ctx context.Context, stampID int64, status string) error {
	_, err := db.Exec(ctx, `
		UPDATE deployment_stamps 
		SET deployment_status = $1, updated_at = now()
		WHERE id = $2
	`, status, stampID)
	return err
}

// GetLatestDeploymentStamp gets the most recent successful deployment stamp for a stack.
func GetLatestDeploymentStamp(ctx context.Context, stackID int64) (*DeploymentStamp, error) {
	var stamp DeploymentStamp
	err := db.QueryRow(ctx, `
		SELECT id, stack_id, deployment_hash, deployment_timestamp, deployment_method,
		       COALESCE(deployment_user, ''), COALESCE(deployment_env_hash, ''), deployment_status,
		       created_at, updated_at
		FROM deployment_stamps
		WHERE stack_id = $1 AND deployment_status = 'success'
		ORDER BY deployment_timestamp DESC
		LIMIT 1
	`, stackID).Scan(
		&stamp.ID, &stamp.StackID, &stamp.DeploymentHash, &stamp.DeploymentTimestamp,
		&stamp.DeploymentMethod, &stamp.DeploymentUser, &stamp.DeploymentEnvHash,
		&stamp.DeploymentStatus, &stamp.CreatedAt, &stamp.UpdatedAt,
	)
	if err != nil {
		// Gracefully handle missing table (migration not applied)
		if strings.Contains(err.Error(), `relation "deployment_stamps" does not exist`) {
			return nil, fmt.Errorf("deployment stamps feature not available - migration 015 not applied")
		}
		return nil, err
	}
	return &stamp, nil
}

// AssociateContainerWithStamp links a single container to a deployment stamp (kept for existing callers).
func AssociateContainerWithStamp(ctx context.Context, containerID string, stampID int64, deploymentHash string) error {
	_, err := db.Exec(ctx, `
		UPDATE containers 
		SET deployment_stamp_id = $1, deployment_hash = $2, updated_at = now()
		WHERE container_id = $3
	`, stampID, deploymentHash, containerID)
	return err
}

// AssociateContainersWithStampIDs links many containers at once (the new bulk path used after compose).
func AssociateContainersWithStampIDs(ctx context.Context, containerIDs []string, stampID int64, deploymentHash string) (int64, error) {
	if len(containerIDs) == 0 {
		return 0, nil
	}
	cmd, err := db.Exec(ctx, `
		UPDATE containers
		SET deployment_stamp_id = $1, deployment_hash = $2, updated_at = now()
		WHERE container_id = ANY($3::text[])
	`, stampID, deploymentHash, containerIDs)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}

// GetContainerDeploymentInfo retrieves deployment information for a container.
func GetContainerDeploymentInfo(ctx context.Context, containerID string) (*DeploymentStamp, error) {
	var stamp DeploymentStamp
	err := db.QueryRow(ctx, `
		SELECT ds.id, ds.stack_id, ds.deployment_hash, ds.deployment_timestamp, ds.deployment_method,
		       COALESCE(ds.deployment_user, ''), COALESCE(ds.deployment_env_hash, ''), ds.deployment_status,
		       ds.created_at, ds.updated_at
		FROM containers c
		JOIN deployment_stamps ds ON ds.id = c.deployment_stamp_id
		WHERE c.container_id = $1
	`, containerID).Scan(
		&stamp.ID, &stamp.StackID, &stamp.DeploymentHash, &stamp.DeploymentTimestamp,
		&stamp.DeploymentMethod, &stamp.DeploymentUser, &stamp.DeploymentEnvHash,
		&stamp.DeploymentStatus, &stamp.CreatedAt, &stamp.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &stamp, nil
}

/* --------- small helpers --------- */

func generateDeploymentHash(config []byte) string {
	sum := sha256.Sum256(config)
	return hex.EncodeToString(sum[:])
}

func generateEnvHash(envVars map[string]string) string {
	// stable JSON -> hash
	jsonBytes, _ := json.Marshal(envVars)
	sum := sha256.Sum256(jsonBytes)
	return hex.EncodeToString(sum[:])
}

// GetDeploymentStampsForStack returns stamps for a stack (useful for UI history panes).
func GetDeploymentStampsForStack(ctx context.Context, stackID int64, limit int) ([]DeploymentStamp, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(ctx, `
		SELECT id, stack_id, deployment_hash, deployment_timestamp, deployment_method,
		       COALESCE(deployment_user, ''), COALESCE(deployment_env_hash, ''), deployment_status,
		       created_at, updated_at
		FROM deployment_stamps
		WHERE stack_id = $1
		ORDER BY deployment_timestamp DESC
		LIMIT $2
	`, stackID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stamps []DeploymentStamp
	for rows.Next() {
		var s DeploymentStamp
		if err := rows.Scan(
			&s.ID, &s.StackID, &s.DeploymentHash, &s.DeploymentTimestamp,
			&s.DeploymentMethod, &s.DeploymentUser, &s.DeploymentEnvHash,
			&s.DeploymentStatus, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		stamps = append(stamps, s)
	}
	return stamps, rows.Err()
}
