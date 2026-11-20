package enduser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type failingTokenOutcomes struct {
	failure ports.TokenProxyFailure
	err     error
}

func (f failingTokenOutcomes) Proxy(context.Context, url.Values, string, *ports.TokenGrantResolution) (ports.TokenProxyResponse, ports.TokenProxyFailure, error) {
	return nil, f.failure, f.err
}

func (f failingTokenOutcomes) CompleteProxy(context.Context, ports.TokenProxyResponse, id.AgentID) (ports.TokenProxyCompletion, ports.TokenProxyFailure, error) {
	return ports.TokenProxyCompletion{}, f.failure, f.err
}

func TestProxyTokenGrantStrategy_ServiceScopeFailureTelemetry(t *testing.T) {
	secrets := []string{"synthetic-refresh-token", "synthetic-access-token", "synthetic-subject-token", "synthetic-client-secret", "synthetic-client-assertion", "synthetic-code", "synthetic-verifier", "synthetic-state", "synthetic-sealed-jwe", "synthetic-rejected-scope"}
	for _, tc := range []struct {
		name, event, level, reason, kind, oauthError string
		failure                                      ports.TokenProxyFailure
		cause                                        error
		status                                       int
	}{
		{"malformed", "scope_rejected", "WARN", "malformed_scope", "definitive", "invalid_scope", ports.TokenProxyScopeNotPermitted, servicescope.ErrMalformedScope, http.StatusBadRequest},
		{"unknown", "scope_rejected", "WARN", "unknown_service", "definitive", "invalid_scope", ports.TokenProxyScopeNotPermitted, servicescope.ErrUnknownService, http.StatusBadRequest},
		{"ineligible", "scope_rejected", "WARN", "service_not_eligible", "definitive", "invalid_scope", ports.TokenProxyScopeNotPermitted, servicescope.ErrServiceNotEligible, http.StatusBadRequest},
		{"binding", "scope_rejected", "WARN", "binding_mismatch", "definitive", "invalid_scope", ports.TokenProxyScopeNotPermitted, servicescope.ErrBindingMismatch, http.StatusBadRequest},
		{"dependency", "resolution_failed", "ERROR", "dependency_failed", "transient", "server_error", ports.TokenProxyScopeLookupFailed, errors.New("dependency unavailable"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			previous := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			t.Cleanup(func() {
				otel.SetTracerProvider(previous)
				require.NoError(t, provider.Shutdown(context.Background()))
			})
			var logs bytes.Buffer
			strategy := NewProxyTokenGrantStrategy("https://idp.example.com/token", failingTokenOutcomes{failure: tc.failure, err: fmt.Errorf("%s: %w", strings.Join(secrets, " "), tc.cause)}, slog.New(slog.NewJSONHandler(&logs, nil)))
			form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {secrets[0]}, "access_token": {secrets[1]}, "subject_token": {secrets[2]}, "client_secret": {secrets[3]}, "client_assertion": {secrets[4]}, "code": {secrets[5]}, "code_verifier": {secrets[6]}, "state": {secrets[7]}, "session_token": {secrets[8]}, "scope": {secrets[9]}}
			request := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			agentID := id.NewAgentID()
			strategy.HandleTokenGrant(response, request, "refresh_token", form, &ports.TokenGrantResolution{AgentID: agentID})
			assert.Equal(t, tc.status, response.Code)
			assert.Contains(t, response.Body.String(), `"error":"`+tc.oauthError+`"`)
			var record map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &record), "exactly one audit record")
			assert.Equal(t, "oauth2.requested_service."+tc.event, record["event"])
			assert.Equal(t, tc.level, record["level"])
			assert.Equal(t, agentID.String(), record["agent_id"])
			assert.Equal(t, tc.reason, record["reason"])
			assert.Equal(t, tc.kind, record["error_kind"])
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			assert.Equal(t, "oauth2.token_proxy", spans[0].Name())
			assert.Equal(t, trace.SpanKindInternal, spans[0].SpanKind())
			assert.Equal(t, codes.Error, spans[0].Status().Code)
			attrs := map[string]any{}
			for _, attr := range spans[0].Attributes() {
				attrs[string(attr.Key)] = attr.Value.AsInterface()
			}
			assert.Equal(t, tc.kind, attrs["error.type"])
			assert.Equal(t, int64(tc.status), attrs["http.response.status_code"])
			for _, secret := range secrets {
				assert.NotContains(t, logs.String(), secret)
				assert.NotContains(t, fmt.Sprint(spans[0].Attributes(), spans[0].Events(), spans[0].Status()), secret)
			}
		})
	}
}

func TestProxyTokenGrantStrategy_UpstreamFailureLogsNoURLQueryOrCause(t *testing.T) {
	const upstreamURL = "https://idp.example.com/oauth2/token?tenant=query-secret"
	leaky := &url.Error{Op: "Post", URL: upstreamURL, Err: errors.New("dial tcp: connection refused")}

	for _, test := range []struct {
		name     string
		failure  ports.TokenProxyFailure
		wantKind string
	}{
		{name: "request creation", failure: ports.TokenProxyRequestCreationFailed, wantKind: security.OutboundErrorTransport},
		{name: "transport", failure: ports.TokenProxyNoFailure, wantKind: security.OutboundErrorTransport},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			strategy := NewProxyTokenGrantStrategy(upstreamURL, failingTokenOutcomes{failure: test.failure, err: leaky},
				slog.New(slog.NewJSONHandler(&logs, nil)))
			request := httptest.NewRequest(http.MethodPost, "/oauth2/token", nil)
			response := httptest.NewRecorder()

			strategy.HandleTokenGrant(response, request, "", url.Values{}, &ports.TokenGrantResolution{AgentID: id.NewAgentID()})

			require.GreaterOrEqual(t, response.Code, http.StatusInternalServerError)
			out := logs.String()
			assert.NotContains(t, out, "query-secret")
			assert.NotContains(t, out, "connection refused")
			assert.Contains(t, out, `"upstream_url":"https://idp.example.com/oauth2/token"`)
			assert.Contains(t, out, `"error_kind":"`+test.wantKind+`"`)
		})
	}
}
