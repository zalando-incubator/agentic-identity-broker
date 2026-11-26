package model

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// ServiceAccountKind states how a service account obtains tokens. It never changes after
// creation.
type ServiceAccountKind string

// ServiceAccountKindJWTBearer obtains its token from a JWT credential.
const ServiceAccountKindJWTBearer ServiceAccountKind = "jwt_bearer"

// Narrowing states whether the broker can ask for less than the ceiling. It is derived from kind
// and credential format.
type Narrowing string

// NarrowingNone means the broker can only hand out what the credential grants.
const NarrowingNone Narrowing = "none"

// ServiceAccountStatus states whether a service account is usable.
type ServiceAccountStatus string

const (
	// ServiceAccountStatusReady means the account's credential is usable.
	ServiceAccountStatusReady ServiceAccountStatus = "ready"
	// ServiceAccountStatusFailed means the account is not usable; a failure reason says why.
	ServiceAccountStatusFailed ServiceAccountStatus = "failed"
)

// ServiceAccountFailureReason says why a service account is failed.
type ServiceAccountFailureReason string

// ServiceAccountFailureCredentialExpired means the stored credential's stated exp has passed.
const ServiceAccountFailureCredentialExpired ServiceAccountFailureReason = "credential_expired"

// ServiceAccountValidationError reports a refused entity member with a constant message.
type ServiceAccountValidationError struct {
	Message string
}

func (e *ServiceAccountValidationError) Error() string {
	return e.Message
}

// ServiceAccount is a machine credential that a third-party service issued to software,
// attached to exactly one service for its whole life and bounded by a ceiling.
type ServiceAccount struct {
	ID          id.ServiceAccountID
	ServiceID   id.ServiceID
	CanonicalID string
	Kind        ServiceAccountKind
	// Scopes is the ceiling, in ascending byte order.
	Scopes []string
	// Credential is the credential document: plaintext before encryption, ciphertext in storage.
	Credential          Secret
	Summary             CredentialSummary
	CredentialExpiresAt *time.Time
	Version             int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// StatusAt derives the account's status at the given instant.
func (a *ServiceAccount) StatusAt(now time.Time) (ServiceAccountStatus, *ServiceAccountFailureReason) {
	return "", nil
}

// Narrowing derives whether the broker can ask for less than the ceiling.
func (a *ServiceAccount) Narrowing() Narrowing {
	return ""
}

// ValidateServiceAccountKind accepts the kinds this release offers.
func ValidateServiceAccountKind(kind ServiceAccountKind) error {
	return nil
}

// ValidateServiceAccountCanonicalID applies the canonical-ID rules to a required name.
func ValidateServiceAccountCanonicalID(canonicalID string) error {
	return nil
}

// NormalizeServiceAccountScopes validates a ceiling and returns it in ascending byte order.
func NormalizeServiceAccountScopes(scopes []string) ([]string, error) {
	return nil, nil
}
