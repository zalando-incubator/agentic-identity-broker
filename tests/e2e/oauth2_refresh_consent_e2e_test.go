package e2e_test

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func expectConsentRefreshError(result *helpers.RefreshTokenResult, status int, code string) {
	ExpectWithOffset(1, result.Status).To(Equal(status))
	ExpectWithOffset(1, result.Error).To(Equal(code))
	for _, field := range []string{"access_token", "refresh_token", "token_type"} {
		_, present := result.Body[field]
		ExpectWithOffset(1, present).To(BeFalse(), field+" must be absent on denial")
	}
	if code == "invalid_grant" {
		ExpectWithOffset(1, result.ErrorURI).To(BeEmpty())
	}
}

func verifyPublishedConsentJWT(server *bootstrap.TestServer, raw string) jwt.Token {
	response, err := server.PublicGET("/oauth2/jwks.json")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	ExpectWithOffset(1, response.StatusCode).To(Equal(http.StatusOK))
	keys, err := jwk.ParseReader(response.Body)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	message, err := jws.Parse([]byte(raw))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, message.Signatures()).To(HaveLen(1))
	keyID, ok := message.Signatures()[0].ProtectedHeaders().KeyID()
	ExpectWithOffset(1, ok).To(BeTrue())
	key, ok := keys.LookupKeyID(keyID)
	ExpectWithOffset(1, ok).To(BeTrue(), "the JWT signing key must be published")

	// The broker provisions ES256 signing keys; do not take the algorithm from the token.
	claims, err := jwt.Parse([]byte(raw), jwt.WithKey(jwa.ES256(), key))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return claims
}

var _ = Describe("Consent-Bound Refresh Sessions / US1", func() {
	var (
		ctx    context.Context
		env    *bootstrap.RefreshEnvironment
		client *fixtures.RefreshClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		config, err := fixtures.RefreshConfig("", "", "")
		Expect(err).NotTo(HaveOccurred())
		env, err = bootstrap.NewRefreshEnvironment(config, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, env.Storage)).To(Succeed())
		client, err = env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if env != nil {
			env.Close()
		}
	})

	// US1-S3 from specs/049-fix-refresh-consent/spec.md: expired consent blocks renewal.
	It("US1-S3 denies renewal at or after the grant deadline", func() {
		finite := client.Grant.Copy()
		deadline := time.Now().UTC().Add(time.Hour)
		finite.ValidUntil = &deadline
		Expect(env.Storage.UserGrants().Update(ctx, finite)).To(Succeed())
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		first, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Status).To(Equal(http.StatusOK), "an active grant must permit renewal")
		Expect(first.Tokens.RefreshToken).NotTo(BeEmpty())

		expired := finite.Copy()
		pastDeadline := time.Now().UTC().Add(-time.Minute)
		expired.ValidUntil = &pastDeadline
		Expect(env.Storage.UserGrants().Update(ctx, expired)).To(Succeed())
		stored, err := env.Storage.UserGrants().Get(ctx, expired.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.ValidUntil).NotTo(BeNil())
		Expect(stored.ValidUntil.Before(time.Now())).To(BeTrue())

		denied, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, first.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		expectConsentRefreshError(denied, http.StatusBadRequest, "invalid_grant")
	})

	// US1-S4 from specs/049-fix-refresh-consent/spec.md: consent overrides a permitted predecessor retry.
	It("US1-S4 checks consent even inside a confirmed bounded retry interval", func() {
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		rotated, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(rotated.Status).To(Equal(http.StatusOK))
		Expect(rotated.Tokens.RefreshToken).NotTo(BeEmpty())

		recovered, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(recovered.Status).To(Equal(http.StatusOK), "the same bound client must first recover the previous result")
		Expect(recovered.Tokens.AccessToken == rotated.Tokens.AccessToken).To(BeTrue(), "the original access token must match")
		Expect(recovered.Tokens.RefreshToken == rotated.Tokens.RefreshToken).To(BeTrue(), "the original refresh token must match")

		response, err := env.Enduser.DirectRequest(http.MethodDelete,
			"/api/consent/agents/"+client.Agent.ID.String()+"/grants", client.Principal.String(), nil, nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusNoContent))

		predecessor, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		expectConsentRefreshError(predecessor, http.StatusBadRequest, "invalid_grant")
		current, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rotated.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		expectConsentRefreshError(current, http.StatusBadRequest, "invalid_grant")
	})

	// US1-S6 from specs/049-fix-refresh-consent/spec.md: an unrelated authenticated client has no authority over another session.
	It("US1-S6 preserves the bound session when a different eligible client presents its token", func() {
		other, err := env.AddClient(ctx, client.Principal, true)
		Expect(err).NotTo(HaveOccurred())
		otherIssued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *other, "read offline_access")
		Expect(err).NotTo(HaveOccurred(), "the other client must have its own active grant and valid credentials")
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())

		intruder, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *other, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		expectConsentRefreshError(intruder, http.StatusBadRequest, "invalid_grant")

		owner, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(owner.Status).To(Equal(http.StatusOK), "mismatched presentation must not consume or revoke the owner's token")
		Expect(owner.Tokens.RefreshToken).NotTo(BeEmpty())
		Expect(owner.Tokens.RefreshToken != issued.Tokens.RefreshToken).To(BeTrue(), "fresh rotation must replace the bound token")
		otherResult, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *other, otherIssued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(otherResult.Status).To(Equal(http.StatusOK))
	})

	// US1-S7 from specs/049-fix-refresh-consent/spec.md: a previously issued JWT remains valid at signature-only consumers.
	It("US1-S7 retains the published-JWKS JWT expiry but blocks new tokens after revocation", func() {
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		rotated, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(rotated.Status).To(Equal(http.StatusOK))
		Expect(rotated.Tokens.AccessToken).NotTo(BeEmpty())
		before := verifyPublishedConsentJWT(env.Enduser, rotated.Tokens.AccessToken)
		expiry, present := before.Expiration()
		Expect(present).To(BeTrue())
		Expect(expiry.After(time.Now())).To(BeTrue())

		response, err := env.Enduser.DirectRequest(http.MethodDelete,
			"/api/consent/agents/"+client.Agent.ID.String()+"/grants", client.Principal.String(), nil, nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusNoContent))

		denied, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rotated.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		expectConsentRefreshError(denied, http.StatusBadRequest, "invalid_grant")
		after := verifyPublishedConsentJWT(env.Enduser, rotated.Tokens.AccessToken)
		remainingExpiry, present := after.Expiration()
		Expect(present).To(BeTrue())
		Expect(remainingExpiry).To(Equal(expiry), "grant revocation must not alter an already signed JWT")
		Expect(remainingExpiry.After(time.Now())).To(BeTrue(), "the previously issued JWT still verifies until its own expiry")
		principal, present := after.Subject()
		Expect(present).To(BeTrue())
		Expect(principal).To(Equal(client.Principal.String()))
		claimsScope, err := jwt.Get[string](after, "scope")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(claimsScope)).To(ConsistOf("read", "offline_access"))
	})
})
