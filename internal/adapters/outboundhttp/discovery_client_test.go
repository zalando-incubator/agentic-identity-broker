package outboundhttp

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"
)

const discoveryTestURL = "https://resource.example"

// discoveryTLSClient trusts the test server while routing a public test hostname
// to it. The adapter still validates the original URL; no production guard is disabled.
func discoveryTLSClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	client := server.Client()
	transport := client.Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = "127.0.0.1"
	originalDial := (&net.Dialer{Timeout: time.Second}).DialContext
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if host, _, err := net.SplitHostPort(address); err == nil && host == "resource.example" {
			address = server.Listener.Addr().String()
		}
		return originalDial(ctx, network, address)
	}
	client.Transport = transport
	t.Cleanup(transport.CloseIdleConnections)
	return client
}

func TestDiscoveryClient_RejectsUnsafeURLsBeforeRequest(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	client := NewDiscoveryClientWithClient(discoveryTLSClient(t, server))

	for _, rawURL := range []string{
		"http://resource.example/metadata",
		"https://user:password@resource.example/metadata",
		"https://resource.example/metadata#fragment",
		"https:///metadata",
		"/metadata",
		server.URL + "/metadata",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, _, err := client.Probe(context.Background(), rawURL)
			require.ErrorIs(t, err, ports.ErrOAuthDiscoveryUnsafeDestination)
			_, err = client.GetJSON(context.Background(), rawURL)
			require.ErrorIs(t, err, ports.ErrOAuthDiscoveryUnsafeDestination)
			_, err = client.PostJSON(context.Background(), rawURL, []byte(`{}`))
			require.ErrorIs(t, err, ports.ErrOAuthDiscoveryUnsafeDestination)
		})
	}
	assert.Zero(t, hits.Load(), "unsafe URLs must not reach the injected HTTP client")
}

func TestDiscoveryClient_ProbePreservesChallengeHeaderCardinality(t *testing.T) {
	first := `Bearer realm="one,two", resource_metadata="https://resource.example/meta-a"`
	second := `DPoP resource_metadata="https://resource.example/meta-b"`
	var hits atomic.Int32
	var closed atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
		w.Header().Add("WWW-Authenticate", first)
		w.Header().Add("WWW-Authenticate", second)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "challenge details must not leak")
	}))
	defer server.Close()

	injected := discoveryTLSClient(t, server)
	injected.Transport = &discoveryTrackingTransport{inner: injected.Transport, closed: &closed}
	status, challenges, err := NewDiscoveryClientWithClient(injected).Probe(context.Background(), discoveryTestURL+"/mcp")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, []string{first, second}, challenges, "do not split quoted commas, join fields, or discard repeated challenges")
	assert.EqualValues(t, 1, hits.Load())
	assert.EqualValues(t, 1, closed.Load(), "a challenge body must be closed even though it is not consumed")
}

func TestDiscoveryClient_GetJSONNotFoundDoesNotTryAnotherLocation(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var closed atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/.well-known/oauth-protected-resource" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{ "resource": "wrong fallback" }`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "private upstream details")
	}))
	defer server.Close()

	injected := discoveryTLSClient(t, server)
	injected.Transport = &discoveryTrackingTransport{inner: injected.Transport, closed: &closed}
	_, err := NewDiscoveryClientWithClient(injected).GetJSON(context.Background(), discoveryTestURL+"/.well-known/oauth-protected-resource/mcp")
	require.ErrorIs(t, err, ports.ErrOAuthDiscoveryNotFound)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"/.well-known/oauth-protected-resource/mcp"}, paths, "the domain, not the adapter, decides fallback order")
	assert.NotContains(t, err.Error(), "private upstream details")
	assert.EqualValues(t, 1, closed.Load(), "a 404 response body must be closed")
}

func TestDiscoveryClient_JSONWireContract(t *testing.T) {
	requestBody := []byte(`{"client_name":"Broker"}`)
	responseBody := []byte(`{"client_id":"registered"}`)
	var mu sync.Mutex
	var calls []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case "/metadata":
			assert.Equal(t, http.MethodGet, r.Method)
			_, _ = w.Write([]byte(`{"issuer":"https://auth.example"}`))
		case "/register":
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.Equal(t, requestBody, body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(responseBody)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	client := NewDiscoveryClientWithClient(discoveryTLSClient(t, server))

	metadata, err := client.GetJSON(context.Background(), discoveryTestURL+"/metadata")
	require.NoError(t, err)
	assert.JSONEq(t, `{"issuer":"https://auth.example"}`, string(metadata))
	registration, err := client.PostJSON(context.Background(), discoveryTestURL+"/register", requestBody)
	require.NoError(t, err)
	assert.Equal(t, responseBody, registration)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"GET /metadata", "POST /register"}, calls)
}

