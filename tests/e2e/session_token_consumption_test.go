package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	domstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

// Session Token Consumption E2E Tests
//
// Covers FR-029 and SR-014 from specs/031-unified-session-token/spec.md:
// session-based consent submission using stateless JWE tokens across all agent modes.
// Anti-replay is enforced by the token TTL — there is no server-side consumed flag.

var _ = Describe("Session Token Grant Submission", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		server         *bootstrap.TestServer
		appInstance    *app.App
		principalStr   string
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		config := fixtures.DefaultOAuth2Config()
		serverFactory := bootstrap.NewServerFactory(config, logger)
		appInstance, err = serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())

		principalStr = fixtures.DefaultPrincipal().String()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// buildToken creates a valid JWE session token for the given agent and principal.
	buildToken := func(agentID id.AgentID, principal, originalURL string) string {
		claims, err := sessiontoken.NewAuthorizationSessionClaims(
			agentID,
			id.Principal(principal),
			originalURL,
			nil,
		)
		Expect(err).NotTo(HaveOccurred())
		token, err := appInstance.SessionTokenService.Create(claims)
		Expect(err).ToNot(HaveOccurred())
		return token
	}

	// buildExpiredToken creates a JWE token with a past ExpiresAt.
	buildExpiredToken := func(agentID id.AgentID, principal string) string {
		past := time.Now().Add(-1 * time.Hour)
		claims := &sessiontoken.AuthorizationSessionClaims{
			AgentID:     agentID,
			Principal:   id.Principal(principal),
			OriginalURL: "/oauth2/authorize?client_id=" + agentID.String(),
			IssuedAt:    past,
			ExpiresAt:   past,
		}
		token, err := appInstance.SessionTokenService.Create(claims)
		Expect(err).ToNot(HaveOccurred())
		return token
	}

	createAgent := func() *domstorage.Agent {
		now := time.Now()
		agent := &domstorage.Agent{
			ID:             id.NewAgentID(),
			ClientID:       ptr.To(id.NewClientID("test-client")),
			DisplayName:    "Session Consumption Agent",
			Description:    "E2E test agent for session consumption",
			RedirectURIs:   []string{"https://agent.example.com/callback"},
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
		Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(principalStr))).To(Succeed())
		return agent
	}

	grantBody := func() *bytes.Reader {
		body, _ := json.Marshal(map[string]interface{}{
			"granted_permission_sets": map[string][]string{
				fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()},
			},
		})
		return bytes.NewReader(body)
	}

	// Scenario 3.1 from specs/031-unified-session-token/spec.md
	Describe("when consent is submitted with a valid session_token", func() {
		It("creates the grant and returns redirect_url from the JWE claims OriginalURL", func() {
			agent := createAgent()
			originalURL := "/oauth2/authorize?client_id=" + agent.ID.String() + "&redirect_uri=https://agent.example.com/callback&scope=repo&response_type=code&state=xyz"
			token := buildToken(agent.ID, principalStr, originalURL)

			path := fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", agent.ID, token)
			resp, err := server.AuthenticatedPOST(path, principalStr, "application/json", grantBody())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			Expect(body).To(HaveKey("redirect_url"))
			Expect(body["redirect_url"]).To(Equal(originalURL))
			Expect(body).To(HaveKey("data"))
		})
	})

	// SR-014 from specs/031-unified-session-token/spec.md — expired token is rejected
	Describe("when consent is submitted with an expired session_token", func() {
		It("rejects with 400 Bad Request", func() {
			agent := createAgent()
			token := buildExpiredToken(agent.ID, principalStr)

			path := fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", agent.ID, token)
			resp, err := server.AuthenticatedPOST(path, principalStr, "application/json", grantBody())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	// SR-014 from specs/031-unified-session-token/spec.md — malformed token is rejected
	Describe("when consent is submitted with a malformed session_token", func() {
		It("rejects with 400 Bad Request", func() {
			agent := createAgent()

			path := fmt.Sprintf("/api/consent/agents/%s/grants?session_token=not-a-valid-jwe", agent.ID)
			resp, err := server.AuthenticatedPOST(path, principalStr, "application/json", grantBody())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	// SR-014 principal isolation from specs/031-unified-session-token/spec.md
	Describe("when session_token belongs to a different principal", func() {
		It("rejects with 403 Forbidden", func() {
			agent := createAgent()
			token := buildToken(agent.ID, "other-user@example.com",
				"/oauth2/authorize?client_id="+agent.ID.String(),
			)

			path := fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", agent.ID, token)
			resp, err := server.AuthenticatedPOST(path, principalStr, "application/json", grantBody())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		})
	})

	// SR-014 agent binding from specs/031-unified-session-token/spec.md
	Describe("when session_token was created for a different agent", func() {
		It("rejects with 400 Bad Request", func() {
			agent := createAgent()
			differentAgentID := id.NewAgentID()
			token := buildToken(differentAgentID, principalStr,
				"/oauth2/authorize?client_id="+differentAgentID.String(),
			)

			path := fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", agent.ID, token)
			resp, err := server.AuthenticatedPOST(path, principalStr, "application/json", grantBody())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})
})
