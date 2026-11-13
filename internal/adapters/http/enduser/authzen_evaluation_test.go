package enduser

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

const (
	evalSubjectMarker   = "subject-token-marker-0001"
	evalAssertionMarker = "assertion-marker-0002"
)

type stubEvaluator struct {
	decision  tokenexchange.DelegationDecision
	calls     int
	request   tokenexchange.AccessEvaluationRequest
	assertion string
}

func (s *stubEvaluator) Evaluate(_ context.Context, req tokenexchange.AccessEvaluationRequest, assertion string) tokenexchange.DelegationDecision {
	s.calls++
	s.request = req
	s.assertion = assertion
	return s.decision
}

func validEvaluationBody() string {
	return `{"subject":{"type":"urn:ietf:params:oauth:token-type:jwt","id":"` + evalSubjectMarker + `"},
		"action":{"name":"urn:ietf:params:oauth:grant-type:token-exchange"},
		"resource":{"type":"protected_resource","id":"https://api.example.com/repos"}}`
}

func newEvaluationRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/access/v1/evaluation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+evalAssertionMarker)
	req.Header.Set("X-Request-ID", "req-123")
	return req
}

func permitDecision() tokenexchange.DelegationDecision {
	return tokenexchange.DelegationDecision{
		Kind:   tokenexchange.DecisionPermit,
		MaxAge: 15,
		Context: &tokenexchange.DelegationContext{
			Subject:        tokenexchange.DelegationSubject{ID: "alice@example.com"},
			Agent:          tokenexchange.DelegationEntityRef{ID: "550e8400-e29b-41d4-a716-446655440000"},
			Resource:       tokenexchange.DelegationResource{ServiceID: "880e8400-e29b-41d4-a716-446655440010", ResourceURI: "https://api.example.com/repos", Scopes: []string{}},
			Grant:          tokenexchange.DelegationGrant{ID: "990e8400-e29b-41d4-a716-446655440020", UpdatedAt: "2026-10-26T09:59:00Z"},
			PermissionSets: []tokenexchange.DelegationPermissionSet{{ID: "770e8400-e29b-41d4-a716-446655440001", Services: []tokenexchange.DelegationService{}}},
		},
		Diagnostic: tokenexchange.SuccessDiagnostic(tokenexchange.ExchangeThirdParty),
	}
}

func serveEvaluation(t *testing.T, evaluator *stubEvaluator, req *http.Request) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	handler := &AuthZENEvaluationHandler{Evaluator: evaluator, Logger: slog.New(telemetry.NewPrincipalPolicyHandler(slog.NewJSONHandler(&logs, nil)))}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec, logs.String()
}

func TestAuthZENEvaluation_AuditRecords(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		decision tokenexchange.DelegationDecision
		event    string
		level    string
	}{
		{"permit", permitDecision(), "tokenexchange.access_evaluation.permitted", "INFO"},
		{"denial", tokenexchange.DelegationDecision{Kind: tokenexchange.DecisionDeny}, "tokenexchange.access_evaluation.denied", "INFO"},
		{"malformed", tokenexchange.DelegationDecision{Kind: tokenexchange.DecisionRequestFailure, Status: http.StatusBadRequest}, "tokenexchange.access_evaluation.failed", "WARN"},
		{"unauthenticated", tokenexchange.DelegationDecision{Kind: tokenexchange.DecisionRequestFailure, Status: http.StatusUnauthorized}, "tokenexchange.access_evaluation.failed", "WARN"},
		{"forbidden", tokenexchange.DelegationDecision{Kind: tokenexchange.DecisionRequestFailure, Status: http.StatusForbidden}, "tokenexchange.access_evaluation.failed", "WARN"},
		{"server failure", tokenexchange.DelegationDecision{Kind: tokenexchange.DecisionRequestFailure, Status: http.StatusInternalServerError}, "tokenexchange.access_evaluation.failed", "ERROR"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			logger := slog.New(telemetry.NewPrincipalPolicyHandler(slog.NewJSONHandler(&logs, nil)))
			handler := &AuthZENEvaluationHandler{Evaluator: &stubEvaluator{decision: tt.decision}, Logger: logger}
			ctx := security.WithCaptureHolder(context.Background(), security.NewCaptureHolder(security.TransportCapture{}))
			security.FinalizeCaptureHolder(ctx, id.Principal("verified-owner"), "gateway")
			handler.ServeHTTP(httptest.NewRecorder(), newEvaluationRequest(validEvaluationBody()).WithContext(ctx))
			var record map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &record))
			assert.Equal(t, tt.event, record["event"])
			assert.Equal(t, tt.level, record["level"])
			assert.Equal(t, "verified-owner", record["principal"])
			assert.NotContains(t, logs.String(), evalSubjectMarker)
			assert.NotContains(t, logs.String(), evalAssertionMarker)
		})
	}
}

