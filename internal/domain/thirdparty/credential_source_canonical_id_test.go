package thirdparty_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T061: canonical changes are registration mutations, not credential imports or
// implicit source changes. Binding lookup at use is covered by the session tests.
func TestCredentialSourceFilesystemCanonicalClearIsRejectedWithoutMutation(t *testing.T) {
	for _, versioned := range []bool{false, true} {
		name := "unversioned"
		if versioned {
			name = "versioned"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
			require.NoError(t, fixture.service.Create(context.Background(), entity))
			before := persistedCredentialSource(t, fixture, entity.ID)
			update := credentialSourceEntity(entity.ID, model.CredentialSourceFilesystem)
			update.CredentialSource = ""
			update.CredentialSourceProvided = false
			update.CanonicalID = nil
			update.ClearCanonicalID = true
			update.DisplayName = "Rejected canonical clear"
			update.ProtectedResources = []string{"https://api.example/rejected"}
			var expectedVersion *int64
			if versioned {
				expectedVersion = &before.Version
			}

			require.Error(t, fixture.service.Update(context.Background(), update, expectedVersion))

			assert.Zero(t, fixture.repo.updates)
			assert.Zero(t, fixture.crypto.encryptCalls)
			assert.Zero(t, fixture.crypto.decryptCalls)
			assertCredentialSourceUnchanged(t, fixture, before)
		})
	}
}

func TestCredentialSourceFilesystemCanonicalValidationPrecedesPersistence(t *testing.T) {
	for _, canonicalID := range []string{"", " ", "path/segment", strings.Repeat("x", 129), "11111111-1111-4111-8111-111111111111"} {
		t.Run("canonical="+canonicalID, func(t *testing.T) {
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
			entity.CanonicalID = &canonicalID
			require.Error(t, fixture.service.Create(context.Background(), entity))
			assert.Zero(t, fixture.repo.creates)
			assert.Zero(t, fixture.crypto.encryptCalls)
			assert.Zero(t, fixture.keys.calls)

			entity = credentialSourceEntity(entity.ID, model.CredentialSourceFilesystem)
			require.NoError(t, fixture.service.Create(context.Background(), entity))
			before := persistedCredentialSource(t, fixture, entity.ID)
			update := credentialSourceEntity(entity.ID, model.CredentialSourceFilesystem)
			update.CanonicalID = &canonicalID
			update.ProtectedResources = []string{"https://api.example/rejected"}
			require.Error(t, fixture.service.Update(context.Background(), update, &before.Version))
			assert.Zero(t, fixture.repo.updates)
			assertCredentialSourceUnchanged(t, fixture, before)
		})
	}
}

func TestCredentialSourceFilesystemCanonicalReassignmentPreservesSourceAndAbsentCredentials(t *testing.T) {
	for _, versioned := range []bool{false, true} {
		name := "unversioned"
		if versioned {
			name = "versioned"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
			require.NoError(t, fixture.service.Create(ctx, entity))
			before := persistedCredentialSource(t, fixture, entity.ID)
			newCanonicalID := "Different.Provider_ID"
			update := credentialSourceEntity(entity.ID, model.CredentialSourceFilesystem)
			update.CredentialSource = ""
			update.CredentialSourceProvided = false
			update.CanonicalID = &newCanonicalID
			update.DisplayName = "Renamed service"
			var expectedVersion *int64
			if versioned {
				expectedVersion = &before.Version
			} else {
				update.ProtectedResources = nil
			}

			require.NoError(t, fixture.service.Update(ctx, update, expectedVersion))

			after := persistedCredentialSource(t, fixture, entity.ID)
			assert.Equal(t, &newCanonicalID, after.CanonicalID)
			assert.Equal(t, model.CredentialSourceFilesystem, after.CredentialSource)
			assert.True(t, after.ClientID.IsZero())
			assert.True(t, after.Secret.IsAbsent())
			assert.False(t, after.CredentialSourceTransitioned, "canonical reassignment is not a credential-source transition")
			assert.Equal(t, before.Version+1, after.Version)
			assert.Equal(t, before.ProtectedResources, after.ProtectedResources)
			resolved, err := fixture.service.ResolveID(ctx, newCanonicalID)
			require.NoError(t, err)
			assert.Equal(t, entity.ID, resolved)
			_, err = fixture.service.ResolveID(ctx, *before.CanonicalID)
			assert.Error(t, err)
			_, err = fixture.service.ResolveID(ctx, strings.ToLower(newCanonicalID))
			assert.Error(t, err, "canonical matching remains case-sensitive")
			assert.Zero(t, fixture.crypto.encryptCalls)
			assert.Zero(t, fixture.crypto.decryptCalls)
		})
	}
}

