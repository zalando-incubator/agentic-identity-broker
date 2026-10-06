// Package routing provides HTTP route configuration for different server types.
package routing

import (
	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// AdminRouteConfig provides optional configuration for admin route setup.
type AdminRouteConfig struct {
	CORS ports.CORSConfig
}

// SetupAdminRoutes registers all administrative API routes.
// Routes include agent and service management endpoints.
//
// Route structure:
//
//	POST   /api/agents                 - Create agent
//	GET    /api/agents                 - List agents
//	GET    /api/agents/{agent-id}      - Get agent details
//	PUT    /api/agents/{agent-id}      - Update agent
//	DELETE /api/agents/{agent-id}      - Delete agent
//
//	POST   /api/services               - Create service
//	GET    /api/services               - List services
//	GET    /api/services/{service-id}  - Get service details
//	PUT    /api/services/{service-id}  - Update service
//	DELETE /api/services/{service-id}  - Delete service
func SetupAdminRoutes(r chi.Router, h *app.AdminHandlers, cfg AdminRouteConfig) {

	r.Route("/api", func(r chi.Router) {
		// Apply CORS middleware (no-op if AllowedOrigins empty)
		r.Use(middleware.CORSMiddleware(cfg.CORS))

		// Agent management routes
		r.Route("/agents", func(r chi.Router) {
			r.Post("/", h.Agents.CreateAgent)             // POST /api/agents
			r.Get("/", h.Agents.ListAgents)               // GET /api/agents
			r.Get("/{agent-id}", h.Agents.GetAgent)       // GET /api/agents/:agent-id
			r.Put("/{agent-id}", h.Agents.UpdateAgent)    // PUT /api/agents/:agent-id
			r.Delete("/{agent-id}", h.Agents.DeleteAgent) // DELETE /api/agents/:agent-id
		})

		// Services management routes
		r.Route("/services", func(r chi.Router) {
			r.Post("/", h.Services.CreateService)               // POST /api/services
			r.Get("/", h.Services.ListServices)                 // GET /api/services
			r.Get("/{service-id}", h.Services.GetService)       // GET /api/services/:service-id
			r.Put("/{service-id}", h.Services.UpdateService)    // PUT /api/services/:service-id
			r.Delete("/{service-id}", h.Services.DeleteService) // DELETE /api/services/:service-id
			// Protected-resource member routes use a catch-all matcher so encoded slashes
			// remain addressable as one escaped URI segment; the handler validates it.
			r.Route("/{service-id}/protected-resources", func(r chi.Router) {
				r.Get("/", h.ProtectedResources.List)
				r.Post("/", h.ProtectedResources.Create)
				r.Put("/{resource:.*}", h.ProtectedResources.Add)
				r.Patch("/{resource:.*}", h.ProtectedResources.Rename)
				r.Delete("/{resource:.*}", h.ProtectedResources.Remove)
			})
		})

		// Permission sets management routes
		r.Route("/permission-sets", func(r chi.Router) {
			r.Post("/", h.PermissionSets.Create) // POST /api/permission-sets
			r.Get("/", h.PermissionSets.List)    // GET /api/permission-sets
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.PermissionSets.Get)       // GET /api/permission-sets/:id
				r.Put("/", h.PermissionSets.Update)    // PUT /api/permission-sets/:id
				r.Delete("/", h.PermissionSets.Delete) // DELETE /api/permission-sets/:id
			})
		})

		// Client credential management routes (per-agent)
		if h.ClientCredentials != nil {
			r.Route("/agents/{agent-id}/client-credentials", func(r chi.Router) {
				r.Post("/", h.ClientCredentials.Generate) // POST /api/agents/:agent-id/client-credentials
				r.Get("/", h.ClientCredentials.Get)       // GET /api/agents/:agent-id/client-credentials
				r.Delete("/", h.ClientCredentials.Revoke) // DELETE /api/agents/:agent-id/client-credentials
			})
		}

		// CIMD client-authentication key management routes are available in every OAuth server mode.
		if h.CIMDClientKeys != nil {
			r.Route("/cimd-client-keys", func(r chi.Router) {
				r.Post("/", h.CIMDClientKeys.Create)
				r.Get("/", h.CIMDClientKeys.List)
				r.Put("/{kid}/current", h.CIMDClientKeys.Promote)
				r.Delete("/{kid}", h.CIMDClientKeys.Remove)
			})
		}

		// Signing key management routes
		if h.SigningKeys != nil {
			r.Route("/oauth2-server/signing-keys", func(r chi.Router) {
				r.Post("/", h.SigningKeys.Add)                    // POST /api/oauth2-server/signing-keys
				r.Get("/", h.SigningKeys.List)                    // GET /api/oauth2-server/signing-keys
				r.Put("/{kid}/current", h.SigningKeys.SetCurrent) // PUT /api/oauth2-server/signing-keys/:kid/current
				r.Delete("/{kid}", h.SigningKeys.Remove)          // DELETE /api/oauth2-server/signing-keys/:kid
			})
		}
	})
}
