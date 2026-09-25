package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("US2: Server Mode Configuration (local mode)", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
	})

	// Scenario 2.1 from specs/025-oauth2-server/spec.md
	It("starts in local mode", func() {
		config := fixtures.LocalConfig()
		testStorage, err := storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = storageFactory.CloseStorage(testStorage) }()

		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		Expect(app).ToNot(BeNil())
	})

	// Scenario 2.2 from specs/025-oauth2-server/spec.md
	It("no upstream URI needed in local mode", func() {
		config := fixtures.LocalConfig()
		Expect(config.OAuth2AuthServer.Proxy.UpstreamIssuerURI).To(BeEmpty())
		Expect(config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint).To(BeEmpty())

		testStorage, err := storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = storageFactory.CloseStorage(testStorage) }()

		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		Expect(app).ToNot(BeNil())
	})

	// Scenario 2.3 from specs/025-oauth2-server/spec.md
	It("default is proxy mode", func() {
		config := fixtures.DefaultOAuth2Config()
		Expect(config.OAuth2AuthServer.Mode).To(Equal(servermode.Proxy))
	})

	// Scenario 2.4 from specs/025-oauth2-server/spec.md
	It("auto-generates an initial signing key at startup", func() {
		config := fixtures.LocalConfig()
		testStorage, err := storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = storageFactory.CloseStorage(testStorage) }()

		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		adminServer, err := bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		defer adminServer.Close()

		enduserServer, err := bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		defer enduserServer.Close()

		resp, err := helpers.HTTPClient().Get(adminServer.BaseURL() + "/api/oauth2-server/signing-keys")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var listed map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&listed)).ToNot(HaveOccurred())
		items := listed["items"].([]interface{})
		Expect(items).To(HaveLen(1))

		key := items[0].(map[string]interface{})
		Expect(key["kid"]).ToNot(BeEmpty())
		Expect(key["is_current"]).To(BeTrue())

		jwkResp, err := helpers.HTTPClient().Get(enduserServer.BaseURL() + "/oauth2/jwks.json")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = jwkResp.Body.Close() }()
		Expect(jwkResp.StatusCode).To(Equal(http.StatusOK))

		var jwks map[string]interface{}
		Expect(json.NewDecoder(jwkResp.Body).Decode(&jwks)).ToNot(HaveOccurred())
		keys := jwks["keys"].([]interface{})
		Expect(keys).To(HaveLen(1))
		Expect(keys[0].(map[string]interface{})["kid"]).To(Equal(key["kid"]))
	})

	// Scenario 2.4 from specs/025-oauth2-server/spec.md
	It("issues tokens without manual signing key provisioning", func() {
		config := fixtures.LocalConfig()
		testStorage, err := storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = storageFactory.CloseStorage(testStorage) }()

		agent := fixtures.LocalAgent()
		Expect(testStorage.Agents().Create(context.Background(), agent)).ToNot(HaveOccurred())

		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		adminServer, err := bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		defer adminServer.Close()

		enduserServer, err := bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		defer enduserServer.Close()

		credentialsResp, err := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json",
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = credentialsResp.Body.Close() }()
		Expect(credentialsResp.StatusCode).To(Equal(http.StatusCreated))

		var credentials map[string]interface{}
		Expect(json.NewDecoder(credentialsResp.Body).Decode(&credentials)).ToNot(HaveOccurred())
		clientSecret, ok := credentials["client_secret"].(string)
		Expect(ok).To(BeTrue())
		Expect(clientSecret).ToNot(BeEmpty())

		form := url.Values{
			"grant_type":    {"client_credentials"},
			"client_id":     {agent.ID.String()},
			"client_secret": {clientSecret},
		}
		resp, err := enduserServer.PublicPOST(
			"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body["access_token"]).ToNot(BeEmpty())
		Expect(body["token_type"]).To(Equal("Bearer"))
	})
})
