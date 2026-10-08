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
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/httpclient"
)

func installSafetyTelemetry(t *testing.T) (*tracetest.SpanRecorder, *sdkmetric.ManualReader) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	oldTracer, oldMeter, oldPropagator := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetMeterProvider(meter)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(oldTracer)
		otel.SetMeterProvider(oldMeter)
		otel.SetTextMapPropagator(oldPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
		require.NoError(t, meter.Shutdown(context.Background()))
	})
	return recorder, reader
}

func assertSafetyTelemetry(t *testing.T, recorder *tracetest.SpanRecorder, reader *sdkmetric.ManualReader, logs string, secrets ...string) {
	t.Helper()
	var telemetry strings.Builder
	telemetry.WriteString(logs)
	for _, span := range recorder.Ended() {
		encoded, err := json.Marshal(tracetest.SpanStubFromReadOnlySpan(span))
		require.NoError(t, err)
		telemetry.Write(encoded)
		_, _ = fmt.Fprint(&telemetry, span.Resource().Attributes())
		assert.Empty(t, span.Events(), "credential-bearing exception events are forbidden")
		for _, attr := range span.Attributes() {
			assert.NotContains(t, string(attr.Key), "url")
			assert.NotContains(t, string(attr.Key), "resource.uri")
		}
	}
	var metrics metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &metrics))
	encoded, err := json.Marshal(metrics)
	require.NoError(t, err)
	telemetry.Write(encoded)
	_, _ = fmt.Fprint(&telemetry, metrics.Resource.Attributes())
	for _, secret := range secrets {
		assert.NotContains(t, telemetry.String(), secret)
	}
}

func TestErrorMetadataBoundsAndPreservesCauses(t *testing.T) {
	cause := &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusUnauthorized},
		Body: []byte("body-secret-sentinel"), ErrorCode: "provider-code-secret-sentinel", ErrorDescription: "description-secret-sentinel"}
	for _, statusCode := range []int{-1, 0, 99, 100, 400, 599, 600, 999999} {
		t.Run(fmt.Sprint(statusCode), func(t *testing.T) {
			metadata := NewErrorMetadata(Operation("operation-secret-sentinel"), ErrorKind("kind-secret-sentinel"), Dependency("dependency-secret-sentinel"), statusCode, cause.ErrorCode)
			assert.Equal(t, OperationExchange, metadata.Operation())
			assert.Equal(t, KindInternal, metadata.Kind())
			assert.Equal(t, DependencyLocal, metadata.Dependency())
			assert.Equal(t, "unknown", metadata.OAuthCode())
			if statusCode >= 100 && statusCode <= 599 {
				assert.Equal(t, statusCode, metadata.StatusCode())
			} else {
				assert.Zero(t, metadata.StatusCode())
			}
			wrapped := fmt.Errorf("nested-wrapper-secret-sentinel: %w", cause)
			err := NewOperationError(metadata, wrapped)
			require.ErrorIs(t, err, cause)
			var retrieved *oauth2.RetrieveError
			require.ErrorAs(t, err, &retrieved)
			assert.Same(t, cause, retrieved)
			assert.NotContains(t, err.Error(), "secret-sentinel")
			assert.NotContains(t, fmt.Sprintf("%+v", err), "secret-sentinel")
			copy := err.Metadata()
			copy.kind = KindRejected
			assert.Equal(t, KindRejected, copy.Kind())
			assert.Equal(t, KindInternal, err.Metadata().Kind(), "metadata is returned by value")
		})
	}
	for _, code := range []string{"invalid_request", "invalid_client", "invalid_grant", "unauthorized_client", "unsupported_grant_type", "invalid_scope", "invalid_target", "invalid_token", "access_denied", "server_error", "temporarily_unavailable", "slow_down", "reauth_required"} {
		assert.Equal(t, code, NewErrorMetadata(OperationExchange, KindRejected, DependencyBroker, http.StatusBadRequest, code).OAuthCode())
	}
	brokerErr := &BrokerExchangeError{StatusCode: 10000, Code: "code-secret-sentinel", Description: "description-secret-sentinel", ErrorURI: "https://uri-secret-sentinel.invalid"}
	assert.NotContains(t, brokerErr.Error(), "secret-sentinel")
	assert.Zero(t, brokerErr.Metadata().StatusCode())
}

