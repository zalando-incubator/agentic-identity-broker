package approval

import (
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Identity struct {
	Principal string
	AgentID   string
}

func (i Identity) Valid() bool {
	return i.Principal != "" && i.AgentID != ""
}

type Record struct {
	ID             string            `json:"id"`
	ToolName       string            `json:"tool_name"`
	ToolPattern    string            `json:"tool_pattern"`
	ParamsPattern  map[string]string `json:"params_pattern"`
	Status         string            `json:"status"`
	Persistence    *string           `json:"persistence"`
	Consumed       bool              `json:"consumed"`
	AgentSessionID *string           `json:"agent_session_id"`
	ApprovedAt     *time.Time        `json:"approved_at"`
	reserved       bool
}

type pairKey struct {
	principal string
	agentID   string
}

type pairEntry struct {
	records  []Record
	lastSeen time.Time
	syncedAt time.Time
}

type Cache struct {
	mu           sync.RWMutex
	pairs        map[pairKey]*pairEntry
	sessions     map[string]time.Time
	etag         string
	lastSync     time.Time
	createdAt    time.Time
	idleTTL      time.Duration
	maxStaleness time.Duration
}

func NewCache(idleTTL, maxStaleness time.Duration) *Cache {
	now := time.Now()
	return &Cache{
		pairs:        make(map[pairKey]*pairEntry),
		sessions:     make(map[string]time.Time),
		createdAt:    now,
		idleTTL:      idleTTL,
		maxStaleness: maxStaleness,
	}
}

func (c *Cache) freshLocked(entry *pairEntry, now time.Time) bool {
	freshest := c.lastSync
	if entry.syncedAt.After(freshest) {
		freshest = entry.syncedAt
	}
	return !freshest.IsZero() && now.Sub(freshest) <= c.maxStaleness
}

func (c *Cache) RecordSession(sessionID string) {
	if sessionID == "" {
		return
	}
	now := time.Now()
	c.mu.Lock()
	c.evictIdleLocked(now)
	c.sessions[sessionID] = now
	c.mu.Unlock()
}

func (c *Cache) ActiveSessions() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sessions := make([]string, 0, len(c.sessions))
	for id := range c.sessions {
		sessions = append(sessions, id)
	}
	sort.Strings(sessions)
	return sessions
}

func (c *Cache) ETag() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.etag
}

func (c *Cache) MarkSynced() {
	c.mu.Lock()
	c.lastSync = time.Now()
	c.mu.Unlock()
}

func (c *Cache) SyncAgeSeconds() int64 {
	c.mu.RLock()
	lastSync := c.lastSync
	createdAt := c.createdAt
	c.mu.RUnlock()
	if lastSync.IsZero() {
		lastSync = createdAt
	}
	return int64(time.Since(lastSync) / time.Second)
}

type Pair struct {
	Identity  Identity `json:"-"`
	Approvals []Record `json:"approvals"`
}

// Replace atomically installs the full unfiltered broker snapshot. Records or
// pairs omitted by the broker are removed, preventing revoked approvals from
// surviving a successful sync response.
func (c *Cache) Replace(pairs []Pair, etag string) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	replacement := make(map[pairKey]*pairEntry, len(pairs))
	for _, pair := range pairs {
		if !pair.Identity.Valid() {
			continue
		}
		key := pairKey{principal: pair.Identity.Principal, agentID: pair.Identity.AgentID}
		replacement[key] = snapshotEntry(pair.Approvals, c.pairs[key], now)
	}
	c.pairs = replacement
	c.etag = etag
	c.lastSync = now
}

// ReplaceForPrincipal atomically installs the snapshot returned from a
// principal-filtered authoritative read. It refreshes sync times only for
// returned entries and leaves every other principal's state, global last sync,
// and long-poll ETag unchanged.
func (c *Cache) ReplaceForPrincipal(principal string, pairs []Pair, etag string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if responseVersion, responseValid := parseSyncVersion(etag); responseValid {
		if cacheVersion, cacheValid := parseSyncVersion(c.etag); cacheValid && responseVersion < cacheVersion {
			return false
		}
	}
	if principal == "" {
		return true
	}

	now := time.Now()
	replacement := make(map[pairKey]*pairEntry, len(pairs))
	for _, pair := range pairs {
		if !pair.Identity.Valid() || pair.Identity.Principal != principal {
			continue
		}
		key := pairKey{principal: pair.Identity.Principal, agentID: pair.Identity.AgentID}
		replacement[key] = snapshotEntry(pair.Approvals, c.pairs[key], now)
	}
	for key := range c.pairs {
		if key.principal == principal {
			delete(c.pairs, key)
		}
	}
	for key, entry := range replacement {
		c.pairs[key] = entry
	}
	return true
}

func parseSyncVersion(etag string) (int64, bool) {
	etag = strings.TrimSpace(etag)
	if strings.HasPrefix(etag, "W/") {
		etag = strings.TrimSpace(strings.TrimPrefix(etag, "W/"))
	}
	etag = strings.Trim(strings.TrimSpace(etag), `"`)
	etag = strings.TrimPrefix(etag, "v")
	version, err := strconv.ParseInt(etag, 10, 64)
	return version, err == nil
}

