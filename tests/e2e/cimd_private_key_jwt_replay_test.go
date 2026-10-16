package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("CIMD private_key_jwt assertion replay protection", func() {
	Context("with in-memory storage", func() {
		var fixture *cimdPrivateKeyJWTFixture

		BeforeEach(func() {
			var err error
			fixture, err = newCIMDPrivateKeyJWTFixture()
			Expect(err).To(Succeed())
			Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
		})

		AfterEach(func() {
			if fixture != nil {
				fixture.Close()
			}
		})

		// Scenario US3-S1 from specs/051-private-key-jwt-auth/spec.md
		It("rejects a second use of the same assertion", func() {
			code, verifier, err := fixture.AuthorizeCode("read offline_access")
			Expect(err).To(Succeed())
			assertion, assertionType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{JTI: "single-use-assertion"})
			Expect(err).To(Succeed())
			form := fixture.CodeForm(code, verifier)
			form.Set("client_assertion_type", assertionType)
			form.Set("client_assertion", assertion)
			first, err := fixture.PostToken(form, "")
			Expect(err).To(Succeed())
			Expect(first.StatusCode).To(Equal(http.StatusOK))
			var body map[string]any
			Expect(json.NewDecoder(first.Body).Decode(&body)).To(Succeed())
			_ = first.Body.Close()
			refreshToken, ok := body["refresh_token"].(string)
			Expect(ok).To(BeTrue())

			replayed := fixture.RefreshForm(refreshToken)
			replayed.Set("client_assertion_type", assertionType)
			replayed.Set("client_assertion", assertion)
			second, err := fixture.PostToken(replayed, "")
			Expect(err).To(Succeed())
			defer func() { _ = second.Body.Close() }()
			Expect(second.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		// Scenario US3-S3 from specs/051-private-key-jwt-auth/spec.md
		It("spends an assertion when the authenticated grant later fails PKCE", func() {
			code, verifier, err := fixture.AuthorizeCode("read")
			Expect(err).To(Succeed())
			assertion, assertionType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{JTI: "spent-before-pkce"})
			Expect(err).To(Succeed())
			wrongVerifier := fixture.CodeForm(code, "incorrect-verifier")
			wrongVerifier.Set("client_assertion_type", assertionType)
			wrongVerifier.Set("client_assertion", assertion)
			failedGrant, err := fixture.PostToken(wrongVerifier, "")
			Expect(err).To(Succeed())
			Expect(failedGrant.StatusCode).To(Equal(http.StatusBadRequest))
			_ = failedGrant.Body.Close()

			validGrant := fixture.CodeForm(code, verifier)
			validGrant.Set("client_assertion_type", assertionType)
			validGrant.Set("client_assertion", assertion)
			replay, err := fixture.PostToken(validGrant, "")
			Expect(err).To(Succeed())
			defer func() { _ = replay.Body.Close() }()
			Expect(replay.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		// Scenario US3-S4 from specs/051-private-key-jwt-auth/spec.md
		It("scopes the same jti independently for two clients", func() {
			clientB := "https://" + cimdPrivateKeyJWTFakeHost + "/client-b"
			redirectB := "https://" + cimdPrivateKeyJWTFakeHost + "/callback-b"
			agentB, err := fixture.AddClient(clientB, redirectB)
			Expect(err).To(Succeed())
			Expect(fixture.SetClientDocument(clientB, redirectB, "private_key_jwt", nil)).To(Succeed())

			codeA, verifierA, err := fixture.AuthorizeCode("read")
			Expect(err).To(Succeed())
			codeB, verifierB, err := fixture.AuthorizeCodeFor(clientB, redirectB, agentB.ID, "read")
			Expect(err).To(Succeed())
			for _, grant := range []struct {
				clientID    string
				redirectURI string
				code        string
				verifier    string
			}{
				{fixture.clientID, fixture.redirectURI, codeA, verifierA},
				{clientB, redirectB, codeB, verifierB},
			} {
				assertion, assertionType, err := fixture.SignAssertionFor(grant.clientID, fixture.assertionKey, helpers.ClientAssertionOptions{JTI: "shared-jti"})
				Expect(err).To(Succeed())
				form := fixture.CodeFormFor(grant.clientID, grant.redirectURI, grant.code, grant.verifier)
				form.Set("client_assertion_type", assertionType)
				form.Set("client_assertion", assertion)
				response, err := fixture.PostToken(form, "")
				Expect(err).To(Succeed())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				_ = response.Body.Close()
			}
		})
	})

	// Scenario US3-S2 from specs/051-private-key-jwt-auth/spec.md
	It("accepts at most one concurrent assertion across two brokers sharing PostgreSQL", Label("docker"), func() {
		Expect(bootstrap.CanAccessContainerRuntime()).To(Succeed())
		ctx := context.Background()
		postgres, err := bootstrap.NewPostgresFixture(ctx)
		Expect(err).To(Succeed())
		defer func() { Expect(postgres.Close(ctx)).To(Succeed()) }()

		storageConfig := ports.StorageConfig{
			Backend:  "postgres",
			Postgres: ports.PostgresConfig{ConnectionURL: postgres.ConnectionURL},
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second},
		}
		storageA, err := storageadapter.NewAdapter(&storageConfig)
		Expect(err).To(Succeed())
		defer func() { Expect(storageA.Close(ctx)).To(Succeed()) }()
		storageB, err := storageadapter.NewAdapter(&storageConfig)
		Expect(err).To(Succeed())
		defer func() { Expect(storageB.Close(ctx)).To(Succeed()) }()

		clientID := "https://" + cimdPrivateKeyJWTFakeHost + "/shared-client"
		redirectURI := "https://" + cimdPrivateKeyJWTFakeHost + "/shared-callback"
		key, err := helpers.GenerateClientAssertionKey(jwa.RS256(), "shared-client-key")
		Expect(err).To(Succeed())
		jwks, err := key.JWKSJSON()
		Expect(err).To(Succeed())
		document, err := json.Marshal(map[string]any{
			"client_id":                       clientID,
			"redirect_uris":                   []string{redirectURI},
			"grant_types":                     []string{"authorization_code", "refresh_token"},
			"response_types":                  []string{"code"},
			"token_endpoint_auth_method":      "private_key_jwt",
			"token_endpoint_auth_signing_alg": "RS256",
			"jwks_uri":                        "https://" + cimdPrivateKeyJWTFakeHost + "/jwks.json",
		})
		Expect(err).To(Succeed())
		metadataServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/jwks.json" {
				_, _ = w.Write(jwks)
				return
			}
			_, _ = w.Write(document)
		}))
		defer metadataServer.Close()

		now := time.Now()
		agent := &domainstorage.Agent{
			ID:             id.NewAgentID(),
			ClientURIs:     []string{clientID},
			DisplayName:    "Concurrent Replay Agent",
			Description:    "Two-broker replay acceptance test",
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(storageA.Services().Create(ctx, fixtures.ServiceWithID(fixtures.PlaceholderServiceID.String()))).To(Succeed())
		Expect(storageA.PermissionSets().Create(ctx, &domainstorage.PermissionSet{
			ID:          fixtures.PlaceholderPermissionSetID,
			Name:        "Replay E2E Permission Set",
			Description: "Permission set for shared PostgreSQL replay test",
			ServiceScopes: []domainstorage.ServiceScope{{
				ServiceID:       fixtures.PlaceholderServiceID,
				RequirementType: domainstorage.RequirementTypeMandatory,
			}},
		})).To(Succeed())
		Expect(storageA.UserSessions().Create(ctx, &domainstorage.UserSession{
			ID:                   id.NewSessionID(),
			Principal:            id.Principal(fixtures.DefaultPrincipal().String()),
			ServiceID:            fixtures.PlaceholderServiceID,
			EncryptedAccessToken: []byte("opaque-test-token"),
			TokenType:            "Bearer",
			Scope:                []string{},
			EncryptionContext:    domainstorage.EncryptionContext{ServiceID: fixtures.PlaceholderServiceID},
			InitiatedAt:          now,
			CreatedAt:            now,
			UpdatedAt:            now,
		})).To(Succeed())
		Expect(storageA.Agents().Create(ctx, agent)).To(Succeed())

		logger := bootstrap.TestLogger(slog.LevelError)
		configA := fixtures.OAuth2ConfigWithCIMD("")
		configB := fixtures.OAuth2ConfigWithCIMD("")
		factoryA := bootstrap.NewServerFactory(configA, logger)
		factoryB := bootstrap.NewServerFactory(configB, logger)
		documentFetcherA, err := bootstrap.NewCIMDTestFetcher(metadataServer, cimdPrivateKeyJWTFakeHost, 5120)
		Expect(err).To(Succeed())
		keyFetcherA, err := bootstrap.NewCIMDTestFetcher(metadataServer, cimdPrivateKeyJWTFakeHost, 65536)
		Expect(err).To(Succeed())
		documentFetcherB, err := bootstrap.NewCIMDTestFetcher(metadataServer, cimdPrivateKeyJWTFakeHost, 5120)
		Expect(err).To(Succeed())
		keyFetcherB, err := bootstrap.NewCIMDTestFetcher(metadataServer, cimdPrivateKeyJWTFakeHost, 65536)
		Expect(err).To(Succeed())
		serverA, err := bootstrap.NewCIMDEndUserTestServerWithFetchers(storageA, factoryA, documentFetcherA, keyFetcherA, logger)
		Expect(err).To(Succeed())
		defer serverA.Close()
		serverB, err := bootstrap.NewCIMDEndUserTestServerWithFetchers(storageB, factoryB, documentFetcherB, keyFetcherB, logger)
		Expect(err).To(Succeed())
		defer serverB.Close()

		fixture := &cimdPrivateKeyJWTFixture{
			server: serverA, agent: agent, clientID: clientID, redirectURI: redirectURI,
			assertionKey: key, logger: logger,
		}
		code, verifier, err := fixture.AuthorizeCode("read")
		Expect(err).To(Succeed())
		tokenURLA := strings.TrimRight(serverA.App().Config.Server.EndUser.PublicURL, "/") + "/oauth2/token"
		tokenURLB := strings.TrimRight(serverB.App().Config.Server.EndUser.PublicURL, "/") + "/oauth2/token"
		assertion, assertionType, err := helpers.SignClientAssertion(clientID, tokenURLA, key, helpers.ClientAssertionOptions{
			Audience: []string{tokenURLA, tokenURLB},
		})
		Expect(err).To(Succeed())
		form := fixture.CodeForm(code, verifier)
		form.Set("client_assertion_type", assertionType)
		form.Set("client_assertion", assertion)

		var wait sync.WaitGroup
		responses := make(chan *http.Response, 2)
		errors := make(chan error, 2)
		for _, server := range []*bootstrap.TestServer{serverA, serverB} {
			wait.Add(1)
			go func(server *bootstrap.TestServer) {
				defer wait.Done()
				response, err := server.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
				if err != nil {
					errors <- err
					return
				}
				responses <- response
			}(server)
		}
		wait.Wait()
		close(responses)
		close(errors)
		for err := range errors {
			Expect(err).ToNot(HaveOccurred())
		}
		statusCounts := map[int]int{}
		for response := range responses {
			statusCounts[response.StatusCode]++
			_ = response.Body.Close()
		}
		Expect(statusCounts[http.StatusOK]).To(Equal(1), fmt.Sprintf("responses: %#v", statusCounts))
		Expect(statusCounts[http.StatusUnauthorized]).To(Equal(1), fmt.Sprintf("responses: %#v", statusCounts))
	})
})
