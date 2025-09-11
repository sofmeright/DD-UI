// src/api/compose_helpers.go
package main

import (
	"context"
	"regexp"
	"strings"
)

// sanitizeProject normalizes a Compose project name to docker-compose-friendly chars.
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

// composeProjectNameFromParts builds the project name from scope & stack.
func composeProjectNameFromParts(scopeName, stackName string) string {
	base := strings.TrimSpace(scopeName) + "_" + strings.TrimSpace(stackName)
	return sanitizeProject(base)
}

// deriveComposeProjectName fetches scope/stack for a stack ID and returns the project name.
func deriveComposeProjectName(ctx context.Context, stackID int64) string {
	var scopeName, stackName string
	_ = db.QueryRow(ctx, `
		SELECT scope_name, stack_name
		FROM iac_stacks
		WHERE id=$1
	`, stackID).Scan(&scopeName, &stackName)
	return composeProjectNameFromParts(scopeName, stackName)
}

// composeProjectName is a back-compat shim for legacy call sites that pass only stackID.
// It uses context.Background() to look up scope/stack.
func composeProjectName(stackID int64) string {
	return deriveComposeProjectName(context.Background(), stackID)
}