func TestCredentialSourceReleasedCanonicalIDDoesNotSelectFilesystemForNewStoredOwner(t *testing.T) {
	ctx := context.Background()
	fixture := newCredentialSourceFixture(t)
	filesystem := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
	require.NoError(t, fixture.service.Create(ctx, filesystem))
	oldCanonicalID := *filesystem.CanonicalID
	newCanonicalID := "moved-filesystem-service"
	update := credentialSourceEntity(filesystem.ID, model.CredentialSourceFilesystem)
	update.CanonicalID = &newCanonicalID
	update.ProtectedResources = nil
	require.NoError(t, fixture.service.Update(ctx, update, nil))

	stored := credentialSourceEntity(id.NewServiceID(), "")
	stored.CanonicalID = &oldCanonicalID
	stored.ClientID = "new-owner-client"
	stored.Secret = model.NewPlaintextSecret("new-owner-secret")
	stored.ProtectedResources = []string{"https://api.example/new-owner"}
	require.NoError(t, fixture.service.Create(ctx, stored))

	filesystemRecord := persistedCredentialSource(t, fixture, filesystem.ID)
	assert.Equal(t, model.CredentialSourceFilesystem, filesystemRecord.CredentialSource)
	assert.True(t, filesystemRecord.Secret.IsAbsent())
	assert.True(t, filesystemRecord.ClientID.IsZero())
	storedRecord, err := fixture.repo.GetByCanonicalID(ctx, oldCanonicalID)
	require.NoError(t, err)
	assert.Equal(t, stored.ID, storedRecord.ID)
	assert.Equal(t, model.CredentialSourceStored, storedRecord.CredentialSource)
	assert.Equal(t, id.ClientID("new-owner-client"), storedRecord.ClientID)
	assert.True(t, storedRecord.Secret.IsEncrypted())
	read, err := fixture.service.Get(ctx, stored.ID)
	require.NoError(t, err)
	secret, err := read.Secret.GetPlaintext()
	require.NoError(t, err)
	assert.Equal(t, "new-owner-secret", secret)
	assert.False(t, storedRecord.CredentialSourceTransitioned)
}

func TestCredentialSourceCanonicalCollisionRetainsFilesystemStateAndIndexes(t *testing.T) {
	ctx := context.Background()
	fixture := newCredentialSourceFixture(t)
	filesystem := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
	require.NoError(t, fixture.service.Create(ctx, filesystem))
	other := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceStored)
	otherCanonical := "already-owned"
	other.CanonicalID = &otherCanonical
	other.ProtectedResources = []string{"https://api.example/other-owner"}
	require.NoError(t, fixture.service.Create(ctx, other))
	before := persistedCredentialSource(t, fixture, filesystem.ID)
	update := credentialSourceEntity(filesystem.ID, model.CredentialSourceFilesystem)
	update.CanonicalID = &otherCanonical
	update.ProtectedResources = []string{"https://api.example/rejected"}

	assertCredentialSourceConflict(t, fixture.service.Update(ctx, update, &before.Version))

	assertCredentialSourceUnchanged(t, fixture, before)
	owner, err := fixture.repo.GetByCanonicalID(ctx, otherCanonical)
	require.NoError(t, err)
	assert.Equal(t, other.ID, owner.ID)
	_, err = fixture.repo.FindByProtectedResource(ctx, "https://api.example/rejected")
	assert.Error(t, err)
}

func TestCredentialSourceStoredCanonicalClearRetainsExistingMetadataSemantics(t *testing.T) {
	ctx := context.Background()
	fixture := newCredentialSourceFixture(t)
	entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceStored)
	require.NoError(t, fixture.service.Create(ctx, entity))
	before := persistedCredentialSource(t, fixture, entity.ID)
	update := credentialSourceEntity(entity.ID, model.CredentialSourceStored)
	update.CanonicalID = nil
	update.ClearCanonicalID = true
	update.CredentialSource = ""
	update.CredentialSourceProvided = false
	update.ProtectedResources = nil

	require.NoError(t, fixture.service.Update(ctx, update, nil))

	after := persistedCredentialSource(t, fixture, entity.ID)
	assert.Nil(t, after.CanonicalID)
	assert.Equal(t, model.CredentialSourceStored, after.CredentialSource)
	assert.Equal(t, before.ClientID, after.ClientID)
	assert.True(t, after.Secret.IsEncrypted())
	assert.False(t, after.CredentialSourceTransitioned)
	assert.Equal(t, before.ProtectedResources, after.ProtectedResources)
	assert.Equal(t, before.Version+1, after.Version)
	_, err := fixture.service.ResolveID(ctx, *before.CanonicalID)
	assert.Error(t, err)
}

func TestCredentialSourceTransitionWithOmittedCanonicalIDPreservesExistingSelector(t *testing.T) {
	fixture := newCredentialSourceFixture(t)
	entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceStored)
	require.NoError(t, fixture.service.Create(context.Background(), entity))
	before := persistedCredentialSource(t, fixture, entity.ID)
	update := credentialSourceEntity(entity.ID, model.CredentialSourceFilesystem)
	update.CanonicalID = nil
	update.ProtectedResources = nil

	require.NoError(t, fixture.service.Update(context.Background(), update, nil))

	after := persistedCredentialSource(t, fixture, entity.ID)
	assert.Equal(t, before.CanonicalID, after.CanonicalID)
	assert.Equal(t, model.CredentialSourceFilesystem, after.CredentialSource)
	assert.True(t, after.Secret.IsAbsent())
	assert.True(t, after.ClientID.IsZero())
	assert.True(t, after.CredentialSourceTransitioned)
	assert.Equal(t, before.ProtectedResources, after.ProtectedResources)
}
