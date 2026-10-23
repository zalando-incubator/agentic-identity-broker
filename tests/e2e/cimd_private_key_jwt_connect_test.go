package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("CIMD private_key_jwt client authentication", func() {
	var (
		logger          *slog.Logger
		storageFactory  *bootstrap.StorageFactory
		testStorage     *storageadapter.Adapter
		server          *bootstrap.TestServer
		cimdServer      *httptest.Server
		agent           *storage.Agent
		clientURL       string
		redirectURI     string
		jwksURI         string
		clientDocument  []byte
		jwksDocument    []byte
		assertionKey    *helpers.ClientAssertionKey
		setDocument     func(string, map[string]any)
		authorizeCode   func(string) (string, string)
		signAssertion   func(helpers.ClientAssertionOptions) (string, string, error)
		exchangeCode    func(string, string, string, string) (*http.Response, error)
		exchangeRefresh func(string, string, string) (*http.Response, error)
	)

	const fakeHost = "private-key-jwt-e2e.test.invalid"

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).To(Succeed())

		clientURL = "https://" + fakeHost + "/client"
		redirectURI = "https://" + fakeHost + "/callback"
		jwksURI = "https://" + fakeHost + "/jwks.json"
		assertionKey, err = helpers.GenerateClientAssertionKey(jwa.RS256(), "client-key-1")
		Expect(err).To(Succeed())
		jwksDocument, err = assertionKey.JWKSJSON()
		Expect(err).To(Succeed())
		setDocument = func(authMethod string, extra map[string]any) {
			metadata := map[string]any{
				"client_id":      clientURL,
				"client_name":    "Private Key JWT E2E Agent",
				"redirect_uris":  []string{redirectURI},
				"grant_types":    []string{"authorization_code", "refresh_token"},
				"response_types": []string{"code"},
			}
			if authMethod != "" {
				metadata["token_endpoint_auth_method"] = authMethod
			}
			if authMethod == "private_key_jwt" {
				metadata["token_endpoint_auth_signing_alg"] = "RS256"
				metadata["jwks_uri"] = jwksURI
			}
			for name, value := range extra {
				metadata[name] = value
			}
			var err error
			clientDocument, err = json.Marshal(metadata)
			Expect(err).To(Succeed())
		}
		setDocument("none", nil)
		cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "max-age=300")
			if r.URL.Path == "/jwks.json" {
				_, _ = w.Write(jwksDocument)
				return
			}
			_, _ = w.Write(clientDocument)
		}))

		now := time.Now()
		agent = &storage.Agent{
			ID:             id.NewAgentID(),
			ClientURIs:     []string{clientURL},
			DisplayName:    "Private Key JWT Baseline Agent",
			Description:    "E2E baseline for unauthenticated CIMD clients",
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
		Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())

		config := fixtures.OAuth2ConfigWithCIMD("")
		serverFactory := bootstrap.NewServerFactory(config, logger)
		cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
		Expect(err).To(Succeed())
		keySetFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 65536)
		Expect(err).To(Succeed())
		server, err = bootstrap.NewCIMDEndUserTestServerWithFetchers(testStorage, serverFactory, cimdFetcher, keySetFetcher, logger)
		Expect(err).To(Succeed())

		authorizeCode = func(scope string) (string, string) {
			principal := fixtures.DefaultPrincipal().String()
			verifier := helpers.PKCEVerifier()
			challenge := helpers.GenerateCodeChallenge(verifier)
			authorizeURL := "/oauth2/authorize?" + url.Values{
				"response_type":         {"code"},
				"client_id":             {clientURL},
				"redirect_uri":          {redirectURI},
				"state":                 {"private-key-jwt-state"},
				"scope":                 {scope},
				"code_challenge":        {challenge},
				"code_challenge_method": {"S256"},
			}.Encode()

			authorizeResp, err := server.AuthenticatedGET(authorizeURL, principal)
			Expect(err).To(Succeed())
			Expect(authorizeResp.StatusCode).To(Equal(http.StatusFound))
			consentLocation, err := url.Parse(authorizeResp.Header.Get("Location"))
			Expect(err).To(Succeed())
			sessionToken := consentLocation.Query().Get("session_token")
			Expect(sessionToken).ToNot(BeEmpty())
			_ = authorizeResp.Body.Close()

			grantResp, err := server.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", agent.ID, url.QueryEscape(sessionToken)),
				principal,
				"application/json",
				bytes.NewReader(mustJSON(map[string]any{
					"granted_permission_sets": map[string][]string{
						fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()},
					},
				})),
			)
			Expect(err).To(Succeed())
			Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))
			var grantBody map[string]any
			Expect(json.NewDecoder(grantResp.Body).Decode(&grantBody)).To(Succeed())
			_ = grantResp.Body.Close()
			resumeURL, ok := grantBody["redirect_url"].(string)
			Expect(ok).To(BeTrue())

			codeResp, err := server.AuthenticatedGET(resumeURL, principal)
			Expect(err).To(Succeed())
			Expect(codeResp.StatusCode).To(Equal(http.StatusFound))
			callbackURL, err := url.Parse(codeResp.Header.Get("Location"))
			Expect(err).To(Succeed())
			code := callbackURL.Query().Get("code")
			Expect(code).ToNot(BeEmpty())
			_ = codeResp.Body.Close()
			return code, verifier
		}
		signAssertion = func(options helpers.ClientAssertionOptions) (string, string, error) {
			issuer := strings.TrimRight(server.App().Config.Server.EndUser.PublicURL, "/")
			return helpers.SignClientAssertion(clientURL, issuer+"/oauth2/token", assertionKey, options)
		}
		exchangeCode = func(code, verifier, assertionType, assertion string) (*http.Response, error) {
			form := url.Values{
				"grant_type":    {"authorization_code"},
				"client_id":     {clientURL},
				"code":          {code},
				"redirect_uri":  {redirectURI},
				"code_verifier": {verifier},
			}
			if assertionType != "" {
				form.Set("client_assertion_type", assertionType)
			}
			if assertion != "" {
				form.Set("client_assertion", assertion)
			}
			return server.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		}
		exchangeRefresh = func(refreshToken, assertionType, assertion string) (*http.Response, error) {
			form := url.Values{
				"grant_type":    {"refresh_token"},
				"client_id":     {clientURL},
				"refresh_token": {refreshToken},
			}
			if assertionType != "" {
				form.Set("client_assertion_type", assertionType)
			}
			if assertion != "" {
				form.Set("client_assertion", assertion)
			}
			return server.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		}
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if cimdServer != nil {
			cimdServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario US1-S5 from specs/051-private-key-jwt-auth/spec.md
	It("should keep code exchange and refresh working for none clients", func() {
		code, verifier := authorizeCode("read offline_access")

		tokenResp, err := exchangeCode(code, verifier, "", "")
		Expect(err).ToNot(HaveOccurred())
		Expect(tokenResp.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]any
		Expect(json.NewDecoder(tokenResp.Body).Decode(&tokenBody)).To(Succeed())
		_ = tokenResp.Body.Close()
		Expect(tokenBody).To(HaveKey("access_token"))
		refreshToken, ok := tokenBody["refresh_token"].(string)
		Expect(ok).To(BeTrue(), "token response: %#v", tokenBody)
		Expect(refreshToken).ToNot(BeEmpty())

		refreshResp, err := exchangeRefresh(refreshToken, "", "")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = refreshResp.Body.Close() }()
		Expect(refreshResp.StatusCode).To(Equal(http.StatusOK))
		var refreshedBody map[string]any
		Expect(json.NewDecoder(refreshResp.Body).Decode(&refreshedBody)).To(Succeed())
		Expect(refreshedBody).To(HaveKey("access_token"))
		Expect(refreshedBody["refresh_token"]).ToNot(Equal(refreshToken))
	})

	// Scenario US1-S1 from specs/051-private-key-jwt-auth/spec.md
	It("should exchange an authorization code with RS256", func() {
		setDocument("private_key_jwt", nil)
		code, verifier := authorizeCode("read offline_access")
		assertion, assertionType, err := signAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		response, err := exchangeCode(code, verifier, assertionType, assertion)
		Expect(err).To(Succeed())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		var body map[string]any
		Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
		Expect(body).To(HaveKey("access_token"))
		Expect(body).To(HaveKey("refresh_token"))
	})

	// Scenario US5-S1 from specs/051-private-key-jwt-auth/spec.md
	It("should exchange an authorization code with ES256", func() {
		var err error
		assertionKey, err = helpers.GenerateClientAssertionKey(jwa.ES256(), "client-key-es256")
		Expect(err).To(Succeed())
		jwksDocument, err = assertionKey.JWKSJSON()
		Expect(err).To(Succeed())
		setDocument("private_key_jwt", map[string]any{"token_endpoint_auth_signing_alg": "ES256"})

		code, verifier := authorizeCode("read offline_access")
		assertion, assertionType, err := signAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		response, err := exchangeCode(code, verifier, assertionType, assertion)
		Expect(err).To(Succeed())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		var body map[string]any
		Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
		Expect(body).To(HaveKey("access_token"))
		Expect(body).To(HaveKey("refresh_token"))
	})

	// Scenario US1-S2 from specs/051-private-key-jwt-auth/spec.md
	It("should refresh tokens with a fresh assertion and rotate the token", func() {
		setDocument("private_key_jwt", nil)
		code, verifier := authorizeCode("read offline_access")
		assertion, assertionType, err := signAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		response, err := exchangeCode(code, verifier, assertionType, assertion)
		Expect(err).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		var body map[string]any
		Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
		_ = response.Body.Close()
		refreshToken, ok := body["refresh_token"].(string)
		Expect(ok).To(BeTrue())

		freshAssertion, freshType, err := signAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		refreshResponse, err := exchangeRefresh(refreshToken, freshType, freshAssertion)
		Expect(err).To(Succeed())
		defer func() { _ = refreshResponse.Body.Close() }()
		Expect(refreshResponse.StatusCode).To(Equal(http.StatusOK))
		var refreshed map[string]any
		Expect(json.NewDecoder(refreshResponse.Body).Decode(&refreshed)).To(Succeed())
		Expect(refreshed).To(HaveKey("access_token"))
		Expect(refreshed["refresh_token"]).ToNot(Equal(refreshToken))
	})

	// Scenario US1-S3 from specs/051-private-key-jwt-auth/spec.md
	It("should accept a private_key_jwt document during authorization", func() {
		setDocument("private_key_jwt", nil)
		code, verifier := authorizeCode("read")
		Expect(code).ToNot(BeEmpty())
		Expect(verifier).ToNot(BeEmpty())
	})

	// Scenario US1-S4 from specs/051-private-key-jwt-auth/spec.md
	It("should use the singular method when the plural field lists none", func() {
		setDocument("private_key_jwt", map[string]any{
			"token_endpoint_auth_methods_supported": []string{"none", "private_key_jwt"},
		})
		code, verifier := authorizeCode("read offline_access")
		unauthenticated, err := exchangeCode(code, verifier, "", "")
		Expect(err).To(Succeed())
		Expect(unauthenticated.StatusCode).To(Equal(http.StatusUnauthorized))
		var errorBody map[string]any
		Expect(json.NewDecoder(unauthenticated.Body).Decode(&errorBody)).To(Succeed())
		_ = unauthenticated.Body.Close()
		Expect(errorBody["error"]).To(Equal("invalid_client"))

		assertion, assertionType, err := signAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		authenticated, err := exchangeCode(code, verifier, assertionType, assertion)
		Expect(err).To(Succeed())
		defer func() { _ = authenticated.Body.Close() }()
		Expect(authenticated.StatusCode).To(Equal(http.StatusOK))
	})
})

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	Expect(err).To(Succeed())
	return encoded
}
