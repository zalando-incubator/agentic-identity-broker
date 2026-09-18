// Package bootstrap provides test infrastructure for the ExtProc Token Exchange E2E tests.
// It starts an in-process ExtProc gRPC server and mock HTTP servers for controlled testing.
//
// This bootstrap is SEPARATE from the main E2E bootstrap (tests/e2e/bootstrap/) and
// does NOT reuse any of the identity broker infrastructure (per spec FR-017).
//
// Architecture:
//
//	MockOAuth2Server (httptest) → ExtProc gRPC Server (in-process) ← gRPC test client
//	MockTokenExchangeServer (httptest) ↗
//
// Usage:
//
//	env := bootstrap.NewTestEnvironment(cfg, logger)
//	env.Start()
//	defer env.Stop()
//	client, conn := env.NewExtProcClient()
//	defer conn.Close()
package bootstrap

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck
	. "github.com/onsi/gomega"    //nolint:staticcheck
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

// TestEnvironment provides a complete in-process test environment for ExtProc E2E tests.
// Each test gets a fresh environment via BeforeEach/AfterEach for isolation.
type TestEnvironment struct {
	// Config for the ExtProc service under test
	Config *extprocconfig.Config

	// MockOAuth2 is the mock client_credentials OAuth2 server (for obtaining client assertions)
	MockOAuth2 *MockOAuth2Server

	// MockTokenExchange is the mock token exchange endpoint (identity broker)
	MockTokenExchange *MockTokenExchangeServer

	// Internal state
	grpcServer   *grpc.Server
	grpcListener net.Listener
	grpcAddress  string
	logger       *slog.Logger
	stopOnce     sync.Once
}

// NewTestEnvironment creates a new test environment with the provided config.
// The config's OAuth2 endpoints will be overridden to point to the mock servers.
// Call Start() to launch the servers.
func NewTestEnvironment(cfg *extprocconfig.Config, logger *slog.Logger) *TestEnvironment {
	return &TestEnvironment{
		Config: cfg,
		logger: logger,
	}
}

// Start launches all mock servers and the in-process ExtProc gRPC server.
// Must be called before making gRPC requests. Uses Gomega Expect so it can be
// called directly in BeforeEach blocks — test fails immediately on setup errors.
func (e *TestEnvironment) Start() {
	e.startMockServers()

	exchanger, err := extprocserver.NewTokenExchanger(e.Config, e.logger)
	Expect(err).NotTo(HaveOccurred(), "failed to create token exchanger")

	var svc *extprocserver.Server
	if e.Config.Authorization.Enabled {
		authorizer, authErr := NewOPAAuthorizer(e.Config, e.logger)
		Expect(authErr).NotTo(HaveOccurred(), "failed to create OPA authorizer")
		svc = extprocserver.NewServerWithAuthorizer(e.Config, exchanger, authorizer, e.logger)
	} else {
		svc = extprocserver.NewServer(e.Config, exchanger, e.logger)
	}

	e.startGRPCServer(svc)
}

// startMockServers initializes and starts the mock OAuth2 and token exchange servers
// and updates the config to point to them.
func (e *TestEnvironment) startMockServers() {
	e.MockOAuth2 = NewMockOAuth2Server()
	e.MockOAuth2.Start()
	e.MockTokenExchange = NewMockTokenExchangeServer()
	e.MockTokenExchange.Start()
	e.Config.OAuth2.ClientCredentialsEndpoint = e.MockOAuth2.URL() + "/oauth/token"
	e.Config.OAuth2.TokenEndpoint = e.MockTokenExchange.URL() + "/oauth2/token"
	e.Config.OAuth2.TLS.AllowHTTP = true
}

// startGRPCServer creates and starts the gRPC server with the provided ExtProc service.
func (e *TestEnvironment) startGRPCServer(svc *extprocserver.Server) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred(), "failed to create gRPC listener")
	e.grpcListener = listener
	e.grpcAddress = listener.Addr().String()

	e.grpcServer = grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(e.grpcServer, svc)

	go func() {
		if serveErr := e.grpcServer.Serve(e.grpcListener); serveErr != nil && serveErr != grpc.ErrServerStopped {
			e.logger.Error("gRPC server error", "error", serveErr)
		}
	}()
}

