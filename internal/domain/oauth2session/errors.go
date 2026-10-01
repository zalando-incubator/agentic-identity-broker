package oauth2session

import (
	"errors"
	"fmt"
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

	// ErrRefreshTokenExpired indicates the stored refresh token can no longer be used: its recorded
	// expiry has passed, or the third-party service answered the refresh with invalid_grant (RFC 6749 §5.2).
	ErrRefreshTokenExpired = errors.New("refresh token expired")

	// ErrRefreshNotAvailable indicates the session has no refresh token or the refresh token has expired.
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

// Unwrap exposes ErrRefreshTokenExpired when the third-party service answered invalid_grant, which
// RFC 6749 §5.2 defines as an invalid, expired, or revoked refresh token.
func (e *RefreshRejectedError) Unwrap() error {
	if e.OAuthError == "invalid_grant" {
		return ErrRefreshTokenExpired
	}
	return nil
}
