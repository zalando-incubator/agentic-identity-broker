package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialSourceMemoryTransitionsAreAtomicAndMonotonic(t *testing.T) {
	for _, initial := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		t.Run(string(initial), func(t *testing.T) {
			ctx := context.Background()
			repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
			provider := credentialSourceProvider(initial, "https://api.example.test/original")
			require.NoError(t, repo.Create(ctx, provider))
			for range 2 {
				before, err := repo.Get(ctx, provider.ID)
				require.NoError(t, err)
				candidate := before.Copy()
				candidate.CredentialSourceTransitioned = false // Caller cannot erase persisted history.
				if before.CredentialSource == model.CredentialSourceStored {
					candidate.CredentialSource = model.CredentialSourceFilesystem
					candidate.ClientID = ""
					candidate.Secret = model.NewAbsentSecret()
				} else {
					candidate.CredentialSource = model.CredentialSourceStored
					candidate.ClientID = "explicit-reverse-client"
					candidate.Secret = model.NewEncryptedSecret([]byte("explicit-reverse-ciphertext"))
				}
				version := before.Version
				require.NoError(t, repo.Update(ctx, candidate, &version))
				assert.True(t, candidate.CredentialSourceTransitioned, "update returns committed transition evidence")
				stored, err := repo.Get(ctx, provider.ID)
				require.NoError(t, err)
				assert.Equal(t, candidate.CredentialSource, stored.CredentialSource)
				assert.True(t, stored.CredentialSourceTransitioned)
				assert.Equal(t, before.Version+1, stored.Version)
				assert.Equal(t, before.CreatedAt, stored.CreatedAt)
				assert.Equal(t, before.CanonicalID, stored.CanonicalID)
				assert.Equal(t, before.ProtectedResources, stored.ProtectedResources)
				if stored.CredentialSource == model.CredentialSourceFilesystem {
					assert.Empty(t, stored.ClientID)
					assert.True(t, stored.Secret.IsAbsent())
				} else {
					assert.Equal(t, id.ClientID("explicit-reverse-client"), stored.ClientID)
					ciphertext, err := stored.Secret.GetCiphertext()
					require.NoError(t, err)
					assert.Equal(t, []byte("explicit-reverse-ciphertext"), ciphertext)
				}
				canonical, err := repo.GetByCanonicalID(ctx, *stored.CanonicalID)
				require.NoError(t, err)
				assert.True(t, canonical.CredentialSourceTransitioned)
				listed, err := repo.List(ctx)
				require.NoError(t, err)
				require.Len(t, listed, 1)
				assert.True(t, listed[0].CredentialSourceTransitioned)
			}
		})
	}
}

