// Package ports defines the ports (interfaces) for OAuth2 functionality.
package ports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// Session token sentinel errors — returned by SessionTokenValidator implementations and
// handled by adapters that consume the port. Defined here so adapters do not need to import
// the concrete sessiontoken package to interpret errors from the interface.
var (
	ErrSessionExpired           = errors.New("authorization session expired")
	ErrSessionInvalidToken      = errors.New("authorization session token invalid")
	ErrSessionAgentMismatch     = errors.New("authorization session does not match requested agent")
	ErrSessionPrincipalMismatch = errors.New("authorization session does not belong to this user")
)

// MultiAgentVerifier verifies agent ID claims in proxied upstream token responses.
// When non-nil (feature enabled), VerifyAgentIDClaim is called after buffering the
// upstream response body. If verification fails, the response is withheld and an
// OAuth2 server_error is returned to the client (fail closed per SR-001).
// Nil means feature disabled — upstream response is passed through unchanged.
type MultiAgentVerifier interface {
	VerifyAgentIDClaim(ctx context.Context, responseBody []byte, expectedAgentID id.AgentID) error
}

// OAuth2Service defines the domain service interface for OAuth2 authorization,
// token proxying, and metadata discovery.
type OAuth2Service interface {
	// HandleAuthorization processes an OAuth2 authorization request, checking
	// client validity and user consent status, returning a decision (redirect URL or error).
	HandleAuthorization(ctx context.Context, req *AuthorizationRequest, principal id.Principal) (*AuthorizationDecision, error)

	// ResolveForTokenGrant resolves the client_id, classifies the agent, and
	// enforces mode boundaries for the token endpoint. Returns the resolved agent
	// and an OAuth2 error if resolution or mode enforcement fails.
	ResolveForTokenGrant(ctx context.Context, clientID id.ClientID) (*TokenGrantResolution, error)

	// GenerateMetadata returns RFC 8414 OAuth2 metadata for auto-discovery.
	GenerateMetadata(ctx context.Context) (*MetadataResponse, error)
}

// TokenGrantResolution is the result of client resolution for the token endpoint.
type TokenGrantResolution struct {
	AgentID    id.AgentID
	ClientID   *id.ClientID // nil for local/CIMD agents
	ClientType storage.ClientType
}

// NewTokenGrantResolution constructs a TokenGrantResolution and enforces that
// ProxyClient always carries a non-nil ClientID.
func NewTokenGrantResolution(agentID id.AgentID, clientID *id.ClientID, mode storage.ClientType) (*TokenGrantResolution, error) {
	if mode == storage.ProxyClient && clientID == nil {
		return nil, fmt.Errorf("TokenGrantResolution: ProxyClient requires a non-nil ClientID")
	}
	return &TokenGrantResolution{AgentID: agentID, ClientID: clientID, ClientType: mode}, nil
}

// AuthorizationRequest represents an OAuth2 authorization request (RFC 6749 Section 4.1.1).
// Fields are parsed from HTTP query parameters.
type AuthorizationRequest struct {
	// REQUIRED: OAuth2 client_id identifying the agent making the request
	ClientID id.ClientID

	// REQUIRED: Client's callback URL for authorization code
	RedirectURI string

	// OPTIONAL: Space-delimited requested scopes
	Scope string

	// RECOMMENDED: Opaque value for CSRF protection
	State string

	// REQUIRED: Must be "code" for authorization code flow
	ResponseType string

	// OPTIONAL: PKCE code challenge (RFC 7636)
	CodeChallenge string

	// OPTIONAL: PKCE method ("S256" or "plain")
	CodeChallengeMethod string

	// INTERNAL: Full original URL for redirect to consent UI
	OriginalURL string
}

