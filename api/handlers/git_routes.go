package handlers

import (
	"github.com/go-chi/chi/v5"
)

// SetupGitSyncRoutes registers all Git sync routes
func SetupGitSyncRoutes(api chi.Router) {
	h := NewGitSyncHandlers()
	
	// Git sync configuration and operations
	api.Get("/git/config", h.GetConfig)
	api.Post("/git/config", h.UpdateConfig)
	api.Put("/git/config", h.UpdateConfig)
	api.Get("/git/status", h.GetStatus)
	api.Post("/git/pull", h.Pull)
	api.Post("/git/push", h.Push)
	api.Post("/git/sync", h.Sync)
	api.Get("/git/logs", h.GetLogs)
	api.Get("/git/conflicts", h.GetConflicts)
	api.Post("/git/conflicts/resolve", h.ResolveConflict)
	api.Get("/git/check-initial-conflict", h.CheckInitialSetupConflict)
	api.Post("/git/test", h.TestConnection)
}