// Package jwtauth provides domain types and interfaces for JWT-based pre-authentication.
// This package defines the port interface for JWT authentication, used by the principal
// middleware to authenticate requests via JWTs from upstream reverse proxies or gateways.
package jwtauth

import "context"

// JWTAuthenticator is the domain port interface for JWT authentication.
// It abstracts JWT parsing, signature verification (JWKS or none), temporal validation,
// and CEL-based claim extraction from the middleware layer.
//
// Implementations:
//   - internal/adapters/jwtauth/jwx_authenticator.go (lestrrat-go/jwx v4)
//
// The middleware calls Authenticate with the raw JWT string (prefix stripped).
// On success, returns AuthResult with extracted principal and optional profile attributes.
// On failure, returns a domain error (ErrInvalidSignature, ErrTokenExpired, etc.).
type JWTAuthenticator interface {
	// Authenticate parses, validates, and extracts claims from a raw JWT string.
	// Returns AuthResult on success or an error describing the validation failure.
	//
	// Errors returned:
	//   - ErrInvalidSignature: JWT signature does not match any key in JWKS
	//   - ErrTokenExpired: JWT exp claim is in the past
	//   - ErrAudienceMismatch: JWT aud claim does not contain expected audience
	//   - ErrIssuerMismatch: JWT iss claim does not match expected issuer
	//   - ErrClaimExtraction: CEL expression failed to extract required claims
	//   - ErrMalformedToken: JWT is not a valid JWT format
	//   - ErrMissingExpiry: JWT has no exp claim (expiry is mandatory)
	Authenticate(ctx context.Context, rawJWT string) (*AuthResult, error)
}
