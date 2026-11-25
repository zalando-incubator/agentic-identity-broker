package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Client identity change", Label("client-identity-change"), func() {
	var (
		journey    *helpers.OAuth2CredentialJourney
		binding    map[string]string
		serviceID  id.ServiceID
		agent      *domainstorage.Agent
		alice, bob string
		verifier   string
	)
	ctx := context.Background()

	BeforeEach(func() {
		var err error
		journey, err = helpers.NewOAuth2CredentialJourney()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(journey.Close)
		alice, bob = fixtures.DefaultPrincipal().String(), fixtures.AnotherPrincipal().String()
		journey.Upstream.WithExpiresIn(3600)
		verifier = helpers.PKCEVerifier()
	})

	connectService := func(principal string, target id.ServiceID) {
		GinkgoHelper()
		journey.Principal = principal
		response, err := journey.Complete(target)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Query().Get("success")).To(Equal("true"))
		Expect(location.Query().Get("error")).To(BeEmpty())
	}

	connect := func(principal string) {
		GinkgoHelper()
		connectService(principal, serviceID)
	}

	session := func(principal string) *domainstorage.UserSession {
		GinkgoHelper()
		value, err := journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(value).NotTo(BeNil())
		return value
	}

	exchange := func(principal string, status int) map[string]any {
		GinkgoHelper()
		form, err := helpers.ClientIdentityExchangeForm(journey, agent.ID, principal)
		Expect(err).NotTo(HaveOccurred())
		return proactiveExchangeBody(proactiveExchange(journey.EndUser, form), status)
	}

	expectDenied := func(principal string) string {
		GinkgoHelper()
		form, err := helpers.ClientIdentityExchangeForm(journey, agent.ID, principal)
		Expect(err).NotTo(HaveOccurred())
		result := proactiveExchange(journey.EndUser, form)
		Expect(result.err).NotTo(HaveOccurred())
		Expect(result.response.Header.Get("WWW-Authenticate")).To(BeEmpty())
		body := proactiveExchangeBody(result, http.StatusBadRequest)
		Expect(body["error"]).To(Equal("invalid_grant"))
		Expect(body).NotTo(HaveKey("access_token"))
		Expect(body).NotTo(HaveKey("refresh_token"))
		recovery, ok := body["error_uri"].(string)
		Expect(ok).To(BeTrue())
		expected := journey.Config.Server.EndUser.PublicURL + "/api/third-party/" + serviceID.String() + "/oauth2/authorize?redirect_uri=" + url.QueryEscape(journey.Config.Server.EndUser.PublicURL+"/sessions")
		Expect(recovery).To(Equal(expected))
		return recovery
	}

	seedDelegations := func() {
		GinkgoHelper()
		agent = fixtures.LocalAgent()
		agent.ServiceRequirements = []domainstorage.ServiceRequirement{{ServiceID: serviceID, RequirementType: domainstorage.RequirementTypeOptional, RequiredScopes: []string{"profile"}}}
		Expect(journey.Storage.Agents().Create(ctx, agent)).To(Succeed())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, journey.Storage, serviceID)).To(Succeed())
		permissions, err := journey.Storage.PermissionSets().Get(ctx, fixtures.PlaceholderPermissionSetID)
		Expect(err).NotTo(HaveOccurred())
		for i := range permissions.ServiceScopes {
			if permissions.ServiceScopes[i].ServiceID == serviceID {
				permissions.ServiceScopes[i].Scopes = []string{"profile"}
			}
		}
		Expect(journey.Storage.PermissionSets().Update(ctx, permissions)).To(Succeed())
		for _, principal := range []string{alice, bob} {
			delegation := fixtures.ActiveGrant(principal, agent.ID.String(), serviceID.String(), []string{"profile"})
			delegation.GrantedPermissionSets[0].IncludedServiceIDs = []id.ServiceID{serviceID}
			Expect(journey.Storage.UserGrants().Create(ctx, delegation)).To(Succeed())
		}
	}

	publish := func(clientID, secret string) {
		GinkgoHelper()
		Expect(helpers.PublishCredentialPair(binding, clientID, secret)).To(Succeed())
	}

	updateStored := func(clientID, secret string) {
		GinkgoHelper()
		request := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "stored")
		delete(request, "protected_resources")
		request["client_id"], request["client_secret"] = clientID, secret
		response, err := journey.AdminJSON(http.MethodPut, "/api/services/"+serviceID.String(), request)
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK), "%v", body)
		Expect(body).NotTo(HaveKey("client_identity"))
		Expect(body).NotTo(HaveKey("client_identity_revision"))
	}

	putService := func(request map[string]any, headers map[string]string, status int) map[string]any {
		GinkgoHelper()
		if headers == nil || headers["If-Match"] == "" {
			delete(request, "protected_resources")
		}
		encoded, err := json.Marshal(request)
		Expect(err).NotTo(HaveOccurred())
		if headers == nil {
			headers = map[string]string{}
		}
		headers["Content-Type"] = "application/json"
		response, err := journey.Admin.DirectRequest(http.MethodPut, "/api/services/"+serviceID.String(), fixtures.AdminPrincipal().String(), headers, bytes.NewReader(encoded))
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(status), "%v", body)
		Expect(body).NotTo(HaveKey("client_identity"))
		Expect(body).NotTo(HaveKey("client_identity_revision"))
		return body
	}

	changeIdentity := func(source, clientID, secret string) {
		GinkgoHelper()
		if source == "filesystem" {
			publish(clientID, secret)
		} else {
			updateStored(clientID, secret)
		}
	}

	expectSessionFlags := func(target id.ServiceID, principal string, reauthentication, expired bool) {
		GinkgoHelper()
		for _, path := range []string{"/api/third-party/sessions", "/api/third-party/" + target.String() + "/session"} {
			response, err := journey.EndUser.AuthenticatedGET(path, principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			body, err := helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			data, ok := body["data"].(map[string]any)
			Expect(ok).To(BeTrue())
			var summary map[string]any
			if path == "/api/third-party/sessions" {
				list, ok := data["sessions"].([]any)
				Expect(ok).To(BeTrue())
				for _, entry := range list {
					candidate, ok := entry.(map[string]any)
					Expect(ok).To(BeTrue())
					if candidate["service_id"] == target.String() {
						summary = candidate
					}
				}
			} else {
				summary, ok = data["session"].(map[string]any)
				Expect(ok).To(BeTrue())
			}
			Expect(summary).NotTo(BeNil())
			Expect(summary).To(HaveKeyWithValue("requires_reauthentication", reauthentication))
			Expect(summary).To(HaveKeyWithValue("is_expired", reauthentication))
			Expect(summary).To(HaveKeyWithValue("access_token_expired", expired))
			Expect(summary).NotTo(HaveKey("upstream_client_id"))
			Expect(summary).NotTo(HaveKey("upstream_client_revision"))
		}
	}

	beginCallback := func() *url.URL {
		GinkgoHelper()
		journey.Principal = alice
		response, err := journey.Authorize(serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		callback, err := helpers.ProviderAuthorizationCallback(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Body.Close()).To(Succeed())
		return callback
	}

	expectFailedCallback := func(result proactiveExchangeResult) {
		GinkgoHelper()
		Expect(result.err).NotTo(HaveOccurred())
		defer func() { _ = result.response.Body.Close() }()
		Expect(result.response.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(result.response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Path).To(Equal("/sessions"))
		Expect(location.Query().Get("error")).To(Equal("callback_failed"))
		Expect(location.Query().Get("success")).To(BeEmpty())
	}

	authorizeAgent := func() *url.URL {
		GinkgoHelper()
		query := url.Values{
			"client_id": {agent.ID.String()}, "response_type": {"code"},
			"redirect_uri": {agent.RedirectURIs[0]}, "scope": {"openid profile"}, "state": {"reports-agent-state"},
			"code_challenge": {helpers.GenerateCodeChallenge(verifier)}, "code_challenge_method": {"S256"},
		}
		response, err := journey.EndUser.AuthenticatedGET("/oauth2/authorize?"+query.Encode(), alice)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		return location
	}

	grant := func() *domainstorage.UserGrant {
		GinkgoHelper()
		value, err := journey.Storage.UserGrants().FindByPrincipalAndAgent(ctx, id.Principal(alice), agent.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(value).NotTo(BeNil())
		return value
	}

	grantResponse := func() map[string]any {
		GinkgoHelper()
		response, err := journey.EndUser.AuthenticatedGET("/api/consent/agents/"+agent.ID.String()+"/grants", alice)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(body["data"]).To(HaveKeyWithValue("agent_id", agent.ID.String()))
		Expect(body["data"]).To(HaveKeyWithValue("principal", alice))
		return body
	}

	consentConnection := func() string {
		GinkgoHelper()
		response, err := journey.EndUser.AuthenticatedGET("/api/consent/agents/"+agent.ID.String(), alice)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		data, ok := body["data"].(map[string]any)
		Expect(ok).To(BeTrue())
		services, ok := data["services"].([]any)
		Expect(ok).To(BeTrue())
		for _, entry := range services {
			service, ok := entry.(map[string]any)
			Expect(ok).To(BeTrue())
			if service["serviceId"] == serviceID.String() {
				status, ok := service["connectionStatus"].(string)
				Expect(ok).To(BeTrue())
				return status
			}
		}
		Fail("Reports must appear in consent service requirements")
		return ""
	}

	Context("with a filesystem Reports connection established under v1", func() {
		BeforeEach(func() {
			var err error
			binding, err = journey.WritePair(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
			Expect(err).NotTo(HaveOccurred())
			document := fixtures.CredentialConfigurationDocument(journey.Upstream.URL(), map[string]map[string]string{"reports-prod": binding}, 8000, 14000)
			document["oauth2_authorization_server"].(map[string]any)["mode"] = "hybrid"
			Expect(journey.StartDocument(document)).To(Succeed())
			serviceID, _, err = journey.Register(fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "filesystem"))
			Expect(err).NotTo(HaveOccurred())
			seedDelegations()
			connect(alice)
		})

		// AS-1.1 from specs/056-invalidate-client-sessions/spec.md (User Story 1)
		It("denies an unexpired token after the client-ID file changes and returns the service recovery URI", func() {
			Expect(exchange(alice, http.StatusOK)).To(HaveKeyWithValue("access_token", "mock-access-token"))
			before := session(alice)
			providerCalls := len(journey.Upstream.GetTokenRequests())
			publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			expectDenied(alice)
			Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(providerCalls))
			Expect(session(alice)).To(Equal(before))
		})

		// AS-1.2 from specs/056-invalidate-client-sessions/spec.md (User Story 1)
		It("requires each affected user to reconnect independently, including sessions without expiry", func() {
			for _, source := range []string{"filesystem", "stored"} {
				By("invalidating both principals through " + source)
				changeIdentity(source, fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
				connect(alice)
				connect(bob)
				unknownExpiry := session(bob)
				unknownExpiry.AccessTokenExpiresAt = nil
				Expect(journey.Storage.UserSessions().Create(ctx, unknownExpiry)).To(Succeed())
				Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
				Expect(exchange(bob, http.StatusOK)).To(HaveKey("access_token"))
				providerCalls := journey.Upstream.TokenGrantCount("refresh_token")
				changeIdentity(source, fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				expectDenied(alice)
				expectDenied(bob)
				expectSessionFlags(serviceID, alice, true, false)
				expectSessionFlags(serviceID, bob, true, false)
				connect(alice)
				Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
				expectSessionFlags(serviceID, alice, false, false)
				expectDenied(bob)
				Expect(session(bob)).To(Equal(unknownExpiry))
				Expect(journey.Upstream.TokenGrantCount("refresh_token")).To(Equal(providerCalls))
			}
		})

		// AS-1.3 from specs/056-invalidate-client-sessions/spec.md (User Story 1)
		It("never refreshes a superseded session on demand, in the background, manually, or in the admin sweep", func() {
			for _, source := range []string{"filesystem", "stored"} {
				for _, untilExpiry := range []time.Duration{time.Minute, -time.Hour} {
					By("denying every refresh path for " + source + " with expiry " + untilExpiry.String())
					changeIdentity(source, fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
					connect(alice)
					before := session(alice)
					expiry := time.Now().Add(untilExpiry)
					before.AccessTokenExpiresAt = &expiry
					Expect(journey.Storage.UserSessions().Create(ctx, before)).To(Succeed())
					providerCalls := journey.Upstream.TokenGrantCount("refresh_token")
					changeIdentity(source, fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
					for index, request := range []string{`{"dry_run":true}`, `{"dry_run":false}`} {
						serviceBefore, err := journey.Storage.Services().Get(ctx, serviceID)
						Expect(err).NotTo(HaveOccurred())
						result := proactiveSweepBody(proactiveSweep(journey.Admin, request))
						Expect(result.DryRun).To(Equal(index == 0))
						Expect(result.TotalEvaluated).To(Equal(1))
						Expect(result.Failed).To(Equal(1))
						Expect(result.Refreshed).To(BeZero())
						Expect(result.Skipped).To(BeZero())
						Expect(session(alice)).To(Equal(before))
						if index == 0 {
							serviceAfter, err := journey.Storage.Services().Get(ctx, serviceID)
							Expect(err).NotTo(HaveOccurred())
							Expect(serviceAfter).To(Equal(serviceBefore))
						}
					}
					expectDenied(alice)
					journey.Principal = alice
					response, err := journey.Refresh(serviceID)
					Expect(err).NotTo(HaveOccurred())
					body, err := helpers.CredentialResponseObject(response)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusConflict))
					Expect(body["error"]).To(Equal("refresh_unavailable"))
					Expect(body).NotTo(HaveKey("access_token"))
					Expect(session(alice)).To(Equal(before))
					Consistently(func() int { return journey.Upstream.TokenGrantCount("refresh_token") }, 200*time.Millisecond).Should(Equal(providerCalls))
				}
			}
		})

		// AS-1.4 from specs/056-invalidate-client-sessions/spec.md (User Story 1)
		It("rejects delayed callbacks and refresh results from the previous identity and allows a fresh connection", func() {
			for _, source := range []string{"filesystem", "stored"} {
				By("rejecting a callback before provider exchange through " + source)
				changeIdentity(source, fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
				connect(alice)
				callback := beginCallback()
				before := session(alice)
				providerCalls := journey.Upstream.TokenGrantCount("authorization_code")
				changeIdentity(source, fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				response, err := journey.EndUser.AuthenticatedGET(callback.RequestURI(), alice)
				expectFailedCallback(proactiveExchangeResult{response: response, err: err})
				Expect(journey.Upstream.TokenGrantCount("authorization_code")).To(Equal(providerCalls))
				Expect(session(alice)).To(Equal(before))
				expectDenied(alice)
				connect(alice)
				Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))

				By("discarding an old code-exchange result after a newer reconnect through " + source)
				changeIdentity(source, fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
				connect(alice)
				callback = beginCallback()
				journey.Upstream.WithAccessToken("obsolete-callback-token")
				codeBarrier := journey.Upstream.GateTokenGrant("authorization_code")
				DeferCleanup(codeBarrier.Release)
				callbackResult := make(chan proactiveExchangeResult, 1)
				go func() {
					response, err := journey.EndUser.AuthenticatedGET(callback.RequestURI(), alice)
					callbackResult <- proactiveExchangeResult{response: response, err: err}
				}()
				Eventually(codeBarrier.Entered).Should(BeClosed())
				changeIdentity(source, fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				journey.Upstream.WithAccessToken("current-callback-token")
				connect(alice)
				current := session(alice)
				codeBarrier.Release()
				var delayed proactiveExchangeResult
				Eventually(callbackResult).Should(Receive(&delayed))
				expectFailedCallback(delayed)
				Expect(session(alice)).To(Equal(current))
				Expect(exchange(alice, http.StatusOK)).To(HaveKeyWithValue("access_token", "current-callback-token"))

				for _, outcome := range []string{"success", "invalid_grant"} {
					By("discarding a refresh " + outcome + " after the identity changes through " + source)
					changeIdentity(source, fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
					connect(alice)
					before = session(alice)
					journey.Upstream.WithAccessToken("obsolete-refresh-token")
					if outcome == "invalid_grant" {
						journey.Upstream.WithErrorResponse("invalid_grant")
					}
					refreshBarrier := journey.Upstream.GateTokenGrant("refresh_token")
					DeferCleanup(refreshBarrier.Release)
					refreshResult := make(chan proactiveExchangeResult, 1)
					go func() {
						response, err := journey.Refresh(serviceID)
						refreshResult <- proactiveExchangeResult{response: response, err: err}
					}()
					Eventually(refreshBarrier.Entered).Should(BeClosed())
					changeIdentity(source, fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
					refreshBarrier.Release()
					Eventually(refreshResult).Should(Receive(&delayed))
					body := proactiveExchangeBody(delayed, http.StatusConflict)
					Expect(body).To(HaveKeyWithValue("error", "refresh_unavailable"))
					Expect(body).NotTo(HaveKey("access_token"))
					Expect(body).NotTo(HaveKey("refresh_token"))
					Expect(session(alice)).To(Equal(before))
					expectDenied(alice)
					journey.Upstream.WithSuccessfulTokenResponse().WithAccessToken("current-refresh-reconnect-token")
					connect(alice)
					Expect(exchange(alice, http.StatusOK)).To(HaveKeyWithValue("access_token", "current-refresh-reconnect-token"))
				}
			}
		})

		// Supplemental AS-1.4 from specs/056-invalidate-client-sessions/spec.md (User Story 1).
		It("rejects pre-upgrade callback state missing identity or revision before any exchange at revision zero", func() {
			for _, source := range []string{"filesystem", "stored"} {
				if source == "stored" {
					updateStored(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
				}
				for _, missing := range [][]string{{"upstream_client_id"}, {"upstream_client_revision"}, {"upstream_client_id", "upstream_client_revision"}} {
					By("requiring explicit identity claims for " + source + " callback state")
					callback := beginCallback()
					before := session(alice)
					Expect(before.UpstreamClientRevision).To(BeZero())
					legacyCallback, err := helpers.RemoveClientIdentityStateClaims(journey, callback, missing...)
					Expect(err).NotTo(HaveOccurred())
					providerCalls := journey.Upstream.TokenGrantCount("authorization_code")
					response, err := journey.EndUser.AuthenticatedGET(legacyCallback.RequestURI(), alice)
					expectFailedCallback(proactiveExchangeResult{response: response, err: err})
					Expect(journey.Upstream.TokenGrantCount("authorization_code")).To(Equal(providerCalls))
					Expect(session(alice)).To(Equal(before))
					Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
					connect(alice)
					Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
				}
			}
		})

		// Supplemental AS-1.6 from specs/056-invalidate-client-sessions/spec.md (User Story 1).
		It("keeps identity-less filesystem and previously transitioned stored sessions rejected", func() {
			connect(bob)
			for _, principal := range []string{alice, bob} {
				legacy := session(principal)
				legacy.UpstreamClientID = nil
				legacy.UpstreamClientRevision = 0
				Expect(journey.Storage.UserSessions().Create(ctx, legacy)).To(Succeed())
			}
			providerCalls := len(journey.Upstream.GetTokenRequests())
			for _, source := range []string{"filesystem", "stored after a source transition"} {
				By("preserving the legacy rejection for " + source)
				if source != "filesystem" {
					updateStored(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
				}
				for _, principal := range []string{alice, bob} {
					before := session(principal)
					expectDenied(principal)
					journey.Principal = principal
					response, err := journey.Refresh(serviceID)
					Expect(err).NotTo(HaveOccurred())
					body, err := helpers.CredentialResponseObject(response)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusConflict))
					Expect(body).To(HaveKeyWithValue("error", "refresh_unavailable"))
					Expect(session(principal)).To(Equal(before))
				}
				Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(providerCalls))
			}
			connect(alice)
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			expectDenied(bob)
			Expect(session(bob).UpstreamClientID).To(BeNil())
		})

		// AS-1.5 from specs/056-invalidate-client-sessions/spec.md (User Story 1)
		It("keeps an invalidated session unusable after the previous client ID is restored", func() {
			for _, source := range []string{"filesystem", "stored"} {
				By("retaining invalidation across a revision-aware " + source + " rollback")
				changeIdentity(source, fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
				connect(alice)
				before := session(alice)
				providerCalls := journey.Upstream.TokenGrantCount("refresh_token")
				changeIdentity(source, fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				expectDenied(alice)
				changeIdentity(source, fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
				expectDenied(alice)
				journey.Principal = alice
				response, err := journey.Refresh(serviceID)
				Expect(err).NotTo(HaveOccurred())
				body, err := helpers.CredentialResponseObject(response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusConflict))
				Expect(body).To(HaveKeyWithValue("error", "refresh_unavailable"))
				Expect(journey.Upstream.TokenGrantCount("refresh_token")).To(Equal(providerCalls))
				Expect(session(alice)).To(Equal(before))
				connect(alice)
				Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			}
		})

		// AS-2.1 from specs/056-invalidate-client-sessions/spec.md (User Story 2)
		It("shows agent consent on the next authorization despite an existing grant", func() {
			before := grant()
			Expect(authorizeAgent().Query().Get("code")).NotTo(BeEmpty())
			publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			location := authorizeAgent()
			Expect(location.Path).To(Equal("/agents/" + agent.ID.String()))
			Expect(location.Query().Get("session_token")).NotTo(BeEmpty())
			Expect(consentConnection()).To(Equal("not_connected"))
			Expect(grant()).To(Equal(before))
		})

		// AS-2.2 from specs/056-invalidate-client-sessions/spec.md (User Story 2)
		It("reconnects from consent under the current identity and completes the original authorization", func() {
			publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			location := authorizeAgent()
			Expect(location.Path).To(Equal("/agents/" + agent.ID.String()))
			sessionToken := location.Query().Get("session_token")
			Expect(sessionToken).NotTo(BeEmpty())
			returnURL := journey.Config.Server.EndUser.PublicURL + location.Path
			const stateID = "990e8400-e29b-41d4-a716-446655440020"
			form := url.Values{"redirect_uri": {returnURL}, "consent_state_id": {stateID}}
			response, err := journey.EndUser.AuthenticatedPOST("/api/third-party/"+serviceID.String()+"/oauth2/authorize", alice, "application/x-www-form-urlencoded", bytes.NewBufferString(form.Encode()))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			callback, err := helpers.ProviderAuthorizationCallback(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.Close()).To(Succeed())
			response, err = journey.EndUser.AuthenticatedGET(callback.RequestURI(), alice)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			returned, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.Close()).To(Succeed())
			Expect(returned.Path).To(Equal(location.Path))
			Expect(returned.Query().Get("success")).To(Equal("true"))
			Expect(returned.Query().Get("consent_state_id")).To(Equal(stateID))
			Expect(consentConnection()).To(Equal("connected"))
			selection, err := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{fixtures.PlaceholderPermissionSetID.String(): {serviceID.String()}}})
			Expect(err).NotTo(HaveOccurred())
			response, err = journey.EndUser.AuthenticatedPOST("/api/consent/agents/"+agent.ID.String()+"/grants?session_token="+url.QueryEscape(sessionToken), alice, "application/json", bytes.NewReader(selection))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			body, err := helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			resume, ok := body["redirect_url"].(string)
			Expect(ok).To(BeTrue())
			resumeURL, err := url.Parse(resume)
			Expect(err).NotTo(HaveOccurred())
			response, err = journey.EndUser.AuthenticatedGET(resumeURL.RequestURI(), alice)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = response.Body.Close() }()
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			completed, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(completed.String()).To(HavePrefix(agent.RedirectURIs[0]))
			Expect(completed.Query().Get("code")).NotTo(BeEmpty())
			Expect(completed.Query().Get("state")).To(Equal("reports-agent-state"))
			tokenResponse := exchangeAuthorizationCode(journey.EndUser, agent.ID.String(), completed.Query().Get("code"), agent.RedirectURIs[0], verifier)
			tokens, err := helpers.CredentialResponseObject(tokenResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenResponse.StatusCode).To(Equal(http.StatusOK), "%v", tokens)
			Expect(tokens["token_type"]).To(Equal("Bearer"))
			Expect(tokens["access_token"]).To(BeAssignableToTypeOf(""))
			Expect(tokens["access_token"]).NotTo(BeEmpty())
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
		})

		// AS-2.3 from specs/056-invalidate-client-sessions/spec.md (User Story 2)
		It("recovers through the token error_uri and returns to sessions", func() {
			publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			recovery, err := url.Parse(expectDenied(alice))
			Expect(err).NotTo(HaveOccurred())
			response, err := journey.EndUser.AuthenticatedGET(recovery.RequestURI(), alice)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			callback, err := helpers.ProviderAuthorizationCallback(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.Close()).To(Succeed())
			response, err = journey.EndUser.AuthenticatedGET(callback.RequestURI(), alice)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = response.Body.Close() }()
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			location, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(location.Path).To(Equal("/sessions"))
			Expect(location.Query().Get("success")).To(Equal("true"))
			Expect(location.Query().Get("service_id")).To(Equal(serviceID.String()))
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
		})

		// AS-2.4 from specs/056-invalidate-client-sessions/spec.md (User Story 2)
		It("keeps the session unusable after a declined reconnect and allows retry without grant expansion", func() {
			before, delegation := session(alice), grant()
			publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			response, err := journey.Authorize(serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			authorization, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.Close()).To(Succeed())
			callback := "/api/third-party/" + serviceID.String() + "/oauth2/callback?" + url.Values{"state": {authorization.Query().Get("state")}, "error": {"access_denied"}}.Encode()
			providerCalls := len(journey.Upstream.GetTokenRequests())
			response, err = journey.EndUser.AuthenticatedGET(callback, alice)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			location, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.Close()).To(Succeed())
			Expect(location.Path).To(Equal("/sessions"))
			Expect(location.Query().Get("error")).NotTo(BeEmpty())
			Expect(session(alice)).To(Equal(before))
			Expect(grant()).To(Equal(delegation))
			Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(providerCalls))
			expectDenied(alice)
			journey.Upstream.WithErrorResponse("access_denied")
			response, err = journey.Complete(serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			failed, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.Close()).To(Succeed())
			Expect(failed.Path).To(Equal("/sessions"))
			Expect(failed.Query().Get("error")).To(Equal("callback_failed"))
			Expect(session(alice)).To(Equal(before))
			Expect(grant()).To(Equal(delegation))
			expectDenied(alice)
			journey.Upstream.WithSuccessfulTokenResponse()
			connect(alice)
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			Expect(grant()).To(Equal(delegation))
		})

		// AS-2.5 from specs/056-invalidate-client-sessions/spec.md (User Story 2)
		It("reports reauthentication on list and detail despite an unexpired token", func() {
			publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			for _, path := range []string{"/api/third-party/sessions", "/api/third-party/" + serviceID.String() + "/session"} {
				response, err := journey.EndUser.AuthenticatedGET(path, alice)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				body, err := helpers.CredentialResponseObject(response)
				Expect(err).NotTo(HaveOccurred())
				data, ok := body["data"].(map[string]any)
				Expect(ok).To(BeTrue())
				var summary any
				if path == "/api/third-party/sessions" {
					list, ok := data["sessions"].([]any)
					Expect(ok).To(BeTrue())
					Expect(list).To(HaveLen(1))
					summary = list[0]
				} else {
					summary = data["session"]
				}
				data, ok = summary.(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(data).To(HaveKeyWithValue("requires_reauthentication", true))
				Expect(data).To(HaveKeyWithValue("is_expired", true))
				Expect(data).To(HaveKeyWithValue("access_token_expired", false))
				Expect(data).NotTo(HaveKey("upstream_client_id"))
				Expect(data).NotTo(HaveKey("upstream_client_revision"))
			}
			Expect(consentConnection()).To(Equal("not_connected"))
		})

		Context("with a separately connected Calendar control service", func() {
			var calendarID id.ServiceID
			BeforeEach(func() {
				var err error
				calendarID, _, err = journey.Register(fixtures.ClientIdentityCalendarRequest(journey.Upstream.URL()))
				Expect(err).NotTo(HaveOccurred())
				connectService(alice, calendarID)
				agent.ServiceRequirements = append(agent.ServiceRequirements, domainstorage.ServiceRequirement{ServiceID: calendarID, RequiredScopes: []string{"profile"}, RequirementType: domainstorage.RequirementTypeOptional})
				Expect(journey.Storage.Agents().Update(ctx, agent)).To(Succeed())
				permissions, err := journey.Storage.PermissionSets().Get(ctx, fixtures.PlaceholderPermissionSetID)
				Expect(err).NotTo(HaveOccurred())
				permissions.ServiceScopes = append(permissions.ServiceScopes, domainstorage.ServiceScope{ServiceID: calendarID, Scopes: []string{"profile"}, RequirementType: domainstorage.RequirementTypeOptional})
				Expect(journey.Storage.PermissionSets().Update(ctx, permissions)).To(Succeed())
				delegation := grant()
				delegation.GrantedPermissionSets[0].IncludedServiceIDs = append(delegation.GrantedPermissionSets[0].IncludedServiceIDs, calendarID)
				Expect(journey.Storage.UserGrants().Update(ctx, delegation)).To(Succeed())
			})

			calendarExchange := func(status int) map[string]any {
				GinkgoHelper()
				form, err := helpers.ClientIdentityExchangeForm(journey, agent.ID, alice)
				Expect(err).NotTo(HaveOccurred())
				form.Set("resource", journey.Upstream.URL()+"/calendar-resource")
				result := proactiveExchange(journey.EndUser, form)
				Expect(result.err).NotTo(HaveOccurred())
				body, err := helpers.CredentialResponseObject(result.response)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.response.StatusCode).To(Equal(status), "%v", body)
				return body
			}

			// Supplemental AS-2.2 from specs/056-invalidate-client-sessions/spec.md; SR-002.
			It("requires mandatory Reports reconnection without selecting its optional permission set", func() {
				calendarSet := fixtures.ClientIdentityPermissionSet(calendarID, "Calendar Access")
				reportsSet := fixtures.ClientIdentityPermissionSet(serviceID, "Optional Reports Access")
				Expect(journey.Storage.PermissionSets().Create(ctx, calendarSet)).To(Succeed())
				Expect(journey.Storage.PermissionSets().Create(ctx, reportsSet)).To(Succeed())
				agent.PermissionSets = []domainstorage.AgentPermissionSetEntry{
					{PermissionSetID: calendarSet.ID, RequirementType: domainstorage.RequirementTypeMandatory},
					{PermissionSetID: reportsSet.ID, RequirementType: domainstorage.RequirementTypeOptional},
				}
				agent.ServiceRequirements = []domainstorage.ServiceRequirement{
					{ServiceID: serviceID, RequiredScopes: []string{"profile"}, RequirementType: domainstorage.RequirementTypeMandatory},
					{ServiceID: calendarID, RequiredScopes: []string{"profile"}, RequirementType: domainstorage.RequirementTypeOptional},
				}
				Expect(journey.Storage.Agents().Update(ctx, agent)).To(Succeed())
				validUntil := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
				selection, err := json.Marshal(map[string]any{
					"valid_until":             validUntil,
					"granted_permission_sets": map[string][]string{calendarSet.ID.String(): {calendarID.String()}},
				})
				Expect(err).NotTo(HaveOccurred())
				grantPath := "/api/consent/agents/" + agent.ID.String() + "/grants"
				response, err := journey.EndUser.AuthenticatedPOST(grantPath, alice, "application/json", bytes.NewReader(selection))
				Expect(err).NotTo(HaveOccurred())
				body, err := helpers.CredentialResponseObject(response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusCreated), "%v", body)
				before := grant()
				publicBefore := grantResponse()
				Expect(before.ValidUntil).To(Equal(&validUntil))
				Expect(before.GrantedPermissionSets).To(Equal([]domainstorage.GrantedPermissionSetEntry{
					{PermissionSetID: calendarSet.ID, IncludedServiceIDs: []id.ServiceID{calendarID}},
				}))

				By("rejecting the same Calendar-only selection while mandatory Reports is identity-invalid")
				publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				location := authorizeAgent()
				Expect(location.Path).To(Equal("/agents/" + agent.ID.String()))
				sessionToken := location.Query().Get("session_token")
				Expect(sessionToken).NotTo(BeEmpty())
				Expect(consentConnection()).To(Equal("not_connected"))
				calendarBefore, err := journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(alice), calendarID)
				Expect(err).NotTo(HaveOccurred())
				providerCalls := len(journey.Upstream.GetTokenRequests())
				submitPath := grantPath + "?session_token=" + url.QueryEscape(sessionToken)
				response, err = journey.EndUser.AuthenticatedPOST(submitPath, alice, "application/json", bytes.NewReader(selection))
				Expect(err).NotTo(HaveOccurred())
				body, err = helpers.CredentialResponseObject(response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusBadRequest), "%v", body)
				Expect(body).To(HaveKeyWithValue("error", "unconnected services"))
				Expect(grant()).To(Equal(before))
				Expect(grantResponse()).To(Equal(publicBefore))
				Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(providerCalls))

				By("reconnecting Reports and completing the original authorization with identical selections")
				connect(alice)
				Expect(consentConnection()).To(Equal("connected"))
				Expect(grant()).To(Equal(before))
				response, err = journey.EndUser.AuthenticatedPOST(submitPath, alice, "application/json", bytes.NewReader(selection))
				Expect(err).NotTo(HaveOccurred())
				body, err = helpers.CredentialResponseObject(response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusCreated), "%v", body)
				Expect(grant().GrantedPermissionSets).To(Equal(before.GrantedPermissionSets))
				Expect(grant().ValidUntil).To(Equal(before.ValidUntil))
				Expect(grantResponse()["data"]).To(HaveKeyWithValue("granted_permission_sets", map[string]any{
					calendarSet.ID.String(): []any{calendarID.String()},
				}))
				calendarAfter, err := journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(alice), calendarID)
				Expect(err).NotTo(HaveOccurred())
				Expect(calendarAfter).To(Equal(calendarBefore))
				resume, ok := body["redirect_url"].(string)
				Expect(ok).To(BeTrue())
				resumeURL, err := url.Parse(resume)
				Expect(err).NotTo(HaveOccurred())
				response, err = journey.EndUser.AuthenticatedGET(resumeURL.RequestURI(), alice)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusFound))
				completed, err := url.Parse(response.Header.Get("Location"))
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Body.Close()).To(Succeed())
				Expect(completed.String()).To(HavePrefix(agent.RedirectURIs[0]))
				Expect(completed.Query().Get("code")).NotTo(BeEmpty())
				Expect(completed.Query().Get("state")).To(Equal("reports-agent-state"))
				tokenResponse := exchangeAuthorizationCode(journey.EndUser, agent.ID.String(), completed.Query().Get("code"), agent.RedirectURIs[0], verifier)
				tokens, err := helpers.CredentialResponseObject(tokenResponse)
				Expect(err).NotTo(HaveOccurred())
				Expect(tokenResponse.StatusCode).To(Equal(http.StatusOK), "%v", tokens)
				Expect(tokens["access_token"]).NotTo(BeEmpty())
				Expect(calendarExchange(http.StatusOK)).To(HaveKey("access_token"))
				Expect(grant().GrantedPermissionSets).To(Equal(before.GrantedPermissionSets))
				Expect(grant().ValidUntil).To(Equal(before.ValidUntil))
			})

			// AS-3.1 from specs/056-invalidate-client-sessions/spec.md (User Story 3)
			It("leaves Calendar, broker login, grants, rejection, and refreshability unchanged", func() {
				delegation := grant()
				publicDelegation := grantResponse()
				for _, control := range []string{"usable", "recorded rejection", "expired and refreshable", "same provider client ID"} {
					By("retaining the Calendar outcome: " + control)
					publish(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
					connect(alice)
					if control == "same provider client ID" {
						request := fixtures.ClientIdentityCalendarRequest(journey.Upstream.URL())
						delete(request, "protected_resources")
						request["client_id"] = fixtures.ReportsClientV1
						response, err := journey.AdminJSON(http.MethodPut, "/api/services/"+calendarID.String(), request)
						Expect(err).NotTo(HaveOccurred())
						body, err := helpers.CredentialResponseObject(response)
						Expect(err).NotTo(HaveOccurred())
						Expect(response.StatusCode).To(Equal(http.StatusOK), "%v", body)
					}
					connectService(alice, calendarID)
					before, err := journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(alice), calendarID)
					Expect(err).NotTo(HaveOccurred())
					Expect(before).NotTo(BeNil())
					switch control {
					case "recorded rejection":
						expiry := time.Now().Add(time.Minute)
						before.AccessTokenExpiresAt = &expiry
						Expect(journey.Storage.UserSessions().Create(ctx, before)).To(Succeed())
						journey.Upstream.WithErrorResponse("invalid_grant")
						response, err := journey.Refresh(calendarID)
						Expect(err).NotTo(HaveOccurred())
						body, err := helpers.CredentialResponseObject(response)
						Expect(err).NotTo(HaveOccurred())
						Expect(response.StatusCode).To(Equal(http.StatusConflict))
						Expect(body).To(HaveKeyWithValue("error", "refresh_rejected"))
						journey.Upstream.WithSuccessfulTokenResponse()
						before, err = journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(alice), calendarID)
						Expect(err).NotTo(HaveOccurred())
						Expect(before.RefreshRejectedAt).NotTo(BeNil())
						expectSessionFlags(calendarID, alice, true, false)
						Expect(calendarExchange(http.StatusBadRequest)).To(HaveKeyWithValue("error", "invalid_grant"))
					case "expired and refreshable":
						expiry := time.Now().Add(-time.Hour)
						before.AccessTokenExpiresAt = &expiry
						Expect(journey.Storage.UserSessions().Create(ctx, before)).To(Succeed())
						expectSessionFlags(calendarID, alice, false, true)
					default:
						Expect(calendarExchange(http.StatusOK)).To(HaveKey("access_token"))
						expectSessionFlags(calendarID, alice, false, false)
					}
					providerCalls := journey.Upstream.TokenGrantCount("refresh_token")
					publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
					expectDenied(alice)
					switch control {
					case "recorded rejection":
						expectSessionFlags(calendarID, alice, true, false)
						body := calendarExchange(http.StatusBadRequest)
						Expect(body).To(HaveKeyWithValue("error", "invalid_grant"))
						Expect(body).NotTo(HaveKey("access_token"))
						Expect(body).NotTo(HaveKey("refresh_token"))
						Expect(body).To(HaveKeyWithValue("error_uri", journey.Config.Server.EndUser.PublicURL+"/api/third-party/"+calendarID.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape(journey.Config.Server.EndUser.PublicURL+"/sessions")))
						response, err := journey.Refresh(calendarID)
						Expect(err).NotTo(HaveOccurred())
						body, err = helpers.CredentialResponseObject(response)
						Expect(err).NotTo(HaveOccurred())
						Expect(response.StatusCode).To(Equal(http.StatusConflict))
						Expect(body).To(HaveKeyWithValue("error", "refresh_rejected"))
						for _, request := range []string{`{"dry_run":true}`, `{"dry_run":false}`} {
							result := proactiveSweepBody(proactiveSweep(journey.Admin, request))
							Expect(result.TotalEvaluated).To(BeZero())
							Expect(result.Failed).To(BeZero())
							Expect(result.Refreshed).To(BeZero())
						}
						Consistently(func() int { return journey.Upstream.TokenGrantCount("refresh_token") }, 200*time.Millisecond).Should(Equal(providerCalls))
					case "expired and refreshable":
						expectSessionFlags(calendarID, alice, false, true)
						Expect(calendarExchange(http.StatusOK)).To(HaveKeyWithValue("access_token", "mock-access-token"))
						Expect(journey.Upstream.TokenGrantCount("refresh_token")).To(Equal(providerCalls + 1))
						requests := journey.Upstream.GetTokenRequests()
						clientID, _, err := helpers.CredentialTokenRequest(requests[len(requests)-1])
						Expect(err).NotTo(HaveOccurred())
						Expect(clientID).To(Equal("calendar-client-v1"))
						expectSessionFlags(calendarID, alice, false, false)
					default:
						Expect(calendarExchange(http.StatusOK)).To(HaveKey("access_token"))
						expectSessionFlags(calendarID, alice, false, false)
						Expect(journey.Upstream.TokenGrantCount("refresh_token")).To(Equal(providerCalls))
					}
					after, err := journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(alice), calendarID)
					Expect(err).NotTo(HaveOccurred())
					if control != "expired and refreshable" {
						Expect(after).To(Equal(before))
					}
					Expect(after.ID).To(Equal(before.ID))
					Expect(after.UpstreamClientID).To(Equal(before.UpstreamClientID))
					Expect(after.UpstreamClientRevision).To(Equal(before.UpstreamClientRevision))
					Expect(grant()).To(Equal(delegation))
					Expect(grantResponse()).To(Equal(publicDelegation))
					response, err := journey.EndUser.AuthenticatedGET("/api/me", alice)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusOK))
					Expect(response.Body.Close()).To(Succeed())
				}
			})

			// AS-3.3 from specs/056-invalidate-client-sessions/spec.md (User Story 3)
			It("does not force consent for a changed optional service excluded from the grant", func() {
				delegation := grant()
				delegation.GrantedPermissionSets[0].IncludedServiceIDs = []id.ServiceID{calendarID}
				Expect(journey.Storage.UserGrants().Update(ctx, delegation)).To(Succeed())
				excludedGrant := grant()
				excludedPublicGrant := grantResponse()
				publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				Expect(authorizeAgent().Query().Get("code")).NotTo(BeEmpty())
				Expect(calendarExchange(http.StatusOK)).To(HaveKey("access_token"))
				Expect(grant()).To(Equal(excludedGrant))
				Expect(grantResponse()).To(Equal(excludedPublicGrant))
				expectSessionFlags(calendarID, alice, false, false)
				delegation.GrantedPermissionSets[0].IncludedServiceIDs = append(delegation.GrantedPermissionSets[0].IncludedServiceIDs, serviceID)
				Expect(journey.Storage.UserGrants().Update(ctx, delegation)).To(Succeed())
				includedGrant := grant()
				includedPublicGrant := grantResponse()
				location := authorizeAgent()
				Expect(location.Path).To(Equal("/agents/" + agent.ID.String()))
				Expect(location.Query().Get("session_token")).NotTo(BeEmpty())
				Expect(consentConnection()).To(Equal("not_connected"))
				Expect(grant()).To(Equal(includedGrant))
				Expect(grantResponse()).To(Equal(includedPublicGrant))
				connect(alice)
				Expect(authorizeAgent().Query().Get("code")).NotTo(BeEmpty())
				Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
				Expect(grant()).To(Equal(includedGrant))
				Expect(grantResponse()).To(Equal(includedPublicGrant))
			})
		})

		// AS-3.2 from specs/056-invalidate-client-sessions/spec.md (User Story 3)
		It("keeps sessions usable through filesystem secret rotation with the same client ID", func() {
			before := session(alice)
			providerCalls := journey.Upstream.TokenGrantCount("refresh_token")
			publish(fixtures.ReportsClientV1, fixtures.ReportsSecretV2)
			Expect(exchange(alice, http.StatusOK)).To(HaveKeyWithValue("access_token", "mock-access-token"))
			Expect(session(alice)).To(Equal(before))
			expectSessionFlags(serviceID, alice, false, false)
			journey.Upstream.WithAccessToken("rotated-secret-access-token")
			response, err := journey.Refresh(serviceID)
			Expect(err).NotTo(HaveOccurred())
			body, err := helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(body["data"]).To(HaveKeyWithValue("requires_reauthentication", false))
			Expect(body["data"]).To(HaveKeyWithValue("is_expired", false))
			Expect(journey.Upstream.TokenGrantCount("refresh_token")).To(Equal(providerCalls + 1))
			requests := journey.Upstream.GetTokenRequests()
			clientID, secret, err := helpers.CredentialTokenRequest(requests[len(requests)-1])
			Expect(err).NotTo(HaveOccurred())
			Expect(clientID).To(Equal(fixtures.ReportsClientV1))
			Expect(secret).To(Equal(fixtures.ReportsSecretV2))
			Expect(session(alice).UpstreamClientRevision).To(Equal(before.UpstreamClientRevision))
			Expect(exchange(alice, http.StatusOK)).To(HaveKeyWithValue("access_token", "rotated-secret-access-token"))

			By("preserving provider-credential failure instead of inventing identity recovery")
			journey.Upstream.WithErrorResponse("invalid_client")
			response, err = journey.Refresh(serviceID)
			Expect(err).NotTo(HaveOccurred())
			body, err = helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusBadGateway))
			Expect(body).To(HaveKeyWithValue("error", "refresh_failed"))
			Expect(body).NotTo(HaveKey("error_uri"))
			Expect(body).NotTo(HaveKey("access_token"))
			expectSessionFlags(serviceID, alice, false, false)
			Expect(exchange(alice, http.StatusOK)).To(HaveKeyWithValue("access_token", "rotated-secret-access-token"))
		})
	})

	Context("with stored Reports connections established under v1", func() {
		BeforeEach(func() {
			var err error
			binding, err = journey.WritePair(fixtures.ReportsClientV1, fixtures.ReportsSecretV2)
			Expect(err).NotTo(HaveOccurred())
			alternate, err := journey.WritePair(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
			Expect(err).NotTo(HaveOccurred())
			Expect(journey.Start(map[string]map[string]string{"reports-prod": binding, "reports-alternate": alternate})).To(Succeed())
			serviceID, _, err = journey.Register(fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "stored"))
			Expect(err).NotTo(HaveOccurred())
			seedDelegations()
			connect(alice)
			connect(bob)
			journey.Principal = alice
		})

		Context("with eligible identity-less legacy sessions", func() {
			BeforeEach(func() {
				for _, principal := range []string{alice, bob} {
					legacy := session(principal)
					legacy.UpstreamClientID = nil
					legacy.UpstreamClientRevision = 0
					if principal == bob {
						legacy.AccessTokenExpiresAt = nil
					}
					Expect(journey.Storage.UserSessions().Create(ctx, legacy)).To(Succeed())
				}
			})

			// AS-1.6 from specs/056-invalidate-client-sessions/spec.md (User Story 1)
			It("keeps legacy sessions usable until the first later identity change without historical backfill", func() {
				for _, principal := range []string{alice, bob} {
					Expect(exchange(principal, http.StatusOK)).To(HaveKey("access_token"))
					expectSessionFlags(serviceID, principal, false, false)
					journey.Principal = principal
					response, err := journey.Refresh(serviceID)
					Expect(err).NotTo(HaveOccurred())
					body, err := helpers.CredentialResponseObject(response)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusOK), "%v", body)
					Expect(session(principal).UpstreamClientID).To(BeNil())
					Expect(session(principal).UpstreamClientRevision).To(BeZero())
				}
				providerCalls := journey.Upstream.TokenGrantCount("refresh_token")
				for _, clientID := range []string{fixtures.ReportsClientV2, fixtures.ReportsClientV1} {
					By("retaining invalidation for legacy sessions at " + clientID)
					updateStored(clientID, fixtures.ReportsSecretV2)
					for _, principal := range []string{alice, bob} {
						before := session(principal)
						expectDenied(principal)
						journey.Principal = principal
						response, err := journey.Refresh(serviceID)
						Expect(err).NotTo(HaveOccurred())
						body, err := helpers.CredentialResponseObject(response)
						Expect(err).NotTo(HaveOccurred())
						Expect(response.StatusCode).To(Equal(http.StatusConflict))
						Expect(body).To(HaveKeyWithValue("error", "refresh_unavailable"))
						Expect(session(principal)).To(Equal(before))
					}
					Expect(journey.Upstream.TokenGrantCount("refresh_token")).To(Equal(providerCalls))
				}
				connect(alice)
				Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
				expectDenied(bob)
				Expect(session(bob).UpstreamClientID).To(BeNil())
			})
		})

		// AS-1.7 from specs/056-invalidate-client-sessions/spec.md (User Story 1)
		It("denies tokens for every user only after an accepted Admin client_id change", func() {
			beforeAlice, beforeBob := session(alice), session(bob)
			providerCalls := len(journey.Upstream.GetTokenRequests())
			response, err := journey.AdminJSON(http.MethodGet, "/api/services/"+serviceID.String(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			staleETag := response.Header.Get("ETag")
			Expect(staleETag).NotTo(BeEmpty())
			Expect(response.Body.Close()).To(Succeed())

			By("leaving both sessions usable after rejected and version-conflicting proposals")
			for _, invalid := range []string{"missing secret", "stale version"} {
				request := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "stored")
				request["client_id"] = fixtures.ReportsClientV2
				if invalid == "missing secret" {
					delete(request, "client_secret")
					putService(request, nil, http.StatusBadRequest)
				} else {
					metadata := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "stored")
					metadata["display_name"] = "Reports metadata without identity change"
					putService(metadata, map[string]string{"If-Match": staleETag}, http.StatusOK)
					putService(request, map[string]string{"If-Match": staleETag}, http.StatusPreconditionFailed)
				}
				for _, principal := range []string{alice, bob} {
					Expect(exchange(principal, http.StatusOK)).To(HaveKeyWithValue("access_token", "mock-access-token"))
					expectSessionFlags(serviceID, principal, false, false)
				}
				Expect(session(alice)).To(Equal(beforeAlice))
				Expect(session(bob)).To(Equal(beforeBob))
				response, err = journey.AdminJSON(http.MethodGet, "/api/services/"+serviceID.String(), nil)
				Expect(err).NotTo(HaveOccurred())
				body, err := helpers.CredentialResponseObject(response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				Expect(body).To(HaveKeyWithValue("client_id", fixtures.ReportsClientV1))
			}

			By("invalidating both principals from the accepted identity update")
			updateStored(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			expectDenied(alice)
			expectDenied(bob)
			expectSessionFlags(serviceID, alice, true, false)
			expectSessionFlags(serviceID, bob, true, false)
			Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(providerCalls))
			Expect(session(alice)).To(Equal(beforeAlice))
			Expect(session(bob)).To(Equal(beforeBob))
			connect(alice)
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			expectDenied(bob)
		})

		// AS-3.4 from specs/056-invalidate-client-sessions/spec.md (User Story 3)
		It("keeps sessions usable through Admin secret rotation, metadata, and same-identity source transitions", func() {
			beforeAlice, beforeBob, delegation := session(alice), session(bob), grant()
			publicDelegation := grantResponse()
			for _, operation := range []struct {
				name, source, canonicalID, secret string
			}{
				{"stored secret rotation", "stored", "reports-prod", fixtures.ReportsSecretV2},
				{"display name and scope metadata", "stored", "reports-prod", fixtures.ReportsSecretV2},
				{"stored to filesystem", "filesystem", "reports-prod", fixtures.ReportsSecretV2},
				{"same-identity canonical ID and binding", "filesystem", "reports-alternate", fixtures.ReportsSecretV1},
				{"filesystem to stored", "stored", "reports-alternate", fixtures.ReportsSecretV2},
				{"source round trip to filesystem", "filesystem", "reports-prod", fixtures.ReportsSecretV2},
				{"source round trip back to stored", "stored", "reports-prod", fixtures.ReportsSecretV2},
			} {
				By(operation.name)
				request := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), operation.source)
				request["canonical_id"] = operation.canonicalID
				if operation.source == "stored" {
					request["client_secret"] = operation.secret
				}
				if operation.name == "display name and scope metadata" {
					request["display_name"] = "Reports renamed"
					request["scopes"] = []map[string]string{{"scope_value": "profile", "description": "Existing Reports access"}, {"scope_value": "reports:write", "description": "Not delegated"}}
				}
				putService(request, nil, http.StatusOK)
				for _, principal := range []string{alice, bob} {
					Expect(exchange(principal, http.StatusOK)).To(HaveKey("access_token"))
					expectSessionFlags(serviceID, principal, false, false)
				}
				providerCalls := journey.Upstream.TokenGrantCount("refresh_token")
				journey.Principal = alice
				response, err := journey.Refresh(serviceID)
				Expect(err).NotTo(HaveOccurred())
				body, err := helpers.CredentialResponseObject(response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK), "%v", body)
				Expect(body["data"]).To(HaveKeyWithValue("requires_reauthentication", false))
				Expect(body["data"]).To(HaveKeyWithValue("is_expired", false))
				Expect(journey.Upstream.TokenGrantCount("refresh_token")).To(Equal(providerCalls + 1))
				requests := journey.Upstream.GetTokenRequests()
				clientID, secret, err := helpers.CredentialTokenRequest(requests[len(requests)-1])
				Expect(err).NotTo(HaveOccurred())
				Expect(clientID).To(Equal(fixtures.ReportsClientV1))
				Expect(secret).To(Equal(operation.secret))
				Expect(session(alice).ID).To(Equal(beforeAlice.ID))
				Expect(session(alice).UpstreamClientID).To(Equal(beforeAlice.UpstreamClientID))
				Expect(session(alice).UpstreamClientRevision).To(Equal(beforeAlice.UpstreamClientRevision))
				Expect(session(bob)).To(Equal(beforeBob))
				Expect(grant()).To(Equal(delegation))
				Expect(grantResponse()).To(Equal(publicDelegation))
			}

			By("retaining source-transition validation without invalidating existing sessions")
			invalid := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "filesystem")
			invalid["client_id"] = fixtures.ReportsClientV1
			putService(invalid, nil, http.StatusBadRequest)
			putService(fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "filesystem"), nil, http.StatusOK)
			invalid = fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "stored")
			delete(invalid, "client_secret")
			putService(invalid, nil, http.StatusBadRequest)
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			Expect(exchange(bob, http.StatusOK)).To(HaveKey("access_token"))
			Expect(session(bob)).To(Equal(beforeBob))
			Expect(grant()).To(Equal(delegation))
			Expect(grantResponse()).To(Equal(publicDelegation))
		})
	})

	Context("with an obsolete filesystem identity read paused before observation", func() {
		var (
			barrier   *bootstrap.ClientIdentityReadBarrier
			alternate map[string]string
		)

		BeforeEach(func() {
			var err error
			binding, err = journey.WritePair(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
			Expect(err).NotTo(HaveOccurred())
			alternate, err = journey.WritePair(fixtures.ReportsClientV1, fixtures.ReportsSecretV2)
			Expect(err).NotTo(HaveOccurred())
			barrier = bootstrap.NewClientIdentityReadBarrier()
			DeferCleanup(barrier.Release)
			document := fixtures.CredentialConfigurationDocument(journey.Upstream.URL(), map[string]map[string]string{"reports-prod": binding, "reports-alternate": alternate}, 8000, 14000)
			Expect(journey.StartBrowserDocument(document, barrier)).To(Succeed())
			serviceID, _, err = journey.Register(fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "filesystem"))
			Expect(err).NotTo(HaveOccurred())
			seedDelegations()
			connect(alice)
			connect(bob)
			journey.Principal = alice
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
		})

		for _, mutation := range []string{"filesystem to stored", "canonical ID and binding", "source round trip", "source round trip with a new canonical ID and binding"} {
			// Supplemental AS-3.4 from specs/056-invalidate-client-sessions/spec.md (User Story 3).
			It("discards the retired file value after "+mutation+" without a later false invalidation", func() {
				beforeAlice, beforeBob, delegation := session(alice), session(bob), grant()
				publicDelegation := grantResponse()
				oldReads := barrier.ReadCount(binding["client_id_file"])
				alternateReads := barrier.ReadCount(alternate["client_id_file"])
				providerCalls := len(journey.Upstream.GetTokenRequests())
				publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				barrier.Arm(binding["client_id_file"])
				form, err := helpers.ClientIdentityExchangeForm(journey, agent.ID, alice)
				Expect(err).NotTo(HaveOccurred())
				result := make(chan proactiveExchangeResult, 1)
				go func() { result <- proactiveExchange(journey.EndUser, form) }()
				Eventually(barrier.Entered).Should(BeClosed())
				Expect(barrier.ReadCount(binding["client_id_file"])).To(Equal(oldReads + 1))

				By("committing the administrative mutation without reading either credential binding")
				canonicalID := "reports-prod"
				if mutation == "canonical ID and binding" || mutation == "source round trip with a new canonical ID and binding" {
					canonicalID = "reports-alternate"
				}
				if mutation != "canonical ID and binding" {
					stored := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "stored")
					stored["canonical_id"] = canonicalID
					putService(stored, nil, http.StatusOK)
				}
				if mutation == "source round trip" {
					publish(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
				}
				if mutation != "filesystem to stored" {
					filesystem := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "filesystem")
					filesystem["canonical_id"] = canonicalID
					putService(filesystem, nil, http.StatusOK)
				}
				Expect(barrier.ReadCount(binding["client_id_file"])).To(Equal(oldReads + 1))
				Expect(barrier.ReadCount(alternate["client_id_file"])).To(Equal(alternateReads))
				barrier.Release()
				var completed proactiveExchangeResult
				Eventually(result).Should(Receive(&completed))
				Expect(proactiveExchangeBody(completed, http.StatusOK)).To(HaveKeyWithValue("access_token", "mock-access-token"))
				switch mutation {
				case "filesystem to stored":
					Expect(barrier.ReadCount(binding["client_id_file"])).To(Equal(oldReads + 1))
					Expect(barrier.ReadCount(alternate["client_id_file"])).To(Equal(alternateReads))
				case "source round trip":
					Expect(barrier.ReadCount(binding["client_id_file"])).To(BeNumerically(">=", oldReads+2))
				default:
					Expect(barrier.ReadCount(alternate["client_id_file"])).To(BeNumerically(">", alternateReads))
				}
				Expect(len(journey.Upstream.GetTokenRequests())).To(Equal(providerCalls))
				Expect(session(alice)).To(Equal(beforeAlice))
				Expect(session(bob)).To(Equal(beforeBob))
				expectSessionFlags(serviceID, alice, false, false)
				expectSessionFlags(serviceID, bob, false, false)

				By("performing a later same-ID stored secret rotation without a spurious revision change")
				updateStored(fixtures.ReportsClientV1, fixtures.ReportsSecretV2)
				Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
				Expect(exchange(bob, http.StatusOK)).To(HaveKey("access_token"))
				response, err := journey.Refresh(serviceID)
				Expect(err).NotTo(HaveOccurred())
				body, err := helpers.CredentialResponseObject(response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK), "%v", body)
				requests := journey.Upstream.GetTokenRequests()
				clientID, secret, err := helpers.CredentialTokenRequest(requests[len(requests)-1])
				Expect(err).NotTo(HaveOccurred())
				Expect(clientID).To(Equal(fixtures.ReportsClientV1))
				Expect(secret).To(Equal(fixtures.ReportsSecretV2))
				Expect(session(alice).UpstreamClientRevision).To(Equal(beforeAlice.UpstreamClientRevision))
				Expect(session(bob)).To(Equal(beforeBob))
				Expect(grant()).To(Equal(delegation))
				Expect(grantResponse()).To(Equal(publicDelegation))
			})
		}
	})
})
