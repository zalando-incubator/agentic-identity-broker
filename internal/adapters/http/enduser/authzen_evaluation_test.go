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

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
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
			Resource:       tokenexchange.DelegationResource{ID: "880e8400-e29b-41d4-a716-446655440010", ResourceURI: "https://api.example.com/repos", Scopes: []string{}},
			Grant:          tokenexchange.DelegationGrant{ID: "990e8400-e29b-41d4-a716-446655440020", UpdatedAt: "2026-10-26T09:59:00Z"},
			PermissionSets: []tokenexchange.DelegationPermissionSet{{ID: "770e8400-e29b-41d4-a716-446655440001", Services: []tokenexchange.DelegationService{}}},
		},
		Diagnostic: tokenexchange.SuccessDiagnostic(tokenexchange.ExchangeThirdParty),
	}
}

func serveEvaluation(t *testing.T, evaluator *stubEvaluator, req *http.Request) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	handler := &AuthZENEvaluationHandler{Evaluator: evaluator, Logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec, logs.String()
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
			return newEvaluationRequest(`{"pad":"` + strings.Repeat("x", MaxAccessEvaluationBodyBytes) + `"}`)
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
