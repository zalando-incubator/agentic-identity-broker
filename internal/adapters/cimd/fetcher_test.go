package cimd

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	domaincimd "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/cimd"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cimdDoc builds a minimal valid CIMD JSON document for the given URL.
func cimdDoc(t *testing.T, clientID string, redirectURIs []string) []byte {
	t.Helper()
	doc := map[string]any{
		"client_id":     clientID,
		"client_name":   "Test Agent",
		"redirect_uris": redirectURIs,
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return data
}

func TestFetcher_Success(t *testing.T) {
	var dialCount int

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dialCount++
		redirectURIs := []string{"https://" + r.Host + "/callback"}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=300")
		w.Header().Set("ETag", `"v1"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cimdDoc(t, "https://"+r.Host+r.URL.Path, redirectURIs))
	}))
	defer srv.Close()

	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)

	fetcher := NewFetcherWithClient(srv.Client(), bl, 5120)

	targetURL := srv.URL + "/client"
	result, err := fetcher.Fetch(context.Background(), targetURL)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Body)
	assert.Equal(t, "max-age=300", result.CacheControl)
	assert.Equal(t, 1, dialCount)
}

func TestFetcher_NonJSONContentType(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer srv.Close()

	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)

	fetcher := NewFetcherWithClient(srv.Client(), bl, 5120)
	_, err = fetcher.Fetch(context.Background(), srv.URL+"/client")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-JSON Content-Type")
}

func TestFetcher_Non200(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)

	fetcher := NewFetcherWithClient(srv.Client(), bl, 5120)
	_, err = fetcher.Fetch(context.Background(), srv.URL+"/client")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
}

func TestFetcher_OversizedResponse(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		large := make([]byte, 6000) // larger than the 5120 limit
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(large)
	}))
	defer srv.Close()

	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)

	fetcher := NewFetcherWithClient(srv.Client(), bl, 5120)
	_, err = fetcher.Fetch(context.Background(), srv.URL+"/client")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "5120 byte limit")
}

func TestFetcher_Redirect_Blocked(t *testing.T) {
	var srvURL string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srvURL+"/other", http.StatusFound)
	}))
	srvURL = srv.URL
	defer srv.Close()

	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)

	fetcher := NewFetcherWithClient(srv.Client(), bl, 5120)
	_, err = fetcher.Fetch(context.Background(), srv.URL+"/client")
	require.Error(t, err)
	// Redirect returns 302 which is not 200
	assert.Contains(t, err.Error(), "status 302")
}

func TestFetcher_ContextCanceled(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until context canceled
		<-r.Context().Done()
	}))
	defer srv.Close()

	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)

	fetcher := NewFetcherWithClient(srv.Client(), bl, 5120)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = fetcher.Fetch(ctx, srv.URL+"/client")
	require.Error(t, err)
}

// dialSpy wraps the SSRF control and records every address it was called with.
// This lets tests assert that the control fired before any TCP SYN was sent.
type dialSpy struct {
	inner func(string, string, syscall.RawConn) error
	calls []string
}

func (s *dialSpy) control(network, address string, c syscall.RawConn) error {
	s.calls = append(s.calls, address)
	return s.inner(network, address, c)
}

// TestFetcher_SSRF_* tests verify that buildSSRFControl rejects each blocked
// IP category before any TCP connection is established. The control fires after
// DNS resolution but before the TCP SYN packet — "no dial" means no IP-layer
// packet was sent to the blocked destination.

func TestFetcher_SSRF_BlocksLoopback(t *testing.T) {
	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)
	spy := &dialSpy{inner: buildSSRFControl(bl)}

	dialer := &net.Dialer{Timeout: 100 * time.Millisecond, Control: spy.control}
	_, err = dialer.DialContext(context.Background(), "tcp", "127.0.0.1:443")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked range")
	assert.Equal(t, []string{"127.0.0.1:443"}, spy.calls, "control must fire before TCP connect")
}

func TestFetcher_SSRF_BlocksLoopbackIPv6(t *testing.T) {
	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)
	spy := &dialSpy{inner: buildSSRFControl(bl)}

	dialer := &net.Dialer{Timeout: 100 * time.Millisecond, Control: spy.control}
	_, err = dialer.DialContext(context.Background(), "tcp", "[::1]:443")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked range")
	assert.Len(t, spy.calls, 1, "control must fire before TCP connect")
}

func TestFetcher_SSRF_BlocksPrivateRFC1918(t *testing.T) {
	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)
	control := buildSSRFControl(bl)

	for _, addr := range []string{"10.0.0.1:443", "172.16.0.1:443", "192.168.1.1:443"} {
		err := control("tcp", addr, nil)
		require.Errorf(t, err, "expected %s to be blocked", addr)
		assert.Contains(t, err.Error(), "blocked range")
	}
}

func TestFetcher_SSRF_BlocksLinkLocal(t *testing.T) {
	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)
	control := buildSSRFControl(bl)

	// 169.254.169.254 is the cloud instance metadata service
	err = control("tcp", "169.254.169.254:80", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked range")
}

func TestFetcher_SSRF_BlocksLinkLocalIPv6(t *testing.T) {
	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)
	control := buildSSRFControl(bl)

	err = control("tcp", "[fe80::1]:443", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked range")
}

func TestFetcher_SSRF_BlocksIPv4TranslationBeforeConnect(t *testing.T) {
	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)
	control := buildSSRFControl(bl)

	for _, addr := range []string{
		"[64:ff9b::a9fe:a9fe]:443",
		"[64:ff9b:1::a9fe:a9fe]:443",
		"[2002:0a00:0001::]:443",
		"[2001:0:0a00:0001::]:443",
	} {
		t.Run(addr, func(t *testing.T) {
			err := control("tcp", addr, nil)
			require.ErrorContains(t, err, "blocked range")
		})
	}
}

func TestFetcher_SSRF_BlockedURLNeverReachesServer(t *testing.T) {
	var reached atomic.Bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		w.Header().Set("Content-Type", "application/json")
	}))
	defer srv.Close()

	fetcher, err := NewFetcher(time.Second, 5120, nil)
	require.NoError(t, err)
	_, err = fetcher.Fetch(context.Background(), srv.URL+"/client")
	var blocked *ports.SSRFBlockedError
	require.ErrorAs(t, err, &blocked)
	assert.False(t, reached.Load())
}

func TestFetcher_SSRF_AllowsPublicIP(t *testing.T) {
	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)
	control := buildSSRFControl(bl)

	// 8.8.8.8 is a public IP — must not be blocked
	require.NoError(t, control("tcp", "8.8.8.8:443", nil))
}

func TestFetcher_SSRF_OperatorExtraCIDRBlocked(t *testing.T) {
	bl, err := domaincimd.NewSSRFBlocklist([]string{"203.0.113.0/24"})
	require.NoError(t, err)
	control := buildSSRFControl(bl)

	err = control("tcp", "203.0.113.42:443", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked range")
}

func TestFetcher_WrapTransport(t *testing.T) {
	bl, err := domaincimd.NewSSRFBlocklist(nil)
	require.NoError(t, err)

	type sentinelTransport struct{ http.RoundTripper }

	base := http.DefaultTransport
	fetcher := NewFetcherWithClient(&http.Client{Transport: base}, bl, 5120)
	fetcher.WrapTransport(func(inner http.RoundTripper) http.RoundTripper {
		assert.Same(t, base, inner)
		return sentinelTransport{inner}
	})

	_, ok := fetcher.client.Transport.(sentinelTransport)
	assert.True(t, ok, "transport should be wrapped by WrapTransport")
}

func TestFetcher_SSRF_OperatorExtraCIDRDoesNotBlockOthers(t *testing.T) {
	bl, err := domaincimd.NewSSRFBlocklist([]string{"203.0.113.0/24"})
	require.NoError(t, err)
	control := buildSSRFControl(bl)

	// A different public IP must still be allowed
	require.NoError(t, control("tcp", "8.8.8.8:443", nil))
}
