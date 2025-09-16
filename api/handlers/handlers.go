package handlers

import (
	"github.com/go-chi/chi/v5"
)

// SetupAllRoutes sets up all the handler routes
// This function is called from web.go to register all handler routes
func SetupAllRoutes(router chi.Router) {
	SetupSystemRoutes(router)
	SetupDockerRoutes(router)
	SetupIacRoutes(router)
	SetupGitopsRoutes(router)
	SetupSshRoutes(router)
	SetupCleanupRoutes(router)
}