package tokenexchange

// RFC 8693 Token Exchange Constants
// These constants define grant types, token types, and other RFC 8693 parameters

// Grant Type Constants
const (
	// TokenExchangeGrantType is the RFC 8693 grant type for token exchange requests.
	// Used in the grant_type parameter of the token exchange request.
	TokenExchangeGrantType = "urn:ietf:params:oauth:grant-type:token-exchange" // #nosec G101 -- RFC-defined grant type URI, not a credential.

	// AuthorizationCodeGrantType is the standard OAuth2 authorization code grant type.
	// Used to distinguish token-exchange requests from authorization_code requests.
	AuthorizationCodeGrantType = "authorization_code"

	// RefreshTokenGrantType is the standard OAuth2 refresh token grant type.
	// Used to distinguish token-exchange requests from refresh_token requests.
	RefreshTokenGrantType = "refresh_token"
)

// Token Type Constants (RFC 8693 Section 3)
const (
	// AccessTokenType is the token type for access tokens.
	// Used in subject_token_type and issued_token_type parameters.
	AccessTokenType = "urn:ietf:params:oauth:token-type:access_token" // #nosec G101 -- RFC-defined token type URI, not a credential.

	// RefreshTokenType is the token type for refresh tokens.
	RefreshTokenType = "urn:ietf:params:oauth:token-type:refresh_token" // #nosec G101 -- RFC-defined token type URI, not a credential.

	// IDTokenType is the token type for ID tokens (OIDC).
	IDTokenType = "urn:ietf:params:oauth:token-type:id_token" // #nosec G101 -- RFC-defined token type URI, not a credential.

	// JWTTokenType is the token type for generic JWTs.
	JWTTokenType = "urn:ietf:params:oauth:token-type:jwt" // #nosec G101 -- RFC-defined token type URI, not a credential.

	// BearerTokenType is the token_type returned in RFC 8693 responses.
	// Indicates the access token follows the Bearer token usage defined in RFC 6750.
	BearerTokenType = "Bearer"
)

// Client Assertion Type Constants (RFC 7523)
const (
	// JWTBearerType is the client assertion type for JWT Bearer tokens.
	// Used for client authentication via signed assertions (RFC 7523).
	JWTBearerType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" // #nosec G101 -- RFC-defined assertion type URI, not a credential.
)

// Request Parameter Names (RFC 8693)
const (
	// GrantTypeParam is the RFC 8693 grant_type parameter name.
	GrantTypeParam = "grant_type"

	// SubjectTokenParam is the RFC 8693 subject_token parameter name.
	SubjectTokenParam = "subject_token"

	// SubjectTokenTypeParam is the RFC 8693 subject_token_type parameter name.
	SubjectTokenTypeParam = "subject_token_type"

	// ClientAssertionParam is the RFC 8693 client_assertion parameter name.
	ClientAssertionParam = "client_assertion"

	// ClientAssertionTypeParam is the RFC 8693 client_assertion_type parameter name.
	ClientAssertionTypeParam = "client_assertion_type"

	// ResourceParam is the RFC 8693 resource parameter name.
	ResourceParam = "resource"

	// ScopeParam is the RFC 8693 scope parameter name.
	ScopeParam = "scope"

	// ActorTokenParam is the RFC 8693 actor_token parameter name.
	ActorTokenParam = "actor_token"

	// ActorTokenTypeParam is the RFC 8693 actor_token_type parameter name.
	ActorTokenTypeParam = "actor_token_type"
)

// Response Field Names (RFC 8693 Section 2.2)
const (
	// AccessTokenField is the RFC 8693 response access_token field name.
	AccessTokenField = "access_token"

	// IssuedTokenTypeField is the RFC 8693 response issued_token_type field name.
	IssuedTokenTypeField = "issued_token_type" // #nosec G101 -- RFC-defined response field name, not a credential.

	// TokenTypeField is the RFC 8693 response token_type field name.
	TokenTypeField = "token_type"

	// ExpiresInField is the RFC 8693 response expires_in field name.
	ExpiresInField = "expires_in"

	// RefreshTokenField is the RFC 8693 response refresh_token field name (if present).
	RefreshTokenField = "refresh_token"

	// ScopeField is the RFC 8693 response scope field name (if present).
	ScopeField = "scope"
)

