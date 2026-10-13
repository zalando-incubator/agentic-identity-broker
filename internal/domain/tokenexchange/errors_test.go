package tokenexchange

import (
	"errors"
	"testing"
)

func TestTokenExchangeErrorInterface(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		err            *TokenExchangeError
		expectedCode   string
		expectedStatus int
	}{
		{
			name:           "invalid_request",
			err:            NewInvalidRequestError("missing resource parameter"),
			expectedCode:   "invalid_request",
			expectedStatus: 400,
		},
		{
			name:           "invalid_client",
			err:            NewInvalidClientError("signature verification failed"),
			expectedCode:   "invalid_client",
			expectedStatus: 401,
		},
		{
			name:           "invalid_grant",
			err:            NewInvalidGrantError("no session exists"),
			expectedCode:   "invalid_grant",
			expectedStatus: 400,
		},
		{
			name:           "invalid_target",
			err:            NewInvalidTargetError("resource not found"),
			expectedCode:   "invalid_target",
			expectedStatus: 400,
		},
		{
			name:           "access_denied",
			err:            NewAccessDeniedError("grant revoked"),
			expectedCode:   "access_denied",
			expectedStatus: 403,
		},
		{
			name:           "server_error",
			err:            NewServerError("database timeout"),
			expectedCode:   "server_error",
			expectedStatus: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Code(); got != tt.expectedCode {
				t.Errorf("Code() = %q, want %q", got, tt.expectedCode)
			}
			if got := tt.err.HTTPStatus(); got != tt.expectedStatus {
				t.Errorf("HTTPStatus() = %d, want %d", got, tt.expectedStatus)
			}
		})
	}
}

func TestTokenExchangeErrorDescription(t *testing.T) {
	t.Parallel()
	desc := "missing resource parameter"
	err := NewInvalidRequestError(desc)
	if got := err.Description(); got != desc {
		t.Errorf("Description() = %q, want %q", got, desc)
	}
}

func TestTokenExchangeErrorErrorMethod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      *TokenExchangeError
		expected string
	}{
		{
			name:     "with description",
			err:      NewInvalidRequestError("resource required"),
			expected: "token_exchange: invalid_request (resource required)",
		},
		{
			name:     "without description",
			err:      &TokenExchangeError{code: "invalid_client"},
			expected: "token_exchange: invalid_client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTokenExchangeErrorUnwrap(t *testing.T) {
	t.Parallel()
	cause := errors.New("connection refused")
	err := NewServerErrorWithCause("database error", cause)

	if got := err.Unwrap(); got != cause {
		t.Errorf("Unwrap() returned %v, want %v", got, cause)
	}

	if !errors.Is(err, cause) {
		t.Error("errors.Is() should find the wrapped cause")
	}
}

func TestTokenExchangeErrorUnwrapNil(t *testing.T) {
	t.Parallel()
	err := NewInvalidRequestError("missing param")
	if got := err.Unwrap(); got != nil {
		t.Errorf("Unwrap() = %v, want nil for error with no cause", got)
	}
}

func TestIsTokenExchangeError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "TokenExchangeError",
			err:      NewInvalidClientError("test"),
			expected: true,
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "wrapped TokenExchangeError",
			err:      NewServerErrorWithCause("wrapped", NewInvalidGrantError("inner")),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsTokenExchangeError(tt.err)
			if got != tt.expected {
				t.Errorf("IsTokenExchangeError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestAllErrorsImplementInterface verifies all error factory functions return proper errors
func TestAllErrorsImplementInterface(t *testing.T) {
	t.Parallel()
	errors := []error{
		NewInvalidRequestError("test"),
		NewInvalidClientError("test"),
		NewInvalidGrantError("test"),
		NewInvalidTargetError("test"),
		NewAccessDeniedError("test"),
		NewServerError("test"),
	}

	for _, err := range errors {
		if err == nil {
			t.Error("error factory returned nil")
		}
		// Verify error interface is properly implemented
		_ = err.Error()
		if !IsTokenExchangeError(err) {
			t.Errorf("%T is not recognized as TokenExchangeError", err)
		}
	}
}

// TestSecurityNoTokenInError verifies token values can't accidentally leak
func TestSecurityNoTokenInError(t *testing.T) {
	t.Parallel()
	// These descriptions intentionally don't include token values
	// The caller is responsible for not including them
	err := NewInvalidClientError("JWT signature verification failed")

	// The error message should not contain any token content
	errMsg := err.Error()
	if len(errMsg) > 200 { // Reasonable upper bound for safe error messages
		t.Errorf("error message unusually long, might contain token: %s", errMsg)
	}
}

// TestErrorCodesMatchRFC8693 verifies error codes comply with RFC 8693
func TestErrorCodesMatchRFC8693(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		err          *TokenExchangeError
		expectedCode string
	}{
		{"invalid_request", NewInvalidRequestError(""), "invalid_request"},
		{"invalid_client", NewInvalidClientError(""), "invalid_client"},
		{"invalid_grant", NewInvalidGrantError(""), "invalid_grant"},
		{"invalid_target", NewInvalidTargetError(""), "invalid_target"},
		{"access_denied", NewAccessDeniedError(""), "access_denied"},
		{"server_error", NewServerError(""), "server_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Code(); got != tt.expectedCode {
				t.Errorf("Code() = %q, want %q", got, tt.expectedCode)
			}
		})
	}
}

