package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

type proactiveExchangeResult struct {
	response *http.Response
	err      error
}

func proactiveExchangeForm(upstream *helpers.MockUpstreamOAuth2Server, principal string, agent *storagedomain.Agent) url.Values {
	now := time.Now()
	claims := map[string]interface{}{
		"sub": principal, "azp": agent.ID.String(), "iss": upstream.URL(),
		"aud": "token-exchange-broker", "exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
	}
	subject, err := helpers.SignTestJWT(claims, upstream.GetPrivateKeyPEM())
	Expect(err).NotTo(HaveOccurred())
	assertion, err := helpers.SignTestJWT(map[string]interface{}{
		"sub": "test-gateway-client", "iss": upstream.URL(), "aud": "token-exchange-broker",
		"exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
	}, upstream.GetPrivateKeyPEM())
	Expect(err).NotTo(HaveOccurred())
	return url.Values{
		"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":         {subject},
		"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {assertion},
		"resource":              {"https://api.github.com"},
	}
}

func proactiveExchange(server *bootstrap.TestServer, form url.Values) proactiveExchangeResult {
	response, err := server.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	return proactiveExchangeResult{response: response, err: err}
}

func proactiveExchangeBody(result proactiveExchangeResult, status int) map[string]any {
	ExpectWithOffset(1, result.err).NotTo(HaveOccurred())
	defer func() { _ = result.response.Body.Close() }()
	ExpectWithOffset(1, result.response.StatusCode).To(Equal(status))
	var body map[string]any
	ExpectWithOffset(1, json.NewDecoder(result.response.Body).Decode(&body)).To(Succeed())
	return body
}

func proactiveEvents(capture *bootstrap.BufferedLogCapture, event string) []map[string]any {
	records, err := capture.Records()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	var matches []map[string]any
	for _, record := range records {
		if record["event"] == event {
			matches = append(matches, record)
		}
	}
	return matches
}

type proactiveSweepSummary struct {
	Refreshed      int  `json:"refreshed"`
	Skipped        int  `json:"skipped"`
	Failed         int  `json:"failed"`
	TotalEvaluated int  `json:"total_evaluated"`
	DryRun         bool `json:"dry_run"`
}

func proactiveSweep(server *bootstrap.TestServer, body string) proactiveExchangeResult {
	response, err := server.AuthenticatedPOST("/api/sessions/sweep", fixtures.AdminPrincipal().String(), "application/json", strings.NewReader(body))
	return proactiveExchangeResult{response: response, err: err}
}

func proactiveSweepBody(result proactiveExchangeResult) proactiveSweepSummary {
	ExpectWithOffset(1, result.err).NotTo(HaveOccurred())
	defer func() { _ = result.response.Body.Close() }()
	ExpectWithOffset(1, result.response.StatusCode).To(Equal(http.StatusOK))
	var summary proactiveSweepSummary
	ExpectWithOffset(1, json.NewDecoder(result.response.Body).Decode(&summary)).To(Succeed())
	ExpectWithOffset(1, summary.Refreshed+summary.Skipped+summary.Failed).To(Equal(summary.TotalEvaluated))
	return summary
}

