package thirdparty

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func TestProviderValidationAppliesObjectMetadataFirst(t *testing.T) {
	invalid := model.RestoreObjectMetadata(map[string]string{"bad key": "v"}, nil)
	t.Run("ValidateForCreate", func(t *testing.T) {
		entity := &model.ThirdpartyOAuth2ProviderEntity{ObjectMetadata: invalid}
		assert.ErrorIs(t, entity.ValidateForCreate(false), model.ErrMetadataLabelsInvalidKey)
	})
	t.Run("ValidateForUpdate", func(t *testing.T) {
		entity := &model.ThirdpartyOAuth2ProviderEntity{ObjectMetadata: invalid}
		assert.ErrorIs(t, entity.ValidateForUpdate(false), model.ErrMetadataLabelsInvalidKey)
	})
	t.Run("ValidateDiscoveryRequest", func(t *testing.T) {
		entity := &model.ThirdpartyOAuth2ProviderEntity{ObjectMetadata: invalid}
		assert.ErrorIs(t, entity.ValidateDiscoveryRequest(), model.ErrMetadataLabelsInvalidKey)
	})
	t.Run("a reserved-prefix key passes the re-check", func(t *testing.T) {
		entity := minimalValidEntity(id.NewServiceID(), model.NewPlaintextSecret("secret"))
		entity.ObjectMetadata = model.RestoreObjectMetadata(map[string]string{"agenticidentitybroker.dev/x": "v"}, nil)
		assert.NoError(t, entity.ValidateForUpdate(false))
	})
}

func TestObjectMetadataRejectsBeforeSideEffects(t *testing.T) {
	const sentinel = "zz-metadata-sentinel"
	for _, operation := range []string{"create", "update", "validate update", "discovery create", "discovery update"} {
		t.Run(operation, func(t *testing.T) {
			var logs bytes.Buffer
			calls := 0
			record := func() { calls++ }
			repo := &functionFieldProviderRepository{
				createFn: func(context.Context, *model.ThirdpartyOAuth2ProviderEntity) error { record(); return nil },
				getFn: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
					record()
					return minimalValidEntity(id.NewServiceID(), model.NewEncryptedSecret([]byte("opaque"))), nil
				},
				updateFn: func(context.Context, *model.ThirdpartyOAuth2ProviderEntity, *int64) error { record(); return nil },
			}
			crypto := &functionFieldEncryption{encryptFn: func(context.Context, []byte, map[string]string) ([]byte, error) {
				record()
				return []byte("ciphertext"), nil
			}}
			keys := &functionFieldBranchKeyManager{createFn: func(context.Context, domainencryption.BranchKeySubject) (string, error) {
				record()
				return "key", nil
			}}
			svc := NewThirdpartyOAuth2ProviderService(repo, crypto, keys, nil, false, slog.New(slog.NewJSONHandler(&logs, nil)))
			entity := minimalValidEntity(id.NewServiceID(), model.NewPlaintextSecret("SECRET_SENTINEL"))
			entity.ObjectMetadata = model.RestoreObjectMetadata(nil, map[string]string{sentinel: sentinel + "\x00"})
			entity.ObjectMetadataProvided = true
			if operation == "discovery create" || operation == "discovery update" {
				resource := "https://must-not-discover.invalid/resource"
				entity.Discovery.ResourceURL = &resource
			}
			before := entity.Copy()
			var err error
			switch operation {
			case "create", "discovery create":
				err = svc.Create(context.Background(), entity)
			case "update", "discovery update":
				err = svc.Update(context.Background(), entity, nil)
			case "validate update":
				err = svc.ValidateUpdate(context.Background(), entity)
			}
			require.ErrorIs(t, err, model.ErrMetadataAnnotationsInvalidValue)
			assert.NotContains(t, err.Error(), sentinel)
			assert.Zero(t, calls, "no repository, discovery, branch-key, or encryption call")
			assert.Equal(t, before, entity)
			assert.NotContains(t, logs.String(), sentinel)
		})
	}
}