func TestCredentialSourceMemoryPreservesHistoryOnEverySupportedWrite(t *testing.T) {
	for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		for _, marker := range []bool{false, true} {
			name := string(source) + "/never-transitioned"
			if marker {
				name = string(source) + "/transitioned"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
				provider := credentialSourceProvider(source, "https://api.example.test/first")
				require.NoError(t, repo.Create(ctx, provider))
				if marker {
					// Establish history through the supported transition path, not by fixture injection.
					for range 2 {
						current, err := repo.Get(ctx, provider.ID)
						require.NoError(t, err)
						if current.CredentialSource == model.CredentialSourceStored {
							current.CredentialSource = model.CredentialSourceFilesystem
							current.ClientID, current.Secret = "", model.NewAbsentSecret()
						} else {
							current.CredentialSource = model.CredentialSourceStored
							current.ClientID, current.Secret = "stored-client", model.NewEncryptedSecret([]byte("ciphertext"))
						}
						version := current.Version
						require.NoError(t, repo.Update(ctx, current, &version))
					}
				}
				for _, metadataOnly := range []bool{false, true} {
					before, err := repo.Get(ctx, provider.ID)
					require.NoError(t, err)
					candidate := before.Copy()
					candidate.CredentialSourceTransitioned = false
					if metadataOnly {
						candidate.DisplayName = "metadata rename"
					}
					candidate.ProtectedResources = []string{"https://ignored.example.test"}
					require.NoError(t, repo.Update(ctx, candidate, nil))
					stored, err := repo.Get(ctx, provider.ID)
					require.NoError(t, err)
					assert.Equal(t, source, stored.CredentialSource)
					assert.Equal(t, marker, stored.CredentialSourceTransitioned)
					assert.Equal(t, before.ClientID, stored.ClientID)
					assert.Equal(t, before.Secret, stored.Secret)
					assert.Equal(t, before.ProtectedResources, stored.ProtectedResources, "nil version preserves resource children")
				}
				_, err := repo.AddProtectedResource(ctx, provider.ID, "https://api.example.test/second")
				require.NoError(t, err)
				_, err = repo.RenameProtectedResource(ctx, provider.ID, "https://api.example.test/second", "https://api.example.test/third")
				require.NoError(t, err)
				_, err = repo.RemoveProtectedResource(ctx, provider.ID, "https://api.example.test/third")
				require.NoError(t, err)
				stored, err := repo.Get(ctx, provider.ID)
				require.NoError(t, err)
				assert.Equal(t, source, stored.CredentialSource)
				assert.Equal(t, marker, stored.CredentialSourceTransitioned)
				resolved, err := repo.FindByProtectedResource(ctx, "https://api.example.test/first")
				require.NoError(t, err)
				assert.Equal(t, marker, resolved.CredentialSourceTransitioned)
				before := stored.Copy()
				candidate := before.Copy()
				candidate.CredentialSourceTransitioned = false
				if source == model.CredentialSourceStored {
					candidate.CredentialSource, candidate.ClientID, candidate.Secret = model.CredentialSourceFilesystem, "", model.NewAbsentSecret()
				} else {
					candidate.CredentialSource, candidate.ClientID, candidate.Secret = model.CredentialSourceStored, "explicit-client", model.NewEncryptedSecret([]byte("explicit-ciphertext"))
				}
				staleVersion := before.Version - 1
				requireStorageKind(t, repo.Update(ctx, candidate, &staleVersion), storage.ErrorKindConflict)
				after, err := repo.Get(ctx, provider.ID)
				require.NoError(t, err)
				assert.Equal(t, before, after, "failed CAS preserves false or true evidence and every credential/resource field")
			})
		}
	}
}

func TestCredentialSourceMemoryFailedTransitionsPreserveEntireRecordAndIndexes(t *testing.T) {
	for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		for _, failure := range []string{"stale CAS", "resource conflict", "canonical conflict", "invalid credentials"} {
			t.Run(string(source)+"/"+failure, func(t *testing.T) {
				ctx := context.Background()
				repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
				provider := credentialSourceProvider(source, "https://api.example.test/original")
				owner := credentialSourceProvider(model.CredentialSourceStored, "https://api.example.test/owned")
				require.NoError(t, repo.Create(ctx, provider))
				require.NoError(t, repo.Create(ctx, owner))
				before, err := repo.Get(ctx, provider.ID)
				require.NoError(t, err)
				candidate := before.Copy()
				candidate.DisplayName = "must not commit"
				candidate.CredentialSourceTransitioned = true
				if source == model.CredentialSourceStored {
					candidate.CredentialSource, candidate.ClientID, candidate.Secret = model.CredentialSourceFilesystem, "", model.NewAbsentSecret()
				} else {
					candidate.CredentialSource, candidate.ClientID, candidate.Secret = model.CredentialSourceStored, "reverse-client", model.NewEncryptedSecret([]byte("reverse-ciphertext"))
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
				requireStorageKind(t, repo.Update(ctx, candidate, &version), kind)
				after, err := repo.Get(ctx, provider.ID)
				require.NoError(t, err)
				assert.Equal(t, before, after)
				canonical, err := repo.GetByCanonicalID(ctx, *before.CanonicalID)
				require.NoError(t, err)
				assert.Equal(t, before, canonical)
				resolved, err := repo.FindByProtectedResource(ctx, before.ProtectedResources[0])
				require.NoError(t, err)
				assert.Equal(t, before, resolved)
				owned, err := repo.FindByProtectedResource(ctx, owner.ProtectedResources[0])
				require.NoError(t, err)
				assert.Equal(t, owner.ID, owned.ID)
			})
		}
	}
}