var _ = Describe("Proactive Token Refresh", func() {
	var (
		ctx          context.Context
		logger       *slog.Logger
		logs         *bootstrap.BufferedLogCapture
		config       *ports.Config
		storeFactory *bootstrap.StorageFactory
		store        *storageadapter.Adapter
		factory      *bootstrap.ServerFactory
		enduser      *bootstrap.TestServer
		admin        *bootstrap.TestServer
		otherAdmin   *bootstrap.TestServer
		upstream     *helpers.MockUpstreamOAuth2Server
		serviceID    id.ServiceID
		agent        *storagedomain.Agent
		release      func()
	)

	seedSession := func(session *storagedomain.UserSession) {
		Expect(store.UserSessions().Create(ctx, session)).To(Succeed())
	}
	storedSession := func(principal string) *storagedomain.UserSession {
		session, err := store.UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(session).NotTo(BeNil())
		return session
	}
	seedGrant := func(principal string) {
		Expect(store.UserGrants().Create(ctx, fixtures.ActiveGrant(principal, agent.ID.String(), serviceID.String(), []string{"repo", "user"}))).To(Succeed())
	}

	BeforeEach(func() {
		enduser, admin, otherAdmin = nil, nil, nil
		release = nil
		agent = nil
		ctx = context.Background()
		logger, logs = bootstrap.NewBufferedJSONLogger(slog.LevelDebug)
		upstream = helpers.NewMockUpstreamOAuth2Server()
		upstream.WithSuccessfulTokenResponse().WithAccessToken("renewed-github-token").WithRefreshToken("renewed-github-refresh")
		config = fixtures.OAuth2ConfigWithTokenExchange(upstream.URL())
		config.TokenRefresh.LookaheadDuration = 5 * time.Minute
		config.TokenRefresh.BackgroundWorkers = 10
		config.TokenRefresh.Sweep.DefaultPageSize = 100
		storeFactory = bootstrap.NewStorageFactory(logger)
		var err error
		store, err = storeFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())

		service := fixtures.GitHubService()
		service.Endpoints.TokenEndpoint = upstream.URL() + "/oauth/token"
		service.Endpoints.AuthorizeEndpoint = upstream.URL() + "/oauth/authorize"
		serviceID = service.ID
		Expect(store.Services().Create(ctx, service)).To(Succeed())
	})

	AfterEach(func() {
		if release != nil {
			release()
		}
		if otherAdmin != nil {
			otherAdmin.Close()
		}
		if enduser != nil {
			enduser.Close()
		}
		if admin != nil {
			admin.Close()
		}
		if upstream != nil {
			upstream.Close()
		}
		if store != nil {
			Expect(storeFactory.CloseStorage(store)).To(Succeed())
		}
	})

	Describe("User Story 1: when an agent exchanges an active session token", func() {
		BeforeEach(func() {
			config.TokenRefresh.BackgroundWorkers = 1
			Expect(fixtures.SeedPlaceholderGrantData(ctx, store, serviceID)).To(Succeed())
			agent = fixtures.ValidAgent()
			Expect(store.Agents().Create(ctx, agent)).To(Succeed())
			factory = bootstrap.NewServerFactory(config, logger)
			app, err := factory.BuildApp(store)
			Expect(err).NotTo(HaveOccurred())
			enduser, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("and the sole worker is occupied by another due session", func() {
			var first, second, healthy string
			BeforeEach(func() {
				first = fixtures.DefaultPrincipal().String()
				second = fixtures.AnotherPrincipal().String()
				healthy = fixtures.AdminPrincipal().String()
				for _, principal := range []string{first, second, healthy} {
					seedGrant(principal)
				}
				for _, principal := range []string{first, second} {
					session := fixtures.GitHubSessionForPrincipal(principal)
					expiry := time.Now().Add(2 * time.Minute)
					session.AccessTokenExpiresAt = &expiry
					seedSession(session)
				}
				seedSession(fixtures.GitHubSessionForPrincipal(healthy))
				release = upstream.WithTokenResponseGate()
			})

			// US1-S1 from specs/029-proactive-token-refresh/spec.md
			It("should drop a saturated refresh without delaying due or healthy exchanges", func() {
				Expect(proactiveExchangeBody(proactiveExchange(enduser, proactiveExchangeForm(upstream, first, agent)), http.StatusOK)["access_token"]).To(Equal("github-token-xyz"))
				Expect(proactiveExchangeBody(proactiveExchange(enduser, proactiveExchangeForm(upstream, second, agent)), http.StatusOK)["access_token"]).To(Equal("github-token-xyz"))
				Expect(proactiveExchangeBody(proactiveExchange(enduser, proactiveExchangeForm(upstream, healthy, agent)), http.StatusOK)["access_token"]).To(Equal("github-token-xyz"))
				Eventually(func() []map[string]any { return proactiveEvents(logs, "session.oauth2.proactive_refresh_dropped") }).Should(HaveLen(1))
				event := proactiveEvents(logs, "session.oauth2.proactive_refresh_dropped")[0]
				Expect(event).To(HaveKeyWithValue("level", "WARN"))
				Expect(event).To(HaveKeyWithValue("session_id", storedSession(second).ID.String()))
				Expect(event).To(HaveKeyWithValue("service_id", serviceID.String()))
				Eventually(func() int { return len(upstream.GetTokenRequests()) }).Should(Equal(1))
				Consistently(func() int { return len(upstream.GetTokenRequests()) }, 200*time.Millisecond).Should(Equal(1))
			})
		})

		Context("and a valid token expires within the lookahead window", func() {
			var principal string
			BeforeEach(func() {
				principal = fixtures.DefaultPrincipal().String()
				seedGrant(principal)
				session := fixtures.GitHubSessionForPrincipal(principal)
				expiry := time.Now().Add(2 * time.Minute)
				session.AccessTokenExpiresAt = &expiry
				seedSession(session)
				release = upstream.WithTokenResponseGate()
			})

			// US1-S2 from specs/029-proactive-token-refresh/spec.md
			It("should return the old token before committing background renewal", func() {
				original := storedSession(principal)
				oldExpiry := *original.AccessTokenExpiresAt
				Expect(proactiveExchangeBody(proactiveExchange(enduser, proactiveExchangeForm(upstream, principal, agent)), http.StatusOK)["access_token"]).To(Equal("github-token-xyz"))
				Eventually(func() int { return len(upstream.GetTokenRequests()) }).Should(Equal(1))
				Expect(*storedSession(principal).AccessTokenExpiresAt).To(Equal(oldExpiry))
				release()
				Eventually(func() bool {
					current := storedSession(principal)
					return current.AccessTokenExpiresAt.After(oldExpiry) && !bytes.Equal(current.EncryptedAccessToken, original.EncryptedAccessToken)
				}).Should(BeTrue())
				Expect(proactiveExchangeBody(proactiveExchange(enduser, proactiveExchangeForm(upstream, principal, agent)), http.StatusOK)["access_token"]).To(Equal("renewed-github-token"))
				Consistently(func() int { return len(upstream.GetTokenRequests()) }, 200*time.Millisecond).Should(Equal(1))
			})

			// US1-S3 from specs/029-proactive-token-refresh/spec.md
			It("should deduplicate concurrent exchanges during an in-flight refresh", func() {
				form := proactiveExchangeForm(upstream, principal, agent)
				first := make(chan proactiveExchangeResult, 1)
				go func() { first <- proactiveExchange(enduser, form) }()
				var initial proactiveExchangeResult
				Eventually(first).Should(Receive(&initial))
				Expect(proactiveExchangeBody(initial, http.StatusOK)["access_token"]).To(Equal("github-token-xyz"))
				Eventually(func() int { return len(upstream.GetTokenRequests()) }).Should(Equal(1))
				second := make(chan proactiveExchangeResult, 1)
				go func() { second <- proactiveExchange(enduser, form) }()
				var joined proactiveExchangeResult
				Eventually(second).Should(Receive(&joined))
				Expect(proactiveExchangeBody(joined, http.StatusOK)["access_token"]).To(Equal("github-token-xyz"))
				Consistently(func() int { return len(upstream.GetTokenRequests()) }, 200*time.Millisecond).Should(Equal(1))
				release()
				Eventually(func() bool {
					return storedSession(principal).AccessTokenExpiresAt.After(time.Now().Add(30 * time.Minute))
				}).Should(BeTrue())
				Expect(len(upstream.GetTokenRequests())).To(Equal(1))
			})
		})
	})

	Describe("User Story 2: when an agent exchanges an expired session token", func() {
		var principal string
		BeforeEach(func() {
			principal = fixtures.DefaultPrincipal().String()
			Expect(fixtures.SeedPlaceholderGrantData(ctx, store, serviceID)).To(Succeed())
			agent = fixtures.ValidAgent()
			Expect(store.Agents().Create(ctx, agent)).To(Succeed())
			seedGrant(principal)
			factory = bootstrap.NewServerFactory(config, logger)
			app, err := factory.BuildApp(store)
			Expect(err).NotTo(HaveOccurred())
			enduser, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("and the provider accepts its stored refresh token", func() {
			BeforeEach(func() { seedSession(fixtures.ExpiredGitHubSessionForPrincipal(principal)) })

			// US2-S1 from specs/029-proactive-token-refresh/spec.md
			It("should synchronously renew once and log success after persistence", func() {
				body := proactiveExchangeBody(proactiveExchange(enduser, proactiveExchangeForm(upstream, principal, agent)), http.StatusOK)
				Expect(body["access_token"]).To(Equal("renewed-github-token"))
				Expect(upstream.GetTokenRequests()).To(HaveLen(1))
				current := storedSession(principal)
				Expect(current.AccessTokenExpiresAt.After(time.Now().Add(30 * time.Minute))).To(BeTrue())
				events := proactiveEvents(logs, "session.oauth2.token_refreshed")
				Expect(events).To(HaveLen(1))
				Expect(events[0]).To(HaveKeyWithValue("level", "INFO"))
				Expect(events[0]).To(HaveKeyWithValue("session_id", current.ID.String()))
				Expect(events[0]).To(HaveKeyWithValue("triggered_by", "on-demand"))
				Expect(events[0]).To(HaveKeyWithValue("principal", principal))
			})
		})

		Context("and its expired access token has no refresh credentials", func() {
			BeforeEach(func() {
				session := fixtures.SessionWithoutRefreshToken(principal, serviceID.String())
				expiry := time.Now().Add(-2 * time.Hour)
				session.AccessTokenExpiresAt = &expiry
				seedSession(session)
			})

			// US2-S2 from specs/029-proactive-token-refresh/spec.md
			It("should require reauthentication without contacting the provider", func() {
				before := storedSession(principal)
				oldAccess := bytes.Clone(before.EncryptedAccessToken)
				oldRefresh := bytes.Clone(before.EncryptedRefreshToken)
				body := proactiveExchangeBody(proactiveExchange(enduser, proactiveExchangeForm(upstream, principal, agent)), http.StatusBadRequest)
				Expect(body).To(HaveKeyWithValue("error", "invalid_grant"))
				Expect(body).To(HaveKeyWithValue("error_uri", config.Server.EndUser.PublicURL+"/sessions"))
				Expect(body).NotTo(HaveKey("access_token"))
				Expect(upstream.GetTokenRequests()).To(BeEmpty())
				Expect(storedSession(principal).EncryptedAccessToken).To(Equal(oldAccess))
				Expect(storedSession(principal).EncryptedRefreshToken).To(Equal(oldRefresh))
				events := proactiveEvents(logs, "session.oauth2.refresh_failed")
				Expect(events).To(HaveLen(1))
				Expect(events[0]).To(HaveKeyWithValue("level", "ERROR"))
				Expect(events[0]).To(HaveKeyWithValue("session_id", storedSession(principal).ID.String()))
				Expect(events[0]).To(HaveKeyWithValue("triggered_by", "on-demand"))
				Expect(events[0]).To(HaveKeyWithValue("principal", principal))
			})
		})

		Context("and the provider rejects its stored refresh token", func() {
			BeforeEach(func() {
				seedSession(fixtures.ExpiredGitHubSessionForPrincipal(principal))
				upstream.WithErrorResponse("invalid_grant")
			})

			// US2-S3 from specs/029-proactive-token-refresh/spec.md
			It("should require reauthentication and preserve encrypted session tokens", func() {
				before := storedSession(principal)
				access, refresh := bytes.Clone(before.EncryptedAccessToken), bytes.Clone(before.EncryptedRefreshToken)
				expiry := *before.AccessTokenExpiresAt
				body := proactiveExchangeBody(proactiveExchange(enduser, proactiveExchangeForm(upstream, principal, agent)), http.StatusBadRequest)
				Expect(body).To(HaveKeyWithValue("error", "invalid_grant"))
				Expect(body).To(HaveKeyWithValue("error_uri", config.Server.EndUser.PublicURL+"/sessions"))
				Expect(body).NotTo(HaveKey("access_token"))
				Expect(upstream.GetTokenRequests()).To(HaveLen(1))
				after := storedSession(principal)
				Expect(after.EncryptedAccessToken).To(Equal(access))
				Expect(after.EncryptedRefreshToken).To(Equal(refresh))
				Expect(*after.AccessTokenExpiresAt).To(Equal(expiry))
				Expect(after.RefreshTokenRejected).To(BeTrue())
				Expect(after.UpdatedAt).To(BeTemporally(">", before.UpdatedAt))
				Expect(after.Scope).To(Equal(before.Scope))
				Expect(after.TokenType).To(Equal(before.TokenType))
				Expect(after.EncryptionContext).To(Equal(before.EncryptionContext))
				Expect(after.InitiatedAt).To(Equal(before.InitiatedAt))
				Expect(after.CreatedAt).To(Equal(before.CreatedAt))
				events := proactiveEvents(logs, "session.oauth2.refresh_failed")
				Expect(events).To(HaveLen(1))
				Expect(events[0]).To(HaveKeyWithValue("level", "ERROR"))
				Expect(events[0]).To(HaveKeyWithValue("session_id", after.ID.String()))
				Expect(events[0]).To(HaveKeyWithValue("triggered_by", "on-demand"))
				Expect(events[0]).To(HaveKeyWithValue("principal", principal))
			})
		})
	})

	Describe("Session recovery: when manual refresh rejects an unexpired access token", func() {
		var principal string
		BeforeEach(func() {
			principal = fixtures.DefaultPrincipal().String()
			Expect(fixtures.SeedPlaceholderGrantData(ctx, store, serviceID)).To(Succeed())
			agent = fixtures.ValidAgent()
			Expect(store.Agents().Create(ctx, agent)).To(Succeed())
			seedGrant(principal)
			seedSession(fixtures.GitHubSessionForPrincipal(principal))
			upstream.WithErrorResponse("invalid_grant")
			factory = bootstrap.NewServerFactory(config, logger)
			builder, err := bootstrap.NewTestServerBuilder(config, store, factory, logger)
			Expect(err).NotTo(HaveOccurred())
			enduser, err = builder.Build()
			Expect(err).NotTo(HaveOccurred())
			admin, err = bootstrap.NewAdminTestServer(enduser.App(), logger)
			Expect(err).NotTo(HaveOccurred())
		})

		// US1-S6 from specs/008-thirdparty-oauth2-sessions/spec.md and
		// US2-S3 from specs/029-proactive-token-refresh/spec.md.
		It("should stop token issuance and exclude sweeps until a real OAuth callback restores the session", func() {
			type sessionListResponse struct {
				Data struct {
					Sessions []map[string]any `json:"sessions"`
				} `json:"data"`
			}
			before := storedSession(principal)
			access, refresh := bytes.Clone(before.EncryptedAccessToken), bytes.Clone(before.EncryptedRefreshToken)
			expiry := *before.AccessTokenExpiresAt
			Expect(expiry.After(time.Now())).To(BeTrue())
			Expect(expiry.Before(time.Now().Add(2 * time.Hour))).To(BeTrue())

			manual, err := enduser.AuthenticatedPOST("/api/third-party/"+serviceID.String()+"/session/refresh", principal, "application/json", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(manual.StatusCode).To(Equal(http.StatusConflict))
			manualError := decodeJSON[map[string]any](manual)
			Expect(manualError).To(Equal(map[string]any{
				"error":   "refresh_rejected",
				"message": "refresh token rejected by provider; reauthentication required",
			}))
			after := storedSession(principal)
			Expect(after.ID).To(Equal(before.ID))
			Expect(after.RefreshTokenRejected).To(BeTrue())
			Expect(after.EncryptedAccessToken).To(Equal(access))
			Expect(after.EncryptedRefreshToken).To(Equal(refresh))
			Expect(*after.AccessTokenExpiresAt).To(Equal(expiry))

			listed, err := enduser.AuthenticatedGET("/api/third-party/sessions", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(listed.StatusCode).To(Equal(http.StatusOK))
			sessions := decodeJSON[sessionListResponse](listed).Data.Sessions
			Expect(sessions).To(HaveLen(1))
			Expect(sessions[0]).To(HaveKeyWithValue("id", before.ID.String()))
			Expect(sessions[0]).To(HaveKeyWithValue("service_id", serviceID.String()))
			Expect(sessions[0]).To(HaveKeyWithValue("requires_reauthentication", true))
			Expect(sessions[0]).To(HaveKeyWithValue("access_token_expired", false))
			Expect(sessions[0]).To(HaveKeyWithValue("has_refresh_token", true))
			Expect(sessions[0]).NotTo(HaveKey("is_expired"))
			Expect(sessions[0]).NotTo(HaveKey("refresh_token_expires_at"))
			type sessionDetailResponse struct {
				Data struct {
					Session map[string]any `json:"session"`
				} `json:"data"`
			}
			detail, err := enduser.AuthenticatedGET("/api/third-party/"+serviceID.String()+"/session", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(detail.StatusCode).To(Equal(http.StatusOK))
			status := decodeJSON[sessionDetailResponse](detail).Data.Session
			Expect(status).To(HaveKeyWithValue("requires_reauthentication", true))
			Expect(status).To(HaveKeyWithValue("access_token_expired", false))
			Expect(status).To(HaveKeyWithValue("has_refresh_token", true))
			Expect(status).NotTo(HaveKey("is_expired"))
			Expect(status).NotTo(HaveKey("refresh_token_expires_at"))
			_, _ = fmt.Fprintf(GinkgoWriter, "Manual refresh HTTP %d: %s; retained session requires_reauthentication=%v access_token_expired=%v has_refresh_token=%v, ciphertext and access expiry unchanged\n", manual.StatusCode, manualError["error"], sessions[0]["requires_reauthentication"], sessions[0]["access_token_expired"], sessions[0]["has_refresh_token"])

			denied := proactiveExchange(enduser, proactiveExchangeForm(upstream, principal, agent))
			deniedBody := proactiveExchangeBody(denied, http.StatusBadRequest)
			Expect(deniedBody).To(HaveKeyWithValue("error", "invalid_grant"))
			Expect(deniedBody).To(HaveKeyWithValue("error_uri", enduser.BaseURL()+"/sessions"))
			Expect(deniedBody).NotTo(HaveKey("access_token"))
			requests := upstream.GetTokenRequests()
			Expect(requests).To(HaveLen(1))
			refreshForm, err := url.ParseQuery(requests[0].Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(refreshForm.Get("grant_type")).To(Equal("refresh_token"))
			_, _ = fmt.Fprintf(GinkgoWriter, "Rejected exchange HTTP %d: %s, recovery=%s, no access token; upstream refresh requests=%d\n", denied.response.StatusCode, deniedBody["error"], deniedBody["error_uri"], len(requests))

			sweep := proactiveSweep(admin, `{"lookahead_duration":"PT2H","page_size":100}`)
			summary := proactiveSweepBody(sweep)
			Expect(summary).To(Equal(proactiveSweepSummary{}))
			Expect(upstream.GetTokenRequests()).To(HaveLen(1))
			_, _ = fmt.Fprintf(GinkgoWriter, "Admin sweep HTTP %d with PT2H: total_evaluated=%d refreshed=%d skipped=%d failed=%d; upstream refresh requests=%d\n", sweep.response.StatusCode, summary.TotalEvaluated, summary.Refreshed, summary.Skipped, summary.Failed, len(upstream.GetTokenRequests()))

			upstream.WithSuccessfulTokenResponse()
			authorize, err := enduser.AuthenticatedGET("/api/third-party/"+serviceID.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape(enduser.BaseURL()+"/sessions"), principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(authorize.StatusCode).To(Equal(http.StatusFound))
			authorizationURL, err := authorize.Location()
			Expect(err).NotTo(HaveOccurred())
			Expect(authorize.Body.Close()).To(Succeed())
			Expect(authorizationURL.Scheme + "://" + authorizationURL.Host).To(Equal(upstream.URL()))
			providerClient := &http.Client{
				Timeout: 5 * time.Second,
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
			provider, err := providerClient.Get(authorizationURL.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(provider.StatusCode).To(Equal(http.StatusFound))
			callbackURL, err := provider.Location()
			Expect(err).NotTo(HaveOccurred())
			Expect(provider.Body.Close()).To(Succeed())
			Expect(upstream.GetAuthorizeCalled()).To(BeTrue())
			Expect(upstream.GetLastRequest().Header.Get("X-Remote-User")).To(BeEmpty())
			Expect(callbackURL.Scheme + "://" + callbackURL.Host).To(Equal(enduser.BaseURL()))
			Expect(callbackURL.Path).To(Equal("/api/third-party/" + serviceID.String() + "/oauth2/callback"))
			Expect(callbackURL.Query().Get("code")).NotTo(BeEmpty())
			Expect(callbackURL.Query().Get("state")).To(Equal(authorizationURL.Query().Get("state")))

			callback, err := enduser.AuthenticatedGET(callbackURL.RequestURI(), principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(callback.StatusCode).To(Equal(http.StatusFound))
			recoveryURL, err := callback.Location()
			Expect(err).NotTo(HaveOccurred())
			Expect(callback.Body.Close()).To(Succeed())
			Expect(recoveryURL.Scheme + "://" + recoveryURL.Host).To(Equal(enduser.BaseURL()))
			Expect(recoveryURL.Path).To(Equal("/sessions"))
			Expect(recoveryURL.Query().Get("success")).To(Equal("true"))
			Expect(recoveryURL.Query().Get("service_id")).To(Equal(serviceID.String()))
			landing, err := enduser.AuthenticatedGET(recoveryURL.RequestURI(), principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(landing.StatusCode).To(Equal(http.StatusOK))
			Expect(landing.Body.Close()).To(Succeed())

			requests = upstream.GetTokenRequests()
			Expect(requests).To(HaveLen(2))
			codeForm, err := url.ParseQuery(requests[1].Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(codeForm.Get("grant_type")).To(Equal("authorization_code"))
			Expect(codeForm.Get("code")).To(Equal(callbackURL.Query().Get("code")))
			Expect(requests[1].Header.Get("X-Remote-User")).To(BeEmpty())
			recovered := storedSession(principal)
			Expect(recovered.ID).To(Equal(before.ID))
			Expect(recovered.RefreshTokenRejected).To(BeFalse())
			listed, err = enduser.AuthenticatedGET("/api/third-party/sessions", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(listed.StatusCode).To(Equal(http.StatusOK))
			sessions = decodeJSON[sessionListResponse](listed).Data.Sessions
			Expect(sessions).To(HaveLen(1))
			Expect(sessions[0]).To(HaveKeyWithValue("id", before.ID.String()))
			Expect(sessions[0]).To(HaveKeyWithValue("requires_reauthentication", false))
			Expect(sessions[0]).To(HaveKeyWithValue("access_token_expired", false))
			Expect(sessions[0]).To(HaveKeyWithValue("has_refresh_token", true))
			Expect(sessions[0]).NotTo(HaveKey("is_expired"))
			Expect(sessions[0]).NotTo(HaveKey("refresh_token_expires_at"))
			detail, err = enduser.AuthenticatedGET("/api/third-party/"+serviceID.String()+"/session", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(detail.StatusCode).To(Equal(http.StatusOK))
			status = decodeJSON[sessionDetailResponse](detail).Data.Session
			Expect(status).To(HaveKeyWithValue("requires_reauthentication", false))
			Expect(status).To(HaveKeyWithValue("access_token_expired", false))
			Expect(status).To(HaveKeyWithValue("has_refresh_token", true))
			Expect(status).NotTo(HaveKey("is_expired"))
			Expect(status).NotTo(HaveKey("refresh_token_expires_at"))
			exchanged := proactiveExchange(enduser, proactiveExchangeForm(upstream, principal, agent))
			Expect(proactiveExchangeBody(exchanged, http.StatusOK)).To(HaveKeyWithValue("access_token", "renewed-github-token"))
			Expect(upstream.GetTokenRequests()).To(HaveLen(2))
			_, _ = fmt.Fprintf(GinkgoWriter, "OAuth recovery authorize/provider/callback HTTP %d/%d/%d -> %s HTTP %d; authorization_code grants=1, requires_reauthentication=%v; subsequent exchange HTTP %d, total upstream requests=%d (one refresh, one authorization_code)\n", authorize.StatusCode, provider.StatusCode, callback.StatusCode, recoveryURL.Path, landing.StatusCode, sessions[0]["requires_reauthentication"], exchanged.response.StatusCode, len(upstream.GetTokenRequests()))
		})
	})

	Describe("User Story 3: when an operator sweeps expiring sessions", func() {
		BeforeEach(func() {
			factory = bootstrap.NewServerFactory(config, logger)
			app, err := factory.BuildApp(store)
			Expect(err).NotTo(HaveOccurred())
			admin, err = bootstrap.NewAdminTestServer(app, logger)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("and due candidates span multiple pages alongside healthy and timeless sessions", func() {
			var due, expired, healthy, timeless string
			BeforeEach(func() {
				due = fixtures.DefaultPrincipal().String()
				expired = fixtures.AnotherPrincipal().String()
				healthy = fixtures.AdminPrincipal().String()
				timeless = "timeless-session@example.com"
				soon := fixtures.GitHubSessionForPrincipal(due)
				expiry := time.Now().Add(2 * time.Minute)
				soon.AccessTokenExpiresAt = &expiry
				seedSession(soon)
				seedSession(fixtures.ExpiredGitHubSessionForPrincipal(expired))
				seedSession(fixtures.GitHubSessionForPrincipal(healthy))
				never := fixtures.GitHubSessionForPrincipal(timeless)
				never.AccessTokenExpiresAt = nil
				seedSession(never)
			})

			// US3-S1 from specs/029-proactive-token-refresh/spec.md
			It("should paginate only due sessions and exclude healthy and timeless rows", func() {
				oldDue := bytes.Clone(storedSession(due).EncryptedAccessToken)
				healthyExpiry := *storedSession(healthy).AccessTokenExpiresAt
				healthyRefresh := bytes.Clone(storedSession(healthy).EncryptedRefreshToken)
				timelessRefresh := bytes.Clone(storedSession(timeless).EncryptedRefreshToken)
				oldExpired := bytes.Clone(storedSession(expired).EncryptedAccessToken)
				oldHealthy := bytes.Clone(storedSession(healthy).EncryptedAccessToken)
				oldTimeless := bytes.Clone(storedSession(timeless).EncryptedAccessToken)
				summary := proactiveSweepBody(proactiveSweep(admin, `{"lookahead_duration":"PT10M","page_size":1}`))
				Expect(summary).To(Equal(proactiveSweepSummary{Refreshed: 2, TotalEvaluated: 2}))
				Expect(upstream.GetTokenRequests()).To(HaveLen(2))
				Expect(storedSession(due).EncryptedAccessToken).NotTo(Equal(oldDue))
				Expect(storedSession(expired).EncryptedAccessToken).NotTo(Equal(oldExpired))
				Expect(storedSession(healthy).EncryptedRefreshToken).To(Equal(healthyRefresh))
				Expect(*storedSession(healthy).AccessTokenExpiresAt).To(Equal(healthyExpiry))
				Expect(storedSession(timeless).EncryptedRefreshToken).To(Equal(timelessRefresh))
				Expect(storedSession(healthy).EncryptedAccessToken).To(Equal(oldHealthy))
				Expect(storedSession(timeless).EncryptedAccessToken).To(Equal(oldTimeless))
				Expect(storedSession(timeless).AccessTokenExpiresAt).To(BeNil())
				owners := map[string]string{storedSession(due).ID.String(): due, storedSession(expired).ID.String(): expired}
				refreshes := proactiveEvents(logs, "session.oauth2.token_refreshed")
				Expect(refreshes).To(HaveLen(2))
				for _, event := range refreshes {
					Expect(event).To(HaveKeyWithValue("principal", owners[event["session_id"].(string)]))
					Expect(event).To(HaveKeyWithValue("actor", fixtures.AdminPrincipal().String()))
				}
				records, err := logs.Records()
				Expect(err).NotTo(HaveOccurred())
				auditCount := 0
				for _, record := range records {
					if record["msg"] == "session sweep" {
						auditCount++
						Expect(record).To(HaveKeyWithValue("actor", fixtures.AdminPrincipal().String()))
						Expect(record).To(HaveKeyWithValue("operator_principal", fixtures.AdminPrincipal().String()))
						Expect(record).NotTo(HaveKey("principal"))
					}
				}
				Expect(auditCount).To(Equal(1))
			})
		})

		Context("and one candidate has no refresh credentials", func() {
			var failed, succeeding string
			BeforeEach(func() {
				failed = fixtures.DefaultPrincipal().String()
				succeeding = fixtures.AnotherPrincipal().String()
				failedSession := fixtures.SessionWithoutRefreshToken(failed, serviceID.String())
				expiry := time.Now().Add(-2 * time.Hour)
				failedSession.AccessTokenExpiresAt = &expiry
				failedSession.ID = id.MustParseSessionID("00000000-0000-0000-0000-000000000001")
				seedSession(failedSession)
				succeedingSession := fixtures.ExpiredGitHubSessionForPrincipal(succeeding)
				succeedingSession.ID = id.MustParseSessionID("00000000-0000-0000-0000-000000000002")
				seedSession(succeedingSession)
			})

			// US3-S2 from specs/029-proactive-token-refresh/spec.md
			It("should count missing refresh credentials as failures and continue the sweep", func() {
				oldFailedRefresh := bytes.Clone(storedSession(failed).EncryptedRefreshToken)
				oldFailedExpiry := *storedSession(failed).AccessTokenExpiresAt
				oldFailed := bytes.Clone(storedSession(failed).EncryptedAccessToken)
				oldSucceeding := bytes.Clone(storedSession(succeeding).EncryptedAccessToken)
				summary := proactiveSweepBody(proactiveSweep(admin, `{"lookahead_duration":"PT10M","page_size":1}`))
				Expect(summary).To(Equal(proactiveSweepSummary{Refreshed: 1, Failed: 1, TotalEvaluated: 2}))
				Expect(upstream.GetTokenRequests()).To(HaveLen(1))
				Expect(storedSession(failed).EncryptedRefreshToken).To(Equal(oldFailedRefresh))
				Expect(*storedSession(failed).AccessTokenExpiresAt).To(Equal(oldFailedExpiry))
				Expect(storedSession(failed).EncryptedAccessToken).To(Equal(oldFailed))
				Expect(storedSession(succeeding).EncryptedAccessToken).NotTo(Equal(oldSucceeding))
				second := proactiveSweepBody(proactiveSweep(admin, `{"lookahead_duration":"PT10M","page_size":1}`))
				Expect(second).To(Equal(proactiveSweepSummary{Failed: 1, TotalEvaluated: 1}))
				Expect(upstream.GetTokenRequests()).To(HaveLen(1))
				Expect(storedSession(failed).EncryptedRefreshToken).To(Equal(oldFailedRefresh))
				Expect(*storedSession(failed).AccessTokenExpiresAt).To(Equal(oldFailedExpiry))
				Expect(storedSession(failed).EncryptedAccessToken).To(Equal(oldFailed))
				failures := proactiveEvents(logs, "session.oauth2.refresh_failed")
				Expect(failures).To(HaveLen(2))
				for _, event := range failures {
					Expect(event).To(HaveKeyWithValue("principal", failed))
					Expect(event).To(HaveKeyWithValue("actor", fixtures.AdminPrincipal().String()))
				}
				refreshes := proactiveEvents(logs, "session.oauth2.token_refreshed")
				Expect(refreshes).To(HaveLen(1))
				Expect(refreshes[0]).To(HaveKeyWithValue("principal", succeeding))
				Expect(refreshes[0]).To(HaveKeyWithValue("actor", fixtures.AdminPrincipal().String()))
				records, err := logs.Records()
				Expect(err).NotTo(HaveOccurred())
				auditCount := 0
				for _, record := range records {
					if record["msg"] == "session sweep" {
						auditCount++
						Expect(record).To(HaveKeyWithValue("actor", fixtures.AdminPrincipal().String()))
						Expect(record).To(HaveKeyWithValue("operator_principal", fixtures.AdminPrincipal().String()))
						Expect(record).NotTo(HaveKey("principal"))
					}
				}
				Expect(auditCount).To(Equal(2))
			})
		})

		Context("and two replicas sweep the same shared storage concurrently", func() {
			var principal string
			BeforeEach(func() {
				principal = fixtures.DefaultPrincipal().String()
				seedSession(fixtures.ExpiredGitHubSessionForPrincipal(principal))
				otherConfig := fixtures.OAuth2ConfigWithTokenExchange(upstream.URL())
				otherConfig.TokenRefresh = config.TokenRefresh
				otherApp, err := bootstrap.NewServerFactory(otherConfig, logger).BuildApp(store)
				Expect(err).NotTo(HaveOccurred())
				otherAdmin, err = bootstrap.NewAdminTestServer(otherApp, logger)
				Expect(err).NotTo(HaveOccurred())
				// The exchange after both sweeps uses the first production App.
				Expect(fixtures.SeedPlaceholderGrantData(ctx, store, serviceID)).To(Succeed())
				agent = fixtures.ValidAgent()
				Expect(store.Agents().Create(ctx, agent)).To(Succeed())
				seedGrant(principal)
				enduser, err = bootstrap.NewEndUserTestServer(admin.App(), logger)
				Expect(err).NotTo(HaveOccurred())
				release = upstream.WithTokenResponseGate()
			})

			// US3-S3 from specs/029-proactive-token-refresh/spec.md
			It("should refresh a shared session only once across concurrent replicas", func() {
				// Check the route before the first replica waits on the provider gate.
				preview := proactiveSweepBody(proactiveSweep(admin, `{"lookahead_duration":"PT10M","dry_run":true}`))
				Expect(preview).To(Equal(proactiveSweepSummary{Refreshed: 1, TotalEvaluated: 1, DryRun: true}))
				original := bytes.Clone(storedSession(principal).EncryptedAccessToken)
				first, second := make(chan proactiveExchangeResult, 1), make(chan proactiveExchangeResult, 1)
				go func() { first <- proactiveSweep(admin, `{"lookahead_duration":"PT10M"}`) }()
				go func() { second <- proactiveSweep(otherAdmin, `{"lookahead_duration":"PT10M"}`) }()
				Eventually(func() int { return len(upstream.GetTokenRequests()) }).Should(Equal(1))
				release()
				var firstResult, secondResult proactiveExchangeResult
				Eventually(first).Should(Receive(&firstResult))
				Eventually(second).Should(Receive(&secondResult))
				one, two := proactiveSweepBody(firstResult), proactiveSweepBody(secondResult)
				Expect(one.Refreshed + two.Refreshed).To(Equal(1))
				Expect(upstream.GetTokenRequests()).To(HaveLen(1))
				Expect(storedSession(principal).EncryptedAccessToken).NotTo(Equal(original))
				body := proactiveExchangeBody(proactiveExchange(enduser, proactiveExchangeForm(upstream, principal, agent)), http.StatusOK)
				Expect(body["access_token"]).To(Equal("renewed-github-token"))
				Expect(upstream.GetTokenRequests()).To(HaveLen(1))
			})
		})

		Context("and dry run previews due candidates", func() {
			var first, second, healthy string
			BeforeEach(func() {
				first = fixtures.DefaultPrincipal().String()
				second = fixtures.AnotherPrincipal().String()
				healthy = fixtures.AdminPrincipal().String()
				for _, principal := range []string{first, second} {
					session := fixtures.GitHubSessionForPrincipal(principal)
					expiry := time.Now().Add(2 * time.Minute)
					session.AccessTokenExpiresAt = &expiry
					seedSession(session)
				}
				seedSession(fixtures.GitHubSessionForPrincipal(healthy))
			})

			// US3-S4 from specs/029-proactive-token-refresh/spec.md
			It("should count due sessions without provider calls or writes", func() {
				principals := []string{first, second, healthy}
				before := make(map[string]storagedomain.UserSession, len(principals))
				for _, principal := range principals {
					session := storedSession(principal)
					before[principal] = *session
				}
				summary := proactiveSweepBody(proactiveSweep(admin, `{"dry_run":true}`))
				Expect(summary).To(Equal(proactiveSweepSummary{Refreshed: 2, TotalEvaluated: 2, DryRun: true}))
				Expect(upstream.GetTokenRequests()).To(BeEmpty())
				for _, principal := range principals {
					after := storedSession(principal)
					Expect(after.EncryptedAccessToken).To(Equal(before[principal].EncryptedAccessToken), fmt.Sprintf("access token for %s", principal))
					Expect(after.EncryptedRefreshToken).To(Equal(before[principal].EncryptedRefreshToken))
					Expect(after.RefreshTokenRejected).To(Equal(before[principal].RefreshTokenRejected))
					Expect(after.UpdatedAt).To(Equal(before[principal].UpdatedAt))
					Expect(after.AccessTokenExpiresAt).To(Equal(before[principal].AccessTokenExpiresAt))
				}
			})
		})
	})
})
