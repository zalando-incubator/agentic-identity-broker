package delegation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestResolutionTelemetry_ClassifiesWithoutCredentials(t *testing.T) {
	tests := []struct {
		name, body, outcome, detail, category string
		status                                int
		assertionError                        error
	}{
		{name: "permit", status: http.StatusOK, body: `{"decision":true,"context":` + permitContextJSON + `}`, outcome: "success", detail: "none"},
		{name: "denial", status: http.StatusOK, body: `{"decision":false,"context":{"reason_admin":{"code":"grant_missing"},"reason_user":{"consent_url":"https://broker.example/consent"}}}`, outcome: "authorization_denied", detail: "delegation_denied"},
		{name: "rejected", status: http.StatusServiceUnavailable, body: testSubject + testAssertion, outcome: "infrastructure_error", detail: "delegation_unavailable", category: CategoryRejected},
		{name: "invalid response", status: http.StatusOK, body: testSubject + testAssertion, outcome: "infrastructure_error", detail: "delegation_unavailable", category: CategoryInvalidResponse},
		{name: "assertion unavailable", assertionError: errors.New("dependency-cause-marker " + testAssertion), outcome: "infrastructure_error", detail: "delegation_unavailable", category: CategoryUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			previous := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			t.Cleanup(func() {
				otel.SetTracerProvider(previous)
				require.NoError(t, provider.Shutdown(context.Background()))
			})
			broker := &brokerStub{status: tt.status, body: tt.body, cacheControl: []string{"private, max-age=60"}}
			client := newTestClient(t, broker.server(t), time.Minute)
			if tt.assertionError != nil {
				client.assertions = staticAssertion{err: tt.assertionError}
			}
			resolver := NewResolver(client, time.Minute)
			t.Cleanup(resolver.Shutdown)
			_, hit, err := resolver.Resolve(context.Background(), testSubject, testResource)
			assert.False(t, hit)
			if tt.outcome == "success" {
				require.NoError(t, err)
				_, hit, err = resolver.Resolve(context.Background(), testSubject, testResource)
				require.NoError(t, err)
				assert.True(t, hit)
			} else {
				require.Error(t, err)
			}
			spans := recorder.Ended()
			expected := 2
			if tt.outcome == "success" {
				expected = 3
			}
			require.Len(t, spans, expected, "one client span, one resolver span per call; a cache hit makes no client call")
			for _, span := range spans {
				attrs := make(map[string]any)
				for _, attr := range span.Attributes() {
					attrs[string(attr.Key)] = attr.Value.AsInterface()
				}
				assert.Equal(t, "delegation_resolution", attrs["token_exchange.operation"])
				assert.Equal(t, tt.outcome, attrs["token_exchange.outcome"])
				assert.Equal(t, tt.detail, attrs["token_exchange.failure_detail"])
				if tt.category != "" {
					assert.Equal(t, tt.category, attrs["error.type"])
					assert.Equal(t, codes.Error, span.Status().Code)
				} else {
					assert.NotEqual(t, codes.Error, span.Status().Code)
				}
				assert.Equal(t, "extproc", span.InstrumentationScope().Name)
				if span.Name() == "extproc.delegation.resolve" {
					assert.Equal(t, trace.SpanKindInternal, span.SpanKind())
					cache := "miss"
					if span == spans[len(spans)-1] && tt.outcome == "success" {
						cache = "hit"
					}
					assert.Equal(t, cache, attrs["delegation.cache"])
				} else {
					assert.Equal(t, "extproc.delegation.evaluate", span.Name())
					assert.Equal(t, trace.SpanKindClient, span.SpanKind())
				}
				encoded, marshalErr := json.Marshal(tracetest.SpanStubFromReadOnlySpan(span))
				require.NoError(t, marshalErr)
				for _, sensitive := range []string{testSubject, testAssertion, testResource, "alice@example.com", "data-assistant", "read:repo", "dependency-cause-marker", "do-not-copy"} {
					assert.NotContains(t, string(encoded), sensitive)
				}
			}
		})
	}
}
