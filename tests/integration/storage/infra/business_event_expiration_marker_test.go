//go:build integration

package storage_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestBusinessEventExpirationMarkers_AtomicRecognitionAndRenewal(t *testing.T) {
	adapter, owner := openLedgerFoundation(t)
	ctx := context.Background()
	agentID := id.NewAgentID()
	_, err := owner.Exec(`INSERT INTO public.agents(id,display_name,description) VALUES ($1,'Expiry agent','Test fixture')`, agentID)
	require.NoError(t, err)
	expiry := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	for _, family := range []string{"grant", "approval"} {
		t.Run(family, func(t *testing.T) {
			var recognize func(context.Context, time.Time) (bool, error)
			var marker func() sql.NullTime
			var changeExpiry func(time.Time)
			var candidates func() int
			if family == "grant" {
				key := id.NewGrantID()
				_, err := owner.Exec(`INSERT INTO public.user_grants(id,principal,agent_id,valid_until,granted_permission_sets) VALUES ($1,$2,$3,$4,'[]'::jsonb)`, key, "expiry-grant-user", agentID, expiry)
				require.NoError(t, err)
				repo := adapter.UserGrants().(ports.UserGrantExpirationRepository)
				recognize = func(ctx context.Context, at time.Time) (bool, error) { return repo.RecordExpiration(ctx, key, at) }
				marker = func() sql.NullTime {
					var at sql.NullTime
					require.NoError(t, owner.Get(&at, `SELECT expiration_recorded_for FROM public.user_grants WHERE id=$1`, key))
					return at
				}
				changeExpiry = func(at time.Time) {
					_, err := owner.Exec(`UPDATE public.user_grants SET valid_until=$2 WHERE id=$1`, key, at)
					require.NoError(t, err)
				}
				candidates = func() int {
					rows, err := repo.ListUnrecordedExpiredForPrincipal(ctx, "expiry-grant-user", time.Now().UTC(), 100)
					require.NoError(t, err)
					n := 0
					for _, row := range rows {
						if row.ID == key {
							n++
						}
					}
					return n
				}
			} else {
				key := id.NewApprovalID()
				_, err := owner.Exec(`INSERT INTO public.tool_approvals(id,principal,agent_id,gateway_client_id,tool_name,tool_pattern,arguments_hash,status,approval_url,expires_at) VALUES ($1,$2,$3,'expiry-gateway','read_file','read_file',$5,'pending','https://broker.example/approval',$4)`, key, "expiry-approval-user", agentID, expiry, key.String())
				require.NoError(t, err)
				repo := adapter.ToolApprovals().(ports.ToolApprovalExpirationRepository)
				recognize = func(ctx context.Context, at time.Time) (bool, error) { return repo.RecordExpiration(ctx, key, at) }
				marker = func() sql.NullTime {
					var at sql.NullTime
					require.NoError(t, owner.Get(&at, `SELECT expiration_recorded_for FROM public.tool_approvals WHERE id=$1`, key))
					return at
				}
				changeExpiry = func(at time.Time) {
					_, err := owner.Exec(`UPDATE public.tool_approvals SET expires_at=$2 WHERE id=$1`, key, at)
					require.NoError(t, err)
				}
				candidates = func() int {
					rows, err := repo.ListUnrecordedExpiredForPrincipal(ctx, "expiry-approval-user", time.Now().UTC(), 100)
					require.NoError(t, err)
					n := 0
					for _, row := range rows {
						if row.ID == key {
							n++
						}
					}
					return n
				}
			}
			require.Equal(t, 1, candidates())
			tx, err := adapter.BeginTX(ctx)
			require.NoError(t, err)
			won, err := recognize(tx, expiry)
			require.NoError(t, err)
			require.NoError(t, adapter.Rollback(tx))
			require.True(t, won)
			require.False(t, marker().Valid, "rollback must not consume the effective expiry")
			type result struct {
				won bool
				err error
			}
			results := make(chan result, 8)
			var workers sync.WaitGroup
			for range 8 {
				workers.Go(func() { won, err := recognize(ctx, expiry); results <- result{won, err} })
			}
			workers.Wait()
			close(results)
			winners := 0
			for result := range results {
				require.NoError(t, result.err)
				if result.won {
					winners++
				}
			}
			require.Equal(t, 1, winners)
			require.Equal(t, expiry, marker().Time.UTC())
			require.Zero(t, candidates())
			second := expiry.Add(time.Minute)
			changeExpiry(second)
			won, err = recognize(ctx, expiry)
			require.NoError(t, err)
			require.False(t, won)
			require.Equal(t, 1, candidates())
			won, err = recognize(ctx, second)
			require.NoError(t, err)
			require.True(t, won)
			require.Equal(t, second, marker().Time.UTC())
		})
	}
}

