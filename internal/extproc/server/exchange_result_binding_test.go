package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
)

func bindingPortExchanger(t *testing.T, lifetime int) (*TokenExchanger, *atomic.Int32, *bytes.Buffer) {
	t.Helper()
	calls := &atomic.Int32{}
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "PROVIDER_SECRET", "expires_in": lifetime, "principal": "PRINCIPAL_SECRET", "agent_id": "agent", "granted_permission_sets": map[string][]string{"extension": {"service"}}})
	}))
	t.Cleanup(broker.Close)
	logs := &bytes.Buffer{}
	te := &TokenExchanger{cfg: &extprocconfig.Config{OAuth2: extprocconfig.OAuth2Config{TokenEndpoint: broker.URL, ExchangeTimeout: time.Second}, Cache: extprocconfig.CacheConfig{DefaultTTL: time.Minute, MaxTTL: time.Hour}}, client: broker.Client(), logger: slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), cache: make(map[tokenCacheKey]*cachedToken)}
	te.assertion.Store(&assertionState{value: "ASSERTION_SECRET"})
	return te, calls, logs
}

func portBinding(scopes ...string) ExchangeResultBinding {
	return ExchangeResultBinding{Principal: "PRINCIPAL_SECRET", AgentID: "agent", Scopes: scopes, GrantUpdatedAt: time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)}
}
func boundPortResult(binding ExchangeResultBinding) ExchangeResult {
	return ExchangeResult{Token: "PROVIDER_SECRET", Principal: "PRINCIPAL_SECRET", AgentID: "agent", handle: &tokenResultHandle{binding: &binding}}
}

func TestBindingPortObservedAndReplacementOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		observed, replacement *ExchangeResultBinding
		wantCalls             int32
	}{
		{"unbound first use", nil, nil, 0}, {"matching detached", ptrBinding("read"), ptrBinding("write"), 0},
		{"matching replacement", ptrBinding("write"), ptrBinding("read"), 0}, {"unbound replacement", ptrBinding("write"), nil, 0},
		{"incompatible replacement", ptrBinding("write"), ptrBinding("other"), 1}, {"absent observed", ptrBinding("write"), nil, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			te, calls, _ := bindingPortExchanger(t, 3600)
			observed := ExchangeResult{Token: "PROVIDER_SECRET", Principal: "PRINCIPAL_SECRET", AgentID: "agent", handle: &tokenResultHandle{binding: tt.observed}}
			key := tokenCacheKey{"SUBJECT_SECRET", "https://resource.example"}
			var replacement *tokenResultHandle
			if tt.name != "absent observed" {
				replacement = &tokenResultHandle{binding: tt.replacement}
				te.cache[key] = &cachedToken{accessToken: "PROVIDER_SECRET", principal: "PRINCIPAL_SECRET", agentID: "agent", handle: replacement, expiresAt: time.Now().Add(time.Minute)}
			}
			result, err := te.ExchangeBound(context.Background(), key.subjectToken, key.resourceURI, &observed, portBinding("read"))
			require.NoError(t, err)
			assert.Equal(t, tt.wantCalls, calls.Load())
			require.NotNil(t, result.handle)
			require.NotNil(t, result.handle.binding, "the delivered handle must be claimed")
			assert.Equal(t, []string{"read"}, result.handle.binding.Scopes)
			if tt.name == "matching replacement" || tt.name == "unbound replacement" {
				assert.Same(t, replacement, result.handle, "same bytes do not make generations identical")
			}
			if tt.name == "matching detached" {
				assert.Same(t, observed.handle, result.handle)
				assert.Same(t, replacement, te.cache[key].handle)
			}
		})
	}
}
func TestBindingIdentityCannotRebindResult(t *testing.T) {
	for _, field := range []string{"principal", "agent"} {
		t.Run(field, func(t *testing.T) {
			handle := &tokenResultHandle{}
			original := portBinding("read")
			require.True(t, handle.bindOnce(original))
			changed := original
			if field == "principal" {
				changed.Principal = "another-principal"
			} else {
				changed.AgentID = "another-agent"
			}
			assert.False(t, handle.bindOnce(changed))
			assert.True(t, handle.bindOnce(original), "identity mismatch must not overwrite the first claim")
		})
	}
}

func ptrBinding(scopes ...string) *ExchangeResultBinding { b := portBinding(scopes...); return &b }

// Done is consulted by awaitResult after this caller has joined the flight.
type joinedBindingContext struct {
	context.Context
	joined chan struct{}
	once   sync.Once
}

func (c *joinedBindingContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.joined) })
	return c.Context.Done()
}

