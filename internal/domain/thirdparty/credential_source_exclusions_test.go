package thirdparty_test

import (
	"context"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const credentialSourceGoogleDocument = `{"type":"service_account","client_id":"google-client","client_email":"synthetic@example.iam.gserviceaccount.com","private_key":"synthetic-test-key-not-used-for-signing","token_uri":"https://oauth2.googleapis.com/token"}`

type credentialSourceCIMDReadiness struct {
	calls int
}

func (r *credentialSourceCIMDReadiness) RequireUsablePublishedKey(context.Context) error {
	r.calls++
	return nil
}

func excludedCredentialSourceEntity(serviceID id.ServiceID, mode string, source model.CredentialSource) *model.ThirdpartyOAuth2ProviderEntity {
	entity := credentialSourceEntity(serviceID, model.CredentialSourceStored)
	entity.CredentialSource = source
	entity.CredentialSourceProvided = source != ""
	switch mode {
	case "public":
		entity.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
		entity.Secret = model.NewAbsentSecret()
		entity.ClientSecretProvided = false
	case "cimd":
		entity.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodPrivateKeyJWT
		entity.ClientID = ""
		entity.ClientIDProvided = false
		entity.Secret = model.NewAbsentSecret()
		entity.ClientSecretProvided = false
	case "google":
		entity.Flavor = model.OAuth2FlavorGoogle
		entity.ClientID = ""
		entity.ClientIDProvided = false
		entity.Secret = model.NewPlaintextSecret(credentialSourceGoogleDocument)
		entity.IssuerURI = ""
		entity.Discovery.EnableDiscovery = false
	}
	return entity
}

func TestCredentialSourceExcludedStoredModesRetainRegistrationAndUpdateBehavior(t *testing.T) {
	for _, mode := range []string{"public", "cimd", "google"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			fixture := newCredentialSourceFixture(t)
			readiness := &credentialSourceCIMDReadiness{}
			fixture.service.WithCIMDPublicURL("https://broker.example").WithCIMDKeyReadiness(readiness)
			entity := excludedCredentialSourceEntity(id.NewServiceID(), mode, "")
			require.NoError(t, fixture.service.Create(ctx, entity))
			before := persistedCredentialSource(t, fixture, entity.ID)
			assert.Equal(t, model.CredentialSourceStored, before.CredentialSource)
			assert.False(t, before.CredentialSourceTransitioned)

			update := excludedCredentialSourceEntity(entity.ID, mode, "")
			update.DisplayName = "Excluded metadata replacement"
			update.ProtectedResources = nil
			require.NoError(t, fixture.service.Update(ctx, update, nil))
			after := persistedCredentialSource(t, fixture, entity.ID)
			assert.Equal(t, model.CredentialSourceStored, after.CredentialSource)
			assert.False(t, after.CredentialSourceTransitioned)
			assert.Equal(t, before.Version+1, after.Version)
			assert.Equal(t, before.ClientID, after.ClientID)
			assert.Equal(t, before.ProtectedResources, after.ProtectedResources)
			switch mode {
			case "public", "cimd":
				assert.True(t, after.Secret.IsAbsent())
				assert.Zero(t, fixture.crypto.encryptCalls)
				assert.Zero(t, fixture.crypto.decryptCalls)
				if mode == "cimd" {
					expectedClientID, err := model.CIMDClientID("https://broker.example", entity.ID)
					require.NoError(t, err)
					assert.Equal(t, expectedClientID, after.ClientID)
				}
			case "google":
				assert.Equal(t, id.ClientID("google-client"), after.ClientID)
				assert.True(t, after.Secret.IsEncrypted())
				read, err := fixture.service.Get(ctx, entity.ID)
				require.NoError(t, err)
				plaintext, err := read.Secret.GetPlaintext()
				require.NoError(t, err)
				assert.Equal(t, credentialSourceGoogleDocument, plaintext)
			}
		})
	}
}

// T052: a filesystem selector must reject otherwise valid excluded-mode
// requests, rather than accidentally relying on an unrelated invalid field.
func TestCredentialSourceExcludedFilesystemCreateAndUpdateAreRejectedWithoutMutation(t *testing.T) {
	for _, mode := range []string{"public", "cimd", "google"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			fixture := newCredentialSourceFixture(t)
			readiness := &credentialSourceCIMDReadiness{}
			fixture.service.WithCIMDPublicURL("https://broker.example").WithCIMDKeyReadiness(readiness)
			invalid := excludedCredentialSourceEntity(id.NewServiceID(), mode, model.CredentialSourceFilesystem)

			require.Error(t, fixture.service.Create(ctx, invalid))
			assert.Zero(t, fixture.repo.creates)
			assert.Zero(t, fixture.crypto.encryptCalls)
			assert.Zero(t, fixture.keys.calls)
			assert.Zero(t, readiness.calls, "unsupported source must fail before CIMD publication readiness")

			existing := excludedCredentialSourceEntity(invalid.ID, mode, model.CredentialSourceStored)
			require.NoError(t, fixture.service.Create(ctx, existing))
			before := persistedCredentialSource(t, fixture, existing.ID)
			invalid = excludedCredentialSourceEntity(existing.ID, mode, model.CredentialSourceFilesystem)
			invalid.DisplayName = "Rejected excluded replacement"
			invalid.ProtectedResources = []string{"https://api.example/rejected"}
			encryptCalls, decryptCalls, keyCalls, readinessCalls := fixture.crypto.encryptCalls, fixture.crypto.decryptCalls, fixture.keys.calls, readiness.calls

			require.Error(t, fixture.service.Update(ctx, invalid, &before.Version))

			assert.Zero(t, fixture.repo.updates)
			assert.Equal(t, encryptCalls, fixture.crypto.encryptCalls)
			assert.Equal(t, decryptCalls, fixture.crypto.decryptCalls)
			assert.Equal(t, keyCalls, fixture.keys.calls)
			assert.Equal(t, readinessCalls, readiness.calls)
			assertCredentialSourceUnchanged(t, fixture, before)
		})
	}
}

func TestCredentialSourceFilesystemCannotBecomeExcludedThroughOmittedSelector(t *testing.T) {
	for _, mode := range []string{"public", "cimd", "google"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			fixture := newCredentialSourceFixture(t)
			readiness := &credentialSourceCIMDReadiness{}
			fixture.service.WithCIMDPublicURL("https://broker.example").WithCIMDKeyReadiness(readiness)
			entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
			require.NoError(t, fixture.service.Create(ctx, entity))
			before := persistedCredentialSource(t, fixture, entity.ID)
			update := excludedCredentialSourceEntity(entity.ID, mode, "")
			update.DisplayName = "Rejected auth-mode replacement"
			update.ProtectedResources = []string{"https://api.example/rejected"}

			require.Error(t, fixture.service.Update(ctx, update, &before.Version))

			assert.Zero(t, fixture.repo.updates)
			assert.Zero(t, fixture.crypto.encryptCalls)
			assert.Zero(t, fixture.crypto.decryptCalls)
			assert.Zero(t, readiness.calls)
			assertCredentialSourceUnchanged(t, fixture, before)
		})
	}
}
