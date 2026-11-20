package storage

import (
	"errors"
	"fmt"
)

// ErrDuplicateDCRClientIdentity marks an issuer/client ID pair that is already stored.
var ErrDuplicateDCRClientIdentity = errors.New("DCR client identity already exists")

// ErrIssuerChangeHasSessions marks a service whose sessions block an issuer change.
var ErrIssuerChangeHasSessions = errors.New("user sessions block issuer change")

// ErrResourceChangeHasSessions marks an effective audience change blocked by sessions.
var ErrResourceChangeHasSessions = errors.New("user sessions block resource change")

// ErrProtectedResourceOwned marks a URI claimed by another service.
var ErrProtectedResourceOwned = errors.New("protected resource URI already owned")

// ErrorKind classifies the type of storage error for consistent handling.
type ErrorKind string

const (
	// ErrorKindConnection means the database was not initialized, or a query, the start of a transaction or a
	// commit failed in a way that the repository does not classify further, so check the database and its
	// connections. Storage spans record it as `error.type`.
	ErrorKindConnection ErrorKind = "connection"

	// ErrorKindTimeout means the operation passed its deadline, from `storage.timeouts.read`,
	// `storage.timeouts.write` or the caller, or PostgreSQL canceled a statement after its statement timeout.
	// Storage spans record it as `error.type`.
	ErrorKindTimeout ErrorKind = "timeout"

	// ErrorKindValidation means the repository rejected the input, such as an empty ID, a nil entity, a value
	// that it could not encode as JSON or a value that a check constraint refuses. Storage spans record it as
	// `storage.outcome`, because the caller handles it.
	ErrorKindValidation ErrorKind = "validation"

	// ErrorKindNotFound means the requested row does not exist, an update or a delete matched no row, or a write
	// referenced a row that does not exist. Storage spans record it as `storage.outcome`, because the caller
	// handles it.
	ErrorKindNotFound ErrorKind = "not_found"

	// ErrorKindConflict means the write conflicts with stored data: a unique value is already taken, the
	// transaction failed to serialize, the stored version is newer, or a referenced row is missing or was deleted
	// concurrently. Storage spans record it as `storage.outcome`, because the caller handles it.
	ErrorKindConflict ErrorKind = "conflict"

	// ErrorKindUnknown means the repository could not classify the failure, such as a stored value that it could
	// not decode, a row count that it could not read or a broken invariant. Storage spans also record it as
	// `error.type` for an error that is not a storage error.
	ErrorKindUnknown ErrorKind = "unknown"
)

// StorageError represents a domain error from storage operations.
// Wraps adapter-specific errors in domain-friendly form without exposing implementation details.
// All storage adapter errors MUST be wrapped in StorageError before returning to domain logic.
type StorageError struct {
	// Operation that failed (e.g., "Initialize", "CreateUser", "UpdateUser")
	Operation string

	// Underlying error (wrapped) - may be adapter-specific but should not be exposed
	Cause error

	// User-friendly error description
	// Does NOT expose SQL, connection strings, or implementation details
	Message string

	// Classification of error type for consistent handling
	Kind ErrorKind
}

// Error implements the error interface for StorageError.
func (se *StorageError) Error() string {
	if se.Message != "" {
		return fmt.Sprintf("%s: %s", se.Operation, se.Message)
	}
	if se.Cause != nil {
		return fmt.Sprintf("%s: %v", se.Operation, se.Cause)
	}
	return se.Operation
}

// Unwrap returns the wrapped error for use with errors.Is and errors.As.
func (se *StorageError) Unwrap() error {
	return se.Cause
}

// Is supports errors.Is comparisons with other StorageErrors.
// Two errors are equal if they have the same Operation and Kind.
func (se *StorageError) Is(target error) bool {
	other, ok := target.(*StorageError)
	if !ok {
		return false
	}
	return se.Operation == other.Operation && se.Kind == other.Kind
}

// NewStorageError creates a new StorageError with the given parameters.
// This is the factory function for creating domain errors from adapter operations.
func NewStorageError(operation string, kind ErrorKind, cause error, message string) *StorageError {
	return &StorageError{
		Operation: operation,
		Kind:      kind,
		Cause:     cause,
		Message:   message,
	}
}
