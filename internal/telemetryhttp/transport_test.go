package telemetryhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func installRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return recorder
}

func TestTransportNetworkPreservesSecretsWithoutRecordingThem(t *testing.T) {
	// Construct before installing the provider to exercise lazy global resolution.
	base := http.DefaultTransport.(*http.Transport).Clone()
	client := &http.Client{Transport: NewTransport(base)}
	defer client.CloseIdleConnections()
	recorder := installRecorder(t)
	const (
		host           = "endpoint-host-sentinel.invalid"
		path           = "/endpoint-path-sentinel"
		query          = "query-secret-sentinel"
		fragment       = "fragment-secret-sentinel"
		user           = "userinfo-user-sentinel"
		password       = "userinfo-password-sentinel"
		body           = "subject-token-and-client-secret-sentinel"
		header         = "Bearer authorization-header-secret-sentinel"
		responseBody   = "provider-description-secret-sentinel"
		responseHeader = "response-header-secret-sentinel"
		baggageValue   = "baggage-secret-sentinel"
	)
	type observedRequest struct {
		host, uri, body, authorization, baggage string
		spanContext                             trace.SpanContext
	}
	seen := make(chan observedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestBody, err := io.ReadAll(request.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		remote := propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(request.Header))
		seen <- observedRequest{request.Host, request.RequestURI, string(requestBody), request.Header.Get("Authorization"), request.Header.Get("Baggage"), trace.SpanContextFromContext(remote)}
		w.Header().Set("X-Provider-Secret", responseHeader)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()
	base.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	base.Proxy = nil

	member, err := baggage.NewMember("credential", baggageValue)
	require.NoError(t, err)
	bag, err := baggage.New(member)
	require.NoError(t, err)
	ctx, parent := otel.Tracer("transport-test").Start(baggage.ContextWithBaggage(context.Background(), bag), "parent")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://"+user+":"+password+"@"+host+path+"?token="+query+"#"+fragment, strings.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Authorization", header)
	originalURL := request.URL.String()
	response, err := client.Do(request)
	require.NoError(t, err)
	responseBytes, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	parent.End()
	assert.Equal(t, responseBody, string(responseBytes))
	assert.Equal(t, responseHeader, response.Header.Get("X-Provider-Secret"))
	assert.Equal(t, originalURL, request.URL.String())
	assert.Empty(t, request.Header.Get("Traceparent"), "instrumentation must not mutate caller headers")
	assert.Empty(t, request.Header.Get("Baggage"))
	observed := <-seen
	assert.Equal(t, host, observed.host)
	assert.Equal(t, path+"?token="+query, observed.uri)
	assert.Equal(t, body, observed.body)
	assert.Equal(t, header, observed.authorization)
	assert.Contains(t, observed.baggage, baggageValue)

	spans := recorder.Ended()
	require.Len(t, spans, 2)
	clientSpan := spans[0]
	assert.Equal(t, "http.request", clientSpan.Name())
	assert.Equal(t, trace.SpanKindClient, clientSpan.SpanKind())
	assert.Equal(t, "telemetryhttp", clientSpan.InstrumentationScope().Name)
	assert.Equal(t, parent.SpanContext(), clientSpan.Parent())
	assert.Equal(t, clientSpan.SpanContext().TraceID(), observed.spanContext.TraceID())
	assert.Equal(t, clientSpan.SpanContext().SpanID(), observed.spanContext.SpanID())
	assert.Equal(t, codes.Error, clientSpan.Status().Code)
	assert.Equal(t, "HTTP request rejected", clientSpan.Status().Description)
	assert.Empty(t, clientSpan.Events())
	assert.Len(t, clientSpan.Attributes(), 3, "only method, status and error type are exported")
	assertNoSpanSecrets(t, spans, host, path, query, fragment, user, password, body, header, responseBody, responseHeader, baggageValue)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type failingBody struct{ cause error }

func (b failingBody) Read([]byte) (int, error) { return 0, b.cause }
func (b failingBody) Close() error             { return b.cause }

func TestTransportRetainsErrorsWithoutExceptionEvents(t *testing.T) {
	for _, failure := range []string{"network", "read", "close"} {
		t.Run(failure, func(t *testing.T) {
			recorder := installRecorder(t)
			cause := errors.New("nested-error-secret-sentinel")
			wrapped := fmt.Errorf("https://endpoint-secret-sentinel.invalid/path: %w", cause)
			base := roundTripFunc(func(*http.Request) (*http.Response, error) {
				if failure == "network" {
					return nil, wrapped
				}
				return &http.Response{StatusCode: http.StatusOK, Body: failingBody{wrapped}}, nil
			})
			request, err := http.NewRequest("method-secret-sentinel", "http://host-secret-sentinel.invalid/path", nil)
			require.NoError(t, err)
			response, err := NewTransport(base).RoundTrip(request)
			if failure == "network" {
				require.ErrorIs(t, err, cause)
				assert.Same(t, wrapped, err)
			} else {
				require.NoError(t, err)
				if failure == "read" {
					_, err = response.Body.Read(make([]byte, 1))
					require.ErrorIs(t, err, cause)
				}
				require.ErrorIs(t, response.Body.Close(), cause)
			}
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			assert.Equal(t, codes.Error, spans[0].Status().Code)
			assert.Empty(t, spans[0].Events())
			assertNoSpanSecrets(t, spans, "nested-error-secret-sentinel", "endpoint-secret-sentinel", "method-secret-sentinel", "host-secret-sentinel")
		})
	}
}

func TestTransportNetworkBodyFailureIsRecordedOnce(t *testing.T) {
	recorder := installRecorder(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "truncated-body-secret-sentinel")
	}))
	defer server.Close()
	client := &http.Client{Transport: NewTransport(http.DefaultTransport)}
	response, err := client.Get(server.URL + "/path-secret-sentinel?secret=query-secret-sentinel")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Equal(t, "truncated-body-secret-sentinel", string(body))
	require.NoError(t, response.Body.Close())
	require.NoError(t, response.Body.Close())
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "HTTP response failed", spans[0].Status().Description)
	assertNoSpanSecrets(t, spans, "truncated-body-secret-sentinel", "path-secret-sentinel", "query-secret-sentinel", server.URL)
}

