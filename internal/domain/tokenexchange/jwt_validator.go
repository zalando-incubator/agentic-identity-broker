// Package tokenexchange provides domain types and services for RFC 8693 OAuth 2.0 Token Exchange.
package tokenexchange

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
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
		return nil, err
	}
	normalizedClientAssertionPolicy, err := normalizeValidationPolicy("client_assertion", clientAssertionPolicy)
	if err != nil {
		return nil, err
	}
	if brokerAudience == "" {
		return nil, fmt.Errorf("expected_audience cannot be empty")
	}
	if clockSkewSeconds < 0 {
		return nil, fmt.Errorf("clock_skew_seconds cannot be negative")
	}
	if clockSkewSeconds > MaxClockSkewTolerance {
		return nil, fmt.Errorf("clock_skew_seconds (%d) cannot exceed max tolerance (%d)", clockSkewSeconds, MaxClockSkewTolerance)
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
// Per spec SR-005: Error messages do NOT expose token content (only metadata like issuer).
//
// Returns:
//   - The parsed and validated JWT token on success
//   - InvalidClientError if signature validation fails (per spec SR-001)
//   - InvalidGrantError if token is expired or malformed
//   - InvalidGrantError if issuer/audience verification fails
//   - ServerError if JWKS fetch fails
func (v *JWTValidator) ValidateSubjectToken(ctx context.Context, tokenString string) (jwt.Token, error) {
	if tokenString == "" {
		return nil, NewInvalidGrantError("subject_token is empty or missing")
	}

	// Fetch JWKS and get the specific key
	keyset, err := v.subjectTokenPolicy.JWKSProvider.GetKeySet(ctx)
	if err != nil {
		return nil, NewServerErrorWithCause("failed to fetch JWKS for token validation", err)
	}

	token, err := v.parseWithPolicy(tokenString, keyset, v.subjectTokenPolicy)
	if err != nil {
		return nil, v.mapParseError(err, "subject_token", tokenString)
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
//   - InvalidClientError if signature validation or claims verification fails
//   - ServerError if JWKS fetch fails
func (v *JWTValidator) ValidateClientAssertion(ctx context.Context, tokenString string) (jwt.Token, error) {
	if tokenString == "" {
		return nil, NewInvalidClientError("client_assertion is empty or missing")
	}

	// Fetch JWKS and get the specific key
	keyset, err := v.clientAssertionPolicy.JWKSProvider.GetKeySet(ctx)
	if err != nil {
		return nil, NewServerErrorWithCause("failed to fetch JWKS for client_assertion validation", err)
	}

	token, err := v.parseWithPolicy(tokenString, keyset, v.clientAssertionPolicy)
	if err != nil {
		return nil, v.mapClientAssertionParseError(err, tokenString)
	}

	return token, nil
}

// mapParseError maps JWT parsing and validation errors to appropriate domain errors for subject tokens.
//
// parseWithPolicy returns parsing or signature errors from jwt.ParseString and claim validation
// errors from jwt.Validate. Specific validation sentinels (InvalidAudienceError, InvalidIssuerError,
// etc.) are reachable through the error chain via Unwrap(). Therefore, specific checks MUST
// appear before the generic ParseError check to avoid misclassifying validation failures as
// parse/signature errors.
//
// Per spec SR-005: Error messages do NOT expose token content (only metadata like issuer).
// The underlying library error is attached as a cause for internal logging only.
// Diagnostic details (expected vs actual claim values, sub claim) are included in the
// details field for structured logging and OTel error events to aid troubleshooting.
func (v *JWTValidator) mapParseError(err error, tokenType string, tokenString string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, jwt.InvalidIssuerError{}):
		return NewInvalidGrantError(
			fmt.Sprintf("%s issuer validation failed: expected %s", tokenType, formatExpectedIssuers(v.subjectTokenPolicy.ExpectedIssuers)),
		).WithCause(err).WithDetails(v.extractDiagnostics(tokenString, v.subjectTokenPolicy.ExpectedIssuers))
	case errors.Is(err, jwt.InvalidAudienceError{}):
		return NewInvalidGrantError(
			fmt.Sprintf("%s audience validation failed: expected aud=%q", tokenType, v.brokerAudience),
		).WithCause(err).WithDetails(v.extractDiagnostics(tokenString, v.subjectTokenPolicy.ExpectedIssuers))
	case errors.Is(err, jwt.TokenExpiredError{}):
		return NewInvalidGrantError(tokenType + " has expired").WithCause(err).WithDetails(v.extractDiagnostics(tokenString, v.subjectTokenPolicy.ExpectedIssuers))
	case errors.Is(err, jwt.TokenNotYetValidError{}):
		return NewInvalidGrantError(tokenType + " is not yet valid (nbf)").WithCause(err).WithDetails(v.extractDiagnostics(tokenString, v.subjectTokenPolicy.ExpectedIssuers))
	case errors.Is(err, jwt.ParseError{}):
		return NewInvalidRequestError(tokenType + " is malformed or signature verification failed").WithCause(err).WithDetails(diagnoseTokenShape(tokenString))
	default:
		return NewInvalidGrantError(tokenType + " validation failed").WithCause(err)
	}
}

// mapClientAssertionParseError maps JWT parsing and validation errors to InvalidClientError for client assertions.
//
// Client assertion failures always result in InvalidClientError per RFC 7523.
// Specific validation sentinels MUST be checked before ParseError (see mapParseError comment).
// The underlying library error is attached as a cause for internal logging only.
// Diagnostic details (expected vs actual claim values) are included in the
// details field for structured logging and OTel error events to aid troubleshooting.
func (v *JWTValidator) mapClientAssertionParseError(err error, tokenString string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, jwt.InvalidIssuerError{}):
		return NewInvalidClientError(
			fmt.Sprintf("client_assertion issuer validation failed: expected %s", formatExpectedIssuers(v.clientAssertionPolicy.ExpectedIssuers)),
		).WithCause(err).WithDetails(v.extractDiagnostics(tokenString, v.clientAssertionPolicy.ExpectedIssuers))
	case errors.Is(err, jwt.InvalidAudienceError{}):
		return NewInvalidClientError(
			fmt.Sprintf("client_assertion audience validation failed: expected aud=%q", v.brokerAudience),
		).WithCause(err).WithDetails(v.extractDiagnostics(tokenString, v.clientAssertionPolicy.ExpectedIssuers))
	case errors.Is(err, jwt.TokenExpiredError{}):
		return NewInvalidClientError("client_assertion has expired").WithCause(err).WithDetails(v.extractDiagnostics(tokenString, v.clientAssertionPolicy.ExpectedIssuers))
	case errors.Is(err, jwt.TokenNotYetValidError{}):
		return NewInvalidClientError("client_assertion is not yet valid (nbf)").WithCause(err).WithDetails(v.extractDiagnostics(tokenString, v.clientAssertionPolicy.ExpectedIssuers))
	case errors.Is(err, jwt.ParseError{}):
		return NewInvalidClientError("client_assertion is malformed or signature verification failed").WithCause(err)
	default:
		return NewInvalidClientError("client_assertion validation failed").WithCause(err)
	}
}

