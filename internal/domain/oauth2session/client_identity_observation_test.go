package oauth2session

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestResolveClientIdentityCommittedObservations(t *testing.T) {
	for _, tc := range []struct {
		name     string
		previous *id.ClientID
		observed string
		revision int64
		changed  bool
	}{
		{name: "initialization", observed: "client-A"},
		{name: "same ID", previous: identityPointer("client-A"), observed: "client-A", revision: 2},
		{name: "changed ID", previous: identityPointer("client-A"), observed: "client-B", revision: 3, changed: true},
		{name: "stale replica or rollback", previous: identityPointer("client-B"), observed: "client-A", revision: 4, changed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			canonical := "first"
			serviceID := id.NewServiceID()
			entity := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, Version: 7, CredentialSource: model.CredentialSourceFilesystem, ClientIdentity: tc.previous, ClientIdentityRevision: tc.revision}
			service, logs := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				t.Fatal("matched observations do not reload configuration")
				return nil, nil
			})
			service.credentialFileReader = identityReader{read: func(path string) (string, error) { assert.Equal(t, "first-file", path); return tc.observed, nil }}
			service.WithClientIdentityRepository(identityObservationRepository{observe: func(_ context.Context, got id.ServiceID, value id.ClientID, expected ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
				assert.Equal(t, serviceID, got)
				assert.Equal(t, id.ClientID(tc.observed), value)
				assert.Equal(t, ports.ClientIdentityObservation{ServiceVersion: 7, CanonicalID: canonical, Revision: tc.revision}, expected)
				revision := tc.revision
				if tc.changed {
					revision++
				}
				return ports.ClientIdentityState{Identity: identityPointer(tc.observed), Revision: revision}, true, tc.changed, nil
			}})
			state, err := service.resolveClientIdentity(context.Background(), entity)
			require.NoError(t, err)
			assert.Equal(t, identityPointer(tc.observed), state.Identity)
			assert.Equal(t, state.Identity, entity.ClientIdentity)
			assert.Equal(t, state.Revision, entity.ClientIdentityRevision)
			assert.Equal(t, int64(7), entity.Version)
			if tc.changed {
				assert.Contains(t, logs.String(), `"event":"session.oauth2.client_identity_changed"`)
				assert.Contains(t, logs.String(), fmt.Sprintf(`"client_identity_revision":%d`, tc.revision+1))
				assert.Contains(t, logs.String(), `"level":"INFO"`)
			} else {
				assert.Empty(t, logs.String())
			}
			assert.NotContains(t, logs.String(), tc.observed)
			assert.NotContains(t, logs.String(), "first-file")
		})
	}
}

func TestIdentityDisplayFallbackUsesReloadedEvidence(t *testing.T) {
	serviceID := id.NewServiceID()
	canonical := "first"
	gets, reads := 0, 0
	service, logs := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
		gets++
		revision := int64(0)
		if gets > 1 {
			revision = 2
		}
		return &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, Version: int64(gets), CredentialSource: model.CredentialSourceFilesystem, ClientIdentity: identityPointer("current"), ClientIdentityRevision: revision}, nil
	})
	service.credentialFileReader = identityReader{read: func(string) (string, error) {
		reads++
		if reads > 1 {
			return "", model.NewCredentialSourceError(model.CredentialSourceReasonReadFailed)
		}
		return "obsolete", nil
	}}
	service.WithClientIdentityRepository(identityObservationRepository{observe: func(context.Context, id.ServiceID, id.ClientID, ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
		return ports.ClientIdentityState{}, false, false, nil
	}})
	result, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{{ServiceID: serviceID, UpstreamClientID: identityPointer("current")}}, ports.UsabilityDisplay)
	require.NoError(t, err)
	assert.True(t, result[serviceID], "fallback must not revive a session using evidence from the rejected configuration")
	assert.Equal(t, 2, gets)
	assert.Equal(t, 2, reads)
	assert.NotContains(t, logs.String(), "session.oauth2.client_identity_changed")
	assert.NotContains(t, logs.String(), "obsolete")
}

func TestIdentityEvaluatorLoadsEachServiceOnce(t *testing.T) {
	serviceID := id.NewServiceID()
	gets := 0
	service, _ := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
		gets++
		return &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, ClientID: "current", CredentialSource: model.CredentialSourceStored, Secret: model.NewEncryptedSecret([]byte("encrypted-secret"))}, nil
	})
	result, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{nil, {ServiceID: serviceID, UpstreamClientID: identityPointer("current")}, {ServiceID: serviceID, UpstreamClientID: identityPointer("old")}}, ports.UsabilityGate)
	require.NoError(t, err)
	assert.Equal(t, 1, gets)
	assert.True(t, result[serviceID])
}

func TestIdentityEvaluatorUnavailableService(t *testing.T) {
	for _, mode := range []ports.UsabilityMode{ports.UsabilityGate, ports.UsabilityDisplay} {
		t.Run(fmt.Sprintf("mode=%d", mode), func(t *testing.T) {
			serviceID := id.NewServiceID()
			service, logs := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return nil, ports.ErrNotFound
			})
			result, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{{ServiceID: serviceID}}, mode)
			if mode == ports.UsabilityGate {
				assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.True(t, result[serviceID])
				assert.Contains(t, logs.String(), `"reason":"read_failed"`)
			}
		})
	}
}
