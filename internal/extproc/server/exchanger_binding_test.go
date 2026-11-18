package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/grantcontext"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

func bindingExchanger(t *testing.T, respond func(http.ResponseWriter, int32), enabled ...bool) (*server.TokenExchanger, *atomic.Int32) {
	t.Helper()
	return bindingConfiguredExchanger(t, func(w http.ResponseWriter, _ *http.Request, call int32) { respond(w, call) },
		func(cfg *extprocconfig.Config) { cfg.GrantContext.Enabled = len(enabled) == 0 || enabled[0] })
}

func bindingConfiguredExchanger(t *testing.T, respond func(http.ResponseWriter, *http.Request, int32), configure func(*extprocconfig.Config)) (*server.TokenExchanger, *atomic.Int32) {
	t.Helper()
	mocks := newMockServers()
	t.Cleanup(mocks.Close)
	mocks.tokenExchServer.Close()
	calls := &atomic.Int32{}
	mocks.tokenExchServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		respond(w, r, calls.Add(1))
	}))
	cfg := configForMocks(mocks)
	cfg.GrantContext.Enabled = true
	configure(cfg)
	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	t.Cleanup(exchanger.Shutdown)
	return exchanger, calls
}

func bindingSuccess(w http.ResponseWriter, token string, expiry int) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": token, "token_type": "Bearer", "expires_in": expiry,
		"principal": "alice@example.com", "agent_id": "agent-uuid",
		"granted_permission_sets": map[string][]string{"from-exchange": {"x"}},
	})
}

type bindingCall struct {
	headers, body *extprocv3.ProcessingResponse
	err           error
}

// callBindingHarness makes no assertions, so a worker goroutine can run it and hand the result to the test goroutine.
func callBindingHarness(h *grantContextHarness, headerOnly bool) bindingCall {
	method := "POST"
	if headerOnly {
		method = "GET"
	}
	// The deadline ends a stream whose response never arrives; the error is returned in bindingCall.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := h.client.Process(ctx)
	if err != nil {
		return bindingCall{err: err}
	}
	if err := stream.Send(&extprocv3.ProcessingRequest{
		MetadataContext: validTokenExchangeMetadata("mcp"),
		Request:         &extprocv3.ProcessingRequest_RequestHeaders{RequestHeaders: extProcHeaders(map[string]string{":method": method}, headerOnly)},
	}); err != nil {
		return bindingCall{err: err}
	}
	if headerOnly {
		_ = stream.CloseSend()
	}
	headers, err := stream.Recv()
	if err != nil || headerOnly || headers.GetImmediateResponse() != nil {
		_ = stream.CloseSend()
		return bindingCall{headers: headers, err: err}
	}
	if err := stream.Send(&extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestBody{RequestBody: &extprocv3.HttpBody{Body: mcpCall, EndOfStream: true}},
	}); err != nil {
		return bindingCall{headers: headers, err: err}
	}
	_ = stream.CloseSend()
	body, err := stream.Recv()
	return bindingCall{headers: headers, body: body, err: err}
}

func requireBindingCall(t *testing.T, call bindingCall, headerOnly bool) *extprocv3.ProcessingResponse {
	t.Helper()
	require.NoError(t, call.err)
	if headerOnly {
		return call.headers
	}
	if call.headers.GetImmediateResponse() == nil {
		require.NotNil(t, call.body)
		require.NotNil(t, call.body.GetRequestBody())
	} else {
		require.Nil(t, call.body)
	}
	return call.headers
}

func bindingHarnessCall(t *testing.T, h *grantContextHarness, headerOnly bool) *extprocv3.ProcessingResponse {
	t.Helper()
	return requireBindingCall(t, callBindingHarness(h, headerOnly), headerOnly)
}

