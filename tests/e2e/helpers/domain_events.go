package helpers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/onsi/gomega"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

// CommittedDomainEvents reads committed ledger rows in recording order without a production reader port.
func CommittedDomainEvents(db *sql.DB) []model.DomainEvent {
	rows, err := db.QueryContext(context.Background(), `
		SELECT id, event, occurred_at, recorded_at, principal, actor, trace_id, attributes
		FROM domain_events ORDER BY recorded_at, id`)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	defer func() { gomega.ExpectWithOffset(1, rows.Close()).To(gomega.Succeed()) }()
	events := make([]model.DomainEvent, 0)
	for rows.Next() {
		var event model.DomainEvent
		var traceID sql.NullString
		var attributes []byte
		gomega.ExpectWithOffset(1, rows.Scan(&event.ID, &event.Event, &event.OccurredAt, &event.RecordedAt,
			&event.Principal, &event.Actor, &traceID, &attributes)).To(gomega.Succeed())
		event.TraceID = traceID.String
		gomega.ExpectWithOffset(1, json.Unmarshal(attributes, &event.Attributes)).To(gomega.Succeed())
		events = append(events, event)
	}
	gomega.ExpectWithOffset(1, rows.Err()).NotTo(gomega.HaveOccurred())
	return events
}

// DomainEventCounterValues totals monotonic counters by meter scope and instrument name.
func DomainEventCounterValues(reader *sdkmetric.ManualReader) map[string]float64 {
	var metrics metricdata.ResourceMetrics
	gomega.ExpectWithOffset(1, reader.Collect(context.Background(), &metrics)).To(gomega.Succeed())
	values := make(map[string]float64)
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			key := scope.Scope.Name + "/" + metric.Name
			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				if data.IsMonotonic {
					for _, point := range data.DataPoints {
						values[key] += float64(point.Value)
					}
				}
			case metricdata.Sum[float64]:
				if data.IsMonotonic {
					for _, point := range data.DataPoints {
						values[key] += point.Value
					}
				}
			}
		}
	}
	return values
}

// CreateDomainEventGrant creates a grant through the consent API and returns its nonzero ID.
func CreateDomainEventGrant(client *http.Client, server *bootstrap.TestServer, agentID id.AgentID, validUntil *time.Time) id.GrantID {
	payload, err := json.Marshal(map[string]any{
		"granted_permission_sets": map[string][]string{
			fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()},
		},
		"valid_until": validUntil,
	})
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	req, err := http.NewRequest(http.MethodPost, server.BaseURL()+"/api/consent/agents/"+agentID.String()+"/grants",
		bytes.NewReader(payload))
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Remote-User", fixtures.DefaultPrincipal().String())
	resp, err := client.Do(req)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	defer func() { gomega.ExpectWithOffset(1, resp.Body.Close()).To(gomega.Succeed()) }()
	body, err := io.ReadAll(resp.Body)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	gomega.ExpectWithOffset(1, resp.StatusCode).To(gomega.Equal(http.StatusCreated), "%s", body)
	var grant struct {
		Data struct {
			ID id.GrantID `json:"id"`
		} `json:"data"`
	}
	gomega.ExpectWithOffset(1, json.Unmarshal(body, &grant)).To(gomega.Succeed())
	gomega.ExpectWithOffset(1, grant.Data.ID).NotTo(gomega.BeZero())
	return grant.Data.ID
}