func spanAttributes(span sdktrace.ReadOnlySpan) map[string]string {
	attributes := make(map[string]string, len(span.Attributes()))
	for _, keyValue := range span.Attributes() {
		attributes[string(keyValue.Key)] = keyValue.Value.AsString()
	}
	return attributes
}

func TestTransportErrorTypeFollowsStatusCode(t *testing.T) {
	tests := []struct {
		status        int
		wantCode      codes.Code
		wantErrorType string
	}{
		{http.StatusOK, codes.Unset, ""},
		{http.StatusNotModified, codes.Unset, ""},
		{http.StatusBadRequest, codes.Error, "400"},
		{http.StatusForbidden, codes.Error, "403"},
		{http.StatusTooManyRequests, codes.Error, "429"},
		{http.StatusInternalServerError, codes.Error, "500"},
		{http.StatusServiceUnavailable, codes.Error, "503"},
		{999, codes.Error, "_OTHER"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			recorder := installRecorder(t)
			base := roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Body: http.NoBody}, nil
			})
			request, err := http.NewRequest(http.MethodGet, "http://service.invalid/path", nil)
			require.NoError(t, err)
			response, err := NewTransport(base).RoundTrip(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())

			spans := recorder.Ended()
			require.Len(t, spans, 1)
			assert.Equal(t, test.wantCode, spans[0].Status().Code)
			assert.Equal(t, test.wantErrorType, spanAttributes(spans[0])["error.type"])
		})
	}
}

// startBlockingServer returns a client and a server that never finishes a response. With
// sendHeaders, the server sends the response headers first, so the client blocks reading the body.
func startBlockingServer(t *testing.T, protocol string, sendHeaders bool) (*http.Client, string, <-chan struct{}) {
	t.Helper()
	reached := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if sendHeaders {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
		}
		reached <- struct{}{}
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	if protocol == "HTTP/2" {
		server.EnableHTTP2 = true
		server.StartTLS()
	} else {
		server.Start()
	}
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	return &http.Client{Transport: NewTransport(server.Client().Transport)}, server.URL, reached
}

