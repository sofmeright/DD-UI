// src/api/routes_ssh.go
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"dd-ui/common"
	"dd-ui/database"
	"dd-ui/utils"
	"github.com/go-chi/chi/v5"
)

// hostProvider implements utils.HostProvider interface
type hostProvider struct{}

func (hp hostProvider) GetHostByName(ctx context.Context, name string) (utils.HostInfo, error) {
	hostRow, err := database.GetHostByName(ctx, name)
	if err != nil {
		return utils.HostInfo{}, err
	}
	return utils.HostInfo{
		Name: hostRow.Name,
		Addr: hostRow.Addr,
		Vars: hostRow.Vars,
	}, nil
}

// envProvider implements utils.EnvProvider interface
type envProvider struct{}

func (ep envProvider) Env(key, defaultValue string) string {
	return common.Env(key, defaultValue)
}

// setupSshRoutes configures SSH-related routes
func SetupSshRoutes(router chi.Router) {
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

		op := utils.SSHDirectOperation{
			HostName: hostName,
			Command:  body.Command,
			Args:     body.Args,
		}

		output, err := utils.ExecuteSSHDirectOperation(r.Context(), hostProvider{}, envProvider{}, op)
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