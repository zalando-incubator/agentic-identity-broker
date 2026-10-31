package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/delegation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

const consentURL = "https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"

type stubResolver struct {
	calls   atomic.Int32
	resolve func() (delegation.Result, error)
}

func (r *stubResolver) Resolve(_ context.Context, _, _ string) (delegation.Result, bool, error) {
	r.calls.Add(1)
	result, err := r.resolve()
	return result, false, err
}

func (r *stubResolver) Shutdown() {}

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func samplePermit(deadline time.Time) delegation.Result {
	return delegation.Result{Deadline: deadline, Context: delegation.Context{
		Principal: "alice@example.com",
		Agent:     delegation.EntityRef{ID: "agent-uuid", CanonicalID: "data-assistant"},
		Resource:  delegation.Resource{ID: "svc-uuid", CanonicalID: "repo-api", ResourceURI: "https://api.example.com/repos", Scopes: []string{"read:repo"}},
		Grant:     delegation.Grant{ID: "grant-uuid", UpdatedAt: "2026-10-26T09:59:00Z"},
		PermissionSets: []delegation.PermissionSet{{ID: "ps-uuid", CanonicalID: "repo-reader",
			Services: []delegation.Service{{ID: "svc-uuid", CanonicalID: "repo-api", Scopes: []string{"read:repo"}}}}},
	}}
}

type delegationHarness struct {
	client     extprocv3.ExternalProcessorClient
	clock      *testClock
	resolver   *stubResolver
	exchanges  atomic.Int32
	policies   atomic.Int32
	lastInput  atomic.Value
	exchangeFn func() (server.ExchangeResult, error)
	policyFn   func(input authorization.OPAInput) (*authorization.OPADecision, error)
}

func newDelegationHarness(t *testing.T, resolve func() (delegation.Result, error)) *delegationHarness {
	t.Helper()
	h := &delegationHarness{
		clock:    &testClock{now: time.Unix(1_000, 0)},
		resolver: &stubResolver{resolve: resolve},
		exchangeFn: func() (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged", GrantedPermissionSets: map[string][]string{"from-exchange": {"x"}},
				Principal: "alice@example.com", AgentID: "agent-uuid"}, nil
		},
		policyFn: func(authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: authorization.ActionAllow}, nil
		},
	}
	exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
		h.exchanges.Add(1)
		return h.exchangeFn()
	}}
	authorizer := &mockAuthorizer{evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
		h.policies.Add(1)
		raw, err := json.Marshal(input["context"])
		require.NoError(t, err)
		h.lastInput.Store(string(raw))
		return h.policyFn(input)
	}}
	cfg := testConfig()
	cfg.Authorization.Enabled = true
	cfg.Authorization.MaxBodySize = 1 << 20
	cfg.Sessions.Extraction.HTTPHeader = "Mcp-Session-Id"
	srv := server.NewServerWithAuthorizer(cfg, exchanger, authorizer, testLogger()).WithDelegationResolver(h.resolver)
	srv.SetClock(h.clock.Now)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcSrv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(listener) }()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
		grpcSrv.GracefulStop()
	})
	h.client = extprocv3.NewExternalProcessorClient(conn)
	return h
}

func (h *delegationHarness) permitFor(d time.Duration) func() (delegation.Result, error) {
	return func() (delegation.Result, error) { return samplePermit(h.clock.Now().Add(d)), nil }
}

func immediate(t *testing.T, response *extprocv3.ProcessingResponse) (int, map[string]string) {
	t.Helper()
	ir, ok := response.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "expected an ImmediateResponse, got %T", response.Response)
	var body map[string]string
	require.NoError(t, json.Unmarshal(ir.ImmediateResponse.Body, &body))
	return int(ir.ImmediateResponse.Status.Code), body
}

var mcpCall = []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_files"}}`)

func TestDelegation_BodyPathGivesPolicyPermitDerivedDirectMembers(t *testing.T) {
	var h *delegationHarness
	h = newDelegationHarness(t, func() (delegation.Result, error) { return h.permitFor(time.Minute)() })

	headersResp, bodyResp := sendHeadersThenBodyWithMetadata(t, h.client, map[string]string{":method": "POST", "mcp-session-id": "sess-1"},
		validTokenExchangeMetadata("mcp"), mcpCall)

	assert.Equal(t, "Bearer exchanged", requireAuthorizationMutation(t, headersResp))
	require.NotNil(t, bodyResp)
	_, allowed := bodyResp.Response.(*extprocv3.ProcessingResponse_RequestBody)
	assert.True(t, allowed)
	assert.Equal(t, int32(1), h.resolver.calls.Load())
	assert.Equal(t, int32(1), h.exchanges.Load())
	assert.JSONEq(t, `{
		"granted_permission_sets_available": true,
		"granted_permission_sets": {"ps-uuid": ["svc-uuid"]},
		"agent_session_id": "sess-1",
		"principal": "alice@example.com",
		"agent": {"id": "agent-uuid", "canonical_id": "data-assistant"},
		"resource": {"id": "svc-uuid", "canonical_id": "repo-api", "resource_uri": "https://api.example.com/repos", "scopes": ["read:repo"]},
		"grant": {"id": "grant-uuid", "updated_at": "2026-10-26T09:59:00Z"},
		"permission_sets": [{"id": "ps-uuid", "canonical_id": "repo-reader",
			"services": [{"id": "svc-uuid", "canonical_id": "repo-api", "scopes": ["read:repo"]}]}]
	}`, h.lastInput.Load().(string))
}