func TestErrorMetadataConcurrentSnapshots(t *testing.T) {
	cause := errors.New("cause-secret-sentinel")
	err := NewOperationError(NewErrorMetadata(OperationExchange, KindUnavailable, DependencyBroker, http.StatusServiceUnavailable, "server_error"), cause)
	var workers sync.WaitGroup
	var mismatches atomic.Int32
	for range 32 {
		workers.Go(func() {
			for range 100 {
				copy := err.Metadata()
				copy.kind = KindRejected
				if copy.Kind() != KindRejected || err.Metadata().Kind() != KindUnavailable || !errors.Is(err, cause) || strings.Contains(err.Error(), "cause-secret-sentinel") {
					mismatches.Add(1)
				}
			}
		})
	}
	workers.Wait()
	assert.Zero(t, mismatches.Load())
}

func TestAssertionFailureMetadata(t *testing.T) {
	cases := []struct {
		name   string
		cause  error
		kind   ErrorKind
		status int
		code   string
	}{
		{"invalid client", &oauth2.RetrieveError{Response: &http.Response{StatusCode: 401}, ErrorCode: "invalid_client", ErrorDescription: "description-secret-sentinel"}, KindRejected, 401, "invalid_client"},
		{"provider outage", &oauth2.RetrieveError{Response: &http.Response{StatusCode: 503}, ErrorCode: "server_error"}, KindUnavailable, 503, "server_error"},
		{"throttled", &oauth2.RetrieveError{Response: &http.Response{StatusCode: 429}, ErrorCode: "slow_down"}, KindUnavailable, 429, "slow_down"},
		{"untrusted code", &oauth2.RetrieveError{Response: &http.Response{StatusCode: 400}, ErrorCode: "code-secret-sentinel"}, KindRejected, 400, "unknown"},
		{"network", &url.Error{Op: "Post", URL: "https://url-secret-sentinel.invalid", Err: errors.New("nested-secret-sentinel")}, KindUnavailable, 0, "unknown"},
		{"shared timeout", context.DeadlineExceeded, KindUnavailable, 0, "unknown"},
		{"invalid response", &json.SyntaxError{}, KindInvalidResponse, 0, "unknown"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			metadata := assertionErrorMetadata(fmt.Errorf("nested-secret-sentinel: %w", test.cause))
			assert.Equal(t, OperationAssertionRefresh, metadata.Operation())
			assert.Equal(t, DependencyOAuthProvider, metadata.Dependency())
			assert.Equal(t, test.kind, metadata.Kind())
			assert.Equal(t, test.status, metadata.StatusCode())
			assert.Equal(t, test.code, metadata.OAuthCode())
		})
	}
}

