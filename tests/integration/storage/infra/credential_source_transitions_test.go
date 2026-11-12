//go:build integration

package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func credentialSourcePostgresSnapshot(t *testing.T, h *credentialSourcePostgresHarness, serviceID id.ServiceID) string {
	t.Helper()
	var snapshot string
	require.NoError(t, h.db.QueryRowContext(h.ctx, `SELECT jsonb_build_object(
		'provider', to_jsonb(s),
		'resources', (SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY resource_uri), '[]'::jsonb) FROM service_protected_resources r WHERE r.service_id = s.id)
	)::text FROM thirdparty_oauth2_services s WHERE s.id = $1`, serviceID).Scan(&snapshot))
	return snapshot
}

func transitionCredentialSourcePostgres(t *testing.T, h *credentialSourcePostgresHarness, serviceID id.ServiceID) *model.ThirdpartyOAuth2ProviderEntity {
	t.Helper()
	candidate, err := h.providers.Get(h.ctx, serviceID)
	require.NoError(t, err)
	candidate.CredentialSourceTransitioned = false
	if candidate.CredentialSource == model.CredentialSourceStored {
		candidate.CredentialSource, candidate.ClientID, candidate.Secret = model.CredentialSourceFilesystem, "", model.NewAbsentSecret()
	} else {
		candidate.CredentialSource, candidate.ClientID = model.CredentialSourceStored, "explicit-reverse-client"
		encryption := newTestEncryption(t)
		ciphertext, err := encryption.Encrypt(h.ctx, []byte("explicit-reverse-secret"), map[string]string{"service_id": serviceID.String()})
		require.NoError(t, err)
		candidate.Secret = model.NewEncryptedSecret(ciphertext)
	}
	version := candidate.Version
	require.NoError(t, h.providers.Update(h.ctx, candidate, &version))
	return candidate
}

