package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Third-party client registration compatibility", func() {
	var (
		admin          *bootstrap.TestServer
		enduser        *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		storage        *storageadapter.Adapter
		logger         *slog.Logger
		principal      string
	)

	requestFor := func(method, registration string) map[string]any {
		request := map[string]any{
			"display_name":  "Registration compatibility service",
			"oauth2_flavor": "standard",
			"issuer_uri":    "https://registration-provider.example.test",
			"discovery":     map[string]any{"enable_discovery": false},
			"endpoints": map[string]any{
				"authorize_endpoint": "https://registration-provider.example.test/authorize",
				"token_endpoint":     "https://registration-provider.example.test/token",
			},
			"scopes": []map[string]any{{"scope_value": "profile", "description": "Read profile"}},
		}
		if registration != "" {
			request["client_registration"] = registration
		}
		switch method {
		case "private_key_jwt":
			request["token_endpoint_auth_method"] = method
		case "none":
			request["token_endpoint_auth_method"] = method
			request["client_id"] = "static-public-client"
		default:
			request["client_id"] = "static-confidential-client"
			request["client_secret"] = "static-confidential-secret"
		}
		return request
	}

	send := func(method, path string, request map[string]any) *http.Response {
		body, err := json.Marshal(request)
		Expect(err).NotTo(HaveOccurred())
		response, err := admin.DirectRequest(method, path, principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	create := func(request map[string]any) map[string]any {
		response := send(http.MethodPost, "/api/services", request)
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		return decodeJSON[map[string]any](response)
	}

	read := func(serviceID string) map[string]any {
		response, err := admin.AuthenticatedGET("/api/services/"+serviceID, principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[map[string]any](response)
	}

	list := func() map[string]map[string]any {
		response, err := admin.AuthenticatedGET("/api/services", principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		services := decodeJSON[[]map[string]any](response)
		byID := make(map[string]map[string]any, len(services))
		for _, service := range services {
			byID[service["id"].(string)] = service
		}
		return byID
	}

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		storage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		principal = fixtures.DefaultPrincipal().String()
	})

	JustBeforeEach(func() {
		config := fixtures.CIMDLocalConfig()
		config.Security.SkipThirdpartyHTTPSValidation = true
		application, err := bootstrap.NewServerFactory(config, logger).BuildApp(storage)
		Expect(err).NotTo(HaveOccurred())
		admin, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
		enduser, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if admin != nil {
			admin.Close()
		}
		if enduser != nil {
			enduser.Close()
		}
		if storage != nil {
			Expect(storageFactory.CloseStorage(storage)).To(Succeed())
		}
	})

	// US7-S1 from specs/046-cimd-upstream-client/spec.md
	It("creates and replaces all legacy method pairs with explicit registration", Label("cimd-upstream-client", "cimd-public-client"), func() {
		key, err := admin.AuthenticatedPOST("/api/cimd-client-keys", principal, "application/json", bytes.NewBufferString(`{"algorithm":"ES256"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(key.StatusCode).To(Equal(http.StatusCreated))
		Expect(key.Body.Close()).To(Succeed())

		modes := []struct{ method, registration string }{
			{"", "static"}, {"none", "static"}, {"private_key_jwt", "cimd"},
		}
		for _, mode := range modes {
			request := requestFor(mode.method, mode.registration)
			created := create(request)
			serviceID := created["id"].(string)
			clientID := request["client_id"]
			if mode.registration == "cimd" {
				clientID = fixtures.CIMDEndUserPublicURL + "/.well-known/oauth-client/" + serviceID
			}
			Expect(created).To(HaveKeyWithValue("client_registration", mode.registration))
			Expect(created).To(HaveKeyWithValue("client_id", clientID))
			request["display_name"] = "Replaced " + mode.registration + " " + mode.method
			replaced := send(http.MethodPut, "/api/services/"+serviceID, request)
			Expect(replaced.StatusCode).To(Equal(http.StatusOK))
			Expect(decodeJSON[map[string]any](replaced)).To(And(
				HaveKeyWithValue("client_registration", mode.registration),
				HaveKeyWithValue("client_id", clientID),
				HaveKeyWithValue("display_name", request["display_name"]),
			))
			Expect(read(serviceID)).To(HaveKeyWithValue("client_registration", mode.registration))
			Expect(list()[serviceID]).To(And(
				HaveKeyWithValue("client_registration", mode.registration),
				HaveKeyWithValue("client_id", clientID),
			))
		}
		Expect(list()).To(HaveLen(len(modes)))
	})

	// US7-S2 from specs/046-cimd-upstream-client/spec.md
	It("rejects incompatible pairs on create and replace before contacting the provider", Label("cimd-upstream-client", "cimd-public-client"), func() {
		var traffic atomic.Int32
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			traffic.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		DeferCleanup(provider.Close)
		created := create(requestFor("none", "static"))
		serviceID := created["id"].(string)
		cases := []struct {
			registration string
			method       any
			omitted      bool
		}{
			{"static", "private_key_jwt", false},
			{"cimd", nil, true},
			{"cimd", nil, false},
		}
		for _, candidate := range cases {
			request := requestFor("none", candidate.registration)
			if candidate.omitted {
				delete(request, "token_endpoint_auth_method")
			} else {
				request["token_endpoint_auth_method"] = candidate.method
			}
			if candidate.registration == "cimd" || candidate.method == "private_key_jwt" {
				delete(request, "client_id")
			}
			request["issuer_uri"] = provider.URL
			request["discovery"] = map[string]any{"enable_discovery": true, "metadata_url": provider.URL + "/.well-known/oauth-authorization-server"}
			delete(request, "endpoints")
			for _, operation := range []struct{ method, path string }{
				{http.MethodPost, "/api/services"}, {http.MethodPut, "/api/services/" + serviceID},
			} {
				response := send(operation.method, operation.path, request)
				Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
				failure := decodeJSON[map[string]any](response)
				Expect(failure).To(HaveKeyWithValue("error", "validation failed"))
				Expect(failure["message"]).To(ContainSubstring("client_registration"))
				Expect(traffic.Load()).To(BeZero())
				Expect(read(serviceID)).To(HaveKeyWithValue("client_registration", "static"))
				Expect(list()).To(HaveLen(1))
			}
		}
	})

	// US7-S3 from specs/046-cimd-upstream-client/spec.md
	It("resolves omitted registration on create and complete replacement without inferring CIMD public", Label("cimd-upstream-client", "cimd-public-client"), func() {
		key, err := admin.AuthenticatedPOST("/api/cimd-client-keys", principal, "application/json", bytes.NewBufferString(`{"algorithm":"ES256"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(key.StatusCode).To(Equal(http.StatusCreated))
		Expect(key.Body.Close()).To(Succeed())
		for _, mode := range []struct{ method, expected string }{
			{"", "static"}, {"none", "static"}, {"private_key_jwt", "cimd"},
		} {
			request := requestFor(mode.method, "")
			created := create(request)
			serviceID := created["id"].(string)
			Expect(created).To(HaveKeyWithValue("client_registration", mode.expected))
			if mode.method == "none" {
				Expect(created).To(HaveKeyWithValue("client_id", "static-public-client"))
			}
			replaced := send(http.MethodPut, "/api/services/"+serviceID, request)
			Expect(replaced.StatusCode).To(Equal(http.StatusOK))
			Expect(decodeJSON[map[string]any](replaced)).To(HaveKeyWithValue("client_registration", mode.expected))
			Expect(read(serviceID)).To(HaveKeyWithValue("client_registration", mode.expected))
			Expect(list()[serviceID]).To(HaveKeyWithValue("client_registration", mode.expected))
		}
	})

	// US7-S4 from specs/046-cimd-upstream-client/spec.md
	It("rejects explicit null, empty, and unknown registrations without provider traffic", Label("cimd-upstream-client", "cimd-public-client"), func() {
		var traffic atomic.Int32
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			traffic.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		DeferCleanup(provider.Close)
		created := create(requestFor("none", "static"))
		serviceID := created["id"].(string)
		for _, value := range []any{nil, "", "unknown"} {
			request := requestFor("none", "")
			request["client_registration"] = value
			request["issuer_uri"] = provider.URL
			request["discovery"] = map[string]any{"enable_discovery": true, "metadata_url": provider.URL + "/.well-known/oauth-authorization-server"}
			delete(request, "endpoints")
			for _, operation := range []struct{ method, path string }{
				{http.MethodPost, "/api/services"}, {http.MethodPut, "/api/services/" + serviceID},
			} {
				response := send(operation.method, operation.path, request)
				Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
				failure := decodeJSON[map[string]any](response)
				Expect(failure).To(HaveKeyWithValue("error", "validation failed"))
				Expect(failure["message"]).To(ContainSubstring("client_registration"))
				Expect(traffic.Load()).To(BeZero())
				Expect(read(serviceID)).To(HaveKeyWithValue("client_registration", "static"))
				Expect(list()).To(HaveLen(1))
			}
		}
	})

	Context("with services persisted before registration became explicit", func() {
		var legacy []string
		type sessionSnapshot struct {
			id              string
			access, refresh []byte
		}
		var sessions map[string]sessionSnapshot
		BeforeEach(func() {
			legacy = nil
			sessions = make(map[string]sessionSnapshot)
			for _, service := range []func() *model.ThirdpartyOAuth2ProviderEntity{
				fixtures.CIMDStaticConfidentialService, fixtures.CIMDPublicService, fixtures.CIMDConfidentialService,
			} {
				entity := service()
				Expect(storage.Services().Create(context.Background(), entity)).To(Succeed())
				legacy = append(legacy, entity.ID.String())
				session := fixtures.SessionForService(principal, entity.ID.String())
				Expect(storage.UserSessions().Create(context.Background(), session)).To(Succeed())
				sessions[entity.ID.String()] = sessionSnapshot{
					id:      session.ID.String(),
					access:  bytes.Clone(session.EncryptedAccessToken),
					refresh: bytes.Clone(session.EncryptedRefreshToken),
				}
			}
		})

		// US7-S5 from specs/046-cimd-upstream-client/spec.md
		It("reports legacy classifications without altering identities, methods, or sessions", Label("cimd-upstream-client", "cimd-public-client"), func() {
			expected := []struct {
				registration, clientID string
				method                 any
			}{
				{"static", "static-confidential-cimd-fixture-client", nil},
				{"static", "public-cimd-fixture-client", "none"},
				{"cimd", fixtures.CIMDEndUserPublicURL + "/.well-known/oauth-client/" + legacy[2], "private_key_jwt"},
			}
			listed := list()
			Expect(listed).To(HaveLen(3))
			for index, serviceID := range legacy {
				for _, reported := range []map[string]any{read(serviceID), listed[serviceID]} {
					Expect(reported).To(And(
						HaveKeyWithValue("client_registration", expected[index].registration),
						HaveKeyWithValue("client_id", expected[index].clientID),
					))
					Expect(reported).To(HaveKey("token_endpoint_auth_method"))
					if expected[index].method == nil {
						Expect(reported["token_endpoint_auth_method"]).To(BeNil())
					} else {
						Expect(reported).To(HaveKeyWithValue("token_endpoint_auth_method", expected[index].method))
					}
				}
				session, err := storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), id.MustParseServiceID(serviceID))
				Expect(err).NotTo(HaveOccurred())
				Expect(session).NotTo(BeNil())
				Expect(session.ID.String()).To(Equal(sessions[serviceID].id))
				Expect(session.EncryptedAccessToken).To(Equal(sessions[serviceID].access))
				Expect(session.EncryptedRefreshToken).To(Equal(sessions[serviceID].refresh))
			}
		})
	})

	// US7-S6 from specs/046-cimd-upstream-client/spec.md
	It("rejects supplied registration before discovery and reports the selected CIMD or DCR method", Label("cimd-upstream-client", "cimd-public-client"), func() {
		for _, selection := range []struct {
			scenario                           fixtures.ProtectedResourceScenario
			registration, method, clientMethod string
		}{
			{fixtures.CIMDAndDCRProtectedResource(), "cimd", "private_key_jwt", "cimd"},
			{fixtures.ConfidentialDCRProtectedResource(), "static", "client_secret_basic", "dcr"},
		} {
			provider := helpers.NewMockProtectedResourceProvider(selection.scenario)
			hosts := make([]string, 0, len(selection.scenario.IssuerURLs)+2)
			for _, raw := range append(append([]string{selection.scenario.ResourceURL}, selection.scenario.IssuerURLs...), selection.scenario.RegistrationURL) {
				if raw != "" {
					parsed, err := url.Parse(raw)
					Expect(err).NotTo(HaveOccurred())
					hosts = append(hosts, parsed.Hostname())
				}
			}
			separateStorage, err := storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			config := fixtures.CIMDLocalConfig()
			config.ThirdPartyOAuth2.ClientName = selection.scenario.ClientName
			application, err := bootstrap.NewServerFactory(config, logger).BuildAppWithProtectedResource(separateStorage, provider.Server, hosts...)
			Expect(err).NotTo(HaveOccurred())
			selectionAdmin, err := bootstrap.NewAdminTestServer(application, logger)
			Expect(err).NotTo(HaveOccurred())
			func() {
				defer provider.Close()
				defer func() { Expect(storageFactory.CloseStorage(separateStorage)).To(Succeed()) }()
				defer selectionAdmin.Close()
				request := map[string]any{
					"display_name":        "Discovered registration",
					"discovery":           map[string]any{"enable_discovery": true, "resource_url": selection.scenario.ResourceURL},
					"client_registration": selection.registration,
				}
				encoded, err := json.Marshal(request)
				Expect(err).NotTo(HaveOccurred())
				rejected, err := selectionAdmin.AuthenticatedPOST("/api/services", principal, "application/json", bytes.NewReader(encoded))
				Expect(err).NotTo(HaveOccurred())
				Expect(rejected.StatusCode).To(Equal(http.StatusBadRequest))
				failure := decodeJSON[map[string]any](rejected)
				Expect(failure).To(HaveKeyWithValue("error", "validation failed"))
				Expect(failure["message"]).To(ContainSubstring("client_registration"))
				Expect(provider.Calls()).To(BeEmpty())
				unchanged, err := selectionAdmin.AuthenticatedGET("/api/services", principal)
				Expect(err).NotTo(HaveOccurred())
				Expect(unchanged.StatusCode).To(Equal(http.StatusOK))
				Expect(decodeJSON[[]map[string]any](unchanged)).To(BeEmpty())
				if selection.clientMethod == "cimd" {
					key, err := selectionAdmin.AuthenticatedPOST("/api/cimd-client-keys", principal, "application/json", bytes.NewBufferString(`{"algorithm":"ES256"}`))
					Expect(err).NotTo(HaveOccurred())
					Expect(key.StatusCode).To(Equal(http.StatusCreated))
					Expect(key.Body.Close()).To(Succeed())
				}
				delete(request, "client_registration")
				encoded, err = json.Marshal(request)
				Expect(err).NotTo(HaveOccurred())
				created, err := selectionAdmin.AuthenticatedPOST("/api/services", principal, "application/json", bytes.NewReader(encoded))
				Expect(err).NotTo(HaveOccurred())
				Expect(created.StatusCode).To(Equal(http.StatusCreated))
				Expect(provider.Calls()).NotTo(BeEmpty())
				service := decodeJSON[map[string]any](created)
				Expect(service).To(And(
					HaveKeyWithValue("client_registration", selection.registration),
					HaveKeyWithValue("token_endpoint_auth_method", selection.method),
				))
				Expect(service["discovery"]).To(HaveKeyWithValue("client_method", selection.clientMethod))
				if selection.clientMethod == "cimd" {
					Expect(service).To(HaveKeyWithValue("client_id", fixtures.CIMDEndUserPublicURL+"/.well-known/oauth-client/"+service["id"].(string)))
					Expect(provider.RegistrationCount()).To(BeZero())
				} else {
					Expect(service).To(HaveKeyWithValue("client_id", "dcr-client-123"))
					Expect(provider.RegistrationCount()).To(Equal(1))
				}
				get, err := selectionAdmin.AuthenticatedGET("/api/services/"+service["id"].(string), principal)
				Expect(err).NotTo(HaveOccurred())
				Expect(get.StatusCode).To(Equal(http.StatusOK))
				Expect(decodeJSON[map[string]any](get)).To(HaveKeyWithValue("client_registration", selection.registration))
				listed, err := selectionAdmin.AuthenticatedGET("/api/services", principal)
				Expect(err).NotTo(HaveOccurred())
				Expect(listed.StatusCode).To(Equal(http.StatusOK))
				Expect(decodeJSON[[]map[string]any](listed)).To(ContainElement(HaveKeyWithValue("client_registration", selection.registration)))
			}()
		}
	})
})