func TestTransportCallerCancellationIsNotAnError(t *testing.T) {
	for _, protocol := range []string{"HTTP/1.1", "HTTP/2"} {
		for _, stage := range []string{"waiting for the response", "reading the body"} {
			t.Run(protocol+"/"+stage, func(t *testing.T) {
				recorder := installRecorder(t)
				readingBody := stage == "reading the body"
				client, url, reached := startBlockingServer(t, protocol, readingBody)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				require.NoError(t, err)

				if readingBody {
					response, err := client.Do(request)
					require.NoError(t, err)
					<-reached
					cancel()
					_, err = io.ReadAll(response.Body)
					require.ErrorIs(t, err, context.Canceled)
					_ = response.Body.Close()
				} else {
					go func() {
						<-reached
						cancel()
					}()
					_, err = client.Do(request)
					require.ErrorIs(t, err, context.Canceled)
				}

				spans := recorder.Ended()
				require.Len(t, spans, 1)
				attributes := spanAttributes(spans[0])
				assert.Equal(t, trace.SpanKindClient, spans[0].SpanKind())
				assert.Equal(t, codes.Unset, spans[0].Status().Code)
				assert.NotContains(t, attributes, "error.type")
				assert.Equal(t, "canceled", attributes["telemetryhttp.outcome"])
			})
		}
	}
}

// A timeout is a failed call. It must not look like a caller that gave up, including the
// http.Client timeout, which also closes the request's Cancel channel.
func TestTransportTimeoutIsAnError(t *testing.T) {
	const timeout = 50 * time.Millisecond
	tests := []struct {
		name          string
		sendHeaders   bool
		wantErrorType string
	}{
		{"waiting for the response", false, "network_error"},
		{"reading the body", true, "response_read_failed"},
	}
	for _, protocol := range []string{"HTTP/1.1", "HTTP/2"} {
		for _, source := range []string{"request deadline", "client timeout"} {
			for _, test := range tests {
				t.Run(protocol+"/"+source+"/"+test.name, func(t *testing.T) {
					recorder := installRecorder(t)
					client, url, _ := startBlockingServer(t, protocol, test.sendHeaders)
					ctx := context.Background()
					if source == "request deadline" {
						var cancel context.CancelFunc
						ctx, cancel = context.WithTimeout(ctx, timeout)
						defer cancel()
					} else {
						client.Timeout = timeout
					}
					request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
					require.NoError(t, err)

					response, err := client.Do(request)
					if test.sendHeaders {
						require.NoError(t, err)
						_, err = io.ReadAll(response.Body)
						_ = response.Body.Close()
					}
					require.Error(t, err)

					spans := recorder.Ended()
					require.Len(t, spans, 1)
					attributes := spanAttributes(spans[0])
					assert.Equal(t, codes.Error, spans[0].Status().Code)
					assert.Equal(t, test.wantErrorType, attributes["error.type"])
					assert.NotContains(t, attributes, "telemetryhttp.outcome")
				})
			}
		}
	}
}

func assertNoSpanSecrets(t *testing.T, spans []sdktrace.ReadOnlySpan, secrets ...string) {
	t.Helper()
	for _, span := range spans {
		encoded, err := json.Marshal(tracetest.SpanStubFromReadOnlySpan(span))
		require.NoError(t, err)
		telemetry := string(encoded) + fmt.Sprint(span.Resource().Attributes())
		for _, secret := range secrets {
			assert.NotContains(t, telemetry, secret)
		}
	}
}

func TestTransportPropagatesWithNilHeaderWithoutMutatingCaller(t *testing.T) {
	installRecorder(t)
	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	require.NoError(t, err)
	request.Header = nil
	base := roundTripFunc(func(sent *http.Request) (*http.Response, error) {
		assert.NotEmpty(t, sent.Header.Get("Traceparent"))
		return &http.Response{StatusCode: http.StatusNoContent}, nil
	})
	_, err = NewTransport(base).RoundTrip(request)
	require.NoError(t, err)
	parent.End()
	assert.Nil(t, request.Header)
}
