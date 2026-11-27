package model

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// ServiceAccountJWTMaxBytes is the fixed upper bound of a stored compact JWS, checked before any
// parsing.
const ServiceAccountJWTMaxBytes = 16384

// CredentialFormat is the shape of a service account's secret within its kind.
type CredentialFormat string

// CredentialFormatSignedJWT is a JWT signed outside the broker, stored exactly as submitted.
const CredentialFormatSignedJWT CredentialFormat = "signed_jwt"

// CredentialUse states what a stored JWT is.
type CredentialUse string

const (
	// CredentialUseAccessToken is a stored JWT that is handed out as an access token. It is the
	// default when a credential states no use.
	CredentialUseAccessToken CredentialUse = "access_token"
	// CredentialUseAssertion is reserved for a stored JWT used as an assertion at a token
	// endpoint. It is refused until a later feature offers it.
	CredentialUseAssertion CredentialUse = "assertion"
)

// CredentialRejectionReason names the credential rule a write failed.
type CredentialRejectionReason string

const (
	// CredentialRejectionUnsupportedFormat means the format is not signed_jwt.
	CredentialRejectionUnsupportedFormat CredentialRejectionReason = "unsupported_format"
	// CredentialRejectionUnsupportedUse means the use is neither absent nor access_token.
	CredentialRejectionUnsupportedUse CredentialRejectionReason = "unsupported_use"
	// CredentialRejectionMissingJWT means the credential carries no jwt.
	CredentialRejectionMissingJWT CredentialRejectionReason = "missing_jwt"
	// CredentialRejectionTooLarge means the jwt is longer than ServiceAccountJWTMaxBytes.
	CredentialRejectionTooLarge CredentialRejectionReason = "too_large"
	// CredentialRejectionMalformed means the jwt is not a compact JWS with valid JWT claims.
	CredentialRejectionMalformed CredentialRejectionReason = "malformed"
	// CredentialRejectionUnsigned means the jwt header states no alg, or alg none.
	CredentialRejectionUnsigned CredentialRejectionReason = "unsigned"
	// CredentialRejectionExpired means the jwt states an exp at or before the request time.
	CredentialRejectionExpired CredentialRejectionReason = "expired"
)

// CredentialValidationError reports the failed credential rule. Its message is constant per
// reason and never contains the submitted value.
type CredentialValidationError struct {
	Reason CredentialRejectionReason
}

var credentialRejectionMessages = map[CredentialRejectionReason]string{
	CredentialRejectionUnsupportedFormat: "credential.format must be signed_jwt",
	CredentialRejectionUnsupportedUse:    "credential.use must be access_token",
	CredentialRejectionMissingJWT:        "credential.jwt is required",
	CredentialRejectionTooLarge:          "credential.jwt must be at most 16384 bytes",
	CredentialRejectionMalformed:         "credential.jwt must be a compact JWS with valid JWT claims",
	CredentialRejectionUnsigned:          "credential.jwt must be a signed compact JWS",
	CredentialRejectionExpired:           "credential.jwt is expired",
}

func (e *CredentialValidationError) Error() string {
	return credentialRejectionMessages[e.Reason]
}

func rejectCredential(reason CredentialRejectionReason) (*ValidatedCredential, error) {
	return nil, &CredentialValidationError{Reason: reason}
}

// ServiceAccountCredentialInput is a credential as submitted on a write. It holds a bearer
// credential, so every rendering of it is redacted.
type ServiceAccountCredentialInput struct {
	Format CredentialFormat
	Use    CredentialUse
	JWT    Secret
}

const redactedCredential = "REDACTED"

// LogValue implements slog.LogValuer with a redacted form.
func (c ServiceAccountCredentialInput) LogValue() slog.Value {
	return slog.StringValue(redactedCredential)
}

// String implements fmt.Stringer with a redacted form.
func (c ServiceAccountCredentialInput) String() string {
	return redactedCredential
}

// GoString implements fmt.GoStringer with a redacted form.
func (c ServiceAccountCredentialInput) GoString() string {
	return redactedCredential
}

// CredentialSummary is the non-secret recognition data of a stored credential. Its claim values
// are read from the token without verification; they are for display and status only.
type CredentialSummary struct {
	Format      CredentialFormat `json:"format"`
	Use         CredentialUse    `json:"use"`
	Iss         *string          `json:"iss,omitempty"`
	Sub         *string          `json:"sub,omitempty"`
	Kid         *string          `json:"kid,omitempty"`
	Exp         *int64           `json:"exp,omitempty"`
	Fingerprint string           `json:"fingerprint"`
}

