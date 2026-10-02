package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const lifecycleScope = "read offline_access"

func lifecycleAuthorize(ctx context.Context, env *bootstrap.RefreshEnvironment, client *fixtures.RefreshClient) *helpers.RefreshAuthorization {
	GinkgoHelper()
	authorization, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, lifecycleScope)
	Expect(err).NotTo(HaveOccurred())
	return authorization
}

func lifecycleRotate(ctx context.Context, env *bootstrap.RefreshEnvironment, client *fixtures.RefreshClient, token string) *helpers.RefreshTokenResult {
	GinkgoHelper()
	result, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, token, "")
	Expect(err).NotTo(HaveOccurred())
	return result
}

func lifecycleExpectTokens(result *helpers.RefreshTokenResult) {
	GinkgoHelper()
	Expect(result.Status).To(Equal(http.StatusOK))
	Expect(result.Tokens.AccessToken).NotTo(BeEmpty())
	Expect(result.Tokens.RefreshToken).NotTo(BeEmpty())
}

func lifecycleExpectNoTokens(result *helpers.RefreshTokenResult) {
	GinkgoHelper()
	Expect(result.Tokens.AccessToken == "").To(BeTrue())
	Expect(result.Tokens.RefreshToken == "").To(BeTrue())
	_, hasAccess := result.Body["access_token"]
	_, hasRefresh := result.Body["refresh_token"]
	_, hasID := result.Body["id_token"]
	Expect(hasAccess).To(BeFalse())
	Expect(hasRefresh).To(BeFalse())
	Expect(hasID).To(BeFalse())
}

func lifecycleExpectInvalidGrant(result *helpers.RefreshTokenResult) {
	GinkgoHelper()
	Expect(result.Status).To(Equal(http.StatusBadRequest))
	Expect(result.Error).To(Equal("invalid_grant"))
	Expect(result.ErrorURI).To(BeEmpty())
	lifecycleExpectNoTokens(result)
}

func lifecycleDelete(ctx context.Context, server *bootstrap.TestServer, path string, principal id.Principal) int {
	GinkgoHelper()
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, server.BaseURL()+path, nil)
	Expect(err).NotTo(HaveOccurred())
	request.Header.Set("X-Remote-User", principal.String())
	response, err := helpers.HTTPClient().Do(request)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode
}

func lifecycleGrantAction(env *bootstrap.RefreshEnvironment, client *fixtures.RefreshClient, grant bool) int {
	GinkgoHelper()
	sets := map[string][]string{}
	if grant {
		sets[fixtures.PlaceholderPermissionSetID.String()] = []string{fixtures.PlaceholderServiceID.String()}
	}
	body, err := json.Marshal(map[string]any{"granted_permission_sets": sets})
	Expect(err).NotTo(HaveOccurred())
	response, err := env.Enduser.AuthenticatedPOST(
		"/api/consent/agents/"+client.Agent.ID.String()+"/grants",
		client.Principal.String(), "application/json", bytes.NewReader(body),
	)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode
}

func lifecycleAddPrincipal(ctx context.Context, env *bootstrap.RefreshEnvironment, first *fixtures.RefreshClient, principal id.Principal) *fixtures.RefreshClient {
	GinkgoHelper()
	grant := fixtures.RefreshGrant(principal, first.Agent.ID)
	Expect(env.Storage.UserGrants().Create(ctx, grant)).To(Succeed())
	return &fixtures.RefreshClient{Agent: first.Agent, Grant: grant, Principal: principal, Secret: first.Secret}
}

func lifecycleConnectService(ctx context.Context, env *bootstrap.RefreshEnvironment, principal id.Principal) {
	GinkgoHelper()
	session := fixtures.SessionForService(principal.String(), fixtures.PlaceholderServiceID.String())
	Expect(env.Storage.UserSessions().Create(ctx, session)).To(Succeed())
}

