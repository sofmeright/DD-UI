package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dd-ui/common"
	"dd-ui/database"
	"dd-ui/services"

	"github.com/go-chi/chi/v5"
)

// setupIacRoutes sets up all Infrastructure as Code (IAC) related routes
// This organizes the extensive IAC functionality from web.go into logical groups:
// - Host-scoped IAC endpoints
// - Group-scoped IAC endpoints  
// - Stack-scoped IAC endpoints (CRUD, files, deployment)
// - Batch operations and scanning
func SetupIacRoutes(router chi.Router) {
	router.Route("/iac", func(r chi.Router) {
		// Host-scoped IAC endpoints
		r.Route("/hosts/{hostname}", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				hostname := chi.URLParam(r, "hostname")
				common.DebugLog("Basic-IAC request for host: %s (using enhanced logic)", hostname)
				items, err := services.ListEnhancedIacStacksForHost(r.Context(), hostname)
				if err != nil {
					common.DebugLog("Basic-IAC (enhanced) failed for host %s: %v", hostname, err)
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				common.DebugLog("Basic-IAC (enhanced) returning %d stacks for host %s", len(items), hostname)
				for i, stack := range items {
					common.DebugLog("Response stack[%d]: %s has %d services, %d rendered_services, compose=%v", i, stack.Name, len(stack.Services), len(stack.RenderedServices), stack.Compose != "")
					if len(stack.RenderedServices) > 0 {
						common.DebugLog("  Stack %s using RENDERED services (decrypted):", stack.Name)
						for j, rs := range stack.RenderedServices {
							common.DebugLog("    rendered_service[%d]: image=%s", j, rs.Image)
						}
					} else {
						common.DebugLog("  Stack %s using raw services:", stack.Name)
						for j, s := range stack.Services {
							common.DebugLog("    service[%d]: image=%s", j, s.Image)
						}
					}
				}
				writeJSON(w, http.StatusOK, items)
			})
		})
		
		// Force IaC scan (local)
		r.Post("/scan", func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
			defer cancel()
			stacks, services, err := services.ScanIacLocal(ctx)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"status":   "ok",
				"stacks":   stacks,
				"services": services,
			})
		})

		// Create a new stack in the local IaC repo
		r.Post("/stacks", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				ScopeKind string `json:"scope_kind"`
				ScopeName string `json:"scope_name"`
				StackName string `json:"stack_name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			if body.ScopeKind == "" || body.ScopeName == "" || body.StackName == "" {
				http.Error(w, "scope_kind, scope_name, stack_name required", http.StatusBadRequest)
				return
			}

			root := strings.TrimSpace(common.Env(services.IacDefaultRootEnv, services.IacDefaultRoot))
			dirname := strings.TrimSpace(common.Env(services.IacDirNameEnv, services.IacDefaultDirName))
			rel := filepath.ToSlash(filepath.Join(dirname, body.ScopeName, body.StackName))
			full, err := services.JoinUnder(root, rel)
			if err != nil {
				http.Error(w, "invalid path", http.StatusBadRequest)
				return
			}
			if err := os.MkdirAll(full, 0o755); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			repoID, err := services.UpsertIacRepoLocal(r.Context(), root)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			id, err := services.UpsertIacStack(r.Context(), repoID, body.ScopeKind, body.ScopeName, body.StackName, rel, "", "compose", "always", "auto", true)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": id, "rel_path": rel})
		})

		// Get a single IaC stack (returns effective auto devops)
		r.Get("/stacks/{id}", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, _ := strconv.ParseInt(idStr, 10, 64)

			var row struct {
				ID         int64   `json:"id"`
				RepoID     int64   `json:"repo_id"`
				ScopeKind  string  `json:"scope_kind"`
				ScopeName  string  `json:"scope_name"`
				StackName  string  `json:"stack_name"`
				RelPath    string  `json:"rel_path"`
				IacEnabled bool    `json:"iac_enabled"`
				DeployKind string  `json:"deploy_kind"`
				SopsStatus string  `json:"sops_status"`
				Override   *bool   `json:"auto_apply_override"`
				UpdatedAt  string  `json:"updated_at"`
				Effective  bool    `json:"effective_auto_devops"`
			}
			var updatedAt time.Time
			err := common.DB.QueryRow(r.Context(), `
				SELECT id, repo_id, scope_kind::text, scope_name, stack_name, rel_path,
					iac_enabled, deploy_kind::text, sops_status::text, auto_apply_override, updated_at
				FROM iac_stacks
				WHERE id=$1
			`, id).Scan(
				&row.ID, &row.RepoID, &row.ScopeKind, &row.ScopeName, &row.StackName, &row.RelPath,
				&row.IacEnabled, &row.DeployKind, &row.SopsStatus, &row.Override, &updatedAt,
			)
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			row.UpdatedAt = updatedAt.Format(time.RFC3339)
			eff, _ := services.ShouldAutoApply(r.Context(), id)
			row.Effective = eff
			writeJSON(w, http.StatusOK, map[string]any{"stack": row})
		})

		// Patch IaC stack (no implicit override writes)
		r.Patch("/stacks/{id}", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, _ := strconv.ParseInt(idStr, 10, 64)

			var body struct {
				IacEnabled        *bool `json:"iac_enabled,omitempty"`
				AutoDevOps        *bool `json:"auto_devops,omitempty"`          // explicit override set/clear (when provided)
				AutoDevOpsInherit *bool `json:"auto_devops_inherit,omitempty"`  // when true, clear override (set NULL)
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}

			// Only update iac_enabled if caller asked
			if body.IacEnabled != nil {
				if _, err := common.DB.Exec(r.Context(), `UPDATE iac_stacks SET iac_enabled=$1 WHERE id=$2`, *body.IacEnabled, id); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}

			// Clear explicit override if requested
			if body.AutoDevOpsInherit != nil && *body.AutoDevOpsInherit {
				if _, err := common.DB.Exec(r.Context(), `UPDATE iac_stacks SET auto_apply_override=NULL WHERE id=$1`, id); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}

			// Set explicit override when provided
			if body.AutoDevOps != nil {
				override := "enable"
				if !*body.AutoDevOps {
					override = "disable"
				}
				if _, err := common.DB.Exec(r.Context(), `UPDATE iac_stacks SET auto_apply_override=$1 WHERE id=$2`, override, id); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}

			writeJSON(w, http.StatusOK, map[string]any{
				"status":                "ok",
			})
		})

		// Delete a stack (optionally delete files too)
		r.Delete("/stacks/{id}", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, _ := strconv.ParseInt(idStr, 10, 64)
			deleteFiles := r.URL.Query().Get("delete_files") == "1" || r.URL.Query().Get("delete_files") == "true"

			root, err := services.GetRepoRootForStack(r.Context(), id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var rel string
			_ = common.DB.QueryRow(r.Context(), `SELECT rel_path FROM iac_stacks WHERE id=$1`, id).Scan(&rel)

			if _, err := common.DB.Exec(r.Context(), `DELETE FROM iac_stacks WHERE id=$1`, id); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if deleteFiles && rel != "" {
				if full, err := services.JoinUnder(root, rel); err == nil {
					_ = os.RemoveAll(full) // best effort
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
		})

		// ===== IaC Editor APIs =====

		// List files tracked for a stack
		r.Get("/stacks/{id}/files", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, _ := strconv.ParseInt(idStr, 10, 64)
			items, err := services.ListFilesForStack(r.Context(), id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"files": items})
		})

		// Read file content for a stack file (with optional decrypt)
		r.Get("/stacks/{id}/file", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, _ := strconv.ParseInt(idStr, 10, 64)
			rel := strings.TrimSpace(r.URL.Query().Get("path"))
			if rel == "" {
				http.Error(w, "missing path", http.StatusBadRequest)
				return
			}
			root, err := services.GetRepoRootForStack(r.Context(), id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			full, err := services.JoinUnder(root, rel)
			if err != nil {
				http.Error(w, "invalid path", http.StatusBadRequest)
				return
			}
			decrypt := r.URL.Query().Get("decrypt") == "1" || r.URL.Query().Get("decrypt") == "true"

			var data []byte
			if decrypt {
				if !common.EnvBool("DD_UI_ALLOW_SOPS_DECRYPT", "false") {
					http.Error(w, "decrypt disabled on server", http.StatusForbidden)
					return
				}
				if strings.ToLower(r.Header.Get("X-Confirm-Reveal")) != "yes" {
					http.Error(w, "confirmation required", http.StatusForbidden)
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "sops", "-d", full)
				out, err := cmd.CombinedOutput()
				if err != nil {
					http.Error(w, "sops decrypt failed: "+string(out), http.StatusNotImplemented)
					return
				}
				data = out
			} else {
				data, err = os.ReadFile(full)
				if err != nil {
					http.Error(w, "file not found", http.StatusNotFound)
					return
				}
			}

			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(data)
		})

		// Create/update file content
		r.Post("/stacks/{id}/file", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, _ := strconv.ParseInt(idStr, 10, 64)
			var body struct {
				Path    string `json:"path"`
				Content string `json:"content"`
				Sops    bool   `json:"sops,omitempty"`
				Role    string `json:"role,omitempty"` // compose|env|script|other
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(body.Path) == "" {
				http.Error(w, "path required", http.StatusBadRequest)
				return
			}
			root, err := services.GetRepoRootForStack(r.Context(), id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			full, err := services.JoinUnder(root, body.Path)
			if err != nil {
				http.Error(w, "invalid path", http.StatusBadRequest)
				return
			}
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			data := []byte(body.Content)
			sz := len(data)
			sum := fmt.Sprintf("%x", sha256.Sum256(data))

			if body.Sops {
				if !common.EnvBool("DD_UI_ALLOW_SOPS_ENCRYPT", "false") {
					http.Error(w, "encrypt disabled on server", http.StatusForbidden)
					return
				}
				tmp := full + ".tmp"
				if err := os.WriteFile(tmp, data, 0o644); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				defer os.Remove(tmp)
				ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "sops", "-e", "-i", tmp)
				out, err := cmd.CombinedOutput()
				if err != nil {
					http.Error(w, "sops encrypt failed: "+string(out), http.StatusBadRequest)
					return
				}
				if err := os.Rename(tmp, full); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			} else {
				if err := os.WriteFile(full, data, 0o644); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "saved", "size": sz, "sha256": sum, "sops": body.Sops})
		})

		// Delete a file from a stack
		r.Delete("/stacks/{id}/file", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, _ := strconv.ParseInt(idStr, 10, 64)
			rel := strings.TrimSpace(r.URL.Query().Get("path"))
			if rel == "" {
				http.Error(w, "missing path", http.StatusBadRequest)
				return
			}
			root, err := services.GetRepoRootForStack(r.Context(), id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			full, err := services.JoinUnder(root, rel)
			if err != nil {
				http.Error(w, "invalid path", http.StatusBadRequest)
				return
			}
			_ = os.Remove(full) // best effort
			_ = services.DeleteIacFileRow(r.Context(), id, rel)
			writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
		})

		// Deploy endpoint
		// - Manual deploys: **default** (for UI). Pass ?auto=1 for background/auto callers.
		r.Post("/stacks/{id}/deploy", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil || id <= 0 {
				http.Error(w, "bad id", http.StatusBadRequest)
				return
			}

			// must have something to deploy
			ok, derr := services.StackHasContent(r.Context(), id)
			if derr != nil {
				http.Error(w, derr.Error(), http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, "stack has no files", http.StatusBadRequest)
				return
			}

			// Manual by default (UI-friendly). Auto callers pass ?auto=1 explicitly.
			manual := !services.IsTrueish(r.URL.Query().Get("auto"))

			// Gate only if NOT manual
			if !manual {
				allowed, aerr := services.ShouldAutoApply(r.Context(), id)
				if aerr != nil {
					http.Error(w, aerr.Error(), http.StatusBadRequest)
					return
				}
				if !allowed {
					writeJSON(w, http.StatusAccepted, map[string]any{
						"status":  "skipped",
						"reason":  "auto_devops_disabled",
						"stackID": id,
					})
					return
				}
			}

			// async deploy with timeout + logging; mark manual flag in context
			go func(stackID int64, manual bool) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				ctx = context.WithValue(ctx, services.CtxManualKey{}, manual)
				if err := services.DeployStack(ctx, stackID); err != nil {
					common.ErrorLog("deploy: stack %d failed: %v", stackID, err)
					return
				}
				common.InfoLog("deploy: stack %d ok", stackID)
			}(id, manual)

			writeJSON(w, http.StatusAccepted, map[string]any{
				"status":  "accepted",
				"stackID": id,
				"allowed": true,
			})
		})

		// Streaming deploy endpoint (compatible with existing frontend)
		r.Get("/stacks/{id}/deploy-stream", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil || id <= 0 {
				http.Error(w, "bad id", http.StatusBadRequest)
				return
			}

			// Check if stack has content
			ok, derr := services.StackHasContent(r.Context(), id)
			if derr != nil {
				http.Error(w, derr.Error(), http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, "stack has no deployable content", http.StatusBadRequest)
				return
			}

			// Set up Server-Sent Events
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			
			// Create event channel
			eventChan := make(chan map[string]interface{}, 100)
			
			// Start deployment in background
			ctx := context.WithValue(r.Context(), services.CtxManualKey{}, true)
			// Check for force parameter to bypass config unchanged checks
			if r.URL.Query().Get("force") == "true" {
				ctx = context.WithValue(ctx, services.CtxForceKey{}, true)
			}
			go func() {
				if err := services.DeployStackWithStream(ctx, id, eventChan); err != nil {
					common.ErrorLog("deploy-stream: stack %d failed: %v", id, err)
				}
			}()

			// Stream events to client
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming not supported", http.StatusInternalServerError)
				return
			}

			for event := range eventChan {
				eventJSON, _ := json.Marshal(event)
				fmt.Fprintf(w, "data: %s\n\n", eventJSON)
				flusher.Flush()

				// Break on completion
				if eventType, ok := event["type"].(string); ok && eventType == "complete" {
					break
				}
			}
		})

		// Check if configuration has changed endpoint
		r.Post("/stacks/{id}/deploy-check", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil || id <= 0 {
				http.Error(w, "bad id", http.StatusBadRequest)
				return
			}

			// Get the current configuration
			_, err = services.GetRepoRootForStack(r.Context(), id)
			if err != nil {
				http.Error(w, "failed to get repo root", http.StatusInternalServerError)
				return
			}
			var rel string
			_ = common.DB.QueryRow(r.Context(), `SELECT rel_path FROM iac_stacks WHERE id=$1`, id).Scan(&rel)
			if strings.TrimSpace(rel) == "" {
				http.Error(w, "stack has no rel_path", http.StatusBadRequest)
				return
			}

			// Stage (SOPS decrypts into tmpfs and is cleaned afterwards)
			_, stagedComposes, cleanup, derr := services.StageStackForCompose(r.Context(), id)
			if derr != nil {
				http.Error(w, fmt.Sprintf("failed to stage stack: %v", derr), http.StatusInternalServerError)
				return
			}
			defer func() {
				if cleanup != nil {
					cleanup()
				}
			}()

			if len(stagedComposes) == 0 {
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"config_unchanged": false,
				})
				return
			}

			var allComposeContent []byte
			for _, f := range stagedComposes {
				b, rerr := os.ReadFile(f)
				if rerr != nil {
					http.Error(w, fmt.Sprintf("failed to read compose file: %v", rerr), http.StatusInternalServerError)
					return
				}
				allComposeContent = append(allComposeContent, b...)
				allComposeContent = append(allComposeContent, '\n')
			}

			// Check if configuration has changed by comparing with latest deployment stamp
			latest, lerr := database.GetLatestDeploymentStamp(r.Context(), id)
			if lerr != nil {
				// No previous deployment, so config is new
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"config_unchanged": false,
				})
				return
			}

			// Calculate hash of current config
			currentHash := fmt.Sprintf("%x", sha256.Sum256(allComposeContent))
			
			// Compare with latest deployment hash
			configUnchanged := (latest.DeploymentHash == currentHash)

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"config_unchanged": configUnchanged,
			})
		})

		// Confirmation endpoint for deploying unchanged configuration
		r.Get("/stacks/{id}/deploy-force", func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil || id <= 0 {
				http.Error(w, "bad id", http.StatusBadRequest)
				return
			}

			// Set up Server-Sent Events
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("Access-Control-Allow-Origin", "*")

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}

			eventChan := make(chan map[string]interface{}, 100)

			// Start forced deployment in background
			ctx := context.WithValue(r.Context(), services.CtxManualKey{}, true)
			ctx = context.WithValue(ctx, services.CtxForceKey{}, true)
			go func() {
				if err := services.DeployStackWithStream(ctx, id, eventChan); err != nil {
					common.ErrorLog("deploy-force: stack %d failed: %v", id, err)
				}
			}()

			// Stream events to client
			for event := range eventChan {
				eventJSON, _ := json.Marshal(event)
				fmt.Fprintf(w, "data: %s\n\n", eventJSON)
				flusher.Flush()

				// Break on completion
				if eventType, ok := event["type"].(string); ok && eventType == "complete" {
					break
				}
			}
		})
	})

	// Scope-based deployment streaming (alternative endpoint)
	router.Get("/scopes/{scope}/stacks/{stackname}/deploy-stream", func(w http.ResponseWriter, r *http.Request) {
		scope := chi.URLParam(r, "scope")
		stackName := chi.URLParam(r, "stackname")

		// Find the stack ID
		var stackID int64
		err := common.DB.QueryRow(r.Context(), `
			SELECT id FROM iac_stacks 
			WHERE scope_name = $1 AND stack_name = $2
		`, scope, stackName).Scan(&stackID)
		if err != nil {
			http.Error(w, "Stack not found", http.StatusNotFound)
			return
		}

		// Check if stack has content
		ok, derr := services.StackHasContent(r.Context(), stackID)
		if derr != nil {
			http.Error(w, derr.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "stack has no deployable content", http.StatusBadRequest)
			return
		}

		// Set up Server-Sent Events
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		
		// Create event channel
		eventChan := make(chan map[string]interface{}, 100)
		
		// Start deployment in background
		ctx := context.WithValue(r.Context(), services.CtxManualKey{}, true)
		// Check for force parameter to bypass config unchanged checks
		if r.URL.Query().Get("force") == "true" {
			ctx = context.WithValue(ctx, services.CtxForceKey{}, true)
		}
		go func() {
			if err := services.DeployStackWithStream(ctx, stackID, eventChan); err != nil {
				common.ErrorLog("deploy-stream: stack %d failed: %v", stackID, err)
			}
		}()

		// Stream events to client
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		for event := range eventChan {
			eventJSON, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", eventJSON)
			flusher.Flush()

			// Break on completion
			if eventType, ok := event["type"].(string); ok && eventType == "complete" {
				break
			}
		}
	})
}