// Error Response Field Names (RFC 8693 Section 5.2)
const (
	// ErrorField is the RFC 8693 error response error field name.
	ErrorField = "error"

	// ErrorDescriptionField is the RFC 8693 error response error_description field name.
	ErrorDescriptionField = "error_description"

	// ErrorURIField is the RFC 8693 error response error_uri field name.
	ErrorURIField = "error_uri"
)

// Error Codes (RFC 8693 Section 5.2)
const (
	// InvalidRequestError indicates a missing or invalid parameter.
	InvalidRequestError = "invalid_request"

	// InvalidScopeError indicates a requested scope is not permitted.
	InvalidScopeError = "invalid_scope"

	// InvalidClientError indicates the client authentication failed.
	InvalidClientError = "invalid_client"

	// InvalidGrantError indicates an invalid, expired, or revoked token.
	InvalidGrantError = "invalid_grant"

	// UnauthorizedClientError indicates the client is not authorized for token exchange.
	UnauthorizedClientError = "unauthorized_client"

	// UnsupportedGrantTypeError indicates the grant type is not supported.
	UnsupportedGrantTypeError = "unsupported_grant_type"

	// InvalidTargetError indicates the token target (resource, audience, etc.) is invalid or unknown.
	InvalidTargetError = "invalid_target"

	// AccessDeniedError indicates the authorization server denied the request.
	AccessDeniedError = "access_denied"

	// ServerErrorCode indicates a server error occurred during processing.
	ServerErrorCode = "server_error"
)

// Default Configuration Values
const (
	// DefaultPrincipalExpression is the default CEL expression for extracting the principal from subject_token.
	// Extracts the 'sub' claim from the JWT.
	DefaultPrincipalExpression = "subject_token.sub"

	// DefaultAgentIDExpression is the default CEL expression for extracting the agent identifier from subject_token.
	// Extracts the 'azp' (authorized party) claim from the JWT.
	DefaultAgentIDExpression = "subject_token.azp"

	// DefaultAuthorizationExpression is the default CEL authorization expression.
	// Allows all valid gateways (returns true for all inputs).
	DefaultAuthorizationExpression = "true"

	// DefaultEvaluationTimeoutMs is the default timeout for CEL expression evaluation in milliseconds.
	DefaultEvaluationTimeoutMs = 100

	// DefaultRefreshEnabled is the default setting for automatic token refresh.
	DefaultRefreshEnabled = true
)

// CEL Environment Configuration
const (
	// MaxCELExpressionLength is the maximum allowed length for CEL expressions.
	// Prevents excessively long expressions that could cause performance issues.
	MaxCELExpressionLength = 10000

	// CELClaimsVariable is the name of the claims variable in CEL context.
	CELClaimsVariable = "claims"

	// CELRequestVariable is the name of the request context variable in CEL context.
	CELRequestVariable = "request"

	// CELSubjectTokenVariable is the name of the subject_token variable in CEL context.
	CELSubjectTokenVariable = "subject_token"
)

// JWT Validation Configuration
const (
	// DefaultClockSkewTolerance is the default clock skew tolerance in seconds.
	// Allows for minor differences between system clocks during JWT validation.
	DefaultClockSkewTolerance = int64(60)

	// MaxClockSkewTolerance is the maximum allowed clock skew in seconds.
	// Prevents accepting tokens that are too far outside their validity period.
	MaxClockSkewTolerance = int64(300)

	// DefaultBrokerAudience is the default audience value the broker expects in subject_token
	// and client_assertion JWT audience (aud) claims.
	// Configurable via token_exchange.expected_audience in the application config.
	DefaultBrokerAudience = "token-exchange-broker"
)

// Resource URI Validation
const (
	// AllowedResourceSchemeHTTP is HTTP scheme for resource URIs (development only).
	AllowedResourceSchemeHTTP = "http"

	// AllowedResourceSchemeHTTPS is HTTPS scheme for resource URIs (production).
	AllowedResourceSchemeHTTPS = "https"
)
