package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	brokerconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
	integrationbootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type ledgerJourney struct {
	h               *bootstrap.LedgerHarness
	upstream        *helpers.MockUpstreamOAuth2Server
	issuer          *helpers.MockJWKSServer
	data            fixtures.LedgerWorkflowData
	auth            *helpers.ApprovalRequestAuthFixture
	logs            *bootstrap.BufferedLogCapture
	start           time.Time
	sensitive       []fixtures.CredentialCanaries
	actionBefore    []*model.BusinessEvent
	noSubjectBefore []*model.BusinessEvent
	expiresAt       time.Time
}

func newLedgerJourney(backend bootstrap.LedgerBackend, eventName string, faults *bootstrap.LedgerStorageFaults) *ledgerJourney {
	return newConfiguredLedgerJourney(backend, eventName, faults, nil, nil)
}

func newConfiguredLedgerJourney(backend bootstrap.LedgerBackend, eventName string, faults *bootstrap.LedgerStorageFaults, configure func(*ports.Config), schemas fs.FS, tracerProvider ...*sdktrace.TracerProvider) *ledgerJourney {
	upstream := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
	DeferCleanup(upstream.Close)
	issuer := helpers.NewMockJWKSServer()
	DeferCleanup(issuer.Close)
	config := fixtures.OAuth2ConfigWithTokenExchange(upstream.URL())
	if strings.HasPrefix(eventName, "agent-") || strings.HasPrefix(eventName, "credential-") || strings.HasPrefix(eventName, "signing-") || eventName == "token-issued" || eventName == "token-request-failed" || strings.HasPrefix(eventName, "impersonation-") {
		config = fixtures.LocalConfig()
	}
	config.BusinessEvents = ports.BusinessEventsConfig{Retention: 2160 * time.Hour, TelemetryCopyEnabled: false}
	config.Security.SkipThirdpartyHTTPSValidation = true
	if strings.HasPrefix(eventName, "impersonation-") {
		config.OAuth2AuthServer.Impersonation = &ports.ImpersonationConfig{AudiencePrefix: imperAudience, Rules: []ports.ImpersonationRuleConfig{imperSignedRule("ledger-gateway", issuer.URL(), issuer.JWKSURL(), `client_assertion.sub == "ledger-trusted-gateway"`)}}
	}
	if configure != nil {
		configure(config)
	}
	logger, logs := bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
	h, err := bootstrap.NewLedgerHarness(backend, config, logger, schemas, faults, tracerProvider...)
	Expect(err).NotTo(HaveOccurred())
	data := fixtures.LedgerWorkflowFixtures(fixtures.LedgerPrincipal())
	upstream.WithAccessToken(data.Canaries.AccessToken).WithRefreshToken(data.Canaries.RefreshToken)
	if eventName == "token-issued" || eventName == "token-request-failed" {
		data.Agent = fixtures.LocalAgent()
	}
	data.Service.IssuerURI = upstream.URL()
	data.Service.Endpoints.AuthorizeEndpoint = upstream.URL() + "/oauth/authorize"
	data.Service.Endpoints.TokenEndpoint = upstream.URL() + "/oauth/token"
	data.Service.Scopes = []model.OAuthScope{{ScopeValue: "read", Description: "Read access"}}
	data.Service.Secret = model.NewPlaintextSecret(data.Canaries.ClientSecret)
	Expect(h.App.ProviderService.Create(context.Background(), data.Service)).To(Succeed())
	Expect(fixtures.SeedPlaceholderGrantData(context.Background(), h.Storage, data.Service.ID)).To(Succeed())
	data.Agent.AllowedScopes = []string{"read"}
	Expect(h.Storage.Agents().Create(context.Background(), data.Agent)).To(Succeed())
	auth, err := helpers.NewApprovalRequestAuthFixture(upstream, data.Agent.ID)
	Expect(err).NotTo(HaveOccurred())
	return &ledgerJourney{h: h, upstream: upstream, issuer: issuer, data: data, auth: auth, logs: logs, start: time.Now().UTC(), sensitive: []fixtures.CredentialCanaries{data.Canaries}}
}

func (j *ledgerJourney) trackSecret(value string, assign func(*fixtures.CredentialCanaries, string)) {
	ExpectWithOffset(1, value).NotTo(BeEmpty())
	canaries := j.data.Canaries
	assign(&canaries, value)
	j.sensitive = append(j.sensitive, canaries)
}

func (j *ledgerJourney) assertCredentialFree(actual any) {
	for _, canaries := range j.sensitive {
		ExpectWithOffset(1, actual).To(matchers.BeFreeOfLedgerCredentials(canaries))
	}
}

func (j *ledgerJourney) captureTokenResponse(response *http.Response) {
	body := decodeJSON[map[string]any](response)
	access, ok := body["access_token"].(string)
	Expect(ok).To(BeTrue())
	j.trackSecret(access, func(c *fixtures.CredentialCanaries, value string) { c.AccessToken = value })
	if refresh, ok := body["refresh_token"].(string); ok {
		j.trackSecret(refresh, func(c *fixtures.CredentialCanaries, value string) { c.RefreshToken = value })
	}
	if jwt, ok := body["id_token"].(string); ok {
		j.trackSecret(jwt, func(c *fixtures.CredentialCanaries, value string) { c.RawJWT = value })
	}
}

func (j *ledgerJourney) createApproval(tool string, arguments map[string]any) helpers.CreateApprovalResponse {
	subject, err := j.auth.SubjectToken(j.data.Principal.String())
	Expect(err).NotTo(HaveOccurred())
	j.trackSecret(subject, func(c *fixtures.CredentialCanaries, value string) { c.RawJWT = value })
	j.trackSecret(j.auth.ClientAssertion, func(c *fixtures.CredentialCanaries, value string) { c.ClientAssertion = value })
	response, err := postJSONWithHeaders(j.h.EndUser, "/api/approvals", helpers.ApprovalCreateHeaders(subject, j.auth.ClientAssertion, j.data.Agent.ID), helpers.CreateApprovalRequest{Metadata: helpers.CreateApprovalMetadata{Description: "Execute " + tool}, ToolName: tool, Arguments: arguments})
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusCreated))
	return decodeJSON[helpers.CreateApprovalResponse](response)
}

func (j *ledgerJourney) consumeApproval(approvalID id.ApprovalID) (*http.Response, error) {
	subject, err := j.auth.SubjectToken(j.data.Principal.String())
	Expect(err).NotTo(HaveOccurred())
	j.trackSecret(subject, func(c *fixtures.CredentialCanaries, value string) { c.RawJWT = value })
	return postJSONWithHeaders(j.h.EndUser, "/api/approvals/"+approvalID.String()+"/consume", helpers.ApprovalSubjectTokenHeaders(subject), nil)
}

func ledgerStatus(response *http.Response, err error, expected int) {
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, response.StatusCode).To(Equal(expected))
	ExpectWithOffset(1, response.Body.Close()).To(Succeed())
}

func ledgerHasID(records []helpers.OTLPLogRecord, eventID id.BusinessEventID) bool {
	for _, record := range records {
		for _, attribute := range record.Record.Attributes {
			if attribute.Key == "id" && attribute.Value.GetStringValue() == eventID.String() {
				return true
			}
		}
	}
	return false
}

func (j *ledgerJourney) adminPost(path string, payload map[string]any) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		body = psJSON(payload)
	}
	return j.h.Admin.DirectRequest(http.MethodPost, path, "", map[string]string{"Content-Type": "application/json"}, body)
}

func (j *ledgerJourney) consentBody(until time.Time) map[string]any {
	return map[string]any{"granted_permission_sets": map[string][]string{fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()}}, "valid_until": until.UTC().Format(time.RFC3339Nano)}
}

func (j *ledgerJourney) seedGrant(expired bool) *domainstorage.UserGrant {
	grant := fixtures.ActiveGrant(j.data.Principal.String(), j.data.Agent.ID.String(), j.data.Service.ID.String(), []string{"read"})
	if expired {
		grant = fixtures.GrantExpiringIn(j.data.Principal.String(), j.data.Agent.ID.String(), j.data.Service.ID.String(), []string{"read"}, 500*time.Millisecond)
	}
	Expect(j.h.Storage.UserGrants().Create(context.Background(), grant)).To(Succeed())
	if expired {
		Eventually(func() bool { return time.Now().After(*grant.ValidUntil) }, 2*time.Second, 10*time.Millisecond).Should(BeTrue())
	}
	return grant
}

func (j *ledgerJourney) authorizationPath() string {
	request := helpers.CreateAuthorizationRequest(j.h.EndUser.BaseURL()+"/oauth2/authorize", j.data.Agent.ID.String(), j.data.Agent.RedirectURIs[0], "ledger-state")
	parsed, err := url.Parse(request)
	Expect(err).NotTo(HaveOccurred())
	return parsed.RequestURI()
}

func (j *ledgerJourney) establishSession() *domainstorage.UserSession {
	response, err := j.h.EndUser.AuthenticatedGET("/api/third-party/"+j.data.Service.ID.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape(j.h.EndUser.BaseURL()+"/done"), j.data.Principal.String())
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusFound))
	authorizeURL := response.Header.Get("Location")
	Expect(response.Body.Close()).To(Succeed())
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err = client.Get(authorizeURL)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusFound))
	callback, err := url.Parse(response.Header.Get("Location"))
	Expect(err).NotTo(HaveOccurred())
	callbackCode := callback.Query().Get("code")
	j.trackSecret(callbackCode, func(c *fixtures.CredentialCanaries, value string) { c.AuthorizationCode = value })
	Expect(response.Body.Close()).To(Succeed())
	response, err = j.h.EndUser.AuthenticatedGET(callback.RequestURI(), j.data.Principal.String())
	ledgerStatus(response, err, http.StatusFound)
	requests := j.upstream.GetTokenRequests()
	Expect(requests).NotTo(BeEmpty())
	form, err := url.ParseQuery(requests[len(requests)-1].Body)
	Expect(err).NotTo(HaveOccurred())
	Expect(form.Get("code") == callbackCode).To(BeTrue(), "the actual upstream authorization code must be exchanged")
	j.trackSecret(form.Get("code_verifier"), func(c *fixtures.CredentialCanaries, value string) { c.PKCEVerifier = value })
	session, err := j.h.Storage.UserSessions().FindByPrincipalAndService(context.Background(), j.data.Principal, j.data.Service.ID)
	Expect(err).NotTo(HaveOccurred())
	return session
}

func (j *ledgerJourney) exchangeForm() url.Values {
	now := time.Now()
	claims := map[string]any{"sub": j.data.Principal.String(), "azp": j.data.Agent.ID.String(), "iss": j.upstream.URL(), "aud": "token-exchange-broker", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}
	subject, err := helpers.SignTestJWT(claims, j.upstream.GetPrivateKeyPEM())
	Expect(err).NotTo(HaveOccurred())
	j.data.Canaries.RawJWT = subject
	j.trackSecret(subject, func(c *fixtures.CredentialCanaries, value string) { c.RawJWT = value })
	j.trackSecret(j.auth.ClientAssertion, func(c *fixtures.CredentialCanaries, value string) { c.ClientAssertion = value })
	j.data.Canaries.ClientAssertion = j.auth.ClientAssertion
	return url.Values{"grant_type": {imperGrant}, "subject_token": {subject}, "subject_token_type": {imperAccessTyp}, "requested_token_type": {imperAccessTyp}, "resource": {j.data.Service.ProtectedResources[0]}, "client_assertion_type": {imperBearerTyp}, "client_assertion": {j.auth.ClientAssertion}}
}

func (j *ledgerJourney) query(eventName string, subject model.BusinessEventSubject) []*model.BusinessEvent {
	start := j.start
	if strings.HasSuffix(eventName, "-expired") {
		start = start.Add(-24 * time.Hour)
	}
	events, err := j.h.App.LedgerService.Query(context.Background(), model.BusinessEventQuery{Subject: subject, Type: "agentic-identity-broker." + eventName, Start: start, End: time.Now().UTC().Add(time.Hour), Limit: 1000})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return events
}

