package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type builderProtectedResourceClient struct {
	calls []string
}

func (c *builderProtectedResourceClient) Probe(_ context.Context, rawURL string) (int, []string, error) {
	c.calls = append(c.calls, rawURL)
	if rawURL != "https://mcp.example.test/mcp" {
		return 0, nil, fmt.Errorf("unexpected resource probe")
	}
	return 200, nil, nil
}

func (c *builderProtectedResourceClient) GetJSON(_ context.Context, rawURL string) ([]byte, error) {
	c.calls = append(c.calls, rawURL)
	switch rawURL {
	case "https://mcp.example.test/.well-known/oauth-protected-resource/mcp":
		return []byte(`{"resource":"https://mcp.example.test/mcp","authorization_servers":["https://auth.example.test"]}`), nil
	case "https://auth.example.test/.well-known/oauth-authorization-server":
		return []byte(`{"issuer":"https://auth.example.test","authorization_endpoint":"https://auth.example.test/authorize","token_endpoint":"https://auth.example.test/token","client_id_metadata_document_supported":true,"token_endpoint_auth_methods_supported":["private_key_jwt"],"token_endpoint_auth_signing_alg_values_supported":["ES256"]}`), nil
	default:
		return nil, fmt.Errorf("unexpected metadata request")
	}
}

func (c *builderProtectedResourceClient) PostJSON(context.Context, string, []byte) ([]byte, error) {
	return nil, fmt.Errorf("unexpected DCR registration")
}

func builderProtectedResourceConfig() *ports.Config {
	return &ports.Config{
		Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server: ports.ServerConfig{
			EndUser: ports.ServerInstanceConfig{
				Port: 8000, Bind: "127.0.0.1", PublicURL: "https://broker.example.test",
				Authentication: ports.AuthenticationConfig{Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"}},
			},
			Admin: ports.ServerInstanceConfig{
				Port: 14000, Bind: "127.0.0.1", PublicURL: "https://broker-admin.example.test",
				Authentication: ports.AuthenticationConfig{Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"}},
			},
			Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
		},
		Storage:          ports.StorageConfig{Backend: "memory", Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second}},
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))},
		Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
		OAuth2AuthServer: ports.OAuth2AuthServerConfig{Mode: "local", Local: ports.LocalModeConfig{TokenTTL: time.Hour}},
	}
}

func TestBuilder_DiscoveryUsesInjectedClientWithoutChangingManualServices(t *testing.T) {
	cfg := builderProtectedResourceConfig()
	adapter, err := storage.NewAdapter(&cfg.Storage)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, adapter.Close(context.Background())) })

	client := &builderProtectedResourceClient{}
	app, err := NewBuilder().WithConfig(cfg).WithStorage(adapter).
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithOAuthDiscoveryClient(client).Build()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })
	_, _, err = app.CIMDKeyService.EnsureInitialKey(context.Background())
	require.NoError(t, err)

	resourceURL := "https://mcp.example.test/mcp"
	discovered := &model.ThirdpartyOAuth2ProviderEntity{
		DisplayName: "Discovered provider",
		Secret:      model.NewAbsentSecret(),
		Discovery:   model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL},
	}
	require.NoError(t, app.ProviderService.Create(context.Background(), discovered))
	assert.Equal(t, model.ClientBootstrapCIMD, discovered.Discovery.ClientMethod)
	assert.Equal(t, resourceURL, discovered.AuthorizationParams["resource"])
	assert.Equal(t, []string{
		resourceURL,
		"https://mcp.example.test/.well-known/oauth-protected-resource/mcp",
		"https://auth.example.test/.well-known/oauth-authorization-server",
	}, client.calls)
	stored, err := adapter.Services().Get(context.Background(), discovered.ID)
	require.NoError(t, err)
	assert.Equal(t, discovered.ClientID, stored.ClientID)
	assert.NotEmpty(t, stored.ClientID)

	manual := &model.ThirdpartyOAuth2ProviderEntity{
		ID: id.NewServiceID(), DisplayName: "Manual provider", ClientID: "manual-client",
		Secret: model.NewPlaintextSecret("manual-secret"), IssuerURI: "https://manual.example.test",
		Discovery: model.DiscoveryConfig{EnableDiscovery: false},
		Endpoints: model.OAuth2Endpoints{AuthorizeEndpoint: "https://manual.example.test/authorize", TokenEndpoint: "https://manual.example.test/token"},
	}
	require.NoError(t, app.ProviderService.Create(context.Background(), manual))
	assert.Len(t, client.calls, 3, "manual creation must not probe the resource provider")
}