// extractDiagnostics attempts to parse the token without verification to extract
// actual claim values for troubleshooting. Returns a formatted string with expected
// vs actual values for iss, aud, sub, exp, and nbf. If parsing fails, returns an
// empty string. Per SR-005, this never includes the raw JWT string, signature, or
// full payload, but it may include specific claim values extracted from the payload.
func (v *JWTValidator) extractDiagnostics(tokenString string, expectedIssuers []string) string {
	if tokenString == "" {
		return ""
	}
	tok, parseErr := jwt.ParseInsecure([]byte(tokenString))
	if parseErr != nil {
		return ""
	}

	var parts []string

	if sub, _ := tok.Subject(); sub != "" {
		parts = append(parts, fmt.Sprintf("sub=%q", sub))
	}

	actualIss, _ := tok.Issuer()
	parts = append(parts, fmt.Sprintf("expected_iss=%s actual_iss=%q", formatExpectedIssuersForDiagnostics(expectedIssuers), actualIss))

	actualAud, _ := tok.Audience()
	quotedAud := make([]string, len(actualAud))
	for i, a := range actualAud {
		quotedAud[i] = fmt.Sprintf("%q", a)
	}
	parts = append(parts, fmt.Sprintf("expected_aud=%q actual_aud=[%s]", v.brokerAudience, strings.Join(quotedAud, ",")))

	if exp, _ := tok.Expiration(); !exp.IsZero() {
		parts = append(parts, fmt.Sprintf("exp=%d", exp.Unix()))
	}
	if nbf, _ := tok.NotBefore(); !nbf.IsZero() {
		parts = append(parts, fmt.Sprintf("nbf=%d", nbf.Unix()))
	}

	return fmt.Sprintf("[%s]", strings.Join(parts, " "))
}

