package thirdparty

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func TestObjectMetadataPrepareUpdatePresence(t *testing.T) {
	original, err := model.NewObjectMetadata(map[string]string{"tier": "gold", "team": "platform"}, map[string]string{"note": "original"})
	require.NoError(t, err)
	labelsOnly, err := model.NewObjectMetadata(map[string]string{"tier": "silver"}, nil)
	require.NoError(t, err)
	both, err := model.NewObjectMetadata(map[string]string{"zone": "eu"}, map[string]string{"owner": "x"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name     string
		value    model.ObjectMetadata
		provided bool
		want     model.ObjectMetadata
	}{
		{"omitted keeps the persisted value", model.ObjectMetadata{}, false, original},
		{"one member replaces both maps without merging", labelsOnly, true, labelsOnly},
		{"both members replace both maps", both, true, both},
		{"empty object clears both maps", model.ObjectMetadata{}, true, model.ObjectMetadata{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := minimalValidEntity(id.NewServiceID(), model.NewEncryptedSecret([]byte("opaque")))
			stored.ObjectMetadata = original
			stored.Version = 4
			repo := &functionFieldProviderRepository{getFn: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return stored.Copy(), nil
			}}
			candidate := minimalValidEntity(stored.ID, model.NewAbsentSecret())
			candidate.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
			candidate.ObjectMetadata, candidate.ObjectMetadataProvided = tc.value, tc.provided
			var written *model.ThirdpartyOAuth2ProviderEntity
			repo.updateFn = func(_ context.Context, value *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
				written = value.Copy()
				return nil
			}
			svc := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, newNoopBranchKeyManager(), nil, false, slog.New(slog.DiscardHandler))
			require.NoError(t, svc.Update(context.Background(), candidate, nil))
			require.NotNil(t, written)
			assert.True(t, tc.want.Equal(written.ObjectMetadata), "stored %v %v", written.ObjectMetadata.Labels(), written.ObjectMetadata.Annotations())
			assert.True(t, original.Equal(stored.ObjectMetadata), "preparation must not mutate the persisted entity")
		})
	}
}