func TestCredentialSourceMemoryConcurrentCASHasOneCompleteWinner(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := credentialSourceProvider(model.CredentialSourceStored, "https://api.example.test/original")
	require.NoError(t, repo.Create(ctx, provider))
	before, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	candidates := []*model.ThirdpartyOAuth2ProviderEntity{before.Copy(), before.Copy()}
	for index, candidate := range candidates {
		candidate.CredentialSource = model.CredentialSourceFilesystem
		candidate.ClientID, candidate.Secret = "", model.NewAbsentSecret()
		candidate.DisplayName = []string{"winner A", "winner B"}[index]
		candidate.ProtectedResources = []string{[]string{"https://api.example.test/a", "https://api.example.test/b"}[index]}
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
			errorsByCandidate[index] = repo.Update(ctx, candidate, &version)
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
			requireStorageKind(t, err, storage.ErrorKindConflict)
		}
	}
	require.NotEqual(t, -1, winner)
	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, candidates[winner].DisplayName, stored.DisplayName)
	assert.Equal(t, candidates[winner].ProtectedResources, stored.ProtectedResources)
	assert.Equal(t, model.CredentialSourceFilesystem, stored.CredentialSource)
	assert.True(t, stored.CredentialSourceTransitioned)
	assert.Empty(t, stored.ClientID)
	assert.True(t, stored.Secret.IsAbsent())
	assert.Equal(t, before.Version+1, stored.Version)
}

func TestCredentialSourceMemorySessionIdentityCopiesAreIsolated(t *testing.T) {
	for _, projection := range []string{"create input", "get", "find", "list", "active list"} {
		t.Run(projection, func(t *testing.T) {
			ctx := context.Background()
			repo := NewInMemoryUserSessionRepository()
			session := refreshTestSession(id.Principal("identity@example.test"), id.NewServiceID())
			identity := id.ClientID("established-client")
			session.UpstreamClientID = &identity
			require.NoError(t, repo.Create(ctx, session))
			var copy *storage.UserSession
			var err error
			switch projection {
			case "create input":
				copy = session
			case "get":
				copy, err = repo.Get(ctx, session.ID)
			case "find":
				copy, err = repo.FindByPrincipalAndService(ctx, session.Principal, session.ServiceID)
			case "list":
				var sessions []*storage.UserSession
				sessions, err = repo.ListByPrincipal(ctx, session.Principal)
				require.Len(t, sessions, 1)
				copy = sessions[0]
			case "active list":
				var sessions []*storage.UserSession
				sessions, err = repo.ListActiveByPrincipal(ctx, session.Principal)
				require.Len(t, sessions, 1)
				copy = sessions[0]
			}
			require.NoError(t, err)
			require.NotNil(t, copy.UpstreamClientID)
			*copy.UpstreamClientID = "caller-mutation"
			stored, err := repo.FindByPrincipalAndService(ctx, session.Principal, session.ServiceID)
			require.NoError(t, err)
			require.NotNil(t, stored.UpstreamClientID)
			assert.Equal(t, id.ClientID("established-client"), *stored.UpstreamClientID)
		})
	}
}

