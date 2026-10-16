package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const jwtAnchorPath = "server.enduser.authentication.jwt"

var _ = Describe("Local key sets for JWT pre-authentication", Label(trustAnchorLabel), func() {
	var (
		env  *trustAnchorEnv
		mesh *helpers.LocalKey
	)

	BeforeEach(func() {
		env = newTrustAnchorEnv()
		mesh = helpers.NewLocalES256Key("mesh-2026-10")
	})

	AfterEach(func() {
		env.close()
	})

	signedConfig := func(configure func(*ports.JWTConfig)) *ports.Config {
		config := fixtures.SignedJWTConfig("")
		config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = env.upstream.URL()
		config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = env.upstream.URL() + "/oauth/authorize"
		config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = env.upstream.URL() + "/oauth/token"
		config.Security.TrustMaterialDir = env.trust.Path
		configure(config.Server.EndUser.Authentication.JWT)
		return config
	}

	startEnduser := func(config *ports.Config) {
		Expect(env.build(config)).To(Succeed())
		var err error
		env.enduserServer, err = bootstrap.NewEndUserTestServer(env.app, env.logger)
		Expect(err).ToNot(HaveOccurred())
	}

	getMe := func(key *helpers.LocalKey) *http.Response {
		token, err := key.Sign(helpers.NewJWTClaims().WithSubject(env.principal).Build())
		Expect(err).ToNot(HaveOccurred())
		resp, err := env.enduserServer.DirectRequest(http.MethodGet, "/api/me", "", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil)
		Expect(err).ToNot(HaveOccurred())
		return resp
	}

	expectPrincipalDerived := func() {
		resp := getMe(mesh)
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		var me jwtPreauthAPIResponse
		Expect(json.Unmarshal(body, &me)).To(Succeed())
		Expect(me.Data.Principal).To(Equal(env.principal))

		rejected := getMe(helpers.NewLocalES256Key("unknown-2026-10"))
		defer func() { _ = rejected.Body.Close() }()
		Expect(rejected.StatusCode).To(Equal(http.StatusUnauthorized))
	}

	stopEnduser := func() {
		env.enduserServer.Close()
		Expect(env.app.Shutdown(context.Background())).To(Succeed())
		env.enduserServer, env.app = nil, nil
	}

	Context("an inline key set or a key-set file", func() {
		// US5-S1 from specs/052-inline-jwks-trust-anchors/spec.md
		It("derives the principal for GET /api/me with an inline source and, separately, with a file source", func() {
			By("verifying with an inline key set")
			startEnduser(signedConfig(func(jwt *ports.JWTConfig) {
				jwt.JWKS = helpers.PublicKeySet(mesh)
			}))
			expectPrincipalDerived()
			stopEnduser()

			By("verifying with a key-set file")
			path := env.trust.WriteFile("mesh-jwks.json", helpers.PublicKeySet(mesh))
			startEnduser(signedConfig(func(jwt *ports.JWTConfig) {
				jwt.JWKSFile = path
			}))
			expectPrincipalDerived()
		})
	})

	Context("separate key sets on the admin and end-user servers", func() {
		// US5-S4 from specs/052-inline-jwks-trust-anchors/spec.md
		It("derives the admin principal only from a JWT signed by the admin server's own key set", func() {
			operator := helpers.NewLocalES256Key("admin-proxy-2026-10")
			path := env.trust.WriteFile("mesh-jwks.json", helpers.PublicKeySet(mesh))
			config := signedConfig(func(jwt *ports.JWTConfig) {
				jwt.JWKSFile = path
			})
			admin := *config.Server.EndUser.Authentication.JWT
			admin.JWKSFile = ""
			admin.JWKS = helpers.PublicKeySet(operator)
			config.Server.Admin.Authentication.JWT = &admin
			Expect(env.build(config)).To(Succeed())
			var err error
			env.adminServer, err = bootstrap.NewAdminTestServer(env.app, env.logger)
			Expect(err).ToNot(HaveOccurred())

			sweep := func(key *helpers.LocalKey) int {
				token, err := key.Sign(helpers.NewJWTClaims().WithSubject("operator-alice").Build())
				Expect(err).ToNot(HaveOccurred())
				resp, err := env.adminServer.DirectRequest(http.MethodPost, "/api/sessions/sweep", "", map[string]string{
					"Authorization": "Bearer " + token,
					"Content-Type":  "application/json",
				}, strings.NewReader(`{"dry_run":true}`))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()
				return resp.StatusCode
			}

			Expect(sweep(operator)).To(Equal(http.StatusOK))
			Expect(sweep(mesh)).ToNot(Equal(http.StatusOK))
			Expect(env.logs.Raw()).To(ContainSubstring(`"trust_anchor":"server.admin.authentication.jwt"`))
		})
	})

	Context("signature verification disabled or without a key source", func() {
		// US5-S2 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails startup when verification none is combined with jwks or with jwks_file", func() {
			unsigned := func(configure func(*ports.JWTConfig)) *ports.Config {
				config := fixtures.UnsignedJWTConfig()
				config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = env.upstream.URL()
				config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = env.upstream.URL() + "/oauth/authorize"
				config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = env.upstream.URL() + "/oauth/token"
				config.Security.TrustMaterialDir = env.trust.Path
				configure(config.Server.EndUser.Authentication.JWT)
				return config
			}

			env.expectStartupFailure(unsigned(func(jwt *ports.JWTConfig) {
				jwt.JWKS = helpers.PublicKeySet(mesh)
			}), jwtAnchorPath+": verification 'none' and jwks are mutually exclusive")

			path := env.trust.WriteFile("mesh-jwks.json", helpers.PublicKeySet(mesh))
			env.expectStartupFailure(unsigned(func(jwt *ports.JWTConfig) {
				jwt.JWKSFile = path
			}), jwtAnchorPath+": verification 'none' and jwks_file are mutually exclusive")
		})

		// US5-S3 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails startup when verification jwks has no key source, as it does today", func() {
			env.expectStartupFailure(signedConfig(func(*ports.JWTConfig) {}),
				jwtAnchorPath+": failed to create JWT authenticator")
		})
	})
})
