package common

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Global variables that are shared between main and handlers
var (
	DB    *pgxpool.Pool  // Database connection pool used across all packages
	SessionManager *scs.SessionManager
)

// Constants
const (
	SessionName = "ddui_sess"
)

// Logging functions moved to logging.go - these are kept for backward compatibility
// but just re-export from logging.go

// RespondJSON sends a JSON response
func RespondJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "failed to encode JSON", http.StatusInternalServerError)
	}
}

// Env gets an environment variable with a default value
func Env(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// EnvBool gets an environment variable as a boolean with a default value
func EnvBool(key, def string) bool {
	v := strings.ToLower(Env(key, def))
	return v == "1" || v == "t" || v == "true" || v == "yes" || v == "on"
}

// ReadSecretMaybeFile reads a secret from a file if the value starts with "@"
// Returns the secret value and an error (if any)
func ReadSecretMaybeFile(value string) (string, error) {
	// If value starts with "@", treat it as a file path
	if strings.HasPrefix(value, "@") {
		filepath := strings.TrimPrefix(value, "@")
		content, err := os.ReadFile(filepath)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(content)), nil
	}
	// Otherwise return the value as-is
	return value, nil
}