func TestCredentialSourceMemoryLockedRefreshPreservesIdentityAndProvider(t *testing.T) {
	ctx := context.Background()
	providers := NewInMemoryThirdpartyOAuth2ProviderRepository()
	sessions := NewInMemoryUserSessionRepository()
	provider := credentialSourceProvider(model.CredentialSourceStored)
	require.NoError(t, providers.Create(ctx, provider))
	for _, legacy := range []bool{false, true} {
		session := refreshTestSession(id.Principal("recorded@example.test"), provider.ID)
		if legacy {
			session.Principal = "legacy@example.test"
		} else {
			identity := id.ClientID("established-client")
			session.UpstreamClientID = &identity
		}
		require.NoError(t, sessions.Create(ctx, session))
	}
	for _, source := range []model.CredentialSource{model.CredentialSourceFilesystem, model.CredentialSourceStored} {
		candidate, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		candidate.CredentialSource = source
		if source == model.CredentialSourceFilesystem {
			candidate.ClientID, candidate.Secret = "", model.NewAbsentSecret()
		} else {
			candidate.ClientID, candidate.Secret = "different-current-client", model.NewEncryptedSecret([]byte("explicit-ciphertext"))
		}
		version := candidate.Version
		require.NoError(t, providers.Update(ctx, candidate, &version))
		before, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		for _, principal := range []id.Principal{"recorded@example.test", "legacy@example.test"} {
			refreshed, err := sessions.WithLockedSession(ctx, principal, provider.ID, func(_ context.Context, locked *storage.UserSession) (bool, error) {
				if principal == "legacy@example.test" {
					assert.Nil(t, locked.UpstreamClientID)
				} else {
					require.NotNil(t, locked.UpstreamClientID)
					assert.Equal(t, id.ClientID("established-client"), *locked.UpstreamClientID)
				}
				locked.EncryptedAccessToken = []byte("rotated-access-ciphertext")
				locked.EncryptedRefreshToken = []byte("rotated-refresh-ciphertext")
				return true, nil
			})
			require.NoError(t, err)
			stored, err := sessions.Get(ctx, refreshed.ID)
			require.NoError(t, err)
			assert.Equal(t, refreshed.UpstreamClientID, stored.UpstreamClientID)
			assert.Equal(t, storage.EncryptionContext{ServiceID: provider.ID}, stored.EncryptionContext)
			assert.Equal(t, []byte("rotated-access-ciphertext"), stored.EncryptedAccessToken)
			if principal == "legacy@example.test" {
				assert.Nil(t, stored.UpstreamClientID, "never infer legacy identity from current service credentials")
			} else {
				require.NotNil(t, refreshed.UpstreamClientID)
				*refreshed.UpstreamClientID = "returned-session-mutation"
				again, err := sessions.Get(ctx, stored.ID)
				require.NoError(t, err)
				assert.Equal(t, id.ClientID("established-client"), *again.UpstreamClientID)
			}
		}
		after, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		assert.Equal(t, before, after, "session token persistence cannot write service credentials, source, marker or version")
	}
}

func TestCredentialSourceMemoryRefreshFailureCannotMutateEstablishedIdentity(t *testing.T) {
	for _, noOp := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed refresh", true: "no-op refresh"}[noOp], func(t *testing.T) {
			ctx := context.Background()
			repo := NewInMemoryUserSessionRepository()
			session := refreshTestSession("identity@example.test", id.NewServiceID())
			identity := id.ClientID("established-client")
			session.UpstreamClientID = &identity
			require.NoError(t, repo.Create(ctx, session))
			failure := errors.New("synthetic provider rejection")
			_, err := repo.WithLockedSession(ctx, session.Principal, session.ServiceID, func(_ context.Context, candidate *storage.UserSession) (bool, error) {
				require.NotNil(t, candidate.UpstreamClientID)
				*candidate.UpstreamClientID = "must-not-commit"
				if noOp {
					return false, nil
				}
				return false, failure
			})
			if noOp {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, failure)
			}
			stored, err := repo.Get(ctx, session.ID)
			require.NoError(t, err)
			assert.Equal(t, id.ClientID("established-client"), *stored.UpstreamClientID)
			assert.Equal(t, []byte("old-access"), stored.EncryptedAccessToken)
		})
	}
}

