// Package impersonation implements the RFC 8693 user-impersonation flow: a privileged
// client mints a locally issued broker token representing a subject user while attributing
// the acting party via the standard act claim. It is a self-contained bounded context that
// reuses the JWKS adapter, the ADR-009 CEL pattern, and the local issuer's signing policy.
package impersonation

import (
	"github.com/lestrrat-go/jwx/v4/jwa"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
)

// RFC 8693 parameter and type identifiers reused from the token-exchange bounded context so
// impersonation and third-party exchange stay consistent on the shared wire protocol.
const (
	// GrantType is the RFC 8693 token-exchange grant type.
	GrantType = tokenexchange.TokenExchangeGrantType
	// JWTTokenType is the RFC 8693 JWT token-type identifier (signed actor and signed subject).
	JWTTokenType = tokenexchange.JWTTokenType
	// AccessTokenType is the RFC 8693 access-token type identifier (issued_token_type, requested_token_type).
	AccessTokenType = tokenexchange.AccessTokenType
	// JWTBearerType is the RFC 7523 JWT bearer client-assertion type.
	JWTBearerType = tokenexchange.JWTBearerType
	// BearerTokenType is the token_type value returned in the RFC 8693 success response.
	BearerTokenType = tokenexchange.BearerTokenType
)

// isApprovedAlgorithm reports whether alg is in the broker-approved asymmetric set (CR-007).
// none and all symmetric (HS*) algorithms are always rejected; a trusted issuer's
// allowed_algorithms must be a non-empty subset of this set.
func isApprovedAlgorithm(alg jwa.SignatureAlgorithm) bool {
	return trustanchor.IsApprovedAlgorithm(alg.String())
}

// IsApprovedAlgorithm reports whether the named algorithm is approved (CR-007). Config supplies
// algorithm names as strings; an unknown name is not approved.
func IsApprovedAlgorithm(name string) bool {
	alg, ok := jwa.LookupSignatureAlgorithm(name)
	return ok && isApprovedAlgorithm(alg)
}
