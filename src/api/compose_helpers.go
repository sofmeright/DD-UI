package main

import (
	"context"
	"regexp"
	"strings"
)

// Compose normalizes the project name internally (lowercase, [a-z0-9_-]) for the
// com.docker.compose.project label. We use this ONLY for lookups, not to change
// what the user typed. We still pass the raw stack_name to `docker compose -p`.
var projRe = regexp.MustCompile(`[^a-z0-9_-]+`)

// sanitizeProject mirrors Compose's normalization enough for label matching.
func sanitizeProject(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = projRe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_-")
	if s == "" {
		s = "default"
	}
	return s
}

// composeProjectLabelFromStack returns the label value Compose will use for the project,
// derived ONLY from the stack name (no scope prefix).
func composeProjectLabelFromStack(stackName string) string {
	return sanitizeProject(stackName)
}

// fetchStackName returns the raw stack_name exactly as stored (what user typed).
func fetchStackName(ctx context.Context, stackID int64) (string, error) {
	var name string
	err := db.QueryRow(ctx, `
		SELECT stack_name
		FROM iac_stacks
		WHERE id=$1
	`, stackID).Scan(&name)
	return name, err
}

// Legacy shim for old callers that expected a "compose project name" from stackID.
// It now returns the **label form** (sanitized) based on stack_name only.
func composeProjectName(stackID int64) string {
	name, _ := fetchStackName(context.Background(), stackID)
	return composeProjectLabelFromStack(name)
}