func TestDiscoveryClient_RejectsNonJSONAndWrongSuccessStatus(t *testing.T) {
	var closed atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/non-json" {
			w.Header().Set("Content-Type", "application/jsonp")
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
			}
			_, _ = io.WriteString(w, `{}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	injected := discoveryTLSClient(t, server)
	injected.Transport = &discoveryTrackingTransport{inner: injected.Transport, closed: &closed}
	client := NewDiscoveryClientWithClient(injected)

	_, err := client.GetJSON(context.Background(), discoveryTestURL+"/non-json")
	require.ErrorIs(t, err, ports.ErrOAuthDiscoveryInvalidResponse)
	_, err = client.PostJSON(context.Background(), discoveryTestURL+"/non-json", []byte(`{}`))
	require.ErrorIs(t, err, ports.ErrOAuthDiscoveryInvalidResponse)
	_, err = client.PostJSON(context.Background(), discoveryTestURL+"/wrong-registration-status", []byte(`{}`))
	require.ErrorIs(t, err, ports.ErrOAuthDiscoveryRejected)
	assert.EqualValues(t, 3, closed.Load(), "invalid content type and status must close each response body")
}

func TestDiscoveryClient_RejectsMalformedMetadataAndRegistrationJSON(t *testing.T) {
	const privateBody = `{"client_secret":"provider-secret",`
	var closed atomic.Int32
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = io.WriteString(w, privateBody)
	}))
	defer server.Close()
	injected := discoveryTLSClient(t, server)
	injected.Transport = &discoveryTrackingTransport{inner: injected.Transport, closed: &closed}
	client := NewDiscoveryClientWithClient(injected)

	_, getErr := client.GetJSON(context.Background(), discoveryTestURL+"/metadata?marker=query-secret")
	_, postErr := client.PostJSON(context.Background(), discoveryTestURL+"/register?marker=query-secret", []byte(`{}`))
	for _, err := range []error{getErr, postErr} {
		require.ErrorIs(t, err, ports.ErrOAuthDiscoveryInvalidResponse)
		assert.NotContains(t, err.Error(), "provider-secret")
		assert.NotContains(t, err.Error(), "query-secret")
	}
	assert.EqualValues(t, 2, hits.Load())
	assert.EqualValues(t, 2, closed.Load(), "malformed JSON bodies must be closed")
}

func TestDiscoveryClient_RejectsUntrustedTLSBeforeSendingRequest(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	injected := discoveryTLSClient(t, server)
	injected.Transport.(*http.Transport).TLSClientConfig.RootCAs = x509.NewCertPool()
	client := NewDiscoveryClientWithClient(injected)

	_, _, probeErr := client.Probe(context.Background(), discoveryTestURL+"/probe")
	_, getErr := client.GetJSON(context.Background(), discoveryTestURL+"/metadata")
	_, postErr := client.PostJSON(context.Background(), discoveryTestURL+"/register", []byte(`{}`))
	for _, err := range []error{probeErr, getErr, postErr} {
		require.ErrorIs(t, err, ports.ErrOAuthDiscoveryUnavailable)
	}
	assert.Zero(t, hits.Load(), "untrusted TLS peers must not receive HTTP requests")
}

func TestDiscoveryClient_DoesNotFollowRedirects(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer target.Close()
	var sourceHits atomic.Int32
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceHits.Add(1)
		http.Redirect(w, r, target.URL+"/private?secret=redirect-marker", http.StatusFound)
	}))
	defer source.Close()
	client := NewDiscoveryClientWithClient(discoveryTLSClient(t, source))

	_, _, probeErr := client.Probe(context.Background(), discoveryTestURL+"/probe")
	require.Error(t, probeErr)
	_, getErr := client.GetJSON(context.Background(), discoveryTestURL+"/metadata")
	require.Error(t, getErr)
	_, postErr := client.PostJSON(context.Background(), discoveryTestURL+"/register", []byte(`{}`))
	require.Error(t, postErr)
	assert.EqualValues(t, 3, sourceHits.Load())
	assert.Zero(t, targetHits.Load(), "a redirect must never reach a private TLS target")
	for _, err := range []error{probeErr, getErr, postErr} {
		assert.NotContains(t, err.Error(), "redirect-marker")
	}
}

func TestDiscoveryClient_DoesNotForwardRegistrationBodyOn307(t *testing.T) {
	var forwarded atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forwarded.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer target.Close()
	var registrations atomic.Int32
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		registrations.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Location", target.URL+"/collect")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	_, err := NewDiscoveryClientWithClient(discoveryTLSClient(t, source)).PostJSON(
		context.Background(), discoveryTestURL+"/register", []byte(`{"client_name":"broker"}`))
	require.ErrorIs(t, err, ports.ErrOAuthDiscoveryUnsafeDestination)
	assert.EqualValues(t, 1, registrations.Load())
	assert.Zero(t, forwarded.Load(), "a 307 must not forward a registration payload to another origin")
}

func TestDiscoveryClient_BoundsAndClosesResponseBodies(t *testing.T) {
	const limit = 256 << 10
	boundary := []byte(`{"pad":"` + strings.Repeat("x", limit-len(`{"pad":""}`)) + `"}`)
	oversized := append(append([]byte(nil), boundary[:len(boundary)-2]...), 'x', '"', '}')
	var closed atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		if r.URL.Path == "/oversized" {
			_, _ = w.Write(oversized)
		} else {
			_, _ = w.Write(boundary)
		}
	}))
	defer server.Close()
	injected := discoveryTLSClient(t, server)
	injected.Transport = &discoveryTrackingTransport{inner: injected.Transport, closed: &closed}
	client := NewDiscoveryClientWithClient(injected)

	body, err := client.GetJSON(context.Background(), discoveryTestURL+"/boundary")
	require.NoError(t, err)
	assert.Equal(t, boundary, body)
	_, err = client.GetJSON(context.Background(), discoveryTestURL+"/oversized")
	require.ErrorIs(t, err, ports.ErrOAuthDiscoveryResponseTooLarge)
	body, err = client.PostJSON(context.Background(), discoveryTestURL+"/boundary", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, boundary, body)
	_, err = client.PostJSON(context.Background(), discoveryTestURL+"/oversized", []byte(`{}`))
	require.ErrorIs(t, err, ports.ErrOAuthDiscoveryResponseTooLarge)
	assert.EqualValues(t, 4, closed.Load(), "both accepted and oversized bodies must be closed")
}

type discoveryTrackingTransport struct {
	inner  http.RoundTripper
	closed *atomic.Int32
}

func (t *discoveryTrackingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.inner.RoundTrip(request)
	if err == nil {
		response.Body = &discoveryTrackingBody{ReadCloser: response.Body, closed: t.closed}
	}
	return response, err
}

type discoveryTrackingBody struct {
	io.ReadCloser
	closed *atomic.Int32
}

func (b *discoveryTrackingBody) Close() error {
	b.closed.Add(1)
	return b.ReadCloser.Close()
}

func TestDiscoveryClient_ReusesCallerDeadlineAcrossOperations(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	client := NewDiscoveryClientWithClient(discoveryTLSClient(t, server))

	ctx, cancel := context.WithCancel(context.Background())
	_, _, err := client.Probe(ctx, discoveryTestURL+"/probe")
	require.NoError(t, err)
	cancel()
	_, _, err = client.Probe(ctx, discoveryTestURL+"/probe")
	require.ErrorIs(t, err, context.Canceled)
	_, err = client.GetJSON(ctx, discoveryTestURL+"/metadata")
	require.ErrorIs(t, err, context.Canceled)
	_, err = client.PostJSON(ctx, discoveryTestURL+"/registration", []byte(`{}`))
	require.ErrorIs(t, err, context.Canceled)
	assert.EqualValues(t, 1, hits.Load(), "later operations must not reset the caller's canceled context")

	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	_, _, probeErr := client.Probe(expired, discoveryTestURL+"/probe")
	_, getErr := client.GetJSON(expired, discoveryTestURL+"/metadata")
	_, postErr := client.PostJSON(expired, discoveryTestURL+"/registration", []byte(`{}`))
	for _, err := range []error{probeErr, getErr, postErr} {
		require.ErrorIs(t, err, ports.ErrOAuthDiscoveryTimeout)
	}
	assert.EqualValues(t, 1, hits.Load(), "no request may run after the shared deadline")
}

type discoveryDeadlineTransport struct {
	inner     http.RoundTripper
	mu        sync.Mutex
	deadlines []time.Time
}

func (t *discoveryDeadlineTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	deadline, ok := request.Context().Deadline()
	if !ok {
		return nil, errors.New("discovery request has no attempt deadline")
	}
	t.mu.Lock()
	t.deadlines = append(t.deadlines, deadline)
	t.mu.Unlock()
	return t.inner.RoundTrip(request)
}

func TestDiscoveryClient_UsesOneDeadlineForProbeMetadataAndDCR(t *testing.T) {
	release := make(chan struct{})
	var stalledRegistrations atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stall" {
			stalledRegistrations.Add(1)
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	defer close(release)
	injected := discoveryTLSClient(t, server)
	spy := &discoveryDeadlineTransport{inner: injected.Transport}
	injected.Transport = spy
	client := NewDiscoveryClientWithClient(injected)

	attempt, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	deadline, ok := attempt.Deadline()
	require.True(t, ok)
	_, _, err := client.Probe(attempt, discoveryTestURL+"/probe")
	require.NoError(t, err)
	_, err = client.GetJSON(attempt, discoveryTestURL+"/metadata")
	require.NoError(t, err)
	_, err = client.PostJSON(attempt, discoveryTestURL+"/register", []byte(`{}`))
	require.NoError(t, err)
	spy.mu.Lock()
	observed := append([]time.Time(nil), spy.deadlines...)
	spy.mu.Unlock()
	require.Len(t, observed, 3)
	for _, got := range observed {
		assert.True(t, got.Equal(deadline), "each outbound request must inherit the same attempt deadline")
	}

	shortAttempt, stop := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer stop()
	_, err = client.PostJSON(shortAttempt, discoveryTestURL+"/stall", []byte(`{}`))
	require.ErrorIs(t, err, ports.ErrOAuthDiscoveryTimeout, "in-flight DCR must stop at the attempt deadline")
	assert.Eventually(t, func() bool { return stalledRegistrations.Load() == 1 }, time.Second, 10*time.Millisecond)
}

func TestDiscoveryClient_UpstreamErrorsDoNotExposeBodyOrQuery(t *testing.T) {
	const secret = "private-provider-secret"
	var closed atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusBadGateway)
		}
		_, _ = io.WriteString(w, secret)
	}))
	defer server.Close()
	injected := discoveryTLSClient(t, server)
	injected.Transport = &discoveryTrackingTransport{inner: injected.Transport, closed: &closed}
	client := NewDiscoveryClientWithClient(injected)
	url := discoveryTestURL + "/metadata?access_token=query-marker"
	_, getErr := client.GetJSON(context.Background(), url)
	require.ErrorIs(t, getErr, ports.ErrOAuthDiscoveryUnavailable)
	_, postErr := client.PostJSON(context.Background(), url, []byte(`{}`))
	require.ErrorIs(t, postErr, ports.ErrOAuthDiscoveryRejected)
	for _, err := range []error{getErr, postErr} {
		assert.NotContains(t, err.Error(), secret)
		assert.NotContains(t, err.Error(), "query-marker")
	}
	assert.EqualValues(t, 2, closed.Load(), "rejected upstream replies must close their bodies")
}

func TestDiscoveryClient_TransportFailureDoesNotExposeQuery(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer server.Close()
	injected := discoveryTLSClient(t, server)
	injected.Transport.(*http.Transport).DialContext = func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("upstream-network-marker")
	}
	client := NewDiscoveryClientWithClient(injected)
	url := discoveryTestURL + "/metadata?access_token=query-marker"
	_, _, probeErr := client.Probe(context.Background(), url)
	_, getErr := client.GetJSON(context.Background(), url)
	_, postErr := client.PostJSON(context.Background(), url, []byte(`{}`))
	for _, err := range []error{probeErr, getErr, postErr} {
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "query-marker")
		assert.NotContains(t, err.Error(), "upstream-network-marker")
	}
	assert.Zero(t, hits.Load())
}

func TestNewGuardedTransport_BlocksPrivateDialsAndBypassesProxy(t *testing.T) {
	var proxyHits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyHits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("https_proxy", proxy.URL)
	t.Setenv("NO_PROXY", "")

	var targetHits atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	transport, err := NewGuardedTransport(nil)
	require.NoError(t, err)
	require.NotNil(t, transport)
	defer transport.CloseIdleConnections()
	require.Nil(t, transport.Proxy, "a proxy can reach private addresses without passing the dial-time IP guard")
	client := &http.Client{Transport: transport, Timeout: time.Second}
	for _, url := range []string{target.URL, "https://10.0.0.1/private", "https://169.254.169.254/latest/meta-data/"} {
		_, err := client.Get(url)
		var blocked *ports.SSRFBlockedError
		require.ErrorAs(t, err, &blocked, "private address %s must be blocked before connecting", url)
	}
	assert.Zero(t, targetHits.Load(), "loopback TLS target must receive no requests")
	assert.Zero(t, proxyHits.Load(), "HTTPS_PROXY must receive no CONNECT or request")

	require.NotNil(t, transport.DialContext)
	for _, address := range []string{"127.0.0.1:443", "[::1]:443", "10.1.2.3:443", "172.16.0.1:443", "192.168.1.1:443", "169.254.169.254:80", "[fe80::1]:443"} {
		connection, err := transport.DialContext(context.Background(), "tcp", address)
		if connection != nil {
			_ = connection.Close()
		}
		var blocked *ports.SSRFBlockedError
		require.ErrorAs(t, err, &blocked, "dial-time policy must block %s before TCP connect", address)
	}
	assert.Zero(t, proxyHits.Load())
}

func TestSSRFControl_BlocksDNSResolvedPrivateTargetsBeforeConnect(t *testing.T) {
	answers := map[string][4]byte{
		"loopback.example": {127, 0, 0, 1},
		"private.example":  {10, 1, 2, 3},
		"metadata.example": {169, 254, 169, 254},
	}
	dns, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		buf := make([]byte, 512)
		for {
			n, peer, readErr := dns.ReadFrom(buf)
			if readErr != nil {
				return
			}
			var query dnsmessage.Message
			if unpackErr := query.Unpack(buf[:n]); unpackErr != nil || len(query.Questions) != 1 {
				t.Errorf("invalid fixture DNS query: %v", unpackErr)
				return
			}
			question := query.Questions[0]
			reply := dnsmessage.Message{
				Header:    dnsmessage.Header{ID: query.ID, Response: true, RecursionAvailable: true},
				Questions: query.Questions,
			}
			if address, found := answers[strings.TrimSuffix(question.Name.String(), ".")]; found && question.Type == dnsmessage.TypeA {
				reply.Answers = []dnsmessage.Resource{{
					Header: dnsmessage.ResourceHeader{Name: question.Name, Class: dnsmessage.ClassINET},
					Body:   &dnsmessage.AResource{A: address},
				}}
			}
			wire, packErr := reply.Pack()
			if packErr != nil {
				t.Errorf("pack fixture DNS response: %v", packErr)
				return
			}
			if _, writeErr := dns.WriteTo(wire, peer); writeErr != nil {
				t.Errorf("send fixture DNS response: %v", writeErr)
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = dns.Close()
		<-finished
	})

	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", dns.LocalAddr().String())
	}}
	guard := buildSSRFControl(discoveryURLBlocklist)
	transport, err := NewGuardedTransport(nil)
	require.NoError(t, err)
	defer transport.CloseIdleConnections()
	transport.DialContext = (&net.Dialer{Resolver: resolver, Control: func(network, address string, conn syscall.RawConn) error {
		if err := guard(network, address, conn); err != nil {
			return err
		}
		return errors.New("fixture prevented a nonlocal connection")
	}}).DialContext
	client := NewDiscoveryClientWithClient(&http.Client{Transport: transport, Timeout: time.Second})

	for host := range answers {
		t.Run(host, func(t *testing.T) {
			url := "https://" + host + "/.well-known/oauth-protected-resource"
			_, err := client.GetJSON(context.Background(), url)
			require.ErrorIs(t, err, ports.ErrOAuthDiscoveryUnsafeDestination,
				"a public-looking hostname that resolves to a blocked IP must fail before TCP connect")
		})
	}
}

func TestNewGuardedTransport_HonorsExtraBlockedCIDR(t *testing.T) {
	transport, err := NewGuardedTransport([]string{"8.8.8.0/24"})
	require.NoError(t, err)
	defer transport.CloseIdleConnections()
	require.NotNil(t, transport.DialContext)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	connection, err := transport.DialContext(ctx, "tcp", "8.8.8.8:443")
	if connection != nil {
		_ = connection.Close()
	}
	var blocked *ports.SSRFBlockedError
	require.ErrorAs(t, err, &blocked, "operator CIDR must be enforced at dial time")
}

func TestNewGuardedTransport_RejectsInvalidOperatorCIDR(t *testing.T) {
	transport, err := NewGuardedTransport([]string{"not-a-cidr"})
	require.ErrorContains(t, err, "invalid blocked CIDR")
	assert.Nil(t, transport)
}
