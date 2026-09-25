// Package tokenexchange provides domain types and errors for RFC 8693 OAuth 2.0 Token Exchange.
package tokenexchange

import (
	"encoding/json"
	"fmt"
)

// TokenExchangeResponse represents an RFC 8693 token exchange response.
// It is an immutable value object that holds the result of a successful token exchange.
//
// Per RFC 8693 Section 2.2, a successful response contains:
// - access_token: REQUIRED - the requested access token for the target resource
// - token_type: REQUIRED - the type of token (typically "Bearer")
// - issued_token_type: REQUIRED - the type of token being returned
// - expires_in: RECOMMENDED - token lifetime in seconds
//
// Domain invariants:
// - AccessToken must not be empty
// - TokenType must be "Bearer" (per RFC 6750)
// - IssuedTokenType must be a valid token type constant
// - ExpiresIn should be positive if set (> 0)
//
// The response is serializable to JSON for HTTP transmission.
// Token values are NOT included in String() representations (per SR-005).
type TokenExchangeResponse struct {
	// AccessToken is the requested access token for the target resource.
	// REQUIRED in RFC 8693 response.
	// This is the token the privileged client/agent will use to access the resource.
	// Must not be empty.
	AccessToken string `json:"access_token"`

	// TokenType indicates how the access token should be used (per RFC 6750).
	// REQUIRED in RFC 8693 response.
	// Must be "Bearer" for Bearer tokens (the standard OAuth2 token type).
	TokenType string `json:"token_type"`

	// IssuedTokenType indicates the type/format of the access token.
	// REQUIRED in RFC 8693 response (Section 2.2).
	// Typical values: "urn:ietf:params:oauth:token-type:access_token"
	// May also indicate other token types depending on the target service.
	IssuedTokenType string `json:"issued_token_type"`

	// ExpiresIn is the access token's lifetime in seconds.
	// RECOMMENDED in RFC 8693 response.
	// If present, indicates when the token will expire.
	// Value of 0 means not set or token does not expire.
	ExpiresIn int64 `json:"expires_in,omitempty"`

	// RefreshToken is an optional refresh token for obtaining new access tokens.
	// Optional in RFC 8693 - only included if the target service provides it.
	// If present, the agent can use this to refresh the access_token when it expires.
	RefreshToken string `json:"refresh_token,omitempty"`

	// Scope is the actual scope granted by the target service.
	// Optional in RFC 8693 - only included if different from requested scope.
	// Space-separated list of scope values.
	Scope string `json:"scope,omitempty"`

	// GrantedPermissionSets maps permission set IDs to their included service IDs (Phase 6 - US4).
	// Optional - included in the response per FR-012 for grant provenance.
	// Keys are permission set UUID strings, values are arrays of service UUID strings.
	GrantedPermissionSets map[string][]string `json:"granted_permission_sets,omitempty"`

	Principal string `json:"principal,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
}

// NewTokenExchangeResponse creates a new token exchange response.
// All parameters are required for a minimal valid response.
// Optional fields (RefreshToken, Scope, ExpiresIn) can be set to zero/empty values.
func NewTokenExchangeResponse(accessToken, tokenType, issuedTokenType string) *TokenExchangeResponse {
	return &TokenExchangeResponse{
		AccessToken:     accessToken,
		TokenType:       tokenType,
		IssuedTokenType: issuedTokenType,
		ExpiresIn:       0, // Not set
		RefreshToken:    "",
		Scope:           "",
	}
}

// NewTokenExchangeResponseWithExpiry creates a token exchange response with expiration time.
func NewTokenExchangeResponseWithExpiry(accessToken, tokenType, issuedTokenType string, expiresIn int64) *TokenExchangeResponse {
	return &TokenExchangeResponse{
		AccessToken:     accessToken,
		TokenType:       tokenType,
		IssuedTokenType: issuedTokenType,
		ExpiresIn:       expiresIn,
		RefreshToken:    "",
		Scope:           "",
	}
}

// NewTokenExchangeResponseFull creates a token exchange response with all optional fields.
func NewTokenExchangeResponseFull(accessToken, tokenType, issuedTokenType, refreshToken, scope string, expiresIn int64) *TokenExchangeResponse {
	return &TokenExchangeResponse{
		AccessToken:     accessToken,
		TokenType:       tokenType,
		IssuedTokenType: issuedTokenType,
		ExpiresIn:       expiresIn,
		RefreshToken:    refreshToken,
		Scope:           scope,
	}
}

// Validate checks if the token exchange response is valid.
// Returns error if required fields are empty.
//
// Validation rules:
// 1. AccessToken must not be empty (REQUIRED)
// 2. TokenType must not be empty and should be "Bearer" (REQUIRED)
// 3. IssuedTokenType must not be empty (REQUIRED per RFC 8693)
// 4. ExpiresIn should be positive if set (> 0), or 0 for unset
func (resp *TokenExchangeResponse) Validate() error {
	if resp.AccessToken == "" {
		return fmt.Errorf("access_token is required")
	}

	if resp.TokenType == "" {
		return fmt.Errorf("token_type is required")
	}

	if resp.IssuedTokenType == "" {
		return fmt.Errorf("issued_token_type is required")
	}

	// ExpiresIn validation: if set (non-zero), should be positive
	if resp.ExpiresIn < 0 {
		return fmt.Errorf("expires_in must be non-negative")
	}

	return nil
}

// ToJSON serializes the response to JSON bytes suitable for HTTP response body.
// Implements RFC 8693 Section 2.2 JSON response format.
// Per SR-005, token values are present in JSON (required for client use)
// but not in logging/String representations.
func (resp *TokenExchangeResponse) ToJSON() ([]byte, error) {
	return json.Marshal(resp) // #nosec G117 -- RFC 8693 token response is serialized for its direct HTTP response, not logging.
}

// String returns a string representation suitable for logging.
// Does NOT include token values to prevent accidental exposure (per SR-005).
// Shows only parameter names and basic metadata.
func (resp *TokenExchangeResponse) String() string {
	return fmt.Sprintf("TokenExchangeResponse{tokenType=%q, issuedTokenType=%q, expiresIn=%d, hasRefreshToken=%v, hasScope=%v}",
		resp.TokenType,
		resp.IssuedTokenType,
		resp.ExpiresIn,
		resp.RefreshToken != "",
		resp.Scope != "",
	)
}

// WithRefreshToken returns a new response with RefreshToken set.
// Useful for building responses incrementally.
func (resp *TokenExchangeResponse) WithRefreshToken(refreshToken string) *TokenExchangeResponse {
	newResp := *resp
	newResp.RefreshToken = refreshToken
	return &newResp
}

// WithScope returns a new response with Scope set.
// Useful for building responses incrementally.
func (resp *TokenExchangeResponse) WithScope(scope string) *TokenExchangeResponse {
	newResp := *resp
	newResp.Scope = scope
	return &newResp
}

// WithExpiresIn returns a new response with ExpiresIn set.
// Useful for building responses incrementally.
func (resp *TokenExchangeResponse) WithExpiresIn(expiresIn int64) *TokenExchangeResponse {
	newResp := *resp
	newResp.ExpiresIn = expiresIn
	return &newResp
}

// IsExpired returns true if the token's expiration time has been reached.
// Compares ExpiresIn with current time (stub for now).
// Per RFC 8693, a token is expired when ExpiresIn seconds have passed.
// Note: This is a simple check - real implementations should compare with current time.
func (resp *TokenExchangeResponse) IsExpired() bool {
	// ExpiresIn == 0 means token doesn't expire or expiration not specified
	return resp.ExpiresIn < 0
}

// HasRefreshToken returns true if a refresh token is available.
func (resp *TokenExchangeResponse) HasRefreshToken() bool {
	return resp.RefreshToken != ""
}

// HasScope returns true if scope information is included.
func (resp *TokenExchangeResponse) HasScope() bool {
	return resp.Scope != ""
}

// GetExpirationSeconds returns the token expiration time in seconds.
// Returns 0 if token doesn't expire.
func (resp *TokenExchangeResponse) GetExpirationSeconds() int64 {
	if resp.ExpiresIn < 0 {
		return 0
	}
	return resp.ExpiresIn
}
