package oauth2session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const identityTelemetryPrincipal = "identity-telemetry-owner@example.invalid"

func identityTelemetryRecorder(t *testing.T) (*tracetest.SpanRecorder, *sdkmetric.ManualReader) {
	t.Helper()
	reader := backgroundMetrics(t)
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		assert.NoError(t, provider.Shutdown(context.Background()))
	})
	return recorder, reader
}

func identityTelemetryFixture(t *testing.T) (*OAuth2SessionService, *model.ThirdpartyOAuth2ProviderEntity, *storage.UserSession, *backgroundStore, *backgroundLog) {
	t.Helper()
	service, provider, session, _, logs := newIdentityCallbackFixture(t, "filesystem")
	session.Principal = identityTelemetryPrincipal
	session.EncryptedAccessToken = []byte("identity-telemetry-access-ciphertext-sentinel")
	session.EncryptedRefreshToken = []byte("identity-telemetry-refresh-ciphertext-sentinel")
	store := newBackgroundStore(session)
	service.sessionRepo = operationSessionRepository{find: store.find}
	service.refreshRepo = operationRefreshRepository(store.withLocked)
	return service, provider, session, store, logs
}

func assertIdentityTelemetrySafe(t *testing.T, recorder *tracetest.SpanRecorder, reader *sdkmetric.ManualReader, logs string, additional ...string) metricdata.ResourceMetrics {
	t.Helper()
	var recorded strings.Builder
	recorded.WriteString(logs)
	for _, span := range recorder.Ended() {
		encoded, err := json.Marshal(tracetest.SpanStubFromReadOnlySpan(span))
		require.NoError(t, err)
		recorded.Write(encoded)
		_, _ = fmt.Fprint(&recorded, span.Resource().Attributes())
		assert.Equal(t, "oauth2session", span.InstrumentationScope().Name)
		assert.Equal(t, "oauth2session.background_refresh", span.Name())
		assert.Empty(t, span.Events(), "raw exception events are forbidden")
		assert.ElementsMatch(t, []attribute.KeyValue{
			attribute.String("oauth2.refresh.triggered_by", "background"),
			attribute.String("oauth2.refresh.outcome", "failed"),
		}, span.Attributes(), "no new namespace or principal span attribute")
	}
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	encoded, err := json.Marshal(collected)
	require.NoError(t, err)
	recorded.Write(encoded)
	_, _ = fmt.Fprint(&recorded, collected.Resource.Attributes())
	allowed := map[string][]string{
		"proactive_refresh_triggered_total": {"triggered_by", "service_id"},
		"proactive_refresh_dropped_total":   {"triggered_by"},
		"proactive_refresh_failed_total":    {"triggered_by", "service_id", "failure_detail"},
		"session_sweep_refreshed_total":     {"triggered_by", "service_id"},
		"session_sweep_skipped_total":       {"triggered_by"},
		"session_sweep_failed_total":        {"triggered_by", "service_id", "failure_detail"},
	}
	for _, scope := range collected.ScopeMetrics {
		assert.Equal(t, "oauth2session", scope.Scope.Name, "no new instrumentation scope")
		for _, instrument := range scope.Metrics {
			keys, known := allowed[instrument.Name]
			require.True(t, known, "unexpected instrument %s", instrument.Name)
			sum, ok := instrument.Data.(metricdata.Sum[int64])
			require.True(t, ok, "existing refresh instruments remain integer counters")
			for _, point := range sum.DataPoints {
				var got []string
				for _, attr := range point.Attributes.ToSlice() {
					got = append(got, string(attr.Key))
				}
				assert.ElementsMatch(t, keys, got, "no identity, revision, principal, session, reason, or channel dimension")
			}
		}
	}
	for _, sensitive := range append([]string{
		"identity-current-sentinel", "identity-previous-sentinel", "identity-secret-sentinel",
		"identity-assertion-sentinel", "identity-pkce-sentinel", "identity-code-sentinel",
		"identity-telemetry-access-ciphertext-sentinel", "identity-telemetry-refresh-ciphertext-sentinel",
		"/identity/path-sentinel/id-basename-sentinel", "id-basename-sentinel",
		"identity-file-content-sentinel", "identity-raw-provider-body-sentinel",
	}, additional...) {
		if sensitive != "" {
			assert.NotContains(t, recorded.String(), sensitive)
		}
	}
	// Principal-bearing logs belong to the production principal-policy tests; spans and metrics never carry one.
	assert.NotContains(t, recorded.String()[len(logs):], identityTelemetryPrincipal)
	return collected
}

func identityTelemetryReadFailure() error {
	return &os.PathError{Op: "read", Path: "/identity/path-sentinel/id-basename-sentinel", Err: errors.New("identity-file-content-sentinel identity-raw-provider-body-sentinel")}
}

