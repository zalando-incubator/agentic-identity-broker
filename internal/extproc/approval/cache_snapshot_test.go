package approval

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotRecordsPreservesLocalState(t *testing.T) {
	for _, tc := range []struct {
		name     string
		incoming Record
		previous []Record
		consumed bool
		reserved bool
	}{
		{name: "new record", incoming: Record{ID: "new"}, previous: []Record{{ID: "old", Consumed: true, reserved: true}}},
		{name: "consumed locally", incoming: Record{ID: "same"}, previous: []Record{{ID: "same", Consumed: true}}, consumed: true},
		{name: "reserved locally", incoming: Record{ID: "same"}, previous: []Record{{ID: "same", reserved: true}}, reserved: true},
		{name: "consumed by broker", incoming: Record{ID: "same", Consumed: true}, previous: []Record{{ID: "same", reserved: true}}, consumed: true},
		{name: "empty IDs never match", incoming: Record{}, previous: []Record{{Consumed: true, reserved: true}}},
		{name: "first duplicate wins", incoming: Record{ID: "same"}, previous: []Record{{ID: "same", reserved: true}, {ID: "same", Consumed: true}}, reserved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := snapshotRecords([]Record{tc.incoming}, tc.previous)
			require.Len(t, snapshot, 1)
			assert.Equal(t, tc.incoming.ID, snapshot[0].ID)
			assert.Equal(t, tc.consumed, snapshot[0].Consumed)
			assert.Equal(t, tc.reserved, snapshot[0].reserved)
		})
	}
}

func TestCacheSnapshotPreservesOnceReservations(t *testing.T) {
	for _, targeted := range []bool{false, true} {
		t.Run(fmt.Sprintf("targeted=%t", targeted), func(t *testing.T) {
			identity := Identity{Principal: "alice", AgentID: "agent"}
			cache := NewCache(time.Minute, time.Minute)
			record := approvedRecord("once", "tool", map[string]string{}, "once", time.Now().UTC())
			pairs := []Pair{{Identity: identity, Approvals: []Record{record}}}
			cache.Replace(pairs, `"v1"`)
			_, matched := cache.Match(identity, "", "tool", nil)
			require.True(t, matched)

			replace := func(etag string) {
				if targeted {
					require.True(t, cache.ReplaceForPrincipal(identity.Principal, pairs, etag))
				} else {
					cache.Replace(pairs, etag)
				}
			}
			replace(`"v2"`)
			_, matched = cache.Match(identity, "", "tool", nil)
			assert.False(t, matched, "snapshot replacement must not release an in-flight reservation")

			cache.ConfirmConsumed(identity, record.ID)
			replace(`"v3"`)
			_, matched = cache.Match(identity, "", "tool", nil)
			assert.False(t, matched, "an unconsumed broker snapshot must not restore a locally consumed approval")
		})
	}
}

func TestSnapshotRecordsClonesBrokerRecords(t *testing.T) {
	now := time.Now().UTC()
	incoming := approvedRecord("approval", "tool", map[string]string{"path": "original"}, "once", now)
	incoming.AgentSessionID = new("session")
	snapshot := snapshotRecords([]Record{incoming}, nil)
	require.Len(t, snapshot, 1)
	snapshot[0].ParamsPattern["path"] = "changed"
	*snapshot[0].Persistence = "permanent"
	*snapshot[0].AgentSessionID = "other"
	*snapshot[0].ApprovedAt = now.Add(time.Hour)

	assert.Equal(t, "original", incoming.ParamsPattern["path"])
	assert.Equal(t, "once", *incoming.Persistence)
	assert.Equal(t, "session", *incoming.AgentSessionID)
	assert.Equal(t, now, *incoming.ApprovedAt)
	assert.Nil(t, snapshotRecords(nil, []Record{incoming}))
}

func BenchmarkSnapshotRecords(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000, 10000} {
		for _, overlap := range []bool{false, true} {
			b.Run(fmt.Sprintf("records=%d/overlap=%t", size, overlap), func(b *testing.B) {
				now := time.Now().UTC()
				records := make([]Record, size)
				previous := make([]Record, size)
				for i := range records {
					records[i] = approvedRecord(fmt.Sprintf("approval-%d", i), "tool", map[string]string{"path": "*"}, "once", now)
					previous[i] = records[i]
					previous[i].reserved = true
					if !overlap {
						previous[i].ID = fmt.Sprintf("removed-%d", i)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					snapshotRecords(records, previous)
				}
			})
		}
	}
}
