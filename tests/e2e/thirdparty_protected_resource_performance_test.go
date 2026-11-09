package e2e_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const protectedResourcePerformanceRegistrations = 20

type protectedResourcePerformanceInstance struct {
	scenario fixtures.ProtectedResourceScenario
	request  []byte
	provider *helpers.MockProtectedResourceProvider
	storage  *storageadapter.Adapter
	admin    *bootstrap.TestServer
	endUser  *bootstrap.TestServer
}

type protectedResourcePerformanceResult struct {
	index        int
	status       int
	elapsed      time.Duration
	serviceID    string
	clientID     string
	clientMethod string
	err          error
}

var _ = Describe("Protected resource registration performance", func() {
	var (
		instances      []*protectedResourcePerformanceInstance
		storageFactory *bootstrap.StorageFactory
	)

	BeforeEach(func() {
		logger := bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)
		instances = make([]*protectedResourcePerformanceInstance, 0, protectedResourcePerformanceRegistrations)

		for range protectedResourcePerformanceRegistrations {
			scenario := fixtures.PublicDCRProtectedResource()
			resource, err := url.Parse(scenario.ResourceURL)
			Expect(err).NotTo(HaveOccurred())
			issuer, err := url.Parse(scenario.IssuerURLs[0])
			Expect(err).NotTo(HaveOccurred())
			registration, err := url.Parse(scenario.RegistrationURL)
			Expect(err).NotTo(HaveOccurred())

			delayed := 0
			for index, reply := range scenario.Replies {
				switch {
				case reply.Host == resource.Hostname() && reply.Method == http.MethodGet && reply.Path == resource.Path:
				case reply.Host == resource.Hostname() && reply.Method == http.MethodGet && reply.Status == http.StatusOK && strings.HasPrefix(reply.Path, "/.well-known/oauth-protected-resource/"):
				case reply.Host == issuer.Hostname() && reply.Method == http.MethodGet && reply.Status == http.StatusOK && strings.HasPrefix(reply.Path, "/.well-known/oauth-authorization-server"):
				case reply.Host == registration.Hostname() && reply.Method == http.MethodPost && reply.Path == registration.Path:
				default:
					continue
				}
				scenario.Replies[index].Delay = time.Second
				delayed++
			}
			Expect(delayed).To(Equal(4), "challenge, PRM, AS metadata, and DCR must each have one-second latency")

			instance := &protectedResourcePerformanceInstance{scenario: scenario}
			instance.request, err = json.Marshal(map[string]any{
				"display_name": "Independent public DCR service",
				"discovery": map[string]any{
					"enable_discovery": true,
					"resource_url":     scenario.ResourceURL,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			instances = append(instances, instance)
			instance.provider = helpers.NewMockProtectedResourceProvider(scenario)
			instance.storage, err = storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())

			config := fixtures.CIMDLocalConfig()
			config.ThirdPartyOAuth2.ClientName = scenario.ClientName
			app, err := bootstrap.NewServerFactory(config, logger).BuildAppWithProtectedResource(
				instance.storage, instance.provider.Server,
				resource.Hostname(), issuer.Hostname(), registration.Hostname(),
			)
			Expect(err).NotTo(HaveOccurred())
			instance.admin, err = bootstrap.NewAdminTestServer(app, logger)
			Expect(err).NotTo(HaveOccurred())
			instance.endUser, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() {
		for index := len(instances) - 1; index >= 0; index-- {
			instance := instances[index]
			if instance.admin != nil {
				instance.admin.Close()
			}
			if instance.endUser != nil {
				instance.endUser.Close()
			}
			if instance.provider != nil {
				instance.provider.Close()
			}
			if instance.storage != nil {
				Expect(storageFactory.CloseStorage(instance.storage)).To(Succeed())
			}
		}
	})

	Context("when twenty independent DCR providers each delay four responses", func() {
		// SC-006 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should complete at least nineteen registrations within five seconds", Label("protected-resource-discovery", "performance"), func() {
			principal := fixtures.DefaultPrincipal().String()
			results := make(chan protectedResourcePerformanceResult, protectedResourcePerformanceRegistrations)
			launch := make(chan struct{})
			var startedAt time.Time
			var ready sync.WaitGroup
			ready.Add(len(instances))

			for index, instance := range instances {
				go func(index int, instance *protectedResourcePerformanceInstance) {
					ready.Done()
					<-launch

					response, err := instance.admin.AuthenticatedPOST("/api/services", principal, "application/json", bytes.NewReader(instance.request))
					result := protectedResourcePerformanceResult{index: index, err: err}
					if err == nil {
						result.status = response.StatusCode
						if response.StatusCode == http.StatusCreated {
							var service struct {
								ID        string `json:"id"`
								ClientID  string `json:"client_id"`
								Discovery struct {
									ClientMethod string `json:"client_method"`
								} `json:"discovery"`
							}
							result.err = json.NewDecoder(response.Body).Decode(&service)
							result.serviceID = service.ID
							result.clientID = service.ClientID
							result.clientMethod = service.Discovery.ClientMethod
						} else {
							_, result.err = io.Copy(io.Discard, response.Body)
						}
						if closeErr := response.Body.Close(); result.err == nil {
							result.err = closeErr
						}
					}
					result.elapsed = time.Since(startedAt)
					results <- result
				}(index, instance)
			}

			ready.Wait()
			startedAt = time.Now()
			close(launch)

			successful := 0
			statuses := make(map[int]int)
			registrationCount := 0
			for range instances {
				result := <-results
				instance := instances[result.index]
				statuses[result.status]++
				registrationCount += instance.provider.RegistrationCount()
				if result.err != nil || result.status != http.StatusCreated || result.elapsed > 5*time.Second ||
					result.serviceID == "" || result.clientID != "public-dcr-client" || result.clientMethod != "dcr" ||
					instance.provider.RegistrationCount() != 1 {
					continue
				}
				validClientName := false
				for _, call := range instance.provider.Calls() {
					if call.Method == http.MethodPost && call.Form.Get("client_name") == instance.scenario.ClientName {
						validClientName = true
					}
				}
				if validClientName {
					successful++
				}
			}
			GinkgoWriter.Printf("SC-006: %d/%d registrations completed within five seconds (Admin statuses: %v; DCR requests: %d)\n", successful, protectedResourcePerformanceRegistrations, statuses, registrationCount)
			Expect(successful).To(BeNumerically(">=", 19), "SC-006 requires at least 19 of 20 registrations within five seconds")
		})
	})
})
