// services/settings.go
package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"dd-ui/common"
)

// IsTrueish checks if a string represents a true value
func IsTrueish(s string) bool {
	if s == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	}
	return false
}

// GetAppSetting retrieves an app setting value
func GetAppSetting(ctx context.Context, key string) (string, bool) {
	var v string
	err := common.DB.QueryRow(ctx, `SELECT value FROM app_settings WHERE key=$1`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

// SetAppSetting sets an app setting value
func SetAppSetting(ctx context.Context, key, value string) error {
	_, err := common.DB.Exec(ctx, `
		INSERT INTO app_settings (key, value) VALUES ($1,$2)
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()
	`, key, value)
	return err
}

// DelAppSetting deletes an app setting
func DelAppSetting(ctx context.Context, key string) error {
	_, err := common.DB.Exec(ctx, `DELETE FROM app_settings WHERE key=$1`, key)
	return err
}

// GetAppSettingBool retrieves an app setting as a boolean pointer
func GetAppSettingBool(ctx context.Context, key string) (*bool, bool) {
	if s, ok := GetAppSetting(ctx, key); ok {
		b := IsTrueish(s)
		return &b, true
	}
	return nil, false
}

// GetGlobalDevopsApply returns global DevOps apply setting with source
func GetGlobalDevopsApply(ctx context.Context) (bool, string) {
	if b, ok := GetAppSettingBool(ctx, "devops_apply"); ok && b != nil {
		return *b, "db"
	}
	return common.EnvBool("DD_UI_DEVOPS_APPLY", "false"), "env"
}

// SetGlobalDevopsApply sets the global DevOps apply setting
func SetGlobalDevopsApply(ctx context.Context, v *bool) error {
	if v == nil {
		return DelAppSetting(ctx, "devops_apply")
	}
	if *v {
		return SetAppSetting(ctx, "devops_apply", "true")
	}
	return SetAppSetting(ctx, "devops_apply", "false")
}

// GetHostDevopsOverride gets the per-host DevOps override setting
func GetHostDevopsOverride(ctx context.Context, host string) (*bool, error) {
	key := fmt.Sprintf("host:%s:devops_apply", host)
	if s, ok := GetAppSetting(ctx, key); ok {
		b := IsTrueish(s)
		return &b, nil
	}
	return nil, nil
}

// SetHostDevopsOverride sets the per-host DevOps override setting  
func SetHostDevopsOverride(ctx context.Context, host string, v *bool) error {
	key := fmt.Sprintf("host:%s:devops_apply", host)
	if v == nil {
		return DelAppSetting(ctx, key)
	}
	if *v {
		return SetAppSetting(ctx, key, "true")
	}
	return SetAppSetting(ctx, key, "false")
}

// GetGroupDevopsOverride gets the per-group DevOps override setting
func GetGroupDevopsOverride(ctx context.Context, group string) (*bool, error) {
	key := fmt.Sprintf("group:%s:devops_apply", group)
	if s, ok := GetAppSetting(ctx, key); ok {
		b := IsTrueish(s)
		return &b, nil
	}
	return nil, nil
}

// SetGroupDevopsOverride sets the per-group DevOps override setting
func SetGroupDevopsOverride(ctx context.Context, group string, v *bool) error {
	key := fmt.Sprintf("group:%s:devops_apply", group)
	if v == nil {
		return DelAppSetting(ctx, key)
	}
	if *v {
		return SetAppSetting(ctx, key, "true")
	}
	return SetAppSetting(ctx, key, "false")
}

// ShouldAutoApply determines if a stack should be auto-deployed based on hierarchy
func ShouldAutoApply(ctx context.Context, stackID int64) (bool, error) {
	// Get stack details
	var scopeKind, scopeName, stackName string
	err := common.DB.QueryRow(ctx, `
		SELECT scope_kind, scope_name, stack_name 
		FROM iac_stacks 
		WHERE id = $1
	`, stackID).Scan(&scopeKind, &scopeName, &stackName)
	if err != nil {
		return false, err
	}

	// 1. Check stack-level override (most specific)
	if stackOverride, _ := GetStackDevopsOverride(ctx, scopeKind, scopeName, stackName); stackOverride != nil {
		return *stackOverride, nil
	}

	// 2. For host stacks, check host-level override
	if scopeKind == "host" {
		if hostOverride, _ := GetHostDevopsOverride(ctx, scopeName); hostOverride != nil {
			return *hostOverride, nil
		}
		
		// 3. Check group-level overrides for this host
		// Get the groups this host belongs to
		var groups []string
		rows, err := common.DB.Query(ctx, `
			SELECT UNNEST(groups) as group_name 
			FROM hosts 
			WHERE name = $1 
			ORDER BY group_name
		`, scopeName)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var groupName string
				if rows.Scan(&groupName) == nil {
					groups = append(groups, groupName)
				}
			}
		}
		
		// Check each group in order (alphabetically sorted for consistency)
		for _, group := range groups {
			if groupOverride, _ := GetGroupDevopsOverride(ctx, group); groupOverride != nil {
				return *groupOverride, nil
			}
		}
	} else if scopeKind == "group" {
		// For group stacks, just check the group-level override
		if groupOverride, _ := GetGroupDevopsOverride(ctx, scopeName); groupOverride != nil {
			return *groupOverride, nil
		}
	}

	// 4. Check global setting (DB or environment variable)
	global, _ := GetGlobalDevopsApply(ctx)
	return global, nil
}

// JoinUnder safely joins paths under a root directory, preventing directory traversal
func JoinUnder(root, rel string) (string, error) {
	clean := filepath.Clean("/" + rel) // force absolute-clean then strip
	clean = strings.TrimPrefix(clean, "/")
	full := filepath.Join(root, clean)
	r, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(r, "..") {
		return "", errors.New("outside root")
	}
	return full, nil
}