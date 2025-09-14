// src/api/db_iac.go
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
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

func pruneEmptyIacStacks(ctx context.Context, repoID int64) (int64, error) {
	if repoID == 0 {
		return 0, errors.New("repoID=0")
	}
	
	// Delete stacks that have no files (abandoned stack creation)
	cmd, err := db.Exec(ctx, `
		DELETE FROM iac_stacks 
		WHERE repo_id=$1 
		AND id NOT IN (
			SELECT DISTINCT stack_id 
			FROM iac_stack_files 
			WHERE stack_id IS NOT NULL
		)
		AND (compose_file IS NULL OR compose_file = '')
	`, repoID)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
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

// Helper function for hierarchical API name-to-ID resolution
func getStackID(ctx context.Context, scopeKind, scopeName, stackName string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, 
		`SELECT id FROM iac_stacks WHERE scope_kind=$1 AND scope_name=$2 AND stack_name=$3`,
		scopeKind, scopeName, stackName).Scan(&id)
	return id, err
}

func listIacStacksForHost(ctx context.Context, hostName string) ([]IacStackOut, error) {
	debugLog("listIacStacksForHost called for host: %s", hostName)
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		debugLog("GetHostByName failed for %s: %v", hostName, err)
		return nil, err
	}

	groups := h.Groups
	if groups == nil {
		groups = []string{}
	}
	debugLog("Host %s belongs to groups: %v", hostName, groups)

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
	
	debugLog("Found %d basic stacks for host %s", len(stacks), hostName)

	// Filter out empty stacks to prevent 500 errors during processing
	var filteredStacks []IacStackOut
	for _, s := range stacks {
		hasContent, err := stackHasContent(ctx, s.ID)
		if err != nil {
			debugLog("Failed to check stack content for %s: %v", s.Name, err)
			continue
		}
		if !hasContent {
			debugLog("Skipping empty stack %s (no files)", s.Name)
			continue
		}
		filteredStacks = append(filteredStacks, s)
	}
	stacks = filteredStacks
	debugLog("After filtering empty stacks: %d stacks remain for host %s", len(stacks), hostName)

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

/* ===== Hashes ===== */

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
		// Filter out comment metadata files from user-facing file listings
		if strings.HasSuffix(it.RelPath, ".comments.json") {
			continue
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
	if err := db.QueryRow(ctx, `SELECT COUNT(1) FROM iac_stack_files WHERE stack_id=$1`, stackID).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	var compose string
	_ = db.QueryRow(ctx, `SELECT COALESCE(compose_file,'') FROM iac_stacks WHERE id=$1`, stackID).Scan(&compose)
	return strings.TrimSpace(compose) != "", nil
}

// ComputeCurrentBundleHash returns a stable roll-up hash of all tracked IaC files (post-decrypt).
func ComputeCurrentBundleHash(ctx context.Context, stackID int64) (string, error) {
	files, err := listFilesForStack(ctx, stackID)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(files))
	for _, f := range files {
		lines = append(lines, fmt.Sprintf("%s|%s|%s|%d",
			strings.ToLower(f.Role), f.RelPath, strings.ToLower(f.Sha256Hex), f.SizeBytes))
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, ln := range lines {
		_, _ = h.Write([]byte(ln))
		_, _ = h.Write([]byte{'\n'})
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum), nil
}

/* ---------- Enhanced IaC (runtime + drift) ---------- */

type ContainerBrief struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Service    string `json:"service"`
	Image      string `json:"image"`
	State      string `json:"state"`
	ConfigHash string `json:"config_hash,omitempty"` // com.docker.compose.config-hash
}

type EnhancedIacStackOut struct {
	IacStackOut
	LatestDeployment  *DeploymentStamp  `json:"latest_deployment,omitempty"`
	DriftDetected     bool              `json:"drift_detected"`
	DriftReason       string            `json:"drift_reason,omitempty"`
	Containers        []ContainerBrief  `json:"containers"`
	RenderedServices  []RenderedService `json:"rendered_services,omitempty"`
	RenderedConfigSha string            `json:"rendered_config_hash,omitempty"`
	EffectiveAutoDevops bool            `json:"effective_auto_devops"`
}

