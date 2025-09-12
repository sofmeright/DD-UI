// src/api/auto_devops_policy.go
package main

import (
	"context"
	"os"
	"slices"
	"sort"
	"strings"
)

// TriState represents an explicit override or lack thereof.
type TriState int

const (
	OverrideUnset TriState = iota
	OverrideDisable
	OverrideEnable
)

func (t TriState) PtrBool() *bool {
	switch t {
	case OverrideEnable:
		v := true
		return &v
	case OverrideDisable:
		v := false
		return &v
	default:
		return nil
	}
}

// parseEnvDefault reads DDUI_DEVOPS_APPLY: "true"/"false" -> bool; anything else => unset (nil).
func parseEnvDefault() *bool {
	raw := strings.TrimSpace(os.Getenv("DDUI_DEVOPS_APPLY"))
	if raw == "" {
		return nil
	}
	switch strings.ToLower(raw) {
	case "true":
		v := true
		return &v
	case "false":
		v := false
		return &v
	default:
		return nil // unset/fallthrough
	}
}

// --- Override fetchers -------------------------------------------------------
// These are intentionally tolerant: if a table/column isn’t present, or no row,
// they return OverrideUnset (nil).

// getHostStackOverride returns an explicit override for (host, stack) if any.
func getHostStackOverride(ctx context.Context, hostName, stackName string) TriState {
	// Try a dedicated override table first (if you have one).
	var v string
	err := db.QueryRow(ctx, `
		SELECT auto_devops_override
		FROM iac_overrides
		WHERE level='host' AND scope_name=$1 AND stack_name=$2
	`, hostName, stackName).Scan(&v)
	if err == nil {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "enable", "on", "true", "1":
			return OverrideEnable
		case "disable", "off", "false", "0":
			return OverrideDisable
		}
	}

	// Optionally: some deployments store it on iac_stacks as a tri-state text column.
	// If this column doesn't exist, this query may error; we ignore it.
	err = db.QueryRow(ctx, `
		SELECT auto_devops_override
		FROM iac_stacks
		WHERE scope_kind='host' AND scope_name=$1 AND stack_name=$2
	`, hostName, stackName).Scan(&v)
	if err == nil {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "enable", "on", "true", "1":
			return OverrideEnable
		case "disable", "off", "false", "0":
			return OverrideDisable
		}
	}

	return OverrideUnset
}

// getGroupStackOverride: evaluate groups sorted ascending by name; pick first explicit override.
func getGroupStackOverride(ctx context.Context, groups []string, stackName string) TriState {
	if len(groups) == 0 {
		return OverrideUnset
	}
	// Ensure deterministic order at SQL level as well.
	rows, err := db.Query(ctx, `
		SELECT scope_name, auto_devops_override
		FROM iac_overrides
		WHERE level='group' AND stack_name=$1 AND scope_name = ANY($2)
		ORDER BY scope_name ASC
	`, stackName, groups)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var g, v string
			if rows.Scan(&g, &v) == nil {
				switch strings.ToLower(strings.TrimSpace(v)) {
				case "enable", "on", "true", "1":
					return OverrideEnable
				case "disable", "off", "false", "0":
					return OverrideDisable
				}
			}
		}
	}

	// Optional alternate storage on a group/stack table.
	rows, err = db.Query(ctx, `
		SELECT scope_name, auto_devops_override
		FROM iac_stacks
		WHERE scope_kind='group' AND stack_name=$1 AND scope_name = ANY($2)
		ORDER BY scope_name ASC
	`, stackName, groups)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var g, v string
			if rows.Scan(&g, &v) == nil {
				switch strings.ToLower(strings.TrimSpace(v)) {
				case "enable", "on", "true", "1":
					return OverrideEnable
				case "disable", "off", "false", "0":
					return OverrideDisable
				}
			}
		}
	}

	return OverrideUnset
}

// getGlobalOverride returns a global override if any (user-set via GUI).
func getGlobalOverride(ctx context.Context) TriState {
	var v string
	err := db.QueryRow(ctx, `
		SELECT value
		FROM settings
		WHERE key='auto_devops_override'
	`).Scan(&v)
	if err == nil {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "enable", "on", "true", "1":
			return OverrideEnable
		case "disable", "off", "false", "0":
			return OverrideDisable
		}
	}
	// Optional alternative table:
	err = db.QueryRow(ctx, `
		SELECT value
		FROM iac_overrides
		WHERE level='global' AND key='auto_devops'
	`).Scan(&v)
	if err == nil {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "enable", "on", "true", "1":
			return OverrideEnable
		case "disable", "off", "false", "0":
			return OverrideDisable
		}
	}
	return OverrideUnset
}

// effectiveAutoDevops computes the policy chain (not gated by content/existence).
// Returns (effective, origin), where origin is one of: host, group, global, env, fallback.
func effectiveAutoDevops(ctx context.Context, stackID int64) (bool, string) {
	// Load host, groups, name.
	var hostName, scopeKind, scopeName, stackName string
	if err := db.QueryRow(ctx, `
		SELECT s.scope_kind, s.scope_name, s.stack_name, h.name
		FROM iac_stacks s
		LEFT JOIN hosts h ON (s.scope_kind='host' AND s.scope_name=h.name)
		WHERE s.id=$1
	`, stackID).Scan(&scopeKind, &scopeName, &stackName, &hostName); err != nil {
		// If we can't read, default to no auto-deploy.
		return false, "fallback"
	}

	// Host groups (if any)
	var groups []string
	if scopeKind == "host" {
		h, err := GetHostByName(ctx, scopeName)
		if err == nil && len(h.Groups) > 0 {
			groups = append(groups, h.Groups...)
			slices.Sort(groups)
		}
	}

	// 1) host/stack
	if scopeKind == "host" {
		if t := getHostStackOverride(ctx, scopeName, stackName); t != OverrideUnset {
			return t == OverrideEnable, "host"
		}
	}
	// 2) group/stack (deterministic first by name)
	if t := getGroupStackOverride(ctx, groups, stackName); t != OverrideUnset {
		return t == OverrideEnable, "group"
	}
	// 3) global override
	if t := getGlobalOverride(ctx); t != OverrideUnset {
		return t == OverrideEnable, "global"
	}
	// 4) env default
	if v := parseEnvDefault(); v != nil {
		return *v, "env"
	}
	// 5) fallback: do not deploy
	return false, "fallback"
}

// shouldAutoApply enforces the **auto** path. Manual deploys bypass the callsite.
func shouldAutoApply(ctx context.Context, stackID int64) (bool, error) {
	ok, _ := effectiveAutoDevops(ctx, stackID)
	return ok, nil
}
