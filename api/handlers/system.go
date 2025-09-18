// routes_system.go - System management routes (hosts, scanning, inventory, view tracking)
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dd-ui/common"
	"dd-ui/database"
	"dd-ui/services"
	"github.com/go-chi/chi/v5"
)

// Helper functions
func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func parseIntDefault(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func parseDurationDefault(s string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false) // Don't escape HTML characters like & in YAML anchors
	encoder.Encode(data)
}

// ViewBoostTracker for performance optimization
type ViewBoostTracker struct {
	views map[string]bool
}

var viewBoostTracker = &ViewBoostTracker{
	views: make(map[string]bool),
}

func (v *ViewBoostTracker) AddView(hostName string) {
	v.views[hostName] = true
}

func (v *ViewBoostTracker) RemoveView(hostName string) {
	delete(v.views, hostName)
}

// setupSystemRoutes configures all system management endpoints
func SetupSystemRoutes(router chi.Router) {
	// Host listing and management
	router.Get("/hosts", func(w http.ResponseWriter, r *http.Request) {
		items, err := database.ListHosts(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		owner := strings.TrimSpace(r.URL.Query().Get("owner"))
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		limit := clamp(parseIntDefault(r.URL.Query().Get("limit"), 200), 1, 1000)
		offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

		filtered := make([]database.HostRow, 0, len(items))
		for _, h := range items {
			if owner != "" && !strings.EqualFold(h.Owner, owner) {
				continue
			}
			if q != "" {
				if !strings.Contains(strings.ToLower(h.Name), q) &&
					!strings.Contains(strings.ToLower(h.Addr), q) {
					continue
				}
			}
			filtered = append(filtered, h)
		}
		lo := offset
		if lo > len(filtered) {
			lo = len(filtered)
		}
		hi := lo + limit
		if hi > len(filtered) {
			hi = len(filtered)
		}
		page := filtered[lo:hi]

		writeJSON(w, http.StatusOK, map[string]any{
			"items":  page,
			"total":  len(filtered),
			"limit":  limit,
			"offset": offset,
		})
	})

	// Host scanning operations
	router.Post("/scan/hosts/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		to := parseDurationDefault(r.URL.Query().Get("timeout"), 45*time.Second)
		ctx, cancel := context.WithTimeout(r.Context(), to)
		defer cancel()

		n, err := services.ScanHostContainers(ctx, name)
		if err != nil {
			if errors.Is(err, services.ErrSkipScan) {
				writeJSON(w, http.StatusOK, map[string]any{
					"host":   name,
					"saved":  0,
					"status": "skipped",
				})
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"host":   name,
			"saved":  n,
			"status": "ok",
		})
	})

	// Global scanning operations
	router.Post("/scan/global", func(w http.ResponseWriter, r *http.Request) {
		// IaC scan (non-fatal)
		if _, _, err := services.ScanIacLocal(r.Context()); err != nil {
			common.ErrorLog("iac: sync scan failed: %v", err)
		}

		hostRows, err := database.ListHosts(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		perHostTO := parseDurationDefault(r.URL.Query().Get("timeout"), 30*time.Second)

		type result struct {
			Host    string `json:"host"`
			Saved   int    `json:"saved,omitempty"`
			Skipped bool   `json:"skipped,omitempty"`
			Reason  string `json:"reason,omitempty"`
			Err     string `json:"error,omitempty"`
		}

		var (
			results []result
			total   int
			scanned int
			skipped int
			failed  int
		)

		for _, h := range hostRows {
			url, _ := services.DockerURLFor(h)
			if services.IsUnixSock(url) && !services.LocalHostAllowed(h) {
				results = append(results, result{
					Host:    h.Name,
					Skipped: true,
					Reason:  "local docker.sock only allowed for the designated local host",
				})
				skipped++
				continue
			}

			ctx, cancel := context.WithTimeout(r.Context(), perHostTO)
			n, err := services.ScanHostContainers(ctx, h.Name)
			cancel()

			if err != nil {
				if errors.Is(err, services.ErrSkipScan) {
					results = append(results, result{
						Host:    h.Name,
						Skipped: true,
						Reason:  err.Error(),
					})
					skipped++
					continue
				}
				results = append(results, result{
					Host: h.Name,
					Err:  err.Error(),
				})
				failed++
				continue
			}
			results = append(results, result{
				Host:  h.Name,
				Saved: n,
			})
			scanned++
			total += n
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"total":    total,
			"scanned":  scanned,
			"skipped":  skipped,
			"failed":   failed,
			"results":  results,
		})
	})

	// Inventory management
	router.Post("/inventory/reload", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Path string `json:"path"` }
		_ = json.NewDecoder(r.Body).Decode(&body)

		var err error
		if strings.TrimSpace(body.Path) != "" {
			err = services.ReloadInventoryWithPath(body.Path)
		} else {
			err = services.ReloadInventory()
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "reloaded"})
	})

	// View tracking endpoints for performance optimization
	router.Post("/view/hosts/{name}/start", func(w http.ResponseWriter, r *http.Request) {
		hostName := chi.URLParam(r, "name")
		viewBoostTracker.AddView(hostName)
		writeJSON(w, http.StatusOK, map[string]any{"status": "view_started", "host": hostName})
	})
	
	router.Post("/view/hosts/{name}/end", func(w http.ResponseWriter, r *http.Request) {
		hostName := chi.URLParam(r, "name")
		viewBoostTracker.RemoveView(hostName)
		writeJSON(w, http.StatusOK, map[string]any{"status": "view_ended", "host": hostName})
	})
}