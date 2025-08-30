// src/api/db_hosts.go
package main

import (
	"context"
	"encoding/json"
	"strings"
)

func UpsertHosts(ctx context.Context, items []Host) error {
	for _, h := range items {
		// ensure non-nil/empty
		if h.Vars == nil {
			h.Vars = map[string]string{}
		}
		g := h.Groups
		if g == nil {
			g = []string{}
		}

		// owner fallback -> env or "unassigned"
		owner := strings.TrimSpace(h.Owner)
		if owner == "" {
			if def := env("DDUI_DEFAULT_OWNER", ""); def != "" {
				owner = def
			} else {
				owner = "unassigned"
			}
		}

		varsJSON, _ := json.Marshal(h.Vars)

		// double guard at SQL: never let NULL/"" through
		_, err := db.Exec(ctx, `
			INSERT INTO hosts (name, addr, vars, "groups", owner, updated_at)
			VALUES ($1, $2, $3::jsonb, $4, COALESCE(NULLIF($5,''), 'unassigned'), now())
			ON CONFLICT (name) DO UPDATE
			SET addr       = EXCLUDED.addr,
			    vars       = EXCLUDED.vars,
			    "groups"   = EXCLUDED."groups",
			    owner      = COALESCE(NULLIF(EXCLUDED.owner,''), hosts.owner, 'unassigned'),
			    updated_at = now()
		`, h.Name, h.Addr, string(varsJSON), g, owner)
		if err != nil {
			return err
		}
	}
	return nil
}