func TestAuthZENEvaluation_EarlyFailureAudit(t *testing.T) {
	t.Parallel()
	req := newEvaluationRequest(validEvaluationBody())
	req.Header.Del("Authorization")
	ctx := security.WithCaptureHolder(req.Context(), security.NewCaptureHolder(security.TransportCapture{}))
	_, logs := serveEvaluation(t, &stubEvaluator{}, req.WithContext(ctx))
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(logs), &record))
	assert.Equal(t, "tokenexchange.access_evaluation.failed", record["event"])
	assert.Equal(t, "WARN", record["level"])
	assert.Equal(t, security.AnonymousActor, record["principal"])
}

func TestAuthZENEvaluation_Permit(t *testing.T) {
	t.Parallel()
	evaluator := &stubEvaluator{decision: permitDecision()}

	rec, logs := serveEvaluation(t, evaluator, newEvaluationRequest(validEvaluationBody()))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "private, max-age=15", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "req-123", rec.Header().Get("X-Request-ID"))
	assert.Equal(t, evalAssertionMarker, evaluator.assertion)
	assert.Equal(t, evalSubjectMarker, evaluator.request.SubjectID)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["decision"])
	assert.Equal(t, "alice@example.com", body["context"].(map[string]any)["subject"].(map[string]any)["id"])
	for _, output := range []string{rec.Body.String(), logs} {
		assert.NotContains(t, output, evalSubjectMarker)
		assert.NotContains(t, output, evalAssertionMarker)
	}
	assert.Contains(t, logs, `"token_exchange.operation":"access_evaluation"`)
}

func TestAuthZENEvaluation_SpansExcludeCredentialsAndContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	for _, decision := range []tokenexchange.DelegationDecision{
		permitDecision(),
		{Kind: tokenexchange.DecisionDeny, Detail: tokenexchange.DetailGrantMissing,
			Diagnostic: tokenexchange.NewDiagnostic(tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantMissing)},
		{Kind: tokenexchange.DecisionRequestFailure, Status: http.StatusInternalServerError,
			Diagnostic: tokenexchange.NewDiagnostic(tokenexchange.StageResourceResolution, tokenexchange.DetailResourceRepositoryUnavailable)},
	} {
		rec, logs := serveEvaluation(t, &stubEvaluator{decision: decision}, newEvaluationRequest(validEvaluationBody()))
		assert.NotContains(t, rec.Body.String(), evalSubjectMarker)
		assert.NotContains(t, logs, evalAssertionMarker)
	}
	spans := recorder.Ended()
	require.Len(t, spans, 3)
	for i, span := range spans {
		assert.Equal(t, "tokenexchange.access_evaluation", span.Name())
		encoded, err := json.Marshal(tracetest.SpanStubFromReadOnlySpan(span))
		require.NoError(t, err)
		for _, sensitive := range []string{evalSubjectMarker, evalAssertionMarker, "alice@example.com", "https://api.example.com/repos"} {
			assert.NotContains(t, string(encoded), sensitive)
		}
		attrs := make(map[string]any)
		for _, attr := range span.Attributes() {
			attrs[string(attr.Key)] = attr.Value.AsInterface()
		}
		assert.Equal(t, "access_evaluation", attrs["token_exchange.operation"])
		assert.Equal(t, []string{"permit", "deny", "error"}[i], attrs["access_evaluation.decision"])
		if i == 0 {
			assert.Equal(t, int64(15), attrs["access_evaluation.max_age"])
		} else {
			assert.NotContains(t, attrs, "access_evaluation.max_age")
		}
		assert.Equal(t, trace.SpanKindInternal, span.SpanKind())
		assert.Equal(t, "tokenexchange", span.InstrumentationScope().Name)
	}
}

