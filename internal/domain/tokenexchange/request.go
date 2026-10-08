// Package tokenexchange provides domain types and errors for RFC 8693 OAuth 2.0 Token Exchange.
package tokenexchange

import (
	"fmt"
)

// TokenExchangeRequest represents an RFC 8693 token exchange request.
// It is an immutable value object parsed from the application/x-www-form-urlencoded request body.
//
// Per RFC 8693 Section 2.1, a token exchange request contains:
// - grant_type: REQUIRED - must be "urn:ietf:params:oauth:grant-type:token-exchange"
// - subject_token: REQUIRED - the token being exchanged
// - subject_token_type: OPTIONAL - type of subject_token (defaults to AccessTokenType)
// - client_assertion: REQUIRED - JWT authenticating the privileged client
// - client_assertion_type: OPTIONAL - type of client_assertion (defaults to JWTBearerType)
// - resource: REQUIRED per FR-008 - the target resource/service for the exchange
// - scope: OPTIONAL - requested scope (space-separated list)
//
// Domain invariants:
// - GrantType must be "urn:ietf:params:oauth:grant-type:token-exchange" (per FR-048)
// - SubjectToken must not be empty (per FR-008)
// - ClientAssertion must not be empty (per FR-008)
// - Resource must not be empty (per FR-008, FR-049a)
// - SubjectTokenType defaults to AccessTokenType if empty
// - ClientAssertionType defaults to JWTBearerType if empty
//
// Validation (per business rules):
// - All required fields must be present
// - GrantType must match RFC 8693 constant
// - SubjectToken and ClientAssertion must not be empty strings
// - Resource must not be empty
type TokenExchangeRequest struct {
	// GrantType is the RFC 8693 grant type parameter.
	// Must be exactly "urn:ietf:params:oauth:grant-type:token-exchange".
	// Immutable after creation.
	GrantType string

	// SubjectToken is the token being exchanged (usually JWT from upstream OAuth2 server).
	// REQUIRED per FR-008. Must not be empty.
	// This is a raw JWT string (not yet validated).
	// Immutable after creation.
	SubjectToken string

	// SubjectTokenType is the type of subject_token (e.g., AccessTokenType, JWTTokenType).
	// OPTIONAL, defaults to AccessTokenType if empty.
	// Per RFC 8693 Section 3, possible values include:
	// - urn:ietf:params:oauth:token-type:access_token (default)
	// - urn:ietf:params:oauth:token-type:jwt
	// - urn:ietf:params:oauth:token-type:id_token
	// Immutable after creation.
	SubjectTokenType string

	// ClientAssertion is the JWT authenticating the privileged client.
	// REQUIRED per FR-008. Must not be empty.
	// This is a raw JWT string (not yet validated).
	// Immutable after creation.
	ClientAssertion string

	// ClientAssertionType is the type of client_assertion.
	// OPTIONAL, defaults to JWTBearerType if empty.
	// Per RFC 7523, the standard type is "urn:ietf:params:oauth:client-assertion-type:jwt-bearer".
	// Immutable after creation.
	ClientAssertionType string

	// Resource is the target resource or service URI.
	// REQUIRED per FR-008 and FR-049a ("resource parameter is required").
	// Must not be empty and must be a valid URI format.
	// Used to discover which third-party service to exchange tokens for.
	// Immutable after creation.
	Resource string

	// Scope is the requested OAuth2 scope (space-separated list).
	// OPTIONAL - may be empty.
	// Per RFC 8693 Section 2.1, if present specifies which scopes to request.
	// Immutable after creation.
	Scope string
}