func credentialSourcePostgresSession(principal id.Principal, serviceID id.ServiceID, identity *id.ClientID) *storage.UserSession {
	now := time.Now().UTC()
	return &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID, UpstreamClientID: identity,
		EncryptedAccessToken: []byte("opaque-access-ciphertext"), EncryptedRefreshToken: []byte("opaque-refresh-ciphertext"),
		TokenType: "Bearer", Scope: []string{"read"}, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func assertCredentialSourcePostgresIdentity(t *testing.T, h *credentialSourcePostgresHarness, session *storage.UserSession, expected *id.ClientID) {
	t.Helper()
	var identity sql.NullString
	require.NoError(t, h.db.QueryRowContext(h.ctx, `SELECT upstream_client_id FROM user_sessions WHERE principal = $1 AND service_id = $2`, session.Principal, session.ServiceID).Scan(&identity))
	if expected == nil {
		assert.False(t, identity.Valid)
	} else {
		assert.True(t, identity.Valid)
		assert.Equal(t, expected.String(), identity.String)
	}
	stored, err := h.sessions.FindByPrincipalAndService(h.ctx, session.Principal, session.ServiceID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, expected, stored.UpstreamClientID)
	assert.Equal(t, storage.EncryptionContext{ServiceID: session.ServiceID}, stored.EncryptionContext)
}

func TestCredentialSourcePostgresTransitionsAndSessionIdentity(t *testing.T) {
	h := setupCredentialSourcePostgresHarness(t)
	t.Run("both directions atomically persist source credentials and irreversible marker", func(t *testing.T) {
		for _, initial := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
			t.Run(string(initial), func(t *testing.T) {
				provider := credentialSourcePostgresProvider(initial, "https://api.example.test/"+id.NewServiceID().String())
				require.NoError(t, h.providers.Create(h.ctx, provider))
				identity := id.ClientID("established-client-before-transition")
				recorded := credentialSourcePostgresSession(id.Principal(provider.ID.String()+"@recorded.test"), provider.ID, &identity)
				legacy := credentialSourcePostgresSession(id.Principal(provider.ID.String()+"@legacy.test"), provider.ID, nil)
				require.NoError(t, h.sessions.Create(h.ctx, recorded))
				require.NoError(t, h.sessions.Create(h.ctx, legacy))
				for range 2 {
					before, err := h.providers.Get(h.ctx, provider.ID)
					require.NoError(t, err)
					committed := transitionCredentialSourcePostgres(t, h, provider.ID)
					assert.True(t, committed.CredentialSourceTransitioned, "RETURNING must expose committed, not caller-supplied, evidence")
					stored, err := h.providers.Get(h.ctx, provider.ID)
					require.NoError(t, err)
					assert.Equal(t, committed.CredentialSource, stored.CredentialSource)
					assert.True(t, stored.CredentialSourceTransitioned)
					assert.Equal(t, before.Version+1, stored.Version)
					assert.Equal(t, before.CreatedAt, stored.CreatedAt)
					assert.Equal(t, before.CanonicalID, stored.CanonicalID)
					assert.Equal(t, before.ProtectedResources, stored.ProtectedResources)
					assertCredentialSourcePostgresRow(t, h, stored)
					if stored.CredentialSource == model.CredentialSourceFilesystem {
						assert.Empty(t, stored.ClientID)
						assert.True(t, stored.Secret.IsAbsent())
					} else {
						assert.Equal(t, id.ClientID("explicit-reverse-client"), stored.ClientID)
						ciphertext, err := stored.Secret.GetCiphertext()
						require.NoError(t, err)
						plaintext, err := newTestEncryption(t).Decrypt(h.ctx, ciphertext, map[string]string{"service_id": provider.ID.String()})
						require.NoError(t, err)
						assert.Equal(t, []byte("explicit-reverse-secret"), plaintext, "reverse transition persists explicit encrypted credentials, never file imports")
					}
					canonical, err := h.providers.GetByCanonicalID(h.ctx, *provider.CanonicalID)
					require.NoError(t, err)
					assert.True(t, canonical.CredentialSourceTransitioned)
					resolved, err := h.providers.FindByProtectedResource(h.ctx, provider.ProtectedResources[0])
					require.NoError(t, err)
					assert.True(t, resolved.CredentialSourceTransitioned)
					listed, err := h.providers.List(h.ctx)
					require.NoError(t, err)
					found := false
					for _, item := range listed {
						if item.ID == provider.ID {
							found = true
							assert.True(t, item.CredentialSourceTransitioned)
						}
					}
					assert.True(t, found)
					assertCredentialSourcePostgresIdentity(t, h, recorded, &identity)
					assertCredentialSourcePostgresIdentity(t, h, legacy, nil)
				}
			})
		}
	})
	t.Run("no-op metadata resources and failed CAS never reset history", func(t *testing.T) {
		for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
			for _, transitioned := range []bool{false, true} {
				name := string(source) + "/never-transitioned"
				if transitioned {
					name = string(source) + "/transitioned"
				}
				t.Run(name, func(t *testing.T) {
					resourceURI := "https://api.example.test/" + id.NewServiceID().String()
					provider := credentialSourcePostgresProvider(source, resourceURI)
					require.NoError(t, h.providers.Create(h.ctx, provider))
					if transitioned {
						for range 2 {
							transitionCredentialSourcePostgres(t, h, provider.ID)
						}
					}
					for _, metadataOnly := range []bool{false, true} {
						before, err := h.providers.Get(h.ctx, provider.ID)
						require.NoError(t, err)
						candidate := before.Copy()
						candidate.CredentialSourceTransitioned = false
						if metadataOnly {
							candidate.DisplayName = "metadata-only rename"
						}
						candidate.ProtectedResources = []string{"https://ignored.example.test"}
						require.NoError(t, h.providers.Update(h.ctx, candidate, nil))
						stored, err := h.providers.Get(h.ctx, provider.ID)
						require.NoError(t, err)
						assert.Equal(t, source, stored.CredentialSource)
						assert.Equal(t, transitioned, stored.CredentialSourceTransitioned)
						assert.Equal(t, before.ClientID, stored.ClientID)
						assert.Equal(t, before.Secret, stored.Secret)
						assert.Equal(t, before.ProtectedResources, stored.ProtectedResources)
						assertCredentialSourcePostgresRow(t, h, stored)
					}
					_, err := h.providers.AddProtectedResource(h.ctx, provider.ID, resourceURI+"/second")
					require.NoError(t, err)
					_, err = h.providers.RenameProtectedResource(h.ctx, provider.ID, resourceURI+"/second", resourceURI+"/third")
					require.NoError(t, err)
					_, err = h.providers.RemoveProtectedResource(h.ctx, provider.ID, resourceURI+"/third")
					require.NoError(t, err)
					before, err := h.providers.Get(h.ctx, provider.ID)
					require.NoError(t, err)
					assert.Equal(t, transitioned, before.CredentialSourceTransitioned)
					snapshot := credentialSourcePostgresSnapshot(t, h, provider.ID)
					candidate := before.Copy()
					candidate.CredentialSourceTransitioned = false
					if source == model.CredentialSourceStored {
						candidate.CredentialSource, candidate.ClientID, candidate.Secret = model.CredentialSourceFilesystem, "", model.NewAbsentSecret()
					} else {
						candidate.CredentialSource, candidate.ClientID, candidate.Secret = model.CredentialSourceStored, "explicit-client", model.NewEncryptedSecret([]byte("explicit-ciphertext"))
					}
					staleVersion := before.Version - 1
					requireCredentialSourcePostgresKind(t, h.providers.Update(h.ctx, candidate, &staleVersion), storage.ErrorKindConflict)
					assert.Equal(t, snapshot, credentialSourcePostgresSnapshot(t, h, provider.ID))
				})
			}
		}
	})
	t.Run("failed transitions roll back the row child transaction and identities", func(t *testing.T) {
		for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
			for _, failure := range []string{"stale CAS", "resource conflict", "canonical conflict", "invalid credentials"} {
				t.Run(string(source)+"/"+failure, func(t *testing.T) {
					provider := credentialSourcePostgresProvider(source, "https://api.example.test/"+id.NewServiceID().String())
					owner := credentialSourcePostgresProvider(model.CredentialSourceStored, "https://api.example.test/"+id.NewServiceID().String())
					require.NoError(t, h.providers.Create(h.ctx, provider))
					require.NoError(t, h.providers.Create(h.ctx, owner))
					identity := id.ClientID("established-client")
					session := credentialSourcePostgresSession(id.Principal(provider.ID.String()+"@example.test"), provider.ID, &identity)
					require.NoError(t, h.sessions.Create(h.ctx, session))
					before, err := h.providers.Get(h.ctx, provider.ID)
					require.NoError(t, err)
					rowBefore := credentialSourcePostgresSnapshot(t, h, provider.ID)
					ownerBefore := credentialSourcePostgresSnapshot(t, h, owner.ID)
					candidate := before.Copy()
					candidate.DisplayName = "must-not-commit"
					candidate.CredentialSourceTransitioned = true
					if source == model.CredentialSourceStored {
						candidate.CredentialSource, candidate.ClientID, candidate.Secret = model.CredentialSourceFilesystem, "", model.NewAbsentSecret()
					} else {
						candidate.CredentialSource, candidate.ClientID, candidate.Secret = model.CredentialSourceStored, "explicit-client", model.NewEncryptedSecret([]byte("explicit-ciphertext"))
					}
					version := before.Version
					kind := storage.ErrorKindConflict
					switch failure {
					case "stale CAS":
						version--
					case "resource conflict":
						candidate.ProtectedResources = owner.ProtectedResources
					case "canonical conflict":
						candidate.CanonicalID = owner.CanonicalID
					case "invalid credentials":
						candidate.Secret = model.NewPlaintextSecret("must-not-persist")
						kind = storage.ErrorKindValidation
					}
					requireCredentialSourcePostgresKind(t, h.providers.Update(h.ctx, candidate, &version), kind)
					assert.Equal(t, rowBefore, credentialSourcePostgresSnapshot(t, h, provider.ID))
					assert.Equal(t, ownerBefore, credentialSourcePostgresSnapshot(t, h, owner.ID))
					after, err := h.providers.Get(h.ctx, provider.ID)
					require.NoError(t, err)
					assert.Equal(t, before, after)
					resolved, err := h.providers.FindByProtectedResource(h.ctx, provider.ProtectedResources[0])
					require.NoError(t, err)
					assert.Equal(t, provider.ID, resolved.ID)
					canonical, err := h.providers.GetByCanonicalID(h.ctx, *provider.CanonicalID)
					require.NoError(t, err)
					assert.Equal(t, provider.ID, canonical.ID)
					assertCredentialSourcePostgresIdentity(t, h, session, &identity)
				})
			}
		}
	})
	t.Run("concurrent CAS commits exactly one complete transition", func(t *testing.T) {
		provider := credentialSourcePostgresProvider(model.CredentialSourceStored, "https://api.example.test/"+id.NewServiceID().String())
		require.NoError(t, h.providers.Create(h.ctx, provider))
		before, err := h.providers.Get(h.ctx, provider.ID)
		require.NoError(t, err)
		candidates := []*model.ThirdpartyOAuth2ProviderEntity{before.Copy(), before.Copy()}
		for _, candidate := range candidates {
			candidate.CredentialSource, candidate.ClientID, candidate.Secret = model.CredentialSourceFilesystem, "", model.NewAbsentSecret()
			candidate.DisplayName = id.NewServiceID().String()
			candidate.ProtectedResources = []string{"https://api.example.test/" + id.NewServiceID().String()}
		}
		start := make(chan struct{})
		errorsByCandidate := make([]error, len(candidates))
		var workers sync.WaitGroup
		for index, candidate := range candidates {
			workers.Add(1)
			go func(index int, candidate *model.ThirdpartyOAuth2ProviderEntity) {
				defer workers.Done()
				<-start
				version := before.Version
				errorsByCandidate[index] = h.providers.Update(h.ctx, candidate, &version)
			}(index, candidate)
		}
		close(start)
		workers.Wait()
		winner := -1
		for index, err := range errorsByCandidate {
			if err == nil {
				require.Equal(t, -1, winner)
				winner = index
			} else {
				requireCredentialSourcePostgresKind(t, err, storage.ErrorKindConflict)
			}
		}
		require.NotEqual(t, -1, winner)
		stored, err := h.providers.Get(h.ctx, provider.ID)
		require.NoError(t, err)
		assert.Equal(t, candidates[winner].DisplayName, stored.DisplayName)
		assert.Equal(t, candidates[winner].ProtectedResources, stored.ProtectedResources)
		assert.Equal(t, model.CredentialSourceFilesystem, stored.CredentialSource)
		assert.True(t, stored.CredentialSourceTransitioned)
		assert.Empty(t, stored.ClientID)
		assert.True(t, stored.Secret.IsAbsent())
		assert.Equal(t, before.Version+1, stored.Version)
		assertCredentialSourcePostgresRow(t, h, stored)
	})
	t.Run("nullable identity round trips projections upsert and refresh", func(t *testing.T) {
		for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
			t.Run(string(source), func(t *testing.T) {
				provider := credentialSourcePostgresProvider(source)
				require.NoError(t, h.providers.Create(h.ctx, provider))
				identity := id.ClientID(strings.Repeat("a", 300))
				session := credentialSourcePostgresSession(id.Principal(provider.ID.String()+"@recorded.test"), provider.ID, &identity)
				require.NoError(t, h.sessions.Create(h.ctx, session))
				assertCredentialSourcePostgresIdentity(t, h, session, &identity)
				get, err := h.sessions.Get(h.ctx, session.ID)
				require.NoError(t, err)
				find, err := h.sessions.FindByPrincipalAndService(h.ctx, session.Principal, provider.ID)
				require.NoError(t, err)
				listed, err := h.sessions.ListByPrincipal(h.ctx, session.Principal)
				require.NoError(t, err)
				require.Len(t, listed, 1)
				active, err := h.sessions.ListActiveByPrincipal(h.ctx, session.Principal)
				require.NoError(t, err)
				require.Len(t, active, 1)
				for _, projection := range []*storage.UserSession{get, find, listed[0], active[0]} {
					require.NotNil(t, projection.UpstreamClientID)
					assert.Equal(t, identity, *projection.UpstreamClientID)
					*projection.UpstreamClientID = "caller-mutation"
					assertCredentialSourcePostgresIdentity(t, h, session, &identity)
				}
				before := credentialSourcePostgresSnapshot(t, h, provider.ID)
				refreshed, err := h.sessions.WithLockedSession(h.ctx, session.Principal, provider.ID, func(_ context.Context, locked *storage.UserSession) (bool, error) {
					require.NotNil(t, locked.UpstreamClientID)
					assert.Equal(t, identity, *locked.UpstreamClientID)
					locked.EncryptedAccessToken, locked.EncryptedRefreshToken = []byte("rotated-access-ciphertext"), []byte("rotated-refresh-ciphertext")
					return true, nil
				})
				require.NoError(t, err)
				assert.Equal(t, &identity, refreshed.UpstreamClientID)
				assertCredentialSourcePostgresIdentity(t, h, session, &identity)
				assert.Equal(t, before, credentialSourcePostgresSnapshot(t, h, provider.ID), "token refresh cannot import credentials or write provider versions/timestamps")
				stored, err := h.sessions.Get(h.ctx, session.ID)
				require.NoError(t, err)
				assert.Equal(t, []byte("rotated-access-ciphertext"), stored.EncryptedAccessToken)
				assert.Equal(t, []byte("rotated-refresh-ciphertext"), stored.EncryptedRefreshToken)
				newIdentity := id.ClientID("new-connection-client")
				reconnected := credentialSourcePostgresSession(session.Principal, provider.ID, &newIdentity)
				reconnected.EncryptedAccessToken = []byte("reconnected-access-ciphertext")
				require.NoError(t, h.sessions.Create(h.ctx, reconnected))
				assertCredentialSourcePostgresIdentity(t, h, session, &newIdentity)
				stored, err = h.sessions.FindByPrincipalAndService(h.ctx, session.Principal, provider.ID)
				require.NoError(t, err)
				assert.Equal(t, session.ID, stored.ID)
				assert.Equal(t, reconnected.EncryptedAccessToken, stored.EncryptedAccessToken)
				empty := id.ClientID("")
				invalid := credentialSourcePostgresSession(session.Principal, provider.ID, &empty)
				require.Error(t, h.sessions.Create(h.ctx, invalid))
				assertCredentialSourcePostgresIdentity(t, h, session, &newIdentity)
			})
		}
	})
	t.Run("legacy identity stays NULL across both transitions and locked refresh", func(t *testing.T) {
		provider := credentialSourcePostgresProvider(model.CredentialSourceStored)
		require.NoError(t, h.providers.Create(h.ctx, provider))
		session := credentialSourcePostgresSession(id.Principal(provider.ID.String()+"@legacy.test"), provider.ID, nil)
		require.NoError(t, h.sessions.Create(h.ctx, session))
		for range 2 {
			transitionCredentialSourcePostgres(t, h, provider.ID)
			before := credentialSourcePostgresSnapshot(t, h, provider.ID)
			refreshed, err := h.sessions.WithLockedSession(h.ctx, session.Principal, provider.ID, func(_ context.Context, locked *storage.UserSession) (bool, error) {
				assert.Nil(t, locked.UpstreamClientID)
				locked.EncryptedAccessToken = []byte("new-access-ciphertext")
				return true, nil
			})
			require.NoError(t, err)
			assert.Nil(t, refreshed.UpstreamClientID)
			assertCredentialSourcePostgresIdentity(t, h, session, nil)
			assert.Equal(t, before, credentialSourcePostgresSnapshot(t, h, provider.ID))
		}
	})
	t.Run("failed refresh cannot rewrite association or ciphertext", func(t *testing.T) {
		provider := credentialSourcePostgresProvider(model.CredentialSourceFilesystem)
		require.NoError(t, h.providers.Create(h.ctx, provider))
		identity := id.ClientID("established-client")
		session := credentialSourcePostgresSession(id.Principal(provider.ID.String()+"@failure.test"), provider.ID, &identity)
		require.NoError(t, h.sessions.Create(h.ctx, session))
		failure := errors.New("synthetic provider rejection")
		_, err := h.sessions.WithLockedSession(h.ctx, session.Principal, provider.ID, func(_ context.Context, candidate *storage.UserSession) (bool, error) {
			require.NotNil(t, candidate.UpstreamClientID)
			*candidate.UpstreamClientID = "must-not-commit"
			candidate.EncryptedAccessToken = []byte("must-not-commit")
			return false, failure
		})
		require.ErrorIs(t, err, failure)
		assertCredentialSourcePostgresIdentity(t, h, session, &identity)
		stored, err := h.sessions.Get(h.ctx, session.ID)
		require.NoError(t, err)
		assert.Equal(t, session.EncryptedAccessToken, stored.EncryptedAccessToken)
	})
	t.Run("filesystem update validation preserves the complete transaction", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			mutate func(*model.ThirdpartyOAuth2ProviderEntity)
		}{
			{"canonical clearing", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.CanonicalID = nil; p.ClearCanonicalID = true }},
			{"invalid canonical reassignment", func(p *model.ThirdpartyOAuth2ProviderEntity) { value := "invalid canonical"; p.CanonicalID = &value }},
			{"public authentication", func(p *model.ThirdpartyOAuth2ProviderEntity) {
				p.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
			}},
			{"CIMD authentication", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.TokenEndpointAuthMethod = cimdPrivateKeyJWTAuthMethod }},
			{"Google flavor", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.Flavor = model.OAuth2FlavorGoogle }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				provider := credentialSourcePostgresProvider(model.CredentialSourceFilesystem, "https://api.example.test/"+id.NewServiceID().String())
				require.NoError(t, h.providers.Create(h.ctx, provider))
				before, err := h.providers.Get(h.ctx, provider.ID)
				require.NoError(t, err)
				snapshot := credentialSourcePostgresSnapshot(t, h, provider.ID)
				candidate := before.Copy()
				candidate.DisplayName = "must-not-commit"
				tc.mutate(candidate)
				version := before.Version
				requireCredentialSourcePostgresKind(t, h.providers.Update(h.ctx, candidate, &version), storage.ErrorKindValidation)
				assert.Equal(t, snapshot, credentialSourcePostgresSnapshot(t, h, provider.ID))
				canonical, err := h.providers.GetByCanonicalID(h.ctx, *before.CanonicalID)
				require.NoError(t, err)
				assert.Equal(t, before, canonical)
				resource, err := h.providers.FindByProtectedResource(h.ctx, before.ProtectedResources[0])
				require.NoError(t, err)
				assert.Equal(t, before, resource)
			})
		}
	})
	t.Run("successful refresh never relabels recorded or legacy associations", func(t *testing.T) {
		for _, legacy := range []bool{false, true} {
			t.Run(map[bool]string{false: "recorded identity", true: "legacy absence"}[legacy], func(t *testing.T) {
				provider := credentialSourcePostgresProvider(model.CredentialSourceStored)
				require.NoError(t, h.providers.Create(h.ctx, provider))
				identity := id.ClientID("established-client")
				var expected *id.ClientID
				if !legacy {
					expected = &identity
				}
				session := credentialSourcePostgresSession(id.Principal(provider.ID.String()+"@refresh.test"), provider.ID, expected)
				require.NoError(t, h.sessions.Create(h.ctx, session))
				refreshed, err := h.sessions.WithLockedSession(h.ctx, session.Principal, provider.ID, func(_ context.Context, locked *storage.UserSession) (bool, error) {
					newIdentity := id.ClientID("different-current-credential-client")
					locked.UpstreamClientID = &newIdentity
					locked.EncryptedAccessToken = []byte("refreshed-access-ciphertext")
					return true, nil
				})
				require.NoError(t, err)
				assert.Equal(t, expected, refreshed.UpstreamClientID)
				assertCredentialSourcePostgresIdentity(t, h, session, expected)
				stored, err := h.sessions.Get(h.ctx, session.ID)
				require.NoError(t, err)
				assert.Equal(t, []byte("refreshed-access-ciphertext"), stored.EncryptedAccessToken)
			})
		}
	})
}
