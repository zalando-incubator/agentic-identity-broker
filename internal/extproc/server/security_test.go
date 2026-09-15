package server

import (
	"context"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/status"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

// SR-001 regression: validateResourceURI must not echo raw URI (including
// sensitive query parameters) in its error message.
func TestValidateResourceURI_DoesNotLeakQueryParams(t *testing.T) {
	err := validateResourceURI("https://example.com/%zz?access_token=secret123")
	require.Error(t, err, "malformed URI must fail validation")
	assert.NotContains(t, err.Error(), "secret123",
		"SR-001: parse error must not echo raw URI with sensitive query parameters")
	assert.Contains(t, err.Error(), "parse error",
		"error should use the generic sanitized message")
}

// SR-001 regression: validateResourceURI must not echo the parsed URI scheme
// for unexpected scheme types (e.g. data:, javascript:, ftp:).
func TestValidateResourceURI_DoesNotLeakScheme(t *testing.T) {
	tests := []struct {
		uri    string
		scheme string // the scheme that must NOT appear in the error
	}{
		{"data:text/plain,hello", "data"},
		{"javascript:alert(1)", "javascript"},
		{"ftp://files.example.com/secret.txt", "ftp"},
	}
	for _, tc := range tests {
		err := validateResourceURI(tc.uri)
		require.Error(t, err, "non-http(s) scheme URI %q must fail validation", tc.uri)
		assert.Contains(t, err.Error(), "http or https scheme",
			"error should use generic scheme message")
		// Check the error does not contain the scheme in any case form.
		errLower := strings.ToLower(err.Error())
		assert.NotContains(t, errLower, tc.scheme,
			"SR-001: error must not contain the parsed scheme %q in any form", tc.scheme)
	}
}

func TestValidateResourceURI_UsesSourceNeutralErrors(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{name: "empty", uri: "", want: "empty resource URI"},
		{name: "malformed", uri: "https://example.com/%zz", want: "invalid resource URI: parse error"},
		{name: "unsupported scheme", uri: "ftp://example.com/mcp", want: "resource URI must have http or https scheme"},
		{name: "missing host", uri: "https:/mcp", want: "resource URI must have a non-empty host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResourceURI(tt.uri)
			require.Error(t, err)
			assert.Equal(t, tt.want, status.Convert(err).Message())
		})
	}
}

func TestSanitizeURIForTelemetry_RedactsCredentialsAndParseFailures(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"userinfo stripped", "https://user:secret@example.com/resource?access_token=SECRET", "https://example.com/resource"},
		{"unparsable userinfo redacted", "https://user:secret@example.com/%zz", "[invalid resource URI]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeURIForTelemetry(tt.in)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "user:secret")
			assert.NotContains(t, got, "SECRET")
		})
	}
}

func TestExtractTraceContext_SelectsFirstValidDuplicateTraceparent(t *testing.T) {
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previousPropagator) })

	const validTraceparent = "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
	s := &Server{cfg: &extprocconfig.Config{Telemetry: extprocconfig.TelemetryConfig{
		Enabled: true,
		Traces:  extprocconfig.TracesConfig{Enabled: true},
	}}}
	headers := &extprocv3.HttpHeaders{Headers: &corev3.HeaderMap{Headers: []*corev3.HeaderValue{
		{Key: "traceparent", Value: "not-a-valid-traceparent"},
		{Key: "traceparent", Value: validTraceparent},
	}}}

	ctx := s.extractTraceContext(context.Background(), headers)
	spanContext := trace.SpanContextFromContext(ctx)
	require.True(t, spanContext.IsValid())
	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", spanContext.TraceID().String())
}

func TestHeaderCarrier_Get_DuplicateTraceparent_ReturnsFirst(t *testing.T) {
	// traceparent is single-valued; duplicate headers must not be concatenated.
	headers := &extprocv3.HttpHeaders{
		Headers: &corev3.HeaderMap{
			Headers: []*corev3.HeaderValue{
				{Key: "traceparent", Value: "00-aaaa-bbbb-01"},
				{Key: "traceparent", Value: "00-cccc-dddd-01"},
			},
		},
	}
	carrier := (*headerCarrier)(headers)
	got := carrier.Get("traceparent")
	assert.Equal(t, "00-aaaa-bbbb-01", got, "single-valued header must return first match only")
}

func TestHeaderCarrier_Get_DuplicateBaggage_ConcatenatesAll(t *testing.T) {
	// baggage is list-valued; duplicate headers must be comma-joined per RFC 9110.
	headers := &extprocv3.HttpHeaders{
		Headers: &corev3.HeaderMap{
			Headers: []*corev3.HeaderValue{
				{Key: "baggage", Value: "key1=val1"},
				{Key: "baggage", Value: "key2=val2"},
			},
		},
	}
	carrier := (*headerCarrier)(headers)
	got := carrier.Get("baggage")
	assert.Equal(t, "key1=val1,key2=val2", got, "list-valued header must concatenate all values")
}

func TestHeaderCarrier_Get_DuplicateTracestate_ConcatenatesAll(t *testing.T) {
	// tracestate is list-valued; duplicate headers must be comma-joined.
	headers := &extprocv3.HttpHeaders{
		Headers: &corev3.HeaderMap{
			Headers: []*corev3.HeaderValue{
				{Key: "tracestate", Value: "vendor1=abc"},
				{Key: "tracestate", Value: "vendor2=def"},
			},
		},
	}
	carrier := (*headerCarrier)(headers)
	got := carrier.Get("tracestate")
	assert.Equal(t, "vendor1=abc,vendor2=def", got, "tracestate must concatenate all values")
}

func TestHeaderCarrier_Keys_DeduplicatesDuplicateHeaders(t *testing.T) {
	headers := &extprocv3.HttpHeaders{
		Headers: &corev3.HeaderMap{
			Headers: []*corev3.HeaderValue{
				{Key: "traceparent", Value: "00-aaaa-bbbb-01"},
				{Key: "baggage", Value: "key1=val1"},
				{Key: "baggage", Value: "key2=val2"},
				{Key: "traceparent", Value: "00-cccc-dddd-01"},
				{Key: "authorization", Value: "Bearer token"},
			},
		},
	}
	carrier := (*headerCarrier)(headers)
	keys := carrier.Keys()
	assert.Equal(t, []string{"traceparent", "baggage", "authorization"}, keys,
		"Keys must return each header name exactly once, in first-seen order")
}

func TestHeaderCarrier_Keys_DeduplicatesMixedCaseHeaders(t *testing.T) {
	headers := &extprocv3.HttpHeaders{
		Headers: &corev3.HeaderMap{
			Headers: []*corev3.HeaderValue{
				{Key: "Baggage", Value: "key1=val1"},
				{Key: "baggage", Value: "key2=val2"},
				{Key: "BAGGAGE", Value: "key3=val3"},
			},
		},
	}
	carrier := (*headerCarrier)(headers)
	keys := carrier.Keys()
	assert.Equal(t, []string{"Baggage"}, keys,
		"Keys must deduplicate case-insensitively, keeping first-seen spelling")
}
