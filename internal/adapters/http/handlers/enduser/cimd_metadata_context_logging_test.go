package enduser

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

func TestCIMDMetadataHandler_AuditRecordCarriesRequestSecurityContext(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(telemetry.NewContextHandler(slog.NewJSONHandler(&logs, nil)))
	handler := NewCIMDMetadataHandler(&cimdMetadataProviderFake{jwkSet: newPublicCIMDJWKSet(t)}, logger)

	serviceID := id.NewServiceID()
	req := newCIMDMetadataRequest(http.MethodGet, "/.well-known/oauth-client/"+serviceID.String()+"/jwks.json", serviceID)
	req = req.WithContext(security.WithSecurityContext(req.Context(), security.SecurityContext{
		TraceID: "fedcba9876543210fedcba9876543210",
		Actor:   id.NewPrincipal(security.AnonymousActor),
	}))
	rec := httptest.NewRecorder()

	handler.JWKS(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry))
	assert.Equal(t, "cimd metadata access", entry["msg"])
	assert.Equal(t, "success", entry["outcome"])
	assert.Equal(t, "fedcba9876543210fedcba9876543210", entry["trace_id"])
	assert.Equal(t, security.AnonymousActor, entry["actor"])
}
