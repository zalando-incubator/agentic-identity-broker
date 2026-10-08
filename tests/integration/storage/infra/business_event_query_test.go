//go:build integration
// +build integration

package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/require"
)

func TestBusinessEventQuery_ExactSubjectNullSubjectTypeOutcomeAndHalfOpenTime(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	repo := adapter.BusinessEvents()
	first := id.NewPrincipal("query-first-subject")
	other := id.NewPrincipal("query-other-subject")
	start := time.Now().UTC().Truncate(time.Second)

	before := ledgerExample(t, "token-issued", first)
	before.OccurredAt = start.Add(-time.Microsecond)
	included := ledgerExample(t, "token-issued", first)
	included.OccurredAt = start
	denied := ledgerExample(t, "token-request-failed", first)
	denied.OccurredAt = start.Add(time.Second)
	wrongSubject := ledgerExample(t, "token-issued", other)
	wrongSubject.OccurredAt = start.Add(time.Second)
	atEnd := ledgerExample(t, "token-issued", first)
	atEnd.OccurredAt = start.Add(2 * time.Second)
	unattributed := ledgerExample(t, "agent-registered", "")
	unattributed.OccurredAt = start.Add(time.Second)
	for _, event := range []*model.BusinessEvent{before, included, denied, wrongSubject, atEnd, unattributed} {
		require.NoError(t, repo.Append(ctx, event, false))
	}

	query := model.BusinessEventQuery{
		Subject: model.BusinessEventSubject{Principal: first}, Start: start, End: start.Add(2 * time.Second),
	}
	found, err := repo.Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, found, 2, "time bounds are [start,end) and subjects are exact")
	require.Equal(t, included.ID, found[0].ID)
	require.Equal(t, denied.ID, found[1].ID)

	query.Type = included.Type
	query.Outcome = model.BusinessEventSuccess
	found, err = repo.Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, included.ID, found[0].ID)
	query.Subject = model.BusinessEventSubject{NoSubject: true}
	query.Type = ""
	query.Outcome = ""
	found, err = repo.Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, found, 1, "a no-subject selection is not an unscoped all-subject search")
	require.Equal(t, unattributed.ID, found[0].ID)

	var sqlIDs []string
	require.NoError(t, db.SelectContext(ctx, &sqlIDs, `SELECT id::text FROM public.business_events
		WHERE subject IS NULL AND occurred_at >= $1 AND occurred_at < $2
		ORDER BY occurred_at, id`, query.Start, query.End))
	require.Equal(t, []string{unattributed.ID.String()}, sqlIDs, "operational SQL must select the same null-subject occurrence")

	for _, invalid := range []model.BusinessEventQuery{
		{Start: start, End: start.Add(time.Second)},
		{Subject: model.BusinessEventSubject{Principal: first, NoSubject: true}, Start: start, End: start.Add(time.Second)},
		{Subject: model.BusinessEventSubject{Principal: first}, Start: start, End: start},
		{Subject: model.BusinessEventSubject{Principal: first}, Start: start.Add(time.Second), End: start},
		{Subject: model.BusinessEventSubject{Principal: first}, Start: start, End: start.Add(time.Second), Limit: 1001},
	} {
		_, err = repo.Query(ctx, invalid)
		require.Error(t, err, "invalid scope, interval and oversized limits must fail closed")
	}
}

func TestBusinessEventQuery_TiedOccurrenceOrderingAndStrictCursor(t *testing.T) {
	adapter, _ := openLedgerFoundation(t)
	ctx := context.Background()
	repo := adapter.BusinessEvents()
	subject := id.NewPrincipal("query-ties-subject")
	tie := time.Now().UTC().Truncate(time.Microsecond)
	originals := make([]*model.BusinessEvent, 5)
	for i := range originals {
		originals[i] = ledgerExample(t, "token-issued", subject)
		originals[i].OccurredAt = tie
		require.NoError(t, repo.Append(ctx, originals[i], false))
	}
	slices.SortFunc(originals, func(left, right *model.BusinessEvent) int {
		return bytes.Compare(left.ID[:], right.ID[:])
	})
	query := model.BusinessEventQuery{
		Subject: model.BusinessEventSubject{Principal: subject},
		Start:   tie.Add(-time.Microsecond), End: tie.Add(time.Microsecond), Limit: 2,
	}
	first, err := repo.Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, first, 2)
	require.Equal(t, originals[0].ID, first[0].ID)
	require.Equal(t, originals[1].ID, first[1].ID)
	query.After = &model.BusinessEventCursor{OccurredAt: first[1].OccurredAt, ID: first[1].ID}
	second, err := repo.Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, second, 2)
	require.Equal(t, originals[2].ID, second[0].ID)
	require.Equal(t, originals[3].ID, second[1].ID)
	query.After = &model.BusinessEventCursor{OccurredAt: second[1].OccurredAt, ID: second[1].ID}
	last, err := repo.Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, last, 1)
	require.Equal(t, originals[4].ID, last[0].ID)

	key := ledgerKey(originals[4])
	loaded, err := repo.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, originals[4].ID, loaded.ID)
	require.Equal(t, originals[4].RecordedAt, loaded.RecordedAt)
	_, err = repo.Get(ctx, model.BusinessEventKey{ID: key.ID, RecordedAt: key.RecordedAt.Add(time.Microsecond)})
	require.Error(t, err, "Get requires the complete recorded-at and ID key")
}

