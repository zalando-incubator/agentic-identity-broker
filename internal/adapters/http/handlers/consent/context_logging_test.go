package consent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
)

func TestResponseWriter_LogsCarryRequestContext(t *testing.T) {
	for _, failure := range []string{"encoding", "write"} {
		t.Run(failure, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(telemetry.NewContextHandler(slog.NewJSONHandler(&logs, nil)))
			writers := map[string]func(context.Context, http.ResponseWriter, int, any){
				"agents":       NewAgentsHandler(nil, logger).writeJSON,
				"agent detail": NewAgentDetailHandler(nil, logger, newTestSessionTokenValidator()).writeJSON,
				"grants":       NewGrantsHandler(nil, logger, newTestSessionTokenValidator()).writeJSON,
				"user info":    NewUserInfoHandler(logger).writeJSON,
			}
			for name, write := range writers {
				t.Run(name, func(t *testing.T) {
					logs.Reset()
					ctx := security.WithSecurityContext(t.Context(), security.SecurityContext{
						TraceID:     "0123456789abcdef0123456789abcdef",
						Actor:       id.NewPrincipal("operator@example.com"),
						CallingPeer: "test-peer",
					})
					var w http.ResponseWriter = httptest.NewRecorder()
					var data any = map[string]any{"client_secret": "response-credential-marker"}
					if failure == "encoding" {
						data = make(chan int)
					} else {
						w = &contextLoggingFailedWriter{ResponseRecorder: httptest.NewRecorder()}
					}
					write(ctx, w, http.StatusOK, data)
					assert.NotContains(t, logs.String(), "response-credential-marker")
					var entry map[string]any
					require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry))
					assert.Equal(t, "0123456789abcdef0123456789abcdef", entry["trace_id"])
					assert.Equal(t, "operator@example.com", entry["actor"])
					assert.Equal(t, "test-peer", entry["calling_peer"])
				})
			}
		})
	}
}

type contextLoggingFailedWriter struct{ *httptest.ResponseRecorder }

func (w *contextLoggingFailedWriter) Write([]byte) (int, error) {
	return 0, errors.New("response write failed")
}