func TestAuthZENEvaluation_Denial(t *testing.T) {
	t.Parallel()
	evaluator := &stubEvaluator{decision: tokenexchange.DelegationDecision{
		Kind:       tokenexchange.DecisionDeny,
		Detail:     tokenexchange.DetailGrantMissing,
		ConsentURL: "https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000",
		Diagnostic: tokenexchange.NewDiagnostic(tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantMissing),
	}}

	rec, _ := serveEvaluation(t, evaluator, newEvaluationRequest(validEvaluationBody()))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "req-123", rec.Header().Get("X-Request-ID"))
	assert.JSONEq(t, `{"decision":false,"context":{"reason_admin":{"code":"grant_missing"},
		"reason_user":{"consent_url":"https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"}}}`, rec.Body.String())
}

func TestAuthZENEvaluation_DomainFailureMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status    int
		errorCode string
		challenge string
	}{
		{http.StatusBadRequest, "invalid_request", ""},
		{http.StatusUnauthorized, "unauthorized", `Bearer error="invalid_token"`},
		{http.StatusForbidden, "forbidden", ""},
		{http.StatusInternalServerError, "server_error", ""},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			t.Parallel()
			evaluator := &stubEvaluator{decision: tokenexchange.DelegationDecision{
				Kind: tokenexchange.DecisionRequestFailure, Status: tt.status,
				Diagnostic: tokenexchange.NewDiagnostic(tokenexchange.StageResourceResolution, tokenexchange.DetailResourceRepositoryUnavailable),
			}}
			rec, logs := serveEvaluation(t, evaluator, newEvaluationRequest(validEvaluationBody()))
			assert.Equal(t, tt.status, rec.Code)
			assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			assert.Equal(t, "req-123", rec.Header().Get("X-Request-ID"))
			assert.Equal(t, tt.challenge, rec.Header().Get("WWW-Authenticate"))
			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tt.errorCode, body["error"])
			assert.NotContains(t, body, "decision")
			assert.NotContains(t, logs, evalAssertionMarker)
		})
	}
}

