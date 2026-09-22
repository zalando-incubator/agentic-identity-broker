package e2e_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

const (
	cimdMetadataPerformanceRequestCount = 100
	cimdMetadataPerformanceClientCount  = 10
)

var _ = Describe("CIMD public metadata performance", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		server         *bootstrap.TestServer
		warmClient     *http.Client
		service        = fixtures.CIMDConfidentialService()
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		storageFactory = bootstrap.NewStorageFactory(logger)

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		service = fixtures.CIMDConfidentialService()
		Expect(testStorage.Services().Create(context.Background(), service)).To(Succeed())

		cimdKeyID := seedCIMDClientAuthenticationKey(testStorage)
		_, err = testStorage.SigningKeys().SetCurrentInDomain(
			context.Background(),
			domainstorage.KeyDomainCIMDClientAuthentication,
			id.NewKeyID(cimdKeyID),
			time.Now().UTC(),
		)
		Expect(err).ToNot(HaveOccurred())

		serverFactory := bootstrap.NewServerFactory(fixtures.CIMDLocalConfig(), logger)
		server, warmClient, err = bootstrap.NewCIMDClientEndUserTestServer(
			testStorage,
			serverFactory,
			fixtures.CIMDEndUserPublicURL,
			logger,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if warmClient != nil {
			warmClient.CloseIdleConnections()
		}
		if server != nil {
			server.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// SC-008 from specs/046-cimd-upstream-client/spec.md
	It("serves warmed metadata and JWK documents within the normal-load p95 budget", Label("cimd-upstream-client", "performance"), func() {
		routes := []struct {
			name string
			url  string
		}{
			{name: "metadata", url: fixtures.CIMDClientIDURL(service.ID)},
			{name: "jwks", url: fixtures.CIMDJWKSURL(service.ID)},
		}

		for _, route := range routes {
			warmCIMDMetadataRoute(warmClient, route.url)
		}

		clients := make([]*http.Client, cimdMetadataPerformanceClientCount)
		for i := range clients {
			var err error
			clients[i], err = bootstrap.CIMDUpstreamHTTPClient(server, fixtures.CIMDEndUserPublicURL)
			Expect(err).ToNot(HaveOccurred())
			defer clients[i].CloseIdleConnections()
		}

		for _, route := range routes {
			durations, statuses, errs := measureCIMDMetadataRoute(clients, route.url)
			for i := range cimdMetadataPerformanceRequestCount {
				Expect(errs[i]).ToNot(HaveOccurred(), "%s request %d failed", route.name, i+1)
				Expect(statuses[i]).To(Equal(http.StatusOK), "%s request %d must return 200 OK", route.name, i+1)
			}

			sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
			p95 := durations[94]
			GinkgoWriter.Printf("SC-008 %s: %d/%d succeeded; p95=%s max=%s min=%s\n", route.name, cimdMetadataPerformanceRequestCount, cimdMetadataPerformanceRequestCount, p95, durations[len(durations)-1], durations[0])
			Expect(p95).To(BeNumerically("<", time.Second), "%s route p95 must complete below one second", route.name)
		}
	})
})

func warmCIMDMetadataRoute(client *http.Client, routeURL string) {
	response, err := client.Get(routeURL)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = response.Body.Close() }()

	Expect(response.StatusCode).To(Equal(http.StatusOK))
	_, err = io.Copy(io.Discard, response.Body)
	Expect(err).ToNot(HaveOccurred())
}

func measureCIMDMetadataRoute(clients []*http.Client, routeURL string) ([]time.Duration, []int, []error) {
	durations := make([]time.Duration, cimdMetadataPerformanceRequestCount)
	statuses := make([]int, cimdMetadataPerformanceRequestCount)
	errs := make([]error, cimdMetadataPerformanceRequestCount)
	requests := make(chan int)

	var workers sync.WaitGroup
	for _, client := range clients {
		workers.Add(1)
		go func(client *http.Client) {
			defer workers.Done()
			for requestIndex := range requests {
				startedAt := time.Now()
				response, err := client.Get(routeURL)
				durations[requestIndex] = time.Since(startedAt)
				if err != nil {
					errs[requestIndex] = err
					continue
				}

				statuses[requestIndex] = response.StatusCode
				_, err = io.Copy(io.Discard, response.Body)
				if closeErr := response.Body.Close(); err == nil {
					err = closeErr
				}
				errs[requestIndex] = err
			}
		}(client)
	}

	for requestIndex := range cimdMetadataPerformanceRequestCount {
		requests <- requestIndex
	}
	close(requests)
	workers.Wait()

	return durations, statuses, errs
}
