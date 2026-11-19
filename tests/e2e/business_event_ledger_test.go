package e2e_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("Business event ledger", Label("docker"), func() {
	var (
		postgres   *bootstrap.PostgresFixture
		db         *sql.DB
		store      *storageadapter.Adapter
		server     *bootstrap.TestServer
		cfg        *ports.Config
		logger     *slog.Logger
		logs       *bootstrap.BufferedLogCapture
		reader     *sdkmetric.ManualReader
		client     *http.Client
		agent      *storagedomain.Agent
		grantID    id.GrantID
		validUntil *time.Time
		failing    bool
	)

	revoke := func(status int) string {
		resp, err := server.DirectRequest(http.MethodDelete, "/api/consent/agents/"+agent.ID.String()+"/grants",
			fixtures.DefaultPrincipal().String(), nil, nil)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		defer func() { ExpectWithOffset(1, resp.Body.Close()).To(Succeed()) }()
		_, err = io.Copy(io.Discard, resp.Body)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, resp.StatusCode).To(Equal(status))
		traceID := requestSecurityTraceID(resp)
		ExpectWithOffset(1, traceID).To(MatchRegexp(`^[0-9a-f]{32}$`))
		return traceID
	}

	BeforeEach(func() {
		postgres, db, store, server = nil, nil, nil, nil
		failing = false
		validUntil = nil
		Expect(bootstrap.CanAccessContainerRuntime()).To(Succeed(), "ledger acceptance journeys require Docker or Podman")
		var err error
		postgres, err = bootstrap.NewPostgresFixture(context.Background())
		Expect(err).NotTo(HaveOccurred())
		db, err = sql.Open("pgx", postgres.ConnectionURL)
		Expect(err).NotTo(HaveOccurred())
		cfg = fixtures.BusinessEventConfig(postgres.ConnectionURL, true)
		logger, logs = bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
		store, err = storageadapter.NewAdapter(&cfg.Storage)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedDefaultConsentData(context.Background(), store, id.NewPrincipal(fixtures.DefaultPrincipal().String()))).To(Succeed())
		agent = fixtures.ValidAgent()
		Expect(store.Agents().Create(context.Background(), agent)).To(Succeed())
		client = &http.Client{Timeout: 10 * time.Second}

		previous := otel.GetMeterProvider()
		reader = sdkmetric.NewManualReader()
		provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		otel.SetMeterProvider(provider)
		DeferCleanup(func() {
			otel.SetMeterProvider(previous)
			Expect(provider.Shutdown(context.Background())).To(Succeed())
		})
	})

	JustBeforeEach(func() {
		application, err := bootstrap.NewServerFactory(cfg, logger).BuildApp(store)
		Expect(err).NotTo(HaveOccurred())
		server, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
		grantID = helpers.CreateBusinessEventGrant(client, server, agent.ID, validUntil)
		Expect(helpers.CommittedBusinessEvents(db)).To(BeEmpty(), "ordinary grant creation is not a business event")
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if client != nil {
			client.CloseIdleConnections()
		}
		if failing && db != nil {
			_, err := db.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS reject_business_event ON business_events")
			Expect(err).NotTo(HaveOccurred())
			_, err = db.ExecContext(context.Background(), "DROP FUNCTION IF EXISTS reject_business_event_insert()")
			Expect(err).NotTo(HaveOccurred())
		}
		if store != nil {
			Expect(store.Close(context.Background())).To(Succeed())
		}
		if db != nil {
			Expect(db.Close()).To(Succeed())
		}
		if postgres != nil {
			Expect(postgres.Close(context.Background())).To(Succeed())
		}
	})

	Context("when the ledger is enabled and Alice has created a grant", func() {
		// Scenario US1.1 from specs/20261118-125900-business-event-ledger/spec.md (User Story 1)
		It("stores exactly one consent.grant.revoked event with request and resource identities", func() {
			traceID := revoke(http.StatusNoContent)
			events := helpers.CommittedBusinessEvents(db)
			Expect(events).To(HaveLen(1))
			Expect(events).To(matchers.HaveBusinessEvent("consent.grant.revoked", fixtures.DefaultPrincipal().String()))
			Expect(events[0].ID).NotTo(BeZero())
			Expect(events[0].OccurredAt).NotTo(BeZero())
			Expect(events[0].RecordedAt).NotTo(BeZero())
			Expect(events[0].Actor).To(HaveValue(Equal(fixtures.DefaultPrincipal().String())))
			Expect(events[0].TraceID).To(Equal(traceID))
			Expect(events[0].Attributes).To(HaveKeyWithValue("agent_id", agent.ID.String()))
			Expect(events[0].Attributes).To(HaveKeyWithValue("grant_id", grantID.String()))
		})
	})

	Context("when grant deletion commits but refresh cleanup fails", func() {
		var snapshot *storagedomain.UserGrant

		BeforeEach(func() {
			expiresAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond)
			validUntil = &expiresAt
		})

		JustBeforeEach(func() {
			var err error
			snapshot, err = store.UserGrants().FindByPrincipalAndAgent(context.Background(), id.NewPrincipal(fixtures.DefaultPrincipal().String()), agent.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(store.RefreshTokenSessions().Create(context.Background(), &storagedomain.RefreshTokenSession{
				Signature: "cleanup-failure-signature", RequestID: "cleanup-failure-request",
				AgentID: agent.ID, ClientID: *agent.ClientID, Principal: snapshot.Principal, GrantID: &grantID,
				CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour).UTC(),
			})).To(Succeed())
			_, err = db.ExecContext(context.Background(), `
				CREATE FUNCTION reject_refresh_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'refresh cleanup rejected'; END; $$`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.ExecContext(context.Background(), `
				CREATE TRIGGER reject_refresh_cleanup BEFORE UPDATE ON refresh_token_sessions
				FOR EACH ROW EXECUTE FUNCTION reject_refresh_cleanup()`)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			_, err := db.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS reject_refresh_cleanup ON refresh_token_sessions")
			Expect(err).NotTo(HaveOccurred())
			_, err = db.ExecContext(context.Background(), "DROP FUNCTION IF EXISTS reject_refresh_cleanup()")
			Expect(err).NotTo(HaveOccurred())
		})

		// Scenario US1.5 from specs/20261118-125900-business-event-ledger/spec.md (User Story 1)
		It("stores the deleted grant snapshot when refresh cleanup returns an HTTP error", func() {
			traceID := revoke(http.StatusInternalServerError)
			events := helpers.CommittedBusinessEvents(db)
			Expect(events).To(HaveLen(1))
			Expect(events).To(matchers.HaveBusinessEvent("consent.grant.revoked", fixtures.DefaultPrincipal().String()))
			Expect(events[0].Actor).To(HaveValue(Equal(fixtures.DefaultPrincipal().String())))
			Expect(events[0].TraceID).To(Equal(traceID))
			Expect(events[0].Attributes).To(HaveKeyWithValue("refresh_cleanup", "failed"))
			Expect(events[0].Attributes).To(HaveKeyWithValue("agent_id", agent.ID.String()))
			Expect(events[0].Attributes).To(HaveKeyWithValue("grant_id", grantID.String()))
			Expect(events[0].Attributes).To(HaveKeyWithValue("valid_until", validUntil.UTC().Format(time.RFC3339Nano)))
			Expect(events[0].Attributes).To(HaveKeyWithValue("updated_at", snapshot.UpdatedAt.UTC().Format(time.RFC3339Nano)))
			Expect(events[0].Attributes).To(HaveKeyWithValue("granted_permission_sets", []any{map[string]any{
				"permission_set_id":    fixtures.PlaceholderPermissionSetID.String(),
				"included_service_ids": []any{fixtures.PlaceholderServiceID.String()},
			}}))
			Expect(events[0].Attributes).To(HaveKeyWithValue("revoked_at", MatchRegexp(`Z$`)))
			var grants, pendingRefresh int
			Expect(db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM user_grants WHERE id = $1", grantID).Scan(&grants)).To(Succeed())
			Expect(grants).To(BeZero())
			Expect(db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM refresh_token_sessions WHERE signature = $1 AND used_at IS NULL", "cleanup-failure-signature").Scan(&pendingRefresh)).To(Succeed())
			Expect(pendingRefresh).To(Equal(1), "the failed cleanup must leave a session row, not undo grant deletion")
			revoke(http.StatusNotFound)
			Expect(db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM user_grants WHERE id = $1", grantID).Scan(&grants)).To(Succeed())
			Expect(grants).To(BeZero())
			Expect(db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM refresh_token_sessions WHERE signature = $1 AND used_at IS NULL", "cleanup-failure-signature").Scan(&pendingRefresh)).To(Succeed())
			Expect(pendingRefresh).To(Equal(1), "the retry must leave the same refresh session uncleaned")
			Expect(helpers.CommittedBusinessEvents(db)).To(Equal(events))
		})
	})

	Context("when the ledger is enabled and Alice has already revoked her grant", func() {
		var original []model.BusinessEvent
		JustBeforeEach(func() {
			revoke(http.StatusNoContent)
			original = helpers.CommittedBusinessEvents(db)
			Expect(original).To(HaveLen(1))
		})

		// Scenario US1.2 from specs/20261118-125900-business-event-ledger/spec.md (User Story 1)
		It("keeps exactly one event when a second revocation returns 404", func() {
			revoke(http.StatusNotFound)
			Expect(helpers.CommittedBusinessEvents(db)).To(Equal(original))
			Expect(proactiveEvents(logs, "consent.grant.revoked")).To(HaveLen(1))
		})
	})

	Context("when the ledger is disabled", func() {
		BeforeEach(func() { cfg.BusinessEvents.Enabled = false })

		// Scenario US1.3 from specs/20261118-125900-business-event-ledger/spec.md (User Story 1)
		It("stores no event and still logs the revocation when the ledger is disabled", func() {
			traceID := revoke(http.StatusNoContent)
			Expect(helpers.CommittedBusinessEvents(db)).To(BeEmpty())
			records := proactiveEvents(logs, "consent.grant.revoked")
			Expect(records).To(HaveLen(1))
			Expect(records[0]).To(HaveKeyWithValue("trace_id", traceID))
			Expect(records[0]).To(HaveKeyWithValue("grant_id", grantID.String()))
		})
	})

	Context("when the ledger is enabled and PostgreSQL rejects event inserts", func() {
		BeforeEach(func() {
			failing = true
			_, err := db.ExecContext(context.Background(), `
				CREATE FUNCTION reject_business_event_insert() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'ledger insert rejected'; END; $$`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.ExecContext(context.Background(), `
				CREATE TRIGGER reject_business_event BEFORE INSERT ON business_events
				FOR EACH ROW EXECUTE FUNCTION reject_business_event_insert()`)
			Expect(err).NotTo(HaveOccurred())
		})

		// Scenario US1.4 from specs/20261118-125900-business-event-ledger/spec.md (User Story 1)
		It("returns 204 and reports exactly one failed write without losing the audit record", func() {
			before := helpers.BusinessEventCounterValues(reader)["business_events/business_events.write_failures"]
			traceID := revoke(http.StatusNoContent)
			Expect(helpers.CommittedBusinessEvents(db)).To(BeEmpty())
			Expect(helpers.BusinessEventCounterValues(reader)["business_events/business_events.write_failures"] - before).To(Equal(float64(1)))
			var metrics metricdata.ResourceMetrics
			Expect(reader.Collect(context.Background(), &metrics)).To(Succeed())
			var unknownFailures int64
			var failedDurations uint64
			for _, scope := range metrics.ScopeMetrics {
				if scope.Scope.Name != "business_events" {
					continue
				}
				for _, metric := range scope.Metrics {
					switch metric.Name {
					case "business_events.write_failures":
						data, ok := metric.Data.(metricdata.Sum[int64])
						Expect(ok).To(BeTrue())
						for _, point := range data.DataPoints {
							kind, _ := point.Attributes.Value("error_kind")
							if kind.AsString() == "unknown" {
								unknownFailures += point.Value
							}
						}
					case "business_events.write.duration":
						data, ok := metric.Data.(metricdata.Histogram[float64])
						Expect(ok).To(BeTrue())
						for _, point := range data.DataPoints {
							outcome, _ := point.Attributes.Value("outcome")
							if outcome.AsString() == "failed" {
								failedDurations += point.Count
							}
						}
					}
				}
			}
			Expect(unknownFailures).To(Equal(int64(1)), "the rejected insert must use the unknown failure series")
			Expect(failedDurations).To(Equal(uint64(1)), "the rejected insert must record one failed duration")
			audit := proactiveEvents(logs, "consent.grant.revoked")
			failed := proactiveEvents(logs, "business_event.ledger.write_failed")
			Expect(audit).To(HaveLen(1))
			Expect(failed).To(HaveLen(1))
			Expect(audit[0]).To(HaveKeyWithValue("trace_id", traceID))
			Expect(failed[0]).To(HaveKeyWithValue("trace_id", traceID))
			Expect(failed[0]).To(HaveKeyWithValue("level", "ERROR"))
			Expect(failed[0]).To(HaveKeyWithValue("failed_event", "consent.grant.revoked"))
			Expect(failed[0]).To(HaveKeyWithValue("error_kind", "unknown"))
		})
	})

	Context("when one replica runs retention with an expired and a fresh event", func() {
		BeforeEach(func() {
			cfg.BusinessEvents.Retention.Period = 24 * time.Hour
			cfg.BusinessEvents.Retention.Interval = time.Second
		})

		// Scenario US2.1 from specs/20261118-125900-business-event-ledger/spec.md (User Story 2)
		It("deletes only the expired event without a deletion log or counter", func() {
			revoke(http.StatusNoContent)
			first := helpers.CommittedBusinessEvents(db)
			Expect(first).To(HaveLen(1))
			secondGrant := helpers.CreateBusinessEventGrant(client, server, agent.ID, nil)
			Expect(secondGrant).NotTo(Equal(grantID))
			revoke(http.StatusNoContent)
			both := helpers.CommittedBusinessEvents(db)
			Expect(both).To(HaveLen(2), "both real revocations must be committed before either event is aged")
			Expect(both[0].ID).To(Equal(first[0].ID))
			Expect(both[1].Attributes).To(HaveKeyWithValue("grant_id", secondGrant.String()))
			counters := helpers.BusinessEventCounterValues(reader)
			logs.Reset()
			result, err := db.ExecContext(context.Background(), `
				UPDATE business_events SET recorded_at = now() - interval '2 days' WHERE id = $1`, first[0].ID)
			Expect(err).NotTo(HaveOccurred())
			changed, err := result.RowsAffected()
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(Equal(int64(1)))
			Eventually(func() []model.BusinessEvent { return helpers.CommittedBusinessEvents(db) }, 5*time.Second, 50*time.Millisecond).
				Should(Equal(both[1:]))
			records, err := logs.Records()
			Expect(err).NotTo(HaveOccurred())
			Expect(records).To(BeEmpty(), "successful retention must not log deletion")
			Expect(helpers.BusinessEventCounterValues(reader)).To(Equal(counters), "successful retention must not increment a counter")
		})
	})
})