// AuthorizationDecision represents the broker's decision for an authorization request.
// Either redirects to upstream or to consent UI, or returns an error.
type AuthorizationDecision struct {
	// Action determines the response: "proceed", "redirect_to_consent", or "error".
	// "proceed" means the user has an active grant and the request can continue.
	// In proxy mode the handler redirects to the upstream OAuth2 server;
	// in local mode the handler issues a local authorization code.
	Action string

	// RedirectURL is the target URL for HTTP 302 redirect
	RedirectURL string

	// ClientType is the resolved agent classification (set on "proceed" actions only).
	// Used by hybrid proceed strategy to dispatch to proxy or local sub-strategy explicitly.
	ClientType storage.ClientType

	// ErrorCode is the OAuth2 error code (if Action == "error")
	ErrorCode string

	// ErrorDesc is the human-readable error description
	ErrorDesc string
}

// ProceedDecision returns a decision allowing the authorization request to proceed.
// clientType is required so the hybrid strategy can dispatch to the correct sub-strategy.
func ProceedDecision(redirectURL string, clientType storage.ClientType) *AuthorizationDecision {
	return &AuthorizationDecision{Action: "proceed", RedirectURL: redirectURL, ClientType: clientType}
}

// ConsentDecision returns a decision that redirects the user to the consent UI.
// ClientType is left as the zero value (UnknownClient); it is irrelevant for non-proceed actions.
func ConsentDecision(consentURL string) *AuthorizationDecision {
	return &AuthorizationDecision{Action: "redirect_to_consent", RedirectURL: consentURL}
}

// ErrorDecision returns a decision that signals an OAuth2 error.
// redirectURL may be empty when the redirect URI has not yet been verified (RFC 6749 §4.1.2.1).
// ClientType is left as the zero value (UnknownClient); it is irrelevant for non-proceed actions.
func ErrorDecision(code, desc, redirectURL string) *AuthorizationDecision {
	return &AuthorizationDecision{Action: "error", ErrorCode: code, ErrorDesc: desc, RedirectURL: redirectURL}
}

// MetadataResponse represents OAuth2 Authorization Server Metadata (RFC 8414).
// Enables OAuth2 clients to auto-discover the broker's endpoints and capabilities.
type MetadataResponse struct {
	// REQUIRED: Issuer identifier (broker's public base URL)
	Issuer string `json:"issuer"`

	// REQUIRED: URL of the authorization endpoint
	AuthorizationEndpoint string `json:"authorization_endpoint"`

	// REQUIRED: URL of the token endpoint
	TokenEndpoint string `json:"token_endpoint"`

	// REQUIRED: Supported response types (at minimum ["code"])
	ResponseTypesSupported []string `json:"response_types_supported"`

	// REQUIRED: Supported grant types (at minimum ["authorization_code"])
	GrantTypesSupported []string `json:"grant_types_supported"`

	// OPTIONAL: Supported token endpoint authentication methods
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`

	// OPTIONAL: Scopes supported by the broker
	ScopesSupported []string `json:"scopes_supported,omitempty"`

	// OPTIONAL: Claim types supported
	ClaimTypesSupported []string `json:"claim_types_supported,omitempty"`

	// OPTIONAL: JWKS URI for public key discovery (present in local mode)
	JWKSURI string `json:"jwks_uri,omitempty"`

	// OPTIONAL: Supported PKCE code challenge methods (present in local mode)
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported,omitempty"`

	// OPTIONAL: Whether CIMD-based client_id resolution is supported (RFC draft)
	ClientIDMetadataDocumentSupported *bool `json:"client_id_metadata_document_supported,omitempty"`
}

// TokenMintingStrategy abstracts local token grant processing in local mode.
// Grants are processed locally by the oauth2server.Provider.
// In proxy mode, grants are handled at the HTTP layer by proxyTokenGrantStrategy.
type TokenMintingStrategy interface {
	// HandleClientCredentials processes a client_credentials grant type request.
	// Returns the token response or an error.
	HandleClientCredentials(ctx context.Context, clientID id.ClientID, clientSecret, scope string) (*TokenResponse, error)

	// HandleAuthorizationCodeExchange processes an authorization_code grant type request.
	// Returns the token response or an error.
	HandleAuthorizationCodeExchange(ctx context.Context, clientID id.ClientID, clientSecret, code, redirectURI, codeVerifier string) (*TokenResponse, error)

	// HandleRefreshToken processes a refresh_token grant type request.
	// Returns the token response or an error.
	HandleRefreshToken(ctx context.Context, clientID id.ClientID, clientSecret, refreshToken, scope string) (*TokenResponse, error)
}

