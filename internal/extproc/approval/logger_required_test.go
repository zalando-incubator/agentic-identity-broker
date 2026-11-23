package approval

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewCacheRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "approval.NewCache: logger must not be nil", func() {
		NewCache(time.Minute, time.Minute, nil)
	})
}

func TestNewGateRequiresLogger(t *testing.T) {
	cache := NewCache(time.Minute, time.Minute, slog.New(slog.DiscardHandler))
	assert.PanicsWithValue(t, "approval.NewGate: logger must not be nil", func() {
		NewGate(cache, nil, nil)
	})
}

func TestCacheLogsMissingApprovedAtToInjectedLogger(t *testing.T) {
	var logs bytes.Buffer
	cache := NewCache(time.Minute, time.Minute, slog.New(slog.NewJSONHandler(&logs, nil)))

	cache.Replace([]Pair{{
		Identity:  Identity{Principal: "alice", AgentID: "agent"},
		Approvals: []Record{{ID: "approval-1", ToolPattern: "tool", Status: "approved"}},
	}}, `"1"`)

	assert.Contains(t, logs.String(), `"msg":"approval broker record missing approved_at"`)
	assert.Contains(t, logs.String(), `"approval_id":"approval-1"`)
}