func TestAuthZENEvaluation_RejectsBeforeResolution(t *testing.T) {
	t.Parallel()
	valid := validEvaluationBody()
	tests := []struct {
		name      string
		request   func() *http.Request
		status    int
		challenge string
	}{
		{"missing authorization", func() *http.Request {
			r := newEvaluationRequest(valid)
			r.Header.Del("Authorization")
			return r
		}, http.StatusUnauthorized, "Bearer"},
		{"non-bearer authorization", func() *http.Request {
			r := newEvaluationRequest(valid)
			r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
			return r
		}, http.StatusUnauthorized, "Bearer"},
		{"empty bearer", func() *http.Request {
			r := newEvaluationRequest(valid)
			r.Header.Set("Authorization", "Bearer ")
			return r
		}, http.StatusUnauthorized, "Bearer"},
		{"missing content type", func() *http.Request {
			r := newEvaluationRequest(valid)
			r.Header.Del("Content-Type")
			return r
		}, http.StatusBadRequest, ""},
		{"form content type", func() *http.Request {
			r := newEvaluationRequest(valid)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return r
		}, http.StatusBadRequest, ""},
		{"malformed JSON", func() *http.Request { return newEvaluationRequest(`{"subject":`) }, http.StatusBadRequest, ""},
		{"trailing data", func() *http.Request { return newEvaluationRequest(valid + `{}`) }, http.StatusBadRequest, ""},
		{"array body", func() *http.Request { return newEvaluationRequest(`[]`) }, http.StatusBadRequest, ""},
		{"oversized body", func() *http.Request {
			return newEvaluationRequest(strings.TrimSuffix(valid, "}") + `,"pad":"` + strings.Repeat("x", MaxAccessEvaluationBodyBytes) + `"}`)
		}, http.StatusBadRequest, ""},
		{"missing subject", func() *http.Request {
			return newEvaluationRequest(`{"action":{"name":"x"},"resource":{"type":"protected_resource","id":"https://a"}}`)
		}, http.StatusBadRequest, ""},
		{"null subject", func() *http.Request {
			return newEvaluationRequest(strings.Replace(valid, `"subject":{"type":"urn:ietf:params:oauth:token-type:jwt","id":"`+evalSubjectMarker+`"}`, `"subject":null`, 1))
		}, http.StatusBadRequest, ""},
		{"wrong subject id type", func() *http.Request {
			return newEvaluationRequest(strings.Replace(valid, `"id":"`+evalSubjectMarker+`"`, `"id":42`, 1))
		}, http.StatusBadRequest, ""},
		{"missing resource id", func() *http.Request {
			return newEvaluationRequest(strings.Replace(valid, `,"id":"https://api.example.com/repos"`, ``, 1))
		}, http.StatusBadRequest, ""},
		{"non-object context", func() *http.Request {
			return newEvaluationRequest(strings.TrimSuffix(valid, "}") + `,"context":"x"}`)
		}, http.StatusBadRequest, ""},
		{"null context", func() *http.Request {
			return newEvaluationRequest(strings.TrimSuffix(valid, "}") + `,"context":null}`)
		}, http.StatusBadRequest, ""},
		{"non-string anchor", func() *http.Request {
			return newEvaluationRequest(strings.TrimSuffix(valid, "}") + `,"context":{"agent":7}}`)
		}, http.StatusBadRequest, ""},
		{"null anchor", func() *http.Request {
			return newEvaluationRequest(strings.TrimSuffix(valid, "}") + `,"context":{"agent":null}}`)
		}, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			evaluator := &stubEvaluator{decision: permitDecision()}
			rec, logs := serveEvaluation(t, evaluator, tt.request())
			assert.Equal(t, tt.status, rec.Code)
			assert.Zero(t, evaluator.calls, "no resolution may happen before decoding and transport authentication succeed")
			assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			assert.Equal(t, "req-123", rec.Header().Get("X-Request-ID"))
			assert.Equal(t, tt.challenge, rec.Header().Get("WWW-Authenticate"))
			assert.NotContains(t, rec.Body.String(), evalSubjectMarker)
			assert.NotContains(t, logs, evalSubjectMarker)
		})
	}
}

func TestAuthZENEvaluation_AcceptsBodyAtSizeLimit(t *testing.T) {
	t.Parallel()
	body := strings.TrimSuffix(validEvaluationBody(), "}") + `,"pad":""}`
	body = strings.Replace(body, `"pad":""`, `"pad":"`+strings.Repeat("x", MaxAccessEvaluationBodyBytes-len(body))+`"`, 1)
	require.Len(t, body, MaxAccessEvaluationBodyBytes)
	evaluator := &stubEvaluator{decision: permitDecision()}
	rec, _ := serveEvaluation(t, evaluator, newEvaluationRequest(body))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, evaluator.calls)
}

