package e2e_test

import (
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

type keySourceOutcome struct {
	Status int
	Error  string
}

// SC-003 from specs/052-inline-jwks-trust-anchors/spec.md: remote, inline, and file key sets
// must give identical outcomes for the same credential corpus.
var _ = Describe("Key source parity for client assertions", Label(trustAnchorParityLabel), func() {
	outcomeFor := func(source string, makeCredential func(env *trustAnchorEnv) string) keySourceOutcome {
		env := newTrustAnchorEnv()
		defer env.close()

		set := helpers.PublicKeySet(env.gateway)
		anchor := ports.ClientAssertionTrustConfig{IssuerURI: env.issuer.URL()}
		switch source {
		case "remote":
			keys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(set))
			}))
			defer keys.Close()
			anchor.JWKSURI = keys.URL + "/jwks"
		case "inline":
			anchor.JWKS = set
		case "file":
			anchor.JWKSFile = env.trust.WriteFile("gateway-jwks.json", set)
		}

		config := env.hybridConfig(anchor)
		config.Security.SkipThirdpartyHTTPSValidation = true
		env.start(config)

		resp := env.exchange(env.upstreamSubjectToken(), makeCredential(env))
		outcome := keySourceOutcome{Status: resp.StatusCode}
		if body := decodeResponseBody(resp); body["error"] != nil {
			outcome.Error, _ = body["error"].(string)
		}
		return outcome
	}

	signed := func(mutate func(map[string]any)) func(*trustAnchorEnv) string {
		return func(env *trustAnchorEnv) string {
			return env.assertion(env.gateway, env.issuer.URL(), mutate)
		}
	}

	DescribeTable("gives the same accept or reject outcome for every key source",
		func(makeCredential func(*trustAnchorEnv) string, wantStatus int) {
			remote := outcomeFor("remote", makeCredential)
			inline := outcomeFor("inline", makeCredential)
			file := outcomeFor("file", makeCredential)

			Expect(remote.Status).To(Equal(wantStatus))
			Expect(inline).To(Equal(remote), "inline outcome differs from remote")
			Expect(file).To(Equal(remote), "file outcome differs from remote")
		},
		Entry("a valid credential", signed(func(map[string]any) {}), http.StatusOK),
		Entry("a wrongly signed credential", func(env *trustAnchorEnv) string {
			return env.assertion(helpers.NewLocalES256Key(gatewayKID), env.issuer.URL())
		}, http.StatusUnauthorized),
		Entry("an expired credential", signed(func(c map[string]any) {
			c["iat"] = time.Now().Add(-2 * time.Hour).Unix()
			c["exp"] = time.Now().Add(-time.Hour).Unix()
		}), http.StatusUnauthorized),
		Entry("a credential for the wrong audience", signed(func(c map[string]any) {
			c["aud"] = "https://other.example.com"
		}), http.StatusUnauthorized),
		Entry("a credential from the wrong issuer", signed(func(c map[string]any) {
			c["iss"] = "https://other.example.com"
		}), http.StatusUnauthorized),
	)
})