func listEnhancedIacStacksForHost(ctx context.Context, hostName string) ([]EnhancedIacStackOut, error) {
	debugLog("listEnhancedIacStacksForHost called for host: %s", hostName)
	base, err := listIacStacksForHost(ctx, hostName)
	if err != nil {
		debugLog("listIacStacksForHost failed for %s: %v", hostName, err)
		return nil, err
	}
	debugLog("Found %d base stacks for host %s", len(base), hostName)

	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, err
	}
	cli, err := dockerClientForHost(h)
	if err != nil {
		debugLog("Failed to create docker client for host %s: %v", hostName, err)
		return nil, err
	}
	defer cli.Close()
	debugLog("Docker client created successfully for host %s", hostName)

	out := make([]EnhancedIacStackOut, 0, len(base))
	for _, s := range base {
		// Skip empty stacks to prevent 500 errors during processing
		hasContent, err := stackHasContent(ctx, s.ID)
		if err != nil {
			debugLog("Failed to check stack content for %s: %v", s.Name, err)
			continue
		}
		if !hasContent {
			debugLog("Skipping empty stack %s (no files)", s.Name)
			continue
		}

		e := EnhancedIacStackOut{IacStackOut: s}

		// Lookup runtime by Compose project label = sanitized(stack_name)
		projectLabel := composeProjectLabelFromStack(s.Name)

		ff := filters.NewArgs()
		ff.Add("label", "com.docker.compose.project="+projectLabel)
		debugLog("Stack %s looking for containers with project label: %s", s.Name, projectLabel)
		ctrs, lerr := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: ff})
		if lerr == nil {
			debugLog("Stack %s found %d containers", s.Name, len(ctrs))
			for _, c := range ctrs {
				lbl := func(k string) string {
					if c.Labels == nil {
						return ""
					}
					return c.Labels[k]
				}
				name := ""
				if len(c.Names) > 0 {
					name = strings.TrimPrefix(c.Names[0], "/")
				}
				e.Containers = append(e.Containers, ContainerBrief{
					ID:         c.ID,
					Name:       name,
					Service:    lbl("com.docker.compose.service"),
					Image:      c.Image,
					State:      c.State,
					ConfigHash: lbl("com.docker.compose.config-hash"),
				})
			}
		}

		// Use hash-based drift detection
		hasDrift, driftReason, err := detectDriftViaHashes(ctx, s.ID, s.Name, cli)
		if err != nil {
			debugLog("Hash-based drift detection failed for stack %s: %v", s.Name, err)
			// Fallback to no drift detected on error
			e.DriftDetected = false
			e.DriftReason = "Unable to determine drift status"
		} else {
			e.DriftDetected = hasDrift
			e.DriftReason = driftReason
			debugLog("Stack %s: drift=%v, reason=%s", s.Name, hasDrift, driftReason)
		}

		// Stage for render (SOPS-aware) - still needed for UI display of rendered services
		stageDir, stagedComposes, cleanup, derr := stageStackForCompose(ctx, s.ID)
		if derr == nil {
			if cleanup != nil {
				defer cleanup()
			}
			// Rendered config hash (best effort)
			e.RenderedConfigSha = computeRenderedConfigHash(ctx, stageDir, s.Name, stagedComposes)

			// Fully rendered services (post-decrypt, post-interpolation)
			if rs, rerr := renderComposeServices(ctx, stageDir, s.Name, stagedComposes); rerr == nil {
				e.RenderedServices = rs
				debugLog("Stack %s rendered %d services", s.Name, len(rs))
				for _, r := range rs {
					debugLog("Service %s -> image %s, container %s", r.ServiceName, r.Image, r.ContainerName)
				}
				debugLog("Stack %s RenderedServices field populated with %d items", s.Name, len(e.RenderedServices))
			} else {
				debugLog("Stack %s render services failed: %v", s.Name, rerr)
			}
		} else {
			debugLog("Stack %s staging failed: %v", s.Name, derr)
		}

		// Calculate effective auto devops for this stack
		e.EffectiveAutoDevops, _ = shouldAutoApply(ctx, s.ID)

		out = append(out, e)
	}
	return out, nil
}

/* ---------- Stack-specific GitOps Configuration ---------- */

