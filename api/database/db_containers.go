// src/api/db_containers.go
package database

import (
	"dd-ui/common"
	"context"
	"encoding/json"
	"time"
)

type ContainerRow struct {
	ID           int64              `json:"id"`
	HostID       int64              `json:"host_id"`
	StackID      *int64             `json:"stack_id,omitempty"`
	ContainerID  string             `json:"container_id"`
	Name         string             `json:"name"`
	Image        string             `json:"image"`
	State        string             `json:"state"`
	Status       string             `json:"status"`
	Health       string             `json:"health,omitempty"`
	Ports        []any              `json:"ports"`                      // flattened list (see scan)
	Labels       map[string]string  `json:"labels"`
	Env          []string           `json:"env"`                        // raw docker env ["K=V", ...]
	Networks     map[string]any     `json:"networks"`                   // map[name]=>summary
	Mounts       []any              `json:"mounts"`
	IPAddr       string             `json:"ip_addr,omitempty"`
	CreatedTS    *time.Time         `json:"created_ts,omitempty"`
	ComposeProj  string             `json:"compose_project,omitempty"`
	ComposeSvc   string             `json:"compose_service,omitempty"`
	Owner        string             `json:"owner"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

// Upsert; stackID may be nil.
// UpsertContainer inserts or updates a container
func UpsertContainer(
	ctx context.Context,
	hostID int64,
	stackID *int64,
	cid, name, image, state, status, owner string,
	created *time.Time,
	ip string,
	ports any,
	labels map[string]string,
	env []string,
	networks any,
	mounts any,
) error {
	portsB, _ := json.Marshal(ports)
	labsB, _ := json.Marshal(labels)
	envB, _ := json.Marshal(env)
	nwB, _ := json.Marshal(networks)
	mountsB, _ := json.Marshal(mounts)

	_, err := common.DB.Exec(ctx, `
		INSERT INTO containers (
		  host_id, stack_id, container_id, name, image, state, status,
		  ports, labels, owner, created_ts, ip_addr, env, networks, mounts
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,COALESCE(NULLIF($10,''),'unassigned'),
		        $11, $12, $13::jsonb, $14::jsonb, $15::jsonb)
		ON CONFLICT (host_id, container_id) DO UPDATE
		  SET stack_id   = EXCLUDED.stack_id,
		      name       = EXCLUDED.name,
		      image      = EXCLUDED.image,
		      state      = EXCLUDED.state,
		      status     = EXCLUDED.status,
		      ports      = EXCLUDED.ports,
		      labels     = EXCLUDED.labels,
		      owner      = COALESCE(EXCLUDED.owner, containers.owner),
		      created_ts = EXCLUDED.created_ts,
		      ip_addr    = EXCLUDED.ip_addr,
		      env        = EXCLUDED.env,
		      networks   = EXCLUDED.networks,
		      mounts     = EXCLUDED.mounts,
		      updated_at = now()
	`, hostID, stackID, cid, name, image, state, status,
		string(portsB), string(labsB), owner,
		created, ip, string(envB), string(nwB), string(mountsB))
	return err
}

// Remove rows that weren't seen in the latest scan for this host.
// PruneMissingContainers removes containers that are no longer running
func PruneMissingContainers(ctx context.Context, hostID int64, keepIDs []string) (int64, error) {
	if len(keepIDs) == 0 {
		cmd, err := common.DB.Exec(ctx, `DELETE FROM containers WHERE host_id=$1`, hostID)
		if err != nil { return 0, err }
		return cmd.RowsAffected(), nil
	}
	cmd, err := common.DB.Exec(ctx, `
		DELETE FROM containers
		WHERE host_id=$1 AND NOT (container_id = ANY($2))
	`, hostID, keepIDs)
	if err != nil { return 0, err }
	return cmd.RowsAffected(), nil
}

// getContainerByHostAndName fetches a single container by host and container name
// GetContainerByHostAndName gets a container by host and container name
func GetContainerByHostAndName(ctx context.Context, hostName, containerName string) (*ContainerRow, error) {
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, err
	}
	
	var (
		cr                  ContainerRow
		portsB, labelsB     []byte
		envB, nwB, mountsB  []byte
		projectFromStack    *string
	)
	
	err = common.DB.QueryRow(ctx, `
		SELECT
		  c.id, c.host_id, c.stack_id, c.container_id, c.name, c.image, c.state, c.status,
		  c.ports, c.labels, c.env, c.networks, c.mounts, c.ip_addr, c.created_ts,
		  s.project, c.owner, c.updated_at
		FROM containers c
		LEFT JOIN stacks s ON s.id = c.stack_id
		WHERE c.host_id = $1 AND c.name = $2
		ORDER BY c.updated_at DESC
		LIMIT 1
	`, h.ID, containerName).Scan(
		&cr.ID, &cr.HostID, &cr.StackID, &cr.ContainerID, &cr.Name, &cr.Image, &cr.State, &cr.Status,
		&portsB, &labelsB, &envB, &nwB, &mountsB, &cr.IPAddr, &cr.CreatedTS,
		&projectFromStack, &cr.Owner, &cr.UpdatedAt,
	)
	
	if err != nil {
		return nil, err
	}
	
	_ = json.Unmarshal(portsB, &cr.Ports)
	_ = json.Unmarshal(labelsB, &cr.Labels)
	_ = json.Unmarshal(envB, &cr.Env)
	_ = json.Unmarshal(nwB, &cr.Networks)
	_ = json.Unmarshal(mountsB, &cr.Mounts)
	
	// Derive compose metadata
	if projectFromStack != nil && *projectFromStack != "" {
		cr.ComposeProj = *projectFromStack
	} else if cr.Labels != nil {
		if lp, ok := cr.Labels["com.docker.compose.project"]; ok {
			cr.ComposeProj = lp
		}
	}
	if cr.Labels != nil {
		if ls, ok := cr.Labels["com.docker.compose.service"]; ok {
			cr.ComposeSvc = ls
		}
	}
	
	// Add health from labels if available
	if cr.Labels != nil {
		if health, ok := cr.Labels["com.docker.compose.health"]; ok {
			cr.Health = health
		} else if health, ok := cr.Labels["org.label-schema.health"]; ok {
			cr.Health = health
		}
	}
	
	return &cr, nil
}

// listContainersByHost includes created_ts, ip, env, networks, mounts, and compose_* derivations.
// ListContainersByHost lists all containers for a host
func ListContainersByHost(ctx context.Context, hostName string) ([]ContainerRow, error) {
	h, err := GetHostByName(ctx, hostName)
	if err != nil {
		return nil, err
	}
	rows, err := common.DB.Query(ctx, `
		SELECT
		  c.id, c.host_id, c.stack_id, c.container_id, c.name, c.image, c.state, c.status,
		  c.ports, c.labels, c.env, c.networks, c.mounts, c.ip_addr, c.created_ts,
		  s.project, c.owner, c.updated_at
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
			envB, nwB, mountsB  []byte
			projectFromStack    *string
		)
		if err := rows.Scan(
			&cr.ID, &cr.HostID, &cr.StackID, &cr.ContainerID, &cr.Name, &cr.Image, &cr.State, &cr.Status,
			&portsB, &labelsB, &envB, &nwB, &mountsB, &cr.IPAddr, &cr.CreatedTS,
			&projectFromStack, &cr.Owner, &cr.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(portsB, &cr.Ports)
		_ = json.Unmarshal(labelsB, &cr.Labels)
		_ = json.Unmarshal(envB, &cr.Env)
		_ = json.Unmarshal(nwB, &cr.Networks)
		_ = json.Unmarshal(mountsB, &cr.Mounts)

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

		// Add health from labels if available
		if cr.Labels != nil {
			if health, ok := cr.Labels["com.docker.compose.health"]; ok {
				cr.Health = health
			} else if health, ok := cr.Labels["org.label-schema.health"]; ok {
				cr.Health = health
			}
		}

		out = append(out, cr)
	}
	return out, rows.Err()
}
