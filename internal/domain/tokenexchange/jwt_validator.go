// Package tokenexchange provides domain types and services for RFC 8693 OAuth 2.0 Token Exchange.
package tokenexchange

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
)

// JWKSProvider defines the interface for JSON Web Key Set (JWKS) operations.
// This interface is defined locally in the domain to avoid circular imports with the ports package.
// Any object implementing GetKeySet and GetKey methods can be used (structural typing in Go).
//
// Typically implemented by: internal/adapters/jwks/adapter.Adapter (JWKSPort)
type JWKSProvider interface {
	// GetKeySet returns the cached JWKS key set from upstream OAuth2 server.
	GetKeySet(ctx context.Context) (jwk.Set, error)

	// GetKey retrieves a specific key from the JWKS by key ID (kid).
	GetKey(ctx context.Context, kid string) (jwk.Key, error)
}

// JWTValidator provides JWT validation for token exchange.
// It validates token signatures, issuer, audience, and expiration using the JWKSProvider adapter.
//
// This service implements claim validation using the lestrrat-go/jwx/v4 library (Principle III - Library-First Security).
// No custom cryptography is used; all validation relies on vetted libraries.
//
// The validator is stateless and thread-safe for concurrent calls.
type JWTValidator struct {
	subjectTokenPolicy    JWTValidationPolicy
	clientAssertionPolicy JWTValidationPolicy
	brokerAudience        string
	clockSkewSeconds      int64
}

// JWTValidationPolicy defines the key source and accepted issuer set for one JWT role.
type JWTValidationPolicy struct {
	JWKSProvider    JWKSProvider
	ExpectedIssuers []string
}

// NewJWTValidator creates a new JWT validator.
//
// Parameters:
//   - jwksProvider: Provider for fetching JWKS from upstream server (e.g., JWKSAdapter)
//     Can be any object implementing the JWKSProvider interface
//   - expectedIssuer: The expected issuer in token claims (e.g., "https://auth.example.com")
//   - brokerAudience: The broker's identifier that should be in token audience claims
//   - clockSkewSeconds: Clock skew tolerance in seconds for expiration checking (e.g., 60)
//     Must be between 0 and MaxClockSkewTolerance (300)
//
// Returns error if parameters are invalid.
func NewJWTValidator(
	jwksProvider JWKSProvider,
	expectedIssuer string,
	brokerAudience string,
	clockSkewSeconds int64,
) (*JWTValidator, error) {
	policy := JWTValidationPolicy{
		JWKSProvider:    jwksProvider,
		ExpectedIssuers: []string{expectedIssuer},
	}
	return NewJWTValidatorWithPolicies(policy, policy, brokerAudience, clockSkewSeconds)
}

// NewJWTValidatorWithPolicies creates a JWT validator with distinct trust policies for
// subject tokens and client assertions.
func NewJWTValidatorWithPolicies(
	subjectTokenPolicy JWTValidationPolicy,
	clientAssertionPolicy JWTValidationPolicy,
	brokerAudience string,
	clockSkewSeconds int64,
) (*JWTValidator, error) {
	normalizedSubjectPolicy, err := normalizeValidationPolicy("subject_token", subjectTokenPolicy)
	if err != nil {
		return nil, NewServerErrorWithCause("subject token validation policy is invalid", err).
			WithDiagnostic(NewDiagnostic(StageSubjectValidation, DetailJWTConfiguration))
	}
	normalizedClientAssertionPolicy, err := normalizeValidationPolicy("client_assertion", clientAssertionPolicy)
	if err != nil {
		return nil, NewServerErrorWithCause("client assertion validation policy is invalid", err).
			WithDiagnostic(NewDiagnostic(StageClientValidation, DetailJWTConfiguration))
	}
	if brokerAudience == "" {
		return nil, NewServerError("JWT validation audience is not configured").
			WithDiagnostic(NewDiagnostic(StageSubjectValidation, DetailJWTConfiguration))
	}
	if clockSkewSeconds < 0 {
		return nil, NewServerError("JWT validation clock skew cannot be negative").
			WithDiagnostic(NewDiagnostic(StageSubjectValidation, DetailJWTConfiguration))
	}
	if clockSkewSeconds > MaxClockSkewTolerance {
		return nil, NewServerError("JWT validation clock skew exceeds the maximum tolerance").
			WithDiagnostic(NewDiagnostic(StageSubjectValidation, DetailJWTConfiguration))
	}

	return &JWTValidator{
		subjectTokenPolicy:    normalizedSubjectPolicy,
		clientAssertionPolicy: normalizedClientAssertionPolicy,
		brokerAudience:        brokerAudience,
		clockSkewSeconds:      clockSkewSeconds,
	}, nil
}

// ValidateSubjectToken validates a subject_token JWT from RFC 8693 token exchange request.
//
// Validation steps:
// 1. Fetch JWKS from upstream server via JWKSProvider
// 2. Parse JWT with comprehensive validation using library options:
//   - Signature verification (validates structure and signature)
//   - Issuer verification (matches configured upstream_oauth2.issuer)
//   - Audience verification (includes broker identifier)
//   - Expiration verification (with configurable clock skew tolerance)
//
// Per spec SR-006: Validation failure always denies the request.
// Error descriptions and diagnostics do not expose claims, JOSE headers, or configured URLs.
//
// Returns:
//   - The parsed and validated JWT token on success
//   - InvalidGrantError if the supplied credential fails parsing, signature, or claim validation
//   - InvalidRequestError if the required subject token is absent
//   - ServerError if JWKS fetch fails
func (v *JWTValidator) ValidateSubjectToken(ctx context.Context, tokenString string) (jwt.Token, error) {
	if tokenString == "" {
		return nil, NewInvalidRequestError("subject_token is required").
			WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailRequestMalformed))
	}

	// Fetch JWKS and get the specific key
	keyset, err := v.subjectTokenPolicy.JWKSProvider.GetKeySet(ctx)
	if err != nil {
		detail := DetailJWKSUnavailable
		if callerErr := ctx.Err(); callerErr != nil {
			detail = DetailCallerCanceled
			if !errors.Is(err, callerErr) {
				err = errors.Join(err, callerErr)
			}
		}
		return nil, NewServerErrorWithCause("failed to fetch JWKS for token validation", err).
			WithDiagnostic(NewDiagnostic(StageSubjectValidation, detail))
	}

	token, err := v.parseWithPolicy(tokenString, keyset, v.subjectTokenPolicy)
	if err != nil {
		return nil, v.mapParseError(err)
	}

	return token, nil
}

