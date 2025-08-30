package main

import (
	"context"
	"encoding/json"
)

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
}

func UpsertHosts(ctx context.Context, items []Host) error {
	for _, h := range items {
		// prefer explicit Host.Owner; fallback to var "owner"; then env default
		owner := h.Owner
		if owner == "" && h.Vars != nil {
			if v, ok := h.Vars["owner"]; ok {
				owner = v
			}
		}
		if owner == "" {
			owner = env("DDUI_DEFAULT_OWNER", "")
		}

		varsJSON, _ := json.Marshal(h.Vars)
		labelsJSON, _ := json.Marshal(map[string]string{}) // reserved for later

		_, err := db.Exec(ctx, `
			INSERT INTO hosts (name, addr, vars, "groups", labels, owner, updated_at)
			VALUES ($1, $2, $3::jsonb, $4, $5::jsonb, $6, now())
			ON CONFLICT (name) DO UPDATE SET
				addr      = EXCLUDED.addr,
				vars      = EXCLUDED.vars,
				"groups"  = EXCLUDED."groups",
				labels    = EXCLUDED.labels,
				owner     = EXCLUDED.owner,
				updated_at = now()
		`, h.Name, h.Addr, string(varsJSON), h.Groups, string(labelsJSON), owner)
		if err != nil {
			return err
		}
	}
	return nil
}

func ListHosts(ctx context.Context) ([]HostRow, error) {
	rows, err := db.Query(ctx, `SELECT id, name, addr, vars, "groups", labels, owner FROM hosts ORDER BY name`)
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
			labelsB []byte
			owner  string
		)
		if err := rows.Scan(&id, &name, &addr, &varsB, &groups, &labelsB, &owner); err != nil {
			return nil, err
		}
		m := map[string]string{}
		_ = json.Unmarshal(varsB, &m)

		l := map[string]string{}
		_ = json.Unmarshal(labelsB, &l)

		h := HostRow{
			ID:     id,
			Name:   name,
			Addr:   deref(addr),
			Vars:   m,
			Groups: groups,
			Labels: l,
			Owner:  owner,
		}
		h.normalize()
		out = append(out, h)
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