func (j *ledgerJourney) queryAll(subject model.BusinessEventSubject) []*model.BusinessEvent {
	events, err := j.h.App.LedgerService.Query(context.Background(), model.BusinessEventQuery{Subject: subject, Start: j.start.Add(-24 * time.Hour), End: time.Now().UTC().Add(time.Hour), Limit: 1000})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return events
}

func (j *ledgerJourney) markAction(subject model.BusinessEventSubject) {
	j.actionBefore = j.queryAll(subject)
	j.noSubjectBefore = nil
	if !subject.NoSubject {
		j.noSubjectBefore = j.queryAll(model.BusinessEventSubject{NoSubject: true})
	}
}

func (j *ledgerJourney) assertActionDelta(subject model.BusinessEventSubject, expected map[string]int) {
	before := make(map[id.BusinessEventID]bool, len(j.actionBefore)+len(j.noSubjectBefore))
	for _, event := range j.actionBefore {
		before[event.ID] = true
	}
	for _, event := range j.noSubjectBefore {
		before[event.ID] = true
	}
	delta := map[string]int{}
	for _, event := range j.queryAll(subject) {
		if !before[event.ID] {
			delta[event.Type]++
		}
	}
	if !subject.NoSubject {
		for _, event := range j.queryAll(model.BusinessEventSubject{NoSubject: true}) {
			if !before[event.ID] {
				delta[event.Type]++
			}
		}
	}
	ExpectWithOffset(1, delta).To(Equal(expected), "the action must produce exactly the named facts, without forbidden companions")
}

func (j *ledgerJourney) actionEvents(expected matchers.BusinessEventExpectation) []*model.BusinessEvent {
	before := make(map[id.BusinessEventID]bool, len(j.actionBefore))
	for _, event := range j.actionBefore {
		before[event.ID] = true
	}
	var events []*model.BusinessEvent
	for _, event := range j.query(expected.Type[len("agentic-identity-broker."):], *expected.Subject) {
		if !before[event.ID] {
			events = append(events, event)
		}
	}
	return events
}

