package memory

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthorizationCodeStore_FindByCodeHash_Expiry(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		expiresAt time.Time
		used      bool
		wantFound bool
	}{
		{name: "active unused", expiresAt: now.Add(time.Hour), wantFound: true},
		{name: "active used", expiresAt: now.Add(time.Hour), used: true, wantFound: true},
		{name: "expired unused", expiresAt: now.Add(-time.Minute)},
		{name: "expired used", expiresAt: now.Add(-time.Minute), used: true},
		{name: "expires now", expiresAt: now},
		{name: "missing expiry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewAuthorizationCodeStore()
			ctx := context.Background()
			code := &storage.AuthorizationCode{
				ID:            id.NewAuthorizationCodeID(),
				CodeHash:      "codehash",
				AgentID:       id.NewAgentID(),
				Principal:     id.NewPrincipal("user@example.com"),
				RedirectURI:   "http://localhost/callback",
				CodeChallenge: "challenge",
				Scope:         "read",
				ExpiresAt:     tt.expiresAt,
				CreatedAt:     now.Add(-time.Hour),
			}
			require.NoError(t, store.Create(ctx, code))
			if tt.used {
				require.NoError(t, store.MarkUsed(ctx, code.ID))
			}

			got, err := store.FindByCodeHash(ctx, code.CodeHash)
			if !tt.wantFound {
				require.Error(t, err)
				var storageErr *storage.StorageError
				require.ErrorAs(t, err, &storageErr)
				assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, code.ID, got.ID)
			assert.Equal(t, tt.used, got.UsedAt != nil)
		})
	}
}
