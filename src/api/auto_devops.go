// src/api/auto_devops.go
package main

import (
	"context"
	"errors"
	"strings"
)

// shouldAutoApply resolves effective Auto DevOps as:
// 1) explicit per-stack override (iac_stacks.auto_apply_override) if set
// 2) scope override: host_settings or group_settings (based on stack scope)
// 3) global DB override (app_settings.devops_apply) if set
// 4) DDUI_DEVOPS_APPLY env flag (default "false")
// Manual deployments bypass this gate.
func shouldAutoApply(ctx context.Context, stackID int64) (bool, error) {
	var scopeKind, scopeName string
	var stackOv *bool

	err := db.QueryRow(ctx, `
		SELECT scope_kind::text, scope_name, auto_apply_override
		FROM iac_stacks WHERE id=$1
	`, stackID).Scan(&scopeKind, &scopeName, &stackOv)
	if err != nil {
		return false, errors.New("stack not found")
	}

	// 1) Stack override wins if present
	if stackOv != nil {
		return *stackOv, nil
	}

	// 2) Scope override
	switch strings.ToLower(scopeKind) {
	case "host":
		if hov, _ := getHostDevopsOverride(ctx, scopeName); hov != nil {
			return *hov, nil
		}
	case "group":
		if gov, _ := getGroupDevopsOverride(ctx, scopeName); gov != nil {
			return *gov, nil
		}
	}

	// 3) Global DB override
	if glob, ok := getAppSettingBool(ctx, "devops_apply"); ok && glob != nil {
		return *glob, nil
	}

	// 4) Env fallback (default false if absent/invalid)
	return envBool("DDUI_DEVOPS_APPLY", "false"), nil
}