func TestNetworkExchangeAndAssertionTelemetryContainsNoSecrets(t *testing.T) {
	recorder, reader := installSafetyTelemetry(t)
	const (
		clientID     = "client-id-secret-sentinel"
		clientSecret = "client-secret-sentinel"
		assertion    = "client-assertion-secret-sentinel"
		subject      = "subject-token-secret-sentinel"
		resource     = "https://resource-host-secret-sentinel.invalid/resource-path-secret-sentinel?token=resource-query-secret-sentinel#fragment-secret-sentinel"
		errorCode    = "broker-code-secret-sentinel"
		description  = "broker-description-secret-sentinel"
		recoveryURL  = "https://recovery-host-secret-sentinel.invalid/recovery-path-secret-sentinel?secret=recovery-query-secret-sentinel"
	)
	var assertionCalls atomic.Int32
	seen := make(chan url.Values, 3)
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.ParseForm() != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		seen <- request.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Secret", "response-header-secret-sentinel")
		if request.PostForm.Get("grant_type") == "client_credentials" && assertionCalls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": assertion, "id_token": assertion, "token_type": "Bearer", "expires_in": 3600})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": errorCode, "error_description": description, "error_uri": recoveryURL})
	}))
	defer broker.Close()
	cfg := &extprocconfig.Config{
		OAuth2: extprocconfig.OAuth2Config{TokenEndpoint: broker.URL + "/exchange-path-secret-sentinel?credential=endpoint-query-secret-sentinel",
			ClientCredentialsEndpoint: broker.URL + "/assertion-path-secret-sentinel?credential=endpoint-query-secret-sentinel",
			ClientID:                  clientID, ClientSecret: clientSecret, ClientAssertionType: "id_token", ExchangeTimeout: time.Second},
		Cache:     extprocconfig.CacheConfig{DefaultTTL: time.Minute, MaxTTL: time.Hour},
		Telemetry: extprocconfig.TelemetryConfig{Enabled: true, Traces: extprocconfig.TracesConfig{Enabled: true}, Metrics: extprocconfig.MetricsConfig{Enabled: true}},
	}
	client, err := httpclient.New(cfg, time.Second)
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	exchanger := &TokenExchanger{cfg: cfg, client: client, logger: logger, cache: make(map[tokenCacheKey]*cachedToken)}
	require.NoError(t, exchanger.refreshClientAssertion())
	initialForm := <-seen
	assert.Equal(t, clientID, initialForm.Get("client_id"))
	assert.Equal(t, clientSecret, initialForm.Get("client_secret"))
	_, exchangeErr := exchanger.Exchange(context.Background(), subject, resource)
	require.Error(t, exchangeErr)
	exchangeForm := <-seen
	assert.Equal(t, subject, exchangeForm.Get("subject_token"))
	assert.Equal(t, assertion, exchangeForm.Get("client_assertion"))
	assert.Equal(t, resource, exchangeForm.Get("resource"))
	var brokerErr *BrokerExchangeError
	require.ErrorAs(t, exchangeErr, &brokerErr)
	assert.Equal(t, description, brokerErr.Description, "protocol information is retained structurally")
	assert.Equal(t, recoveryURL, brokerErr.ErrorURI)
	assert.NotContains(t, exchangeErr.Error(), "secret-sentinel")

	// Exercise the actual server error telemetry with an untrusted wrapping string.
	server := NewServer(cfg, &safetyExchanger{err: fmt.Errorf("wrapper-secret-sentinel: %w", exchangeErr)}, logger)
	headers := &extprocv3.HttpHeaders{Headers: &corev3.HeaderMap{Headers: []*corev3.HeaderValue{{Key: "Authorization", RawValue: []byte("header-secret-sentinel")}}}}
	request := safetyRequest(subject, resource, headers)
	response := server.processRequestHeaders(context.Background(), request, headers)
	require.NotNil(t, response.GetImmediateResponse())
	assert.Contains(t, string(response.GetImmediateResponse().GetBody()), recoveryURL, "elicitation URL remains in direct protocol response")
	assert.Contains(t, string(response.GetImmediateResponse().GetBody()), description)
	refreshErr := exchanger.refreshClientAssertion()
	require.Error(t, refreshErr)
	<-seen
	var retrieveErr *oauth2.RetrieveError
	require.ErrorAs(t, refreshErr, &retrieveErr)
	assert.Equal(t, description, retrieveErr.ErrorDescription, "the original provider error is still accessible")
	assert.NotContains(t, refreshErr.Error(), "secret-sentinel")
	require.NotEmpty(t, recorder.Ended())
	require.NotEmpty(t, logs.String())
	assert.Contains(t, logs.String(), `"oauth_error_code":"unknown"`)
	assertSafetyTelemetry(t, recorder, reader, logs.String(), clientID, clientSecret, assertion, subject, resource, errorCode, description, recoveryURL,
		"exchange-path-secret-sentinel", "assertion-path-secret-sentinel", "endpoint-query-secret-sentinel", "response-header-secret-sentinel", "header-secret-sentinel", "wrapper-secret-sentinel", broker.URL)
}

type safetyExchanger struct{ err error }

func (e *safetyExchanger) Exchange(context.Context, string, string) (ExchangeResult, error) {
	return ExchangeResult{Token: "exchanged-token-secret-sentinel"}, e.err
}
func (*safetyExchanger) Shutdown() {}

func safetyRequest(subject, resource string, headers *extprocv3.HttpHeaders) *extprocv3.ProcessingRequest {
	return &extprocv3.ProcessingRequest{Request: &extprocv3.ProcessingRequest_RequestHeaders{RequestHeaders: headers},
		MetadataContext: &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{
			tokenExchangeMetadataNamespace:  {Fields: map[string]*structpb.Value{subjectTokenFieldKey: structpb.NewStringValue(subject), resourceURIFieldKey: structpb.NewStringValue(resource)}},
			agentgatewayProtocolMetadataKey: {Fields: map[string]*structpb.Value{agentgatewayProtocolFieldKey: structpb.NewStringValue("mcp"), agentgatewayMCPServerFieldKey: structpb.NewStringValue("target-server-secret-sentinel")}},
		}}}
}

type safetyAuthorizer struct {
	err     error
	reasons []string
}

func (a *safetyAuthorizer) Evaluate(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
	return &authorization.OPADecision{Action: "deny", Reasons: a.reasons}, a.err
}
func (*safetyAuthorizer) Stop(context.Context) {}

