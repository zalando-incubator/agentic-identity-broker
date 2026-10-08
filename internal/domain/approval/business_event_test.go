package approval

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

type ledgerApprovalExpirations struct {
	repo    *mockApprovalRepo
	markers map[id.ApprovalID]time.Time
}

func (e *ledgerApprovalExpirations) ListUnrecordedExpiredForPrincipal(_ context.Context, principal id.Principal, at time.Time, limit int) ([]*storage.ToolApproval, error) {
	var candidates []*storage.ToolApproval
	for _, approval := range e.repo.approvals {
		if approval.Principal == principal && approval.Status == storage.ApprovalStatusPending && approval.IsExpired(at) && !e.markers[approval.ID].Equal(approval.ExpiresAt) {
			copy := *approval
			candidates = append(candidates, &copy)
			if len(candidates) == limit {
				break
			}
		}
	}
	return candidates, nil
}
func (e *ledgerApprovalExpirations) RecordExpiration(_ context.Context, key id.ApprovalID, expiry time.Time) (bool, error) {
	approval := e.repo.approvals[key]
	if approval == nil || approval.Status != storage.ApprovalStatusPending || !approval.IsExpired(time.Now()) || !approval.ExpiresAt.Equal(expiry) || e.markers[key].Equal(expiry) {
		return false, nil
	}
	e.markers[key] = expiry
	return true, nil
}

func newLedgerApproval(t *testing.T) (*Service, *mockApprovalRepo, *ledgerfixture.Store, *mockSyncStateRepo, *ledgerApprovalExpirations) {
	t.Helper()
	repo := newMockApprovalRepo()
	syncState := &mockSyncStateRepo{}
	markers := &ledgerApprovalExpirations{repo: repo, markers: repo.expirationRecordedFor}
	store := &ledgerfixture.Store{Snapshot: func() func() {
		approvals := make(map[id.ApprovalID]*storage.ToolApproval, len(repo.approvals))
		for key, approval := range repo.approvals {
			copy := *approval
			approvals[key] = &copy
		}
		expirations := make(map[id.ApprovalID]time.Time, len(markers.markers))
		for key, at := range markers.markers {
			expirations[key] = at
		}
		version := syncState.version
		return func() {
			repo.approvals, markers.markers, syncState.version = approvals, expirations, version
			repo.expirationRecordedFor = markers.markers
		}
	}}
	svc := newTestService(repo)
	svc.syncState = syncState
	svc.ledger, svc.expirations = store.Recorder(t), markers
	return svc, repo, store, syncState, markers
}

