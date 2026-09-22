package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Aggregated JWKS Endpoint", func() {
	var (
		storageFactory *bootstrap.StorageFactory
		logger         *slog.Logger
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
	})

	// ==================== User Story 1: Unified Token Verification Surface ====================

	Describe("local mode JWKS", func() {
		var (
			testStorage *storageadapter.Adapter
			server      *bootstrap.TestServer
		)

		BeforeEach(func() {
			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			config := fixtures.LocalConfig()
			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			adminSrv, err := bootstrap.NewAdminTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(adminSrv.Close)
			Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
			if testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		// Scenario 1.1 from specs/032-aggregated-jwks/spec.md
		It("returns only local signing keys", func() {
			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var jwks map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
			Expect(jwks).To(HaveKey("keys"))
			kids := extractKids(jwks)
			Expect(kids).To(HaveLen(1))
			Expect(kids).ToNot(ContainElement("test-key"))
		})

		// FR-029 from specs/046-cimd-upstream-client/spec.md
		It("excludes CIMD client-authentication keys while retaining token-signing keys", func() {
			cimdKID := seedCIMDClientAuthenticationKey(testStorage)
			tokenSigningKey, err := testStorage.SigningKeys().GetCurrentInDomain(context.Background(), domstorage.KeyDomainTokenSigning)
			Expect(err).ToNot(HaveOccurred())

			jwks := fetchJWKSJSON(server.BaseURL() + "/oauth2/jwks.json")
			Expect(extractKids(jwks)).To(ConsistOf(tokenSigningKey.KID.String()))
			Expect(extractKids(jwks)).ToNot(ContainElement(cimdKID))
		})

		// Scenario 1.7 from specs/032-aggregated-jwks/spec.md
		It("never exposes private key material", func() {
			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var jwks map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
			for _, k := range jwks["keys"].([]interface{}) {
				keyMap := k.(map[string]interface{})
				Expect(keyMap).ToNot(HaveKey("d"))
				Expect(keyMap).ToNot(HaveKey("p"))
				Expect(keyMap).ToNot(HaveKey("q"))
				Expect(keyMap).ToNot(HaveKey("dp"))
				Expect(keyMap).ToNot(HaveKey("dq"))
				Expect(keyMap).ToNot(HaveKey("qi"))
			}
		})

		// Scenario 1.8 from specs/032-aggregated-jwks/spec.md
		It("includes Cache-Control: public, max-age=300", func() {
			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Cache-Control")).To(ContainSubstring("public"))
			Expect(resp.Header.Get("Cache-Control")).To(ContainSubstring("max-age=300"))
		})
	})

	Describe("proxy mode JWKS", func() {
		var (
			testStorage  *storageadapter.Adapter
			mockUpstream *helpers.MockUpstreamOAuth2Server
			server       *bootstrap.TestServer
		)

		BeforeEach(func() {
			mockUpstream = helpers.NewMockUpstreamOAuth2Server()
			config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.URL())

			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(app, logger)
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

		// Scenario 1.2 from specs/032-aggregated-jwks/spec.md
		It("republishes only upstream keys", func() {
			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var jwks map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
			Expect(jwks).To(HaveKey("keys"))
			Expect(extractKids(jwks)).To(ConsistOf("test-key"))
		})

		// Scenario 1.8 from specs/032-aggregated-jwks/spec.md
		It("includes Cache-Control: public, max-age=300", func() {
			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Cache-Control")).To(ContainSubstring("public"))
			Expect(resp.Header.Get("Cache-Control")).To(ContainSubstring("max-age=300"))
		})

		// Scenario 1.6 from specs/032-aggregated-jwks/spec.md
		It("validates an upstream-issued token with the broker-hosted JWKS", func() {
			upstreamToken, err := helpers.SignTestJWT(
				helpers.NewJWTClaims().
					WithIssuer(mockUpstream.URL()).
					WithClaim("scope", "openid profile").
					Build(),
				mockUpstream.GetPrivateKeyPEM(),
			)
			Expect(err).ToNot(HaveOccurred())

			Expect(validateJWTWithJWKS(upstreamToken, fetchBrokerJWKS(server.BaseURL()))).To(Succeed())
		})
	})

	Describe("hybrid mode JWKS", func() {
		var (
			testStorage  *storageadapter.Adapter
			mockUpstream *helpers.MockUpstreamOAuth2Server
			adminServer  *bootstrap.TestServer
			server       *bootstrap.TestServer
		)

		BeforeEach(func() {
			mockUpstream = helpers.NewMockUpstreamOAuth2Server()
			config := fixtures.HybridConfig(mockUpstream.URL())

			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			adminServer, err = bootstrap.NewAdminTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if adminServer != nil {
				adminServer.Close()
			}
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

		// Scenario 1.3 from specs/032-aggregated-jwks/spec.md
		It("returns union of local and upstream keys", func() {
			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var jwks map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
			Expect(jwks).To(HaveKey("keys"))
			keys := jwks["keys"].([]interface{})
			// At least local key + upstream key
			Expect(len(keys)).To(BeNumerically(">=", 2))
		})

		// Scenario 1.4 from specs/032-aggregated-jwks/spec.md
		It("validates a locally-issued token with the broker-hosted JWKS", func() {
			agent := fixtures.LocalAgent()
			Expect(testStorage.Agents().Create(context.Background(), agent)).ToNot(HaveOccurred())

			credResp, err := http.Post(
				adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
				"application/json",
				nil,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = credResp.Body.Close() }()
			Expect(credResp.StatusCode).To(Equal(http.StatusCreated))

			var creds map[string]interface{}
			Expect(json.NewDecoder(credResp.Body).Decode(&creds)).ToNot(HaveOccurred())
			clientSecret := creds["client_secret"].(string)

			tokenResp, err := http.Post(
				server.BaseURL()+"/oauth2/token",
				"application/x-www-form-urlencoded",
				strings.NewReader(url.Values{
					"grant_type":    {"client_credentials"},
					"client_id":     {agent.ID.String()},
					"client_secret": {clientSecret},
				}.Encode()),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = tokenResp.Body.Close() }()
			Expect(tokenResp.StatusCode).To(Equal(http.StatusOK))

			var tokenBody map[string]interface{}
			Expect(json.NewDecoder(tokenResp.Body).Decode(&tokenBody)).ToNot(HaveOccurred())
			accessToken, ok := tokenBody["access_token"].(string)
			Expect(ok).To(BeTrue())
			Expect(accessToken).ToNot(BeEmpty())

			Expect(validateJWTWithJWKS(accessToken, fetchBrokerJWKS(server.BaseURL()))).To(Succeed())
		})

		// Scenario 1.8 from specs/032-aggregated-jwks/spec.md
		It("includes Cache-Control: public, max-age=300", func() {
			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Cache-Control")).To(ContainSubstring("public"))
			Expect(resp.Header.Get("Cache-Control")).To(ContainSubstring("max-age=300"))
		})

		// Scenario 1.5 from specs/032-aggregated-jwks/spec.md
		It("validates an upstream-issued token with the broker-hosted JWKS", func() {
			upstreamToken, err := helpers.SignTestJWT(
				helpers.NewJWTClaims().
					WithIssuer(mockUpstream.URL()).
					WithClaim("scope", "openid profile").
					Build(),
				mockUpstream.GetPrivateKeyPEM(),
			)
			Expect(err).ToNot(HaveOccurred())

			Expect(validateJWTWithJWKS(upstreamToken, fetchBrokerJWKS(server.BaseURL()))).To(Succeed())
		})

		// FR-029 from specs/046-cimd-upstream-client/spec.md
		It("retains token and upstream keys while excluding CIMD client-authentication keys", func() {
			cimdKID := seedCIMDClientAuthenticationKey(testStorage)
			tokenSigningKey, err := testStorage.SigningKeys().GetCurrentInDomain(context.Background(), domstorage.KeyDomainTokenSigning)
			Expect(err).ToNot(HaveOccurred())

			jwks := fetchJWKSJSON(server.BaseURL() + "/oauth2/jwks.json")
			Expect(extractKids(jwks)).To(ConsistOf(tokenSigningKey.KID.String(), "test-key"))
			Expect(extractKids(jwks)).ToNot(ContainElement(cimdKID))
		})
	})

	// ==================== User Story 2: Discovery Advertises Broker-Hosted JWKS ====================

	Describe("discovery endpoint includes jwks_uri in all modes", func() {
		// Scenario 2.1 from specs/032-aggregated-jwks/spec.md
		It("local mode discovery includes jwks_uri", func() {
			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.LocalConfig()
			app, err := bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := http.Get(server.BaseURL() + "/.well-known/oauth-authorization-server")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
			Expect(body).To(HaveKey("jwks_uri"))
			Expect(body["jwks_uri"].(string)).To(ContainSubstring("/oauth2/jwks.json"))
		})

		// Scenario 2.2 from specs/032-aggregated-jwks/spec.md
		It("proxy mode discovery includes jwks_uri pointing to broker endpoint", func() {
			mockUpstream := helpers.NewMockUpstreamOAuth2Server()
			defer mockUpstream.Close()

			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.URL())
			app, err := bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := http.Get(server.BaseURL() + "/.well-known/oauth-authorization-server")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
			Expect(body).To(HaveKey("jwks_uri"))
			// Must point to the broker endpoint, not the upstream server
			jwksURI := body["jwks_uri"].(string)
			Expect(jwksURI).To(ContainSubstring("/oauth2/jwks.json"))
			Expect(jwksURI).ToNot(ContainSubstring(mockUpstream.URL()))
		})

		// Scenario 2.3 from specs/032-aggregated-jwks/spec.md
		It("hybrid mode discovery includes jwks_uri", func() {
			mockUpstream := helpers.NewMockUpstreamOAuth2Server()
			defer mockUpstream.Close()

			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.HybridConfig(mockUpstream.URL())
			app, err := bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := http.Get(server.BaseURL() + "/.well-known/oauth-authorization-server")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
			Expect(body).To(HaveKey("jwks_uri"))
			Expect(body["jwks_uri"].(string)).To(ContainSubstring("/oauth2/jwks.json"))
		})

		// Scenario 2.4 from specs/032-aggregated-jwks/spec.md
		It("proxy mode jwks_uri from discovery resolves to upstream keys", func() {
			mockUpstream := helpers.NewMockUpstreamOAuth2Server()
			defer mockUpstream.Close()

			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.URL())
			app, err := bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			// Verify the discovery document includes a jwks_uri ending in /oauth2/jwks.json
			discResp, err := http.Get(server.BaseURL() + "/.well-known/oauth-authorization-server")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = discResp.Body.Close() }()

			var discBody map[string]interface{}
			Expect(json.NewDecoder(discResp.Body).Decode(&discBody)).ToNot(HaveOccurred())
			jwksURI := discBody["jwks_uri"].(string)
			Expect(jwksURI).To(ContainSubstring("/oauth2/jwks.json"))

			// Follow the path portion of jwks_uri against the test server
			// (the config public URL may differ from the test server's bound address)
			jwksResp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = jwksResp.Body.Close() }()
			Expect(jwksResp.StatusCode).To(Equal(http.StatusOK))

			var jwks map[string]interface{}
			Expect(json.NewDecoder(jwksResp.Body).Decode(&jwks)).ToNot(HaveOccurred())
			Expect(jwks).To(HaveKey("keys"))
			// Upstream keys are present (mock serves kid "test-key")
			Expect(extractKids(jwks)).To(ContainElement("test-key"))
		})
	})

	// ==================== User Story 3: Fail-Closed Upstream JWKS Availability ====================

	Describe("upstream unavailability", func() {
		// Scenario 3.1 from specs/032-aggregated-jwks/spec.md
		It("proxy mode fails startup when upstream metadata is unreachable", func() {
			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.OAuth2ConfigWithUpstream("http://localhost:19099")
			_, err = bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to discover OAuth2 server metadata"))
		})

		// Scenario 3.2 from specs/032-aggregated-jwks/spec.md
		It("hybrid mode fails startup when upstream metadata is unreachable", func() {
			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.HybridConfig("http://localhost:19099")
			_, err = bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to discover OAuth2 server metadata"))
		})

		// Scenario 3.3 from specs/032-aggregated-jwks/spec.md
		// Full-stack test: upstream is discoverable but its JWKS endpoint returns errors.
		// The adapter is created (non-nil) but GetKeySet fails at runtime.
		It("proxy mode returns 503 when upstream JWKS endpoint returns errors", func() {
			brokenUpstream := helpers.NewMockUpstreamWithBrokenJWKS()
			defer brokenUpstream.Close()

			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.OAuth2ConfigWithUpstream(brokenUpstream.URL())
			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		})

		// Edge case from specs/032-aggregated-jwks/spec.md — malformed upstream JWKS is treated as unavailable.
		It("proxy mode returns 503 when upstream JWKS is malformed JSON", func() {
			malformedUpstream := helpers.NewMockUpstreamWithMalformedJWKS()
			defer malformedUpstream.Close()

			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.OAuth2ConfigWithUpstream(malformedUpstream.URL())
			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		})

		// Scenario 3.4 from specs/032-aggregated-jwks/spec.md
		// Full-stack test: hybrid mode does not serve partial key set when upstream fails.
		It("hybrid mode returns 503 when upstream JWKS endpoint returns errors (no partial set)", func() {
			brokenUpstream := helpers.NewMockUpstreamWithBrokenJWKS()
			defer brokenUpstream.Close()

			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.HybridConfig(brokenUpstream.URL())
			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			adminSrv, err := bootstrap.NewAdminTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(adminSrv.Close)
			Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		})

		// Scenario 3.5 from specs/032-aggregated-jwks/spec.md
		It("local mode starts without upstream and JWKS is available", func() {
			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.LocalConfig()
			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			adminSrv, err := bootstrap.NewAdminTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(adminSrv.Close)
			Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	// ==================== Observability ====================

	Describe("health endpoint component status", func() {
		// OB-002 from specs/032-aggregated-jwks/spec.md
		It("reports upstream_jwks as degraded after hybrid upstream refresh failures", func() {
			mockUpstream := helpers.NewMockUpstreamOAuth2Server()

			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.HybridConfig(mockUpstream.URL())
			app, err := bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			adminSrv, err := bootstrap.NewAdminTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer adminSrv.Close()
			Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			health := fetchHealth(server.BaseURL())
			Expect(health.Status).To(Equal("healthy"))
			Expect(health.Components).To(HaveKeyWithValue("upstream_jwks", "healthy"))

			mockUpstream.Close()

			Eventually(func() map[string]string {
				return fetchHealth(server.BaseURL()).Components
			}, 8*time.Second, 200*time.Millisecond).Should(HaveKeyWithValue("upstream_jwks", "degraded"))
		})
	})

	// ==================== User Story 4: Duplicate Key ID Detection ====================

	Describe("kid conflict detection", func() {
		// Scenario 4.1 from specs/032-aggregated-jwks/spec.md
		// Full-stack test: provision a local signing key, then build a hybrid app with
		// a mock upstream serving the same kid. The real service detects the conflict.
		It("hybrid mode returns 503 when kid conflict is detected", func() {
			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			// Phase 1: Build local app to provision a signing key and learn its kid.
			localConfig := fixtures.LocalConfig()
			localApp, err := bootstrap.NewServerFactory(localConfig, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			adminSrv, err := bootstrap.NewAdminTestServer(localApp, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

			localSrv, err := bootstrap.NewEndUserTestServer(localApp, logger)
			Expect(err).ToNot(HaveOccurred())
			resp, err := http.Get(localSrv.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			var jwksBody map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&jwksBody)).ToNot(HaveOccurred())
			_ = resp.Body.Close()
			localKid := extractKids(jwksBody)[0]
			localSrv.Close()
			adminSrv.Close()

			// Phase 2: Create mock upstream with the SAME kid, then build hybrid app.
			conflictingUpstream := helpers.NewMockUpstreamWithKid(localKid)
			defer conflictingUpstream.Close()

			hybridConfig := fixtures.HybridConfig(conflictingUpstream.URL())
			hybridApp, err := bootstrap.NewServerFactory(hybridConfig, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(hybridApp, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err = http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		})

		// Scenario 4.2 from specs/032-aggregated-jwks/spec.md
		It("hybrid mode publishes all keys when no kid conflict exists", func() {
			mockUpstream := helpers.NewMockUpstreamOAuth2Server()
			defer mockUpstream.Close()

			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.HybridConfig(mockUpstream.URL())
			app, err := bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			adminSrv, err := bootstrap.NewAdminTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(adminSrv.Close)
			Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			jwks := fetchJWKSJSON(server.BaseURL() + "/oauth2/jwks.json")
			keys := jwks["keys"].([]any)
			Expect(len(keys)).To(BeNumerically(">=", 2))

			upstreamJWKS := fetchJWKSJSON(mockUpstream.URL() + "/.well-known/jwks.json")
			upstreamKey := extractKeyByKid(upstreamJWKS, "test-key")
			Expect(upstreamKey).ToNot(BeNil())

			publishedUpstreamKey := extractKeyByKid(jwks, "test-key")
			Expect(publishedUpstreamKey).ToNot(BeNil())

			for _, field := range []string{"kid", "alg", "kty", "use", "n", "e"} {
				Expect(publishedUpstreamKey[field]).To(Equal(upstreamKey[field]), "field %s should be preserved", field)
			}
		})

		// Scenario 4.3 from specs/032-aggregated-jwks/spec.md
		// Full-stack test: kid conflict detected at startup via builder's PublishJWKS check.
		It("returns 503 when kid conflict was detected at startup", func() {
			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			// Phase 1: Provision a signing key and learn its kid.
			localConfig := fixtures.LocalConfig()
			localApp, err := bootstrap.NewServerFactory(localConfig, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			adminSrv, err := bootstrap.NewAdminTestServer(localApp, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

			localSrv, err := bootstrap.NewEndUserTestServer(localApp, logger)
			Expect(err).ToNot(HaveOccurred())
			resp, err := http.Get(localSrv.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			var jwksBody map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&jwksBody)).ToNot(HaveOccurred())
			_ = resp.Body.Close()
			localKid := extractKids(jwksBody)[0]
			localSrv.Close()
			adminSrv.Close()

			// Phase 2: Build hybrid app with conflicting upstream.
			conflictingUpstream := helpers.NewMockUpstreamWithKid(localKid)
			defer conflictingUpstream.Close()

			hybridConfig := fixtures.HybridConfig(conflictingUpstream.URL())
			hybridApp, err := bootstrap.NewServerFactory(hybridConfig, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(hybridApp, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err = http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		})

		// Scenario 4.4 from specs/032-aggregated-jwks/spec.md
		It("returns 503 when runtime upstream refresh introduces kid conflict", func() {
			rotatingUpstream := helpers.NewRotatingJWKSUpstream("runtime-upstream-kid")
			defer rotatingUpstream.Close()

			testStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(testStorage) }()

			config := fixtures.HybridConfig(rotatingUpstream.URL())
			app, err := bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			adminSrv, err := bootstrap.NewAdminTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(adminSrv.Close)
			Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var jwksBody map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&jwksBody)).ToNot(HaveOccurred())
			_ = resp.Body.Close()

			kids := extractKids(jwksBody)
			Expect(kids).To(ContainElement("runtime-upstream-kid"))

			var localKid string
			for _, kid := range kids {
				if kid != "runtime-upstream-kid" {
					localKid = kid
					break
				}
			}
			Expect(localKid).ToNot(BeEmpty())

			rotatingUpstream.SetKID(localKid)

			Eventually(func() int {
				resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
				if err != nil {
					return 0
				}
				defer func() { _ = resp.Body.Close() }()
				return resp.StatusCode
			}, 8*time.Second, 200*time.Millisecond).Should(Equal(http.StatusServiceUnavailable))

			resp, err = http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		})
	})
})

type healthResponse struct {
	Status     string            `json:"status"`
	Components map[string]string `json:"components"`
}

func fetchHealth(baseURL string) healthResponse {
	resp, err := http.Get(baseURL + "/health")
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	var health healthResponse
	Expect(json.NewDecoder(resp.Body).Decode(&health)).ToNot(HaveOccurred())
	return health
}

func fetchBrokerJWKS(baseURL string) jwk.Set {
	resp, err := http.Get(baseURL + "/oauth2/jwks.json")
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	body, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())

	keySet, err := jwk.Parse(body)
	Expect(err).ToNot(HaveOccurred())
	return keySet
}

func fetchJWKSJSON(url string) map[string]any {
	resp, err := http.Get(url)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	var jwks map[string]any
	Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
	return jwks
}

func validateJWTWithJWKS(token string, keySet jwk.Set) error {
	_, err := jwt.Parse([]byte(token), jwt.WithKeySet(keySet, jws.WithInferAlgorithmFromKey(true)))
	return err
}

// extractKids returns all kid values from a JWKS map.
func extractKids(jwks map[string]interface{}) []string {
	keys, ok := jwks["keys"].([]interface{})
	if !ok {
		return nil
	}
	kids := make([]string, 0, len(keys))
	for _, k := range keys {
		if keyMap, ok := k.(map[string]interface{}); ok {
			if kid, ok := keyMap["kid"].(string); ok && kid != "" {
				kids = append(kids, kid)
			}
		}
	}
	return kids
}

func extractKeyByKid(jwks map[string]any, kid string) map[string]any {
	keys, ok := jwks["keys"].([]any)
	if !ok {
		return nil
	}
	for _, key := range keys {
		keyMap, ok := key.(map[string]any)
		if !ok {
			continue
		}
		if keyID, ok := keyMap["kid"].(string); ok && keyID == kid {
			return keyMap
		}
	}
	return nil
}

func seedCIMDClientAuthenticationKey(store *storageadapter.Adapter) string {
	kid := id.NewKeyID("cimd-client-authentication-jwks-test-key")
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	Expect(err).ToNot(HaveOccurred())
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})
	Expect(privateKeyPEM).ToNot(BeNil())

	encryptedPrivateKey, err := testutil.NewPanicTestEncryptionAdapter().Encrypt(
		context.Background(),
		privateKeyPEM,
		map[string]string{"kid": kid.String()},
	)
	Expect(err).ToNot(HaveOccurred())

	now := time.Now().UTC()
	Expect(store.SigningKeys().Create(context.Background(), &domstorage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 kid,
		KeyDomain:           domstorage.KeyDomainCIMDClientAuthentication,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: encryptedPrivateKey,
		ActivatesAt:         now,
		CreatedAt:           now,
	})).To(Succeed())

	return kid.String()
}
