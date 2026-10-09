//go:build integration

package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/postgres"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type credentialSourcePostgresHarness struct {
	ctx       context.Context
	providers *postgres.PostgresThirdpartyOAuth2ProviderRepository
	sessions  *postgres.PostgresUserSessionRepository
	db        *sql.DB
}

func setupCredentialSourcePostgresHarness(t *testing.T) *credentialSourcePostgresHarness {
	t.Helper()
	// Reuse the suite's shared container, 035 template and adapter lifecycle.
	ctx, providers, _, shared, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	t.Cleanup(cleanup)
	projectRoot, err := bootstrap.FindProjectRoot()
	require.NoError(t, err)
	runner, err := migrate.New("file://"+filepath.Join(projectRoot, "migrations"), shared.ConnectionString(dbName))
	require.NoError(t, err)
	err = runner.Migrate(36)
	_, _ = runner.Close()
	if err != migrate.ErrNoChange {
		require.NoError(t, err, "migration 036 must provide the source/session schema")
	}
	adapter, err := postgres.NewAdapter(&ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: shared.ConnectionString(dbName)}})
	require.NoError(t, err)
	require.NoError(t, adapter.Initialize(ctx))
	t.Cleanup(func() { require.NoError(t, adapter.Close(ctx)) })
	db, err := sql.Open("pgx", shared.ConnectionString(dbName))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return &credentialSourcePostgresHarness{ctx: ctx, providers: providers, sessions: postgres.NewUserSessionRepository(adapter), db: db}
}

func credentialSourcePostgresProvider(source model.CredentialSource, resources ...string) *model.ThirdpartyOAuth2ProviderEntity {
	provider := createTestService(id.NewServiceID().String(), "Credential source", resources)
	canonicalID := "credential-source-" + provider.ID.String()
	provider.CanonicalID = &canonicalID
	provider.CredentialSource = source
	provider.Flavor = model.OAuth2FlavorStandard
	provider.Secret = model.NewEncryptedSecret([]byte("opaque-stored-ciphertext"))
	if source == model.CredentialSourceFilesystem {
		provider.ClientID, provider.Secret = "", model.NewAbsentSecret()
	}
	return provider
}

func requireCredentialSourcePostgresKind(t *testing.T, err error, kind storage.ErrorKind) {
	t.Helper()
	require.Error(t, err)
	var storageError *storage.StorageError
	require.True(t, errors.As(err, &storageError))
	assert.Equal(t, kind, storageError.Kind)
}

func assertCredentialSourcePostgresRow(t *testing.T, h *credentialSourcePostgresHarness, expected *model.ThirdpartyOAuth2ProviderEntity) {
	t.Helper()
	var source string
	var marker bool
	var clientID, method sql.NullString
	var ciphertext []byte
	var version int64
	require.NoError(t, h.db.QueryRowContext(h.ctx, `SELECT credential_source, credential_source_transitioned, client_id, client_secret_encrypted, token_endpoint_auth_method, version
		FROM thirdparty_oauth2_services WHERE id = $1`, expected.ID).Scan(&source, &marker, &clientID, &ciphertext, &method, &version))
	assert.Equal(t, string(expected.CredentialSource), source)
	assert.Equal(t, expected.CredentialSourceTransitioned, marker)
	assert.Equal(t, expected.Version, version)
	if expected.CredentialSource == model.CredentialSourceFilesystem {
		assert.False(t, clientID.Valid, "filesystem writes SQL NULL, not empty client ID")
		assert.Nil(t, ciphertext, "filesystem writes SQL NULL, not empty/encrypted placeholder")
		assert.False(t, method.Valid, "shared-secret default remains SQL NULL")
	} else {
		assert.True(t, clientID.Valid)
		assert.Equal(t, expected.ClientID.String(), clientID.String)
		if expected.Secret.IsAbsent() {
			assert.Nil(t, ciphertext)
		} else {
			wantCiphertext, err := expected.Secret.GetCiphertext()
			require.NoError(t, err)
			assert.Equal(t, wantCiphertext, ciphertext)
		}
	}
}

