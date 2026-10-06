package bootstrap

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	adaptercmd "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/cimd"
	httpAdapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/routing"
	brokerapp "github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	domaincimd "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/cimd"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// CIMDTestHTTPClient returns an *http.Client that routes requests for fakeHostname
// to server. Uses InsecureSkipVerify since the test server cert covers 127.0.0.1,
// not the fake hostname. Safe for test use only.
//
// Use NewCIMDTestFetcher for the common case. Use this when you need to configure
// the client before building a fetcher (e.g., set Timeout for timeout tests).
func CIMDTestHTTPClient(server *httptest.Server, fakeHostname string) *http.Client {
	parsed, _ := url.Parse(server.URL)
	serverAddr := parsed.Host
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 -- fake-host test routing uses an httptest certificate issued only for 127.0.0.1.
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, _ := net.SplitHostPort(addr)
				if host == fakeHostname {
					addr = serverAddr
				}
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
		},
	}
}

// NewCIMDTestFetcher returns a ports.CIMDFetcher wired to route requests for
// fakeHostname to server.
func NewCIMDTestFetcher(server *httptest.Server, fakeHostname string, maxResponseBytes int64) (ports.CIMDFetcher, error) {
	return NewCIMDTestFetcherFromClient(CIMDTestHTTPClient(server, fakeHostname), maxResponseBytes)
}

// NewCIMDTestFetcherFromClient builds a CIMDFetcher from a pre-configured *http.Client.
// Use with CIMDTestHTTPClient when custom client configuration is needed (e.g., Timeout).
func NewCIMDTestFetcherFromClient(client *http.Client, maxResponseBytes int64) (ports.CIMDFetcher, error) {
	bl, err := domaincimd.NewSSRFBlocklist(nil)
	if err != nil {
		return nil, err
	}
	return adaptercmd.NewFetcherWithClient(client, bl, maxResponseBytes), nil
}

// NewCIMDEndUserTestServer creates a production-built end-user server for CIMD tests.
// It preserves a configured HTTPS public URL and starts a TLS test server for outbound
// CIMD-client scenarios. Other CIMD tests keep the existing HTTP URL-alignment behavior.
func NewCIMDEndUserTestServer(storage interface{}, sf *ServerFactory, cimdFetcher ports.CIMDFetcher, logger *slog.Logger) (*TestServer, error) {
	if sf == nil || sf.config == nil {
		return nil, fmt.Errorf("CIMD server factory config is required")
	}
	if _, ok := cimdHTTPSOrigin(sf.config.Server.EndUser.PublicURL); ok {
		appInstance, err := sf.BuildAppWithCIMDFetcher(storage, cimdFetcher)
		if err != nil {
			return nil, fmt.Errorf("failed to build CIMD HTTPS app: %w", err)
		}
		server := httptest.NewUnstartedServer(newCIMDEndUserHandler(appInstance))
		server.Config.ReadHeaderTimeout = 5 * time.Second
		server.StartTLS()
		appInstance.Logger.Info("CIMD HTTPS test server listening", "url", server.URL, "public_url", sf.config.Server.EndUser.PublicURL, "type", "end-user")
		return &TestServer{app: appInstance, server: server, logger: appInstance.Logger, client: cimdTestServerClient(server)}, nil
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen for CIMD test server: %w", err)
	}

	sf.config.Server.EndUser.PublicURL = "http://" + listener.Addr().String()
	appInstance, err := sf.BuildAppWithCIMDFetcher(storage, cimdFetcher)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("failed to build CIMD app: %w", err)
	}

	testServer := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: newCIMDEndUserHandler(appInstance), ReadHeaderTimeout: 5 * time.Second},
	}
	testServer.Start()

	appInstance.Logger.Info("CIMD test server listening on fixed listener", "url", testServer.URL, "type", "end-user")
	return &TestServer{app: appInstance, server: testServer, logger: appInstance.Logger, client: cimdTestServerClient(testServer)}, nil
}

// NewCIMDClientEndUserTestServer serves a production-built broker behind a
// stable HTTPS public URL and returns the upstream client's mapped transport.
func NewCIMDClientEndUserTestServer(storage interface{}, sf *ServerFactory, publicURL string, logger *slog.Logger) (*TestServer, *http.Client, error) {
	if sf == nil || sf.config == nil {
		return nil, nil, fmt.Errorf("CIMD client server factory config is required")
	}
	if _, ok := cimdHTTPSOrigin(publicURL); !ok {
		return nil, nil, fmt.Errorf("CIMD client public URL must be an HTTPS origin")
	}

	sf.config.Server.EndUser.PublicURL = publicURL
	appInstance, err := sf.BuildApp(storage)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build CIMD client app: %w", err)
	}

	server := httptest.NewUnstartedServer(newCIMDEndUserHandler(appInstance))
	server.Config.ReadHeaderTimeout = 5 * time.Second
	server.StartTLS()

	testServer := &TestServer{app: appInstance, server: server, logger: appInstance.Logger, client: cimdTestServerClient(server)}
	brokerClient, err := CIMDUpstreamHTTPClient(testServer, publicURL)
	if err != nil {
		testServer.Close()
		return nil, nil, err
	}
	appInstance.Logger.Info("CIMD client test server listening", "url", server.URL, "public_url", publicURL, "type", "end-user")
	return testServer, brokerClient, nil
}

// CIMDUpstreamHTTPClient routes a stable HTTPS public URL to an E2E test server.
func CIMDUpstreamHTTPClient(server *TestServer, publicURL string) (*http.Client, error) {
	if server == nil || server.server == nil {
		return nil, fmt.Errorf("CIMD test server is required")
	}
	parsedPublicURL, ok := cimdHTTPSOrigin(publicURL)
	if !ok {
		return nil, fmt.Errorf("CIMD client public URL must be an HTTPS origin")
	}
	return CIMDTestHTTPClient(server.server, parsedPublicURL.Hostname()), nil
}

func cimdHTTPSOrigin(publicURL string) (*url.URL, bool) {
	parsedPublicURL, err := url.Parse(publicURL)
	if err != nil || parsedPublicURL.Scheme != "https" || parsedPublicURL.Host == "" || parsedPublicURL.User != nil || parsedPublicURL.RawQuery != "" || parsedPublicURL.Fragment != "" {
		return nil, false
	}
	return parsedPublicURL, true
}

func newCIMDEndUserHandler(appInstance *brokerapp.App) http.Handler {
	serverCfg := httpAdapter.ServerConfig{
		Name:             "enduser",
		Telemetry:        appInstance.Config.Telemetry,
		RequestContext:   &appInstance.Config.RequestContext,
		Authentication:   appInstance.Config.Server.EndUser.Authentication,
		JWTAuthenticator: appInstance.JWTAuthenticator,
	}
	routeSetup := func(r chi.Router) {
		routing.SetupEnduserRoutes(r, appInstance.EnduserHandlers, routing.EnduserRouteConfig{
			Authentication:               appInstance.Config.Server.EndUser.Authentication,
			JWTAuthenticator:             appInstance.JWTAuthenticator,
			ApprovalRequestAuthenticator: appInstance.ApprovalRequestAuthenticator,
			Logger:                       appInstance.Logger,
			CORS:                         appInstance.Config.Server.EndUser.CORS,
			Telemetry:                    appInstance.Config.Telemetry,
		})
	}
	router := httpAdapter.NewHandler(serverCfg, routeSetup, appInstance.Logger)
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	})
	return router
}

func cimdTestServerClient(server *httptest.Server) *http.Client {
	client := server.Client()
	client.Timeout = 5 * time.Second
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client
}
