package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/grantcontext"
)

type identityExchanger struct {
	result ExchangeResult
}

func (e identityExchanger) Exchange(context.Context, string, string) (ExchangeResult, error) {
	return e.result, nil
}
func (e identityExchanger) ExchangeBound(_ context.Context, _, _ string, _ *ExchangeResult, binding ExchangeResultBinding) (ExchangeResult, error) {
	if !e.result.matchesIdentity(binding) {
		return ExchangeResult{}, ErrExchangeResultBindingConflict
	}
	return e.result, nil
}
func (identityExchanger) Shutdown() {}

type identityAuthorizer struct {
	calls int
	input authorization.ContextInput
}

func (a *identityAuthorizer) Evaluate(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
	a.calls++
	a.input = input["context"].(authorization.ContextInput)
	return &authorization.OPADecision{Action: authorization.ActionApprovalRequired}, nil
}
func (*identityAuthorizer) Stop(context.Context) {}

type identityGate struct {
	invocations []approval.Invocation
}

func (g *identityGate) Evaluate(_ context.Context, invocation approval.Invocation) approval.Outcome {
	g.invocations = append(g.invocations, invocation)
	return approval.Outcome{Proceed: true}
}

func TestApprovalIdentityUsesExchangeAndRequiresPermitConsistencyWhenEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, exchangeIdentity := range []approval.Identity{{Principal: "cached-principal", AgentID: "cached-agent"}, {}, {Principal: "permit-principal", AgentID: "permit-agent"}} {
			t.Run(fmt.Sprintf("enabled=%t/identity=%s/%s", enabled, exchangeIdentity.Principal, exchangeIdentity.AgentID), func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
				permit := grantcontext.Result{Deadline: time.Now().Add(time.Minute), Context: grantcontext.Context{
					Principal: "permit-principal", Agent: grantcontext.EntityRef{ID: "permit-agent"},
					Grant: grantcontext.Grant{UpdatedAt: "2026-10-26T09:59:00Z"},
				}}
				exchanger := identityExchanger{result: ExchangeResult{Token: "provider-token-marker", Principal: exchangeIdentity.Principal, AgentID: exchangeIdentity.AgentID}}
				authorizer, gate := &identityAuthorizer{}, &identityGate{}
				s := NewServerWithApprovalGate(&extprocconfig.Config{}, exchanger, authorizer, gate, logger)
				if enabled {
					s.WithPermitResolver(observedGrantResolver{result: &permit})
				}
				headers := &extprocv3.HttpHeaders{Headers: &corev3.HeaderMap{Headers: []*corev3.HeaderValue{{Key: ":method", RawValue: []byte("POST")}, {Key: "mcp-session-id", RawValue: []byte("mcp-session")}}}}
				req := safetyRequest("subject-token-marker", "https://resource.example", headers)
				response, state := s.processRequestHeadersOPA(context.Background(), false, req, headers)
				if enabled && exchangeIdentity != (approval.Identity{Principal: permit.Context.Principal, AgentID: permit.Context.Agent.ID}) {
					require.NotNil(t, response.GetImmediateResponse())
					assert.Equal(t, int32(503), int32(response.GetImmediateResponse().Status.Code))
					assert.JSONEq(t, grantContextBindingUnavailableBody, string(response.GetImmediateResponse().Body))
					assert.Empty(t, gate.invocations)
					assert.Zero(t, authorizer.calls, "an identity mismatch must stop before OPA")
					assert.Nil(t, state)
					for _, marker := range []string{"subject-token-marker", "provider-token-marker", "permit-principal", "cached-principal"} {
						assert.NotContains(t, logs.String(), marker)
					}
					return
				}
				require.NotNil(t, response.GetRequestHeaders())
				require.NotNil(t, state)
				body := &extprocv3.HttpBody{Body: []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_files"}}`), EndOfStream: true}
				response = s.processRequestBody(context.Background(), state, body)
				require.NotNil(t, response.GetRequestBody())
				require.Len(t, gate.invocations, 1)
				assert.Equal(t, 1, authorizer.calls)
				want := exchangeIdentity
				if enabled {
					assert.Equal(t, want.Principal, authorizer.input.Principal)
					require.NotNil(t, authorizer.input.Agent)
					assert.Equal(t, want.AgentID, authorizer.input.Agent.ID)
				}
				assert.Equal(t, want, gate.invocations[0].Identity)
				assert.Equal(t, "mcp-session", gate.invocations[0].MCPSessionID)
				for _, marker := range []string{"subject-token-marker", "provider-token-marker", "permit-principal", "cached-principal"} {
					assert.NotContains(t, logs.String(), marker)
				}
			})
		}
	}
}

