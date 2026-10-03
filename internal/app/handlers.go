// Package app provides application-layer wiring and orchestration.
package app

import (
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/enduser"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers/admin"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers/consent"
	enduserHandlers "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers/enduser"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/oauth2_sessions"
)

// AdminHandlers groups all handler instances needed by the admin server.
// This struct is passed to routing functions to avoid scattering handler creation.
type AdminHandlers struct {
	// Agents handler for admin API - manages agent CRUD operations
	Agents *admin.AgentsHandler

	// Services handler for admin API - manages OAuth2 service CRUD operations
	Services *admin.ServicesHandler

	// ProtectedResources handler manages a service's protected-resource set.
	ProtectedResources *admin.ProtectedResourcesHandler

	// PermissionSets handler for admin API - manages permission set CRUD operations
	PermissionSets *admin.PermissionSetsHandler

	// ClientCredentials handler for admin API - manages broker client credentials
	ClientCredentials *admin.ClientCredentialsHandler

	// SigningKeys handler for admin API - manages signing key lifecycle
	SigningKeys *admin.SigningKeysHandler

	// CIMDClientKeys manages the dedicated outbound CIMD key lifecycle.
	CIMDClientKeys *admin.CIMDClientKeysHandler
}

// EnduserHandlers groups all handler instances needed by the enduser server.
// This struct is passed to routing functions to avoid scattering handler creation.
type EnduserHandlers struct {
	// UserInfo handler for GET /api/me - returns current user information
	UserInfo *consent.UserInfoHandler

	// Consent handlers for /api/consent routes
	Agents      *consent.AgentsHandler
	AgentDetail *consent.AgentDetailHandler
	Grants      *consent.GrantsHandler

	// OAuth2 sessions handler for /api/third-party routes
	OAuth2Sessions *oauth2_sessions.Handler

	// OAuth2 authorization server handlers (serve both proxy and local mode)
	OAuth2Authorize *enduser.OAuth2AuthorizeHandler
	OAuth2Token     *enduser.OAuth2TokenHandler
	OAuth2Metadata  *enduser.OAuth2MetadataHandler

	// Approval handlers for /api/approvals routes
	ApprovalCreate       *approval.CreateHandler
	ApprovalGet          *approval.GetHandler
	ApprovalApprove      *approval.ApproveHandler
	ApprovalScopePreview *approval.ScopePreviewHandler
	ApprovalDeny         *approval.DenyHandler
	ApprovalConsume      *approval.ConsumeHandler
	ApprovalRevoke       *approval.RevokeHandler
	ApprovalSync         *approval.SyncHandler
	ApprovalPermanent    *approval.PermanentHandler
	ApprovalPending      *approval.PendingHandler

	// JWKS handler (all OAuth2 modes — serves aggregated signing key public material)
	JWKS *enduserHandlers.JWKSHandler

	// CIMDMetadata serves the public broker-hosted metadata and JWK documents.
	CIMDMetadata *enduserHandlers.CIMDMetadataHandler

	// SPA handler for serving static files (must be last)
	SPA *handlers.SPAHandler
}
