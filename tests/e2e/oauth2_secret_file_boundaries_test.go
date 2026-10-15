package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("OAuth2 credential source boundaries", func() {
	Describe("US4: preserve excluded authentication modes", func() {
		var journey *helpers.OAuth2CredentialJourney

		BeforeEach(func() {
			var err error
			journey, err = helpers.NewOAuth2CredentialJourney()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(journey.Close)
		})

		unavailableBinding := func() map[string]string {
			directory, err := os.MkdirTemp(journey.Directory, "unused-binding-")
			Expect(err).NotTo(HaveOccurred())
			binding := map[string]string{
				"client_id_file":     filepath.Join(directory, "missing-identity"),
				"client_secret_file": filepath.Join(directory, "broken-credential"),
			}
			Expect(os.Symlink("unpublished-credential", binding["client_secret_file"])).To(Succeed())
			return binding
		}

		assertBindingsIgnored := func(bindings map[string]map[string]string) {
			GinkgoHelper()
			records, err := journey.Logs.Records()
			Expect(err).NotTo(HaveOccurred())
			for _, record := range records {
				event, _ := record["event"].(string)
				Expect(event).NotTo(BeElementOf(
					"session.oauth2.credential_files_used",
					"session.oauth2.credential_source_failed",
					"session.oauth2.credential_provider_rejected",
				))
			}
			for _, binding := range bindings {
				for _, path := range binding {
					_, err := os.Stat(path)
					Expect(os.IsNotExist(err)).To(BeTrue())
					Expect(journey.Logs.Raw()).NotTo(ContainSubstring(path))
				}
			}
		}

		storedService := func(serviceID id.ServiceID) *model.ThirdpartyOAuth2ProviderEntity {
			GinkgoHelper()
			service, err := journey.Storage.Services().Get(context.Background(), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(service).NotTo(BeNil())
			return service
		}

		connect := func(serviceID id.ServiceID, clientID string) *domainstorage.UserSession {
			GinkgoHelper()
			response, err := journey.Authorize(serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			authorizationURL, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.Close()).To(Succeed())
			Expect(authorizationURL.Query().Get("client_id")).To(Equal(clientID))
			Expect(authorizationURL.Query().Get("code_challenge")).NotTo(BeEmpty())
			Expect(authorizationURL.Query().Get("code_challenge_method")).To(Equal("S256"))
			callback, err := helpers.ProviderAuthorizationCallback(authorizationURL.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(callback.Query().Get("code")).NotTo(BeEmpty())
			Expect(callback.Query().Get("error")).To(BeEmpty())
			response, err = journey.EndUser.AuthenticatedGET(callback.RequestURI(), journey.Principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			location, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.Close()).To(Succeed())
			Expect(location.Path).To(Equal("/done"))
			Expect(location.Query().Get("error")).To(BeEmpty())
			session, err := journey.Storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(journey.Principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(session).NotTo(BeNil())
			Expect(session.EncryptedAccessToken).NotTo(BeEmpty())
			Expect(session.EncryptedRefreshToken).NotTo(BeEmpty())
			return session
		}

		refresh := func(serviceID id.ServiceID, accessToken, refreshToken string) *domainstorage.UserSession {
			GinkgoHelper()
			response, err := journey.Refresh(serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			session, err := journey.Storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(journey.Principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(session).NotTo(BeNil())
			access, err := journey.Application.OAuth2SessionService.DecryptAccessToken(context.Background(), session)
			Expect(err).NotTo(HaveOccurred())
			Expect(access).To(Equal(accessToken))
			refreshValue, err := journey.Application.OAuth2SessionService.DecryptRefreshToken(context.Background(), session)
			Expect(err).NotTo(HaveOccurred())
			Expect(refreshValue).To(Equal(refreshToken))
			return session
		}

		googleRequest := func(canonicalID string) (map[string]any, string) {
			var document map[string]any
			Expect(json.Unmarshal([]byte(fixtures.GoogleServiceAccountFixtureJSON()), &document)).To(Succeed())
			document["token_uri"] = journey.Upstream.URL() + "/oauth/token"
			encoded, err := json.Marshal(document)
			Expect(err).NotTo(HaveOccurred())
			request := fixtures.CredentialServiceRequest(canonicalID, journey.Upstream.URL(), "")
			request["oauth2_flavor"] = "google"
			request["client_secret"] = string(encoded)
			delete(request, "client_id")
			delete(request, "issuer_uri")
			return request, string(encoded)
		}

		startCIMDBroker := func(bindings map[string]map[string]string) *http.Client {
			document := fixtures.CredentialConfigurationDocument(journey.Upstream.URL(), bindings, 8000, 14000)
			document["server"].(map[string]any)["enduser"].(map[string]any)["public_url"] = fixtures.CIMDEndUserPublicURL
			document["oauth2_authorization_server"] = map[string]any{"mode": "local"}
			delete(document, "token_exchange")
			encoded, err := json.Marshal(document)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(journey.ConfigPath, encoded, 0o644)).To(Succeed())
			journey.Config, err = bootstrap.LoadCredentialConfiguration(journey.ConfigPath, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			journey.Storage, err = bootstrap.NewStorageFactory(journey.Logger).NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			var providerClient *http.Client
			journey.EndUser, providerClient, err = bootstrap.NewCIMDClientEndUserTestServer(
				journey.Storage, bootstrap.NewServerFactory(journey.Config, journey.Logger), fixtures.CIMDEndUserPublicURL, journey.Logger,
			)
			Expect(err).NotTo(HaveOccurred())
			journey.Application = journey.EndUser.App()
			journey.Admin, err = bootstrap.NewAdminTestServer(journey.Application, journey.Logger)
			Expect(err).NotTo(HaveOccurred())
			return providerClient
		}

		seedUsableCIMDKey := func() string {
			keyID := seedCIMDClientAuthenticationKey(journey.Storage)
			_, err := journey.Storage.SigningKeys().SetCurrentInDomain(
				context.Background(), domainstorage.KeyDomainCIMDClientAuthentication, id.NewKeyID(keyID), time.Now().UTC(),
			)
			Expect(err).NotTo(HaveOccurred())
			return keyID
		}

		Context("when a public PKCE client has an unusable binding", func() {
			var (
				serviceID id.ServiceID
				before    *model.ThirdpartyOAuth2ProviderEntity
				bindings  map[string]map[string]string
			)

			BeforeEach(func() {
				journey.Upstream.WithStrictPublicClientMode().WithAccessToken("public-access-initial").WithRefreshToken("public-refresh-initial")
				bindings = map[string]map[string]string{"excluded-public": unavailableBinding()}
				Expect(journey.Start(bindings)).To(Succeed())
				request := fixtures.CredentialServiceRequest("excluded-public", journey.Upstream.URL(), "")
				request["token_endpoint_auth_method"] = "none"
				delete(request, "client_secret")
				var err error
				serviceID, _, err = journey.Register(request)
				Expect(err).NotTo(HaveOccurred())
				before = storedService(serviceID)
				Expect(before.IsPublicClient()).To(BeTrue())
				Expect(before.Secret.IsAbsent()).To(BeTrue())
			})

			// US4-S1 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US4-S1 connects and refreshes without a shared secret or filesystem selection", Label("oauth2-secret-file-overrides"), func() {
				initial := connect(serviceID, fixtures.CredentialClientID)
				journey.Upstream.WithAccessToken("public-access-renewed").WithRefreshToken("public-refresh-renewed")
				renewed := refresh(serviceID, "public-access-renewed", "public-refresh-renewed")
				Expect(renewed.ID).To(Equal(initial.ID))
				Expect(renewed.EncryptedAccessToken).NotTo(Equal(initial.EncryptedAccessToken))
				Expect(renewed.EncryptedRefreshToken).NotTo(Equal(initial.EncryptedRefreshToken))

				requests := journey.Upstream.GetTokenRequests()
				Expect(requests).To(HaveLen(2), "public clients must not negotiate by retrying with credentials")
				for index, request := range requests {
					Expect(request.Header.Values("Authorization")).To(BeEmpty())
					form, err := url.ParseQuery(request.Body)
					Expect(err).NotTo(HaveOccurred())
					Expect(form.Get("client_id")).To(Equal(fixtures.CredentialClientID))
					Expect(form).NotTo(HaveKey("client_secret"))
					Expect(form).NotTo(HaveKey("client_assertion"))
					if index == 0 {
						Expect(form.Get("grant_type")).To(Equal("authorization_code"))
						Expect(form.Get("code_verifier")).NotTo(BeEmpty())
					} else {
						Expect(form.Get("grant_type")).To(Equal("refresh_token"))
						Expect(form.Get("refresh_token")).To(Equal("public-refresh-initial"))
					}
				}
				Expect(storedService(serviceID)).To(Equal(before))
				assertBindingsIgnored(bindings)
			})
		})

		Context("when a CIMD private_key_jwt client has an unusable binding", func() {
			var (
				serviceID id.ServiceID
				before    *model.ThirdpartyOAuth2ProviderEntity
				bindings  map[string]map[string]string
				upstream  *helpers.MockCIMDUpstream
				keyID     string
			)

			BeforeEach(func() {
				bindings = map[string]map[string]string{"excluded-cimd": unavailableBinding()}
				providerClient := startCIMDBroker(bindings)
				upstream = helpers.NewMockCIMDUpstream(helpers.WithCIMDUpstreamHTTPClient(providerClient))
				DeferCleanup(upstream.Close)
				keyID = seedUsableCIMDKey()
				request := fixtures.CredentialServiceRequest("excluded-cimd", upstream.URL(), "")
				request["token_endpoint_auth_method"] = "private_key_jwt"
				delete(request, "client_id")
				delete(request, "client_secret")
				var err error
				serviceID, _, err = journey.Register(request)
				Expect(err).NotTo(HaveOccurred())
				before = storedService(serviceID)
				Expect(before.IsCIMDConfidentialClient()).To(BeTrue())
				Expect(before.Secret.IsAbsent()).To(BeTrue())
			})

			// US4-S2 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US4-S2 connects and refreshes with provider-validated CIMD assertions", Label("oauth2-secret-file-overrides"), func() {
				initial := connect(serviceID, fixtures.CIMDClientIDURL(serviceID))
				renewed := refresh(serviceID, "cimd-upstream-access-token", "cimd-upstream-refresh-token")
				Expect(renewed.ID).To(Equal(initial.ID))
				Expect(renewed.EncryptedAccessToken).NotTo(Equal(initial.EncryptedAccessToken))
				Expect(renewed.EncryptedRefreshToken).NotTo(Equal(initial.EncryptedRefreshToken))

				observations := upstream.Observations()
				Expect(observations.AuthorizationRequests).To(HaveLen(1))
				Expect(observations.AuthorizationRequests[0].MetadataValidated).To(BeTrue())
				Expect(observations.AuthorizationRequests[0].PKCEValidated).To(BeTrue())
				Expect(observations.TokenRequests).To(HaveLen(2))
				Expect(observations.TokenRequests[0].GrantType).To(Equal("authorization_code"))
				Expect(observations.TokenRequests[0].PKCEValidated).To(BeTrue())
				Expect(observations.TokenRequests[1].GrantType).To(Equal("refresh_token"))
				for _, request := range observations.TokenRequests {
					Expect(request.ClientIDValidated).To(BeTrue())
					Expect(request.ClientAssertionCount).To(Equal(1))
					Expect(request.ClientAssertionTypeCount).To(Equal(1))
					Expect(request.JWTBearerAssertionType).To(BeTrue())
					Expect(request.AssertionValidated).To(BeTrue())
					Expect(request.AssertionKeyID).To(Equal(keyID))
					Expect(request.SecretAuthenticationReject).To(BeFalse())
				}
				Expect(observations.CachedKeyIDs).To(ContainElement(keyID))
				Expect(storedService(serviceID)).To(Equal(before))
				assertBindingsIgnored(bindings)
			})
		})

		Context("when a Google-flavor service has an unusable binding", func() {
			var (
				serviceID          id.ServiceID
				before             *model.ThirdpartyOAuth2ProviderEntity
				bindings           map[string]map[string]string
				credentialDocument string
			)

			BeforeEach(func() {
				bindings = map[string]map[string]string{"excluded-google": unavailableBinding()}
				Expect(journey.Start(bindings)).To(Succeed())
				request, document := googleRequest("excluded-google")
				credentialDocument = document
				var err error
				var created map[string]any
				serviceID, created, err = journey.Register(request)
				Expect(err).NotTo(HaveOccurred())
				Expect(created).To(HaveKeyWithValue("oauth2_flavor", "google"))
				Expect(created).To(HaveKeyWithValue("client_id", "112233445566778899001"))
				Expect(created).To(HaveKeyWithValue("client_secret", "REDACTED"))
				service := storedService(serviceID)
				Expect(service.Flavor).To(Equal(model.OAuth2FlavorGoogle))
				Expect(service.Secret.IsEncrypted()).To(BeTrue())
				Expect(service.Endpoints.TokenEndpoint).To(Equal(journey.Upstream.URL() + "/oauth/token"))
				Expect(service.Endpoints.AuthorizeEndpoint).To(Equal("https://accounts.google.com/o/oauth2/auth"))
				// Route only the fixture's authorization endpoint; keep Google enrichment and its credential document intact.
				service.Endpoints.AuthorizeEndpoint = journey.Upstream.URL() + "/oauth/authorize"
				version := service.Version
				Expect(journey.Storage.Services().Update(context.Background(), service, &version)).To(Succeed())
				before = storedService(serviceID)
				journey.Upstream.WithAccessToken("google-access-initial").WithRefreshToken("google-refresh-initial")
			})

			// US4-S3 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US4-S3 connects and refreshes with the stored Google credential document", Label("oauth2-secret-file-overrides"), func() {
				initial := connect(serviceID, "112233445566778899001")
				journey.Upstream.WithAccessToken("google-access-renewed").WithRefreshToken("google-refresh-renewed")
				renewed := refresh(serviceID, "google-access-renewed", "google-refresh-renewed")
				Expect(renewed.ID).To(Equal(initial.ID))
				Expect(renewed.EncryptedAccessToken).NotTo(Equal(initial.EncryptedAccessToken))
				Expect(renewed.EncryptedRefreshToken).NotTo(Equal(initial.EncryptedRefreshToken))

				requests := journey.Upstream.GetTokenRequests()
				Expect(requests).To(HaveLen(2))
				for index, request := range requests {
					clientID, clientSecret, err := helpers.CredentialTokenRequest(request)
					Expect(err).NotTo(HaveOccurred())
					Expect(clientID).To(Equal("112233445566778899001"))
					Expect(clientSecret).To(Equal(credentialDocument))
					form, err := url.ParseQuery(request.Body)
					Expect(err).NotTo(HaveOccurred())
					if index == 0 {
						Expect(form.Get("grant_type")).To(Equal("authorization_code"))
						Expect(form.Get("code_verifier")).NotTo(BeEmpty())
					} else {
						Expect(form.Get("grant_type")).To(Equal("refresh_token"))
						Expect(form.Get("refresh_token")).To(Equal("google-refresh-initial"))
					}
				}
				Expect(storedService(serviceID)).To(Equal(before))
				assertBindingsIgnored(bindings)
			})
		})

		Context("when an administrator selects filesystem for excluded modes", func() {
			type excludedService struct {
				mode      string
				serviceID id.ServiceID
				before    *model.ThirdpartyOAuth2ProviderEntity
				request   map[string]any
			}
			var (
				services []excludedService
				bindings map[string]map[string]string
			)

			BeforeEach(func() {
				bindings = make(map[string]map[string]string)
				for _, mode := range []string{"public", "cimd", "google"} {
					bindings["excluded-"+mode+"-existing"] = unavailableBinding()
					bindings["excluded-"+mode+"-new"] = unavailableBinding()
				}
				startCIMDBroker(bindings)
				seedUsableCIMDKey()
				services = nil
				for _, mode := range []string{"public", "cimd", "google"} {
					canonicalID := "excluded-" + mode + "-existing"
					request := fixtures.CredentialServiceRequest(canonicalID, journey.Upstream.URL(), "")
					switch mode {
					case "public":
						request["token_endpoint_auth_method"] = "none"
						delete(request, "client_secret")
					case "cimd":
						request["token_endpoint_auth_method"] = "private_key_jwt"
						delete(request, "client_id")
						delete(request, "client_secret")
					case "google":
						request, _ = googleRequest(canonicalID)
					}
					request["protected_resources"] = []string{journey.Upstream.URL() + "/resource/" + mode}
					serviceID, _, err := journey.Register(request)
					Expect(err).NotTo(HaveOccurred())
					services = append(services, excludedService{mode: mode, serviceID: serviceID, before: storedService(serviceID), request: request})
				}
			})

			// US4-S4 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US4-S4 rejects filesystem creation and updates for every excluded mode without mutation", Label("oauth2-secret-file-overrides"), func() {
				for _, service := range services {
					By("rejecting filesystem creation and update for " + service.mode)
					request := make(map[string]any, len(service.request))
					for key, value := range service.request {
						request[key] = value
					}
					request["credential_source"] = "filesystem"
					delete(request, "client_id")
					delete(request, "client_secret")
					delete(request, "protected_resources")
					for _, method := range []string{http.MethodPost, http.MethodPut} {
						path := "/api/services"
						request["canonical_id"] = "excluded-" + service.mode + "-new"
						if method == http.MethodPut {
							path += "/" + service.serviceID.String()
							request["canonical_id"] = "excluded-" + service.mode + "-existing"
						}
						response, err := journey.AdminJSON(method, path, request)
						Expect(err).NotTo(HaveOccurred())
						Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
						body, err := helpers.CredentialResponseObject(response)
						Expect(err).NotTo(HaveOccurred())
						Expect(body).To(HaveKeyWithValue("error", "validation failed"))
						Expect(body["message"]).To(ContainSubstring("filesystem"), "reject the unsupported source, not an unrelated missing credential")
						Expect(storedService(service.serviceID)).To(Equal(service.before))
						registered, err := journey.Storage.Services().List(context.Background())
						Expect(err).NotTo(HaveOccurred())
						Expect(registered).To(HaveLen(3))
						Expect(journey.Upstream.GetAuthorizeCalled()).To(BeFalse())
						Expect(journey.Upstream.GetTokenRequests()).To(BeEmpty())
						assertBindingsIgnored(bindings)
					}
				}
			})
		})
	})
})

