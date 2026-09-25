// Package app provides application-layer wiring and orchestration.
package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/contrib/propagators/ot"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	adaptercmd "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/cimd"
	awsencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/aws"
	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/enduser"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers/admin"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers/consent"
	enduserHandlers "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers/enduser"
	httpmiddleware "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/oauth2_sessions"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/jwks"
	jwtauthadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/jwtauth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	postgresstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/postgres"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	agentsservice "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/agents"
	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	consentservice "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/impersonation"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	domjwtauth "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtauth"
	oauth2service "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	domaincimd "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/cimd"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	domstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// App represents a fully-wired application with all services and handlers initialized.
// This is returned by Builder.Build() after dependency injection.
type App struct {
	// Configuration
	Config *ports.Config

	// Repositories
	Storage          *storage.Adapter
	BranchKeyManager ports.BranchKeyManager

	// Domain services
	ConsentService         *consentservice.Service
	ProviderService        *thirdparty.ThirdpartyOAuth2ProviderService
	PermissionSetService   *permissionset.Service
	OAuth2SessionService   *oauth2session.OAuth2SessionService
	OAuth2Service          ports.OAuth2Service
	TokenExchangeService   *tokenexchange.TokenExchangeService
	ApprovalService        *domainapproval.Service
	ApprovalSyncSubscriber *postgresstorage.ApprovalSyncSubscriber // nil when storage is not postgres
	SessionTokenService    *sessiontoken.Service

	// JWT pre-authentication (optional, nil when not configured)
	JWTAuthenticator             domjwtauth.JWTAuthenticator
	ApprovalRequestAuthenticator *httpmiddleware.ApprovalRequestAuthenticator

	// Handler groups for routing
	AdminHandlers   *AdminHandlers
	EnduserHandlers *EnduserHandlers

	// Logger
	Logger *slog.Logger

	jwksPublisherHealth ports.JWKSPublisherHealthPort

	// Shutdown must be called on graceful shutdown to release background resources
	// (e.g. permission-set eviction and OAuth2 expired-record cleanup).
	Shutdown func(context.Context) error
}

// EnduserHealthComponents returns optional component-level health for the public /health endpoint.
func (a *App) EnduserHealthComponents() map[string]string {
	if a == nil || a.jwksPublisherHealth == nil {
		return nil
	}
	return map[string]string{"upstream_jwks": string(a.jwksPublisherHealth.HealthState())}
}

// Builder is a chainable builder for constructing App instances.
// Following the builder pattern for flexible configuration and clear intent.
//
// Example usage:
//
//	app, err := NewBuilder().
//		WithConfig(cfg).
//		WithStorage(storageAdapter).
//		WithLogger(logger).
//		Build()
type Builder struct {
	config                 *ports.Config
	storage                *storage.Adapter
	logger                 *slog.Logger
	staticWebResourcesPath string
	tracerProvider         *sdktrace.TracerProvider // Optional: custom TracerProvider for testing
	cimdFetcher            ports.CIMDFetcher        // Optional: overrides auto-created CIMD fetcher for testing
	jwksPublisher          ports.JWKSPublisherPort  // Optional: overrides JWKS publisher for testing
}

// NewBuilder creates a new application builder.
func NewBuilder() *Builder {
	return &Builder{
		staticWebResourcesPath: "web/dist",
	}
}

// WithConfig sets the application configuration for the builder.
func (b *Builder) WithConfig(cfg *ports.Config) *Builder {
	b.config = cfg
	return b
}

// WithStorage sets the storage adapter for the builder.
func (b *Builder) WithStorage(storage *storage.Adapter) *Builder {
	b.storage = storage
	return b
}

// WithLogger sets the logger for the builder.
func (b *Builder) WithLogger(logger *slog.Logger) *Builder {
	b.logger = logger
	return b
}

func (b *Builder) WithStaticWebResourcesPath(path string) *Builder {
	b.staticWebResourcesPath = path
	return b
}

// WithTracerProvider sets a custom TracerProvider for testing.
// When set, this provider is registered as the global provider instead of
// the one created by NewProvider().
func (b *Builder) WithTracerProvider(tp *sdktrace.TracerProvider) *Builder {
	b.tracerProvider = tp
	return b
}

// WithCIMDFetcher injects a custom CIMDFetcher, bypassing the production fetcher
// created from CIMDConfig. Intended for testing — allows injecting an HTTP client
// that trusts test TLS certificates (e.g., from httptest.NewTLSServer).
// Only effective when cimd.enabled is true.
func (b *Builder) WithCIMDFetcher(f ports.CIMDFetcher) *Builder {
	b.cimdFetcher = f
	return b
}

// WithJWKSPublisher injects a pre-built JWKS publisher for the public /oauth2/jwks.json
// endpoint. Intended for testing only. When no other upstream-verification consumer
// (token exchange or multi-agent verification) is active, this bypasses the builder's
// upstream metadata discovery for JWKS publishing.
func (b *Builder) WithJWKSPublisher(p ports.JWKSPublisherPort) *Builder {
	b.jwksPublisher = p
	return b
}

// oauthResolved holds values extracted from a resolved OAuth2ModeConfig for use
// throughout the builder. Populated once via extractOAuthValues, consumed many times.
type oauthResolved struct {
	upstreamIssuerURI         string
	upstreamAuthorizeEndpoint string
	upstreamTokenEndpoint     string
	upstreamTimeout           time.Duration
	jwksMinRefresh            time.Duration
	jwksMaxRefresh            time.Duration
	localIssuerURI            string
	localTokenTTL             time.Duration
	localRefreshTokenTTL      time.Duration
	localClaimsExpression     string
	responseTypes             []string
	grantTypes                []string
	supportedScopes           []string
	multiAgentClient          ports.MultiAgentClientConfig
	cimdConfig                ports.CIMDConfig
	cimdEnabled               bool
}

