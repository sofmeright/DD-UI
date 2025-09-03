package main

import (
	"context"
	"encoding/json"
	"errors"
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
	ID            int64                    `json:"id"`
	StackID       int64                    `json:"stack_id"`
	ServiceName   string                   `json:"service_name"`
	ContainerName string                   `json:"container_name,omitempty"`
	Image         string                   `json:"image,omitempty"`
	Labels        map[string]string        `json:"labels"`
	EnvKeys       []string                 `json:"env_keys"`
	EnvFiles      []IacEnvFile             `json:"env_files"`
	Ports         []map[string]any         `json:"ports"`
	Volumes       []map[string]any         `json:"volumes"`
	Deploy        map[string]any           `json:"deploy"`
}

func upsertIacRepoLocal(ctx context.Context, root string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO iac_repos (kind, root_path, enabled)
		VALUES ('local', $1, TRUE)
		ON CONFLICT (root_path) WHERE kind='local'
		DO UPDATE SET enabled=TRUE
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
		if err != nil { return 0, err }
		return cmd.RowsAffected(), nil
	}
	cmd, err := db.Exec(ctx, `DELETE FROM iac_stacks WHERE repo_id=$1 AND id <> ALL($2)`, repoID, keepIDs)
	if err != nil { return 0, err }
	return cmd.RowsAffected(), nil
}

// ---- Read for API

type IacStackOut struct {
	ID         int64             `json:"id"`
	Name       string            `json:"name"`          // stack_name
	ScopeKind  string            `json:"scope_kind"`
	ScopeName  string            `json:"scope_name"`
	DeployKind string            `json:"deploy_kind"`
	PullPolicy string            `json:"pull_policy"`
	SopsStatus string            `json:"sops_status"`
	IacEnabled bool              `json:"iac_enabled"`
	RelPath    string            `json:"rel_path"`
	Compose    string            `json:"compose_file,omitempty"`
	Services   []IacServiceRow   `json:"services"`
}

func listIacStacksForHost(ctx context.Context, hostName string) ([]IacStackOut, error) {
	h, err := GetHostByName(ctx, hostName)
	if err != nil { return nil, err }

	// gather group names from host row (assuming stored in DB; fallback empty)
	groups := h.Groups

	rows, err := db.Query(ctx, `
	  SELECT id, repo_id, scope_kind, scope_name, stack_name, rel_path, compose_file, deploy_kind, pull_policy, sops_status, iac_enabled
	  FROM iac_stacks
	  WHERE (scope_kind='host' AND scope_name=$1)
	     OR (scope_kind='group' AND scope_name = ANY($2))
	  ORDER BY scope_kind, scope_name, stack_name
	`, hostName, groups)
	if err != nil { return nil, err }
	defer rows.Close()

	var stacks []IacStackOut
	var ids []int64
	for rows.Next() {
		var s IacStackOut
		var repoID int64
		if err := rows.Scan(&s.ID, &repoID, &s.ScopeKind, &s.ScopeName, &s.Name, &s.RelPath, &s.Compose, &s.DeployKind, &s.PullPolicy, &s.SopsStatus, &s.IacEnabled); err != nil {
			return nil, err
		}
		ids = append(ids, s.ID)
		stacks = append(stacks, s)
	}
	if err := rows.Err(); err != nil { return nil, err }

	// load services per stack
	for i := range stacks {
		rs, err := db.Query(ctx, `
		 SELECT id, stack_id, service_name, container_name, image, labels, env_keys, env_files, ports, volumes, deploy
		 FROM iac_services WHERE stack_id=$1 ORDER BY service_name
		`, stacks[i].ID)
		if err != nil { return nil, err }
		var svcs []IacServiceRow
		for rs.Next() {
			var s IacServiceRow
			var lb, ek, ef, pp, vv, dp []byte
			if err := rs.Scan(&s.ID, &s.StackID, &s.ServiceName, &s.ContainerName, &s.Image, &lb, &ek, &ef, &pp, &vv, &dp); err != nil {
				_ = rs.Close()
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
		_ = rs.Close()
		stacks[i].Services = svcs
	}

	return stacks, nil
}
