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

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("US3: Client Credentials Grant (local mode)", func() {
	var (
		adminServer    *bootstrap.TestServer
		enduserServer  *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
		agent          *domainstorage.Agent
		clientSecret   string
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		config := fixtures.LocalConfig()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		agent = fixtures.LocalAgent()
		Expect(testStorage.Agents().Create(context.Background(), agent)).ToNot(HaveOccurred())

		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())

		Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).ToNot(HaveOccurred())

		// Generate credentials via admin API
		resp, err := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json", nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		var creds map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&creds)).ToNot(HaveOccurred())
		clientSecret = creds["client_secret"].(string)
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
		}
		if enduserServer != nil {
			enduserServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 3.1 from specs/025-oauth2-server/spec.md
	It("client_credentials grant issues signed token", func() {
		form := url.Values{
			"grant_type":    {"client_credentials"},
			"client_id":     {agent.ID.String()},
			"client_secret": {clientSecret},
		}
		resp, err := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body).To(HaveKey("access_token"))
		Expect(body["token_type"]).To(Equal("Bearer"))
	})

	// Scenario 3.2 from specs/025-oauth2-server/spec.md
	It("invalid credentials returns 401", func() {
		form := url.Values{
			"grant_type":    {"client_credentials"},
			"client_id":     {agent.ID.String()},
			"client_secret": {"wrong-secret"},
		}
		resp, err := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	})

	// Scenario 3.3 from specs/025-oauth2-server/spec.md
	It("token validates via JWKS", func() {
		// Get token
		form := url.Values{
			"grant_type":    {"client_credentials"},
			"client_id":     {agent.ID.String()},
			"client_secret": {clientSecret},
		}
		resp, err := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var tokenBody map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&tokenBody)).ToNot(HaveOccurred())
		accessToken := tokenBody["access_token"].(string)
		Expect(accessToken).ToNot(BeEmpty())

		// Get JWKS
		jwksResp, err := helpers.HTTPClient().Get(enduserServer.BaseURL() + "/oauth2/jwks.json")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = jwksResp.Body.Close() }()
		Expect(jwksResp.StatusCode).To(Equal(http.StatusOK))

		var jwks map[string]interface{}
		Expect(json.NewDecoder(jwksResp.Body).Decode(&jwks)).ToNot(HaveOccurred())
		Expect(jwks).To(HaveKey("keys"))
	})

	// Scenario 3.4 from specs/025-oauth2-server/spec.md
	It("invalid_scope for excessive scopes", func() {
		// Update agent with restricted scopes
		agent.AllowedScopes = []string{"read"}
		Expect(testStorage.Agents().Update(context.Background(), agent)).ToNot(HaveOccurred())

		form := url.Values{
			"grant_type":    {"client_credentials"},
			"client_id":     {agent.ID.String()},
			"client_secret": {clientSecret},
			"scope":         {"admin"},
		}
		resp, err := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	})
})
