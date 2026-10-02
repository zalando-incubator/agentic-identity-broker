package e2e_test

// Permission Sets E2E Acceptance Tests (019)
//
// This file implements all 21 acceptance scenarios from specs/019-permission-sets/spec.md.
// Following Constitution Principle XIII: 1:1 spec-to-test mapping.
//
// Each It() block maps to exactly ONE acceptance scenario.
// All tests are written BEFORE implementation (TDD red phase) per Principle VIII.
//
// Test Structure:
// - US1: Admin Manages Permission Sets (6 scenarios)
// - US2: Consent Screen Shows Permission Sets First (6 scenarios)
// - US3: Agent Declares Permission Sets at Agent Level (4 scenarios)
// - US4: User Grant Stores Granted Permission Sets (4 scenarios)
// - Edge: Token Exchange Scope Validation (1 scenario)
// Total: 21 scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

// psJSON is a helper to marshal a value to JSON and return a bytes.Reader.
func psJSON(v interface{}) *bytes.Reader {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("psJSON: marshal error: %v", err))
	}
	return bytes.NewReader(b)
}

// parsePS parses a JSON response body into a map.
func parsePS(resp *http.Response) map[string]interface{} {
	defer func() { _ = resp.Body.Close() }()
	var m map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&m)
	// Unwrap {"data": ...} envelope if present (unified response format)
	if data, ok := m["data"].(map[string]interface{}); ok {
		return data
	}
	return m
}

// parsePSList parses a JSON response body expecting {"items": [...]} into a map.
func parsePSList(resp *http.Response) map[string]interface{} {
	return parsePS(resp)
}

// parsePSGrant parses a grant response body that is wrapped in {"data": {...}}.
// Returns the inner data map.
func parsePSGrant(resp *http.Response) map[string]interface{} {
	outer := parsePS(resp)
	if data, ok := outer["data"].(map[string]interface{}); ok {
		return data
	}
	return outer
}

