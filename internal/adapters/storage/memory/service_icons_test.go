package memory

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceIconMemoryRoundTripAndIndependentCopies(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	const source = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n<svg xmlns=\"http://www.w3.org/2000/svg\"><text>Ä &amp; 東京 🗺</text></svg>\n"
	icon := source
	canonicalID := "atlas-files"
	first := testProvider(id.NewServiceID(), "https://api.example.test/atlas")
	first.CanonicalID = &canonicalID
	first.IconSVG, first.IconSVGProvided = &icon, true
	first.CreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	first.UpdatedAt = first.CreatedAt
	second := testProvider(id.NewServiceID())
	second.IconSVG, second.IconSVGProvided = &icon, true
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, second))
	icon = "caller mutation"

	reads := map[string]func() (*model.ThirdpartyOAuth2ProviderEntity, error){
		"get":       func() (*model.ThirdpartyOAuth2ProviderEntity, error) { return repo.Get(ctx, first.ID) },
		"canonical": func() (*model.ThirdpartyOAuth2ProviderEntity, error) { return repo.GetByCanonicalID(ctx, canonicalID) },
		"resource": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
			return repo.FindByProtectedResource(ctx, first.ProtectedResources[0])
		},
		"batch": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
			entities, err := repo.GetByIDs(ctx, []id.ServiceID{first.ID, first.ID, id.NewServiceID()})
			require.NoError(t, err)
			require.Len(t, entities, 1)
			return entities[0], nil
		},
		"list": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
			entities, err := repo.List(ctx)
			require.NoError(t, err)
			for _, entity := range entities {
				if entity.ID == first.ID {
					return entity, nil
				}
			}
			t.Fatal("created provider missing from list")
			return nil, nil
		},
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			stored, err := read()
			require.NoError(t, err)
			require.NotNil(t, stored.IconSVG)
			assert.Equal(t, []byte(source), []byte(*stored.IconSVG))
			assert.False(t, stored.IconSVGProvided)
			assert.Equal(t, first.CreatedAt, stored.CreatedAt)
			assert.Equal(t, first.ProtectedResources, stored.ProtectedResources)
			*stored.IconSVG = "read mutation"
			again, err := repo.Get(ctx, first.ID)
			require.NoError(t, err)
			require.NotNil(t, again.IconSVG)
			assert.Equal(t, source, *again.IconSVG)
		})
	}

	stored, err := repo.Get(ctx, first.ID)
	require.NoError(t, err)
	replacement := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>`
	stored.IconSVG, stored.IconSVGProvided = &replacement, true
	version := stored.Version
	require.NoError(t, repo.Update(ctx, stored, &version))
	replacement = "write mutation"
	updated, err := repo.Get(ctx, first.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.IconSVG)
	assert.Contains(t, *updated.IconSVG, `<circle r="4"/>`)
	assert.Equal(t, version+1, updated.Version)
	assert.False(t, updated.IconSVGProvided)
	assert.Equal(t, first.CreatedAt, updated.CreatedAt)
	assert.Equal(t, first.ClientID, updated.ClientID)
	beforeClear := updated.Copy()
	updated.IconSVG, updated.IconSVGProvided = nil, true
	require.Error(t, repo.Update(ctx, updated, &version), "stale CAS must not clear the source")
	unchanged, err := repo.Get(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, beforeClear, unchanged)
	require.NoError(t, repo.Update(ctx, updated, nil))
	cleared, err := repo.Get(ctx, first.ID)
	require.NoError(t, err)
	assert.Nil(t, cleared.IconSVG)
	assert.False(t, cleared.IconSVGProvided)
	other, err := repo.Get(ctx, second.ID)
	require.NoError(t, err)
	require.NotNil(t, other.IconSVG)
	assert.Equal(t, source, *other.IconSVG)
}

func TestServiceIconMemoryAbsentRecord(t *testing.T) {
	entity := testProvider(id.NewServiceID())
	entity.IconSVGProvided = true
	record, err := providerEntityToRecord(entity)
	require.NoError(t, err)
	assert.Nil(t, record.entity.IconSVG)
	assert.False(t, record.entity.IconSVGProvided)
	assert.Nil(t, providerRecordToEntity(record, nil).IconSVG)
}
