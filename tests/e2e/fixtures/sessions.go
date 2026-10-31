package fixtures

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// GitHubSessionForPrincipal returns a fixture for an active GitHub OAuth2 session.
func GitHubSessionForPrincipal(principal string) *storage.UserSession {
	now := time.Now()
	accessTokenExpires := now.Add(1 * time.Hour)
	refreshTokenExpires := now.Add(30 * 24 * time.Hour)
	ghServiceID := id.MustParseServiceID("a0000000-0000-0000-0000-000000000001")

	return &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             id.Principal(principal),
		ServiceID:             ghServiceID,
		EncryptedAccessToken:  EncryptedToken(ghServiceID.String(), "github-token-xyz"),
		EncryptedRefreshToken: EncryptedToken(ghServiceID.String(), "github-refresh-xyz"),
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  &accessTokenExpires,
		RefreshTokenExpiresAt: &refreshTokenExpires,
		Scope:                 []string{"repo", "user"},
		EncryptionContext: storage.EncryptionContext{
			ServiceID: ghServiceID,
		},
		InitiatedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// ExpiredGitHubSessionForPrincipal returns a GitHub session where the access token is expired.
func ExpiredGitHubSessionForPrincipal(principal string) *storage.UserSession {
	now := time.Now()
	accessTokenExpired := now.Add(-1 * time.Hour)
	refreshTokenValid := now.Add(7 * 24 * time.Hour)
	ghServiceID := id.MustParseServiceID("a0000000-0000-0000-0000-000000000001")

	return &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             id.Principal(principal),
		ServiceID:             ghServiceID,
		EncryptedAccessToken:  EncryptedToken(ghServiceID.String(), "expired-github-token"),
		EncryptedRefreshToken: EncryptedToken(ghServiceID.String(), "github-refresh-xyz"),
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  &accessTokenExpired,
		RefreshTokenExpiresAt: &refreshTokenValid,
		Scope:                 []string{"repo", "user"},
		EncryptionContext: storage.EncryptionContext{
			ServiceID: ghServiceID,
		},
		InitiatedAt: now.Add(-2 * time.Hour),
		CreatedAt:   now.Add(-2 * time.Hour),
		UpdatedAt:   now.Add(-1 * time.Hour),
	}
}

// FullyExpiredSessionForPrincipal returns a session where both tokens are expired.
func FullyExpiredSessionForPrincipal(principal, serviceID string) *storage.UserSession {
	now := time.Now()
	accessTokenExpired := now.Add(-2 * time.Hour)
	refreshTokenExpired := now.Add(-1 * time.Hour)
	svcID := id.MustParseServiceID(serviceID)

	return &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             id.Principal(principal),
		ServiceID:             svcID,
		EncryptedAccessToken:  EncryptedToken(serviceID, "fully-expired-access-token"),
		EncryptedRefreshToken: EncryptedToken(serviceID, "fully-expired-refresh-token"),
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  &accessTokenExpired,
		RefreshTokenExpiresAt: &refreshTokenExpired,
		Scope:                 []string{"read", "write"},
		EncryptionContext: storage.EncryptionContext{
			ServiceID: svcID,
		},
		InitiatedAt: now.Add(-7 * 24 * time.Hour),
		CreatedAt:   now.Add(-7 * 24 * time.Hour),
		UpdatedAt:   now.Add(-1 * time.Hour),
	}
}

// SessionForService returns a generic active session for the given principal and service.
func SessionForService(principal, serviceID string) *storage.UserSession {
	now := time.Now()
	accessTokenExpires := now.Add(1 * time.Hour)
	refreshTokenExpires := now.Add(30 * 24 * time.Hour)
	svcID := id.MustParseServiceID(serviceID)

	return &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             id.Principal(principal),
		ServiceID:             svcID,
		EncryptedAccessToken:  EncryptedToken(serviceID, "token-"+serviceID),
		EncryptedRefreshToken: EncryptedToken(serviceID, "refresh-"+serviceID),
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  &accessTokenExpires,
		RefreshTokenExpiresAt: &refreshTokenExpires,
		Scope:                 []string{"read", "write"},
		EncryptionContext: storage.EncryptionContext{
			ServiceID: svcID,
		},
		InitiatedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// SessionForServiceWithScopes returns an active session for the given principal, service, and
// explicit scope list. Use this when the session must satisfy specific scope requirements.
func SessionForServiceWithScopes(principal, serviceID string, scopes []string) *storage.UserSession {
	s := SessionForService(principal, serviceID)
	s.Scope = scopes
	return s
}

// SessionWithoutRefreshToken returns an active session that does NOT have a refresh token.
func SessionWithoutRefreshToken(principal, serviceID string) *storage.UserSession {
	now := time.Now()
	accessTokenExpires := now.Add(1 * time.Hour)
	svcID := id.MustParseServiceID(serviceID)

	return &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             id.Principal(principal),
		ServiceID:             svcID,
		EncryptedAccessToken:  EncryptedToken(serviceID, "access-token-only"),
		EncryptedRefreshToken: nil,
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  &accessTokenExpires,
		RefreshTokenExpiresAt: nil,
		Scope:                 []string{"read"},
		EncryptionContext: storage.EncryptionContext{
			ServiceID: svcID,
		},
		InitiatedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// ExpiredAccessWithRefresh returns a session whose access token expired while its stored
// refresh token is still considered refreshable, so usability depends on the provider refresh.
func ExpiredAccessWithRefresh(principal, serviceID string, scopes []string) *storage.UserSession {
	session := SessionForServiceWithScopes(principal, serviceID, scopes)
	expired := time.Now().Add(-time.Minute)
	session.AccessTokenExpiresAt = &expired
	return session
}

// RejectedRefreshSession returns a session carrying the persisted provider-rejection marker:
// expired access and a refresh expiry at the rejection time.
func RejectedRefreshSession(principal, serviceID string, scopes []string) *storage.UserSession {
	session := ExpiredAccessWithRefresh(principal, serviceID, scopes)
	rejectedAt := time.Now().Add(-time.Second)
	session.RefreshTokenExpiresAt = &rejectedAt
	return session
}
