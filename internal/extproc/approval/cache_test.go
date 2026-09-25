package approval

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval/toolpattern"
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

func TestCacheStopsMatchingAfterMaxStaleness(t *testing.T) {
	identity := Identity{Principal: "alice", AgentID: "agent"}
	cache := NewCache(time.Minute, 50*time.Millisecond)
	cache.Replace([]Pair{{
		Identity:  identity,
		Approvals: []Record{approvedRecord("permanent", "tool", map[string]string{}, "permanent", time.Now().UTC())},
	}}, `"v1"`)

	_, matched := cache.Match(identity, "", "tool", map[string]any{})
	require.True(t, matched)

	time.Sleep(80 * time.Millisecond)
	_, matched = cache.Match(identity, "", "tool", map[string]any{})
	assert.False(t, matched)

	cache.MarkSynced()
	_, matched = cache.Match(identity, "", "tool", map[string]any{})
	assert.True(t, matched)
}

func TestCacheMatchFiltersScopeAndMissingDecisionTime(t *testing.T) {
	now := time.Now().UTC()
	cache := NewCache(time.Minute, time.Minute)
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
			cache := NewCache(time.Minute, time.Minute)
			cache.Replace([]Pair{{
				Identity:  identity,
				Approvals: []Record{approvedRecord("vector", vector.ToolPattern, vector.ParamsPattern, "permanent", now)},
			}}, `"v1"`)

			_, matched := cache.Match(identity, "", vector.ToolName, vector.Arguments)
			assert.Equal(t, vector.Match, matched)
		})
	}
}

func TestCacheUsesPrecedenceVectors(t *testing.T) {
	identity := Identity{Principal: "alice", AgentID: "agent"}
	for _, vector := range precedenceVectors() {
		t.Run(vector.Name, func(t *testing.T) {
			records := make([]Record, 0, len(vector.Candidates))
			for _, candidate := range vector.Candidates {
				decidedAt, err := time.Parse(time.RFC3339, candidate.DecidedAt)
				require.NoError(t, err)
				records = append(records, approvedRecord(candidate.ID, candidate.ToolPattern, candidate.ParamsPattern, "permanent", decidedAt))
			}
			cache := NewCache(time.Minute, time.Minute)
			cache.Replace([]Pair{{Identity: identity, Approvals: records}}, `"v1"`)
			record, matched := cache.Match(identity, "", vector.ToolName, vector.Arguments)
			require.True(t, matched)
			assert.Equal(t, vector.WinnerID, record.ID)
		})
	}
}

func TestCacheReservesOnceApprovalExactlyOnce(t *testing.T) {
	now := time.Now().UTC()
	cache := NewCache(time.Minute, time.Minute)
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
	cache := NewCache(time.Minute, time.Minute)
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
	cache := NewCache(time.Minute, time.Minute)
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
	assert.True(t, cache.ReplaceForPrincipal("alice", nil, `"v3"`))
	_, matched = cache.Match(alice, "", "tool", map[string]any{})
	assert.False(t, matched, "a targeted empty snapshot must remove the principal's pairs")
	record, matched = cache.Match(bob, "", "tool", map[string]any{})
	require.True(t, matched)
	assert.Equal(t, "bob-current", record.ID)
	assert.Equal(t, `"v3"`, cache.ETag(), "targeted reads must not advance the global sync ETag")
}

func TestReplaceForPrincipalRejectsOlderVersion(t *testing.T) {
	now := time.Now().UTC()
	cache := NewCache(time.Minute, time.Minute)
	identity := Identity{Principal: "alice", AgentID: "agent"}
	cache.Replace([]Pair{{
		Identity:  identity,
		Approvals: []Record{approvedRecord("current", "tool", map[string]string{}, "permanent", now)},
	}}, `"v7"`)

	applied := cache.ReplaceForPrincipal("alice", []Pair{{
		Identity:  identity,
		Approvals: []Record{approvedRecord("outdated", "tool", map[string]string{}, "permanent", now)},
	}}, `"v5"`)
	assert.False(t, applied)
	record, matched := cache.Match(identity, "", "tool", map[string]any{})
	require.True(t, matched)
	assert.Equal(t, "current", record.ID)

	applied = cache.ReplaceForPrincipal("alice", []Pair{{
		Identity:  identity,
		Approvals: []Record{approvedRecord("latest", "tool", map[string]string{}, "permanent", now)},
	}}, `"v9"`)
	require.True(t, applied)
	record, matched = cache.Match(identity, "", "tool", map[string]any{})
	require.True(t, matched)
	assert.Equal(t, "latest", record.ID)
}

func TestParseSyncVersion(t *testing.T) {
	tests := []struct {
		name    string
		etag    string
		version int64
		valid   bool
	}{
		{name: "strong quoted version", etag: ` "v7" `, version: 7, valid: true},
		{name: "weak quoted version", etag: `W/"v8"`, version: 8, valid: true},
		{name: "unprefixed version", etag: `"9"`, version: 9, valid: true},
		{name: "invalid version", etag: `"vno"`, valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version, valid := parseSyncVersion(test.etag)
			assert.Equal(t, test.valid, valid)
			assert.Equal(t, test.version, version)
		})
	}
}

func TestCacheEvictsInactiveSessionsAndIdlePairs(t *testing.T) {
	idleTTL := time.Minute
	now := time.Now().UTC()
	cache := NewCache(idleTTL, time.Minute)
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

func TestCacheEvictsExpiredSessionBeforeRecordingRequest(t *testing.T) {
	const sessionID = "session-a"
	idleTTL := 10 * time.Millisecond
	cache := NewCache(idleTTL, time.Minute)
	identity := Identity{Principal: "alice", AgentID: "agent"}
	key := pairKey{principal: identity.Principal, agentID: identity.AgentID}

	cache.RecordSession(sessionID)
	cache.Replace([]Pair{{Identity: identity, Approvals: []Record{
		approvedRecord("session", "session-tool", map[string]string{}, "session", time.Now().UTC()),
	}}}, `"v1"`)
	cache.pairs[key].records[0].AgentSessionID = new(sessionID)
	_, matched := cache.Match(identity, sessionID, "session-tool", map[string]any{})
	require.True(t, matched)

	time.Sleep(2 * idleTTL)
	cache.mu.Lock()
	cache.pairs[key].lastSeen = time.Now()
	cache.mu.Unlock()

	cache.RecordSession(sessionID)
	_, matched = cache.Match(identity, sessionID, "session-tool", map[string]any{})
	assert.False(t, matched)
}
