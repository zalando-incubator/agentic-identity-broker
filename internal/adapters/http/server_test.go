package http

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestNewHandlerAllowsWriteDeadlineLiftThroughAdminMiddleware(t *testing.T) {
	const writeTimeout = 100 * time.Millisecond

	deadlineResult := make(chan error, 1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(ServerConfig{
		Name:      "admin",
		Telemetry: ports.TelemetryConfig{Enabled: true},
	}, func(router chi.Router) {
		router.Post("/api/sessions/sweep", func(w http.ResponseWriter, r *http.Request) {
			err := http.NewResponseController(w).SetWriteDeadline(time.Time{})
			deadlineResult <- err
			if err != nil {
				http.Error(w, "unable to lift write deadline", http.StatusInternalServerError)
				return
			}

			timer := time.NewTimer(3 * writeTimeout)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "deadline lifted")
		})
	}, logger)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &http.Server{Handler: handler, WriteTimeout: writeTimeout}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, server.Shutdown(ctx))
		require.ErrorIs(t, <-serveResult, http.ErrServerClosed)
	})

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Post("http://"+listener.Addr().String()+"/api/sessions/sweep", "", nil)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	select {
	case err := <-deadlineResult:
		require.NoError(t, err, "the admin middleware must expose the underlying ResponseWriter to ResponseController")
	default:
		t.Fatal("the request did not reach the deadline-lifting handler")
	}
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "deadline lifted", string(body))
}
