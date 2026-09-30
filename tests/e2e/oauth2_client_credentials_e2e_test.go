package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("US1: Client Credential Management (local mode)", func() {
	var (
		adminServer    *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
		agent          *domainstorage.Agent
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		config := fixtures.LocalConfig()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		agent = fixtures.ValidAgent()
		Expect(testStorage.Agents().Create(context.Background(), agent)).ToNot(HaveOccurred())

		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 1.1 from specs/025-oauth2-server/spec.md
	It("generates credentials for an agent", func() {
		resp, err := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json", nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body).To(HaveKey("client_id"))
		Expect(body).To(HaveKey("client_secret"))
		Expect(body).To(HaveKey("created_at"))
	})

	// Scenario 1.2 from specs/025-oauth2-server/spec.md: only an accepted rotation invalidates the old credential.
	It("rejects hostile credential rotations and accepts an operator rotation", func() {
		credentialPath := "/api/agents/" + agent.ID.String() + "/client-credentials"
		adminPrincipal := fixtures.AdminPrincipal().String()
		resp, err := adminServer.AuthenticatedPOST(credentialPath, adminPrincipal, "application/json", nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		var initialCredentials map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&initialCredentials)).To(Succeed())
		_ = resp.Body.Close()
		initialSecret, ok := initialCredentials["client_secret"].(string)
		Expect(ok).To(BeTrue())
		Expect(initialSecret).NotTo(BeEmpty())

		readMetadata := func() map[string]any {
			response, err := adminServer.AuthenticatedGET(credentialPath, adminPrincipal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = response.Body.Close() }()
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			var metadata map[string]any
			Expect(json.NewDecoder(response.Body).Decode(&metadata)).To(Succeed())
			Expect(metadata).NotTo(HaveKey("client_secret"))
			return metadata
		}
		originalMetadata := readMetadata()
		Expect(originalMetadata).To(HaveKeyWithValue("client_id", initialCredentials["client_id"]))
		Expect(originalMetadata).To(HaveKeyWithValue("created_at", initialCredentials["created_at"]))
		Expect(originalMetadata).NotTo(HaveKey("rotated_at"))

		for _, attack := range []struct {
			name, contentType, origin, fetchSite, host string
			status                                     int
		}{
			{name: "simple content type with no body", contentType: "text/plain", status: http.StatusUnsupportedMediaType},
			{name: "cross-origin browser", contentType: "application/json", origin: "https://attacker.example", fetchSite: "cross-site", status: http.StatusForbidden},
			{name: "same-origin rebinding host", contentType: "application/json", origin: "http://rebind.attacker.test", fetchSite: "same-origin", host: "rebind.attacker.test", status: http.StatusForbidden},
		} {
			By(attack.name)
			request, err := http.NewRequest(http.MethodPost, adminServer.BaseURL()+credentialPath, nil)
			Expect(err).ToNot(HaveOccurred())
			request.Header.Set("X-Remote-User", adminPrincipal)
			request.Header.Set("Content-Type", attack.contentType)
			if attack.origin != "" {
				request.Header.Set("Origin", attack.origin)
				request.Header.Set("Sec-Fetch-Site", attack.fetchSite)
			}
			if attack.host != "" {
				request.Host = attack.host
			}
			response, err := helpers.HTTPClient().Do(request)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.StatusCode).To(Equal(attack.status))
			responseBody, err := helpers.ReadResponseBody(response)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).NotTo(ContainSubstring("client_secret"))
			Expect(responseBody).NotTo(ContainSubstring(initialSecret))
			Expect(readMetadata()).To(Equal(originalMetadata), "rejected rotations must leave the stored credential unchanged")
		}

		resp, err = adminServer.AuthenticatedPOST(credentialPath, adminPrincipal, "application/json", nil)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		var rotatedCredentials map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&rotatedCredentials)).To(Succeed())
		Expect(rotatedCredentials["client_id"]).To(Equal(initialCredentials["client_id"]))
		rotatedSecret, ok := rotatedCredentials["client_secret"].(string)
		Expect(ok).To(BeTrue())
		Expect(rotatedSecret).NotTo(BeEmpty())
		Expect(rotatedSecret).NotTo(Equal(initialSecret))
		Expect(rotatedCredentials).To(HaveKey("previous_invalidated_at"))
		Expect(readMetadata()).To(HaveKey("rotated_at"))
	})

	// Scenario 1.3 from specs/025-oauth2-server/spec.md
	It("returns 404 for missing agent", func() {
		resp, err := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/00000000-0000-0000-0000-000000000099/client-credentials",
			"application/json", nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	// Scenario 1.4 from specs/025-oauth2-server/spec.md
	It("gets credential metadata without secret", func() {
		resp, _ := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json", nil,
		)
		_ = resp.Body.Close()

		resp, err := helpers.HTTPClient().Get(
			adminServer.BaseURL() + "/api/agents/" + agent.ID.String() + "/client-credentials",
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body).To(HaveKey("client_id"))
		Expect(body).NotTo(HaveKey("client_secret"))
	})
})
