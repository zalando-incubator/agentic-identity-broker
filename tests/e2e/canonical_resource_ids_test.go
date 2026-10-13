package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

func canonicalJSON(value any) *bytes.Reader {
	body, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())
	return bytes.NewReader(body)
}

func canonicalResponse(resp *http.Response) map[string]any {
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
	if data, ok := body["data"].(map[string]any); ok {
		return data
	}
	return body
}

var _ = Describe("Canonical Resource IDs", func() {
	var (
		server         *bootstrap.TestServer
		testStorage    *storageadapter.Adapter
		storageFactory *bootstrap.StorageFactory
		serverFactory  *bootstrap.ServerFactory
		upstream       *helpers.MockUpstreamOAuth2Server
		principal      string
		serviceID      id.ServiceID
		permissionID   id.PermissionSetID
	)

	request := func(method, path string, body any, headers map[string]string) (*http.Response, error) {
		var reader *bytes.Reader
		if body != nil {
			reader = canonicalJSON(body)
		} else {
			reader = bytes.NewReader(nil)
		}
		if headers == nil {
			headers = map[string]string{}
		}
		if body != nil || method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
			headers["Content-Type"] = "application/json"
		}
		return server.DirectRequest(method, path, principal, headers, reader)
	}

	createAgent := func(canonicalID any, serviceReference, permissionReference string) map[string]any {
		body := map[string]any{
			"client_id":    "canonical-client-" + fmt.Sprint(canonicalID),
			"display_name": "Canonical Agent",
			"description":  "Canonical resource acceptance test agent",
			"service_requirements": []map[string]any{{
				"service_id": serviceReference, "requirement_type": "mandatory", "required_scopes": []string{"read"},
			}},
			"permission_sets": []map[string]any{{"permission_set_id": permissionReference, "requirement_type": "mandatory"}},
		}
		if canonicalID != nil {
			body["canonical_id"] = canonicalID
		}
		resp, err := request(http.MethodPost, "/api/agents/", body, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		return canonicalResponse(resp)
	}

	BeforeEach(func() {
		logger := bootstrap.TestLogger(slog.LevelWarn)
		upstream = helpers.NewMockUpstreamOAuth2Server()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		serverFactory = bootstrap.NewServerFactory(fixtures.LocalConfig(), logger)
		application, err := serverFactory.BuildApp(testStorage)
		Expect(err).NotTo(HaveOccurred())
		server, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
		principal = "admin@example.com"

		serviceID = id.NewServiceID()
		serviceCanonicalID := "github-service"
		Expect(testStorage.Services().Create(context.Background(), &model.ThirdpartyOAuth2ProviderEntity{
			ID:          serviceID,
			CanonicalID: &serviceCanonicalID,
			DisplayName: "GitHub",
			ClientID:    id.ClientID("github-client"),
			Secret:      fixtures.EncryptedSecret(serviceID.String(), "secret"),
			IssuerURI:   "https://github.com",
			Endpoints:   model.OAuth2Endpoints{TokenEndpoint: upstream.URL() + "/oauth/token", AuthorizeEndpoint: "https://github.com/login/oauth/authorize"},
			Scopes:      []model.OAuthScope{{ScopeValue: "read", Description: "Read"}},
		})).To(Succeed())
		permissionID = id.NewPermissionSetID()
		permissionCanonicalID := "github-read"
		Expect(testStorage.PermissionSets().Create(context.Background(), &storage.PermissionSet{
			ID:          permissionID,
			CanonicalID: &permissionCanonicalID,
			Name:        "GitHub Read",
			Description: "Read GitHub",
			ServiceScopes: []storage.ServiceScope{{
				ServiceID: serviceID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeMandatory,
			}},
		})).To(Succeed())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if upstream != nil {
			upstream.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 1.1 from specs/036-canonical-resource-ids/spec.md
	It("creates a managed resource with canonical and UUID identifiers", func() {
		created := createAgent("research-agent", "github-service", "github-read")
		Expect(created["id"]).To(MatchRegexp(`^[0-9a-f-]{36}$`))
		Expect(created["canonical_id"]).To(Equal("research-agent"))
	})

	// Scenario 1.2 from specs/036-canonical-resource-ids/spec.md
	It("manages an agent through canonical ID and UUID", func() {
		created := createAgent("path-agent", "github-service", "github-read")
		for _, identifier := range []string{"path-agent", created["id"].(string)} {
			resp, err := request(http.MethodGet, "/api/agents/"+identifier, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(canonicalResponse(resp)["id"]).To(Equal(created["id"]))
		}
		resp, err := request(http.MethodPost, "/api/agents/path-agent/client-credentials", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusOK)))
		_ = canonicalResponse(resp)
		resp, err = request(http.MethodGet, "/api/agents/"+created["id"].(string)+"/client-credentials", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		_ = canonicalResponse(resp)
		resp, err = request(http.MethodDelete, "/api/agents/path-agent/client-credentials", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

		serviceBody := map[string]any{
			"canonical_id": "lifecycle-service", "display_name": "Lifecycle Service", "client_id": "lifecycle-client", "client_secret": "lifecycle-secret", "issuer_uri": "https://lifecycle.example.com",
			"discovery": map[string]any{"enable_discovery": false},
			"endpoints": map[string]any{"token_endpoint": "https://lifecycle.example.com/token", "authorize_endpoint": "https://lifecycle.example.com/authorize"},
			"scopes":    []map[string]any{{"scope_value": "read", "description": "Read"}},
		}
		resp, err = request(http.MethodPost, "/api/services/", serviceBody, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		createdService := canonicalResponse(resp)
		for _, identifier := range []string{"lifecycle-service", createdService["id"].(string)} {
			resp, err = request(http.MethodGet, "/api/services/"+identifier, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		}
		resp, err = request(http.MethodPut, "/api/services/lifecycle-service", serviceBody, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp, err = request(http.MethodDelete, "/api/services/lifecycle-service", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

		permissionSetBody := map[string]any{
			"canonical_id": "lifecycle-permission-set", "name": "Lifecycle Permission Set", "description": "Lifecycle permission set", "service_scopes": []map[string]any{{"service_id": "github-service", "scopes": []string{"read"}, "requirement_type": "mandatory"}},
		}
		resp, err = request(http.MethodPost, "/api/permission-sets/", permissionSetBody, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		createdPermissionSet := canonicalResponse(resp)
		for _, identifier := range []string{"lifecycle-permission-set", createdPermissionSet["id"].(string)} {
			resp, err = request(http.MethodGet, "/api/permission-sets/"+identifier, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		}
		resp, err = request(http.MethodPut, "/api/permission-sets/lifecycle-permission-set", permissionSetBody, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp, err = request(http.MethodDelete, "/api/permission-sets/lifecycle-permission-set", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
	})

	// Scenario 1.3 from specs/036-canonical-resource-ids/spec.md
	It("keeps a UUID-only resource manageable", func() {
		created := createAgent(nil, serviceID.String(), permissionID.String())
		Expect(created["canonical_id"]).To(BeNil())
		resp, err := request(http.MethodGet, "/api/agents/"+created["id"].(string), nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	// Scenario 1.4 from specs/036-canonical-resource-ids/spec.md
	It("preserves, replaces, and removes canonical IDs on update", func() {
		created := createAgent("mutable-agent", "github-service", "github-read")
		base := map[string]any{"client_id": "canonical-client-mutable-agent", "display_name": "Mutable", "description": "Updated", "service_requirements": []map[string]any{{"service_id": "github-service", "requirement_type": "mandatory", "required_scopes": []string{"read"}}}, "permission_sets": []map[string]any{{"permission_set_id": "github-read", "requirement_type": "mandatory"}}}
		resp, err := request(http.MethodPut, "/api/agents/"+created["id"].(string), base, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(canonicalResponse(resp)["canonical_id"]).To(Equal("mutable-agent"))
		base["canonical_id"] = "replaced-agent"
		resp, err = request(http.MethodPut, "/api/agents/"+created["id"].(string), base, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(canonicalResponse(resp)["canonical_id"]).To(Equal("replaced-agent"))
		base["canonical_id"] = nil
		resp, err = request(http.MethodPut, "/api/agents/"+created["id"].(string), base, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(canonicalResponse(resp)["canonical_id"]).To(BeNil())
	})

	// Scenario 2.1 from specs/036-canonical-resource-ids/spec.md
	It("resolves every supported cross-resource reference by canonical ID", func() {
		created := createAgent("canonical-reference-agent", "github-service", "github-read")
		requirements := created["service_requirements"].([]any)
		Expect(requirements[0].(map[string]any)["service_id"]).To(Equal(serviceID.String()))
		permissionSets := created["permission_sets"].([]any)
		Expect(permissionSets[0].(map[string]any)["permission_set_id"]).To(Equal(permissionID.String()))

		body := map[string]any{
			"canonical_id": "canonical-service-scope",
			"name":         "Canonical Service Scope",
			"description":  "Resolves a canonical service reference",
			"service_scopes": []map[string]any{{
				"service_id": "github-service", "scopes": []string{"read"}, "requirement_type": "mandatory",
			}},
		}
		resp, err := request(http.MethodPost, "/api/permission-sets/", body, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		permissionSet := canonicalResponse(resp)
		Expect(permissionSet["service_scopes"].([]any)[0].(map[string]any)["service_id"]).To(Equal(serviceID.String()))
	})

	// Scenario 2.2 from specs/036-canonical-resource-ids/spec.md
	It("resolves every supported cross-resource reference by UUID", func() {
		created := createAgent("uuid-reference-agent", serviceID.String(), permissionID.String())
		Expect(created["id"]).ToNot(BeEmpty())

		body := map[string]any{
			"name":        "UUID Service Scope",
			"description": "Resolves a UUID service reference",
			"service_scopes": []map[string]any{{
				"service_id": serviceID.String(), "scopes": []string{"read"}, "requirement_type": "mandatory",
			}},
		}
		resp, err := request(http.MethodPost, "/api/permission-sets/", body, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		Expect(canonicalResponse(resp)["service_scopes"].([]any)[0].(map[string]any)["service_id"]).To(Equal(serviceID.String()))
	})

	// Scenario 2.3 from specs/036-canonical-resource-ids/spec.md
	It("rejects unresolved references without persistence", func() {
		body := map[string]any{"client_id": "missing-reference-client", "display_name": "Missing", "description": "Missing reference", "service_requirements": []map[string]any{{"service_id": "missing-service", "requirement_type": "mandatory", "required_scopes": []string{"read"}}}, "permission_sets": []map[string]any{{"permission_set_id": "github-read", "requirement_type": "mandatory"}}}
		resp, err := request(http.MethodPost, "/api/agents/", body, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		_ = canonicalResponse(resp)
		list, err := request(http.MethodGet, "/api/agents/", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = list.Body.Close() }()
		var agents []any
		Expect(json.NewDecoder(list.Body).Decode(&agents)).To(Succeed())
		Expect(agents).To(BeEmpty())
		permissionSetBody := map[string]any{
			"name":        "Unresolved Service Scope",
			"description": "Must not persist",
			"service_scopes": []map[string]any{{
				"service_id": "missing-service", "scopes": []string{"read"}, "requirement_type": "mandatory",
			}},
		}
		resp, err = request(http.MethodPost, "/api/permission-sets/", permissionSetBody, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		_ = canonicalResponse(resp)
		resp, err = request(http.MethodGet, "/api/permission-sets/", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		items := canonicalResponse(resp)["items"].([]any)
		Expect(items).To(HaveLen(1))
	})

	// Scenario 3.1 from specs/036-canonical-resource-ids/spec.md
	It("returns stable UUID IDs and canonical metadata by default", func() {
		created := createAgent("readable-agent", "github-service", "github-read")
		resp, err := request(http.MethodGet, "/api/agents/"+created["id"].(string), nil, nil)
		Expect(err).NotTo(HaveOccurred())
		result := canonicalResponse(resp)
		Expect(result["id"]).To(Equal(created["id"]))
		Expect(result["canonical_id"]).To(Equal("readable-agent"))
		Expect(result["service_requirements"].([]any)[0].(map[string]any)["service_id"]).To(Equal(serviceID.String()))

		resp, err = request(http.MethodGet, "/api/services/"+serviceID.String(), nil, nil)
		Expect(err).NotTo(HaveOccurred())
		service := canonicalResponse(resp)
		Expect(service["id"]).To(Equal(serviceID.String()))
		Expect(service["canonical_id"]).To(Equal("github-service"))

		resp, err = request(http.MethodGet, "/api/permission-sets/"+permissionID.String(), nil, nil)
		Expect(err).NotTo(HaveOccurred())
		permissionSet := canonicalResponse(resp)
		Expect(permissionSet["id"]).To(Equal(permissionID.String()))
		Expect(permissionSet["canonical_id"]).To(Equal("github-read"))
		Expect(permissionSet["service_scopes"].([]any)[0].(map[string]any)["service_id"]).To(Equal(serviceID.String()))
	})

	// Scenario 3.2 from specs/036-canonical-resource-ids/spec.md
	It("returns null canonical metadata for UUID-only resources", func() {
		created := createAgent(nil, serviceID.String(), permissionID.String())
		resp, err := request(http.MethodGet, "/api/agents/"+created["id"].(string), nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(canonicalResponse(resp)["canonical_id"]).To(BeNil())
	})

	// Scenario 3.3 from specs/036-canonical-resource-ids/spec.md
	It("renders nested references canonically when requested", func() {
		created := createAgent("preferred-agent", "github-service", "github-read")
		resp, err := request(http.MethodGet, "/api/agents/"+created["id"].(string), nil, map[string]string{"Prefer": "reference-id=canonical"})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Header.Get("Preference-Applied")).To(Equal("reference-id=canonical"))
		Expect(resp.Header.Get("Vary")).To(ContainSubstring("Prefer"))
		result := canonicalResponse(resp)
		Expect(result["id"]).To(Equal(created["id"]))
		Expect(result["service_requirements"].([]any)[0].(map[string]any)["service_id"]).To(Equal("github-service"))
		Expect(result["permission_sets"].([]any)[0].(map[string]any)["permission_set_id"]).To(Equal("github-read"))

		resp, err = request(http.MethodGet, "/api/permission-sets/"+permissionID.String(), nil, map[string]string{"Prefer": "reference-id=canonical"})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Header.Get("Preference-Applied")).To(Equal("reference-id=canonical"))
		Expect(resp.Header.Get("Vary")).To(ContainSubstring("Prefer"))
		permissionSet := canonicalResponse(resp)
		Expect(permissionSet["id"]).To(Equal(permissionID.String()))
		Expect(permissionSet["service_scopes"].([]any)[0].(map[string]any)["service_id"]).To(Equal("github-service"))
	})
})
