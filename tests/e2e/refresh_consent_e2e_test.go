package e2e_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Continuing consent for local refresh", func() {
	for _, mode := range []string{"local", "hybrid"} {
		Context(mode, func() {
			var (
				store          *storageadapter.Adapter
				admin, enduser *bootstrap.TestServer
				agent          *domainstorage.Agent
				grant          *domainstorage.UserGrant
				secret, token  string
				baseline       map[string]any
			)
			principal := fixtures.DefaultPrincipal().String()

			postToken := func(form url.Values) (int, map[string]any) {
				form.Set("client_id", agent.ID.String())
				if secret != "" {
					form.Set("client_secret", secret)
				}
				resp, err := enduser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()
				var body map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				return resp.StatusCode, body
			}
			issue := func() string {
				verifier := helpers.PKCEVerifier()
				resp, err := enduser.AuthenticatedGET("/oauth2/authorize?"+url.Values{
					"response_type": {"code"}, "client_id": {agent.ID.String()},
					"redirect_uri": {agent.RedirectURIs[0]}, "state": {"refresh-consent"},
					"scope": {"read offline_access"}, "code_challenge": {helpers.GenerateCodeChallenge(verifier)},
					"code_challenge_method": {"S256"},
				}.Encode(), principal)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()
				Expect(resp.StatusCode).To(Equal(http.StatusFound))
				location, err := helpers.ExtractRedirectURL(resp)
				Expect(err).NotTo(HaveOccurred())
				code := location.Query().Get("code")
				Expect(code).NotTo(BeEmpty())
				status, body := postToken(url.Values{
					"grant_type": {"authorization_code"}, "code": {code},
					"redirect_uri": {agent.RedirectURIs[0]}, "code_verifier": {verifier},
				})
				Expect(status).To(Equal(http.StatusOK), "OAuth2 error: %v", body["error"])
				Expect(body["access_token"]).To(BeAssignableToTypeOf(""))
				Expect(body["access_token"]).NotTo(BeEmpty())
				Expect(body["refresh_token"]).To(BeAssignableToTypeOf(""))
				Expect(body["refresh_token"]).NotTo(BeEmpty())
				return body["refresh_token"].(string)
			}
			refresh := func(value string) (int, map[string]any) {
				return postToken(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {value}})
			}
			rotate := func(value string) string {
				status, body := refresh(value)
				Expect(status).To(Equal(http.StatusOK), "OAuth2 error: %v", body["error"])
				Expect(body["access_token"]).To(BeAssignableToTypeOf(""))
				Expect(body["access_token"]).NotTo(BeEmpty())
				Expect(body["refresh_token"]).To(BeAssignableToTypeOf(""))
				Expect(body["refresh_token"]).NotTo(BeEmpty())
				Expect(body["refresh_token"] == value).To(BeFalse(), "refresh must rotate the token")
				return body["refresh_token"].(string)
			}
			deleteAt := func(server *bootstrap.TestServer, path, user string) {
				resp, err := server.DirectRequest(http.MethodDelete, path, user, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()
				Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
			}
			revokeGrant := func() {
				deleteAt(enduser, "/api/consent/agents/"+agent.ID.String()+"/grants", principal)
			}
			expectDenied := func() {
				status, body := refresh(token)
				Expect(status).To(Equal(http.StatusBadRequest), "OAuth2 error: %v", body["error"])
				Expect(body["error"]).To(Equal("invalid_grant"))
				Expect(body).NotTo(HaveKey("access_token"))
				Expect(body).NotTo(HaveKey("refresh_token"))
				Expect(body).To(Equal(baseline))
			}
			startChain := func() {
				old := issue()
				rotate(old)
				status, body := refresh(old)
				Expect(status).To(Equal(http.StatusBadRequest))
				Expect(body["error"]).To(Equal("invalid_grant"))
				baseline = body
				token = rotate(issue())
			}

			BeforeEach(func() {
				secret, token, baseline = "", "", nil
				logger := bootstrap.TestLogger(slog.LevelInfo)
				var err error
				store, err = bootstrap.NewStorageFactory(logger).NewTestStorage()
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { Expect(store.Close(context.Background())).To(Succeed()) })
				ctx := context.Background()
				Expect(fixtures.SeedPlaceholderGrantData(ctx, store)).To(Succeed())
				agent = fixtures.LocalAgent()
				Expect(store.Agents().Create(ctx, agent)).To(Succeed())
				grant = fixtures.ActiveGrant(principal, agent.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"})
				Expect(store.UserGrants().Create(ctx, grant)).To(Succeed())
				Expect(store.UserSessions().Create(ctx, fixtures.SessionForService(principal, fixtures.PlaceholderServiceID.String()))).To(Succeed())
				config := fixtures.LocalConfig()
				if mode == "hybrid" {
					config = fixtures.HybridConfig(os.Getenv(e2eUpstreamBaseURLEnv))
				}
				application, err := bootstrap.NewServerFactory(config, logger).BuildApp(store)
				Expect(err).NotTo(HaveOccurred())
				admin, err = bootstrap.NewAdminTestServer(application, logger)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(admin.Close)
				enduser, err = bootstrap.NewEndUserTestServer(application, logger)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(enduser.Close)
				Expect(helpers.ProvisionSigningKey(admin.BaseURL())).To(Succeed())
			})

			Context("confidential client", func() {
				BeforeEach(func() {
					resp, err := admin.PublicPOST("/api/agents/"+agent.ID.String()+"/client-credentials", "application/json", nil)
					Expect(err).NotTo(HaveOccurred())
					defer func() { _ = resp.Body.Close() }()
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))
					var credentials map[string]any
					Expect(json.NewDecoder(resp.Body).Decode(&credentials)).To(Succeed())
					secret = credentials["client_secret"].(string)
					startChain()
				})

				// S1 from specs/049-fix-refresh-consent/spec.md.
				It("rejects refresh after grant deletion", func() {
					revokeGrant()
					expectDenied()
				})

				// S2 from specs/049-fix-refresh-consent/spec.md.
				It("does not resurrect the old chain after re-grant", func() {
					revokeGrant()
					payload := `{"granted_permission_sets":{"` + fixtures.PlaceholderPermissionSetID.String() + `":["` + fixtures.PlaceholderServiceID.String() + `"]}}`
					resp, err := enduser.AuthenticatedPOST("/api/consent/agents/"+agent.ID.String()+"/grants", principal, "application/json", strings.NewReader(payload))
					Expect(err).NotTo(HaveOccurred())
					defer func() { _ = resp.Body.Close() }()
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))
					expectDenied()
					rotate(issue())
				})

				// S3 from specs/049-fix-refresh-consent/spec.md.
				It("rejects refresh after grant expiration", func() {
					past := time.Now().Add(-time.Hour)
					grant.ValidUntil = &past
					Expect(store.UserGrants().Update(context.Background(), grant)).To(Succeed())
					expectDenied()
				})

				// S4 from specs/049-fix-refresh-consent/spec.md.
				It("rejects the old chain when credential deletion makes the client public", func() {
					deleteAt(admin, "/api/agents/"+agent.ID.String()+"/client-credentials", "")
					secret = ""
					expectDenied()
				})

				// S5 from specs/049-fix-refresh-consent/spec.md.
				It("cannot mint tokens after agent deletion", func() {
					deleteAt(admin, "/api/agents/"+agent.ID.String(), "")
					status, body := refresh(token)
					Expect(status).To(BeNumerically(">=", http.StatusBadRequest))
					Expect(body).NotTo(HaveKey("access_token"))
					Expect(body).NotTo(HaveKey("refresh_token"))
				})
			})

			Context("public client", func() {
				BeforeEach(startChain)

				// S6 from specs/049-fix-refresh-consent/spec.md.
				It("ends public-client refresh authority when consent is deleted", func() {
					revokeGrant()
					expectDenied()
				})
			})
		})
	}
})
