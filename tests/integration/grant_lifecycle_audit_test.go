package integration

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

// Only the read barrier is artificial; both writes run the production atomic upsert.
type absentGrantLookupBarrier struct {
	ports.UserGrantRepository
	arrived    chan struct{}
	release    chan struct{}
	mu         sync.Mutex
	candidates []id.GrantID
	committed  map[id.GrantID]*storage.UserGrant
}

func (r *absentGrantLookupBarrier) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	grant, err := r.UserGrantRepository.FindByPrincipalAndAgent(ctx, principal, agentID)
	if grant != nil || !errors.Is(err, ports.ErrNotFound) {
		return grant, err
	}
	r.arrived <- struct{}{}
	select {
	case <-r.release:
		return grant, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *absentGrantLookupBarrier) Create(ctx context.Context, grant *storage.UserGrant) error {
	candidate := grant.ID
	r.mu.Lock()
	r.candidates = append(r.candidates, candidate)
	r.mu.Unlock()
	if err := r.UserGrantRepository.Create(ctx, grant); err != nil {
		return err
	}
	r.mu.Lock()
	if r.committed == nil {
		r.committed = make(map[id.GrantID]*storage.UserGrant)
	}
	r.committed[candidate] = grant.Copy()
	r.mu.Unlock()
	return nil
}

func TestGrantLifecycleAudit_ConcurrentInitialConsent(t *testing.T) {
	for _, identical := range []bool{false, true} {
		name := "distinct submissions"
		if identical {
			name = "identical submissions still perform two writes"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			logger, logs := bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
			agents := memory.NewAgentRepository()
			permissionSets := memory.NewPermissionSetRepository()
			sessions := memory.NewInMemoryUserSessionRepository()
			grants := memory.NewUserGrantRepository()
			barrier := &absentGrantLookupBarrier{
				UserGrantRepository: grants,
				arrived:             make(chan struct{}, 2),
				release:             make(chan struct{}),
			}
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
			svc := consent.NewService(agents, nil, barrier, sessions, psService, logger)
			type mutationResult struct {
				grant *storage.UserGrant
				err   error
			}
			results := make(chan mutationResult, 2)
			validUntil := time.Now().Add(time.Hour)
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
					grant, err := svc.GrantConsent(ctx, request)
					results <- mutationResult{grant: grant, err: err}
				}()
			}
			for range 2 {
				select {
				case <-barrier.arrived:
				case <-ctx.Done():
					t.Fatal("both initial consent reads must observe absence before either write")
				}
			}
			close(barrier.release)
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
			require.Len(t, barrier.candidates, 2)
			assert.NotEqual(t, barrier.candidates[0], barrier.candidates[1])
			winningCandidates := 0
			for _, candidate := range barrier.candidates {
				if candidate == committed[0].ID {
					winningCandidates++
				}
			}
			assert.Equal(t, 1, winningCandidates)
			stored, err := grants.FindByPrincipalAndAgent(ctx, principal, agentID)
			require.NoError(t, err)
			assert.Equal(t, committed[0].ID, stored.ID)
			assert.Equal(t, committed[0].CreatedAt, stored.CreatedAt)
			all, err := grants.ListByPrincipalAndAgent(ctx, principal, agentID)
			require.NoError(t, err)
			require.Len(t, all, 1)
			records, err := logs.Records()
			require.NoError(t, err)
			require.Len(t, records, 2)
			require.Len(t, barrier.committed, 2)
			expectedByAction := make(map[string]*storage.UserGrant, 2)
			for candidate, snapshot := range barrier.committed {
				action := "grant_updated"
				if candidate == snapshot.ID {
					action = "grant_created"
				}
				expectedByAction[action] = snapshot
			}
			require.Len(t, expectedByAction, 2)
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
				for key := range record {
					assert.False(t, strings.HasPrefix(key, "previous_observed_"), "neither raced initial write observed a before-snapshot")
				}
				assert.NotContains(t, record, "actor")
				assert.NotContains(t, record, "trace_id")
			}
			assert.Empty(t, expectedByAction, "each successful write must be observed exactly once")
			assert.Equal(t, map[string]int{"grant_created": 1, "grant_updated": 1}, actions)
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
			assert.Len(t, records, 2)
		})
	}
}
