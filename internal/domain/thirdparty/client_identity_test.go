package thirdparty

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestGetForIdentityEvaluation(t *testing.T) {
	for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		t.Run(string(source), func(t *testing.T) {
			serviceID := id.NewServiceID()
			identity := id.ClientID("current")
			stored := minimalValidEntity(serviceID, model.NewEncryptedSecret([]byte("opaque-secret")))
			stored.CredentialSource = source
			stored.ClientIdentity = &identity
			stored.ClientIdentityRevision = 4
			repo := &functionFieldProviderRepository{getFn: func(_ context.Context, got id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				require.Equal(t, serviceID, got)
				return stored, nil
			}}
			encryption := &functionFieldEncryption{decryptFn: func(context.Context, []byte, map[string]string) ([]byte, error) {
				t.Fatal("identity evaluation must not decrypt secrets")
				return nil, nil
			}}
			service := NewThirdpartyOAuth2ProviderService(repo, encryption, newNoopBranchKeyManager(), nil, false, slog.New(slog.DiscardHandler))
			got, err := service.GetForIdentityEvaluation(context.Background(), serviceID)
			require.NoError(t, err)
			assert.True(t, got.Secret.IsAbsent())
			assert.Equal(t, stored.ClientIdentity, got.ClientIdentity)
			assert.Equal(t, int64(4), got.ClientIdentityRevision)
			*got.ClientIdentity = "mutated"
			assert.Equal(t, id.ClientID("current"), *stored.ClientIdentity)
			assert.True(t, stored.Secret.IsEncrypted())
		})
	}
}

func TestGetForIdentityEvaluationUnavailable(t *testing.T) {
	for _, problem := range []string{"missing", "nil", "wrong ID"} {
		t.Run(problem, func(t *testing.T) {
			repo := &functionFieldProviderRepository{getFn: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				switch problem {
				case "missing":
					return nil, ports.ErrNotFound
				case "nil":
					return nil, nil
				default:
					return &model.ThirdpartyOAuth2ProviderEntity{ID: id.NewServiceID()}, nil
				}
			}}
			service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, newNoopBranchKeyManager(), nil, false, slog.New(slog.DiscardHandler))
			got, err := service.GetForIdentityEvaluation(context.Background(), id.NewServiceID())
			require.Error(t, err)
			assert.Nil(t, got)
			if problem == "missing" {
				assert.ErrorIs(t, err, ports.ErrNotFound)
			}
		})
	}
}
