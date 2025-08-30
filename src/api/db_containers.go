package main

import (
	"context"
	"encoding/json"
)

type StackRow struct {
	ID      int64  `json:"id"`
	HostID  int64  `json:"host_id"`
	Project string `json:"project"`
	Source  string `json:"source"`
	Owner   string `json:"owner"`
}

type ContainerRow struct {
	ID           int64             `json:"id"`
	HostID       int64             `json:"host_id"`
	StackID      *int64            `json:"stack_id,omitempty"`
	ContainerID  string            `json:"container_id"`
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	State        string            `json:"state"`
	Status       string            `json:"status"`
	Ports        []any             `json:"ports"`
	Labels       map[string]string `json:"labels"`
	ComposeProj  string            `json:"compose_project,omitempty"`
	ComposeSvc   string            `json:"compose_service,omitempty"`
	Owner        string            `json:"owner"`
}

func hostIDByName(ctx context.Context, name string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `SELECT id FROM hosts WHERE name=$1`, name).Scan(&id)
	return id, err
}

func hostOwnerByID(ctx context.Context, id int64) (string, error) {
	var owner string
	err := db.QueryRow(ctx, `SELECT owner FROM hosts WHERE id=$1`, id).Scan(&owner)
	return owner, err
}

func upsertStack(ctx context.Context, hostID int64, project string, owner string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO stacks(host_id, project, owner) VALUES($1,$2,$3)
		ON CONFLICT(host_id, project) DO UPDATE SET
			project = EXCLUDED.project,
			owner   = EXCLUDED.owner
		RETURNING id
	`, hostID, project, owner).Scan(&id)
	return id, err
}

func upsertContainer(ctx context.Context, c ContainerRow) error {
	portsJSON, _ := json.Marshal(c.Ports)
	labelsJSON, _ := json.Marshal(c.Labels)

	var stackID *int64
	if c.ComposeProj != "" {
		id, err := upsertStack(ctx, c.HostID, c.ComposeProj, c.Owner)
		if err == nil {
			stackID = &id
		}
	}

	_, err := db.Exec(ctx, `
		INSERT INTO containers
		  (host_id, stack_id, container_id, name, image, state, status, ports, labels, owner)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT(host_id, container_id) DO UPDATE SET
		  stack_id = EXCLUDED.stack_id,
		  name     = EXCLUDED.name,
		  image    = EXCLUDED.image,
		  state    = EXCLUDED.state,
		  status   = EXCLUDED.status,
		  ports    = EXCLUDED.ports,
		  labels   = EXCLUDED.labels,
		  owner    = EXCLUDED.owner,
		  updated_at = now()
	`, c.HostID, stackID, c.ContainerID, c.Name, c.Image, c.State, c.Status, string(portsJSON), string(labelsJSON), c.Owner)
	return err
}

func listContainersByHost(ctx context.Context, hostName string) ([]ContainerRow, error) {
	var out []ContainerRow
	rows, err := db.Query(ctx, `
		SELECT c.container_id, c.name, c.image, c.state, c.status, c.ports, c.labels,
		       s.project, c.stack_id, c.host_id, c.owner
		FROM containers c
		LEFT JOIN stacks s ON s.id = c.stack_id
		JOIN hosts h ON h.id = c.host_id
		WHERE h.name = $1
		ORDER BY c.name
	`, hostName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var portsB []byte
		var labelsB []byte
		var proj *string
		var stackID *int64
		var c ContainerRow
		if err := rows.Scan(&c.ContainerID, &c.Name, &c.Image, &c.State, &c.Status, &portsB, &labelsB, &proj, &stackID, &c.HostID, &c.Owner); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(portsB, &c.Ports)
		_ = json.Unmarshal(labelsB, &c.Labels)
		if proj != nil {
			c.ComposeProj = *proj
		}
		c.StackID = stackID
		out = append(out, c)
	}
	return out, rows.Err()
}
