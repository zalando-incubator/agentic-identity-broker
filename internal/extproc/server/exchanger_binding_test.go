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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/delegation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

func bindingExchanger(t *testing.T, respond func(http.ResponseWriter, int32), enabled ...bool) (*server.TokenExchanger, *atomic.Int32) {
	t.Helper()
	return bindingConfiguredExchanger(t, func(w http.ResponseWriter, _ *http.Request, call int32) { respond(w, call) },
		func(cfg *extprocconfig.Config) { cfg.DelegationContext.Enabled = len(enabled) == 0 || enabled[0] })
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
	cfg.DelegationContext.Enabled = true
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

func bindingHarnessCall(t *testing.T, h *delegationHarness, headerOnly bool) *extprocv3.ProcessingResponse {
	t.Helper()
	if headerOnly {
		response, err := sendRequestHeadersWithMetadata(t, h.client, map[string]string{":method": "GET"}, validTokenExchangeMetadata("mcp"), true)
		require.NoError(t, err)
		return response
	}
	headers, body := sendHeadersThenBodyWithMetadata(t, h.client, map[string]string{":method": "POST"}, validTokenExchangeMetadata("mcp"), mcpCall)
	if headers.GetImmediateResponse() == nil {
		require.NotNil(t, body)
		require.NotNil(t, body.GetRequestBody())
	} else {
		require.Nil(t, body)
	}
	return headers
}

func TestTokenBinding_ReuseComparesScopeSetsAndGrantInstants(t *testing.T) {
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
				h := newDelegationHarnessWithExchanger(t, func() (delegation.Result, error) { return permit, nil }, exchanger)
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
				assert.Equal(t, int32(2), h.policies.Load())
				assert.Equal(t, int32(2), h.resolver.calls.Load())
				assert.Contains(t, h.lastInput.Load().(string), tt.updatedAt)
			})
		}
	}
}

func TestTokenBinding_FailedReplacementCannotForwardThePriorToken(t *testing.T) {
	for _, headerOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "body", true: "header-only"}[headerOnly], func(t *testing.T) {
			exchanger, calls := bindingExchanger(t, func(w http.ResponseWriter, call int32) {
				if call == 1 {
					bindingSuccess(w, "SYNTHETIC_OLD_RESULT", 3600)
					return
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"temporarily_unavailable"}`))
			})
			permit := samplePermit(time.Unix(1_000, 0).Add(time.Minute))
			h := newDelegationHarnessWithExchanger(t, func() (delegation.Result, error) { return permit, nil }, exchanger)
			require.Equal(t, "Bearer SYNTHETIC_OLD_RESULT", requireAuthorizationMutation(t, bindingHarnessCall(t, h, headerOnly)))
			permit.Context.Grant.UpdatedAt = "2026-10-26T10:00:00Z"
			response := bindingHarnessCall(t, h, headerOnly)
			status, _ := immediate(t, response)
			assert.GreaterOrEqual(t, status, 500)
			assert.Nil(t, response.GetRequestHeaders(), "a failed replacement emits no Authorization mutation")
			assert.Equal(t, int32(2), calls.Load())
		})
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

func (e *observedExchange) ExchangeBound(ctx context.Context, subject, resource string, observed *server.ExchangeResult, binding server.TokenBinding) (server.ExchangeResult, error) {
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

func TestTokenBinding_PreservesIdenticalTokenReplacementAndDetachedDelivery(t *testing.T) {
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
			warm := newDelegationHarnessWithExchanger(t, func() (delegation.Result, error) { return original, nil }, exchanger)
			require.Equal(t, "Bearer SYNTHETIC_IDENTICAL_PROVIDER_TOKEN", requireAuthorizationMutation(t, bindingHarnessCall(t, warm, false)))
			require.Equal(t, int32(1), calls.Load())

			resolved := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			observed := &observedExchange{inner: exchanger, delivered: make(chan struct{})}
			waitingPermit := original
			waitingPermit.Context.Grant.UpdatedAt = tt.updatedAt
			waiting := newDelegationHarnessWithExchanger(t, func() (delegation.Result, error) {
				close(resolved)
				<-release
				return waitingPermit, nil
			}, observed)
			t.Cleanup(unblock)
			response := make(chan *extprocv3.ProcessingResponse, 1)
			go func() { response <- bindingHarnessCall(t, waiting, false) }()
			awaitBindingBarrier(t, resolved)
			awaitBindingBarrier(t, observed.delivered)

			successorPermit := original
			successorPermit.Context.Grant.UpdatedAt = "2026-10-26T10:00:00Z"
			successor := newDelegationHarnessWithExchanger(t, func() (delegation.Result, error) { return successorPermit, nil }, exchanger)
			assert.Equal(t, "Bearer SYNTHETIC_IDENTICAL_PROVIDER_TOKEN", requireAuthorizationMutation(t, bindingHarnessCall(t, successor, true)))
			assert.Equal(t, int32(2), calls.Load(), "a changed binding installs a new generation even when token bytes repeat")
			unblock()
			select {
			case result := <-response:
				assert.Equal(t, "Bearer SYNTHETIC_IDENTICAL_PROVIDER_TOKEN", requireAuthorizationMutation(t, result))
			case <-time.After(5 * time.Second):
				t.Fatal("request holding the detached result did not complete")
			}
			assert.Equal(t, tt.wantCalls, calls.Load(), "compare-and-delete must preserve the replacement identity")
			assert.Equal(t, int32(1), waiting.resolver.calls.Load())
			assert.Equal(t, int32(1), waiting.policies.Load())
			assert.Contains(t, waiting.lastInput.Load().(string), tt.updatedAt)
			assert.Equal(t, "Bearer SYNTHETIC_IDENTICAL_PROVIDER_TOKEN", requireAuthorizationMutation(t, bindingHarnessCall(t, successor, true)))
			wantAfterSuccessor := tt.wantCalls
			if tt.wantCalls == 3 {
				wantAfterSuccessor++
			}
			assert.Equal(t, wantAfterSuccessor, calls.Load(), "the successor remains reusable unless replaced by the later incompatible permit")
		})
	}
}

func TestTokenBinding_NonCacheableSuccessPreservesZeroLifetimeAndExchangeIdentity(t *testing.T) {
	for _, headerOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "body", true: "header-only"}[headerOnly], func(t *testing.T) {
			exchanger, calls := bindingExchanger(t, func(w http.ResponseWriter, _ int32) {
				bindingSuccess(w, "SYNTHETIC_NONCACHEABLE_TOKEN", 0)
			})
			permit := samplePermit(time.Unix(1_000, 0).Add(time.Minute))
			h := newDelegationHarnessWithExchanger(t, func() (delegation.Result, error) { return permit, nil }, exchanger)
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

func TestTokenBinding_DisabledExchangeRetainsCacheSemantics(t *testing.T) {
	exchanger, calls := bindingExchanger(t, func(w http.ResponseWriter, _ int32) { bindingSuccess(w, "SYNTHETIC_OLD_RESULT", 3600) }, false)
	first, err := exchanger.Exchange(context.Background(), "subject", "https://api.example.com/repos")
	require.NoError(t, err)
	second, err := exchanger.Exchange(context.Background(), "subject", "https://api.example.com/repos")
	require.NoError(t, err)
	assert.Equal(t, first.Token, second.Token)
	assert.Equal(t, first.Principal, second.Principal)
	assert.Equal(t, first.AgentID, second.AgentID)
	assert.Equal(t, first.GrantedPermissionSets, second.GrantedPermissionSets)
	assert.Equal(t, int32(1), calls.Load())
}
