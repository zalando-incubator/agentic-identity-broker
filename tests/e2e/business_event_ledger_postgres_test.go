//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLedgerChild(t *testing.T) {
	if os.Getenv(bootstrap.LedgerChildEnvironment) != "1" {
		return
	}
	if err := bootstrap.RunLedgerChild(os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

var _ = Describe("Business Event Ledger PostgreSQL", Label("business-event-ledger"), func() {
	Context("committed history across process boundaries", func() {
		// US1-AS2 from specs/048-business-event-ledger/spec.md.
		It("retains grant, session and approval transitions after post-commit crashes", func() {
			for _, eventName := range []string{"grant-revoked", "session-terminated", "approval-approved"} {
				journey := newLedgerJourney(bootstrap.LedgerPostgres, eventName, nil)
				method, path, body := http.MethodDelete, "/api/consent/agents/"+journey.data.Agent.ID.String()+"/grants", ""
				switch eventName {
				case "grant-revoked":
					journey.seedGrant(false)
				case "session-terminated":
					journey.establishSession()
					path = "/api/third-party/" + journey.data.Service.ID.String() + "/session"
				case "approval-approved":
					created := createPendingApproval(journey.h.EndUser, journey.auth, journey.data.Principal.String(), "ledger_crash", map[string]any{})
					method, path, body = http.MethodPost, "/api/approvals/"+created.Data.ID+"/approve", `{"persistence":"once"}`
				}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				DeferCleanup(cancel)
				executable, err := os.Executable()
				Expect(err).NotTo(HaveOccurred())
				child, err := bootstrap.StartLedgerChild(ctx, executable, journey.h.App.Config)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(child.Kill)
				ready, err := child.Await(ctx, "ready")
				Expect(err).NotTo(HaveOccurred())
				Expect(child.Send("hold_commit")).To(Succeed())
				_, err = child.Await(ctx, "armed")
				Expect(err).NotTo(HaveOccurred())
				request, err := http.NewRequestWithContext(ctx, method, ready.EndUserURL+path, strings.NewReader(body))
				Expect(err).NotTo(HaveOccurred())
				request.Header.Set("X-Remote-User", journey.data.Principal.String())
				request.Header.Set("Content-Type", "application/json")
				type requestResult struct {
					status int
					err    error
				}
				responses := make(chan requestResult, 1)
				go func() {
					response, err := helpers.HTTPClient().Do(request)
					result := requestResult{err: err}
					if err == nil {
						result.status = response.StatusCode
						result.err = response.Body.Close()
					}
					responses <- result
				}()
				query := model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: journey.data.Principal}, Type: "agentic-identity-broker." + eventName, Start: journey.start, End: time.Now().UTC().Add(time.Hour)}
				var observed []*model.BusinessEvent
				Eventually(func(g Gomega) {
					var err error
					observed, err = helpers.QueryLedgerEvents(ctx, journey.h.ReaderDB, query)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(observed).To(HaveLen(1))
				}, 5*time.Second, 20*time.Millisecond).Should(Succeed())
				_, err = child.Await(ctx, "committed")
				Expect(err).NotTo(HaveOccurred())
				Expect(responses).NotTo(Receive())
				Expect(child.CloseInput()).To(Succeed())
				Consistently(responses, 100*time.Millisecond, 5*time.Millisecond).ShouldNot(Receive(), "stdin EOF must not release a held post-commit response before the process is killed")
				originalID := observed[0].ID
				Expect(child.Kill()).To(Succeed())
				var result requestResult
				Eventually(responses, 5*time.Second).Should(Receive(&result))
				Expect(result.err).To(HaveOccurred(), "a killed child must not send an HTTP response after its commit")
				Expect(result.status).To(BeZero())
				restarted, err := bootstrap.StartLedgerChild(ctx, executable, journey.h.App.Config)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(restarted.Kill)
				ready, err = restarted.Await(ctx, "ready")
				Expect(err).NotTo(HaveOccurred())
				observed, err = helpers.QueryLedgerEvents(ctx, journey.h.ReaderDB, query)
				Expect(err).NotTo(HaveOccurred())
				Expect(observed).To(HaveLen(1))
				Expect(observed[0].ID).To(Equal(originalID))
				request, err = http.NewRequestWithContext(ctx, http.MethodGet, ready.EndUserURL+strings.TrimSuffix(path, "/approve"), nil)
				Expect(err).NotTo(HaveOccurred())
				request.Header.Set("X-Remote-User", journey.data.Principal.String())
				response, err := helpers.HTTPClient().Do(request)
				Expect(err).NotTo(HaveOccurred())
				if eventName == "approval-approved" {
					Expect(response.StatusCode).To(Equal(http.StatusOK))
					var approval helpers.ApprovalDetailResponse
					Expect(json.NewDecoder(response.Body).Decode(&approval)).To(Succeed())
					Expect(approval.Data.Status).To(Equal("approved"))
				} else if eventName == "grant-revoked" {
					Expect(response.StatusCode).To(Equal(http.StatusOK))
					var grants struct {
						Data []json.RawMessage `json:"data"`
					}
					Expect(json.NewDecoder(response.Body).Decode(&grants)).To(Succeed())
					Expect(grants.Data).To(BeEmpty())
				} else {
					Expect(response.StatusCode).To(Equal(http.StatusNotFound))
				}
				Expect(response.Body.Close()).To(Succeed())
				Expect(restarted.Kill()).To(Succeed())
				Expect(journey.h.Close()).To(Succeed())
			}
		})

		// US1-AS7 from specs/048-business-event-ledger/spec.md.
		It("matches raw PostgreSQL catalogue contents and scoped results in process-local memory", func() {
			for _, eventType := range fixtures.LedgerCatalogueTypes() {
				name := strings.TrimPrefix(eventType, "agentic-identity-broker.")
				var reference []map[string]any
				for _, backend := range []bootstrap.LedgerBackend{bootstrap.LedgerPostgres, bootstrap.LedgerMemory} {
					journey := newLedgerJourney(backend, name, nil)
					expected := journey.action(name)
					known := make(map[id.BusinessEventID]bool, len(journey.actionBefore)+len(journey.noSubjectBefore))
					for _, before := range journey.actionBefore {
						known[before.ID] = true
					}
					for _, before := range journey.noSubjectBefore {
						known[before.ID] = true
					}
					envelopes := make([]map[string]any, 0)
					var target []*model.BusinessEvent
					for _, subject := range []model.BusinessEventSubject{{Principal: journey.data.Principal}, {NoSubject: true}} {
						query := model.BusinessEventQuery{Subject: subject, Start: journey.start.Add(-24 * time.Hour), End: time.Now().UTC().Add(time.Hour), Limit: 1000}
						var events []*model.BusinessEvent
						var err error
						if backend == bootstrap.LedgerPostgres {
							events, err = helpers.QueryLedgerEvents(context.Background(), journey.h.ReaderDB, query)
						} else {
							events, err = journey.h.App.LedgerService.Query(context.Background(), query)
						}
						Expect(err).NotTo(HaveOccurred())
						for _, event := range events {
							if event.Type == expected.Type && subject.NoSubject == expected.Subject.NoSubject && !known[event.ID] {
								target = append(target, event)
							}
							var wire map[string]any
							if backend == bootstrap.LedgerPostgres {
								wire, err = helpers.QueryLedgerRawEnvelope(context.Background(), journey.h.ReaderDB, event.ID)
							} else {
								var encoded []byte
								encoded, err = json.Marshal(event)
								if err == nil {
									err = json.Unmarshal(encoded, &wire)
								}
							}
							Expect(err).NotTo(HaveOccurred())
							normalized, err := helpers.NormalizeLedgerRawEnvelope(wire)
							Expect(err).NotTo(HaveOccurred())
							envelopes = append(envelopes, normalized)
						}
					}
					Expect(target).To(HaveLen(1), "one target fact in the complete subject/no-subject result")
					Expect(target[0]).To(matchers.HaveBusinessEventEnvelope(expected))
					if reference == nil {
						reference = envelopes
					} else {
						Expect(envelopes).To(ConsistOf(reference), "compare the complete multiset, including companion and no-subject facts")
					}
					Expect(journey.h.Close()).To(Succeed())
				}
			}
		})
	})

	Context("retention and exact-subject erasure", func() {
		// US3-AS2 from specs/048-business-event-ledger/spec.md.
		It("drops only wholly expired partition pairs at default and changed retention", func() {
			for _, retention := range []time.Duration{2160 * time.Hour, 36 * time.Hour, 2 * time.Hour} {
				journey := newConfiguredLedgerJourney(bootstrap.LedgerPostgres, "grant-created", nil, func(config *ports.Config) {
					config.BusinessEvents.Retention = retention
				}, nil)
				expected := journey.action("grant-created")
				events := journey.query("grant-created", *expected.Subject)
				Expect(events).To(HaveLen(1))
				live := events[0]
				var now time.Time
				Expect(journey.h.OwnerDB.Get(&now, "SELECT clock_timestamp()")).To(Succeed())
				cutoff := now.UTC().Add(-retention)
				mixedStart := cutoff.Truncate(6 * time.Hour)
				mixedUpper := mixedStart.Add(6 * time.Hour)
				Expect(mixedStart.Before(cutoff)).To(BeTrue())
				fullyExpired, mixedOld, mixedYoung, oldOccurrence := *live, *live, *live, *live
				fullyExpired.ID, mixedOld.ID = id.NewBusinessEventID(), id.NewBusinessEventID()
				mixedYoung.ID, oldOccurrence.ID = id.NewBusinessEventID(), id.NewBusinessEventID()
				fullyExpired.RecordedAt = mixedStart.Add(-time.Microsecond)
				mixedOld.RecordedAt = mixedStart
				mixedYoung.RecordedAt = cutoff.Add(mixedUpper.Sub(cutoff) / 2)
				oldOccurrence.RecordedAt = now.UTC().Add(-retention / 2)
				for _, historical := range []*model.BusinessEvent{&fullyExpired, &mixedOld, &mixedYoung} {
					historical.OccurredAt = historical.RecordedAt
				}
				oldOccurrence.OccurredAt = now.UTC().Add(-retention - 48*time.Hour)
				Expect(oldOccurrence.OccurredAt.Before(cutoff)).To(BeTrue())
				Expect(oldOccurrence.RecordedAt.After(cutoff)).To(BeTrue())
				Expect(mixedOld.RecordedAt.Before(cutoff)).To(BeTrue())
				Expect(mixedYoung.RecordedAt.After(cutoff)).To(BeTrue())
				for _, historical := range []*model.BusinessEvent{&fullyExpired, &mixedOld, &mixedYoung, &oldOccurrence} {
					Expect(helpers.SeedLedgerHistory(context.Background(), journey.h.OwnerDB, historical, journey.h.App.LedgerService.Validate)).To(Succeed())
					_, err := journey.h.OwnerDB.Exec("INSERT INTO public.business_event_delivery_pending(recorded_at,event_id,next_attempt_at) VALUES($1,$2,clock_timestamp())", historical.RecordedAt.Truncate(time.Microsecond), historical.ID)
					Expect(err).NotTo(HaveOccurred())
				}
				partitionPair := func(eventID id.BusinessEventID) (string, string, time.Time) {
					var eventPartition, deliveryPartition string
					var upper time.Time
					Expect(journey.h.OwnerDB.Get(&eventPartition, "SELECT tableoid::regclass::text FROM public.business_events WHERE id=$1", eventID)).To(Succeed())
					Expect(journey.h.OwnerDB.Get(&deliveryPartition, `SELECT child.oid::regclass::text FROM pg_inherits inheritance JOIN pg_class child ON child.oid=inheritance.inhrelid WHERE inheritance.inhparent='public.business_event_delivery_pending'::regclass AND pg_get_expr(child.relpartbound,child.oid)=(SELECT pg_get_expr(relpartbound,oid) FROM pg_class WHERE oid=$1::regclass)`, eventPartition)).To(Succeed())
					Expect(journey.h.OwnerDB.Get(&upper, `SELECT (regexp_match(pg_get_expr(relpartbound,oid), 'TO [(]''([^'']+)'''))[1]::timestamptz FROM pg_class WHERE oid=$1::regclass`, eventPartition)).To(Succeed())
					return eventPartition, deliveryPartition, upper
				}
				expiredEventPartition, expiredDeliveryPartition, expiredUpper := partitionPair(fullyExpired.ID)
				mixedEventPartition, mixedDeliveryPartition, retainedUpper := partitionPair(mixedOld.ID)
				Expect(expiredUpper.After(cutoff)).To(BeFalse(), "drop only partitions whose upper bound has expired")
				Expect(retainedUpper.After(cutoff)).To(BeTrue(), "retain the entire partition containing young records")
				Expect(retainedUpper).To(BeTemporally("~", mixedUpper, time.Microsecond))
				var youngPartition string
				Expect(journey.h.OwnerDB.Get(&youngPartition, "SELECT tableoid::regclass::text FROM public.business_events WHERE id=$1", mixedYoung.ID)).To(Succeed())
				Expect(youngPartition).To(Equal(mixedEventPartition))
				Expect(helpers.MaintainLedgerPartitions(context.Background(), journey.h.OwnerDB)).To(Succeed())
				query := model.BusinessEventQuery{Subject: *expected.Subject, Type: expected.Type, Start: oldOccurrence.OccurredAt.Add(-time.Hour), End: now.UTC().Add(time.Hour), Limit: 1000}
				remaining, err := helpers.QueryLedgerEvents(context.Background(), journey.h.ReaderDB, query)
				Expect(err).NotTo(HaveOccurred())
				ids := make([]id.BusinessEventID, 0, len(remaining))
				for _, event := range remaining {
					ids = append(ids, event.ID)
				}
				Expect(ids).To(ConsistOf(live.ID, mixedOld.ID, mixedYoung.ID, oldOccurrence.ID))
				var expiredAbsent, mixedPresent bool
				Expect(journey.h.OwnerDB.Get(&expiredAbsent, "SELECT to_regclass($1) IS NULL AND to_regclass($2) IS NULL", expiredEventPartition, expiredDeliveryPartition)).To(Succeed())
				Expect(expiredAbsent).To(BeTrue())
				Expect(journey.h.OwnerDB.Get(&mixedPresent, "SELECT to_regclass($1) IS NOT NULL AND to_regclass($2) IS NOT NULL", mixedEventPartition, mixedDeliveryPartition)).To(Succeed())
				Expect(mixedPresent).To(BeTrue())
				for _, historical := range []*model.BusinessEvent{&fullyExpired, &mixedOld, &mixedYoung, &oldOccurrence} {
					var count int
					Expect(journey.h.OwnerDB.Get(&count, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", historical.ID)).To(Succeed())
					if historical.ID == fullyExpired.ID {
						Expect(count).To(BeZero())
					} else {
						Expect(count).To(Equal(1), "unexpired partition references remain pending")
					}
				}
				Expect(journey.h.Close()).To(Succeed())
			}
		})

		// US3-AS3 from specs/048-business-event-ledger/spec.md.
		It("erases an in-flight predecessor and permits a later new occurrence", func() {
			faults := &bootstrap.LedgerStorageFaults{}
			journey := newLedgerJourney(bootstrap.LedgerPostgres, "grant-created", faults)
			expected := journey.action("grant-created")
			before := journey.query("grant-created", *expected.Subject)
			Expect(before).To(HaveLen(1))
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			DeferCleanup(cancel)
			path := "/api/consent/agents/" + journey.data.Agent.ID.String() + "/grants"
			type responseResult struct {
				status int
				err    error
			}
			postUpdate := func(until time.Time) <-chan responseResult {
				completed := make(chan responseResult, 1)
				go func() {
					response, err := postJSON(journey.h.EndUser, path, journey.data.Principal.String(), journey.consentBody(until))
					result := responseResult{err: err}
					if err == nil {
						result.status = response.StatusCode
						result.err = response.Body.Close()
					}
					completed <- result
				}()
				return completed
			}

			// Hold a real workflow owner after append, while its transaction still
			// owns the subject gate; a post-commit pause cannot prove this ordering.
			release := make(chan struct{})
			DeferCleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			reached := faults.HoldBeforeNextCommit(release)
			recorded := postUpdate(time.Now().Add(3 * time.Hour))
			Eventually(reached, 5*time.Second).Should(BeClosed(), "recording must reach the real pre-commit storage barrier")
			Expect(journey.query("grant-updated", *expected.Subject)).To(BeEmpty(), "uncommitted predecessor must not be visible")

			erasure, err := journey.h.ErasureDB.BeginTxx(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
				_ = erasure.Rollback()
			})
			var erasurePID int
			Expect(erasure.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&erasurePID)).To(Succeed())
			type erasureResult struct {
				count int64
				err   error
			}
			erased := make(chan erasureResult, 1)
			go func() {
				var result erasureResult
				result.err = erasure.QueryRowContext(ctx, "SELECT public.business_event_erase_subject($1)", journey.data.Principal.String()).Scan(&result.count)
				if result.err == nil {
					result.err = erasure.Commit()
				}
				erased <- result
			}()
			Eventually(func() int {
				var waiting int
				Expect(journey.h.OwnerDB.Get(&waiting, "SELECT count(*) FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND NOT granted", erasurePID)).To(Succeed())
				return waiting
			}, 5*time.Second, 20*time.Millisecond).Should(BeNumerically(">", 0), "erasure must wait on the held advisory subject gate")
			Consistently(erased, 100*time.Millisecond, 10*time.Millisecond).ShouldNot(Receive())
			close(release)
			var recording responseResult
			Eventually(recorded, 5*time.Second).Should(Receive(&recording))
			Expect(recording.err).NotTo(HaveOccurred())
			Expect(recording.status).To(Equal(http.StatusCreated))
			var completed erasureResult
			Eventually(erased, 5*time.Second).Should(Receive(&completed))
			Expect(completed.err).NotTo(HaveOccurred())
			Expect(completed.count).To(Equal(int64(2)), "the eraser must delete both the committed initial event and in-flight predecessor")
			Expect(journey.query("grant-created", *expected.Subject)).To(BeEmpty())
			Expect(journey.query("grant-updated", *expected.Subject)).To(BeEmpty())

			// Preserve the inverse ordering: once erasure holds the subject gate,
			// a subsequent legitimate grant change must survive its commit.
			second, err := journey.h.ErasureDB.BeginTxx(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = second.Rollback() })
			var count int64
			Expect(second.QueryRowContext(ctx, "SELECT public.business_event_erase_subject($1)", journey.data.Principal.String()).Scan(&count)).To(Succeed())
			Expect(count).To(BeZero())
			laterResponse := postUpdate(time.Now().Add(4 * time.Hour))
			Consistently(laterResponse, 100*time.Millisecond, 10*time.Millisecond).ShouldNot(Receive())
			Expect(second.Commit()).To(Succeed())
			Eventually(laterResponse, 5*time.Second).Should(Receive(&recording))
			Expect(recording.err).NotTo(HaveOccurred())
			Expect(recording.status).To(Equal(http.StatusCreated))
			later := journey.query("grant-updated", *expected.Subject)
			Expect(later).To(HaveLen(1))
			Expect(later[0].ID).NotTo(Equal(before[0].ID))
			Expect(later[0].GrantID).To(Equal(before[0].GrantID))
			Expect(journey.query("grant-created", *expected.Subject)).To(BeEmpty())
			Expect(journey.h.Close()).To(Succeed())
		})

		// US3-AS4 from specs/048-business-event-ledger/spec.md.
		It("does not resurrect erased or retention-deleted history after deferred-copy restart", func() {
			for _, deletion := range []string{"erase", "retention"} {
				receiver, err := helpers.NewOTLPReceiver()
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(receiver.Close)
				receiver.SetUnavailable(true)
				DeferCleanup(func() { receiver.SetUnavailable(false) })
				// Keep the fixture broker's telemetry disabled; only the production-builder
				// child owns pending copies during the collector outage.
				journey := newLedgerJourney(bootstrap.LedgerPostgres, "grant-created", nil)
				other := fixtures.LedgerOtherPrincipal()
				Expect(journey.h.Storage.UserSessions().Create(context.Background(), fixtures.SessionForService(other.String(), fixtures.PlaceholderServiceID.String()))).To(Succeed())
				configuration := *journey.h.App.Config
				configuration.Telemetry = fixtures.TelemetryGRPCConfig().Telemetry
				configuration.Telemetry.Traces.Enabled = false
				configuration.Telemetry.Logs.Enabled = true
				configuration.Telemetry.Exporter.Endpoint = receiver.Endpoint()
				configuration.BusinessEvents.TelemetryCopyEnabled = true
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				DeferCleanup(cancel)
				executable, err := os.Executable()
				Expect(err).NotTo(HaveOccurred())
				child, err := bootstrap.StartLedgerChild(ctx, executable, &configuration)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(child.Kill)
				ready, err := child.Await(ctx, "ready")
				Expect(err).NotTo(HaveOccurred())
				path := "/api/consent/agents/" + journey.data.Agent.ID.String() + "/grants"
				postGrant := func(principal id.Principal) {
					request, err := http.NewRequestWithContext(ctx, http.MethodPost, ready.EndUserURL+path, psJSON(journey.consentBody(time.Now().Add(3*time.Hour))))
					Expect(err).NotTo(HaveOccurred())
					request.Header.Set("X-Remote-User", principal.String())
					request.Header.Set("Content-Type", "application/json")
					response, err := helpers.HTTPClient().Do(request)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusCreated))
					Expect(response.Body.Close()).To(Succeed())
				}
				query := func(principal id.Principal, start time.Time) []*model.BusinessEvent {
					events, err := helpers.QueryLedgerEvents(ctx, journey.h.ReaderDB, model.BusinessEventQuery{
						Subject: model.BusinessEventSubject{Principal: principal}, Type: "agentic-identity-broker.grant-created", Start: start, End: time.Now().UTC().Add(time.Hour), Limit: 1000,
					})
					Expect(err).NotTo(HaveOccurred())
					return events
				}
				var deletedID id.BusinessEventID
				if deletion == "erase" {
					Expect(journey.h.Storage.UserSessions().Create(ctx, fixtures.SessionForService(journey.data.Principal.String(), fixtures.PlaceholderServiceID.String()))).To(Succeed())
					postGrant(journey.data.Principal)
					created := query(journey.data.Principal, journey.start)
					Expect(created).To(HaveLen(1))
					deletedID = created[0].ID
				} else {
					expected := journey.action("grant-created")
					created := journey.query("grant-created", *expected.Subject)
					Expect(created).To(HaveLen(1))
					history := *created[0]
					history.ID = id.NewBusinessEventID()
					history.RecordedAt = time.Now().UTC().Add(-91 * 24 * time.Hour)
					history.OccurredAt = history.RecordedAt
					Expect(helpers.SeedLedgerHistory(ctx, journey.h.OwnerDB, &history, journey.h.App.LedgerService.Validate)).To(Succeed())
					_, err := journey.h.OwnerDB.ExecContext(ctx, "INSERT INTO public.business_event_delivery_pending(recorded_at,event_id,next_attempt_at) VALUES($1,$2,clock_timestamp())", history.RecordedAt.Truncate(time.Microsecond), history.ID)
					Expect(err).NotTo(HaveOccurred())
					deletedID = history.ID
				}
				var pending int
				Expect(journey.h.OwnerDB.Get(&pending, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", deletedID)).To(Succeed())
				Expect(pending).To(Equal(1))
				// No other ledger reference exists yet: this rejected attempt belongs to
				// the event we will delete, not to the unaffected recovery control.
				Expect(receiver.WaitForLedgerAttempts(ctx, 1)).To(Succeed())
				postGrant(other)
				control := query(other, journey.start)
				Expect(control).To(HaveLen(1))
				controlID := control[0].ID
				Expect(journey.h.OwnerDB.Get(&pending, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", controlID)).To(Succeed())
				Expect(pending).To(Equal(1))
				Expect(receiver.LedgerRecords()).To(BeEmpty())
				Expect(child.Kill()).To(Succeed())
				if deletion == "erase" {
					count, err := helpers.EraseLedgerSubject(ctx, journey.h.ErasureDB, journey.data.Principal)
					Expect(err).NotTo(HaveOccurred())
					Expect(count).To(Equal(int64(1)))
				} else {
					Expect(helpers.MaintainLedgerPartitions(ctx, journey.h.OwnerDB)).To(Succeed())
				}
				receiver.SetUnavailable(false)
				restarted, err := bootstrap.StartLedgerChild(ctx, executable, &configuration)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(restarted.Kill)
				_, err = restarted.Await(ctx, "ready")
				Expect(err).NotTo(HaveOccurred())
				copyCount := func(eventID id.BusinessEventID) int {
					count := 0
					for _, record := range receiver.LedgerRecords() {
						for _, attribute := range record.Record.Attributes {
							if attribute.Key == "id" && attribute.Value.GetStringValue() == eventID.String() {
								count++
							}
						}
					}
					return count
				}
				Eventually(func() int { return copyCount(controlID) }, 10*time.Second, 20*time.Millisecond).Should(Equal(1), "a retained pending event must resume delivery with its original ID")
				Eventually(func() int {
					var count int
					Expect(journey.h.OwnerDB.Get(&count, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", controlID)).To(Succeed())
					return count
				}, 10*time.Second, 20*time.Millisecond).Should(BeZero(), "the resumed worker must acknowledge the control")
				Expect(copyCount(deletedID)).To(BeZero(), "deleted history must not be emitted during recovery")
				var rows int
				Expect(journey.h.OwnerDB.Get(&rows, "SELECT count(*) FROM public.business_events WHERE id=$1", deletedID)).To(Succeed())
				Expect(rows).To(BeZero())
				Expect(journey.h.OwnerDB.Get(&rows, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", deletedID)).To(Succeed())
				Expect(rows).To(BeZero())
				Consistently(func() int { return copyCount(deletedID) }, time.Second, 20*time.Millisecond).Should(BeZero())
				Expect(restarted.Kill()).To(Succeed())
				Expect(journey.h.Close()).To(Succeed())
			}
		})
	})

	Context("monitoring continuity", func() {
		// US4-AS3 from specs/048-business-event-ledger/spec.md.
		It("recovers retained post-commit copies after collector outage and process crash without exporting rollback", func() {
			receiver, err := helpers.NewOTLPReceiver()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(receiver.Close)
			receiver.SetUnavailable(true)
			DeferCleanup(func() { receiver.SetUnavailable(false) })
			// The fixture broker does not copy: only a restarted production-builder
			// child may acknowledge the child transaction's pending reference.
			journey := newLedgerJourney(bootstrap.LedgerPostgres, "grant-created", nil)
			Expect(journey.h.Storage.UserSessions().Create(context.Background(), fixtures.SessionForService(journey.data.Principal.String(), fixtures.PlaceholderServiceID.String()))).To(Succeed())
			configuration := *journey.h.App.Config
			configuration.Telemetry = fixtures.TelemetryGRPCConfig().Telemetry
			configuration.Telemetry.Traces.Enabled = false
			configuration.Telemetry.Logs.Enabled = true
			configuration.Telemetry.Exporter.Endpoint = receiver.Endpoint()
			configuration.BusinessEvents.TelemetryCopyEnabled = true
			childCtx, cancelChildren := context.WithCancel(context.Background())
			DeferCleanup(cancelChildren)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			DeferCleanup(cancel)
			executable, err := os.Executable()
			Expect(err).NotTo(HaveOccurred())
			child, err := bootstrap.StartLedgerChild(childCtx, executable, &configuration)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(child.Kill)
			ready, err := child.Await(ctx, "ready")
			Expect(err).NotTo(HaveOccurred())
			Expect(child.Send("hold_commit")).To(Succeed())
			_, err = child.Await(ctx, "armed")
			Expect(err).NotTo(HaveOccurred())
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, ready.EndUserURL+"/api/consent/agents/"+journey.data.Agent.ID.String()+"/grants", psJSON(journey.consentBody(time.Now().Add(time.Hour))))
			Expect(err).NotTo(HaveOccurred())
			request.Header.Set("X-Remote-User", journey.data.Principal.String())
			request.Header.Set("Content-Type", "application/json")
			type requestResult struct {
				status int
				err    error
			}
			responses := make(chan requestResult, 1)
			go func() {
				response, err := helpers.HTTPClient().Do(request)
				result := requestResult{err: err}
				if err == nil {
					result.status = response.StatusCode
					result.err = response.Body.Close()
				}
				responses <- result
			}()
			query := model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: journey.data.Principal}, Type: "agentic-identity-broker.grant-created", Start: journey.start, End: time.Now().UTC().Add(time.Hour)}
			var events []*model.BusinessEvent
			Eventually(func(g Gomega) {
				var err error
				events, err = helpers.QueryLedgerEvents(ctx, journey.h.ReaderDB, query)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(events).To(HaveLen(1))
			}, 5*time.Second, 20*time.Millisecond).Should(Succeed())
			_, err = child.Await(ctx, "committed")
			Expect(err).NotTo(HaveOccurred())
			Expect(responses).NotTo(Receive())
			retainedID := events[0].ID
			var pending int
			Expect(journey.h.OwnerDB.Get(&pending, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", retainedID)).To(Succeed())
			Expect(pending).To(Equal(1))
			Expect(child.Kill()).To(Succeed())
			var result requestResult
			Eventually(responses, 5*time.Second).Should(Receive(&result))
			Expect(result.err).To(HaveOccurred(), "the child must die before sending a successful response")
			Expect(result.status).To(BeZero())
			Expect(receiver.LedgerRecords()).To(BeEmpty())
			restarted, err := bootstrap.StartLedgerChild(childCtx, executable, &configuration)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(restarted.Kill)
			_, err = restarted.Await(ctx, "ready")
			Expect(err).NotTo(HaveOccurred())
			Expect(receiver.WaitForLedgerAttempts(ctx, 1)).To(Succeed(), "the unavailable destination must reject a real ledger export")
			Expect(journey.h.OwnerDB.Get(&pending, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", retainedID)).To(Succeed())
			Expect(pending).To(Equal(1))
			var retryDelay float64
			Eventually(func() bool {
				Expect(journey.h.OwnerDB.Get(&retryDelay, `SELECT EXTRACT(EPOCH FROM next_attempt_at-clock_timestamp())
					FROM public.business_event_delivery_pending WHERE event_id=$1`, retainedID)).To(Succeed())
				return retryDelay > 0
			}, 5*time.Second, 20*time.Millisecond).Should(BeTrue(), "failed export must commit its fixed retry delay")
			receiver.SetUnavailable(false)
			waitCtx, cancelWait := context.WithTimeout(context.Background(), 30*time.Second)
			DeferCleanup(cancelWait)
			retryTimer := time.NewTimer(time.Duration(retryDelay * float64(time.Second)))
			DeferCleanup(retryTimer.Stop)
			select {
			case <-retryTimer.C:
			case <-waitCtx.Done():
				Fail("persisted retry did not become due within its fixed backoff")
			}
			ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
			DeferCleanup(cancel)
			hasCopy := func(eventID id.BusinessEventID) bool {
				for _, record := range receiver.LedgerRecords() {
					for _, attribute := range record.Record.Attributes {
						if attribute.Key == "id" && attribute.Value.GetStringValue() == eventID.String() {
							return true
						}
					}
				}
				return false
			}
			Eventually(func() bool { return hasCopy(retainedID) }, 10*time.Second, 20*time.Millisecond).Should(BeTrue())
			Eventually(func() int {
				var count int
				Expect(journey.h.OwnerDB.Get(&count, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", retainedID)).To(Succeed())
				return count
			}, 10*time.Second, 20*time.Millisecond).Should(BeZero())
			retained, err := helpers.QueryLedgerEvents(ctx, journey.h.ReaderDB, query)
			Expect(err).NotTo(HaveOccurred())
			Expect(retained).To(HaveLen(1))
			Expect(retained[0].ID).To(Equal(retainedID))
			Expect(restarted.Kill()).To(Succeed())
			Expect(journey.h.Close()).To(Succeed())

			// An independently enabled, fault-wrapped broker must refuse a failed
			// revoke, yet successfully export a later real grant update.
			faults := &bootstrap.LedgerStorageFaults{}
			rollback := newConfiguredLedgerJourney(bootstrap.LedgerPostgres, "grant-updated", faults, func(config *ports.Config) {
				config.Telemetry = fixtures.TelemetryGRPCConfig().Telemetry
				config.Telemetry.Traces.Enabled = false
				config.Telemetry.Logs.Enabled = true
				config.Telemetry.Exporter.Endpoint = receiver.Endpoint()
				config.BusinessEvents.TelemetryCopyEnabled = true
			}, nil)
			rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
			DeferCleanup(rollbackCancel)
			grant := rollback.seedGrant(false)
			Expect(rollback.h.Storage.UserSessions().Create(rollbackCtx, fixtures.SessionForService(rollback.data.Principal.String(), fixtures.PlaceholderServiceID.String()))).To(Succeed())
			var before string
			Expect(rollback.h.OwnerDB.GetContext(rollbackCtx, &before, "SELECT to_jsonb(g)::text FROM public.user_grants g WHERE id=$1", grant.ID)).To(Succeed())
			faults.FailAppend("agentic-identity-broker.grant-revoked", errors.New("forced event append failure"))
			response, err := rollback.h.EndUser.DirectRequest(http.MethodDelete, "/api/consent/agents/"+rollback.data.Agent.ID.String()+"/grants", rollback.data.Principal.String(), nil, nil)
			ledgerStatus(response, err, http.StatusInternalServerError)
			var after string
			Expect(rollback.h.OwnerDB.GetContext(rollbackCtx, &after, "SELECT to_jsonb(g)::text FROM public.user_grants g WHERE id=$1", grant.ID)).To(Succeed())
			Expect(after).To(Equal(before), "failed append must preserve the entire committed grant row")
			revoked, err := helpers.QueryLedgerEvents(rollbackCtx, rollback.h.ReaderDB, model.BusinessEventQuery{
				Subject: model.BusinessEventSubject{Principal: rollback.data.Principal}, Type: "agentic-identity-broker.grant-revoked", Start: rollback.start, End: time.Now().UTC().Add(time.Hour),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(revoked).To(BeEmpty())
			Expect(rollback.h.OwnerDB.Get(&pending, "SELECT count(*) FROM public.business_event_delivery_pending")).To(Succeed())
			Expect(pending).To(BeZero(), "rollback must leave no pending ledger copy")
			control, err := postJSON(rollback.h.EndUser, "/api/consent/agents/"+rollback.data.Agent.ID.String()+"/grants", rollback.data.Principal.String(), rollback.consentBody(time.Now().Add(2*time.Hour)))
			Expect(err).NotTo(HaveOccurred())
			Expect(control.StatusCode).To(Equal(http.StatusCreated))
			Expect(control.Body.Close()).To(Succeed())
			updated, err := helpers.QueryLedgerEvents(rollbackCtx, rollback.h.ReaderDB, model.BusinessEventQuery{
				Subject: model.BusinessEventSubject{Principal: rollback.data.Principal}, Type: "agentic-identity-broker.grant-updated", Start: rollback.start, End: time.Now().UTC().Add(time.Hour),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(updated).To(HaveLen(1))
			Expect(updated[0].GrantID).To(Equal(grant.ID))
			Eventually(func() bool { return hasCopy(updated[0].ID) }, 10*time.Second, 20*time.Millisecond).Should(BeTrue(), "the fault-wrapped broker must export a committed control")
			Eventually(func() int {
				var count int
				Expect(rollback.h.OwnerDB.Get(&count, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", updated[0].ID)).To(Succeed())
				return count
			}, 10*time.Second, 20*time.Millisecond).Should(BeZero())
			Expect(rollback.h.Close()).To(Succeed())
			for _, record := range receiver.LedgerRecords() {
				Expect(record.Record.EventName).NotTo(Equal("agentic-identity-broker.grant-revoked"))
			}
		})
	})
})
