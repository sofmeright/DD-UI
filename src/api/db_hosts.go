// src/api/db_hosts.go
package main

import (
	"context"
	"encoding/json"
	"time"
)

type HostRow struct {
	ID     int64             `json:"id"`
	Name   string            `json:"name"`
	Addr   string            `json:"addr"`
	Vars   map[string]string `json:"vars"`
	Groups []string          `json:"groups"`
}

func UpsertHosts(ctx context.Context, items []Host) error {
	for _, h := range items {
		// normalize so we never send NULL to NOT NULL columns
		if h.Vars == nil {
			h.Vars = map[string]string{}
		}
		if h.Groups == nil {
			h.Groups = []string{}
		}

		varsJSON, _ := json.Marshal(h.Vars)

		if _, err := db.Exec(ctx, `
			INSERT INTO hosts (name, addr, vars, "groups", updated_at)
			VALUES ($1, $2, $3::jsonb, $4, now())
			ON CONFLICT (name) DO UPDATE
			SET addr      = EXCLUDED.addr,
			    vars      = EXCLUDED.vars,
			    "groups"  = EXCLUDED."groups",
			    updated_at = now()
		`, h.Name, h.Addr, string(varsJSON), h.Groups); err != nil {
			return err
		}
	}
	return nil
}

func ListHosts(ctx context.Context) ([]HostRow, error) {
	rows, err := db.Query(ctx, `SELECT id, name, addr, vars, "groups" FROM hosts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HostRow
	for rows.Next() {
		var (
			id     int64
			name   string
			addr   *string
			varsB  []byte
			groups []string
		)
		if err := rows.Scan(&id, &name, &addr, &varsB, &groups); err != nil {
			return nil, err
		}
		m := map[string]string{}
		_ = json.Unmarshal(varsB, &m)
		out = append(out, HostRow{
			ID:     id,
			Name:   name,
			Addr:   deref(addr),
			Vars:   m,
			Groups: groups,
		})
	}
	return out, rows.Err()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Called by the inventory loader after parsing
func ImportInventoryToDB(ctx context.Context, hs []Host) error {
	return UpsertHosts(ctx, hs)
}

// Optional keepalive; not required
func DBHealth() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return db.Ping(ctx)
}
