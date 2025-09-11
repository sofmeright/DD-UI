// src/api/compose_helpers.go
package main

import (
	"context"
	"regexp"
	"strings"
)

// Compose allows lowercase letters, digits, hyphen, underscore.
// We accept any user input, but normalize for Docker at deploy time.
// NOTE: We do NOT trim leading/trailing '-' or '_' so we don't "refuse" them.
var projRe = regexp.MustCompile(`[^a-z0-9_-]+`)

// sanitizeProject converts a user-facing stack name into a Compose project name.
// - lowercase
// - spaces -> underscore
// - any char outside [a-z0-9_-] -> underscore
// If the result ends up empty, fall back to "default".
func sanitizeProject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "default"
	}
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = projRe.ReplaceAllString(s, "_")
	if s == "" {
		return "default"
	}
	return s
}

// composeProjectNameFromStack uses ONLY the stack name (no scope) as the project basis.
func composeProjectNameFromStack(stackName string) string {
	return sanitizeProject(stackName)
}

// Back-compat shim: if older call sites still pass (scope, stack),
// we now intentionally ignore scope and keep only the stack.
func composeProjectNameFromParts(_ string, stackName string) string {
	return composeProjectNameFromStack(stackName)
}

// deriveComposeProjectName reads stack_name and returns the sanitized Compose project name.
func deriveComposeProjectName(ctx context.Context, stackID int64) string {
	var stackName string
	_ = db.QueryRow(ctx, `SELECT stack_name FROM iac_stacks WHERE id=$1`, stackID).Scan(&stackName)
	return composeProjectNameFromStack(stackName)
}

// Legacy convenience (lookups with background context).
func composeProjectName(stackID int64) string {
	return deriveComposeProjectName(context.Background(), stackID)
}
