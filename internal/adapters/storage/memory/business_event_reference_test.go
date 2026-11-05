package memory

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/require"
)

func TestMemoryEventReferencesCommitAndRollbackWithTheirOccurrences(t *testing.T) {
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	transactions := NewTransactionManager()
	repo := NewBusinessEventRepository(transactions, registry)
	contents, err := fs.ReadFile(eventschemas.Schemas, "examples.json")
	require.NoError(t, err)
	var examples []model.BusinessEvent
	require.NoError(t, json.Unmarshal(contents, &examples))
	ctx := context.Background()
	for _, scenario := range []struct {
		name         string
		copy, commit bool
	}{{"enabled copy commits its reference", true, true}, {"disabled copy retains no reference", false, true}, {"rollback removes event and reference", true, false}} {
		t.Run(scenario.name, func(t *testing.T) {
			event := examples[0]
			event.ID = id.NewBusinessEventID()
			owner, err := transactions.BeginTX(ctx)
			require.NoError(t, err)
			defer func() { require.NoError(t, transactions.Rollback(owner)) }()
			require.NoError(t, repo.Append(owner, &event, scenario.copy))
			key := model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
			if scenario.commit {
				require.NoError(t, transactions.Commit(owner))
			} else {
				require.NoError(t, transactions.Rollback(owner))
			}
			_, retained := repo.events[key]
			require.Equal(t, scenario.commit, retained)
			nextAttemptAt, pending := repo.pending[key]
			require.Equal(t, scenario.copy && scenario.commit, pending)
			if pending {
				require.False(t, nextAttemptAt.After(event.RecordedAt), "a newly committed reference must already be due")
			}
		})
	}
}
