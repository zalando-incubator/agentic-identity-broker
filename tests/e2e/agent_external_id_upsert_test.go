package e2e_test

import (
	"bytes"
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
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Admin agent create-or-update by external ID", func() {
	const selector = id.ExternalID("urn:example:ai-registry:agent:7f3a9c")
	var (
		server          *bootstrap.TestServer
		store           *storageadapter.Adapter
		factory         *bootstrap.StorageFactory
		selected, other *storage.Agent
		grant           *storage.UserGrant
		definition      map[string]any
	)
	ctx := context.Background()
	principal := fixtures.AdminPrincipal().String()
	filterPath := func(value id.ExternalID) string {
		return "/api/agents?external_id=" + url.QueryEscape(string(value))
	}
	readObject := func(resp *http.Response, status int) map[string]any {
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(status))
		var body map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		return body
	}
	readList := func(path string) []map[string]any {
		resp, err := server.AuthenticatedGET(path, principal)
		Expect(err).To(Succeed())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		var body []map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		Expect(body).NotTo(BeNil())
		return body
	}
	put := func(body any, status int) map[string]any {
		resp, err := helpers.ReconcileAgent(server, principal, selector, body, "")
		Expect(err).To(Succeed())
		return readObject(resp, status)
	}
	write := func(method, path string, body any, status int) map[string]any {
		encoded, err := json.Marshal(body)
		Expect(err).To(Succeed())
		resp, err := server.DirectRequest(method, path, principal,
			map[string]string{"Content-Type": "application/json"}, bytes.NewReader(encoded))
		Expect(err).To(Succeed())
		return readObject(resp, status)
	}
	stored := func(agentID id.AgentID) *storage.Agent {
		agent, err := store.Agents().Get(ctx, agentID)
		Expect(err).To(Succeed())
		return agent
	}
	expectUnchanged := func(before *storage.Agent) {
		Expect(stored(before.ID)).To(Equal(before))
		if grant != nil {
			actual, err := store.UserGrants().Get(ctx, grant.ID)
			Expect(err).To(Succeed())
			Expect(actual).To(Equal(grant))
		}
	}
	seedSelected := func() {
		selected = fixtures.AgentWithExternalID(selector, fixtures.PlaceholderPermissionSetID)
		canonical := "research-assistant"
		selected.CanonicalID = &canonical
		Expect(store.Agents().Create(ctx, selected)).To(Succeed())
		grant = fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), selected.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"})
		Expect(store.UserGrants().Create(ctx, grant)).To(Succeed())
		var err error
		grant, err = store.UserGrants().Get(ctx, grant.ID)
		Expect(err).To(Succeed())
	}
	BeforeEach(func() {
		selected, other, grant = nil, nil, nil
		logger := bootstrap.TestLogger(slog.LevelWarn)
		factory = bootstrap.NewStorageFactory(logger)
		var err error
		store, err = factory.NewTestStorage()
		Expect(err).To(Succeed())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, store)).To(Succeed())
		permission, err := store.PermissionSets().Get(ctx, fixtures.PlaceholderPermissionSetID)
		Expect(err).To(Succeed())
		canonical := "github-read"
		permission.CanonicalID = &canonical
		Expect(store.PermissionSets().Update(ctx, permission)).To(Succeed())
		application, err := bootstrap.NewServerFactory(fixtures.LocalConfig(), logger).BuildApp(store)
		Expect(err).To(Succeed())
		server, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).To(Succeed())
		definition = map[string]any{
			"display_name": "Research Assistant", "description": "Helps with academic research",
			"permission_sets": []map[string]any{{"permission_set_id": "github-read", "requirement_type": "mandatory"}},
		}
	})
	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if store != nil {
			Expect(factory.CloseStorage(store)).To(Succeed())
		}
	})

	Context("US1: reconcile an agent in one retry-safe call", func() {
		// Scenario US1.1 from specs/055-agent-external-id-upsert/spec.md
		It("should create the agent with 201 and a UUID Location when no agent holds the external ID", func() {
			resp, err := helpers.ReconcileAgent(server, principal, selector, definition, "")
			Expect(err).To(Succeed())
			body := readObject(resp, http.StatusCreated)
			agentID, err := id.ParseAgentID(body["id"].(string))
			Expect(err).To(Succeed())
			Expect(resp.Header.Get("Location")).To(Equal("/api/agents/" + agentID.String()))
			Expect(body["external_id"]).To(Equal(string(selector)))
			Expect(body).NotTo(HaveKey("client_id"))
			Expect(body["created_at"]).To(Equal(body["updated_at"]))
			Expect(readList(filterPath(selector))).To(ConsistOf(body))
			Expect(stored(agentID).PermissionSets).To(HaveLen(1))
		})

		Context("when the selected agent has identities, metadata, collections and a grant", func() {
			BeforeEach(func() {
				seedSelected()
				clientID := id.ClientID("research-upstream")
				selected.ClientID = &clientID
				governance, docs, api := "https://example.com/research/governance", "https://example.com/research/docs", "https://example.com/research/api"
				selected.GovernanceURL, selected.UserDocumentationURL, selected.AgentInterfaceURL = &governance, &docs, &api
				selected.AllowedScopes = []string{"openid"}
				selected.ServiceRequirements = []storage.ServiceRequirement{{ServiceID: fixtures.PlaceholderServiceID, RequirementType: storage.RequirementTypeMandatory}}
				Expect(store.Agents().Update(ctx, selected)).To(Succeed())
				selected = stored(selected.ID)
			})

			// Scenario US1.5 from specs/055-agent-external-id-upsert/spec.md
			It("should preserve identities and clear metadata and collections when optional fields are omitted", func() {
				before := selected.Copy()
				body := put(definition, http.StatusOK)
				actual := stored(selected.ID)
				Expect(actual.CanonicalID).To(Equal(before.CanonicalID))
				Expect(actual.ClientID).To(Equal(before.ClientID))
				Expect(actual.ExternalID).To(Equal(before.ExternalID))
				Expect(actual.CreatedAt).To(Equal(before.CreatedAt))
				Expect(actual.UpdatedAt.After(before.UpdatedAt)).To(BeTrue())
				Expect(actual.GovernanceURL).To(BeNil())
				Expect(actual.UserDocumentationURL).To(BeNil())
				Expect(actual.AgentInterfaceURL).To(BeNil())
				Expect(actual.ServiceRequirements).To(BeEmpty())
				Expect(actual.RedirectURIs).To(BeEmpty())
				Expect(actual.AllowedScopes).To(BeEmpty())
				Expect(actual.ClientURIs).To(BeEmpty())
				Expect(put(definition, http.StatusOK)).To(Equal(body))
				expectUnchanged(actual)
			})

			// Scenario US1.6 from specs/055-agent-external-id-upsert/spec.md
			It("should clear the canonical ID, metadata, and collections on explicit nulls without touching grants", func() {
				before := selected.Copy()
				for _, field := range []string{"canonical_id", "governance_url", "user_documentation_url", "agent_interface_url", "service_requirements", "redirect_uris", "allowed_scopes", "client_uris"} {
					definition[field] = nil
				}
				body := put(definition, http.StatusOK)
				actual := stored(selected.ID)
				Expect(body).To(HaveKeyWithValue("canonical_id", BeNil()))
				Expect(actual.CanonicalID).To(BeNil())
				Expect(actual.ClientID).To(Equal(before.ClientID))
				Expect(actual.ExternalID).To(Equal(before.ExternalID))
				Expect(actual.CreatedAt).To(Equal(before.CreatedAt))
				for _, field := range []string{"governance_url", "user_documentation_url", "agent_interface_url", "service_requirements", "redirect_uris", "allowed_scopes", "client_uris"} {
					Expect(body).NotTo(HaveKey(field))
				}
				Expect(put(definition, http.StatusOK)).To(Equal(body))
				expectUnchanged(actual)
			})

			// Scenario US1.7 from specs/055-agent-external-id-upsert/spec.md
			It("should reject a null client_id with 400 and change nothing", func() {
				before := selected.Copy()
				definition["client_id"] = nil
				Expect(put(definition, http.StatusBadRequest)).To(HaveKeyWithValue("error", "validation failed"))
				expectUnchanged(before)
				definition["client_id"] = "research-replacement"
				Expect(put(definition, http.StatusOK)).To(HaveKeyWithValue("client_id", "research-replacement"))
				actual := stored(before.ID)
				Expect(actual.ClientID).NotTo(BeNil())
				Expect(*actual.ClientID).To(Equal(id.ClientID("research-replacement")))
			})
		})
	})

	Context("US2: keep external identities unambiguous across all writes", func() {
		BeforeEach(func() {
			seedSelected()
			other = fixtures.AgentWithExternalID(id.ExternalID("urn:example:ai-registry:agent:1b44e0"), fixtures.PlaceholderPermissionSetID)
			canonical := "data-analyst"
			other.CanonicalID = &canonical
			other.DisplayName = "Data Analyst"
			Expect(store.Agents().Create(ctx, other)).To(Succeed())
		})

		// Scenario US2.1 from specs/055-agent-external-id-upsert/spec.md
		It("should reject POST with an external ID held by another agent", func() {
			before := stored(selected.ID)
			definition["external_id"] = selector
			body := write(http.MethodPost, "/api/agents", definition, http.StatusConflict)
			Expect(body).To(HaveKeyWithValue("error", "conflict"))
			Expect(body).To(HaveKeyWithValue("message", "external_id is already held by another agent"))
			Expect(readList("/api/agents")).To(HaveLen(2))
			expectUnchanged(before)
		})

		// Scenario US2.2 from specs/055-agent-external-id-upsert/spec.md
		It("should reject an ID-addressed update to another agent's external ID", func() {
			beforeSelected, beforeOther := stored(selected.ID), stored(other.ID)
			definition["external_id"] = selector
			for _, address := range []string{other.ID.String(), *other.CanonicalID} {
				body := write(http.MethodPut, "/api/agents/"+address, definition, http.StatusConflict)
				Expect(body).To(HaveKeyWithValue("error", "conflict"))
				Expect(body).To(HaveKeyWithValue("message", "external_id is already held by another agent"))
				expectUnchanged(beforeSelected)
				Expect(stored(other.ID)).To(Equal(beforeOther))
			}
		})

		// Scenario US2.3 from specs/055-agent-external-id-upsert/spec.md
		It("should allow an agent to keep its own external ID on update", func() {
			definition["external_id"] = selector
			definition["display_name"] = "Research Assistant v2"
			body := write(http.MethodPut, "/api/agents/"+selected.ID.String(), definition, http.StatusOK)
			Expect(body["id"]).To(Equal(selected.ID.String()))
			Expect(body["external_id"]).To(Equal(string(selector)))
			Expect(stored(selected.ID).DisplayName).To(Equal("Research Assistant v2"))
			Expect(readList(filterPath(selector))).To(HaveLen(1))
		})

		// Scenario US2.4 from specs/055-agent-external-id-upsert/spec.md
		It("should reject missing, empty, repeated, overlong, invalid UTF-8, NUL, and malformed selectors", func() {
			beforeSelected, beforeOther := stored(selected.ID), stored(other.ID)
			for _, path := range []string{
				"/api/agents", "/api/agents?externalId=x", "/api/agents?external_id=", "/api/agents?external_id",
				"/api/agents?external_id=x&%65xternal_id=x", "/api/agents?external_id=%FF", "/api/agents?external_id=a%00b",
				"/api/agents?external_id=%ZZ", "/api/agents?external_id=a;revision=1", filterPath(id.ExternalID(strings.Repeat("界", 256))),
			} {
				body := write(http.MethodPut, path, definition, http.StatusBadRequest)
				Expect(body).To(HaveKeyWithValue("error", "invalid external_id"), path)
				Expect(body).To(HaveLen(2))
				expectUnchanged(beforeSelected)
				Expect(stored(other.ID)).To(Equal(beforeOther))
			}
			Expect(readList("/api/agents")).To(HaveLen(2))
		})

		// Scenario US2.5 from specs/055-agent-external-id-upsert/spec.md
		It("should reject a body external ID that differs from, empties, or nulls the selector", func() {
			beforeSelected, beforeOther := stored(selected.ID), stored(other.ID)
			for _, bodyID := range []any{*other.ExternalID, "", nil} {
				definition["external_id"] = bodyID
				Expect(put(definition, http.StatusBadRequest)).To(HaveKeyWithValue("error", "invalid external_id"))
				expectUnchanged(beforeSelected)
				Expect(stored(other.ID)).To(Equal(beforeOther))
			}
		})

		// Scenario US2.6 from specs/055-agent-external-id-upsert/spec.md
		It("should reject an invalid agent definition without creating or changing an agent", func() {
			beforeSelected, beforeOther := stored(selected.ID), stored(other.ID)
			for _, value := range []id.ExternalID{selector, "urn:example:missing"} {
				for _, invalid := range []map[string]any{
					{"description": "valid description", "permission_sets": definition["permission_sets"]},
					{"display_name": "valid name", "permission_sets": definition["permission_sets"]},
					{"display_name": "valid name", "description": "valid description", "permission_sets": []any{}},
					{"display_name": "valid name", "description": "valid description", "permission_sets": []map[string]any{{"permission_set_id": id.NewPermissionSetID().String(), "requirement_type": "mandatory"}}},
					{"display_name": "valid name", "description": "valid description", "permission_sets": definition["permission_sets"], "governance_url": "not-a-url"},
					{"display_name": "valid name", "description": "valid description", "permission_sets": definition["permission_sets"], "service_requirements": []map[string]any{{"service_id": id.NewServiceID().String(), "requirement_type": "mandatory"}}},
				} {
					resp, err := helpers.ReconcileAgent(server, principal, value, invalid, "")
					Expect(err).To(Succeed())
					Expect(readObject(resp, http.StatusBadRequest)).To(HaveKeyWithValue("error", "validation failed"))
					expectUnchanged(beforeSelected)
					Expect(stored(other.ID)).To(Equal(beforeOther))
				}
			}
			Expect(readList("/api/agents")).To(HaveLen(2))
		})

		// Scenario US2.7 from specs/055-agent-external-id-upsert/spec.md
		It("should return 409 for canonical and client ID conflicts on both branches", func() {
			clientID := id.ClientID("analyst-upstream")
			other.ClientID = &clientID
			Expect(store.Agents().Update(ctx, other)).To(Succeed())
			beforeSelected, beforeOther := stored(selected.ID), stored(other.ID)
			for _, value := range []id.ExternalID{selector, "urn:example:missing"} {
				for _, field := range []string{"canonical_id", "client_id"} {
					if field == "canonical_id" {
						definition[field] = *other.CanonicalID
					} else {
						definition[field] = clientID
					}
					resp, err := helpers.ReconcileAgent(server, principal, value, definition, "")
					Expect(err).To(Succeed())
					Expect(readObject(resp, http.StatusConflict)).To(HaveKeyWithValue("error", "conflict"))
					delete(definition, field)
					expectUnchanged(beforeSelected)
					Expect(stored(other.ID)).To(Equal(beforeOther))
				}
			}
			Expect(readList("/api/agents")).To(HaveLen(2))
		})

		// Scenario US2.8 from specs/055-agent-external-id-upsert/spec.md
		It("should allow many agents without an external ID and reject the empty string", func() {
			before := stored(selected.ID)
			for i := 0; i < 3; i++ {
				delete(definition, "external_id")
				write(http.MethodPost, "/api/agents", definition, http.StatusCreated)
				definition["external_id"] = nil
				write(http.MethodPost, "/api/agents", definition, http.StatusCreated)
			}
			definition["external_id"] = ""
			for _, operation := range []struct{ method, path string }{{http.MethodPost, "/api/agents"}, {http.MethodPut, "/api/agents/" + selected.ID.String()}} {
				Expect(write(operation.method, operation.path, definition, http.StatusBadRequest)).To(HaveKeyWithValue("error", "validation failed"))
			}
			Expect(readList("/api/agents")).To(HaveLen(8))
			expectUnchanged(before)
		})
	})

	Context("US4: keep lookup and reference presentation compatible", func() {
		BeforeEach(seedSelected)

		// Scenario US4.1 from specs/055-agent-external-id-upsert/spec.md
		It("should return an empty or one-element array for a valid filter", func() {
			before := stored(selected.ID)
			Expect(readList(filterPath(selector))).To(ConsistOf(HaveKeyWithValue("id", selected.ID.String())))
			for _, value := range []id.ExternalID{"urn:example:missing", id.ExternalID(strings.ToUpper(string(selector))), id.ExternalID(" " + string(selector)), id.ExternalID(selected.ID.String()), id.ExternalID(*selected.CanonicalID)} {
				Expect(readList(filterPath(value))).To(BeEmpty())
			}
			expectUnchanged(before)
		})

		// Scenario US4.2 from specs/055-agent-external-id-upsert/spec.md
		It("should keep the empty-filter 400 and the unfiltered list", func() {
			resp, err := server.AuthenticatedGET("/api/agents?external_id=", principal)
			Expect(err).To(Succeed())
			Expect(readObject(resp, http.StatusBadRequest)).To(HaveKeyWithValue("error", "invalid external_id"))
			Expect(readList("/api/agents")).To(ConsistOf(HaveKeyWithValue("id", selected.ID.String())))
		})

		// Scenario US4.3 from specs/055-agent-external-id-upsert/spec.md
		It("should honor canonical references on create-or-update responses and replays", func() {
			newSelector := id.ExternalID("urn:example:release-writer")
			definition["client_uris"] = []string{"https://agent.example.com/z-client.json", "https://agent.example.com/a-client.json"}
			resp, err := helpers.ReconcileAgent(server, principal, newSelector, definition, "reference-id=canonical")
			Expect(err).To(Succeed())
			body := readObject(resp, http.StatusCreated)
			agentID, err := id.ParseAgentID(body["id"].(string))
			Expect(err).To(Succeed())
			Expect(resp.Header.Get("Location")).To(Equal("/api/agents/" + agentID.String()))
			Expect(resp.Header.Get("Preference-Applied")).To(Equal("reference-id=canonical"))
			Expect(resp.Header.Values("Vary")).To(ContainElement(ContainSubstring("Prefer")))
			Expect(body["permission_sets"]).To(ConsistOf(HaveKeyWithValue("permission_set_id", "github-read")))
			Expect(body["client_uris"]).To(Equal([]any{"https://agent.example.com/a-client.json", "https://agent.example.com/z-client.json"}))
			Expect(stored(agentID).PermissionSets[0].PermissionSetID).To(Equal(fixtures.PlaceholderPermissionSetID))
			for _, name := range []string{"Research Assistant", "Release Notes Writer", "Release Notes Writer"} {
				definition["display_name"] = name
				resp, err = helpers.ReconcileAgent(server, principal, newSelector, definition, "reference-id=canonical")
				Expect(err).To(Succeed())
				actual := readObject(resp, http.StatusOK)
				Expect(actual["id"]).To(Equal(body["id"]))
				Expect(actual["created_at"]).To(Equal(body["created_at"]))
				if name == body["display_name"] {
					Expect(actual).To(Equal(body))
				}
				Expect(resp.Header.Get("Preference-Applied")).To(Equal("reference-id=canonical"))
				Expect(actual["permission_sets"]).To(Equal(body["permission_sets"]))
				Expect(actual["client_uris"]).To(Equal(body["client_uris"]))
				body = actual
			}
		})
	})
})
