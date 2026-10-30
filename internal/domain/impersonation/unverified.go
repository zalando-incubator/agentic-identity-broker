package impersonation

import (
	"fmt"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtclaims"
)

// parseUnverifiedSubject reads the claims of an unsigned (alg:none) subject JWT without verifying
// or requiring a signature, and without resolving it against any trusted issuer (FR-003d, ADR 20260825-120000).
// It is reachable only when the matching rule declares verification: none.
func parseUnverifiedSubject(tokenString string) (map[string]interface{}, error) {
	sig, err := soleSignature(tokenString)
	if err != nil {
		return nil, fmt.Errorf("cannot read unverified subject: %w", err)
	}
	if len(sig.Signature()) != 0 {
		return nil, fmt.Errorf("unverified subject must be unsigned with an empty signature segment")
	}
	if alg, ok := sig.ProtectedHeaders().Algorithm(); !ok || alg != jwa.NoSignature() {
		return nil, fmt.Errorf("unverified subject must use alg:none, got %q", alg)
	}
	token, err := jwt.ParseString(tokenString, jwt.WithVerify(false), jwt.WithValidate(false))
	if err != nil {
		return nil, fmt.Errorf("cannot parse unverified subject claims: %w", err)
	}
	return jwtclaims.FromToken(token), nil
}
