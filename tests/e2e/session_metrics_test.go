package e2e_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

func collectNamedMetric(reader *sdkmetric.ManualReader, name string) (metricdata.Metrics, bool) {
	var metrics metricdata.ResourceMetrics
	ExpectWithOffset(1, reader.Collect(context.Background(), &metrics)).To(Succeed())
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return metric, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

// sessionHealthCounts returns oauth2_sessions values keyed by service ID, then "expiry/refreshable".
func sessionHealthCounts(reader *sdkmetric.ManualReader) (map[string]map[string]int64, []attribute.Set) {
	metric, ok := collectNamedMetric(reader, "oauth2_sessions")
	if !ok {
		return nil, nil
	}
	gauge, ok := metric.Data.(metricdata.Gauge[int64])
	ExpectWithOffset(1, ok).To(BeTrue(), "oauth2_sessions is %T, want Gauge[int64]", metric.Data)
	counts := map[string]map[string]int64{}
	var attributes []attribute.Set
	for _, point := range gauge.DataPoints {
		service, _ := point.Attributes.Value("service_id")
		expiry, _ := point.Attributes.Value("expiry")
		refreshable, _ := point.Attributes.Value("refreshable")
		if counts[service.AsString()] == nil {
			counts[service.AsString()] = map[string]int64{}
		}
		counts[service.AsString()][expiry.AsString()+"/"+refreshable.AsString()] = point.Value
		attributes = append(attributes, point.Attributes)
	}
	return counts, attributes
}

// sweepDurationCounts returns session_sweep_duration_seconds run counts keyed by "outcome/dry_run".
func sweepDurationCounts(reader *sdkmetric.ManualReader) map[string]uint64 {
	counts := map[string]uint64{}
	metric, ok := collectNamedMetric(reader, "session_sweep_duration_seconds")
	if !ok {
		return counts
	}
	histogram, ok := metric.Data.(metricdata.Histogram[float64])
	ExpectWithOffset(1, ok).To(BeTrue(), "session_sweep_duration_seconds is %T, want Histogram[float64]", metric.Data)
	for _, point := range histogram.DataPoints {
		outcome, _ := point.Attributes.Value("outcome")
		dryRun, _ := point.Attributes.Value("dry_run")
		counts[outcome.AsString()+"/"+dryRun.AsString()] += point.Count
	}
	return counts
}

func sweepSkippedTotal(reader *sdkmetric.ManualReader) int64 {
	metric, ok := collectNamedMetric(reader, "session_sweep_skipped_total")
	if !ok {
		return 0
	}
	sum, ok := metric.Data.(metricdata.Sum[int64])
	ExpectWithOffset(1, ok).To(BeTrue(), "session_sweep_skipped_total is %T, want Sum[int64]", metric.Data)
	var total int64
	for _, point := range sum.DataPoints {
		trigger, _ := point.Attributes.Value("triggered_by")
		ExpectWithOffset(1, trigger.AsString()).To(Equal("sweep"))
		total += point.Value
	}
	return total
}

// healthSeries returns the eight zero-filled oauth2_sessions series of one service with overrides.
func healthSeries(overrides map[string]int64) map[string]int64 {
	series := map[string]int64{}
	for _, expiry := range []string{"valid", "expiring", "expired", "unknown"} {
		for _, refreshable := range []string{"true", "false"} {
			series[expiry+"/"+refreshable] = 0
		}
	}
	for key, value := range overrides {
		series[key] = value
	}
	return series
}

func sweepRequest(admin *bootstrap.TestServer, ctx context.Context, principal, body string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, admin.BaseURL()+"/api/sessions/sweep", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if principal != "" {
		req.Header.Set("X-Remote-User", principal)
	}
	return http.DefaultClient.Do(req)
}

func sweepStatus(admin *bootstrap.TestServer, principal, body string) int {
	resp, err := sweepRequest(admin, context.Background(), principal, body)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func warnRecords(logs *bootstrap.BufferedLogCapture, msg string) []map[string]any {
	records, err := logs.Records()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	var matches []map[string]any
	for _, record := range records {
		if record["msg"] == msg && record["level"] == "WARN" {
			matches = append(matches, record)
		}
	}
	return matches
}

var _ = Describe("Session Metrics", func() {
	var (
		ctx          context.Context
		reader       *sdkmetric.ManualReader
		logger       *slog.Logger
		logs         *bootstrap.BufferedLogCapture
		config       *ports.Config
		storeFactory *bootstrap.StorageFactory
		store        *storageadapter.Adapter
		admin        *bootstrap.TestServer
		upstream     *helpers.MockUpstreamOAuth2Server
		calendar     id.ServiceID
		tickets      id.ServiceID
		release      func()
	)

	createServices := func() {
		github := fixtures.GitHubService()
		github.Endpoints.TokenEndpoint = upstream.URL() + "/oauth/token"
		github.Endpoints.AuthorizeEndpoint = upstream.URL() + "/oauth/authorize"
		google := fixtures.GoogleService()
		calendar, tickets = github.ID, google.ID
		Expect(store.Services().Create(ctx, github)).To(Succeed())
		Expect(store.Services().Create(ctx, google)).To(Succeed())
	}
	seed := func(sessions ...*storagedomain.UserSession) {
		for _, session := range sessions {
			Expect(store.UserSessions().Create(ctx, session)).To(Succeed())
		}
	}
	boot := func() {
		app, err := bootstrap.NewServerFactory(config, logger).BuildApp(store)
		Expect(err).NotTo(HaveOccurred())
		admin, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).NotTo(HaveOccurred())
	}

	BeforeEach(func() {
		ctx = context.Background()
		admin, release = nil, nil
		previous := otel.GetMeterProvider()
		reader = sdkmetric.NewManualReader()
		provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		otel.SetMeterProvider(provider)
		DeferCleanup(func() {
			otel.SetMeterProvider(previous)
			Expect(provider.Shutdown(context.Background())).To(Succeed())
		})
		logger, logs = bootstrap.NewBufferedJSONLogger(slog.LevelDebug)
		upstream = helpers.NewMockUpstreamOAuth2Server()
		upstream.WithSuccessfulTokenResponse().WithAccessToken("renewed-github-token").WithRefreshToken("renewed-github-refresh")
		config = fixtures.OAuth2ConfigWithTokenExchange(upstream.URL())
		config.TokenRefresh = ports.TokenRefreshConfig{
			LookaheadDuration: 5 * time.Minute,
			BackgroundWorkers: 10,
			Sweep:             ports.TokenRefreshSweepConfig{DefaultPageSize: 100},
			HealthMetrics:     ports.TokenRefreshHealthMetricsConfig{Enabled: true, QueryTimeout: 2 * time.Second},
		}
		storeFactory = bootstrap.NewStorageFactory(logger)
	})

	AfterEach(func() {
		if release != nil {
			release()
		}
		if admin != nil {
			admin.Close()
		}
		if upstream != nil {
			upstream.Close()
		}
		if store != nil {
			Expect(storeFactory.CloseStorage(store)).To(Succeed())
			store = nil
		}
	})

	Context("with in-memory storage", func() {
		BeforeEach(func() {
			var err error
			store, err = storeFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			createServices()
		})

		Describe("User Story 2: session health per service", func() {
			// US2-S1 from specs/053-session-auto-refresh/spec.md
			It("reports one value per service and series that sums to the stored sessions", func() {
				seed(fixtures.HealthJourneySessions(time.Now(), calendar, tickets)...)
				boot()
				counts, _ := sessionHealthCounts(reader)
				Expect(counts).To(HaveLen(2))
				for _, service := range []id.ServiceID{calendar, tickets} {
					Expect(counts[service.String()]).To(HaveLen(8))
				}
				sum := func(states map[string]int64) (total int64) {
					for _, value := range states {
						total += value
					}
					return total
				}
				Expect(sum(counts[calendar.String()])).To(Equal(int64(6)))
				Expect(sum(counts[tickets.String()])).To(Equal(int64(2)))
			})

			// US2-S2 from specs/053-session-auto-refresh/spec.md
			It("reports zero for series without sessions", func() {
				seed(fixtures.HealthJourneySessions(time.Now(), calendar, tickets)...)
				boot()
				counts, _ := sessionHealthCounts(reader)
				Expect(counts[tickets.String()]).To(Equal(healthSeries(map[string]int64{"valid/true": 1, "expired/false": 1})))
			})

			// US2-S3 from specs/053-session-auto-refresh/spec.md
			It("counts a session expiring after the lookahead as valid", func() {
				now := time.Now()
				seed(fixtures.ValidSession("valid-a@example.com", calendar, now), fixtures.ValidSession("valid-b@example.com", calendar, now))
				boot()
				counts, _ := sessionHealthCounts(reader)
				Expect(counts[calendar.String()]).To(Equal(healthSeries(map[string]int64{"valid/true": 2})))
			})

			// US2-S4 from specs/053-session-auto-refresh/spec.md
			It("counts a due session as expiring and an already expired one as expired, both refreshable", func() {
				now := time.Now()
				seed(fixtures.ExpiringRefreshableSession("due@example.com", calendar, now), fixtures.ExpiredRefreshableSession("expired@example.com", calendar, now))
				boot()
				counts, _ := sessionHealthCounts(reader)
				Expect(counts[calendar.String()]).To(Equal(healthSeries(map[string]int64{"expiring/true": 1, "expired/true": 1})))
			})

			// US2-S5 from specs/053-session-auto-refresh/spec.md
			It("counts expired sessions without a usable refresh token as expired and unrefreshable", func() {
				now := time.Now()
				seed(fixtures.ExpiredUnrefreshableSession("lapsed@example.com", calendar, now), fixtures.ExpiredWithoutRefreshSession("norefresh@example.com", calendar, now))
				boot()
				counts, _ := sessionHealthCounts(reader)
				Expect(counts[calendar.String()]).To(Equal(healthSeries(map[string]int64{"expired/false": 2})))
			})

			// US2-S6 from specs/053-session-auto-refresh/spec.md
			It("counts a session without recorded expiry under unknown", func() {
				seed(fixtures.UnknownExpirySession("timeless@example.com", tickets, time.Now()))
				boot()
				counts, _ := sessionHealthCounts(reader)
				Expect(counts[tickets.String()]).To(Equal(healthSeries(map[string]int64{"unknown/true": 1})))
			})

			// US2-S9 from specs/053-session-auto-refresh/spec.md
			It("exports only service_id, expiry, and refreshable attributes", func() {
				seed(fixtures.HealthJourneySessions(time.Now(), calendar, tickets)...)
				boot()
				_, attributes := sessionHealthCounts(reader)
				Expect(attributes).To(HaveLen(16))
				for _, set := range attributes {
					var keys []string
					for _, kv := range set.ToSlice() {
						keys = append(keys, string(kv.Key))
					}
					Expect(keys).To(ConsistOf("service_id", "expiry", "refreshable"))
				}
			})
		})

		Describe("User Story 3: sweep runs in metrics", func() {
			// US3-S1 from specs/053-session-auto-refresh/spec.md
			It("records a completed run with its dry_run value in the duration histogram", func() {
				due := fixtures.GitHubSessionForPrincipal(fixtures.DefaultPrincipal().String())
				soon := time.Now().Add(2 * time.Minute)
				due.AccessTokenExpiresAt = &soon
				seed(due)
				boot()
				Expect(sweepStatus(admin, fixtures.AdminPrincipal().String(), `{"lookahead_duration":"PT10M"}`)).To(Equal(http.StatusOK))
				Expect(sweepStatus(admin, fixtures.AdminPrincipal().String(), `{"lookahead_duration":"PT10M","dry_run":true}`)).To(Equal(http.StatusOK))
				Expect(sweepDurationCounts(reader)).To(Equal(map[string]uint64{"completed/false": 1, "completed/true": 1}))
			})

			// US3-S3 from specs/053-session-auto-refresh/spec.md
			It("increments the skipped counter for already-refreshed or deleted candidates", func() {
				now := time.Now()
				principals := []string{fixtures.DefaultPrincipal().String(), fixtures.AnotherPrincipal().String(), fixtures.AdminPrincipal().String()}
				for i, principal := range principals {
					session := fixtures.GitHubSessionForPrincipal(principal)
					expiry := now.Add(time.Duration(i+1) * time.Minute)
					session.AccessTokenExpiresAt = &expiry
					seed(session)
				}
				release = upstream.WithTokenResponseGate()
				boot()
				type sweepResult struct {
					status int
					body   map[string]any
					err    error
				}
				done := make(chan sweepResult, 1)
				go func() {
					resp, err := sweepRequest(admin, context.Background(), fixtures.AdminPrincipal().String(), `{"lookahead_duration":"PT10M"}`)
					if err != nil {
						done <- sweepResult{err: err}
						return
					}
					defer func() { _ = resp.Body.Close() }()
					var body map[string]any
					err = json.NewDecoder(resp.Body).Decode(&body)
					done <- sweepResult{status: resp.StatusCode, body: body, err: err}
				}()
				Eventually(upstream.GetTokenRequests).Should(HaveLen(1))

				renewed := now.Add(2 * time.Hour)
				_, err := store.SessionRefresh().WithLockedSession(ctx, id.Principal(principals[1]), calendar, func(_ context.Context, session *storagedomain.UserSession) (bool, error) {
					session.AccessTokenExpiresAt = &renewed
					return true, nil
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(store.UserSessions().DeleteByPrincipalAndService(ctx, id.Principal(principals[2]), calendar)).To(Succeed())
				release()
				release = nil

				var result sweepResult
				Eventually(done, 10*time.Second).Should(Receive(&result))
				Expect(result.err).NotTo(HaveOccurred())
				Expect(result.status).To(Equal(http.StatusOK))
				Expect(result.body).To(HaveKeyWithValue("refreshed", BeNumerically("==", 1)))
				Expect(result.body).To(HaveKeyWithValue("skipped", BeNumerically("==", 2)))
				Expect(sweepSkippedTotal(reader)).To(Equal(int64(2)))
			})
		})
	})

	Context("with PostgreSQL storage", Label("docker"), func() {
		var (
			postgres *bootstrap.PostgresFixture
			db       *sql.DB
		)

		lockSessions := func() (unlock func()) {
			tx, err := db.BeginTx(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			_, err = tx.ExecContext(ctx, "LOCK TABLE user_sessions IN ACCESS EXCLUSIVE MODE")
			Expect(err).NotTo(HaveOccurred())
			return func() { Expect(tx.Rollback()).To(Succeed()) }
		}

		BeforeEach(func() {
			Expect(bootstrap.CanAccessContainerRuntime()).To(Succeed(), "PostgreSQL E2E scenarios need Docker or Podman")
			var err error
			postgres, err = bootstrap.NewPostgresFixture(ctx)
			Expect(err).NotTo(HaveOccurred())
			config.Storage = ports.StorageConfig{
				Backend:  "postgres",
				Postgres: ports.PostgresConfig{ConnectionURL: postgres.ConnectionURL},
				Timeouts: ports.StorageTimeouts{Read: time.Second, Write: 10 * time.Second},
			}
			store, err = storageadapter.NewAdapter(&config.Storage)
			Expect(err).NotTo(HaveOccurred())
			db, err = sql.Open("pgx", postgres.ConnectionURL)
			Expect(err).NotTo(HaveOccurred())
			createServices()
		})

		AfterEach(func() {
			if db != nil {
				Expect(db.Close()).To(Succeed())
			}
			if admin != nil {
				admin.Close()
				admin = nil
			}
			if store != nil {
				Expect(storeFactory.CloseStorage(store)).To(Succeed())
				store = nil
			}
			if postgres != nil {
				Expect(postgres.Close(context.Background())).To(Succeed())
			}
		})

		// US2-S7 from specs/053-session-auto-refresh/spec.md
		It("reports no points and logs a warning when the query fails, then recovers", func() {
			seed(fixtures.HealthJourneySessions(time.Now(), calendar, tickets)...)
			boot()
			unlock := lockSessions()
			counts, _ := sessionHealthCounts(reader)
			Expect(counts).To(BeNil())
			warnings := warnRecords(logs, "session health query failed")
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(HaveKey("error_kind"))
			unlock()
			counts, _ = sessionHealthCounts(reader)
			Expect(counts).To(HaveLen(2))
		})

		// US2-S8 from specs/053-session-auto-refresh/spec.md
		It("issues no health query when health metrics are disabled", func() {
			config.TokenRefresh.HealthMetrics.Enabled = false
			seed(fixtures.HealthJourneySessions(time.Now(), calendar, tickets)...)
			boot()
			unlock := lockSessions()
			defer unlock()
			started := time.Now()
			counts, _ := sessionHealthCounts(reader)
			Expect(time.Since(started)).To(BeNumerically("<", time.Second))
			Expect(counts).To(BeNil())
			Expect(warnRecords(logs, "session health query failed")).To(BeEmpty())

			// The same lock is observable once health metrics are enabled, so the absence above
			// means no query was issued rather than a query that went unnoticed.
			admin.Close()
			admin = nil
			config.TokenRefresh.HealthMetrics.Enabled = true
			boot()
			counts, _ = sessionHealthCounts(reader)
			Expect(counts).To(BeNil())
			Expect(warnRecords(logs, "session health query failed")).To(HaveLen(1))
		})

		// US3-S2 from specs/053-session-auto-refresh/spec.md
		It("records rejected, aborted, and canceled runs under their outcome and no completed run", func() {
			due := fixtures.GitHubSessionForPrincipal(fixtures.DefaultPrincipal().String())
			soon := time.Now().Add(2 * time.Minute)
			due.AccessTokenExpiresAt = &soon
			seed(due)
			config.TokenRefresh.HealthMetrics.Enabled = false
			release = upstream.WithTokenResponseGate()
			boot()

			Expect(sweepStatus(admin, fixtures.AdminPrincipal().String(), `{"unknown":1}`)).To(Equal(http.StatusBadRequest))
			Expect(sweepStatus(admin, "", `{"lookahead_duration":"PT10M"}`)).To(Equal(http.StatusInternalServerError))

			cancelCtx, cancel := context.WithCancel(ctx)
			canceled := make(chan error, 1)
			go func() {
				resp, err := sweepRequest(admin, cancelCtx, fixtures.AdminPrincipal().String(), `{"lookahead_duration":"PT10M"}`)
				if resp != nil {
					_ = resp.Body.Close()
				}
				canceled <- err
			}()
			Eventually(upstream.GetTokenRequests).Should(HaveLen(1))
			cancel()
			Eventually(canceled, 10*time.Second).Should(Receive(HaveOccurred()))
			release()
			release = nil
			Eventually(func() map[string]uint64 { return sweepDurationCounts(reader) }, 10*time.Second).Should(HaveKeyWithValue("canceled/false", uint64(1)))

			unlock := lockSessions()
			Expect(sweepStatus(admin, fixtures.AdminPrincipal().String(), `{"lookahead_duration":"PT10M"}`)).To(Equal(http.StatusInternalServerError))
			unlock()

			Expect(sweepDurationCounts(reader)).To(Equal(map[string]uint64{
				"rejected/false": 1, "canceled/false": 1, "aborted/false": 1,
			}), "histogram outcomes after rejected, canceled, aborted runs")
		})
	})
})
