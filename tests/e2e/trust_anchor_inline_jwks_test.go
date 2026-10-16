package e2e_test

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

const clientAssertionPath = "token_exchange.client_assertion"

var _ = Describe("Inline key sets for the client-assertion trust anchor", Label(trustAnchorLabel), func() {
	var env *trustAnchorEnv

	BeforeEach(func() {
		env = newTrustAnchorEnv()
	})

	AfterEach(func() {
		env.close()
	})

	Context("local mode with an inline key set and an issuer that publishes nothing", func() {
		BeforeEach(func() {
			env.start(env.localConfig(ports.ClientAssertionTrustConfig{
				IssuerURI: env.issuer.URL(),
				JWKS:      helpers.PublicKeySet(env.gateway),
			}))
		})

		// US1-S1 from specs/052-inline-jwks-trust-anchors/spec.md
		It("starts and verifies without any request to the issuer host", func() {
			Expect(env.issuer.Count()).To(BeZero())

			resp := env.exchange(env.mintLocalSubjectToken(), env.assertion(env.gateway, env.issuer.URL()))
			defer func() { _ = resp.Body.Close() }()

			Expect(env.issuer.Count()).To(BeZero())
		})

		// US1-S2 from specs/052-inline-jwks-trust-anchors/spec.md
		It("exchanges a subject token when the client assertion is signed by an inline key", func() {
			resp := env.exchange(env.mintLocalSubjectToken(), env.assertion(env.gateway, env.issuer.URL()))

			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			Expect(decodeResponseBody(resp)["access_token"]).To(Equal("github-token-xyz"))
		})

		// US1-S3 from specs/052-inline-jwks-trust-anchors/spec.md
		It("rejects a client assertion signed by a key outside the inline set", func() {
			rogue := helpers.NewLocalES256Key("rogue-2026-10")

			resp := env.exchange(env.mintLocalSubjectToken(), env.assertion(rogue, env.issuer.URL()))

			Expect(resp).To(matchers.HaveStatusCode(http.StatusUnauthorized))
			body := decodeResponseBody(resp)
			Expect(body["error"]).To(Equal("invalid_client"))
			Expect(body).ToNot(HaveKey("access_token"))
		})
	})

	Context("proxy mode with machine-facing approval authentication", func() {
		BeforeEach(func() {
			env.start(env.proxyConfig(ports.ClientAssertionTrustConfig{
				JWKS: helpers.PublicKeySet(env.gateway),
			}))
		})

		// US1-S4 from specs/052-inline-jwks-trust-anchors/spec.md
		It("authenticates the gateway on POST /api/approvals with an inline-key X-Client-Assertion", func() {
			headers := helpers.ApprovalCreateHeaders(
				env.upstreamSubjectToken(), env.assertion(env.gateway, env.upstream.URL()), env.agent.ID)

			resp, err := postJSONWithHeaders(env.enduserServer, "/api/approvals", headers, helpers.CreateApprovalRequest{
				Metadata:  helpers.CreateApprovalMetadata{Description: "Run tests"},
				ToolName:  "run_tests",
				Arguments: map[string]any{"suite": "unit"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusCreated), Equal(http.StatusOK)))
		})
	})

	Context("hybrid mode with an inline key set and no client-assertion issuer", func() {
		BeforeEach(func() {
			env.start(env.hybridConfig(ports.ClientAssertionTrustConfig{
				JWKS: helpers.PublicKeySet(env.gateway),
			}))
		})

		// US1-S5 from specs/052-inline-jwks-trust-anchors/spec.md
		It("expects the upstream issuer, trusts only the inline keys for assertions, and keeps upstream keys for subject tokens", func() {
			subject := env.upstreamSubjectToken()

			accepted := env.exchange(subject, env.assertion(env.gateway, env.upstream.URL()))
			Expect(accepted).To(matchers.HaveStatusCode(http.StatusOK))
			_ = decodeResponseBody(accepted)

			upstreamSigned, err := helpers.SignTestJWT(map[string]any{
				"sub": gatewaySubject,
				"iss": env.upstream.URL(),
				"aud": brokerAudience,
				"exp": time.Now().Add(time.Hour).Unix(),
			}, env.upstream.GetPrivateKeyPEM())
			Expect(err).ToNot(HaveOccurred())
			rejected := env.exchange(subject, upstreamSigned)
			Expect(rejected).To(matchers.HaveStatusCode(http.StatusUnauthorized))
			_ = decodeResponseBody(rejected)

			gatewaySigned := env.exchange(env.assertion(env.gateway, env.upstream.URL(), func(c map[string]any) {
				c["azp"] = env.agent.ID.String()
				c["sub"] = env.principal
			}), env.assertion(env.gateway, env.upstream.URL()))
			Expect(gatewaySigned.StatusCode).ToNot(Equal(http.StatusOK))
			Expect(decodeResponseBody(gatewaySigned)).ToNot(HaveKey("access_token"))
		})
	})
})

