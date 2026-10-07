// Package telemetryhttp instruments outbound HTTP without credential-bearing data.
package telemetryhttp

import (
	"io"
	"net/http"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// NewTransport traces requests without serializing URLs, headers, bodies or
// errors. Propagation uses the actual child span; network requests retain their
// original URL, payload and existing headers. Providers resolve at request time.
func NewTransport(base http.RoundTripper) http.RoundTripper {
	return &transport{base: base}
}

type transport struct {
	base http.RoundTripper
}

func (t *transport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx, span := otel.Tracer("telemetryhttp").Start(request.Context(), "http.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("http.request.method", MethodName(request.Method))))
	request = request.WithContext(ctx)
	request.Header = request.Header.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(request.Header))
	response, err := t.base.RoundTrip(request)
	if err != nil {
		span.SetAttributes(attribute.String("error.type", "network_error"))
		span.SetStatus(codes.Error, "HTTP request failed")
		span.End()
		return response, err
	}
	if response.StatusCode >= 100 && response.StatusCode <= 599 {
		span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
	}
	if response.StatusCode >= 400 {
		span.SetStatus(codes.Error, "HTTP request rejected")
	}
	if response.Body == nil {
		span.End()
	} else {
		response.Body = &spanBody{ReadCloser: response.Body, span: span}
	}
	return response, nil
}

func (t *transport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

type spanBody struct {
	io.ReadCloser
	span trace.Span
	once sync.Once
}

func (b *spanBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.finish(err, "response_read_failed")
	}
	return n, err
}

func (b *spanBody) Close() error {
	err := b.ReadCloser.Close()
	b.finish(err, "response_close_failed")
	return err
}

func (b *spanBody) finish(err error, kind string) {
	b.once.Do(func() {
		if err != nil && err != io.EOF {
			b.span.SetAttributes(attribute.String("error.type", kind))
			b.span.SetStatus(codes.Error, "HTTP response failed")
		}
		b.span.End()
	})
}

func MethodName(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch:
		return method
	default:
		return "_OTHER"
	}
}
