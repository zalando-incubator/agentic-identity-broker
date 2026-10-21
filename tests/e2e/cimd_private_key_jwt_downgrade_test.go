package e2e_test

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

type invalidCIMDAssertionBuilder func(*cimdPrivateKeyJWTFixture) (string, string, error)

func signAssertionWithKey(fixture *cimdPrivateKeyJWTFixture, key *helpers.ClientAssertionKey, options helpers.ClientAssertionOptions) (string, string, error) {
	issuer := fixture.server.App().Config.Server.EndUser.PublicURL
	return helpers.SignClientAssertion(fixture.clientID, issuer+"/oauth2/token", key, options)
}

func responseOAuthError(response *http.Response) string {
	defer func() { _ = response.Body.Close() }()
	var body map[string]any
	Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
	errorCode, _ := body["error"].(string)
	return errorCode
}

var _ = Describe("CIMD private_key_jwt downgrade protection", func() {
	var fixture *cimdPrivateKeyJWTFixture

	BeforeEach(func() {
		var err error
		fixture, err = newCIMDPrivateKeyJWTFixture()
		Expect(err).To(Succeed())
	})

	AfterEach(func() {
		if fixture != nil {
			fixture.Close()
		}
	})

	// Scenario US2-S1 from specs/051-private-key-jwt-auth/spec.md
	It("should reject code exchange and refresh without assertions", func() {
		Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
		code, verifier, err := fixture.AuthorizeCode("read offline_access")
		Expect(err).To(Succeed())
		assertion, assertionType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())

		missingCodeAssertion, err := fixture.PostToken(fixture.CodeForm(code, verifier), "")
		Expect(err).To(Succeed())
		Expect(missingCodeAssertion.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(responseOAuthError(missingCodeAssertion)).To(Equal("invalid_client"))

		codeForm := fixture.CodeForm(code, verifier)
		codeForm.Set("client_assertion_type", assertionType)
		codeForm.Set("client_assertion", assertion)
		tokenResponse, err := fixture.PostToken(codeForm, "")
		Expect(err).To(Succeed())
		Expect(tokenResponse.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]any
		Expect(json.NewDecoder(tokenResponse.Body).Decode(&tokenBody)).To(Succeed())
		_ = tokenResponse.Body.Close()
		refreshToken, ok := tokenBody["refresh_token"].(string)
		Expect(ok).To(BeTrue())

		missingRefreshAssertion, err := fixture.PostToken(fixture.RefreshForm(refreshToken), "")
		Expect(err).To(Succeed())
		Expect(missingRefreshAssertion.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(responseOAuthError(missingRefreshAssertion)).To(Equal("invalid_client"))
	})

	// Scenario US2-S2 from specs/051-private-key-jwt-auth/spec.md
	DescribeTable("rejects each invalid client assertion without identifying the cause",
		func(build invalidCIMDAssertionBuilder) {
			Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
			code, verifier, err := fixture.AuthorizeCode("read offline_access")
			Expect(err).To(Succeed())
			assertionType, assertion, err := build(fixture)
			Expect(err).To(Succeed())
			form := fixture.CodeForm(code, verifier)
			form.Set("client_assertion_type", assertionType)
			form.Set("client_assertion", assertion)
			response, err := fixture.PostToken(form, "")
			Expect(err).To(Succeed())
			Expect(response.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(responseOAuthError(response)).To(Equal("invalid_client"))
			logs := fixture.logBuffer.String()
			Expect(logs).ToNot(ContainSubstring(assertion))
			Expect(logs).ToNot(ContainSubstring(code))

			validAssertion, validType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
			Expect(err).To(Succeed())
			validCodeForm := fixture.CodeForm(code, verifier)
			validCodeForm.Set("client_assertion_type", validType)
			validCodeForm.Set("client_assertion", validAssertion)
			issued, err := fixture.PostToken(validCodeForm, "")
			Expect(err).To(Succeed())
			Expect(issued.StatusCode).To(Equal(http.StatusOK))
			var tokenBody map[string]any
			Expect(json.NewDecoder(issued.Body).Decode(&tokenBody)).To(Succeed())
			_ = issued.Body.Close()
			refreshToken, ok := tokenBody["refresh_token"].(string)
			Expect(ok).To(BeTrue())

			invalidRefreshForm := fixture.RefreshForm(refreshToken)
			invalidRefreshForm.Set("client_assertion_type", assertionType)
			invalidRefreshForm.Set("client_assertion", assertion)
			invalidRefresh, err := fixture.PostToken(invalidRefreshForm, "")
			Expect(err).To(Succeed())
			Expect(invalidRefresh.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(responseOAuthError(invalidRefresh)).To(Equal("invalid_client"))

			freshAssertion, freshType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
			Expect(err).To(Succeed())
			validRefreshForm := fixture.RefreshForm(refreshToken)
			validRefreshForm.Set("client_assertion_type", freshType)
			validRefreshForm.Set("client_assertion", freshAssertion)
			validRefresh, err := fixture.PostToken(validRefreshForm, "")
			Expect(err).To(Succeed())
			defer func() { _ = validRefresh.Body.Close() }()
			Expect(validRefresh.StatusCode).To(Equal(http.StatusOK))
		},
		Entry("bad signature", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			key, err := helpers.GenerateClientAssertionKey(jwa.RS256(), f.assertionKey.KID)
			if err != nil {
				return "", "", err
			}
			return signAssertionWithKey(f, key, helpers.ClientAssertionOptions{})
		}),
		Entry("wrong issuer", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{Issuer: "https://wrong.example/client"})
		}),
		Entry("wrong subject", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{Subject: "https://wrong.example/client"})
		}),
		Entry("wrong audience", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{Audience: []string{"https://wrong.example/token"}})
		}),
		Entry("expired assertion", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{Lifetime: -time.Minute})
		}),
		Entry("lifetime over five minutes", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{Lifetime: 6 * time.Minute})
		}),
		Entry("missing expiration", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{OmitExpiration: true})
		}),
		Entry("missing kid", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{OmitKID: true})
		}),
		Entry("missing jti", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{OmitJTI: true})
		}),
		Entry("empty jti", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{EmptyJTI: true})
		}),
		Entry("wrong assertion type", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{AssertionType: "urn:wrong:type"})
		}),
		Entry("algorithm differs from the document", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{Algorithm: jwa.PS256()})
		}),
		Entry("unsigned algorithm", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			return f.SignAssertion(helpers.ClientAssertionOptions{Algorithm: jwa.NoSignature()})
		}),
		Entry("future issued-at", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			future := time.Now().Add(time.Minute)
			return f.SignAssertion(helpers.ClientAssertionOptions{IssuedAt: &future})
		}),
		Entry("future not-before", func(f *cimdPrivateKeyJWTFixture) (string, string, error) {
			future := time.Now().Add(time.Minute)
			return f.SignAssertion(helpers.ClientAssertionOptions{NotBefore: &future})
		}),
	)

	// Scenario US2-S3 from specs/051-private-key-jwt-auth/spec.md
	It("should preserve grants after failed authentication", func() {
		Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
		code, verifier, err := fixture.AuthorizeCode("read offline_access")
		Expect(err).To(Succeed())
		badAssertion, assertionType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{Audience: []string{"https://wrong.example/token"}})
		Expect(err).To(Succeed())
		failedCodeForm := fixture.CodeForm(code, verifier)
		failedCodeForm.Set("client_assertion_type", assertionType)
		failedCodeForm.Set("client_assertion", badAssertion)
		failedCode, err := fixture.PostToken(failedCodeForm, "")
		Expect(err).To(Succeed())
		Expect(failedCode.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(responseOAuthError(failedCode)).To(Equal("invalid_client"))

		validAssertion, validType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		validCodeForm := fixture.CodeForm(code, verifier)
		validCodeForm.Set("client_assertion_type", validType)
		validCodeForm.Set("client_assertion", validAssertion)
		tokenResponse, err := fixture.PostToken(validCodeForm, "")
		Expect(err).To(Succeed())
		Expect(tokenResponse.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]any
		Expect(json.NewDecoder(tokenResponse.Body).Decode(&tokenBody)).To(Succeed())
		_ = tokenResponse.Body.Close()
		refreshToken, ok := tokenBody["refresh_token"].(string)
		Expect(ok).To(BeTrue())

		badRefreshAssertion, badRefreshType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{Audience: []string{"https://wrong.example/token"}})
		Expect(err).To(Succeed())
		badRefreshForm := fixture.RefreshForm(refreshToken)
		badRefreshForm.Set("client_assertion_type", badRefreshType)
		badRefreshForm.Set("client_assertion", badRefreshAssertion)
		failedRefresh, err := fixture.PostToken(badRefreshForm, "")
		Expect(err).To(Succeed())
		Expect(failedRefresh.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(responseOAuthError(failedRefresh)).To(Equal("invalid_client"))

		freshAssertion, freshType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		validRefreshForm := fixture.RefreshForm(refreshToken)
		validRefreshForm.Set("client_assertion_type", freshType)
		validRefreshForm.Set("client_assertion", freshAssertion)
		validRefresh, err := fixture.PostToken(validRefreshForm, "")
		Expect(err).To(Succeed())
		defer func() { _ = validRefresh.Body.Close() }()
		Expect(validRefresh.StatusCode).To(Equal(http.StatusOK))
	})

	// Scenario US2-S4 from specs/051-private-key-jwt-auth/spec.md
	It("should use the singular private_key_jwt method over plural none", func() {
		Expect(fixture.SetDocument("private_key_jwt", map[string]any{
			"token_endpoint_auth_methods_supported": []string{"none", "private_key_jwt"},
		})).To(Succeed())
		code, verifier, err := fixture.AuthorizeCode("read offline_access")
		Expect(err).To(Succeed())
		response, err := fixture.PostToken(fixture.CodeForm(code, verifier), "")
		Expect(err).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(responseOAuthError(response)).To(Equal("invalid_client"))
	})

	// Scenario US2-S5 from specs/051-private-key-jwt-auth/spec.md
	It("should require authentication after the client document downgrades", func() {
		Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
		code, verifier, err := fixture.AuthorizeCode("read offline_access")
		Expect(err).To(Succeed())
		assertion, assertionType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		form := fixture.CodeForm(code, verifier)
		form.Set("client_assertion_type", assertionType)
		form.Set("client_assertion", assertion)
		issued, err := fixture.PostToken(form, "")
		Expect(err).To(Succeed())
		Expect(issued.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]any
		Expect(json.NewDecoder(issued.Body).Decode(&tokenBody)).To(Succeed())
		_ = issued.Body.Close()
		refreshToken, ok := tokenBody["refresh_token"].(string)
		Expect(ok).To(BeTrue())

		Expect(fixture.SetDocument("none", nil)).To(Succeed())
		missing, err := fixture.PostToken(fixture.RefreshForm(refreshToken), "")
		Expect(err).To(Succeed())
		Expect(missing.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(responseOAuthError(missing)).To(Equal("invalid_client"))
	})

	// Scenario US2-S6 from specs/051-private-key-jwt-auth/spec.md
	It("should ignore assertion-supplied key URLs and embedded keys", func() {
		Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
		code, verifier, err := fixture.AuthorizeCode("read")
		Expect(err).To(Succeed())
		attackerKey, err := helpers.GenerateClientAssertionKey(jwa.RS256(), "attacker-key")
		Expect(err).To(Succeed())
		assertion, assertionType, err := signAssertionWithKey(fixture, attackerKey, helpers.ClientAssertionOptions{
			JKU:        "https://attacker.example/keys.json",
			IncludeJWK: true,
		})
		Expect(err).To(Succeed())
		form := fixture.CodeForm(code, verifier)
		form.Set("client_assertion_type", assertionType)
		form.Set("client_assertion", assertion)
		response, err := fixture.PostToken(form, "")
		Expect(err).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(responseOAuthError(response)).To(Equal("invalid_client"))
	})

	// Scenario US2-S7 from specs/051-private-key-jwt-auth/spec.md
	It("should reject assertions from none clients on both grants", func() {
		Expect(fixture.SetDocument("none", nil)).To(Succeed())
		code, verifier, err := fixture.AuthorizeCode("read offline_access")
		Expect(err).To(Succeed())
		assertion, assertionType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		form := fixture.CodeForm(code, verifier)
		form.Set("client_assertion_type", assertionType)
		form.Set("client_assertion", assertion)
		response, err := fixture.PostToken(form, "")
		Expect(err).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(responseOAuthError(response)).To(Equal("invalid_client"))

		codeResponse, err := fixture.PostToken(fixture.CodeForm(code, verifier), "")
		Expect(err).To(Succeed())
		Expect(codeResponse.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]any
		Expect(json.NewDecoder(codeResponse.Body).Decode(&tokenBody)).To(Succeed())
		_ = codeResponse.Body.Close()
		refreshToken, ok := tokenBody["refresh_token"].(string)
		Expect(ok).To(BeTrue())

		refreshForm := fixture.RefreshForm(refreshToken)
		refreshForm.Set("client_assertion_type", assertionType)
		refreshForm.Set("client_assertion", assertion)
		refreshResponse, err := fixture.PostToken(refreshForm, "")
		Expect(err).To(Succeed())
		Expect(refreshResponse.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(responseOAuthError(refreshResponse)).To(Equal("invalid_client"))
	})

	// Scenario US2-S8 from specs/051-private-key-jwt-auth/spec.md
	It("should reject assertions combined with secret or authorization", func() {
		Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
		code, verifier, err := fixture.AuthorizeCode("read offline_access")
		Expect(err).To(Succeed())
		assertion, assertionType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		ambiguousCode := fixture.CodeForm(code, verifier)
		ambiguousCode.Set("client_assertion_type", assertionType)
		ambiguousCode.Set("client_assertion", assertion)
		ambiguousCode.Set("client_secret", "unexpected-secret")
		badCode, err := fixture.PostToken(ambiguousCode, "")
		Expect(err).To(Succeed())
		Expect(badCode.StatusCode).To(Equal(http.StatusBadRequest))
		Expect(responseOAuthError(badCode)).To(Equal("invalid_request"))

		validCode := fixture.CodeForm(code, verifier)
		validCode.Set("client_assertion_type", assertionType)
		validCode.Set("client_assertion", assertion)
		issued, err := fixture.PostToken(validCode, "")
		Expect(err).To(Succeed())
		Expect(issued.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]any
		Expect(json.NewDecoder(issued.Body).Decode(&tokenBody)).To(Succeed())
		_ = issued.Body.Close()
		refreshToken, ok := tokenBody["refresh_token"].(string)
		Expect(ok).To(BeTrue())

		freshAssertion, freshType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		ambiguousRefresh := fixture.RefreshForm(refreshToken)
		ambiguousRefresh.Set("client_assertion_type", freshType)
		ambiguousRefresh.Set("client_assertion", freshAssertion)
		badRefresh, err := fixture.PostToken(ambiguousRefresh, "Basic dGVzdA==")
		Expect(err).To(Succeed())
		Expect(badRefresh.StatusCode).To(Equal(http.StatusBadRequest))
		Expect(responseOAuthError(badRefresh)).To(Equal("invalid_request"))

		validRefresh := fixture.RefreshForm(refreshToken)
		validRefresh.Set("client_assertion_type", freshType)
		validRefresh.Set("client_assertion", freshAssertion)
		refresh, err := fixture.PostToken(validRefresh, "")
		Expect(err).To(Succeed())
		defer func() { _ = refresh.Body.Close() }()
		Expect(refresh.StatusCode).To(Equal(http.StatusOK))
	})

	// Scenario US2-S9 from specs/051-private-key-jwt-auth/spec.md
	It("should reject another client using the owner's grants", func() {
		Expect(fixture.SetDocument("private_key_jwt", nil)).To(Succeed())
		clientB := "https://" + cimdPrivateKeyJWTFakeHost + "/client-b"
		redirectB := "https://" + cimdPrivateKeyJWTFakeHost + "/callback-b"
		agentB, err := fixture.AddClient(clientB, redirectB)
		Expect(err).To(Succeed())
		Expect(fixture.SetClientDocument(clientB, redirectB, "private_key_jwt", nil)).To(Succeed())

		codeA, verifierA, err := fixture.AuthorizeCode("read offline_access")
		Expect(err).To(Succeed())
		assertionB, assertionTypeB, err := fixture.SignAssertionFor(clientB, fixture.assertionKey, helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		crossCode := fixture.CodeFormFor(clientB, redirectB, codeA, verifierA)
		crossCode.Set("client_assertion_type", assertionTypeB)
		crossCode.Set("client_assertion", assertionB)
		rejectedCode, err := fixture.PostToken(crossCode, "")
		Expect(err).To(Succeed())
		Expect(rejectedCode.StatusCode).To(Or(Equal(http.StatusBadRequest), Equal(http.StatusUnauthorized)))
		_ = rejectedCode.Body.Close()

		assertionA, assertionTypeA, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		validCode := fixture.CodeForm(codeA, verifierA)
		validCode.Set("client_assertion_type", assertionTypeA)
		validCode.Set("client_assertion", assertionA)
		issued, err := fixture.PostToken(validCode, "")
		Expect(err).To(Succeed())
		Expect(issued.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]any
		Expect(json.NewDecoder(issued.Body).Decode(&tokenBody)).To(Succeed())
		_ = issued.Body.Close()
		refreshA, ok := tokenBody["refresh_token"].(string)
		Expect(ok).To(BeTrue())

		crossRefresh := fixture.RefreshFormFor(clientB, refreshA)
		crossRefresh.Set("client_assertion_type", assertionTypeB)
		crossRefresh.Set("client_assertion", assertionB)
		rejectedRefresh, err := fixture.PostToken(crossRefresh, "")
		Expect(err).To(Succeed())
		Expect(rejectedRefresh.StatusCode).To(Or(Equal(http.StatusBadRequest), Equal(http.StatusUnauthorized)))
		_ = rejectedRefresh.Body.Close()

		freshAssertion, freshType, err := fixture.SignAssertion(helpers.ClientAssertionOptions{})
		Expect(err).To(Succeed())
		refreshForm := fixture.RefreshForm(refreshA)
		refreshForm.Set("client_assertion_type", freshType)
		refreshForm.Set("client_assertion", freshAssertion)
		validRefresh, err := fixture.PostToken(refreshForm, "")
		Expect(err).To(Succeed())
		defer func() { _ = validRefresh.Body.Close() }()
		Expect(validRefresh.StatusCode).To(Equal(http.StatusOK))
		_ = agentB
	})
})