func extractOAuthValues(cfg ports.OAuth2ModeConfig, publicURL string) oauthResolved {
	// Keep the 30s upstream default even for local mode: the builder always constructs
	// an outbound HTTP client for upstream OAuth2 and JWKS-related calls elsewhere.
	r := oauthResolved{upstreamTimeout: 30 * time.Second, localIssuerURI: publicURL}
	switch c := cfg.(type) {
	case *ports.ProxyOAuth2Config:
		r.upstreamIssuerURI = c.UpstreamIssuerURI
		r.upstreamAuthorizeEndpoint = c.UpstreamAuthorizeEndpoint
		r.upstreamTokenEndpoint = c.UpstreamTokenEndpoint
		r.upstreamTimeout = c.UpstreamTimeout
		r.jwksMinRefresh = c.JWKSMinRefresh()
		r.jwksMaxRefresh = c.JWKSMaxRefresh()
		r.responseTypes = c.SupportedResponseTypes
		r.grantTypes = c.SupportedGrantTypes
		r.supportedScopes = c.SupportedScopes
		r.multiAgentClient = c.MultiAgentClient
	case *ports.LocalOAuth2Config:
		if c.IssuerURI != "" {
			r.localIssuerURI = c.IssuerURI
		}
		r.localTokenTTL = c.TokenTTL
		r.localRefreshTokenTTL = c.RefreshTokenTTL
		r.localClaimsExpression = c.TokenClaimsExpression
		r.responseTypes = c.SupportedResponseTypes
		r.grantTypes = c.SupportedGrantTypes
		r.supportedScopes = c.SupportedScopes
		r.cimdConfig = c.CIMD
		r.cimdEnabled = c.CIMD.Enabled
	case *ports.HybridOAuth2Config:
		r.upstreamIssuerURI = c.Proxy.UpstreamIssuerURI
		r.upstreamAuthorizeEndpoint = c.Proxy.UpstreamAuthorizeEndpoint
		r.upstreamTokenEndpoint = c.Proxy.UpstreamTokenEndpoint
		r.upstreamTimeout = c.Proxy.UpstreamTimeout
		r.jwksMinRefresh = c.Proxy.JWKSMinRefresh()
		r.jwksMaxRefresh = c.Proxy.JWKSMaxRefresh()
		r.responseTypes = c.ResponseTypes()
		r.grantTypes = c.GrantTypes()
		r.supportedScopes = c.Local.SupportedScopes
		r.multiAgentClient = c.Proxy.MultiAgentClient
		if c.Local.IssuerURI != "" {
			r.localIssuerURI = c.Local.IssuerURI
		}
		r.localTokenTTL = c.Local.TokenTTL
		r.localRefreshTokenTTL = c.Local.RefreshTokenTTL
		r.localClaimsExpression = c.Local.TokenClaimsExpression
		r.cimdConfig = c.Local.CIMD
		r.cimdEnabled = c.Local.CIMD.Enabled
	default:
		panic(fmt.Sprintf("BUG: unhandled OAuth2ModeConfig type %T — update extractOAuthValues", cfg))
	}
	return r
}

func modeStrategyFor(mode servermode.Mode) oauth2service.ModeStrategy {
	switch mode {
	case servermode.Proxy:
		return oauth2service.NewProxyModeStrategy()
	case servermode.Local:
		return oauth2service.NewLocalModeStrategy()
	case servermode.Hybrid:
		return oauth2service.NewHybridModeStrategy()
	default:
		panic(fmt.Sprintf("BUG: unhandled servermode.Mode %q — update modeStrategyFor", mode))
	}
}

