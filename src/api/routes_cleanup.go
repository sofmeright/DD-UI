package main

import (
	"github.com/go-chi/chi/v5"
)

// setupCleanupRoutes sets up all Docker cleanup related routes
// This is a cleaner way to organize the cleanup routes from web.go
func setupCleanupRoutes(router chi.Router) {
	router.Route("/cleanup", func(r chi.Router) {
		// Single host cleanup operations
		r.Route("/hosts/{hostname}", func(r chi.Router) {
			r.Post("/system", handleCleanupSystemPrune)
			r.Post("/images", handleCleanupImagePrune)
			r.Post("/containers", handleCleanupContainerPrune)
			r.Post("/volumes", handleCleanupVolumePrune)
			r.Post("/networks", handleCleanupNetworkPrune)
			r.Post("/build-cache", handleCleanupBuildCachePrune)
		})

		// All hosts cleanup operations
		r.Route("/all-hosts", func(r chi.Router) {
			r.Post("/system", handleCleanupAllHostsSystem)
		})

		// Space preview endpoints
		r.Route("/preview", func(r chi.Router) {
			r.Get("/{operation}/{hostname}", handleCleanupSpacePreview)
			r.Get("/{operation}/all-hosts", handleCleanupAllHostsPreview)
		})

		// Job management and monitoring
		r.Route("/jobs", func(r chi.Router) {
			r.Get("/{jobId}", handleGetCleanupJob)
			r.Get("/{jobId}/stream", handleCleanupJobStream)
		})
	})
}