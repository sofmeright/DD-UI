// src/api/db_hosts.go
package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// ----- DB rows & helpers -----

type HostRow struct {
	ID     int64             `json:"id"`
	Name   string            `json:"name"`
	Addr   string            `json:"addr"`
	Vars   map[string]string `json:"vars"`
	Groups []string          `json:"groups"`
	Labels map[string]string `json:"labels"`
	Owner  string            `json:"owner"`
}

func (h *HostRow) normalize() {
	if h.Groups == nil {
		h.Groups = []string{}
	}
	if h.Labels == nil {
		h.Labels = map[string]string{}
	}
	if strings.TrimSpace(h.Owner) == "" {
		h.Owner = "unassigned"
	}
}

// ----- Inventory import -----

// ImportInventoryToDB is called by inventory.go after parsing.
// Keep this name/signature so other files can link against it.
func ImportInventoryToDB(ctx context.Context, hs []Host) error {
	return UpsertHosts(ctx, hs)
}

// Upsert hosts parsed from inventory into DB.
func UpsertHosts(ctx context.Context, items []Host) error {
	for _, h := range items {
		if h.Vars == nil {
			h.Vars = map[string]string{}
		}
		g := h.Groups
		if g == nil {
			g = []string{}
		}

		owner := strings.TrimSpace(h.Owner)
		if owner == "" {
			if def := env("DDUI_DEFAULT_OWNER", ""); def != "" {
				owner = def
			} else {
				owner = "unassigned"
			}
		}

		varsJSON, _ := json.Marshal(h.Vars)

		// NOTE: labels is set to '{}'::jsonb directly (no column reference in VALUES).
		if _, err := db.Exec(ctx, `
			INSERT INTO hosts (name, addr, vars, "groups", labels, owner, updated_at)
			VALUES ($1, $2, $3::jsonb, $4, '{}'::jsonb, COALESCE(NULLIF($5,''), 'unassigned'), now())
			ON CONFLICT (name) DO UPDATE
			SET addr       = EXCLUDED.addr,
			    vars       = EXCLUDED.vars,
			    "groups"   = EXCLUDED."groups",
			    owner      = COALESCE(NULLIF(EXCLUDED.owner,''), hosts.owner, 'unassigned'),
			    updated_at = now()
		`, h.Name, h.Addr, string(varsJSON), g, owner); err != nil {
			return err
		}
	}
	return nil
}

// ----- Queries used by web.go / scan_docker.go -----

func ListHosts(ctx context.Context) ([]HostRow, error) {
	rows, err := db.Query(ctx, `
		SELECT
			id,
			name,
			addr,
			COALESCE(vars, '{}'::jsonb),
			COALESCE("groups", '{}'::text[]),
			COALESCE(labels, '{}'::jsonb),
			COALESCE(owner, 'unassigned')
		FROM hosts
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HostRow
	for rows.Next() {
		var (
			id      int64
			name    string
			addrPtr *string
			varsB   []byte
			groups  []string
			labelsB []byte
			owner   string
		)
		if err := rows.Scan(&id, &name, &addrPtr, &varsB, &groups, &labelsB, &owner); err != nil {
			return nil, err
		}

		mVars := map[string]string{}
		_ = json.Unmarshal(varsB, &mVars)

		mLabels := map[string]string{}
		_ = json.Unmarshal(labelsB, &mLabels)

		h := HostRow{
			ID:     id,
			Name:   name,
			Addr:   deref(addrPtr),
			Vars:   mVars,
			Groups: groups,
			Labels: mLabels,
			Owner:  owner,
		}
		h.normalize()
		out = append(out, h)
	}
	return out, rows.Err()
}

// light ping used elsewhere
func DBHealth() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return db.Ping(ctx)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
