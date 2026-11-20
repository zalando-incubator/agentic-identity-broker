//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/postgres"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceIconStorageParity(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			var repo ports.ThirdpartyOAuth2ProviderRepository = memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
			if backend == "postgres" {
				_, postgresRepo, _, cleanup := setupThirdpartyProviderTestHarness(t)
				defer cleanup()
				repo = postgresRepo
			}
			const source = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n<svg xmlns=\"http://www.w3.org/2000/svg\"><text>Ä &amp; 東京 🗺</text></svg>\n"
			icon := source
			canonicalID := "atlas-files"
			first := createTestService(id.NewServiceID().String(), "Atlas Files", []string{"https://api.example.test/atlas"})
			first.Secret = model.NewEncryptedSecret([]byte("opaque credential bytes"))
			first.CanonicalID = &canonicalID
			second := createTestService(id.NewServiceID().String(), "Other Files", nil)
			second.Secret = model.NewEncryptedSecret([]byte("other credential bytes"))
			require.NoError(t, repo.Create(ctx, first))
			absent, err := repo.Get(ctx, first.ID)
			require.NoError(t, err)
			assert.Nil(t, absent.IconSVG)
			first.IconSVG, first.IconSVGProvided = &icon, true
			require.NoError(t, repo.Update(ctx, first, nil))
			second.IconSVG, second.IconSVGProvided = &icon, true
			require.NoError(t, repo.Create(ctx, second))
			icon = "caller mutation"

			check := func(entity *model.ThirdpartyOAuth2ProviderEntity) {
				t.Helper()
				require.NotNil(t, entity.IconSVG)
				assert.Equal(t, []byte(source), []byte(*entity.IconSVG))
				assert.False(t, entity.IconSVGProvided)
				*entity.IconSVG = "reader mutation"
			}
			stored, err := repo.Get(ctx, first.ID)
			require.NoError(t, err)
			check(stored)
			canonical, err := repo.(ports.ThirdpartyOAuth2ProviderCanonicalIDRepository).GetByCanonicalID(ctx, canonicalID)
			require.NoError(t, err)
			check(canonical)
			found, err := repo.FindByProtectedResource(ctx, first.ProtectedResources[0])
			require.NoError(t, err)
			check(found)
			batch, err := repo.GetByIDs(ctx, []id.ServiceID{first.ID, second.ID, first.ID, id.NewServiceID()})
			require.NoError(t, err)
			require.Len(t, batch, 2)
			assert.Equal(t, first.ID, batch[0].ID)
			assert.Equal(t, second.ID, batch[1].ID)
			for _, entity := range batch {
				check(entity)
			}
			listed, err := repo.List(ctx)
			require.NoError(t, err)
			require.Len(t, listed, 2)
			for _, entity := range listed {
				check(entity)
			}

			before, err := repo.Get(ctx, first.ID)
			require.NoError(t, err)
			check(before.Copy())
			replacement := "<svg xmlns=\"http://www.w3.org/2000/svg\"><text>新しい # % &amp;</text></svg>\n"
			candidate := before.Copy()
			candidate.IconSVG, candidate.IconSVGProvided = &replacement, true
			version := before.Version
			require.NoError(t, repo.Update(ctx, candidate, &version))
			updated, err := repo.Get(ctx, first.ID)
			require.NoError(t, err)
			expected := before.Copy()
			expected.IconSVG, expected.Version = &replacement, version+1
			assert.Equal(t, expected, updated, "icon replacement must preserve all other service state")
			replacement = "write mutation"
			updated.IconSVG, updated.IconSVGProvided = nil, true
			require.Error(t, repo.Update(ctx, updated, &version))
			unchanged, err := repo.Get(ctx, first.ID)
			require.NoError(t, err)
			require.NotNil(t, unchanged.IconSVG)
			assert.Contains(t, *unchanged.IconSVG, "新しい")
			assert.Equal(t, version+1, unchanged.Version)
			require.NoError(t, repo.Update(ctx, updated, nil))
			cleared, err := repo.Get(ctx, first.ID)
			require.NoError(t, err)
			assert.Nil(t, cleared.IconSVG)
			assert.False(t, cleared.IconSVGProvided)
			other, err := repo.Get(ctx, second.ID)
			require.NoError(t, err)
			check(other)
		})
	}
}

func TestServiceIconPostgresReconnectPersistence(t *testing.T) {
	ctx := context.Background()
	pg := bootstrap.RequireSharedPostgres(t)
	_, url, cleanup := pg.SetupMigratedDatabase(t)
	defer cleanup()
	config := &ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: url}}
	adapter, err := postgres.NewAdapter(config)
	require.NoError(t, err)
	require.NoError(t, adapter.Initialize(ctx))
	t.Cleanup(func() { _ = adapter.Close(ctx) })
	repo := postgres.NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	entity := createTestService(id.NewServiceID().String(), "Durable Atlas", nil)
	entity.Secret = model.NewEncryptedSecret([]byte("durable credential bytes"))
	source := "<svg xmlns=\"http://www.w3.org/2000/svg\"><text>東京 🗺</text></svg>\r\n"
	entity.IconSVG, entity.IconSVGProvided = &source, true
	require.NoError(t, repo.Create(ctx, entity))
	before, err := repo.Get(ctx, entity.ID)
	require.NoError(t, err)
	require.NoError(t, adapter.Close(ctx))

	reconnected, err := postgres.NewAdapter(config)
	require.NoError(t, err)
	require.NoError(t, reconnected.Initialize(ctx))
	defer func() { require.NoError(t, reconnected.Close(ctx)) }()
	freshRepo := postgres.NewPostgresThirdpartyOAuth2ProviderRepository(reconnected)
	after, err := freshRepo.Get(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	require.NotNil(t, after.IconSVG)
	assert.Equal(t, []byte(source), []byte(*after.IconSVG))
	after.IconSVG, after.IconSVGProvided = nil, true
	require.NoError(t, freshRepo.Update(ctx, after, nil))
	require.NoError(t, reconnected.Close(ctx))
	require.NoError(t, reconnected.Initialize(ctx))
	cleared, err := freshRepo.Get(ctx, entity.ID)
	require.NoError(t, err)
	assert.Nil(t, cleared.IconSVG)
}

func TestServiceIconPostgresInvalidUpdateIsAtomic(t *testing.T) {
	ctx, repo, service, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	entity := createTestService(id.NewServiceID().String(), "Atlas Files", nil)
	source := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>`
	entity.IconSVG, entity.IconSVGProvided = &source, true
	require.NoError(t, service.Create(ctx, entity))
	before, err := repo.Get(ctx, entity.ID)
	require.NoError(t, err)
	candidate := before.Copy()
	invalid := `<svg xmlns="http://www.w3.org/2000/svg"><script>unexpected()</script></svg>`
	candidate.IconSVG, candidate.IconSVGProvided = &invalid, true
	candidate.DisplayName = "Must not persist"
	candidate.ClientID = "must-not-persist"
	candidate.Secret = model.NewPlaintextSecret("must-not-persist")
	require.Error(t, service.Update(ctx, candidate, nil))
	after, err := repo.Get(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}
