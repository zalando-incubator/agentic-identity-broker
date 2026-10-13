// Package http provides HTTP server adapters for the identity broker.
package http

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	domjwtauth "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtauth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// ServerConfig contains the configuration for a Server instance.
type ServerConfig struct {
	Port             int
	Bind             string
	PublicURL        string
	Name             string
	Telemetry        ports.TelemetryConfig
	RequestContext   *ports.RequestContextConfig
	Authentication   ports.AuthenticationConfig
	JWTAuthenticator domjwtauth.JWTAuthenticator // nil when JWT not configured
	HealthComponents func() map[string]string
}

// Server implements HTTP server lifecycle management using the chi router framework.
// It is generic and not aware of server type ("admin" vs "enduser").
// Route registration is handled via a provided RouteSetupFunc closure.
//
// Responsibilities:
// - Bind to port (Listen)
// - Serve HTTP requests (Serve)
// - Graceful shutdown (Shutdown)
// - Health status reporting (HealthStatus)
// - Common middleware setup (recovery, logging, optional principal)
// - Public health endpoint (/health)
type Server struct {
	config      ServerConfig       // Server configuration (port, bind address, auth)
	routeSetup  func(r chi.Router) // Route registration function (provided by caller)
	httpServer  *http.Server       // Underlying HTTP server
	healthState int32              // Atomic health state
	startTime   time.Time          // Time when server started serving requests
	logger      *slog.Logger       // Structured logger
}

// NewServer creates a new HTTP server instance.
//
// Parameters:
//   - config: Server configuration (port, bind address, authentication)
//   - routeSetup: Function that registers routes on the provided chi.Router
//   - logger: Structured logger for this server instance
//
// The routeSetup function is called during Serve() after middleware setup,
// allowing the caller to register application-specific routes without the
// server knowing about them.
func NewServer(config ServerConfig, routeSetup func(r chi.Router), logger *slog.Logger) *Server {
	return &Server{
		config:      config,
		routeSetup:  routeSetup,
		healthState: int32(HealthStateStarting),
		logger:      logger,
	}
}

func TraceContextNormalizationMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			values := r.Header.Values("traceparent")
			if len(values) > 1 {
				if traceparent, ok := firstValidTraceparent(r.Header.Clone(), values); ok {
					r.Header.Del("traceparent")
					r.Header.Set("traceparent", traceparent)
				} else {
					r.Header.Del("traceparent")
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func firstValidTraceparent(header http.Header, values []string) (string, bool) {
	for _, value := range values {
		candidate := header.Clone()
		candidate.Del("traceparent")
		candidate.Set("traceparent", value)
		ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(candidate))
		if trace.SpanContextFromContext(ctx).IsValid() {
			return value, true
		}
	}

	return "", false
}

// Listen binds to the configured address and port and returns a listener.
// This is a fast operation that only binds the socket, does not start serving.
// Implements IPv6 dual-stack support with automatic IPv4 fallback.
func (s *Server) Listen() (net.Listener, error) {
	// Try IPv6 dual-stack first (:: means all interfaces, both IPv4 and IPv6)
	if s.config.Bind == "::" {
		addr := fmt.Sprintf("[::]:%d", s.config.Port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			// IPv6 not available, fallback to IPv4
			s.logger.Warn("IPv6 bind failed, falling back to IPv4",
				"address", addr,
				"error", err)

			// Fallback to IPv4-only (0.0.0.0)
			addr = fmt.Sprintf("0.0.0.0:%d", s.config.Port)
			listener, err = net.Listen("tcp", addr)
			if err != nil {
				return nil, fmt.Errorf("failed to bind to %s: %w", addr, err)
			}
			s.logger.Info("Server bound to IPv4",
				"address", addr,
				"port", s.config.Port)
			return listener, nil
		}

		s.logger.Info("Server bound to dual-stack (IPv6/IPv4)",
			"address", addr,
			"port", s.config.Port)
		return listener, nil
	}

	// Specific bind address provided
	addr := fmt.Sprintf("%s:%d", s.config.Bind, s.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind to %s: %w", addr, err)
	}

	s.logger.Info("Server bound",
		"address", addr,
		"port", s.config.Port,
		"bind", s.config.Bind)
	return listener, nil
}

// NewHandler assembles the chi router with standard middleware and application routes.
// Does not register /health — the caller adds a context-appropriate health endpoint.
// Used by Server.Serve() and directly by tests.
func NewHandler(config ServerConfig, routeSetup func(chi.Router), logger *slog.Logger) *chi.Mux {
	requestContext := middleware.ResolveRequestContextConfig(config.RequestContext)
	router := chi.NewRouter()
	router.Use(RecoveryMiddleware(logger, requestContext.Trace.ResponseEnabled))
	router.Use(TraceContextNormalizationMiddleware())
	if config.Telemetry.Enabled {
		name := config.Name
		if name == "" {
			name = "http"
		}
		router.Use(tokenEndpointTelemetry(otelchi.Middleware(name,
			otelchi.WithChiRoutes(router),
			otelchi.WithRequestMethodInSpanName(true),
			otelchi.WithPropagators(otel.GetTextMapPropagator()),
		)))
	}
	router.Use(middleware.OptionalPrincipalMiddleware(config.Authentication, config.JWTAuthenticator, logger))
	router.Use(middleware.SecurityContextMiddleware(requestContext, config.Telemetry.Enabled && config.Telemetry.Traces.Enabled))
	router.Use(LoggingMiddleware(logger, "/api/", "/oauth2/", "/.well-known/", "/health"))
	router.Use(ContextRecoveryMiddleware(logger, requestContext.Trace.ResponseEnabled))
	routeSetup(router)
	return router
}

// Serve starts serving HTTP requests on the provided listener.
// This is a blocking call that runs until the server is shut down or encounters an error.
// The context can be used to cancel the server operation.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	// Build router via NewHandler, then add lifecycle-aware health endpoint
	router := NewHandler(s.config, s.routeSetup, s.logger)
	router.Get("/health", s.handleHealth())

	// Create HTTP server
	s.httpServer = &http.Server{
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Set health state to healthy and record start time
	atomic.StoreInt32(&s.healthState, int32(HealthStateHealthy))
	s.startTime = time.Now()

	s.logger.Info("Server started serving requests",
		"address", listener.Addr().String())

	// Start serving (blocking call)
	if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		atomic.StoreInt32(&s.healthState, int32(HealthStateUnhealthy))
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Shutdown initiates graceful shutdown of the server.
// Sets health state to shutting down, then calls http.Server.Shutdown with timeout.
func (s *Server) Shutdown(parentCtx context.Context, timeout time.Duration) error {
	s.logger.Info("Initiating graceful shutdown",
		"timeout", timeout)

	// Set health state to shutting down
	atomic.StoreInt32(&s.healthState, int32(HealthStateShuttingDown))

	// Create timeout context for shutdown
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	// Shutdown with timeout
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("Shutdown error", "error", err)
		return fmt.Errorf("shutdown failed: %w", err)
	}

	s.logger.Info("Server shut down successfully")
	return nil
}

// HealthStatus returns the current health state of the server.
// This is a fast, non-blocking operation using atomic reads.
func (s *Server) HealthStatus() ports.HealthState {
	return ports.HealthState(atomic.LoadInt32(&s.healthState))
}