func TestExchangeResultBinding_ReuseComparesScopeSetsAndGrantInstants(t *testing.T) {
	tests := []struct {
		name      string
		scopes    []string
		updatedAt string
		wantCalls int32
	}{
		{"same permit", []string{"read:repo", "write:repo"}, "2026-10-26T09:59:00Z", 1},
		{"reordered and duplicate scopes", []string{"write:repo", "read:repo", "read:repo"}, "2026-10-26T09:59:00Z", 1},
		{"same timestamp instant", []string{"read:repo", "write:repo"}, "2026-10-26T11:59:00+02:00", 1},
		{"same instant with fractional zero", []string{"read:repo", "write:repo"}, "2026-10-26T09:59:00.000000000Z", 1},
		{"changed scopes only", []string{"read:repo"}, "2026-10-26T09:59:00Z", 2},
		{"changed grant only", []string{"read:repo", "write:repo"}, "2026-10-26T09:59:01Z", 2},
		{"empty scope set", []string{}, "2026-10-26T09:59:00Z", 2},
	}
	for _, tt := range tests {
		for _, headerOnly := range []bool{false, true} {
			path := "body"
			if headerOnly {
				path = "header-only"
			}
			t.Run(tt.name+"/"+path, func(t *testing.T) {
				exchanger, calls := bindingExchanger(t, func(w http.ResponseWriter, call int32) {
					token := "SYNTHETIC_OLD_RESULT"
					if call > 1 {
						token = "SYNTHETIC_NEW_RESULT"
					}
					bindingSuccess(w, token, 3600)
				})
				permit := samplePermit(time.Unix(1_000, 0).Add(time.Minute))
				permit.Context.Resource.Scopes = []string{"read:repo", "write:repo"}
				h := newGrantContextHarnessWithExchanger(t, func() (grantcontext.Result, error) { return permit, nil }, exchanger)
				assert.Equal(t, "Bearer SYNTHETIC_OLD_RESULT", requireAuthorizationMutation(t, bindingHarnessCall(t, h, headerOnly)))
				permit.Context.Resource.Scopes = tt.scopes
				permit.Context.Grant.UpdatedAt = tt.updatedAt
				response := bindingHarnessCall(t, h, headerOnly)
				token := "Bearer SYNTHETIC_OLD_RESULT"
				if tt.wantCalls == 2 {
					token = "Bearer SYNTHETIC_NEW_RESULT"
				}
				assert.Equal(t, token, requireAuthorizationMutation(t, response))
				assert.Equal(t, tt.wantCalls, calls.Load(), "equal bindings reuse; different bindings exchange exactly once more")
				require.Equal(t, int32(2), h.policies.Load())
				assert.Equal(t, int32(2), h.resolver.calls.Load())
				assert.Contains(t, requirePolicyInput(t, h), tt.updatedAt)
			})
		}
	}
}

func TestExchangeResultBinding_IdentityMismatchFailsClosed(t *testing.T) {
	for _, identity := range []struct {
		name, principal, agentID string
	}{
		{"principal mismatch", "another-principal", "agent-uuid"},
		{"agent mismatch", "alice@example.com", "another-agent"},
		{"absent principal", "", "agent-uuid"},
		{"absent agent", "alice@example.com", ""},
	} {
		for _, headerOnly := range []bool{false, true} {
			t.Run(identity.name+"/"+map[bool]string{false: "body", true: "header-only"}[headerOnly], func(t *testing.T) {
				exchanger, calls := bindingExchanger(t, func(w http.ResponseWriter, _ int32) {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"access_token": "SYNTHETIC_WRONG_IDENTITY", "expires_in": 3600,
						"principal": identity.principal, "agent_id": identity.agentID,
					})
				})
				permit := samplePermit(time.Unix(1_000, 0).Add(time.Minute))
				h := newGrantContextHarnessWithExchanger(t, func() (grantcontext.Result, error) { return permit, nil }, exchanger)
				response := bindingHarnessCall(t, h, headerOnly)
				status, body := immediate(t, response)
				assert.Equal(t, 503, status)
				assert.Equal(t, map[string]string{"error": "service_unavailable", "error_description": "exchange result binding unavailable"}, body)
				assert.Nil(t, response.GetRequestHeaders(), "no credential mutation or body buffering")
				assert.Equal(t, int32(1), calls.Load(), "identity mismatch is terminal, not a grant-change retry")
				wantPolicies := int32(0)
				if headerOnly {
					wantPolicies = 1
				}
				assert.Equal(t, wantPolicies, h.policies.Load())
			})
		}
	}
}

type observedExchange struct {
	inner     server.Exchanger
	delivered chan struct{}
	once      sync.Once
}

func (e *observedExchange) Exchange(ctx context.Context, subject, resource string) (server.ExchangeResult, error) {
	result, err := e.inner.Exchange(ctx, subject, resource)
	e.once.Do(func() { close(e.delivered) })
	return result, err
}

