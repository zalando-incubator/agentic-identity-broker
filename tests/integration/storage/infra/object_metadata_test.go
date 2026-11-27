//go:build integration

package storage_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/postgres"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type objectMetadataHarness struct {
	ctx      context.Context
	exec     func(string)
	query    func(string) string
	agents   *postgres.AgentRepository
	sets     *postgres.PermissionSetRepository
	services *postgres.PostgresThirdpartyOAuth2ProviderRepository
	service  *model.ThirdpartyOAuth2ProviderEntity
}

func setupObjectMetadataHarness(t *testing.T) *objectMetadataHarness {
	t.Helper()
	ctx, services, providerService, shared, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	t.Cleanup(cleanup)
	adapter, err := postgres.NewAdapter(&ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: shared.ConnectionString(dbName)}}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	require.NoError(t, adapter.Initialize(ctx))
	t.Cleanup(func() { _ = adapter.Close(ctx) })
	service := createTestService(id.NewServiceID().String(), "Metadata Service", []string{"https://api.metadata.example.test/" + id.NewServiceID().String()})
	require.NoError(t, providerService.Create(ctx, service))
	return &objectMetadataHarness{
		ctx:      ctx,
		exec:     func(statement string) { shared.ExecuteSQL(t, dbName, statement) },
		query:    func(statement string) string { return strings.TrimSpace(shared.QuerySQL(t, dbName, statement)) },
		agents:   postgres.NewAgentRepository(adapter),
		sets:     postgres.NewPermissionSetRepository(adapter),
		services: services,
		service:  service,
	}
}

func metadataFixture(t *testing.T) model.ObjectMetadata {
	t.Helper()
	value, err := model.NewObjectMetadata(
		map[string]string{"deployment": "local", "team.example.com/owner": "platform", "Exposure": "public", "exposure": "internal"},
		map[string]string{"gitops.example.com/revision": "4f2c9e1", "note": "Owned by 東京 équipe\n"},
	)
	require.NoError(t, err)
	return value
}

func (h *objectMetadataHarness) permissionSet(t *testing.T, metadata model.ObjectMetadata) *storage.PermissionSet {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	ps := &storage.PermissionSet{
		ID: id.NewPermissionSetID(), Name: "Metadata " + id.NewPermissionSetID().String(), Description: "Read",
		ServiceScopes:  []storage.ServiceScope{{ServiceID: h.service.ID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeMandatory}},
		ObjectMetadata: metadata, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, h.sets.Create(h.ctx, ps))
	return ps
}

func assertMetadataStorageFailure(t *testing.T, entity any, err error) {
	t.Helper()
	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr), "expected storage error, got %v", err)
	assert.Equal(t, storage.ErrorKindUnknown, storageErr.Kind)
	assert.Nil(t, entity)
}

