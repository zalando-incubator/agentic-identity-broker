package jwks

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/telemetryhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestJWKSFailureTelemetryOmitsURLAndProviderSecrets(t *testing.T) {
	const secret = "SENTINEL_JWKS_SECRET"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/"+secret, r.URL.Path)
		w.Header().Set("X-Provider-Credential", secret)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, secret)
	}))
	defer server.Close()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	client := server.Client()
	client.Timeout = time.Second
	client.Transport = telemetryhttp.NewTransport(client.Transport)
	var logs safeLogBuffer
	endpoint := strings.Replace(server.URL, "://", "://user:"+secret+"@", 1) + "/" + secret + "?credential=" + secret + "#" + secret
	adapter, err := NewJWKSAdapter(endpoint, client, time.Hour, time.Hour, slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = adapter.GetKeySet(ctx)
	require.Error(t, err)
	require.NoError(t, adapter.Shutdown(context.Background()))
	assert.NotContains(t, logs.String(), secret)
	spans := recorder.Ended()
	require.NotEmpty(t, spans)
	for _, span := range spans {
		assert.NotContains(t, fmt.Sprint(span.Name(), span.Attributes(), span.Events(), span.Status(), span.Resource().Attributes()), secret)
	}
}