func TestCredentialSourceMemoryReconnectReplacesOnlyItsSessionAssociation(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryUserSessionRepository()
	session := refreshTestSession("identity@example.test", id.NewServiceID())
	identity := id.ClientID(strings.Repeat("a", 300))
	session.UpstreamClientID = &identity
	require.NoError(t, repo.Create(ctx, session))
	reconnected := refreshTestSession(session.Principal, session.ServiceID)
	newIdentity := id.ClientID("new-connection-client")
	reconnected.UpstreamClientID = &newIdentity
	reconnected.EncryptedAccessToken = []byte("new-connection-ciphertext")
	require.NoError(t, repo.Create(ctx, reconnected))
	stored, err := repo.FindByPrincipalAndService(ctx, session.Principal, session.ServiceID)
	require.NoError(t, err)
	assert.Equal(t, session.ID, stored.ID)
	require.NotNil(t, stored.UpstreamClientID)
	assert.Equal(t, newIdentity, *stored.UpstreamClientID)
	assert.Equal(t, reconnected.EncryptedAccessToken, stored.EncryptedAccessToken)
	invalid := refreshTestSession(session.Principal, session.ServiceID)
	empty := id.ClientID("")
	invalid.UpstreamClientID = &empty
	require.Error(t, repo.Create(ctx, invalid))
	after, err := repo.Get(ctx, stored.ID)
	require.NoError(t, err)
	require.NotNil(t, after.UpstreamClientID)
	assert.Equal(t, newIdentity, *after.UpstreamClientID)
	assert.Equal(t, stored.EncryptedAccessToken, after.EncryptedAccessToken)
}

func TestCredentialSourceMemoryFilesystemUpdateValidationIsAtomic(t *testing.T) {
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
			ctx := context.Background()
			repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
			provider := credentialSourceProvider(model.CredentialSourceFilesystem, "https://api.example.test/filesystem")
			require.NoError(t, repo.Create(ctx, provider))
			before, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			candidate := before.Copy()
			candidate.DisplayName = "must-not-commit"
			tc.mutate(candidate)
			version := before.Version
			requireStorageKind(t, repo.Update(ctx, candidate, &version), storage.ErrorKindValidation)
			after, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			canonical, err := repo.GetByCanonicalID(ctx, *before.CanonicalID)
			require.NoError(t, err)
			assert.Equal(t, before, canonical)
			resource, err := repo.FindByProtectedResource(ctx, before.ProtectedResources[0])
			require.NoError(t, err)
			assert.Equal(t, before, resource)
		})
	}
}

func TestCredentialSourceMemorySuccessfulRefreshCannotRelabelAssociation(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "recorded identity", true: "legacy absence"}[legacy], func(t *testing.T) {
			ctx := context.Background()
			repo := NewInMemoryUserSessionRepository()
			identity := id.ClientID("established-client")
			session := refreshTestSession("identity@example.test", id.NewServiceID())
			if !legacy {
				session.UpstreamClientID = &identity
			}
			require.NoError(t, repo.Create(ctx, session))
			refreshed, err := repo.WithLockedSession(ctx, session.Principal, session.ServiceID, func(_ context.Context, locked *storage.UserSession) (bool, error) {
				newIdentity := id.ClientID("different-current-credential-client")
				locked.UpstreamClientID = &newIdentity
				locked.EncryptedAccessToken = []byte("refreshed-access-ciphertext")
				return true, nil
			})
			require.NoError(t, err)
			stored, err := repo.Get(ctx, session.ID)
			require.NoError(t, err)
			assert.Equal(t, []byte("refreshed-access-ciphertext"), stored.EncryptedAccessToken)
			if legacy {
				assert.Nil(t, refreshed.UpstreamClientID)
				assert.Nil(t, stored.UpstreamClientID)
			} else {
				assert.Equal(t, &identity, refreshed.UpstreamClientID)
				assert.Equal(t, &identity, stored.UpstreamClientID)
			}
		})
	}
}
