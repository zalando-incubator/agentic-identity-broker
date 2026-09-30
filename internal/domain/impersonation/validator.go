package impersonation

import (
	"context"
	"fmt"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtclaims"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// defaultClockSkew is the acceptable clock skew for impersonation credential validation.
const defaultClockSkew = 60 * time.Second

// signedValidator validates a signed impersonation credential (client assertion, actor token,
// or signed subject token) against one trusted issuer, enforcing an explicit per-issuer
// asymmetric algorithm allow-list (CR-007) in addition to signature, issuer, audience, expiry,
// and not-before checks (FR-004).
type signedValidator struct {
	jwksProvider      tokenexchange.JWKSProvider
	issuerURI         string
	allowedAlgorithms map[jwa.SignatureAlgorithm]struct{}
	clockSkew         time.Duration
	cacheByProvider   bool
}

// verifiedCredentialCache is local to one Service.Impersonate call and shared only across its rules.
// Equal keys in later calls use a new cache.
type verifiedCredentialCache struct {
	verified map[verificationCacheKey]verificationResult
}

type verificationCacheKey struct {
	role     ports.CredentialRole
	token    string
	provider tokenexchange.JWKSProvider
}

type verificationResult struct {
	token jwt.Token
	err   error
}

func (c *verifiedCredentialCache) verify(ctx context.Context, role ports.CredentialRole, tokenString string, issuer *compiledIssuer) (jwt.Token, error) {
	if !issuer.validator.cacheByProvider {
		return issuer.validator.verify(ctx, tokenString)
	}
	key := verificationCacheKey{
		role:     role,
		token:    tokenString,
		provider: issuer.validator.jwksProvider,
	}
	if result, ok := c.verified[key]; ok {
		return result.token, result.err
	}

	token, err := issuer.validator.verify(ctx, tokenString)
	if c.verified == nil {
		c.verified = make(map[verificationCacheKey]verificationResult)
	}
	if err != nil {
		c.verified[key] = verificationResult{err: err}
		return nil, err
	}
	c.verified[key] = verificationResult{token: token}
	return token, nil
}

// validate verifies the token against the issuer's keys and the expected audience, returning
// the validated claims. It rejects any algorithm not in the broker-approved asymmetric set or
// not explicitly allowed for the issuer BEFORE attempting signature verification (CR-007), so a
// symmetric or none algorithm can never be accepted for a signed role (FR-005).
func (v *signedValidator) validate(ctx context.Context, tokenString, expectedAudience string) (map[string]interface{}, error) {
	return v.validateCredential(ctx, tokenString, expectedAudience, false)
}

// validateWithoutAudience verifies the token and rejects any credential that carries an aud claim (CR-009).
func (v *signedValidator) validateWithoutAudience(ctx context.Context, tokenString string) (map[string]interface{}, error) {
	return v.validateCredential(ctx, tokenString, "", true)
}

func (v *signedValidator) validateCredential(ctx context.Context, tokenString, expectedAudience string, requireAbsentAudience bool) (map[string]interface{}, error) {
	if err := v.validateAlgorithm(tokenString); err != nil {
		return nil, err
	}

	token, err := v.verify(ctx, tokenString)
	if err != nil {
		return nil, err
	}
	return v.validateClaims(token, expectedAudience, requireAbsentAudience)
}

func (v *signedValidator) validateAlgorithm(tokenString string) error {
	alg, err := protectedHeaderAlgorithm(tokenString)
	if err != nil {
		return fmt.Errorf("cannot read token algorithm: %w", err)
	}
	if !isApprovedAlgorithm(alg) {
		return fmt.Errorf("algorithm %q is not an approved asymmetric algorithm", alg)
	}
	if _, ok := v.allowedAlgorithms[alg]; !ok {
		return fmt.Errorf("algorithm %q is not permitted for issuer", alg)
	}
	return nil
}

func (v *signedValidator) verify(ctx context.Context, tokenString string) (jwt.Token, error) {
	keyset, err := v.jwksProvider.GetKeySet(ctx)
	if err != nil {
		return nil, fmt.Errorf("jwks unavailable for issuer: %w", err)
	}
	return jwt.ParseString(
		tokenString,
		jwt.WithVerify(true),
		jwt.WithKeySet(keyset),
		jwt.WithValidate(false),
	)
}

func (v *signedValidator) validateClaims(token jwt.Token, expectedAudience string, requireAbsentAudience bool) (map[string]interface{}, error) {
	var err error
	if requireAbsentAudience {
		err = jwt.Validate(
			token,
			jwt.WithRequiredClaim(jwt.ExpirationKey),
			jwt.WithIssuer(v.issuerURI),
			jwt.WithAcceptableSkew(v.clockSkew),
		)
	} else {
		err = jwt.Validate(
			token,
			jwt.WithRequiredClaim(jwt.ExpirationKey),
			jwt.WithIssuer(v.issuerURI),
			jwt.WithAudience(expectedAudience),
			jwt.WithAcceptableSkew(v.clockSkew),
		)
	}
	if err != nil {
		return nil, err
	}
	if requireAbsentAudience {
		if _, ok := token.Audience(); ok {
			return nil, fmt.Errorf("token must not contain an audience claim")
		}
	}
	return jwtclaims.FromToken(token), nil
}

// soleSignature parses a compact JWS and returns its single signature without verifying it, so
// the protected-header algorithm (and, for the unverified subject, the empty signature segment)
// can be inspected before any trust decision.
func soleSignature(tokenString string) (*jws.Signature, error) {
	msg, err := jws.Parse([]byte(tokenString))
	if err != nil {
		return nil, fmt.Errorf("not a parseable compact JWS: %w", err)
	}
	sigs := msg.Signatures()
	if len(sigs) != 1 {
		return nil, fmt.Errorf("compact JWT must carry exactly one signature segment")
	}
	return sigs[0], nil
}

// protectedHeaderAlgorithm returns the typed alg parameter from a compact JWT's protected header,
// without verifying the signature, so the per-issuer allow-list can be enforced against the actual
// header value (CR-007).
func protectedHeaderAlgorithm(tokenString string) (jwa.SignatureAlgorithm, error) {
	sig, err := soleSignature(tokenString)
	if err != nil {
		return jwa.EmptySignatureAlgorithm(), err
	}
	alg, ok := sig.ProtectedHeaders().Algorithm()
	if !ok {
		return jwa.EmptySignatureAlgorithm(), fmt.Errorf("protected header has no alg")
	}
	return alg, nil
}
