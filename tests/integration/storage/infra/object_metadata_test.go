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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type objectMetadataHarness struct {
	ctx       context.Context
	adapter   *postgres.Adapter
	exec      func(string)
	query     func(string) string
	agents    *postgres.AgentRepository
	sets      *postgres.PermissionSetRepository
	services  *postgres.PostgresThirdpartyOAuth2ProviderRepository
	providers *thirdparty.ThirdpartyOAuth2ProviderService
	service   *model.ThirdpartyOAuth2ProviderEntity
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
		ctx:       ctx,
		adapter:   adapter,
		exec:      func(statement string) { shared.ExecuteSQL(t, dbName, statement) },
		query:     func(statement string) string { return strings.TrimSpace(shared.QuerySQL(t, dbName, statement)) },
		agents:    postgres.NewAgentRepository(adapter),
		sets:      postgres.NewPermissionSetRepository(adapter),
		services:  services,
		providers: providerService,
		service:   service,
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

// restoreMetadataColumns resets a row's metadata after a subtest writes
// columns that every list read of the table would otherwise fail on.
func (h *objectMetadataHarness) restoreMetadataColumns(t *testing.T, table, rowID string) {
	t.Helper()
	t.Cleanup(func() {
		h.exec(`UPDATE ` + table + ` SET labels = '{}', annotations = '{}' WHERE id = '` + rowID + `'`)
	})
}

func (h *objectMetadataHarness) agent(t *testing.T, setID id.PermissionSetID, metadata model.ObjectMetadata, clientID *id.ClientID, clientURI string) *storage.Agent {
	t.Helper()
	external := id.ExternalID("registry/" + id.NewAgentID().String())
	now := time.Now().UTC().Truncate(time.Second)
	agent := &storage.Agent{
		ID: id.NewAgentID(), ClientID: clientID, ExternalID: &external, DisplayName: "Agent", Description: "Agent",
		PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: setID, RequirementType: storage.RequirementTypeMandatory}},
		ObjectMetadata: metadata, CreatedAt: now, UpdatedAt: now,
	}
	if clientURI != "" {
		agent.ClientURIs = []string{clientURI}
	}
	require.NoError(t, h.agents.Create(h.ctx, agent))
	return agent
}

// clientAgents creates one Agent found by client ID and one found by client URI.
func (h *objectMetadataHarness) clientAgents(t *testing.T, setID id.PermissionSetID, metadata model.ObjectMetadata) (*storage.Agent, id.ClientID, *storage.Agent, string) {
	t.Helper()
	clientID := id.ClientID("metadata-" + id.NewAgentID().String())
	clientURI := "https://agent.metadata.example.test/" + id.NewAgentID().String() + ".json"
	return h.agent(t, setID, metadata, &clientID, ""), clientID, h.agent(t, setID, metadata, nil, clientURI), clientURI
}

func (h *objectMetadataHarness) metadataColumns(table, rowID string) string {
	return h.query(`SELECT labels::text || '|' || annotations::text FROM ` + table + ` WHERE id = '` + rowID + `'`)
}