func TestPostgresAgentObjectMetadata(t *testing.T) {
	h := setupObjectMetadataHarness(t)
	ps := h.permissionSet(t, model.ObjectMetadata{})
	metadata := metadataFixture(t)
	newAgent := func(clientID *id.ClientID, clientURI string) *storage.Agent {
		external := id.ExternalID("registry/" + id.NewAgentID().String())
		now := time.Now().UTC().Truncate(time.Second)
		agent := &storage.Agent{
			ID: id.NewAgentID(), ClientID: clientID, ExternalID: &external, DisplayName: "Agent", Description: "Agent",
			PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: ps.ID, RequirementType: storage.RequirementTypeMandatory}},
			ObjectMetadata: metadata, CreatedAt: now, UpdatedAt: now,
		}
		if clientURI != "" {
			agent.ClientURIs = []string{clientURI}
		}
		require.NoError(t, h.agents.Create(h.ctx, agent))
		return agent
	}
	clientID := id.ClientID("metadata-" + id.NewAgentID().String())
	proxy := newAgent(&clientID, "")
	clientURI := "https://agent.metadata.example.test/" + id.NewAgentID().String() + ".json"
	cimd := newAgent(nil, clientURI)

	t.Run("every read returns the stored metadata", func(t *testing.T) {
		reads := map[string]func() (*storage.Agent, error){
			"Get":             func() (*storage.Agent, error) { return h.agents.Get(h.ctx, proxy.ID) },
			"GetByExternalID": func() (*storage.Agent, error) { return h.agents.GetByExternalID(h.ctx, *proxy.ExternalID) },
			"GetByClientID":   func() (*storage.Agent, error) { return h.agents.GetByClientID(h.ctx, clientID) },
			"GetByClientURI":  func() (*storage.Agent, error) { return h.agents.GetByClientURI(h.ctx, clientURI) },
			"GetByIDs": func() (*storage.Agent, error) {
				agents, err := h.agents.GetByIDs(h.ctx, []id.AgentID{proxy.ID})
				if err != nil || len(agents) != 1 {
					return nil, err
				}
				return agents[0], nil
			},
			"List": func() (*storage.Agent, error) {
				agents, err := h.agents.List(h.ctx)
				for _, agent := range agents {
					if agent.ID == cimd.ID {
						return agent, err
					}
				}
				return nil, err
			},
		}
		for name, read := range reads {
			t.Run(name, func(t *testing.T) {
				agent, err := read()
				require.NoError(t, err)
				require.NotNil(t, agent)
				assert.Equal(t, metadata.Labels(), agent.ObjectMetadata.Labels())
				assert.Equal(t, metadata.Annotations(), agent.ObjectMetadata.Annotations())
			})
		}
	})
	t.Run("update replaces and empty metadata is stored as an empty object", func(t *testing.T) {
		updated := proxy.Copy()
		updated.ObjectMetadata = model.ObjectMetadata{}
		updated.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		require.NoError(t, h.agents.Update(h.ctx, updated))
		stored, err := h.agents.Get(h.ctx, proxy.ID)
		require.NoError(t, err)
		assert.True(t, stored.ObjectMetadata.IsEmpty())
		assert.Equal(t, "{}|{}", h.query(`SELECT labels::text || '|' || annotations::text FROM agents WHERE id = '`+proxy.ID.String()+`'`))
	})
	t.Run("stored keys outside the write rules read back unchanged", func(t *testing.T) {
		h.exec(`UPDATE agents SET labels = '{"agenticidentitybroker.dev/x":"v","bad key":"v"}', annotations = '{"a.agenticidentitybroker.dev/y":""}' WHERE id IN ('` + proxy.ID.String() + `','` + cimd.ID.String() + `')`)
		byClientID, err := h.agents.GetByClientID(h.ctx, clientID)
		require.NoError(t, err)
		byClientURI, err := h.agents.GetByClientURI(h.ctx, clientURI)
		require.NoError(t, err)
		for _, agent := range []*storage.Agent{byClientID, byClientURI} {
			assert.Equal(t, map[string]string{"agenticidentitybroker.dev/x": "v", "bad key": "v"}, agent.ObjectMetadata.Labels())
			assert.Equal(t, map[string]string{"a.agenticidentitybroker.dev/y": ""}, agent.ObjectMetadata.Annotations())
		}
	})
	t.Run("a column that is not an object of strings fails closed", func(t *testing.T) {
		h.exec(`UPDATE agents SET annotations = '{"k":1}' WHERE id IN ('` + proxy.ID.String() + `','` + cimd.ID.String() + `')`)
		agent, err := h.agents.Get(h.ctx, proxy.ID)
		assertMetadataStorageFailure(t, agent, err)
		agent, err = h.agents.GetByClientID(h.ctx, clientID)
		assertMetadataStorageFailure(t, agent, err)
		agent, err = h.agents.GetByClientURI(h.ctx, clientURI)
		assertMetadataStorageFailure(t, agent, err)
		agent, err = h.agents.GetByExternalID(h.ctx, *proxy.ExternalID)
		assertMetadataStorageFailure(t, agent, err)
		agents, err := h.agents.List(h.ctx)
		assert.Error(t, err)
		assert.Nil(t, agents)
	})
}

func TestPostgresPermissionSetObjectMetadata(t *testing.T) {
	h := setupObjectMetadataHarness(t)
	metadata := metadataFixture(t)
	ps := h.permissionSet(t, metadata)
	empty := h.permissionSet(t, model.ObjectMetadata{})

	t.Run("every read returns the stored metadata", func(t *testing.T) {
		get, err := h.sets.Get(h.ctx, ps.ID)
		require.NoError(t, err)
		batch, err := h.sets.GetByIDs(h.ctx, []id.PermissionSetID{ps.ID})
		require.NoError(t, err)
		require.Len(t, batch, 1)
		all, err := h.sets.List(h.ctx, id.ServiceID{})
		require.NoError(t, err)
		byService, err := h.sets.List(h.ctx, h.service.ID)
		require.NoError(t, err)
		found := []*storage.PermissionSet{get, batch[0]}
		for _, list := range [][]*storage.PermissionSet{all, byService} {
			for _, candidate := range list {
				if candidate.ID == ps.ID {
					found = append(found, candidate)
				}
			}
		}
		require.Len(t, found, 4)
		for _, candidate := range found {
			assert.True(t, metadata.Equal(candidate.ObjectMetadata))
		}
		assert.Equal(t, "{}|{}", h.query(`SELECT labels::text || '|' || annotations::text FROM permission_sets WHERE id = '`+empty.ID.String()+`'`))
	})
	t.Run("update replaces metadata in full", func(t *testing.T) {
		replacement, err := model.NewObjectMetadata(map[string]string{"tier": "gold"}, nil)
		require.NoError(t, err)
		updated := ps.Copy()
		updated.ObjectMetadata = replacement
		updated.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		require.NoError(t, h.sets.Update(h.ctx, updated))
		stored, err := h.sets.Get(h.ctx, ps.ID)
		require.NoError(t, err)
		assert.True(t, replacement.Equal(stored.ObjectMetadata))
		assert.Equal(t, `{"tier": "gold"}|{}`, h.query(`SELECT labels::text || '|' || annotations::text FROM permission_sets WHERE id = '`+ps.ID.String()+`'`))
	})
	t.Run("a stored reserved-prefix key reads back unchanged", func(t *testing.T) {
		h.exec(`UPDATE permission_sets SET labels = '{"agenticidentitybroker.dev/x":"v"}' WHERE id = '` + ps.ID.String() + `'`)
		stored, err := h.sets.Get(h.ctx, ps.ID)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"agenticidentitybroker.dev/x": "v"}, stored.ObjectMetadata.Labels())
	})
	t.Run("a column that is not an object of strings fails closed", func(t *testing.T) {
		h.exec(`UPDATE permission_sets SET labels = '{"k":["v"]}' WHERE id = '` + ps.ID.String() + `'`)
		stored, err := h.sets.Get(h.ctx, ps.ID)
		assertMetadataStorageFailure(t, stored, err)
		batch, err := h.sets.GetByIDs(h.ctx, []id.PermissionSetID{ps.ID})
		assert.Error(t, err)
		assert.Nil(t, batch)
		listed, err := h.sets.List(h.ctx, id.ServiceID{})
		assert.Error(t, err)
		assert.Nil(t, listed)
	})
}