func TestDelegation_BodyPathPrecedence(t *testing.T) {
	denied := &delegation.DeniedError{Code: "grant_missing", ConsentURL: consentURL}
	unavailable := &delegation.Error{Category: delegation.CategoryUnavailable}
	exchangeErr := errors.New("broker exchange failed")
	tests := []struct {
		name        string
		resolveErr  error
		exchangeErr error
		status      int
		errorCode   string
	}{
		{"denial wins over exchange error", denied, exchangeErr, 403, "access_denied"},
		{"resolution error wins over exchange error", unavailable, exchangeErr, 503, "service_unavailable"},
		{"exchange error after permit", nil, exchangeErr, 500, ""},
	}
	for _, tt := range tests {
		for _, resolverFirst := range []bool{true, false} {
			t.Run(tt.name, func(t *testing.T) {
				resolverDone := make(chan struct{})
				var h *delegationHarness
				h = newDelegationHarness(t, func() (delegation.Result, error) {
					if !resolverFirst {
						time.Sleep(20 * time.Millisecond)
					}
					defer close(resolverDone)
					if tt.resolveErr != nil {
						return delegation.Result{}, tt.resolveErr
					}
					return h.permitFor(time.Minute)()
				})
				h.exchangeFn = func() (server.ExchangeResult, error) {
					if resolverFirst {
						<-resolverDone
					}
					return server.ExchangeResult{}, tt.exchangeErr
				}

				headersResp, bodyResp := sendHeadersThenBodyWithMetadata(t, h.client, map[string]string{":method": "POST"}, validTokenExchangeMetadata("mcp"), mcpCall)

				assert.Nil(t, bodyResp)
				status, body := immediate(t, headersResp)
				assert.Equal(t, int32(1), h.exchanges.Load(), "both calls complete on the body path")
				assert.Equal(t, int32(1), h.resolver.calls.Load())
				assert.Zero(t, h.policies.Load())
				if tt.errorCode == "" {
					assert.GreaterOrEqual(t, status, 500)
					return
				}
				assert.Equal(t, tt.status, status)
				assert.Equal(t, tt.errorCode, body["error"])
				if tt.status == 403 {
					assert.Equal(t, "delegation denied: grant_missing", body["error_description"])
					assert.Equal(t, consentURL, body["error_uri"])
				} else {
					assert.Equal(t, "delegation context unavailable", body["error_description"])
				}
			})
		}
	}
}

func TestDelegation_BodyPathRejectsPermitExpiredBeforePolicy(t *testing.T) {
	start := time.Unix(1_000, 0)
	h := newDelegationHarness(t, func() (delegation.Result, error) { return samplePermit(start.Add(time.Second)), nil })
	h.exchangeFn = func() (server.ExchangeResult, error) {
		h.clock.Advance(time.Second)
		return server.ExchangeResult{Token: "exchanged"}, nil
	}

	_, bodyResp := sendHeadersThenBodyWithMetadata(t, h.client, map[string]string{":method": "POST"}, validTokenExchangeMetadata("mcp"), mcpCall)

	require.NotNil(t, bodyResp)
	status, body := immediate(t, bodyResp)
	assert.Equal(t, 503, status)
	assert.Equal(t, "delegation context unavailable", body["error_description"])
	assert.Zero(t, h.policies.Load(), "an expired permit never reaches policy")
	assert.Equal(t, int32(1), h.resolver.calls.Load(), "no re-resolution")
}

func TestDelegation_BatchRejectsPermitExpiringBetweenDecisions(t *testing.T) {
	var h *delegationHarness
	h = newDelegationHarness(t, func() (delegation.Result, error) { return h.permitFor(2 * time.Second)() })
	h.policyFn = func(authorization.OPAInput) (*authorization.OPADecision, error) {
		h.clock.Advance(2 * time.Second)
		return &authorization.OPADecision{Action: authorization.ActionAllow}, nil
	}
	batch := []byte(`[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"a"}},{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"b"}}]`)

	_, bodyResp := sendHeadersThenBodyWithMetadata(t, h.client, map[string]string{":method": "POST"}, validTokenExchangeMetadata("mcp"), batch)

	require.NotNil(t, bodyResp)
	status, _ := immediate(t, bodyResp)
	assert.Equal(t, 503, status)
	assert.Equal(t, int32(1), h.policies.Load(), "the expired second decision is not evaluated")
}

