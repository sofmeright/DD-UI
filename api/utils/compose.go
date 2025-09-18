// src/api/utils/compose.go
package utils

import (
	"context"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// We keep the user's *display* stack name untouched for -p.
// Only for **lookups** we normalize to Compose's label form.
var projRe = regexp.MustCompile(`[^a-z0-9_-]+`)

func SanitizeProject(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = projRe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_-")
	if s == "" {
		s = "default"
	}
	return s
}

// For lookups: com.docker.compose.project=<sanitized(stack_name)>
func ComposeProjectLabelFromStack(stackName string) string {
	return SanitizeProject(stackName)
}

// fetchStackName retrieves a stack name by ID from the database
// Note: This function signature changed - callers must pass their db connection
func FetchStackName(ctx context.Context, db *pgxpool.Pool, stackID int64) (string, error) {
	var name string
	err := db.QueryRow(ctx, `
		SELECT stack_name
		FROM iac_stacks
		WHERE id=$1
	`, stackID).Scan(&name)
	return name, err
}

// Legacy shim (don't add new callers): returns **label form** of stack name.
// Note: This function has been deprecated - use fetchStackName + composeProjectLabelFromStack instead
func ComposeProjectName(db *pgxpool.Pool, stackID int64) string {
	name, _ := FetchStackName(context.Background(), db, stackID)
	return ComposeProjectLabelFromStack(name)
}
