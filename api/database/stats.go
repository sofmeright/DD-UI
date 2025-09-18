package database

import (
	"context"
	"time"
	"dd-ui/common"
)

// Stack represents a Docker Compose stack
type Stack struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Status    string    `json:"status"`
	State     string    `json:"state"` // running, stopped, partial
	Path      string    `json:"path"`
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Host represents a host in the system
type Host struct {
	ID       int64  `json:"id"`
	Hostname string `json:"hostname"`
	Address  string `json:"address"`
	Owner    string `json:"owner"`
}

// GetHosts returns all hosts in the system
func GetHosts(ctx context.Context) ([]Host, error) {
	query := `SELECT id, name, addr, COALESCE(owner, 'unassigned') FROM hosts ORDER BY name`
	rows, err := common.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []Host
	for rows.Next() {
		var host Host
		err := rows.Scan(&host.ID, &host.Hostname, &host.Address, &host.Owner)
		if err != nil {
			continue
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

// GetHostCount returns the total number of hosts
func GetHostCount(ctx context.Context) (int, error) {
	var count int
	err := common.DB.QueryRow(ctx, "SELECT COUNT(*) FROM hosts").Scan(&count)
	return count, err
}

// GetStackCount returns the total number of stacks
func GetStackCount(ctx context.Context) (int, error) {
	var count int
	err := common.DB.QueryRow(ctx, "SELECT COUNT(DISTINCT name) FROM stacks").Scan(&count)
	return count, err
}

// GetContainerCount returns the total number of containers
func GetContainerCount(ctx context.Context) (int, error) {
	var count int
	err := common.DB.QueryRow(ctx, "SELECT COUNT(*) FROM containers").Scan(&count)
	return count, err
}

// GetStacksByHost returns all stacks for a specific host
func GetStacksByHost(ctx context.Context, hostname string) ([]Stack, error) {
	query := `
		SELECT DISTINCT ON (s.name)
			s.id, s.name, s.host, s.status, s.path, 
			s.created_at, s.updated_at, s.owner,
			COUNT(c.id) as container_count
		FROM stacks s
		LEFT JOIN containers c ON c.stack = s.name AND c.host = s.host
		WHERE s.host = $1
		GROUP BY s.id, s.name, s.host, s.status, s.path, s.created_at, s.updated_at, s.owner
		ORDER BY s.name, s.updated_at DESC
	`
	
	rows, err := common.DB.Query(ctx, query, hostname)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stacks []Stack
	for rows.Next() {
		var stack Stack
		var containerCount int
		err := rows.Scan(
			&stack.ID, &stack.Name, &stack.Host, &stack.Status,
			&stack.Path, &stack.CreatedAt, &stack.UpdatedAt, &stack.Owner,
			&containerCount,
		)
		if err != nil {
			continue
		}
		
		// Set state based on status
		if stack.Status == "running" && containerCount > 0 {
			stack.State = "running"
		} else if containerCount == 0 {
			stack.State = "stopped"
		} else {
			stack.State = "partial"
		}
		
		stacks = append(stacks, stack)
	}

	return stacks, nil
}