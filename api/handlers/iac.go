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
	"strings"
	"time"

	"dd-ui/common"
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
				writeJSON(w, http.StatusOK, map[string]any{"stacks": items})
			})
			
			// Create stack for this host
			r.Post("/stacks", func(w http.ResponseWriter, r *http.Request) {
				hostname := chi.URLParam(r, "hostname")
				
				var body struct {
					StackName  string `json:"stack_name"`
					IacEnabled bool   `json:"iac_enabled"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				
				// Create the stack with host scope
				ctx := r.Context()
				id, err := services.CreateIacStack(ctx, "host", hostname, body.StackName, body.IacEnabled)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				
				writeJSON(w, http.StatusOK, map[string]any{"id": id, "created": true})
			})
			
			// Stack-specific hierarchical endpoints
			r.Route("/stacks/{stackname}", func(r chi.Router) {
				// Get stack details
				r.Get("/", func(w http.ResponseWriter, r *http.Request) {
					hostname := chi.URLParam(r, "hostname")
					stackname := chi.URLParam(r, "stackname")
					
					stackID, err := services.GetStackIDByHostAndName(r.Context(), hostname, stackname)
					if err != nil {
						http.Error(w, "Stack not found", http.StatusNotFound)
						return
					}
					
					// Get stack details
					var stack services.IacStackRow
					var autoApplyOverride *bool
					var updatedAt time.Time
					err = common.DB.QueryRow(r.Context(), `
						SELECT id, repo_id, scope_kind, scope_name, name, rel_path, iac_enabled, 
						       deploy_kind, auto_apply_override, updated_at
						FROM iac_stacks WHERE id=$1
					`, stackID).Scan(&stack.ID, &stack.RepoID, &stack.ScopeKind, &stack.ScopeName, 
						&stack.StackName, &stack.RelPath, &stack.IacEnabled, &stack.DeployKind, 
						&autoApplyOverride, &updatedAt)
					if err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					
					// Add extra fields to response
					response := map[string]any{
						"stack": stack,
						"auto_apply_override": autoApplyOverride,
						"updated_at": updatedAt.Format(time.RFC3339),
					}
					writeJSON(w, http.StatusOK, response)
				})
				
				// Update stack settings
				r.Patch("/", func(w http.ResponseWriter, r *http.Request) {
					hostname := chi.URLParam(r, "hostname")
					stackname := chi.URLParam(r, "stackname")
					
					stackID, err := services.GetStackIDByHostAndName(r.Context(), hostname, stackname)
					if err != nil {
						http.Error(w, "Stack not found", http.StatusNotFound)
						return
					}
					
					var body struct {
						IacEnabled        *bool   `json:"iac_enabled,omitempty"`
						AutoApplyOverride *bool   `json:"auto_apply_override,omitempty"`
						DeployKind        *string `json:"deploy_kind,omitempty"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, "bad json", http.StatusBadRequest)
						return
					}
					
					// Update fields if provided
					if body.IacEnabled != nil {
						_, err = common.DB.Exec(r.Context(), 
							`UPDATE iac_stacks SET iac_enabled=$1, updated_at=now() WHERE id=$2`,
							*body.IacEnabled, stackID)
						if err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
					}
					
					if body.AutoApplyOverride != nil {
						_, err = common.DB.Exec(r.Context(),
							`UPDATE iac_stacks SET auto_apply_override=$1, updated_at=now() WHERE id=$2`,
							*body.AutoApplyOverride, stackID)
						if err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
					}
					
					if body.DeployKind != nil {
						_, err = common.DB.Exec(r.Context(),
							`UPDATE iac_stacks SET deploy_kind=$1, updated_at=now() WHERE id=$2`,
							*body.DeployKind, stackID)
						if err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
					}
					
					writeJSON(w, http.StatusOK, map[string]any{"updated": true})
				})
				
				// Delete stack
				r.Delete("/", func(w http.ResponseWriter, r *http.Request) {
					hostname := chi.URLParam(r, "hostname")
					stackname := chi.URLParam(r, "stackname")
					
					stackID, err := services.GetStackIDByHostAndName(r.Context(), hostname, stackname)
					if err != nil {
						http.Error(w, "Stack not found", http.StatusNotFound)
						return
					}
					
					// Delete the stack
					if _, err := common.DB.Exec(r.Context(), `DELETE FROM iac_stacks WHERE id=$1`, stackID); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					
					writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
				})
				
				// Get stack files
				r.Get("/files", func(w http.ResponseWriter, r *http.Request) {
					hostname := chi.URLParam(r, "hostname")
					stackname := chi.URLParam(r, "stackname")
					
					// Find the stack ID from host and stack name
					stackID, err := services.GetStackIDByHostAndName(r.Context(), hostname, stackname)
					if err != nil {
						http.Error(w, "Stack not found", http.StatusNotFound)
						return
					}
					
					items, err := services.ListFilesForStack(r.Context(), stackID)
					if err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					writeJSON(w, http.StatusOK, map[string]any{"files": items})
				})
				
				// Get file content
				r.Get("/file", func(w http.ResponseWriter, r *http.Request) {
					hostname := chi.URLParam(r, "hostname")
					stackname := chi.URLParam(r, "stackname")
					rel := strings.TrimSpace(r.URL.Query().Get("path"))
					if rel == "" {
						http.Error(w, "missing path", http.StatusBadRequest)
						return
					}
					
					stackID, err := services.GetStackIDByHostAndName(r.Context(), hostname, stackname)
					if err != nil {
						http.Error(w, "Stack not found", http.StatusNotFound)
						return
					}
					
					// Get the file content using the existing logic
					root, err := services.GetRepoRootForStack(r.Context(), stackID)
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
						// This check ONLY gates the UI reveal functionality, NOT deployments
						// Deployments always decrypt SOPS files if keys are available (see deploy_sops.go)
						if !common.EnvBool("DD_UI_ALLOW_SOPS_DECRYPT", "false") {
							http.Error(w, "decrypt disabled on server", http.StatusForbidden)
							return
						}
						if strings.ToLower(r.Header.Get("X-Confirm-Reveal")) != "yes" {
							http.Error(w, "confirmation required", http.StatusForbidden)
							return
						}
						
						// For .env files, reconstruct with preserved comments
						if strings.HasSuffix(strings.ToLower(rel), ".env") {
							// Decrypt the file
							ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
							defer cancel()
							
							// Use dotenv input/output types for SOPS
							cmd := exec.CommandContext(ctx, "sops", "-d", "--input-type", "dotenv", "--output-type", "dotenv", full)
							out, err := cmd.CombinedOutput()
							if err != nil {
								// Fallback to normal decryption if dotenv type fails
								cmd = exec.CommandContext(ctx, "sops", "-d", full)
								out, err = cmd.CombinedOutput()
								if err != nil {
									http.Error(w, "sops decrypt failed: "+string(out), http.StatusNotImplemented)
									return
								}
							}
							
							// Try to load and apply preserved comments
							commentsFile := full + ".comments.json"
							if commentData, err := os.ReadFile(commentsFile); err == nil {
								var comments services.DotenvComments
								if err := json.Unmarshal(commentData, &comments); err == nil && len(comments.Comments) > 0 {
									// Reconstruct with comments
									data = []byte(services.ReconstructDotenvWithComments(string(out), comments))
								} else {
									data = out
								}
							} else {
								data = out
							}
						} else {
							// Non-.env files: decrypt with proper type detection
							ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
							defer cancel()
							
							// Detect file type for SOPS decryption
							var cmd *exec.Cmd
							lowerPath := strings.ToLower(rel)
							if strings.HasSuffix(lowerPath, ".yaml") || strings.HasSuffix(lowerPath, ".yml") {
								// Explicitly set YAML type for proper decryption
								cmd = exec.CommandContext(ctx, "sops", "-d", "--input-type", "yaml", "--output-type", "yaml", full)
							} else if strings.HasSuffix(lowerPath, ".json") {
								// Explicitly set JSON type
								cmd = exec.CommandContext(ctx, "sops", "-d", "--input-type", "json", "--output-type", "json", full)
							} else {
								// Let SOPS auto-detect for other file types
								cmd = exec.CommandContext(ctx, "sops", "-d", full)
							}
							
							out, err := cmd.CombinedOutput()
							if err != nil {
								http.Error(w, "sops decrypt failed: "+string(out), http.StatusNotImplemented)
								return
							}
							data = out
						}
					} else {
						// Read file without decryption
						var err error
						data, err = os.ReadFile(full)
						if err != nil {
							if os.IsNotExist(err) {
								http.Error(w, "file not found", http.StatusNotFound)
								return
							}
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
					}
					
					// Return raw content, not JSON
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					w.Header().Set("Cache-Control", "no-store")
					_, _ = w.Write(data)
				})
				
				// Save/update file - use existing logic with resolved stack ID
				r.Post("/file", func(w http.ResponseWriter, r *http.Request) {
					hostname := chi.URLParam(r, "hostname")
					stackname := chi.URLParam(r, "stackname")
					
					// Get or create stack ID
					id, err := services.GetStackIDByHostAndName(r.Context(), hostname, stackname)
					if err != nil {
						// Stack doesn't exist, create it
						id, err = services.CreateIacStack(r.Context(), "host", hostname, stackname, false)
						if err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
					}
					
					// Now use the same logic as /stacks/{id}/file endpoint
					var body struct {
						Path    string `json:"path"`
						Content string `json:"content"`
						Sops    bool   `json:"sops,omitempty"`
						Role    string `json:"role,omitempty"`
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

					// Auto-detect role from filename if not provided
					if body.Role == "" {
						base := strings.ToLower(filepath.Base(body.Path))
						if strings.Contains(base, "compose") && (strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")) {
							body.Role = "compose"
						} else if strings.HasSuffix(base, ".env") {
							body.Role = "env"
						} else if strings.HasSuffix(base, ".sh") {
							body.Role = "script"
						} else {
							body.Role = "other"
						}
					}

					data := []byte(body.Content)
					sz := len(data)
					sum := fmt.Sprintf("%x", sha256.Sum256(data))
					
					// Debug: Check for YAML anchors
					if strings.Contains(body.Content, "&") || strings.Contains(body.Content, "*") {
						common.DebugLog("File save: Found YAML anchors in %s", body.Path)
					}

					// Handle SOPS encryption with comment preservation for .env files
					if body.Sops {
						// No need to check for permission - encryption should always be allowed
						// The user explicitly requested it and it's necessary for security
						
						// For .env files, preserve comments
						if strings.HasSuffix(strings.ToLower(body.Path), ".env") {
							cleanContent, comments := services.ParseDotenvWithComments(body.Content)
							
							// Save comments to .comments.json file
							if len(comments.Comments) > 0 {
								commentsFile := full + ".comments.json"
								commentsJSON, _ := json.MarshalIndent(comments, "", "  ")
								if err := os.WriteFile(commentsFile, commentsJSON, 0o644); err != nil {
									common.InfoLog("Failed to save comments file: %v", err)
								}
							}
							
							// Encrypt the cleaned content
							tmp := full + ".tmp"
							if err := os.WriteFile(tmp, []byte(cleanContent), 0o644); err != nil {
								http.Error(w, err.Error(), http.StatusBadRequest)
								return
							}
							defer os.Remove(tmp)
							
							ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
							defer cancel()
							cmd := exec.CommandContext(ctx, "sops", "-e", "-i", "--input-type", "dotenv", "--output-type", "dotenv", tmp)
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
							// Non-.env files: detect file type and encrypt accordingly
							tmp := full + ".tmp"
							if err := os.WriteFile(tmp, data, 0o644); err != nil {
								http.Error(w, err.Error(), http.StatusBadRequest)
								return
							}
							defer os.Remove(tmp)
							ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
							defer cancel()
							
							// Detect file type for SOPS
							var cmd *exec.Cmd
							lowerPath := strings.ToLower(body.Path)
							if strings.HasSuffix(lowerPath, ".yaml") || strings.HasSuffix(lowerPath, ".yml") {
								// Explicitly set YAML type to prevent SOPS from using JSON format
								cmd = exec.CommandContext(ctx, "sops", "-e", "-i", "--input-type", "yaml", "--output-type", "yaml", tmp)
							} else if strings.HasSuffix(lowerPath, ".json") {
								// Explicitly set JSON type
								cmd = exec.CommandContext(ctx, "sops", "-e", "-i", "--input-type", "json", "--output-type", "json", tmp)
							} else {
								// Let SOPS auto-detect for other file types
								cmd = exec.CommandContext(ctx, "sops", "-e", "-i", tmp)
							}
							
							out, err := cmd.CombinedOutput()
							if err != nil {
								http.Error(w, "sops encrypt failed: "+string(out), http.StatusBadRequest)
								return
							}
							if err := os.Rename(tmp, full); err != nil {
								http.Error(w, err.Error(), http.StatusBadRequest)
								return
							}
						}
					} else {
						// No encryption: write directly
						if err := os.WriteFile(full, data, 0o644); err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						
						// Clean up any old comment files if saving without SOPS
						if strings.HasSuffix(strings.ToLower(body.Path), ".env") {
							commentsFile := full + ".comments.json"
							_ = os.Remove(commentsFile)
						}
					}
					
					// Update database with file metadata
					if err := services.UpsertIacFile(r.Context(), id, body.Role, body.Path, body.Sops, sum, int64(sz)); err != nil {
						common.InfoLog("Failed to update database for file %s: %v", body.Path, err)
					}
					
					writeJSON(w, http.StatusOK, map[string]any{"status": "saved", "size": sz, "sha256": sum, "sops": body.Sops})
				})
				
				// Delete file
				r.Delete("/file", func(w http.ResponseWriter, r *http.Request) {
					hostname := chi.URLParam(r, "hostname")
					stackname := chi.URLParam(r, "stackname")
					rel := strings.TrimSpace(r.URL.Query().Get("path"))
					if rel == "" {
						http.Error(w, "missing path", http.StatusBadRequest)
						return
					}
					
					stackID, err := services.GetStackIDByHostAndName(r.Context(), hostname, stackname)
					if err != nil {
						http.Error(w, "Stack not found", http.StatusNotFound)
						return
					}
					
					if err := services.DeleteIacFileRow(r.Context(), stackID, rel); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					
					root, err := services.GetRepoRootForStack(r.Context(), stackID)
					if err == nil {
						fullPath := filepath.Join(root, rel)
						os.Remove(fullPath)
					}
					
					writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
				})
				
				// Deploy endpoint (non-streaming)
				r.Post("/deploy", func(w http.ResponseWriter, r *http.Request) {
					hostname := chi.URLParam(r, "hostname")
					stackname := chi.URLParam(r, "stackname")
					
					stackID, err := services.GetStackIDByHostAndName(r.Context(), hostname, stackname)
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
					
					// Deploy in background
					manual := r.URL.Query().Get("auto") != "1"
					go func(id int64, manual bool) {
						ctx := context.Background()
						if manual {
							ctx = context.WithValue(ctx, services.CtxManualKey{}, true)
						}
						if err := services.DeployStack(ctx, id); err != nil {
							common.ErrorLog("deploy: stack %d failed: %v", id, err)
							return
						}
						common.InfoLog("deploy: stack %d ok", id)
					}(stackID, manual)
					
					writeJSON(w, http.StatusAccepted, map[string]any{
						"status":  "accepted",
						"stackID": stackID,
						"allowed": true,
					})
				})
				
				// Deploy check endpoint
				r.Post("/deploy-check", func(w http.ResponseWriter, r *http.Request) {
					hostname := chi.URLParam(r, "hostname")
					stackname := chi.URLParam(r, "stackname")
					
					_, err := services.GetStackIDByHostAndName(r.Context(), hostname, stackname)
					if err != nil {
						http.Error(w, "Stack not found", http.StatusNotFound)
						return
					}
					
					// For now, just return that deployment is allowed
					// TODO: Add actual configuration change detection
					writeJSON(w, http.StatusOK, map[string]any{
						"config_unchanged": false,
						"allowed": true,
					})
				})
				
				// Streaming deploy endpoint
				r.Get("/deploy-stream", func(w http.ResponseWriter, r *http.Request) {
					hostname := chi.URLParam(r, "hostname")
					stackname := chi.URLParam(r, "stackname")
					
					stackID, err := services.GetStackIDByHostAndName(r.Context(), hostname, stackname)
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
					if r.URL.Query().Get("force") == "true" {
						ctx = context.WithValue(ctx, services.CtxForceKey{}, true)
					}
					go func() {
						if err := services.DeployStackWithStream(ctx, stackID, eventChan); err != nil {
							common.ErrorLog("deploy-stream: stack %d failed: %v", stackID, err)
						}
					}()
					
					// Create encoder for SSE
					flusher, ok := w.(http.Flusher)
					if !ok {
						http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
						return
					}
					
					// Send events to client
					for event := range eventChan {
						data, _ := json.Marshal(event)
						fmt.Fprintf(w, "data: %s\n\n", string(data))
						flusher.Flush()
					}
				})
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