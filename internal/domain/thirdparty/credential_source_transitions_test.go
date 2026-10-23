package thirdparty_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T055: transitions use the same Update operation as ordinary replacement; no
// second transition API or file-import path is part of the registration contract.
func TestCredentialSourceTransitionsCommitCredentialsHistoryAndResourcesTogether(t *testing.T) {
	for _, initialSource := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		t.Run(string(initialSource), func(t *testing.T) {
			ctx := context.Background()
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), initialSource)
			require.NoError(t, fixture.service.Create(ctx, entity))
			before := persistedCredentialSource(t, fixture, entity.ID)
			nextSource := model.CredentialSourceFilesystem
			if initialSource == model.CredentialSourceFilesystem {
				nextSource = model.CredentialSourceStored
			}
			update := credentialSourceEntity(entity.ID, nextSource)
			update.ProtectedResources = []string{"https://api.example/replacement"}
			if nextSource == model.CredentialSourceStored {
				update.ClientID = "explicit-reverse-client"
				update.Secret = model.NewPlaintextSecret("explicit-reverse-secret")
			}
			encryptCalls := fixture.crypto.encryptCalls

			require.NoError(t, fixture.service.Update(ctx, update, &before.Version))

			after := persistedCredentialSource(t, fixture, entity.ID)
			assert.Equal(t, nextSource, after.CredentialSource)
			assert.True(t, after.CredentialSourceTransitioned)
			assert.Equal(t, before.Version+1, after.Version)
			assert.Equal(t, before.ID, after.ID)
			assert.Equal(t, before.CanonicalID, after.CanonicalID)
			assert.Equal(t, before.CreatedAt, after.CreatedAt)
			assert.Equal(t, update.ProtectedResources, after.ProtectedResources)
			owner, err := fixture.repo.FindByProtectedResource(ctx, "https://api.example/replacement")
			require.NoError(t, err)
			assert.Equal(t, entity.ID, owner.ID)
			_, err = fixture.repo.FindByProtectedResource(ctx, "https://api.example/original")
			assert.Error(t, err, "successful versioned replacement releases the old resource index")
			if nextSource == model.CredentialSourceFilesystem {
				assert.True(t, after.ClientID.IsZero())
				assert.True(t, after.Secret.IsAbsent())
				_, err := after.Secret.GetCiphertext()
				assert.Error(t, err, "old ciphertext must be removed, not retained behind filesystem selection")
				assert.Equal(t, encryptCalls, fixture.crypto.encryptCalls)
			} else {
				assert.Equal(t, id.ClientID("explicit-reverse-client"), after.ClientID)
				require.True(t, after.Secret.IsEncrypted())
				ciphertext, err := after.Secret.GetCiphertext()
				require.NoError(t, err)
				assert.NotEqual(t, []byte("explicit-reverse-secret"), ciphertext)
				plaintext, err := fixture.crypto.EncryptionPort.Decrypt(ctx, ciphertext, map[string]string{"service_id": entity.ID.String()})
				require.NoError(t, err)
				assert.Equal(t, "explicit-reverse-secret", string(plaintext))
				_, err = fixture.crypto.EncryptionPort.Decrypt(ctx, ciphertext, map[string]string{"service_id": id.NewServiceID().String()})
				assert.Error(t, err, "reverse credentials retain the established service-ID encryption context")
			}
		})
	}
}

func TestCredentialSourceTransitionCASConflictRetainsOldCredentialsHistoryAndIndexes(t *testing.T) {
	for _, test := range []struct {
		source       model.CredentialSource
		transitioned bool
	}{
		{model.CredentialSourceStored, false},
		{model.CredentialSourceStored, true},
		{model.CredentialSourceFilesystem, false},
		{model.CredentialSourceFilesystem, true},
	} {
		initialSource := test.source
		name := string(initialSource) + "/never transitioned"
		if test.transitioned {
			name = string(initialSource) + "/round trip"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), initialSource)
			require.NoError(t, fixture.service.Create(ctx, entity))
			if test.transitioned {
				oppositeSource := model.CredentialSourceFilesystem
				if initialSource == model.CredentialSourceFilesystem {
					oppositeSource = model.CredentialSourceStored
				}
				for _, source := range []model.CredentialSource{oppositeSource, initialSource} {
					current := persistedCredentialSource(t, fixture, entity.ID)
					roundTrip := credentialSourceEntity(entity.ID, source)
					require.NoError(t, fixture.service.Update(ctx, roundTrip, &current.Version))
				}
			}
			current := persistedCredentialSource(t, fixture, entity.ID)
			assert.Equal(t, test.transitioned, current.CredentialSourceTransitioned)
			staleVersion := current.Version
			_, err := fixture.repo.AddProtectedResource(ctx, entity.ID, "https://api.example/concurrent")
			require.NoError(t, err)
			before := persistedCredentialSource(t, fixture, entity.ID)
			nextSource := model.CredentialSourceFilesystem
			if initialSource == model.CredentialSourceFilesystem {
				nextSource = model.CredentialSourceStored
			}
			update := credentialSourceEntity(entity.ID, nextSource)
			newCanonicalID := "rejected-canonical"
			update.CanonicalID = &newCanonicalID
			update.ProtectedResources = []string{"https://api.example/rejected"}

			err = fixture.service.Update(ctx, update, &staleVersion)

			assertCredentialSourceConflict(t, err)
			assertCredentialSourceUnchanged(t, fixture, before)
			_, err = fixture.repo.GetByCanonicalID(ctx, newCanonicalID)
			assert.Error(t, err)
			_, err = fixture.repo.FindByProtectedResource(ctx, "https://api.example/rejected")
			assert.Error(t, err)
		})
	}
}

