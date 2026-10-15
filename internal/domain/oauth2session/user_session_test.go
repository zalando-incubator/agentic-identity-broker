package oauth2session_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestUserSession_Validate(t *testing.T) {
	tests := []struct {
		name    string
		session *storage.UserSession
		wantErr bool
	}{
		{
			name: "valid session",
			session: &storage.UserSession{
				ID:                   id.NewSessionID(),
				Principal:            id.Principal("user@example.com"),
				ServiceID:            id.NewServiceID(),
				EncryptedAccessToken: []byte("encrypted-token"),
				TokenType:            "Bearer",
			},
			wantErr: false,
		},
		{
			name: "empty ID",
			session: &storage.UserSession{
				Principal:            id.Principal("user@example.com"),
				ServiceID:            id.NewServiceID(),
				EncryptedAccessToken: []byte("token"),
				TokenType:            "Bearer",
			},
			wantErr: true,
		},
		{
			name: "empty principal",
			session: &storage.UserSession{
				ID:                   id.NewSessionID(),
				ServiceID:            id.NewServiceID(),
				EncryptedAccessToken: []byte("token"),
				TokenType:            "Bearer",
			},
			wantErr: true,
		},
		{
			name: "principal exceeds 200 chars",
			session: &storage.UserSession{
				ID:                   id.NewSessionID(),
				Principal:            id.Principal(string(make([]byte, 201))),
				ServiceID:            id.NewServiceID(),
				EncryptedAccessToken: []byte("token"),
				TokenType:            "Bearer",
			},
			wantErr: true,
		},
		{
			name: "empty service ID",
			session: &storage.UserSession{
				ID:                   id.NewSessionID(),
				Principal:            id.Principal("user@example.com"),
				EncryptedAccessToken: []byte("token"),
				TokenType:            "Bearer",
			},
			wantErr: true,
		},
		{
			name: "empty encrypted token",
			session: &storage.UserSession{
				ID:        id.NewSessionID(),
				Principal: id.Principal("user@example.com"),
				ServiceID: id.NewServiceID(),
				TokenType: "Bearer",
			},
			wantErr: true,
		},
		{
			name: "empty token type",
			session: &storage.UserSession{
				ID:                   id.NewSessionID(),
				Principal:            id.Principal("user@example.com"),
				ServiceID:            id.NewServiceID(),
				EncryptedAccessToken: []byte("token"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.session.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUserSession_RequiresReauthentication(t *testing.T) {
	future := ptrTime(time.Now().Add(time.Hour))
	past := ptrTime(time.Now().Add(-time.Hour))
	for _, tc := range []struct {
		name    string
		session storage.UserSession
		want    bool
	}{
		{name: "unknown access expiry", session: storage.UserSession{}},
		{name: "valid access only", session: storage.UserSession{AccessTokenExpiresAt: future}},
		{name: "expired access only", session: storage.UserSession{AccessTokenExpiresAt: past}, want: true},
		{name: "expired access with refresh authority", session: storage.UserSession{AccessTokenExpiresAt: past, EncryptedRefreshToken: []byte("refresh")}},
		{name: "rejected with valid access", session: storage.UserSession{AccessTokenExpiresAt: future, EncryptedRefreshToken: []byte("refresh"), RefreshTokenRejected: true}, want: true},
		{name: "rejected with unknown access expiry", session: storage.UserSession{RefreshTokenRejected: true}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.session.RequiresReauthentication())
		})
	}
}

func TestUserSession_CanRefresh(t *testing.T) {
	for _, tc := range []struct {
		name     string
		token    []byte
		rejected bool
		want     bool
	}{
		{name: "absent"},
		{name: "empty", token: []byte{}},
		{name: "stored", token: []byte("ciphertext"), want: true},
		{name: "stored but rejected", token: []byte("ciphertext"), rejected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := storage.UserSession{EncryptedRefreshToken: tc.token, RefreshTokenRejected: tc.rejected}
			assert.Equal(t, tc.want, session.CanRefresh())
		})
	}
}

func TestUserSession_HasValidAccessToken(t *testing.T) {
	tests := []struct {
		name    string
		session *storage.UserSession
		want    bool
	}{
		{
			name: "no expiration",
			session: &storage.UserSession{
				AccessTokenExpiresAt: nil,
			},
			want: true,
		},
		{
			name: "valid",
			session: &storage.UserSession{
				AccessTokenExpiresAt: ptrTime(time.Now().Add(1 * time.Hour)),
			},
			want: true,
		},
		{
			name: "expired",
			session: &storage.UserSession{
				AccessTokenExpiresAt: ptrTime(time.Now().Add(-1 * time.Hour)),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.session.HasValidAccessToken())
		})
	}
}

// Helper function
func ptrTime(t time.Time) *time.Time {
	return &t
}