func TestClientIdentityTelemetryBoundarySafety(t *testing.T) {
	for _, boundary := range []string{"initiation", "callback missing identity", "callback missing revision", "callback stale revision", "callback mismatched identity", "valid token", "unknown expiry token", "unavailable token", "display fallback", "gate source failure"} {
		t.Run(boundary, func(t *testing.T) {
			recorder, reader := identityTelemetryRecorder(t)
			service, provider, session, store, logs := identityTelemetryFixture(t)
			var calls atomic.Int32
			service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return operationTokenResponse(200, `{"access_token":"identity-raw-provider-body-sentinel"}`), nil
			})
			future := time.Now().Add(time.Hour)
			session.AccessTokenExpiresAt = &future
			before, err := store.find(context.Background(), session.Principal, session.ServiceID)
			require.NoError(t, err)
			var state string
			var operation string
			want := model.CredentialSourceReasonReadFailed
			switch boundary {
			case "initiation":
				service.credentialFileReader = enforcementCredentialReader{err: identityTelemetryReadFailure()}
				flow, flowErr := service.InitiateOAuth2Flow(context.Background(), session.Principal, provider.ID, "https://broker.example.com/sessions")
				assert.Nil(t, flow)
				err, operation = flowErr, "authorization_initiation"
			case "callback missing identity", "callback missing revision", "callback stale revision", "callback mismatched identity":
				identity, revision := session.UpstreamClientID, provider.ClientIdentityRevision
				revisionClaim := &revision
				want = model.CredentialSourceReasonIdentityMissing
				switch boundary {
				case "callback missing identity":
					identity = nil
				case "callback missing revision":
					revisionClaim = nil
				case "callback stale revision":
					revision--
					want = model.CredentialSourceReasonIdentitySuperseded
				case "callback mismatched identity":
					identity = identityPointer("identity-previous-sentinel")
					want = model.CredentialSourceReasonIdentityMismatch
				}
				state = identityCallbackState(t, service, provider, session, identity, revisionClaim)
				result, callbackErr := service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: provider.ID, Code: "identity-code-sentinel", State: state})
				assert.Nil(t, result)
				err, operation = callbackErr, "code_exchange"
			case "valid token", "unknown expiry token", "unavailable token":
				if boundary == "unknown expiry token" {
					session.AccessTokenExpiresAt = nil
				}
				if boundary == "unavailable token" {
					service.credentialFileReader = enforcementCredentialReader{err: identityTelemetryReadFailure()}
				} else {
					session.UpstreamClientID = identityPointer("identity-previous-sentinel")
					want = model.CredentialSourceReasonIdentityMismatch
				}
				result, token, tokenErr := service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
				assert.Nil(t, result)
				assert.Empty(t, token)
				err, operation = tokenErr, "token_acquisition"
			case "display fallback", "gate source failure":
				service.credentialFileReader = enforcementCredentialReader{err: identityTelemetryReadFailure()}
				mode := ports.UsabilityDisplay
				if boundary == "gate source failure" {
					mode = ports.UsabilityGate
				}
				result, evaluationErr := service.RequiresReauthentication(context.Background(), []*storage.UserSession{session}, mode)
				if mode == ports.UsabilityDisplay {
					require.NoError(t, evaluationErr)
					assert.False(t, result[session.ServiceID], "only display may retain matching persisted evidence")
					operation = "usability_display"
				} else {
					assert.Nil(t, result)
					assert.ErrorIs(t, evaluationErr, model.ErrCredentialSourceUnavailable)
					assert.Empty(t, logs.text(), "the consuming gate owns the failure record")
				}
			}
			if operation != "" && operation != "usability_display" {
				var source *model.CredentialSourceError
				require.ErrorAs(t, err, &source)
				assert.Equal(t, want, source.Reason)
			}
			if operation != "" {
				assertIdentityFailureEvent(t, logs, provider.ID, operation, want)
			}
			assert.Zero(t, calls.Load())
			after, findErr := store.find(context.Background(), session.Principal, provider.ID)
			require.NoError(t, findErr)
			assert.Equal(t, before.EncryptedAccessToken, after.EncryptedAccessToken)
			assert.Equal(t, before.EncryptedRefreshToken, after.EncryptedRefreshToken)
			require.NoError(t, service.Close(context.Background()))
			assertIdentityTelemetrySafe(t, recorder, reader, logs.text(), state)
			assert.Empty(t, recorder.Ended(), "synchronous identity evaluation adds no span")
		})
	}
}

type identityTelemetryExpiry struct {
	ports.UserSessionExpiryRepository
	session *storage.UserSession
}

func (r identityTelemetryExpiry) ListExpiringSessions(context.Context, time.Time, storage.SessionExpiryCursor, int) ([]*storage.UserSession, error) {
	return []*storage.UserSession{r.session}, nil
}

