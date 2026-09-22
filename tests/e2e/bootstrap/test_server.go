// Package bootstrap provides test infrastructure for E2E testing.
package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"time"
	"unsafe"

	httpAdapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/routing"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
)

// TestServer wraps production app with HTTP test interface.
// This is a THIN WRAPPER that provides test-friendly methods while using
// PRODUCTION app and routes exactly as deployed.
//
// Key design principles:
// - Uses PRODUCTION app from app.Builder.Build()
// - Uses httptest.Server for HTTP testing (standard Go approach)
// - No test-specific routing logic
// - Provides authenticated HTTP methods (GET/POST with Principal injection)
//
// Architecture:
// - Wraps httptest.Server with production route setup
// - Provides convenience methods for authenticated requests
// - Handles principal injection via X-Remote-User header
type TestServer struct {
	app                     *app.App
	server                  *httptest.Server
	logger                  *slog.Logger
	client                  *http.Client
	requestSecurityObserver *SecurityContextObserver
}

// SecurityContextObservation captures a test-only request-security-context observation.
type SecurityContextObservation struct {
	Layer         string
	TraceID       string
	Actor         string
	CallingPeer   string
	ClientIP      string
	UserAgent     string
	RequestMethod string
	RequestTarget string
}

// SecurityContextObserver is a bootstrap seam for observing downstream propagation.
type SecurityContextObserver struct {
	mu           sync.RWMutex
	observations []SecurityContextObservation
}

// NewSecurityContextObserver creates an empty observer.
func NewSecurityContextObserver() *SecurityContextObserver {
	return &SecurityContextObserver{}
}

// Record appends a propagation observation.
func (o *SecurityContextObserver) Record(observation SecurityContextObservation) {
	if o == nil {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	o.observations = append(o.observations, observation)
}

// Snapshot returns a copy of the recorded observations.
func (o *SecurityContextObserver) Snapshot() []SecurityContextObservation {
	if o == nil {
		return nil
	}

	o.mu.RLock()
	defer o.mu.RUnlock()

	snapshot := make([]SecurityContextObservation, len(o.observations))
	copy(snapshot, o.observations)
	return snapshot
}

// Last returns the most recent observation, if any.
func (o *SecurityContextObserver) Last() (SecurityContextObservation, bool) {
	if o == nil {
		return SecurityContextObservation{}, false
	}

	o.mu.RLock()
	defer o.mu.RUnlock()
	if len(o.observations) == 0 {
		return SecurityContextObservation{}, false
	}

	return o.observations[len(o.observations)-1], true
}

type observingThirdpartyOAuth2ProviderRepository struct {
	layer    string
	observer *SecurityContextObserver
	next     ports.ThirdpartyOAuth2ProviderRepository
}

func (r *observingThirdpartyOAuth2ProviderRepository) Create(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error {
	recordObservedSecurityContext(r.observer, r.layer, ctx)
	return r.next.Create(ctx, entity)
}

func (r *observingThirdpartyOAuth2ProviderRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	recordObservedSecurityContext(r.observer, r.layer, ctx)
	return r.next.Get(ctx, serviceID)
}

func (r *observingThirdpartyOAuth2ProviderRepository) Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error {
	recordObservedSecurityContext(r.observer, r.layer, ctx)
	return r.next.Update(ctx, entity, expectedVersion)
}

func (r *observingThirdpartyOAuth2ProviderRepository) Delete(ctx context.Context, serviceID id.ServiceID) error {
	recordObservedSecurityContext(r.observer, r.layer, ctx)
	return r.next.Delete(ctx, serviceID)
}

func (r *observingThirdpartyOAuth2ProviderRepository) List(ctx context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	recordObservedSecurityContext(r.observer, r.layer, ctx)
	return r.next.List(ctx)
}

func (r *observingThirdpartyOAuth2ProviderRepository) FindByProtectedResource(ctx context.Context, resourceURI string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	recordObservedSecurityContext(r.observer, r.layer, ctx)
	return r.next.FindByProtectedResource(ctx, resourceURI)
}

func (r *observingThirdpartyOAuth2ProviderRepository) AddProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	recordObservedSecurityContext(r.observer, r.layer, ctx)
	return r.next.AddProtectedResource(ctx, serviceID, resourceURI)
}

func (r *observingThirdpartyOAuth2ProviderRepository) RemoveProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	recordObservedSecurityContext(r.observer, r.layer, ctx)
	return r.next.RemoveProtectedResource(ctx, serviceID, resourceURI)
}