// NewTokenExchangeRequest creates a new TokenExchangeRequest from raw parameters.
// All fields are taken as-is without parsing or validation of JWT content.
// Validation of JWT signatures and claims happens in JWT validation layer.
// This constructor is used by HTTP layer to parse form-urlencoded request bodies.
//
// Per FR-049a, this also populates Scope if provided in parameters.
func NewTokenExchangeRequest(grantType, subjectToken, subjectTokenType, clientAssertion, clientAssertionType, resource, scope string) *TokenExchangeRequest {
	req := &TokenExchangeRequest{
		GrantType:           grantType,
		SubjectToken:        subjectToken,
		SubjectTokenType:    subjectTokenType,
		ClientAssertion:     clientAssertion,
		ClientAssertionType: clientAssertionType,
		Resource:            resource,
		Scope:               scope,
	}

	// Set defaults for optional fields
	if req.SubjectTokenType == "" {
		req.SubjectTokenType = AccessTokenType
	}
	if req.ClientAssertionType == "" {
		req.ClientAssertionType = JWTBearerType
	}

	return req
}

// Validate checks if the token exchange request is valid.
// Returns a TokenExchangeError with code and description suitable for RFC 8693 error responses.
//
// Validation rules (business logic):
// 1. GrantType must be exactly "urn:ietf:params:oauth:grant-type:token-exchange" (FR-048)
// 2. SubjectToken must not be empty (FR-008)
// 3. ClientAssertion must not be empty (FR-008)
// 4. Resource must not be empty (FR-008, FR-049a)
// 5. SubjectTokenType and ClientAssertionType are optional with sensible defaults
//
// Returns NewInvalidRequestError for any validation failure.
// Validation does NOT verify JWT signatures or contents - that's handled by JWT validation layer.
func (req *TokenExchangeRequest) Validate() error {
	// Validate grant_type
	if req.GrantType == "" {
		return NewInvalidRequestError("grant_type parameter is required").WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailRequestMalformed))
	}
	if req.GrantType != TokenExchangeGrantType {
		return NewInvalidRequestError("grant_type must identify token exchange").WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailRequestMalformed))
	}

	// Validate subject_token (required, must not be empty)
	if req.SubjectToken == "" {
		return NewInvalidRequestError("subject_token parameter is required").WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailRequestMalformed))
	}

	// Validate client_assertion (required, must not be empty)
	if req.ClientAssertion == "" {
		return NewInvalidRequestError("client_assertion parameter is required").WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailRequestMalformed))
	}

	// Validate resource (required per FR-008, FR-049a)
	if req.Resource == "" {
		return NewInvalidRequestError("resource parameter is required").WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailResourceMissing))
	}
	if _, err := NewResourceURI(req.Resource); err != nil {
		return NewInvalidRequestError("resource parameter is invalid").WithCause(err).WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailRequestMalformed))
	}

	return nil
}

// String returns a string representation of the request suitable for logging.
// Does NOT include token values to prevent accidental exposure (per SR-005).
// Shows only parameter names and counts, never token content.
func (req *TokenExchangeRequest) String() string {
	return fmt.Sprintf("TokenExchangeRequest{grantType=%q, resource=%q, subjectTokenType=%q, clientAssertionType=%q, scope=%q}",
		req.GrantType,
		req.Resource,
		req.SubjectTokenType,
		req.ClientAssertionType,
		req.Scope,
	)
}

// GrantType returns the grant type parameter.
// Used by HTTP layer to detect if this is a token exchange request.
func (req *TokenExchangeRequest) GetGrantType() string {
	return req.GrantType
}

// SubjectTokenType returns the subject token type.
// Returns the explicitly set value or the default if empty.
func (req *TokenExchangeRequest) GetSubjectTokenType() string {
	if req.SubjectTokenType != "" {
		return req.SubjectTokenType
	}
	return AccessTokenType
}

// ClientAssertionType returns the client assertion type.
// Returns the explicitly set value or the default if empty.
func (req *TokenExchangeRequest) GetClientAssertionType() string {
	if req.ClientAssertionType != "" {
		return req.ClientAssertionType
	}
	return JWTBearerType
}

// IsTokenExchangeRequest returns true if this request is a token exchange request.
// Checks if grant_type matches RFC 8693 token exchange grant type.
func (req *TokenExchangeRequest) IsTokenExchangeRequest() bool {
	return req.GrantType == TokenExchangeGrantType
}
