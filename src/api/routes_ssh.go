// src/api/routes_ssh.go
package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// setupSshRoutes configures SSH-related routes
func setupSshRoutes(router chi.Router) {
	// SSH endpoint for direct command execution on hosts
	router.Post("/ssh/hosts/{name}", func(w http.ResponseWriter, r *http.Request) {
		hostName := chi.URLParam(r, "name")
		var body struct {
			Command string   `json:"command"`
			Args    []string `json:"args,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(body.Command) == "" {
			http.Error(w, "command is required", http.StatusBadRequest)
			return
		}

		op := SSHDirectOperation{
			HostName: hostName,
			Command:  body.Command,
			Args:     body.Args,
		}

		output, err := ExecuteSSHDirectOperation(r.Context(), op)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"output":  output,
		})
	})
}