func waitBuilderConnectionState(t *testing.T, events <-chan string, remotes ...string) {
	t.Helper()
	pending := make(map[string]struct{}, len(remotes))
	for _, remote := range remotes {
		pending[remote] = struct{}{}
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for len(pending) != 0 {
		select {
		case observed := <-events:
			delete(pending, observed)
		case <-timer.C:
			t.Fatalf("connections %v did not enter expected state", pending)
		}
	}
}

func TestBuilder_ShutdownClosesSharedAndDiscoveryTokenIdleConnections(t *testing.T) {
	sharedRequests := make(chan string, 8)
	sharedIdle := make(chan string, 16)
	sharedClosed := make(chan string, 16)
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		issuer := "http://" + r.Host
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
				"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks",
			})
		case "/jwks":
			sharedRequests <- r.RemoteAddr
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	upstream.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		switch state {
		case http.StateIdle:
			sharedIdle <- conn.RemoteAddr().String()
		case http.StateClosed:
			sharedClosed <- conn.RemoteAddr().String()
		}
	}
	upstream.Start()
	defer upstream.Close()

	guardedRequests := make(chan string, 1)
	guardedIdle := make(chan string, 2)
	guardedClosed := make(chan string, 2)
	provider := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guardedRequests <- r.RemoteAddr
		w.WriteHeader(http.StatusOK)
	}))
	provider.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		switch state {
		case http.StateIdle:
			guardedIdle <- conn.RemoteAddr().String()
		case http.StateClosed:
			guardedClosed <- conn.RemoteAddr().String()
		}
	}
	provider.StartTLS()
	defer provider.Close()
	injectedTokenClient := provider.Client()

	cfg := builderProtectedResourceConfig()
	cfg.Security.SkipThirdpartyHTTPSValidation = true
	cfg.OAuth2AuthServer = ports.OAuth2AuthServerConfig{
		Mode: "proxy",
		Proxy: ports.ProxyModeConfig{
			UpstreamIssuerURI: upstream.URL, UpstreamAuthorizeEndpoint: upstream.URL + "/authorize",
			UpstreamTokenEndpoint: upstream.URL + "/token", UpstreamTimeout: time.Second,
			UpstreamJWKSMinRefresh: time.Hour, UpstreamJWKSMaxRefresh: time.Hour,
		},
	}
	adapter, err := storage.NewAdapter(&cfg.Storage)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, adapter.Close(context.Background())) })
	app, err := NewBuilder().WithConfig(cfg).WithStorage(adapter).
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithDiscoveryTokenHTTPClient(injectedTokenClient).Build()
	require.NoError(t, err)
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			assert.NoError(t, app.Shutdown(context.Background()))
		}
	})

	jwks := httptest.NewRecorder()
	app.EnduserHandlers.JWKS.ServeJWKS(jwks, httptest.NewRequest(http.MethodGet, "/oauth2/jwks.json", nil))
	require.Equal(t, http.StatusOK, jwks.Code)
	var sharedRemote string
	select {
	case sharedRemote = <-sharedRequests:
	case <-time.After(5 * time.Second):
		t.Fatal("shared upstream transport did not fetch JWKS")
	}
	waitBuilderConnectionState(t, sharedIdle, sharedRemote)

	response, err := injectedTokenClient.Get(provider.URL)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	guardedRemote := <-guardedRequests
	waitBuilderConnectionState(t, guardedIdle, guardedRemote)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
	stopped = true
	waitBuilderConnectionState(t, sharedClosed, sharedRemote)
	waitBuilderConnectionState(t, guardedClosed, guardedRemote)
}

