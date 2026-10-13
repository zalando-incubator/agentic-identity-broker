// Package tokenexchange provides domain types and errors for RFC 8693 OAuth 2.0 Token Exchange.
package tokenexchange

import (
	"errors"
	"fmt"
)

// TokenExchangeError represents an error that occurred during token exchange (RFC 8693).
// This is a domain error that provides both RFC 8693 error codes for client responses
// and HTTP status codes for the HTTP layer. Token values are NEVER included in error
// messages to prevent accidental exposure in logs or responses (per security requirement SR-005).
//
// Implements the error interface and is compatible with Go 1.13+ error wrapping.
type TokenExchangeError struct {
	// code is the RFC 8693 error code (e.g., "invalid_request", "invalid_client")
	code string

	// description is a human-readable error message suitable for the error_description
	// response field in RFC 8693 error responses. MUST NOT contain token values.
	description string

	// httpStatus is the HTTP status code for this error response (per RFC 8693 Section 5.2)
	httpStatus int

	// Causes remain inspectable with errors.Is/As, but must not be serialized into telemetry.
	cause error

	// errorURI is the RFC 6749 §5.2 error_uri — a URI pointing to a human-readable page
	// with more information about the error. For session-not-found / session-expired errors
	// this is the sessions landing page where the user can inspect the session.
	// May be empty when no re-authentication URL is available.
	errorURI string

	diagnostic    Diagnostic
	service       ServiceRef
	authorization AuthorizationRef
}

var (
	ErrResourceUnregistered = errors.New("requested resource is not registered")
	ErrResourceAmbiguous    = errors.New("registered resource configuration is ambiguous")
)

func (e *TokenExchangeError) Diagnostic() Diagnostic {
	if e.diagnostic.outcome == "" {
		return NewDiagnostic(StageExchangeRouting, DetailInternalUnclassified)
	}
	return e.diagnostic
}

func (e *TokenExchangeError) Service() ServiceRef {
	return e.service
}

func (e *TokenExchangeError) Authorization() AuthorizationRef {
	return e.authorization
}

// Error implements the error interface, returning a formatted error message.
// The message includes the RFC 8693 error code and description.
// Token values are never included.
func (e *TokenExchangeError) Error() string {
	if e.description != "" {
		return fmt.Sprintf("token_exchange: %s (%s)", e.code, e.description)
	}
	return fmt.Sprintf("token_exchange: %s", e.code)
}

// Unwrap returns the underlying error for use with errors.Is and errors.As.
// Enables proper error chain inspection in Go 1.13+.
func (e *TokenExchangeError) Unwrap() error {
	return e.cause
}

// Code returns the RFC 8693 error code (e.g., "invalid_request", "access_denied").
// This value is included in the error_code field of RFC 8693 error responses.
func (e *TokenExchangeError) Code() string {
	return e.code
}

// Description returns the user-facing error description for RFC 8693 responses.
// This value is included in the error_description field. Does not contain token values.
func (e *TokenExchangeError) Description() string {
	return e.description
}

// HTTPStatus returns the HTTP status code for this error.
// Per RFC 8693 Section 5.2:
// - 400 Bad Request: invalid_request, invalid_grant, invalid_target
// - 401 Unauthorized: invalid_client
// - 403 Forbidden: access_denied
// - 500 Internal Server Error: server_error
func (e *TokenExchangeError) HTTPStatus() int {
	return e.httpStatus
}

// NewInvalidRequestError creates an error for malformed token exchange requests.
// RFC 8693 Section 5.2: invalid_request (400)
// Used when required parameters are missing, malformed, or invalid.
// Examples: missing resource parameter, invalid JWT format, invalid URI syntax
//
// Per spec FR-008, resource parameter is required and this error is returned when omitted.
// Per spec FR-049a, this error includes error_description "resource parameter is required" when applicable.
func NewInvalidRequestError(description string) *TokenExchangeError {
	return &TokenExchangeError{
		code:        "invalid_request",
		description: description,
		httpStatus:  400,
	}
}

// NewInvalidScopeError creates an error for a requested scope that is not permitted.
func NewInvalidScopeError(description string) *TokenExchangeError {
	return &TokenExchangeError{
		code:        InvalidScopeError,
		description: description,
		httpStatus:  400,
	}
}

// NewInvalidClientError creates an error for invalid client authentication.
// RFC 8693 Section 5.2: invalid_client (401)
// Used when client_assertion is missing, invalid, expired, or fails signature verification.
// Per spec SR-001 and SR-006, JWT signature verification must use JWKS and failures deny the request.
func NewInvalidClientError(description string) *TokenExchangeError {
	return &TokenExchangeError{
		code:        "invalid_client",
		description: description,
		httpStatus:  401,
	}
}

