package model

import (
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBusinessEventValidate(t *testing.T) {
	valid := BusinessEvent{ID: id.NewBusinessEventID(), Event: "consent.grant.revoked", OccurredAt: time.Now()}
	require.NoError(t, valid.Validate())
	for _, tc := range []struct {
		name       string
		invalidate func(*BusinessEvent)
	}{
		{"zero ID", func(e *BusinessEvent) { e.ID = id.BusinessEventID{} }},
		{"empty event", func(e *BusinessEvent) { e.Event = "" }},
		{"zero occurrence time", func(e *BusinessEvent) { e.OccurredAt = time.Time{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := valid
			tc.invalidate(&event)
			require.Error(t, event.Validate())
		})
	}
}

func TestBusinessEventCopy(t *testing.T) {
	principal, actor := "owner", "operator"
	event := BusinessEvent{
		ID: id.NewBusinessEventID(), Event: "consent.grant.revoked", OccurredAt: time.Now(),
		Principal: &principal, Actor: &actor,
		Attributes: map[string]any{"nested": map[string]any{"items": []any{map[string]any{"value": "original"}}}, "names": []string{"original"}},
	}
	copied := event.Copy()
	require.NotNil(t, copied)
	require.Equal(t, &event, copied)
	*copied.Principal = "changed"
	*copied.Actor = "changed"
	copied.Attributes["nested"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"] = "changed"
	copied.Attributes["names"].([]string)[0] = "changed"
	assert.Equal(t, "owner", principal)
	assert.Equal(t, "operator", actor)
	assert.Equal(t, "original", event.Attributes["nested"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"])
	assert.Equal(t, "original", event.Attributes["names"].([]string)[0])
	assert.Nil(t, (&BusinessEvent{}).Copy().Principal)
	assert.Nil(t, (&BusinessEvent{}).Copy().Actor)
}
