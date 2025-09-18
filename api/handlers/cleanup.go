package handlers

import (
	"net/http"
	
	"dd-ui/common"
	"github.com/go-chi/chi/v5"
)

func init() {
	common.DebugLog("Cleanup routes module initialized")
}

// SetupCleanupRoutes sets up all Docker cleanup related routes
// Following the hierarchical API design pattern:
// - /api/cleanup/hosts/{hostname}/... for host-scoped operations
// - /api/cleanup/global/... for global operations
func SetupCleanupRoutes(router chi.Router) {
	router.Route("/cleanup", func(r chi.Router) {
		// Test endpoint to verify routing works
		r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "cleanup routes are working"}`))
		})
		
		// Host-scoped cleanup operations
		r.Route("/hosts/{hostname}", func(r chi.Router) {
			// Preview endpoints
			r.Get("/preview/{operation}", handleCleanupSpacePreview)
			
			// Execution endpoints
			r.Post("/system", handleCleanupSystemPrune)
			r.Post("/images", handleCleanupImagePrune)
			r.Post("/containers", handleCleanupContainerPrune)
			r.Post("/volumes", handleCleanupVolumePrune)
			r.Post("/networks", handleCleanupNetworkPrune)
			r.Post("/build-cache", handleCleanupBuildCachePrune)
		})

		// Global cleanup operations (all hosts)
		r.Route("/global", func(r chi.Router) {
			// Preview endpoints
			r.Get("/preview/{operation}", handleCleanupGlobalPreview)
			
			// Execution endpoints
			r.Post("/system", handleCleanupGlobalSystem)
		})

		// Job management and monitoring
		r.Route("/jobs", func(r chi.Router) {
			r.Get("/{jobId}", handleGetCleanupJob)
			r.Get("/{jobId}/stream", handleCleanupJobStream)
		})
	})
}