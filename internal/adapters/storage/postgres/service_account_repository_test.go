//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func serviceAccountIntegrationFixture(serviceID id.ServiceID, canonicalID string) *model.ServiceAccount {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &model.ServiceAccount{
		ID: id.NewServiceAccountID(), ServiceID: serviceID, CanonicalID: canonicalID, Kind: model.ServiceAccountKindJWTBearer,
		Scopes: []string{"read"}, Credential: model.NewEncryptedSecret([]byte("ciphertext")),
		Summary:   model.CredentialSummary{Format: model.CredentialFormatSignedJWT, Use: model.CredentialUseAccessToken, Fingerprint: "SHA256:x"},
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestServiceAccountRepository_PostgresErrorsAndSpans(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	providers := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	service := &model.ThirdpartyOAuth2ProviderEntity{
		ID: id.NewServiceID(), DisplayName: "Company Platform", ClientID: "platform-client",
		Secret: model.NewEncryptedSecret([]byte("secret")), IssuerURI: "https://idp.example.test",
		Endpoints: model.OAuth2Endpoints{TokenEndpoint: "https://idp.example.test/token", AuthorizeEndpoint: "https://idp.example.test/authorize"},
	}
	require.NoError(t, providers.Create(ctx, service))
	repo := NewServiceAccountRepository(adapter)
	first := serviceAccountIntegrationFixture(service.ID, "platform-triage-bot")
	require.NoError(t, repo.Create(ctx, first))
	second := serviceAccountIntegrationFixture(service.ID, "platform-planner-bot")
	require.NoError(t, repo.Create(ctx, second))

	t.Run("a taken name maps 23505 to the canonical-ID sentinel", func(t *testing.T) {
		recorder := recordStorageSpans(t)
		err := repo.Create(ctx, serviceAccountIntegrationFixture(service.ID, "platform-triage-bot"))
		require.ErrorIs(t, err, ports.ErrServiceAccountCanonicalIDConflict)
		assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, "storage.create.service_account"), codes.Unset, "", "conflict")
	})

	t.Run("a rename to a taken name maps to the canonical-ID sentinel", func(t *testing.T) {
		recorder := recordStorageSpans(t)
		renamed := *first
		renamed.CanonicalID = "platform-planner-bot"
		require.ErrorIs(t, repo.Update(ctx, &renamed, nil), ports.ErrServiceAccountCanonicalIDConflict)
		assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, "storage.update.service_account"), codes.Unset, "", "conflict")
	})

	t.Run("a stale version maps to the version sentinel", func(t *testing.T) {
		recorder := recordStorageSpans(t)
		require.ErrorIs(t, repo.Update(ctx, first, new(int64(7))), ports.ErrServiceAccountVersionMismatch)
		assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, "storage.update.service_account"), codes.Unset, "", "conflict")

		recorder = recordStorageSpans(t)
		require.ErrorIs(t, repo.Delete(ctx, service.ID, first.ID, new(int64(7))), ports.ErrServiceAccountVersionMismatch)
		assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, "storage.delete.service_account"), codes.Unset, "", "conflict")
	})

	t.Run("a missing account is a not-found outcome", func(t *testing.T) {
		recorder := recordStorageSpans(t)
		_, err := repo.Get(ctx, service.ID, id.NewServiceAccountID())
		require.True(t, ports.IsNotFoundErr(err))
		assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, "storage.get.service_account"), codes.Unset, "", "not_found")

		recorder = recordStorageSpans(t)
		_, err = repo.GetByCanonicalID(ctx, service.ID, "unknown-bot")
		require.True(t, ports.IsNotFoundErr(err))
		assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, "storage.get_by_canonical_id.service_account"), codes.Unset, "", "not_found")

		recorder = recordStorageSpans(t)
		require.True(t, ports.IsNotFoundErr(repo.Delete(ctx, service.ID, id.NewServiceAccountID(), nil)))
		assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, "storage.delete.service_account"), codes.Unset, "", "not_found")
	})

	t.Run("successful reads end their spans without an outcome", func(t *testing.T) {
		recorder := recordStorageSpans(t)
		_, err := repo.ListByService(ctx, service.ID)
		require.NoError(t, err)
		assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, "storage.list.service_account"), codes.Unset, "", "")

		recorder = recordStorageSpans(t)
		count, err := repo.CountByService(ctx, service.ID)
		require.NoError(t, err)
		require.Equal(t, 2, count)
		assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, "storage.count.service_account"), codes.Unset, "", "")
	})
}