// maxHeaderValueLen bounds each surfaced JOSE header value so a hostile or oversized
// header cannot bloat structured logs.
const maxHeaderValueLen = 64

// diagnoseTokenShape produces non-reversible structural diagnostics for a token that
// failed compact/JSON parsing (the jwt.ParseError branch). Per SR-005 it never emits
// raw token bytes, the payload, or the signature. It reports only:
//   - overall length and dot/segment counts
//   - a coarse classification of the first byte (never the byte itself)
//   - per-segment base64url decodability and decoded byte length
//   - allowlisted JOSE header metadata (alg/enc/typ/cty/kid) decoded from the
//     protected header segment of a compact JWS (3 segments) or JWE (5 segments)
//
// This is enough to distinguish an encrypted JWE, a signed-but-unverifiable JWS, an
// opaque token, and a misrouted JSON document without exposing credential material.
func diagnoseTokenShape(tokenString string) string {
	if tokenString == "" {
		return ""
	}

	segments := strings.Split(tokenString, ".")
	parts := []string{
		fmt.Sprintf("len=%d", len(tokenString)),
		fmt.Sprintf("dot_count=%d", len(segments)-1),
		fmt.Sprintf("segments=%d", len(segments)),
		"first_byte_class=" + classifyFirstByte(tokenString),
	}

	b64 := make([]string, len(segments))
	for i, seg := range segments {
		if decoded, err := base64.RawURLEncoding.DecodeString(seg); err == nil {
			b64[i] = fmt.Sprintf("ok(%d)", len(decoded))
		} else {
			b64[i] = "bad"
		}
	}
	parts = append(parts, fmt.Sprintf("seg_b64url=[%s]", strings.Join(b64, ",")))

	// Compact JWS (3 segments) and JWE (5 segments) both carry the protected JOSE
	// header in segment 0; surface allowlisted metadata to identify the algorithm.
	if len(segments) == 3 || len(segments) == 5 {
		if header := decodeJOSEHeader(segments[0]); header != "" {
			parts = append(parts, "header="+header)
		}
	}

	return "[" + strings.Join(parts, " ") + "]"
}

// classifyFirstByte returns a coarse, non-reversible class of the leading byte so
// logs can distinguish a JSON document, a base64url compact token, and garbage
// without ever recording the byte value itself.
func classifyFirstByte(s string) string {
	if s == "" {
		return "empty"
	}
	switch c := s[0]; {
	case c == '{':
		return "json_open_brace"
	case c == ' ' || c == '\t' || c == '\n' || c == '\r':
		return "whitespace"
	case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_':
		return "base64url"
	default:
		return "other"
	}
}

// decodeJOSEHeader base64url-decodes a compact-serialization protected header segment
// and returns allowlisted metadata fields only. Returns an empty string when the
// segment is not base64url-encoded JSON or carries none of the allowlisted fields.
// The payload, signature, and any non-allowlisted header parameters are never emitted.
func decodeJOSEHeader(segment string) string {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return ""
	}
	var hdr struct {
		Alg string `json:"alg"`
		Enc string `json:"enc"`
		Typ string `json:"typ"`
		Cty string `json:"cty"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(raw, &hdr); err != nil {
		return ""
	}

	var fields []string
	for _, f := range []struct{ name, value string }{
		{"alg", hdr.Alg},
		{"enc", hdr.Enc},
		{"typ", hdr.Typ},
		{"cty", hdr.Cty},
		{"kid", hdr.Kid},
	} {
		if f.value != "" {
			fields = append(fields, fmt.Sprintf("%s=%q", f.name, truncateHeaderValue(f.value)))
		}
	}
	if len(fields) == 0 {
		return ""
	}
	return "{" + strings.Join(fields, " ") + "}"
}

func truncateHeaderValue(s string) string {
	if len(s) <= maxHeaderValueLen {
		return s
	}
	return s[:maxHeaderValueLen] + "..."
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

func formatExpectedIssuers(issuers []string) string {
	quoted := quoteStrings(issuers)
	if len(quoted) == 1 {
		return fmt.Sprintf(`iss=%s`, quoted[0])
	}
	return fmt.Sprintf("iss in [%s]", strings.Join(quoted, ","))
}

func formatExpectedIssuersForDiagnostics(issuers []string) string {
	quoted := quoteStrings(issuers)
	if len(quoted) == 1 {
		return quoted[0]
	}
	return fmt.Sprintf("[%s]", strings.Join(quoted, ","))
}

func quoteStrings(values []string) []string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("%q", value)
	}
	return quoted
}
