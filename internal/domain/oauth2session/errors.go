package oauth2session

import "errors"

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

	// ErrSessionExpired indicates both access token and refresh token are expired.
	// Corresponds to RFC 8693 T076: invalid_grant error with re-auth hint.
	ErrSessionExpired = errors.New("session expired: both access and refresh tokens expired")

	// ErrRefreshFailed indicates token refresh operation failed with upstream provider.
	ErrRefreshFailed = errors.New("failed to refresh access token with upstream provider")

	// ErrRefreshNotAvailable indicates the session has no refresh token or the refresh token has expired.
	ErrRefreshNotAvailable = errors.New("no valid refresh token available for session")
)