func TestCredentialSourcePostgresRepository(t *testing.T) {
	h := setupCredentialSourcePostgresHarness(t)
	t.Run("source and absent credentials survive every projection", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			source model.CredentialSource
			method model.TokenEndpointAuthMethod
			flavor model.OAuth2Flavor
			absent bool
		}{
			{"stored shared secret", model.CredentialSourceStored, "", model.OAuth2FlavorStandard, false},
			{"filesystem standard", model.CredentialSourceFilesystem, "", model.OAuth2FlavorStandard, true},
			{"filesystem github", model.CredentialSourceFilesystem, "", model.OAuth2FlavorGitHub, true},
			{"stored public", model.CredentialSourceStored, model.TokenEndpointAuthMethodNone, model.OAuth2FlavorStandard, true},
			{"stored CIMD", model.CredentialSourceStored, cimdPrivateKeyJWTAuthMethod, model.OAuth2FlavorStandard, true},
			{"stored Google", model.CredentialSourceStored, "", model.OAuth2FlavorGoogle, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resourceURI := "https://api.example.test/" + id.NewServiceID().String()
				provider := credentialSourcePostgresProvider(tc.source, resourceURI)
				provider.TokenEndpointAuthMethod, provider.Flavor = tc.method, tc.flavor
				if tc.absent {
					provider.Secret = model.NewAbsentSecret()
				}
				if tc.method == cimdPrivateKeyJWTAuthMethod {
					provider.ClientID = cimdClientIDForService(provider.ID)
				}
				require.NoError(t, h.providers.Create(h.ctx, provider))
				assertCredentialSourcePostgresRow(t, h, provider)
				get, err := h.providers.Get(h.ctx, provider.ID)
				require.NoError(t, err)
				canonical, err := h.providers.GetByCanonicalID(h.ctx, *provider.CanonicalID)
				require.NoError(t, err)
				resource, err := h.providers.FindByProtectedResource(h.ctx, resourceURI)
				require.NoError(t, err)
				listed, err := h.providers.List(h.ctx)
				require.NoError(t, err)
				var listedProvider *model.ThirdpartyOAuth2ProviderEntity
				for _, item := range listed {
					if item.ID == provider.ID {
						listedProvider = item
						break
					}
				}
				require.NotNil(t, listedProvider)
				for _, stored := range []*model.ThirdpartyOAuth2ProviderEntity{get, canonical, resource, listedProvider} {
					assert.Equal(t, tc.source, stored.CredentialSource)
					assert.False(t, stored.CredentialSourceTransitioned)
					assert.Equal(t, provider.ClientID, stored.ClientID)
					assert.Equal(t, provider.Secret, stored.Secret)
					assert.Equal(t, tc.method, stored.TokenEndpointAuthMethod)
					assert.Equal(t, tc.flavor, stored.Flavor)
					assert.Equal(t, provider.ProtectedResources, stored.ProtectedResources)
				}
			})
		}
	})
	t.Run("stored ciphertext retains service scoped encryption", func(t *testing.T) {
		provider := credentialSourcePostgresProvider(model.CredentialSourceStored)
		encryption := newTestEncryption(t)
		context := map[string]string{"service_id": provider.ID.String()}
		plaintext := []byte("synthetic-stored-secret")
		ciphertext, err := encryption.Encrypt(h.ctx, plaintext, context)
		require.NoError(t, err)
		provider.Secret = model.NewEncryptedSecret(ciphertext)
		require.NoError(t, h.providers.Create(h.ctx, provider))
		assertCredentialSourcePostgresRow(t, h, provider)
		stored, err := h.providers.Get(h.ctx, provider.ID)
		require.NoError(t, err)
		persistedCiphertext, err := stored.Secret.GetCiphertext()
		require.NoError(t, err)
		assert.Equal(t, ciphertext, persistedCiphertext)
		assert.NotContains(t, string(persistedCiphertext), string(plaintext))
		decrypted, err := encryption.Decrypt(h.ctx, persistedCiphertext, context)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
		_, err = encryption.Decrypt(h.ctx, persistedCiphertext, map[string]string{"service_id": id.NewServiceID().String()})
		assert.Error(t, err)
	})
	t.Run("invalid source records do not reserve indexes or resource children", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			mutate func(*model.ThirdpartyOAuth2ProviderEntity)
		}{
			{"unknown source", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.CredentialSource = "other" }},
			{"stored missing ID", func(p *model.ThirdpartyOAuth2ProviderEntity) {
				p.CredentialSource = model.CredentialSourceStored
				p.Secret = model.NewEncryptedSecret([]byte("ciphertext"))
			}},
			{"stored absent secret", func(p *model.ThirdpartyOAuth2ProviderEntity) {
				p.CredentialSource = model.CredentialSourceStored
				p.ClientID = "stored-client"
			}},
			{"stored plaintext secret", func(p *model.ThirdpartyOAuth2ProviderEntity) {
				p.CredentialSource = model.CredentialSourceStored
				p.ClientID = "stored-client"
				p.Secret = model.NewPlaintextSecret("must-not-persist")
			}},
			{"stored empty ciphertext", func(p *model.ThirdpartyOAuth2ProviderEntity) {
				p.CredentialSource = model.CredentialSourceStored
				p.ClientID = "stored-client"
				p.Secret = model.NewEncryptedSecret(nil)
			}},
			{"filesystem inline ID", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.ClientID = "inline-client" }},
			{"filesystem inline ciphertext", func(p *model.ThirdpartyOAuth2ProviderEntity) {
				p.Secret = model.NewEncryptedSecret([]byte("ciphertext"))
			}},
			{"filesystem missing canonical", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.CanonicalID = nil }},
			{"filesystem invalid canonical", func(p *model.ThirdpartyOAuth2ProviderEntity) { value := "not canonical"; p.CanonicalID = &value }},
			{"filesystem overlong canonical", func(p *model.ThirdpartyOAuth2ProviderEntity) {
				value := strings.Repeat("a", 129)
				p.CanonicalID = &value
			}},
			{"filesystem UUID canonical", func(p *model.ThirdpartyOAuth2ProviderEntity) {
				value := id.NewServiceID().String()
				p.CanonicalID = &value
			}},
			{"filesystem public", func(p *model.ThirdpartyOAuth2ProviderEntity) {
				p.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
			}},
			{"filesystem CIMD", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.TokenEndpointAuthMethod = cimdPrivateKeyJWTAuthMethod }},
			{"filesystem Google", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.Flavor = model.OAuth2FlavorGoogle }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resourceURI := "https://api.example.test/" + id.NewServiceID().String()
				candidate := credentialSourcePostgresProvider(model.CredentialSourceFilesystem, resourceURI)
				canonicalID := *candidate.CanonicalID
				tc.mutate(candidate)
				requireCredentialSourcePostgresKind(t, h.providers.Create(h.ctx, candidate), storage.ErrorKindValidation)
				var exists bool
				require.NoError(t, h.db.QueryRowContext(h.ctx, `SELECT EXISTS(SELECT 1 FROM thirdparty_oauth2_services WHERE id = $1) OR EXISTS(SELECT 1 FROM service_protected_resources WHERE service_id = $1)`, candidate.ID).Scan(&exists))
				assert.False(t, exists)
				valid := credentialSourcePostgresProvider(model.CredentialSourceStored, resourceURI)
				valid.CanonicalID = &canonicalID
				require.NoError(t, h.providers.Create(h.ctx, valid))
				found, err := h.providers.GetByCanonicalID(h.ctx, canonicalID)
				require.NoError(t, err)
				assert.Equal(t, valid.ID, found.ID)
			})
		}
	})
	t.Run("canonical ownership remains exact and unique", func(t *testing.T) {
		upper := credentialSourcePostgresProvider(model.CredentialSourceFilesystem)
		lower := credentialSourcePostgresProvider(model.CredentialSourceFilesystem)
		upperName, lowerName := "Platform.Repository", "platform.Repository"
		upper.CanonicalID, lower.CanonicalID = &upperName, &lowerName
		require.NoError(t, h.providers.Create(h.ctx, upper))
		require.NoError(t, h.providers.Create(h.ctx, lower))
		for _, provider := range []*model.ThirdpartyOAuth2ProviderEntity{upper, lower} {
			found, err := h.providers.GetByCanonicalID(h.ctx, *provider.CanonicalID)
			require.NoError(t, err)
			assert.Equal(t, provider.ID, found.ID)
		}
		duplicate := credentialSourcePostgresProvider(model.CredentialSourceFilesystem)
		duplicate.CanonicalID = &upperName
		requireCredentialSourcePostgresKind(t, h.providers.Create(h.ctx, duplicate), storage.ErrorKindConflict)
	})
}
