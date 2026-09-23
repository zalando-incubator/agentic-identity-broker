// Package routing provides HTTP route configuration for different server types.
package routing

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtauth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// EnduserRouteConfig provides optional configuration for enduser route setup.
type EnduserRouteConfig struct {
	Authentication   ports.AuthenticationConfig
	JWTAuthenticator jwtauth.JWTAuthenticator

	ApprovalRequestAuthenticator *middleware.ApprovalRequestAuthenticator

	// Logger for middleware
	Logger *slog.Logger

	// CORS configuration for API routes
	CORS ports.CORSConfig

	// Telemetry contains observability configuration. When Telemetry.Enabled and
	// Telemetry.Traces.Enabled are both true, otelchi HTTP tracing middleware is registered.
	Telemetry ports.TelemetryConfig
}

// SetupEnduserRoutes registers all end-user API routes and optional SPA serving.
//
// Route structure:
//
//	API Routes:
//	GET    /api/me                                    - Get current user info
//
//	Consent Routes (authenticated):
//	GET    /api/consent/agents                        - List agents with delegations
//	GET    /api/consent/agents/{agent-id}             - Get agent details
//	GET    /api/consent/agents/{agent-id}/grants      - Get agent grants
//	POST   /api/consent/agents/{agent-id}/grants      - Create grant
//	DELETE /api/consent/agents/{agent-id}/grants      - Revoke grant (FR-014)
//
//	OAuth2 Session Routes (authenticated, optional):
//	GET    /api/third-party/sessions                  - List sessions
//	GET    /api/third-party/{serviceId}/oauth2/authorize - Initiate auth
//	POST   /api/third-party/{serviceId}/oauth2/authorize - Initiate auth with consent selection
//	GET    /api/third-party/{serviceId}/oauth2/callback  - Handle callback
//	GET    /api/third-party/{serviceId}/session       - Get session details
//	DELETE /api/third-party/{serviceId}/session       - Terminate session
//
//	Approval Routes (machine-facing):
//	POST   /api/approvals                             - Create pending approval (dual auth: subject token + client assertion)
//	GET    /api/approvals                             - Sync approvals (long-poll, client assertion)
//	POST   /api/approvals/{id}/consume                - Consume approved token (subject token)
//
//	Approval Routes (browser-facing, require principal):
//	GET    /api/approvals/permanent                   - List permanent approvals
//	GET    /api/approvals/pending                     - List pending approvals
//	GET    /api/approvals/{id}                        - Get approval detail
//	POST   /api/approvals/{id}/approve                - Approve
//	POST   /api/approvals/{id}/scope-preview          - Validate and render scope
//	POST   /api/approvals/{id}/deny                   - Deny
//	POST   /api/approvals/{id}/revoke                 - Revoke permanent approval
//
//	OAuth2 Authorization Server Routes (optional):
//	GET    /oauth2/authorize                          - Authorization endpoint
//	POST   /oauth2/token                              - Token endpoint
//	GET    /.well-known/oauth-authorization-server    - Metadata endpoint
//
//	SPA Serving (optional):
//	GET    /*                                         - Serve static SPA files
func SetupEnduserRoutes(r chi.Router, h *app.EnduserHandlers, cfg EnduserRouteConfig) {

	// Register API routes with CORS middleware
	r.Route("/api", func(r chi.Router) {
		// Apply CORS middleware (no-op if AllowedOrigins empty)
		r.Use(middleware.CORSMiddleware(cfg.CORS))

		// Approval routes — machine-facing endpoints use dedicated approval auth middleware,
		// while browser endpoints continue to use the acting-user principal middleware.
		if h.ApprovalGet != nil {
			requirePrincipal := middleware.RequirePrincipalMiddleware(cfg.Authentication, cfg.JWTAuthenticator, cfg.Logger)
			browserMutationProtection := http.NewCrossOriginProtection()

			r.Route("/approvals", func(approvalRouter chi.Router) {
				// Gateway/control-plane endpoints.
				approvalRouter.With(middleware.RequireApprovalSubjectTokenAndClientAssertion(cfg.ApprovalRequestAuthenticator)).Post("/", h.ApprovalCreate.ServeHTTP)
				approvalRouter.With(middleware.RequireApprovalClientAssertion(cfg.ApprovalRequestAuthenticator)).Get("/", h.ApprovalSync.ServeHTTP)

				// Browser-facing principal-scoped listings.
				approvalRouter.With(requirePrincipal).Get("/permanent", h.ApprovalPermanent.ServeHTTP)
				approvalRouter.With(requirePrincipal).Get("/pending", h.ApprovalPending.ServeHTTP)

				approvalRouter.Route("/{id}", func(r chi.Router) {
					r.With(requirePrincipal).Get("/", h.ApprovalGet.ServeHTTP)
					r.With(requirePrincipal, browserMutationProtection.Handler).Post("/approve", h.ApprovalApprove.ServeHTTP)
					r.With(requirePrincipal, browserMutationProtection.Handler).Post("/scope-preview", h.ApprovalScopePreview.ServeHTTP)
					r.With(requirePrincipal, browserMutationProtection.Handler).Post("/deny", h.ApprovalDeny.ServeHTTP)
					r.With(requirePrincipal, browserMutationProtection.Handler).Post("/revoke", h.ApprovalRevoke.ServeHTTP)
					r.With(middleware.RequireApprovalSubjectToken(cfg.ApprovalRequestAuthenticator)).Post("/consume", h.ApprovalConsume.ServeHTTP)
				})
			})
		}

		// Create subrouter for authenticated routes
		r.Route("/", func(authRouter chi.Router) {
			// Apply authentication middleware FIRST, before registering any routes
			authRouter.Use(middleware.RequirePrincipalMiddleware(cfg.Authentication, cfg.JWTAuthenticator, cfg.Logger))

			// Register user info endpoint (GET /api/me)
			authRouter.Get("/me", http.HandlerFunc(h.UserInfo.GetUserInfo).ServeHTTP)

			// Register OAuth2 sessions routes if handler is available
			if h.OAuth2Sessions != nil {
				h.OAuth2Sessions.RegisterRoutes(authRouter)
			}

			// Register consent routes if handlers are available
			if h.Agents != nil && h.AgentDetail != nil && h.Grants != nil {
				authRouter.Route("/consent", func(consentRouter chi.Router) {
					cop := http.NewCrossOriginProtection()
					consentRouter.Use(cop.Handler)

					// Agents list endpoint
					consentRouter.Get("/agents", h.Agents.GetAgentDelegations)

					// Agent-specific routes: /api/consent/agents/{agent-id}/...
					consentRouter.Route("/agents/{agent-id}", func(r chi.Router) {
						r.Get("/", h.AgentDetail.GetAgentDetail)
						r.Get("/grants", h.Grants.GetGrant)
						r.Post("/grants", h.Grants.CreateGrant)
						r.Delete("/grants", h.Grants.RevokeGrant)
					})
				})
			}
		})
	})

	// Register OAuth2 authorization server endpoints (optional, public routes).
	if h.JWKS == nil && (h.OAuth2Authorize != nil || h.OAuth2Token != nil || h.OAuth2Metadata != nil) {
		panic("BUG: JWKS handler required when OAuth2 routes are enabled")
	}
	r.Route("/oauth2", func(oauth2Router chi.Router) {
		// Single authorize handler serves both proxy and local mode.
		// In local mode, the handler's CodeIssuer strategy issues local codes.
		if h.OAuth2Authorize != nil {
			oauth2Router.With(
				middleware.OAuth2AuditMiddleware(cfg.Logger),
				middleware.RequirePrincipalMiddleware(cfg.Authentication, cfg.JWTAuthenticator, cfg.Logger),
			).Get("/authorize", h.OAuth2Authorize.ServeHTTP)
		}

		// Single token handler serves both proxy and local mode.
		// In local mode, the handler's TokenMinting strategy mints local tokens.
		if h.OAuth2Token != nil {
			oauth2Router.With(middleware.OAuth2AuditMiddleware(cfg.Logger)).Post("/token", h.OAuth2Token.ServeHTTP)
		}

		// JWKS endpoint — serves aggregated public key material in all modes.
		if h.JWKS != nil {
			oauth2Router.Get("/jwks.json", h.JWKS.ServeJWKS)
		}
	})

	// RFC 8414 discovery endpoint — single handler serves both modes.
	// The OAuth2Service.GenerateMetadata() includes JWKS URI in local mode.
	r.Route("/.well-known", func(discoveryRouter chi.Router) {
		if h.OAuth2Metadata != nil {
			discoveryRouter.Get("/oauth-authorization-server", h.OAuth2Metadata.ServeHTTP)
		}
	})

	// Reserve the health namespace; Server registers its GET endpoint after route setup.
	r.Route("/health", func(chi.Router) {})

	// Register SPA handler after reserved namespaces, for static-view GET and HEAD requests.
	if h.SPA != nil {
		r.Method(http.MethodGet, "/*", h.SPA)
		r.Method(http.MethodHead, "/*", h.SPA)

	}
}