func TestPostgresAgentObjectMetadata(t *testing.T) {
	h := setupObjectMetadataHarness(t)
	ps := h.permissionSet(t, model.ObjectMetadata{})
	metadata := metadataFixture(t)

	t.Run("every read returns the stored metadata", func(t *testing.T) {
		proxy, clientID, cimd, clientURI := h.clientAgents(t, ps.ID, metadata)
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
		agent := h.agent(t, ps.ID, metadata, nil, "")
		updated := agent.Copy()
		updated.ObjectMetadata = model.ObjectMetadata{}
		updated.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		require.NoError(t, h.agents.Update(h.ctx, updated))
		stored, err := h.agents.Get(h.ctx, agent.ID)
		require.NoError(t, err)
		assert.True(t, stored.ObjectMetadata.IsEmpty())
		assert.Equal(t, "{}|{}", h.metadataColumns("agents", agent.ID.String()))
	})
	t.Run("stored keys outside the write rules read back unchanged", func(t *testing.T) {
		proxy, clientID, cimd, clientURI := h.clientAgents(t, ps.ID, metadata)
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
		proxy, clientID, cimd, clientURI := h.clientAgents(t, ps.ID, metadata)
		h.restoreMetadataColumns(t, "agents", proxy.ID.String())
		h.restoreMetadataColumns(t, "agents", cimd.ID.String())
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

	t.Run("every read returns the stored metadata", func(t *testing.T) {
		ps := h.permissionSet(t, metadata)
		empty := h.permissionSet(t, model.ObjectMetadata{})
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
		assert.Equal(t, "{}|{}", h.metadataColumns("permission_sets", empty.ID.String()))
	})
	t.Run("update replaces metadata in full", func(t *testing.T) {
		ps := h.permissionSet(t, metadata)
		replacement, err := model.NewObjectMetadata(map[string]string{"tier": "gold"}, nil)
		require.NoError(t, err)
		updated := ps.Copy()
		updated.ObjectMetadata = replacement
		updated.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		require.NoError(t, h.sets.Update(h.ctx, updated))
		stored, err := h.sets.Get(h.ctx, ps.ID)
		require.NoError(t, err)
		assert.True(t, replacement.Equal(stored.ObjectMetadata))
		assert.Equal(t, `{"tier": "gold"}|{}`, h.metadataColumns("permission_sets", ps.ID.String()))
	})
	t.Run("a stored reserved-prefix key reads back unchanged", func(t *testing.T) {
		ps := h.permissionSet(t, metadata)
		h.exec(`UPDATE permission_sets SET labels = '{"agenticidentitybroker.dev/x":"v"}' WHERE id = '` + ps.ID.String() + `'`)
		stored, err := h.sets.Get(h.ctx, ps.ID)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"agenticidentitybroker.dev/x": "v"}, stored.ObjectMetadata.Labels())
	})
	t.Run("a column that is not an object of strings fails closed", func(t *testing.T) {
		ps := h.permissionSet(t, metadata)
		h.restoreMetadataColumns(t, "permission_sets", ps.ID.String())
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

// annotatedService creates a Service whose stored metadata is the fixture and
// whose canonical ID and protected resource are unique to the subtest.
func (h *objectMetadataHarness) annotatedService(t *testing.T, metadata model.ObjectMetadata) *model.ThirdpartyOAuth2ProviderEntity {
	t.Helper()
	service := createTestService(id.NewServiceID().String(), "Metadata Service", []string{"https://api.metadata.example.test/" + id.NewServiceID().String()})
	require.NoError(t, h.providers.Create(h.ctx, service))
	stored, err := h.services.Get(h.ctx, service.ID)
	require.NoError(t, err)
	assert.True(t, stored.ObjectMetadata.IsEmpty())
	assert.Equal(t, "{}|{}", h.metadataColumns("thirdparty_oauth2_services", service.ID.String()))
	canonicalID := "metadata-" + strings.ReplaceAll(id.NewServiceID().String(), "-", "")[:12]
	stored.ObjectMetadata, stored.ObjectMetadataProvided = metadata, true
	stored.CanonicalID = &canonicalID
	require.NoError(t, h.services.Update(h.ctx, stored, &stored.Version))
	updated, err := h.services.Get(h.ctx, service.ID)
	require.NoError(t, err)
	updated.ProtectedResources = service.ProtectedResources
	return updated
}

func TestPostgresServiceObjectMetadata(t *testing.T) {
	h := setupObjectMetadataHarness(t)
	metadata := metadataFixture(t)

	t.Run("every read returns the stored metadata", func(t *testing.T) {
		service := h.annotatedService(t, metadata)
		reads := map[string]func() (*model.ThirdpartyOAuth2ProviderEntity, error){
			"Get": func() (*model.ThirdpartyOAuth2ProviderEntity, error) { return h.services.Get(h.ctx, service.ID) },
			"GetByCanonicalID": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return h.services.GetByCanonicalID(h.ctx, *service.CanonicalID)
			},
			"FindByProtectedResource": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return h.services.FindByProtectedResource(h.ctx, service.ProtectedResources[0])
			},
			"GetByIDs": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
				batch, err := h.services.GetByIDs(h.ctx, []id.ServiceID{service.ID})
				if err != nil || len(batch) != 1 {
					return nil, err
				}
				return batch[0], nil
			},
			"List": func() (*model.ThirdpartyOAuth2ProviderEntity, error) {
				listed, err := h.services.List(h.ctx)
				for _, candidate := range listed {
					if candidate.ID == service.ID {
						return candidate, err
					}
				}
				return nil, err
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
		current := h.annotatedService(t, metadata)
		before := current.Version
		current.ObjectMetadata = model.ObjectMetadata{}
		require.NoError(t, h.services.Update(h.ctx, current, &before))
		after, err := h.services.Get(h.ctx, current.ID)
		require.NoError(t, err)
		assert.Equal(t, before+1, after.Version)
		assert.True(t, after.ObjectMetadata.IsEmpty())
		assert.Equal(t, "{}|{}", h.metadataColumns("thirdparty_oauth2_services", current.ID.String()))
	})
	t.Run("a stored reserved-prefix key reads back unchanged", func(t *testing.T) {
		service := h.annotatedService(t, metadata)
		h.exec(`UPDATE thirdparty_oauth2_services SET annotations = '{"agenticidentitybroker.dev/x":"v"}' WHERE id = '` + service.ID.String() + `'`)
		entity, err := h.services.Get(h.ctx, service.ID)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"agenticidentitybroker.dev/x": "v"}, entity.ObjectMetadata.Annotations())
	})
	t.Run("a column that is not an object of strings fails closed", func(t *testing.T) {
		service := h.annotatedService(t, metadata)
		h.restoreMetadataColumns(t, "thirdparty_oauth2_services", service.ID.String())
		h.exec(`UPDATE thirdparty_oauth2_services SET labels = '{"k":null}' WHERE id = '` + service.ID.String() + `'`)
		entity, err := h.services.Get(h.ctx, service.ID)
		assert.Error(t, err)
		assert.Nil(t, entity)
		listed, err := h.services.List(h.ctx)
		if err == nil {
			for _, candidate := range listed {
				assert.NotEqual(t, service.ID, candidate.ID, "a corrupt service must not be returned")
			}
		}
	})
}

