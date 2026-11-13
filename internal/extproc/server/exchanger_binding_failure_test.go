package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/delegation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

func TestExchangeResultBinding_ReplacementFailuresRetainExchangeErrorHandling(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		error    string
		cooldown bool
		respond  func(http.ResponseWriter, *http.Request)
		prepare  func(*server.TokenExchanger)
	}{
		{
			name: "broker unavailable", status: 500, error: "token_exchange_failed",
			respond: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"temporarily_unavailable"}`))
			},
		},
		{
			name: "exchange timeout", status: 500, error: "token_exchange_failed",
			respond: func(_ http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				<-r.Context().Done()
			},
		},
		{
			name: "malformed success", status: 500, error: "token_exchange_failed",
			respond: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"access_token":`)) },
		},
		{
			name: "circuit open", status: 503, error: "service_unavailable",
			respond: func(w http.ResponseWriter, _ *http.Request) { bindingSuccess(w, "SYNTHETIC_UNEXPECTED_RESULT", 3600) },
			prepare: func(exchanger *server.TokenExchanger) { exchanger.TripCircuitBreaker(5) },
		},
		{
			name: "reauthentication preserves cooldown", status: 403, error: "insufficient_scope", cooldown: true,
			respond: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "invalid_grant", "error_description": "reauthentication required", "error_uri": consentURL,
				})
			},
		},
	}
	for _, tt := range tests {
		for _, headerOnly := range []bool{false, true} {
			t.Run(tt.name+"/"+map[bool]string{false: "body", true: "header-only"}[headerOnly], func(t *testing.T) {
				exchanger, calls := bindingConfiguredExchanger(t, func(w http.ResponseWriter, r *http.Request, call int32) {
					if call == 1 {
						bindingSuccess(w, "SYNTHETIC_PRIOR_RESULT", 3600)
						return
					}
					tt.respond(w, r)
				}, func(cfg *extprocconfig.Config) { cfg.OAuth2.ExchangeTimeout = 100 * time.Millisecond })
				permit := samplePermit(time.Unix(1_000, 0).Add(time.Minute))
				h := newDelegationHarnessWithExchanger(t, func() (delegation.Result, error) { return permit, nil }, exchanger)
				require.Equal(t, "Bearer SYNTHETIC_PRIOR_RESULT", requireAuthorizationMutation(t, bindingHarnessCall(t, h, headerOnly)))
				if tt.prepare != nil {
					tt.prepare(exchanger)
				}
				permit.Context.Grant.UpdatedAt = "2026-10-26T10:00:00Z"
				response := bindingHarnessCall(t, h, headerOnly)
				ir := response.GetImmediateResponse()
				require.NotNil(t, ir, "a failed replacement cannot emit Authorization or buffer the body")
				assert.Equal(t, tt.status, int(ir.Status.Code))
				if tt.cooldown {
					assert.Equal(t, `Bearer error="insufficient_scope", error_description="reauthentication required", error_uri="`+consentURL+`"`,
						headerMutationValue(ir.Headers, "www-authenticate"))
					assert.Contains(t, string(ir.Body), consentURL)
				} else {
					_, body := immediate(t, response)
					assert.Equal(t, tt.error, body["error"])
				}
				assert.Nil(t, response.GetRequestHeaders())
				assert.Empty(t, headerMutationValue(ir.Headers, "authorization"))
				wantCalls := int32(2)
				if tt.prepare != nil {
					wantCalls = 1
				}
				assert.Equal(t, wantCalls, calls.Load(), "there is no third exchange or mismatched stale fallback")
				if tt.cooldown {
					cooldown := bindingHarnessCall(t, h, headerOnly).GetImmediateResponse()
					require.NotNil(t, cooldown)
					assert.Equal(t, http.StatusForbidden, int(cooldown.Status.Code))
					assert.Equal(t, headerMutationValue(ir.Headers, "www-authenticate"), headerMutationValue(cooldown.Headers, "www-authenticate"))
					assert.Empty(t, headerMutationValue(cooldown.Headers, "authorization"))
					assert.Equal(t, int32(2), calls.Load(), "the tokenless five-second cooldown still prevents repeated broker calls")
				}
			})
		}
	}
}

type sharedInitialExchange struct {
	*server.TokenExchanger
	result    server.ExchangeResult
	delivered chan struct{}
}

func (e *sharedInitialExchange) Exchange(context.Context, string, string) (server.ExchangeResult, error) {
	e.delivered <- struct{}{}
	return e.result, nil
}

