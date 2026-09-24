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
	"golang.org/x/sync/errgroup"

	httpAdapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
)

// TestServerStartup tests basic dual-server startup and health endpoints
func TestServerStartup(t *testing.T) {
	// Create logger for tests
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	enduserConfig := httpAdapter.ServerConfig{Port: 0, Bind: "::"}
	adminConfig := httpAdapter.ServerConfig{Port: 0, Bind: "::"}

	// Simple route setup function
	routeSetup := func(r chi.Router) {
		// Routes already handled by Server.setupRoutes()
	}

	// Create server instances
	enduserServer := httpAdapter.NewServer(enduserConfig, routeSetup, logger)
	adminServer := httpAdapter.NewServer(adminConfig, routeSetup, logger)

	// Start servers with dual-server coordination using errgroup (like root.go)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Phase 1: Bind both servers
	g, bindCtx := errgroup.WithContext(ctx)
	var enduserListener, adminListener net.Listener
	var enduserErr, adminErr error
	defer func() {
		if enduserListener != nil {
			_ = enduserListener.Close()
		}
		if adminListener != nil {
			_ = adminListener.Close()
		}
	}()

	g.Go(func() error {
		enduserListener, enduserErr = enduserServer.Listen()
		return enduserErr
	})
	g.Go(func() error {
		adminListener, adminErr = adminServer.Listen()
		return adminErr
	})

	if err := g.Wait(); err != nil {
		t.Fatalf("Failed to bind servers: %v", err)
	}

	// Phase 2: Serve both servers
	g, serveCtx := errgroup.WithContext(bindCtx)

	g.Go(func() error {
		return enduserServer.Serve(serveCtx, enduserListener)
	})
	g.Go(func() error {
		return adminServer.Serve(serveCtx, adminListener)
	})

	enduserURL := fmt.Sprintf("http://localhost:%d/health", enduserListener.Addr().(*net.TCPAddr).Port)
	adminURL := fmt.Sprintf("http://localhost:%d/health", adminListener.Addr().(*net.TCPAddr).Port)
	waitForEndpoint(t, enduserURL)
	waitForEndpoint(t, adminURL)

	// Test enduser server health endpoint
	t.Run("EndUserHealth", func(t *testing.T) {
		resp, err := http.Get(enduserURL)
		if err != nil {
			t.Fatalf("Failed to reach enduser health endpoint: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var health httpAdapter.HealthResponse
		if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
			t.Fatalf("Failed to decode health response: %v", err)
		}

		if health.Status != "healthy" {
			t.Errorf("Expected status 'healthy', got '%s'", health.Status)
		}
	})

	// Test admin server health endpoint
	t.Run("AdminHealth", func(t *testing.T) {
		resp, err := http.Get(adminURL)
		if err != nil {
			t.Fatalf("Failed to reach admin health endpoint: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var health httpAdapter.HealthResponse
		if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
			t.Fatalf("Failed to decode health response: %v", err)
		}

		if health.Status != "healthy" {
			t.Errorf("Expected status 'healthy', got '%s'", health.Status)
		}
	})

	// Shutdown servers gracefully
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()

	if err := enduserServer.Shutdown(shutdownCtx, 1*time.Second); err != nil {
		t.Logf("EndUser shutdown error (may be expected): %v", err)
	}
	if err := adminServer.Shutdown(shutdownCtx, 1*time.Second); err != nil {
		t.Logf("Admin shutdown error (may be expected): %v", err)
	}

	// Cancel context to ensure goroutines exit
	cancel()
	if err := g.Wait(); err != nil {
		t.Errorf("Serving dual servers: %v", err)
	}
}

// TestPortConnectivity tests IPv4 and IPv6 connectivity
func TestPortConnectivity(t *testing.T) {
	// Create logger for tests
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	config := httpAdapter.ServerConfig{Port: 0, Bind: "::"}

	// Simple route setup function
	routeSetup := func(r chi.Router) {
		// Routes already handled by Server.setupRoutes()
	}

	// Create and start server
	srv := httpAdapter.NewServer(config, routeSetup, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := srv.Listen()
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Serve(ctx, listener)
	}()

	// Wait for server to start
	waitForEndpoint(t, fmt.Sprintf("http://127.0.0.1:%d/health", port))

	// Test IPv4 connectivity
	t.Run("IPv4", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if err != nil {
			t.Fatalf("Failed to connect via IPv4: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Test IPv6 connectivity
	t.Run("IPv6", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://[::1]:%d/health", port))
		if err != nil {
			t.Skipf("IPv6 not available: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Test localhost DNS resolution
	t.Run("Localhost", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
		if err != nil {
			t.Fatalf("Failed to connect via localhost: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
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
		if err != nil {
			t.Errorf("Serving connectivity test: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not stop within timeout")
	}
}

// TestPortConflictRejectsSecondServer verifies that an occupied port cannot be rebound.
func TestPortConflictRejectsSecondServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	routeSetup := func(r chi.Router) {}

	first := httpAdapter.NewServer(httpAdapter.ServerConfig{Port: 0, Bind: "::"}, routeSetup, logger)
	listener, err := first.Listen()
	if err != nil {
		t.Fatalf("Bind first server: %v", err)
	}
	defer func() { _ = listener.Close() }()

	port := listener.Addr().(*net.TCPAddr).Port
	second := httpAdapter.NewServer(httpAdapter.ServerConfig{Port: port, Bind: "::"}, routeSetup, logger)
	otherListener, err := second.Listen()
	if otherListener != nil {
		_ = otherListener.Close()
	}
	if err == nil {
		t.Fatal("Expected binding the occupied port to fail")
	}
}
