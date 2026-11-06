package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

// Spec: US1 S4 — Every failure answered with a 5xx records a server error on the span.
func TestServer_TokenExchangeSpan_UnavailableResponsesAreErrors(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantHTTP      httpv3.StatusCode
		wantErrorType string
	}{
		{"assertion expired", server.ErrAssertionExpired, httpv3.StatusCode_ServiceUnavailable, "assertion_expired"},
		{"circuit open", server.ErrCircuitOpen, httpv3.StatusCode_ServiceUnavailable, "circuit_open"},
		{"unclassified failure", errors.New("unexpected"), httpv3.StatusCode_InternalServerError, "internal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := installSpanRecorder(t)
			exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
				return server.ExchangeResult{}, test.err
			}}
			client, cleanup := startTestServerWithConfig(t, testConfigWithTelemetry(), exchanger)
			defer cleanup()

			resp, err := sendRequestHeadersWithMetadata(t, client, nil, validTokenExchangeMetadata(""), false)
			require.NoError(t, err)
			require.Equal(t, int32(test.wantHTTP), int32(resp.GetImmediateResponse().GetStatus().GetCode()))

			span := findTokenExchangeSpan(t, recorder.Ended())
			assert.Equal(t, trace.SpanKindServer, span.SpanKind())
			assert.Equal(t, otelcodes.Error, span.Status().Code)
			assert.Equal(t, test.wantErrorType, attributeMap(span.Attributes())["error.type"])
		})
	}
}

// The real exchanger reports cancellation through an OperationError, which is a different path
// from a plain context error.
func TestServer_TokenExchangeSpan_CancelDuringBrokerCallIsNotAnError(t *testing.T) {
	recorder := installSpanRecorder(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	broker := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
	}))
	defer broker.Close()
	defer close(release)

	mocks := newMockServers()
	defer mocks.Close()
	cfg := configForMocks(mocks)
	cfg.OAuth2.TokenEndpoint = broker.URL + "/token"
	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	client, cleanup := startTestServerWithConfig(t, testConfigWithTelemetry(), exchanger)
	defer cleanup()

	assertCanceledExchangeSpan(t, recorder, client, started)
}

// assertCanceledExchangeSpan sends a request, cancels the stream once the exchange has started,
// and checks that the span records an outcome and no error.
func assertCanceledExchangeSpan(t *testing.T, recorder *tracetest.SpanRecorder, client extprocv3.ExternalProcessorClient, started <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.Process(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&extprocv3.ProcessingRequest{
		MetadataContext: validTokenExchangeMetadata(""),
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: extProcHeaders(nil, false),
		},
	}))
	<-started
	cancel()

	var span sdktrace.ReadOnlySpan
	require.Eventually(t, func() bool {
		for _, ended := range recorder.Ended() {
			if ended.Name() == "extproc.token_exchange" {
				span = ended
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, "canceled", attributeMap(span.Attributes())["token_exchange.outcome"])
	assert.Equal(t, trace.SpanKindServer, span.SpanKind())
	assert.Equal(t, otelcodes.Unset, span.Status().Code)
}
