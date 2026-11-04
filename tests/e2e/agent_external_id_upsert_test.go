package e2e_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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

		// Scenario US1.2 from specs/055-agent-external-id-upsert/spec.md
		It("should return 200 with an unchanged representation and timestamps on replay", Label("docker"), func() {
			for _, backend := range []string{"memory", "postgres"} {
				withExternalIDBackend(backend, func(s *bootstrap.TestServer, st *storageadapter.Adapter) {
					definition["client_uris"] = []string{"https://agent.example/z.json", "https://agent.example/a.json"}
					resp, err := helpers.ReconcileAgent(s, principal, selector, definition, "")
					Expect(err).To(Succeed())
					created := readObject(resp, http.StatusCreated)
					Expect(created["client_uris"]).To(Equal([]any{"https://agent.example/a.json", "https://agent.example/z.json"}))
					agentID, err := id.ParseAgentID(created["id"].(string))
					Expect(err).To(Succeed())
					beforeReplay, err := st.Agents().Get(ctx, agentID)
					Expect(err).To(Succeed())
					Expect(created["created_at"]).To(Equal(beforeReplay.CreatedAt.UTC().Format(time.RFC3339)))
					Expect(created["updated_at"]).To(Equal(beforeReplay.UpdatedAt.UTC().Format(time.RFC3339)))
					for range 10 {
						resp, err := helpers.ReconcileAgent(s, principal, selector, definition, "")
						Expect(err).To(Succeed())
						Expect(readObject(resp, http.StatusOK)).To(Equal(created))
					}
					found, err := externalIDMatches(ctx, st.Agents(), selector)
					Expect(err).To(Succeed())
					Expect(found).To(HaveLen(1))
					Expect(found[0]).To(Equal(beforeReplay))
					resp, err = s.AuthenticatedGET("/api/agents/"+agentID.String(), principal)
					Expect(err).To(Succeed())
					Expect(readObject(resp, http.StatusOK)).To(Equal(created))
				})
			}
		})

		// Scenario US1.3 from specs/055-agent-external-id-upsert/spec.md
		It("should update the same agent and preserve its identity and creation time when the definition changes", Label("docker"), func() {
			for _, backend := range []string{"memory", "postgres"} {
				withExternalIDBackend(backend, func(s *bootstrap.TestServer, st *storageadapter.Adapter) {
					definition["display_name"] = "Research Assistant"
					delete(definition, "client_uris")
					resp, err := helpers.ReconcileAgent(s, principal, selector, definition, "")
					Expect(err).To(Succeed())
					created := readObject(resp, http.StatusCreated)
					agentID, err := id.ParseAgentID(created["id"].(string))
					Expect(err).To(Succeed())
					a, err := st.Agents().Get(ctx, agentID)
					Expect(err).To(Succeed())
					a.UpdatedAt = time.Now().UTC().Add(time.Hour)
					Expect(st.Agents().Update(ctx, a)).To(Succeed())
					definition["display_name"] = "Changed Research Assistant"
					definition["client_uris"] = []string{"https://agent.example/z.json", "https://agent.example/a.json"}
					started := time.Now().UTC()
					resp, err = helpers.ReconcileAgent(s, principal, selector, definition, "")
					Expect(err).To(Succeed())
					changed := readObject(resp, http.StatusOK)
					finished := time.Now().UTC()
					Expect(changed["display_name"]).To(Equal("Changed Research Assistant"))
					Expect(changed["id"]).To(Equal(created["id"]))
					Expect(changed["created_at"]).To(Equal(created["created_at"]))
					updated, err := time.Parse(time.RFC3339, changed["updated_at"].(string))
					Expect(err).To(Succeed())
					Expect(changed["updated_at"]).To(Equal(updated.UTC().Format(time.RFC3339)))
					Expect(updated.Before(started.Truncate(time.Second))).To(BeFalse())
					Expect(updated.After(finished)).To(BeFalse())
					saved, err := st.Agents().Get(ctx, agentID)
					Expect(err).To(Succeed())
					Expect(saved.UpdatedAt.Before(started.Add(-time.Microsecond))).To(BeFalse())
					Expect(saved.UpdatedAt.After(finished.Add(time.Microsecond))).To(BeFalse())
					Expect(changed["client_uris"]).To(Equal([]any{"https://agent.example/a.json", "https://agent.example/z.json"}))
					definition["client_uris"] = []string{"https://agent.example/a.json", "https://agent.example/z.json"}
					for range 10 {
						resp, err = helpers.ReconcileAgent(s, principal, selector, definition, "")
						Expect(err).To(Succeed())
						Expect(readObject(resp, http.StatusOK)).To(Equal(changed))
					}
					afterReplay, err := st.Agents().Get(ctx, agentID)
					Expect(err).To(Succeed())
					Expect(afterReplay).To(Equal(saved))
					resp, err = s.AuthenticatedGET("/api/agents/"+agentID.String(), principal)
					Expect(err).To(Succeed())
					Expect(readObject(resp, http.StatusOK)).To(Equal(changed))
				})
			}
		})

		// Scenario US1.4 from specs/055-agent-external-id-upsert/spec.md
		It("should create exactly one agent when two PostgreSQL brokers race on the first call", Label("docker"), func() {
			withExternalIDPostgresBrokers(func(first, second *bootstrap.TestServer, st *storageadapter.Adapter) {
				for _, calls := range []int{2, 10} {
					key := id.ExternalID(id.NewAgentID().String())
					var wg sync.WaitGroup
					results := make(chan *http.Response, calls)
					for i := range calls {
						wg.Go(func() {
							defer GinkgoRecover()
							target := first
							if i%2 == 1 {
								target = second
							}
							resp, err := helpers.ReconcileAgent(target, principal, key, definition, "")
							Expect(err).To(Succeed())
							results <- resp
						})
					}
					wg.Wait()
					close(results)
					created := 0
					var identity any
					for resp := range results {
						if resp.StatusCode == http.StatusCreated {
							created++
						} else {
							Expect(resp.StatusCode).To(Equal(http.StatusOK))
						}
						body := readObject(resp, resp.StatusCode)
						if identity == nil {
							identity = body["id"]
						}
						Expect(body["id"]).To(Equal(identity))
					}
					Expect(created).To(Equal(1))
					found, err := externalIDMatches(ctx, st.Agents(), key)
					Expect(err).To(Succeed())
					Expect(found).To(HaveLen(1))
					Expect(found[0].ID.String()).To(Equal(identity))
				}
				key := id.ExternalID("mixed-definitions")
				var wg sync.WaitGroup
				for i := range 10 {
					wg.Go(func() {
						defer GinkgoRecover()
						body := map[string]any{"display_name": fmt.Sprintf("Definition %d", i), "description": fmt.Sprintf("Description %d", i), "permission_sets": definition["permission_sets"]}
						target := first
						if i%2 == 1 {
							target = second
						}
						resp, err := helpers.ReconcileAgent(target, principal, key, body, "")
						Expect(err).To(Succeed())
						Expect(resp.StatusCode).To(BeElementOf(http.StatusOK, http.StatusCreated))
						readObject(resp, resp.StatusCode)
					})
				}
				wg.Wait()
				found, err := externalIDMatches(ctx, st.Agents(), key)
				Expect(err).To(Succeed())
				Expect(found).To(HaveLen(1))
				Expect(found[0].Description).To(Equal(strings.Replace(found[0].DisplayName, "Definition", "Description", 1)))
			})
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
			governance := "https://research.example/governance"
			selected.GovernanceURL = &governance
			selected.RedirectURIs = []string{"https://research.example/callback"}
			selected.AllowedScopes = []string{"read"}
			selected.ClientURIs = []string{"https://research.example/client.json"}
			selected.ServiceRequirements = []storage.ServiceRequirement{{ServiceID: fixtures.PlaceholderServiceID, RequirementType: storage.RequirementTypeOptional}}
			Expect(store.Agents().Update(ctx, selected)).To(Succeed())
			selected = stored(selected.ID)
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

			other.ClientID = nil
			other.ClientURIs = []string{"https://analyst.example/client.json"}
			Expect(store.Agents().Update(ctx, other)).To(Succeed())
			beforeOther = stored(other.ID)
			definition["client_uris"] = other.ClientURIs
			for _, value := range []id.ExternalID{selector, "urn:example:missing"} {
				resp, err := helpers.ReconcileAgent(server, principal, value, definition, "")
				Expect(err).To(Succeed())
				Expect(readObject(resp, http.StatusConflict)).To(HaveKeyWithValue("error", "conflict"))
				expectUnchanged(beforeSelected)
				Expect(stored(other.ID)).To(Equal(beforeOther))
			}
			delete(definition, "client_uris")
			other.ClientURIs = nil
			other.ClientID = &clientID
			Expect(store.Agents().Update(ctx, other)).To(Succeed())
			beforeOther = stored(other.ID)
			cfg := fixtures.DefaultOAuth2Config()
			cfg.OAuth2AuthServer.MultiAgentClient = fixtures.MultiAgentEnabledConfig("http://upstream.example").OAuth2AuthServer.MultiAgentClient
			logger := bootstrap.TestLogger(slog.LevelWarn)
			application, err := bootstrap.NewServerFactory(cfg, logger).BuildApp(store)
			Expect(err).To(Succeed())
			sharingServer, err := bootstrap.NewAdminTestServer(application, logger)
			Expect(err).To(Succeed())
			defer sharingServer.Close()
			definition["client_id"] = clientID
			for i, value := range []id.ExternalID{selector, "urn:example:sharing"} {
				resp, err := helpers.ReconcileAgent(sharingServer, principal, value, definition, "")
				Expect(err).To(Succeed())
				status := http.StatusOK
				if i == 1 {
					status = http.StatusCreated
				}
				Expect(readObject(resp, status)).To(HaveKeyWithValue("client_id", string(clientID)))
				Expect(stored(other.ID)).To(Equal(beforeOther))
				definition["canonical_id"] = *other.CanonicalID
				resp, err = helpers.ReconcileAgent(sharingServer, principal, value, definition, "")
				Expect(err).To(Succeed())
				Expect(readObject(resp, http.StatusConflict)).To(HaveKeyWithValue("error", "conflict"))
				delete(definition, "canonical_id")
			}
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

	Context("US3: upgrade safely without losing governed agents", Label("docker"), func() {
		var (
			postgres     *bootstrap.PostgresFixture
			db           *sql.DB
			legacyStore  *storageadapter.Adapter
			legacyServer *bootstrap.TestServer
			legacy       []*storage.Agent
		)
		BeforeEach(func() {
			var err error
			postgres, err = bootstrap.NewPostgresFixtureAtVersion(ctx, 46)
			Expect(err).To(Succeed())
			Expect(postgres.RecoverExternalIDMigration(ctx)).NotTo(Succeed())
			legacyStore = externalIDPostgresStorage(postgres)
			legacyServer = externalIDAdminServer(legacyStore)
			db, err = sql.Open("pgx", postgres.ConnectionURL)
			Expect(err).To(Succeed())
			legacy = nil
			for range 3 {
				a := fixtures.AgentWithExternalID(id.ExternalID(id.NewAgentID().String()), fixtures.PlaceholderPermissionSetID)
				a.ServiceRequirements = []storage.ServiceRequirement{{ServiceID: fixtures.PlaceholderServiceID, RequirementType: storage.RequirementTypeOptional}}
				a.ClientURIs = []string{"https://agent.example/" + id.NewAgentID().String() + ".json"}
				Expect(legacyStore.Agents().Create(ctx, a)).To(Succeed())
				g := fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), a.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"})
				Expect(legacyStore.UserGrants().Create(ctx, g)).To(Succeed())
				legacy = append(legacy, a)
			}
		})
		AfterEach(func() {
			if legacyServer != nil {
				legacyServer.Close()
			}
			if db != nil {
				Expect(db.Close()).To(Succeed())
			}
			if legacyStore != nil {
				Expect(legacyStore.Close(ctx)).To(Succeed())
			}
			if postgres != nil {
				Expect(postgres.Close(ctx)).To(Succeed())
			}
		})
		setID := func(agent *storage.Agent, value any) {
			_, err := db.ExecContext(ctx, "UPDATE agents SET external_id=$1 WHERE id=$2", value, agent.ID)
			Expect(err).To(Succeed())
		}
		snapshot := func() string {
			var result string
			err := db.QueryRowContext(ctx, `SELECT jsonb_build_object(
				'agents',(SELECT jsonb_agg(jsonb_build_object('id',id,'external_id',external_id,'permission_sets',permission_sets,'service_requirements',service_requirements,'created_at',created_at,'updated_at',updated_at) ORDER BY id) FROM agents),
				'grants',(SELECT jsonb_agg(jsonb_build_object('id',id,'agent_id',agent_id,'permission_sets',granted_permission_sets,'created_at',created_at,'updated_at',updated_at) ORDER BY id) FROM user_grants),
				'uris',(SELECT jsonb_agg(jsonb_build_object('agent_id',agent_id,'client_uri',client_uri) ORDER BY agent_id,client_uri) FROM agent_client_uris))::text`).Scan(&result)
			Expect(err).To(Succeed())
			return result
		}
		expectBlocked := func(affected ...*storage.Agent) {
			before := snapshot()
			err := postgres.MigrateTo(0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("agent external_id uniqueness migration blocked"))
			for _, a := range affected {
				Expect(err.Error()).To(ContainSubstring(a.ID.String()))
			}
			Expect(snapshot()).To(Equal(before))
			var newIndex, oldIndex bool
			Expect(db.QueryRowContext(ctx, `SELECT to_regclass('uq_agents_external_id') IS NOT NULL, to_regclass('idx_agents_external_id') IS NOT NULL`).Scan(&newIndex, &oldIndex)).To(Succeed())
			Expect(newIndex).To(BeFalse())
			Expect(oldIndex).To(BeTrue())
			version, dirty, err := postgres.MigrationVersion()
			Expect(err).To(Succeed())
			Expect(version).To(Equal(uint(47)))
			Expect(dirty).To(BeTrue())
		}
		// Scenario US3.1 from specs/055-agent-external-id-upsert/spec.md
		It("should block the migration on duplicates and report the value and agent IDs", func() {
			setID(legacy[0], string(selector))
			setID(legacy[1], string(selector))
			expectBlocked(legacy[0], legacy[1])
			resp, err := legacyServer.AuthenticatedGET("/api/agents/"+legacy[0].ID.String(), principal)
			Expect(err).To(Succeed())
			Expect(readObject(resp, http.StatusOK)["external_id"]).To(Equal(string(selector)))
		})
		// Scenario US3.2 from specs/055-agent-external-id-upsert/spec.md
		It("should block the migration on empty values and report both categories", func() {
			setID(legacy[0], string(selector))
			setID(legacy[1], string(selector))
			setID(legacy[2], "")
			expectBlocked(legacy...)
		})
		// Scenario US3.3 from specs/055-agent-external-id-upsert/spec.md
		It("should succeed after deliberate remediation and then enforce uniqueness", func() {
			setID(legacy[0], string(selector))
			setID(legacy[1], string(selector))
			setID(legacy[2], "")
			expectBlocked(legacy...)
			for i, a := range legacy[1:] {
				desired := map[string]any{"display_name": a.DisplayName, "description": a.Description, "permission_sets": definition["permission_sets"], "client_uris": a.ClientURIs, "service_requirements": a.ServiceRequirements}
				if i == 0 {
					desired["external_id"] = string(selector) + "-eu"
				}
				encoded, err := json.Marshal(desired)
				Expect(err).To(Succeed())
				resp, err := legacyServer.DirectRequest(http.MethodPut, "/api/agents/"+a.ID.String(), principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(encoded))
				Expect(err).To(Succeed())
				Expect(readObject(resp, http.StatusOK)["id"]).To(Equal(a.ID.String()))
			}
			before := snapshot()
			Expect(postgres.RecoverExternalIDMigration(ctx)).To(Succeed())
			Expect(postgres.MigrateTo(0)).To(Succeed())
			Expect(snapshot()).To(Equal(before))
			definition["external_id"] = string(selector)
			encoded, err := json.Marshal(definition)
			Expect(err).To(Succeed())
			resp, err := legacyServer.DirectRequest(http.MethodPost, "/api/agents", principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(encoded))
			Expect(err).To(Succeed())
			Expect(readObject(resp, http.StatusConflict)["error"]).To(Equal("conflict"))
			Expect(snapshot()).To(Equal(before))
		})
		// Scenario US3.4 from specs/055-agent-external-id-upsert/spec.md
		It("should preserve agents and relationships across rollback and reapply", func() {
			setID(legacy[2], nil)
			before := snapshot()
			Expect(postgres.MigrateTo(0)).To(Succeed())
			Expect(snapshot()).To(Equal(before))
			Expect(postgres.MigrateTo(46)).To(Succeed())
			Expect(snapshot()).To(Equal(before))
			Expect(postgres.MigrateTo(0)).To(Succeed())
			Expect(snapshot()).To(Equal(before))
			resp, err := legacyServer.AuthenticatedGET("/api/agents/"+legacy[0].ID.String(), principal)
			Expect(err).To(Succeed())
			Expect(readObject(resp, http.StatusOK)["id"]).To(Equal(legacy[0].ID.String()))

			definition["external_id"] = string(*legacy[0].ExternalID)
			encoded, err := json.Marshal(definition)
			Expect(err).To(Succeed())
			resp, err = legacyServer.DirectRequest(http.MethodPost, "/api/agents", principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(encoded))
			Expect(err).To(Succeed())
			Expect(readObject(resp, http.StatusConflict)["error"]).To(Equal("conflict"))
			Expect(snapshot()).To(Equal(before))

			delete(definition, "external_id")
			encoded, err = json.Marshal(definition)
			Expect(err).To(Succeed())
			resp, err = legacyServer.DirectRequest(http.MethodPost, "/api/agents", principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(encoded))
			Expect(err).To(Succeed())
			Expect(readObject(resp, http.StatusCreated)["external_id"]).To(BeNil())
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
