package thirdparty

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func TestServiceIconRejectsBeforeSideEffects(t *testing.T) {
	for _, operation := range []string{"create", "update", "validate update"} {
		t.Run(operation, func(t *testing.T) {
			var logs bytes.Buffer
			repo := &functionFieldProviderRepository{}
			crypto := &functionFieldEncryption{}
			keys := &functionFieldBranchKeyManager{}
			svc := NewThirdpartyOAuth2ProviderService(repo, crypto, keys, nil, false, slog.New(slog.NewJSONHandler(&logs, nil)))
			entity := minimalValidEntity(id.NewServiceID(), model.NewPlaintextSecret("SECRET_SENTINEL"))
			invalid := `<svg xmlns="http://www.w3.org/2000/svg" onload="SVG_SENTINEL"/>`
			entity.IconSVG, entity.IconSVGProvided = &invalid, true
			resource := "https://must-not-discover.invalid/resource"
			entity.Discovery.ResourceURL = &resource
			before := entity.Copy()
			var err error
			switch operation {
			case "create":
				err = svc.Create(context.Background(), entity)
			case "update":
				err = svc.Update(context.Background(), entity, nil)
			case "validate update":
				err = svc.ValidateUpdate(context.Background(), entity)
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "icon_svg")
			assert.Equal(t, before, entity)
			assert.NotContains(t, logs.String(), "SVG_SENTINEL")
			assert.NotContains(t, logs.String(), "SECRET_SENTINEL")
		})
	}
}

func TestServiceIconPrepareUpdatePresence(t *testing.T) {
	original := `<svg xmlns="http://www.w3.org/2000/svg"><text>original 星</text></svg>`
	replacement := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="8"/></svg>`
	for _, tc := range []struct {
		name     string
		source   *string
		provided bool
		want     *string
	}{
		{"omitted preserves", nil, false, &original},
		{"null clears", nil, true, nil},
		{"source replaces", &replacement, true, &replacement},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := minimalValidEntity(id.NewServiceID(), model.NewEncryptedSecret([]byte("opaque")))
			stored.IconSVG = &original
			repo := &functionFieldProviderRepository{getFn: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) { return stored, nil }}
			svc := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, &functionFieldBranchKeyManager{}, nil, false, slog.New(slog.DiscardHandler))
			candidate := minimalValidEntity(stored.ID, model.NewPlaintextSecret("new-secret"))
			candidate.IconSVG, candidate.IconSVGProvided = tc.source, tc.provided
			_, err := svc.prepareUpdate(context.Background(), candidate)
			require.NoError(t, err)
			assert.Equal(t, tc.want, candidate.IconSVG)
			assert.Equal(t, &original, stored.IconSVG, "preparation must not mutate repository pointers")
			candidate = minimalValidEntity(stored.ID, model.NewAbsentSecret())
			candidate.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
			candidate.IconSVG, candidate.IconSVGProvided = tc.source, tc.provided
			keys := newNoopBranchKeyManager()
			var written *model.ThirdpartyOAuth2ProviderEntity
			repo.updateFn = func(_ context.Context, value *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
				written = value
				return nil
			}
			svc = NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, keys, nil, false, slog.New(slog.DiscardHandler))
			require.NoError(t, svc.Update(context.Background(), candidate, nil))
			require.NotNil(t, written)
			assert.Equal(t, tc.want, written.IconSVG, "actual Update must apply the merge")
			assert.Equal(t, &original, stored.IconSVG)
		})
	}
}
