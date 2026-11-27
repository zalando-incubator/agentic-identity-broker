// Package contract holds repository contracts that every storage backend must pass.
package contract

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// NewStorage returns a fresh, empty storage adapter for one subtest.
type NewStorage func(t *testing.T) *storageadapter.Adapter

func contractService(t *testing.T, st *storageadapter.Adapter) id.ServiceID {
	t.Helper()
	serviceID := id.NewServiceID()
	now := time.Now().UTC().Truncate(time.Microsecond)
	service := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceID,
		DisplayName: "Contract Service " + serviceID.String(),
		ClientID:    id.ClientID("contract-" + serviceID.String()),
		Secret:      model.NewEncryptedSecret([]byte("opaque service secret")),
		IssuerURI:   "https://issuer.example.test",
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://issuer.example.test/token",
			AuthorizeEndpoint: "https://issuer.example.test/authorize",
		},
		Scopes:    []model.OAuthScope{{ScopeValue: "read", Description: "Read"}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, st.Services().Create(context.Background(), service))
	return serviceID
}

func contractAccount(serviceID id.ServiceID, canonicalID string) *model.ServiceAccount {
	now := time.Now().UTC().Truncate(time.Microsecond)
	expiry := now.Add(time.Hour)
	exp := expiry.Unix()
	return &model.ServiceAccount{
		ID:          id.NewServiceAccountID(),
		ServiceID:   serviceID,
		CanonicalID: canonicalID,
		Kind:        model.ServiceAccountKindJWTBearer,
		Scopes:      []string{"read", "write"},
		Credential:  model.NewEncryptedSecret([]byte("ciphertext of " + canonicalID)),
		Summary: model.CredentialSummary{
			Format: model.CredentialFormatSignedJWT, Use: model.CredentialUseAccessToken,
			Iss: new("https://idp.example.test"), Sub: new("triage-bot"), Kid: new("2026-11"), Exp: &exp,
			Fingerprint: "SHA256:fhbwk8g0dDlEiuXzadc7Ytx+R1pSlxRBBNgzBxN+3UQ",
		},
		CredentialExpiresAt: &expiry,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

func requireKind(t *testing.T, err error, kind storage.ErrorKind) {
	t.Helper()
	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr), "expected a StorageError, got %v", err)
	assert.Equal(t, kind, storageErr.Kind)
}

func requireSentinel(t *testing.T, err error, sentinel error) {
	t.Helper()
	requireKind(t, err, storage.ErrorKindConflict)
	assert.True(t, errors.Is(err, sentinel), "expected %v, got %v", sentinel, err)
}

func assertSameAccount(t *testing.T, want, got *model.ServiceAccount) {
	t.Helper()
	require.NotNil(t, got)
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.ServiceID, got.ServiceID)
	assert.Equal(t, want.CanonicalID, got.CanonicalID)
	assert.Equal(t, want.Kind, got.Kind)
	assert.Equal(t, want.Scopes, got.Scopes)
	wantCiphertext, err := want.Credential.GetCiphertext()
	require.NoError(t, err)
	gotCiphertext, err := got.Credential.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, wantCiphertext, gotCiphertext)
	assert.Equal(t, want.Summary, got.Summary)
	if want.CredentialExpiresAt == nil {
		assert.Nil(t, got.CredentialExpiresAt)
	} else {
		require.NotNil(t, got.CredentialExpiresAt)
		assert.True(t, want.CredentialExpiresAt.Equal(*got.CredentialExpiresAt))
	}
	assert.Equal(t, want.Version, got.Version)
	assert.True(t, want.CreatedAt.Equal(got.CreatedAt), "created_at %v != %v", want.CreatedAt, got.CreatedAt)
	assert.True(t, want.UpdatedAt.Equal(got.UpdatedAt), "updated_at %v != %v", want.UpdatedAt, got.UpdatedAt)
}

