package e2e_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Domain event ledger SC-004 performance", Label("performance"), func() {
	var (
		postgres *bootstrap.PostgresFixture
		db       *sql.DB
		stores   [2]*storageadapter.Adapter
		servers  [2]*bootstrap.TestServer
		logs     [2]*bootstrap.BufferedLogCapture
		client   *http.Client
		agent    *storagedomain.Agent
	)

	BeforeEach(func() {
		postgres, db = nil, nil
		stores = [2]*storageadapter.Adapter{}
		servers = [2]*bootstrap.TestServer{}
		Expect(bootstrap.CanAccessContainerRuntime()).To(Succeed(), "SC-004 requires Docker or Podman")
		var err error
		postgres, err = bootstrap.NewPostgresFixture(context.Background())
		Expect(err).NotTo(HaveOccurred())
		db, err = sql.Open("pgx", postgres.ConnectionURL)
		Expect(err).NotTo(HaveOccurred())
		client = &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxConnsPerHost:     1,
				MaxIdleConns:        1,
				MaxIdleConnsPerHost: 1,
			},
		}
		agent = fixtures.ValidAgent()
		for i := range servers {
			cfg := fixtures.DomainEventConfig(postgres.ConnectionURL, i == 1)
			logger, capture := bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
			logs[i] = capture
			stores[i], err = storageadapter.NewAdapter(&cfg.Storage)
			Expect(err).NotTo(HaveOccurred())
			if i == 0 {
				Expect(fixtures.SeedDefaultConsentData(context.Background(), stores[i], id.NewPrincipal(fixtures.DefaultPrincipal().String()))).To(Succeed())
				Expect(stores[i].Agents().Create(context.Background(), agent)).To(Succeed())
			}
			application, err := bootstrap.NewServerFactory(cfg, logger).BuildApp(stores[i])
			Expect(err).NotTo(HaveOccurred())
			servers[i], err = bootstrap.NewEndUserTestServer(application, logger)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() {
		if client != nil {
			client.CloseIdleConnections()
		}
		for _, server := range servers {
			if server != nil {
				server.Close()
			}
		}
		for _, store := range stores {
			if store != nil {
				Expect(store.Close(context.Background())).To(Succeed())
			}
		}
		if db != nil {
			Expect(db.Close()).To(Succeed())
		}
		if postgres != nil {
			Expect(postgres.Close(context.Background())).To(Succeed())
		}
	})

	// Success criterion SC-004 from specs/20261118-125900-domain-event-ledger/spec.md; protocol in plan.md.
	It("keeps enabled p99 revocation latency within 5 ms of disabled p99 with every write verified", func() {
		samples := [2][]time.Duration{make([]time.Duration, 0, 1000), make([]time.Duration, 0, 1000)}
		for block := range 11 {
			for broker, server := range servers {
				before := helpers.CommittedDomainEvents(db)
				Expect(proactiveEvents(logs[broker], "domain_event.ledger.write_failed")).To(BeEmpty())
				for range 100 {
					helpers.CreateDomainEventGrant(client, server, agent.ID, nil)
					req, err := http.NewRequest(http.MethodDelete,
						server.BaseURL()+"/api/consent/agents/"+agent.ID.String()+"/grants", nil)
					Expect(err).NotTo(HaveOccurred())
					req.Header.Set("X-Remote-User", fixtures.DefaultPrincipal().String())

					started := time.Now()
					resp, requestErr := client.Do(req)
					var readErr, closeErr error
					if requestErr == nil {
						_, readErr = io.Copy(io.Discard, resp.Body)
						closeErr = resp.Body.Close()
					}
					elapsed := time.Since(started)

					Expect(requestErr).NotTo(HaveOccurred())
					Expect(readErr).NotTo(HaveOccurred())
					Expect(closeErr).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
					if block != 0 {
						samples[broker] = append(samples[broker], elapsed)
					}
				}
				after := helpers.CommittedDomainEvents(db)
				expectedWrites := 0
				if broker == 1 {
					expectedWrites = 100
				}
				Expect(len(after)-len(before)).To(Equal(expectedWrites), "broker %d, block %d (block 0 is warm-up)", broker, block)
				Expect(proactiveEvents(logs[broker], "domain_event.ledger.write_failed")).To(BeEmpty())
			}
		}
		for _, durations := range samples {
			Expect(durations).To(HaveLen(1000))
			sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		}
		offP99, onP99 := samples[0][989], samples[1][989]
		offMedian := samples[0][499] + (samples[0][500]-samples[0][499])/2
		onMedian := samples[1][499] + (samples[1][500]-samples[1][499])/2
		result := fmt.Sprintf("SC-004: off p99=%s median=%s; on p99=%s median=%s; p99 delta=%s", offP99, offMedian, onP99, onMedian, onP99-offP99)
		GinkgoWriter.Println(result)
		AddReportEntry("SC-004", result)
		Expect(onP99-offP99).To(BeNumerically("<=", 5*time.Millisecond), result)
	})
})
