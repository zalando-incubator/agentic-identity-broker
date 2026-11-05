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
	)
	ctx := context.Background()

	BeforeEach(func() {
		var err error
		journey, err = helpers.NewOAuth2CredentialJourney()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(journey.Close)
		alice, bob = fixtures.DefaultPrincipal().String(), fixtures.AnotherPrincipal().String()
		journey.Upstream.WithExpiresIn(3600)
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

	authorizeAgent := func() *url.URL {
		GinkgoHelper()
		query := url.Values{
			"client_id": {agent.ID.String()}, "response_type": {"code"},
			"redirect_uri": {agent.RedirectURIs[0]}, "scope": {"openid profile"}, "state": {"reports-agent-state"},
			"code_challenge": {helpers.GenerateCodeChallenge(helpers.PKCEVerifier())}, "code_challenge_method": {"S256"},
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

		// AS-1.1 from specs/055-invalidate-client-sessions/spec.md
		It("denies an unexpired token after the client-ID file changes and returns the service recovery URI", func() {
			Expect(exchange(alice, http.StatusOK)).To(HaveKeyWithValue("access_token", "mock-access-token"))
			before := session(alice)
			providerCalls := len(journey.Upstream.GetTokenRequests())
			publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			expectDenied(alice)
			Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(providerCalls))
			Expect(session(alice)).To(Equal(before))
		})

		// AS-1.2 from specs/055-invalidate-client-sessions/spec.md
		It("requires each affected user to reconnect independently, including sessions without expiry", func() {
			connect(bob)
			unknownExpiry := session(bob)
			unknownExpiry.AccessTokenExpiresAt = nil
			Expect(journey.Storage.UserSessions().Create(ctx, unknownExpiry)).To(Succeed())
			publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			expectDenied(alice)
			expectDenied(bob)
			connect(alice)
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			expectDenied(bob)
			Expect(session(bob)).To(Equal(unknownExpiry))
		})

		// AS-1.3 from specs/055-invalidate-client-sessions/spec.md
		It("never refreshes a superseded session on demand, manually, or in the admin sweep", func() {
			before := session(alice)
			past := time.Now().Add(-time.Hour)
			before.AccessTokenExpiresAt = &past
			Expect(journey.Storage.UserSessions().Create(ctx, before)).To(Succeed())
			providerCalls := len(journey.Upstream.GetTokenRequests())
			publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			for _, request := range []string{`{"dry_run":true}`, `{"dry_run":false}`} {
				result := proactiveSweepBody(proactiveSweep(journey.Admin, request))
				Expect(result.TotalEvaluated).To(Equal(1))
				Expect(result.Failed).To(Equal(1))
				Expect(result.Refreshed).To(BeZero())
			}
			expectDenied(alice)
			journey.Principal = alice
			response, err := journey.Refresh(serviceID)
			Expect(err).NotTo(HaveOccurred())
			body, err := helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusConflict))
			Expect(body["error"]).To(Equal("refresh_unavailable"))
			Expect(session(alice)).To(Equal(before))
			Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(providerCalls))
		})

		Context("with an authorization code issued but not yet exchanged", func() {
			var callback *url.URL
			BeforeEach(func() {
				response, err := journey.Authorize(serviceID)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = response.Body.Close() }()
				Expect(response.StatusCode).To(Equal(http.StatusFound))
				callback, err = helpers.ProviderAuthorizationCallback(response.Header.Get("Location"))
				Expect(err).NotTo(HaveOccurred())
			})

			// AS-1.4 from specs/055-invalidate-client-sessions/spec.md
			It("rejects a callback that began under the previous identity and allows a fresh connection", func() {
				before := session(alice)
				providerCalls := len(journey.Upstream.GetTokenRequests())
				publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				response, err := journey.EndUser.AuthenticatedGET(callback.RequestURI(), alice)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = response.Body.Close() }()
				Expect(response.StatusCode).To(Equal(http.StatusFound))
				location, err := url.Parse(response.Header.Get("Location"))
				Expect(err).NotTo(HaveOccurred())
				Expect(location.Path).To(Equal("/sessions"))
				Expect(location.Query().Get("error")).To(Equal("callback_failed"))
				Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(providerCalls))
				Expect(session(alice)).To(Equal(before))
				connect(alice)
				Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			})
		})

		// AS-1.5 from specs/055-invalidate-client-sessions/spec.md
		It("keeps an invalidated session unusable after the previous client ID is restored", func() {
			publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			expectDenied(alice)
			publish(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
			expectDenied(alice)
			connect(alice)
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
		})

		// AS-2.1 from specs/055-invalidate-client-sessions/spec.md
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

		// AS-2.2 from specs/055-invalidate-client-sessions/spec.md
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
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
		})

		// AS-2.3 from specs/055-invalidate-client-sessions/spec.md
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

		// AS-2.4 from specs/055-invalidate-client-sessions/spec.md
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

		// AS-2.5 from specs/055-invalidate-client-sessions/spec.md
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

			// AS-3.1 from specs/055-invalidate-client-sessions/spec.md
			It("leaves Calendar, broker login, grants, rejection, and refreshability unchanged", func() {
				before, err := journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(alice), calendarID)
				Expect(err).NotTo(HaveOccurred())
				delegation := grant()
				Expect(calendarExchange(http.StatusOK)).To(HaveKey("access_token"))
				publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				expectDenied(alice)
				Expect(calendarExchange(http.StatusOK)).To(HaveKey("access_token"))
				after, err := journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(alice), calendarID)
				Expect(err).NotTo(HaveOccurred())
				Expect(after).To(Equal(before))
				Expect(grant()).To(Equal(delegation))
				response, err := journey.EndUser.AuthenticatedGET("/api/me", alice)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				Expect(response.Body.Close()).To(Succeed())
				now := time.Now()
				after.RefreshRejectedAt = &now
				Expect(journey.Storage.UserSessions().Create(ctx, after)).To(Succeed())
				Expect(calendarExchange(http.StatusBadRequest)).To(HaveKeyWithValue("error", "invalid_grant"))
				after.RefreshRejectedAt = nil
				past := time.Now().Add(-time.Hour)
				after.AccessTokenExpiresAt = &past
				Expect(journey.Storage.UserSessions().Create(ctx, after)).To(Succeed())
				Expect(calendarExchange(http.StatusOK)).To(HaveKey("access_token"))
				Expect(grant()).To(Equal(delegation))
			})

			// AS-3.3 from specs/055-invalidate-client-sessions/spec.md
			It("does not force consent for a changed optional service excluded from the grant", func() {
				delegation := grant()
				delegation.GrantedPermissionSets[0].IncludedServiceIDs = []id.ServiceID{calendarID}
				Expect(journey.Storage.UserGrants().Update(ctx, delegation)).To(Succeed())
				publish(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				Expect(authorizeAgent().Query().Get("code")).NotTo(BeEmpty())
				Expect(calendarExchange(http.StatusOK)).To(HaveKey("access_token"))
				delegation.GrantedPermissionSets[0].IncludedServiceIDs = append(delegation.GrantedPermissionSets[0].IncludedServiceIDs, serviceID)
				Expect(journey.Storage.UserGrants().Update(ctx, delegation)).To(Succeed())
				location := authorizeAgent()
				Expect(location.Path).To(Equal("/agents/" + agent.ID.String()))
				Expect(location.Query().Get("session_token")).NotTo(BeEmpty())
			})
		})

		// AS-3.2 from specs/055-invalidate-client-sessions/spec.md
		It("keeps sessions usable through filesystem secret rotation with the same client ID", func() {
			publish(fixtures.ReportsClientV1, fixtures.ReportsSecretV2)
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			response, err := journey.Refresh(serviceID)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = response.Body.Close() }()
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			requests := journey.Upstream.GetTokenRequests()
			clientID, secret, err := helpers.CredentialTokenRequest(requests[len(requests)-1])
			Expect(err).NotTo(HaveOccurred())
			Expect(clientID).To(Equal(fixtures.ReportsClientV1))
			Expect(secret).To(Equal(fixtures.ReportsSecretV2))
		})
	})

	Context("with stored Reports connections established under v1", func() {
		BeforeEach(func() {
			Expect(journey.Start(nil)).To(Succeed())
			var err error
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
					Expect(journey.Storage.UserSessions().Create(ctx, legacy)).To(Succeed())
				}
			})

			// AS-1.6 from specs/055-invalidate-client-sessions/spec.md
			It("keeps legacy sessions usable until the first later identity change without historical backfill", func() {
				for _, principal := range []string{alice, bob} {
					Expect(exchange(principal, http.StatusOK)).To(HaveKey("access_token"))
					Expect(session(principal).UpstreamClientID).To(BeNil())
				}
				updateStored(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
				expectDenied(alice)
				expectDenied(bob)
				updateStored(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
				expectDenied(alice)
				connect(alice)
				Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
				expectDenied(bob)
			})
		})

		// AS-1.7 from specs/055-invalidate-client-sessions/spec.md
		It("denies tokens for every user after an accepted Admin client_id change", func() {
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			Expect(exchange(bob, http.StatusOK)).To(HaveKey("access_token"))
			providerCalls := len(journey.Upstream.GetTokenRequests())
			updateStored(fixtures.ReportsClientV2, fixtures.ReportsSecretV2)
			expectDenied(alice)
			expectDenied(bob)
			Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(providerCalls))
			connect(alice)
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			expectDenied(bob)
		})

		// AS-3.4 from specs/055-invalidate-client-sessions/spec.md
		It("keeps sessions usable through Admin secret rotation and metadata updates", func() {
			updateStored(fixtures.ReportsClientV1, fixtures.ReportsSecretV2)
			metadata := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "stored")
			delete(metadata, "protected_resources")
			metadata["display_name"] = "Reports renamed"
			metadata["client_secret"] = fixtures.ReportsSecretV2
			updated, err := journey.AdminJSON(http.MethodPut, "/api/services/"+serviceID.String(), metadata)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.StatusCode).To(Equal(http.StatusOK))
			Expect(updated.Body.Close()).To(Succeed())
			Expect(exchange(alice, http.StatusOK)).To(HaveKey("access_token"))
			Expect(exchange(bob, http.StatusOK)).To(HaveKey("access_token"))
			response, err := journey.Refresh(serviceID)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = response.Body.Close() }()
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			requests := journey.Upstream.GetTokenRequests()
			clientID, secret, err := helpers.CredentialTokenRequest(requests[len(requests)-1])
			Expect(err).NotTo(HaveOccurred())
			Expect(clientID).To(Equal(fixtures.ReportsClientV1))
			Expect(secret).To(Equal(fixtures.ReportsSecretV2))
		})
	})
})
