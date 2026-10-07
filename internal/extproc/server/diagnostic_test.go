package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

func TestDiagnosticEveryDetailMappingAndBoundedFallback(t *testing.T) {
	cases := []struct {
		detail  FailureDetail
		outcome Outcome
		action  RecoveryAction
		target  RecoveryTarget
	}{
		{DetailResourceMissing, OutcomeInvalidRequest, RecoveryNone, TargetNone},
		{DetailRequestMalformed, OutcomeInvalidRequest, RecoveryNone, TargetNone},
		{DetailSubjectInvalid, OutcomeAuthenticationFailed, RecoveryNone, TargetNone},
		{DetailClientInvalid, OutcomeAuthenticationFailed, RecoveryNone, TargetNone},
		{DetailClientExpired, OutcomeAuthenticationFailed, RecoveryNone, TargetNone},
		{DetailCredentialRejected, OutcomeAuthenticationFailed, RecoveryNone, TargetNone},
		{DetailAuthorizationDenied, OutcomeAuthorizationDenied, RecoveryNone, TargetNone},
		{DetailConsentRequired, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{DetailReauthRequired, OutcomeReauthRequired, RecoveryNone, TargetNone},
		{DetailSessionReauthRequired, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{DetailBrokerUnavailable, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailBrokerResponseInvalid, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailProviderUnavailable, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailProviderResponseInvalid, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailCircuitOpen, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailConfiguration, OutcomeConfigurationError, RecoveryFixConfiguration, TargetNone},
		{DetailPolicyConfiguration, OutcomeConfigurationError, RecoveryFixConfiguration, TargetNone},
		{DetailPolicyEvaluationFailed, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailPolicyDenied, OutcomeAuthorizationDenied, RecoveryNone, TargetNone},
		{DetailResponseWriteFailed, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailInternalUnclassified, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailCallerCanceled, OutcomeCanceled, RecoveryNone, TargetNone},
	}
	for _, test := range cases {
		t.Run(string(test.detail), func(t *testing.T) {
			for _, stage := range []FailureStage{StageRequestValidation, StageExchangeRouting, StageClientAuthorization, StageRefresh, StageResponseWrite} {
				d := NewDiagnostic(stage, test.detail)
				assert.Equal(t, test.outcome, d.Outcome())
				assert.Equal(t, stage, d.Stage(), "unseen broker origin stages must not be inferred")
				assert.Equal(t, test.detail, d.Detail())
				assert.Equal(t, test.action, d.RecoveryAction())
				assert.Equal(t, test.target, d.RecoveryTarget())
				assert.Equal(t, ExchangeUnknown, d.ExchangeKind())
			}
		})
	}
	unknown := NewDiagnostic(FailureStage("stage-secret-sentinel"), FailureDetail("detail-secret-sentinel"))
	assert.Equal(t, OutcomeInfrastructureError, unknown.Outcome())
	assert.Equal(t, StageExchangeRouting, unknown.Stage())
	assert.Equal(t, DetailInternalUnclassified, unknown.Detail())
	assert.Equal(t, RecoveryRetry, unknown.RecoveryAction())
	for _, kind := range []ExchangeKind{ExchangeUnknown, ExchangeThirdParty, ExchangeImpersonation, ExchangeKind("kind-secret-sentinel")} {
		success := SuccessDiagnostic(kind)
		assert.Equal(t, OutcomeSuccess, success.Outcome())
		assert.Equal(t, StageNone, success.Stage())
		assert.Equal(t, DetailNone, success.Detail())
		assert.Equal(t, RecoveryNone, success.RecoveryAction())
		assert.Equal(t, TargetNone, success.RecoveryTarget())
		if kind == "kind-secret-sentinel" {
			assert.Equal(t, ExchangeUnknown, success.ExchangeKind())
		} else {
			assert.Equal(t, kind, success.ExchangeKind())
		}
	}
}

func TestBrokerDiagnosticUsesOnlyStatusCodeAndRecoveryPresence(t *testing.T) {
	cases := []struct {
		status  int
		code    string
		uri     bool
		detail  FailureDetail
		outcome Outcome
		action  RecoveryAction
		target  RecoveryTarget
	}{
		{400, "invalid_request", false, DetailRequestMalformed, OutcomeInvalidRequest, RecoveryNone, TargetNone},
		{400, "invalid_scope", false, DetailRequestMalformed, OutcomeInvalidRequest, RecoveryNone, TargetNone},
		{400, "invalid_target", false, DetailRequestMalformed, OutcomeInvalidRequest, RecoveryNone, TargetNone},
		{400, "unsupported_grant_type", false, DetailRequestMalformed, OutcomeInvalidRequest, RecoveryNone, TargetNone},
		{401, "invalid_client", false, DetailClientInvalid, OutcomeAuthenticationFailed, RecoveryNone, TargetNone},
		{401, "unauthorized_client", false, DetailClientInvalid, OutcomeAuthenticationFailed, RecoveryNone, TargetNone},
		{401, "invalid_token", false, DetailSubjectInvalid, OutcomeAuthenticationFailed, RecoveryNone, TargetNone},
		{400, "invalid_grant", false, DetailCredentialRejected, OutcomeAuthenticationFailed, RecoveryNone, TargetNone},
		{400, "invalid_grant", true, DetailSessionReauthRequired, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{401, "reauth_required", true, DetailSessionReauthRequired, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{401, "reauth_required", false, DetailReauthRequired, OutcomeReauthRequired, RecoveryNone, TargetNone},
		{403, "access_denied", false, DetailAuthorizationDenied, OutcomeAuthorizationDenied, RecoveryNone, TargetNone},
		{403, "access_denied", true, DetailConsentRequired, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{429, "invalid_grant", true, DetailSessionReauthRequired, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{429, "slow_down", false, DetailBrokerUnavailable, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{500, "invalid_grant", true, DetailBrokerUnavailable, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{400, "server_error", false, DetailBrokerUnavailable, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{400, "temporarily_unavailable", false, DetailBrokerUnavailable, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{400, "code-secret-sentinel", true, DetailInternalUnclassified, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{400, "", false, DetailInternalUnclassified, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
	}
	for _, test := range cases {
		t.Run(fmt.Sprintf("%d/%s/uri=%t", test.status, test.code, test.uri), func(t *testing.T) {
			cause := &BrokerExchangeError{StatusCode: test.status, Code: test.code, Description: "client expired session missing grant revoked description-secret-sentinel"}
			if test.uri {
				cause.ErrorURI = "https://endpoint-secret-sentinel.invalid/scope"
			}
			d := diagnosticFromError(context.Background(), NewOperationError(cause.Metadata(), cause))
			assert.Equal(t, StageExchangeRouting, d.Stage())
			assert.Equal(t, test.detail, d.Detail())
			assert.Equal(t, test.outcome, d.Outcome())
			assert.Equal(t, test.action, d.RecoveryAction())
			assert.Equal(t, test.target, d.RecoveryTarget())
			assert.Equal(t, ExchangeUnknown, d.ExchangeKind())
		})
	}
}

func TestOperationErrorEnrichmentReturnsIndependentCopies(t *testing.T) {
	cause := errors.New("cause-secret-sentinel")
	original := NewOperationError(NewErrorMetadata(OperationExchange, KindInternal, DependencyLocal, 0, ""), cause)
	var workers sync.WaitGroup
	var mismatches atomic.Int32
	for _, stage := range []FailureStage{StageRefresh, StageResponseWrite, StageClientAuthorization} {
		workers.Go(func() {
			for range 100 {
				enriched := original.WithDiagnostic(NewDiagnostic(stage, DetailCallerCanceled).WithExchangeKind(ExchangeImpersonation))
				if enriched == original || enriched.Diagnostic().Stage() != stage || enriched.Diagnostic().Outcome() != OutcomeCanceled || !errors.Is(enriched, cause) || original.Diagnostic().Stage() != StageExchangeRouting || original.Diagnostic().Detail() != DetailInternalUnclassified {
					mismatches.Add(1)
				}
			}
		})
	}
	workers.Wait()
	assert.Zero(t, mismatches.Load())
	assert.Equal(t, DetailInternalUnclassified, original.WithDiagnostic(Diagnostic{}).Diagnostic().Detail())
}

func TestCommonExchangeFieldsMatchLogsSpansAndExistingMetrics(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		invalidInput bool
		canceled     bool
		outcome      Outcome
		stage        FailureStage
		detail       FailureDetail
		action       RecoveryAction
		target       RecoveryTarget
	}{
		{"success", nil, false, false, OutcomeSuccess, StageNone, DetailNone, RecoveryNone, TargetNone},
		{"invalid request", nil, true, false, OutcomeInvalidRequest, StageRequestValidation, DetailRequestMalformed, RecoveryNone, TargetNone},
		{"authentication", &BrokerExchangeError{StatusCode: 401, Code: "invalid_client"}, false, false, OutcomeAuthenticationFailed, StageExchangeRouting, DetailClientInvalid, RecoveryNone, TargetNone},
		{"authorization", &BrokerExchangeError{StatusCode: 403, Code: "access_denied"}, false, false, OutcomeAuthorizationDenied, StageExchangeRouting, DetailAuthorizationDenied, RecoveryNone, TargetNone},
		{"reauth", &BrokerExchangeError{StatusCode: 400, Code: "invalid_grant", ErrorURI: "https://uri-secret-sentinel.invalid"}, false, false, OutcomeReauthRequired, StageExchangeRouting, DetailSessionReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{"infrastructure", errors.New("nested-error-secret-sentinel"), false, false, OutcomeInfrastructureError, StageExchangeRouting, DetailInternalUnclassified, RecoveryRetry, TargetNone},
		{"configuration", NewOperationError(NewErrorMetadata(OperationConfiguration, KindConfiguration, DependencyLocal, 0, ""), errors.New("config-secret-sentinel")), false, false, OutcomeConfigurationError, StageExchangeRouting, DetailConfiguration, RecoveryFixConfiguration, TargetNone},
		{"canceled", context.Canceled, false, true, OutcomeCanceled, StageExchangeRouting, DetailCallerCanceled, RecoveryNone, TargetNone},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			recorder, reader := installSafetyTelemetry(t)
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			cfg := &extprocconfig.Config{Telemetry: extprocconfig.TelemetryConfig{Enabled: true, Traces: extprocconfig.TracesConfig{Enabled: true}, Metrics: extprocconfig.MetricsConfig{Enabled: true}}}
			server := NewServer(cfg, &safetyExchanger{err: test.err}, logger)
			ctx := context.Background()
			if test.canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			headers := &extprocv3.HttpHeaders{Headers: &corev3.HeaderMap{}}
			request := safetyRequest("subject-secret-sentinel", "https://resource-secret-sentinel.invalid/path", headers)
			if test.invalidInput {
				request.MetadataContext = nil
			}
			require.NotNil(t, server.processRequestHeaders(ctx, request, headers))
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			expected := map[string]string{"token_exchange.outcome": string(test.outcome), "token_exchange.failure_stage": string(test.stage), "token_exchange.failure_detail": string(test.detail), "token_exchange.recovery_action": string(test.action), "token_exchange.recovery_target": string(test.target), "token_exchange.exchange_kind": "unknown"}
			spanFields := make(map[string]string)
			for _, attr := range spans[0].Attributes() {
				spanFields[string(attr.Key)] = attr.Value.AsString()
			}
			assertCommonFields(t, spanFields, expected, test.outcome == OutcomeSuccess)
			for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte{'\n'}) {
				var record map[string]any
				require.NoError(t, json.Unmarshal(line, &record))
				fields := make(map[string]string)
				for key, value := range record {
					if value, ok := value.(string); ok {
						fields[key] = value
					}
				}
				assertCommonFields(t, fields, expected, test.outcome == OutcomeSuccess)
			}
			var metrics metricdata.ResourceMetrics
			require.NoError(t, reader.Collect(context.Background(), &metrics))
			observed := 0
			for _, scope := range metrics.ScopeMetrics {
				for _, metric := range scope.Metrics {
					var fields []map[string]string
					switch data := metric.Data.(type) {
					case metricdata.Sum[int64]:
						for _, point := range data.DataPoints {
							values := make(map[string]string)
							for _, attr := range point.Attributes.ToSlice() {
								values[string(attr.Key)] = attr.Value.AsString()
							}
							fields = append(fields, values)
						}
					case metricdata.Histogram[float64]:
						for _, point := range data.DataPoints {
							values := make(map[string]string)
							for _, attr := range point.Attributes.ToSlice() {
								values[string(attr.Key)] = attr.Value.AsString()
							}
							fields = append(fields, values)
						}
					}
					for _, point := range fields {
						observed++
						assertCommonFields(t, point, expected, false)
						assert.Len(t, point, 6)
					}
				}
			}
			assert.Equal(t, 2, observed, "both existing instruments must carry the complete common labels")
			assertSafetyTelemetry(t, recorder, reader, logs.String(), "subject-secret-sentinel", "resource-secret-sentinel", "uri-secret-sentinel", "nested-error-secret-sentinel", "config-secret-sentinel")
		})
	}
}

func assertCommonFields(t *testing.T, fields, expected map[string]string, omitSuccessFailure bool) {
	t.Helper()
	assert.NotContains(t, fields, "outcome")
	assert.NotContains(t, fields, "error.type")
	for key, value := range expected {
		if omitSuccessFailure && (key == "token_exchange.failure_stage" || key == "token_exchange.failure_detail") {
			assert.NotContains(t, fields, key)
		} else {
			assert.Equal(t, value, fields[key], key)
		}
	}
}

func TestMetadataResourceAbsenceIsClassifiedAtRequestValidation(t *testing.T) {
	for _, missing := range []bool{true, false} {
		request := safetyRequest("subject", "", &extprocv3.HttpHeaders{})
		if missing {
			delete(request.MetadataContext.FilterMetadata[tokenExchangeMetadataNamespace].Fields, resourceURIFieldKey)
		}
		_, rejection := extractTokenExchangeInput(request)
		require.NotNil(t, rejection)
		assert.Equal(t, StageRequestValidation, rejection.Diagnostic().Stage())
		assert.Equal(t, DetailResourceMissing, rejection.Diagnostic().Detail())
		assert.Equal(t, OutcomeInvalidRequest, rejection.Diagnostic().Outcome())
	}
}

func TestCallerCancellationPreservesKnownEnrichedStage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewOperationError(NewErrorMetadata(OperationExchange, KindCallerCanceled, DependencyLocal, 0, ""), context.Canceled).
		WithDiagnostic(NewDiagnostic(StageResponseWrite, DetailResponseWriteFailed))
	d := diagnosticFromError(ctx, err)
	assert.Equal(t, StageResponseWrite, d.Stage())
	assert.Equal(t, OutcomeCanceled, d.Outcome())
	assert.Equal(t, DetailCallerCanceled, d.Detail())
	assert.Equal(t, RecoveryNone, d.RecoveryAction())
}

func TestDependencyDeadlineClassificationSurvivesLaterCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dependency := NewOperationError(NewErrorMetadata(OperationAssertionRefresh, KindUnavailable, DependencyOAuthProvider, 0, ""), context.DeadlineExceeded)
	cancel()
	metadata := metadataFromError(ctx, dependency)
	assert.Equal(t, KindUnavailable, metadata.Kind())
	assert.Equal(t, DependencyOAuthProvider, metadata.Dependency())
	diagnostic := diagnosticFromError(ctx, dependency)
	assert.Equal(t, OutcomeInfrastructureError, diagnostic.Outcome())
	assert.Equal(t, StageRefresh, diagnostic.Stage())
	assert.Equal(t, DetailProviderUnavailable, diagnostic.Detail())
	assert.ErrorIs(t, dependency, context.DeadlineExceeded)
	assert.NotErrorIs(t, dependency, context.Canceled)
}
