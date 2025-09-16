// services/settings.go
package services

import (
	"context"
	"errors"
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
	var val *bool
	err := common.DB.QueryRow(ctx, `SELECT auto_apply_override FROM host_settings WHERE host_name=$1`, host).Scan(&val)
	if err != nil {
		return nil, nil // treat as absent
	}
	return val, nil
}

// SetHostDevopsOverride sets the per-host DevOps override setting
func SetHostDevopsOverride(ctx context.Context, host string, v *bool) error {
	if v == nil {
		_, err := common.DB.Exec(ctx, `DELETE FROM host_settings WHERE host_name=$1`, host)
		return err
	}
	_, err := common.DB.Exec(ctx, `
		INSERT INTO host_settings (host_name, auto_apply_override)
		VALUES ($1,$2)
		ON CONFLICT (host_name) DO UPDATE SET auto_apply_override=EXCLUDED.auto_apply_override, updated_at=now()
	`, host, *v)
	return err
}

// GetGroupDevopsOverride gets the per-group DevOps override setting
func GetGroupDevopsOverride(ctx context.Context, group string) (*bool, error) {
	var val *bool
	err := common.DB.QueryRow(ctx, `SELECT auto_apply_override FROM group_settings WHERE group_name=$1`, group).Scan(&val)
	if err != nil {
		return nil, nil // treat as absent
	}
	return val, nil
}

// SetGroupDevopsOverride sets the per-group DevOps override setting
func SetGroupDevopsOverride(ctx context.Context, group string, v *bool) error {
	if v == nil {
		_, err := common.DB.Exec(ctx, `DELETE FROM group_settings WHERE group_name=$1`, group)
		return err
	}
	_, err := common.DB.Exec(ctx, `
		INSERT INTO group_settings (group_name, auto_apply_override)
		VALUES ($1,$2)
		ON CONFLICT (group_name) DO UPDATE SET auto_apply_override=EXCLUDED.auto_apply_override, updated_at=now()
	`, group, *v)
	return err
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