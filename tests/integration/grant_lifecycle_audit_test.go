package integration

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

type grantAuditWrites struct {
	ports.UserGrantRepository
	mu        sync.Mutex
	committed map[string]*storage.UserGrant
}

func (r *grantAuditWrites) Create(ctx context.Context, grant *storage.UserGrant) error {
	if err := r.UserGrantRepository.Create(ctx, grant); err != nil {
		return err
	}
	r.mu.Lock()
	r.committed["grant_created"] = grant.Copy()
	r.mu.Unlock()
	return nil
}

func (r *grantAuditWrites) Update(ctx context.Context, grant *storage.UserGrant) error {
	if err := r.UserGrantRepository.Update(ctx, grant); err != nil {
		return err
	}
	r.mu.Lock()
	r.committed["grant_updated"] = grant.Copy()
	r.mu.Unlock()
	return nil
}

func TestGrantLifecycleAudit_ConcurrentInitialConsent(t *testing.T) {
	for _, identical := range []bool{false, true} {
		name := "distinct submissions"
		if identical {
			name = "identical submissions mutate once"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			logger, logs := bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
			transactions := memory.NewTransactionManager()
			agents := memory.NewAgentRepository(transactions)
			permissionSets := memory.NewPermissionSetRepository(transactions)
			sessions := memory.NewInMemoryUserSessionRepository(transactions)
			grants := memory.NewUserGrantRepository(transactions)
			registry, err := ledger.NewRegistry(eventschemas.Schemas)
			require.NoError(t, err)
			events := memory.NewBusinessEventRepository(transactions, registry)
			recorder := ledger.NewService(registry, events, events, transactions, false)
			writes := &grantAuditWrites{UserGrantRepository: grants, committed: make(map[string]*storage.UserGrant)}
			principal := id.Principal("grant-race-owner")
			agentID, psID, serviceID := id.NewAgentID(), id.NewPermissionSetID(), id.NewServiceID()
			require.NoError(t, agents.Create(ctx, &storage.Agent{
				ID: agentID, DisplayName: "PRIVATE_AGENT_PROFILE_SENTINEL", Description: "PRIVATE_AGENT_PROFILE_SENTINEL",
				PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: psID, RequirementType: storage.RequirementTypeMandatory}},
			}))
			require.NoError(t, permissionSets.Create(ctx, &storage.PermissionSet{
				ID: psID, Name: "race permission set", Description: "PRIVATE_PS_DESCRIPTION_SENTINEL",
				ServiceScopes: []storage.ServiceScope{{ServiceID: serviceID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}},
			}))
			require.NoError(t, sessions.Create(ctx, fixtures.SessionForServiceWithScopes(principal.String(), serviceID.String(), []string{"read"})))
			psService := permissionset.NewPermissionSetService(permissionSets, grants, logger)
			t.Cleanup(psService.Close)
			svc := consent.NewService(agents, nil, writes, sessions, memory.NewRefreshTokenSessionStore(transactions), psService, logger, recorder)
			type mutationResult struct {
				grant *storage.UserGrant
				err   error
			}
			results := make(chan mutationResult, 2)
			validUntil := time.Now().Add(time.Hour)
			start := make(chan struct{})
			for i := range 2 {
				validity := validUntil
				if !identical {
					validity = validity.Add(time.Duration(i) * time.Hour)
				}
				request := &consent.GrantRequest{
					Principal: principal, AgentID: agentID, ValidUntil: &validity,
					GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{serviceID}}},
				}
				go func() {
					<-start
					grant, err := svc.GrantConsent(ctx, request)
					results <- mutationResult{grant: grant, err: err}
				}()
			}
			close(start)
			var committed []*storage.UserGrant
			for range 2 {
				select {
				case result := <-results:
					require.NoError(t, result.err)
					require.NotNil(t, result.grant)
					committed = append(committed, result.grant)
				case <-ctx.Done():
					t.Fatal("production atomic grant writes did not complete")
				}
			}
			assert.Equal(t, committed[0].ID, committed[1].ID)
			assert.Equal(t, committed[0].CreatedAt, committed[1].CreatedAt)
			stored, err := grants.FindByPrincipalAndAgent(ctx, principal, agentID)
			require.NoError(t, err)
			assert.Equal(t, committed[0].ID, stored.ID)
			assert.Equal(t, committed[0].CreatedAt, stored.CreatedAt)
			all, err := grants.ListByPrincipalAndAgent(ctx, principal, agentID)
			require.NoError(t, err)
			require.Len(t, all, 1)
			records, err := logs.Records()
			require.NoError(t, err)
			wantWrites := 2
			wantActions := map[string]int{"grant_created": 1, "grant_updated": 1}
			if identical {
				wantWrites = 1
				wantActions = map[string]int{"grant_created": 1}
			}
			require.Len(t, records, wantWrites)
			require.Len(t, writes.committed, wantWrites)
			expectedByAction := writes.committed
			actions := map[string]int{}
			for _, record := range records {
				action, ok := record["action"].(string)
				require.True(t, ok)
				actions[action]++
				assert.Equal(t, principal.String(), record["principal"])
				assert.Equal(t, agentID.String(), record["agent_id"])
				assert.Equal(t, stored.ID.String(), record["grant_id"])
				assert.Equal(t, stored.CreatedAt.UTC().Format(time.RFC3339Nano), record["created_at"])
				snapshot, exists := expectedByAction[action]
				require.True(t, exists, "each action must identify its own actual committed write")
				assert.Equal(t, snapshot.UpdatedAt.UTC().Format(time.RFC3339Nano), record["updated_at"])
				assert.Equal(t, snapshot.ValidUntil.UTC().Format(time.RFC3339Nano), record["valid_until"])
				delete(expectedByAction, action)
				assert.Equal(t, []any{map[string]any{"permission_set_id": psID.String(), "included_service_ids": []any{serviceID.String()}}}, record["granted_permission_sets"])
				if action == "grant_updated" {
					assert.Contains(t, record, "previous_observed_valid_until")
					assert.Contains(t, record, "previous_observed_updated_at")
					assert.Contains(t, record, "previous_observed_granted_permission_sets")
				} else {
					assert.NotContains(t, record, "previous_observed_valid_until")
					assert.NotContains(t, record, "previous_observed_updated_at")
					assert.NotContains(t, record, "previous_observed_granted_permission_sets")
				}
				assert.NotContains(t, record, "actor")
				assert.NotContains(t, record, "trace_id")
			}
			assert.Empty(t, expectedByAction, "each successful write must be observed exactly once")
			assert.Equal(t, wantActions, actions)
			assert.NotContains(t, logs.Raw(), "PRIVATE_AGENT_PROFILE_SENTINEL")
			assert.NotContains(t, logs.Raw(), "PRIVATE_PS_DESCRIPTION_SENTINEL")

			// Once racing writes finish, the same persisted submission is a genuine no-op.
			_, err = svc.GrantConsent(ctx, &consent.GrantRequest{
				Principal: principal, AgentID: agentID, ValidUntil: stored.ValidUntil,
				GrantedPermissionSets: stored.GrantedPermissionSets,
			})
			require.NoError(t, err)
			records, err = logs.Records()
			require.NoError(t, err)
			assert.Len(t, records, wantWrites)
		})
	}
}