func (e *observedExchange) ExchangeBound(ctx context.Context, subject, resource string, observed *server.ExchangeResult, binding server.ExchangeResultBinding) (server.ExchangeResult, error) {
	return e.inner.ExchangeBound(ctx, subject, resource, observed, binding)
}

func (e *observedExchange) Shutdown() {}

func awaitBindingBarrier(t *testing.T, barrier <-chan struct{}) {
	t.Helper()
	select {
	case <-barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("binding fixture did not reach its synchronization barrier")
	}
}

func TestExchangeResultBinding_PreservesIdenticalTokenReplacementAndDetachedDelivery(t *testing.T) {
	tests := []struct {
		name      string
		updatedAt string
		wantCalls int32
	}{
		{"compatible replacement is reused", "2026-10-26T10:00:00Z", 2},
		{"incompatible replacement requires one exchange", "2026-10-26T10:01:00Z", 3},
		{"matching detached result remains usable", "2026-10-26T09:59:00Z", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exchanger, calls := bindingExchanger(t, func(w http.ResponseWriter, _ int32) {
				bindingSuccess(w, "SYNTHETIC_IDENTICAL_PROVIDER_TOKEN", 3600)
			})
			original := samplePermit(time.Unix(1_000, 0).Add(time.Minute))
			warm := newGrantContextHarnessWithExchanger(t, func() (grantcontext.Result, error) { return original, nil }, exchanger)
			require.Equal(t, "Bearer SYNTHETIC_IDENTICAL_PROVIDER_TOKEN", requireAuthorizationMutation(t, bindingHarnessCall(t, warm, false)))
			require.Equal(t, int32(1), calls.Load())

			resolved := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			observed := &observedExchange{inner: exchanger, delivered: make(chan struct{})}
			waitingPermit := original
			waitingPermit.Context.Grant.UpdatedAt = tt.updatedAt
			waiting := newGrantContextHarnessWithExchanger(t, func() (grantcontext.Result, error) {
				close(resolved)
				<-release
				return waitingPermit, nil
			}, observed)
			t.Cleanup(unblock)
			response := make(chan bindingCall, 1)
			go func() { response <- callBindingHarness(waiting, false) }()
			awaitBindingBarrier(t, resolved)
			awaitBindingBarrier(t, observed.delivered)

			successorPermit := original
			successorPermit.Context.Grant.UpdatedAt = "2026-10-26T10:00:00Z"
			successor := newGrantContextHarnessWithExchanger(t, func() (grantcontext.Result, error) { return successorPermit, nil }, exchanger)
			assert.Equal(t, "Bearer SYNTHETIC_IDENTICAL_PROVIDER_TOKEN", requireAuthorizationMutation(t, bindingHarnessCall(t, successor, true)))
			assert.Equal(t, int32(2), calls.Load(), "a changed binding installs a new generation even when token bytes repeat")
			unblock()
			select {
			case result := <-response:
				assert.Equal(t, "Bearer SYNTHETIC_IDENTICAL_PROVIDER_TOKEN", requireAuthorizationMutation(t, requireBindingCall(t, result, false)))
			case <-time.After(5 * time.Second):
				t.Fatal("request holding the detached result did not complete")
			}
			assert.Equal(t, tt.wantCalls, calls.Load(), "compare-and-delete must preserve the replacement identity")
			assert.Equal(t, int32(1), waiting.resolver.calls.Load())
			require.Equal(t, int32(1), waiting.policies.Load())
			assert.Contains(t, requirePolicyInput(t, waiting), tt.updatedAt)
			assert.Equal(t, "Bearer SYNTHETIC_IDENTICAL_PROVIDER_TOKEN", requireAuthorizationMutation(t, bindingHarnessCall(t, successor, true)))
			wantAfterSuccessor := tt.wantCalls
			if tt.wantCalls == 3 {
				wantAfterSuccessor++
			}
			assert.Equal(t, wantAfterSuccessor, calls.Load(), "the successor remains reusable unless replaced by the later incompatible permit")
		})
	}
}