// RunServiceAccountRepository checks the ports.ServiceAccountRepository contract.
func RunServiceAccountRepository(t *testing.T, newStorage NewStorage) {
	ctx := context.Background()

	t.Run("create sets version 1 and reads return the stored account", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		account := contractAccount(contractService(t, st), "platform-triage-bot")

		require.NoError(t, repo.Create(ctx, account))

		assert.Equal(t, int64(1), account.Version)
		byID, err := repo.Get(ctx, account.ServiceID, account.ID)
		require.NoError(t, err)
		assertSameAccount(t, account, byID)
		byName, err := repo.GetByCanonicalID(ctx, account.ServiceID, "platform-triage-bot")
		require.NoError(t, err)
		assertSameAccount(t, account, byName)
	})

	t.Run("an account without expiry and summary claims round-trips", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		account := contractAccount(contractService(t, st), "no-expiry")
		account.CredentialExpiresAt = nil
		account.Summary = model.CredentialSummary{Format: model.CredentialFormatSignedJWT, Use: model.CredentialUseAccessToken, Fingerprint: "SHA256:x"}
		require.NoError(t, repo.Create(ctx, account))
		stored, err := repo.Get(ctx, account.ServiceID, account.ID)
		require.NoError(t, err)
		assertSameAccount(t, account, stored)
	})

	t.Run("create under an unknown service is not found", func(t *testing.T) {
		st := newStorage(t)
		account := contractAccount(id.NewServiceID(), "orphan")
		requireKind(t, st.ServiceAccounts().Create(ctx, account), storage.ErrorKindNotFound)
		count, err := st.ServiceAccounts().CountByService(ctx, account.ServiceID)
		require.NoError(t, err)
		assert.Zero(t, count)
	})

	t.Run("reads are scoped by service", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		account := contractAccount(contractService(t, st), "platform-triage-bot")
		require.NoError(t, repo.Create(ctx, account))
		other := contractService(t, st)

		_, err := repo.Get(ctx, other, account.ID)
		requireKind(t, err, storage.ErrorKindNotFound)
		_, err = repo.GetByCanonicalID(ctx, other, "platform-triage-bot")
		requireKind(t, err, storage.ErrorKindNotFound)
		_, err = repo.Get(ctx, account.ServiceID, id.NewServiceAccountID())
		requireKind(t, err, storage.ErrorKindNotFound)
		listed, err := repo.ListByService(ctx, other)
		require.NoError(t, err)
		assert.NotNil(t, listed)
		assert.Empty(t, listed)
	})

	t.Run("list is ordered by canonical_id in byte order", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		serviceID := contractService(t, st)
		for _, name := range []string{"b", "a_1", "B", "a-1", "a.1"} {
			require.NoError(t, repo.Create(ctx, contractAccount(serviceID, name)))
		}
		listed, err := repo.ListByService(ctx, serviceID)
		require.NoError(t, err)
		names := make([]string, len(listed))
		for i, account := range listed {
			names[i] = account.CanonicalID
		}
		assert.Equal(t, []string{"B", "a-1", "a.1", "a_1", "b"}, names)
		count, err := repo.CountByService(ctx, serviceID)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
	})

	t.Run("canonical_id is unique within a service and free across services", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		serviceID := contractService(t, st)
		require.NoError(t, repo.Create(ctx, contractAccount(serviceID, "platform-triage-bot")))

		requireSentinel(t, repo.Create(ctx, contractAccount(serviceID, "platform-triage-bot")), ports.ErrServiceAccountCanonicalIDConflict)
		require.NoError(t, repo.Create(ctx, contractAccount(contractService(t, st), "platform-triage-bot")))
		count, err := repo.CountByService(ctx, serviceID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("update writes every mutable member and increments the version", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		account := contractAccount(contractService(t, st), "platform-triage-bot")
		require.NoError(t, repo.Create(ctx, account))

		changed := *account
		changed.CanonicalID = "platform-triage"
		changed.Scopes = []string{"admin"}
		changed.Credential = model.NewEncryptedSecret([]byte("rotated ciphertext"))
		changed.Summary.Kid = new("2026-12")
		changed.CredentialExpiresAt = nil
		changed.UpdatedAt = account.UpdatedAt.Add(time.Minute)
		require.NoError(t, repo.Update(ctx, &changed, nil))

		assert.Equal(t, int64(2), changed.Version)
		stored, err := repo.Get(ctx, account.ServiceID, account.ID)
		require.NoError(t, err)
		assertSameAccount(t, &changed, stored)
		_, err = repo.GetByCanonicalID(ctx, account.ServiceID, "platform-triage-bot")
		requireKind(t, err, storage.ErrorKindNotFound)
		require.NoError(t, repo.Create(ctx, contractAccount(account.ServiceID, "platform-triage-bot")), "a rename frees the old name")
	})

	t.Run("update to a taken name is a canonical-ID conflict and changes nothing", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		serviceID := contractService(t, st)
		first, second := contractAccount(serviceID, "platform-triage-bot"), contractAccount(serviceID, "platform-planner-bot")
		require.NoError(t, repo.Create(ctx, first))
		require.NoError(t, repo.Create(ctx, second))

		renamed := *first
		renamed.CanonicalID = "platform-planner-bot"
		requireSentinel(t, repo.Update(ctx, &renamed, nil), ports.ErrServiceAccountCanonicalIDConflict)
		stored, err := repo.Get(ctx, serviceID, first.ID)
		require.NoError(t, err)
		assertSameAccount(t, first, stored)
	})

	t.Run("update honours a supplied version and is unconditional without one", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		account := contractAccount(contractService(t, st), "platform-triage-bot")
		require.NoError(t, repo.Create(ctx, account))

		stale := *account
		stale.Scopes = []string{"stale"}
		requireSentinel(t, repo.Update(ctx, &stale, new(int64(2))), ports.ErrServiceAccountVersionMismatch)
		stored, err := repo.Get(ctx, account.ServiceID, account.ID)
		require.NoError(t, err)
		assertSameAccount(t, account, stored)

		current := *account
		current.Scopes = []string{"current"}
		require.NoError(t, repo.Update(ctx, &current, new(int64(1))))
		assert.Equal(t, int64(2), current.Version)
		unconditional := current
		unconditional.Scopes = []string{"unconditional"}
		require.NoError(t, repo.Update(ctx, &unconditional, nil))
		assert.Equal(t, int64(3), unconditional.Version)
	})

	t.Run("a credential that is not ciphertext is refused and nothing is stored", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		serviceID := contractService(t, st)
		plaintext := contractAccount(serviceID, "platform-triage-bot")
		plaintext.Credential = model.NewPlaintextSecret("plaintext")
		requireKind(t, repo.Create(ctx, plaintext), storage.ErrorKindValidation)
		count, err := repo.CountByService(ctx, serviceID)
		require.NoError(t, err)
		assert.Zero(t, count)

		account := contractAccount(serviceID, "platform-triage-bot")
		require.NoError(t, repo.Create(ctx, account))
		changed := *account
		changed.Scopes = []string{"changed"}
		changed.Credential = model.NewPlaintextSecret("plaintext")
		requireKind(t, repo.Update(ctx, &changed, nil), storage.ErrorKindValidation)
		stored, err := repo.Get(ctx, serviceID, account.ID)
		require.NoError(t, err)
		assertSameAccount(t, account, stored)
	})

	t.Run("update of a missing or foreign account is not found", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		account := contractAccount(contractService(t, st), "platform-triage-bot")
		requireKind(t, repo.Update(ctx, account, nil), storage.ErrorKindNotFound)
		requireKind(t, repo.Update(ctx, account, new(int64(1))), storage.ErrorKindNotFound)
		require.NoError(t, repo.Create(ctx, account))
		foreign := *account
		foreign.ServiceID = contractService(t, st)
		requireKind(t, repo.Update(ctx, &foreign, nil), storage.ErrorKindNotFound)
	})

	t.Run("delete honours a supplied version and erases the account", func(t *testing.T) {
		st := newStorage(t)
		repo := st.ServiceAccounts()
		account := contractAccount(contractService(t, st), "platform-triage-bot")
		require.NoError(t, repo.Create(ctx, account))

		requireSentinel(t, repo.Delete(ctx, account.ServiceID, account.ID, new(int64(2))), ports.ErrServiceAccountVersionMismatch)
		requireKind(t, repo.Delete(ctx, contractService(t, st), account.ID, nil), storage.ErrorKindNotFound)
		require.NoError(t, repo.Delete(ctx, account.ServiceID, account.ID, new(int64(1))))

		_, err := repo.Get(ctx, account.ServiceID, account.ID)
		requireKind(t, err, storage.ErrorKindNotFound)
		requireKind(t, repo.Delete(ctx, account.ServiceID, account.ID, nil), storage.ErrorKindNotFound)
		requireKind(t, repo.Delete(ctx, account.ServiceID, account.ID, new(int64(1))), storage.ErrorKindNotFound)
		count, err := repo.CountByService(ctx, account.ServiceID)
		require.NoError(t, err)
		assert.Zero(t, count)
	})

	t.Run("a service that holds an account cannot be deleted", func(t *testing.T) {
		st := newStorage(t)
		account := contractAccount(contractService(t, st), "platform-triage-bot")
		require.NoError(t, st.ServiceAccounts().Create(ctx, account))

		requireKind(t, st.Services().Delete(ctx, account.ServiceID), storage.ErrorKindConflict)
		_, err := st.Services().Get(ctx, account.ServiceID)
		require.NoError(t, err)
		require.NoError(t, st.ServiceAccounts().Delete(ctx, account.ServiceID, account.ID, nil))
		require.NoError(t, st.Services().Delete(ctx, account.ServiceID))
	})
}
