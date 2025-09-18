// src/api/db_stacks.go
package database

import (
	"context"
	
	"dd-ui/common"
)

// EnsureStack ensures a stack exists for the given host and project
func EnsureStack(ctx context.Context, hostID int64, project, owner string) (int64, error) {
	var id int64
	err := common.DB.QueryRow(ctx, `
		INSERT INTO stacks (host_id, project, owner)
		VALUES ($1, $2, COALESCE(NULLIF($3,''), 'unassigned'))
		ON CONFLICT (host_id, project) DO UPDATE
		  SET owner = COALESCE(EXCLUDED.owner, stacks.owner)
		RETURNING id
	`, hostID, project, owner).Scan(&id)
	return id, err
}