var newSigningKeyStartupContext = func(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// Build constructs the App with all wired dependencies.
// Returns error if required dependencies are missing or initialization fails.
//
// Dependencies are created in this order:
// 1. Validate inputs (Config, Storage, Logger required)
// 2. Create domain services (ConsentService, OAuth2Service, OAuth2SessionService)
// 3. Create handler instances (AdminHandlers, EnduserHandlers)
// 4. Return fully-wired App
func (b *Builder) Build() (*App, error) {
	// Validate required dependencies
	if b.config == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if b.storage == nil {
		return nil, fmt.Errorf("storage adapter is required")
	}
	if b.logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	oauthCfg, err := b.config.OAuth2AuthServer.Resolve()
	if err != nil {
		return nil, fmt.Errorf("oauth2_authorization_server configuration invalid: %w", err)
	}
	ov := extractOAuthValues(oauthCfg, b.config.Server.EndUser.PublicURL)

	app := &App{
		Config:  b.config,
		Storage: b.storage,
		Logger:  b.logger,
	}

	// T029: Initialize telemetry provider
	// Per ADR-011: OTel provider wired at app layer, no port interface needed.
	// When b.tracerProvider is set (test override), register it globally and wrap
	// it in a shutdown function. Otherwise, initialize the full OTel provider from config.
	if b.tracerProvider != nil {
		// Test override: register the provided TracerProvider globally
		otel.SetTracerProvider(b.tracerProvider)
		// Set default propagators for test environment — must match the production
		// default set (tracecontext, ottrace, b3multi, baggage) to ensure span connectivity.
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			ot.OT{},
			b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader)),
			propagation.Baggage{},
		))
		app.Shutdown = func(ctx context.Context) error {
			return b.tracerProvider.Shutdown(ctx)
		}
	} else {
		// Production path: initialize full provider from config.
		// Use a bounded context for initialization to avoid blocking indefinitely if the
		// OTLP endpoint is unreachable at startup. The exporter timeout is a reasonable bound;
		// per-export retries are handled by the OTel SDK independently of this context.
		initCtx, initCancel := context.WithTimeout(context.Background(), b.config.Telemetry.Exporter.Timeout)
		defer initCancel()
		shutdownTelemetry, telErr := telemetry.NewProvider(initCtx, b.config.Telemetry, b.logger)
		if telErr != nil {
			return nil, fmt.Errorf("failed to initialize telemetry: %w", telErr)
		}
		app.Shutdown = shutdownTelemetry
	}

	baseHandler := b.logger.Handler()
	if b.config.Telemetry.Enabled && b.config.Telemetry.Logs.Enabled {
		otelHandler := otelslog.NewHandler(b.config.Telemetry.ServiceName,
			otelslog.WithLoggerProvider(global.GetLoggerProvider()))
		baseHandler = telemetry.NewMultiHandler(baseHandler, otelHandler)
	}
	b.logger = slog.New(telemetry.NewContextHandler(baseHandler))
	app.Logger = b.logger

	// Phase 1: Initialize encryption adapter (must happen before domain services).
	// Encryption is mandatory — no fallback. Config must specify memory or aws_kms backend.
	// Constitution Principle VII: Configuration-Driven Design.
	if b.config.Encryption.AWSKMS == nil && b.config.Encryption.Memory == nil {
		return nil, fmt.Errorf("encryption configuration required: set encryption.memory.raw_key or encryption.aws_kms in configuration (no fallback)")
	}

	encryptor, branchKeyManager, err := awsencryption.NewEncryptionAdapter(&b.config.Encryption)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize encryption adapter: %w", err)
	}

	if b.config.Encryption.AWSKMS != nil {
		b.logger.Info("AWS KMS encryption adapter initialized",
			"dynamodb_table", b.config.Encryption.AWSKMS.DynamoDBTableName,
			"dynamodb_region", b.config.Encryption.AWSKMS.DynamoDBRegion,
			"branch_key_ttl", b.config.Encryption.AWSKMS.BranchKeyTTL,
			"dynamodb_timeout", b.config.Encryption.AWSKMS.DynamoDBTimeout,
			"branch_key_manager_wired", branchKeyManager != nil)
	} else {
		b.logger.Info("Memory encryption adapter initialized")
	}

	if branchKeyManager != nil {
		app.BranchKeyManager = branchKeyManager
	} else {
		app.BranchKeyManager = &encryptionnoop.BranchKeyManager{}
	}

	// Decode and import JWE signing key — required for both OAuth2SessionService and OAuth2Service
	// (CIMD consent flows). Fail fast here before constructing any domain services.
	keyBytes, err := base64.StdEncoding.DecodeString(b.config.ThirdPartyOAuth2.JWESigningKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWE signing key: %w", err)
	}
	jweKey, err := jwk.Import[jwk.Key](keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to import JWE signing key: %w", err)
	}
	jweTokenService := domjwe.New(jweKey)
	sessionTokenSvc := sessiontoken.NewService(jweTokenService)
	app.SessionTokenService = sessionTokenSvc

	// Phase 2: Create domain services
	// Constitution Principle VI: domain depends on ports (repository interfaces), not adapters

	// Create PermissionSetService early so it can be wired into ProviderService, ConsentService, and AgentsHandler.
	if b.storage.PermissionSets() != nil {
		app.PermissionSetService = permissionset.NewPermissionSetService(
			b.storage.PermissionSets(),
			b.storage.UserGrants(),
			b.logger,
		)
		// Stop the background eviction goroutine on graceful shutdown.
		prevShutdown := app.Shutdown
		ps := app.PermissionSetService
		app.Shutdown = func(ctx context.Context) error {
			ps.Close()
			if prevShutdown != nil {
				return prevShutdown(ctx)
			}
			return nil
		}
	}

	// Create ThirdpartyOAuth2ProviderService (handles encryption, decryption, and branch key provisioning).
	// This consolidated domain service replaces the previous ServiceManager + AuthProvider split.
	// encryptor is guaranteed to be initialized from Phase 1.
	if b.storage.Services() != nil {
		app.ProviderService = thirdparty.NewThirdpartyOAuth2ProviderService(
			b.storage.Services(),
			encryptor,
			app.BranchKeyManager,
			b.storage.PermissionSets(),
			b.config.Security.SkipThirdpartyHTTPSValidation,
			b.logger,
		)
	}

	// Create consent service if repositories available.
	// ConsentService depends on ProviderService (not raw repository) so all service access
	// goes through the domain service layer including encryption/decryption.
	if b.storage.Agents() != nil && app.ProviderService != nil && b.storage.UserGrants() != nil {
		if b.storage.UserSessions() == nil {
			return nil, fmt.Errorf("UserSessionRepository must be available: FR-020 enforcement requires session data")
		}
		if app.PermissionSetService == nil {
			return nil, fmt.Errorf("PermissionSetService must be available for ConsentService")
		}
		app.ConsentService = consentservice.NewService(
			b.storage.Agents(),
			app.ProviderService,
			b.storage.UserGrants(),
			b.storage.UserSessions(),
			app.PermissionSetService,
			b.logger,
		)
	}

	// The external client-assertion trust anchor defaults to the proxy upstream in
	// proxy and hybrid modes. Local mode has no upstream and requires it explicitly.
	clientAssertionIssuerURI := b.config.TokenExchange.ClientAssertion.IssuerURI
	if clientAssertionIssuerURI == "" {
		clientAssertionIssuerURI = ov.upstreamIssuerURI
	}
	switch oauthCfg.(type) {
	case *ports.LocalOAuth2Config, *ports.HybridOAuth2Config:
		if clientAssertionIssuerURI != "" && normalizedIssuerURI(clientAssertionIssuerURI) == normalizedIssuerURI(ov.localIssuerURI) {
			return nil, fmt.Errorf("token_exchange.client_assertion.issuer_uri must be an external identity provider, not the broker's own issuer (%q); broker-minted tokens must never be accepted as privileged-client assertions", ov.localIssuerURI)
		}
	}

	approvalAuthRequested := b.config.TokenExchange.ClaimExtraction.PrincipalExpression != "" ||
		b.config.TokenExchange.ClaimExtraction.AgentIDExpression != "" ||
		b.config.TokenExchange.Authorization.CEL.Expression != ""
	if approvalAuthRequested && clientAssertionIssuerURI == "" {
		return nil, fmt.Errorf("token exchange / approval authentication is configured (token_exchange.claim_extraction or authorization) but no client-assertion trust anchor is set: set token_exchange.client_assertion.issuer_uri (required in local mode; defaults to oauth2_authorization_server.proxy.upstream_issuer_uri in proxy/hybrid mode)")
	}
	tokenExchangeEnabled := clientAssertionIssuerURI != "" &&
		b.config.TokenExchange.ClaimExtraction.PrincipalExpression != "" &&
		b.config.TokenExchange.Authorization.CEL.Expression != ""
	approvalAuthEnabled := clientAssertionIssuerURI != "" && approvalAuthRequested

	// Create OAuth2 service — mode-specific config drives all decisions.
	var clientResolver ports.ClientResolver
	{
		oauth2Config := &oauth2service.OAuth2Config{
			UpstreamAuthorizeEndpoint: ov.upstreamAuthorizeEndpoint,
			UpstreamTokenEndpoint:     ov.upstreamTokenEndpoint,
			PublicURL:                 b.config.Server.EndUser.PublicURL,
			IssuerURI:                 ov.localIssuerURI,
			SupportedResponseTypes:    ov.responseTypes,
			SupportedGrantTypes:       ov.grantTypes,
			SupportedScopes:           ov.supportedScopes,
			MultiAgentClient:          ov.multiAgentClient,
			CIMDEnabled:               ov.cimdEnabled,
			ModeStrategy:              modeStrategyFor(oauthCfg.ServerMode()),
			TokenExchangeEnabled:      tokenExchangeEnabled,
		}

		if ov.cimdEnabled {
			activeFetcher := b.cimdFetcher
			if activeFetcher == nil {
				var concreteFetcher *adaptercmd.Fetcher
				var fetchErr error
				if b.config.Security.SkipCIMDSSRFValidation {
					b.logger.Warn("CIMD SSRF validation disabled — dev/test only, never use in production")
					concreteFetcher, fetchErr = adaptercmd.NewFetcherInsecure(
						ov.cimdConfig.FetchTimeout,
						int64(ov.cimdConfig.MaxResponseBytes),
					)
				} else {
					concreteFetcher, fetchErr = adaptercmd.NewFetcher(
						ov.cimdConfig.FetchTimeout,
						int64(ov.cimdConfig.MaxResponseBytes),
						ov.cimdConfig.SSRF.ExtraBlockedCIDRs,
					)
				}
				if fetchErr != nil {
					return nil, fmt.Errorf("failed to create CIMD fetcher: %w", fetchErr)
				}
				if b.config.Telemetry.Enabled && b.config.Telemetry.Traces.Enabled {
					concreteFetcher.WrapTransport(func(base http.RoundTripper) http.RoundTripper {
						return otelhttp.NewTransport(base)
					})
				}
				activeFetcher = concreteFetcher
			}
			cimdCache, cacheErr := domaincimd.NewCIMDCache(ov.cimdConfig.Cache.MinTTL, ov.cimdConfig.Cache.MaxTTL, ov.cimdConfig.Cache.MaxEntries)
			if cacheErr != nil {
				return nil, fmt.Errorf("failed to create CIMD cache: %w", cacheErr)
			}
			cimdSvc := domaincimd.NewService(activeFetcher, cimdCache, ov.cimdConfig.ClientNameBlocklist, b.logger)
			clientResolver = oauth2service.NewAgentClientResolverWithCIMD(b.storage.Agents(), cimdSvc, b.logger)
			b.logger.Info("CIMD client resolution enabled",
				"fetch_timeout", ov.cimdConfig.FetchTimeout,
				"max_response_bytes", ov.cimdConfig.MaxResponseBytes,
			)
		} else {
			clientResolver = oauth2service.NewAgentClientResolver(b.storage.Agents(), b.logger)
		}

		authService := oauth2service.NewAuthorizationService(
			b.storage.UserGrants(),
			b.storage.UserSessions(),
			clientResolver,
			oauth2Config,
			b.logger,
			sessionTokenSvc,
		)
		app.OAuth2Service = authService
	}

	// OAuth2SessionService is always created because JWESigningKey is mandatory.
	// Unlike ConsentService and OAuth2Service (which are conditionally created based on
	// storage availability and config), OAuth2SessionService requires the JWESigningKey
	// which is marked as REQUIRED in config validation (internal/config/validator.go).
	// The application will fail to start if JWESigningKey is not provided, so we can
	// safely create OAuth2SessionService unconditionally here.
	//
	// Note: OAuth2SessionService requires a providerService for decrypting client secrets.
	// If services repository is not available, providerService will be nil and OAuth2SessionService
	// will fail to fetch services. This is acceptable since the application is non-functional
	// without the services repository anyway.

	// Build service configuration from application config
	// Constitution Principle VII: Configuration-Driven Design
	cfg := oauth2session.NewConfigFromPorts(b.config.ThirdPartyOAuth2, b.config.Server.EndUser.PublicURL)

	upstreamClient := &http.Client{
		Timeout: ov.upstreamTimeout,
	}

	// Wrap the HTTP transport with OTel instrumentation when tracing is enabled.
	// This is the "last resort" layer: even operations without an explicit custom span will
	// still emit a client span and propagate W3C traceparent/tracestate headers to every
	// outgoing HTTP call (JWKS fetches, upstream token proxy, OAuth2 session token exchange).
	if b.config.Telemetry.Enabled && b.config.Telemetry.Traces.Enabled {
		base := upstreamClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		upstreamClient.Transport = otelhttp.NewTransport(base)
	}

	app.OAuth2SessionService = oauth2session.NewOAuth2SessionService(
		app.ProviderService,
		b.storage.UserSessions(),
		b.storage.UserGrants(),
		b.storage.Agents(),
		encryptor,
		upstreamClient,
		jweTokenService,
		cfg,
		b.logger,
	)

	// Create agent domain service (used by admin handlers and CEL resolver)
	agentService := agentsservice.NewService(
		b.storage.Agents(),
		app.ProviderService,
		b.logger,
		ov.multiAgentClient.Enabled,
	)

	// Shared upstream JWKS adapter is required when token exchange, approval authentication,
	// multi-agent verification, or proxy/hybrid JWKS publishing needs upstream keys.
	var sharedUpstreamJWKS *jwks.Adapter
	if ov.upstreamIssuerURI != "" && (b.jwksPublisher == nil || tokenExchangeEnabled || approvalAuthEnabled || ov.multiAgentClient.Enabled) {
		discoveryCtx, discoveryCancel := context.WithTimeout(context.Background(), ov.upstreamTimeout)
		discovered, err := domstorage.DiscoverOAuth2Endpoints(
			discoveryCtx,
			ov.upstreamIssuerURI,
			nil,
			b.config.Security.SkipThirdpartyHTTPSValidation,
		)
		discoveryCancel()
		if err != nil {
			return nil, fmt.Errorf("failed to discover OAuth2 server metadata: %w", err)
		}
		if discovered.JWKsURI == "" {
			return nil, fmt.Errorf("OAuth2 server metadata did not include a jwks_uri")
		}

		sharedUpstreamJWKS, err = jwks.NewJWKSAdapter(
			discovered.JWKsURI,
			upstreamClient,
			ov.jwksMinRefresh,
			ov.jwksMaxRefresh,
			b.logger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create upstream JWKS adapter: %w", err)
		}

		// Chain shutdown once — callers that receive the shared adapter must not re-chain.
		prevShutdown := app.Shutdown
		shared := sharedUpstreamJWKS
		app.Shutdown = func(ctx context.Context) error {
			var prevErr error
			if prevShutdown != nil {
				prevErr = prevShutdown(ctx)
			}
			return errors.Join(shared.Shutdown(ctx), prevErr)
		}
	}

	var clientAssertionJWKS *jwks.Adapter
	clientAssertionConfig := b.config.TokenExchange.ClientAssertion
	clientAssertionConfigUnset := clientAssertionConfig.IssuerURI == "" &&
		clientAssertionConfig.JWKSURI == "" &&
		clientAssertionConfig.JWKSMinRefresh == 0 &&
		clientAssertionConfig.JWKSMaxRefresh == 0
	switch {
	case clientAssertionIssuerURI == "":
		// Token exchange and approval authentication are disabled.
	case clientAssertionConfigUnset:
		clientAssertionJWKS = sharedUpstreamJWKS
	default:
		clientAssertionJWKSURI := clientAssertionConfig.JWKSURI
		if clientAssertionJWKSURI == "" {
			discoveryCtx, discoveryCancel := context.WithTimeout(context.Background(), ov.upstreamTimeout)
			discovered, err := domstorage.DiscoverOAuth2Endpoints(
				discoveryCtx,
				clientAssertionIssuerURI,
				nil,
				b.config.Security.SkipThirdpartyHTTPSValidation,
			)
			discoveryCancel()
			if err != nil {
				return nil, fmt.Errorf("failed to discover token_exchange client-assertion issuer metadata: %w", err)
			}
			if discovered.JWKsURI == "" {
				return nil, fmt.Errorf("token_exchange client-assertion issuer metadata did not include a jwks_uri")
			}
			clientAssertionJWKSURI = discovered.JWKsURI
		}
		clientAssertionJWKSMinRefresh := clientAssertionConfig.JWKSMinRefresh
		if clientAssertionJWKSMinRefresh == 0 {
			clientAssertionJWKSMinRefresh = 15 * time.Minute
		}
		clientAssertionJWKSMaxRefresh := clientAssertionConfig.JWKSMaxRefresh
		if clientAssertionJWKSMaxRefresh == 0 || clientAssertionJWKSMaxRefresh < clientAssertionJWKSMinRefresh {
			if clientAssertionJWKSMinRefresh > time.Hour {
				clientAssertionJWKSMaxRefresh = clientAssertionJWKSMinRefresh
			} else {
				clientAssertionJWKSMaxRefresh = time.Hour
			}
		}
		adapter, err := jwks.NewJWKSAdapter(
			clientAssertionJWKSURI,
			upstreamClient,
			clientAssertionJWKSMinRefresh,
			clientAssertionJWKSMaxRefresh,
			b.logger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create client-assertion JWKS adapter: %w", err)
		}
		prevShutdown := app.Shutdown
		app.Shutdown = func(ctx context.Context) error {
			var prevErr error
			if prevShutdown != nil {
				prevErr = prevShutdown(ctx)
			}
			return errors.Join(adapter.Shutdown(ctx), prevErr)
		}
		clientAssertionJWKS = adapter
	}

	// Create JWT pre-authentication adapter if configured
	// Per Constitution Principle VII: Configuration-Driven Design — only create when JWT block present
	if b.config.Server.EndUser.Authentication.JWT != nil {
		jwtCfg := b.config.Server.EndUser.Authentication.JWT

		// Validate JWT config mutual exclusivity (defense-in-depth, also checked by config validator)
		if jwtCfg.Verification == "none" && jwtCfg.JWKSURI != "" {
			return nil, fmt.Errorf("authentication.jwt: verification 'none' and jwks_uri are mutually exclusive")
		}

		// Create CEL evaluator for JWT claim extraction (domain layer)
		celConfig := domjwtauth.CELEvaluatorConfig{
			PrincipalExpression:   jwtCfg.ClaimExtraction.PrincipalExpression,
			DisplayNameExpression: jwtCfg.ClaimExtraction.DisplayNameExpression,
			EmailExpression:       jwtCfg.ClaimExtraction.EmailExpression,
			PictureURLExpression:  jwtCfg.ClaimExtraction.PictureURLExpression,
		}
		celEval, err := domjwtauth.NewCELEvaluator(celConfig, b.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create CEL evaluator for JWT pre-auth: %w", err)
		}

		// Create JWT authenticator adapter (uses lestrrat-go/jwx v4)
		jwtAuthenticator, err := jwtauthadapter.NewJWXAuthenticator(jwtauthadapter.JWXAuthenticatorConfig{
			JWTConfig:    jwtCfg,
			CELEvaluator: celEval,
			HTTPClient:   &http.Client{Timeout: 10 * time.Second},
			Logger:       b.logger,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create JWT authenticator: %w", err)
		}

		app.JWTAuthenticator = jwtAuthenticator
		prevShutdown := app.Shutdown
		app.Shutdown = func(ctx context.Context) error {
			var prevErr error
			if prevShutdown != nil {
				prevErr = prevShutdown(ctx)
			}
			return errors.Join(jwtAuthenticator.Shutdown(ctx), prevErr)
		}
		b.logger.Info("JWT pre-authentication enabled",
			"header_name", jwtCfg.HeaderName,
			"verification", jwtCfg.Verification,
			"has_audience", jwtCfg.ExpectedAudience != "",
			"has_issuer", jwtCfg.ExpectedIssuer != "",
		)
	}

	// Phase 3: Create handler instances

	// Approval service
	approvalRateLimiter := domainapproval.NewApprovalRateLimiter(
		b.config.Approvals.RateLimit.MaxPendingPerPair,
		b.config.Approvals.RateLimit.MaxRequestsPerMinute,
	)
	approvalBroadcaster := domainapproval.NewApprovalSyncBroadcaster(b.config.Approvals.SyncCoalesceWindow)
	app.ApprovalService = domainapproval.NewService(
		b.storage.ToolApprovals(),
		b.storage.ToolApprovalQueries(),
		b.storage.ToolApprovalMetrics(),
		b.storage.ApprovalSyncState(),
		b.storage.Agents(),
		approvalRateLimiter,
		approvalBroadcaster,
		b.config.Approvals.PendingTTL,
		b.config.Server.EndUser.PublicURL,
		b.logger,
	)

	// Wire approval sync subscriber for PostgreSQL backend (cross-instance long-poll wake-up)
	if b.config.Storage.Backend == "postgres" {
		app.ApprovalSyncSubscriber = postgresstorage.NewApprovalSyncSubscriber(
			b.config.Storage.Postgres.ConnectionURL,
			approvalBroadcaster,
			b.logger,
		)
	}

	// Assert PermissionSetService is available — FR-006 and FR-019 require it.
	// Both storage backends always wire PermissionSets(), so nil means a wiring bug.
	if app.PermissionSetService == nil {
		return nil, fmt.Errorf("permission set service is required: ensure storage.PermissionSets() is wired")
	}

	// Admin handlers
	app.AdminHandlers = &AdminHandlers{
		Agents:             admin.NewAgentsHandler(agentService, app.ProviderService, app.PermissionSetService, b.logger),
		Services:           admin.NewServicesHandler(app.ProviderService, b.config, b.logger),
		ProtectedResources: admin.NewProtectedResourcesHandler(app.ProviderService, b.logger),
		PermissionSets:     admin.NewPermissionSetsHandler(app.PermissionSetService, app.ProviderService, b.logger),
	}

	agentDetailHandler := consent.NewAgentDetailHandler(app.ConsentService, b.logger, app.SessionTokenService)

	// T040: Build OAuth2TokenHandler — fail-fast if multi-agent verifier construction fails.
	// Config validation makes this error unreachable in practice, but structural fail-closed
	// guarantees (SR-001) are not conditional on upstream validation alone.
	var multiAgentVerifier ports.MultiAgentVerifier
	if ov.multiAgentClient.Enabled {
		verifier, err := oauth2service.NewMultiAgentTokenVerifier(
			ov.multiAgentClient.AgentIDClaimName,
			sharedUpstreamJWKS,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create multi-agent token verifier: %w", err)
		}
		multiAgentVerifier = verifier
	}
	var grantHandler enduser.TokenGrantStrategy
	var proceedHandler enduser.AuthorizationProceedStrategy
	var jwksHandler *enduserHandlers.JWKSHandler
	var jwksPublisherHealth ports.JWKSPublisherHealthPort
	var jwksPublisher ports.JWKSPublisherPort

	signingKeyRepo := b.storage.SigningKeys()
	signingKeyBootstrapCoordinator := b.storage.SigningKeyBootstrapCoordinator()
	if signingKeyBootstrapCoordinator == nil {
		return nil, fmt.Errorf("signing key bootstrap coordinator is required")
	}

	localIssuerURI := ov.localIssuerURI

	// buildLocalProvider constructs the local token issuance infrastructure.
	// Used in both "local" and "hybrid" modes.
	buildLocalProvider := func(signingKeyService *oauth2server.SigningKeyService, tokenTTL time.Duration, claimsExpr string, bootstrapTimeout time.Duration) (*oauth2server.Provider, error) {
		provider, err := oauth2server.NewProvider(
			b.storage.AuthorizationCodes(),
			b.storage.RefreshTokenSessions(),
			b.storage.PKCESessions(),
			b.storage.BrokerCredentials(),
			clientResolver,
			signingKeyService,
			localIssuerURI,
			tokenTTL,
			ov.localRefreshTokenTTL,
			claimsExpr,
			b.logger,
			b.storage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OAuth2 server provider: %w", err)
		}

		startupCtx, cancel := newSigningKeyStartupContext(bootstrapTimeout)
		defer cancel()
		if key, created, err := signingKeyService.EnsureInitialKey(startupCtx, "ES256"); err != nil {
			return nil, fmt.Errorf("failed to ensure initial signing key: %w", err)
		} else if created {
			b.logger.Info("auto-generated initial signing key", "kid", key.KID, "algorithm", key.Algorithm)
		}

		readinessCtx, readinessCancel := newSigningKeyStartupContext(bootstrapTimeout)
		defer readinessCancel()
		if _, err := signingKeyService.GetCurrent(readinessCtx); err != nil {
			var storageErr *domstorage.StorageError
			if errors.As(err, &storageErr) && storageErr.Kind == domstorage.ErrorKindNotFound {
				// Keep startup non-fatal here so the broker can continue serving JWKS while a
				// grace-period key warms downstream caches before it starts signing tokens.
				b.logger.Warn("no currently-active signing key available — local token issuance is unavailable until a key activates or is promoted",
					"hint", "wait for activates_at or PUT /api/oauth2-server/signing-keys/{kid}/current")
				return provider, nil
			}
			return nil, fmt.Errorf("failed to check current signing key: %w", err)
		}
		return provider, nil
	}

	// wireLocalAdminHandlers constructs the local-mode admin services and handlers.
	// Used in both "local" and "hybrid" modes.
	wireLocalAdminHandlers := func() *oauth2server.SigningKeyService {
		signingKeyService := oauth2server.NewSigningKeyService(signingKeyRepo, signingKeyBootstrapCoordinator, encryptor, app.BranchKeyManager, b.logger)
		clientAuthService := oauth2server.NewClientAuthService(b.storage.BrokerCredentials(), clientResolver, b.logger)
		app.AdminHandlers.ClientCredentials = admin.NewClientCredentialsHandler(b.storage.BrokerCredentials(), agentService, clientAuthService, b.logger)
		app.AdminHandlers.SigningKeys = admin.NewSigningKeysHandler(signingKeyService, b.logger)
		return signingKeyService
	}

	// buildProxyStrategies constructs the proxy path strategies.
	// Used in both "proxy" and "hybrid" modes.
	buildProxyStrategies := func(upstreamTokenEndpoint string) (enduser.TokenGrantStrategy, enduser.AuthorizationProceedStrategy) {
		grant := enduser.NewProxyTokenGrantStrategy(
			upstreamTokenEndpoint,
			upstreamClient,
			multiAgentVerifier,
			b.logger,
		)
		proceed := enduser.NewProxyProceedStrategy()
		return grant, proceed
	}

	// impersonationIssuer is the local token issuer used to mint impersonated broker tokens.
	// It is set only in local mode; impersonation is rejected outside local mode at config time.
	var impersonationIssuer ports.ImpersonationTokenIssuer

	switch cfg := oauthCfg.(type) {
	case *ports.LocalOAuth2Config:
		signingKeyService := wireLocalAdminHandlers()
		provider, err := buildLocalProvider(signingKeyService, cfg.TokenTTL, cfg.TokenClaimsExpression, cfg.SigningKeys.BootstrapTimeout)
		if err != nil {
			return nil, err
		}
		impersonationIssuer = provider
		grantHandler = enduser.NewLocalGrantStrategy(newLocalMintingStrategy(provider), b.logger)
		proceedHandler = enduser.NewLocalProceedStrategy(newLocalCodeIssuer(provider), b.logger)
		b.logger.Info("OAuth2 server mode: local — local token minting enabled",
			"issuer_uri", localIssuerURI,
			"token_ttl", cfg.TokenTTL,
		)
		publisher := ports.JWKSPublisherPort(oauth2service.NewLocalJWKSPublisher(signingKeyService, b.logger))
		if b.jwksPublisher != nil {
			publisher = b.jwksPublisher
		}
		jwksPublisher = publisher
		jwksHandler = enduserHandlers.NewJWKSHandler(publisher, b.logger)
	case *ports.HybridOAuth2Config:
		signingKeyService := wireLocalAdminHandlers()
		provider, err := buildLocalProvider(signingKeyService, cfg.Local.TokenTTL, cfg.Local.TokenClaimsExpression, cfg.Local.SigningKeys.BootstrapTimeout)
		if err != nil {
			return nil, err
		}
		proxyGrant, proxyProceed := buildProxyStrategies(cfg.Proxy.UpstreamTokenEndpoint)
		localGrant := enduser.NewLocalGrantStrategy(newLocalMintingStrategy(provider), b.logger)
		localProceed := enduser.NewLocalProceedStrategy(newLocalCodeIssuer(provider), b.logger)

		grantHandler = enduser.NewHybridTokenGrantStrategy(proxyGrant, localGrant, b.logger)
		proceedHandler = enduser.NewHybridProceedStrategy(proxyProceed, localProceed, b.logger)
		b.logger.Info("OAuth2 server mode: hybrid — proxy and local token minting enabled",
			"issuer_uri", localIssuerURI,
			"token_ttl", cfg.Local.TokenTTL,
		)
		var hybridPublisher ports.JWKSPublisherPort
		if b.jwksPublisher != nil {
			hybridPublisher = b.jwksPublisher
		} else {
			if sharedUpstreamJWKS == nil {
				return nil, fmt.Errorf("upstream JWKS adapter not initialized")
			}
			svc := oauth2service.NewHybridJWKSPublisher(signingKeyService, sharedUpstreamJWKS, b.logger)
			// T046: Check for kid conflicts at startup; log error but allow startup to continue.
			kidCtx, kidCancel := context.WithTimeout(context.Background(), ov.upstreamTimeout)
			_, kidErr := svc.PublishJWKS(kidCtx)
			kidCancel()
			if errors.Is(kidErr, ports.ErrKidConflict) {
				b.logger.Error("startup kid conflict in hybrid JWKS aggregation — /oauth2/jwks.json will return 500 until resolved", "error", kidErr)
			} else if kidErr != nil {
				b.logger.Warn("startup upstream JWKS probe failed — /oauth2/jwks.json will return 503 until upstream recovers", "error", kidErr)
			}
			hybridPublisher = svc
		}
		if healthPublisher, ok := hybridPublisher.(ports.JWKSPublisherHealthPort); ok {
			jwksPublisherHealth = healthPublisher
		}
		jwksPublisher = hybridPublisher
		jwksHandler = enduserHandlers.NewJWKSHandler(hybridPublisher, b.logger)
	case *ports.ProxyOAuth2Config:
		grantHandler, proceedHandler = buildProxyStrategies(cfg.UpstreamTokenEndpoint)
		var proxyPublisher ports.JWKSPublisherPort
		if b.jwksPublisher != nil {
			proxyPublisher = b.jwksPublisher
		} else {
			if sharedUpstreamJWKS == nil {
				return nil, fmt.Errorf("upstream JWKS adapter not initialized")
			}
			proxyPublisher = oauth2service.NewProxyJWKSPublisher(sharedUpstreamJWKS, b.logger)
		}
		if healthPublisher, ok := proxyPublisher.(ports.JWKSPublisherHealthPort); ok {
			jwksPublisherHealth = healthPublisher
		}
		jwksPublisher = proxyPublisher
		jwksHandler = enduserHandlers.NewJWKSHandler(proxyPublisher, b.logger)
	default:
		panic(fmt.Sprintf("BUG: unhandled OAuth2ModeConfig type %T — update strategy switch", oauthCfg))
	}

	// Build the impersonation service (local mode only). Config validation already rejected
	// impersonation outside local mode and validated the static rule shape (CR-001..CR-008).
	var impersonationService enduser.ImpersonationService
	if impCfg := b.config.OAuth2AuthServer.Impersonation; impCfg != nil {
		if impersonationIssuer == nil {
			return nil, fmt.Errorf("oauth2_authorization_server.impersonation requires local mode with a local token issuer")
		}
		if app.ConsentService == nil {
			return nil, fmt.Errorf("oauth2_authorization_server.impersonation requires user delegation verification, but ConsentService is unavailable")
		}
		consentBaseURL := strings.TrimRight(b.config.Server.EndUser.PublicURL, "/")
		if consentBaseURL == "" {
			return nil, fmt.Errorf("oauth2_authorization_server.impersonation requires server.enduser.public_url for consent error_uri")
		}
		// CR-004: the broker's own issuer must never sign the client_assertion role.
		normalizedLocalIssuer := normalizedIssuerURI(ov.localIssuerURI)
		for i, rule := range impCfg.Rules {
			for j, issuer := range rule.TrustedIssuers {
				for _, role := range issuer.SignsRoles {
					if ports.CredentialRole(role) == ports.CredentialRoleClientAssertion &&
						normalizedIssuerURI(issuer.IssuerURI) == normalizedLocalIssuer {
						return nil, fmt.Errorf("oauth2_authorization_server.impersonation.rules[%d].trusted_issuers[%d].issuer_uri must be an external identity provider, not the broker's own issuer (%q); broker-minted tokens must never be accepted as privileged-client assertions", i, j, ov.localIssuerURI)
					}
				}
			}
		}

		svc, err := impersonation.NewService(impCfg, b.newImpersonationJWKSFactory(upstreamClient), b.storage.Agents(), impersonationIssuer, 0, b.logger, newUserDelegationVerifier(app.ConsentService), consentBaseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to build impersonation service: %w", err)
		}
		impersonationService = svc
		b.logger.Info("OAuth2 impersonation enabled", "audience_prefix", impCfg.AudiencePrefix, "rules", len(impCfg.Rules))
	}

	app.ApprovalRequestAuthenticator = httpmiddleware.NewApprovalRequestAuthenticator(nil, nil)

	var approvalJWTValidator *tokenexchange.JWTValidator
	var approvalCELEvaluator *tokenexchange.CELEvaluator
	if approvalAuthEnabled {
		if jwksPublisher == nil {
			return nil, fmt.Errorf("approval authentication requires JWKS publisher, but none was wired")
		}
		if clientAssertionJWKS == nil {
			return nil, fmt.Errorf("approval authentication requires a client-assertion JWKS trust anchor, but none was initialized")
		}

		celConfig := tokenexchange.CELEvaluatorConfig{
			PrincipalExpression:     b.config.TokenExchange.ClaimExtraction.PrincipalExpression,
			AgentIDExpression:       b.config.TokenExchange.ClaimExtraction.AgentIDExpression,
			AuthorizationExpression: b.config.TokenExchange.Authorization.CEL.Expression,
			EvaluationTimeout:       b.config.TokenExchange.Authorization.CEL.EvaluationTimeout,
		}
		if !ov.multiAgentClient.Enabled {
			celConfig.ResolveAgentIDByClientID = newTokenExchangeAgentIDResolver(agentService, b.config.Storage.Timeouts.Read)
		}

		var err error
		approvalCELEvaluator, err = tokenexchange.NewCELEvaluator(celConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create CEL evaluator for approval authentication: %w", err)
		}

		subjectTokenJWKSAdapter, err := jwks.NewPublishedAdapter(jwksPublisher)
		if err != nil {
			return nil, fmt.Errorf("failed to create published JWKS adapter for approval subject tokens: %w", err)
		}

		var subjectTokenIssuers []string
		switch oauthCfg.(type) {
		case *ports.LocalOAuth2Config:
			subjectTokenIssuers = []string{localIssuerURI}
		case *ports.HybridOAuth2Config:
			subjectTokenIssuers = []string{ov.upstreamIssuerURI, localIssuerURI}
		default:
			subjectTokenIssuers = []string{ov.upstreamIssuerURI}
		}

		brokerAudience := b.config.TokenExchange.ExpectedAudience
		if brokerAudience == "" {
			brokerAudience = tokenexchange.DefaultBrokerAudience
		}

		approvalJWTValidator, err = tokenexchange.NewJWTValidatorWithPolicies(
			tokenexchange.JWTValidationPolicy{
				JWKSProvider:    subjectTokenJWKSAdapter,
				ExpectedIssuers: subjectTokenIssuers,
			},
			tokenexchange.JWTValidationPolicy{
				JWKSProvider:    clientAssertionJWKS,
				ExpectedIssuers: []string{clientAssertionIssuerURI},
			},
			brokerAudience,
			tokenexchange.DefaultClockSkewTolerance,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create JWT validator for approval authentication: %w", err)
		}
		app.ApprovalRequestAuthenticator = httpmiddleware.NewApprovalRequestAuthenticator(approvalJWTValidator, approvalCELEvaluator)
	}

	if tokenExchangeEnabled {
		if app.ConsentService == nil {
			return nil, fmt.Errorf("token exchange service requires consent service, but storage repositories (Agents, Services, UserGrants) are not available")
		}

		tokenExchangeService, err := tokenexchange.NewTokenExchangeService(
			approvalJWTValidator,
			approvalCELEvaluator,
			app.ProviderService,
			app.OAuth2SessionService,
			app.ConsentService,
			app.PermissionSetService,
			b.storage.Agents(),
			&b.config.TokenExchange,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create token exchange service: %w", err)
		}
		app.TokenExchangeService = tokenExchangeService
	}

	app.jwksPublisherHealth = jwksPublisherHealth

	oauth2MetadataHandler := &enduser.OAuth2MetadataHandler{
		Service: app.OAuth2Service,
	}

	app.EnduserHandlers = &EnduserHandlers{
		UserInfo:       consent.NewUserInfoHandler(b.logger),
		Agents:         consent.NewAgentsHandler(app.ConsentService, b.logger),
		AgentDetail:    agentDetailHandler,
		Grants:         consent.NewGrantsHandler(app.ConsentService, b.logger, app.SessionTokenService),
		OAuth2Sessions: oauth2_sessions.NewHandler(app.OAuth2SessionService),
		OAuth2Authorize: &enduser.OAuth2AuthorizeHandler{
			Service:        app.OAuth2Service,
			Logger:         b.logger,
			ProceedHandler: proceedHandler,
		},
		OAuth2Token: &enduser.OAuth2TokenHandler{
			TokenExchange: app.TokenExchangeService,
			OAuth2Service: app.OAuth2Service,
			Logger:        b.logger,
			GrantHandler:  grantHandler,
			Impersonation: impersonationService,
		},
		OAuth2Metadata:       oauth2MetadataHandler,
		ApprovalCreate:       approval.NewCreateHandler(app.ApprovalService),
		ApprovalGet:          approval.NewGetHandler(app.ApprovalService),
		ApprovalApprove:      approval.NewApproveHandler(app.ApprovalService),
		ApprovalScopePreview: approval.NewScopePreviewHandler(app.ApprovalService),
		ApprovalDeny:         approval.NewDenyHandler(app.ApprovalService),
		ApprovalConsume:      approval.NewConsumeHandler(app.ApprovalService),
		ApprovalRevoke:       approval.NewRevokeHandler(app.ApprovalService),
		ApprovalSync:         approval.NewSyncHandler(app.ApprovalService),
		ApprovalPermanent:    approval.NewPermanentHandler(app.ApprovalService),
		ApprovalPending:      approval.NewPendingHandler(app.ApprovalService),
		JWKS:                 jwksHandler,
		SPA:                  handlers.NewSPAHandler(b.staticWebResourcesPath, b.logger),
	}

	// Start maintenance only after all fallible construction has completed.
	switch oauthCfg.(type) {
	case *ports.LocalOAuth2Config, *ports.HybridOAuth2Config:
		cleanup := oauth2server.NewSessionCleanup(
			b.storage.AuthorizationCodes(),
			b.storage.PKCESessions(),
			b.storage.RefreshTokenSessions(),
			b.logger,
		)
		cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
		cleanupDone := make(chan struct{})
		go func() {
			defer close(cleanupDone)
			cleanup.Run(cleanupCtx)
		}()
		prevShutdown := app.Shutdown
		app.Shutdown = func(ctx context.Context) error {
			cancelCleanup()
			<-cleanupDone
			if prevShutdown != nil {
				return prevShutdown(ctx)
			}
			return nil
		}
	}

	return app, nil
}