// TokenResponse represents a successful OAuth2 token response from a minting strategy.
type TokenResponse struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int64
	Scope        string
}

// AuthorizationCodeIssuer abstracts how the authorize endpoint issues authorization codes.
// In proxy mode, this is nil and the handler redirects to an upstream OAuth2 server.
// In local mode, the endpoint issues authorization codes locally.
type AuthorizationCodeIssuer interface {
	// IssueAuthorizationCode processes a validated authorization request and returns
	// an authorization code. The handler is responsible for redirect_uri validation
	// and PKCE enforcement before calling this method.
	IssueAuthorizationCode(ctx context.Context, req *AuthorizationRequest, principal id.Principal) (code string, err error)
}

// SessionCIMDMetadata carries CIMD-resolved client metadata from a validated session token.
type SessionCIMDMetadata struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name,omitempty"`
	LogoURI      string   `json:"logo_uri,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
}

// Validate checks that required fields are present and normalises nil slices.
// Must be called before sealing a SessionCIMDMetadata into a JWE session token.
func (m *SessionCIMDMetadata) Validate() error {
	if m.ClientID == "" {
		return errors.New("SessionCIMDMetadata: ClientID must not be empty")
	}
	if m.RedirectURIs == nil {
		m.RedirectURIs = []string{}
	}
	return nil
}

// AuthorizationSession is the port-local DTO returned by SessionTokenValidator.
type AuthorizationSession struct {
	AgentID      id.AgentID
	Principal    id.Principal
	OriginalURL  string
	CIMDMetadata *SessionCIMDMetadata
}

// SessionTokenValidator validates JWE authorization session tokens.
type SessionTokenValidator interface {
	ValidateAuthorizationSessionToken(token string, agentID id.AgentID, principal id.Principal) (*AuthorizationSession, error)
}

// ImpersonationMintInput carries extracted identities and the audience-selected target agent.
// It never carries credential values or privileged-client identity.
type ImpersonationMintInput struct {
	// Subject is the impersonated user identity, minted as the token sub claim.
	Subject string
	// Email is the optional subject email provided to local token-claims policy.
	Email *string
	// Actor is the acting-party identity, minted as nested act.sub.
	Actor string
	// ActorIssuer is the validated actor token issuer, minted as nested act.iss.
	ActorIssuer string
	// TargetAgent supplies minted agent_id and local-token CEL agent.* context.
	TargetAgent *storage.Agent
	// Scopes are validated granted scopes minted into the access token.
	Scopes []string
}

// ImpersonationTokenIssuer mints a locally signed impersonated broker token.
// Implemented by the local oauth2server.Provider; keeps the impersonation domain service
// insulated from the fosite/oauth2server concrete (Principle VI).
type ImpersonationTokenIssuer interface {
	// IssueImpersonationToken returns the signed access token or an error.
	IssueImpersonationToken(ctx context.Context, input ImpersonationMintInput) (string, error)
}

// UserDelegationStatus describes the active state of a principal's delegation to an agent.
type UserDelegationStatus string

const (
	UserDelegationActive  UserDelegationStatus = "active"
	UserDelegationMissing UserDelegationStatus = "missing"
	UserDelegationExpired UserDelegationStatus = "expired"
)

type UserDelegationDecision struct {
	Status     UserDelegationStatus
	GrantID    id.GrantID
	ValidUntil *time.Time
}

// UserDelegationVerifier reports whether a principal has an active delegation to an agent.
// A non-nil error means the status could not be determined and callers must fail closed.
type UserDelegationVerifier interface {
	VerifyUserDelegation(ctx context.Context, principal id.Principal, agentID id.AgentID, decisionTime time.Time) (UserDelegationDecision, error)
}