func TestCredentialSourceTransitionResourceConflictRollsBackCredentialsAndHistory(t *testing.T) {
	for _, initialSource := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		t.Run(string(initialSource), func(t *testing.T) {
			ctx := context.Background()
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), initialSource)
			require.NoError(t, fixture.service.Create(ctx, entity))
			other := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceStored)
			otherCanonical := "other-service"
			other.CanonicalID = &otherCanonical
			other.ProtectedResources = []string{"https://api.example/owned"}
			require.NoError(t, fixture.service.Create(ctx, other))
			before := persistedCredentialSource(t, fixture, entity.ID)
			nextSource := model.CredentialSourceFilesystem
			if initialSource == model.CredentialSourceFilesystem {
				nextSource = model.CredentialSourceStored
			}
			update := credentialSourceEntity(entity.ID, nextSource)
			update.ProtectedResources = other.ProtectedResources

			assertCredentialSourceConflict(t, fixture.service.Update(ctx, update, &before.Version))

			assertCredentialSourceUnchanged(t, fixture, before)
			owner, err := fixture.repo.FindByProtectedResource(ctx, other.ProtectedResources[0])
			require.NoError(t, err)
			assert.Equal(t, other.ID, owner.ID)
		})
	}
}

func TestCredentialSourceUnversionedTransitionsPreserveAuthoritativeResources(t *testing.T) {
	for _, initialSource := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		for _, requested := range [][]string{nil, {}, {"https://api.example/not-a-replacement"}} {
			t.Run(string(initialSource)+"/resources="+stringResourceCase(requested), func(t *testing.T) {
				ctx := context.Background()
				fixture := newCredentialSourceFixture(t)
				entity := credentialSourceEntity(id.NewServiceID(), initialSource)
				require.NoError(t, fixture.service.Create(ctx, entity))
				_, err := fixture.repo.AddProtectedResource(ctx, entity.ID, "https://api.example/child")
				require.NoError(t, err)
				before := persistedCredentialSource(t, fixture, entity.ID)
				nextSource := model.CredentialSourceFilesystem
				if initialSource == model.CredentialSourceFilesystem {
					nextSource = model.CredentialSourceStored
				}
				update := credentialSourceEntity(entity.ID, nextSource)
				update.ProtectedResources = requested

				require.NoError(t, fixture.service.Update(ctx, update, nil))

				after := persistedCredentialSource(t, fixture, entity.ID)
				assert.Equal(t, nextSource, after.CredentialSource)
				assert.True(t, after.CredentialSourceTransitioned)
				assert.Equal(t, before.Version+1, after.Version)
				assert.Equal(t, before.ProtectedResources, after.ProtectedResources, "internal source guarding must not turn a nil-version metadata update into resource replacement")
				for _, resource := range before.ProtectedResources {
					owner, err := fixture.repo.FindByProtectedResource(ctx, resource)
					require.NoError(t, err)
					assert.Equal(t, entity.ID, owner.ID)
				}
				_, err = fixture.repo.FindByProtectedResource(ctx, "https://api.example/not-a-replacement")
				assert.Error(t, err)
			})
		}
	}
}

func stringResourceCase(resources []string) string {
	if resources == nil {
		return "omitted"
	}
	if len(resources) == 0 {
		return "empty"
	}
	return "supplied"
}

