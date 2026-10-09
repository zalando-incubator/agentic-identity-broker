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

		Context("and its refresh token remains valid", func() {
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
				Expect(fmt.Sprint(events[0])).NotTo(ContainSubstring(principal))
			})
		})

		Context("and both stored tokens have expired", func() {
			BeforeEach(func() { seedSession(fixtures.FullyExpiredSessionForPrincipal(principal, serviceID.String())) })

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
				Expect(fmt.Sprint(events[0])).NotTo(ContainSubstring(principal))
			})
		})

		Context("and the provider rejects its valid refresh token", func() {
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
				events := proactiveEvents(logs, "session.oauth2.refresh_failed")
				Expect(events).To(HaveLen(1))
				Expect(events[0]).To(HaveKeyWithValue("level", "ERROR"))
				Expect(events[0]).To(HaveKeyWithValue("session_id", after.ID.String()))
				Expect(events[0]).To(HaveKeyWithValue("triggered_by", "on-demand"))
				Expect(fmt.Sprint(events[0])).NotTo(ContainSubstring(principal))
				records, err := logs.Records()
				Expect(err).NotTo(HaveOccurred())
				for _, record := range records {
					if record["component"] != "oauth2session" {
						continue
					}
					Expect(record).NotTo(HaveKey("actor"))
					Expect(record).NotTo(HaveKey("calling_peer"))
				}
			})
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
			})
		})

		Context("and one candidate cannot use its expired refresh token", func() {
			var failed, succeeding string
			BeforeEach(func() {
				failed = fixtures.DefaultPrincipal().String()
				succeeding = fixtures.AnotherPrincipal().String()
				failedSession := fixtures.FullyExpiredSessionForPrincipal(failed, serviceID.String())
				failedSession.ID = id.MustParseSessionID("00000000-0000-0000-0000-000000000001")
				seedSession(failedSession)
				succeedingSession := fixtures.ExpiredGitHubSessionForPrincipal(succeeding)
				succeedingSession.ID = id.MustParseSessionID("00000000-0000-0000-0000-000000000002")
				seedSession(succeedingSession)
			})

			// US3-S2 from specs/029-proactive-token-refresh/spec.md
			It("should count expired refresh tokens as failures and continue the sweep", func() {
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
					Expect(after.RefreshTokenExpiresAt).To(Equal(before[principal].RefreshTokenExpiresAt))
					Expect(after.UpdatedAt).To(Equal(before[principal].UpdatedAt))
					Expect(after.AccessTokenExpiresAt).To(Equal(before[principal].AccessTokenExpiresAt))
				}
			})
		})
	})
})
