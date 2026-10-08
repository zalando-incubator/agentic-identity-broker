package oauth2session

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

var (
	// ErrStateTokenExpired is returned when a state token has expired.
	ErrStateTokenExpired = errors.New("state token expired")

	// ErrPrincipalMismatch is returned when the principal in the state token doesn't match the request.
	ErrPrincipalMismatch = errors.New("principal mismatch (CSRF protection)")

	// ErrServiceNotFound is returned when the service ID doesn't exist.
	ErrServiceNotFound = errors.New("service not found")

	// ErrSessionNotFound is returned when a session doesn't exist.
	ErrSessionNotFound = errors.New("session not found")

	// ErrUnauthorized is returned when access to a resource is denied (principal mismatch on resource ownership).
	ErrUnauthorized = errors.New("unauthorized access to resource")

	// ErrInvalidPKCE is returned when PKCE validation fails.
	ErrInvalidPKCE = errors.New("invalid PKCE")

	// ErrInvalidStateToken is returned when state token is invalid or tampered.
	ErrInvalidStateToken = errors.New("invalid state token")

	// ErrStateTokenTooLarge prevents oversized state from reaching the provider.
	ErrStateTokenTooLarge = errors.New("state token exceeds provider size limit")

	// ErrServiceIDMismatch is returned when service_id in token doesn't match callback.
	ErrServiceIDMismatch = errors.New("service_id mismatch")

	// ErrTokenExchange is returned when code-for-token exchange fails.
	ErrTokenExchange = errors.New("token exchange failed")

	// ErrEncryptionFailed is returned when token encryption fails.
	ErrEncryptionFailed = errors.New("token encryption failed")

	// ErrInvalidConfiguration is returned when OAuth2 service configuration is invalid.
	ErrInvalidConfiguration = errors.New("invalid OAuth2 service configuration")

	// ErrSessionExpired indicates the access token expired and no usable refresh token remains;
	// ErrRefreshTokenExpired is also wrapped when a stored refresh token has expired.
	ErrSessionExpired = errors.New("session expired: both access and refresh tokens expired")

	// ErrRefreshFailed indicates token refresh operation failed with upstream provider.
	ErrRefreshFailed = errors.New("failed to refresh access token with upstream provider")

	// ErrRefreshTokenExpired indicates that a stored refresh token's recorded expiry has passed.
	ErrRefreshTokenExpired = errors.New("refresh token expired")

	// ErrRefreshRejected indicates that a provider rejected a refresh with invalid_grant.
	ErrRefreshRejected = errors.New("refresh token rejected by provider")

	// ErrRefreshNotAvailable indicates the session has no usable refresh token.
	ErrRefreshNotAvailable = errors.New("no valid refresh token available for session")
)

// RefreshRejectedError reports that the third-party service's token endpoint answered a refresh request
// with a non-2xx status. The message omits the response body, which may echo credentials.
type RefreshRejectedError struct {
	StatusCode int
	// OAuthError is the RFC 6749 §5.2 "error" code from a JSON error body; empty otherwise.
	OAuthError string
}

func (e *RefreshRejectedError) Error() string {
	return fmt.Sprintf("third-party token endpoint returned error status %d", e.StatusCode)
}

// Unwrap distinguishes provider rejection from a locally recorded token expiry.
func (e *RefreshRejectedError) Unwrap() error {
	if e.OAuthError == "invalid_grant" && e.StatusCode >= 400 && e.StatusCode <= 499 && e.StatusCode != http.StatusTooManyRequests {
		return ErrRefreshRejected
	}
	return nil
}

type Operation string

const (
	OperationUnknown         Operation = "unknown"
	OperationSessionLookup   Operation = "session_lookup"
	OperationRefresh         Operation = "refresh"
	OperationScopeValidation Operation = "scope_validation"
	OperationCodeExchange    Operation = "code_exchange"
)

type ErrorDetail string

const (
	DetailSessionMissing          ErrorDetail = "session_missing"
	DetailAccessTokenExpired      ErrorDetail = "access_token_expired"
	DetailRefreshTokenExpired     ErrorDetail = "refresh_token_expired"
	DetailRefreshUnavailable      ErrorDetail = "refresh_unavailable"
	DetailRefreshRejected         ErrorDetail = "refresh_rejected"
	DetailProviderClientRejected  ErrorDetail = "provider_client_rejected"
	DetailProviderRejected        ErrorDetail = "provider_rejected"
	DetailProviderUnavailable     ErrorDetail = "provider_unavailable"
	DetailProviderResponseInvalid ErrorDetail = "provider_response_invalid"
	DetailRepositoryUnavailable   ErrorDetail = "repository_unavailable"
	DetailDecryptionFailed        ErrorDetail = "decryption_failed"
	DetailEncryptionFailed        ErrorDetail = "encryption_failed"
	DetailPersistenceFailed       ErrorDetail = "persistence_failed"
	DetailConfiguration           ErrorDetail = "configuration"
	DetailCallerCanceled          ErrorDetail = "caller_canceled"
	DetailInternalUnclassified    ErrorDetail = "internal_unclassified"
)

