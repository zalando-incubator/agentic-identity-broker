package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/routing"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleHealth_IncludesComponentHealth(t *testing.T) {
	server := &Server{
		config: ServerConfig{
			HealthComponents: func() map[string]string {
				return map[string]string{"upstream_jwks": "degraded"}
			},
		},
		healthState: int32(HealthStateHealthy),
		startTime:   time.Now().Add(-2 * time.Minute),
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	server.handleHealth().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var response HealthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, "degraded", response.Components["upstream_jwks"])
}

func TestHandleHealth_NativeMaintenanceLossAndRecovery(t *testing.T) {
	maintenanceState := "healthy"
	server := &Server{
		config: ServerConfig{
			HealthComponents: func() map[string]string {
				return map[string]string{
					"refresh_sessions": maintenanceState,
					"upstream_jwks":    "degraded",
				}
			},
		},
		healthState: int32(HealthStateHealthy),
		startTime:   time.Now(),
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := chi.NewRouter()
	router.Get("/health", server.handleHealth())
	healthServer := httptest.NewServer(router)
	defer healthServer.Close()

	check := func(wantCode int, wantStatus string) {
		t.Helper()
		responseHTTP, err := healthServer.Client().Get(healthServer.URL + "/health")
		require.NoError(t, err)
		defer func() { _ = responseHTTP.Body.Close() }()
		require.Equal(t, wantCode, responseHTTP.StatusCode)
		var response HealthResponse
		require.NoError(t, json.NewDecoder(responseHTTP.Body).Decode(&response))
		assert.Equal(t, wantStatus, response.Status)
		assert.Equal(t, maintenanceState, response.Components["refresh_sessions"])
		assert.Equal(t, "degraded", response.Components["upstream_jwks"])
	}

	check(http.StatusOK, "healthy")
	maintenanceState = "unhealthy"
	check(http.StatusServiceUnavailable, "unhealthy")
	server.healthState = int32(HealthStateStarting)
	check(http.StatusServiceUnavailable, "starting")
	server.healthState = int32(HealthStateShuttingDown)
	check(http.StatusServiceUnavailable, "shutting_down")
	server.healthState = int32(HealthStateHealthy)
	maintenanceState = "healthy"
	check(http.StatusOK, "healthy")
}

func TestHandleHealth_UsesCurrentServerStartTime(t *testing.T) {
	server := &Server{
		healthState: int32(HealthStateHealthy),
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	handler := server.handleHealth()
	server.startTime = time.Now().Add(-2 * time.Second)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var response HealthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.GreaterOrEqual(t, response.UptimeSeconds, int64(1))
	assert.Less(t, response.UptimeSeconds, int64(10))
}

func TestServerServe_HealthRouteRemainsReachableWithRootSPA(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	staticPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "index.html"), []byte("index"), 0o600))

	server := NewServer(ServerConfig{}, func(router chi.Router) {
		routing.SetupEnduserRoutes(router, &app.EnduserHandlers{
			SPA: handlers.NewSPAHandler(staticPath, logger),
		}, routing.EnduserRouteConfig{Logger: logger})
	}, logger)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(context.Background(), listener)
	}()

	url := "http://" + listener.Addr().String() + "/health"
	require.Eventually(t, func() bool {
		response, err := http.Get(url)
		if err != nil {
			return false
		}
		defer func() { _ = response.Body.Close() }()
		return response.StatusCode == http.StatusOK
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, server.Shutdown(context.Background(), time.Second))
	require.NoError(t, <-serveErr)
}
