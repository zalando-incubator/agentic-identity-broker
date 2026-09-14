package e2e_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("CIMD wildcard client URI registration", func() {
	const fakeHost = "cimd-wildcard.test.invalid"

	var (
		logger         *slog.Logger
		logCapture     *bootstrap.BufferedLogCapture
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
	)

	BeforeEach(func() {
		logger, logCapture = bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	newAgent := func(name string, clientURIs []string) *storage.Agent {
		now := time.Now()
		return &storage.Agent{
			ID:             id.NewAgentID(),
			ClientURIs:     clientURIs,
			DisplayName:    name,
			Description:    "E2E test agent for CIMD wildcard client URI registration",
			PermissionSets: fixtures.DefaultPermissionSets(),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}

	newServer := func(clientURL, redirectURI string) (*bootstrap.TestServer, *httptest.Server) {
		cimdServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cimdDocument(clientURL, []string{redirectURI}))
		}))

		config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
		serverFactory := bootstrap.NewServerFactory(config, logger)
		cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
		Expect(err).ToNot(HaveOccurred())
		appInstance, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
		Expect(err).ToNot(HaveOccurred())
		server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())
		return server, cimdServer
	}

	authorize := func(server *bootstrap.TestServer, clientURL, redirectURI string) *http.Response {
		resp, err := server.AuthenticatedGET(
			fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz", clientURL, redirectURI),
			fixtures.DefaultPrincipal().String(),
		)
		Expect(err).ToNot(HaveOccurred())
		return resp
	}

	// User Story 1, acceptance scenario 1 from specs/028c-cimd-wildcard-client-uris/spec.md.
	It("resolves a concrete hosted client URL through a path-segment wildcard", func() {
		clientURL := "https://" + fakeHost + "/oauth/codex/dIwd44EtAHp-/client.json"
		redirectURI := "https://" + fakeHost + "/callback"
		agent := newAgent("Wildcard Agent", []string{"https://" + fakeHost + "/oauth/codex/*/client.json"})
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
		server, cimdServer := newServer(clientURL, redirectURI)
		defer server.Close()
		defer cimdServer.Close()

		resp := authorize(server, clientURL, redirectURI)
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(resp.Header.Get("Location"))
		Expect(err).ToNot(HaveOccurred())
		Expect(location.Path).To(ContainSubstring("/agents/" + agent.ID.String()))
	})

	// User Story 1, acceptance scenario 2 from specs/028c-cimd-wildcard-client-uris/spec.md.
	It("rejects a client URL with an extra path segment", func() {
		clientURL := "https://" + fakeHost + "/oauth/codex/tenant/nested/client.json"
		redirectURI := "https://" + fakeHost + "/callback"
		agent := newAgent("Wildcard Agent", []string{"https://" + fakeHost + "/oauth/codex/*/client.json"})
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
		server, cimdServer := newServer(clientURL, redirectURI)
		defer server.Close()
		defer cimdServer.Close()

		resp := authorize(server, clientURL, redirectURI)
		defer func() { _ = resp.Body.Close() }()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
		Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
	})

	// User Story 1, acceptance scenario 3 from specs/028c-cimd-wildcard-client-uris/spec.md.
	It("rejects a client URL with a different literal path segment", func() {
		clientURL := "https://" + fakeHost + "/oauth/other/tenant/client.json"
		redirectURI := "https://" + fakeHost + "/callback"
		agent := newAgent("Wildcard Agent", []string{"https://" + fakeHost + "/oauth/codex/*/client.json"})
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
		server, cimdServer := newServer(clientURL, redirectURI)
		defer server.Close()
		defer cimdServer.Close()

		resp := authorize(server, clientURL, redirectURI)
		defer func() { _ = resp.Body.Close() }()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
		Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
	})

	// User Story 2, acceptance scenario 1 from specs/028c-cimd-wildcard-client-uris/spec.md.
	It("uses an exact client URI before a matching pattern", func() {
		clientURL := "https://" + fakeHost + "/oauth/codex/literal/client.json"
		redirectURI := "https://" + fakeHost + "/callback"
		patternAgent := newAgent("Pattern Agent", []string{"https://" + fakeHost + "/oauth/codex/*/client.json"})
		exactAgent := newAgent("Exact Agent", []string{clientURL})
		Expect(testStorage.Agents().Create(context.Background(), patternAgent)).To(Succeed())
		Expect(testStorage.Agents().Create(context.Background(), exactAgent)).To(Succeed())
		server, cimdServer := newServer(clientURL, redirectURI)
		defer server.Close()
		defer cimdServer.Close()

		resp := authorize(server, clientURL, redirectURI)
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusFound))
		Expect(resp.Header.Get("Location")).To(ContainSubstring("/agents/" + exactAgent.ID.String()))
	})

	// User Story 3, acceptance scenario 1 from specs/028c-cimd-wildcard-client-uris/spec.md.
	It("rejects a client URL that matches patterns on different agents", func() {
		clientURL := "https://" + fakeHost + "/oauth/test/foo/client.json"
		redirectURI := "https://" + fakeHost + "/callback"
		first := newAgent("First Pattern Agent", []string{"https://" + fakeHost + "/oauth/*/foo/client.json"})
		second := newAgent("Second Pattern Agent", []string{"https://" + fakeHost + "/oauth/test/*/client.json"})
		Expect(testStorage.Agents().Create(context.Background(), first)).To(Succeed())
		Expect(testStorage.Agents().Create(context.Background(), second)).To(Succeed())
		server, cimdServer := newServer(clientURL, redirectURI)
		defer server.Close()
		defer cimdServer.Close()

		logCapture.Reset()

		resp := authorize(server, clientURL, redirectURI)
		defer func() { _ = resp.Body.Close() }()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
		Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		records, err := logCapture.Records()
		Expect(err).ToNot(HaveOccurred())
		found := false
		for _, record := range records {
			if record["msg"] == "cimd_client_uri_ambiguous" && record["client_uri"] == clientURL {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue())
	})
})
