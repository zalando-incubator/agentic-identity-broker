package oauth2_sessions

import (
	"bytes"
	"encoding/json"
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

func TestListSessions_LogRecordCarriesRequestSecurityContext(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(telemetry.NewContextHandler(slog.NewJSONHandler(&logs, nil)))
	handler := NewHandler(nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/third-party/sessions", nil)
	req = req.WithContext(security.WithSecurityContext(req.Context(), security.SecurityContext{
		TraceID: "0123456789abcdef0123456789abcdef",
		Actor:   id.NewPrincipal("alice@example.com"),
	}))
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry))
	assert.Equal(t, "principal not found in context", entry["msg"])
	assert.Equal(t, "0123456789abcdef0123456789abcdef", entry["trace_id"])
	assert.Equal(t, "alice@example.com", entry["actor"])
}
