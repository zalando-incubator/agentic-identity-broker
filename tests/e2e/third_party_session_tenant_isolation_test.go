package e2e_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

var _ = Describe("Third-party session tenant isolation", func() {
	var (
		server         *bootstrap.TestServer
		testStorage    *storageadapter.Adapter
		storageFactory *bootstrap.StorageFactory
		logger         *slog.Logger
		principalA     string
		agentAID       string
		agentAName     string
		agentBID       string
		agentBName     string
	)

	BeforeEach(func() {
		ctx := context.Background()
		logger = bootstrap.TestLogger(slog.LevelInfo)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, testStorage)).To(Succeed())

		principalA = fixtures.DefaultPrincipal().String()
		principalB := fixtures.AnotherPrincipal().String()
		agentA := fixtures.ValidAgent()
		agentB := fixtures.AnotherAgent()
		agentAID = agentA.ID.String()
		agentAName = agentA.DisplayName
		agentBID = agentB.ID.String()
		agentBName = agentB.DisplayName

		Expect(testStorage.Agents().Create(ctx, agentA)).To(Succeed())
		Expect(testStorage.Agents().Create(ctx, agentB)).To(Succeed())
		Expect(testStorage.UserSessions().Create(ctx, fixtures.SessionForService(principalA, fixtures.PlaceholderServiceID.String()))).To(Succeed())
		Expect(testStorage.UserSessions().Create(ctx, fixtures.SessionForService(principalB, fixtures.PlaceholderServiceID.String()))).To(Succeed())
		Expect(testStorage.UserGrants().Create(ctx, fixtures.ActiveGrant(principalA, agentAID, fixtures.PlaceholderServiceID.String(), []string{"read"}))).To(Succeed())
		Expect(testStorage.UserGrants().Create(ctx, fixtures.ActiveGrant(principalB, agentBID, fixtures.PlaceholderServiceID.String(), []string{"read"}))).To(Succeed())

		app, err := bootstrap.NewServerFactory(fixtures.DefaultOAuth2Config(), logger).BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		server, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if storageFactory != nil {
			Expect(storageFactory.CloseStorage(testStorage)).To(Succeed())
		}
	})

	// US3.S1 — tenant-isolation security regression; public contract: api/enduser/openapi.yaml.
	It("returns only the authenticated principal's dependent agents", func() {
		resp, err := server.AuthenticatedGET("/api/third-party/"+fixtures.PlaceholderServiceID.String()+"/session", principalA)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body struct {
			Data struct {
				DependentAgents []struct {
					ID          string `json:"id"`
					DisplayName string `json:"display_name"`
				} `json:"dependent_agents"`
				DependentAgentCount int `json:"dependent_agent_count"`
			} `json:"data"`
		}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		Expect(body.Data.DependentAgents).To(HaveLen(1))
		Expect(body.Data.DependentAgents[0].ID).To(Equal(agentAID))
		Expect(body.Data.DependentAgents[0].DisplayName).To(Equal(agentAName))
		Expect(body.Data.DependentAgents[0].ID).ToNot(Equal(agentBID))
		Expect(body.Data.DependentAgents[0].DisplayName).ToNot(Equal(agentBName))
		Expect(body.Data.DependentAgentCount).To(Equal(1))
	})

	// US1.S5 — tenant-isolation security regression; public contract: api/enduser/openapi.yaml.
	It("counts only the authenticated principal's dependent agents", func() {
		resp, err := server.AuthenticatedGET("/api/third-party/sessions", principalA)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body struct {
			Data struct {
				Sessions []struct {
					DependentAgentCount int `json:"dependent_agent_count"`
				} `json:"sessions"`
			} `json:"data"`
		}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		Expect(body.Data.Sessions).To(HaveLen(1))
		Expect(body.Data.Sessions[0].DependentAgentCount).To(Equal(1))
	})
})
