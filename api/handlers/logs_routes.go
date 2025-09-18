package handlers

import (
	"github.com/go-chi/chi/v5"
)

// SetupLoggingRoutes registers all logging-related routes
func SetupLoggingRoutes(r chi.Router) {
	// Log streaming endpoint (SSE)
	r.Get("/logs/stream", HandleLogStream)
	
	// Get available log sources
	r.Get("/logs/sources", HandleGetLogSources)
	
	// Additional endpoints can be added here:
	// r.Get("/logs/history", HandleGetLogHistory)        // Get historical logs
	// r.Delete("/logs/cleanup", HandleLogCleanup)        // Manual log cleanup
	// r.Get("/logs/export", HandleExportLogs)           // Export logs
}