type ErrorKind string

const (
	KindSession        ErrorKind = "session"
	KindProvider       ErrorKind = "provider"
	KindInfrastructure ErrorKind = "infrastructure"
	KindConfiguration  ErrorKind = "configuration"
	KindCanceled       ErrorKind = "canceled"
	KindInternal       ErrorKind = "internal"
)

type Dependency string

const (
	DependencyNone               Dependency = "none"
	DependencySessionRepository  Dependency = "session_repository"
	DependencyProviderRepository Dependency = "provider_repository"
	DependencyProvider           Dependency = "provider"
	DependencyEncryption         Dependency = "encryption"
	DependencySigning            Dependency = "signing"
)

// ErrorMetadata contains only bounded, credential-free classifications.
// Copies can be enriched independently, including after a shared refresh fails.
type ErrorMetadata struct {
	operation  Operation
	detail     ErrorDetail
	kind       ErrorKind
	dependency Dependency
	statusCode int
	oauthCode  string
}

func NewErrorMetadata(operation Operation, detail ErrorDetail) ErrorMetadata {
	switch operation {
	case OperationSessionLookup, OperationRefresh, OperationScopeValidation, OperationCodeExchange:
	default:
		operation = OperationUnknown
	}
	metadata := ErrorMetadata{operation: operation, detail: detail, dependency: DependencyNone}
	switch detail {
	case DetailSessionMissing, DetailAccessTokenExpired, DetailRefreshTokenExpired, DetailRefreshUnavailable:
		metadata.kind, metadata.dependency = KindSession, DependencySessionRepository
	case DetailRefreshRejected, DetailProviderRejected, DetailProviderResponseInvalid:
		metadata.kind, metadata.dependency = KindProvider, DependencyProvider
	case DetailProviderClientRejected:
		metadata.kind, metadata.dependency = KindConfiguration, DependencyProvider
	case DetailProviderUnavailable:
		metadata.kind, metadata.dependency = KindInfrastructure, DependencyProvider
	case DetailRepositoryUnavailable, DetailPersistenceFailed:
		metadata.kind, metadata.dependency = KindInfrastructure, DependencySessionRepository
	case DetailDecryptionFailed, DetailEncryptionFailed:
		metadata.kind, metadata.dependency = KindInfrastructure, DependencyEncryption
	case DetailConfiguration:
		metadata.kind = KindConfiguration
	case DetailCallerCanceled:
		metadata.kind = KindCanceled
	default:
		metadata.detail, metadata.kind = DetailInternalUnclassified, KindInternal
	}
	return metadata
}

func (m ErrorMetadata) Operation() Operation {
	if m.operation == "" {
		return OperationUnknown
	}
	return m.operation
}

func (m ErrorMetadata) Detail() ErrorDetail {
	if m.detail == "" {
		return DetailInternalUnclassified
	}
	return m.detail
}

func (m ErrorMetadata) Kind() ErrorKind {
	if m.kind == "" {
		return KindInternal
	}
	return m.kind
}

func (m ErrorMetadata) Dependency() Dependency {
	if m.dependency == "" {
		return DependencyNone
	}
	return m.dependency
}

func (m ErrorMetadata) StatusCode() int   { return m.statusCode }
func (m ErrorMetadata) OAuthCode() string { return m.oauthCode }

func (m ErrorMetadata) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("operation", string(m.Operation())),
		slog.String("failure_detail", string(m.Detail())),
		slog.String("error_kind", string(m.Kind())),
		slog.String("dependency", string(m.Dependency())),
		slog.Int("http_status_code", m.StatusCode()),
		slog.String("oauth_error_code", m.OAuthCode()),
	)
}

func (m ErrorMetadata) WithDependency(dependency Dependency) ErrorMetadata {
	switch dependency {
	case DependencyNone, DependencySessionRepository, DependencyProviderRepository, DependencyProvider, DependencyEncryption, DependencySigning:
		m.dependency = dependency
	default:
		m.dependency = DependencyNone
	}
	return m
}

func (m ErrorMetadata) WithProviderResponse(status int, code string) ErrorMetadata {
	m.statusCode = 0
	if status >= 100 && status <= 599 {
		m.statusCode = status
	}
	m.oauthCode = code
	if code != "" && !IsSafeOAuthErrorCode(code) {
		m.oauthCode = "unknown"
	}
	return m
}

type OperationError struct {
	metadata ErrorMetadata
	cause    error
}

func NewOperationError(metadata ErrorMetadata, cause error) *OperationError {
	return &OperationError{metadata: metadata, cause: cause}
}

func (e *OperationError) Error() string {
	return fmt.Sprintf("OAuth2 session operation failed: operation=%s detail=%s", e.metadata.Operation(), e.metadata.Detail())
}

func (e *OperationError) Metadata() ErrorMetadata { return e.metadata }
func (e *OperationError) Unwrap() error           { return e.cause }

func (e *OperationError) WithMetadata(metadata ErrorMetadata) *OperationError {
	return NewOperationError(metadata, e.cause)
}