func (j *ledgerJourney) action(eventName string) matchers.BusinessEventExpectation {
	subject := model.BusinessEventSubject{Principal: j.data.Principal}
	expected := matchers.BusinessEventExpectation{Type: "agentic-identity-broker." + eventName, Subject: &subject, AgentID: &j.data.Agent.ID, Outcome: model.BusinessEventSuccess, Data: map[string]any{}}
	path := "/api/consent/agents/" + j.data.Agent.ID.String() + "/grants"
	ctx := context.Background()
	switch eventName {
	case "grant-created", "grant-updated":
		Expect(j.h.Storage.UserSessions().Create(ctx, fixtures.SessionForService(j.data.Principal.String(), fixtures.PlaceholderServiceID.String()))).To(Succeed())
		if eventName == "grant-updated" {
			j.seedGrant(false)
		}
		j.markAction(subject)
		expected.PermissionSetIDs = []id.PermissionSetID{fixtures.PlaceholderPermissionSetID}
		response, err := postJSON(j.h.EndUser, path, j.data.Principal.String(), j.consentBody(time.Now().Add(2*time.Hour)))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		body := decodeJSON[map[string]any](response)
		if data, ok := body["data"].(map[string]any); ok {
			body = data
		}
		grantID := id.MustParseGrantID(body["id"].(string))
		expected.GrantID = &grantID
	case "grant-revoked":
		grant := j.seedGrant(false)
		expected.GrantID = &grant.ID
		j.markAction(subject)
		expected.PermissionSetIDs = []id.PermissionSetID{fixtures.PlaceholderPermissionSetID}
		response, err := j.h.EndUser.DirectRequest(http.MethodDelete, path, j.data.Principal.String(), nil, nil)
		ledgerStatus(response, err, http.StatusNoContent)
	case "grant-expired":
		grant := j.seedGrant(true)
		expected.GrantID = &grant.ID
		expected.PermissionSetIDs = []id.PermissionSetID{fixtures.PlaceholderPermissionSetID}
		j.expiresAt = *grant.ValidUntil
		j.markAction(subject)
		response, err := j.h.EndUser.AuthenticatedGET(j.authorizationPath(), j.data.Principal.String())
		ledgerStatus(response, err, http.StatusFound)
	case "session-established", "session-refreshed", "session-refresh-failed", "session-terminated":
		if eventName == "session-established" {
			j.markAction(subject)
		}
		session := j.establishSession()
		expected.SessionID, expected.ServiceID, expected.AgentID = &session.ID, &j.data.Service.ID, nil
		if eventName != "session-established" {
			j.markAction(subject)
		}
		switch eventName {
		case "session-refreshed":
			j.upstream.WithAccessToken(j.data.Canaries.AccessToken + "-refreshed").WithRefreshToken(j.data.Canaries.RefreshToken + "-refreshed")
			j.trackSecret(j.data.Canaries.AccessToken+"-refreshed", func(c *fixtures.CredentialCanaries, value string) { c.AccessToken = value })
			j.trackSecret(j.data.Canaries.RefreshToken+"-refreshed", func(c *fixtures.CredentialCanaries, value string) { c.RefreshToken = value })
			response, err := j.h.EndUser.AuthenticatedPOST("/api/third-party/"+j.data.Service.ID.String()+"/session/refresh", j.data.Principal.String(), "application/json", nil)
			ledgerStatus(response, err, http.StatusOK)
		case "session-refresh-failed":
			j.upstream.WithErrorResponseAndDescription("invalid_grant", j.data.Canaries.AccessToken)
			response, err := j.h.EndUser.AuthenticatedPOST("/api/third-party/"+j.data.Service.ID.String()+"/session/refresh", j.data.Principal.String(), "application/json", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(BeNumerically(">=", 400))
			Expect(response.Body.Close()).To(Succeed())
			expected.Outcome, expected.Data = model.BusinessEventFailure, map[string]any{"reason_code": "upstream_rejected"}
		case "session-terminated":
			response, err := j.h.EndUser.DirectRequest(http.MethodDelete, "/api/third-party/"+j.data.Service.ID.String()+"/session", j.data.Principal.String(), nil, nil)
			ledgerStatus(response, err, http.StatusOK)
		}
	case "authorization-requested":
		j.markAction(subject)
		response, err := j.h.EndUser.AuthenticatedGET(j.authorizationPath(), j.data.Principal.String())
		ledgerStatus(response, err, http.StatusFound)
		expected.Outcome = model.BusinessEventPending
	case "token-issued", "token-request-failed":
		Expect(helpers.ProvisionSigningKey(j.h.Admin.BaseURL())).To(Succeed())
		response, err := j.adminPost("/api/agents/"+j.data.Agent.ID.String()+"/client-credentials", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		credentials := decodeJSON[map[string]any](response)
		secret := credentials["client_secret"].(string)
		j.data.Canaries.ClientSecret = secret
		j.trackSecret(secret, func(c *fixtures.CredentialCanaries, value string) { c.ClientSecret = value })
		if eventName == "token-request-failed" {
			secret = j.data.Canaries.PKCEVerifier
		}
		j.markAction(model.BusinessEventSubject{NoSubject: true})
		form := url.Values{"grant_type": {"client_credentials"}, "client_id": {j.data.Agent.ID.String()}, "client_secret": {secret}}
		response, err = j.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		Expect(err).NotTo(HaveOccurred())
		if eventName == "token-issued" {
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			j.captureTokenResponse(response)
		} else {
			Expect(response.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(response.Body.Close()).To(Succeed())
		}
		subject = model.BusinessEventSubject{NoSubject: true}
		if eventName == "token-request-failed" {
			expected.Outcome, expected.Data = model.BusinessEventFailure, map[string]any{"reason_code": "authentication_failed"}
		}
	case "token-exchanged", "token-exchange-denied":
		j.establishSession()
		if eventName == "token-exchanged" {
			j.seedGrant(false)
		}
		j.markAction(subject)
		response, err := j.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(j.exchangeForm().Encode()))
		Expect(err).NotTo(HaveOccurred())
		if eventName == "token-exchanged" {
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			j.captureTokenResponse(response)
		} else {
			Expect(response.StatusCode).To(Equal(http.StatusForbidden))
			expected.Outcome, expected.Data = model.BusinessEventDenied, map[string]any{"reason_code": "authorization_failed"}
			Expect(response.Body.Close()).To(Succeed())
		}
	case "impersonation-granted", "impersonation-denied":
		Expect(helpers.ProvisionSigningKey(j.h.Admin.BaseURL())).To(Succeed())
		if eventName == "impersonation-granted" {
			j.seedGrant(false)
		}
		sign := func(subject string) string {
			now := time.Now()
			token, err := j.issuer.SignJWT(map[string]any{"iss": j.issuer.URL(), "sub": subject, "aud": imperAudience, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
			Expect(err).NotTo(HaveOccurred())
			j.trackSecret(token, func(c *fixtures.CredentialCanaries, value string) { c.RawJWT = value })
			return token
		}
		form := url.Values{"grant_type": {imperGrant}, "audience": {imperAudience + "/" + j.data.Agent.ID.String()}, "client_assertion_type": {imperBearerTyp}, "client_assertion": {sign("ledger-trusted-gateway")}, "actor_token_type": {imperJWTType}, "actor_token": {sign("ledger-delegating-actor")}, "subject_token_type": {imperJWTType}, "subject_token": {sign(j.data.Principal.String())}}
		j.trackSecret(form.Get("client_assertion"), func(c *fixtures.CredentialCanaries, value string) { c.ClientAssertion = value })
		j.markAction(subject)
		response, err := j.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		Expect(err).NotTo(HaveOccurred())
		if eventName == "impersonation-granted" {
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			expected.Data = map[string]any{"delegating_actor_id": "ledger-delegating-actor"}
			j.captureTokenResponse(response)
		} else {
			Expect(response.StatusCode).To(Equal(http.StatusForbidden))
			expected.Outcome, expected.Data = model.BusinessEventDenied, map[string]any{"reason_code": "delegation_missing", "delegating_actor_id": "ledger-delegating-actor"}
			Expect(response.Body.Close()).To(Succeed())
		}
	case "approval-requested", "approval-approved", "approval-denied", "approval-consumed", "approval-revoked":
		if eventName == "approval-requested" {
			j.markAction(subject)
		}
		created := j.createApproval("ledger_tool", map[string]any{"canary": j.data.Canaries.AccessToken})
		approvalID := id.MustParseApprovalID(created.Data.ID)
		expected.ApprovalID = &approvalID
		expected.GatewayClientID = func() *id.ClientID { client := id.NewClientID("approval-gateway-client"); return &client }()
		if eventName != "approval-requested" {
			j.markAction(subject)
		}
		switch eventName {
		case "approval-requested":
			expected.Outcome = model.BusinessEventPending
		case "approval-approved", "approval-consumed", "approval-revoked":
			persistence := "once"
			if eventName == "approval-revoked" {
				persistence = "permanent"
			}
			response, err := postJSON(j.h.EndUser, "/api/approvals/"+approvalID.String()+"/approve", j.data.Principal.String(), helpers.ApproveRequest{Persistence: persistence})
			ledgerStatus(response, err, http.StatusOK)
			if eventName == "approval-consumed" || eventName == "approval-revoked" {
				j.markAction(subject)
			}
			if eventName == "approval-consumed" {
				response, err = j.consumeApproval(approvalID)
				ledgerStatus(response, err, http.StatusOK)
			}
			if eventName == "approval-revoked" {
				response, err = postJSON(j.h.EndUser, "/api/approvals/"+approvalID.String()+"/revoke", j.data.Principal.String(), nil)
				ledgerStatus(response, err, http.StatusOK)
			}
		case "approval-denied":
			response, err := postJSON(j.h.EndUser, "/api/approvals/"+approvalID.String()+"/deny", j.data.Principal.String(), nil)
			ledgerStatus(response, err, http.StatusOK)
			expected.Outcome, expected.Data = model.BusinessEventDenied, map[string]any{"reason_code": "user_denied"}
		}
	case "approval-expired":
		approval := &domainstorage.ToolApproval{ID: id.NewApprovalID(), Principal: j.data.Principal, AgentID: j.data.Agent.ID, ToolName: "ledger_expired", Arguments: map[string]any{}, ArgumentsHash: "ledger-expired-hash", Status: domainstorage.ApprovalStatusPending, CreatedAt: time.Now().UTC().Add(-2 * time.Hour), ExpiresAt: time.Now().UTC().Add(-time.Hour), ApprovalURL: "https://broker.example.test/approval"}
		Expect(domainapproval.ApplyExactPatterns(approval)).To(Succeed())
		_, err := j.h.Storage.ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())
		expected.ApprovalID = &approval.ID
		j.expiresAt = approval.ExpiresAt
		j.markAction(subject)
		response, err := j.h.EndUser.AuthenticatedGET("/api/approvals/"+approval.ID.String(), j.data.Principal.String())
		ledgerStatus(response, err, http.StatusGone)
	case "agent-registered", "agent-updated", "agent-deleted":
		body := map[string]any{"display_name": "Ledger Changed Agent", "description": "Catalogue acceptance agent", "permission_sets": fixtures.DefaultPermissionSets()}
		subject = model.BusinessEventSubject{NoSubject: true}
		j.markAction(subject)
		var response *http.Response
		var err error
		switch eventName {
		case "agent-registered":
			response, err = j.adminPost("/api/agents", body)
		case "agent-updated":
			response, err = j.h.Admin.DirectRequest(http.MethodPut, "/api/agents/"+j.data.Agent.ID.String(), "", map[string]string{"Content-Type": "application/json"}, psJSON(body))
		case "agent-deleted":
			response, err = j.h.Admin.DirectRequest(http.MethodDelete, "/api/agents/"+j.data.Agent.ID.String(), "", nil, nil)
		}
		Expect(err).NotTo(HaveOccurred())
		if eventName == "agent-deleted" {
			ledgerStatus(response, nil, http.StatusNoContent)
		} else {
			status := http.StatusOK
			if eventName == "agent-registered" {
				status = http.StatusCreated
			}
			Expect(response.StatusCode).To(Equal(status))
			result := decodeJSON[map[string]any](response)
			agentID := id.MustParseAgentID(result["id"].(string))
			expected.AgentID = &agentID
		}
	case "credential-generated", "credential-rotated", "credential-revoked":
		subject = model.BusinessEventSubject{NoSubject: true}
		credentialPath := "/api/agents/" + j.data.Agent.ID.String() + "/client-credentials"
		j.markAction(subject)
		response, err := j.adminPost(credentialPath, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		credentials := decodeJSON[map[string]any](response)
		j.data.Canaries.ClientSecret = credentials["client_secret"].(string)
		j.trackSecret(j.data.Canaries.ClientSecret, func(c *fixtures.CredentialCanaries, value string) { c.ClientSecret = value })
		if eventName != "credential-generated" {
			j.markAction(subject)
		}
		if eventName == "credential-rotated" {
			response, err = j.adminPost(credentialPath, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			credentials = decodeJSON[map[string]any](response)
			j.data.Canaries.ClientSecret = credentials["client_secret"].(string)
			j.trackSecret(j.data.Canaries.ClientSecret, func(c *fixtures.CredentialCanaries, value string) { c.ClientSecret = value })
		}
		credential, err := j.h.Storage.BrokerCredentials().GetByAgentID(ctx, j.data.Agent.ID)
		Expect(err).NotTo(HaveOccurred())
		expected.Data = map[string]any{"credential_id": credential.ID.String()}
		if eventName == "credential-revoked" {
			response, err = j.h.Admin.DirectRequest(http.MethodDelete, credentialPath, "", nil, nil)
			ledgerStatus(response, err, http.StatusNoContent)
		}
	case "signing-key-promoted":
		subject = model.BusinessEventSubject{NoSubject: true}
		expected.AgentID = nil
		operator := fixtures.LedgerAdminID()
		addKey := func() map[string]any {
			response, err := j.h.Admin.AuthenticatedPOST("/api/oauth2-server/signing-keys", operator, "application/json", psJSON(map[string]any{"algorithm": "ES256"}))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			return decodeJSON[map[string]any](response)
		}
		assertGenerated := func(created map[string]any) {
			key, err := j.h.Storage.SigningKeys().GetByKIDInDomain(ctx, domainstorage.KeyDomainTokenSigning, id.NewKeyID(created["kid"].(string)))
			Expect(err).NotTo(HaveOccurred())
			facts := j.actionEvents(matchers.BusinessEventExpectation{Type: expected.Type, Subject: &subject})
			Expect(facts).To(HaveLen(1), "selection during generation is a separate fact")
			Expect(facts[0]).To(matchers.HaveBusinessEventEnvelope(matchers.BusinessEventExpectation{Type: expected.Type, Subject: &subject, Actor: &model.BusinessEventActor{Kind: "admin", ID: &operator}, Outcome: model.BusinessEventSuccess, Data: map[string]any{"signing_key_id": key.ID.String(), "activates_at": key.ActivatesAt.UTC().Format(time.RFC3339Nano)}}))
			j.assertActionDelta(subject, map[string]int{expected.Type: 1})
		}
		j.markAction(subject)
		first := addKey()
		assertGenerated(first)
		j.markAction(subject)
		second := addKey()
		assertGenerated(second)
		Expect(second["kid"]).NotTo(Equal(first["kid"]))
		initial, err := j.h.Storage.SigningKeys().GetByKIDInDomain(ctx, domainstorage.KeyDomainTokenSigning, id.NewKeyID(first["kid"].(string)))
		Expect(err).NotTo(HaveOccurred())
		Expect(initial.IsCurrent).To(BeFalse(), "the PUT target must not already be current")
		j.markAction(subject)
		promote := func(kid string) map[string]any {
			response, err := j.h.Admin.DirectRequest(http.MethodPut, "/api/oauth2-server/signing-keys/"+kid+"/current", operator, map[string]string{"Content-Type": "application/json"}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			return decodeJSON[map[string]any](response)
		}
		selected := promote(first["kid"].(string))
		Expect(selected["is_current"]).To(BeTrue())
		storedKey, err := j.h.Storage.SigningKeys().GetByKIDInDomain(ctx, domainstorage.KeyDomainTokenSigning, id.NewKeyID(first["kid"].(string)))
		Expect(err).NotTo(HaveOccurred())
		Expect(storedKey.IsCurrent).To(BeTrue())
		expected.Data = map[string]any{"signing_key_id": storedKey.ID.String(), "activates_at": storedKey.ActivatesAt.UTC().Format(time.RFC3339Nano)}
		expected.Actor = &model.BusinessEventActor{Kind: "admin", ID: &operator}
		promoted := j.queryAll(subject)
		Expect(promote(first["kid"].(string))["is_current"]).To(BeTrue())
		Expect(j.queryAll(subject)).To(Equal(promoted), "promoting the already-current key must not add a fact")
	default:
		Fail("catalogue workflow is not mapped")
	}
	expected.Subject = &subject
	principal := j.data.Principal.String()
	switch eventName {
	case "grant-expired", "approval-expired":
		system := "broker-lifecycle"
		expected.Actor = &model.BusinessEventActor{Kind: "system", ID: &system}
	case "token-exchanged", "token-exchange-denied":
		caller := "approval-gateway-client"
		gateway := id.NewClientID(caller)
		expected.Actor = &model.BusinessEventActor{Kind: "gateway", ID: &caller}
		if eventName == "token-exchanged" {
			expected.Actor.OnBehalfOf = &j.data.Principal
		}
		expected.GatewayClientID, expected.ServiceID = &gateway, &j.data.Service.ID
	case "impersonation-granted", "impersonation-denied":
		caller := fixtures.LedgerGatewayClientID()
		expected.Actor = &model.BusinessEventActor{Kind: "gateway", ID: func() *string { value := caller.String(); return &value }()}
		if eventName == "impersonation-granted" {
			expected.Actor.OnBehalfOf = &j.data.Principal
		}
		expected.GatewayClientID = &caller
	case "approval-requested":
		caller := "approval-gateway-client"
		expected.Actor = &model.BusinessEventActor{Kind: "gateway", ID: &caller, OnBehalfOf: &j.data.Principal}
	case "approval-consumed":
		expected.Actor = &model.BusinessEventActor{Kind: "gateway", OnBehalfOf: &j.data.Principal}
	case "token-issued":
		caller := j.data.Agent.ID.String()
		expected.Actor = &model.BusinessEventActor{Kind: "agent", ID: &caller}
	case "token-request-failed":
		expected.Actor = &model.BusinessEventActor{Kind: "agent"}
	case "agent-registered", "agent-updated", "agent-deleted", "credential-generated", "credential-rotated", "credential-revoked":
		expected.Actor = &model.BusinessEventActor{Kind: "admin"}
	case "signing-key-promoted":
		// The authenticated signing-key operator is set by the promotion workflow.
	default:
		expected.Actor = &model.BusinessEventActor{Kind: "user", ID: &principal}
	}
	return expected
}

func (j *ledgerJourney) legacyAction(name, variant string) matchers.BusinessEventExpectation {
	if name == "grant-updated" {
		j.action("grant-created")
	}
	if name == "signing-key-promoted" {
		operator := fixtures.LedgerAdminID()
		var first map[string]any
		for range 2 {
			response, err := j.h.Admin.AuthenticatedPOST("/api/oauth2-server/signing-keys", operator, "application/json", psJSON(map[string]any{"algorithm": "ES256"}))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			created := decodeJSON[map[string]any](response)
			if first == nil {
				first = created
			}
		}
		subject := model.BusinessEventSubject{NoSubject: true}
		j.markAction(subject)
		response, err := j.h.Admin.DirectRequest(http.MethodPut, "/api/oauth2-server/signing-keys/"+first["kid"].(string)+"/current", operator, map[string]string{"Content-Type": "application/json"}, nil)
		ledgerStatus(response, err, http.StatusOK)
		return matchers.BusinessEventExpectation{Type: "agentic-identity-broker.signing-key-promoted", Subject: &subject}
	}
	if name == "token-exchange-denied" && variant == "default" {
		j.establishSession()
		subject := model.BusinessEventSubject{NoSubject: true}
		j.markAction(subject)
		form := j.exchangeForm()
		assertion, err := j.issuer.SignJWT(map[string]any{"iss": j.upstream.URL(), "sub": "approval-gateway-client", "aud": "token-exchange-broker", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()})
		Expect(err).NotTo(HaveOccurred())
		j.trackSecret(assertion, func(c *fixtures.CredentialCanaries, value string) { c.ClientAssertion = value })
		form.Set("client_assertion", assertion)
		response, err := j.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		ledgerStatus(response, err, http.StatusUnauthorized)
		return matchers.BusinessEventExpectation{Type: "agentic-identity-broker.token-exchange-denied", Subject: &subject}
	}
	if name == "authorization-requested" {
		return j.legacyAuthorizationCode("hybrid", name)
	}
	if name == "token-issued" && variant != "local-client-credentials" {
		return j.legacyAuthorizationCode(variant, name)
	}
	expected := j.action(name)
	if name == "session-refreshed" {
		j.markAction(*expected.Subject)
		response, err := j.h.EndUser.AuthenticatedPOST("/api/third-party/"+j.data.Service.ID.String()+"/session/refresh", j.data.Principal.String(), "application/json", nil)
		ledgerStatus(response, err, http.StatusOK)
	}
	return expected
}

func (j *ledgerJourney) legacyAuthorizationCode(variant, name string) matchers.BusinessEventExpectation {
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {j.data.Agent.ID.String()}, "redirect_uri": {j.data.Agent.RedirectURIs[0]}}
	subject := model.BusinessEventSubject{NoSubject: true}
	if variant == "proxy" {
		form.Set("code", j.data.Canaries.AuthorizationCode)
		form.Set("client_secret", j.data.Canaries.ClientSecret)
	} else {
		if variant == "hybrid" {
			j.data.Agent.ClientID = nil
			Expect(j.h.Storage.Agents().Update(context.Background(), j.data.Agent)).To(Succeed())
			Expect(helpers.ProvisionSigningKey(j.h.Admin.BaseURL())).To(Succeed())
			response, err := j.adminPost("/api/agents/"+j.data.Agent.ID.String()+"/client-credentials", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			secret := decodeJSON[map[string]any](response)["client_secret"].(string)
			j.trackSecret(secret, func(c *fixtures.CredentialCanaries, value string) { c.ClientSecret = value })
			form.Set("client_secret", secret)
			subject = model.BusinessEventSubject{Principal: j.data.Principal}
		}
		verifier := helpers.PKCEVerifier()
		j.trackSecret(verifier, func(c *fixtures.CredentialCanaries, value string) { c.PKCEVerifier = value })
		form.Set("code_verifier", verifier)
		authorize, err := url.Parse(j.authorizationPath())
		Expect(err).NotTo(HaveOccurred())
		query := authorize.Query()
		query.Set("code_challenge", helpers.GenerateCodeChallenge(verifier))
		query.Set("code_challenge_method", "S256")
		authorize.RawQuery = query.Encode()
		if name == "authorization-requested" {
			j.seedGrant(false)
			Expect(j.h.Storage.UserSessions().Create(context.Background(), fixtures.SessionForService(j.data.Principal.String(), fixtures.PlaceholderServiceID.String()))).To(Succeed())
			j.markAction(subject)
		}
		response, err := j.h.EndUser.AuthenticatedGET(authorize.RequestURI(), j.data.Principal.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location := response.Header.Get("Location")
		Expect(response.Body.Close()).To(Succeed())
		if name != "authorization-requested" {
			consent, parseErr := url.Parse(location)
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(consent.Query().Get("session_token")).NotTo(BeEmpty())
			Expect(j.h.Storage.UserSessions().Create(context.Background(), fixtures.SessionForService(j.data.Principal.String(), fixtures.PlaceholderServiceID.String()))).To(Succeed())
			response, err = postJSON(j.h.EndUser, "/api/consent/agents/"+j.data.Agent.ID.String()+"/grants?session_token="+url.QueryEscape(consent.Query().Get("session_token")), j.data.Principal.String(), j.consentBody(time.Now().Add(time.Hour)))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			redirect := decodeJSON[map[string]any](response)["redirect_url"].(string)
			response, err = j.h.EndUser.AuthenticatedGET(redirect, j.data.Principal.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			location = response.Header.Get("Location")
			Expect(response.Body.Close()).To(Succeed())
		}
		if variant == "hybrid-proxy" {
			Expect(location).To(HavePrefix(j.upstream.URL()))
			client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			response, err = client.Get(location)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			location = response.Header.Get("Location")
			Expect(response.Body.Close()).To(Succeed())
		}
		callback, err := url.Parse(location)
		Expect(err).NotTo(HaveOccurred())
		code := callback.Query().Get("code")
		Expect(code).NotTo(BeEmpty())
		j.trackSecret(code, func(c *fixtures.CredentialCanaries, value string) { c.AuthorizationCode = value })
		form.Set("code", code)
	}
	if name != "authorization-requested" {
		j.markAction(subject)
	}
	response, err := j.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusOK))
	j.captureTokenResponse(response)
	return matchers.BusinessEventExpectation{Type: "agentic-identity-broker." + name, Subject: &subject}
}

var _ = Describe("Business Event Ledger", Label("business-event-ledger"), func() {
	Context("committed broker facts", func() {
		// US1-AS1 from specs/048-business-event-ledger/spec.md.
		It("records each catalogue occurrence once with its required envelope", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				for _, eventType := range fixtures.LedgerCatalogueTypes() {
					eventName := strings.TrimPrefix(eventType, "agentic-identity-broker.")
					By(string(backend) + ": " + eventName)
					journey := newLedgerJourney(backend, eventName, nil)
					expected := journey.action(eventName)
					events := journey.actionEvents(expected)
					Expect(events).To(HaveLen(1), "one retained catalogue occurrence from the named action")
					Expect(events[0]).To(matchers.HaveBusinessEventEnvelope(expected))
					if strings.HasPrefix(eventName, "session-") || eventName == "signing-key-promoted" {
						Expect(events[0].AgentID).To(BeZero(), "the workflow has no receiving agent reference")
					}
					delta := map[string]int{eventType: 1}
					switch eventName {
					case "grant-expired":
						delta["agentic-identity-broker.authorization-requested"] = 1
					case "impersonation-granted":
						delta["agentic-identity-broker.token-exchanged"] = 1
					case "impersonation-denied":
						delta["agentic-identity-broker.token-exchange-denied"] = 1
					}
					journey.assertActionDelta(*expected.Subject, delta)
					journey.assertCredentialFree(journey.queryAll(*expected.Subject))
					Expect(journey.h.Close()).To(Succeed())
				}
			}
		})

		// US1-AS3 from specs/048-business-event-ledger/spec.md.
		It("rolls back owned state and withholds credentials when event append fails", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				for _, eventName := range []string{"grant-revoked", "session-terminated", "approval-approved", "credential-generated", "credential-rotated", "credential-revoked", "signing-key-promoted", "signing-key-generated-as-current", "token-issued", "token-exchanged"} {
					By(string(backend) + ": " + eventName)
					receiver, err := helpers.NewOTLPReceiver()
					Expect(err).NotTo(HaveOccurred())
					DeferCleanup(receiver.Close)
					faults := &bootstrap.LedgerStorageFaults{}
					journey := newConfiguredLedgerJourney(backend, eventName, faults, func(config *ports.Config) {
						config.Telemetry = fixtures.TelemetryGRPCConfig().Telemetry
						config.Telemetry.Traces.Enabled = false
						config.Telemetry.Logs.Enabled = true
						config.Telemetry.Exporter.Endpoint = receiver.Endpoint()
						config.BusinessEvents.TelemetryCopyEnabled = true
					}, nil)
					control := journey.action("agent-registered")
					controlEvents := journey.actionEvents(control)
					Expect(controlEvents).To(HaveLen(1))
					Eventually(func() bool { return ledgerHasID(receiver.LedgerRecords(), controlEvents[0].ID) }, 5*time.Second, 20*time.Millisecond).Should(BeTrue(), "export is enabled on the fault-wrapped broker")
					subject := model.BusinessEventSubject{Principal: journey.data.Principal}
					eventType := "agentic-identity-broker." + eventName
					if eventName == "signing-key-generated-as-current" {
						eventType = "agentic-identity-broker.signing-key-promoted"
					}
					var request func() (*http.Response, error)
					var unchanged func()
					ctx := context.Background()
					switch eventName {
					case "grant-revoked":
						grant := journey.seedGrant(false)
						stored, err := journey.h.Storage.UserGrants().Get(ctx, grant.ID)
						Expect(err).NotTo(HaveOccurred())
						before := *stored
						request = func() (*http.Response, error) {
							return journey.h.EndUser.DirectRequest(http.MethodDelete, "/api/consent/agents/"+journey.data.Agent.ID.String()+"/grants", journey.data.Principal.String(), nil, nil)
						}
						unchanged = func() {
							after, err := journey.h.Storage.UserGrants().Get(ctx, grant.ID)
							Expect(err).NotTo(HaveOccurred())
							Expect(after).To(Equal(&before))
						}
					case "session-terminated":
						session := journey.establishSession()
						before := *session
						request = func() (*http.Response, error) {
							return journey.h.EndUser.DirectRequest(http.MethodDelete, "/api/third-party/"+journey.data.Service.ID.String()+"/session", journey.data.Principal.String(), nil, nil)
						}
						unchanged = func() {
							after, err := journey.h.Storage.UserSessions().Get(ctx, session.ID)
							Expect(err).NotTo(HaveOccurred())
							Expect(after).To(Equal(&before))
						}
					case "approval-approved":
						created := journey.createApproval("ledger_atomicity", map[string]any{})
						approvalID := id.MustParseApprovalID(created.Data.ID)
						before, err := journey.h.Storage.ToolApprovals().Get(ctx, approvalID)
						Expect(err).NotTo(HaveOccurred())
						Expect(before.Status).To(Equal(domainstorage.ApprovalStatusPending))
						syncVersion, err := journey.h.Storage.ApprovalSyncState().GetVersion(ctx)
						Expect(err).NotTo(HaveOccurred())
						request = func() (*http.Response, error) {
							return postJSON(journey.h.EndUser, "/api/approvals/"+approvalID.String()+"/approve", journey.data.Principal.String(), helpers.ApproveRequest{Persistence: "once"})
						}
						unchanged = func() {
							after, err := journey.h.Storage.ToolApprovals().Get(ctx, approvalID)
							Expect(err).NotTo(HaveOccurred())
							Expect(after.Status).To(Equal(domainstorage.ApprovalStatusPending))
							Expect(after.ApprovedAt).To(BeNil())
							Expect(after.Consumed).To(BeFalse())
							version, err := journey.h.Storage.ApprovalSyncState().GetVersion(ctx)
							Expect(err).NotTo(HaveOccurred())
							Expect(version).To(Equal(syncVersion))
						}
					case "credential-generated", "credential-rotated", "credential-revoked":
						subject = model.BusinessEventSubject{NoSubject: true}
						path := "/api/agents/" + journey.data.Agent.ID.String() + "/client-credentials"
						if eventName != "credential-generated" {
							response, err := journey.adminPost(path, nil)
							Expect(err).NotTo(HaveOccurred())
							Expect(response.StatusCode).To(Equal(http.StatusCreated))
							secret := decodeJSON[map[string]any](response)["client_secret"].(string)
							journey.trackSecret(secret, func(c *fixtures.CredentialCanaries, value string) { c.ClientSecret = value })
						}
						var before *domainstorage.ClientCredential
						if eventName != "credential-generated" {
							before, err = journey.h.Storage.BrokerCredentials().GetByAgentID(ctx, journey.data.Agent.ID)
							Expect(err).NotTo(HaveOccurred())
						}
						if before != nil {
							copy := *before
							before = &copy
						}
						if eventName == "credential-revoked" {
							request = func() (*http.Response, error) {
								return journey.h.Admin.DirectRequest(http.MethodDelete, path, "", nil, nil)
							}
						} else {
							request = func() (*http.Response, error) { return journey.adminPost(path, nil) }
						}
						unchanged = func() {
							after, err := journey.h.Storage.BrokerCredentials().GetByAgentID(ctx, journey.data.Agent.ID)
							if before == nil {
								Expect(err).To(HaveOccurred())
								Expect(after).To(BeNil())
								return
							}
							Expect(err).NotTo(HaveOccurred())
							Expect(after).To(Equal(before))
						}
					case "signing-key-promoted", "signing-key-generated-as-current":
						subject = model.BusinessEventSubject{NoSubject: true}
						addKey := func() string {
							response, err := journey.h.Admin.AuthenticatedPOST("/api/oauth2-server/signing-keys", fixtures.LedgerAdminID(), "application/json", psJSON(map[string]any{"algorithm": "ES256"}))
							Expect(err).NotTo(HaveOccurred())
							Expect(response.StatusCode).To(Equal(http.StatusCreated))
							return decodeJSON[map[string]any](response)["kid"].(string)
						}
						previousKID := addKey()
						currentKID := previousKID
						if eventName == "signing-key-promoted" {
							currentKID = addKey()
						}
						current, err := journey.h.Storage.SigningKeys().GetByKIDInDomain(ctx, domainstorage.KeyDomainTokenSigning, id.NewKeyID(currentKID))
						Expect(err).NotTo(HaveOccurred())
						Expect(current.KID.String()).To(Equal(currentKID))
						Expect(current.IsCurrent).To(BeTrue())
						eligible, err := journey.h.Storage.SigningKeys().GetCurrentInDomain(ctx, domainstorage.KeyDomainTokenSigning)
						Expect(err).NotTo(HaveOccurred())
						allKeys, err := journey.h.Storage.SigningKeys().ListActiveInDomain(ctx, domainstorage.KeyDomainTokenSigning)
						Expect(err).NotTo(HaveOccurred())
						copy := *current
						current = &copy
						active, err := journey.h.Storage.SigningKeys().CountActiveInDomain(ctx, domainstorage.KeyDomainTokenSigning)
						Expect(err).NotTo(HaveOccurred())
						if eventName == "signing-key-promoted" {
							request = func() (*http.Response, error) {
								return journey.h.Admin.DirectRequest(http.MethodPut, "/api/oauth2-server/signing-keys/"+previousKID+"/current", fixtures.LedgerAdminID(), map[string]string{"Content-Type": "application/json"}, nil)
							}
						} else {
							request = func() (*http.Response, error) {
								return journey.h.Admin.AuthenticatedPOST("/api/oauth2-server/signing-keys", fixtures.LedgerAdminID(), "application/json", psJSON(map[string]any{"algorithm": "ES256"}))
							}
						}
						unchanged = func() {
							after, err := journey.h.Storage.SigningKeys().GetByKIDInDomain(ctx, domainstorage.KeyDomainTokenSigning, id.NewKeyID(currentKID))
							Expect(err).NotTo(HaveOccurred())
							Expect(after).To(Equal(current))
							Expect(after.KID.String()).To(Equal(currentKID))
							stillEligible, err := journey.h.Storage.SigningKeys().GetCurrentInDomain(ctx, domainstorage.KeyDomainTokenSigning)
							Expect(err).NotTo(HaveOccurred())
							Expect(stillEligible).To(Equal(eligible))
							for _, before := range allKeys {
								unchangedKey, err := journey.h.Storage.SigningKeys().GetByKIDInDomain(ctx, domainstorage.KeyDomainTokenSigning, before.KID)
								Expect(err).NotTo(HaveOccurred())
								Expect(unchangedKey).To(Equal(before))
							}
							count, err := journey.h.Storage.SigningKeys().CountActiveInDomain(ctx, domainstorage.KeyDomainTokenSigning)
							Expect(err).NotTo(HaveOccurred())
							Expect(count).To(Equal(active))
						}
					case "token-issued":
						subject = model.BusinessEventSubject{NoSubject: true}
						Expect(helpers.ProvisionSigningKey(journey.h.Admin.BaseURL())).To(Succeed())
						path := "/api/agents/" + journey.data.Agent.ID.String() + "/client-credentials"
						response, err := journey.adminPost(path, nil)
						Expect(err).NotTo(HaveOccurred())
						Expect(response.StatusCode).To(Equal(http.StatusCreated))
						secret := decodeJSON[map[string]any](response)["client_secret"].(string)
						credential, err := journey.h.Storage.BrokerCredentials().GetByAgentID(ctx, journey.data.Agent.ID)
						Expect(err).NotTo(HaveOccurred())
						copy := *credential
						credential = &copy
						form := url.Values{"grant_type": {"client_credentials"}, "client_id": {journey.data.Agent.ID.String()}, "client_secret": {secret}}
						request = func() (*http.Response, error) {
							return journey.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
						}
						unchanged = func() {
							after, err := journey.h.Storage.BrokerCredentials().GetByAgentID(ctx, journey.data.Agent.ID)
							Expect(err).NotTo(HaveOccurred())
							Expect(after).To(Equal(credential))
						}
					case "token-exchanged":
						session := journey.establishSession()
						sessionCopy := *session
						grant := journey.seedGrant(false)
						storedGrant, err := journey.h.Storage.UserGrants().Get(ctx, grant.ID)
						Expect(err).NotTo(HaveOccurred())
						grantCopy := *storedGrant
						form := journey.exchangeForm()
						request = func() (*http.Response, error) {
							return journey.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
						}
						unchanged = func() {
							after, err := journey.h.Storage.UserSessions().Get(ctx, session.ID)
							Expect(err).NotTo(HaveOccurred())
							Expect(after).To(Equal(&sessionCopy))
							retained, err := journey.h.Storage.UserGrants().Get(ctx, grant.ID)
							Expect(err).NotTo(HaveOccurred())
							Expect(retained).To(Equal(&grantCopy))
						}
					}
					Expect(request).NotTo(BeNil())
					Expect(unchanged).NotTo(BeNil())
					baseline := journey.queryAll(subject)
					known := map[id.BusinessEventID]bool{}
					for _, event := range baseline {
						known[event.ID] = true
					}
					if backend == bootstrap.LedgerPostgres {
						Eventually(func() int {
							var count int
							Expect(journey.h.OwnerDB.Get(&count, "SELECT count(*) FROM public.business_event_delivery_pending")).To(Succeed())
							return count
						}, 5*time.Second, 20*time.Millisecond).Should(BeZero(), "valid setup facts must be acknowledged before the fault")
					}
					faults.FailAppend(eventType, errors.New("forced event persistence failure"))
					response, err := request()
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
					body, err := io.ReadAll(response.Body)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.Body.Close()).To(Succeed())
					for _, credentialField := range []string{`"client_secret"`, `"access_token"`, `"refresh_token"`, `"id_token"`} {
						Expect(string(body)).NotTo(ContainSubstring(credentialField), "a failed commit must not release credentials")
					}
					unchanged()
					for _, event := range journey.queryAll(subject) {
						if event.Type == eventType {
							Expect(known[event.ID]).To(BeTrue(), "rollback must not retain a success event")
						}
					}
					if backend == bootstrap.LedgerPostgres {
						var count int
						Expect(journey.h.OwnerDB.Get(&count, `SELECT count(*) FROM public.business_event_delivery_pending d
							JOIN public.business_events e ON e.recorded_at=d.recorded_at AND e.id=d.event_id WHERE e.type=$1`, eventType)).To(Succeed())
						Expect(count).To(BeZero(), "rollback must not enqueue its success copy, independently of a committed failure fact")
					}
					Expect(journey.h.Close()).To(Succeed())
					for _, record := range receiver.LedgerRecords() {
						if record.Record.EventName != eventType {
							continue
						}
						for _, attribute := range record.Record.Attributes {
							if attribute.Key != "id" {
								continue
							}
							observed := id.MustParseBusinessEventID(attribute.Value.GetStringValue())
							Expect(known[observed]).To(BeTrue(), "a rolled-back success must not reach the real collector")
						}
					}
					Expect(receiver.Close()).To(Succeed())
				}
			}
		})

		// US1-AS4 from specs/048-business-event-ledger/spec.md.
		It("records final refusals and failures independently of mutations", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				for _, eventName := range []string{"token-exchange-denied", "impersonation-denied", "token-request-failed", "session-refresh-failed"} {
					journey := newLedgerJourney(backend, eventName, nil)
					expected := journey.action(eventName)
					events := journey.actionEvents(expected)
					Expect(events).To(HaveLen(1))
					Expect(events[0]).To(matchers.HaveBusinessEventEnvelope(expected))
					delta := map[string]int{expected.Type: 1}
					if eventName == "impersonation-denied" {
						delta["agentic-identity-broker.token-exchange-denied"] = 1
					}
					journey.assertActionDelta(*expected.Subject, delta)
					Expect(journey.h.Close()).To(Succeed())
				}
				journey := newLedgerJourney(backend, "token-exchange-denied", nil)
				permissionSet, err := journey.h.Storage.PermissionSets().Get(context.Background(), fixtures.PlaceholderPermissionSetID)
				Expect(err).NotTo(HaveOccurred())
				for i := range permissionSet.ServiceScopes {
					if permissionSet.ServiceScopes[i].ServiceID == journey.data.Service.ID {
						permissionSet.ServiceScopes[i].Scopes = []string{"read"}
					}
				}
				Expect(journey.h.Storage.PermissionSets().Update(context.Background(), permissionSet)).To(Succeed())
				grant := journey.seedGrant(false)
				session := journey.establishSession()
				session.Scope = []string{"unrelated"}
				Expect(journey.h.Storage.UserSessions().Create(context.Background(), session)).To(Succeed())
				session, err = journey.h.Storage.UserSessions().Get(context.Background(), session.ID)
				Expect(err).NotTo(HaveOccurred())
				subject := model.BusinessEventSubject{Principal: journey.data.Principal}
				journey.markAction(subject)
				response, err := journey.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(journey.exchangeForm().Encode()))
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
				body := decodeJSON[map[string]any](response)
				Expect(body["error"]).To(Equal("invalid_grant"))
				Expect(body["error_uri"]).To(Equal(journey.h.App.OAuth2SessionService.SessionRecoveryURL()))
				caller := "approval-gateway-client"
				principal := journey.data.Principal
				expected := matchers.BusinessEventExpectation{
					Type: "agentic-identity-broker.token-exchange-denied", Subject: &subject,
					Actor:   &model.BusinessEventActor{Kind: "gateway", ID: &caller, OnBehalfOf: &principal},
					Outcome: model.BusinessEventDenied, Data: map[string]any{"reason_code": "authorization_failed"},
					AgentID: &journey.data.Agent.ID, GrantID: &grant.ID, ServiceID: &journey.data.Service.ID, SessionID: &session.ID,
					PermissionSetIDs: []id.PermissionSetID{fixtures.PlaceholderPermissionSetID},
				}
				events := journey.actionEvents(expected)
				Expect(events).To(HaveLen(1))
				Expect(events[0]).To(matchers.HaveBusinessEventEnvelope(expected))
				journey.assertActionDelta(subject, map[string]int{expected.Type: 1})
				retained, err := journey.h.Storage.UserSessions().Get(context.Background(), session.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(retained).To(Equal(session), "the scope refusal must not change the session")
				Expect(journey.h.Close()).To(Succeed())
			}
		})

		// US1-AS5 from specs/048-business-event-ledger/spec.md.
		It("records only winning grant, session and approval transitions", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				for _, eventName := range []string{"grant-revoked", "session-terminated", "approval-approved"} {
					journey := newLedgerJourney(backend, eventName, nil)
					method, path := http.MethodDelete, "/api/consent/agents/"+journey.data.Agent.ID.String()+"/grants"
					caller := journey.data.Principal.String()
					expected := matchers.BusinessEventExpectation{Type: "agentic-identity-broker." + eventName, Subject: &model.BusinessEventSubject{Principal: journey.data.Principal}, Actor: &model.BusinessEventActor{Kind: "user", ID: &caller}, Outcome: model.BusinessEventSuccess, Data: map[string]any{}}
					switch eventName {
					case "grant-revoked":
						grant := journey.seedGrant(false)
						expected.GrantID = &grant.ID
						expected.AgentID = &journey.data.Agent.ID
						expected.PermissionSetIDs = []id.PermissionSetID{fixtures.PlaceholderPermissionSetID}
					case "session-terminated":
						session := journey.establishSession()
						path = "/api/third-party/" + journey.data.Service.ID.String() + "/session"
						expected.SessionID = &session.ID
						expected.ServiceID = &journey.data.Service.ID
					case "approval-approved":
						created := journey.createApproval("ledger_race", map[string]any{})
						approvalID := id.MustParseApprovalID(created.Data.ID)
						method, path = http.MethodPost, "/api/approvals/"+created.Data.ID
						expected.ApprovalID = &approvalID
						expected.AgentID = &journey.data.Agent.ID
						gateway := id.NewClientID("approval-gateway-client")
						expected.GatewayClientID = &gateway
					}
					var wg sync.WaitGroup
					statuses := make(chan int, 8)
					start := make(chan struct{})
					for attempt := range 8 {
						wg.Go(func() {
							defer GinkgoRecover()
							<-start
							requestPath := path
							var body io.Reader
							if eventName == "approval-approved" {
								requestPath += "/approve"
								body = strings.NewReader(`{"persistence":"once"}`)
								if attempt%2 == 1 {
									requestPath = path + "/deny"
									body = strings.NewReader(`{}`)
								}
							}
							response, err := journey.h.EndUser.DirectRequest(method, requestPath, journey.data.Principal.String(), map[string]string{"Content-Type": "application/json"}, body)
							Expect(err).NotTo(HaveOccurred())
							statuses <- response.StatusCode
							Expect(response.Body.Close()).To(Succeed())
						})
					}
					close(start)
					wg.Wait()
					close(statuses)
					for status := range statuses {
						Expect(status).To(SatisfyAny(Equal(http.StatusOK), Equal(http.StatusNoContent), BeNumerically(">=", 400)))
						Expect(status).To(BeNumerically("<", 500))
					}
					events := journey.query(eventName, *expected.Subject)
					if eventName == "approval-approved" {
						events = append(events, journey.query("approval-denied", *expected.Subject)...)
					}
					Expect(events).To(HaveLen(1))
					if eventName == "approval-approved" {
						expected.Type = events[0].Type
						if events[0].Type == "agentic-identity-broker.approval-denied" {
							expected.Outcome = model.BusinessEventDenied
							expected.Data = map[string]any{"reason_code": "user_denied"}
						}
					}
					Expect(events[0]).To(matchers.HaveBusinessEventEnvelope(expected))
					response, err := journey.h.EndUser.AuthenticatedGET(path, journey.data.Principal.String())
					Expect(err).NotTo(HaveOccurred())
					switch eventName {
					case "approval-approved":
						Expect(response.StatusCode).To(Equal(http.StatusOK))
						approval := decodeJSON[helpers.ApprovalDetailResponse](response)
						if events[0].Type == "agentic-identity-broker.approval-approved" {
							Expect(approval.Data.Status).To(Equal("approved"))
						} else {
							Expect(approval.Data.Status).To(Equal("denied"))
						}
					case "grant-revoked":
						ledgerStatus(response, nil, http.StatusOK)
						grant, err := journey.h.Storage.UserGrants().FindByPrincipalAndAgent(context.Background(), journey.data.Principal, journey.data.Agent.ID)
						Expect(err == nil || ports.IsNotFoundErr(err)).To(BeTrue())
						Expect(grant).To(BeNil(), "the winning HTTP revocation must remove the delegation")
					default:
						ledgerStatus(response, nil, http.StatusNotFound)
					}
					Expect(journey.h.Close()).To(Succeed())
				}
			}
		})

		// US1-AS6 from specs/048-business-event-ledger/spec.md.
		It("recognizes effective expiry once across repeated access", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				for _, eventName := range []string{"grant-expired", "approval-expired"} {
					journey := newLedgerJourney(backend, eventName, nil)
					clock := func() time.Time {
						if backend == bootstrap.LedgerPostgres {
							var now time.Time
							Expect(journey.h.OwnerDB.Get(&now, "SELECT clock_timestamp()")).To(Succeed())
							return now.UTC()
						}
						return time.Now().UTC()
					}
					recognitionStart := clock()
					expected := journey.action(eventName)
					for range 3 {
						path, status := journey.authorizationPath(), http.StatusFound
						if expected.ApprovalID != nil {
							path, status = "/api/approvals/"+expected.ApprovalID.String(), http.StatusGone
						}
						response, err := journey.h.EndUser.AuthenticatedGET(path, journey.data.Principal.String())
						ledgerStatus(response, err, status)
					}
					recognitionEnd := clock()
					events := journey.query(eventName, *expected.Subject)
					Expect(events).To(HaveLen(1))
					Expect(events[0]).To(matchers.HaveBusinessEventEnvelope(expected))
					Expect(events[0].OccurredAt.Truncate(time.Microsecond)).To(Equal(journey.expiresAt.UTC().Truncate(time.Microsecond)))
					Expect(events[0].RecordedAt).To(BeTemporally(">=", recognitionStart.Add(-time.Microsecond)))
					Expect(events[0].RecordedAt).To(BeTemporally("<=", recognitionEnd.Add(time.Microsecond)))
					Expect(journey.h.Close()).To(Succeed())
				}
			}
		})
	})

	Context("safe operational investigation", func() {
		// US2-AS1 from specs/048-business-event-ledger/spec.md.
		It("returns exactly the receiving agents in the selected subject and time interval", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				journey := newLedgerJourney(backend, "token-exchanged", nil)
				agents := []*domainstorage.Agent{journey.data.Agent, fixtures.AgentWithClientID("ledger-b"), fixtures.AgentWithClientID("ledger-c")}
				for _, agent := range agents[1:] {
					Expect(journey.h.Storage.Agents().Create(context.Background(), agent)).To(Succeed())
				}
				selectCaller := func(agent *domainstorage.Agent, principal id.Principal) {
					journey.data.Agent, journey.data.Principal = agent, principal
					var err error
					journey.auth, err = helpers.NewApprovalRequestAuthFixture(journey.upstream, agent.ID)
					Expect(err).NotTo(HaveOccurred())
				}
				journey.action("token-exchanged")
				start := time.Now().UTC()
				journey.action("token-exchanged")
				selectCaller(agents[1], fixtures.LedgerPrincipal())
				journey.action("token-exchanged")
				selectCaller(agents[2], fixtures.LedgerPrincipal())
				journey.action("token-exchange-denied")
				selectCaller(agents[2], fixtures.LedgerOtherPrincipal())
				journey.action("token-exchanged")
				end := time.Now().UTC()
				selectCaller(agents[2], fixtures.LedgerPrincipal())
				journey.action("token-exchanged")
				query := model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: fixtures.LedgerPrincipal()}, Type: "agentic-identity-broker.token-exchanged", Outcome: model.BusinessEventSuccess, Start: start, End: end}
				events, err := journey.h.App.LedgerService.Query(context.Background(), query)
				Expect(err).NotTo(HaveOccurred())
				Expect(events).To(HaveLen(2))
				receivers := map[id.AgentID]bool{}
				for _, event := range events {
					receivers[event.AgentID] = true
					Expect(event.Subject).To(Equal(&query.Subject.Principal))
					Expect(event.OccurredAt.Before(end)).To(BeTrue())
					Expect(event.OccurredAt.Before(start)).To(BeFalse())
				}
				Expect(receivers).To(Equal(map[id.AgentID]bool{agents[0].ID: true, agents[1].ID: true}))
				journey.assertCredentialFree(events)
				if backend == bootstrap.LedgerPostgres {
					sqlEvents, err := helpers.QueryLedgerEvents(context.Background(), journey.h.ReaderDB, query)
					Expect(err).NotTo(HaveOccurred())
					Expect(sqlEvents).To(Equal(events))
				}
			}
		})

		// US2-AS2 from specs/048-business-event-ledger/spec.md.
		It("retains verified initiating caller, represented subject and authoritative request correlation", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				spanExporter := tracetest.NewInMemoryExporter()
				tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
				journey := newConfiguredLedgerJourney(backend, "token-exchanged", nil, func(config *ports.Config) {
					config.Telemetry = fixtures.TelemetryEnabledConfig().Telemetry
				}, nil, tracerProvider)
				journey.establishSession()
				journey.seedGrant(false)
				form := journey.exchangeForm()
				traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
				response, err := journey.h.EndUser.DirectRequest(http.MethodPost, "/oauth2/token", "", map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Traceparent": "00-" + traceID + "-00f067aa0ba902b7-01", "User-Agent": "curl/8.17 (" + journey.data.Canaries.ClientSecret + ")", "X-Forwarded-For": "198.51.100.99"}, strings.NewReader(form.Encode()))
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				observed := response.Header.Get("traceresponse")
				Expect(observed).To(MatchRegexp(`^00-` + traceID + `-[0-9a-f]{16}-[0-9a-f]{2}$`))
				spanID := strings.Split(observed, "-")[2]
				Expect(spanID).NotTo(Equal("00f067aa0ba902b7"), "use the server span, not the inbound parent")
				Eventually(func(g Gomega) {
					serverSpan := false
					for _, captured := range spanExporter.GetSpans() {
						if captured.SpanContext.TraceID().String() == traceID && captured.SpanContext.SpanID().String() == spanID {
							g.Expect(captured.Parent.SpanID().String()).To(Equal("00f067aa0ba902b7"))
							serverSpan = true
						}
					}
					g.Expect(serverSpan).To(BeTrue(), "traceresponse must name an actual recorded server span")
				}, 2*time.Second, 10*time.Millisecond).Should(Succeed())
				journey.captureTokenResponse(response)
				events := journey.query("token-exchanged", model.BusinessEventSubject{Principal: journey.data.Principal})
				Expect(events).To(HaveLen(1))
				caller := "approval-gateway-client"
				gateway := id.NewClientID(caller)
				Expect(events[0]).To(matchers.HaveBusinessEventEnvelope(matchers.BusinessEventExpectation{Type: "agentic-identity-broker.token-exchanged", Subject: &model.BusinessEventSubject{Principal: journey.data.Principal}, Actor: &model.BusinessEventActor{Kind: "gateway", ID: &caller, OnBehalfOf: &journey.data.Principal}, AgentID: &journey.data.Agent.ID, GatewayClientID: &gateway, ServiceID: &journey.data.Service.ID, TraceID: &traceID, SpanID: &spanID, Client: &model.BusinessEventClient{IP: "127.0.0.1", UserAgent: "curl"}, Outcome: model.BusinessEventSuccess, Data: map[string]any{}}))
				journey.assertCredentialFree(events)
				Expect(journey.h.Close()).To(Succeed())

				untraced := newLedgerJourney(backend, "token-exchanged", nil)
				untraced.establishSession()
				untraced.seedGrant(false)
				response, err = untraced.h.EndUser.DirectRequest(http.MethodPost, "/oauth2/token", "", map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Traceparent": "00-" + traceID + "-00f067aa0ba902b7-01"}, strings.NewReader(untraced.exchangeForm().Encode()))
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				generated := response.Header.Get("traceresponse")
				Expect(generated).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`))
				Expect(strings.Split(generated, "-")[1]).NotTo(Equal(traceID))
				untraced.captureTokenResponse(response)
				events = untraced.query("token-exchanged", model.BusinessEventSubject{Principal: untraced.data.Principal})
				Expect(events).To(HaveLen(1))
				Expect(events[0].TraceID).To(Equal(strings.Split(generated, "-")[1]))
				Expect(events[0].SpanID).To(BeEmpty(), "a traceresponse random identifier is not a request span")
			}
		})

		// US2-AS3 from specs/048-business-event-ledger/spec.md.
		It("excludes all seven credential classes from every stored and OTLP catalogue envelope", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				for _, eventType := range fixtures.LedgerCatalogueTypes() {
					receiver, err := helpers.NewOTLPReceiver()
					Expect(err).NotTo(HaveOccurred())
					DeferCleanup(receiver.Close)
					name := strings.TrimPrefix(eventType, "agentic-identity-broker.")
					journey := newConfiguredLedgerJourney(backend, name, nil, func(config *ports.Config) {
						config.Telemetry = fixtures.TelemetryGRPCConfig().Telemetry
						config.Telemetry.Traces.Enabled = false
						config.Telemetry.Logs.Enabled = true
						config.Telemetry.Exporter.Endpoint = receiver.Endpoint()
						config.BusinessEvents.TelemetryCopyEnabled = true
					}, nil)
					canaries := journey.data.Canaries
					hostile := strings.Join([]string{canaries.AccessToken, canaries.RefreshToken, canaries.ClientSecret, canaries.ClientAssertion, canaries.RawJWT, canaries.AuthorizationCode, canaries.PKCEVerifier}, " ")
					headers := http.Header{"User-Agent": {"curl/8.17 (" + hostile + ")"}}
					journey.h.Admin.SetLedgerRequestHeaders(headers)
					journey.h.EndUser.SetLedgerRequestHeaders(headers)
					expected := journey.action(name)
					events := journey.actionEvents(expected)
					Expect(events).To(HaveLen(1))
					journey.assertCredentialFree(journey.queryAll(*expected.Subject))
					if !expected.Subject.NoSubject {
						journey.assertCredentialFree(journey.queryAll(model.BusinessEventSubject{NoSubject: true}))
					}
					Eventually(func() bool {
						for _, record := range receiver.LedgerRecords() {
							if record.Record.EventName == eventType {
								return true
							}
						}
						return false
					}, 5*time.Second, 20*time.Millisecond).Should(BeTrue())
					Expect(journey.h.Close()).To(Succeed())
					journey.assertCredentialFree(receiver.LedgerRecords())
					Expect(receiver.Close()).To(Succeed())
				}
			}
		})

		// US2-AS4 from specs/048-business-event-ledger/spec.md.
		It("uses explicit null identities without trusting supplied pre-authentication claims", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				journey := newLedgerJourney(backend, "token-request-failed", nil)
				response, err := journey.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader("grant_type=client_credentials&client_id=unknown&client_secret=invalid"))
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(BeNumerically(">=", 400))
				Expect(response.Body.Close()).To(Succeed())
				events := journey.query("token-request-failed", model.BusinessEventSubject{NoSubject: true})
				Expect(events).To(HaveLen(1))
				Expect(events[0].Subject).To(BeNil())
				Expect(events[0].Actor.ID).To(BeNil())
				Expect(events[0].Actor.OnBehalfOf).To(BeNil())
				Expect(events[0].Source).To(Equal("urn:agentic-identity-broker:broker"))
				Expect(journey.query("token-request-failed", model.BusinessEventSubject{Principal: fixtures.LedgerPrincipal()})).To(BeEmpty())
				registered := newLedgerJourney(backend, "token-request-failed", nil)
				failure := registered.action("token-request-failed")
				registeredEvents := registered.actionEvents(failure)
				Expect(registeredEvents).To(HaveLen(1))
				Expect(registeredEvents[0]).To(matchers.HaveBusinessEventEnvelope(failure))
				Expect(registeredEvents[0].Subject).To(BeNil())
				Expect(registeredEvents[0].Actor.ID).To(BeNil(), "registered client identity is not authenticated by a wrong secret")
				Expect(registeredEvents[0].Actor.OnBehalfOf).To(BeNil())
				Expect(registered.queryAll(model.BusinessEventSubject{Principal: registered.data.Principal})).To(BeEmpty())

				forged := newLedgerJourney(backend, "token-exchanged", nil)
				forged.establishSession()
				forged.seedGrant(false)
				form := forged.exchangeForm()
				privateKey, _, err := helpers.GenerateTestRSAKeyPair()
				Expect(err).NotTo(HaveOccurred())
				now := time.Now()
				subjectJWT, err := helpers.SignTestJWT(map[string]any{"sub": forged.data.Principal.String(), "azp": forged.data.Agent.ID.String(), "iss": forged.upstream.URL(), "aud": "token-exchange-broker", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}, privateKey)
				Expect(err).NotTo(HaveOccurred())
				assertionJWT, err := helpers.SignTestJWT(map[string]any{"sub": "approval-gateway-client", "iss": forged.upstream.URL(), "aud": "token-exchange-broker", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}, privateKey)
				Expect(err).NotTo(HaveOccurred())
				form.Set("subject_token", subjectJWT)
				form.Set("client_assertion", assertionJWT)
				response, err = forged.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(response.Body.Close()).To(Succeed())
				denials := forged.query("token-exchange-denied", model.BusinessEventSubject{NoSubject: true})
				Expect(denials).To(HaveLen(1))
				Expect(denials[0].Subject).To(BeNil())
				Expect(denials[0].Actor.Kind).To(Equal("gateway"))
				Expect(denials[0].Actor.ID).To(BeNil())
				Expect(denials[0].Actor.OnBehalfOf).To(BeNil())
				Expect(denials[0].GatewayClientID).To(BeZero(), "an unverified assertion cannot identify the gateway")
				Expect(denials[0].Data).To(Equal(map[string]any{"reason_code": "authentication_failed"}))
				Expect(denials[0].AgentID).To(BeZero(), "an unverified azp cannot identify the receiving agent")
				Expect(forged.query("token-exchange-denied", model.BusinessEventSubject{Principal: forged.data.Principal})).To(BeEmpty())
				Expect(forged.query("token-request-failed", model.BusinessEventSubject{NoSubject: true})).To(BeEmpty(), "specific exchange authentication refusal must not create a generic companion failure")
				admin := newLedgerJourney(backend, "agent-registered", nil)
				expected := admin.action("agent-registered")
				adminEvents := admin.query("agent-registered", model.BusinessEventSubject{NoSubject: true})
				Expect(adminEvents).To(HaveLen(1))
				Expect(adminEvents[0]).To(matchers.HaveBusinessEventEnvelope(expected))
				Expect(adminEvents[0].Actor).To(Equal(model.BusinessEventActor{Kind: "admin"}))
				automated := newLedgerJourney(backend, "grant-expired", nil)
				expected = automated.action("grant-expired")
				expiryEvents := automated.query("grant-expired", *expected.Subject)
				Expect(expiryEvents).To(HaveLen(1))
				Expect(expiryEvents[0]).To(matchers.HaveBusinessEventEnvelope(expected))
				systemID := "broker-lifecycle"
				Expect(expiryEvents[0].Actor).To(Equal(model.BusinessEventActor{Kind: "system", ID: &systemID}))
			}
		})

		// US2-AS5 from specs/048-business-event-ledger/spec.md.
		It("adds an offline reviewed type without reinterpreting retained events or changing the database", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				receiver, err := helpers.NewOTLPReceiver()
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(receiver.Close)
				journey := newConfiguredLedgerJourney(backend, "grant-created", nil, func(config *ports.Config) {
					config.Telemetry = fixtures.TelemetryGRPCConfig().Telemetry
					config.Telemetry.Traces.Enabled = false
					config.Telemetry.Logs.Enabled = true
					config.Telemetry.Exporter.Endpoint = receiver.Endpoint()
					config.BusinessEvents.TelemetryCopyEnabled = true
				}, fixtures.LedgerFixtureSchemas())
				expected := journey.action("grant-created")
				before := journey.query("grant-created", *expected.Subject)
				Expect(before).To(HaveLen(1))
				event := &model.BusinessEvent{ID: id.NewBusinessEventID(), Type: fixtures.LedgerFixtureEventType, Source: "urn:agentic-identity-broker:broker", OccurredAt: time.Now().UTC(), Subject: &journey.data.Principal, Actor: model.BusinessEventActor{Kind: "user", ID: func() *string { value := journey.data.Principal.String(); return &value }()}, Outcome: model.BusinessEventSuccess, ReasonUser: "The fixture action completed.", ReasonAdmin: "The reviewed fixture action completed.", Data: map[string]any{"marker": "reviewed-fixture"}}
				event.Data = map[string]any{
					"marker": "reviewed-fixture", "large_integer": int64(9007199254740993),
					"large_integers": []int64{9007199254740993, -9007199254740993},
					"small_integer":  int32(7), "small_integers": []int32{7, -7},
					"fraction": float32(0.1), "fractions": []float32{0.1, -0.1},
				}
				Expect(journey.h.App.LedgerService.Record(context.Background(), event)).To(Succeed())
				events := journey.query("fixture-recorded", *expected.Subject)
				Expect(events).To(HaveLen(1))
				Expect(events[0].Type).To(Equal(fixtures.LedgerFixtureEventType))
				Expect(events[0].ID).To(Equal(event.ID))
				const numericJSON = `{"fraction":0.1,"fractions":[0.1,-0.1],"large_integer":9007199254740993,"large_integers":[9007199254740993,-9007199254740993],"marker":"reviewed-fixture","small_integer":7,"small_integers":[7,-7]}`
				queriedData, err := json.Marshal(events[0].Data)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(queriedData)).To(Equal(numericJSON), "scoped queries must preserve exact numeric payload values")
				envelope := events[0].Wire()
				if backend == bootstrap.LedgerPostgres {
					envelope, err = helpers.QueryLedgerRawEnvelope(context.Background(), journey.h.ReaderDB, event.ID)
					Expect(err).NotTo(HaveOccurred())
				}
				rawData, err := json.Marshal(envelope["data"])
				Expect(err).NotTo(HaveOccurred())
				Expect(string(rawData)).To(Equal(numericJSON), "raw retained JSON must match the scoped query without rounding")
				Expect(journey.query("grant-created", *expected.Subject)).To(Equal(before))
				Eventually(func() error {
					for _, record := range receiver.LedgerRecords() {
						if ledgerHasID([]helpers.OTLPLogRecord{record}, event.ID) {
							return helpers.LedgerTelemetryEnvelope(events[0], envelope, record)
						}
					}
					return errors.New("numeric fixture has not reached the ledger exporter")
				}, 5*time.Second, 20*time.Millisecond).Should(Succeed(), "every numeric value must reach actual OTLP unchanged with the original ID")
				if backend == bootstrap.LedgerPostgres {
					Eventually(func() (int, error) {
						var count int
						err := journey.h.OwnerDB.Get(&count, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", event.ID)
						return count, err
					}, 5*time.Second, 20*time.Millisecond).Should(BeZero(), "successful numeric export must be acknowledged")
				}
				retained := journey.queryAll(*expected.Subject)
				unattributed := journey.queryAll(model.BusinessEventSubject{NoSubject: true})
				unknown := *event
				unknown.ID, unknown.Type = id.NewBusinessEventID(), "agentic-identity-broker.unknown"
				Expect(journey.h.App.LedgerService.Record(context.Background(), &unknown)).NotTo(Succeed())
				invalid := *event
				invalid.ID, invalid.Data = id.NewBusinessEventID(), map[string]any{"marker": "reviewed-fixture", "unexpected": "forbidden"}
				Expect(journey.h.App.LedgerService.Record(context.Background(), &invalid)).NotTo(Succeed())
				Expect(journey.queryAll(*expected.Subject)).To(Equal(retained), "unknown/invalid events must not be persisted under the known subject")
				Expect(journey.queryAll(model.BusinessEventSubject{NoSubject: true})).To(Equal(unattributed), "rejected events must not escape into an unfiltered null-subject query")
				if backend == bootstrap.LedgerPostgres {
					for _, rejectedID := range []id.BusinessEventID{unknown.ID, invalid.ID} {
						var count int
						Expect(journey.h.OwnerDB.Get(&count, "SELECT count(*) FROM public.business_events WHERE id=$1", rejectedID)).To(Succeed())
						Expect(count).To(BeZero())
						Expect(journey.h.OwnerDB.Get(&count, "SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1", rejectedID)).To(Succeed())
						Expect(count).To(BeZero(), "a rejected event must never create a pending delivery reference")
					}
				}
				Expect(journey.h.Close()).To(Succeed())
				copies := receiver.LedgerRecords()
				Expect(ledgerHasID(copies, events[0].ID)).To(BeTrue())
				Expect(ledgerHasID(copies, unknown.ID)).To(BeFalse())
				Expect(ledgerHasID(copies, invalid.ID)).To(BeFalse())
				Expect(receiver.Close()).To(Succeed())
			}
		})
	})

	Context("retention and exact-subject erasure", func() {
		// US3-AS1 from specs/048-business-event-ledger/spec.md.
		It("erases every subject fact across families and partitions without affecting another principal", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				journey := newLedgerJourney(backend, "grant-created", nil)
				journey.action("grant-created")
				journey.action("token-exchanged")
				query := model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: fixtures.LedgerPrincipal()}, Start: journey.start.Add(-24 * time.Hour), End: time.Now().UTC().Add(time.Hour), Limit: 1000}
				journey.data.Principal = fixtures.LedgerOtherPrincipal()
				journey.action("approval-requested")
				signCaller := func(caller string) string {
					now := time.Now()
					assertion, err := helpers.SignTestJWT(map[string]any{"sub": caller, "iss": journey.upstream.URL(), "aud": []string{"token-exchange-broker"}, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}, journey.upstream.GetPrivateKeyPEM())
					Expect(err).NotTo(HaveOccurred())
					return assertion
				}
				journey.auth.ClientAssertion = signCaller(fixtures.LedgerPrincipal().String())
				delegated := journey.action("token-exchanged")
				crossActorEvents := journey.actionEvents(delegated)
				Expect(crossActorEvents).To(HaveLen(1))
				Expect(crossActorEvents[0].Subject).To(Equal(&journey.data.Principal))
				Expect(crossActorEvents[0].Actor.ID).To(HaveValue(Equal(fixtures.LedgerPrincipal().String())))
				Expect(crossActorEvents[0].Actor.OnBehalfOf).To(Equal(&journey.data.Principal))
				otherQuery := query
				otherQuery.Subject.Principal = fixtures.LedgerOtherPrincipal()
				other, err := journey.h.App.LedgerService.Query(context.Background(), otherQuery)
				Expect(err).NotTo(HaveOccurred())
				Expect(other).To(HaveLen(3)) // approval, session, delegated exchange
				if backend == bootstrap.LedgerPostgres {
					history := *crossActorEvents[0]
					history.ID = id.NewBusinessEventID()
					history.RecordedAt, history.OccurredAt = time.Now().UTC().Add(-12*time.Hour), time.Now().UTC().Add(-12*time.Hour)
					Expect(helpers.SeedLedgerHistory(context.Background(), journey.h.OwnerDB, &history, journey.h.App.LedgerService.Validate)).To(Succeed())
					other, err = journey.h.App.LedgerService.Query(context.Background(), otherQuery)
					Expect(err).NotTo(HaveOccurred())
					Expect(other).To(HaveLen(4))
				}
				journey.data.Principal = fixtures.LedgerPrincipal()
				journey.auth.ClientAssertion = signCaller(fixtures.LedgerOtherPrincipal().String())
				if backend == bootstrap.LedgerPostgres {
					history := *journey.queryAll(query.Subject)[0]
					history.ID = id.NewBusinessEventID()
					history.RecordedAt, history.OccurredAt = time.Now().UTC().Add(-12*time.Hour), time.Now().UTC().Add(-12*time.Hour)
					Expect(helpers.SeedLedgerHistory(context.Background(), journey.h.OwnerDB, &history, journey.h.App.LedgerService.Validate)).To(Succeed())
				}
				original := journey.queryAll(query.Subject)
				response, err := journey.h.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(journey.exchangeForm().Encode()))
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				journey.captureTokenResponse(response)
				before, err := journey.h.App.LedgerService.Query(context.Background(), query)
				Expect(err).NotTo(HaveOccurred())
				Expect(before).To(HaveLen(len(original) + 1))
				inverse := before[0]
				for _, event := range before {
					if event.Actor.ID != nil && *event.Actor.ID == fixtures.LedgerOtherPrincipal().String() {
						inverse = event
					}
				}
				Expect(inverse.Subject).To(Equal(&journey.data.Principal))
				Expect(inverse.Actor.ID).To(HaveValue(Equal(fixtures.LedgerOtherPrincipal().String())))
				Expect(inverse.Actor.OnBehalfOf).To(Equal(&journey.data.Principal))
				erase := func() (int64, error) {
					if backend == bootstrap.LedgerPostgres {
						return helpers.EraseLedgerSubject(context.Background(), journey.h.ErasureDB, fixtures.LedgerPrincipal())
					}
					return journey.h.App.LedgerService.EraseSubject(context.Background(), fixtures.LedgerPrincipal())
				}
				count, err := erase()
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(int64(len(before))))
				remaining, err := journey.h.App.LedgerService.Query(context.Background(), query)
				Expect(err).NotTo(HaveOccurred())
				Expect(remaining).To(BeEmpty())
				unchanged, err := journey.h.App.LedgerService.Query(context.Background(), otherQuery)
				Expect(err).NotTo(HaveOccurred())
				Expect(unchanged).To(Equal(other))
				count, err = erase()
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(BeZero())
			}
		})

		// US3-AS5 from specs/048-business-event-ledger/spec.md.
		It("rejects malformed or non-positive retention before serving", Serial, func() {
			root, err := integrationbootstrap.FindProjectRoot()
			Expect(err).NotTo(HaveOccurred())
			GinkgoT().Setenv("IDENTITY_BROKER_CONFIG_PATH", filepath.Join(root, "examples/config/business-event-ledger.yaml"))
			GinkgoT().Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", fixtures.TestKEKMaterialDeterministic())
			GinkgoT().Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", fixtures.DefaultOAuth2Config().ThirdPartyOAuth2.JWESigningKey)
			GinkgoT().Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION", "2160h")
			valid, err := brokerconfig.NewLoader().GetConfig(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(valid.BusinessEvents.Retention).To(Equal(2160 * time.Hour))
			for _, retention := range []string{"0", "-1h", "not-a-duration", "999999999999999999999h"} {
				GinkgoT().Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION", retention)
				configuration, err := brokerconfig.NewLoader().GetConfig(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(configuration).To(BeNil())
			}
		})
	})

	Context("monitoring continuity", func() {
		// US4-AS1 from specs/048-business-event-ledger/spec.md.
		It("preserves existing telemetry outcomes while exporting the complete ledger envelope", Serial, func() {
			for _, backend := range bootstrap.LedgerBackends() {
				for _, eventType := range fixtures.LedgerCatalogueTypes() {
					name := strings.TrimPrefix(eventType, "agentic-identity-broker.")
					for _, workflow := range fixtures.BusinessEventLegacySlog[name] {
						receiver, err := helpers.NewOTLPReceiver()
						Expect(err).NotTo(HaveOccurred())
						DeferCleanup(receiver.Close)
						variant := workflow.Variant
						setupName := name
						if name == "token-issued" && variant != "local-client-credentials" {
							setupName = "token-exchanged"
						}
						if name == "authorization-requested" {
							setupName = "token-issued"
						}
						previous := slog.Default()
						globalLogger, globalLogs := bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
						slog.SetDefault(globalLogger)
						DeferCleanup(slog.SetDefault, previous)
						journey := newConfiguredLedgerJourney(backend, setupName, nil, func(config *ports.Config) {
							if name == "token-issued" && strings.HasPrefix(variant, "hybrid") {
								config.OAuth2AuthServer.Mode = "hybrid"
								config.OAuth2AuthServer.Local.TokenTTL = time.Hour
							}
							config.Telemetry = fixtures.TelemetryGRPCConfig().Telemetry
							config.Telemetry.Traces.Enabled = false
							config.Telemetry.Logs.Enabled = true
							config.Telemetry.Exporter.Endpoint = receiver.Endpoint()
							config.BusinessEvents.TelemetryCopyEnabled = true
						}, nil)
						expected := journey.legacyAction(name, variant)
						events := journey.actionEvents(expected)
						Expect(events).To(HaveLen(1))
						local, err := journey.logs.Records()
						Expect(err).NotTo(HaveOccurred())
						global, err := globalLogs.Records()
						Expect(err).NotTo(HaveOccurred())
						records := append(local, global...)
						journey.assertCredentialFree(records)
						for _, descriptor := range workflow.Records {
							count := 0
							for _, record := range records {
								if record["msg"] != descriptor.Message || record["level"] != descriptor.Level {
									continue
								}
								actualEvent, _ := record["event"].(string)
								if actualEvent != descriptor.Event {
									continue
								}
								switch name {
								case "token-exchanged", "token-exchange-denied":
									expectedOutcome := "success"
									if name == "token-exchange-denied" {
										expectedOutcome = "authentication_failed"
										if variant == "verified-client-no-grant" {
											expectedOutcome = "authorization_denied"
										}
									}
									Expect(record).To(HaveKeyWithValue("token_exchange.outcome", expectedOutcome))
									Expect(record).To(HaveKeyWithValue("token_exchange.exchange_kind", "third_party"))
									for _, key := range []string{"error", "cause", "details", "resource", "failure_reason", "token_exchange.service.name"} {
										Expect(record).NotTo(HaveKey(key))
									}
								case "impersonation-granted", "impersonation-denied":
									expectedOutcome := "success"
									if name == "impersonation-denied" {
										expectedOutcome = "access_denied"
									}
									Expect(record).To(HaveKeyWithValue("outcome", expectedOutcome))
									Expect(record).To(HaveKeyWithValue("target_agent_id", journey.data.Agent.ID.String()))
									Expect(record).NotTo(HaveKey("audience"))
									Expect(record).NotTo(HaveKey("issuer_identifiers"))
									Expect(record).NotTo(HaveKey("failure_category"))
								case "session-refreshed", "session-refresh-failed":
									Expect(record).To(HaveKeyWithValue("service_id", journey.data.Service.ID.String()))
									Expect(record).NotTo(HaveKey("token_endpoint"))
									if name == "session-refreshed" && descriptor.Event == "" {
										Expect(record).To(HaveKeyWithValue("operation", "refresh"))
									}
									if descriptor.Event == "session.oauth2.refresh_failed" {
										Expect(record).NotTo(HaveKey("error"))
										Expect(record["oauth2_session"]).To(HaveKeyWithValue("failure_detail", "refresh_rejected"))
									}
								default:
									keys := make([]string, 0, len(record))
									for key := range record {
										keys = append(keys, key)
									}
									sort.Strings(keys)
									Expect(keys).To(Equal(descriptor.FieldKeys), "legacy field keys changed for "+name+" ("+variant+")")
								}
								count++
							}
							Expect(count).To(Equal(descriptor.Count), "legacy event name, level, message, field keys or multiplicity changed for "+name+" ("+variant+")")
						}
						var envelope map[string]any
						if backend == bootstrap.LedgerPostgres {
							envelope, err = helpers.QueryLedgerRawEnvelope(context.Background(), journey.h.ReaderDB, events[0].ID)
							Expect(err).NotTo(HaveOccurred())
						} else {
							encoded, err := json.Marshal(events[0])
							Expect(err).NotTo(HaveOccurred())
							Expect(json.Unmarshal(encoded, &envelope)).To(Succeed())
						}
						Eventually(func() error {
							for _, record := range receiver.LedgerRecords() {
								for _, attribute := range record.Record.Attributes {
									if attribute.Key == "id" && attribute.Value.GetStringValue() == events[0].ID.String() {
										return helpers.LedgerTelemetryEnvelope(events[0], envelope, record)
									}
								}
							}
							return errors.New("retained event has no ledger telemetry copy")
						}, 5*time.Second, 20*time.Millisecond).Should(Succeed())
						if name == "credential-generated" {
							Eventually(func() bool {
								for _, record := range receiver.Records() {
									if record.Scope.GetName() == helpers.LedgerOTLPScope {
										continue
									}
									for _, attribute := range record.Record.Attributes {
										if attribute.Key == "event" && attribute.Value.GetStringValue() == "CredentialGenerated" {
											return true
										}
									}
								}
								return false
							}, 10*time.Second, 20*time.Millisecond).Should(BeTrue(), "existing telemetry must remain available alongside the ledger copy")
						}
						Expect(journey.h.Close()).To(Succeed())
						journey.assertCredentialFree(receiver.LedgerRecords())
						slog.SetDefault(previous)
						Expect(receiver.Close()).To(Succeed())
					}
				}
			}
		})

		// US4-AS2 from specs/048-business-event-ledger/spec.md.
		It("retains mandatory recording and existing telemetry when only ledger copying is disabled", func() {
			for _, backend := range bootstrap.LedgerBackends() {
				receiver, err := helpers.NewOTLPReceiver()
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(receiver.Close)
				journey := newConfiguredLedgerJourney(backend, "credential-generated", nil, func(config *ports.Config) {
					config.Telemetry = fixtures.TelemetryGRPCConfig().Telemetry
					config.Telemetry.Traces.Enabled = false
					config.Telemetry.Logs.Enabled = true
					config.Telemetry.Exporter.Endpoint = receiver.Endpoint()
					config.BusinessEvents.TelemetryCopyEnabled = false
				}, nil)
				expected := journey.action("credential-generated")
				events := journey.query("credential-generated", *expected.Subject)
				Expect(events).To(HaveLen(1))
				Expect(events[0]).To(matchers.HaveBusinessEventEnvelope(expected))
				Expect(journey.h.Close()).To(Succeed())
				Expect(receiver.LedgerRecords()).To(BeEmpty())
				legacy := false
				for _, record := range receiver.Records() {
					for _, attribute := range record.Record.Attributes {
						if attribute.Key == "event" && attribute.Value.GetStringValue() == "CredentialGenerated" {
							legacy = true
						}
					}
				}
				Expect(legacy).To(BeTrue(), "existing CredentialGenerated telemetry is missing")
			}
		})
	})
})