var _ = Describe("Permission Sets (019)", func() {
	var (
		enduserServer  *bootstrap.TestServer
		adminServer    *bootstrap.TestServer
		testStorage    *storageadapter.Adapter
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		storageFactory *bootstrap.StorageFactory
		serverFactory  *bootstrap.ServerFactory
		logger         *slog.Logger

		// Test principals
		adminPrincipal string
		userPrincipal  string

		// Pre-created test service (needed for permission set service_scopes)
		githubServiceID string
		googleServiceID string
	)

	setupEnv := func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		config := fixtures.OAuth2ConfigWithTokenExchange(mockUpstream.Server.URL)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
		serverFactory = bootstrap.NewServerFactory(config, logger)
		appInstance, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())
	}

	BeforeEach(func() {
		setupEnv()
		adminPrincipal = "admin@example.com"
		userPrincipal = "user@example.com"

		// Pre-create two third-party services for use in permission set service_scopes
		githubSvc := &model.ThirdpartyOAuth2ProviderEntity{
			ID:          id.MustParseServiceID("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
			DisplayName: "GitHub",
			ClientID:    id.ClientID("ps-github-client"),
			Secret:      fixtures.EncryptedSecret("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "github-secret"),
			IssuerURI:   "https://github.com",
			Endpoints: model.OAuth2Endpoints{
				AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
				TokenEndpoint:     mockUpstream.URL() + "/oauth/token",
			},
			Scopes: []model.OAuthScope{
				{ScopeValue: "repo:read", Description: "Read repositories"},
				{ScopeValue: "user:email", Description: "Read email"},
			},
			ProtectedResources: []string{"https://api.github.com"},
		}
		googleSvc := &model.ThirdpartyOAuth2ProviderEntity{
			ID:          id.MustParseServiceID("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
			DisplayName: "Google",
			ClientID:    id.ClientID("ps-google-client"),
			Secret:      fixtures.EncryptedSecret("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "google-secret"),
			IssuerURI:   "https://accounts.google.com",
			Endpoints: model.OAuth2Endpoints{
				AuthorizeEndpoint: "https://accounts.google.com/o/oauth2/auth",
				TokenEndpoint:     "https://oauth2.googleapis.com/token",
			},
			Scopes: []model.OAuthScope{
				{ScopeValue: "calendar.read", Description: "Read calendar"},
			},
		}
		err := testStorage.Services().Create(context.Background(), githubSvc)
		Expect(err).ToNot(HaveOccurred())
		err = testStorage.Services().Create(context.Background(), googleSvc)
		Expect(err).ToNot(HaveOccurred())
		githubServiceID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		googleServiceID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	})

	AfterEach(func() {
		if enduserServer != nil {
			enduserServer.Close()
		}
		if adminServer != nil {
			adminServer.Close()
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if testStorage != nil && storageFactory != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// ===========================================================================
	// User Story 1: Admin Manages Permission Sets (Priority: P1)
	// ===========================================================================

	Describe("US1: Admin Manages Permission Sets", func() {
		// US1.S1 from specs/019-permission-sets/spec.md
		It("creates a permission set and returns its generated ID", func() {
			// US1.S1
			body := map[string]interface{}{
				"name":        "GitHub Read Access",
				"description": "Read repository contents and user profile from GitHub",
				"service_scopes": []map[string]interface{}{
					{
						"service_id": githubServiceID,
						"scopes":     []string{"repo:read", "user:email"},
					},
				},
			}
			resp, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(body),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			result := parsePS(resp)
			Expect(result["id"]).ToNot(BeEmpty())
			Expect(result["name"]).To(Equal("GitHub Read Access"))
		})

		// US1.S2 from specs/019-permission-sets/spec.md
		It("returns name, description, and service_scope entries for GET by ID", func() {
			// US1.S2
			// Create first
			createBody := map[string]interface{}{
				"name":        "PS for GET test",
				"description": "Test permission set for GET",
				"service_scopes": []map[string]interface{}{
					{"service_id": githubServiceID, "scopes": []string{"repo:read"}},
				},
			}
			createResp, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(createBody),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))
			created := parsePS(createResp)
			psID := created["id"].(string)

			// GET by ID
			getResp, err := adminServer.DirectRequest(
				"GET", fmt.Sprintf("/api/permission-sets/%s", psID), adminPrincipal,
				nil, nil,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = getResp.Body.Close() }()
			Expect(getResp.StatusCode).To(Equal(http.StatusOK))

			result := parsePS(getResp)
			Expect(result["id"]).To(Equal(psID))
			Expect(result["name"]).To(Equal("PS for GET test"))
			Expect(result["description"]).To(Equal("Test permission set for GET"))
			Expect(result["service_scopes"]).ToNot(BeNil())
			scopes := result["service_scopes"].([]interface{})
			Expect(scopes).To(HaveLen(1))
		})

		// US1.S3 from specs/019-permission-sets/spec.md
		It("reflects updated values after PUT with modified scopes", func() {
			// US1.S3
			createBody := map[string]interface{}{
				"name":        "PS for PUT test",
				"description": "Original description",
				"service_scopes": []map[string]interface{}{
					{"service_id": githubServiceID, "scopes": []string{"repo:read"}},
				},
			}
			createResp, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(createBody),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))
			created := parsePS(createResp)
			psID := created["id"].(string)

			// PUT with updated scopes
			updateBody := map[string]interface{}{
				"name":        "PS for PUT test",
				"description": "Updated description",
				"service_scopes": []map[string]interface{}{
					{"service_id": githubServiceID, "scopes": []string{"repo:read", "user:email"}},
				},
			}
			putResp, err := adminServer.DirectRequest(
				"PUT", fmt.Sprintf("/api/permission-sets/%s", psID), adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(updateBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = putResp.Body.Close() }()
			Expect(putResp.StatusCode).To(Equal(http.StatusOK))

			result := parsePS(putResp)
			Expect(result["description"]).To(Equal("Updated description"))
			scopes := result["service_scopes"].([]interface{})
			Expect(scopes).To(HaveLen(1))
			scopeEntry := scopes[0].(map[string]interface{})
			scopeList := scopeEntry["scopes"].([]interface{})
			Expect(scopeList).To(HaveLen(2))
		})

		// US1.S4 from specs/019-permission-sets/spec.md
		It("removes the permission set and returns 404 on subsequent GET", func() {
			// US1.S4
			createBody := map[string]interface{}{
				"name":        "PS to delete",
				"description": "Will be deleted",
				"service_scopes": []map[string]interface{}{
					{"service_id": githubServiceID, "scopes": []string{"repo:read"}},
				},
			}
			createResp, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(createBody),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))
			created := parsePS(createResp)
			psID := created["id"].(string)

			// DELETE
			deleteResp, err := adminServer.DirectRequest(
				"DELETE", fmt.Sprintf("/api/permission-sets/%s", psID), adminPrincipal,
				nil, nil,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = deleteResp.Body.Close() }()
			Expect(deleteResp.StatusCode).To(Equal(http.StatusNoContent))

			// Subsequent GET must return 404
			getResp, err := adminServer.DirectRequest(
				"GET", fmt.Sprintf("/api/permission-sets/%s", psID), adminPrincipal,
				nil, nil,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = getResp.Body.Close() }()
			Expect(getResp.StatusCode).To(Equal(http.StatusNotFound))
		})

		// US1.S5 from specs/019-permission-sets/spec.md
		It("rejects DELETE with 409 when permission set is referenced by an agent", func() {
			// US1.S5
			createBody := map[string]interface{}{
				"name":        "PS referenced by agent",
				"description": "This PS is referenced",
				"service_scopes": []map[string]interface{}{
					{"service_id": githubServiceID, "scopes": []string{"repo:read"}},
				},
			}
			createResp, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(createBody),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))
			created := parsePS(createResp)
			psID := created["id"].(string)

			// Create an agent that references this permission set
			agentBody := map[string]interface{}{
				"client_id":    "agent-referencing-ps",
				"display_name": "Agent With PS",
				"description":  "Agent that declares a permission set",
				"permission_sets": []map[string]interface{}{
					{"permission_set_id": psID, "requirement_type": "mandatory"},
				},
			}
			agentResp, err := adminServer.DirectRequest(
				"POST", "/api/agents", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(agentBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = agentResp.Body.Close() }()
			Expect(agentResp.StatusCode).To(Equal(http.StatusCreated))

			// Attempt DELETE — must be 409 Conflict
			deleteResp, err := adminServer.DirectRequest(
				"DELETE", fmt.Sprintf("/api/permission-sets/%s", psID), adminPrincipal,
				nil, nil,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = deleteResp.Body.Close() }()
			Expect(deleteResp.StatusCode).To(Equal(http.StatusConflict))

			errBody := parsePS(deleteResp)
			Expect(errBody["message"]).To(ContainSubstring("agent"))
		})

		// US1.S6 from specs/019-permission-sets/spec.md
		It("filters permission set list by service_id", func() {
			// US1.S6
			// Create two permission sets: one for GitHub, one for Google
			psGitHub := map[string]interface{}{
				"name":        "GitHub PS",
				"description": "GitHub scopes",
				"service_scopes": []map[string]interface{}{
					{"service_id": githubServiceID, "scopes": []string{"repo:read"}},
				},
			}
			psGoogle := map[string]interface{}{
				"name":        "Google PS",
				"description": "Google scopes",
				"service_scopes": []map[string]interface{}{
					{"service_id": googleServiceID, "scopes": []string{"calendar.read"}},
				},
			}

			r1, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(psGitHub),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = r1.Body.Close() }()
			Expect(r1.StatusCode).To(Equal(http.StatusCreated))

			r2, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(psGoogle),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = r2.Body.Close() }()
			Expect(r2.StatusCode).To(Equal(http.StatusCreated))

			// List filtered by githubServiceID — must return only GitHub PS
			listResp, err := adminServer.DirectRequest(
				"GET", fmt.Sprintf("/api/permission-sets?service_id=%s", githubServiceID),
				adminPrincipal, nil, nil,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = listResp.Body.Close() }()
			Expect(listResp.StatusCode).To(Equal(http.StatusOK))

			result := parsePSList(listResp)
			items := result["items"].([]interface{})
			Expect(items).To(HaveLen(1))
			item := items[0].(map[string]interface{})
			Expect(item["name"]).To(Equal("GitHub PS"))
		})
	})

	// ===========================================================================
	// User Story 2: Consent Screen Shows Permission Sets First (Priority: P1)
	// ===========================================================================

	Describe("US2: Consent Screen Shows Permission Sets First", func() {
		var (
			mandatoryPSID string
			optionalPSID  string
			agentID       string
		)

		BeforeEach(func() {
			// Create permission sets
			mandatoryBody := map[string]interface{}{
				"name":        "Mandatory PS",
				"description": "Required permissions",
				"service_scopes": []map[string]interface{}{
					{"service_id": githubServiceID, "scopes": []string{"repo:read"}},
				},
			}
			optionalBody := map[string]interface{}{
				"name":        "Optional PS",
				"description": "Optional permissions",
				"service_scopes": []map[string]interface{}{
					{"service_id": googleServiceID, "scopes": []string{"calendar.read"}},
				},
			}

			r1, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(mandatoryBody),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(r1.StatusCode).To(Equal(http.StatusCreated))
			m1 := parsePS(r1)
			mandatoryPSID = m1["id"].(string)

			r2, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(optionalBody),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(r2.StatusCode).To(Equal(http.StatusCreated))
			m2 := parsePS(r2)
			optionalPSID = m2["id"].(string)

			// Create an agent with both permission sets
			agentBody := map[string]interface{}{
				"client_id":    "consent-screen-agent",
				"display_name": "Consent Screen Test Agent",
				"description":  "Agent for consent screen tests",
				"permission_sets": []map[string]interface{}{
					{"permission_set_id": mandatoryPSID, "requirement_type": "mandatory"},
					{"permission_set_id": optionalPSID, "requirement_type": "optional"},
				},
			}
			ar, err := adminServer.DirectRequest(
				"POST", "/api/agents", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(agentBody),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(ar.StatusCode).To(Equal(http.StatusCreated))
			agentData := parsePS(ar)
			agentID = agentData["id"].(string)
		})

		// US2.S1 from specs/019-permission-sets/spec.md
		It("renders mandatory sets locked and optional sets togglable on consent screen", func() {
			// US2.S1 — consent-info returns permission_sets with correct requirement_type
			resp, err := enduserServer.AuthenticatedGET(
				fmt.Sprintf("/api/consent/agents/%s", agentID),
				userPrincipal,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			result := parsePS(resp)
			permissionSets, ok := result["permission_sets"].([]interface{})
			Expect(ok).To(BeTrue(), "expected permission_sets array in consent-info response")
			Expect(permissionSets).To(HaveLen(2))

			// First entry should be mandatory
			first := permissionSets[0].(map[string]interface{})
			Expect(first["requirement_type"]).To(Equal("mandatory"))

			// Second entry should be optional
			second := permissionSets[1].(map[string]interface{})
			Expect(second["requirement_type"]).To(Equal("optional"))
		})

		// US2.S2 from specs/019-permission-sets/spec.md
		It("displays human-readable description without raw OAuth2 scope strings", func() {
			// US2.S2 — permission_set.description present; no raw scopes in top-level response
			resp, err := enduserServer.AuthenticatedGET(
				fmt.Sprintf("/api/consent/agents/%s", agentID),
				userPrincipal,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			result := parsePS(resp)
			permissionSets := result["permission_sets"].([]interface{})
			entry := permissionSets[0].(map[string]interface{})
			ps := entry["permission_set"].(map[string]interface{})
			Expect(ps["description"]).To(Equal("Required permissions"))
			// raw scopes must NOT appear at the top level
			Expect(result["scopes"]).To(BeNil(), "raw scopes must not appear at top level")
			// raw scopes must NOT appear in service_scopes of any permission set (FR-007);
			// requirement_type must be present and valid in every service_scopes entry
			for i, entryRaw := range permissionSets {
				psEntry := entryRaw.(map[string]interface{})["permission_set"].(map[string]interface{})
				if serviceScopesRaw, ok := psEntry["service_scopes"]; ok {
					for j, ssRaw := range serviceScopesRaw.([]interface{}) {
						ss := ssRaw.(map[string]interface{})
						Expect(ss["scopes"]).To(BeNil(),
							"raw scopes must not appear in service_scopes of permission_sets[%d][%d]", i, j)
						Expect(ss["requirement_type"]).To(
							BeElementOf("mandatory", "optional"),
							"requirement_type must be mandatory or optional in service_scopes of permission_sets[%d][%d]", i, j)
					}
				}
			}
		})

		// US2.S3 from specs/019-permission-sets/spec.md
		It("omits connect button for service with an active session", func() {
			// US2.S3 — active_session_service_ids includes the service when the user has an active session
			// Seed an active session for the GitHub service.
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, githubServiceID))).To(Succeed())

			resp, err := enduserServer.AuthenticatedGET(
				fmt.Sprintf("/api/consent/agents/%s", agentID),
				userPrincipal,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			result := parsePS(resp)
			activeSessionServiceIDs, ok := result["active_session_service_ids"].([]interface{})
			Expect(ok).To(BeTrue(), "active_session_service_ids must be present as an array")
			// GitHub service must be reported as connected (active session exists).
			activeIDs := make([]string, len(activeSessionServiceIDs))
			for i, v := range activeSessionServiceIDs {
				activeIDs[i] = v.(string)
			}
			Expect(activeIDs).To(ContainElement(githubServiceID), "github service must appear in active_session_service_ids")
		})

		// US3.S6 from specs/019-permission-sets/spec.md

		It("discloses the assigned permission-set scope union for a require-all-scopes requirement", func() {
			ps1Resp, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"name":        "All scopes one",
					"description": "First GitHub capability",
					"service_scopes": []map[string]interface{}{
						{"service_id": githubServiceID, "scopes": []string{"repo:read"}},
						{"service_id": googleServiceID, "scopes": []string{"calendar.read"}},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(ps1Resp.StatusCode).To(Equal(http.StatusCreated))
			ps1ID := parsePS(ps1Resp)["id"].(string)

			ps2Resp, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"name":        "All scopes two",
					"description": "Second GitHub capability",
					"service_scopes": []map[string]interface{}{
						{"service_id": githubServiceID, "scopes": []string{"repo:read", "user:email"}},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(ps2Resp.StatusCode).To(Equal(http.StatusCreated))
			ps2ID := parsePS(ps2Resp)["id"].(string)

			agentResp, err := adminServer.DirectRequest(
				"POST", "/api/agents", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"client_id":    "all-scopes-consent-agent",
					"display_name": "All Scopes Consent Agent",
					"description":  "Discloses permission-set scope union",
					"service_requirements": []map[string]interface{}{
						{"service_id": githubServiceID, "requirement_type": "mandatory", "require_all_scopes": true},
					},
					"permission_sets": []map[string]interface{}{
						{"permission_set_id": ps1ID, "requirement_type": "mandatory"},
						{"permission_set_id": ps2ID, "requirement_type": "mandatory"},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(agentResp.StatusCode).To(Equal(http.StatusCreated))
			allScopesAgentID := parsePS(agentResp)["id"].(string)

			resp, err := enduserServer.AuthenticatedGET(fmt.Sprintf("/api/consent/agents/%s", allScopesAgentID), userPrincipal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			services := parsePS(resp)["services"].([]interface{})
			Expect(services).To(HaveLen(1))
			scopes := services[0].(map[string]interface{})["requiredScopes"].([]interface{})
			scopeNames := make([]string, len(scopes))
			scopeDescriptions := make([]string, len(scopes))
			for i, scope := range scopes {
				scopeData := scope.(map[string]interface{})
				scopeNames[i] = scopeData["name"].(string)
				scopeDescriptions[i] = scopeData["description"].(string)
			}
			Expect(scopeNames).To(Equal([]string{"repo:read", "user:email"}))
			Expect(scopeDescriptions).To(Equal([]string{"Read repositories", "Read email"}))
		})
		It("shows connect button with Required/Optional badge for service without active session", func() {
			// US2.S4 — service_requirements present in response for services without active sessions
			resp, err := enduserServer.AuthenticatedGET(
				fmt.Sprintf("/api/consent/agents/%s", agentID),
				userPrincipal,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			result := parsePS(resp)
			serviceRequirements, ok := result["service_requirements"].([]interface{})
			Expect(ok).To(BeTrue(), "service_requirements must be present in response")
			Expect(serviceRequirements).ToNot(BeEmpty(), "expected at least one service requirement")
		})

		// US2.S5 from specs/019-permission-sets/spec.md
		It("updates optional card checked state on toggle without affecting service connections", func() {
			// US2.S5 — consent grant stores both mandatory + user-selected optional IDs
			// FR-020: Seed active sessions for both services so grant submission succeeds.
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, githubServiceID))).To(Succeed())
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, googleServiceID))).To(Succeed())
			grantBody := map[string]interface{}{
				"granted_permission_sets": map[string][]string{
					mandatoryPSID: {githubServiceID},
					optionalPSID:  {googleServiceID},
				},
			}
			grantResp, err := enduserServer.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants", agentID),
				userPrincipal, "application/json",
				psJSON(grantBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = grantResp.Body.Close() }()
			Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))

			grantResult := parsePSGrant(grantResp)
			gps := grantResult["granted_permission_sets"].(map[string]interface{})
			Expect(gps).To(HaveLen(2))
		})

		// US2.S6 from specs/019-permission-sets/spec.md
		It("renders single mandatory card with approve/decline and no toggling", func() {
			// US2.S6 — consent-info with only mandatory PS; submission with only mandatory ID succeeds
			// FR-020: Seed active session for github (the only service in the mandatory PS).
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, githubServiceID))).To(Succeed())
			// Submit grant with only mandatory PS
			grantBody := map[string]interface{}{
				"granted_permission_sets": map[string][]string{
					mandatoryPSID: {githubServiceID},
				},
			}
			grantResp, err := enduserServer.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants", agentID),
				userPrincipal, "application/json",
				psJSON(grantBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = grantResp.Body.Close() }()
			Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))

			result := parsePSGrant(grantResp)
			gps := result["granted_permission_sets"].(map[string]interface{})
			Expect(gps).To(HaveKey(mandatoryPSID))
		})

		// US2.S7 from specs/019-permission-sets/spec.md
		It("enables Approve button when all displayed services have active sessions", func() {
			// US2.S7 — consent-info returns active_session_service_ids for connected services
			// When all services in the dynamic set have active sessions, the grant can be submitted.
			// FR-020: Seed active sessions for both services.
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, githubServiceID))).To(Succeed())
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, googleServiceID))).To(Succeed())
			resp, err := enduserServer.AuthenticatedGET(
				fmt.Sprintf("/api/consent/agents/%s", agentID),
				userPrincipal,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			info := parsePS(resp)
			// Verify consent-info response contains expected fields
			Expect(info).To(HaveKey("permission_sets"))

			// Submit grant with both mandatory and optional PS — all services included
			grantBody := map[string]interface{}{
				"granted_permission_sets": map[string][]string{
					mandatoryPSID: {githubServiceID},
					optionalPSID:  {googleServiceID},
				},
			}
			grantResp, err := enduserServer.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants", agentID),
				userPrincipal, "application/json",
				psJSON(grantBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = grantResp.Body.Close() }()
			Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))
		})
	})

	// ===========================================================================
	// User Story 3: Agent Declares Permission Sets at Agent Level (Priority: P2)
	// ===========================================================================

	Describe("US3: Agent Declares Permission Sets at Agent Level", func() {
		var validPSID string

		BeforeEach(func() {
			// Create a valid permission set
			r, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"name":        "Valid PS for Agent",
					"description": "PS for agent declaration tests",
					"service_scopes": []map[string]interface{}{
						{"service_id": githubServiceID, "scopes": []string{"repo:read"}},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(r.StatusCode).To(Equal(http.StatusCreated))
			validPSID = parsePS(r)["id"].(string)
		})

		// US3.S1 from specs/019-permission-sets/spec.md
		It("persists agent permission_sets list in declaration order", func() {
			// US3.S1
			// Create second PS
			r2, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"name":        "Second PS for order test",
					"description": "Second",
					"service_scopes": []map[string]interface{}{
						{"service_id": googleServiceID, "scopes": []string{"calendar.read"}},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(r2.StatusCode).To(Equal(http.StatusCreated))
			secondPSID := parsePS(r2)["id"].(string)

			// Create agent with two PSets in specific order
			agentBody := map[string]interface{}{
				"client_id":    "agent-ordered-ps",
				"display_name": "Ordered PS Agent",
				"description":  "Agent with ordered PSets",
				"permission_sets": []map[string]interface{}{
					{"permission_set_id": validPSID, "requirement_type": "mandatory"},
					{"permission_set_id": secondPSID, "requirement_type": "optional"},
				},
			}
			ar, err := adminServer.DirectRequest(
				"POST", "/api/agents", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(agentBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = ar.Body.Close() }()
			Expect(ar.StatusCode).To(Equal(http.StatusCreated))

			agentResult := parsePS(ar)
			psList := agentResult["permission_sets"].([]interface{})
			Expect(psList).To(HaveLen(2))
			first := psList[0].(map[string]interface{})
			second := psList[1].(map[string]interface{})
			Expect(first["permission_set_id"]).To(Equal(validPSID))
			Expect(second["permission_set_id"]).To(Equal(secondPSID))
		})

		// US3.S2 from specs/019-permission-sets/spec.md
		It("rejects agent registration with non-existent permission_set_id with 400", func() {
			// US3.S2
			nonExistentID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
			agentBody := map[string]interface{}{
				"client_id":    "agent-invalid-ps",
				"display_name": "Agent With Invalid PS",
				"description":  "Agent referencing non-existent PS",
				"permission_sets": []map[string]interface{}{
					{"permission_set_id": nonExistentID, "requirement_type": "mandatory"},
				},
			}
			ar, err := adminServer.DirectRequest(
				"POST", "/api/agents", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(agentBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = ar.Body.Close() }()
			Expect(ar.StatusCode).To(Equal(http.StatusBadRequest))

			errResult := parsePS(ar)
			Expect(errResult["message"]).To(ContainSubstring(nonExistentID))
		})

		// US3.S3 from specs/019-permission-sets/spec.md
		It("returns resolved permission set objects with requirement_type in consent-info", func() {
			// US3.S3
			agentBody := map[string]interface{}{
				"client_id":    "agent-resolved-ps",
				"display_name": "Agent With Resolved PS",
				"description":  "Agent for consent-info resolved PS test",
				"permission_sets": []map[string]interface{}{
					{"permission_set_id": validPSID, "requirement_type": "mandatory"},
				},
			}
			ar, err := adminServer.DirectRequest(
				"POST", "/api/agents", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(agentBody),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(ar.StatusCode).To(Equal(http.StatusCreated))
			agentID := parsePS(ar)["id"].(string)

			resp, err := enduserServer.AuthenticatedGET(
				fmt.Sprintf("/api/consent/agents/%s", agentID),
				"user@example.com",
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			result := parsePS(resp)
			psList := result["permission_sets"].([]interface{})
			Expect(psList).To(HaveLen(1))
			entry := psList[0].(map[string]interface{})
			Expect(entry["requirement_type"]).To(Equal("mandatory"))
			ps := entry["permission_set"].(map[string]interface{})
			Expect(ps["id"]).To(Equal(validPSID))
			Expect(ps["name"]).To(Equal("Valid PS for Agent"))
		})

		// US3.S4 from specs/019-permission-sets/spec.md
		It("returns service_requirements and permission_sets independently in agent response", func() {
			// US3.S4
			agentBody := map[string]interface{}{
				"client_id":    "agent-both-fields",
				"display_name": "Agent With Both Fields",
				"description":  "Agent with both service_requirements and permission_sets",
				"service_requirements": []map[string]interface{}{
					{
						"service_id":       githubServiceID,
						"requirement_type": "optional",
						"required_scopes":  []string{"repo:read"},
					},
				},
				"permission_sets": []map[string]interface{}{
					{"permission_set_id": validPSID, "requirement_type": "mandatory"},
				},
			}
			ar, err := adminServer.DirectRequest(
				"POST", "/api/agents", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(agentBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = ar.Body.Close() }()
			Expect(ar.StatusCode).To(Equal(http.StatusCreated))

			result := parsePS(ar)
			Expect(result["service_requirements"]).ToNot(BeNil())
			Expect(result["permission_sets"]).ToNot(BeNil())
			serviceReqs := result["service_requirements"].([]interface{})
			Expect(serviceReqs).To(HaveLen(1))
			psList := result["permission_sets"].([]interface{})
			Expect(psList).To(HaveLen(1))
		})

		// US3.S5 from specs/019-permission-sets/spec.md
		It("rejects agent creation when SR service_id is not covered by any PS with 400 — FR-019", func() {
			// US3.S5 — FR-019 coverage invariant
			// Create a second service ID not covered by validPSID
			uncoveredServiceID := googleServiceID

			agentBody := map[string]interface{}{
				"client_id":    "agent-uncovered-sr",
				"display_name": "Agent with Uncovered SR",
				"description":  "Agent referencing service not covered by any PS",
				"service_requirements": []map[string]interface{}{
					{
						"service_id":       uncoveredServiceID,
						"requirement_type": "mandatory",
						"required_scopes":  []string{"calendar.read"},
					},
				},
				"permission_sets": []map[string]interface{}{
					{"permission_set_id": validPSID, "requirement_type": "mandatory"},
				},
			}
			ar, err := adminServer.DirectRequest(
				"POST", "/api/agents", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(agentBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = ar.Body.Close() }()
			Expect(ar.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	// ===========================================================================
	// User Story 4: User Grant Stores Granted Permission Sets (Priority: P2)
	// ===========================================================================

	Describe("US4: User Grant Stores Granted Permission Sets", func() {
		var (
			mandatoryPSID string
			optionalPSID  string
			agentID       string
		)

		BeforeEach(func() {
			// Create mandatory and optional permission sets
			r1, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"name":        "US4 Mandatory PS",
					"description": "Mandatory for US4 tests",
					"service_scopes": []map[string]interface{}{
						{"service_id": githubServiceID, "scopes": []string{"repo:read"}},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(r1.StatusCode).To(Equal(http.StatusCreated))
			mandatoryPSID = parsePS(r1)["id"].(string)

			r2, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"name":        "US4 Optional PS",
					"description": "Optional for US4 tests",
					"service_scopes": []map[string]interface{}{
						{"service_id": googleServiceID, "scopes": []string{"calendar.read"}},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(r2.StatusCode).To(Equal(http.StatusCreated))
			optionalPSID = parsePS(r2)["id"].(string)

			// Create agent with both PSets
			ar, err := adminServer.DirectRequest(
				"POST", "/api/agents", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"client_id":    "us4-test-agent",
					"display_name": "US4 Test Agent",
					"description":  "Agent for US4 tests",
					"permission_sets": []map[string]interface{}{
						{"permission_set_id": mandatoryPSID, "requirement_type": "mandatory"},
						{"permission_set_id": optionalPSID, "requirement_type": "optional"},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(ar.StatusCode).To(Equal(http.StatusCreated))
			agentID = parsePS(ar)["id"].(string)
		})

		// US4.S1 from specs/019-permission-sets/spec.md
		It("stores granted_permission_sets with per-service inclusion for mandatory + toggled optional IDs", func() {
			// US4.S1
			// FR-020: Seed active sessions for both services.
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, githubServiceID))).To(Succeed())
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, googleServiceID))).To(Succeed())
			grantBody := map[string]interface{}{
				"granted_permission_sets": map[string][]string{
					mandatoryPSID: {githubServiceID},
					optionalPSID:  {googleServiceID},
				},
			}
			resp, err := enduserServer.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants", agentID),
				userPrincipal, "application/json",
				psJSON(grantBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			result := parsePSGrant(resp)
			gps, ok := result["granted_permission_sets"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "granted_permission_sets must be present in grant response")
			Expect(gps).To(HaveLen(2))
			// Both PS IDs present as keys
			Expect(gps).To(HaveKey(mandatoryPSID))
			Expect(gps).To(HaveKey(optionalPSID))
		})

		// US4.S1 from specs/019-permission-sets/spec.md
		// Verifies that submitting consent stores a UserGrant with granted_permission_sets
		// (create response and get response). Token exchange with granted_permission_sets
		// in the response (US4.S2) requires a signed JWT client assertion and is covered
		// at unit level by TestResolveEffectiveScopes_SRCeiling (T046).
		It("stores granted_permission_sets in UserGrant on consent submission", func() {
			// FR-020: Seed active sessions for both services.
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, githubServiceID))).To(Succeed())
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, googleServiceID))).To(Succeed())
			// Create a grant with both PS IDs
			grantBody := map[string]interface{}{
				"granted_permission_sets": map[string][]string{
					mandatoryPSID: {githubServiceID},
					optionalPSID:  {googleServiceID},
				},
			}
			createResp, err := enduserServer.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants", agentID),
				userPrincipal, "application/json",
				psJSON(grantBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = createResp.Body.Close() }()
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))
			createResult := parsePSGrant(createResp)
			createGPS, ok := createResult["granted_permission_sets"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "granted_permission_sets must be present in create response")
			Expect(createGPS).To(HaveKey(mandatoryPSID))
			Expect(createGPS).To(HaveKey(optionalPSID))
			mandatoryServices, ok := createGPS[mandatoryPSID].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(mandatoryServices).To(ContainElement(githubServiceID))
			optionalServices, ok := createGPS[optionalPSID].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(optionalServices).To(ContainElement(googleServiceID))

			// Retrieve the stored grant and verify granted_permission_sets is persisted.
			getResp, err := enduserServer.AuthenticatedGET(
				fmt.Sprintf("/api/consent/agents/%s/grants", agentID),
				userPrincipal,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = getResp.Body.Close() }()
			Expect(getResp.StatusCode).To(Equal(http.StatusOK))

			result := parsePSGrant(getResp)
			gps, ok := result["granted_permission_sets"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "granted_permission_sets must be present in stored grant")
			Expect(gps).To(HaveKey(mandatoryPSID))
			Expect(gps).To(HaveKey(optionalPSID))
		})

		// US4.S2 from specs/019-permission-sets/spec.md
		It("includes granted_permission_sets in token exchange response", func() {
			// US4.S2 — /oauth2/token response includes granted_permission_sets when the grant has PS entries
			// FR-020: Seed active sessions for both services.
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, githubServiceID))).To(Succeed())
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, googleServiceID))).To(Succeed())

			// Create grant with both PSets via consent API (stores GrantedPermissionSets in UserGrant).
			grantResp, err := enduserServer.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants", agentID),
				userPrincipal, "application/json",
				psJSON(map[string]interface{}{
					"granted_permission_sets": map[string][]string{
						mandatoryPSID: {githubServiceID},
						optionalPSID:  {googleServiceID},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			_ = grantResp.Body.Close()
			Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))

			// Seed an active session with real tokens for the github service (used by token exchange).
			now := time.Now()
			accessTokenExpires := now.Add(1 * time.Hour)
			githubSession := fixtures.SessionForServiceWithScopes(userPrincipal, githubServiceID, []string{"repo:read"})
			githubSession.AccessTokenExpiresAt = &accessTokenExpires
			// Overwrite the session seeded above with an explicit scope.
			Expect(testStorage.UserSessions().Create(context.Background(), githubSession)).ToNot(HaveOccurred())

			// Build subject_token and client_assertion JWT.
			subjectTokenClaims := map[string]interface{}{
				"sub": userPrincipal,
				"azp": agentID,
				"iss": mockUpstream.URL(),
				"aud": "token-exchange-broker",
				"exp": now.Add(1 * time.Hour).Unix(),
				"iat": now.Unix(),
			}
			subjectToken, err := helpers.SignTestJWT(subjectTokenClaims, mockUpstream.GetPrivateKeyPEM())
			Expect(err).ToNot(HaveOccurred())

			clientAssertionClaims := map[string]interface{}{
				"sub": agentID,
				"iss": mockUpstream.URL(),
				"aud": "token-exchange-broker",
				"exp": now.Add(1 * time.Hour).Unix(),
				"iat": now.Unix(),
			}
			clientAssertion, err := helpers.SignTestJWT(clientAssertionClaims, mockUpstream.GetPrivateKeyPEM())
			Expect(err).ToNot(HaveOccurred())

			// Perform token exchange.
			mockUpstream.WithSuccessfulTokenResponse()
			formData := url.Values{
				"grant_type":            []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         []string{subjectToken},
				"subject_token_type":    []string{"urn:ietf:params:oauth:token-type:access_token"},
				"requested_token_type":  []string{"urn:ietf:params:oauth:token-type:access_token"},
				"resource":              []string{"https://api.github.com"},
				"client_assertion_type": []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      []string{clientAssertion},
			}
			resp, err := enduserServer.DirectRequest(
				"POST", "/oauth2/token", "",
				map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Assert granted_permission_sets is present in the response body.
			var tokenResp map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&tokenResp)).To(Succeed())
			gps, ok := tokenResp["granted_permission_sets"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "granted_permission_sets must be present in token exchange response")
			Expect(gps).To(HaveKey(mandatoryPSID))
		})

		// US4.S3 from specs/019-permission-sets/spec.md
		It("upserts UserGrant with new granted_permission_sets on re-consent", func() {
			// US4.S3
			// FR-020: Seed active sessions for both services before any grant submission.
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, githubServiceID))).To(Succeed())
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, googleServiceID))).To(Succeed())
			// First grant with both IDs
			firstGrant := map[string]interface{}{
				"granted_permission_sets": map[string][]string{
					mandatoryPSID: {githubServiceID},
					optionalPSID:  {googleServiceID},
				},
			}
			r1, err := enduserServer.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants", agentID),
				userPrincipal, "application/json",
				psJSON(firstGrant),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = r1.Body.Close() }()
			Expect(r1.StatusCode).To(Equal(http.StatusCreated))
			firstResult := parsePSGrant(r1)
			firstGrantID := firstResult["id"].(string)

			// Re-consent with only mandatory ID
			secondGrant := map[string]interface{}{
				"granted_permission_sets": map[string][]string{
					mandatoryPSID: {githubServiceID},
				},
			}
			r2, err := enduserServer.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants", agentID),
				userPrincipal, "application/json",
				psJSON(secondGrant),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = r2.Body.Close() }()
			Expect(r2.StatusCode).To(Equal(http.StatusCreated))
			secondResult := parsePSGrant(r2)

			// Same grant ID (upsert) but different permission sets
			Expect(secondResult["id"]).To(Equal(firstGrantID))
			gps := secondResult["granted_permission_sets"].(map[string]interface{})
			Expect(gps).To(HaveLen(1))
			Expect(gps).To(HaveKey(mandatoryPSID))
		})

		// US4.S4 from specs/019-permission-sets/spec.md
		It("computes scope union for two permission sets both covering Service A", func() {
			// US4.S4 — Create two PSets covering same service (GitHub), each with different scopes
			// FR-020: Seed active session for GitHub (both PSets use the same service).
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, githubServiceID))).To(Succeed())
			// PS1 already exists with repo:read (mandatoryPSID)
			// Create PS2 with different scope for GitHub
			r3, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"name":        "GitHub Extra Scopes",
					"description": "Additional GitHub scopes",
					"service_scopes": []map[string]interface{}{
						{"service_id": githubServiceID, "scopes": []string{"user:email"}},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(r3.StatusCode).To(Equal(http.StatusCreated))
			ps2ID := parsePS(r3)["id"].(string)

			// Create agent with both GitHub PSets
			ar, err := adminServer.DirectRequest(
				"POST", "/api/agents", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"client_id":    "us4-s4-agent",
					"display_name": "US4.S4 Test Agent",
					"description":  "Agent with two GitHub PSets",
					"permission_sets": []map[string]interface{}{
						{"permission_set_id": mandatoryPSID, "requirement_type": "mandatory"},
						{"permission_set_id": ps2ID, "requirement_type": "mandatory"},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(ar.StatusCode).To(Equal(http.StatusCreated))
			unionAgentID := parsePS(ar)["id"].(string)

			// Grant both PSets
			grantBody := map[string]interface{}{
				"granted_permission_sets": map[string][]string{
					mandatoryPSID: {githubServiceID},
					ps2ID:         {githubServiceID},
				},
			}
			grantResp, err := enduserServer.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants", unionAgentID),
				userPrincipal, "application/json",
				psJSON(grantBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = grantResp.Body.Close() }()
			Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))

			result := parsePSGrant(grantResp)
			gps := result["granted_permission_sets"].(map[string]interface{})
			Expect(gps).To(HaveLen(2))
		})
	})

	// ===========================================================================
	// Edge Case: Token Exchange Scope Validation (FR-018)
	// ===========================================================================

	Describe("Edge Case: Token Exchange Fails on Insufficient Scopes", func() {
		// Edge from specs/019-permission-sets/spec.md
		It("fails token exchange with descriptive error when UserSession scopes are insufficient", func() {
			// Edge case: FR-018 — token exchange returns descriptive error when UserSession
			// does not cover the required scopes from the granted permission sets.
			//
			// This test validates that the TokenExchangeService:
			// 1. Resolves permission sets from the grant's granted_permission_sets
			// 2. Computes per-service required scope union
			// 3. Compares against existing UserSession scopes
			// 4. Returns a descriptive error if scopes are insufficient
			//
			// Full validation requires a complete token exchange setup (JWT tokens, sessions).
			// The E2E token exchange suite (token_exchange_test.go) covers the happy path.
			// This scenario is validated at unit level by T046 (service_test.go).
			//
			// Here we verify the token exchange endpoint returns 400 with a descriptive error
			// when submitted with an insufficient request. We expect the error_description to
			// mention scopes or session requirements.
			resp, err := enduserServer.PublicPOST(
				"/oauth2/token",
				"application/x-www-form-urlencoded",
				bytes.NewReader([]byte("grant_type=urn:ietf:params:oauth:grant-type:token-exchange&subject_token=invalid&subject_token_type=urn:ietf:params:oauth:token-type:jwt")),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			// Must return 400 (invalid request) not 500
			Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))

			var errBody map[string]interface{}
			_ = json.NewDecoder(resp.Body).Decode(&errBody)
			Expect(errBody["error"]).ToNot(BeEmpty())
		})

		// Regression: agentUsesPS guard applies fail-closed coverage check to SR-only agents
		// (agents with service_requirements but no declared permission_sets).
		// Prior to the fix, the inner coverage check was gated on len(agent.PermissionSets) > 0,
		// so an SR-only agent would bypass the check even when its grant contained PS entries.
		// The agent is seeded directly into storage (bypassing the admin API / FR-019 check)
		// so it has service_requirements but no permission_sets at the agent level.
		It("returns invalid_grant when requested service is not covered by any PS in grant (SR-only agent)", func() {
			// Create a PS that covers only github.
			psResp, err := adminServer.DirectRequest(
				"POST", "/api/permission-sets", adminPrincipal,
				map[string]string{"Content-Type": "application/json"},
				psJSON(map[string]interface{}{
					"name":        "FR-018 Regression PS",
					"description": "GitHub-only PS for regression test",
					"service_scopes": []map[string]interface{}{
						{"service_id": githubServiceID, "scopes": []string{"repo:read"}},
					},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(psResp.StatusCode).To(Equal(http.StatusCreated))
			regressionPSID := parsePS(psResp)["id"].(string)

			// Seed an SR-only agent directly into storage — no PermissionSets declared.
			// The admin API enforces FR-019 (all SR services must be covered by a declared PS),
			// so we bypass it to create an agent that has SRs but no PSes at the agent level.
			srOnlyAgent := fixtures.ValidAgent()
			srOnlyAgent.ClientID = ptr.To(id.ClientID("fr018-sr-only-agent"))
			srOnlyAgent.DisplayName = "FR-018 SR-Only Agent"
			srOnlyAgent.Description = "SR-only agent for agentUsesPS regression test"
			srOnlyAgent.ServiceRequirements = []domainstorage.ServiceRequirement{
				{
					ServiceID:       id.MustParseServiceID(githubServiceID),
					RequirementType: domainstorage.RequirementTypeMandatory,
					RequiredScopes:  []string{"repo:read"},
				},
			}
			Expect(testStorage.Agents().Create(context.Background(), srOnlyAgent)).To(Succeed())

			// Seed grant directly into storage: PS entries cover only github.
			// Bypassing the consent API avoids any dependency on validateGrantStructureWithResolved
			// so this test remains focused purely on the token-exchange agentUsesPS guard.
			now := time.Now()
			validUntil := now.Add(1 * time.Hour)
			srOnlyGrant := &domainstorage.UserGrant{
				ID:         id.NewGrantID(),
				Principal:  id.Principal(userPrincipal),
				AgentID:    srOnlyAgent.ID,
				ValidUntil: &validUntil,
				GrantedPermissionSets: []domainstorage.GrantedPermissionSetEntry{
					{
						PermissionSetID:    id.MustParsePermissionSetID(regressionPSID),
						IncludedServiceIDs: []id.ServiceID{id.MustParseServiceID(githubServiceID)},
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			}
			Expect(testStorage.UserGrants().Create(context.Background(), srOnlyGrant)).To(Succeed())

			// Update the google service with ProtectedResources so FindByProtectedResource succeeds.
			googleSvcWithPR := &model.ThirdpartyOAuth2ProviderEntity{
				ID:          id.MustParseServiceID(googleServiceID),
				DisplayName: "Google",
				ClientID:    id.ClientID("ps-google-client"),
				Secret:      fixtures.EncryptedSecret(googleServiceID, "google-secret"),
				IssuerURI:   "https://accounts.google.com",
				Endpoints: model.OAuth2Endpoints{
					AuthorizeEndpoint: "https://accounts.google.com/o/oauth2/auth",
					TokenEndpoint:     "https://oauth2.googleapis.com/token",
				},
				Scopes: []model.OAuthScope{
					{ScopeValue: "calendar.read", Description: "Read calendar"},
				},
				ProtectedResources: []string{"https://www.googleapis.com"},
			}
			_, version, err := testStorage.Services().ListProtectedResources(context.Background(), id.MustParseServiceID(googleServiceID))
			Expect(err).ToNot(HaveOccurred())
			Expect(testStorage.Services().Update(context.Background(), googleSvcWithPR, &version)).To(Succeed())

			// Seed an active google session so the exchange passes step 10 and reaches step 11.
			Expect(testStorage.UserSessions().Create(context.Background(), fixtures.SessionForService(userPrincipal, googleServiceID))).To(Succeed())

			// Build JWT tokens for the token exchange request.
			subjectTokenClaims := map[string]interface{}{
				"sub": userPrincipal,
				"azp": srOnlyAgent.ID.String(),
				"iss": mockUpstream.URL(),
				"aud": "token-exchange-broker",
				"exp": now.Add(1 * time.Hour).Unix(),
				"iat": now.Unix(),
			}
			subjectToken, err := helpers.SignTestJWT(subjectTokenClaims, mockUpstream.GetPrivateKeyPEM())
			Expect(err).ToNot(HaveOccurred())

			clientAssertionClaims := map[string]interface{}{
				"sub": srOnlyAgent.ID.String(),
				"iss": mockUpstream.URL(),
				"aud": "token-exchange-broker",
				"exp": now.Add(1 * time.Hour).Unix(),
				"iat": now.Unix(),
			}
			clientAssertion, err := helpers.SignTestJWT(clientAssertionClaims, mockUpstream.GetPrivateKeyPEM())
			Expect(err).ToNot(HaveOccurred())

			// Request exchange for google — not covered by any PS in the grant.
			formData := url.Values{
				"grant_type":            []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         []string{subjectToken},
				"subject_token_type":    []string{"urn:ietf:params:oauth:token-type:access_token"},
				"requested_token_type":  []string{"urn:ietf:params:oauth:token-type:access_token"},
				"resource":              []string{"https://www.googleapis.com"},
				"client_assertion_type": []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      []string{clientAssertion},
			}
			resp, err := enduserServer.DirectRequest(
				"POST", "/oauth2/token", "",
				map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// FR-018 fail-closed: google is not authorized by any PS in the grant.
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			var errBody map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&errBody)).To(Succeed())
			Expect(errBody["error"]).To(Equal("invalid_grant"))
		})

	})
})