func TestBindingPortSharedRetryNeverRebindsOrLoops(t *testing.T) {
	for _, lifetime := range []int{3600, 0} {
		for _, outcome := range []string{"matching", "unbound", "conflict", "failure"} {
			t.Run(outcome+"/"+map[int]string{3600: "cacheable", 0: "non-cacheable"}[lifetime], func(t *testing.T) {
				te, calls, _ := bindingPortExchanger(t, lifetime)
				initial := boundPortResult(portBinding("old"))
				replacement, err := te.Exchange(context.Background(), "seed", "https://resource.example")
				require.NoError(t, err)
				require.NotNil(t, replacement.handle)
				if outcome != "unbound" {
					binding := portBinding("read")
					if outcome == "conflict" {
						binding = portBinding("winner")
					}
					_, err = te.ExchangeBound(context.Background(), "seed", "https://resource.example", &replacement, binding)
					require.NoError(t, err)
				}
				callsBefore := calls.Load()
				release := make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				t.Cleanup(unblock)
				entered := make(chan struct{})
				brokerErr := errors.New("DEPENDENCY_SECRET")
				leader := te.sfGroup.DoChan("SUBJECT_SECRET\x00https://resource.example", func() (any, error) {
					close(entered)
					<-release
					if outcome == "failure" {
						return ExchangeResult{}, brokerErr
					}
					return replacement, nil
				})
				<-entered
				ctx := &joinedBindingContext{Context: context.Background(), joined: make(chan struct{})}
				done := make(chan singleflight.Result, 1)
				go func() {
					r, e := te.ExchangeBound(ctx, "SUBJECT_SECRET", "https://resource.example", &initial, portBinding("read"))
					done <- singleflight.Result{Val: r, Err: e}
				}()
				select {
				case <-ctx.joined:
				case r := <-done:
					t.Fatalf("required additional flight was not joined: error=%v result=%T", r.Err, r.Val)
				case <-time.After(time.Second):
					t.Fatal("retry not joined")
				}
				unblock()
				<-leader
				r := <-done
				switch outcome {
				case "conflict":
					assert.ErrorIs(t, r.Err, ErrExchangeResultBindingConflict)
					assert.Equal(t, []string{"winner"}, replacement.handle.binding.Scopes)
					assert.Empty(t, r.Val.(ExchangeResult).Token)
				case "failure":
					assert.ErrorIs(t, r.Err, brokerErr)
					assert.Empty(t, r.Val.(ExchangeResult).Token)
				default:
					require.NoError(t, r.Err)
					assert.Same(t, replacement.handle, r.Val.(ExchangeResult).handle)
					assert.Equal(t, []string{"read"}, replacement.handle.binding.Scopes)
				}
				assert.Equal(t, callsBefore, calls.Load(), "no third flight or broker call")
			})
		}
	}
}

func TestExchangeResultBindingFormattingDoesNotExposeGrantOrScopes(t *testing.T) {
	binding := portBinding("SCOPE_SECRET")
	for _, value := range []string{fmt.Sprint(binding), fmt.Sprintf("%#v", binding)} {
		assert.NotContains(t, value, "SCOPE_SECRET")
		assert.NotContains(t, value, "PRINCIPAL_SECRET")
		assert.NotContains(t, value, "2026")
	}
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("binding diagnostic", "binding", binding)
	assert.NotContains(t, logs.String(), "SCOPE_SECRET")
	assert.NotContains(t, logs.String(), "PRINCIPAL_SECRET")
	assert.NotContains(t, logs.String(), "2026-10-26")
}

func TestBindingPortDiagnosticIsLogOnlyAndCredentialSafe(t *testing.T) {
	recorder, reader := installSafetyTelemetry(t)
	te, calls, logs := bindingPortExchanger(t, 3600)
	initial := boundPortResult(portBinding("SCOPE_SECRET"))
	result, err := te.ExchangeBound(context.Background(), "SUBJECT_SECRET", "https://resource.example", &initial, portBinding("new"))
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, "PROVIDER_SECRET", result.Token)
	assert.Contains(t, logs.String(), `"delegation_binding":"mismatch"`)
	assert.Empty(t, recorder.Ended(), "binding adds no span or span attribute")
	assertSafetyTelemetry(t, recorder, reader, logs.String(), "PROVIDER_SECRET", "ASSERTION_SECRET", "SUBJECT_SECRET", "SCOPE_SECRET", "PRINCIPAL_SECRET", "resource.example")
	for _, secret := range []string{"PROVIDER_SECRET", "ASSERTION_SECRET", "SUBJECT_SECRET", "SCOPE_SECRET", "PRINCIPAL_SECRET", "resource.example"} {
		assert.NotContains(t, logs.String(), secret)
	}
}