func normalizedIssuerURI(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")

	return parsed.String()
}

func newTokenExchangeAgentIDResolver(agentService *agentsservice.Service, timeout time.Duration) func(string) (string, error) {
	return func(rawIdentifier string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		agent, err := agentService.ResolveUniqueByClientID(ctx, id.ClientID(rawIdentifier))
		if err == nil {
			return agent.ID.String(), nil
		}
		if !ports.IsNotFoundErr(err) {
			return "", fmt.Errorf("resolveAgentIdByClientId: %w", err)
		}

		parsedAgentID, parseErr := id.ParseAgentID(rawIdentifier)
		if parseErr != nil {
			return "", fmt.Errorf("resolveAgentIdByClientId: %w", parseErr)
		}

		agent, err = agentService.Get(ctx, parsedAgentID)
		if err != nil {
			return "", fmt.Errorf("resolveAgentIdByClientId: %w", err)
		}

		return agent.ID.String(), nil
	}
}

// impersonationDiscoveryTimeout bounds JWKS discovery for an impersonation trusted issuer whose
// jwks_uri is not set explicitly.
const impersonationDiscoveryTimeout = 30 * time.Second

// newImpersonationJWKSFactory returns a factory that builds a cached JWKS provider per trusted
// impersonation issuer, discovering the JWKS URI from issuer metadata when it is not configured.
func (b *Builder) newImpersonationJWKSFactory(httpClient *http.Client) impersonation.JWKSProviderFactory {
	return func(issuer ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		jwksURI := issuer.JWKSURI
		if jwksURI == "" {
			discoveryCtx, cancel := context.WithTimeout(context.Background(), impersonationDiscoveryTimeout)
			discovered, err := domstorage.DiscoverOAuth2Endpoints(discoveryCtx, issuer.IssuerURI, nil, b.config.Security.SkipThirdpartyHTTPSValidation)
			cancel()
			if err != nil {
				return nil, fmt.Errorf("failed to discover JWKS for issuer %q: %w", issuer.IssuerURI, err)
			}
			jwksURI = discovered.JWKsURI
		}
		minRefresh := issuer.JWKSMinRefresh
		if minRefresh == 0 {
			minRefresh = 15 * time.Minute
		}
		maxRefresh := issuer.JWKSMaxRefresh
		if maxRefresh == 0 || maxRefresh < minRefresh {
			if minRefresh > time.Hour {
				maxRefresh = minRefresh
			} else {
				maxRefresh = time.Hour
			}
		}
		adapter, err := jwks.NewJWKSAdapter(jwksURI, httpClient, minRefresh, maxRefresh, b.logger)
		if err != nil {
			return nil, err
		}
		return adapter, nil
	}
}
