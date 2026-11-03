package principal

import (
	"context"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string {
	return &s
}

func TestPrincipalProfile_Construction(t *testing.T) {
	tests := []struct {
		name            string
		principal       id.Principal
		displayName     string
		email           *string
		pictureURL      *string
		expectedDisplay string
	}{
		{
			name:            "basic construction with principal only",
			principal:       "alice@example.com",
			expectedDisplay: "alice@example.com",
		},
		{
			name:            "with explicit display name",
			principal:       "alice@example.com",
			displayName:     "Alice Smith",
			expectedDisplay: "Alice Smith",
		},
		{
			name:            "with all fields",
			principal:       "alice@example.com",
			displayName:     "Alice Smith",
			email:           strPtr("alice@example.com"),
			pictureURL:      strPtr("https://example.com/alice.jpg"),
			expectedDisplay: "Alice Smith",
		},
		{
			name:            "empty display name falls back to principal",
			principal:       "bob",
			displayName:     "",
			expectedDisplay: "bob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := NewProfile(tt.principal)

			if tt.displayName != "" {
				profile = profile.WithDisplayName(tt.displayName)
			}
			if tt.email != nil {
				profile = profile.WithEmail(tt.email)
			}
			if tt.pictureURL != nil {
				profile = profile.WithPictureURL(tt.pictureURL)
			}

			assert.Equal(t, tt.principal, profile.Principal())
			assert.Equal(t, tt.expectedDisplay, profile.DisplayName())
			assert.Equal(t, tt.email, profile.Email())
			assert.Equal(t, tt.pictureURL, profile.PictureURL())
		})
	}
}

func TestPrincipalProfile_DisplayNameDefaultsToPrincipal(t *testing.T) {
	profile := NewProfile("john@example.com")
	assert.Equal(t, "john@example.com", profile.DisplayName(),
		"DisplayName should default to Principal when not set")
}

func TestPrincipalProfile_WithDisplayNameEmpty(t *testing.T) {
	profile := NewProfile("user123").WithDisplayName("")
	assert.Equal(t, "user123", profile.DisplayName(),
		"Empty display name should fall back to principal")
}

func TestPrincipalProfile_NilOptionalFields(t *testing.T) {
	profile := NewProfile("user123")
	assert.Nil(t, profile.Email(), "Email should be nil by default")
	assert.Nil(t, profile.PictureURL(), "PictureURL should be nil by default")
}

func TestPrincipalProfile_ContextStorageRetrieval(t *testing.T) {
	t.Run("store and retrieve profile from context", func(t *testing.T) {
		email := "alice@example.com"
		picture := "https://example.com/alice.jpg"
		profile := NewProfile("alice").
			WithDisplayName("Alice Smith").
			WithEmail(&email).
			WithPictureURL(&picture)

		ctx := WithProfile(context.Background(), profile)

		retrieved, ok := ProfileFromContext(ctx)
		require.True(t, ok, "ProfileFromContext should return true when profile is set")
		assert.Equal(t, id.Principal("alice"), retrieved.Principal())
		assert.Equal(t, "Alice Smith", retrieved.DisplayName())
		assert.Equal(t, &email, retrieved.Email())
		assert.Equal(t, &picture, retrieved.PictureURL())
	})

	t.Run("retrieve from context without profile returns false", func(t *testing.T) {
		ctx := context.Background()
		_, ok := ProfileFromContext(ctx)
		assert.False(t, ok, "ProfileFromContext should return false when no profile is set")
	})

	t.Run("profile and principal context keys are independent", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithPrincipal(ctx, "legacy-user")

		profile := NewProfile("new-user").WithDisplayName("New User")
		ctx = WithProfile(ctx, profile)

		principalValue, ok := FromContext(ctx)
		require.True(t, ok)
		assert.Equal(t, id.Principal("legacy-user"), principalValue)

		profileVal, ok := ProfileFromContext(ctx)
		require.True(t, ok)
		assert.Equal(t, id.Principal("new-user"), profileVal.Principal())
		assert.Equal(t, "New User", profileVal.DisplayName())
	})
}

func TestPrincipalProfile_Immutability(t *testing.T) {
	original := NewProfile("user123")
	modified := original.WithDisplayName("Modified")

	assert.Equal(t, "user123", original.DisplayName(),
		"Original should not be mutated")
	assert.Equal(t, "Modified", modified.DisplayName(),
		"Modified copy should have new display name")
}