var _ = Describe("OAuth2 credential source metadata and transitions", Label("oauth2-secret-file-overrides"), func() {
	var journey *helpers.OAuth2CredentialJourney
	var binding map[string]string
	var serviceID id.ServiceID
	canonicalID := "metadata-source"

	BeforeEach(func() {
		var err error
		journey, err = helpers.NewOAuth2CredentialJourney()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(journey.Close)
		binding, err = journey.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
		Expect(err).NotTo(HaveOccurred())
		Expect(journey.Start(map[string]map[string]string{canonicalID: binding})).To(Succeed())
	})

	register := func(source string) id.ServiceID {
		registeredID, _, err := journey.Register(fixtures.CredentialServiceRequest(canonicalID, journey.Upstream.URL(), source))
		Expect(err).NotTo(HaveOccurred())
		return registeredID
	}
	complete := func() {
		response, err := journey.Complete(serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Query().Get("error")).To(BeEmpty())
		Expect(response.Body.Close()).To(Succeed())
		_, err = journey.Storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(journey.Principal), serviceID)
		Expect(err).NotTo(HaveOccurred())
	}
	readService := func() map[string]any {
		response, err := journey.AdminJSON(http.MethodGet, "/api/services/"+serviceID.String(), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		return body
	}
	assertFilesystemMetadata := func(body map[string]any) {
		Expect(body).To(HaveKeyWithValue("credential_source", "filesystem"))
		for _, field := range []string{"client_id", "client_secret", "client_id_file", "client_secret_file", "credential_source_transitioned"} {
			Expect(body).NotTo(HaveKey(field))
		}
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		for _, marker := range []string{fixtures.CredentialClientID, fixtures.CredentialClientSecret, binding["client_id_file"], binding["client_secret_file"]} {
			Expect(string(encoded)).NotTo(ContainSubstring(marker))
		}
	}

	Context("when a filesystem service exposes only source metadata", func() {
		BeforeEach(func() { serviceID = register("filesystem") })

		// US5-S1 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US5-S1 keeps file credentials absent from administration and persistence after exchange and refresh", func() {
			complete()
			response, err := journey.Refresh(serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			assertFilesystemMetadata(readService())
			stored, err := journey.Storage.Services().Get(context.Background(), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(stored.CredentialSource).To(Equal(model.CredentialSourceFilesystem))
			Expect(stored.ClientID.IsZero()).To(BeTrue())
			Expect(stored.Secret.IsAbsent()).To(BeTrue())
			session, err := journey.Storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(journey.Principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(session.UpstreamClientID).NotTo(BeNil())
			Expect(session.UpstreamClientID.String()).To(Equal(fixtures.CredentialClientID))
		})

		// US5-S2 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US5-S2 reads and updates metadata during either file outage without changing the selected source", func() {
			for _, field := range []string{"client_id_file", "client_secret_file"} {
				path := binding[field]
				Expect(os.Rename(path, path+".saved")).To(Succeed())
				assertFilesystemMetadata(readService())
				request := fixtures.CredentialServiceRequest(canonicalID, journey.Upstream.URL(), "filesystem")
				delete(request, "credential_source")
				delete(request, "protected_resources")
				request["display_name"] = "Updated during " + field + " outage"
				response, err := journey.AdminJSON(http.MethodPut, "/api/services/"+serviceID.String(), request)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				body, err := helpers.CredentialResponseObject(response)
				Expect(err).NotTo(HaveOccurred())
				assertFilesystemMetadata(body)
				Expect(body["display_name"]).To(Equal(request["display_name"]))
				Expect(os.Rename(path+".saved", path)).To(Succeed())
			}
			Expect(journey.Upstream.GetAuthorizationRequestCount()).To(BeZero())
			Expect(journey.Upstream.GetTokenRequests()).To(BeEmpty())
			stored, err := journey.Storage.Services().Get(context.Background(), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(stored.CredentialSource).To(Equal(model.CredentialSourceFilesystem))
			Expect(stored.CredentialSourceTransitioned).To(BeFalse())
		})

		Context("with established consent and session metadata", func() {
			var agentID id.AgentID
			BeforeEach(func() {
				complete()
				ctx := context.Background()
				Expect(fixtures.SeedPlaceholderGrantData(ctx, journey.Storage, serviceID)).To(Succeed())
				agent := fixtures.ValidAgent()
				agentID = agent.ID
				Expect(journey.Storage.Agents().Create(ctx, agent)).To(Succeed())
				grant := fixtures.ActiveGrant(journey.Principal, agent.ID.String(), serviceID.String(), []string{"profile"})
				Expect(journey.Storage.UserGrants().Create(ctx, grant)).To(Succeed())
			})

			// US5-S3 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US5-S3 keeps affected consent and session metadata available while initiation requires only the client ID", func() {
				Expect(os.Rename(binding["client_secret_file"], binding["client_secret_file"]+".saved")).To(Succeed())
				authorized, err := journey.Authorize(serviceID)
				Expect(err).NotTo(HaveOccurred())
				Expect(authorized.StatusCode).To(Equal(http.StatusFound))
				Expect(authorized.Body.Close()).To(Succeed())
				Expect(os.Rename(binding["client_id_file"], binding["client_id_file"]+".saved")).To(Succeed())
				for _, path := range []string{"/api/consent/agents/" + agentID.String(), "/api/consent/agents/" + agentID.String() + "/grants", "/api/third-party/" + serviceID.String() + "/session"} {
					response, err := journey.EndUser.AuthenticatedGET(path, journey.Principal)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusOK))
					body, err := helpers.ReadResponseBody(response)
					Expect(err).NotTo(HaveOccurred())
					Expect(body).To(ContainSubstring(serviceID.String()))
					for _, marker := range []string{fixtures.CredentialClientID, fixtures.CredentialClientSecret, binding["client_id_file"], binding["client_secret_file"]} {
						Expect(body).NotTo(ContainSubstring(marker))
					}
				}
				before := len(journey.Upstream.GetTokenRequests())
				authorized, err = journey.Authorize(serviceID)
				Expect(err).NotTo(HaveOccurred())
				Expect(authorized.StatusCode).To(Equal(http.StatusInternalServerError))
				body, err := helpers.CredentialResponseObject(authorized)
				Expect(err).NotTo(HaveOccurred())
				Expect(body).To(HaveKeyWithValue("error", "internal_error"))
				Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(before))
			})
		})

		// US5-S4 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US5-S4 excludes file values and paths from metadata errors events and configuration diagnostics", func() {
			complete()
			markers := []string{fixtures.CredentialClientID, fixtures.CredentialClientSecret, binding["client_id_file"], binding["client_secret_file"]}
			journey.Upstream.WithErrorResponseAndDescription("invalid_client", strings.Join(markers, " "))
			response, err := journey.Refresh(serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusBadGateway))
			providerBody, err := helpers.ReadResponseBody(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Rename(binding["client_secret_file"], binding["client_secret_file"]+".saved")).To(Succeed())
			response, err = journey.Refresh(serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
			sourceBody, err := helpers.ReadResponseBody(response)
			Expect(err).NotTo(HaveOccurred())
			assertFilesystemMetadata(readService())
			invalid := map[string]any{canonicalID: map[string]any{"client_id_file": binding["client_id_file"], "client_secret_file": false}}
			encoded, err := json.Marshal(invalid)
			Expect(err).NotTo(HaveOccurred())
			_, diagnostic := bootstrap.LoadCredentialConfiguration(journey.ConfigPath, map[string]string{"IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES": string(encoded)}, nil)
			Expect(diagnostic).To(HaveOccurred())
			for _, marker := range markers {
				Expect(providerBody).NotTo(ContainSubstring(marker))
				Expect(sourceBody).NotTo(ContainSubstring(marker))
				Expect(journey.Logs.Raw()).NotTo(ContainSubstring(marker))
				Expect(diagnostic.Error()).NotTo(ContainSubstring(marker))
			}
			server, err := bootstrap.StartCredentialServer(bootstrap.CredentialServerOptions{
				Directory: journey.Directory,
				Document:  fixtures.CredentialConfigurationDocument(journey.Upstream.URL(), map[string]map[string]string{canonicalID: binding}, 8000, 14000),
				Principal: journey.Principal,
			})
			if server != nil {
				DeferCleanup(server.Close)
			}
			Expect(err).NotTo(HaveOccurred())
			for _, marker := range markers {
				Expect(server.Logs.Raw()).NotTo(ContainSubstring(marker))
			}
			records, err := journey.Logs.Records()
			Expect(err).NotTo(HaveOccurred())
			Expect(records).To(ContainElement(And(HaveKeyWithValue("event", "session.oauth2.credential_source_failed"), HaveKeyWithValue("service_id", serviceID.String()), HaveKeyWithValue("operation", "refresh"), HaveKeyWithValue("reason", "not_found"))))
			Expect(records).To(ContainElement(And(HaveKeyWithValue("event", "session.oauth2.credential_provider_rejected"), HaveKeyWithValue("reason", "provider_rejected"))))
		})
	})

	type transitionCase struct {
		name            string
		journey         *helpers.OAuth2CredentialJourney
		binding         map[string]string
		serviceID       id.ServiceID
		callback        *url.URL
		selectedID      string
		missingIdentity bool
		initialSource   string
	}
	prepareTransition := func(name, initialSource string) transitionCase {
		j, err := helpers.NewOAuth2CredentialJourney()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(j.Close)
		pair, err := j.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
		Expect(err).NotTo(HaveOccurred())
		Expect(j.Start(map[string]map[string]string{canonicalID: pair})).To(Succeed())
		registeredID, _, err := j.Register(fixtures.CredentialServiceRequest(canonicalID, j.Upstream.URL(), initialSource))
		Expect(err).NotTo(HaveOccurred())
		response, err := j.Complete(registeredID)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Query().Get("error")).To(BeEmpty())
		Expect(response.Body.Close()).To(Succeed())
		authorized, err := j.Authorize(registeredID)
		Expect(err).NotTo(HaveOccurred())
		Expect(authorized.StatusCode).To(Equal(http.StatusFound))
		callback, err := helpers.ProviderAuthorizationCallback(authorized.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(authorized.Body.Close()).To(Succeed())
		missing := name == "missing" || name == "legacy-round-trip"
		if missing {
			claims, err := j.Application.OAuth2SessionService.ValidateStateToken(callback.Query().Get("state"), id.Principal(j.Principal), registeredID)
			Expect(err).NotTo(HaveOccurred())
			claims.UpstreamClientID = nil
			sealed, err := j.Application.OAuth2SessionService.CreateStateToken(claims)
			Expect(err).NotTo(HaveOccurred())
			query := callback.Query()
			query.Set("state", sealed)
			callback.RawQuery = query.Encode()
			session, err := j.Storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(j.Principal), registeredID)
			Expect(err).NotTo(HaveOccurred())
			session.UpstreamClientID = nil
			Expect(j.Storage.UserSessions().Create(context.Background(), session)).To(Succeed())
		}
		selectedID := fixtures.CredentialClientID
		if name == "mismatched" {
			selectedID = fixtures.AlternateCredentialClientID
		}
		Expect(helpers.PublishCredentialPair(pair, selectedID, fixtures.RotatedCredentialSecret)).To(Succeed())
		return transitionCase{name: name, journey: j, binding: pair, serviceID: registeredID, callback: callback, selectedID: selectedID, missingIdentity: missing, initialSource: initialSource}
	}
	transition := func(tc transitionCase, source string) map[string]any {
		request := fixtures.CredentialServiceRequest(canonicalID, tc.journey.Upstream.URL(), source)
		delete(request, "protected_resources")
		if source == "stored" {
			request["client_id"] = tc.selectedID
			request["client_secret"] = "explicit-reverse-secret"
		}
		response, err := tc.journey.AdminJSON(http.MethodPut, "/api/services/"+tc.serviceID.String(), request)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		return body
	}
	assertEstablishedContexts := func(tc transitionCase, selectedSecret string) {
		j := tc.journey
		before := len(j.Upstream.GetTokenRequests())
		response, err := j.Refresh(tc.serviceID)
		Expect(err).NotTo(HaveOccurred())
		blocked := tc.missingIdentity || tc.selectedID != fixtures.CredentialClientID
		if blocked {
			Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
			body, err := helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(body).To(HaveKeyWithValue("error", "internal_error"))
		} else {
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
		}
		response, err = j.EndUser.AuthenticatedGET(tc.callback.RequestURI(), j.Principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Body.Close()).To(Succeed())
		if blocked {
			Expect(location.Query().Get("error")).To(Equal("callback_failed"))
			Expect(j.Upstream.GetTokenRequests()).To(HaveLen(before))
		} else {
			Expect(location.Query().Get("error")).To(BeEmpty())
			requests := j.Upstream.GetTokenRequests()[before:]
			Expect(requests).To(HaveLen(2))
			for _, request := range requests {
				clientID, secret, err := helpers.CredentialTokenRequest(request)
				Expect(err).NotTo(HaveOccurred())
				Expect(clientID).To(Equal(tc.selectedID))
				Expect(secret).To(Equal(selectedSecret))
			}
		}
	}

	Context("when source transitions retain established identity", func() {
		Context("from stored to filesystem", func() {
			var cases []transitionCase
			BeforeEach(func() {
				for _, name := range []string{"matching", "mismatched", "missing"} {
					cases = append(cases, prepareTransition(name, "stored"))
				}
			})
			AfterEach(func() { cases = nil })

			// US5-S5 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US5-S5 atomically removes stored credentials and enforces matching mismatched and legacy identities", func() {
				for _, tc := range cases {
					By("transitioning an established " + tc.name + " context")
					body := transition(tc, "filesystem")
					Expect(body).To(HaveKeyWithValue("credential_source", "filesystem"))
					Expect(body).NotTo(HaveKey("client_id"))
					Expect(body).NotTo(HaveKey("client_secret"))
					stored, err := tc.journey.Storage.Services().Get(context.Background(), tc.serviceID)
					Expect(err).NotTo(HaveOccurred())
					Expect(stored.CredentialSource).To(Equal(model.CredentialSourceFilesystem))
					Expect(stored.CredentialSourceTransitioned).To(BeTrue())
					Expect(stored.ClientID.IsZero()).To(BeTrue())
					Expect(stored.Secret.IsAbsent()).To(BeTrue())
					assertEstablishedContexts(tc, fixtures.RotatedCredentialSecret)
					fresh, err := helpers.NewOAuth2CredentialJourney()
					Expect(err).NotTo(HaveOccurred())
					DeferCleanup(fresh.Close)
					Expect(fresh.Start(nil)).To(Succeed())
					Expect(fresh.Storage.Services().Create(context.Background(), stored)).To(Succeed())
					response, err := fresh.Authorize(tc.serviceID)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
					Expect(response.Body.Close()).To(Succeed())
					response, err = fresh.AdminJSON(http.MethodGet, "/api/services/"+tc.serviceID.String(), nil)
					Expect(err).NotTo(HaveOccurred())
					body, err = helpers.CredentialResponseObject(response)
					Expect(err).NotTo(HaveOccurred())
					Expect(body).To(HaveKeyWithValue("credential_source", "filesystem"))
					Expect(body).NotTo(HaveKey("client_id"))
					Expect(fresh.Upstream.GetAuthorizationRequestCount()).To(BeZero())
					Expect(fresh.Upstream.GetTokenRequests()).To(BeEmpty())
				}
			})
		})

		Context("from filesystem to stored, including a legacy source round trip", func() {
			var cases []transitionCase
			BeforeEach(func() {
				for _, name := range []string{"matching", "mismatched", "missing", "legacy-round-trip"} {
					source := "filesystem"
					if name == "legacy-round-trip" {
						source = "stored"
					}
					cases = append(cases, prepareTransition(name, source))
				}
			})
			AfterEach(func() { cases = nil })

			// US5-S6 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US5-S6 requires explicit encrypted reverse credentials and never revives legacy contexts after a round trip", func() {
				for _, tc := range cases {
					By("returning an established " + tc.name + " context to stored mode")
					if tc.initialSource == "stored" {
						transition(tc, "filesystem")
					}
					before, err := tc.journey.Storage.Services().Get(context.Background(), tc.serviceID)
					Expect(err).NotTo(HaveOccurred())
					incomplete := fixtures.CredentialServiceRequest(canonicalID, tc.journey.Upstream.URL(), "filesystem")
					incomplete["credential_source"] = "stored"
					delete(incomplete, "protected_resources")
					response, err := tc.journey.AdminJSON(http.MethodPut, "/api/services/"+tc.serviceID.String(), incomplete)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(response.Body.Close()).To(Succeed())
					unchanged, err := tc.journey.Storage.Services().Get(context.Background(), tc.serviceID)
					Expect(err).NotTo(HaveOccurred())
					Expect(unchanged).To(Equal(before))
					body := transition(tc, "stored")
					Expect(body).To(HaveKeyWithValue("credential_source", "stored"))
					Expect(body).To(HaveKeyWithValue("client_id", tc.selectedID))
					Expect(body).To(HaveKeyWithValue("client_secret", "REDACTED"))
					stored, err := tc.journey.Storage.Services().Get(context.Background(), tc.serviceID)
					Expect(err).NotTo(HaveOccurred())
					Expect(stored.CredentialSourceTransitioned).To(BeTrue())
					Expect(stored.Secret.IsEncrypted()).To(BeTrue())
					decrypted, err := tc.journey.Application.ProviderService.Get(context.Background(), tc.serviceID)
					Expect(err).NotTo(HaveOccurred())
					secret, err := decrypted.Secret.GetPlaintext()
					Expect(err).NotTo(HaveOccurred())
					Expect(secret).To(Equal("explicit-reverse-secret"))
					for _, path := range tc.binding {
						Expect(os.Remove(path)).To(Succeed())
					}
					assertEstablishedContexts(tc, "explicit-reverse-secret")
				}
			})
		})
	})

	Context("when stored defaults and legacy records remain compatible", func() {
		// US5-S7 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US5-S7 preserves stored credentials and redaction on omitted-source updates", func() {
			serviceID = register("")
			request := fixtures.CredentialServiceRequest(canonicalID, journey.Upstream.URL(), "")
			delete(request, "protected_resources")
			request["display_name"] = "Stored metadata update"
			response, err := journey.AdminJSON(http.MethodPut, "/api/services/"+serviceID.String(), request)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			body, err := helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(body).To(HaveKeyWithValue("credential_source", "stored"))
			Expect(body).To(HaveKeyWithValue("client_id", fixtures.CredentialClientID))
			Expect(body).To(HaveKeyWithValue("client_secret", "REDACTED"))
			complete()
			response, err = journey.Refresh(serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			stored, err := journey.Storage.Services().Get(context.Background(), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(stored.CredentialSourceTransitioned).To(BeFalse())
		})

		Context("with a pre-feature stored service and missing established identity", func() {
			BeforeEach(func() {
				serviceID = id.NewServiceID()
				legacy := fixtures.ServiceWithID(serviceID.String())
				legacy.CanonicalID = &canonicalID
				legacy.ClientID = id.ClientID(fixtures.CredentialClientID)
				legacy.Secret = fixtures.EncryptedSecret(serviceID.String(), fixtures.CredentialClientSecret)
				legacy.IssuerURI = journey.Upstream.URL()
				legacy.Endpoints = model.OAuth2Endpoints{AuthorizeEndpoint: journey.Upstream.URL() + "/oauth/authorize", TokenEndpoint: journey.Upstream.URL() + "/oauth/token"}
				Expect(journey.Storage.Services().Create(context.Background(), legacy)).To(Succeed())
			})

			// US5-S8 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US5-S8 reads upgraded legacy services as stored and preserves never-transitioned authentication", func() {
				before, err := journey.Storage.Services().Get(context.Background(), serviceID)
				Expect(err).NotTo(HaveOccurred())
				body := readService()
				Expect(body).To(HaveKeyWithValue("credential_source", "stored"))
				Expect(body).To(HaveKeyWithValue("client_id", fixtures.CredentialClientID))
				Expect(body).To(HaveKeyWithValue("client_secret", "REDACTED"))
				complete()
				session, err := journey.Storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(journey.Principal), serviceID)
				Expect(err).NotTo(HaveOccurred())
				session.UpstreamClientID = nil
				Expect(journey.Storage.UserSessions().Create(context.Background(), session)).To(Succeed())
				response, err := journey.Refresh(serviceID)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				Expect(response.Body.Close()).To(Succeed())
				after, err := journey.Storage.Services().Get(context.Background(), serviceID)
				Expect(err).NotTo(HaveOccurred())
				Expect(after).To(Equal(before))
				Expect(after.CredentialSourceTransitioned).To(BeFalse())
				for _, request := range journey.Upstream.GetTokenRequests() {
					clientID, secret, err := helpers.CredentialTokenRequest(request)
					Expect(err).NotTo(HaveOccurred())
					Expect(clientID).To(Equal(fixtures.CredentialClientID))
					Expect(secret).To(Equal(fixtures.CredentialClientSecret))
				}
			})
		})
	})
})