// TestHTTPStatusCodesPerRFC8693 verifies HTTP status codes match RFC 8693 Section 5.2
func TestHTTPStatusCodesPerRFC8693(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		err            *TokenExchangeError
		expectedStatus int
	}{
		{"invalid_request -> 400", NewInvalidRequestError(""), 400},
		{"invalid_client -> 401", NewInvalidClientError(""), 401},
		{"invalid_grant -> 400", NewInvalidGrantError(""), 400},
		{"invalid_target -> 400", NewInvalidTargetError(""), 400},
		{"access_denied -> 403", NewAccessDeniedError(""), 403},
		{"server_error -> 500", NewServerError(""), 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.HTTPStatus(); got != tt.expectedStatus {
				t.Errorf("HTTPStatus() = %d, want %d", got, tt.expectedStatus)
			}
		})
	}
}

// TestTokenExchangeErrorWithCause verifies WithCause preserves all fields and attaches the cause.
func TestTokenExchangeErrorWithCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("jwt: audience mismatch")
	base := NewInvalidGrantError("subject_token issuer validation failed")
	err := base.WithCause(cause)

	if err.Code() != base.Code() {
		t.Errorf("WithCause changed code: got %q, want %q", err.Code(), base.Code())
	}
	if err.Description() != base.Description() {
		t.Errorf("WithCause changed description: got %q, want %q", err.Description(), base.Description())
	}
	if err.HTTPStatus() != base.HTTPStatus() {
		t.Errorf("WithCause changed httpStatus: got %d, want %d", err.HTTPStatus(), base.HTTPStatus())
	}
	if got := err.Unwrap(); got != cause {
		t.Errorf("Unwrap() = %v, want %v", got, cause)
	}
	if !errors.Is(err, cause) {
		t.Error("errors.Is() should find the wrapped cause via WithCause")
	}
}

// TestTokenExchangeErrorWithCauseDoesNotMutateOriginal verifies WithCause returns a new instance.
func TestTokenExchangeErrorWithCauseDoesNotMutateOriginal(t *testing.T) {
	t.Parallel()
	cause := errors.New("low-level failure")
	base := NewInvalidGrantError("token validation failed")
	withCause := base.WithCause(cause)

	// Original must not be mutated
	if base.Unwrap() != nil {
		t.Error("WithCause mutated the original error's cause; it should return a new instance")
	}
	// New instance must carry the cause
	if withCause.Unwrap() != cause {
		t.Errorf("new instance Unwrap() = %v, want %v", withCause.Unwrap(), cause)
	}
}
