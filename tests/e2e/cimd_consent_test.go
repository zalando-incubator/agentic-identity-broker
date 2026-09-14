package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	domstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// createCIMDSessionToken builds a JWE authorization session token for use in CIMD tests.
func createCIMDSessionToken(svc *sessiontoken.Service, agentID id.AgentID, principal string, redirectURI string, meta *ports.SessionCIMDMetadata) string {
	q := url.Values{}
	q.Set("client_id", "https://agent.example.com/client")
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "repo")
	claims, err := sessiontoken.NewAuthorizationSessionClaims(
		agentID,
		id.Principal(principal),
		"/oauth2/authorize?"+q.Encode(),
		meta,
	)
	Expect(err).ToNot(HaveOccurred())
	token, err := svc.Create(claims)
	Expect(err).ToNot(HaveOccurred())
	return token
}

// createExpiredCIMDSessionToken builds a JWE token with a past ExpiresAt.
func createExpiredCIMDSessionToken(svc *sessiontoken.Service, agentID id.AgentID, principal string, meta *ports.SessionCIMDMetadata) string {
	past := time.Now().Add(-1 * time.Hour)
	claims := &sessiontoken.AuthorizationSessionClaims{
		AgentID:      agentID,
		Principal:    id.Principal(principal),
		OriginalURL:  "/oauth2/authorize?client_id=https://agent.example.com/client",
		CIMDMetadata: meta,
		IssuedAt:     past,
		ExpiresAt:    past,
	}
	token, err := svc.Create(claims)
	Expect(err).ToNot(HaveOccurred())
	return token
}