func TestDelegation_HeaderOnlyOrdering(t *testing.T) {
	t.Run("resolve, then policy, then exchange", func(t *testing.T) {
		var h *delegationHarness
		h = newDelegationHarness(t, func() (delegation.Result, error) {
			assert.Zero(t, h.policies.Load())
			return h.permitFor(time.Minute)()
		})
		h.policyFn = func(authorization.OPAInput) (*authorization.OPADecision, error) {
			assert.Zero(t, h.exchanges.Load(), "policy runs before exchange")
			return &authorization.OPADecision{Action: authorization.ActionAllow}, nil
		}
		resp, err := sendRequestHeadersWithMetadata(t, h.client, map[string]string{":method": "GET"}, validTokenExchangeMetadata("mcp"), true)
		require.NoError(t, err)
		assert.Equal(t, "Bearer exchanged", requireAuthorizationMutation(t, resp))
		var ctxInput map[string]any
		require.NoError(t, json.Unmarshal([]byte(h.lastInput.Load().(string)), &ctxInput))
		assert.Equal(t, true, ctxInput["granted_permission_sets_available"])
		assert.Equal(t, "alice@example.com", ctxInput["principal"])
		for _, forbidden := range []string{"delegation", "delegation_available", "subject"} {
			assert.NotContains(t, ctxInput, forbidden)
		}
	})

	t.Run("policy deny makes no exchange call", func(t *testing.T) {
		var h *delegationHarness
		h = newDelegationHarness(t, func() (delegation.Result, error) { return h.permitFor(time.Minute)() })
		h.policyFn = func(authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "deny", Reasons: []string{"not repo-reader"}}, nil
		}
		resp, err := sendRequestHeadersWithMetadata(t, h.client, map[string]string{":method": "GET"}, validTokenExchangeMetadata("mcp"), true)
		require.NoError(t, err)
		status, _ := immediate(t, resp)
		assert.Equal(t, 403, status)
		assert.Zero(t, h.exchanges.Load())
	})

	for name, tc := range map[string]struct {
		resolve func(h *delegationHarness) (delegation.Result, error)
		status  int
	}{
		"denial": {func(*delegationHarness) (delegation.Result, error) {
			return delegation.Result{}, &delegation.DeniedError{Code: "grant_expired", ConsentURL: consentURL}
		}, 403},
		"resolution error": {func(*delegationHarness) (delegation.Result, error) {
			return delegation.Result{}, &delegation.Error{Category: delegation.CategoryTimeout}
		}, 503},
		"expired permit": {func(h *delegationHarness) (delegation.Result, error) { return samplePermit(h.clock.Now()), nil }, 503},
	} {
		t.Run(name+" stops before policy and exchange", func(t *testing.T) {
			var h *delegationHarness
			h = newDelegationHarness(t, func() (delegation.Result, error) { return tc.resolve(h) })
			resp, err := sendRequestHeadersWithMetadata(t, h.client, map[string]string{":method": "GET"}, validTokenExchangeMetadata("mcp"), true)
			require.NoError(t, err)
			status, body := immediate(t, resp)
			assert.Equal(t, tc.status, status)
			assert.Zero(t, h.policies.Load())
			assert.Zero(t, h.exchanges.Load())
			assert.Equal(t, int32(1), h.resolver.calls.Load())
			assert.NotContains(t, body["error_description"], testSubjectToken)
		})
	}
}

func TestContextInputFromDelegation_DeepCopiesAndIgnoresExchange(t *testing.T) {
	result := samplePermit(time.Now().Add(time.Minute))
	input := server.ContextInputFromDelegation(&result, "sess-1")

	input.Resource.Scopes[0] = "mutated"
	input.PermissionSets[0].Services[0].Scopes[0] = "mutated"
	input.GrantedPermissionSets["ps-uuid"][0] = "mutated"

	assert.Equal(t, "read:repo", result.Context.Resource.Scopes[0])
	assert.Equal(t, "read:repo", result.Context.PermissionSets[0].Services[0].Scopes[0])
	assert.Equal(t, "svc-uuid", result.Context.PermissionSets[0].Services[0].ID)
	assert.True(t, input.GrantedPermissionSetsAvailable)
	assert.Equal(t, "sess-1", input.AgentSessionID)
}

func TestDelegation_DisabledModeMakesNoResolverCallsAndKeepsInput(t *testing.T) {
	var captured atomic.Value
	authorizer := &mockAuthorizer{evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
		raw, _ := json.Marshal(input["context"])
		captured.Store(string(raw))
		return &authorization.OPADecision{Action: authorization.ActionAllow}, nil
	}}
	cfg := testConfig()
	cfg.Sessions.Extraction.HTTPHeader = "Mcp-Session-Id"
	client, cleanup := startTestServerWithAuthorizerConfig(t, cfg, &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
		return server.ExchangeResult{Token: "t"}, nil
	}}, authorizer)
	defer cleanup()

	_, err := sendRequestHeadersWithMetadata(t, client, map[string]string{":method": "GET", "mcp-session-id": "s"}, validTokenExchangeMetadata("mcp"), true)
	require.NoError(t, err)
	assert.Equal(t, `{"granted_permission_sets_available":false,"agent_session_id":"s"}`, captured.Load())
}
