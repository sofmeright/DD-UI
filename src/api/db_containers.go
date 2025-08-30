// src/api/db_containers.go
package main

import (
	"context"
	"encoding/json"
	"time"
)

type ContainerRow struct {
	ID           int64             `json:"id"`
	HostID       int64             `json:"host_id"`
	StackID      *int64            `json:"stack_id,omitempty"`
	ContainerID  string            `json:"container_id"`
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	State        string            `json:"state"`
	Status       string            `json:"status"`
	Ports        []any             `json:"ports"`                    // stored as JSONB array
	Labels       map[string]string `json:"labels"`                   // stored as JSONB object
	ComposeProj  string            `json:"compose_project,omitempty"`// from compose/stack labels or stacks.project
	ComposeSvc   string            `json:"compose_service,omitempty"`
	Owner        string            `json:"owner"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// upsertContainer inserts/updates a container row.
// Pass stackID as nil when no project is inferred.
func upsertContainer(
	ctx context.Context,
	hostID int64,
	stackID *int64,
	cid, name, image, state, status, owner string,
	ports any,                       // e.g. docker's []types.Port or already-marshaled shape
	labels map[string]string,
) error {
	portsB, _ := json.Marshal(ports)
	labsB, _ := json.Marshal(labels)

	_, err := db.Exec(ctx, `
		INSERT INTO containers (host_id, stack_id, container_id, name, image, state, status, ports, labels, owner)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,COALESCE(NULLIF($10,''), 'unassigned'))
		ON CONFLICT (host_id, container_id) DO UPDATE
		  SET stack_id   = EXCLUDED.stack_id,
		      name       = EXCLUDED.name,
		      image      = EXCLUDED.image,
		      state      = EXCLUDED.state,
		      status     = EXCLUDED.status,
		      ports      = EXCLUDED.ports,
		      labels     = EXCLUDED.labels,
		      owner      = COALESCE(EXCLUDED.owner, containers.owner),
		      updated_at = now()
	`, hostID, stackID, cid, name, image, state, status, string(portsB), string(labsB), owner)
	return err
}

// listContainersByHost returns the persisted container state for a host.
func listContainersByHost(ctx context.Context, hostName string) ([]ContainerRow, error) {
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(ctx, `
		SELECT
		  c.id, c.host_id, c.stack_id, c.container_id, c.name, c.image, c.state, c.status,
		  c.ports, c.labels, s.project, c.owner, c.updated_at
		FROM containers c
		LEFT JOIN stacks s ON s.id = c.stack_id
		WHERE c.host_id = $1
		ORDER BY c.name
	`, h.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContainerRow
	for rows.Next() {
		var (
			cr                  ContainerRow
			portsB, labelsB     []byte
			projectFromStack    *string
		)
		if err := rows.Scan(
			&cr.ID, &cr.HostID, &cr.StackID, &cr.ContainerID, &cr.Name, &cr.Image, &cr.State, &cr.Status,
			&portsB, &labelsB, &projectFromStack, &cr.Owner, &cr.UpdatedAt,
		); err != nil {
			return nil, err
		}

		// Decode JSON fields
		_ = json.Unmarshal(portsB, &cr.Ports)   // []any
		_ = json.Unmarshal(labelsB, &cr.Labels) // map[string]string

		// Derive compose metadata
		if projectFromStack != nil && *projectFromStack != "" {
			cr.ComposeProj = *projectFromStack
		} else if v, ok := cr.Labels["com.docker.compose.project"]; ok && v != "" {
			cr.ComposeProj = v
		} else if v, ok := cr.Labels["com.docker.stack.namespace"]; ok && v != "" {
			cr.ComposeProj = v
		}

		if v, ok := cr.Labels["com.docker.compose.service"]; ok && v != "" {
			cr.ComposeSvc = v
		} else if v, ok := cr.Labels["com.docker.service.name"]; ok && v != "" {
			cr.ComposeSvc = v
		}

		out = append(out, cr)
	}
	return out, rows.Err()
}