// ValidateClientAssertion validates a client_assertion JWT from RFC 8693 token exchange request.
//
// Similar to ValidateSubjectToken but used for client authentication (RFC 7523).
//
// Validation steps:
// 1. Fetch JWKS from upstream server
// 2. Parse JWT with comprehensive validation using library options:
//   - Signature verification (validates structure and signature)
//   - Issuer verification (matches configured upstream_oauth2.issuer)
//   - Audience verification (includes broker identifier)
//   - Expiration verification (with configurable clock skew tolerance)
//
// Returns:
//   - The parsed and validated JWT token on success
//   - InvalidRequestError if the required client assertion is absent
//   - InvalidClientError if signature validation or claims verification fails
//   - ServerError if JWKS fetch fails
func (v *JWTValidator) ValidateClientAssertion(ctx context.Context, tokenString string) (jwt.Token, error) {
	if tokenString == "" {
		return nil, NewInvalidRequestError("client_assertion is required").
			WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailRequestMalformed))
	}

	// Fetch JWKS and get the specific key
	keyset, err := v.clientAssertionPolicy.JWKSProvider.GetKeySet(ctx)
	if err != nil {
		detail := DetailJWKSUnavailable
		if callerErr := ctx.Err(); callerErr != nil {
			detail = DetailCallerCanceled
			if !errors.Is(err, callerErr) {
				err = errors.Join(err, callerErr)
			}
		}
		return nil, NewServerErrorWithCause("failed to fetch JWKS for client assertion validation", err).
			WithDiagnostic(NewDiagnostic(StageClientValidation, detail))
	}

	token, err := v.parseWithPolicy(tokenString, keyset, v.clientAssertionPolicy)
	if err != nil {
		return nil, v.mapClientAssertionParseError(err)
	}

	return token, nil
}

// mapParseError classifies rejection of a supplied subject credential independently
// of malformed OAuth request parameters. Library causes remain inspectable, not serialized.
func (v *JWTValidator) mapParseError(err error) error {
	if err == nil {
		return nil
	}
	detail := DetailSubjectInvalid
	if errors.Is(err, jwt.TokenExpiredError{}) {
		detail = DetailSubjectExpired
	}
	return NewInvalidGrantError("subject_token validation failed").WithCause(err).
		WithDiagnostic(NewDiagnostic(StageSubjectValidation, detail))
}

// mapClientAssertionParseError classifies rejection of a supplied client credential.
func (v *JWTValidator) mapClientAssertionParseError(err error) error {
	if err == nil {
		return nil
	}
	detail := DetailClientInvalid
	if errors.Is(err, jwt.TokenExpiredError{}) {
		detail = DetailClientExpired
	}
	return NewInvalidClientError("client_assertion validation failed").WithCause(err).
		WithDiagnostic(NewDiagnostic(StageClientValidation, detail))
}

func (v *JWTValidator) parseWithPolicy(tokenString string, keyset jwk.Set, policy JWTValidationPolicy) (jwt.Token, error) {
	clockSkew := time.Duration(v.clockSkewSeconds) * time.Second
	token, err := jwt.ParseString(
		tokenString,
		jwt.WithVerify(true),
		jwt.WithKeySet(keyset),
		jwt.WithValidate(false),
	)
	if err != nil {
		return nil, err
	}

	var (
		firstErr     error
		preferredErr error
	)
	for _, issuer := range policy.ExpectedIssuers {
		err := jwt.Validate(
			token,
			jwt.WithIssuer(issuer),
			jwt.WithAudience(v.brokerAudience),
			jwt.WithAcceptableSkew(clockSkew),
		)
		if err == nil {
			return token, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if !errors.Is(err, jwt.InvalidIssuerError{}) {
			preferredErr = err
		}
	}

	if preferredErr != nil {
		return nil, preferredErr
	}
	return nil, firstErr
}

func normalizeValidationPolicy(name string, policy JWTValidationPolicy) (JWTValidationPolicy, error) {
	if policy.JWKSProvider == nil {
		return JWTValidationPolicy{}, fmt.Errorf("%s jwks_provider cannot be nil", name)
	}

	issuers := make([]string, 0, len(policy.ExpectedIssuers))
	for _, issuer := range policy.ExpectedIssuers {
		if issuer == "" {
			return JWTValidationPolicy{}, fmt.Errorf("%s expected_issuer cannot be empty", name)
		}
		if !slices.Contains(issuers, issuer) {
			issuers = append(issuers, issuer)
		}
	}
	if len(issuers) == 0 {
		return JWTValidationPolicy{}, fmt.Errorf("%s expected_issuer cannot be empty", name)
	}

	return JWTValidationPolicy{
		JWKSProvider:    policy.JWKSProvider,
		ExpectedIssuers: issuers,
	}, nil
}