func TestBusinessEventExpirationMarkers_ScopedCandidatesFilterBeforeLimit(t *testing.T) {
	adapter, owner := openLedgerFoundation(t)
	agentID := id.NewAgentID()
	_, err := owner.Exec(`INSERT INTO public.agents(id,display_name,description) VALUES ($1,'Scoped expiry agent','Fixture')`, agentID)
	require.NoError(t, err)
	requested, other := id.NewGrantID(), id.NewGrantID()
	now := time.Now().UTC().Truncate(time.Microsecond)
	for key, principal := range map[id.GrantID]string{requested: "requested-user", other: "other-user"} {
		expiry := now.Add(-time.Hour)
		if key == other {
			expiry = expiry.Add(-time.Hour)
		}
		_, err := owner.Exec(`INSERT INTO public.user_grants(id,principal,agent_id,valid_until,granted_permission_sets) VALUES ($1,$2,$3,$4,'[]'::jsonb)`, key, principal, agentID, expiry)
		require.NoError(t, err)
	}
	rows, err := adapter.UserGrants().(ports.UserGrantExpirationRepository).ListUnrecordedExpiredForPrincipal(context.Background(), "requested-user", now, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, requested, rows[0].ID)
}

func TestBusinessEventGrantExpiry_PreservesInstantAcrossTimezones(t *testing.T) {
	adapter, owner := openLedgerFoundation(t)
	ctx := context.Background()
	agentID := id.NewAgentID()
	_, err := owner.Exec(`INSERT INTO public.agents(id,display_name,description) VALUES ($1,'Timezone expiry agent','Fixture')`, agentID)
	require.NoError(t, err)
	for _, offset := range []int{2 * 3600, -7 * 3600} {
		t.Run((time.Duration(offset) * time.Second).String(), func(t *testing.T) {
			expiry := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond).In(time.FixedZone("fixture", offset))
			grant := &storage.UserGrant{ID: id.NewGrantID(), Principal: id.NewPrincipal(t.Name()), AgentID: agentID, ValidUntil: &expiry, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), GrantedPermissionSets: []storage.GrantedPermissionSetEntry{}}
			require.NoError(t, adapter.UserGrants().Create(ctx, grant))
			stored, err := adapter.UserGrants().Get(ctx, grant.ID)
			require.NoError(t, err)
			require.True(t, stored.ValidUntil.Equal(expiry), "create must retain the expiry instant, not its local wall time")
			expiry = expiry.Add(time.Hour)
			require.NoError(t, adapter.UserGrants().Create(ctx, grant))
			stored, err = adapter.UserGrants().Get(ctx, grant.ID)
			require.NoError(t, err)
			require.True(t, stored.ValidUntil.Equal(expiry), "upsert must retain the expiry instant")
			expiry = expiry.Add(time.Hour)
			require.NoError(t, adapter.UserGrants().Update(ctx, grant))
			stored, err = adapter.UserGrants().Get(ctx, grant.ID)
			require.NoError(t, err)
			require.True(t, stored.ValidUntil.Equal(expiry), "update must retain the expiry instant")
			grant.ValidUntil = nil
			require.NoError(t, adapter.UserGrants().Update(ctx, grant))
			stored, err = adapter.UserGrants().Get(ctx, grant.ID)
			require.NoError(t, err)
			require.Nil(t, stored.ValidUntil, "an indefinite grant must remain indefinite")
		})
	}
}
