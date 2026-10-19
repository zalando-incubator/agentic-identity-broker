package e2e_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	appcore "github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("Local key sets for impersonation trusted issuers", Label(trustAnchorLabel), func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		application    *appcore.App
		enduserServer  *bootstrap.TestServer
		adminServer    *bootstrap.TestServer
		issuer         *helpers.CountingServer
		gateway        *helpers.LocalKey
		trust          *helpers.TrustDir
		targetAudience string
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
		issuer = helpers.NewCountingServer()
		gateway = helpers.NewLocalES256Key(gatewayKID)
		trust = helpers.NewTrustDir(GinkgoT().TempDir())

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		target := fixtures.AgentWithClientID("impersonation-target")
		canonicalID := "impersonation-target"
		target.CanonicalID = &canonicalID
		target.AllowedScopes = []string{"read"}
		targetAudience = imperAudience + "/" + target.ID.String()
		Expect(testStorage.Agents().Create(context.Background(), target)).To(Succeed())
		Expect(testStorage.UserGrants().Create(context.Background(), fixtures.ActiveGrant(
			"user-1", target.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"},
		))).To(Succeed())
	})

	AfterEach(func() {
		for _, server := range []*bootstrap.TestServer{enduserServer, adminServer} {
			if server != nil {
				server.CloseListener()
			}
		}
		var shutdownErr error
		if application != nil && application.Shutdown != nil {
			shutdownErr = application.Shutdown(context.Background())
		}
		if issuer != nil {
			issuer.Close()
		}
		if testStorage != nil && storageFactory != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
		application, enduserServer, adminServer, issuer, testStorage = nil, nil, nil, nil, nil
		Expect(shutdownErr).To(Succeed())
	})

	localRule := func(name, authz string, issuerConfig func(*ports.TrustedTokenIssuerConfig)) ports.ImpersonationRuleConfig {
		rule := imperSignedRule(name, issuer.URL(), "", authz)
		rule.TrustedIssuers[0].JWKSMinRefresh = 0
		rule.TrustedIssuers[0].JWKSMaxRefresh = 0
		issuerConfig(&rule.TrustedIssuers[0])
		return rule
	}

	boot := func(rules ...ports.ImpersonationRuleConfig) {
		config := fixtures.LocalConfig()
		config.Security.SkipThirdpartyHTTPSValidation = true
		config.Security.TrustMaterialDir = trust.Path
		config.OAuth2AuthServer.Local.TokenClaimsExpression = `{"aud": "https://policy.example.com"}`
		config.OAuth2AuthServer.Impersonation = &ports.ImpersonationConfig{AudiencePrefix: imperAudience, Rules: rules}

		var err error
		application, err = bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).To(Succeed())
	}

	credential := func(key *helpers.LocalKey, sub string) string {
		now := time.Now()
		token, err := key.Sign(map[string]any{
			"iss": issuer.URL(),
			"sub": sub,
			"aud": imperAudience,
			"iat": now.Unix(),
			"exp": now.Add(time.Hour).Unix(),
		})
		Expect(err).ToNot(HaveOccurred())
		return token
	}

	impersonate := func(key *helpers.LocalKey, clientSub string) *http.Response {
		resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(url.Values{
			"grant_type":            {imperGrant},
			"audience":              {targetAudience},
			"client_assertion_type": {imperBearerTyp},
			"client_assertion":      {credential(key, clientSub)},
			"actor_token_type":      {imperJWTType},
			"actor_token":           {credential(key, "actor-1")},
			"subject_token_type":    {imperJWTType},
			"subject_token":         {credential(key, "user-1")},
		}.Encode()))
		Expect(err).ToNot(HaveOccurred())
		return resp
	}

	Context("a trusted issuer with an inline key set", func() {
		// US4-S1 from specs/052-inline-jwks-trust-anchors/spec.md
		It("starts without discovery and mints a token whose act claim identifies the gateway's actor", func() {
			boot(localRule("gateway", `client_assertion.sub == "gateway-prod"`, func(c *ports.TrustedTokenIssuerConfig) {
				c.JWKS = helpers.PublicKeySet(gateway)
			}))
			Expect(issuer.Count()).To(BeZero())

			resp := impersonate(gateway, "gateway-prod")

			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			claims := decodeJWTClaims(decodeResponseBody(resp)["access_token"].(string))
			Expect(claims["sub"]).To(Equal("user-1"))
			act, ok := claims["act"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(act["iss"]).To(Equal(issuer.URL()))
			Expect(act["sub"]).To(Equal("actor-1"))
			Expect(issuer.Count()).To(BeZero())
		})
	})

	Context("a trusted issuer with a key-set file", func() {
		// US4-S2 from specs/052-inline-jwks-trust-anchors/spec.md
		It("validates credentials signed by the file's keys", func() {
			path := trust.WriteFile("gateway-jwks.json", helpers.PublicKeySet(gateway))
			boot(localRule("gateway", `client_assertion.sub == "gateway-prod"`, func(c *ports.TrustedTokenIssuerConfig) {
				c.JWKSFile = path
			}))

			resp := impersonate(gateway, "gateway-prod")

			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			_ = decodeResponseBody(resp)
			Expect(issuer.Count()).To(BeZero())
		})
	})

	Context("a trusted issuer that permits only ES256", func() {
		// US4-S3 from specs/052-inline-jwks-trust-anchors/spec.md
		It("rejects a credential signed with RS256 by a key in its local set", func() {
			rsaKey := helpers.NewLocalRS256Key("agentgateway-rsa")
			boot(localRule("gateway", `client_assertion.sub == "gateway-prod"`, func(c *ports.TrustedTokenIssuerConfig) {
				c.JWKS = helpers.PublicKeySet(gateway, rsaKey)
				c.AllowedAlgorithms = []string{"ES256"}
			}))

			rejected := impersonate(rsaKey, "gateway-prod")
			Expect(rejected.StatusCode).ToNot(Equal(http.StatusOK))
			Expect(decodeResponseBody(rejected)).ToNot(HaveKey("access_token"))

			accepted := impersonate(gateway, "gateway-prod")
			Expect(accepted).To(matchers.HaveStatusCode(http.StatusOK))
			_ = decodeResponseBody(accepted)
		})
	})

	Context("two rules that trust the same issuer identifier with different inline keys", func() {
		// US4-S4 from specs/052-inline-jwks-trust-anchors/spec.md
		It("verifies each rule only against its own keys", func() {
			keyA := helpers.NewLocalES256Key("rule-a")
			keyB := helpers.NewLocalES256Key("rule-b")
			boot(
				localRule("rule-a", `client_assertion.sub == "gateway-a"`, func(c *ports.TrustedTokenIssuerConfig) {
					c.JWKS = helpers.PublicKeySet(keyA)
				}),
				localRule("rule-b", `client_assertion.sub == "gateway-b"`, func(c *ports.TrustedTokenIssuerConfig) {
					c.JWKS = helpers.PublicKeySet(keyB)
				}),
			)

			ownKeys := impersonate(keyB, "gateway-b")
			Expect(ownKeys).To(matchers.HaveStatusCode(http.StatusOK))
			_ = decodeResponseBody(ownKeys)

			// Rule A would authorize gateway-a, but a credential signed with rule B's key never verifies in rule A.
			crossed := impersonate(keyB, "gateway-a")
			Expect(crossed.StatusCode).ToNot(Equal(http.StatusOK))
			Expect(decodeResponseBody(crossed)).ToNot(HaveKey("access_token"))
		})
	})
})
