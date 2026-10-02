package agents

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These port doubles model scoped publication and the receipt projection. Actual
// PostgreSQL receipt durability and FK cascades belong to the storage contracts.
type deleteScopeKey struct{}

type deleteScope struct {
	agentID  id.AgentID
	at       time.Time
	roots    map[id.RefreshSessionID]*storage.RefreshSession
	receipts map[id.RefreshSessionID]storage.RefreshRevocationReceipt
	deleting bool
}

// deleteLegacyRow models only the old-binary row data needed by this lifecycle test.
type deleteLegacyRow struct {
	Signature string
	RequestID string
	AgentID   id.AgentID
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type deleteState struct {
	repo     *deleteAgentRepo
	roots    map[id.RefreshSessionID]*storage.RefreshSession
	tokens   map[string]*storage.RefreshToken
	legacy   map[string]*deleteLegacyRow
	receipts map[id.RefreshSessionID]storage.RefreshRevocationReceipt
}

type deleteAgentRepo struct {
	*mockAgentRepo
	state     *deleteState
	deleteErr error
}

func (r *deleteAgentRepo) Delete(ctx context.Context, agentID id.AgentID) error {
	scope, ok := ctx.Value(deleteScopeKey{}).(*deleteScope)
	if !ok || scope.agentID != agentID {
		return errors.New("agent deletion requires an agent-scoped authorization transaction")
	}
	if r.deleteErr != nil {
		return r.deleteErr
	}
	// The repository must not cascade away the only evidence of active roots.
	for _, root := range r.state.roots {
		if root.AgentID != agentID || root.TerminalReason != nil {
			continue
		}
		revoked := scope.roots[root.ID]
		receipt, recorded := scope.receipts[root.ID]
		if revoked == nil || revoked.TerminalReason == nil || *revoked.TerminalReason != storage.RefreshReasonAgentDeleted ||
			!recorded || receipt.Reason != storage.RefreshReasonAgentDeleted {
			return errors.New("agent deletion requires revocation receipts before cascade")
		}
	}
	scope.deleting = true
	return nil
}

type deleteCoordinator struct {
	state     *deleteState
	at        time.Time
	commitErr error
}

func (c *deleteCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	if c.state.repo.agents[agentID] == nil {
		return storage.NewStorageError("DeleteAgent", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}
	scope := &deleteScope{agentID: agentID, at: c.at, roots: make(map[id.RefreshSessionID]*storage.RefreshSession), receipts: make(map[id.RefreshSessionID]storage.RefreshRevocationReceipt)}
	if err := operation(context.WithValue(ctx, deleteScopeKey{}, scope), c.at); err != nil {
		return err
	}
	if c.commitErr != nil {
		return c.commitErr // Confirmed pre-publication rollback.
	}
	for rootID, receipt := range scope.receipts {
		c.state.receipts[rootID] = receipt // Independent of root and agent cascades.
	}
	for rootID, root := range scope.roots {
		c.state.roots[rootID] = root
	}
	if !scope.deleting {
		return nil
	}
	removed := make(map[id.RefreshSessionID]bool)
	for rootID, root := range c.state.roots {
		if root.AgentID == agentID {
			removed[rootID] = true
			delete(c.state.roots, rootID)
		}
	}
	for signature, token := range c.state.tokens {
		if removed[token.SessionID] {
			delete(c.state.tokens, signature)
		}
	}
	for signature, token := range c.state.legacy {
		if token.AgentID == agentID {
			delete(c.state.legacy, signature)
		}
	}
	delete(c.state.repo.agents, agentID)
	return nil
}

type deleteRevocations struct {
	state      *deleteState
	receiptErr error
	revokeErr  error
}

func (r *deleteRevocations) ListActiveByAgent(ctx context.Context, agentID id.AgentID, principal *id.Principal) ([]storage.RefreshSessionAuditIdentity, error) {
	scope, ok := ctx.Value(deleteScopeKey{}).(*deleteScope)
	if !ok || scope.agentID != agentID {
		return nil, errors.New("agent listing requires its scoped authorization transaction")
	}
	var active []storage.RefreshSessionAuditIdentity
	for _, original := range r.state.roots {
		root := original
		if staged, ok := scope.roots[root.ID]; ok {
			root = staged
		}
		if root.AgentID != agentID || root.TerminalReason != nil || (principal != nil && root.Principal != *principal) {
			continue
		}
		active = append(active, root.AuditIdentity())
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID.String() < active[j].ID.String() })
	return active, nil
}

func (r *deleteRevocations) RevokeByAgent(ctx context.Context, agentID id.AgentID, at time.Time, reason storage.RefreshRevocationReason) error {
	scope, ok := ctx.Value(deleteScopeKey{}).(*deleteScope)
	if !ok || scope.agentID != agentID || !at.Equal(scope.at) || reason != storage.RefreshReasonAgentDeleted {
		return errors.New("agent revocation requires its scoped decision time and deletion reason")
	}
	if r.receiptErr != nil {
		return r.receiptErr
	}
	for _, root := range r.state.roots {
		if root.AgentID != agentID || root.TerminalReason != nil {
			continue
		}
		// A receipt snapshots non-credential provenance rather than referring to
		// a root that the agent deletion will remove.
		scope.receipts[root.ID] = storage.RefreshRevocationReceipt{
			SessionID: root.ID, AgentID: root.AgentID, Principal: root.Principal,
			ClientID: root.ClientID, Reason: reason, At: at, RedactedContext: security.RedactedAuditContext(ctx),
		}
	}
	if r.revokeErr != nil {
		return r.revokeErr // Receipt writes must roll back as well.
	}
	for _, root := range r.state.roots {
		if root.AgentID != agentID || root.TerminalReason != nil {
			continue
		}
		changed := *root
		changed.RevokedAt = &at
		changed.TerminalReason = &reason
		changed.RetryCiphertext = nil
		if err := changed.ValidateTransition(root, at); err != nil {
			return err
		}
		scope.roots[root.ID] = &changed
	}
	return nil
}

func (r *deleteRevocations) RevokeByID(context.Context, id.RefreshSessionID, time.Time, storage.RefreshRevocationReason) error {
	return errors.New("agent deletion must not revoke one session at a time")
}

func (r *deleteRevocations) RevokeByPrincipalAndAgent(context.Context, id.Principal, id.AgentID, time.Time, storage.RefreshRevocationReason) error {
	return errors.New("agent deletion must cover every principal")
}

type deleteFixture struct {
	service     *Service
	state       *deleteState
	coordinator *deleteCoordinator
	revocations *deleteRevocations
	logs        *bytes.Buffer
	agentID     id.AgentID
	otherID     id.AgentID
	at          time.Time
}

func newDeleteFixture() *deleteFixture {
	at := time.Date(2034, time.June, 10, 12, 0, 0, 0, time.UTC)
	agentID, otherID := id.NewAgentID(), id.NewAgentID()
	repo := &deleteAgentRepo{mockAgentRepo: newMockAgentRepo()}
	repo.agents[agentID] = &storage.Agent{ID: agentID, DisplayName: "Target", Description: "Target agent", PermissionSets: testPermissionSets()}
	repo.agents[otherID] = &storage.Agent{ID: otherID, DisplayName: "Other", Description: "Other agent", PermissionSets: testPermissionSets()}
	state := &deleteState{repo: repo, roots: make(map[id.RefreshSessionID]*storage.RefreshSession), tokens: make(map[string]*storage.RefreshToken), legacy: make(map[string]*deleteLegacyRow), receipts: make(map[id.RefreshSessionID]storage.RefreshRevocationReceipt)}
	repo.state = state
	logs := &bytes.Buffer{}
	coordinator := &deleteCoordinator{state: state, at: at}
	revocations := &deleteRevocations{state: state}
	service := NewService(repo, &mockServiceReqValidator{}, slog.New(slog.NewTextHandler(logs, nil)), true, coordinator, revocations)
	return &deleteFixture{service: service, state: state, coordinator: coordinator, revocations: revocations, logs: logs, agentID: agentID, otherID: otherID, at: at}
}

func deleteSignature(rootID id.RefreshSessionID, suffix string) string {
	digest := sha256.Sum256([]byte(rootID.String() + suffix))
	return hex.EncodeToString(digest[:])
}

func (f *deleteFixture) addRoot(t *testing.T, agentID id.AgentID, principal id.Principal, withPredecessor bool) *storage.RefreshSession {
	t.Helper()
	rootID := id.NewRefreshSessionID()
	started := f.at.Add(-time.Hour)
	initial := deleteSignature(rootID, "-initial")
	clientID := id.ClientID("client-" + agentID.String())
	root := &storage.RefreshSession{
		ID: rootID, OriginalGrantID: id.NewGrantID(), OriginalTokenSignature: initial,
		AgentID: agentID, Principal: principal, ClientID: clientID, Scope: "read",
		StartedAt: started, LastFreshAt: started, InactivityExpiresAt: f.at.Add(time.Hour),
		RetainUntil: f.at.Add(48 * time.Hour), BranchKeyID: "refresh_" + rootID.String() + "_branch_key",
		CurrentSignature: initial,
	}
	first := &storage.RefreshToken{Signature: initial, SessionID: rootID, IssuedAt: started, ExpiresAt: f.at.Add(time.Hour)}
	if withPredecessor {
		freshAt, reuseUntil := f.at.Add(-5*time.Second), f.at.Add(25*time.Second)
		accessUntil := f.at.Add(time.Hour)
		requestedScope := "read"
		fingerprint := deleteSignature(rootID, "-request")
		root.LastFreshAt = freshAt
		root.CurrentSignature = deleteSignature(rootID, "-current")
		root.PreviousSignature = &initial
		root.PreviousConsumedAt = &freshAt
		root.ReuseUntil = &reuseUntil
		root.RetryExpiresAt = &reuseUntil
		root.RetryAccessExpiresAt = &accessUntil
		root.OriginalRequestedScope = &requestedScope
		root.OriginalRequestContextFingerprint = &fingerprint
		root.RetryCiphertext = []byte("sealed-private-retry-payload")
		first.UsedAt = &freshAt
		current := &storage.RefreshToken{Signature: root.CurrentSignature, SessionID: rootID, IssuedAt: freshAt, ExpiresAt: accessUntil}
		require.NoError(t, current.Validate())
		f.state.tokens[current.Signature] = current
	}
	require.NoError(t, root.Validate())
	require.NoError(t, first.Validate())
	f.state.roots[rootID] = root
	f.state.tokens[first.Signature] = first
	for _, token := range f.state.tokens {
		if token.SessionID != rootID {
			continue
		}
		f.state.legacy[token.Signature] = &deleteLegacyRow{
			Signature: token.Signature, RequestID: rootID.String(), AgentID: agentID,
			ExpiresAt: token.ExpiresAt, UsedAt: token.UsedAt,
		}
	}
	return root
}

// A consumed predecessor is still live while its saved retry is eligible.
func (f *deleteFixture) canRenew(signature string) bool {
	token := f.state.tokens[signature]
	if token == nil || !f.at.Before(token.ExpiresAt) {
		return false
	}
	root := f.state.roots[token.SessionID]
	if root == nil || root.TerminalReason != nil || f.state.repo.agents[root.AgentID] == nil {
		return false
	}
	if token.UsedAt == nil {
		return true
	}
	return root.PreviousSignature != nil && *root.PreviousSignature == signature &&
		root.RetryExpiresAt != nil && f.at.Before(*root.RetryExpiresAt) && len(root.RetryCiphertext) != 0
}

func (f *deleteFixture) legacyCanRenew(signature string) bool {
	legacy := f.state.legacy[signature]
	return legacy != nil && legacy.UsedAt == nil && f.state.repo.agents[legacy.AgentID] != nil && f.at.Before(legacy.ExpiresAt)
}

func TestService_Delete_RevokesEveryPrincipalAndKeepsIndependentReceipts(t *testing.T) {
	f := newDeleteFixture()
	alice, bob := id.Principal("alice@example.test"), id.Principal("bob@example.test")
	rotated := f.addRoot(t, f.agentID, alice, true)
	anotherChain := f.addRoot(t, f.agentID, alice, false)
	otherPrincipal := f.addRoot(t, f.agentID, bob, false)
	otherAgent := f.addRoot(t, f.otherID, alice, true)
	for _, signature := range []string{rotated.CurrentSignature, *rotated.PreviousSignature, anotherChain.CurrentSignature, otherPrincipal.CurrentSignature, otherAgent.CurrentSignature, *otherAgent.PreviousSignature} {
		assert.True(t, f.canRenew(signature), "control: %s should be eligible before deletion", signature)
	}
	assert.True(t, f.legacyCanRenew(rotated.CurrentSignature))
	assert.True(t, f.legacyCanRenew(otherAgent.CurrentSignature))
	unrelatedRoot := *otherAgent
	unrelatedRoot.RetryCiphertext = bytes.Clone(otherAgent.RetryCiphertext)
	unrelatedCurrent := *f.state.tokens[otherAgent.CurrentSignature]
	unrelatedLegacy := *f.state.legacy[otherAgent.CurrentSignature]

	err := f.service.Delete(context.Background(), f.agentID)
	require.NoError(t, err)
	_, err = f.service.Get(context.Background(), f.agentID)
	require.ErrorIs(t, err, ports.ErrNotFound)
	_, err = f.service.Get(context.Background(), f.otherID)
	require.NoError(t, err)

	for _, previous := range []*storage.RefreshSession{rotated, anotherChain, otherPrincipal} {
		assert.NotContains(t, f.state.roots, previous.ID, "deleted root must no longer authorize even after re-registration")
		assert.NotContains(t, f.state.tokens, previous.CurrentSignature)
		assert.NotContains(t, f.state.legacy, previous.CurrentSignature)
		receipt, ok := f.state.receipts[previous.ID]
		require.True(t, ok, "agent cascade must retain an independent receipt for each root")
		assert.Equal(t, previous.ID, receipt.SessionID)
		assert.Equal(t, f.agentID, receipt.AgentID)
		assert.Equal(t, previous.Principal, receipt.Principal)
		assert.Equal(t, previous.ClientID, receipt.ClientID)
		assert.Equal(t, storage.RefreshReasonAgentDeleted, receipt.Reason)
		assert.Equal(t, f.at, receipt.At)
	}
	assert.NotContains(t, f.state.tokens, *rotated.PreviousSignature)
	assert.NotContains(t, f.state.legacy, *rotated.PreviousSignature)
	assert.NotContains(t, f.state.receipts, otherAgent.ID)
	assert.Equal(t, &unrelatedRoot, f.state.roots[otherAgent.ID])
	assert.Equal(t, &unrelatedCurrent, f.state.tokens[otherAgent.CurrentSignature])
	assert.Equal(t, &unrelatedLegacy, f.state.legacy[otherAgent.CurrentSignature])
	assert.True(t, f.canRenew(otherAgent.CurrentSignature))
	assert.True(t, f.canRenew(*otherAgent.PreviousSignature))
	assert.True(t, f.legacyCanRenew(otherAgent.CurrentSignature))
	beforeReceipts := make(map[id.RefreshSessionID]storage.RefreshRevocationReceipt, len(f.state.receipts))
	for rootID, receipt := range f.state.receipts {
		beforeReceipts[rootID] = receipt
	}

	// Re-registering the same ID cannot recover native or old-writer authority.
	require.NoError(t, f.service.Create(context.Background(), &storage.Agent{
		ID: f.agentID, DisplayName: "Recreated", Description: "New registration", PermissionSets: testPermissionSets(),
	}))
	for _, signature := range []string{rotated.CurrentSignature, *rotated.PreviousSignature, anotherChain.CurrentSignature, otherPrincipal.CurrentSignature} {
		assert.False(t, f.canRenew(signature))
		assert.False(t, f.legacyCanRenew(signature))
	}
	assert.Equal(t, beforeReceipts, f.state.receipts, "new registration must not erase or change revocation evidence")
}

func TestService_Delete_FailuresLeaveAgentAndNativeAuthorityUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name   string
		inject func(*deleteFixture, error)
	}{
		{"receipt write", func(f *deleteFixture, err error) { f.revocations.receiptErr = err }},
		{"revocation after receipt", func(f *deleteFixture, err error) { f.revocations.revokeErr = err }},
		{"agent delete after revocation", func(f *deleteFixture, err error) { f.state.repo.deleteErr = err }},
		{"commit before publication", func(f *deleteFixture, err error) { f.coordinator.commitErr = err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeleteFixture()
			first := f.addRoot(t, f.agentID, id.Principal("alice@example.test"), true)
			second := f.addRoot(t, f.agentID, id.Principal("bob@example.test"), false)
			unrelated := f.addRoot(t, f.otherID, id.Principal("alice@example.test"), false)
			beforeAgent := f.state.repo.agents[f.agentID].Copy()
			beforeRoots := make(map[id.RefreshSessionID]storage.RefreshSession)
			beforeTokens := make(map[string]storage.RefreshToken)
			beforeLegacy := make(map[string]deleteLegacyRow)
			for rootID, root := range f.state.roots {
				copy := *root
				copy.RetryCiphertext = bytes.Clone(root.RetryCiphertext)
				beforeRoots[rootID] = copy
			}
			for signature, token := range f.state.tokens {
				beforeTokens[signature] = *token
			}
			for signature, token := range f.state.legacy {
				beforeLegacy[signature] = *token
			}
			fault := errors.New("delete lifecycle " + tc.name + " failed")
			tc.inject(f, fault)

			err := f.service.Delete(context.Background(), f.agentID)
			require.ErrorIs(t, err, fault)
			assert.NotContains(t, err.Error(), "sealed-private-retry-payload")
			assert.NotContains(t, f.logs.String(), "agent deleted", "success log requires successful commit")
			assert.NotContains(t, f.logs.String(), "sealed-private-retry-payload")
			agent, err := f.service.Get(context.Background(), f.agentID)
			require.NoError(t, err)
			assert.Equal(t, beforeAgent, agent)
			assert.Empty(t, f.state.receipts, "failed operations must not publish a misleading receipt")
			assert.Len(t, f.state.roots, len(beforeRoots))
			for rootID, root := range beforeRoots {
				assert.Equal(t, &root, f.state.roots[rootID], "failed deletion must not mutate root authority")
			}
			assert.Len(t, f.state.tokens, len(beforeTokens))
			for signature, token := range beforeTokens {
				assert.Equal(t, &token, f.state.tokens[signature], "failed deletion must not consume native tokens")
			}
			assert.Len(t, f.state.legacy, len(beforeLegacy))
			for signature, token := range beforeLegacy {
				assert.Equal(t, &token, f.state.legacy[signature], "failed deletion must not consume old-writer tokens")
			}
			for _, root := range []*storage.RefreshSession{first, second, unrelated} {
				assert.True(t, f.canRenew(root.CurrentSignature))
				assert.True(t, f.legacyCanRenew(root.CurrentSignature))
			}
			assert.True(t, f.canRenew(*first.PreviousSignature))
		})
	}
}

func TestService_Delete_MissingAgentInsideScopeDoesNotRevoke(t *testing.T) {
	f := newDeleteFixture()
	root := f.addRoot(t, f.agentID, id.Principal("alice@example.test"), true)
	f.state.repo.getFn = func(context.Context, id.AgentID) (*storage.Agent, error) {
		return nil, storage.NewStorageError("GetAgent", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}

	err := f.service.Delete(context.Background(), f.agentID)
	require.ErrorIs(t, err, ports.ErrNotFound)
	assert.Contains(t, f.state.repo.agents, f.agentID)
	assert.Equal(t, root, f.state.roots[root.ID])
	assert.True(t, f.canRenew(root.CurrentSignature))
	assert.Empty(t, f.state.receipts)
	assert.NotContains(t, f.logs.String(), "agent deleted")
}
