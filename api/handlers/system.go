// routes_system.go - System management routes (hosts, scanning, inventory, view tracking)
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// Debug endpoint for migration status
	router.Get("/debug/migrations", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		
		// Check migration version
		var currentVersion int
		err := common.DB.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
		if err != nil {
			http.Error(w, "Failed to get migration version: " + err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Check if groups table exists
		var groupsExists bool
		err = common.DB.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = 'public' 
				AND table_name = 'groups'
			)
		`).Scan(&groupsExists)
		if err != nil {
			http.Error(w, "Failed to check groups table: " + err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Check if host_groups table exists
		var hostGroupsExists bool
		err = common.DB.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = 'public' 
				AND table_name = 'host_groups'
			)
		`).Scan(&hostGroupsExists)
		if err != nil {
			http.Error(w, "Failed to check host_groups table: " + err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Get list of applied migrations
		rows, err := common.DB.Query(ctx, "SELECT version, applied_at FROM schema_migrations ORDER BY version")
		if err != nil {
			http.Error(w, "Failed to list migrations: " + err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		
		type Migration struct {
			Version   int       `json:"version"`
			AppliedAt time.Time `json:"applied_at"`
		}
		
		var migrations []Migration
		for rows.Next() {
			var m Migration
			if err := rows.Scan(&m.Version, &m.AppliedAt); err != nil {
				continue
			}
			migrations = append(migrations, m)
		}
		
		result := map[string]interface{}{
			"current_version": currentVersion,
			"groups_table_exists": groupsExists,
			"host_groups_table_exists": hostGroupsExists,
			"applied_migrations": migrations,
		}
		
		writeJSON(w, http.StatusOK, result)
	})

	// Host listing and management - uses inventory manager as source of truth
	router.Get("/hosts", func(w http.ResponseWriter, r *http.Request) {
		// Get hosts from inventory manager
		invMgr := services.GetInventoryManager()
		hosts, err := invMgr.GetHosts()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Query parameters
		owner := strings.TrimSpace(r.URL.Query().Get("owner"))
		tenant := strings.TrimSpace(r.URL.Query().Get("tenant"))
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		limit := clamp(parseIntDefault(r.URL.Query().Get("limit"), 200), 1, 1000)
		offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

		// Convert and filter hosts
		filtered := make([]map[string]interface{}, 0, len(hosts))
		for _, h := range hosts {
			// Apply filters
			if owner != "" && !strings.EqualFold(h.Owner, owner) {
				continue
			}
			if tenant != "" && !strings.EqualFold(h.Tenant, tenant) {
				continue
			}
			if q != "" {
				// Search in name, address, alt_name, description, and tags
				matched := false
				if strings.Contains(strings.ToLower(h.Name), q) ||
					strings.Contains(strings.ToLower(h.Addr), q) ||
					strings.Contains(strings.ToLower(h.AltName), q) ||
					strings.Contains(strings.ToLower(h.Description), q) {
					matched = true
				}
				// Search in tags
				for _, tag := range h.Tags {
					if strings.Contains(strings.ToLower(tag), q) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			
			// Convert to API format
			filtered = append(filtered, map[string]interface{}{
				"name":          h.Name,
				"addr":          h.Addr,
				"vars":          h.Vars,
				"groups":        h.Groups,
				"tags":          h.Tags,
				"description":   h.Description,
				"alt_name":      h.AltName,
				"tenant":        h.Tenant,
				"allowed_users": h.AllowedUsers,
				"owner":         h.Owner,
				"env":           h.Env,
				"labels":        map[string]string{}, // For compatibility
			})
		}
		
		// Apply pagination
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

	// Host CRUD operations via IaC (inventory file management)
	router.Post("/iac/hosts", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name         string            `json:"name"`
			Addr         string            `json:"addr"` // ansible_host
			Description  string            `json:"description"`
			AltName      string            `json:"alt_name"`
			Tenant       string            `json:"tenant"`
			Owner        string            `json:"owner"`
			Tags         []string          `json:"tags"`
			AllowedUsers []string          `json:"allowed_users"`
			Env          map[string]string `json:"env"`
		}
		
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		
		// Validate required fields
		if req.Name == "" || req.Addr == "" {
			http.Error(w, "Host name and addr (IP/FQDN) are required", http.StatusBadRequest)
			return
		}
		
		// Build metadata
		metadata := services.HostMetadata{
			Tags:         req.Tags,
			Description:  req.Description,
			AltName:      req.AltName,
			Tenant:       req.Tenant,
			AllowedUsers: req.AllowedUsers,
			Owner:        req.Owner,
			Env:          req.Env,
		}
		
		// Create host
		invMgr := services.GetInventoryManager()
		if err := invMgr.CreateHost(req.Name, req.Addr, metadata); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				http.Error(w, err.Error(), http.StatusConflict)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		
		// Return success
		writeJSON(w, http.StatusCreated, map[string]any{
			"message": fmt.Sprintf("Host %s created successfully", req.Name),
			"name":    req.Name,
		})
	})
	
	router.Put("/iac/hosts/{name}", func(w http.ResponseWriter, r *http.Request) {
		hostName := chi.URLParam(r, "name")
		
		var req struct {
			Addr         string            `json:"addr"` // ansible_host
			Description  string            `json:"description"`
			AltName      string            `json:"alt_name"`
			Tenant       string            `json:"tenant"`
			Owner        string            `json:"owner"`
			Tags         []string          `json:"tags"`
			AllowedUsers []string          `json:"allowed_users"`
			Env          map[string]string `json:"env"`
		}
		
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		
		// Build metadata
		metadata := services.HostMetadata{
			Tags:         req.Tags,
			Description:  req.Description,
			AltName:      req.AltName,
			Tenant:       req.Tenant,
			AllowedUsers: req.AllowedUsers,
			Owner:        req.Owner,
			Env:          req.Env,
		}
		
		// Update host
		invMgr := services.GetInventoryManager()
		if err := invMgr.UpdateHost(hostName, req.Addr, metadata); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		
		// Return success
		writeJSON(w, http.StatusOK, map[string]any{
			"message": fmt.Sprintf("Host %s updated successfully", hostName),
			"name":    hostName,
		})
	})
	
	router.Delete("/iac/hosts/{name}", func(w http.ResponseWriter, r *http.Request) {
		hostName := chi.URLParam(r, "name")
		
		// Delete host
		invMgr := services.GetInventoryManager()
		if err := invMgr.DeleteHost(hostName); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		
		// Return success
		writeJSON(w, http.StatusOK, map[string]any{
			"message": fmt.Sprintf("Host %s deleted successfully", hostName),
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

		// Get hosts from inventory
		invMgr := services.GetInventoryManager()
		invHosts, err := invMgr.GetHosts()
		if err != nil {
			http.Error(w, "Failed to get hosts from inventory: "+err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Convert inventory hosts to database format for compatibility
		// TODO: Eventually update services to use inventory hosts directly
		hostRows := make([]database.HostRow, 0, len(invHosts))
		for _, h := range invHosts {
			varsMap := make(map[string]string)
			for k, v := range h.Vars {
				if s, ok := v.(string); ok {
					varsMap[k] = s
				}
			}
			hostRows = append(hostRows, database.HostRow{
				Name:   h.Name,
				Addr:   h.Addr,
				Vars:   varsMap,
				Groups: h.Groups,
				Owner:  h.Owner,
				Labels: map[string]string{},
			})
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