// NewInvalidGrantError creates an error when no valid tokens exist for exchange.
// RFC 8693 Section 5.2: invalid_grant (400)
// Used when:
// - subject_token is invalid, expired, or fails signature verification
// - User has no session with target service (both access and refresh tokens missing)
// - All stored tokens (access and refresh) have fully expired
// Per spec US5-S1 and US5-S2, this error enables agents to trigger re-authentication flows.
func NewInvalidGrantError(description string) *TokenExchangeError {
	return &TokenExchangeError{
		code:        "invalid_grant",
		description: description,
		httpStatus:  400,
	}
}

// NewInvalidTargetError creates an error when resource cannot be resolved to a service.
// RFC 8693 Section 5.2: invalid_target (400)
// Used when:
// - resource parameter doesn't match any service's protected_resources (no matching service found)
// - resource parameter matches multiple services ambiguously (misconfiguration)
// - Multiple resource parameters map to different services (per FR-008a)
// Per spec US2-S4 and US2-S5, this error handles resource resolution failures.
func NewInvalidTargetError(description string) *TokenExchangeError {
	return &TokenExchangeError{
		code:        "invalid_target",
		description: description,
		httpStatus:  400,
	}
}

func NewResourceUnregisteredError() *TokenExchangeError {
	return NewInvalidTargetError("no service configured for the requested resource").
		WithCause(ErrResourceUnregistered).WithDiagnostic(NewDiagnostic(StageResourceResolution, DetailResourceUnregistered))
}

func NewResourceAmbiguousError() *TokenExchangeError {
	return NewServerError("registered resource configuration is ambiguous").
		WithCause(ErrResourceAmbiguous).WithDiagnostic(NewDiagnostic(StageResourceResolution, DetailResourceAmbiguous))
}

func IsResourceNotConfigured(err error) bool {
	return errors.Is(err, ErrResourceUnregistered)
}

func IsResourceAmbiguous(err error) bool {
	return errors.Is(err, ErrResourceAmbiguous)
}

// NewAccessDeniedError creates an error for authorization failures.
// RFC 8693 Section 5.2: access_denied (403)
// Used when:
// - User lacks UserGrant for the agent+service (no authorization/consent)
// - User's grant is revoked (per spec US3-S3)
// - User's grant has expired (per spec US3-S4)
// - CEL authorization expression evaluates to false (per spec US4-S3)
// Per spec SR-007, UserGrant verification is mandatory before returning tokens.
// This error is distinct from invalid_grant: access_denied means no permission,
// invalid_grant means no session exists.
func NewAccessDeniedError(description string) *TokenExchangeError {
	return &TokenExchangeError{
		code:        "access_denied",
		description: description,
		httpStatus:  403,
	}
}

// NewServerError creates an error for internal server errors.
// RFC 8693 Section 5.2: server_error (500)
// Used when unexpected errors occur:
// - CEL expression evaluation timeout (per spec SC-005 100ms timeout)
// - Storage/database failures
// - Upstream service failures (JWKS fetch timeout, token refresh failure)
// - Any other unexpected internal error
func NewServerError(description string) *TokenExchangeError {
	return &TokenExchangeError{
		code:        "server_error",
		description: description,
		httpStatus:  500,
	}
}

// NewServerErrorWithCause creates an error for internal server errors wrapping an underlying cause.
// This is useful for preserving error chains for logging and debugging.
func NewServerErrorWithCause(description string, cause error) *TokenExchangeError {
	return &TokenExchangeError{
		code:        "server_error",
		description: description,
		httpStatus:  500,
		cause:       cause,
	}
}

// IsTokenExchangeError checks if an error is a TokenExchangeError.
// Useful for type-safe error handling at API boundaries.
func IsTokenExchangeError(err error) bool {
	_, ok := err.(*TokenExchangeError)
	return ok
}

// WithCause returns a copy of the error with the given underlying cause attached.
// The cause is available for error chain inspection, not telemetry serialization.
// The cause is NOT included in the RFC 8693 error response to the client.
func (e *TokenExchangeError) WithCause(cause error) *TokenExchangeError {
	clone := *e
	clone.cause = cause
	return &clone
}

// ErrorURI returns the RFC 6749 §5.2 error_uri, if set.
func (e *TokenExchangeError) ErrorURI() string {
	return e.errorURI
}

// WithErrorURI returns a copy of the error with the given error_uri attached.
func (e *TokenExchangeError) WithErrorURI(uri string) *TokenExchangeError {
	clone := *e
	clone.errorURI = uri
	return &clone
}

func (e *TokenExchangeError) WithDiagnostic(diagnostic Diagnostic) *TokenExchangeError {
	clone := *e
	clone.diagnostic = diagnostic
	return &clone
}

func (e *TokenExchangeError) WithObservation(diagnostic Diagnostic, service ServiceRef, authorization AuthorizationRef) *TokenExchangeError {
	clone := *e
	clone.diagnostic = diagnostic
	clone.service = service
	clone.authorization = authorization
	return &clone
}