func (r *observingThirdpartyOAuth2ProviderRepository) RenameProtectedResource(ctx context.Context, serviceID id.ServiceID, fromURI, toURI string) (ports.ProtectedResourceMutationResult, error) {
	recordObservedSecurityContext(r.observer, r.layer, ctx)
	return r.next.RenameProtectedResource(ctx, serviceID, fromURI, toURI)
}

func (r *observingThirdpartyOAuth2ProviderRepository) ListProtectedResources(ctx context.Context, serviceID id.ServiceID) ([]string, int64, error) {
	recordObservedSecurityContext(r.observer, r.layer, ctx)
	return r.next.ListProtectedResources(ctx, serviceID)
}

func recordObservedSecurityContext(observer *SecurityContextObserver, layer string, ctx context.Context) {
	if observer == nil {
		return
	}

	sc, ok := security.FromContext(ctx)
	if !ok {
		return
	}

	observer.Record(SecurityContextObservation{
		Layer:         layer,
		TraceID:       sc.TraceID,
		Actor:         sc.Actor,
		CallingPeer:   sc.CallingPeer,
		ClientIP:      sc.ClientIP,
		UserAgent:     sc.UserAgent,
		RequestMethod: sc.RequestMethod,
		RequestTarget: sc.RequestTarget,
	})
}

// attachRequestSecurityObserver wraps the built ProviderService's repository with
// observing decorators so grey-box E2E tests can assert the SecurityContext that
// reaches the service and repository layers (invisible at the HTTP boundary).
//
// It reaches into the unexported repo field via reflection because
// ThirdpartyOAuth2ProviderService exposes no supported repo-injection seam, and
// adding one would be a test-only production API forbidden by AGENTS.md. The
// alternative — wrapping at storage-adapter construction — is likewise blocked:
// Adapter.providers is unexported with no injection constructor. Until a
// production change is independently justified, reflection is the only test-side
// seam. It therefore fails loudly (rather than silently no-op'ing) when the field
// can no longer be found, so a future refactor surfaces a precise error here
// instead of a mysterious empty-observations failure in a downstream assertion.
func attachRequestSecurityObserver(app *app.App, observer *SecurityContextObserver) error {
	if observer == nil {
		return nil
	}
	if app == nil || app.ProviderService == nil {
		return fmt.Errorf("request security observer requested but app.ProviderService is nil")
	}

	repo, ok := currentProviderRepo(app.ProviderService)
	if !ok {
		return fmt.Errorf("request security observer seam is stale: could not read the unexported \"repo\" field on *thirdparty.ThirdpartyOAuth2ProviderService via reflection — a production refactor likely renamed or removed it; update currentProviderRepo/setProviderRepo in tests/e2e/bootstrap/test_server.go")
	}

	repositoryObserved := &observingThirdpartyOAuth2ProviderRepository{
		layer:    "repository",
		observer: observer,
		next:     repo,
	}
	serviceObserved := &observingThirdpartyOAuth2ProviderRepository{
		layer:    "service",
		observer: observer,
		next:     repositoryObserved,
	}
	if !setProviderRepo(app.ProviderService, serviceObserved) {
		return fmt.Errorf("request security observer seam is stale: could not set the unexported \"repo\" field on *thirdparty.ThirdpartyOAuth2ProviderService via reflection — update setProviderRepo in tests/e2e/bootstrap/test_server.go")
	}
	return nil
}

