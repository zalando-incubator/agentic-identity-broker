package approval

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/toolpattern"
)

func approvedRecord(id, toolPattern string, params map[string]string, persistence string, decidedAt time.Time) Record {
	return Record{
		ID:            id,
		ToolPattern:   toolPattern,
		ParamsPattern: params,
		Status:        "approved",
		Persistence:   new(persistence),
		ApprovedAt:    new(decidedAt),
	}
}

func TestCacheMatchFiltersScopeAndMissingDecisionTime(t *testing.T) {
	now := time.Now().UTC()
	cache := NewCache(time.Minute)
	identity := Identity{Principal: "alice", AgentID: "agent"}
	otherIdentity := Identity{Principal: "bob", AgentID: "agent"}
	cache.Replace([]Pair{
		{Identity: identity, Approvals: []Record{
			{ID: "missing-time", ToolPattern: "create_*", ParamsPattern: map[string]string{}, Status: "approved", Persistence: new("permanent")},
			approvedRecord("pending", "create_*", map[string]string{}, "permanent", now),
			approvedRecord("session", "create_*", map[string]string{}, "session", now),
			approvedRecord("permanent", "create_*", map[string]string{}, "permanent", now),
		}},
		{Identity: otherIdentity, Approvals: []Record{approvedRecord("other", "create_*", map[string]string{}, "permanent", now)}},
	}, `"v1"`)

	entry := &cache.pairs[pairKey{principal: identity.Principal, agentID: identity.AgentID}].records[1]
	entry.Status = "pending"
	entry.AgentSessionID = new("session-a")
	cache.pairs[pairKey{principal: identity.Principal, agentID: identity.AgentID}].records[2].AgentSessionID = new("session-a")

	record, matched := cache.Match(identity, "session-b", "create_issue", map[string]any{})
	require.True(t, matched)
	assert.Equal(t, "permanent", record.ID)
	assert.Equal(t, `"v1"`, cache.ETag())

	_, matched = cache.Match(otherIdentity, "session-b", "create_issue", map[string]any{})
	assert.True(t, matched)
	_, matched = cache.Match(Identity{Principal: "alice", AgentID: "other-agent"}, "session-b", "create_issue", map[string]any{})
	assert.False(t, matched)
}

func TestCacheMatchesSharedVectors(t *testing.T) {
	now := time.Now().UTC()
	identity := Identity{Principal: "alice", AgentID: "agent"}
	for _, vector := range toolpattern.MatchVectors() {
		t.Run(vector.Name, func(t *testing.T) {
			cache := NewCache(time.Minute)
			cache.Replace([]Pair{{
				Identity:  identity,
				Approvals: []Record{approvedRecord("vector", vector.ToolPattern, vector.ParamsPattern, "permanent", now)},
			}}, `"v1"`)

			_, matched := cache.Match(identity, "", vector.ToolName, vector.Arguments)
			assert.Equal(t, vector.Match, matched)
		})
	}
}

func TestCacheUsesSharedPrecedenceVectors(t *testing.T) {
	identity := Identity{Principal: "alice", AgentID: "agent"}
	for _, vector := range toolpattern.PrecedenceVectors() {
		t.Run(vector.Name, func(t *testing.T) {
			records := make([]Record, 0, len(vector.Candidates))
			for _, candidate := range vector.Candidates {
				decidedAt, err := time.Parse(time.RFC3339, candidate.DecidedAt)
				require.NoError(t, err)
				records = append(records, approvedRecord(candidate.ID, candidate.ToolPattern, candidate.ParamsPattern, "permanent", decidedAt))
			}
			cache := NewCache(time.Minute)
			cache.Replace([]Pair{{Identity: identity, Approvals: records}}, `"v1"`)

			record, matched := cache.Match(identity, "", vector.ToolName, vector.Arguments)
			require.True(t, matched)
			assert.Equal(t, vector.WinnerID, record.ID)
		})
	}
}

func TestCacheReservesOnceApprovalExactlyOnce(t *testing.T) {
	now := time.Now().UTC()
	cache := NewCache(time.Minute)
	identity := Identity{Principal: "alice", AgentID: "agent"}
	cache.Replace([]Pair{{Identity: identity, Approvals: []Record{approvedRecord("once", "tool", map[string]string{}, "once", now)}}}, `"v1"`)

	first, matched := cache.Match(identity, "", "tool", map[string]any{})
	require.True(t, matched)
	assert.Equal(t, "once", first.ID)
	_, matched = cache.Match(identity, "", "tool", map[string]any{})
	assert.False(t, matched)

	cache.Release(identity, "once")
	_, matched = cache.Match(identity, "", "tool", map[string]any{})
	assert.True(t, matched)
	cache.ConfirmConsumed(identity, "once")
	_, matched = cache.Match(identity, "", "tool", map[string]any{})
	assert.False(t, matched)
}

