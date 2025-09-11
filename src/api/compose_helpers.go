// src/api/compose_helpers.go
package main

import (
	"context"
	"regexp"
	"strings"
)

// NOTE: Compose projects accept [a-z0-9][a-z0-9_-]* (Compose normalizes).
// We keep the user's *display* name untouched in the UI, and only
// normalize the internal *project label* passed to docker compose -p.
var projRe = regexp.MustCompile(`[^a-z0-9_-]+`)

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

// Compose project label = <scope_name>_<stack_name> (normalized for Compose)
func composeProjectNameFromParts(scopeName, stackName string) string {
	base := strings.TrimSpace(scopeName) + "_" + strings.TrimSpace(stackName)
	return sanitizeProject(base)
}

func deriveComposeProjectName(ctx context.Context, stackID int64) string {
	var scopeName, stackName string
	_ = db.QueryRow(ctx, `
		SELECT scope_name, stack_name
		FROM iac_stacks
		WHERE id=$1
	`, stackID).Scan(&scopeName, &stackName)
	return composeProjectNameFromParts(scopeName, stackName)
}

// Legacy shim (do not add new callers)
func composeProjectName(stackID int64) string {
	return deriveComposeProjectName(context.Background(), stackID)
}