func TestCredentialSourceOmittedUnversionedUpdateCannotOverwriteConcurrentTransition(t *testing.T) {
	ctx := context.Background()
	fixture := newCredentialSourceFixture(t)
	entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceStored)
	require.NoError(t, fixture.service.Create(ctx, entity))
	before := persistedCredentialSource(t, fixture, entity.ID)
	staleMetadata := credentialSourceEntity(entity.ID, model.CredentialSourceStored)
	staleMetadata.CredentialSource = ""
	staleMetadata.CredentialSourceProvided = false
	staleMetadata.DisplayName = "Stale metadata"
	staleMetadata.ProtectedResources = nil
	var winner *model.ThirdpartyOAuth2ProviderEntity
	fixture.repo.afterGet = func() {
		transition := credentialSourceEntity(entity.ID, model.CredentialSourceFilesystem)
		transition.ProtectedResources = []string{"https://api.example/winner"}
		require.NoError(t, fixture.service.Update(ctx, transition, &before.Version))
		winner = persistedCredentialSource(t, fixture, entity.ID)
	}

	err := fixture.service.Update(ctx, staleMetadata, nil)

	require.NotNil(t, winner, "source preservation must use the persisted source, not the request's zero value")
	assertCredentialSourceConflict(t, err)
	assertCredentialSourceUnchanged(t, fixture, winner)
	assert.Equal(t, model.CredentialSourceFilesystem, winner.CredentialSource)
	assert.True(t, winner.Secret.IsAbsent())
	assert.True(t, winner.CredentialSourceTransitioned)
}

func TestCredentialSourceReverseTransitionRequiresExplicitNonemptyCredentials(t *testing.T) {
	cases := []struct {
		name       string
		invalidate func(*model.ThirdpartyOAuth2ProviderEntity)
	}{
		{"both omitted", func(e *model.ThirdpartyOAuth2ProviderEntity) {
			e.ClientID = ""
			e.Secret = model.NewAbsentSecret()
			e.ClientIDProvided = false
			e.ClientSecretProvided = false
		}},
		{"client ID omitted", func(e *model.ThirdpartyOAuth2ProviderEntity) { e.ClientID = ""; e.ClientIDProvided = false }},
		{"client ID empty", func(e *model.ThirdpartyOAuth2ProviderEntity) { e.ClientID = "" }},
		{"secret omitted", func(e *model.ThirdpartyOAuth2ProviderEntity) {
			e.Secret = model.NewAbsentSecret()
			e.ClientSecretProvided = false
		}},
		{"secret null", func(e *model.ThirdpartyOAuth2ProviderEntity) { e.Secret = model.NewAbsentSecret() }},
		{"secret empty", func(e *model.ThirdpartyOAuth2ProviderEntity) { e.Secret = model.NewPlaintextSecret("") }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
			require.NoError(t, fixture.service.Create(context.Background(), entity))
			before := persistedCredentialSource(t, fixture, entity.ID)
			update := credentialSourceEntity(entity.ID, model.CredentialSourceStored)
			test.invalidate(update)
			update.ProtectedResources = []string{"https://api.example/rejected"}

			require.Error(t, fixture.service.Update(context.Background(), update, &before.Version))

			assert.Zero(t, fixture.repo.updates)
			assert.Zero(t, fixture.crypto.encryptCalls)
			assertCredentialSourceUnchanged(t, fixture, before)
		})
	}
}

func TestCredentialSourceReverseEncryptionFailureDoesNotCommitSourceOrHistory(t *testing.T) {
	fixture := newCredentialSourceFixture(t)
	entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
	require.NoError(t, fixture.service.Create(context.Background(), entity))
	before := persistedCredentialSource(t, fixture, entity.ID)
	failure := errors.New("test encryption unavailable")
	fixture.crypto.encryptErr = failure
	update := credentialSourceEntity(entity.ID, model.CredentialSourceStored)
	update.ProtectedResources = []string{"https://api.example/rejected"}

	require.ErrorIs(t, fixture.service.Update(context.Background(), update, &before.Version), failure)

	assert.Zero(t, fixture.repo.updates)
	assertCredentialSourceUnchanged(t, fixture, before)
}

func TestCredentialSourceHistorySurvivesNoopMetadataAndRoundTrip(t *testing.T) {
	for _, initialSource := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		t.Run(string(initialSource), func(t *testing.T) {
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), initialSource)
			require.NoError(t, fixture.service.Create(context.Background(), entity))
			otherSource := model.CredentialSourceFilesystem
			if initialSource == model.CredentialSourceFilesystem {
				otherSource = model.CredentialSourceStored
			}
			steps := []struct {
				source       model.CredentialSource
				omitted      bool
				transitioned bool
			}{
				{initialSource, false, false},
				{initialSource, true, false},
				{otherSource, false, true},
				{otherSource, false, true},
				{otherSource, true, true},
				{initialSource, false, true},
				{initialSource, false, true},
				{initialSource, true, true},
			}
			for _, step := range steps {
				update := credentialSourceEntity(entity.ID, step.source)
				update.DisplayName = "Metadata replacement"
				update.CredentialSourceTransitioned = false // fresh request must not reset durable history
				update.ProtectedResources = nil
				if step.omitted {
					update.CredentialSource = ""
					update.CredentialSourceProvided = false
				}
				require.NoError(t, fixture.service.Update(context.Background(), update, nil))
				stored := persistedCredentialSource(t, fixture, entity.ID)
				assert.Equal(t, step.source, stored.CredentialSource)
				assert.Equal(t, step.transitioned, stored.CredentialSourceTransitioned)
			}
		})
	}
}