func TestApprovalLedgerWinningTransitionsAndSilentRepeats(t *testing.T) {
	svc, _, store, _, _ := newLedgerApproval(t)
	ctx := context.Background()
	principal, agentID := id.Principal("ledger-user"), id.NewAgentID()
	request := CreateApprovalRequest{Principal: principal, AgentID: agentID, ToolName: "read-file", Arguments: map[string]any{"path": "credential-canary"}}
	created, err := svc.CreatePendingApproval(ctx, request)
	require.NoError(t, err)
	require.True(t, created.IsNew)
	repeated, err := svc.CreatePendingApproval(ctx, request)
	require.NoError(t, err)
	require.False(t, repeated.IsNew)
	require.Equal(t, created.Approval.ID, repeated.Approval.ID)
	_, err = svc.ApproveApproval(ctx, created.Approval.ID, principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
	require.NoError(t, err)
	_, err = svc.ConsumeApproval(ctx, created.Approval.ID, principal)
	require.NoError(t, err)
	_, err = svc.ConsumeApproval(ctx, created.Approval.ID, principal)
	require.NoError(t, err)
	require.Len(t, store.Events, 3)
	for i, name := range []string{"approval-requested", "approval-approved", "approval-consumed"} {
		event := store.Events[i]
		require.Equal(t, model.BusinessEventTypePrefix+name, event.Type)
		require.Equal(t, created.Approval.ID, event.ApprovalID)
		require.Equal(t, agentID, event.AgentID)
		require.Equal(t, principal, *event.Subject)
		require.NotContains(t, event.Data, "arguments")
	}
}

func TestApprovalLedgerConsumptionDoesNotReuseCreatorGatewayAsCaller(t *testing.T) {
	svc, _, store, _, _ := newLedgerApproval(t)
	principal := id.Principal("approval-owner")
	created, err := svc.CreatePendingApproval(context.Background(), CreateApprovalRequest{
		Principal: principal, AgentID: id.NewAgentID(), GatewayClientID: "creation-gateway",
		ToolName: "read-file", Arguments: map[string]any{"path": "safe"},
	})
	require.NoError(t, err)
	_, err = svc.ApproveApproval(context.Background(), created.Approval.ID, principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
	require.NoError(t, err)
	_, err = svc.ConsumeApproval(context.Background(), created.Approval.ID, principal)
	require.NoError(t, err)
	require.Len(t, store.Events, 3)
	consumed := store.Events[2]
	require.Equal(t, model.BusinessEventTypePrefix+"approval-consumed", consumed.Type)
	require.Equal(t, "gateway", consumed.Actor.Kind)
	require.Nil(t, consumed.Actor.ID, "subject-token authentication does not establish a consuming gateway")
	require.NotNil(t, consumed.Actor.OnBehalfOf)
	require.Equal(t, principal, *consumed.Actor.OnBehalfOf)
	require.NotNil(t, consumed.Subject)
	require.Equal(t, principal, *consumed.Subject)
	require.Equal(t, created.Approval.AgentID, consumed.AgentID)
}

func TestApprovalLedgerPollingLeavesExpiryDiscoveryToScopedOperations(t *testing.T) {
	svc, repo, store, syncState, markers := newLedgerApproval(t)
	principal := id.Principal("polling-owner")
	expired := makePendingApproval(principal, id.NewAgentID())
	expired.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	active := makePendingApproval(principal, id.NewAgentID())
	repo.approvals[expired.ID], repo.approvals[active.ID] = expired, active
	svc.queries = repo
	syncState.version = 8
	for _, filter := range []*id.Principal{nil, &principal} {
		state, err := svc.GetSyncState(context.Background(), filter, nil)
		require.NoError(t, err)
		require.Equal(t, int64(8), state.Version)
		require.Len(t, state.Pairs, 1)
		require.Equal(t, active.ID, state.Pairs[0].Approvals[0].ID)
		require.Empty(t, markers.markers, "sync polling must not discover expired approvals")
		require.Empty(t, store.Events)
	}
	_, err := svc.GetApproval(context.Background(), expired.ID, principal)
	require.ErrorIs(t, err, ErrApprovalGone)
	require.Equal(t, expired.ExpiresAt, markers.markers[expired.ID])
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"approval-expired", store.Events[0].Type)
	_, err = svc.GetSyncState(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, store.Events, 1)
}

func TestApprovalLedgerPendingListRecognizesOnlyBoundedPrincipalExpirations(t *testing.T) {
	svc, repo, store, _, markers := newLedgerApproval(t)
	principal := id.Principal("pending-list-owner")
	for range 201 {
		pending := makePendingApproval(principal, id.NewAgentID())
		pending.ExpiresAt = time.Now().UTC().Add(-time.Hour)
		repo.approvals[pending.ID] = pending
	}
	other := makePendingApproval("another-owner", id.NewAgentID())
	other.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	repo.approvals[other.ID] = other
	svc.queries = repo
	pending, err := svc.ListPendingApprovals(context.Background(), principal)
	require.NoError(t, err)
	require.Empty(t, pending)
	require.Len(t, markers.markers, 200, "one request recognizes at most one bounded batch")
	require.Len(t, store.Events, 200)
	require.NotContains(t, markers.markers, other.ID)
	pending, err = svc.ListPendingApprovals(context.Background(), principal)
	require.NoError(t, err)
	require.Empty(t, pending)
	require.Len(t, markers.markers, 201)
	require.Len(t, store.Events, 201)
	require.NotContains(t, markers.markers, other.ID)
}

func TestApprovalLedgerCreationDoesNotSweepOtherPrincipals(t *testing.T) {
	svc, repo, store, _, markers := newLedgerApproval(t)
	other := makePendingApproval("unrelated-owner", id.NewAgentID())
	other.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	repo.approvals[other.ID] = other
	created, err := svc.CreatePendingApproval(context.Background(), CreateApprovalRequest{
		Principal: "request-owner", AgentID: id.NewAgentID(), ToolName: "read-file", Arguments: map[string]any{"path": "safe"},
	})
	require.NoError(t, err)
	require.True(t, created.IsNew)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"approval-requested", store.Events[0].Type)
	require.NotContains(t, markers.markers, other.ID)
	_, err = svc.GetApproval(context.Background(), other.ID, other.Principal)
	require.ErrorIs(t, err, ErrApprovalGone)
	require.Equal(t, other.ExpiresAt, markers.markers[other.ID])
	require.Len(t, store.Events, 2)
}

