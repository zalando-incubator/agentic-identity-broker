package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	httpAdapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
)

// TestHealthEndpoint tests the health endpoint format and responses
func TestHealthEndpoint(t *testing.T) {
	// Create logger for tests
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	config := httpAdapter.ServerConfig{Port: 0, Bind: "::"}

	// Simple route setup function that just adds health endpoint
	routeSetup := func(r chi.Router) {
		// This is already handled by Server.setupRoutes(), so we just need an empty setup
	}

	// Create and start server
	srv := httpAdapter.NewServer(config, routeSetup, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := srv.Listen()
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port
	errChan := make(chan error, 1)
	go func() { errChan <- srv.Serve(ctx, listener) }()

	// Wait for server to start
	waitForEndpoint(t, fmt.Sprintf("http://localhost:%d/health", port))

	// Test health endpoint format
	t.Run("ResponseFormat", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
		if err != nil {
			t.Fatalf("Failed to reach health endpoint: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Check status code
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Check content type
		contentType := resp.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
		}

		// Decode response
		var health httpAdapter.HealthResponse
		if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
			t.Fatalf("Failed to decode health response: %v", err)
		}

		// Verify all required fields are present
		if health.Status == "" {
			t.Error("Status field is missing")
		}
		if health.Timestamp.IsZero() {
			t.Error("Timestamp field is missing or zero")
		}
		if health.UptimeSeconds < 0 {
			t.Error("UptimeSeconds field is negative")
		}

		// Verify status value (Server field removed - no longer per-server identification needed)
		if health.Status != "healthy" {
			t.Errorf("Expected status 'healthy', got '%s'", health.Status)
		}
	})

	// Test health status values
	t.Run("StatusValues", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
		if err != nil {
			t.Fatalf("Failed to reach health endpoint: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		var health httpAdapter.HealthResponse
		if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
			t.Fatalf("Failed to decode health response: %v", err)
		}

		// Verify status is one of the valid values
		validStatuses := []string{"starting", "healthy", "shutting_down", "unhealthy"}
		validStatus := false
		for _, status := range validStatuses {
			if health.Status == status {
				validStatus = true
				break
			}
		}
		if !validStatus {
			t.Errorf("Status '%s' is not a valid health status value", health.Status)
		}
	})

	// Test uptime increases
	t.Run("UptimeIncreases", func(t *testing.T) {
		// Get first health response
		resp1, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
		if err != nil {
			t.Fatalf("Failed to reach health endpoint: %v", err)
		}
		defer func() { _ = resp1.Body.Close() }()

		var health1 httpAdapter.HealthResponse
		if err := json.NewDecoder(resp1.Body).Decode(&health1); err != nil {
			t.Fatalf("Failed to decode first health response: %v", err)
		}

		// Wait until uptime advances
		require.Eventually(t, func() bool {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
			if err != nil {
				return false
			}
			defer func() { _ = resp.Body.Close() }()

			var health httpAdapter.HealthResponse
			if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
				return false
			}
			return health.UptimeSeconds > health1.UptimeSeconds
		}, 2*time.Second, 25*time.Millisecond)

		// Get second health response
		resp2, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
		if err != nil {
			t.Fatalf("Failed to reach health endpoint: %v", err)
		}
		defer func() { _ = resp2.Body.Close() }()

		var health2 httpAdapter.HealthResponse
		if err := json.NewDecoder(resp2.Body).Decode(&health2); err != nil {
			t.Fatalf("Failed to decode second health response: %v", err)
		}

		// Verify uptime increased
		if health2.UptimeSeconds <= health1.UptimeSeconds {
			t.Errorf("Uptime did not increase: first=%d, second=%d", health1.UptimeSeconds, health2.UptimeSeconds)
		}
	})

	// Shutdown server gracefully
	if err := srv.Shutdown(context.Background(), 1*time.Second); err != nil {
		t.Logf("Shutdown error (may be expected): %v", err)
	}

	// Cancel context to ensure goroutine exits
	cancel()

	select {
	case err := <-errChan:
		require.NoError(t, err, "health server stopped unexpectedly")
	case <-time.After(2 * time.Second):
		t.Error("Server did not stop within timeout")
	}
}
