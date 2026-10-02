// Package http provides HTTP server adapters for the identity broker.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Convenience constants for use in this package
const (
	// HealthStateStarting indicates the server is initializing.
	HealthStateStarting = ports.HealthStateStarting

	// HealthStateHealthy indicates the server is operational.
	HealthStateHealthy = ports.HealthStateHealthy

	// HealthStateShuttingDown indicates graceful shutdown is in progress.
	HealthStateShuttingDown = ports.HealthStateShuttingDown

	// HealthStateUnhealthy indicates the server encountered an error.
	HealthStateUnhealthy = ports.HealthStateUnhealthy
)

// healthStateString returns the string representation of a health state.
func healthStateString(state ports.HealthState) string {
	switch state {
	case ports.HealthStateStarting:
		return "starting"
	case ports.HealthStateHealthy:
		return "healthy"
	case ports.HealthStateShuttingDown:
		return "shutting_down"
	case ports.HealthStateUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// healthStateHTTPStatus returns the HTTP status code for this health state.
func healthStateHTTPStatus(state ports.HealthState) int {
	switch state {
	case ports.HealthStateHealthy:
		return http.StatusOK // 200
	case ports.HealthStateStarting, ports.HealthStateShuttingDown, ports.HealthStateUnhealthy:
		return http.StatusServiceUnavailable // 503
	default:
		return http.StatusServiceUnavailable
	}
}

// HealthResponse represents the JSON response for the health endpoint.
// The end-user OpenAPI documents optional component health details when present.
type HealthResponse struct {
	Status        string            `json:"status"`               // Health status: "starting", "healthy", "shutting_down", "unhealthy"
	Timestamp     time.Time         `json:"timestamp"`            // Current server time (ISO 8601)
	UptimeSeconds int64             `json:"uptime_seconds"`       // Time since server started serving requests
	Components    map[string]string `json:"components,omitempty"` // Optional component-level health details
}

// NewHealthHandler returns the production health endpoint handler.
// It is shared by the server adapter and test bootstrap so E2E tests exercise
// the same response shape and component-health serialization as production.
func NewHealthHandler(
	healthStatus func() ports.HealthState,
	startTime time.Time,
	healthComponents func() map[string]string,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		healthState := ports.HealthStateHealthy
		if healthStatus != nil {
			healthState = healthStatus()
		}

		uptime := time.Since(startTime)
		response := HealthResponse{
			Timestamp:     time.Now().UTC(),
			UptimeSeconds: int64(uptime.Seconds()),
		}
		if healthComponents != nil {
			components := healthComponents()
			if len(components) > 0 {
				response.Components = make(map[string]string, len(components))
				for name, state := range components {
					response.Components[name] = state
					if healthState == ports.HealthStateHealthy && state == "unhealthy" {
						healthState = ports.HealthStateUnhealthy
					}
				}
			}
		}
		response.Status = healthStateString(healthState)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(healthStateHTTPStatus(healthState))

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to encode health response",
				"error", err)
			// Don't try to write another response; headers already sent
		}
	}
}

// handleHealth returns an HTTP handler for the health endpoint.
// It reads the server's health state and returns a JSON response with appropriate HTTP status code.
// The handler is designed to be fast and non-blocking, using atomic operations for health state.
func (s *Server) handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		NewHealthHandler(s.HealthStatus, s.startTime, s.config.HealthComponents, s.logger)(w, r)
	}
}