// logClaimMaxBytes caps each claim value in log output; the values are untrusted input.
const logClaimMaxBytes = 256

// LogValue renders the summary for audit records. Claim strings are capped, and sub is rendered
// as an id.Principal so that it passes through the principal policy.
func (s CredentialSummary) LogValue() slog.Value {
	attrs := []slog.Attr{slog.String("format", string(s.Format)), slog.String("use", string(s.Use))}
	if s.Iss != nil {
		attrs = append(attrs, slog.String("iss", capLogValue(*s.Iss)))
	}
	if s.Sub != nil {
		attrs = append(attrs, slog.Any("sub", id.NewPrincipal(capLogValue(*s.Sub))))
	}
	if s.Kid != nil {
		attrs = append(attrs, slog.String("kid", capLogValue(*s.Kid)))
	}
	if s.Exp != nil {
		attrs = append(attrs, slog.Int64("exp", *s.Exp))
	}
	attrs = append(attrs, slog.String("fingerprint", s.Fingerprint))
	return slog.GroupValue(attrs...)
}

func capLogValue(value string) string {
	if len(value) <= logClaimMaxBytes {
		return value
	}
	end := logClaimMaxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

// ValidatedCredential is a credential that passed every rule, with its derived values.
type ValidatedCredential struct {
	// Document is the plaintext credential document to encrypt.
	Document  Secret
	Summary   CredentialSummary
	ExpiresAt *time.Time
}

// ValidateServiceAccountCredential applies the credential rules in order at the request time.
// The token is parsed with jwx without a key: no signature is verified and nothing is fetched.
// Library errors are never returned, because they can quote the input.
func ValidateServiceAccountCredential(input ServiceAccountCredentialInput, now time.Time) (*ValidatedCredential, error) {
	if input.Format != CredentialFormatSignedJWT {
		return rejectCredential(CredentialRejectionUnsupportedFormat)
	}
	use := input.Use
	if use == "" {
		use = CredentialUseAccessToken
	}
	if use != CredentialUseAccessToken {
		return rejectCredential(CredentialRejectionUnsupportedUse)
	}
	token, err := input.JWT.GetPlaintext()
	if err != nil {
		return rejectCredential(CredentialRejectionMissingJWT)
	}
	if len(token) > ServiceAccountJWTMaxBytes {
		return rejectCredential(CredentialRejectionTooLarge)
	}
	message, err := jws.Parse([]byte(token), jws.WithCompact())
	if err != nil || len(message.Signatures()) != 1 {
		return rejectCredential(CredentialRejectionMalformed)
	}
	claims := jwt.New()
	if err := json.Unmarshal(message.Payload(), claims); err != nil {
		return rejectCredential(CredentialRejectionMalformed)
	}
	headers := message.Signatures()[0].ProtectedHeaders()
	alg, hasAlg := headers.Algorithm()
	if !hasAlg || alg == jwa.NoSignature() {
		return rejectCredential(CredentialRejectionUnsigned)
	}
	summary := CredentialSummary{Format: CredentialFormatSignedJWT, Use: use, Fingerprint: CredentialFingerprint(token)}
	var expiresAt *time.Time
	if expiration, ok := claims.Expiration(); ok {
		if !now.Before(expiration) {
			return rejectCredential(CredentialRejectionExpired)
		}
		utc := expiration.UTC()
		seconds := utc.Unix()
		expiresAt, summary.Exp = &utc, &seconds
	}
	if issuer, ok := claims.Issuer(); ok {
		summary.Iss = &issuer
	}
	if subject, ok := claims.Subject(); ok {
		summary.Sub = &subject
	}
	if kid, ok := headers.KeyID(); ok {
		summary.Kid = &kid
	}
	document, err := json.Marshal(credentialDocument{Format: CredentialFormatSignedJWT, Use: use, JWT: token})
	if err != nil {
		return rejectCredential(CredentialRejectionMalformed)
	}
	return &ValidatedCredential{Document: NewPlaintextSecret(string(document)), Summary: summary, ExpiresAt: expiresAt}, nil
}

// credentialDocument is the encrypted plaintext of a signed_jwt credential, discriminated by
// format so that later formats share the stored column.
type credentialDocument struct {
	Format CredentialFormat `json:"format"`
	Use    CredentialUse    `json:"use"`
	JWT    string           `json:"jwt"`
}

// CredentialFingerprint returns the fingerprint of a stored credential.
// It is "SHA256:" and the unpadded standard base64 SHA-256 digest of the exact credential.
func CredentialFingerprint(credential string) string {
	digest := sha256.Sum256([]byte(credential))
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:])
}
