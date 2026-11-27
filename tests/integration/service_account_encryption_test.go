package integration_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/aws"
	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type failingEncryption struct{}

func (failingEncryption) Encrypt(context.Context, []byte, map[string]string) ([]byte, error) {
	return nil, errors.New("encryption unavailable")
}

func (failingEncryption) Decrypt(context.Context, []byte, map[string]string) ([]byte, error) {
	return nil, errors.New("encryption unavailable")
}

func serviceAccountEncryptionStore(t *testing.T) (*storageadapter.Adapter, id.ServiceID, id.ServiceID) {
	t.Helper()
	st, err := storageadapter.NewAdapter(&ports.StorageConfig{Backend: "memory"}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close(context.Background()) })
	ids := []id.ServiceID{id.NewServiceID(), id.NewServiceID()}
	for _, serviceID := range ids {
		require.NoError(t, st.Services().Create(context.Background(), &model.ThirdpartyOAuth2ProviderEntity{
			ID: serviceID, DisplayName: "Service " + serviceID.String(), ClientID: id.ClientID("client-" + serviceID.String()),
			Secret: model.NewEncryptedSecret([]byte("opaque")), IssuerURI: "https://idp.example.test",
			Endpoints: model.OAuth2Endpoints{TokenEndpoint: "https://idp.example.test/token", AuthorizeEndpoint: "https://idp.example.test/authorize"},
		}))
	}
	return st, ids[0], ids[1]
}

func signedServiceAccountJWT(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signed, err := jws.Sign([]byte(`{"sub":"triage-bot","exp":`+strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)+`}`), jws.WithKey(jwa.ES256(), key))
	require.NoError(t, err)
	return string(signed)
}

func createRequest(jwt string) thirdparty.ServiceAccountCreate {
	return thirdparty.ServiceAccountCreate{
		CanonicalID: "platform-triage-bot", Kind: model.ServiceAccountKindJWTBearer, Scopes: []string{"read"},
		Credential: &model.ServiceAccountCredentialInput{Format: model.CredentialFormatSignedJWT, JWT: model.NewPlaintextSecret(jwt)},
	}
}

// SR-001: a credential encrypted for one service does not decrypt under another service's context.
func TestServiceAccountCredentialIsBoundToItsService(t *testing.T) {
	ctx := context.Background()
	encryption, _, err := awsencryption.NewAWSEncryption("ASNFZ4mrze/+3LqYdlQyEAEjRWeJq83v/ty6mHZUMhA=", "", 0, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	st, owner, other := serviceAccountEncryptionStore(t)
	service := thirdparty.NewServiceAccountService(st.ServiceAccounts(), encryption, st.Services(), thirdparty.NoServiceAccountBindings{}, time.Now, slog.New(slog.DiscardHandler))
	jwt := signedServiceAccountJWT(t)

	view, err := service.Create(ctx, "ops-admin@example.com", owner, createRequest(jwt))
	require.NoError(t, err)

	stored, err := st.ServiceAccounts().Get(ctx, owner, view.Account.ID)
	require.NoError(t, err)
	ciphertext, err := stored.Credential.GetCiphertext()
	require.NoError(t, err)
	assert.NotContains(t, string(ciphertext), jwt)

	plaintext, err := encryption.Decrypt(ctx, ciphertext, domainencryption.NewServiceBranchKeySubject(owner).EncryptionContext())
	require.NoError(t, err)
	assert.Contains(t, string(plaintext), jwt)

	_, err = encryption.Decrypt(ctx, ciphertext, domainencryption.NewServiceBranchKeySubject(other).EncryptionContext())
	require.Error(t, err, "another service's context must not decrypt the credential")
}

// SR-002: without encryption nothing is stored; there is no plaintext fallback.
func TestServiceAccountCreateWithFailingEncryptionStoresNothing(t *testing.T) {
	ctx := context.Background()
	st, owner, _ := serviceAccountEncryptionStore(t)
	service := thirdparty.NewServiceAccountService(st.ServiceAccounts(), failingEncryption{}, st.Services(), thirdparty.NoServiceAccountBindings{}, time.Now, slog.New(slog.DiscardHandler))

	_, err := service.Create(ctx, "ops-admin@example.com", owner, createRequest(signedServiceAccountJWT(t)))

	require.Error(t, err)
	count, err := st.ServiceAccounts().CountByService(ctx, owner)
	require.NoError(t, err)
	assert.Zero(t, count)
}