func snapshotEntry(records []Record, current *pairEntry, now time.Time) *pairEntry {
	lastSeen := now
	var previous []Record
	if current != nil {
		lastSeen = current.lastSeen
		previous = current.records
	}
	if lastSeen.IsZero() {
		lastSeen = now
	}
	return &pairEntry{records: snapshotRecords(records, previous), lastSeen: lastSeen, syncedAt: now}
}

func snapshotRecords(records, previous []Record) []Record {
	if len(records) == 0 {
		return nil
	}

	snapshot := make([]Record, len(records))
	for i, record := range records {
		snapshot[i] = cloneRecord(record)
		if snapshot[i].Status == "approved" && snapshot[i].ApprovedAt == nil {
			slog.Warn("approval broker record missing approved_at", "approval_id", snapshot[i].ID)
		}
		for _, old := range previous {
			if snapshot[i].ID == "" || old.ID != snapshot[i].ID {
				continue
			}
			snapshot[i].Consumed = snapshot[i].Consumed || old.Consumed
			snapshot[i].reserved = !snapshot[i].Consumed && old.reserved
			break
		}
	}
	return snapshot
}

func cloneRecord(record Record) Record {
	clone := record
	clone.ParamsPattern = cloneStringMap(record.ParamsPattern)
	if record.Persistence != nil {
		persistence := *record.Persistence
		clone.Persistence = &persistence
	}
	if record.AgentSessionID != nil {
		agentSessionID := *record.AgentSessionID
		clone.AgentSessionID = &agentSessionID
	}
	if record.ApprovedAt != nil {
		approvedAt := *record.ApprovedAt
		clone.ApprovedAt = &approvedAt
	}
	return clone
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func (c *Cache) Match(identity Identity, sessionID, tool string, arguments map[string]any) (Record, bool) {
	if !identity.Valid() {
		return Record{}, false
	}

	key := pairKey{principal: identity.Principal, agentID: identity.AgentID}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictIdleLocked(now)

	entry := c.pairs[key]
	if entry == nil {
		return Record{}, false
	}
	if !c.freshLocked(entry, now) {
		entry.lastSeen = now
		return Record{}, false
	}
	entry.lastSeen = now

	candidates := make([]candidate, 0, len(entry.records))
	indices := make([]int, 0, len(entry.records))
	for i := range entry.records {
		record := &entry.records[i]
		if !record.matchable(sessionID) {
			continue
		}
		candidates = append(candidates, candidate{
			id:            record.ID,
			toolPattern:   record.ToolPattern,
			paramsPattern: record.ParamsPattern,
			decidedAt:     *record.ApprovedAt,
		})
		indices = append(indices, i)
	}

	best := selectBest(candidates, tool, arguments)
	if best < 0 {
		return Record{}, false
	}

	record := &entry.records[indices[best]]
	if *record.Persistence == "once" {
		record.reserved = true
	}
	return *record, true
}

func (r Record) matchable(sessionID string) bool {
	if r.ID == "" || r.ToolPattern == "" || r.Status != "approved" || r.Consumed || r.reserved || r.ApprovedAt == nil || r.Persistence == nil {
		return false
	}
	switch *r.Persistence {
	case "permanent", "once":
		return true
	case "session":
		return r.AgentSessionID != nil && *r.AgentSessionID == sessionID
	default:
		return false
	}
}

func (c *Cache) ConfirmConsumed(identity Identity, approvalID string) {
	c.setReservation(identity, approvalID, true)
}

func (c *Cache) Release(identity Identity, approvalID string) {
	c.setReservation(identity, approvalID, false)
}

func (c *Cache) setReservation(identity Identity, approvalID string, consumed bool) {
	if !identity.Valid() || approvalID == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.pairs[pairKey{principal: identity.Principal, agentID: identity.AgentID}]
	if entry == nil {
		return
	}
	for i := range entry.records {
		if entry.records[i].ID != approvalID {
			continue
		}
		entry.records[i].reserved = false
		if consumed {
			entry.records[i].Consumed = true
		}
		return
	}
}

func (c *Cache) EvictIdle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictIdleLocked(time.Now())
}

func (c *Cache) evictIdleLocked(now time.Time) {
	for sessionID, lastSeen := range c.sessions {
		if now.Sub(lastSeen) <= c.idleTTL {
			continue
		}
		delete(c.sessions, sessionID)
		for _, entry := range c.pairs {
			entry.records = filterSessionRecords(entry.records, sessionID)
		}
	}
	for key, entry := range c.pairs {
		if now.Sub(entry.lastSeen) > c.idleTTL {
			delete(c.pairs, key)
		}
	}
}

func filterSessionRecords(records []Record, sessionID string) []Record {
	filtered := records[:0]
	for _, record := range records {
		if record.Persistence != nil && *record.Persistence == "session" && record.AgentSessionID != nil && *record.AgentSessionID == sessionID {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}
