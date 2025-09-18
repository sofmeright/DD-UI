// handlers/devops.go
package handlers

import (
	"encoding/json"
	"net/http"

	"dd-ui/common"
	"dd-ui/services"
	"github.com/go-chi/chi/v5"
)

// SetupDevopsRoutes configures all DevOps automation configuration routes
func SetupDevopsRoutes(router chi.Router) {
	router.Route("/devops", func(r chi.Router) {
		// Global DevOps configuration
		r.Get("/global", func(w http.ResponseWriter, r *http.Request) {
			val, src := services.GetGlobalDevopsApply(r.Context())
			writeJSON(w, http.StatusOK, map[string]any{
				"auto_deploy": val,
				"source":      src, // "db" or "env"
			})
		})

		// PATCH global: { "auto_deploy": true|false } or { "auto_deploy": null } to clear to ENV
		r.Patch("/global", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				AutoDeploy *bool `json:"auto_deploy"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			if err := services.SetGlobalDevopsApply(r.Context(), body.AutoDeploy); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			val, src := services.GetGlobalDevopsApply(r.Context())
			writeJSON(w, http.StatusOK, map[string]any{"auto_deploy": val, "source": src, "status": "ok"})
		})

		// Host-specific DevOps configuration
		r.Route("/hosts/{name}", func(r chi.Router) {
			// GET host auto-deployment override + effective setting
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				override, _ := services.GetHostDevopsOverride(r.Context(), host)
				global, _ := services.GetAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := common.EnvBool("DD_UI_DEVOPS_APPLY", "false")
					global = &d
				}
				effective := *global
				if override != nil {
					effective = *override
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"override":   override,  // null means inherit from global
					"effective":  effective, // actual value used
					"inherits_from": "global",
				})
			})

			// PATCH host auto-deployment: { "auto_deploy": true|false|null }
			r.Patch("/", func(w http.ResponseWriter, r *http.Request) {
				host := chi.URLParam(r, "name")
				var body struct {
					AutoDeploy *bool `json:"auto_deploy"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if err := services.SetHostDevopsOverride(r.Context(), host, body.AutoDeploy); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				override, _ := services.GetHostDevopsOverride(r.Context(), host)
				global, _ := services.GetAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := common.EnvBool("DD_UI_DEVOPS_APPLY", "false")
					global = &d
				}
				effective := *global
				if override != nil {
					effective = *override
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"override": override, 
					"effective": effective, 
					"inherits_from": "global",
					"status": "ok",
				})
			})

			// Stack-specific DevOps configuration for hosts
			r.Route("/stacks/{stackname}", func(r chi.Router) {
				// GET /api/devops/hosts/{name}/stacks/{stackname}
				r.Get("/", func(w http.ResponseWriter, r *http.Request) {
					host := chi.URLParam(r, "name")
					stackName := chi.URLParam(r, "stackname")
					override, err := services.GetStackDevopsOverride(r.Context(), "host", host, stackName)
					if err != nil {
						http.Error(w, err.Error(), http.StatusNotFound)
						return
					}
					
					// Determine effective value via hierarchy
					hostOverride, _ := services.GetHostDevopsOverride(r.Context(), host)
					global, _ := services.GetAppSettingBool(r.Context(), "devops_apply")
					if global == nil {
						d := common.EnvBool("DD_UI_DEVOPS_APPLY", "false")
						global = &d
					}
					
					effective := *global
					inheritsFrom := "global"
					if hostOverride != nil {
						effective = *hostOverride
						inheritsFrom = "host"
					}
					if override != nil {
						effective = *override
						inheritsFrom = "stack"
					}
					
					writeJSON(w, http.StatusOK, map[string]any{
						"override": override, 
						"effective": effective, 
						"inherits_from": inheritsFrom,
						"status": "ok",
					})
				})

				// PATCH /api/devops/hosts/{name}/stacks/{stackname}
				r.Patch("/", func(w http.ResponseWriter, r *http.Request) {
					host := chi.URLParam(r, "name")
					stackName := chi.URLParam(r, "stackname")
					var body struct {
						AutoDeploy *bool `json:"auto_deploy"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, "bad json", http.StatusBadRequest)
						return
					}
					if err := services.SetStackDevopsOverride(r.Context(), "host", host, stackName, body.AutoDeploy); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					
					// Return updated configuration
					override, _ := services.GetStackDevopsOverride(r.Context(), "host", host, stackName)
					hostOverride, _ := services.GetHostDevopsOverride(r.Context(), host)
					global, _ := services.GetAppSettingBool(r.Context(), "devops_apply")
					if global == nil {
						d := common.EnvBool("DD_UI_DEVOPS_APPLY", "false")
						global = &d
					}
					
					effective := *global
					inheritsFrom := "global"
					if hostOverride != nil {
						effective = *hostOverride
						inheritsFrom = "host"
					}
					if override != nil {
						effective = *override
						inheritsFrom = "stack"
					}
					
					writeJSON(w, http.StatusOK, map[string]any{
						"override": override, 
						"effective": effective, 
						"inherits_from": inheritsFrom,
						"status": "ok",
					})
				})
			})
		})

		// Group-specific DevOps configuration
		r.Route("/groups/{name}", func(r chi.Router) {
			// GET group auto-deployment override + effective setting
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				group := chi.URLParam(r, "name")
				override, _ := services.GetGroupDevopsOverride(r.Context(), group)
				global, _ := services.GetAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := common.EnvBool("DD_UI_DEVOPS_APPLY", "false")
					global = &d
				}
				effective := *global
				if override != nil {
					effective = *override
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"override":   override,  // null means inherit from global
					"effective":  effective, // actual value used
					"inherits_from": "global",
				})
			})

			// PATCH group auto-deployment: { "auto_deploy": true|false|null }
			r.Patch("/", func(w http.ResponseWriter, r *http.Request) {
				group := chi.URLParam(r, "name")
				var body struct {
					AutoDeploy *bool `json:"auto_deploy"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if err := services.SetGroupDevopsOverride(r.Context(), group, body.AutoDeploy); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				override, _ := services.GetGroupDevopsOverride(r.Context(), group)
				global, _ := services.GetAppSettingBool(r.Context(), "devops_apply")
				if global == nil {
					d := common.EnvBool("DD_UI_DEVOPS_APPLY", "false")
					global = &d
				}
				effective := *global
				if override != nil {
					effective = *override
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"override": override, 
					"effective": effective, 
					"inherits_from": "global",
					"status": "ok",
				})
			})

			// Stack-specific DevOps configuration for groups
			r.Route("/stacks/{stackname}", func(r chi.Router) {
				// GET /api/devops/groups/{name}/stacks/{stackname}
				r.Get("/", func(w http.ResponseWriter, r *http.Request) {
					group := chi.URLParam(r, "name")
					stackName := chi.URLParam(r, "stackname")
					override, err := services.GetStackDevopsOverride(r.Context(), "group", group, stackName)
					if err != nil {
						http.Error(w, err.Error(), http.StatusNotFound)
						return
					}
					
					// Determine effective value via hierarchy
					groupOverride, _ := services.GetGroupDevopsOverride(r.Context(), group)
					global, _ := services.GetAppSettingBool(r.Context(), "devops_apply")
					if global == nil {
						d := common.EnvBool("DD_UI_DEVOPS_APPLY", "false")
						global = &d
					}
					
					effective := *global
					inheritsFrom := "global"
					if groupOverride != nil {
						effective = *groupOverride
						inheritsFrom = "group"
					}
					if override != nil {
						effective = *override
						inheritsFrom = "stack"
					}
					
					writeJSON(w, http.StatusOK, map[string]any{
						"override": override, 
						"effective": effective, 
						"inherits_from": inheritsFrom,
						"status": "ok",
					})
				})

				// PATCH /api/devops/groups/{name}/stacks/{stackname}
				r.Patch("/", func(w http.ResponseWriter, r *http.Request) {
					group := chi.URLParam(r, "name")
					stackName := chi.URLParam(r, "stackname")
					var body struct {
						AutoDeploy *bool `json:"auto_deploy"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, "bad json", http.StatusBadRequest)
						return
					}
					if err := services.SetStackDevopsOverride(r.Context(), "group", group, stackName, body.AutoDeploy); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					
					// Return updated configuration
					override, _ := services.GetStackDevopsOverride(r.Context(), "group", group, stackName)
					groupOverride, _ := services.GetGroupDevopsOverride(r.Context(), group)
					global, _ := services.GetAppSettingBool(r.Context(), "devops_apply")
					if global == nil {
						d := common.EnvBool("DD_UI_DEVOPS_APPLY", "false")
						global = &d
					}
					
					effective := *global
					inheritsFrom := "global"
					if groupOverride != nil {
						effective = *groupOverride
						inheritsFrom = "group"
					}
					if override != nil {
						effective = *override
						inheritsFrom = "stack"
					}
					
					writeJSON(w, http.StatusOK, map[string]any{
						"override": override, 
						"effective": effective, 
						"inherits_from": inheritsFrom,
						"status": "ok",
					})
				})
			})
		})
	})
}