// Stop shuts down all servers. Safe to call multiple times.
func (e *TestEnvironment) Stop() {
	e.stopOnce.Do(func() {
		if e.grpcServer != nil {
			e.grpcServer.GracefulStop()
		}
		if e.MockOAuth2 != nil {
			e.MockOAuth2.Stop()
		}
		if e.MockTokenExchange != nil {
			e.MockTokenExchange.Stop()
		}
	})
}

// GRPCAddress returns the address of the in-process gRPC server.
func (e *TestEnvironment) GRPCAddress() string {
	return e.grpcAddress
}

// NewExtProcClient creates a new gRPC client connected to the in-process ExtProc server.
// The caller is responsible for closing the connection via conn.Close().
func (e *TestEnvironment) NewExtProcClient() (extprocv3.ExternalProcessorClient, *grpc.ClientConn) {
	conn, err := grpc.NewClient(
		e.grpcAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	Expect(err).NotTo(HaveOccurred(), "failed to create gRPC client connection")
	return extprocv3.NewExternalProcessorClient(conn), conn
}

// MockOAuth2Server is a controllable mock for the OAuth2 client_credentials endpoint.
// Used to simulate the upstream OAuth2 server that issues client assertions (ID tokens).
type MockOAuth2Server struct {
	server      *httptest.Server
	mu          sync.RWMutex
	callCount   int
	idToken     string
	accessToken string
	statusCode  int
	errorCode   string
	lastForm    string
}

// NewMockOAuth2Server creates a new mock OAuth2 server with default successful responses.
func NewMockOAuth2Server() *MockOAuth2Server {
	return &MockOAuth2Server{ // #nosec G101 -- deterministic test-only token values.
		idToken:     "mock-id-token-for-client-assertion",
		accessToken: "mock-access-token",
		statusCode:  http.StatusOK,
	}
}

// Start launches the mock HTTP server.
func (m *MockOAuth2Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", m.handleToken)
	m.server = httptest.NewServer(mux)
}

// Stop shuts down the mock server.
func (m *MockOAuth2Server) Stop() {
	if m.server != nil {
		m.server.Close()
	}
}

// URL returns the base URL of the mock server.
func (m *MockOAuth2Server) URL() string {
	if m.server == nil {
		return ""
	}
	return m.server.URL
}

// CallCount returns how many times the token endpoint was called (thread-safe).
func (m *MockOAuth2Server) CallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount
}

// WithIDToken configures the ID token returned in client_credentials responses.
func (m *MockOAuth2Server) WithIDToken(idToken string) *MockOAuth2Server {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.idToken = idToken
	return m
}

// WithError configures the server to return an error response.
func (m *MockOAuth2Server) WithError(statusCode int, errorCode string) *MockOAuth2Server {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusCode = statusCode
	m.errorCode = errorCode
	return m
}

// LastForm returns the last form-encoded request body received (thread-safe).
// Use this in tests to assert on parameters sent to the client_credentials endpoint.
func (m *MockOAuth2Server) LastForm() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastForm
}

