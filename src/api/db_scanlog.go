package main

import (
	"context"
	"encoding/json"
	"log"
)

func scanLog(ctx context.Context, hostID int64, level, msg string, data map[string]any) {
	if data == nil { data = map[string]any{} }
	b, _ := json.Marshal(data)
	if _, err := db.Exec(ctx, `INSERT INTO scan_logs (host_id, level, message, data) VALUES ($1,$2,$3,$4::jsonb)`,
		hostID, level, msg, string(b)); err != nil {
		log.Printf("scanlog insert failed: %v (msg=%s)", err, msg)
	}
}