// getStackDevopsOverride gets the GitOps auto-deployment setting for a specific stack
func getStackDevopsOverride(ctx context.Context, scopeKind, scopeName, stackName string) (*bool, error) {
	var autoDeploy *bool
	var result sql.NullString
	err := db.QueryRow(ctx, `
		SELECT auto_devops_override 
		FROM iac_stacks 
		WHERE scope_kind=$1 AND scope_name=$2 AND stack_name=$3
	`, scopeKind, scopeName, stackName).Scan(&result)
	
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("stack not found: %s/%s/%s", scopeKind, scopeName, stackName)
	}
	if err != nil {
		return nil, err
	}
	
	if result.Valid {
		if result.String == "true" {
			val := true
			autoDeploy = &val
		} else if result.String == "false" {
			val := false
			autoDeploy = &val
		}
		// null/empty string means no override
	}
	
	return autoDeploy, nil
}

// setStackDevopsOverride sets the GitOps auto-deployment setting for a specific stack
func setStackDevopsOverride(ctx context.Context, scopeKind, scopeName, stackName string, autoDeploy *bool) error {
	var updateField interface{}
	if autoDeploy == nil {
		updateField = nil
	} else if *autoDeploy {
		updateField = "true"
	} else {
		updateField = "false"
	}
	
	result, err := db.Exec(ctx, `
		UPDATE iac_stacks 
		SET auto_devops_override = $4
		WHERE scope_kind=$1 AND scope_name=$2 AND stack_name=$3
	`, scopeKind, scopeName, stackName, updateField)
	
	if err != nil {
		return err
	}
	
	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("stack not found: %s/%s/%s", scopeKind, scopeName, stackName)
	}
	
	return nil
}

// getStacksBatch retrieves multiple stacks by their IDs for the batch endpoint
func getStacksBatch(ctx context.Context, stackIds []int64) ([]map[string]any, error) {
	if len(stackIds) == 0 {
		return []map[string]any{}, nil
	}
	
	var result []map[string]any
	
	for _, id := range stackIds {
		var stack struct {
			ID         int64   `json:"id"`
			RepoID     int64   `json:"repo_id"`
			ScopeKind  string  `json:"scope_kind"`
			ScopeName  string  `json:"scope_name"`
			StackName  string  `json:"stack_name"`
			RelPath    string  `json:"rel_path"`
			IacEnabled bool    `json:"iac_enabled"`
			DeployKind string  `json:"deploy_kind"`
			SopsStatus string  `json:"sops_status"`
			Override   *bool   `json:"auto_apply_override"`
			UpdatedAt  string  `json:"updated_at"`
		}
		
		var updatedAt time.Time
		err := db.QueryRow(ctx, `
			SELECT id, repo_id, scope_kind::text, scope_name, stack_name, rel_path,
				iac_enabled, deploy_kind::text, sops_status::text, auto_apply_override, updated_at
			FROM iac_stacks
			WHERE id=$1
		`, id).Scan(
			&stack.ID, &stack.RepoID, &stack.ScopeKind, &stack.ScopeName, &stack.StackName, &stack.RelPath,
			&stack.IacEnabled, &stack.DeployKind, &stack.SopsStatus, &stack.Override, &updatedAt,
		)
		
		if err != nil {
			if err == sql.ErrNoRows {
				// Skip missing stacks
				continue
			}
			return nil, fmt.Errorf("failed to get stack %d: %v", id, err)
		}
		
		stack.UpdatedAt = updatedAt.Format(time.RFC3339)
		effectiveAutoDevops, _ := shouldAutoApply(ctx, id)
		
		stackMap := map[string]any{
			"id":                    stack.ID,
			"repo_id":               stack.RepoID,
			"scope_kind":            stack.ScopeKind,
			"scope_name":            stack.ScopeName,
			"stack_name":            stack.StackName,
			"rel_path":              stack.RelPath,
			"iac_enabled":           stack.IacEnabled,
			"deploy_kind":           stack.DeployKind,
			"sops_status":           stack.SopsStatus,
			"auto_apply_override":   stack.Override,
			"updated_at":            stack.UpdatedAt,
			"effective_auto_devops": effectiveAutoDevops,
		}
		
		result = append(result, stackMap)
	}
	
	return result, nil
}