func TestClientIdentityTelemetryRefreshAndSweepSafety(t *testing.T) {
	for _, trigger := range []string{"manual", "on-demand", "background", "sweep", "dry-run sweep"} {
		for _, unavailable := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/unavailable=%t", trigger, unavailable), func(t *testing.T) {
				recorder, reader := identityTelemetryRecorder(t)
				service, provider, session, store, logs := identityTelemetryFixture(t)
				want := model.CredentialSourceReasonIdentitySuperseded
				if unavailable {
					service.credentialFileReader = enforcementCredentialReader{err: identityTelemetryReadFailure()}
					want = model.CredentialSourceReasonReadFailed
				} else {
					session.UpstreamClientRevision--
				}
				before, err := store.find(context.Background(), session.Principal, provider.ID)
				require.NoError(t, err)
				var calls atomic.Int32
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					calls.Add(1)
					return operationTokenResponse(200, `{"access_token":"identity-raw-provider-body-sentinel"}`), nil
				})
				switch trigger {
				case "manual":
					_, err = service.ForceRefreshSession(context.Background(), session.Principal, provider.ID)
				case "on-demand":
					initial := *session
					initial.UpstreamClientRevision = provider.ClientIdentityRevision
					service.sessionRepo = operationSessionRepository{find: func(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error) {
						return &initial, nil
					}}
					service.credentialFileReader = enforcementCredentialReader{}
					service.refreshRepo = operationRefreshRepository(func(ctx context.Context, principal id.Principal, serviceID id.ServiceID, update func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
						if unavailable {
							service.credentialFileReader = enforcementCredentialReader{err: identityTelemetryReadFailure()}
						}
						return store.withLocked(ctx, principal, serviceID, update)
					})
					var token string
					_, token, err = service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
					assert.Empty(t, token)
				case "background":
					submitBackground(t, service.background, context.Background(), session)
					require.NoError(t, service.Close(context.Background()))
				case "sweep", "dry-run sweep":
					dry := trigger == "dry-run sweep"
					result, sweepErr := NewSessionSweepService(identityTelemetryExpiry{session: session}, service, service.logger).Sweep(context.Background(), SweepRequest{DryRun: dry, Lookahead: time.Minute, PageSize: 2})
					require.NoError(t, sweepErr)
					assert.Equal(t, 1, result.TotalEvaluated)
					assert.Equal(t, 1, result.Failed)
					assert.Zero(t, result.Refreshed)
					assert.Equal(t, dry, result.DryRun)
				}
				if trigger == "manual" || trigger == "on-demand" {
					var source *model.CredentialSourceError
					require.ErrorAs(t, err, &source)
					assert.Equal(t, want, source.Reason)
				}
				require.NoError(t, service.Close(context.Background()))
				assert.Zero(t, calls.Load())
				after, findErr := store.find(context.Background(), session.Principal, provider.ID)
				require.NoError(t, findErr)
				assert.Equal(t, before, after)
				assertIdentityFailureEvent(t, logs, provider.ID, "refresh", want)
				collected := assertIdentityTelemetrySafe(t, recorder, reader, logs.text())
				if trigger == "background" {
					require.Len(t, recorder.Ended(), 1, "the safety scan must exercise a real refresh span")
					assertBackgroundMetrics(t, reader, map[string]int64{"proactive_refresh_triggered_total": 1, "proactive_refresh_failed_total": 1})
				} else {
					assert.Empty(t, recorder.Ended(), "identity checks add no span")
				}
				if trigger == "sweep" {
					var failures int64
					for _, scope := range collected.ScopeMetrics {
						for _, instrument := range scope.Metrics {
							if instrument.Name != "session_sweep_failed_total" {
								continue
							}
							for _, point := range instrument.Data.(metricdata.Sum[int64]).DataPoints {
								failures += point.Value
								triggerValue, _ := point.Attributes.Value("triggered_by")
								detail, _ := point.Attributes.Value("failure_detail")
								assert.Equal(t, "sweep", triggerValue.AsString())
								assert.Equal(t, "credential_source_unavailable", detail.AsString())
							}
						}
					}
					assert.Equal(t, int64(1), failures, "scan a real failed-candidate metric point")
				}
				if trigger == "dry-run sweep" {
					for _, scope := range collected.ScopeMetrics {
						assert.Empty(t, scope.Metrics, "dry runs do not change refresh counters")
					}
				}
			})
		}
	}
}

func TestClientIdentityTelemetryDisabledPreservesDenialAndDisplayFallback(t *testing.T) {
	previousTracer, previousMeter := otel.GetTracerProvider(), otel.GetMeterProvider()
	otel.SetTracerProvider(tracenoop.NewTracerProvider())
	otel.SetMeterProvider(metricnoop.NewMeterProvider())
	t.Cleanup(func() {
		otel.SetTracerProvider(previousTracer)
		otel.SetMeterProvider(previousMeter)
	})
	service, provider, session, _, _ := identityTelemetryFixture(t)
	session.UpstreamClientRevision--
	_, token, err := service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
	assert.Empty(t, token)
	var source *model.CredentialSourceError
	require.ErrorAs(t, err, &source)
	assert.Equal(t, model.CredentialSourceReasonIdentitySuperseded, source.Reason)
	_, err = service.ForceRefreshSession(context.Background(), session.Principal, provider.ID)
	require.ErrorAs(t, err, &source)
	assert.Equal(t, model.CredentialSourceReasonIdentitySuperseded, source.Reason)
	service.credentialFileReader = enforcementCredentialReader{err: identityTelemetryReadFailure()}
	result, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{session}, ports.UsabilityDisplay)
	require.NoError(t, err)
	assert.True(t, result[session.ServiceID], "disabled telemetry cannot revive superseded persisted evidence")
}