func TestCacheReservationIsCompareAndSwap(t *testing.T) {
	cache := NewCache(time.Minute)
	identity := Identity{Principal: "alice", AgentID: "agent"}
	cache.Replace([]Pair{{Identity: identity, Approvals: []Record{approvedRecord("once", "tool", map[string]string{}, "once", time.Now().UTC())}}}, `"v1"`)

	const requests = 32
	start := make(chan struct{})
	var matched atomic.Int32
	var wg sync.WaitGroup
	for range requests {
		wg.Go(func() {
			<-start
			if _, ok := cache.Match(identity, "", "tool", map[string]any{}); ok {
				matched.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int32(1), matched.Load())
}

func TestCacheReplacesGlobalAndTargetedSnapshots(t *testing.T) {
	now := time.Now().UTC()
	cache := NewCache(time.Minute)
	alice := Identity{Principal: "alice", AgentID: "agent-a"}
	bob := Identity{Principal: "bob", AgentID: "agent-b"}
	cache.Replace([]Pair{
		{Identity: alice, Approvals: []Record{approvedRecord("alice-old", "tool", map[string]string{}, "permanent", now)}},
		{Identity: bob, Approvals: []Record{approvedRecord("bob-old", "tool", map[string]string{}, "permanent", now)}},
	}, `"v1"`)

	cache.Replace([]Pair{{Identity: bob, Approvals: []Record{approvedRecord("bob-new", "tool", map[string]string{}, "permanent", now)}}}, `"v2"`)
	_, matched := cache.Match(alice, "", "tool", map[string]any{})
	assert.False(t, matched, "a global snapshot must remove revoked pairs")
	record, matched := cache.Match(bob, "", "tool", map[string]any{})
	require.True(t, matched)
	assert.Equal(t, "bob-new", record.ID)

	cache.Replace([]Pair{
		{Identity: alice, Approvals: []Record{approvedRecord("alice-current", "tool", map[string]string{}, "permanent", now)}},
		{Identity: bob, Approvals: []Record{approvedRecord("bob-current", "tool", map[string]string{}, "permanent", now)}},
	}, `"v3"`)
	cache.ReplaceForPrincipal("alice", nil)
	_, matched = cache.Match(alice, "", "tool", map[string]any{})
	assert.False(t, matched, "a targeted empty snapshot must remove the principal's pairs")
	record, matched = cache.Match(bob, "", "tool", map[string]any{})
	require.True(t, matched)
	assert.Equal(t, "bob-current", record.ID)
	assert.Equal(t, `"v3"`, cache.ETag(), "targeted reads must not advance the global sync ETag")
}

func TestCacheEvictsInactiveSessionsAndIdlePairs(t *testing.T) {
	idleTTL := time.Minute
	now := time.Now().UTC()
	cache := NewCache(idleTTL)
	identity := Identity{Principal: "alice", AgentID: "agent"}
	cache.RecordSession("expired-session")
	cache.Replace([]Pair{{Identity: identity, Approvals: []Record{
		approvedRecord("session", "session-tool", map[string]string{}, "session", now),
		approvedRecord("permanent", "permanent-tool", map[string]string{}, "permanent", now),
	}}}, `"v1"`)
	cache.pairs[pairKey{principal: identity.Principal, agentID: identity.AgentID}].records[0].AgentSessionID = new("expired-session")

	cache.mu.Lock()
	cache.sessions["expired-session"] = time.Now().Add(-2 * idleTTL)
	cache.mu.Unlock()
	cache.EvictIdle()
	assert.Empty(t, cache.ActiveSessions())
	_, matched := cache.Match(identity, "expired-session", "session-tool", map[string]any{})
	assert.False(t, matched)
	_, matched = cache.Match(identity, "", "permanent-tool", map[string]any{})
	assert.True(t, matched)

	cache.mu.Lock()
	cache.pairs[pairKey{principal: identity.Principal, agentID: identity.AgentID}].lastSeen = time.Now().Add(-2 * idleTTL)
	cache.mu.Unlock()
	cache.EvictIdle()
	_, matched = cache.Match(identity, "", "permanent-tool", map[string]any{})
	assert.False(t, matched)
}