func TestApprovalLedgerDenialAndRevocationAreDistinct(t *testing.T) {
	svc, repo, store, _, _ := newLedgerApproval(t)
	ctx := context.Background()
	principal, agentID := id.Principal("ledger-user"), id.NewAgentID()
	pending := makePendingApproval(principal, agentID)
	repo.approvals[pending.ID] = pending
	_, err := svc.DenyApproval(ctx, pending.ID, principal, nil)
	require.NoError(t, err)
	permanent := makePendingApproval(principal, agentID)
	persistence := storage.ApprovalPersistencePermanent
	permanent.Status, permanent.Persistence = storage.ApprovalStatusApproved, &persistence
	repo.approvals[permanent.ID] = permanent
	_, err = svc.RevokePermanentApproval(ctx, permanent.ID, principal)
	require.NoError(t, err)
	_, err = svc.RevokePermanentApproval(ctx, permanent.ID, principal)
	require.ErrorIs(t, err, ErrApprovalNotRevocable)
	require.Len(t, store.Events, 2)
	require.Equal(t, model.BusinessEventTypePrefix+"approval-denied", store.Events[0].Type)
	require.Equal(t, pending.ID, store.Events[0].ApprovalID)
	require.Equal(t, model.BusinessEventTypePrefix+"approval-revoked", store.Events[1].Type)
	require.Equal(t, permanent.ID, store.Events[1].ApprovalID)
}

