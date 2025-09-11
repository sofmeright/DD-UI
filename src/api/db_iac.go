// src/api/db_iac.go
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type IacRepoRow struct {
	ID         int64  `json:"id"`
	Kind       string `json:"kind"`
	RootPath   string `json:"root_path"`
	URL        string `json:"url"`
	Branch     string `json:"branch"`
	LastCommit string `json:"last_commit"`
	Enabled    bool   `json:"enabled"`
}

type IacStackRow struct {
	ID         int64  `json:"id"`
	RepoID     int64  `json:"repo_id"`
	ScopeKind  string `json:"scope_kind"`
	ScopeName  string `json:"scope_name"`
	StackName  string `json:"stack_name"`
	RelPath    string `json:"rel_path"`
	Compose    string `json:"compose_file,omitempty"`
	DeployKind string `json:"deploy_kind"`
	PullPolicy string `json:"pull_policy,omitempty"`
	SopsStatus string `json:"sops_status"` // all|partial|none
	IacEnabled bool   `json:"iac_enabled"`
}

type IacEnvFile struct {
	Path string `json:"path"`
	Sops bool   `json:"sops"`
}

type IacServiceRow struct {
	ID            int64             `json:"id"`
	StackID       int64             `json:"stack_id"`
	ServiceName   string            `json:"service_name"`
	ContainerName string            `json:"container_name,omitempty"`
	Image         string            `json:"image,omitempty"`
	Labels        map[string]string `json:"labels"`
	EnvKeys       []string          `json:"env_keys"`
	EnvFiles      []IacEnvFile      `json:"env_files"`
	Ports         []map[string]any  `json:"ports"`
	Volumes       []map[string]any  `json:"volumes"`
	Deploy        map[string]any    `json:"deploy"`
}

func upsertIacRepoLocal(ctx context.Context, root string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO iac_repos (kind, root_path, enabled)
		VALUES ('local', $1, TRUE)
		ON CONFLICT (kind, root_path)
		DO UPDATE SET enabled=TRUE, updated_at=now()
		RETURNING id
	`, root).Scan(&id)
	return id, err
}

func upsertIacStack(ctx context.Context, repoID int64, scopeKind, scopeName, stackName, relPath, composeFile, deployKind, pullPolicy, sopsStatus string, enabled bool) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO iac_stacks (repo_id, scope_kind, scope_name, stack_name, rel_path, compose_file, deploy_kind, pull_policy, sops_status, iac_enabled, last_scan_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, now())
		ON CONFLICT (repo_id, scope_kind, scope_name, stack_name)
		DO UPDATE SET compose_file=EXCLUDED.compose_file, deploy_kind=EXCLUDED.deploy_kind, pull_policy=EXCLUDED.pull_policy,
		             sops_status=EXCLUDED.sops_status, iac_enabled=EXCLUDED.iac_enabled, last_scan_at=now()
		RETURNING id
	`, repoID, scopeKind, scopeName, stackName, relPath, composeFile, deployKind, pullPolicy, sopsStatus, enabled).Scan(&id)
	return id, err
}

func upsertIacService(ctx context.Context, s IacServiceRow) error {
	lb, _ := json.Marshal(s.Labels)
	ek, _ := json.Marshal(s.EnvKeys)
	ef, _ := json.Marshal(s.EnvFiles)
	pp, _ := json.Marshal(s.Ports)
	vv, _ := json.Marshal(s.Volumes)
	dp, _ := json.Marshal(s.Deploy)
	_, err := db.Exec(ctx, `
		INSERT INTO iac_services (stack_id, service_name, container_name, image, labels, env_keys, env_files, ports, volumes, deploy)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7::jsonb,$8::jsonb,$9::jsonb,$10::jsonb)
		ON CONFLICT (stack_id, service_name)
		DO UPDATE SET container_name=EXCLUDED.container_name, image=EXCLUDED.image, labels=EXCLUDED.labels,
		              env_keys=EXCLUDED.env_keys, env_files=EXCLUDED.env_files, ports=EXCLUDED.ports,
		              volumes=EXCLUDED.volumes, deploy=EXCLUDED.deploy, updated_at=now()
	`, s.StackID, s.ServiceName, s.ContainerName, s.Image, string(lb), string(ek), string(ef), string(pp), string(vv), string(dp))
	return err
}

func upsertIacFile(ctx context.Context, stackID int64, role, relPath string, sops bool, sha256Hex string, sizeBytes int64) error {
	_, err := db.Exec(ctx, `
		INSERT INTO iac_stack_files (stack_id, role, rel_path, sops, sha256_hex, size_bytes)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (stack_id, rel_path)
		DO UPDATE SET role=EXCLUDED.role, sops=EXCLUDED.sops, sha256_hex=EXCLUDED.sha256_hex, size_bytes=EXCLUDED.size_bytes, updated_at=now()
	`, stackID, role, relPath, sops, sha256Hex, sizeBytes)
	return err
}