func TestExchangeResultBinding_CompetingFirstUseOfSharedResults(t *testing.T) {
	for _, lifetime := range []int{3600, 0} {
		t.Run(map[int]string{3600: "cacheable", 0: "non-cacheable"}[lifetime], func(t *testing.T) {
			exchanger, calls := bindingExchanger(t, func(w http.ResponseWriter, call int32) {
				token := "SYNTHETIC_SHARED_INITIAL"
				if call > 1 {
					token = "SYNTHETIC_REPLACEMENT"
				}
				bindingSuccess(w, token, lifetime)
			})
			initial, err := exchanger.Exchange(context.Background(), testSubjectToken, testResourceURI)
			require.NoError(t, err)
			shared := &sharedInitialExchange{TokenExchanger: exchanger, result: initial, delivered: make(chan struct{}, 2)}
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			permits := []delegation.Result{
				samplePermit(time.Unix(1_000, 0).Add(time.Minute)),
				samplePermit(time.Unix(1_000, 0).Add(time.Minute)),
			}
			permits[1].Context.Resource.Scopes = []string{"read:repo", "write:repo"}
			responses := make(chan *extprocv3.ProcessingResponse, 2)
			for _, permit := range permits {
				h := newDelegationHarnessWithExchanger(t, func() (delegation.Result, error) {
					<-release
					return permit, nil
				}, shared)
				go func() { responses <- bindingHarnessCall(t, h, false) }()
			}
			t.Cleanup(unblock)
			awaitBindingBarrier(t, shared.delivered)
			awaitBindingBarrier(t, shared.delivered)
			unblock()
			tokens := make([]string, 0, 2)
			for range 2 {
				select {
				case response := <-responses:
					tokens = append(tokens, requireAuthorizationMutation(t, response))
				case <-time.After(5 * time.Second):
					t.Fatal("competing first-use requests did not complete")
				}
			}
			assert.ElementsMatch(t, []string{"Bearer SYNTHETIC_SHARED_INITIAL", "Bearer SYNTHETIC_REPLACEMENT"}, tokens,
				"only one permit can claim a shared result; the incompatible caller needs one additional exchange")
			assert.Equal(t, int32(2), calls.Load())
		})
	}
}

func TestExchangeResultBinding_CanceledReplacementCannotForwardPriorResult(t *testing.T) {
	for _, headerOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "body", true: "header-only"}[headerOnly], func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce, startOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			exchanger, calls := bindingConfiguredExchanger(t, func(w http.ResponseWriter, r *http.Request, call int32) {
				if call == 1 {
					bindingSuccess(w, "SYNTHETIC_PRIOR_RESULT", 3600)
					return
				}
				startOnce.Do(func() { close(started) })
				select {
				case <-release:
				case <-r.Context().Done():
				}
				w.WriteHeader(http.StatusServiceUnavailable)
			}, func(cfg *extprocconfig.Config) { cfg.OAuth2.ExchangeTimeout = time.Second })
			permit := samplePermit(time.Unix(1_000, 0).Add(time.Minute))
			h := newDelegationHarnessWithExchanger(t, func() (delegation.Result, error) { return permit, nil }, exchanger)
			t.Cleanup(unblock)
			require.Equal(t, "Bearer SYNTHETIC_PRIOR_RESULT", requireAuthorizationMutation(t, bindingHarnessCall(t, h, headerOnly)))
			permit.Context.Grant.UpdatedAt = "2026-10-26T10:00:00Z"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream, err := h.client.Process(ctx)
			require.NoError(t, err)
			method := "POST"
			if headerOnly {
				method = "GET"
			}
			require.NoError(t, stream.Send(&extprocv3.ProcessingRequest{
				MetadataContext: validTokenExchangeMetadata("mcp"),
				Request: &extprocv3.ProcessingRequest_RequestHeaders{
					RequestHeaders: extProcHeaders(map[string]string{":method": method}, headerOnly),
				},
			}))
			type received struct {
				response *extprocv3.ProcessingResponse
				err      error
			}
			receivedResult := make(chan received, 1)
			go func() {
				response, recvErr := stream.Recv()
				receivedResult <- received{response: response, err: recvErr}
			}()
			select {
			case <-started:
			case result := <-receivedResult:
				t.Fatalf("request completed before its required replacement exchange: response=%T error=%v", result.response.GetResponse(), result.err)
			case <-time.After(5 * time.Second):
				t.Fatal("replacement exchange did not start")
			}
			cancel()
			select {
			case result := <-receivedResult:
				assert.Nil(t, result.response, "cancellation cannot emit the prior Authorization result")
				assert.Equal(t, codes.Canceled, status.Code(result.err))
			case <-time.After(5 * time.Second):
				t.Fatal("canceled request did not finish")
			}
			unblock()
			assert.Equal(t, int32(2), calls.Load())
		})
	}
}