var _ = Describe("CIMD Consent Screen", func() {
	var (
		logger         *slog.Logger
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		server         *bootstrap.TestServer
		appInstance    *app.App
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
		serverFactory := bootstrap.NewServerFactory(config, logger)
		appInstance, err = serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 5.1 from specs/028-cimd-support/spec.md
	Describe("when the authorization request uses a CIMD-based client_id", func() {
		It("returns cimd_metadata with verified_domain when session_token is provided", func() {
			now := time.Now()
			agent := &domstorage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Example CIMD Agent",
				Description:    "E2E test agent for CIMD consent scenario",
				ClientURIs:     []string{"https://agent.example.com/client"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			token := createCIMDSessionToken(appInstance.SessionTokenService, agent.ID,
				fixtures.DefaultPrincipal().String(),
				"https://agent.example.com/callback",
				&ports.SessionCIMDMetadata{
					ClientID:     "https://agent.example.com/client",
					ClientName:   "Test CIMD Agent",
					RedirectURIs: []string{"https://agent.example.com/callback"},
				},
			)

			path := fmt.Sprintf("/api/consent/agents/%s?session_token=%s", agent.ID, token)
			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			data, ok := body["data"].(map[string]any)
			Expect(ok).To(BeTrue())

			cimdMeta, ok := data["cimd_metadata"].(map[string]any)
			Expect(ok).To(BeTrue(), "cimd_metadata should be present")
			Expect(cimdMeta["verified_domain"]).To(Equal("agent.example.com"))
		})
	})

	// Scenario 5.2 from specs/028-cimd-support/spec.md
	Describe("when redirect_uri points to localhost", func() {
		It("returns cimd_metadata with localhost redirect_uri", func() {
			now := time.Now()
			agent := &domstorage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Localhost Redirect Agent",
				Description:    "E2E test agent for localhost redirect CIMD scenario",
				ClientURIs:     []string{"https://agent.example.com/client"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			token := createCIMDSessionToken(appInstance.SessionTokenService, agent.ID,
				fixtures.DefaultPrincipal().String(),
				"http://localhost:3000/callback",
				&ports.SessionCIMDMetadata{
					ClientID:     "https://agent.example.com/client",
					ClientName:   "Test CIMD Agent",
					RedirectURIs: []string{"http://localhost:3000/callback"},
				},
			)

			path := fmt.Sprintf("/api/consent/agents/%s?session_token=%s", agent.ID, token)
			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			data, ok := body["data"].(map[string]any)
			Expect(ok).To(BeTrue())

			cimdMeta, ok := data["cimd_metadata"].(map[string]any)
			Expect(ok).To(BeTrue(), "cimd_metadata should be present")
			Expect(cimdMeta["redirect_uri"]).To(Equal("http://localhost:3000/callback"))
		})
	})

	// Scenario 5.3 from specs/028-cimd-support/spec.md
	Describe("when the authorization request uses a CIMD-based client_id with scope", func() {
		It("returns cimd_metadata with client_id_url, redirect_uri, and requested_scopes populated", func() {
			now := time.Now()
			agent := &domstorage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Advanced Detail Agent",
				Description:    "E2E test agent for CIMD advanced detail scenario",
				ClientURIs:     []string{"https://agent.example.com/client"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			svc := appInstance.SessionTokenService
			origQ := url.Values{}
			origQ.Set("client_id", "https://agent.example.com/client")
			origQ.Set("redirect_uri", "https://agent.example.com/callback")
			origQ.Set("scope", "repo read:user")
			claims, err := sessiontoken.NewAuthorizationSessionClaims(
				agent.ID,
				id.Principal(fixtures.DefaultPrincipal().String()),
				"/oauth2/authorize?"+origQ.Encode(),
				&ports.SessionCIMDMetadata{
					ClientID:     "https://agent.example.com/client",
					ClientName:   "Test CIMD Agent",
					RedirectURIs: []string{"https://agent.example.com/callback"},
				},
			)
			Expect(err).ToNot(HaveOccurred())
			token, err := svc.Create(claims)
			Expect(err).ToNot(HaveOccurred())

			path := fmt.Sprintf("/api/consent/agents/%s?session_token=%s", agent.ID, token)
			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			data, ok := body["data"].(map[string]any)
			Expect(ok).To(BeTrue())

			cimdMeta, ok := data["cimd_metadata"].(map[string]any)
			Expect(ok).To(BeTrue(), "cimd_metadata should be present")
			Expect(cimdMeta["client_id_url"]).To(Equal("https://agent.example.com/client"))
			Expect(cimdMeta["redirect_uri"]).To(Equal("https://agent.example.com/callback"))

			scopes, ok := cimdMeta["requested_scopes"].([]any)
			Expect(ok).To(BeTrue(), "requested_scopes should be an array")
			Expect(scopes).To(HaveLen(2))
		})
	})

	// Scenario 5.4 from specs/028-cimd-support/spec.md
	Describe("when the authorization request uses an opaque (UUID) client_id", func() {
		It("does not include cimd_metadata in the response", func() {
			now := time.Now()
			agentID := id.NewAgentID()
			agent := &domstorage.Agent{
				ID:             agentID,
				ClientID:       ptr.To(id.ClientID(agentID.String())),
				DisplayName:    "Opaque Client Agent",
				Description:    "E2E test agent for opaque client_id scenario",
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			path := fmt.Sprintf("/api/consent/agents/%s", agent.ID)
			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			data, ok := body["data"].(map[string]any)
			Expect(ok).To(BeTrue())

			_, hasCIMDMeta := data["cimd_metadata"]
			Expect(hasCIMDMeta).To(BeFalse(), "cimd_metadata should be absent for opaque client_id")
		})
	})

	// T119 Session Edge Cases from specs/028-cimd-support/spec.md

	// Scenario 5.5 from specs/028-cimd-support/spec.md
	Describe("when session_token carries an expired authorization session", func() {
		It("rejects with 400 Bad Request", func() {
			now := time.Now()
			agent := &domstorage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Expired Session Agent",
				Description:    "E2E test for expired session rejection",
				ClientURIs:     []string{"https://agent.example.com/client"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			token := createExpiredCIMDSessionToken(appInstance.SessionTokenService, agent.ID,
				fixtures.DefaultPrincipal().String(),
				&ports.SessionCIMDMetadata{
					ClientID:     "https://agent.example.com/client",
					ClientName:   "Test CIMD Agent",
					RedirectURIs: []string{"https://agent.example.com/callback"},
				},
			)

			path := fmt.Sprintf("/api/consent/agents/%s?session_token=%s", agent.ID, token)
			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	// Scenario 5.6 from specs/028-cimd-support/spec.md
	Describe("when session_token is malformed or invalid", func() {
		It("rejects with 400 Bad Request", func() {
			now := time.Now()
			agent := &domstorage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Invalid Token Agent",
				Description:    "E2E test for invalid token rejection",
				ClientURIs:     []string{"https://agent.example.com/client"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			path := fmt.Sprintf("/api/consent/agents/%s?session_token=not-a-valid-jwe-token", agent.ID)
			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	// Scenario 5.7 from specs/028-cimd-support/spec.md — session_token for wrong agent
	Describe("when session_token refers to a different agent", func() {
		It("rejects with 400 Bad Request", func() {
			now := time.Now()
			agent := &domstorage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Agent Mismatch Agent",
				Description:    "E2E test for agent mismatch rejection",
				ClientURIs:     []string{"https://agent.example.com/client"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			// Create token for a different (non-existent) agent ID
			differentAgentID := id.NewAgentID()
			token := createCIMDSessionToken(appInstance.SessionTokenService, differentAgentID,
				fixtures.DefaultPrincipal().String(),
				"https://agent.example.com/callback",
				&ports.SessionCIMDMetadata{
					ClientID:     "https://agent.example.com/client",
					ClientName:   "Test CIMD Agent",
					RedirectURIs: []string{"https://agent.example.com/callback"},
				},
			)

			path := fmt.Sprintf("/api/consent/agents/%s?session_token=%s", agent.ID, token)
			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	// SR-013 principal isolation from specs/028-cimd-support/spec.md
	Describe("when session_token belongs to a different principal", func() {
		It("rejects with 400 Bad Request", func() {
			now := time.Now()
			agent := &domstorage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Principal Isolation Agent",
				Description:    "E2E test for principal mismatch on GET consent",
				ClientURIs:     []string{"https://agent.example.com/client"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			// Token issued for a different user
			token := createCIMDSessionToken(appInstance.SessionTokenService, agent.ID,
				"other-user@example.com",
				"https://agent.example.com/callback",
				&ports.SessionCIMDMetadata{
					ClientID:     "https://agent.example.com/client",
					ClientName:   "Test CIMD Agent",
					RedirectURIs: []string{"https://agent.example.com/callback"},
				},
			)

			path := fmt.Sprintf("/api/consent/agents/%s?session_token=%s", agent.ID, token)
			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

})
