package e2e_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

var _ = Describe("Admin agent list filtered by external ID", func() {
	const (
		analystExternalID  = "urn:example:ai-registry:agent:1b44e0"
		researchExternalID = "urn:example:ai-registry:agent:7f3a9c"
		missingExternalID  = "urn:example:ai-registry:agent:9d0e11"
	)

	var (
		server         *bootstrap.TestServer
		testStorage    *storageadapter.Adapter
		storageFactory *bootstrap.StorageFactory
		permissionID   id.PermissionSetID
		analyst        *storage.Agent
		research       *storage.Agent
		researchEU     *storage.Agent
		withoutID      *storage.Agent
	)

	principal := fixtures.AdminPrincipal().String()
	filterPath := func(value string) string { return "/api/agents?external_id=" + url.QueryEscape(value) }

	createAgent := func(name, canonical, external string) *storage.Agent {
		agent := fixtures.LocalAgent()
		agent.DisplayName = name
		agent.CanonicalID = &canonical
		if external != "" {
			externalID := id.ExternalID(external)
			agent.ExternalID = &externalID
		}
		agent.PermissionSets = []storage.AgentPermissionSetEntry{{
			PermissionSetID: permissionID,
			RequirementType: storage.RequirementTypeMandatory,
		}}
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
		return agent
	}

	readAgents := func(resp *http.Response) []map[string]any {
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))
		Expect(resp.Header.Values("Vary")).To(ContainElement(ContainSubstring("Prefer")))
		var agents []map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&agents)).To(Succeed())
		Expect(agents).NotTo(BeNil())
		return agents
	}

	agentIDs := func(agents []map[string]any) []string {
		ids := make([]string, 0, len(agents))
		for _, agent := range agents {
			idValue, ok := agent["id"].(string)
			Expect(ok).To(BeTrue())
			ids = append(ids, idValue)
		}
		return ids
	}

	expectInvalidFilter := func(path string) {
		resp, err := server.AuthenticatedGET(path, principal)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "path: %s", path)
		var body map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		Expect(body).To(HaveKeyWithValue("error", "invalid external_id"))
		Expect(body["message"]).To(BeAssignableToTypeOf(""))
		Expect(body["message"]).NotTo(BeEmpty())
		Expect(body).To(HaveLen(2), "error responses must not contain agent data")
	}

	BeforeEach(func() {
		logger := bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		factory := bootstrap.NewServerFactory(fixtures.LocalConfig(), logger)
		application, err := factory.BuildApp(testStorage)
		Expect(err).NotTo(HaveOccurred())
		server, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())

		service := fixtures.ServiceWithID(id.NewServiceID().String())
		Expect(testStorage.Services().Create(context.Background(), service)).To(Succeed())
		permissionID = id.NewPermissionSetID()
		canonical := "github-read"
		Expect(testStorage.PermissionSets().Create(context.Background(), &storage.PermissionSet{
			ID: permissionID, CanonicalID: &canonical, Name: "GitHub Read Access", Description: "Read access",
			ServiceScopes: []storage.ServiceScope{{
				ServiceID: service.ID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeMandatory,
			}},
		})).To(Succeed())

		analyst = createAgent("Data Analyst", "data-analyst", analystExternalID)
		research = createAgent("Research Assistant", "research-assistant", researchExternalID)
		researchEU = createAgent("Research Assistant (EU)", "research-assistant-eu", researchExternalID)
		withoutID = createAgent("No Governance ID", "no-governance-id", "")
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if testStorage != nil {
			Expect(storageFactory.CloseStorage(testStorage)).To(Succeed())
		}
	})

	Describe("when searching by a valid external ID", func() {
		var literal, reserved, unicode *storage.Agent
		const reservedID = "urn:example:ai-registry:r&d:agent+7;rev=2"
		const literalID = "urn:example:*%_?"
		unicodeID := strings.Repeat("界", 255)

		BeforeEach(func() {
			literal = createAgent("Literal Pattern", "literal-pattern", literalID)
			createAgent("Similar Literal Pattern", "similar-literal-pattern", "urn:example:*xY?")
			reserved = createAgent("Reserved Characters", "reserved-characters", reservedID)
			unicode = createAgent("Unicode Boundary", "unicode-boundary", unicodeID)
		})

		// US1-S1 from specs/054-agent-external-id-filter/spec.md
		It("should return only the exact matching agent for each unique ID", func() {
			for _, tc := range []struct {
				value string
				agent *storage.Agent
			}{
				{analystExternalID, analyst},
				{literalID, literal},
				{reservedID, reserved},
				{unicodeID, unicode},
			} {
				resp, err := server.AuthenticatedGET(filterPath(tc.value), principal)
				Expect(err).NotTo(HaveOccurred())
				agents := readAgents(resp)
				Expect(agentIDs(agents)).To(ConsistOf(tc.agent.ID.String()))
				Expect(agents[0]["external_id"]).To(Equal(tc.value))
				Expect(agents[0]["display_name"]).To(Equal(tc.agent.DisplayName))
			}
			resp, err := server.AuthenticatedGET(filterPath(analystExternalID)+"&unrecognized=%ZZ", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(agentIDs(readAgents(resp))).To(ConsistOf(analyst.ID.String()))
		})

		// US1-S2 from specs/054-agent-external-id-filter/spec.md
		It("should return an empty array for absent and non-exact IDs", func() {
			for _, value := range []string{
				missingExternalID,
				strings.ToUpper(researchExternalID),
				" " + analystExternalID,
				"research-assistant", // canonical ID is not the external ID
				analyst.ID.String(),  // UUID is not the external ID
				"urn:example:*",      // wildcards do not match longer external IDs
			} {
				resp, err := server.AuthenticatedGET(filterPath(value), principal)
				Expect(err).NotTo(HaveOccurred())
				Expect(readAgents(resp)).To(BeEmpty(), value)
			}
		})
	})

	// US1-S3 from specs/054-agent-external-id-filter/spec.md
	It("should return every agent sharing an external ID and no others", func() {
		resp, err := server.AuthenticatedGET(filterPath(researchExternalID), principal)
		Expect(err).NotTo(HaveOccurred())
		agents := readAgents(resp)
		Expect(agentIDs(agents)).To(ConsistOf(research.ID.String(), researchEU.ID.String()))
		for _, agent := range agents {
			Expect(agent["external_id"]).To(Equal(researchExternalID))
		}
	})

	Describe("when the external ID filter is unusable", func() {
		// US2-S1 from specs/054-agent-external-id-filter/spec.md
		It("should reject a present but empty external ID", func() {
			for _, path := range []string{"/api/agents?external_id=", "/api/agents?external_id", "/api/agents?%65xternal_id="} {
				expectInvalidFilter(path)
			}
		})

		// US2-S2 from specs/054-agent-external-id-filter/spec.md
		It("should reject repeated external IDs even if identical", func() {
			value := url.QueryEscape(researchExternalID)
			expectInvalidFilter("/api/agents?external_id=" + value + "&external_id=" + value)
			expectInvalidFilter("/api/agents?external_id=" + value + "&%65xternal_id=" + value)
		})

		// US2-S3 from specs/054-agent-external-id-filter/spec.md
		It("should reject overlong, invalid UTF-8, and NUL values", func() {
			for _, path := range []string{
				filterPath(strings.Repeat("界", 256)), // count Unicode code points, not bytes
				"/api/agents?external_id=%FF",
				"/api/agents?external_id=a%00b",
			} {
				expectInvalidFilter(path)
			}
		})

		// US2-S4 from specs/054-agent-external-id-filter/spec.md
		It("should reject malformed escapes and raw semicolons in the filter", func() {
			for _, path := range []string{
				"/api/agents?external_id=urn%3Aexample%3",
				"/api/agents?external_id=urn:example:x;rev=2",
			} {
				expectInvalidFilter(path)
			}
		})
	})

	// US3-S1 from specs/054-agent-external-id-filter/spec.md
	It("should retain the complete list when no external ID is supplied", func() {
		for _, path := range []string{"/api/agents", "/api/agents?unrecognized=value&malformed=%ZZ"} {
			resp, err := server.AuthenticatedGET(path, principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(agentIDs(readAgents(resp))).To(ConsistOf(
				analyst.ID.String(), research.ID.String(), researchEU.ID.String(), withoutID.ID.String(),
			))
		}
	})

	Describe("when canonical nested references are requested", func() {
		var releaseWriter *storage.Agent
		BeforeEach(func() {
			releaseWriter = createAgent("Release Notes Writer", "release-notes-writer", missingExternalID)
		})

		// US3-S2 from specs/054-agent-external-id-filter/spec.md
		It("should apply the canonical permission set reference to filtered results", func() {
			resp, err := server.DirectRequest(http.MethodGet, filterPath(missingExternalID), principal,
				map[string]string{"Prefer": "reference-id=canonical"}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Header.Get("Preference-Applied")).To(Equal("reference-id=canonical"))
			agents := readAgents(resp)
			Expect(agentIDs(agents)).To(ConsistOf(releaseWriter.ID.String()))
			Expect(agents[0]["external_id"]).To(Equal(missingExternalID))
			Expect(agents[0]["permission_sets"]).To(ConsistOf(HaveKeyWithValue("permission_set_id", "github-read")))
		})
	})
})