func TestExchangeErrorDoesNotClassifyPermitClientErrorsAsGrantTimestamps(t *testing.T) {
	for _, category := range []string{grantcontext.CategoryTimeout, grantcontext.CategoryUnavailable, grantcontext.CategoryRejected, grantcontext.CategoryInvalidResponse} {
		t.Run(category, func(t *testing.T) {
			var logs bytes.Buffer
			s := NewServer(&extprocconfig.Config{}, &safetyExchanger{}, slog.New(slog.NewJSONHandler(&logs, nil)))
			err := fmt.Errorf("wrapped: %w", &grantcontext.Error{Category: category})
			response, _ := s.exchangeErrorResponse(context.Background(), err, nil)
			require.NotNil(t, response.GetImmediateResponse())
			assert.Equal(t, int32(500), int32(response.GetImmediateResponse().Status.Code))
			assert.JSONEq(t, `{"error":"token_exchange_failed","error_description":"token exchange request failed"}`, string(response.GetImmediateResponse().Body))
			var record map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &record))
			assert.NotEqual(t, "extproc.grant_context.unavailable", record["event"])
		})
	}
}

type expiringApprovalAuthorizer struct {
	advance func()
}

func (a expiringApprovalAuthorizer) Evaluate(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
	a.advance()
	return &authorization.OPADecision{Action: authorization.ActionApprovalRequired}, nil
}
func (expiringApprovalAuthorizer) Stop(context.Context) {}

func TestApprovalGateIsNotConsultedForPermitExpiredDuringPolicy(t *testing.T) {
	now := time.Unix(1_000, 0)
	permit := grantcontext.Result{Deadline: now.Add(time.Second), Context: grantcontext.Context{
		Principal: "permit-principal", Agent: grantcontext.EntityRef{ID: "permit-agent"},
		Grant: grantcontext.Grant{UpdatedAt: "2026-10-26T09:59:00Z"},
	}}
	exchanger := identityExchanger{result: ExchangeResult{Token: "provider-token-marker", Principal: "permit-principal", AgentID: "permit-agent"}}
	gate := &identityGate{}
	authorizer := expiringApprovalAuthorizer{advance: func() { now = now.Add(time.Second) }}
	s := NewServerWithApprovalGate(&extprocconfig.Config{}, exchanger, authorizer, gate, slog.New(slog.DiscardHandler)).WithPermitResolver(observedGrantResolver{result: &permit})
	s.SetClock(func() time.Time { return now })
	headers := &extprocv3.HttpHeaders{Headers: &corev3.HeaderMap{Headers: []*corev3.HeaderValue{{Key: ":method", RawValue: []byte("POST")}, {Key: "mcp-session-id", RawValue: []byte("mcp-session")}}}}
	_, state := s.processRequestHeadersOPA(context.Background(), false, safetyRequest("subject-token-marker", "https://resource.example", headers), headers)
	require.NotNil(t, state)

	body := &extprocv3.HttpBody{Body: []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_files"}}`), EndOfStream: true}
	response := s.processRequestBody(context.Background(), state, body)

	require.NotNil(t, response.GetImmediateResponse())
	assert.Equal(t, int32(503), int32(response.GetImmediateResponse().Status.Code))
	assert.JSONEq(t, grantContextUnavailableBody, string(response.GetImmediateResponse().Body))
	assert.Empty(t, gate.invocations, "an expired permit must not consume a one-time approval")
}

type expiringConsumeGate struct {
	advance func()
	calls   int
}

func (g *expiringConsumeGate) Evaluate(context.Context, approval.Invocation) approval.Outcome {
	g.calls++
	g.advance()
	return approval.Outcome{Proceed: true}
}

func TestGrantedApprovalForwardsWhenPermitExpiresDuringGate(t *testing.T) {
	now := time.Unix(1_000, 0)
	permit := grantcontext.Result{Deadline: now.Add(2 * time.Second), Context: grantcontext.Context{
		Principal: "permit-principal", Agent: grantcontext.EntityRef{ID: "permit-agent"},
		Grant: grantcontext.Grant{UpdatedAt: "2026-10-26T09:59:00Z"},
	}}
	exchanger := identityExchanger{result: ExchangeResult{Token: "provider-token-marker", Principal: "permit-principal", AgentID: "permit-agent"}}
	gate := &expiringConsumeGate{advance: func() { now = now.Add(5 * time.Second) }}
	authorizer := expiringApprovalAuthorizer{advance: func() {}}
	s := NewServerWithApprovalGate(&extprocconfig.Config{}, exchanger, authorizer, gate, slog.New(slog.DiscardHandler)).WithPermitResolver(observedGrantResolver{result: &permit})
	s.SetClock(func() time.Time { return now })
	headers := &extprocv3.HttpHeaders{Headers: &corev3.HeaderMap{Headers: []*corev3.HeaderValue{{Key: ":method", RawValue: []byte("POST")}, {Key: "mcp-session-id", RawValue: []byte("mcp-session")}}}}
	_, state := s.processRequestHeadersOPA(context.Background(), false, safetyRequest("subject-token-marker", "https://resource.example", headers), headers)
	require.NotNil(t, state)

	body := &extprocv3.HttpBody{Body: []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_files"}}`), EndOfStream: true}
	response := s.processRequestBody(context.Background(), state, body)

	require.Equal(t, 1, gate.calls)
	require.Nil(t, response.GetImmediateResponse(), "a granted, possibly consumed approval must not be lost to a permit that expired during the gate")
	require.NotNil(t, response.GetRequestBody())
}
