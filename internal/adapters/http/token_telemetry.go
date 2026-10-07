package http

import (
	"context"
	"net/http"
	"net/url"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/telemetryhttp"
	"go.opentelemetry.io/otel"
)

type originalTokenRequestKey struct{}

// Instrument a credential-free view, then restore the original request for the handler.
// otelchi otherwise records caller-controlled URL, Host and User-Agent attributes.
func tokenEndpointTelemetry(instrument func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		traced := instrument(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if original, ok := r.Context().Value(originalTokenRequestKey{}).(*http.Request); ok {
				next.ServeHTTP(w, original.WithContext(r.Context()))
				return
			}
			next.ServeHTTP(w, r)
		}))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/oauth2/token" {
				traced.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), originalTokenRequestKey{}, r)
			safe := r.WithContext(ctx)
			safe.Method = telemetryhttp.MethodName(r.Method)
			safe.URL = &url.URL{Path: "/oauth2/token"}
			safe.Host, safe.RemoteAddr, safe.RequestURI = "broker", "", "/oauth2/token"
			safe.Header = make(http.Header)
			for _, field := range otel.GetTextMapPropagator().Fields() {
				for _, value := range r.Header.Values(field) {
					safe.Header.Add(field, value)
				}
			}
			traced.ServeHTTP(w, safe)
		})
	}
}