// currentProviderRepo reads the unexported repo field via reflection. See
// attachRequestSecurityObserver for why this reflection-based seam is used.
func currentProviderRepo(providerService any) (ports.ThirdpartyOAuth2ProviderRepository, bool) {
	value := reflect.ValueOf(providerService)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return nil, false
	}

	field := value.Elem().FieldByName("repo")
	if !field.IsValid() {
		return nil, false
	}

	repo, ok := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(ports.ThirdpartyOAuth2ProviderRepository) // #nosec G103 -- test-only reflection seam reads the verified unexported repository field.
	return repo, ok
}

// setProviderRepo writes the unexported repo field via reflection. See
// attachRequestSecurityObserver for why this reflection-based seam is used.
func setProviderRepo(providerService any, repo ports.ThirdpartyOAuth2ProviderRepository) bool {
	value := reflect.ValueOf(providerService)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return false
	}

	field := value.Elem().FieldByName("repo")
	if !field.IsValid() {
		return false
	}

	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(repo)) // #nosec G103 -- test-only reflection seam writes the verified unexported repository field.
	return true
}

// TestServerConfig holds parameters for building a TestServer that needs URL alignment.
// This is used when creating a test server where the app's PublicURL must match
// the actual test server's listening address.
type TestServerConfig struct {
	Config *ports.Config
	Logger *slog.Logger
}

// ServerType indicates what type of routes the test server should serve.
type ServerType int

const (
	// ServerTypeEndUser serves end-user routes (OAuth2, consent UI, etc.).
	ServerTypeEndUser ServerType = iota
	// ServerTypeAdmin serves admin routes (agent/service management).
	ServerTypeAdmin

	// DevEndUserPort is the fixed port used for end-user servers in dev mode.
	// This matches the Vite proxy configuration for local development.
	DevEndUserPort = 8000
)

// TestServerOption configures a TestServer.
type TestServerOption func(*testServerOptions)

// testServerOptions holds configuration for building a TestServer.
type testServerOptions struct {
	serverType              ServerType
	port                    int  // 0 for random port, non-zero for fixed port
	fixedPort               bool // true to use fixed port
	requestSecurityObserver *SecurityContextObserver
}

// validate checks that the options are internally consistent.
func (o *testServerOptions) validate() error {
	if o.fixedPort && o.port == 0 {
		return fmt.Errorf("fixedPort is true but port is 0: specify a port number")
	}
	if !o.fixedPort && o.port != 0 {
		return fmt.Errorf("fixedPort is false but port is %d: use WithFixedPort() to set a fixed port", o.port)
	}
	return nil
}

// WithServerType sets the server type (EndUser or Admin).
func WithServerType(st ServerType) TestServerOption {
	return func(o *testServerOptions) {
		o.serverType = st
	}
}

// WithRandomPort configures the server to use a random port (default).
func WithRandomPort() TestServerOption {
	return func(o *testServerOptions) {
		o.port = 0
		o.fixedPort = false
	}
}

// WithFixedPort configures the server to use a specific port.
// Use this for development mode where the Vite proxy expects port 8000.
func WithFixedPort(port int) TestServerOption {
	return func(o *testServerOptions) {
		o.port = port
		o.fixedPort = true
	}
}

// WithDevMode configures the server for development mode.
// For end-user servers, this uses fixed port 8000 (for Vite proxy).
// For admin servers, this uses a random port.
func WithDevMode() TestServerOption {
	return func(o *testServerOptions) {
		// Only use fixed port for end-user server in dev mode
		if o.serverType == ServerTypeEndUser {
			o.port = DevEndUserPort
			o.fixedPort = true
		} else {
			o.port = 0
			o.fixedPort = false
		}
	}
}

// WithRequestSecurityObserver attaches a test-only observer seam for request-context assertions.
func WithRequestSecurityObserver(observer *SecurityContextObserver) TestServerOption {
	return func(o *testServerOptions) {
		o.requestSecurityObserver = observer
	}
}