func TestPostgresServiceObjectMetadata(t *testing.T) {
	h := setupObjectMetadataHarness(t)
	metadata := metadataFixture(t)
	canonicalID := "metadata-" + strings.ReplaceAll(id.NewServiceID().String(), "-", "")[:12]
	stored, err := h.services.Get(h.ctx, h.service.ID)
	require.NoError(t, err)
	assert.True(t, stored.ObjectMetadata.IsEmpty())
	assert.Equal(t, "{}|{}", h.query(`SELECT labels::text || '|' || annotations::text FROM thirdparty_oauth2_services WHERE id = '`+h.service.ID.String()+`'`))
	stored.ObjectMetadata, stored.ObjectMetadataProvided = metadata, true
	stored.CanonicalID = &canonicalID
	require.NoError(t, h.services.Update(h.ctx, stored, &stored.Version))

	t.Run("every read returns the stored metadata", func(t *testing.T) {
		reads := map[string]func() (*model.ThirdpartyOAuth2ProviderEntity, error){
			"Get": func() (*model.ThirdpartyOAuth2ProviderEntity, error) { return h.services.Get(h.ctx, h.service.ID) },
			"GetByCanonicalID": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return h.services.GetByCanonicalID(h.ctx, canonicalID)
			},
			"FindByProtectedResource": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return h.services.FindByProtectedResource(h.ctx, h.service.ProtectedResources[0])
			},
			"GetByIDs": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
				batch, err := h.services.GetByIDs(h.ctx, []id.ServiceID{h.service.ID})
				if err != nil || len(batch) != 1 {
					return nil, err
				}
				return batch[0], nil
			},
			"List": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
				listed, err := h.services.List(h.ctx)
				if err != nil || len(listed) != 1 {
					return nil, err
				}
				return listed[0], nil
			},
		}
		for name, read := range reads {
			t.Run(name, func(t *testing.T) {
				entity, err := read()
				require.NoError(t, err)
				require.NotNil(t, entity)
				assert.True(t, metadata.Equal(entity.ObjectMetadata))
				assert.False(t, entity.ObjectMetadataProvided)
			})
		}
	})
	t.Run("an accepted change advances the row version", func(t *testing.T) {
		current, err := h.services.Get(h.ctx, h.service.ID)
		require.NoError(t, err)
		before := current.Version
		current.ObjectMetadata = model.ObjectMetadata{}
		require.NoError(t, h.services.Update(h.ctx, current, &before))
		after, err := h.services.Get(h.ctx, h.service.ID)
		require.NoError(t, err)
		assert.Equal(t, before+1, after.Version)
		assert.True(t, after.ObjectMetadata.IsEmpty())
		assert.Equal(t, "{}|{}", h.query(`SELECT labels::text || '|' || annotations::text FROM thirdparty_oauth2_services WHERE id = '`+h.service.ID.String()+`'`))
	})
	t.Run("a stored reserved-prefix key reads back unchanged", func(t *testing.T) {
		h.exec(`UPDATE thirdparty_oauth2_services SET annotations = '{"agenticidentitybroker.dev/x":"v"}' WHERE id = '` + h.service.ID.String() + `'`)
		entity, err := h.services.Get(h.ctx, h.service.ID)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"agenticidentitybroker.dev/x": "v"}, entity.ObjectMetadata.Annotations())
	})
	t.Run("a column that is not an object of strings fails closed", func(t *testing.T) {
		h.exec(`UPDATE thirdparty_oauth2_services SET labels = '{"k":null}' WHERE id = '` + h.service.ID.String() + `'`)
		entity, err := h.services.Get(h.ctx, h.service.ID)
		assert.Error(t, err)
		assert.Nil(t, entity)
		listed, err := h.services.List(h.ctx)
		if err == nil {
			for _, candidate := range listed {
				assert.NotEqual(t, h.service.ID, candidate.ID, "a corrupt service must not be returned")
			}
		}
	})
}
