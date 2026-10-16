package e2e_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("CIMD private_key_jwt key retrieval", func() {
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

	// Scenario US4-S1 from specs/051-private-key-jwt-auth/spec.md
	It("refetches an unknown kid and accepts the rotated key", func() {
		fixture.Close()
		var err error
		fixture, err = newCIMDPrivateKeyJWTFixtureWithKeySetSettings(false, time.Nanosecond, 0, 0)
		Expect(err).To(Succeed())
		Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
		code, verifier, err := fixture.AuthorizeCode("read offline_access")
		Expect(err).To(Succeed())
		warmKey, err := helpers.GenerateClientAssertionKey(jwa.RS256(), "warm-unknown-key")
		Expect(err).To(Succeed())
		warmAssertion, warmType, err := fixture.SignAssertionFor(fixture.clientID, warmKey, helpers.ClientAssertionOptions{JTI: "warm-key-cache"})
		Expect(err).To(Succeed())
		warmForm := fixture.CodeForm(code, verifier)
		warmForm.Set("client_assertion_type", warmType)
		warmForm.Set("client_assertion", warmAssertion)
		warmResponse, err := fixture.PostToken(warmForm, "")
		Expect(err).To(Succeed())
		Expect(warmResponse.StatusCode).To(Equal(http.StatusUnauthorized))
		_ = warmResponse.Body.Close()

		rotatedKey, err := helpers.GenerateClientAssertionKey(jwa.RS256(), "client-key-2")
		Expect(err).To(Succeed())
		rotatedJWKS, err := rotatedKey.JWKSJSON()
		Expect(err).To(Succeed())
		fixture.SetJWKS(rotatedJWKS)
		rotatedAssertion, rotatedType, err := fixture.SignAssertionFor(fixture.clientID, rotatedKey, helpers.ClientAssertionOptions{JTI: "rotated-key-assertion"})
		Expect(err).To(Succeed())
		form := fixture.CodeForm(code, verifier)
		form.Set("client_assertion_type", rotatedType)
		form.Set("client_assertion", rotatedAssertion)
		response, err := fixture.PostToken(form, "")
		Expect(err).To(Succeed())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
	})

	// Scenario US4-S2 from specs/051-private-key-jwt-auth/spec.md
	It("rejects a removed key after refreshing the key set", func() {
		fixture.Close()
		var err error
		fixture, err = newCIMDPrivateKeyJWTFixtureWithKeySetSettings(false, time.Nanosecond, 0, 0)
		Expect(err).To(Succeed())
		Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
		code, verifier, err := fixture.AuthorizeCode("read offline_access")
		Expect(err).To(Succeed())
		warmKey, err := helpers.GenerateClientAssertionKey(jwa.RS256(), "warm-unknown-key")
		Expect(err).To(Succeed())
		warmAssertion, warmType, err := fixture.SignAssertionFor(fixture.clientID, warmKey, helpers.ClientAssertionOptions{JTI: "warm-key-cache"})
		Expect(err).To(Succeed())
		warmForm := fixture.CodeForm(code, verifier)
		warmForm.Set("client_assertion_type", warmType)
		warmForm.Set("client_assertion", warmAssertion)
		warm, err := fixture.PostToken(warmForm, "")
		Expect(err).To(Succeed())
		Expect(warm.StatusCode).To(Equal(http.StatusUnauthorized))
		_ = warm.Body.Close()

		newKey, err := helpers.GenerateClientAssertionKey(jwa.RS256(), "client-key-new")
		Expect(err).To(Succeed())
		newJWKS, err := newKey.JWKSJSON()
		Expect(err).To(Succeed())
		fixture.SetJWKS(newJWKS)
		codeAssertion, codeType, err := fixture.SignAssertionFor(fixture.clientID, newKey, helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		codeForm := fixture.CodeForm(code, verifier)
		codeForm.Set("client_assertion_type", codeType)
		codeForm.Set("client_assertion", codeAssertion)
		accepted, err := fixture.PostToken(codeForm, "")
		Expect(err).To(Succeed())
		Expect(accepted.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]any
		Expect(json.NewDecoder(accepted.Body).Decode(&tokenBody)).To(Succeed())
		_ = accepted.Body.Close()
		refreshToken, ok := tokenBody["refresh_token"].(string)
		Expect(ok).To(BeTrue())

		oldKeyAssertion, oldType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		refreshForm := fixture.RefreshForm(refreshToken)
		refreshForm.Set("client_assertion_type", oldType)
		refreshForm.Set("client_assertion", oldKeyAssertion)
		removedKey, err := fixture.PostToken(refreshForm, "")
		Expect(err).To(Succeed())
		defer func() { _ = removedKey.Body.Close() }()
		Expect(removedKey.StatusCode).To(Equal(http.StatusUnauthorized))
	})

	// Scenario US4-S3 from specs/051-private-key-jwt-auth/spec.md
	It("refuses a jwks_uri that resolves to a blocked address", func() {
		fixture.Close()
		fixture = nil
		var err error
		fixture, err = newCIMDPrivateKeyJWTFixtureWithProductionKeySetFetcher(true)
		Expect(err).To(Succeed())
		Expect(fixture.SetDocument("private_key_jwt", map[string]any{
			"jwks_uri": "https://10.0.0.1/jwks.json",
		})).To(Succeed())
		code, verifier, err := fixture.AuthorizeCode("read")
		Expect(err).To(Succeed())
		assertion, assertionType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		form := fixture.CodeForm(code, verifier)
		form.Set("client_assertion_type", assertionType)
		form.Set("client_assertion", assertion)
		response, err := fixture.PostToken(form, "")
		Expect(err).To(Succeed())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(BeNumerically(">=", 500))
	})

	// Scenario US4-S4 from specs/051-private-key-jwt-auth/spec.md
	DescribeTable("fails closed when the JWKS dependency fails", func(mode string) {
		if mode == "timeout" {
			fixture.Close()
			var err error
			fixture, err = newCIMDPrivateKeyJWTFixtureWithKeySetSettings(false, 0, 250*time.Millisecond, 250*time.Millisecond)
			Expect(err).To(Succeed())
			Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
		}
		fixture.SetJWKSHandler(func(w http.ResponseWriter, r *http.Request) {
			switch mode {
			case "redirect":
				http.Redirect(w, r, "/other.json", http.StatusFound)
			case "oversized":
				_, _ = w.Write([]byte(strings.Repeat("x", 70000)))
			case "malformed":
				_, _ = w.Write([]byte("{"))
			case "timeout":
				<-r.Context().Done()
			}
		})
		code, verifier, err := fixture.AuthorizeCode("read")
		Expect(err).To(Succeed())
		assertion, assertionType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		form := fixture.CodeForm(code, verifier)
		form.Set("client_assertion_type", assertionType)
		form.Set("client_assertion", assertion)
		response, err := fixture.PostToken(form, "")
		Expect(err).To(Succeed())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(BeNumerically(">=", 500))
	},
		Entry("redirect", "redirect"),
		Entry("oversized body", "oversized"),
		Entry("malformed JSON", "malformed"),
		Entry("timeout", "timeout"),
	)

	// Scenario US4-S5 from specs/051-private-key-jwt-auth/spec.md
	It("never selects weak, non-signature, or unsupported keys", func() {
		weakRSA, err := rsa.GenerateKey(rand.Reader, 1024)
		Expect(err).To(Succeed())
		weakJWK, err := json.Marshal(map[string]string{
			"kty": "RSA",
			"kid": "weak-rsa",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(weakRSA.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}),
		})
		Expect(err).To(Succeed())
		clientRSA := fixture.assertionKey.PrivateKey.(*rsa.PrivateKey)
		encJWK, err := jwk.Import[jwk.Key](&clientRSA.PublicKey)
		Expect(err).To(Succeed())
		Expect(encJWK.Set(jwk.KeyIDKey, "encryption-only")).To(Succeed())
		Expect(encJWK.Set(jwk.KeyUsageKey, "enc")).To(Succeed())
		keySet := jwk.NewSet()
		Expect(keySet.AddKey(encJWK)).To(Succeed())
		unsupported := map[string]any{"kty": "oct", "kid": "unsupported", "k": "AQAB"}
		encoded, err := json.Marshal(keySet)
		Expect(err).To(Succeed())
		var keySetDocument map[string][]json.RawMessage
		Expect(json.Unmarshal(encoded, &keySetDocument)).To(Succeed())
		unsupportedEncoded, err := json.Marshal(unsupported)
		Expect(err).To(Succeed())
		keySetDocument["keys"] = append(keySetDocument["keys"], weakJWK, unsupportedEncoded)
		encoded, err = json.Marshal(keySetDocument)
		Expect(err).To(Succeed())
		fixture.SetJWKS(encoded)
		code, verifier, err := fixture.AuthorizeCode("read")
		Expect(err).To(Succeed())
		assertion, assertionType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		form := fixture.CodeForm(code, verifier)
		form.Set("client_assertion_type", assertionType)
		form.Set("client_assertion", assertion)
		response, err := fixture.PostToken(form, "")
		Expect(err).To(Succeed())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusUnauthorized))
	})

	// Scenario US4-S6 from specs/051-private-key-jwt-auth/spec.md
	It("limits repeated unknown-kid fetches", func() {
		code, verifier, err := fixture.AuthorizeCode("read")
		Expect(err).To(Succeed())
		unknownKey, err := helpers.GenerateClientAssertionKey(jwa.RS256(), "unknown-kid")
		Expect(err).To(Succeed())
		assertion, assertionType, err := fixture.SignAssertionFor(fixture.clientID, unknownKey, helpers.ClientAssertionOptions{JTI: "unknown-kid-jti"})
		Expect(err).To(Succeed())
		for range 3 {
			form := fixture.CodeForm(code, verifier)
			form.Set("client_assertion_type", assertionType)
			form.Set("client_assertion", assertion)
			response, err := fixture.PostToken(form, "")
			Expect(err).To(Succeed())
			_ = response.Body.Close()
		}
		Expect(fixture.JWKSRequestCount()).To(BeNumerically("<=", 2))
	})
})