func TestPostgresServiceAccountObjectMetadata(t *testing.T) {
	h := setupObjectMetadataHarness(t)
	repo := postgres.NewServiceAccountRepository(h.adapter)
	account := func(t *testing.T, metadata model.ObjectMetadata) *model.ServiceAccount {
		t.Helper()
		now := time.Now().UTC().Truncate(time.Microsecond)
		created := &model.ServiceAccount{
			ID: id.NewServiceAccountID(), ServiceID: h.service.ID, CanonicalID: "bot-" + id.NewServiceAccountID().String(),
			Kind: model.ServiceAccountKindJWTBearer, Scopes: []string{"read"}, Credential: model.NewEncryptedSecret([]byte("opaque")),
			Summary:        model.CredentialSummary{Format: model.CredentialFormatSignedJWT, Use: model.CredentialUseAccessToken, Fingerprint: "SHA256:x"},
			ObjectMetadata: metadata, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.Create(h.ctx, created))
		return created
	}

	t.Run("every read returns the stored metadata byte for byte", func(t *testing.T) {
		metadata := metadataFixture(t)
		created := account(t, metadata)
		byID, err := repo.Get(h.ctx, h.service.ID, created.ID)
		require.NoError(t, err)
		assert.True(t, metadata.Equal(byID.ObjectMetadata))
		byName, err := repo.GetByCanonicalID(h.ctx, h.service.ID, created.CanonicalID)
		require.NoError(t, err)
		assert.True(t, metadata.Equal(byName.ObjectMetadata))
		listed, err := repo.ListByService(h.ctx, h.service.ID)
		require.NoError(t, err)
		for _, candidate := range listed {
			if candidate.ID == created.ID {
				assert.True(t, metadata.Equal(candidate.ObjectMetadata))
			}
		}
	})
	t.Run("empty metadata is written as empty objects", func(t *testing.T) {
		created := account(t, model.ObjectMetadata{})
		assert.Equal(t, "{}|{}", h.metadataColumns("service_accounts", created.ID.String()))
		created.ObjectMetadata = metadataFixture(t)
		require.NoError(t, repo.Update(h.ctx, created, nil))
		created.ObjectMetadata = model.ObjectMetadata{}
		require.NoError(t, repo.Update(h.ctx, created, nil))
		assert.Equal(t, "{}|{}", h.metadataColumns("service_accounts", created.ID.String()))
	})
	t.Run("a stored reserved-prefix key reads back unchanged", func(t *testing.T) {
		created := account(t, model.ObjectMetadata{})
		h.exec(`UPDATE service_accounts SET labels = '{"agenticidentitybroker.dev/x":"v"}' WHERE id = '` + created.ID.String() + `'`)
		stored, err := repo.Get(h.ctx, h.service.ID, created.ID)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"agenticidentitybroker.dev/x": "v"}, stored.ObjectMetadata.Labels())
	})
	t.Run("a column that is not an object of strings fails closed", func(t *testing.T) {
		created := account(t, model.ObjectMetadata{})
		h.restoreMetadataColumns(t, "service_accounts", created.ID.String())
		h.exec(`UPDATE service_accounts SET annotations = '{"k":1}' WHERE id = '` + created.ID.String() + `'`)
		stored, err := repo.Get(h.ctx, h.service.ID, created.ID)
		assertMetadataStorageFailure(t, stored, err)
		listed, err := repo.ListByService(h.ctx, h.service.ID)
		assertMetadataStorageFailure(t, listed, err)
	})
}