func TestBusinessEventQuery_DefaultAndMaximumPageSizes(t *testing.T) {
	adapter, _ := openLedgerFoundation(t)
	ctx := context.Background()
	repo := adapter.BusinessEvents()
	subject := id.NewPrincipal("query-pagination-subject")
	at := time.Now().UTC().Truncate(time.Microsecond)
	owner, err := adapter.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(owner) }()
	sample := ledgerExample(t, "token-issued", subject)
	for range 1001 {
		event := *sample
		event.ID = id.NewBusinessEventID()
		event.OccurredAt = at
		require.NoError(t, repo.Append(owner, &event, false))
	}
	require.NoError(t, adapter.Commit(owner))

	query := model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: subject},
		Start: at.Add(-time.Second), End: at.Add(time.Second)}
	page, err := repo.Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, page, 200, "unspecified limit defaults to 200")
	query.Limit = 1000
	full, err := repo.Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, full, 1000, "1000 is the maximum valid page size")
	for i := range page {
		require.Equal(t, page[i].ID, full[i].ID)
	}
	query.After = &model.BusinessEventCursor{OccurredAt: full[999].OccurredAt, ID: full[999].ID}
	rest, err := repo.Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, rest, 1)
	require.NotEqual(t, full[999].ID, rest[0].ID)
}

func TestBusinessEventQuery_OperationalAgentSetSurvivesBusinessRowDeletion(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	repo := adapter.BusinessEvents()
	subject := id.NewPrincipal("query-investigation-subject")
	other := id.NewPrincipal("query-investigation-other")
	firstAgent, secondAgent := id.NewAgentID(), id.NewAgentID()
	for _, agent := range []id.AgentID{firstAgent, secondAgent} {
		_, err := db.ExecContext(ctx, `INSERT INTO public.agents(id,client_id,display_name,description)
			VALUES ($1,$2,'Receiving agent','Can be removed without erasing ledger history')`, agent, "investigation-"+agent.String())
		require.NoError(t, err)
	}
	start := time.Now().UTC().Add(-time.Minute)
	for _, entry := range []struct {
		subject id.Principal
		agent   id.AgentID
	}{
		{subject, firstAgent}, {subject, firstAgent}, {subject, secondAgent}, {other, secondAgent},
	} {
		event := ledgerExample(t, "token-exchanged", entry.subject)
		event.AgentID = entry.agent
		require.NoError(t, repo.Append(ctx, event, false))
	}
	end := time.Now().UTC().Add(time.Minute)
	_, err := db.ExecContext(ctx, `DELETE FROM public.agents WHERE id IN ($1,$2)`, firstAgent, secondAgent)
	require.NoError(t, err)

	found, err := repo.Query(ctx, model.BusinessEventQuery{
		Subject: model.BusinessEventSubject{Principal: subject}, Type: model.BusinessEventTypePrefix + "token-exchanged",
		Outcome: model.BusinessEventSuccess, Start: start, End: end,
	})
	require.NoError(t, err)
	require.Len(t, found, 3, "a deleted referenced agent cannot remove retained event history")
	var operationalAgents []string
	require.NoError(t, db.SelectContext(ctx, &operationalAgents, `SELECT DISTINCT envelope->>'agent_id' FROM public.business_events
		WHERE subject=$1 AND type=$2 AND outcome=$3 AND occurred_at >= $4 AND occurred_at < $5
		ORDER BY 1`, subject.String(), model.BusinessEventTypePrefix+"token-exchanged", "success", start, end))
	expected := []string{firstAgent.String(), secondAgent.String()}
	slices.Sort(expected)
	require.Equal(t, expected, operationalAgents, "operational investigation must recover the exact receiving-agent set")
}

