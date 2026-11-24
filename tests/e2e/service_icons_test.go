package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Service icons", func() {
	const originalSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
  <!-- Original Atlas artwork; whitespace and Unicode must survive storage. -->
  <title>atlas-files-private-art — Dossiers &amp; Dateien</title>
  <path fill="#2463eb" d="M4 4h16v16H4z"/>
</svg>
`
	const replacementSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><title>Atlas replacement — cercle</title><circle fill="#16a34a" cx="12" cy="12" r="8"/></svg>`

	var (
		logger         *slog.Logger
		config         *ports.Config
		storageFactory *bootstrap.StorageFactory
		store          *storageadapter.Adapter
		postgres       *bootstrap.PostgresFixture
		usePostgres    bool
		upstream       *helpers.MockUpstreamOAuth2Server
		admin          *bootstrap.TestServer
		enduser        *bootstrap.TestServer
		operator       string
		principal      string
		serviceID      string
		serviceRequest map[string]any
		createdService map[string]any
		agent          *storagedomain.Agent
		grant          *storagedomain.UserGrant
		session        *storagedomain.UserSession
	)

	startServers := func() {
		application, err := bootstrap.NewServerFactory(config, logger).BuildApp(store)
		Expect(err).NotTo(HaveOccurred())
		enduser, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
		admin, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
	}
	stopServers := func() {
		if admin != nil {
			admin.CloseListener()
			admin = nil
		}
		if enduser != nil {
			enduser.Close()
			enduser = nil
		}
	}
	writeService := func(method, path string, body map[string]any, status int) map[string]any {
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		response, err := admin.DirectRequest(method, path, operator, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(encoded))
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(response.Body.Close()).To(Succeed()) }()
		Expect(response.StatusCode).To(Equal(status))
		return decodeJSON[map[string]any](response)
	}
	readService := func() (map[string]any, string) {
		response, err := admin.AuthenticatedGET("/api/services/"+serviceID, operator)
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(response.Body.Close()).To(Succeed()) }()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		etag := response.Header.Get("ETag")
		Expect(etag).NotTo(BeEmpty())
		return decodeJSON[map[string]any](response), etag
	}
	readUserData := func(path string) map[string]any {
		response, err := enduser.AuthenticatedGET(path, principal)
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(response.Body.Close()).To(Succeed()) }()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[map[string]any](response)["data"].(map[string]any)
	}
	readConsentService := func() map[string]any {
		data := readUserData("/api/consent/agents/" + agent.ID.String())
		Expect(data).NotTo(HaveKey("available_services"))
		Expect(data["services"]).To(HaveLen(1))
		return data["services"].([]any)[0].(map[string]any)
	}
	readSession := func() map[string]any {
		return readUserData("/api/third-party/" + serviceID + "/session")
	}
	readGrants := func() map[string]any {
		response, err := enduser.AuthenticatedGET("/api/consent/agents/"+agent.ID.String()+"/grants", principal)
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(response.Body.Close()).To(Succeed()) }()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[map[string]any](response)["data"].(map[string]any)
	}
	expectCurrentViews := func(source string) {
		Expect(readConsentService()).To(And(
			HaveKeyWithValue("serviceId", serviceID),
			HaveKeyWithValue("serviceName", "Atlas Files"),
			HaveKeyWithValue("iconSvg", source),
			HaveKeyWithValue("connectionStatus", "connected"),
		))
		listed := readUserData("/api/third-party/sessions")["sessions"]
		Expect(listed).To(HaveLen(1))
		Expect(listed.([]any)[0]).To(And(
			HaveKeyWithValue("id", session.ID.String()),
			HaveKeyWithValue("service_id", serviceID),
			HaveKeyWithValue("service_icon_svg", source),
			HaveKeyWithValue("requires_reauthentication", false),
		))
		Expect(readSession()["session"]).To(HaveKeyWithValue("service_icon_svg", source))
	}
	refreshSession := func() map[string]any {
		response, err := enduser.AuthenticatedPOST("/api/third-party/"+serviceID+"/session/refresh", principal, "application/json", nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(response.Body.Close()).To(Succeed()) }()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		data := decodeJSON[map[string]any](response)["data"].(map[string]any)
		requests := upstream.GetTokenRequests()
		Expect(requests).NotTo(BeEmpty())
		request := requests[len(requests)-1]
		clientID, secret, err := helpers.CredentialTokenRequest(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(clientID).To(Equal(fixtures.CredentialClientID))
		Expect(secret).To(Equal(fixtures.CredentialClientSecret))
		form, err := url.ParseQuery(request.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(form.Get("grant_type")).To(Equal("refresh_token"))
		Expect(form.Get("refresh_token")).NotTo(BeEmpty())
		return data
	}
	expectNoIconSource := func(response *http.Response) {
		defer func() { Expect(response.Body.Close()).To(Succeed()) }()
		body, err := io.ReadAll(response.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).NotTo(ContainSubstring("atlas-files-private-art"))
		Expect(string(body)).NotTo(ContainSubstring("<svg"))
		Expect(string(body)).NotTo(ContainSubstring(`\u003csvg`))
	}

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		operator = fixtures.AdminPrincipal().String()
		principal = fixtures.DefaultPrincipal().String()
		upstream = helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
		config = fixtures.OAuth2ConfigWithUpstream(upstream.URL())
		storageFactory = bootstrap.NewStorageFactory(logger)
		usePostgres = false
	})

	JustBeforeEach(func() {
		var err error
		if usePostgres {
			Expect(bootstrap.CanAccessContainerRuntime()).To(Succeed())
			postgres, err = bootstrap.NewPostgresFixture(context.Background())
			Expect(err).NotTo(HaveOccurred())
			config.Storage = ports.StorageConfig{
				Backend:  "postgres",
				Postgres: ports.PostgresConfig{ConnectionURL: postgres.ConnectionURL},
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second},
			}
			store, err = storageadapter.NewAdapter(&config.Storage)
		} else {
			store, err = storageFactory.NewTestStorage()
		}
		Expect(err).NotTo(HaveOccurred())
		startServers()

		serviceRequest = fixtures.CredentialServiceRequest("atlas-files", upstream.URL(), "stored")
		delete(serviceRequest, "protected_resources")
		serviceRequest["display_name"] = "Atlas Files"
		serviceRequest["authorization_params"] = map[string]string{"prompt": "consent"}
		serviceRequest["icon_svg"] = originalSVG
		createdService = writeService(http.MethodPost, "/api/services", serviceRequest, http.StatusCreated)
		Expect(createdService["id"]).To(BeAssignableToTypeOf(""))
		serviceID = createdService["id"].(string)
		svcID := id.MustParseServiceID(serviceID)

		permissionSetID := id.NewPermissionSetID()
		permissionSet := &storagedomain.PermissionSet{
			ID:          permissionSetID,
			Name:        "Atlas Files access",
			Description: "Read the synthetic Atlas profile",
			ServiceScopes: []storagedomain.ServiceScope{{
				ServiceID: svcID, Scopes: []string{"profile"}, RequirementType: storagedomain.RequirementTypeMandatory,
			}},
		}
		ctx := context.Background()
		Expect(store.PermissionSets().Create(ctx, permissionSet)).To(Succeed())
		agent = fixtures.ValidAgent()
		agent.PermissionSets = []storagedomain.AgentPermissionSetEntry{{PermissionSetID: permissionSetID, RequirementType: storagedomain.RequirementTypeMandatory}}
		agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: svcID, RequirementType: storagedomain.RequirementTypeMandatory, RequiredScopes: []string{"profile"}}}
		Expect(store.Agents().Create(ctx, agent)).To(Succeed())
		session = fixtures.SessionForServiceWithScopes(principal, serviceID, []string{"profile"})
		Expect(store.UserSessions().Create(ctx, session)).To(Succeed())
		grant = fixtures.ActiveGrant(principal, agent.ID.String(), serviceID, []string{"profile"})
		grant.GrantedPermissionSets = []storagedomain.GrantedPermissionSetEntry{{PermissionSetID: permissionSetID, IncludedServiceIDs: []id.ServiceID{svcID}}}
		Expect(store.UserGrants().Create(ctx, grant)).To(Succeed())
	})

	AfterEach(func() {
		stopServers()
		if store != nil {
			Expect(storageFactory.CloseStorage(store)).To(Succeed())
			store = nil
		}
		if upstream != nil {
			upstream.Close()
		}
		if postgres != nil {
			Expect(postgres.Close(context.Background())).To(Succeed())
			postgres = nil
		}
	})

	Context("when an operator creates an icon in PostgreSQL", func() {
		BeforeEach(func() { usePostgres = true })

		// US1.1 from specs/20261118-224349-configure-service-icons/spec.md (User Story 1).
		It("should retain exact SVG source after a PostgreSQL-backed broker restart", Label("docker"), func() {
			By("returning the exact document through Admin creation, detail and listing")
			Expect(createdService["icon_svg"]).To(Equal(originalSVG))
			beforeService, beforeETag := readService()
			Expect(beforeService["icon_svg"]).To(Equal(originalSVG))
			response, err := admin.AuthenticatedGET("/api/services", operator)
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(response.Body.Close()).To(Succeed()) }()
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(decodeJSON[[]map[string]any](response)).To(ConsistOf(beforeService))
			beforeSession := readSession()
			beforeGrants := readGrants()
			Expect(beforeGrants).To(HaveKeyWithValue("id", grant.ID.String()))
			expectCurrentViews(originalSVG)

			By("closing both listeners, the production application and its database connection")
			stopServers()
			Expect(storageFactory.CloseStorage(store)).To(Succeed())
			store = nil

			By("booting a fresh application against the same database without reseeding")
			store, err = storageadapter.NewAdapter(&config.Storage)
			Expect(err).NotTo(HaveOccurred())
			startServers()
			afterService, afterETag := readService()
			Expect(afterService).To(Equal(beforeService))
			Expect(afterETag).To(Equal(beforeETag))
			expectCurrentViews(originalSVG)
			Expect(readSession()).To(Equal(beforeSession))
			Expect(readGrants()).To(Equal(beforeGrants))
		})
	})

	Context("when an operator replaces a configured icon", func() {
		// US1.2 from specs/20261118-224349-configure-service-icons/spec.md (User Story 1).
		It("should replace the icon without changing service identity grants sessions or OAuth settings", func() {
			beforeService, _ := readService()
			Expect(beforeService["icon_svg"]).To(Equal(originalSVG))
			beforeGrants := readGrants()
			Expect(beforeGrants).To(HaveKeyWithValue("id", grant.ID.String()))
			beforeSession := readSession()
			Expect(beforeSession["session"]).To(And(
				HaveKeyWithValue("id", session.ID.String()),
				HaveKeyWithValue("requires_reauthentication", false),
				HaveKeyWithValue("dependent_agent_count", float64(1)),
			))

			By("replacing only the artwork in a full service PUT without a new If-Match requirement")
			replacement := maps.Clone(serviceRequest)
			replacement["icon_svg"] = replacementSVG
			updated := writeService(http.MethodPut, "/api/services/"+serviceID, replacement, http.StatusOK)
			Expect(updated).To(And(HaveKeyWithValue("id", serviceID), HaveKeyWithValue("icon_svg", replacementSVG)))
			afterService, _ := readService()
			Expect(afterService).To(Equal(updated))
			expectedService := maps.Clone(beforeService)
			expectedService["icon_svg"] = replacementSVG
			delete(expectedService, "updated_at")
			delete(afterService, "updated_at")
			Expect(afterService).To(Equal(expectedService), "only icon source and the service update timestamp may change")

			By("reading the current service artwork on the existing consent and session identities")
			expectCurrentViews(replacementSVG)
			Expect(readGrants()).To(Equal(beforeGrants))
			expectedSession := maps.Clone(beforeSession["session"].(map[string]any))
			expectedSession["service_icon_svg"] = replacementSVG
			afterSession := readSession()
			Expect(afterSession["session"]).To(Equal(expectedSession))
			Expect(afterSession["dependent_agents"]).To(Equal(beforeSession["dependent_agents"]))
			Expect(refreshSession()).To(And(HaveKeyWithValue("id", session.ID.String()), HaveKeyWithValue("service_icon_svg", replacementSVG)))
		})
	})

	Context("when icon-bearing JSON requires an end-user principal", func() {
		// US2.4 from specs/20261118-224349-configure-service-icons/spec.md (User Story 2).
		It("should protect every icon delivery response and preserve ownership and rejected-write atomicity", func() {
			By("proving every delivery path returns source for its authenticated owner")
			expectCurrentViews(originalSVG)
			Expect(refreshSession()).To(HaveKeyWithValue("service_icon_svg", originalSVG))

			By("rejecting anonymous reads and summary-returning refresh without SVG bytes")
			for _, route := range []struct{ method, path string }{
				{http.MethodGet, "/api/consent/agents/" + agent.ID.String()},
				{http.MethodGet, "/api/third-party/sessions"},
				{http.MethodGet, "/api/third-party/" + serviceID + "/session"},
				{http.MethodPost, "/api/third-party/" + serviceID + "/session/refresh"},
			} {
				response, err := enduser.DirectRequest(route.method, route.path, "", nil, nil)
				Expect(err).NotTo(HaveOccurred())
				expectNoIconSource(response)
				Expect(response.StatusCode).To(Equal(http.StatusUnauthorized), "%s %s", route.method, route.path)
			}

			By("keeping another authenticated user's sessions and grants separate")
			other := fixtures.AnotherPrincipal().String()
			response, err := enduser.AuthenticatedGET("/api/third-party/sessions", other)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(decodeJSON[map[string]any](response)["data"]).To(HaveKeyWithValue("sessions", BeEmpty()))
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				path := "/api/third-party/" + serviceID + "/session"
				if method == http.MethodPost {
					path += "/refresh"
				}
				response, err = enduser.DirectRequest(method, path, other, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				expectNoIconSource(response)
				Expect(response.StatusCode).To(Equal(http.StatusNotFound))
			}
			response, err = enduser.AuthenticatedGET("/api/consent/agents/"+agent.ID.String()+"/grants", other)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(decodeJSON[map[string]any](response)["data"]).To(BeNil())

			By("rejecting active content atomically even alongside metadata and credential replacement")
			beforeService, beforeETag := readService()
			beforeSession, beforeGrants := readSession(), readGrants()
			invalid := maps.Clone(serviceRequest)
			invalid["icon_svg"] = `<svg xmlns="http://www.w3.org/2000/svg"><script>document.title='rejected-icon'</script></svg>`
			invalid["display_name"] = "Rejected service rename"
			invalid["client_id"] = fixtures.AlternateCredentialClientID
			invalid["client_secret"] = fixtures.RotatedCredentialSecret
			rejected := writeService(http.MethodPut, "/api/services/"+serviceID, invalid, http.StatusBadRequest)
			Expect(rejected["error"]).To(Equal("validation failed"))
			Expect(rejected["message"]).To(ContainSubstring("icon_svg"))
			Expect(rejected["message"]).NotTo(ContainSubstring("rejected-icon"))
			afterService, afterETag := readService()
			Expect(afterService).To(Equal(beforeService))
			Expect(afterETag).To(Equal(beforeETag))
			Expect(readSession()).To(Equal(beforeSession))
			Expect(readGrants()).To(Equal(beforeGrants))
			expectCurrentViews(originalSVG)
			Expect(refreshSession()).To(And(HaveKeyWithValue("id", session.ID.String()), HaveKeyWithValue("service_icon_svg", originalSVG)))
		})
	})
})
