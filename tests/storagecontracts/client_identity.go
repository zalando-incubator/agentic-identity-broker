package storagecontracts

import (
	"context"
	"sync"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ClientIdentityRepository(t *testing.T, providers ports.ThirdpartyOAuth2ProviderRepository, identities ports.ThirdpartyOAuth2ClientIdentityRepository) {
	t.Helper()
	ctx := context.Background()
	create := func(t *testing.T, filesystem bool) *model.ThirdpartyOAuth2ProviderEntity {
		t.Helper()
		serviceID := id.NewServiceID()
		provider := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: new("reports-" + serviceID.String()), DisplayName: "Reports", ClientID: "reports-v1", Secret: model.NewEncryptedSecret([]byte("ciphertext")), IssuerURI: "https://reports.example.com", ProtectedResources: []string{"https://reports.example.com/" + serviceID.String()}, ClientIdentity: new(id.ClientID("untrusted")), ClientIdentityRevision: 99}
		if filesystem {
			provider.CredentialSource = model.CredentialSourceFilesystem
			provider.ClientID = ""
			provider.Secret = model.NewAbsentSecret()
		}
		require.NoError(t, providers.Create(ctx, provider))
		require.Zero(t, provider.ClientIdentityRevision)
		if filesystem {
			require.Nil(t, provider.ClientIdentity)
		} else {
			require.Equal(t, new(id.ClientID("reports-v1")), provider.ClientIdentity)
		}
		return provider
	}
	snapshot := func(p *model.ThirdpartyOAuth2ProviderEntity) ports.ClientIdentityObservation {
		return ports.ClientIdentityObservation{ServiceVersion: p.Version, CanonicalID: *p.CanonicalID, Revision: p.ClientIdentityRevision}
	}
	observe := func(t *testing.T, p *model.ThirdpartyOAuth2ProviderEntity, value id.ClientID, expected ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool) {
		t.Helper()
		state, applied, changed, err := identities.RecordObservedClientIdentity(ctx, p.ID, value, expected)
		require.NoError(t, err)
		return state, applied, changed
	}
	t.Run("stored updates own increments and rollback is a new revision", func(t *testing.T) {
		provider := create(t, false)
		for _, value := range []id.ClientID{"reports-v1", "reports-v2", "reports-v1"} {
			before := provider.Copy()
			provider.ClientID = value
			provider.Secret = model.NewEncryptedSecret([]byte("rotated-ciphertext"))
			provider.DisplayName = "Updated metadata"
			provider.ClientIdentity = new(id.ClientID("forged"))
			provider.ClientIdentityRevision = 100
			changed, err := providers.Update(ctx, provider, &before.Version)
			require.NoError(t, err)
			require.Equal(t, value != before.ClientID, changed)
			revision := before.ClientIdentityRevision
			if changed {
				revision++
			}
			require.Equal(t, revision, provider.ClientIdentityRevision)
			require.Equal(t, new(value), provider.ClientIdentity)
		}
		before, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		provider.ClientID = "rejected"
		changed, err := providers.Update(ctx, provider, new(before.Version-1))
		require.Error(t, err)
		assert.False(t, changed)
		found, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		assert.Equal(t, before, found)
		invalid := found.Copy()
		invalid.ClientID = ""
		changed, err = providers.Update(ctx, invalid, &found.Version)
		require.Error(t, err)
		assert.False(t, changed)
		after, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		assert.Equal(t, before, after)
	})
	t.Run("filesystem metadata update preserves an intervening observation without claiming it", func(t *testing.T) {
		provider := create(t, true)
		initial := snapshot(provider)
		state, applied, changed := observe(t, provider, "reports-v1", initial)
		require.True(t, applied)
		require.False(t, changed)
		require.Zero(t, state.Revision)
		stale, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		state, applied, changed = observe(t, provider, "reports-v2", snapshot(stale))
		require.True(t, applied)
		require.True(t, changed)
		require.Equal(t, int64(1), state.Revision)
		stale.DisplayName = "Metadata only"
		changed, err = providers.Update(ctx, stale, &stale.Version)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, int64(1), stale.ClientIdentityRevision)
		assert.Equal(t, new(id.ClientID("reports-v2")), stale.ClientIdentity)
	})
	t.Run("all observation preconditions apply to initialization changes and no-ops", func(t *testing.T) {
		for _, kind := range []string{"initial", "same", "different"} {
			for _, fence := range []string{"version", "canonical", "revision", "source"} {
				t.Run(kind+"/"+fence, func(t *testing.T) {
					provider := create(t, true)
					if kind != "initial" {
						observe(t, provider, "reports-v1", snapshot(provider))
					}
					current, err := providers.Get(ctx, provider.ID)
					require.NoError(t, err)
					expected := snapshot(current)
					switch fence {
					case "version":
						expected.ServiceVersion++
					case "canonical":
						expected.CanonicalID = "retired-binding"
					case "revision":
						expected.Revision++
					case "source":
						current.CredentialSource = model.CredentialSourceStored
						current.ClientID = "reports-v1"
						current.Secret = model.NewEncryptedSecret([]byte("ciphertext"))
						changed, err := providers.Update(ctx, current, &current.Version)
						require.NoError(t, err)
						require.False(t, changed)
						expected = snapshot(current)
					}
					before, err := providers.Get(ctx, provider.ID)
					require.NoError(t, err)
					value := id.ClientID("reports-v1")
					if kind == "different" {
						value = "reports-v2"
					}
					_, applied, changed := observe(t, provider, value, expected)
					assert.False(t, applied)
					assert.False(t, changed)
					after, err := providers.Get(ctx, provider.ID)
					require.NoError(t, err)
					assert.Equal(t, before, after)
				})
			}
		}
	})
	t.Run("concurrent observations increment once and never mutate admin metadata", func(t *testing.T) {
		provider := create(t, true)
		observe(t, provider, "reports-v1", snapshot(provider))
		before, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		expected := snapshot(before)
		type result struct {
			state            ports.ClientIdentityState
			applied, changed bool
			err              error
		}
		results := make(chan result, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		start := make(chan struct{})
		for range 2 {
			go func() {
				ready.Done()
				<-start
				state, applied, changed, err := identities.RecordObservedClientIdentity(ctx, provider.ID, "reports-v2", expected)
				results <- result{state, applied, changed, err}
			}()
		}
		ready.Wait()
		close(start)
		increments := 0
		for range 2 {
			result := <-results
			require.NoError(t, result.err)
			if result.changed {
				increments++
				assert.True(t, result.applied)
			}
		}
		assert.Equal(t, 1, increments)
		after, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		assert.Equal(t, before.Version, after.Version)
		assert.Equal(t, before.UpdatedAt, after.UpdatedAt)
		require.Equal(t, int64(1), after.ClientIdentityRevision)
		state, applied, changed := observe(t, after, "reports-v2", snapshot(after))
		assert.True(t, applied)
		assert.False(t, changed)
		*state.Identity = "mutated"
		*after.ClientIdentity = "also mutated"
		found, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		assert.Equal(t, new(id.ClientID("reports-v2")), found.ClientIdentity)
	})
	t.Run("source round trips and canonical changes fence obsolete observers without invalidation", func(t *testing.T) {
		for _, roundTrip := range []bool{false, true} {
			provider := create(t, true)
			observe(t, provider, "reports-v1", snapshot(provider))
			old := snapshot(provider)
			current, err := providers.Get(ctx, provider.ID)
			require.NoError(t, err)
			if roundTrip {
				current.CredentialSource = model.CredentialSourceStored
				current.ClientID = "reports-v1"
				current.Secret = model.NewEncryptedSecret([]byte("ciphertext"))
			} else {
				current.CanonicalID = new("changed-" + current.ID.String())
			}
			changed, err := providers.Update(ctx, current, &current.Version)
			require.NoError(t, err)
			require.False(t, changed)
			if roundTrip {
				current.CredentialSource = model.CredentialSourceFilesystem
				current.ClientID = ""
				current.Secret = model.NewAbsentSecret()
				changed, err = providers.Update(ctx, current, &current.Version)
				require.NoError(t, err)
				require.False(t, changed)
			}
			_, applied, changed := observe(t, current, "obsolete", old)
			assert.False(t, applied)
			assert.False(t, changed)
			current.CredentialSource = model.CredentialSourceStored
			current.ClientID = "reports-v1"
			current.Secret = model.NewEncryptedSecret([]byte("rotated"))
			changed, err = providers.Update(ctx, current, &current.Version)
			require.NoError(t, err)
			assert.False(t, changed)
			assert.Zero(t, current.ClientIdentityRevision)
		}
	})
	t.Run("all service projections return independent evidence", func(t *testing.T) {
		provider := create(t, false)
		provider.ClientID = "reports-v2"
		changed, err := providers.Update(ctx, provider, &provider.Version)
		require.NoError(t, err)
		require.True(t, changed)
		batch, err := providers.GetByIDs(ctx, []id.ServiceID{provider.ID})
		require.NoError(t, err)
		require.Len(t, batch, 1)
		all, err := providers.List(ctx)
		require.NoError(t, err)
		resolved, err := providers.FindByProtectedResource(ctx, provider.ProtectedResources[0])
		require.NoError(t, err)
		require.NotNil(t, resolved)
		projections := append(batch, resolved)
		for _, candidate := range all {
			if candidate.ID == provider.ID {
				projections = append(projections, candidate)
			}
		}
		require.Len(t, projections, 3)
		for _, candidate := range projections {
			require.Equal(t, new(id.ClientID("reports-v2")), candidate.ClientIdentity)
			assert.Equal(t, int64(1), candidate.ClientIdentityRevision)
			*candidate.ClientIdentity = "mutated"
		}
		found, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		assert.Equal(t, new(id.ClientID("reports-v2")), found.ClientIdentity)
	})
	t.Run("empty observations and missing services fail without successful outcomes", func(t *testing.T) {
		provider := create(t, true)
		_, applied, changed, err := identities.RecordObservedClientIdentity(ctx, provider.ID, "", snapshot(provider))
		require.Error(t, err)
		assert.False(t, applied)
		assert.False(t, changed)
		_, applied, changed, err = identities.RecordObservedClientIdentity(ctx, id.NewServiceID(), "reports", snapshot(provider))
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))
		assert.False(t, applied)
		assert.False(t, changed)
	})
}
