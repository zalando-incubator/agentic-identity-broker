package fixtures

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// ValidSession has an access token that expires well after the default refresh lookahead.
func ValidSession(principal string, serviceID id.ServiceID, now time.Time) *storage.UserSession {
	access := now.Add(2 * time.Hour)
	refresh := now.Add(30 * 24 * time.Hour)
	return healthSession(principal, serviceID, now, &access, &refresh, true)
}

// ExpiringRefreshableSession is due within a five-minute lookahead and has a usable refresh token.
func ExpiringRefreshableSession(principal string, serviceID id.ServiceID, now time.Time) *storage.UserSession {
	access := now.Add(2 * time.Minute)
	refresh := now.Add(30 * 24 * time.Hour)
	return healthSession(principal, serviceID, now, &access, &refresh, true)
}

// ExpiredRefreshableSession has an expired access token and a usable refresh token.
func ExpiredRefreshableSession(principal string, serviceID id.ServiceID, now time.Time) *storage.UserSession {
	access := now.Add(-time.Hour)
	refresh := now.Add(30 * 24 * time.Hour)
	return healthSession(principal, serviceID, now, &access, &refresh, true)
}

// ExpiredUnrefreshableSession has an expired access token and an expired refresh token.
func ExpiredUnrefreshableSession(principal string, serviceID id.ServiceID, now time.Time) *storage.UserSession {
	access := now.Add(-7 * time.Hour)
	refresh := now.Add(-10 * time.Minute)
	return healthSession(principal, serviceID, now, &access, &refresh, true)
}

// ExpiredWithoutRefreshSession has an expired access token and no refresh token.
func ExpiredWithoutRefreshSession(principal string, serviceID id.ServiceID, now time.Time) *storage.UserSession {
	access := now.Add(-4 * time.Hour)
	return healthSession(principal, serviceID, now, &access, nil, false)
}

// UnknownExpirySession has no recorded access-token expiry.
func UnknownExpirySession(principal string, serviceID id.ServiceID, now time.Time) *storage.UserSession {
	refresh := now.Add(30 * 24 * time.Hour)
	return healthSession(principal, serviceID, now, nil, &refresh, true)
}

// HealthJourneySessions reproduces the eight sessions of Journey 2 in
// specs/053-session-auto-refresh/spec.md: six for calendar, two for tickets.
func HealthJourneySessions(now time.Time, calendar, tickets id.ServiceID) []*storage.UserSession {
	return []*storage.UserSession{
		ValidSession("u-7f3a@example.com", calendar, now),
		ExpiringRefreshableSession("u-0b19@example.com", calendar, now),
		ExpiredUnrefreshableSession("u-5c44@example.com", calendar, now),
		ExpiredRefreshableSession("u-3c21@example.com", calendar, now),
		UnknownExpirySession("u-9e01@example.com", calendar, now),
		ValidSession("u-2d87@example.com", calendar, now),
		ValidSession("u-7f3a@example.com", tickets, now),
		ExpiredWithoutRefreshSession("u-4a60@example.com", tickets, now),
	}
}

func healthSession(principal string, serviceID id.ServiceID, now time.Time, accessExpiry, refreshExpiry *time.Time, withRefresh bool) *storage.UserSession {
	session := &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             id.Principal(principal),
		ServiceID:             serviceID,
		EncryptedAccessToken:  EncryptedToken(serviceID.String(), "health-access-"+principal),
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  accessExpiry,
		RefreshTokenExpiresAt: refreshExpiry,
		Scope:                 []string{"read"},
		EncryptionContext:     storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:           now.Add(-8 * time.Hour),
		CreatedAt:             now.Add(-8 * time.Hour),
		UpdatedAt:             now.Add(-8 * time.Hour),
	}
	if withRefresh {
		session.EncryptedRefreshToken = EncryptedToken(serviceID.String(), "health-refresh-"+principal)
	}
	return session
}