func TestExchangeResultBinding_NonCacheableSuccessPreservesZeroLifetimeAndExchangeIdentity(t *testing.T) {
	for _, headerOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "body", true: "header-only"}[headerOnly], func(t *testing.T) {
			exchanger, calls := bindingExchanger(t, func(w http.ResponseWriter, _ int32) {
				bindingSuccess(w, "SYNTHETIC_NONCACHEABLE_TOKEN", 0)
			})
			permit := samplePermit(time.Unix(1_000, 0).Add(time.Minute))
			h := newGrantContextHarnessWithExchanger(t, func() (grantcontext.Result, error) { return permit, nil }, exchanger)
			for i := range 2 {
				assert.Equal(t, "Bearer SYNTHETIC_NONCACHEABLE_TOKEN", requireAuthorizationMutation(t, bindingHarnessCall(t, h, headerOnly)))
				assert.Equal(t, int32(i+1), calls.Load(), "binding does not create a cache lifetime for expires_in=0")
			}
			result, err := exchanger.Exchange(context.Background(), "subject", "https://api.example.com/repos")
			require.NoError(t, err)
			assert.Equal(t, "alice@example.com", result.Principal)
			assert.Equal(t, "agent-uuid", result.AgentID)
			assert.Equal(t, map[string][]string{"from-exchange": {"x"}}, result.GrantedPermissionSets)
			assert.Equal(t, int32(3), calls.Load())
		})
	}
}

func TestExchangeResultBinding_NonCacheableRetryPreservesCompetingReplacement(t *testing.T) {
	exchanger, calls := bindingExchanger(t, func(w http.ResponseWriter, call int32) {
		switch call {
		case 1:
			bindingSuccess(w, "SYNTHETIC_DETACHED_RESULT", 0)
		case 2:
			bindingSuccess(w, "SYNTHETIC_COMPETING_REPLACEMENT", 3600)
		default:
			bindingSuccess(w, "SYNTHETIC_NONCACHEABLE_RETRY", 0)
		}
	})
	ctx := context.Background()
	const subject, resource = "subject", "https://api.example.com/repos"
	permit := func(updatedAt time.Time) server.ExchangeResultBinding {
		return server.ExchangeResultBinding{Principal: "alice@example.com", AgentID: "agent-uuid", Scopes: []string{"read:repo"}, GrantUpdatedAt: updatedAt}
	}
	bound := permit(time.Date(2026, 10, 26, 9, 59, 0, 0, time.UTC))
	other := permit(time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC))

	detached, err := exchanger.Exchange(ctx, subject, resource)
	require.NoError(t, err)
	_, err = exchanger.ExchangeBound(ctx, subject, resource, &detached, bound)
	require.NoError(t, err)
	replacement, err := exchanger.Exchange(ctx, subject, resource)
	require.NoError(t, err)
	_, err = exchanger.ExchangeBound(ctx, subject, resource, &replacement, bound)
	require.NoError(t, err)

	retried, err := exchanger.ExchangeBound(ctx, subject, resource, &detached, other)
	require.NoError(t, err)
	assert.Equal(t, "SYNTHETIC_NONCACHEABLE_RETRY", retried.Token)
	require.Equal(t, int32(3), calls.Load())

	reused, err := exchanger.Exchange(ctx, subject, resource)
	require.NoError(t, err)
	assert.Equal(t, "SYNTHETIC_COMPETING_REPLACEMENT", reused.Token, "a non-cacheable retry must not delete the preserved replacement")
	assert.Equal(t, int32(3), calls.Load())
}

func TestExchangeResultBinding_DisabledExchangeRetainsCacheSemantics(t *testing.T) {
	exchanger, calls := bindingExchanger(t, func(w http.ResponseWriter, _ int32) { bindingSuccess(w, "SYNTHETIC_OLD_RESULT", 3600) }, false)
	first, err := exchanger.Exchange(context.Background(), "subject", "https://api.example.com/repos")
	require.NoError(t, err)
	second, err := exchanger.Exchange(context.Background(), "subject", "https://api.example.com/repos")
	require.NoError(t, err)
	for _, result := range []server.ExchangeResult{first, second} {
		assert.Equal(t, "SYNTHETIC_OLD_RESULT", result.Token)
		assert.Equal(t, "alice@example.com", result.Principal, "the broker fixture's principal survives exchange and cache hit")
		assert.Equal(t, "agent-uuid", result.AgentID, "the broker fixture's agent ID survives exchange and cache hit")
		assert.Equal(t, map[string][]string{"from-exchange": {"x"}}, result.GrantedPermissionSets)
	}
	assert.Equal(t, int32(1), calls.Load())
}