func pruneIacStacksNotIn(ctx context.Context, repoID int64, keepIDs []int64) (int64, error) {
	if repoID == 0 {
		return 0, errors.New("repoID=0")
	}
	if len(keepIDs) == 0 {
		cmd, err := db.Exec(ctx, `DELETE FROM iac_stacks WHERE repo_id=$1`, repoID)
		if err != nil {
			return 0, err
		}
		return cmd.RowsAffected(), nil
	}
	cmd, err := db.Exec(ctx, `DELETE FROM iac_stacks WHERE repo_id=$1 AND id <> ALL($2)`, repoID, keepIDs)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}

/* ---------- Read for API ---------- */

type IacStackOut struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"` // stack_name
	ScopeKind  string          `json:"scope_kind"`
	ScopeName  string          `json:"scope_name"`
	DeployKind string          `json:"deploy_kind"`
	PullPolicy string          `json:"pull_policy"`
	SopsStatus string          `json:"sops_status"`
	IacEnabled bool            `json:"iac_enabled"`
	RelPath    string          `json:"rel_path"`
	Compose    string          `json:"compose_file,omitempty"`
	Services   []IacServiceRow `json:"services"`
}

func listIacStacksForHost(ctx context.Context, hostName string) ([]IacStackOut, error) {
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, err
	}

	// gather group names from host row (assuming stored in DB; fallback empty)
	groups := h.Groups
	if groups == nil {
		groups = []string{}
	}

	rows, err := db.Query(ctx, `
	  SELECT id, repo_id, scope_kind, scope_name, stack_name, rel_path, compose_file, deploy_kind, pull_policy, sops_status, iac_enabled
	  FROM iac_stacks
	  WHERE (scope_kind='host' AND scope_name=$1)
	     OR (scope_kind='group' AND scope_name = ANY($2))
	  ORDER BY scope_kind, scope_name, stack_name
	`, hostName, groups)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stacks []IacStackOut
	for rows.Next() {
		var s IacStackOut
		var repoID int64
		if err := rows.Scan(&s.ID, &repoID, &s.ScopeKind, &s.ScopeName, &s.Name, &s.RelPath, &s.Compose, &s.DeployKind, &s.PullPolicy, &s.SopsStatus, &s.IacEnabled); err != nil {
			return nil, err
		}
		stacks = append(stacks, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// load services per stack
	for i := range stacks {
		rs, err := db.Query(ctx, `
		 SELECT id, stack_id, service_name, container_name, image, labels, env_keys, env_files, ports, volumes, deploy
		 FROM iac_services WHERE stack_id=$1 ORDER BY service_name
		`, stacks[i].ID)
		if err != nil {
			return nil, err
		}
		var svcs []IacServiceRow
		for rs.Next() {
			var s IacServiceRow
			var lb, ek, ef, pp, vv, dp []byte
			if err := rs.Scan(&s.ID, &s.StackID, &s.ServiceName, &s.ContainerName, &s.Image, &lb, &ek, &ef, &pp, &vv, &dp); err != nil {
				rs.Close()
				return nil, err
			}
			_ = json.Unmarshal(lb, &s.Labels)
			_ = json.Unmarshal(ek, &s.EnvKeys)
			_ = json.Unmarshal(ef, &s.EnvFiles)
			_ = json.Unmarshal(pp, &s.Ports)
			_ = json.Unmarshal(vv, &s.Volumes)
			_ = json.Unmarshal(dp, &s.Deploy)
			svcs = append(svcs, s)
		}
		rs.Close()
		stacks[i].Services = svcs
	}

	return stacks, nil
}

/* ===== New helpers for editor APIs and bundle hashing ===== */

type IacFileMetaRow struct {
	Role      string    `json:"role"`
	RelPath   string    `json:"rel_path"`
	Sops      bool      `json:"sops"`
	Sha256Hex string    `json:"sha256_hex"`
	SizeBytes int64     `json:"size_bytes"`
	UpdatedAt time.Time `json:"updated_at"`
}

func getRepoRootForStack(ctx context.Context, stackID int64) (string, error) {
	var root string
	err := db.QueryRow(ctx, `
		SELECT r.root_path
		FROM iac_stacks s
		JOIN iac_repos r ON r.id = s.repo_id
		WHERE s.id=$1
	`, stackID).Scan(&root)
	return root, err
}

func listFilesForStack(ctx context.Context, stackID int64) ([]IacFileMetaRow, error) {
	rows, err := db.Query(ctx, `
	  SELECT role, rel_path, sops, sha256_hex, size_bytes, updated_at
	  FROM iac_stack_files
	  WHERE stack_id=$1
	  ORDER BY role, rel_path
	`, stackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []IacFileMetaRow
	for rows.Next() {
		var it IacFileMetaRow
		if err := rows.Scan(&it.Role, &it.RelPath, &it.Sops, &it.Sha256Hex, &it.SizeBytes, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func deleteIacFileRow(ctx context.Context, stackID int64, relPath string) error {
	_, err := db.Exec(ctx, `DELETE FROM iac_stack_files WHERE stack_id=$1 AND rel_path=$2`, stackID, relPath)
	return err
}

// stackHasContent returns true if the stack has any tracked files or a compose_file set.
func stackHasContent(ctx context.Context, stackID int64) (bool, error) {
	var n int64
	if err := db.QueryRow(ctx, `SELECT COUNT(1) FROM iac_stack_files WHERE stack_id=$1`).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	var compose string
	_ = db.QueryRow(ctx, `SELECT COALESCE(compose_file,'') FROM iac_stacks WHERE id=$1`, stackID).Scan(&compose)
	return strings.TrimSpace(compose) != "", nil
}

// ComputeCurrentBundleHash calculates a deterministic hash over all tracked files
// (compose/env/scripts/etc.) for the given stack. Any change to a tracked file
// will change this hash and thus signal "needs redeploy".
func ComputeCurrentBundleHash(ctx context.Context, stackID int64) (string, error) {
	type row struct {
		role string
		path string
		sha  string
		size int64
	}
	rows, err := db.Query(ctx, `
		SELECT role, rel_path, sha256_hex, size_bytes
		FROM iac_stack_files
		WHERE stack_id=$1
	`, stackID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var files []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.role, &r.path, &r.sha, &r.size); err != nil {
			return "", err
		}
		files = append(files, r)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	// Deterministic ordering
	sort.Slice(files, func(i, j int) bool {
		if files[i].role != files[j].role {
			return files[i].role < files[j].role
		}
		return files[i].path < files[j].path
	})

	h := sha256.New()
	for _, f := range files {
		// stable serialization: role \t path \t sha \t size \n
		fmt.Fprintf(h, "%s\t%s\t%s\t%d\n", f.role, f.path, f.sha, f.size)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

/* ===== Enhanced stacks + stamp-based drift (used by /hosts/{name}/enhanced-iac) ===== */

type EnhancedIacStackOut struct {
	IacStackOut
	LatestDeployment *DeploymentStamp `json:"latest_deployment,omitempty"`
	DriftDetected    bool             `json:"drift_detected"`
	DriftReason      string           `json:"drift_reason,omitempty"`
}

func listEnhancedIacStacksForHost(ctx context.Context, hostName string) ([]EnhancedIacStackOut, error) {
	// Get base stacks
	baseStacks, err := listIacStacksForHost(ctx, hostName)
	if err != nil {
		return nil, err
	}

	var enhancedStacks []EnhancedIacStackOut
	for _, stack := range baseStacks {
		enhanced := EnhancedIacStackOut{
			IacStackOut: stack,
		}

		// Get latest deployment stamp
		latestStamp, err := GetLatestDeploymentStamp(ctx, stack.ID)
		if err == nil {
			enhanced.LatestDeployment = latestStamp

			// Check for drift by comparing running container stamps with expected
			driftDetected, reason := checkStackDrift(ctx, stack.ID, latestStamp.DeploymentHash)
			enhanced.DriftDetected = driftDetected
			enhanced.DriftReason = reason
		} else {
			// If stamps table is missing, mark as unavailable rather than hard-fail
			if strings.Contains(err.Error(), "migration 015 not applied") ||
				strings.Contains(strings.ToLower(err.Error()), "not available") {
				enhanced.DriftDetected = false
				enhanced.DriftReason = "Enhanced drift detection not available - migration 015 not applied"
			} else {
				// No successful deployment yet → do not scream drift; surface as info
				enhanced.DriftDetected = false
				enhanced.DriftReason = "No successful deployment recorded yet"
			}
		}

		enhancedStacks = append(enhancedStacks, enhanced)
	}

	return enhancedStacks, nil
}

// checkStackDrift compares expected deployment hash with running containers.
// It avoids immediate false-positives if stamps aren’t associated yet.
func checkStackDrift(ctx context.Context, stackID int64, expectedHash string) (bool, string) {
	rows, err := db.Query(ctx, `
		SELECT container_id, COALESCE(deployment_hash, ''), state
		FROM containers 
		WHERE stack_id = $1 
		  AND state IN ('running', 'paused', 'restarting')
	`, stackID)
	if err != nil {
		// If column is missing (migration not applied), don’t flag drift.
		if strings.Contains(err.Error(), "column \"deployment_hash\" does not exist") {
			return false, "Enhanced drift detection not available - migration 015 not applied"
		}
		return true, fmt.Sprintf("Failed to query containers: %v", err)
	}
	defer rows.Close()

	var runningContainers int
	var wrongHash int
	var missingHash int

	for rows.Next() {
		var containerID, deploymentHash, state string
		if err := rows.Scan(&containerID, &deploymentHash, &state); err != nil {
			continue
		}
		runningContainers++
		if deploymentHash == "" {
			missingHash++
		} else if deploymentHash != expectedHash {
			wrongHash++
		}
	}

	if runningContainers == 0 {
		// No containers — can be “not deployed yet”; don’t flag as drift.
		return false, "No running containers found for stack"
	}
	if missingHash > 0 {
		// Likely just deployed; association happens async. Don’t hard-fail.
		return false, fmt.Sprintf("%d containers pending deployment stamp association", missingHash)
	}
	if wrongHash > 0 {
		return true, fmt.Sprintf("%d containers have different deployment hash", wrongHash)
	}
	return false, "All containers match expected deployment"
}
