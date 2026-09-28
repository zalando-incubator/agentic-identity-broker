package httpclient

import (
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRejectsRedirects(t *testing.T) {
	var redirectRequests atomic.Int32
	redirect := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectRequests.Add(1)
	}))
	defer redirect.Close()

	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirect.URL, http.StatusFound)
	}))
	defer broker.Close()

	client, err := New(&extprocconfig.Config{}, time.Second)
	require.NoError(t, err)

	response, err := client.Get(broker.URL)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	assert.Equal(t, http.StatusFound, response.StatusCode)
	assert.Zero(t, redirectRequests.Load())
}

func TestNewNegotiatesHTTP2WithCustomCA(t *testing.T) {
	broker := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	broker.EnableHTTP2 = true
	broker.StartTLS()
	defer broker.Close()

	client, err := New(trustedBrokerConfig(t, broker), 5*time.Second)
	require.NoError(t, err)

	response, err := client.Get(broker.URL)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	assert.Equal(t, http.StatusNoContent, response.StatusCode)
	assert.Equal(t, 2, response.ProtoMajor)
}

func TestNewReusesConcurrentHTTPSConnections(t *testing.T) {
	const concurrentRequests = 6
	started := make(chan struct{}, concurrentRequests)
	permit := make(chan struct{}, concurrentRequests)
	var newConnections atomic.Int32

	broker := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		started <- struct{}{}
		<-permit
		w.WriteHeader(http.StatusNoContent)
	}))
	broker.EnableHTTP2 = false
	broker.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections.Add(1)
		}
	}
	broker.StartTLS()
	defer broker.Close()
	defer close(permit)

	client, err := New(trustedBrokerConfig(t, broker), 5*time.Second)
	require.NoError(t, err)

	for range 2 {
		results := make(chan error, concurrentRequests)
		for range concurrentRequests {
			go func() {
				response, err := client.Get(broker.URL)
				if err == nil {
					err = response.Body.Close()
				}
				results <- err
			}()
		}
		for range concurrentRequests {
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for concurrent requests")
			}
		}
		for range concurrentRequests {
			permit <- struct{}{}
		}
		for range concurrentRequests {
			require.NoError(t, <-results)
		}
	}

	assert.Equal(t, int32(concurrentRequests), newConnections.Load())
}

func trustedBrokerConfig(t *testing.T, broker *httptest.Server) *extprocconfig.Config {
	t.Helper()
	caFile := filepath.Join(t.TempDir(), "broker-ca.pem")
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: broker.Certificate().Raw})
	require.NoError(t, os.WriteFile(caFile, cert, 0o600))
	return &extprocconfig.Config{OAuth2: extprocconfig.OAuth2Config{
		TLS: extprocconfig.TLSConfig{CaBundlePath: caFile},
	}}
}
