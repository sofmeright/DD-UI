// src/api/db_scanlog.go
package database

import (
	"context"
	"encoding/json"
	
	"dd-ui/common"
)

// ScanLog logs scan operations for a host
func ScanLog(ctx context.Context, hostID int64, level, msg string, data map[string]any) {
	if data == nil { data = map[string]any{} }
	b, _ := json.Marshal(data)
	if _, err := common.DB.Exec(ctx, `INSERT INTO scan_logs (host_id, level, message, data) VALUES ($1,$2,$3,$4::jsonb)`,
		hostID, level, msg, string(b)); err != nil {
		common.ErrorLog("scanlog insert failed: %v (msg=%s)", err, msg)
	}
}