func TestAuthZENEvaluation_RequiredMemberTypes(t *testing.T) {
	t.Parallel()
	for _, path := range [][]string{{"subject"}, {"action"}, {"resource"}, {"subject", "type"}, {"subject", "id"}, {"action", "name"}, {"resource", "type"}, {"resource", "id"}} {
		for _, value := range []string{"missing", "null", "true", "42", "[]", `"wrong-shape"`} {
			if len(path) == 2 && value == `"wrong-shape"` {
				continue // String value invariants belong to the domain service.
			}
			t.Run(strings.Join(path, ".")+"/"+value, func(t *testing.T) {
				t.Parallel()
				var body map[string]any
				require.NoError(t, json.Unmarshal([]byte(validEvaluationBody()), &body))
				members := body
				if len(path) == 2 {
					members = body[path[0]].(map[string]any)
				}
				key := path[len(path)-1]
				if value == "missing" {
					delete(members, key)
				} else {
					var invalid any
					require.NoError(t, json.Unmarshal([]byte(value), &invalid))
					members[key] = invalid
				}
				encoded, err := json.Marshal(body)
				require.NoError(t, err)
				evaluator := &stubEvaluator{decision: permitDecision()}
				rec, _ := serveEvaluation(t, evaluator, newEvaluationRequest(string(encoded)))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Zero(t, evaluator.calls)
				assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
				assert.Equal(t, "req-123", rec.Header().Get("X-Request-ID"))
			})
		}
	}
}

func TestAuthZENEvaluation_UnknownMembersCannotChangeDomainInputs(t *testing.T) {
	t.Parallel()
	baseline := &stubEvaluator{decision: permitDecision()}
	baselineResponse, _ := serveEvaluation(t, baseline, newEvaluationRequest(validEvaluationBody()))
	for _, path := range []string{"top", "subject", "action", "resource", "context"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(validEvaluationBody()), &body))
			members := body
			if path == "context" {
				members = map[string]any{}
				body[path] = members
			} else if path != "top" {
				members = body[path].(map[string]any)
			}
			members["properties"] = map[string]any{"principal": "other-user", "agent": "other-agent", "resource": "https://other.example.com"}
			members["actor"] = map[string]any{"id": "other-actor"}
			members["extension"] = []any{nil, true, 42}
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			evaluator := &stubEvaluator{decision: permitDecision()}
			rec, _ := serveEvaluation(t, evaluator, newEvaluationRequest(string(encoded)))
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, baseline.request, evaluator.request, "ignored members cannot reach identity resolution or caller CEL")
			assert.Equal(t, baseline.assertion, evaluator.assertion)
			assert.JSONEq(t, baselineResponse.Body.String(), rec.Body.String())
		})
	}
}

func TestAuthZENEvaluation_IgnoresUnknownMembersAndPassesAnchor(t *testing.T) {
	t.Parallel()
	anchor := id.NewAgentID().String()
	body := `{"subject":{"type":"urn:ietf:params:oauth:token-type:jwt","id":"s","properties":{"department":"x"}},
		"action":{"name":"urn:ietf:params:oauth:grant-type:token-exchange","properties":{"method":"GET"}},
		"resource":{"type":"protected_resource","id":"https://api.example.com/repos","properties":{}},
		"context":{"agent":"` + anchor + `","time":"2026-10-26T10:00:00Z","actor":{"id":"x"}},"extension":true}`
	evaluator := &stubEvaluator{decision: permitDecision()}

	rec, _ := serveEvaluation(t, evaluator, newEvaluationRequest(body))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, ptr.To(anchor), evaluator.request.AgentAnchor)
}

func TestAuthZENEvaluation_NoRequestIDEchoWhenAbsent(t *testing.T) {
	t.Parallel()
	req := newEvaluationRequest(validEvaluationBody())
	req.Header.Del("X-Request-ID")
	rec, _ := serveEvaluation(t, &stubEvaluator{decision: permitDecision()}, req)
	assert.Empty(t, rec.Header().Get("X-Request-ID"))
}

func TestAuthZENConfiguration_PublishesMetadataFromPublicURL(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/authzen-configuration", nil)
	req.Header.Set("X-Request-ID", "disc-1")
	rec := httptest.NewRecorder()

	NewAuthZENConfigurationHandler("https://broker.example.com/").ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "max-age=3600", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "disc-1", rec.Header().Get("X-Request-ID"))
	assert.JSONEq(t, `{"policy_decision_point":"https://broker.example.com",
		"access_evaluation_endpoint":"https://broker.example.com/access/v1/evaluation"}`, rec.Body.String())
}