var _ = Describe("Consent-Bound Refresh Sessions / US2", func() {
	var (
		ctx context.Context
		env *bootstrap.RefreshEnvironment
	)

	BeforeEach(func() {
		ctx = context.Background()
		config, err := fixtures.RefreshConfig("2m", "0s", "720h")
		Expect(err).NotTo(HaveOccurred())
		env, err = bootstrap.NewRefreshEnvironment(config, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, env.Storage)).To(Succeed())
	})

	AfterEach(func() {
		if env != nil {
			env.Close()
		}
	})

	// US2-S1 from specs/049-fix-refresh-consent/spec.md
	It("ends every session of one principal without ending the other principal's same-agent session", func() {
		first, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		second := lifecycleAddPrincipal(ctx, env, first, id.Principal(fixtures.AnotherPrincipal().String()))
		firstA := lifecycleAuthorize(ctx, env, first)
		firstB := lifecycleAuthorize(ctx, env, first)
		other := lifecycleAuthorize(ctx, env, second)

		Expect(lifecycleDelete(ctx, env.Enduser, "/api/consent/agents/"+first.Agent.ID.String()+"/grants", first.Principal)).To(Equal(http.StatusNoContent))
		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, first, firstA.Tokens.RefreshToken))
		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, first, firstB.Tokens.RefreshToken))
		lifecycleExpectTokens(lifecycleRotate(ctx, env, second, other.Tokens.RefreshToken))
	})

	// US2-S2 from specs/049-fix-refresh-consent/spec.md
	It("keeps other agents and a connected provider session after each consent-deletion route", func() {
		principal := id.Principal(fixtures.DefaultPrincipal().String())
		first, err := env.AddClient(ctx, principal, true)
		Expect(err).NotTo(HaveOccurred())
		second, err := env.AddClient(ctx, principal, true)
		Expect(err).NotTo(HaveOccurred())
		unrelated, err := env.AddClient(ctx, principal, true)
		Expect(err).NotTo(HaveOccurred())
		provider := fixtures.SessionForService(principal.String(), fixtures.PlaceholderServiceID.String())
		Expect(env.Storage.UserSessions().Create(ctx, provider)).To(Succeed())
		firstAuth := lifecycleAuthorize(ctx, env, first)
		secondAuth := lifecycleAuthorize(ctx, env, second)
		unrelatedAuth := lifecycleAuthorize(ctx, env, unrelated)

		Expect(lifecycleDelete(ctx, env.Enduser, "/api/consent/agents/"+first.Agent.ID.String()+"/grants", principal)).To(Equal(http.StatusNoContent))
		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, first, firstAuth.Tokens.RefreshToken))
		secondRotated := lifecycleRotate(ctx, env, second, secondAuth.Tokens.RefreshToken)
		lifecycleExpectTokens(secondRotated)
		unrelatedRotated := lifecycleRotate(ctx, env, unrelated, unrelatedAuth.Tokens.RefreshToken)
		lifecycleExpectTokens(unrelatedRotated)
		connected, err := env.Storage.UserSessions().Get(ctx, provider.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(connected.Principal).To(Equal(principal))
		Expect(connected.ServiceID).To(Equal(provider.ServiceID))
		Expect(helpers.RefreshSignature(string(connected.EncryptedAccessToken))).To(Equal(helpers.RefreshSignature(string(provider.EncryptedAccessToken))))
		Expect(helpers.RefreshSignature(string(connected.EncryptedRefreshToken))).To(Equal(helpers.RefreshSignature(string(provider.EncryptedRefreshToken))))
		Expect(connected.RefreshTokenExpiresAt).To(Equal(provider.RefreshTokenExpiresAt))

		Expect(lifecycleGrantAction(env, second, false)).To(Equal(http.StatusNoContent))
		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, second, secondRotated.Tokens.RefreshToken))
		lifecycleExpectTokens(lifecycleRotate(ctx, env, unrelated, unrelatedRotated.Tokens.RefreshToken))
		connected, err = env.Storage.UserSessions().Get(ctx, provider.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(connected.Principal).To(Equal(principal))
		Expect(connected.ServiceID).To(Equal(provider.ServiceID))
		Expect(helpers.RefreshSignature(string(connected.EncryptedAccessToken))).To(Equal(helpers.RefreshSignature(string(provider.EncryptedAccessToken))))
		Expect(helpers.RefreshSignature(string(connected.EncryptedRefreshToken))).To(Equal(helpers.RefreshSignature(string(provider.EncryptedRefreshToken))))
		Expect(connected.RefreshTokenExpiresAt).To(Equal(provider.RefreshTokenExpiresAt))
	})

	// US2-S3 from specs/049-fix-refresh-consent/spec.md
	It("ends every user's session on agent deletion without ending another agent's session", func() {
		first, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		second := lifecycleAddPrincipal(ctx, env, first, id.Principal(fixtures.AnotherPrincipal().String()))
		other, err := env.AddClient(ctx, first.Principal, true)
		Expect(err).NotTo(HaveOccurred())
		firstAuth := lifecycleAuthorize(ctx, env, first)
		secondAuth := lifecycleAuthorize(ctx, env, second)
		otherAuth := lifecycleAuthorize(ctx, env, other)

		Expect(lifecycleDelete(ctx, env.Admin, "/api/agents/"+first.Agent.ID.String(), id.Principal(fixtures.AdminPrincipal().String()))).To(Equal(http.StatusNoContent))
		for _, pair := range []struct {
			client *fixtures.RefreshClient
			token  string
		}{{first, firstAuth.Tokens.RefreshToken}, {second, secondAuth.Tokens.RefreshToken}} {
			denied := lifecycleRotate(ctx, env, pair.client, pair.token)
			Expect(denied.Status).To(Or(Equal(http.StatusBadRequest), Equal(http.StatusUnauthorized)))
			lifecycleExpectNoTokens(denied)
		}
		lifecycleExpectTokens(lifecycleRotate(ctx, env, other, otherAuth.Tokens.RefreshToken))
	})

	// US2-S4 from specs/049-fix-refresh-consent/spec.md
	It("does not restore explicitly revoked confidential sessions through public-client fallback", func() {
		first, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		second := lifecycleAddPrincipal(ctx, env, first, id.Principal(fixtures.AnotherPrincipal().String()))
		other, err := env.AddClient(ctx, first.Principal, true)
		Expect(err).NotTo(HaveOccurred())
		firstAuth := lifecycleAuthorize(ctx, env, first)
		secondAuth := lifecycleAuthorize(ctx, env, second)
		otherAuth := lifecycleAuthorize(ctx, env, other)

		Expect(lifecycleDelete(ctx, env.Admin, "/api/agents/"+first.Agent.ID.String()+"/client-credentials", id.Principal(fixtures.AdminPrincipal().String()))).To(Equal(http.StatusNoContent))
		for _, pair := range []struct {
			client *fixtures.RefreshClient
			token  string
		}{{first, firstAuth.Tokens.RefreshToken}, {second, secondAuth.Tokens.RefreshToken}} {
			withoutSecret := *pair.client
			withoutSecret.Secret = ""
			lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, &withoutSecret, pair.token))
			withOldSecret := lifecycleRotate(ctx, env, pair.client, pair.token)
			Expect(withOldSecret.Status).NotTo(Equal(http.StatusOK))
			lifecycleExpectNoTokens(withOldSecret)
		}
		lifecycleExpectTokens(lifecycleRotate(ctx, env, other, otherAuth.Tokens.RefreshToken))
	})

	// US2-S5 from specs/049-fix-refresh-consent/spec.md
	It("keeps revoked tokens dead after regrant or new credentials and permits fresh authorization", func() {
		principal := id.Principal(fixtures.DefaultPrincipal().String())
		consentClient, err := env.AddClient(ctx, principal, true)
		Expect(err).NotTo(HaveOccurred())
		credentialClient, err := env.AddClient(ctx, principal, true)
		Expect(err).NotTo(HaveOccurred())
		lifecycleConnectService(ctx, env, principal)
		oldConsent := lifecycleAuthorize(ctx, env, consentClient)
		oldCredential := lifecycleAuthorize(ctx, env, credentialClient)

		Expect(lifecycleDelete(ctx, env.Enduser, "/api/consent/agents/"+consentClient.Agent.ID.String()+"/grants", principal)).To(Equal(http.StatusNoContent))
		Expect(lifecycleDelete(ctx, env.Admin, "/api/agents/"+credentialClient.Agent.ID.String()+"/client-credentials", id.Principal(fixtures.AdminPrincipal().String()))).To(Equal(http.StatusNoContent))
		Expect(lifecycleGrantAction(env, consentClient, true)).To(Equal(http.StatusCreated))
		Expect(env.ReplaceCredentials(ctx, credentialClient)).To(Succeed())

		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, consentClient, oldConsent.Tokens.RefreshToken))
		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, credentialClient, oldCredential.Tokens.RefreshToken))
		freshConsent := lifecycleAuthorize(ctx, env, consentClient)
		freshCredential := lifecycleAuthorize(ctx, env, credentialClient)
		lifecycleExpectTokens(lifecycleRotate(ctx, env, consentClient, freshConsent.Tokens.RefreshToken))
		lifecycleExpectTokens(lifecycleRotate(ctx, env, credentialClient, freshCredential.Tokens.RefreshToken))
	})

	// US2-S9 from specs/049-fix-refresh-consent/spec.md
	It("revokes a leftover session through an idempotent deletion even though its grant is already absent", func() {
		principal := id.Principal(fixtures.DefaultPrincipal().String())
		client, err := env.AddClient(ctx, principal, true)
		Expect(err).NotTo(HaveOccurred())
		lifecycleConnectService(ctx, env, principal)
		authorization := lifecycleAuthorize(ctx, env, client)
		// Arrange the missing-grant state without invoking either lifecycle endpoint.
		Expect(env.Storage.UserGrants().Delete(ctx, client.Grant.ID)).To(Succeed())

		Expect(lifecycleGrantAction(env, client, false)).To(Equal(http.StatusNoContent))
		Expect(lifecycleGrantAction(env, client, true)).To(Equal(http.StatusCreated))
		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, client, authorization.Tokens.RefreshToken))
		fresh := lifecycleAuthorize(ctx, env, client)
		lifecycleExpectTokens(lifecycleRotate(ctx, env, client, fresh.Tokens.RefreshToken))
	})

	// US2-S10 from specs/049-fix-refresh-consent/spec.md
	It("recovers an identical predecessor result before another authenticated client replays its code", func() {
		client, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		other, err := env.AddClient(ctx, client.Principal, true)
		Expect(err).NotTo(HaveOccurred())
		authorization := lifecycleAuthorize(ctx, env, client)
		unrelated := lifecycleAuthorize(ctx, env, other)
		rotated := lifecycleRotate(ctx, env, client, authorization.Tokens.RefreshToken)
		lifecycleExpectTokens(rotated)

		// Feature-specific positive recovery must succeed before the unchanged FR-040 replay check.
		recovered := lifecycleRotate(ctx, env, client, authorization.Tokens.RefreshToken)
		lifecycleExpectTokens(recovered)
		Expect(helpers.RefreshSignature(recovered.Tokens.AccessToken)).To(Equal(helpers.RefreshSignature(rotated.Tokens.AccessToken)))
		Expect(helpers.RefreshSignature(recovered.Tokens.RefreshToken)).To(Equal(helpers.RefreshSignature(rotated.Tokens.RefreshToken)))

		replay, err := helpers.ExchangeRefreshCode(ctx, env.Enduser.BaseURL(), *other, authorization.Code, authorization.Verifier)
		Expect(err).NotTo(HaveOccurred())
		Expect(replay.Status).To(Equal(http.StatusBadRequest))
		Expect(replay.Error).To(Equal("invalid_grant"))
		lifecycleExpectNoTokens(replay)
		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, client, rotated.Tokens.RefreshToken))
		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, client, authorization.Tokens.RefreshToken))
		lifecycleExpectTokens(lifecycleRotate(ctx, env, other, unrelated.Tokens.RefreshToken))
	})
})
