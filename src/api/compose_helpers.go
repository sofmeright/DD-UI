// src/api/compose_helpers.go
package main

import (
	"context"
	"regexp"
	"strings"
)

// We keep the user's *display* stack name untouched for -p.
// Only for **lookups** we normalize to Compose's label form.
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

// For lookups: com.docker.compose.project=<sanitized(stack_name)>
func composeProjectLabelFromStack(stackName string) string {
	return sanitizeProject(stackName)
}

func fetchStackName(ctx context.Context, stackID int64) (string, error) {
	var name string
	err := db.QueryRow(ctx, `
		SELECT stack_name
		FROM iac_stacks
		WHERE id=$1
	`, stackID).Scan(&name)
	return name, err
}

// Legacy shim (don’t add new callers): returns **label form** of stack name.
func composeProjectName(stackID int64) string {
	name, _ := fetchStackName(context.Background(), stackID)
	return composeProjectLabelFromStack(name)
}
