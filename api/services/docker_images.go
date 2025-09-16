// src/api/services/docker_images.go
package services

import (
	"context"
	"strconv"
	"strings"

	"dd-ui/common"
)

// UpsertImageTag inserts or updates an image tag record in the database
func UpsertImageTag(ctx context.Context, host, id, repo, tag string) error {
	_, err := common.DB.Exec(ctx, `
		INSERT INTO image_tags (host_name, image_id, repo, tag)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (host_name, image_id)
		DO UPDATE SET repo=EXCLUDED.repo, tag=EXCLUDED.tag, last_seen=now();
	`, host, id, repo, tag)
	return err
}

// GetImageTagMap retrieves a map of image IDs to [repo, tag] pairs for a given host
func GetImageTagMap(ctx context.Context, host string) (map[string][2]string, error) {
	rows, err := common.DB.Query(ctx, `SELECT image_id, repo, tag FROM image_tags WHERE host_name=$1`, host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][2]string)
	for rows.Next() {
		var id, repo, tag string
		if err := rows.Scan(&id, &repo, &tag); err != nil {
			return nil, err
		}
		out[id] = [2]string{repo, tag}
	}
	return out, nil
}

// CleanupImageTags removes image tag records that are not in the keepIDs set
func CleanupImageTags(ctx context.Context, host string, keepIDs map[string]struct{}) error {
	ids := make([]string, 0, len(keepIDs))
	for id := range keepIDs {
		ids = append(ids, id)
	}
	_, err := common.DB.Exec(ctx, `
		DELETE FROM image_tags t
		WHERE t.host_name = $1
		  AND NOT EXISTS (
		    SELECT 1
		    FROM UNNEST($2::text[]) AS u(id)
		    WHERE u.id = t.image_id
		  );
	`, host, ids)
	return err
}

// HumanSize converts bytes to human-readable format (e.g., "1.5 GB")
func HumanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatInt(b, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strings.TrimSuffix(strings.TrimSpace(
		strconv.FormatFloat(float64(b)/float64(div), 'f', 1, 64)), ".0") + " " + string("KMGTPE"[exp]) + "B"
}