var _ = Describe("Invalid or ambiguous key configuration for the client-assertion trust anchor", Label(trustAnchorLabel), func() {
	var env *trustAnchorEnv

	BeforeEach(func() {
		env = newTrustAnchorEnv()
	})

	AfterEach(func() {
		env.close()
	})

	inlineConfig := func(document string) *ports.Config {
		return env.localConfig(ports.ClientAssertionTrustConfig{IssuerURI: env.issuer.URL(), JWKS: document})
	}

	Context("startup validation", func() {
		// US3-S1 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails when two key sources are set and names the trust anchor and both settings", func() {
			config := env.localConfig(ports.ClientAssertionTrustConfig{
				IssuerURI: env.issuer.URL(),
				JWKSURI:   env.issuer.URL() + "/keys",
				JWKS:      helpers.PublicKeySet(env.gateway),
			})

			env.expectStartupFailure(config,
				clientAssertionPath+": only one of jwks_uri, jwks, jwks_file may be set (found: jwks_uri, jwks)")
			Expect(env.issuer.Count()).To(BeZero())
		})

		// US3-S2 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails for a bare JWK, an empty key set, and an oversized key set", func() {
			env.expectStartupFailure(inlineConfig(helpers.BareJWK(env.gateway)),
				clientAssertionPath+".jwks: key set is not a valid JWK Set document")
			env.expectStartupFailure(inlineConfig(helpers.EmptyKeySet()),
				clientAssertionPath+".jwks: key set contains no keys")
			env.expectStartupFailure(inlineConfig(helpers.OversizedKeySet(env.gateway)),
				clientAssertionPath+".jwks: key set exceeds 1048576 bytes")
		})

		// US3-S3 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails for private key material without echoing any key member", func() {
			rsaKey := helpers.NewLocalRS256Key("rsa-private")

			ecErr := env.expectStartupFailure(inlineConfig(helpers.PrivateKeySet(env.gateway)),
				clientAssertionPath+".jwks: key set contains private key material; only public keys are allowed")
			rsaErr := env.expectStartupFailure(inlineConfig(helpers.PrivateKeySet(rsaKey)),
				clientAssertionPath+".jwks: key set contains private key material; only public keys are allowed")

			env.expectNoKeyMaterial(ecErr, env.gateway, rsaKey)
			env.expectNoKeyMaterial(rsaErr, env.gateway, rsaKey)
		})

		// US3-S4 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails for a symmetric key and for an algorithm outside the approved asymmetric set", func() {
			env.expectStartupFailure(inlineConfig(helpers.SymmetricKeySet("shared-1")),
				clientAssertionPath+".jwks: key set contains a symmetric key; only public asymmetric keys are allowed")
			env.expectStartupFailure(inlineConfig(helpers.UnapprovedAlgorithmKeySet(env.gateway, "HS256")),
				clientAssertionPath+".jwks: key \""+gatewayKID+"\" declares an algorithm outside the approved asymmetric set")
		})

		// US3-S5 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails for a duplicate key identifier and names it", func() {
			other := helpers.NewLocalES256Key("other")

			env.expectStartupFailure(inlineConfig(helpers.DuplicateKIDKeySet(env.gateway, other, "dup-1")),
				clientAssertionPath+".jwks: key identifier \"dup-1\" appears more than once")
		})

		// US3-S6 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails when an inline key set is combined with explicit refresh bounds", func() {
			config := env.localConfig(ports.ClientAssertionTrustConfig{
				IssuerURI:      env.issuer.URL(),
				JWKS:           helpers.PublicKeySet(env.gateway),
				JWKSMinRefresh: time.Minute,
			})

			env.expectStartupFailure(config,
				clientAssertionPath,
				"jwks_min_refresh and jwks_max_refresh are not allowed with jwks; an inline key set never refreshes")
		})

		// US3-S7 from specs/052-inline-jwks-trust-anchors/spec.md
		It("still rejects the broker's own issuer with an inline source and with a file source", func() {
			const ownIssuerMessage = "must be an external identity provider, not the broker's own issuer"

			env.expectStartupFailure(
				env.localConfig(ports.ClientAssertionTrustConfig{
					IssuerURI: brokerOwnIssuer,
					JWKS:      helpers.PublicKeySet(env.gateway),
				}), ownIssuerMessage)

			path := env.trust.WriteFile("gateway-jwks.json", helpers.PublicKeySet(env.gateway))
			env.expectStartupFailure(
				env.localConfig(ports.ClientAssertionTrustConfig{
					IssuerURI: brokerOwnIssuer,
					JWKSFile:  path,
				}), ownIssuerMessage)
		})
	})

	Context("diagnostics for a valid inline key set", func() {
		// US3-S8 from specs/052-inline-jwks-trust-anchors/spec.md
		It("logs the source kind, key count, and key identifiers without key material", func() {
			set := helpers.PublicKeySet(env.gateway)
			Expect(env.build(inlineConfig(set))).To(Succeed())

			records := env.logRecords("trust anchor key set loaded")
			Expect(records).To(HaveLen(1))
			Expect(records[0]).To(HaveKeyWithValue("trust_anchor", clientAssertionPath))
			Expect(records[0]).To(HaveKeyWithValue("key_source", "inline"))
			Expect(records[0]).To(HaveKeyWithValue("key_count", BeNumerically("==", 1)))
			Expect(records[0]).To(HaveKeyWithValue("key_ids", ConsistOf(gatewayKID)))
			env.expectNoKeyMaterial(nil, env.gateway)
			Expect(env.logs.Raw()).ToNot(ContainSubstring(set))
		})
	})
})
