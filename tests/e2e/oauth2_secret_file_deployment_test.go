package e2e_test

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// Rendering assertions for these scenarios live in tests/integration/helm_chart_test.go.
type credentialDeploymentBroker interface {
	AdminJSON(method, path string, body any) (*http.Response, error)
	EndUserRequest(method, path string, body io.Reader) (*http.Response, error)
}

var _ = Describe("OAuth2 credential-file deployment", func() {
	var (
		journey    *helpers.OAuth2CredentialJourney
		broker     credentialDeploymentBroker
		endUserURL string
		logs       *bootstrap.BufferedLogCapture
		serviceID  string
	)

	BeforeEach(func() {
		var err error
		journey, err = helpers.NewOAuth2CredentialJourney()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(journey.Close)
		broker = nil
		endUserURL = ""
		logs = nil
		serviceID = ""
	})

	startBroker := func(bindings map[string]map[string]string, nonRoot bool) {
		GinkgoHelper()
		document := fixtures.CredentialConfigurationDocument(journey.Upstream.URL(), bindings, 8000, 14000)
		if nonRoot {
			process, err := bootstrap.StartCredentialProcess(bootstrap.CredentialProcessOptions{
				Directory: journey.Directory, Document: document, Principal: journey.Principal, NonRoot: true,
			})
			if process != nil {
				DeferCleanup(process.Close)
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(process.UID).To(BeNumerically(">", 0), "the real broker process must not run as root")
			broker, endUserURL, logs = process, process.EndUserURL, process.Logs
			return
		}
		server, err := bootstrap.StartCredentialServer(bootstrap.CredentialServerOptions{
			Directory: journey.Directory, Document: document, Principal: journey.Principal,
		})
		if server != nil {
			DeferCleanup(server.Close)
		}
		Expect(err).NotTo(HaveOccurred())
		broker, endUserURL, logs = server, server.EndUserURL, server.Logs
	}

	registerFilesystem := func(canonicalID string) map[string]any {
		GinkgoHelper()
		request := fixtures.CredentialServiceRequest(canonicalID, journey.Upstream.URL(), "filesystem")
		Expect(request).NotTo(HaveKey("client_id"))
		Expect(request).NotTo(HaveKey("client_secret"))
		response, err := broker.AdminJSON(http.MethodPost, "/api/services", request)
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		Expect(body).To(HaveKeyWithValue("canonical_id", canonicalID))
		Expect(body).To(HaveKeyWithValue("credential_source", "filesystem"))
		Expect(body).NotTo(HaveKey("client_id"))
		Expect(body).NotTo(HaveKey("client_secret"))
		serviceID, _ = body["id"].(string)
		_, err = id.ParseServiceID(serviceID)
		Expect(err).NotTo(HaveOccurred())
		return body
	}

	sessionDetails := func() map[string]any {
		GinkgoHelper()
		response, err := broker.EndUserRequest(http.MethodGet, "/api/third-party/"+serviceID+"/session", nil)
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		data, ok := body["data"].(map[string]any)
		Expect(ok).To(BeTrue())
		session, ok := data["session"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(session).To(HaveKeyWithValue("service_id", serviceID))
		Expect(session).To(HaveKeyWithValue("principal", journey.Principal))
		return session
	}

	connectUser := func() map[string]any {
		GinkgoHelper()
		response, err := broker.EndUserRequest(http.MethodGet, "/api/third-party/"+serviceID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(endUserURL+"/done"), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		authorizationURL, err := url.Parse(response.Header.Get("Location"))
		Expect(response.Body.Close()).To(Succeed())
		Expect(err).NotTo(HaveOccurred())
		Expect(authorizationURL.Query().Get("client_id")).To(Equal(fixtures.CredentialClientID))
		callbackURL, err := helpers.ProviderAuthorizationCallback(authorizationURL.String())
		Expect(err).NotTo(HaveOccurred())
		response, err = broker.EndUserRequest(http.MethodGet, callbackURL.RequestURI(), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(response.Body.Close()).To(Succeed())
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Path).To(Equal("/done"))
		Expect(location.Query().Get("success")).To(Equal("true"))
		Expect(location.Query().Get("service_id")).To(Equal(serviceID))
		Expect(location.Query().Get("error")).To(BeEmpty())
		Expect(journey.Upstream.GetAuthorizationRequestCount()).To(Equal(1))
		requests := journey.Upstream.GetTokenRequests()
		Expect(requests).To(HaveLen(1))
		clientID, secret, err := helpers.CredentialTokenRequest(requests[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(clientID).To(Equal(fixtures.CredentialClientID))
		Expect(secret).To(Equal(fixtures.CredentialClientSecret))
		form, err := url.ParseQuery(requests[0].Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(form.Get("grant_type")).To(Equal("authorization_code"))
		Expect(form.Get("code_verifier")).NotTo(BeEmpty())
		return sessionDetails()
	}

	Context("with a readable synthetic credential directory", func() {
		var binding map[string]string

		BeforeEach(func() {
			var err error
			binding, err = journey.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chmod(binding["client_id_file"], 0o444)).To(Succeed())
			Expect(os.Chmod(binding["client_secret_file"], 0o444)).To(Succeed())
		})

		Context("when an operator applies the paired broker configuration", func() {
			// US7-S1 from specs/051-oauth2-secret-file-overrides/spec.md.
			// Default omission and rendered mount preservation are covered by TestHelmCredentialFilesDefaultsOmitBinding and TestHelmCredentialFilesPairsAndExistingMounts.
			It("US7-S1 applies both configured paths and connects a filesystem service", Label("oauth2-secret-file-overrides"), func() {
				startBroker(map[string]map[string]string{"deployment-service": binding}, false)
				registerFilesystem("deployment-service")
				connectUser()
			})
		})

		Context("when the credential publisher atomically replaces a complete immutable pair", func() {
			// US7-S2 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US7-S2 connects and renews access in the real non-root broker using the current file credentials", Label("oauth2-secret-file-overrides"), func() {
				startBroker(map[string]map[string]string{"deployment-service": binding}, true)
				registerFilesystem("deployment-service")
				originalSession := connectUser()
				previousGeneration, err := os.Readlink(filepath.Join(filepath.Dir(binding["client_id_file"]), "..data"))
				Expect(err).NotTo(HaveOccurred())
				Expect(helpers.PublishCredentialPair(binding, fixtures.CredentialClientID, fixtures.RotatedCredentialSecret)).To(Succeed())
				Expect(os.Chmod(binding["client_id_file"], 0o444)).To(Succeed())
				Expect(os.Chmod(binding["client_secret_file"], 0o444)).To(Succeed())
				currentGeneration, err := os.Readlink(filepath.Join(filepath.Dir(binding["client_id_file"]), "..data"))
				Expect(err).NotTo(HaveOccurred())
				Expect(currentGeneration).NotTo(Equal(previousGeneration))
				response, err := broker.EndUserRequest(http.MethodPost, "/api/third-party/"+serviceID+"/session/refresh", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				Expect(response.Body.Close()).To(Succeed())
				requests := journey.Upstream.GetTokenRequests()
				Expect(requests).To(HaveLen(2))
				clientID, secret, err := helpers.CredentialTokenRequest(requests[1])
				Expect(err).NotTo(HaveOccurred())
				Expect(clientID).To(Equal(fixtures.CredentialClientID))
				Expect(secret).To(Equal(fixtures.RotatedCredentialSecret))
				form, err := url.ParseQuery(requests[1].Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(form.Get("grant_type")).To(Equal("refresh_token"))
				Expect(form.Get("refresh_token")).To(Equal("mock-refresh-token"))
				Expect(journey.Upstream.GetAuthorizationRequestCount()).To(Equal(1))
				Expect(sessionDetails()["id"]).To(Equal(originalSession["id"]))
			})
		})
	})

	Context("with portable target-deployment values and no mounted target Secret on the host", func() {
		var bindings map[string]map[string]string

		BeforeEach(func() {
			encoded, err := os.ReadFile("fixtures/oauth2-secret-file-values.yaml")
			Expect(err).NotTo(HaveOccurred())
			var values struct {
				Broker struct {
					ThirdPartyOAuth2 struct {
						CredentialFiles map[string]map[string]string `yaml:"credentialFiles"`
					} `yaml:"thirdPartyOauth2"`
				} `yaml:"broker"`
			}
			Expect(yaml.Unmarshal(encoded, &values)).To(Succeed())
			bindings = values.Broker.ThirdPartyOAuth2.CredentialFiles
			Expect(bindings).To(Equal(map[string]map[string]string{
				"zalando-platform": {
					"client_id_file":     "/meta/credentials/employee-client-id",
					"client_secret_file": "/meta/credentials/employee-client-secret",
				},
			}))
		})

		// US7-S3 from specs/051-oauth2-secret-file-overrides/spec.md.
		// TestHelmCredentialFilesTargetDeployment renders these same portable inputs.
		It("US7-S3 applies the target employee pair and permits explicit filesystem registration without inline placeholders", Label("oauth2-secret-file-overrides"), func() {
			for _, path := range []string{"/meta/credentials/employee-client-id", "/meta/credentials/employee-client-secret"} {
				_, err := os.Stat(path)
				Expect(os.IsNotExist(err)).To(BeTrue(), "the portable fixture must not consume host credentials")
			}
			startBroker(bindings, false)
			registered := registerFilesystem("zalando-platform")
			response, err := broker.AdminJSON(http.MethodGet, "/api/services/"+serviceID, nil)
			Expect(err).NotTo(HaveOccurred())
			metadata, err := helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(metadata).To(Equal(registered), "metadata must be available even though the target files are not mounted")
			logs.Reset()
			beforeAuthorization := journey.Upstream.GetAuthorizationRequestCount()
			beforeTokens := len(journey.Upstream.GetTokenRequests())
			response, err = broker.EndUserRequest(http.MethodGet, "/api/third-party/"+serviceID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(endUserURL+"/done"), nil)
			Expect(err).NotTo(HaveOccurred())
			failure, err := helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
			Expect(failure).To(HaveKeyWithValue("error", "internal_error"))
			Expect(journey.Upstream.GetAuthorizationRequestCount()).To(Equal(beforeAuthorization))
			Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(beforeTokens))
			Eventually(logs.Records).Should(ContainElement(And(
				HaveKeyWithValue("event", "session.oauth2.credential_source_failed"),
				HaveKeyWithValue("service_id", serviceID),
				HaveKeyWithValue("operation", "authorization_initiation"),
				HaveKeyWithValue("outcome", "failed"),
				HaveKeyWithValue("reason", "not_found"),
			)), "the actual loader must retain the target binding; missing_binding is not equivalent to an unavailable mounted target")
			response, err = broker.AdminJSON(http.MethodGet, "/api/services/"+serviceID, nil)
			Expect(err).NotTo(HaveOccurred())
			metadata, err = helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(metadata).To(Equal(registered), "a use-time source failure must not change filesystem registration")
		})
	})
})
