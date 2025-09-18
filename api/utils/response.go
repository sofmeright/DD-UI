package utils

import (
	"encoding/json"
	"net/http"
)

// WriteJSON writes a JSON response with the given status code
func WriteJSON(w http.ResponseWriter, status int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}

// WriteError writes an error response with the given status code
func WriteError(w http.ResponseWriter, err error, status int) {
	http.Error(w, err.Error(), status)
}

// WriteErrorMessage writes an error response with a custom message
func WriteErrorMessage(w http.ResponseWriter, message string, status int) {
	http.Error(w, message, status)
}

// WriteSuccess writes a success response with status 200
func WriteSuccess(w http.ResponseWriter, data interface{}) error {
	return WriteJSON(w, http.StatusOK, data)
}

// WriteStatus writes a simple status response
func WriteStatus(w http.ResponseWriter, status int, message string) error {
	return WriteJSON(w, status, map[string]string{"status": message})
}