// NewTestServerV2 creates a test server with the specified options.
// This replaces NewTestServer() and provides separate end-user and admin servers.
//
// Example usage:
//
//	// End-user server with random port
//	server, err := NewTestServerV2(app, logger, WithServerType(ServerTypeEndUser))
//
//	// Admin server with random port
//	server, err := NewTestServerV2(app, logger, WithServerType(ServerTypeAdmin))
//
//	// End-user server for dev mode (fixed port 8000)
//	if os.Getenv("E2E_FRONTEND_MODE") == "dev" {
//		server, err := NewTestServerV2(app, logger, WithServerType(ServerTypeEndUser), WithDevMode())
//	}
//
// Parameters:
//   - app: Fully-wired application from app.Builder.Build()
//   - logger: Structured logger
//   - opts: Configuration options
//
// Returns:
//   - *TestServer: Ready to make authenticated requests
//   - error: If server setup fails
func NewTestServerV2(app *app.App, logger *slog.Logger, opts ...TestServerOption) (*TestServer, error) {
	if app == nil {
		return nil, fmt.Errorf("app is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Apply options
	options := &testServerOptions{
		serverType: ServerTypeEndUser, // Default to end-user
		port:       0,                 // Default to random port
		fixedPort:  false,
	}
	for _, opt := range opts {
		opt(options)
	}

	// Validate options
	if err := options.validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}
	if err := attachRequestSecurityObserver(app, options.requestSecurityObserver); err != nil {
		return nil, err
	}

	// Determine route setup and server config based on server type.
	// Use production NewHandler to align bootstrap with production server path.
	var (
		routeSetup       func(chi.Router)
		healthComponents func() map[string]string
	)
	serverCfg := httpAdapter.ServerConfig{
		Telemetry:      app.Config.Telemetry,
		RequestContext: &app.Config.RequestContext,
	}

	switch options.serverType {
	case ServerTypeEndUser:
		healthComponents = app.EnduserHealthComponents
		routeSetup = func(r chi.Router) {
			routing.SetupEnduserRoutes(r, app.EnduserHandlers, routing.EnduserRouteConfig{
				Authentication:               app.Config.Server.EndUser.Authentication,
				JWTAuthenticator:             app.JWTAuthenticator,
				ApprovalRequestAuthenticator: app.ApprovalRequestAuthenticator,
				Logger:                       app.Logger,
				CORS:                         app.Config.Server.EndUser.CORS,
				Telemetry:                    app.Config.Telemetry,
			})
		}
		serverCfg.Name = "enduser"
		serverCfg.Authentication = app.Config.Server.EndUser.Authentication
		serverCfg.JWTAuthenticator = app.JWTAuthenticator

	case ServerTypeAdmin:
		if app.AdminHandlers == nil {
			return nil, fmt.Errorf("admin handlers not available: ensure app was built with admin handlers enabled")
		}
		routeSetup = func(r chi.Router) {
			routing.SetupAdminRoutes(r, app.AdminHandlers, routing.AdminRouteConfig{
				CORS: app.Config.Server.Admin.CORS,
			})
		}
		serverCfg.Name = "admin"
		serverCfg.Authentication = app.Config.Server.Admin.Authentication

	default:
		return nil, fmt.Errorf("unknown server type: %d", options.serverType)
	}

	// Build router using the production NewHandler (same middleware stack as production).
	router := httpAdapter.NewHandler(serverCfg, routeSetup, app.Logger)

	router.Get("/health", httpAdapter.NewHealthHandler(
		func() ports.HealthState { return ports.HealthStateHealthy },
		time.Now(),
		healthComponents,
		app.Logger,
	))

	// Create httptest server with appropriate port configuration
	var server *httptest.Server
	if options.fixedPort {
		// Fixed port mode (for dev mode with Vite proxy)
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", options.port))
		if err != nil {
			return nil, fmt.Errorf("failed to listen on port %d: %w", options.port, err)
		}
		server = &httptest.Server{
			Listener: listener,
			Config:   &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second},
		}
		server.Start()
		logger.Info("Test server listening on fixed port",
			"port", options.port,
			"url", server.URL,
			"type", serverTypeName(options.serverType))
	} else {
		// Random port mode (default for test isolation)
		server = httptest.NewServer(router)
		logger.Info("Test server listening on random port",
			"url", server.URL,
			"type", serverTypeName(options.serverType))
	}

	return &TestServer{
		app:                     app,
		server:                  server,
		logger:                  logger,
		requestSecurityObserver: options.requestSecurityObserver,
		client: &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// serverTypeName returns a human-readable name for the server type.
func serverTypeName(st ServerType) string {
	switch st {
	case ServerTypeEndUser:
		return "end-user"
	case ServerTypeAdmin:
		return "admin"
	default:
		return "unknown"
	}
}

// NewEndUserTestServer creates an end-user test server.
// This is a convenience function that wraps NewTestServerV2 with ServerTypeEndUser.
//
// The server will use:
//   - Fixed port 8000 if E2E_FRONTEND_MODE=dev (for Vite proxy)
//   - Random port otherwise (for test isolation)
//
// Example:
//
//	server, err := NewEndUserTestServer(app, logger)
//	require.NoError(t, err)
//	defer server.Close()
func NewEndUserTestServer(app *app.App, logger *slog.Logger) (*TestServer, error) {
	opts := []TestServerOption{WithServerType(ServerTypeEndUser)}

	// In dev mode, use fixed port for Vite proxy
	if os.Getenv("E2E_FRONTEND_MODE") == "dev" {
		opts = append(opts, WithDevMode())
	}

	return NewTestServerV2(app, logger, opts...)
}

// NewAdminTestServer creates an admin test server.
// This is a convenience function that wraps NewTestServerV2 with ServerTypeAdmin.
//
// The server always uses a random port for test isolation.
//
// Example:
//
//	server, err := NewAdminTestServer(app, logger)
//	require.NoError(t, err)
//	defer server.Close()
func NewAdminTestServer(app *app.App, logger *slog.Logger) (*TestServer, error) {
	return NewTestServerV2(app, logger, WithServerType(ServerTypeAdmin))
}

// Close gracefully shuts down the server.
// Safe to call multiple times (httptest.Server.Close is idempotent).
func (ts *TestServer) Close() {
	if ts.server != nil {
		ts.server.Close()
	}
}

// App returns the underlying App instance, giving tests access to domain services
// for building test fixtures (e.g. JWE tokens via OAuth2Service).
func (ts *TestServer) App() *app.App {
	return ts.app
}

// RequestSecurityObserver returns the test-only propagation observer attached at bootstrap time.
func (ts *TestServer) RequestSecurityObserver() *SecurityContextObserver {
	return ts.requestSecurityObserver
}

// BaseURL returns the server's base URL for requests.
// Returns: "http://127.0.0.1:PORT" format string
func (ts *TestServer) BaseURL() string {
	if ts.server == nil {
		return ""
	}
	return ts.server.URL
}

// AuthenticatedGET makes an authenticated GET request with Principal injection.
// This is a convenience method that adds X-Remote-User header from Principal.
//
// Parameters:
//   - path: Request path (e.g., "/api/agents")
//   - principal: Principal value to inject (e.g., "user@example.com")
//
// Returns:
//   - *http.Response: Response from server (caller must close Body)
//   - error: If request fails
//
// Header injection:
// - Adds X-Remote-User: {principal} header
// - This simulates authentication from reverse proxy
// - Follows production middleware setup exactly
//
// Example:
//
//	resp, err := server.AuthenticatedGET("/api/agents", "user@example.com")
//	require.NoError(t, err)
//	defer resp.Body.Close()
//	assert.Equal(t, http.StatusOK, resp.StatusCode)
func (ts *TestServer) AuthenticatedGET(path string, principal string) (*http.Response, error) {
	if principal == "" {
		return nil, fmt.Errorf("principal is required")
	}

	req, err := http.NewRequest("GET", ts.BaseURL()+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Inject principal via header (production middleware configuration)
	req.Header.Set("X-Remote-User", principal)

	// Make request using HTTP client that does NOT follow redirects
	// E2E tests need to verify redirect responses themselves
	resp, err := ts.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// AuthenticatedPOST makes an authenticated POST request with Principal injection.
// This is a convenience method that adds X-Remote-User header from Principal.
//
// Parameters:
//   - path: Request path (e.g., "/api/agents")
//   - principal: Principal value to inject (e.g., "user@example.com")
//   - contentType: Content-Type header (e.g., "application/json")
//   - body: Request body reader (e.g., strings.NewReader(jsonData))
//
// Returns:
//   - *http.Response: Response from server (caller must close Body)
//   - error: If request fails
//
// Header injection:
// - Adds X-Remote-User: {principal} header
// - Sets Content-Type header as specified
// - Follows production middleware setup exactly
//
// Example:
//
//	jsonBody := strings.NewReader(`{"id":"agent-1"}`)
//	resp, err := server.AuthenticatedPOST("/api/agents", "user@example.com", "application/json", jsonBody)
//	require.NoError(t, err)
//	defer resp.Body.Close()
//	assert.Equal(t, http.StatusCreated, resp.StatusCode)
func (ts *TestServer) AuthenticatedPOST(path string, principal string, contentType string, body io.Reader) (*http.Response, error) {
	if principal == "" {
		return nil, fmt.Errorf("principal is required")
	}

	req, err := http.NewRequest("POST", ts.BaseURL()+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Inject principal via header (production middleware configuration)
	req.Header.Set("X-Remote-User", principal)

	// Set content type if provided
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Make request using HTTP client that does NOT follow redirects
	// E2E tests need to verify redirect responses themselves
	resp, err := ts.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// PublicGET makes an unauthenticated GET request (no Principal).
// Use this for testing public endpoints (health, metadata, etc.).
//
// Parameters:
//   - path: Request path (e.g., "/health")
//
// Returns:
//   - *http.Response: Response from server (caller must close Body)
//   - error: If request fails
//
// Example:
//
//	resp, err := server.PublicGET("/health")
//	require.NoError(t, err)
//	defer resp.Body.Close()
//	assert.Equal(t, http.StatusOK, resp.StatusCode)
func (ts *TestServer) PublicGET(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", ts.BaseURL()+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// No authentication header (public endpoint)

	// Make request using HTTP client that does NOT follow redirects
	// E2E tests need to verify redirect responses themselves
	resp, err := ts.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// PublicPOST makes an unauthenticated POST request (no Principal).
// Use this for public endpoints like /oauth2/token that authenticate via request body (JWTs).
// Do NOT use X-Remote-User header as authentication comes from subject_token and client_assertion.
//
// Parameters:
//   - path: Request path (e.g., "/oauth2/token")
//   - contentType: Content-Type header (e.g., "application/x-www-form-urlencoded")
//   - body: Request body reader (e.g., strings.NewReader(urlEncodedData))
//
// Returns:
//   - *http.Response: Response from server (caller must close Body)
//   - error: If request fails
//
// Example:
//
//	data := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:token-exchange"}, ...}
//	resp, err := server.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
//	require.NoError(t, err)
//	defer resp.Body.Close()
//	assert.Equal(t, http.StatusOK, resp.StatusCode)
func (ts *TestServer) PublicPOST(path string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", ts.BaseURL()+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set content type if provided
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// No authentication header (public endpoint, authenticates via request body JWTs)

	// Make request using HTTP client that does NOT follow redirects
	// E2E tests need to verify redirect responses themselves
	resp, err := ts.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// DirectRequest makes a raw HTTP request (for advanced testing).
// Use AuthenticatedGET/POST for most tests.
//
// Parameters:
//   - method: HTTP method (GET, POST, etc.)
//   - path: Request path
//   - principal: Principal to inject (empty for unauthenticated)
//   - headers: Additional headers to set
//   - body: Request body (can be nil)
//
// Returns:
//   - *http.Response: Response from server (caller must close Body)
//   - error: If request fails
func (ts *TestServer) DirectRequest(method string, path string, principal string, headers map[string]string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, ts.BaseURL()+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add principal if provided
	if principal != "" {
		req.Header.Set("X-Remote-User", principal)
	}

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Make request using HTTP client that does NOT follow redirects
	// E2E tests need to verify redirect responses themselves
	resp, err := ts.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// NewTestServerBuilder creates a test server with proper URL alignment.
// This builder handles the critical flow where the test server's actual listening URL
// must be known BEFORE building the app, because services are initialized with the
// PublicURL during app.Builder.Build().
//
// Flow:
// 1. Accept config and factory
// 2. Create a placeholder httptest server to determine its listening address
// 3. Update config.Server.EndUser.PublicURL with the actual test server URL
// 4. Build the app with the updated config
// 5. Register routes and start the actual server
// 6. Return fully configured TestServer
//
// Parameters:
//   - config: Application configuration (will be modified with actual server URL)
//   - storage: Storage adapter to pass to app builder
//   - factory: ServerFactory to build the app with updated config
//   - logger: Structured logger
//
// Returns:
//   - *TestServer: TestServer with app configured to use its actual listening URL
//   - error: If setup fails
//
// Example:
//
//	config := fixtures.OAuth2ConfigWithPublicURL("https://broker.example.com")
//	builder := NewTestServerBuilder(config, storage, factory, logger)
//	server, err := builder.Build()
//	require.NoError(t, err)
//	defer server.Close()
//	// server.BaseURL() will match what OAuth2 services use internally
func NewTestServerBuilder(
	config *ports.Config,
	storage interface{},
	factory *ServerFactory,
	logger *slog.Logger,
) (*TestServerBuilderImpl, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if storage == nil {
		return nil, fmt.Errorf("storage is required")
	}
	if factory == nil {
		return nil, fmt.Errorf("factory is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &TestServerBuilderImpl{
		config:  config,
		storage: storage,
		factory: factory,
		logger:  logger,
	}, nil
}

// TestServerBuilderImpl implements the builder pattern for TestServer with URL alignment.
type TestServerBuilderImpl struct {
	config  *ports.Config
	storage interface{}
	factory *ServerFactory
	logger  *slog.Logger
}

// Build constructs and returns a fully configured TestServer with URL alignment.
// This uses the elegant approach: create mux first, start server to get port,
// then build app with correct URL and register routes on the existing mux.
//
// Returns:
//   - *TestServer: Fully configured and ready to use
//   - error: If any step fails
func (b *TestServerBuilderImpl) Build() (*TestServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen for test server: %w", err)
	}

	actualURL := "http://" + listener.Addr().String()
	b.logger.Info(
		"Updating PublicURL for test server alignment",
		"configured_url", b.config.Server.EndUser.PublicURL,
		"actual_url", actualURL,
	)
	b.config.Server.EndUser.PublicURL = actualURL

	appInstance, err := b.factory.BuildApp(b.storage)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("failed to build app: %w", err)
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

	serverCfg := httpAdapter.ServerConfig{
		Name:             "enduser",
		Telemetry:        appInstance.Config.Telemetry,
		RequestContext:   &appInstance.Config.RequestContext,
		Authentication:   appInstance.Config.Server.EndUser.Authentication,
		JWTAuthenticator: appInstance.JWTAuthenticator,
	}
	router := httpAdapter.NewHandler(serverCfg, routeSetup, appInstance.Logger)
	router.Get("/health", httpAdapter.NewHealthHandler(
		func() ports.HealthState { return ports.HealthStateHealthy },
		time.Now(),
		nil,
		appInstance.Logger,
	))

	testServer := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second},
	}
	testServer.Start()

	appInstance.Logger.Info("Test server created and configured", "url", testServer.URL)

	return &TestServer{
		app:                     appInstance,
		server:                  testServer,
		logger:                  appInstance.Logger,
		requestSecurityObserver: nil,
		client: &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}