func TestApprovalLedgerCASLoserDoesNotInventFact(t *testing.T) {
	svc, repo, store, syncState, _ := newLedgerApproval(t)
	pending := makePendingApproval(id.Principal("ledger-user"), id.NewAgentID())
	repo.approvals[pending.ID] = pending
	repo.approveFunc = func(context.Context, id.ApprovalID, storage.ApprovalDecision, time.Time) (*storage.ToolApproval, error) {
		repo.approvals[pending.ID].Status = storage.ApprovalStatusDenied
		return nil, storage.NewStorageError("Approve", storage.ErrorKindNotFound, ports.ErrNotFound, "lost transition")
	}
	_, err := svc.ApproveApproval(context.Background(), pending.ID, pending.Principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
	require.ErrorIs(t, err, ErrApprovalNotPending)
	require.Empty(t, store.Events)
	require.Zero(t, syncState.version)
}

func TestApprovalLedgerFailureRollsBackStateAndSyncVersion(t *testing.T) {
	for _, action := range []string{"request", "approve", "deny", "consume", "revoke"} {
		for _, failure := range []string{"append", "commit"} {
			t.Run(action+"/"+failure, func(t *testing.T) {
				svc, repo, store, syncState, _ := newLedgerApproval(t)
				principal, agentID := id.Principal("ledger-user"), id.NewAgentID()
				pending := makePendingApproval(principal, agentID)
				if action == "consume" || action == "revoke" {
					persistence := storage.ApprovalPersistenceOnce
					if action == "revoke" {
						persistence = storage.ApprovalPersistencePermanent
					}
					pending.Status, pending.Persistence = storage.ApprovalStatusApproved, &persistence
				}
				if action != "request" {
					repo.approvals[pending.ID] = pending
				}
				before := *pending
				failed := errors.New("ledger unavailable")
				if failure == "append" {
					store.AppendError = failed
				} else {
					store.CommitError = failed
				}
				ctx := context.Background()
				var err error
				switch action {
				case "request":
					result, failure := svc.CreatePendingApproval(ctx, CreateApprovalRequest{Principal: principal, AgentID: agentID, ToolName: "read-file", Arguments: map[string]any{}})
					err = failure
					require.Nil(t, result)
				case "approve":
					_, err = svc.ApproveApproval(ctx, pending.ID, principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
				case "deny":
					_, err = svc.DenyApproval(ctx, pending.ID, principal, nil)
				case "consume":
					_, err = svc.ConsumeApproval(ctx, pending.ID, principal)
				case "revoke":
					_, err = svc.RevokePermanentApproval(ctx, pending.ID, principal)
				}
				require.Error(t, err)
				require.Empty(t, store.Events)
				require.Zero(t, syncState.version)
				if action == "request" {
					require.Empty(t, repo.approvals)
				} else {
					require.Equal(t, &before, repo.approvals[pending.ID])
				}
			})
		}
	}
}

func TestApprovalLedgerLazyExpiryIsOncePerEffectiveExpiry(t *testing.T) {
	svc, repo, store, _, markers := newLedgerApproval(t)
	pending := makePendingApproval(id.Principal("ledger-user"), id.NewAgentID())
	pending.ExpiresAt = time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	repo.approvals[pending.ID] = pending
	for range 2 {
		_, err := svc.GetApproval(context.Background(), pending.ID, pending.Principal)
		require.ErrorIs(t, err, ErrApprovalGone)
	}
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"approval-expired", store.Events[0].Type)
	require.Equal(t, pending.ExpiresAt, store.Events[0].OccurredAt)
	require.Equal(t, pending.ExpiresAt, markers.markers[pending.ID])
	pending.ExpiresAt = pending.ExpiresAt.Add(time.Minute)
	_, err := svc.GetApproval(context.Background(), pending.ID, pending.Principal)
	require.ErrorIs(t, err, ErrApprovalGone)
	require.Len(t, store.Events, 2)
	require.Equal(t, pending.ExpiresAt, store.Events[1].OccurredAt)
}

func TestApprovalLedgerExpiredMutationCommitsRecognitionAfterRollback(t *testing.T) {
	for _, action := range []string{"approve", "deny"} {
		t.Run(action, func(t *testing.T) {
			svc, repo, store, syncState, markers := newLedgerApproval(t)
			pending := makePendingApproval(id.Principal("expired-mutation-user"), id.NewAgentID())
			pending.ExpiresAt = time.Now().UTC().Add(-time.Hour)
			repo.approvals[pending.ID] = pending
			var err error
			if action == "approve" {
				_, err = svc.ApproveApproval(context.Background(), pending.ID, pending.Principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
			} else {
				_, err = svc.DenyApproval(context.Background(), pending.ID, pending.Principal, nil)
			}
			require.ErrorIs(t, err, ErrApprovalGone)
			require.Equal(t, storage.ApprovalStatusPending, repo.approvals[pending.ID].Status)
			require.Zero(t, syncState.version)
			require.Equal(t, pending.ExpiresAt, markers.markers[pending.ID])
			require.Len(t, store.Events, 1)
			require.Equal(t, model.BusinessEventTypePrefix+"approval-expired", store.Events[0].Type)
		})
	}
}

func TestApprovalLedgerFailedExpiryRecordingDoesNotConsumeMarker(t *testing.T) {
	svc, repo, store, _, markers := newLedgerApproval(t)
	pending := makePendingApproval(id.Principal("expiry-rollback-user"), id.NewAgentID())
	pending.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	repo.approvals[pending.ID] = pending
	store.AppendError = errors.New("recording unavailable")
	_, err := svc.GetApproval(context.Background(), pending.ID, pending.Principal)
	require.Error(t, err)
	require.Empty(t, store.Events)
	require.Empty(t, markers.markers)
	store.AppendError = nil
	_, err = svc.GetApproval(context.Background(), pending.ID, pending.Principal)
	require.ErrorIs(t, err, ErrApprovalGone)
	require.Equal(t, pending.ExpiresAt, markers.markers[pending.ID])
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"approval-expired", store.Events[0].Type)
}

func TestApprovalLedgerExpiryCrossingDuringCreateRecordsBeforeRetirement(t *testing.T) {
	svc, repo, store, syncState, expirations := newLedgerApproval(t)
	pending := makePendingApproval(id.Principal("deadline-crossing-user"), id.NewAgentID())
	pending.Arguments = map[string]any{"path": "safe-fixture"}
	pending.ArgumentsHash = storage.ComputeArgumentsHash(pending.Arguments)
	repo.approvals[pending.ID] = pending
	svc.queries = &mockQueryRepo{listActiveByPairFunc: func(context.Context, id.Principal, id.AgentID) ([]*storage.ToolApproval, error) {
		pending.ExpiresAt = time.Now().UTC().Add(-time.Second)
		return nil, nil
	}}
	created, err := svc.CreatePendingApproval(context.Background(), CreateApprovalRequest{Principal: pending.Principal, AgentID: pending.AgentID, ToolName: pending.ToolName, Arguments: pending.Arguments})
	require.NoError(t, err)
	require.True(t, created.IsNew)
	require.NotEqual(t, pending.ID, created.Approval.ID)
	require.True(t, repo.approvals[pending.ID].Consumed)
	require.Equal(t, pending.ExpiresAt, expirations.markers[pending.ID])
	require.Equal(t, int64(1), syncState.version)
	require.Len(t, store.Events, 2)
	require.Equal(t, model.BusinessEventTypePrefix+"approval-expired", store.Events[0].Type)
	require.Equal(t, pending.ID, store.Events[0].ApprovalID)
	require.Equal(t, model.BusinessEventTypePrefix+"approval-requested", store.Events[1].Type)
	require.Equal(t, created.Approval.ID, store.Events[1].ApprovalID)
}

func TestApprovalLedgerConcurrentCreationWaitsForCommittedWinner(t *testing.T) {
	svc, _, store, _, _ := newLedgerApproval(t)
	request := CreateApprovalRequest{Principal: id.Principal("creation-commit-user"), AgentID: id.NewAgentID(), ToolName: "read-file", Arguments: map[string]any{"path": "safe"}}
	ready, release := make(chan struct{}), make(chan struct{})
	var paused atomic.Bool
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	store.BeforeCommit = func() {
		if paused.CompareAndSwap(false, true) {
			close(ready)
			<-release
		}
	}
	type outcome struct {
		result *CreateApprovalResult
		err    error
	}
	first, second := make(chan outcome, 1), make(chan outcome, 1)
	create := func(done chan<- outcome) {
		result, err := svc.CreatePendingApproval(context.Background(), request)
		done <- outcome{result, err}
	}
	go create(first)
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("first creation did not reach commit")
	}
	go create(second)
	select {
	case result := <-second:
		t.Fatalf("concurrent creation returned before the winning commit: %v", result.err)
	case <-time.After(200 * time.Millisecond):
	}
	unblock()
	var winner, repeated outcome
	select {
	case winner = <-first:
	case <-time.After(time.Second):
		t.Fatal("winning creation did not commit")
	}
	select {
	case repeated = <-second:
	case <-time.After(time.Second):
		t.Fatal("waiting creation did not complete after commit")
	}
	require.NoError(t, winner.err)
	require.NoError(t, repeated.err)
	require.True(t, winner.result.IsNew)
	require.False(t, repeated.result.IsNew)
	require.Equal(t, winner.result.Approval.ID, repeated.result.Approval.ID)
	require.Len(t, store.Events, 1)
	require.Equal(t, winner.result.Approval.ID, store.Events[0].ApprovalID)
}
