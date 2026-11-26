package model

import (
	"log/slog"
	"time"
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

func (e *CredentialValidationError) Error() string {
	return ""
}

// ServiceAccountCredentialInput is a credential as submitted on a write.
type ServiceAccountCredentialInput struct {
	Format CredentialFormat
	Use    CredentialUse
	JWT    Secret
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

// LogValue renders the summary for audit records.
func (s CredentialSummary) LogValue() slog.Value {
	return slog.Value{}
}

// ValidatedCredential is a credential that passed every rule, with its derived values.
type ValidatedCredential struct {
	// Document is the plaintext credential document to encrypt.
	Document  Secret
	Summary   CredentialSummary
	ExpiresAt *time.Time
}

// ValidateServiceAccountCredential applies the credential rules in order at the request time.
func ValidateServiceAccountCredential(input ServiceAccountCredentialInput, now time.Time) (*ValidatedCredential, error) {
	return nil, nil
}

// CredentialFingerprint returns the fingerprint of a stored credential.
func CredentialFingerprint(credential string) string {
	return ""
}
