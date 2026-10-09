package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Explicit OAuth2 credential-source selection", Label("oauth2-secret-file-overrides"), func() {
	var journey *helpers.OAuth2CredentialJourney

	BeforeEach(func() {
		var err error
		journey, err = helpers.NewOAuth2CredentialJourney()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(journey.Close)
	})

	register := func(j *helpers.OAuth2CredentialJourney, canonicalID, source string) id.ServiceID {
		GinkgoHelper()
		request := fixtures.CredentialServiceRequest(canonicalID, j.Upstream.URL(), source)
		request["protected_resources"] = []string{j.Upstream.URL() + "/resource/" + canonicalID}
		serviceID, _, err := j.Register(request)
		Expect(err).NotTo(HaveOccurred())
		return serviceID
	}

	record := func(j *helpers.OAuth2CredentialJourney, serviceID id.ServiceID) *model.ThirdpartyOAuth2ProviderEntity {
		GinkgoHelper()
		service, err := j.Storage.Services().Get(context.Background(), serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(service).NotTo(BeNil())
		return service
	}

	readService := func(j *helpers.OAuth2CredentialJourney, serviceID id.ServiceID) map[string]any {
		GinkgoHelper()
		response, err := j.AdminJSON(http.MethodGet, "/api/services/"+serviceID.String(), nil)
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return body
	}

	session := func(j *helpers.OAuth2CredentialJourney, serviceID id.ServiceID) *storage.UserSession {
		GinkgoHelper()
		stored, err := j.Storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(j.Principal), serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).NotTo(BeNil())
		return stored
	}

	expectCredentials := func(request helpers.CapturedTokenRequest, grantType, clientID, clientSecret string) {
		GinkgoHelper()
		form, err := url.ParseQuery(request.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(form.Get("grant_type")).To(Equal(grantType))
		selectedID, selectedSecret, err := helpers.CredentialTokenRequest(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(selectedID).To(Equal(clientID))
		Expect(selectedSecret).To(Equal(clientSecret))
	}

	connect := func(j *helpers.OAuth2CredentialJourney, serviceID id.ServiceID, clientID, clientSecret string) {
		GinkgoHelper()
		tokenCount := len(j.Upstream.GetTokenRequests())
		authorizationCount := j.Upstream.GetAuthorizationRequestCount()
		response, err := j.Authorize(serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Body.Close()).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		authorizationURL, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(authorizationURL.Query().Get("client_id")).To(Equal(clientID))
		Expect(authorizationURL.Query().Get("client_secret")).To(BeEmpty())
		Expect(authorizationURL.Query().Get("code_challenge_method")).To(Equal("S256"))
		callbackURL, err := helpers.ProviderAuthorizationCallback(authorizationURL.String())
		Expect(err).NotTo(HaveOccurred())
		callback, err := j.EndUser.AuthenticatedGET(callbackURL.RequestURI(), j.Principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(callback.Body.Close()).To(Succeed())
		Expect(callback.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(callback.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Path).To(Equal("/done"))
		Expect(location.Query().Get("error")).To(BeEmpty())
		Expect(location.Query().Get("success")).To(Equal("true"))
		Expect(location.Query().Get("service_id")).To(Equal(serviceID.String()))
		Expect(j.Upstream.GetAuthorizationRequestCount()).To(Equal(authorizationCount + 1))
		requests := j.Upstream.GetTokenRequests()
		Expect(requests).To(HaveLen(tokenCount + 1))
		expectCredentials(requests[tokenCount], "authorization_code", clientID, clientSecret)
		stored := session(j, serviceID)
		Expect(stored.EncryptedAccessToken).NotTo(BeEmpty())
		Expect(stored.EncryptedRefreshToken).NotTo(BeEmpty())
	}

	renew := func(j *helpers.OAuth2CredentialJourney, serviceID id.ServiceID, clientID, clientSecret string) {
		GinkgoHelper()
		before := session(j, serviceID)
		refreshToken, err := j.Application.OAuth2SessionService.DecryptRefreshToken(context.Background(), before)
		Expect(err).NotTo(HaveOccurred())
		Expect(refreshToken).NotTo(BeEmpty())
		tokenCount := len(j.Upstream.GetTokenRequests())
		response, err := j.Refresh(serviceID)
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(body).To(HaveKey("data"))
		requests := j.Upstream.GetTokenRequests()
		Expect(requests).To(HaveLen(tokenCount + 1))
		expectCredentials(requests[tokenCount], "refresh_token", clientID, clientSecret)
		form, err := url.ParseQuery(requests[tokenCount].Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(form.Get("refresh_token")).To(Equal(refreshToken))
		Expect(session(j, serviceID).ID).To(Equal(before.ID))
	}

	Context("when the service selects stored credentials", func() {
		Context("with an empty effective mapping", func() {
			var serviceID id.ServiceID

			BeforeEach(func() {
				Expect(journey.Start(map[string]map[string]string{})).To(Succeed())
				serviceID = register(journey, "stored-empty", "")
			})

			// US1-S1 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US1-S1 connects and renews with stored credentials without a file binding", func() {
				before := record(journey, serviceID)
				connect(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				renew(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				Expect(record(journey, serviceID)).To(Equal(before))
			})
		})

		Context("with matching usable and unavailable bindings", func() {
			var serviceIDs []id.ServiceID

			BeforeEach(func() {
				binding, err := journey.WritePair(fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				Expect(err).NotTo(HaveOccurred())
				Expect(journey.Start(map[string]map[string]string{
					"stored-mapped": binding,
					"stored-unavailable": {
						"client_id_file":     filepath.Join(journey.Directory, "missing-identity"),
						"client_secret_file": filepath.Join(journey.Directory, "missing-credential"),
					},
				})).To(Succeed())
				serviceIDs = []id.ServiceID{
					register(journey, "stored-mapped", "stored"),
					register(journey, "stored-unavailable", "stored"),
				}
			})

			// US1-S4 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US1-S4 ignores bindings during stored-source connection and renewal", func() {
				for _, serviceID := range serviceIDs {
					before := record(journey, serviceID)
					connect(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
					renew(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
					Expect(record(journey, serviceID)).To(Equal(before))
				}
			})
		})
	})

	Context("when the service explicitly selects filesystem credentials", func() {
		Context("with a valid mounted pair", func() {
			var (
				serviceID id.ServiceID
				binding   map[string]string
				created   map[string]any
			)

			BeforeEach(func() {
				var err error
				binding, err = journey.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				Expect(err).NotTo(HaveOccurred())
				Expect(journey.Start(map[string]map[string]string{"filesystem-pair": binding})).To(Succeed())
				serviceID, created, err = journey.Register(fixtures.CredentialServiceRequest("filesystem-pair", journey.Upstream.URL(), "filesystem"))
				Expect(err).NotTo(HaveOccurred())
			})

			// US1-S2 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US1-S2 redirects with the file client ID and exchanges the code with the file pair", func() {
				connect(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				stored := session(journey, serviceID)
				Expect(stored.UpstreamClientID).NotTo(BeNil())
				Expect(*stored.UpstreamClientID).To(Equal(id.ClientID(fixtures.CredentialClientID)))
			})

			// US1-S3 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US1-S3 renews an established session with its current file credentials", func() {
				connect(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				before := record(journey, serviceID)
				Expect(helpers.PublishCredentialPair(binding, fixtures.CredentialClientID, fixtures.RotatedCredentialSecret)).To(Succeed())
				journey.Upstream.WithAccessToken("synthetic-renewed-access")
				renew(journey, serviceID, fixtures.CredentialClientID, fixtures.RotatedCredentialSecret)
				stored := session(journey, serviceID)
				Expect(stored.UpstreamClientID).NotTo(BeNil())
				Expect(*stored.UpstreamClientID).To(Equal(id.ClientID(fixtures.CredentialClientID)))
				accessToken, err := journey.Application.OAuth2SessionService.DecryptAccessToken(context.Background(), stored)
				Expect(err).NotTo(HaveOccurred())
				Expect(accessToken).To(Equal("synthetic-renewed-access"))
				Expect(record(journey, serviceID)).To(Equal(before))
			})

			// US1-S7 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US1-S7 creates and reads filesystem mode without inline fields or stored file values", func() {
				expectRepresentation := func(body map[string]any) {
					GinkgoHelper()
					Expect(body).To(HaveKeyWithValue("credential_source", "filesystem"))
					Expect(body).NotTo(HaveKey("client_id"))
					Expect(body).NotTo(HaveKey("client_secret"))
					encoded, err := json.Marshal(body)
					Expect(err).NotTo(HaveOccurred())
					for _, value := range []string{fixtures.CredentialClientID, fixtures.CredentialClientSecret, binding["client_id_file"], binding["client_secret_file"]} {
						Expect(string(encoded)).NotTo(ContainSubstring(value))
					}
				}
				expectRepresentation(created)
				expectRepresentation(readService(journey, serviceID))
				before := record(journey, serviceID)
				Expect(before.CredentialSource).To(Equal(model.CredentialSourceFilesystem))
				Expect(before.ClientID.IsZero()).To(BeTrue())
				Expect(before.Secret.IsAbsent()).To(BeTrue())
				Expect(before.CredentialSourceTransitioned).To(BeFalse())
				connect(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				renew(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				expectRepresentation(readService(journey, serviceID))
				Expect(record(journey, serviceID)).To(Equal(before))
			})
		})

		Context("with unrelated identifiers and decoy bindings", func() {
			var serviceID id.ServiceID

			BeforeEach(func() {
				selected, err := journey.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				Expect(err).NotTo(HaveOccurred())
				decoy, err := journey.WritePair(fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				Expect(err).NotTo(HaveOccurred())
				Expect(journey.Start(map[string]map[string]string{
					"GitHub-Prod":               selected,
					"github-prod":               decoy,
					fixtures.CredentialClientID: decoy,
					"identity":                  decoy,
					"credential":                decoy,
				})).To(Succeed())
				request := fixtures.CredentialServiceRequest("GitHub-Prod", journey.Upstream.URL(), "filesystem")
				request["display_name"] = "Unrelated administrative label"
				serviceID, _, err = journey.Register(request)
				Expect(err).NotTo(HaveOccurred())
			})

			// US1-S5 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US1-S5 selects only the exact canonical ID despite unrelated UUID, display label and filenames", func() {
				metadata := readService(journey, serviceID)
				Expect(metadata["canonical_id"]).To(Equal("GitHub-Prod"))
				Expect(metadata["display_name"]).To(Equal("Unrelated administrative label"))
				Expect(serviceID.String()).NotTo(Equal("GitHub-Prod"))
				connect(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			})
		})

		Context("with two independently bound services", func() {
			var serviceIDs []id.ServiceID

			BeforeEach(func() {
				first, err := journey.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				Expect(err).NotTo(HaveOccurred())
				second, err := journey.WritePair(fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				Expect(err).NotTo(HaveOccurred())
				Expect(journey.Start(map[string]map[string]string{"filesystem-first": first, "filesystem-second": second})).To(Succeed())
				serviceIDs = []id.ServiceID{register(journey, "filesystem-first", "filesystem"), register(journey, "filesystem-second", "filesystem")}
			})

			// US1-S6 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US1-S6 isolates each service's file pair for connection and renewal", func() {
				firstBefore := record(journey, serviceIDs[0])
				secondBefore := record(journey, serviceIDs[1])
				connect(journey, serviceIDs[0], fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				connect(journey, serviceIDs[1], fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				renew(journey, serviceIDs[1], fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				renew(journey, serviceIDs[0], fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				Expect(session(journey, serviceIDs[0]).ID).NotTo(Equal(session(journey, serviceIDs[1]).ID))
				Expect(record(journey, serviceIDs[0])).To(Equal(firstBefore))
				Expect(record(journey, serviceIDs[1])).To(Equal(secondBefore))
			})
		})

		Context("without a matching binding", func() {
			var cases []struct {
				journey   *helpers.OAuth2CredentialJourney
				serviceID id.ServiceID
			}

			BeforeEach(func() {
				cases = nil
				for _, empty := range []bool{true, false} {
					j := journey
					if !empty {
						var err error
						j, err = helpers.NewOAuth2CredentialJourney()
						Expect(err).NotTo(HaveOccurred())
						DeferCleanup(j.Close)
					}
					bindings := map[string]map[string]string{}
					if !empty {
						other, err := j.WritePair(fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
						Expect(err).NotTo(HaveOccurred())
						bindings["unrelated-service"] = other
					}
					Expect(j.Start(bindings)).To(Succeed())
					serviceID := register(j, "filesystem-unbound", "filesystem")
					stored := fixtures.SessionForServiceWithScopes(j.Principal, serviceID.String(), []string{"profile"})
					clientID := id.ClientID(fixtures.CredentialClientID)
					stored.UpstreamClientID = &clientID
					Expect(j.Storage.UserSessions().Create(context.Background(), stored)).To(Succeed())
					cases = append(cases, struct {
						journey   *helpers.OAuth2CredentialJourney
						serviceID id.ServiceID
					}{j, serviceID})
				}
			})

			// US1-S8 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US1-S8 fails initiation and renewal before provider requests for absent and empty mappings", func() {
				for _, fixture := range cases {
					j, serviceID := fixture.journey, fixture.serviceID
					before := record(j, serviceID)
					beforeSession := *session(j, serviceID)
					authorizationCount := j.Upstream.GetAuthorizationRequestCount()
					tokenCount := len(j.Upstream.GetTokenRequests())
					for _, operation := range []func(id.ServiceID) (*http.Response, error){j.Authorize, j.Refresh} {
						response, err := operation(serviceID)
						Expect(err).NotTo(HaveOccurred())
						body, err := helpers.CredentialResponseObject(response)
						Expect(err).NotTo(HaveOccurred())
						Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
						Expect(body).To(HaveKeyWithValue("error", "internal_error"))
						Expect(j.Upstream.GetAuthorizationRequestCount()).To(Equal(authorizationCount))
						Expect(j.Upstream.GetTokenRequests()).To(HaveLen(tokenCount))
					}
					Expect(readService(j, serviceID)).To(HaveKeyWithValue("credential_source", "filesystem"))
					Expect(record(j, serviceID)).To(Equal(before))
					Expect(*session(j, serviceID)).To(Equal(beforeSession))
				}
			})
		})
	})

	Context("when administrative input supplies mixed sources", func() {
		var serviceID id.ServiceID

		BeforeEach(func() {
			Expect(journey.Start(map[string]map[string]string{
				"stored-original": {
					"client_id_file":     filepath.Join(journey.Directory, "unreadable-identity"),
					"client_secret_file": filepath.Join(journey.Directory, "unreadable-credential"),
				},
			})).To(Succeed())
			serviceID = register(journey, "stored-original", "stored")
		})

		// US1-S9 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US1-S9 rejects either supplied inline field, including null and empty values, without mutation", func() {
			before := record(journey, serviceID)
			beforeRepresentation := readService(journey, serviceID)
			beforeServices, err := journey.Storage.Services().List(context.Background())
			Expect(err).NotTo(HaveOccurred())
			authorizationCount := journey.Upstream.GetAuthorizationRequestCount()
			tokenCount := len(journey.Upstream.GetTokenRequests())
			variants := []map[string]any{
				{"client_id": fixtures.CredentialClientID}, {"client_id": ""}, {"client_id": nil},
				{"client_secret": fixtures.CredentialClientSecret}, {"client_secret": ""}, {"client_secret": nil},
				{"client_id": fixtures.CredentialClientID, "client_secret": fixtures.CredentialClientSecret},
				{"client_id": "", "client_secret": ""}, {"client_id": nil, "client_secret": nil},
			}
			for index, inline := range variants {
				for _, method := range []string{http.MethodPost, http.MethodPut} {
					canonicalID, path := fmt.Sprintf("mixed-rejected-%d", index), "/api/services"
					if method == http.MethodPut {
						canonicalID, path = "stored-original", path+"/"+serviceID.String()
					}
					request := fixtures.CredentialServiceRequest(canonicalID, journey.Upstream.URL(), "filesystem")
					request["display_name"] = "Must not replace the existing service"
					request["protected_resources"] = []string{journey.Upstream.URL() + "/resource/" + canonicalID}
					if method == http.MethodPut {
						delete(request, "protected_resources")
					}
					for key, value := range inline {
						request[key] = value
					}
					response, err := journey.AdminJSON(method, path, request)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.Body.Close()).To(Succeed())
					Expect(response.StatusCode).To(Equal(http.StatusBadRequest), "%s variant %d: %v", method, index, inline)
					Expect(record(journey, serviceID)).To(Equal(before))
					Expect(readService(journey, serviceID)).To(Equal(beforeRepresentation))
					services, err := journey.Storage.Services().List(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(services).To(Equal(beforeServices))
					Expect(journey.Upstream.GetAuthorizationRequestCount()).To(Equal(authorizationCount))
					Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(tokenCount))
				}
			}
			connect(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			renew(journey, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			Expect(record(journey, serviceID)).To(Equal(before))
		})
	})
})
