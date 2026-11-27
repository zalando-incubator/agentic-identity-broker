package model

import (
	"slices"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/canonical"
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
	ObjectMetadata      ObjectMetadata
	Version             int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// StatusAt derives the account's status at the given instant.
// The account is failed from the instant a stated expiry has passed, without skew allowance.
func (a *ServiceAccount) StatusAt(now time.Time) (ServiceAccountStatus, *ServiceAccountFailureReason) {
	if a.CredentialExpiresAt != nil && !now.Before(*a.CredentialExpiresAt) {
		reason := ServiceAccountFailureCredentialExpired
		return ServiceAccountStatusFailed, &reason
	}
	return ServiceAccountStatusReady, nil
}

// Narrowing derives whether the broker can ask for less than the ceiling.
// A stored signed JWT is handed out as it is, so it cannot be narrowed.
func (a *ServiceAccount) Narrowing() Narrowing {
	return NarrowingNone
}

// ValidateServiceAccountKind accepts the kinds this release offers.
func ValidateServiceAccountKind(kind ServiceAccountKind) error {
	if kind != ServiceAccountKindJWTBearer {
		return &ServiceAccountValidationError{Message: "kind must be jwt_bearer"}
	}
	return nil
}

// ValidateServiceAccountCanonicalID applies the canonical-ID rules to a required name.
func ValidateServiceAccountCanonicalID(canonicalID string) error {
	if err := canonical.Validate(&canonicalID); err != nil {
		return &ServiceAccountValidationError{Message: err.Error()}
	}
	return nil
}

// NormalizeServiceAccountScopes validates a ceiling and returns it in ascending byte order.
func NormalizeServiceAccountScopes(scopes []string) ([]string, error) {
	invalid := &ServiceAccountValidationError{Message: "scopes must be a non-empty list of unique, non-empty values"}
	if len(scopes) == 0 {
		return nil, invalid
	}
	sorted := slices.Clone(scopes)
	slices.Sort(sorted)
	for i, scope := range sorted {
		if scope == "" || (i > 0 && sorted[i-1] == scope) {
			return nil, invalid
		}
	}
	return sorted, nil
}
