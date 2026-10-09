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
	// ErrorKindConnection: Storage backend connection failed or disconnected.
	// Example: PostgreSQL connection refused, network timeout to database
	ErrorKindConnection ErrorKind = "connection"

	// ErrorKindTimeout: Operation exceeded its timeout deadline.
	// Example: Query took longer than configured read timeout
	ErrorKindTimeout ErrorKind = "timeout"

	// ErrorKindValidation: Input validation failed.
	// Example: Invalid connection string format, missing required parameters
	ErrorKindValidation ErrorKind = "validation"

	// ErrorKindNotFound: Requested entity does not exist.
	// Example: User with given ID not found in database
	ErrorKindNotFound ErrorKind = "not_found"

	// ErrorKindConflict: Operation conflicts with existing data.
	// Example: User ID already exists (duplicate key)
	ErrorKindConflict ErrorKind = "conflict"

	// ErrorKindUnknown: Error cause is unknown or unexpected.
	// Used as fallback for unmapped error scenarios.
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
