package enduser

import (
	"bytes"
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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
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
	assert.Equal(t, "cimd.client_jwks.served", entry["event"])
	assert.Equal(t, "success", entry["outcome"])
	assert.Equal(t, "fedcba9876543210fedcba9876543210", entry["trace_id"])
	assert.Equal(t, security.AnonymousActor, entry["actor"])
}

func TestCIMDMetadataHandler_AuditEvents(t *testing.T) {
	unavailable := errors.New("service unavailable")
	tests := []struct {
		name          string
		provider      *cimdMetadataProviderFake
		serveJWKS     bool
		wantEvent     string
		wantOperation string
		wantOutcome   string
	}{
		{name: "metadata served", provider: &cimdMetadataProviderFake{metadata: &ports.CIMDClientMetadata{}}, wantEvent: "cimd.client_metadata.served", wantOperation: "metadata", wantOutcome: "success"},
		{name: "metadata rejected", provider: &cimdMetadataProviderFake{metadataErr: unavailable}, wantEvent: "cimd.client_metadata.serve_rejected", wantOperation: "metadata", wantOutcome: "rejected"},
		{name: "jwks served", provider: &cimdMetadataProviderFake{jwkSet: newPublicCIMDJWKSet(t)}, serveJWKS: true, wantEvent: "cimd.client_jwks.served", wantOperation: "jwks", wantOutcome: "success"},
		{name: "jwks rejected", provider: &cimdMetadataProviderFake{jwkSetErr: unavailable}, serveJWKS: true, wantEvent: "cimd.client_jwks.serve_rejected", wantOperation: "jwks", wantOutcome: "rejected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			handler := NewCIMDMetadataHandler(tt.provider, slog.New(slog.NewJSONHandler(&logs, nil)))
			serviceID := id.NewServiceID()
			rec := httptest.NewRecorder()

			if tt.serveJWKS {
				handler.JWKS(rec, newCIMDMetadataRequest(http.MethodGet, "/.well-known/oauth-client/"+serviceID.String()+"/jwks.json", serviceID))
			} else {
				handler.Metadata(rec, newCIMDMetadataRequest(http.MethodGet, "/.well-known/oauth-client/"+serviceID.String(), serviceID))
			}

			var entry map[string]any
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry))
			assert.Equal(t, tt.wantEvent, entry["event"])
			assert.Equal(t, serviceID.String(), entry["service_id"])
			assert.Equal(t, tt.wantOperation, entry["operation"])
			assert.Equal(t, tt.wantOutcome, entry["outcome"])
		})
	}
}