func TestBusinessEventQuery_SubMicrosecondBoundariesRemainExact(t *testing.T) {
	adapter, _ := openLedgerFoundation(t)
	ctx := context.Background()
	subject := id.NewPrincipal("query-microsecond-boundaries")
	base := time.Now().UTC().Truncate(time.Microsecond)
	events := make([]*model.BusinessEvent, 3)
	for i := range events {
		events[i] = ledgerExample(t, "token-issued", subject)
		events[i].OccurredAt = base.Add(time.Duration(i) * time.Microsecond)
		require.NoError(t, adapter.BusinessEvents().Append(ctx, events[i], false))
	}
	for _, scenario := range []struct {
		name       string
		start, end time.Time
		after      *model.BusinessEventCursor
		want       []id.BusinessEventID
	}{
		{"fractional lower bound", base.Add(500 * time.Nanosecond), base.Add(2 * time.Microsecond), nil, []id.BusinessEventID{events[1].ID}},
		{"fractional exclusive upper bound", base, base.Add(1500 * time.Nanosecond), nil, []id.BusinessEventID{events[0].ID, events[1].ID}},
		{"fractional cursor", base, base.Add(3 * time.Microsecond), &model.BusinessEventCursor{OccurredAt: base.Add(500 * time.Nanosecond), ID: id.MustParseBusinessEventID("01997100-0000-7000-8000-000000000001")}, []id.BusinessEventID{events[1].ID, events[2].ID}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			rows, err := adapter.BusinessEvents().Query(ctx, model.BusinessEventQuery{
				Subject: model.BusinessEventSubject{Principal: subject}, Start: scenario.start, End: scenario.end, After: scenario.after,
			})
			require.NoError(t, err)
			got := make([]id.BusinessEventID, len(rows))
			for i, event := range rows {
				got[i] = event.ID
			}
			require.Equal(t, scenario.want, got)
		})
	}
}

func TestBusinessEventQuery_GetPreservesExactRegisteredIntegers(t *testing.T) {
	const schema = `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"$id":"urn:agentic-identity-broker:events:v1:numeric-retention",
		"description":"A numeric event is retained.",
		"allOf":[
			{"$ref":"urn:agentic-identity-broker:events:v1:envelope"},
			{"properties":{
				"type":{"const":"agentic-identity-broker.numeric-retention"},
				"outcome":{"const":"success"},
				"data":{"type":"object","additionalProperties":false,
					"required":["scalar","values"],
					"properties":{
						"scalar":{"type":"integer"},
						"values":{"type":"array","items":{"type":"integer"}}
					}}
			}}
		]
	}`
	registry, err := ledger.NewRegistry(eventschemas.Schemas, fstest.MapFS{
		"numeric-retention.schema.json": &fstest.MapFile{Data: []byte(schema)},
	})
	require.NoError(t, err)
	adapter, db := openLedgerFoundation(t)
	adapter.ConfigureBusinessEventValidation(registry)
	ctx := context.Background()
	subject := id.NewPrincipal("retained-numeric-query-subject")
	event := ledgerExample(t, "token-issued", subject)
	event.Type = model.BusinessEventTypePrefix + "numeric-retention"
	event.ReasonUser, event.ReasonAdmin = "A numeric event is retained.", "A numeric event is retained."
	const large = int64(9007199254740993)
	event.Data = map[string]any{"scalar": large, "values": []int64{7, large}}
	_, err = registry.Validate(event)
	require.NoError(t, err, "append must start with a valid closed numeric fixture")
	require.NoError(t, adapter.BusinessEvents().Append(ctx, event, false))
	key := ledgerKey(event)

	var rawEnvelope []byte
	require.NoError(t, db.GetContext(ctx, &rawEnvelope,
		`SELECT envelope FROM public.business_events WHERE recorded_at=$1 AND id=$2`, key.RecordedAt, key.ID))
	var stored map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rawEnvelope, &stored))
	var storedID string
	require.NoError(t, json.Unmarshal(stored["id"], &storedID))
	require.Equal(t, event.ID.String(), storedID)
	requirePostgresExactNumericJSON(t, stored["data"])

	get, err := adapter.BusinessEvents().Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, key.ID, get.ID)
	require.Equal(t, key.RecordedAt, get.RecordedAt)
	_, err = registry.Validate(get)
	require.NoError(t, err, "decoded Get event must satisfy the registered schema")
	getData, err := json.Marshal(get.Data)
	require.NoError(t, err)
	requirePostgresExactNumericJSON(t, getData)

	rows, err := adapter.BusinessEvents().Query(ctx, model.BusinessEventQuery{
		Subject: model.BusinessEventSubject{Principal: subject},
		Start:   event.OccurredAt.Add(-time.Second), End: event.OccurredAt.Add(time.Second),
		Type: event.Type, Outcome: event.Outcome,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, key.ID, rows[0].ID)
	_, err = registry.Validate(rows[0])
	require.NoError(t, err, "decoded Query event must satisfy the registered schema")
	queryData, err := json.Marshal(rows[0].Data)
	require.NoError(t, err)
	requirePostgresExactNumericJSON(t, queryData)

	get.Data["unreviewed"] = "not in the schema"
	_, err = registry.Validate(get)
	require.Error(t, err, "lossless decoding must not relax the closed payload schema")
}

func requirePostgresExactNumericJSON(t *testing.T, encoded []byte) {
	t.Helper()
	var decoded struct {
		Scalar int64   `json:"scalar"`
		Values []int64 `json:"values"`
	}
	require.NoError(t, json.NewDecoder(bytes.NewReader(encoded)).Decode(&decoded))
	require.Equal(t, int64(9007199254740993), decoded.Scalar)
	require.Equal(t, []int64{7, 9007199254740993}, decoded.Values)
}
