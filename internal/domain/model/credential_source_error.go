package model

import "errors"

var ErrCredentialSourceUnavailable = errors.New("credential source unavailable")

type CredentialSourceReason string

const (
	CredentialSourceReasonMissingBinding    CredentialSourceReason = "missing_binding"
	CredentialSourceReasonIdentityMissing   CredentialSourceReason = "identity_missing"
	CredentialSourceReasonIdentityMismatch  CredentialSourceReason = "identity_mismatch"
	CredentialSourceReasonGenerationChanged CredentialSourceReason = "generation_changed"
	CredentialSourceReasonNotFound          CredentialSourceReason = "not_found"
	CredentialSourceReasonPermissionDenied  CredentialSourceReason = "permission_denied"
	CredentialSourceReasonNotRegular        CredentialSourceReason = "not_regular"
	CredentialSourceReasonTooLarge          CredentialSourceReason = "too_large"
	CredentialSourceReasonEmpty             CredentialSourceReason = "empty"
	CredentialSourceReasonReadFailed        CredentialSourceReason = "read_failed"
)

type CredentialSourceError struct {
	Reason CredentialSourceReason
}

func NewCredentialSourceError(reason CredentialSourceReason) *CredentialSourceError {
	switch reason {
	case CredentialSourceReasonMissingBinding, CredentialSourceReasonIdentityMissing,
		CredentialSourceReasonIdentityMismatch, CredentialSourceReasonGenerationChanged,
		CredentialSourceReasonNotFound, CredentialSourceReasonPermissionDenied,
		CredentialSourceReasonNotRegular, CredentialSourceReasonTooLarge,
		CredentialSourceReasonEmpty, CredentialSourceReasonReadFailed:
	default:
		reason = CredentialSourceReasonReadFailed
	}
	return &CredentialSourceError{Reason: reason}
}

func (e *CredentialSourceError) Error() string {
	return ErrCredentialSourceUnavailable.Error()
}

func (e *CredentialSourceError) Is(target error) bool {
	return target == ErrCredentialSourceUnavailable
}