func TestOPABodyHeaderAndPolicyTextNeverReachTelemetry(t *testing.T) {
	for _, phase := range []string{"body", "batch", "header-only"} {
		for _, failure := range []string{"evaluation", "policy-denial"} {
			t.Run(phase+"/"+failure, func(t *testing.T) {
				recorder, reader := installSafetyTelemetry(t)
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
				authorizer := &safetyAuthorizer{reasons: []string{"policy-reason-body-header-secret-sentinel"}}
				if failure == "evaluation" {
					authorizer.err = fmt.Errorf("nested-policy-secret-sentinel: %w", errors.New("request-body-header-secret-sentinel"))
				}
				cfg := &extprocconfig.Config{Telemetry: extprocconfig.TelemetryConfig{Enabled: true, Traces: extprocconfig.TracesConfig{Enabled: true}, Metrics: extprocconfig.MetricsConfig{Enabled: true}}}
				server := NewServerWithAuthorizer(cfg, &safetyExchanger{}, authorizer, logger)
				ctx, finish := server.beginTokenExchangeObservation(context.Background())
				state := &requestState{protocol: "mcp", resourceURI: "https://resource-secret-sentinel.invalid/path", targetServerName: "target-server-secret-sentinel",
					headers: map[string]string{":method": "POST", "authorization": "header-secret-sentinel"}, finishObservation: finish}
				body := []byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"tool-secret-sentinel","arguments":{"secret":"body-secret-sentinel"}}}`)
				var response *extprocv3.ProcessingResponse
				if phase == "header-only" {
					state.headers[":method"] = "GET"
					response = server.processHeadersOnlyOPA(ctx, state)
				} else {
					if phase == "batch" {
						body = append(append([]byte{'['}, body...), ']')
					}
					response = server.processRequestBody(ctx, state, &extprocv3.HttpBody{Body: body})
					finish(NewDiagnostic(StageClientAuthorization, DetailPolicyDenied))
				}
				require.NotNil(t, response.GetImmediateResponse())
				require.NotEmpty(t, logs.String())
				assertSafetyTelemetry(t, recorder, reader, logs.String(), "resource-secret-sentinel", "target-server-secret-sentinel", "header-secret-sentinel", "body-secret-sentinel", "tool-secret-sentinel", "policy-reason-body-header-secret-sentinel", "nested-policy-secret-sentinel", "request-body-header-secret-sentinel")
			})
		}
	}
}

func TestExchangeMetadataDistinguishesCallerAndSharedDeadline(t *testing.T) {
	shared := NewOperationError(NewErrorMetadata(OperationExchange, KindUnavailable, DependencyBroker, 0, ""), context.DeadlineExceeded)
	assert.Equal(t, KindUnavailable, metadataFromError(context.Background(), shared).Kind())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	caller := NewOperationError(NewErrorMetadata(OperationExchange, KindCallerCanceled, DependencyLocal, 0, ""), ctx.Err())
	require.ErrorIs(t, caller, context.Canceled)
	assert.Equal(t, KindCallerCanceled, metadataFromError(ctx, caller).Kind())
	assert.Equal(t, DependencyLocal, metadataFromError(ctx, caller).Dependency())
}

func TestAssertionRefreshOwnedDeadlineNeverLabelsCallerCancellation(t *testing.T) {
	recorder, reader := installSafetyTelemetry(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		select {
		case <-request.Context().Done():
		case <-time.After(time.Second):
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer provider.Close()
	var logs bytes.Buffer
	cfg := &extprocconfig.Config{OAuth2: extprocconfig.OAuth2Config{ClientCredentialsEndpoint: provider.URL + "/endpoint-path-secret-sentinel?token=query-secret-sentinel",
		ClientID: "client-id-secret-sentinel", ClientSecret: "client-secret-sentinel", ClientAssertionType: "access_token", ExchangeTimeout: 50 * time.Millisecond}}
	client, err := httpclient.New(cfg, cfg.OAuth2.ExchangeTimeout)
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	exchanger := &TokenExchanger{cfg: cfg, client: client, logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	err = exchanger.refreshClientAssertion()
	require.Error(t, err)
	var operationErr *OperationError
	require.ErrorAs(t, err, &operationErr)
	assert.Equal(t, OperationAssertionRefresh, operationErr.Metadata().Operation())
	assert.Equal(t, KindUnavailable, operationErr.Metadata().Kind())
	assert.NotContains(t, logs.String(), "caller_canceled")
	assert.Contains(t, logs.String(), `"operation":"assertion_refresh"`)
	assert.Contains(t, logs.String(), `"error_kind":"unavailable"`)
	assertSafetyTelemetry(t, recorder, reader, logs.String(), "endpoint-path-secret-sentinel", "query-secret-sentinel", "client-id-secret-sentinel", "client-secret-sentinel", provider.URL)
}