func TestBuilder_ShutdownClosesProductionSharedAndGuardedPools(t *testing.T) {
	cfg := builderProtectedResourceConfig()
	adapter, err := storage.NewAdapter(&cfg.Storage)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, adapter.Close(context.Background())) })
	app, err := NewBuilder().WithConfig(cfg).WithStorage(adapter).
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))).Build()
	require.NoError(t, err)
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			assert.NoError(t, app.Shutdown(context.Background()))
		}
	})
	require.NotNil(t, app.upstreamTransport)
	require.NotNil(t, app.guardedTransport)
	assert.Nil(t, app.guardedTransport.Proxy, "guarded transport cannot inherit the upstream proxy")

	type request struct{ host, remote string }
	requests := make(chan request, 2)
	idle := make(chan string, 4)
	closed := make(chan string, 4)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- request{host: r.Host, remote: r.RemoteAddr}
		w.WriteHeader(http.StatusNoContent)
	}))
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		switch state {
		case http.StateIdle:
			idle <- conn.RemoteAddr().String()
		case http.StateClosed:
			closed <- conn.RemoteAddr().String()
		}
	}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())

	// Route only this test's private transports to loopback. Production retains its dial-time guard.
	dialTestServer := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	for _, transport := range []*http.Transport{app.upstreamTransport, app.guardedTransport} {
		transport.Proxy = nil
		transport.DialContext = dialTestServer
		transport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "127.0.0.1",
		}
	}

	for _, pair := range []struct {
		transport *http.Transport
		host      string
	}{
		{app.upstreamTransport, "manual.example.test"},
		{app.guardedTransport, "discovery.example.test"},
	} {
		response, err := (&http.Client{Transport: pair.transport}).Get("https://" + pair.host + "/ping")
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	first, second := <-requests, <-requests
	assert.NotEqual(t, first.remote, second.remote, "the two upstream modes must use separate connection pools")
	waitBuilderConnectionState(t, idle, first.remote, second.remote)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
	stopped = true
	waitBuilderConnectionState(t, closed, first.remote, second.remote)
}

func TestBuilder_GuardedDiscoveryTracingDoesNotExportURLQueries(t *testing.T) {
	const sensitiveValue = "private-query-sentinel"
	previousTracer := otel.GetTracerProvider()
	spans := tracetest.NewSpanRecorder()
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	t.Cleanup(func() { otel.SetTracerProvider(previousTracer) })
	config := builderProtectedResourceConfig()
	config.Telemetry.Enabled = true
	config.Telemetry.Traces.Enabled = true
	config.OAuth2AuthServer.CIMD.SSRF.ExtraBlockedCIDRs = []string{"8.8.8.8/32"}
	adapter, err := storage.NewAdapter(&config.Storage)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, adapter.Close(context.Background())) })
	application, err := NewBuilder().WithConfig(config).WithStorage(adapter).
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))).WithTracerProvider(tracer).Build()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, application.Shutdown(context.Background())) })

	create := func(resource string) {
		provider := &model.ThirdpartyOAuth2ProviderEntity{
			DisplayName: "Blocked resource", Secret: model.NewAbsentSecret(),
			Discovery: model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resource},
		}
		require.Error(t, application.ProviderService.Create(context.Background(), provider))
	}
	create("https://8.8.8.8/mcp")
	baseline := len(spans.Ended())
	require.Positive(t, baseline, "an ordinary blocked discovery request must emit a client span")
	create("https://8.8.8.8/mcp?token=" + sensitiveValue)
	for _, span := range spans.Ended()[baseline:] {
		assert.NotContains(t, fmt.Sprint(span.Attributes()), sensitiveValue, "traces must not export provider-controlled URL queries")
	}
}
