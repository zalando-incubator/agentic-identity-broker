package jwtauth

import "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"

// AuthResult is a value object representing the result of successful JWT authentication.
// Produced by the JWTAuthenticator adapter, consumed by the principal middleware to
// construct a PrincipalProfile and set context values.
//
// Invariants:
//   - Principal must be non-empty (CEL extraction must produce a non-empty string)
//   - Optional fields are nil when the CEL expression is not configured or evaluates to non-string
//   - Claims is never nil (even empty JWT has at least header claims)
type AuthResult struct {
	// Principal is the extracted user identifier from the JWT claims.
	// Extracted via the configured principal CEL expression (default: claims.sub).
	// Must be non-empty — authentication fails if extraction produces empty string.
	Principal id.Principal

	// DisplayName is the extracted display name from JWT claims.
	// Nil if no display_name_expression is configured or if the expression
	// evaluates to a non-string value.
	DisplayName *string

	// Email is the extracted email address from JWT claims.
	// Nil if no email_expression is configured or if the expression
	// evaluates to a non-string value.
	Email *string

	// PictureURL is the extracted profile picture URL from JWT claims.
	// Nil if no picture_url_expression is configured or if the expression
	// evaluates to a non-string value.
	PictureURL *string

	// Claims is the full JWT claims map. Used for audit logging and debugging.
	// Never nil.
	Claims map[string]interface{}
}
