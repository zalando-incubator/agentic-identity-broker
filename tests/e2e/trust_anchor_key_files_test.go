package e2e_test

import (
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

const (
	gatewayKeyFile = "agentgateway-jwks.json"
	fileAnchorPath = clientAssertionPath + ".jwks_file"
)

var _ = Describe("File key sets for the client-assertion trust anchor", Label(trustAnchorLabel), func() {
	var env *trustAnchorEnv

	BeforeEach(func() {
		env = newTrustAnchorEnv()
	})

	AfterEach(func() {
		env.close()
	})

	fileConfig := func(path string) *ports.Config {
		return env.hybridConfig(ports.ClientAssertionTrustConfig{
			IssuerURI:      env.issuer.URL(),
			JWKSFile:       path,
			JWKSMinRefresh: fastFileMinRefresh,
			JWKSMaxRefresh: fastFileMaxRefresh,
		})
	}

	exchangeStatus := func(key *helpers.LocalKey) int {
		resp := env.exchange(env.upstreamSubjectToken(), env.assertion(key, env.issuer.URL()))
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	outsideMessage := func(path string) string {
		return fmt.Sprintf("%s: jwks_file %q is outside security.trust_material_dir %q", clientAssertionPath, path, env.trust.Path)
	}

	Context("a key-set file inside the trust directory", func() {
		// US2-S1 from specs/052-inline-jwks-trust-anchors/spec.md
		It("starts and verifies client assertions with the file's keys", func() {
			path := env.trust.WriteFile(gatewayKeyFile, helpers.PublicKeySet(env.gateway))
			env.start(fileConfig(path))

			Expect(exchangeStatus(env.gateway)).To(Equal(http.StatusOK))
			Expect(exchangeStatus(helpers.NewLocalES256Key("rogue-2026-10"))).To(Equal(http.StatusUnauthorized))
		})

		// US2-S2 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails startup for a missing path and for a directory path, naming the setting and the path", func() {
			missing := env.trust.File("missing.json")
			env.expectStartupFailure(fileConfig(missing),
				fmt.Sprintf("%s: jwks_file %q does not exist", clientAssertionPath, missing))

			directory := env.trust.File("a-directory")
			Expect(os.Mkdir(directory, 0o755)).To(Succeed())
			env.expectStartupFailure(fileConfig(directory),
				fmt.Sprintf("%s: jwks_file %q is not a regular file", clientAssertionPath, directory))
		})

		// US2-S9 from specs/052-inline-jwks-trust-anchors/spec.md
		It("starts with the Kubernetes ..data relative-link layout", func() {
			path := env.trust.WriteKubernetesLayout(gatewayKeyFile, helpers.PublicKeySet(env.gateway), "2026_10_16_08_50_00.123")
			env.start(fileConfig(path))

			Expect(exchangeStatus(env.gateway)).To(Equal(http.StatusOK))
		})
	})

	Context("a running broker with a file-based key set", func() {
		var path string

		BeforeEach(func() {
			path = env.trust.WriteKubernetesLayout(gatewayKeyFile, helpers.PublicKeySet(env.gateway), "gen1")
			env.start(fileConfig(path))
		})

		// US2-S3 from specs/052-inline-jwks-trust-anchors/spec.md
		It("applies a replaced file within the refresh bounds and rejects keys the replacement removed", func() {
			replacement := helpers.NewLocalES256Key("agentgateway-2027-01")
			Expect(exchangeStatus(env.gateway)).To(Equal(http.StatusOK))

			env.trust.SwapKubernetesLayout(gatewayKeyFile, helpers.PublicKeySet(replacement), "gen2")

			Eventually(func() int { return exchangeStatus(replacement) }, 3*time.Second, 100*time.Millisecond).
				Should(Equal(http.StatusOK))
			Eventually(func() int { return exchangeStatus(env.gateway) }, 3*time.Second, 100*time.Millisecond).
				Should(Equal(http.StatusUnauthorized))
			Expect(env.logRecords("trust anchor key set changed")).ToNot(BeEmpty())
		})

		// US2-S4 from specs/052-inline-jwks-trust-anchors/spec.md
		It("keeps verifying after a corrupt re-read, reports degraded health without logging contents, and recovers", func() {
			const marker = "FILE-CONTENT-MARKER-7f3a9c"
			env.trust.SwapKubernetesLayout(gatewayKeyFile, `{"keys":["`+marker, "gen2")

			Eventually(func() any { return env.healthComponents()["trust_anchor_key_files"] }, 3*time.Second, 50*time.Millisecond).
				Should(Equal("degraded"))
			Expect(exchangeStatus(env.gateway)).To(Equal(http.StatusOK))

			failures := env.logRecords("trust anchor key set re-read failed")
			Expect(failures).ToNot(BeEmpty())
			Expect(failures[0]).To(HaveKeyWithValue("trust_anchor", clientAssertionPath))
			Expect(failures[0]).To(HaveKeyWithValue("key_source", "file"))
			Expect(failures[0]).To(HaveKeyWithValue("error_kind", "not_a_jwk_set"))
			Expect(failures[0]).To(HaveKey("last_success_at"))
			Expect(failures[0]).To(HaveKey("fails_closed_at"))
			Expect(env.logs.Raw()).ToNot(ContainSubstring(marker))

			env.trust.SwapKubernetesLayout(gatewayKeyFile, helpers.PublicKeySet(env.gateway), "gen3")

			Eventually(func() any { return env.healthComponents()["trust_anchor_key_files"] }, 3*time.Second, 50*time.Millisecond).
				Should(Equal("healthy"))
			Expect(env.logRecords("trust anchor key set recovered")).ToNot(BeEmpty())
		})
	})

	Context("a key-set file from a rendered Helm chart", func() {
		// US2-S5 from specs/052-inline-jwks-trust-anchors/spec.md
		It("boots from the rendered configuration with the Secret projected into the trust directory", func() {
			rendered, err := helpers.RenderChart(fmt.Sprintf(`
broker:
  oauth2AuthorizationServer:
    local:
      tokenClaimsExpression: '{"azp": agent.id, "aud": "token-exchange-broker"}'
  security:
    skipThirdpartyHttpsValidation: true
    trustMaterial:
      directory: %[1]s
      sources:
        - secret:
            name: agentgateway-jwks
            items:
              - key: jwks.json
                path: %[2]s
  tokenExchange:
    clientAssertion:
      issuerUri: %[3]s
      jwksFile: %[1]s/%[2]s
    claimExtraction:
      agentIdExpression: "subject_token.azp"
`, env.trust.Path, gatewayKeyFile, env.issuer.URL()))
			Expect(err).ToNot(HaveOccurred())
			Expect(rendered.TrustMount).To(HaveKeyWithValue("mountPath", env.trust.Path))
			Expect(rendered.TrustMount).To(HaveKeyWithValue("readOnly", true))
			Expect(rendered.TrustMount).ToNot(HaveKey("subPath"))

			env.trust.WriteKubernetesLayout(gatewayKeyFile, helpers.PublicKeySet(env.gateway), "2026_10_16_08_50_00.123")

			configFile := env.trust.WriteOutsideFile("config.yaml", rendered.ConfigYAML)
			GinkgoT().Setenv("IDENTITY_BROKER_CONFIG_PATH", configFile)
			GinkgoT().Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
			GinkgoT().Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", "YWJjZGVmMDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODk=")
			GinkgoT().Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			GinkgoT().Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			loaded, err := config.NewLoader().GetConfig(GinkgoT().Context())
			Expect(err).ToNot(HaveOccurred())

			env.start(loaded)
			resp := env.exchange(env.mintLocalSubjectToken(), env.assertion(env.gateway, env.issuer.URL()))
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			_ = decodeResponseBody(resp)
		})
	})

	Context("key-set paths outside the trust directory", func() {
		// US2-S6 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails startup without opening a file outside the trust directory", func() {
			fifo := env.trust.File("../outside/config.fifo")
			Expect(syscall.Mkfifo(fifo, 0o600)).To(Succeed())
			DeferCleanup(func() {
				if writer, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
					_ = writer.Close()
				}
			})

			built := make(chan error, 1)
			go func() { built <- env.build(fileConfig(fifo)) }()

			var err error
			Eventually(built, 2*time.Second).Should(Receive(&err))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(outsideMessage(fifo)))
		})

		// US2-S7 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails startup for a .. escape, a relative-link escape, and an absolute link into the trust directory", func() {
			env.trust.WriteOutsideFile("secret.json", helpers.PublicKeySet(env.gateway))

			dotDot := env.trust.DotDotPath("secret.json")
			env.expectStartupFailure(fileConfig(dotDot), fileAnchorPath+": jwks_file", "is outside security.trust_material_dir")

			relative := env.trust.LinkRelativeEscape("relative.json", "secret.json")
			env.expectStartupFailure(fileConfig(relative), outsideMessage(relative))

			real := env.trust.WriteFile("real.json", helpers.PublicKeySet(env.gateway))
			absolute := env.trust.LinkAbsolute("absolute.json", real)
			env.expectStartupFailure(fileConfig(absolute), outsideMessage(absolute))
		})

		// US2-S8 from specs/052-inline-jwks-trust-anchors/spec.md
		It("fails startup for jwks_file when no trust directory is configured", func() {
			path := env.trust.WriteFile(gatewayKeyFile, helpers.PublicKeySet(env.gateway))
			cfg := fileConfig(path)
			cfg.Security.TrustMaterialDir = ""

			env.expectStartupFailure(cfg, fileAnchorPath+": jwks_file requires security.trust_material_dir to be set")
		})
	})
})