// handleToken handles POST /oauth/token requests.
func (m *MockOAuth2Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.callCount++
	statusCode := m.statusCode
	idToken := m.idToken
	accessToken := m.accessToken
	errorCode := m.errorCode
	m.lastForm = r.Form.Encode()
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
		fmt.Fprintf(w, `{"error":%q}`, errorCode) //nolint:errcheck
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"access_token":%q,"id_token":%q,"token_type":"Bearer","expires_in":3600}`, //nolint:errcheck
		accessToken, idToken)
}

// MockTokenExchangeServer is a controllable mock for the identity broker token exchange endpoint.
// Used to simulate the RFC 8693 token exchange endpoint (POST /oauth2/token).
type MockTokenExchangeServer struct {
	server             *httptest.Server
	mu                 sync.RWMutex
	callCount          int
	exchangedToken     string
	expiresIn          *int
	statusCode         int
	errorCode          string
	lastBody           string
	lastRequestHeaders http.Header
}

// NewMockTokenExchangeServer creates a mock token exchange server with default responses.
func NewMockTokenExchangeServer() *MockTokenExchangeServer {
	defaultExpiry := 3600
	return &MockTokenExchangeServer{
		exchangedToken: "exchanged-access-token-default",
		expiresIn:      &defaultExpiry,
		statusCode:     http.StatusOK,
	}
}

// Start launches the mock HTTP server.
func (m *MockTokenExchangeServer) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", m.handleExchange)
	m.server = httptest.NewServer(mux)
}

// Stop shuts down the mock server.
func (m *MockTokenExchangeServer) Stop() {
	if m.server != nil {
		m.server.Close()
	}
}

// URL returns the base URL of the mock server.
func (m *MockTokenExchangeServer) URL() string {
	if m.server == nil {
		return ""
	}
	return m.server.URL
}

// CallCount returns how many times the exchange endpoint was called (thread-safe).
func (m *MockTokenExchangeServer) CallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount
}

// LastBody returns the last form-encoded request body received (thread-safe).
func (m *MockTokenExchangeServer) LastBody() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastBody
}

// LastRequestHeaders returns a copy of the HTTP headers from the last received request (thread-safe).
// Use this to assert on trace context headers (e.g., traceparent) injected by otelhttp.NewTransport.
func (m *MockTokenExchangeServer) LastRequestHeaders() http.Header {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lastRequestHeaders == nil {
		return nil
	}
	headers := make(http.Header, len(m.lastRequestHeaders))
	for k, v := range m.lastRequestHeaders {
		headers[k] = append([]string{}, v...)
	}
	return headers
}

// WithExchangedToken configures the access_token in successful exchange responses.
func (m *MockTokenExchangeServer) WithExchangedToken(token string) *MockTokenExchangeServer {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exchangedToken = token
	return m
}

// WithExpiresIn configures the expires_in field in successful responses.
// Pass nil to omit expires_in (tests the default_ttl path per FR-012).
func (m *MockTokenExchangeServer) WithExpiresIn(seconds *int) *MockTokenExchangeServer {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expiresIn = seconds
	return m
}

// WithError configures the server to return an error response.
func (m *MockTokenExchangeServer) WithError(statusCode int, errorCode string) *MockTokenExchangeServer {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusCode = statusCode
	m.errorCode = errorCode
	return m
}

// Reset clears call history and resets to default responses for reuse across test scenarios.
func (m *MockTokenExchangeServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount = 0
	m.lastBody = ""
}

// handleExchange handles POST /oauth2/token token exchange requests.
func (m *MockTokenExchangeServer) handleExchange(w http.ResponseWriter, r *http.Request) {
	// Capture headers before ParseForm since form parsing does not affect headers.
	capturedHeaders := r.Header.Clone()

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.callCount++
	statusCode := m.statusCode
	exchangedToken := m.exchangedToken
	expiresIn := m.expiresIn
	errorCode := m.errorCode
	// Capture raw form-encoded body and request headers for assertion in tests
	m.lastBody = r.Form.Encode()
	m.lastRequestHeaders = capturedHeaders
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
		fmt.Fprintf(w, `{"error":%q,"error_description":"mock exchange error"}`, errorCode) //nolint:errcheck
		return
	}

	w.WriteHeader(http.StatusOK)
	if expiresIn != nil && *expiresIn > 0 {
		// Include expires_in in response (normal path)
		fmt.Fprintf(w, `{"access_token":%q,"issued_token_type":"urn:ietf:params:oauth:token-type:access_token","token_type":"Bearer","expires_in":%d}`, //nolint:errcheck
			exchangedToken, *expiresIn)
	} else {
		// Omit expires_in — tests the default_ttl path (FR-012)
		fmt.Fprintf(w, `{"access_token":%q,"issued_token_type":"urn:ietf:params:oauth:token-type:access_token","token_type":"Bearer"}`, //nolint:errcheck
			exchangedToken)
	}
}

// NewTestLogger returns a logger that writes to GinkgoWriter for test visibility.
// Output appears in Ginkgo test output when tests fail or when running with -v.
func NewTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}
