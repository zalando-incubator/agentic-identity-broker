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

// NewCIMDEndUserTestServer creates an end-user test server with CIMD fetcher injection and
// URL alignment. URL alignment ensures that the OAuth2 service's consent redirect URL
// (derived from config.Server.EndUser.PublicURL) matches the actual httptest server port.
//
// This uses a two-phase approach:
//  1. Start a minimal httptest.Server to claim a random port.
//  2. Update sf.config.Server.EndUser.PublicURL with that port, then build the app.
//  3. Register end-user routes on the already-started router.
func NewCIMDEndUserTestServer(storage interface{}, sf *ServerFactory, cimdFetcher ports.CIMDFetcher, logger *slog.Logger) (*TestServer, error) {
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

	testServer := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second},
	}
	testServer.Start()

	appInstance.Logger.Info("CIMD test server listening on fixed listener", "url", testServer.URL, "type", "end-user")

	return &TestServer{app: appInstance, server: testServer, logger: appInstance.Logger, client: newTestHTTPClient()}, nil
}
