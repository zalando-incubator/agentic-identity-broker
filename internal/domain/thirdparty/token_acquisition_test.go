package thirdparty

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/url"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenAcquisitionRetainsDecryptionCauseWithoutSerializingIt(t *testing.T) {
	const secret = "SENTINEL_PROVIDER_DECRYPTION_SECRET"
	serviceID := id.NewServiceID()
	stored := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, Secret: model.NewEncryptedSecret([]byte("ciphertext"))}
	cause := &url.Error{Op: "decrypt", URL: "https://provider.example/" + secret, Err: errors.New(secret)}
	repo := &functionFieldProviderRepository{getFn: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) { return stored, nil }}
	encryption := &functionFieldEncryption{decryptFn: func(context.Context, []byte, map[string]string) ([]byte, error) { return nil, cause }}
	var logs bytes.Buffer
	service := NewThirdpartyOAuth2ProviderService(repo, encryption, newNoopBranchKeyManager(), nil, false, slog.New(slog.NewJSONHandler(&logs, nil)))
	result, err := service.GetForTokenAcquisition(context.Background(), serviceID)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrSecretDecryption)
	assert.ErrorIs(t, err, cause)
	var original *url.Error
	require.ErrorAs(t, err, &original)
	assert.Same(t, cause, original)
	assert.True(t, stored.Secret.IsEncrypted())
	administrative, err := service.Get(context.Background(), serviceID)
	require.NoError(t, err)
	assert.True(t, administrative.Secret.IsEncrypted())
	assert.NotContains(t, logs.String(), secret)
}

func TestTokenAcquisitionRejectsMismatchedProviderIdentity(t *testing.T) {
	requested := id.NewServiceID()
	repo := &functionFieldProviderRepository{getFn: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
		return &model.ThirdpartyOAuth2ProviderEntity{ID: id.NewServiceID(), Secret: model.NewPlaintextSecret("secret")}, nil
	}}
	service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, newNoopBranchKeyManager(), nil, false, slog.Default())
	_, err := service.GetForTokenAcquisition(context.Background(), requested)
	assert.ErrorIs(t, err, ErrProviderConfiguration)
}
