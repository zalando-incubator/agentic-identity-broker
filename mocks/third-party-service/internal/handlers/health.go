package handlers

import (
	"encoding/json"
	"net/http"
)

// HealthResponse is the response from the health check endpoint
type HealthResponse struct {
	Status string `json:"status"`
}

// Health handles the health check endpoint
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(HealthResponse{Status: "ok"}) // #nosec G104 -- response is committed and a client-disconnect error cannot be recovered.
}
