package e2e_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const (
	authzenPublicURL     = "https://broker.example.com"
	authzenRepoResource  = "https://authzen.example.com/repos"
	authzenOtherResource = "https://authzen.example.com/wiki"
	authzenSubjectMarker = "AUTHZEN_SUBJECT_SENTINEL"
	authzenPeerMarker    = "AUTHZEN_ASSERTION_SENTINEL"
)

// authzenFixture seeds one delegation through production storage and boots the production
// end-user server. Alice's grant covers the repo service through two permission sets; the
// agent ceiling allows only read:repo. It deliberately creates no provider session.
type authzenFixture struct {
	Storage            *storageadapter.Adapter
	Enduser            *bootstrap.TestServer
	Upstream           *helpers.MockUpstreamOAuth2Server
	Config             *ports.Config
	Logs               *bootstrap.BufferedLogCapture
	EvaluationObserver *bootstrap.EvaluationSecurityObserver
	Principal          id.Principal
	Agent              *storagedomain.Agent
	OtherAgent         *storagedomain.Agent
	Reader             *storagedomain.PermissionSet
	Writer             *storagedomain.PermissionSet
	Repo               *model.ThirdpartyOAuth2ProviderEntity
	Wiki               *model.ThirdpartyOAuth2ProviderEntity
	Grant              *storagedomain.UserGrant
	SubjectToken       string
	ClientAssertion    string
	client             *http.Client
	storageFactory     *bootstrap.StorageFactory
	logger             *slog.Logger
}

func newAuthZENFixture(configure func(*ports.Config)) *authzenFixture {
	f := newAuthZENData(nil)
	f.Boot(configure)
	return f
}

// newAuthZENData seeds data into storage (memory when nil) without starting servers.
func newAuthZENData(storage *storageadapter.Adapter) *authzenFixture {
	f := &authzenFixture{Principal: id.Principal("alice@example.com"), client: &http.Client{Timeout: 10 * time.Second}}
	f.logger, f.Logs = bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
	f.Upstream = helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
	DeferCleanup(f.Upstream.Close)
	f.storageFactory = bootstrap.NewStorageFactory(f.logger)
	if storage == nil {
		var err error
		storage, err = f.storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(f.storageFactory.CloseStorage(storage)).To(Succeed()) })
	}
	f.Storage = storage
	ctx := context.Background()

	f.Repo = fixtures.ServiceWithID(id.NewServiceID().String())
	f.Repo.CanonicalID = ptr.To("repo-api")
	f.Repo.ProtectedResources = []string{authzenRepoResource}
	f.Wiki = fixtures.ServiceWithID(id.NewServiceID().String())
	f.Wiki.ProtectedResources = []string{authzenOtherResource}
	for _, service := range []*model.ThirdpartyOAuth2ProviderEntity{f.Repo, f.Wiki} {
		service.Endpoints.TokenEndpoint = f.Upstream.URL() + "/oauth/token"
		service.Endpoints.AuthorizeEndpoint = f.Upstream.URL() + "/oauth/authorize"
		Expect(f.Storage.Services().Create(ctx, service)).To(Succeed())
	}

	now := time.Now().UTC()
	f.Reader = &storagedomain.PermissionSet{
		ID: id.NewPermissionSetID(), CanonicalID: ptr.To("repo-reader"), Name: "Repo reader", Description: "Read repositories",
		ServiceScopes: []storagedomain.ServiceScope{{ServiceID: f.Repo.ID, Scopes: []string{"read:repo"}, RequirementType: storagedomain.RequirementTypeOptional}},
		CreatedAt:     now, UpdatedAt: now,
	}
	f.Writer = &storagedomain.PermissionSet{
		ID: id.NewPermissionSetID(), Name: "Repo writer", Description: "Write repositories and read the wiki",
		ServiceScopes: []storagedomain.ServiceScope{
			{ServiceID: f.Repo.ID, Scopes: []string{"write:repo", "read:repo"}, RequirementType: storagedomain.RequirementTypeOptional},
		},
		CreatedAt: now, UpdatedAt: now,
	}
	for _, ps := range []*storagedomain.PermissionSet{f.Reader, f.Writer} {
		Expect(f.Storage.PermissionSets().Create(ctx, ps)).To(Succeed())
	}

	f.Agent = fixtures.ValidAgent()
	f.Agent.CanonicalID = ptr.To("data-assistant")
	f.Agent.PermissionSets = []storagedomain.AgentPermissionSetEntry{
		{PermissionSetID: f.Reader.ID, RequirementType: storagedomain.RequirementTypeOptional},
		{PermissionSetID: f.Writer.ID, RequirementType: storagedomain.RequirementTypeOptional},
	}
	f.Agent.ServiceRequirements = []storagedomain.ServiceRequirement{
		{ServiceID: f.Repo.ID, RequiredScopes: []string{"read:repo"}, RequirementType: storagedomain.RequirementTypeOptional},
		{ServiceID: f.Wiki.ID, RequireAllScopes: true, RequirementType: storagedomain.RequirementTypeOptional},
	}
	Expect(f.Storage.Agents().Create(ctx, f.Agent)).To(Succeed())
	f.OtherAgent = fixtures.ValidAgent()
	f.OtherAgent.ClientID = ptr.To(id.ClientID("authzen-other-agent"))
	Expect(f.Storage.Agents().Create(ctx, f.OtherAgent)).To(Succeed())

	f.Grant = &storagedomain.UserGrant{
		ID: id.NewGrantID(), Principal: f.Principal, AgentID: f.Agent.ID,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: f.Reader.ID, IncludedServiceIDs: []id.ServiceID{f.Repo.ID}},
			{PermissionSetID: f.Writer.ID, IncludedServiceIDs: []id.ServiceID{f.Repo.ID, f.Wiki.ID}},
		},
		CreatedAt: now, UpdatedAt: now,
	}
	Expect(f.Storage.UserGrants().Create(ctx, f.Grant)).To(Succeed())

	f.SubjectToken = f.SignSubject(now.Add(time.Hour))
	f.ClientAssertion = f.SignAssertion(now.Add(time.Hour))
	return f
}

// Boot starts the production broker for the seeded storage.
func (f *authzenFixture) Boot(configure func(*ports.Config)) {
	f.Config = fixtures.OAuth2ConfigWithTokenExchange(f.Upstream.URL())
	f.Config.Server.EndUser.PublicURL = authzenPublicURL
	if configure != nil {
		configure(f.Config)
	}
	application, err := bootstrap.NewServerFactory(f.Config, f.logger).BuildApp(f.Storage)
	Expect(err).NotTo(HaveOccurred())
	f.EvaluationObserver = bootstrap.ObserveAuthZENEvaluation(application)
	f.Enduser, err = bootstrap.NewEndUserTestServer(application, f.logger)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(f.Enduser.Close)
	f.Logs.Reset()
}

// BootAligned starts the production broker with public_url equal to its listening origin,
// so a gateway can discover and verify the PDP identifier.
func (f *authzenFixture) BootAligned() {
	f.Config = fixtures.OAuth2ConfigWithTokenExchange(f.Upstream.URL())
	builder, err := bootstrap.NewTestServerBuilder(f.Config, f.Storage, bootstrap.NewServerFactory(f.Config, f.logger), f.logger)
	Expect(err).NotTo(HaveOccurred())
	f.Enduser, err = builder.Build()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(f.Enduser.Close)
	f.Logs.Reset()
}

// EvaluationCount reports how many access evaluations the broker answered.
func (f *authzenFixture) EvaluationCount() int {
	records, err := f.Logs.Records()
	Expect(err).NotTo(HaveOccurred())
	count := 0
	for _, record := range records {
		if record["token_exchange.operation"] == "access_evaluation" {
			count++
		}
	}
	return count
}

// ExchangeCount reports how many token exchanges the broker answered.
func (f *authzenFixture) ExchangeCount() int {
	records, err := f.Logs.Records()
	Expect(err).NotTo(HaveOccurred())
	count := 0
	for _, record := range records {
		if _, ok := record["token_exchange.exchange_kind"]; ok && record["token_exchange.operation"] == nil {
			count++
		}
	}
	return count
}

func (f *authzenFixture) SignSubject(exp time.Time) string {
	token, err := helpers.SignTestJWT(map[string]interface{}{
		"sub": f.Principal.String(), "azp": f.Agent.ID.String(), "iss": f.Upstream.URL(), "aud": "token-exchange-broker",
		"iat": time.Now().Add(-2 * time.Hour).Unix(), "exp": exp.Unix(), "marker": authzenSubjectMarker,
	}, f.Upstream.GetPrivateKeyPEM())
	Expect(err).NotTo(HaveOccurred())
	return token
}

func (f *authzenFixture) SignAssertion(exp time.Time) string {
	return f.SignAssertionFor("authzen-gateway", exp)
}

func (f *authzenFixture) SignAssertionFor(subject string, exp time.Time) string {
	return f.signAssertionWith(f.Upstream.GetPrivateKeyPEM(), subject, exp)
}

// SignUntrustedAssertion signs an otherwise valid gateway assertion with a fresh key that
// the configured trust anchor does not publish.
func (f *authzenFixture) SignUntrustedAssertion(exp time.Time) string {
	privateKeyPEM, _, err := helpers.GenerateTestRSAKeyPair()
	Expect(err).NotTo(HaveOccurred())
	return f.signAssertionWith(privateKeyPEM, "authzen-gateway", exp)
}

func (f *authzenFixture) signAssertionWith(privateKeyPEM, subject string, exp time.Time) string {
	token, err := helpers.SignTestJWT(map[string]interface{}{
		"sub": subject, "iss": f.Upstream.URL(), "aud": "token-exchange-broker",
		"iat": time.Now().Add(-2 * time.Hour).Unix(), "exp": exp.Unix(), "marker": authzenPeerMarker,
	}, privateKeyPEM)
	Expect(err).NotTo(HaveOccurred())
	return token
}

// Evaluate posts an evaluation with the fixture's assertion and request ID.
func (f *authzenFixture) Evaluate(resource string, context map[string]any) *helpers.AuthZENResponse {
	return f.EvaluateRaw(f.ClientAssertion, helpers.AuthZENEvaluationBody(f.SubjectToken, resource, context))
}

func (f *authzenFixture) EvaluateRaw(assertion string, body []byte) *helpers.AuthZENResponse {
	resp, err := helpers.DoAuthZENEvaluation(f.client, f.Enduser.BaseURL(), assertion, "delegation-001", body)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.Header.Get("X-Request-ID")).To(Equal("delegation-001"), "the supplied request ID is echoed on every path")
	f.ExpectCredentialFree(resp.Raw, assertion, jwtSubjectOf(body))
	return resp
}

// jwtSubjectOf returns the JWT subject token of an evaluation body, or empty when the
// body is malformed or its subject is not a JWT.
func jwtSubjectOf(body []byte) string {
	var request struct {
		Subject struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"subject"`
	}
	if json.Unmarshal(body, &request) != nil || request.Subject.Type != helpers.AuthZENSubjectTypeJWT {
		return ""
	}
	return request.Subject.ID
}

func (f *authzenFixture) ExpectEvaluationIdentity(resp *helpers.AuthZENResponse, actor, peer string, reachedService bool) {
	parts := strings.Split(resp.Header.Get("traceresponse"), "-")
	Expect(parts).To(HaveLen(4))
	traceID := parts[1]
	var handlerAudits, routeAudits, perimeterLogs []map[string]any
	Eventually(func(g Gomega) {
		records, err := f.Logs.Records()
		g.Expect(err).NotTo(HaveOccurred())
		handlerAudits, routeAudits, perimeterLogs = nil, nil, nil
		for _, record := range records {
			if record["trace_id"] != traceID {
				continue
			}
			_, hasEvent := record["event"]
			_, hasClientIP := record["client_ip"]
			switch {
			case record["token_exchange.operation"] == "access_evaluation":
				handlerAudits = append(handlerAudits, record)
			case record["event"] == "tokenexchange.access_evaluation_request.completed":
				routeAudits = append(routeAudits, record)
			case !hasEvent && hasClientIP:
				perimeterLogs = append(perimeterLogs, record)
			}
		}
		g.Expect(handlerAudits).To(HaveLen(1), "exactly one in-handler audit shares the response trace ID")
		g.Expect(routeAudits).To(HaveLen(1), "exactly one route audit shares the response trace ID")
		g.Expect(perimeterLogs).To(HaveLen(1), "exactly one perimeter access log shares the response trace ID")
	}).Should(Succeed())
	Expect(handlerAudits[0]).To(HaveKey("event"))
	for _, audit := range []map[string]any{handlerAudits[0], routeAudits[0]} {
		Expect(audit).To(HaveKeyWithValue("principal", actor), "audits name the finalized Actor")
	}
	Expect(routeAudits[0]).To(HaveKeyWithValue("path", "/access/v1/evaluation"))
	Expect(perimeterLogs[0]).To(HaveKeyWithValue("path", "/access/v1/evaluation"))
	for _, record := range []map[string]any{handlerAudits[0], routeAudits[0], perimeterLogs[0]} {
		Expect(record).To(HaveKeyWithValue("actor", actor))
		if peer == "" {
			Expect(record).NotTo(HaveKey("calling_peer"))
		} else {
			Expect(record).To(HaveKeyWithValue("calling_peer", peer))
		}
	}
	observed := false
	for _, observation := range f.EvaluationObserver.Snapshot() {
		Expect(observation.Holder).NotTo(BeNil())
		if observation.Holder.Capture().TraceID != traceID {
			continue
		}
		observed = true
		Expect(observation.FinalizedBeforeEvaluation).To(BeFalse(), "only the shared resolver may establish verified identities")
		sc, finalized := observation.Holder.Finalized()
		Expect(finalized).To(BeTrue())
		Expect(sc.TraceID).To(Equal(traceID))
		Expect(sc.Actor).To(Equal(id.Principal(actor)))
		Expect(sc.CallingPeer).To(Equal(peer))
		second, changed := observation.Holder.Finalize("must-not-replace-actor", "must-not-replace-peer")
		Expect(changed).To(BeFalse(), "the context must finalize exactly once")
		Expect(second).To(Equal(sc))
	}
	Expect(observed).To(Equal(reachedService))
}

// Exchange performs the equivalent RFC 8693 exchange for comparison.
func (f *authzenFixture) Exchange(resource string) (int, map[string]any) {
	form := url.Values{
		"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":         {f.SubjectToken},
		"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
		"client_assertion":      {f.ClientAssertion},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"resource":              {resource},
	}
	resp, err := f.Enduser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
	return resp.StatusCode, body
}

func (f *authzenFixture) SeedSession(service *model.ThirdpartyOAuth2ProviderEntity, scopes []string) {
	session := fixtures.SessionForServiceWithScopes(f.Principal.String(), service.ID.String(), scopes)
	Expect(f.Storage.UserSessions().Create(context.Background(), session)).To(Succeed())
}

func (f *authzenFixture) RevokeGrant() {
	resp, err := f.Enduser.DirectRequest(http.MethodDelete, "/api/consent/agents/"+f.Agent.ID.String()+"/grants", f.Principal.String(), nil, nil)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
}

func (f *authzenFixture) ConsentToRepo() {
	body, err := json.Marshal(map[string]any{
		"granted_permission_sets": map[string][]string{
			f.Reader.ID.String(): {f.Repo.ID.String()},
			f.Writer.ID.String(): {f.Repo.ID.String()},
		},
	})
	Expect(err).NotTo(HaveOccurred())
	resp, err := f.Enduser.AuthenticatedPOST("/api/consent/agents/"+f.Agent.ID.String()+"/grants", f.Principal.String(), "application/json", strings.NewReader(string(body)))
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
}

func (f *authzenFixture) SaveGrant() {
	Expect(f.Storage.UserGrants().Update(context.Background(), f.Grant)).To(Succeed())
}

func (f *authzenFixture) SaveAgent() {
	Expect(f.Storage.Agents().Update(context.Background(), f.Agent)).To(Succeed())
}

// ExpectCredentialFree asserts that neither the fixture credentials, the credentials a
// request actually sent, nor their private claims leak into the response body or captured
// logs. Empty sent credentials are skipped.
func (f *authzenFixture) ExpectCredentialFree(raw string, sent ...string) {
	logs := f.Logs.Raw()
	secrets := []string{f.SubjectToken, f.ClientAssertion, authzenSubjectMarker, authzenPeerMarker}
	for _, credential := range sent {
		if credential != "" {
			secrets = append(secrets, credential)
		}
	}
	for _, secret := range secrets {
		Expect(raw).NotTo(ContainSubstring(secret))
		Expect(logs).NotTo(ContainSubstring(secret))
	}
}

func (f *authzenFixture) ConsentURL() string {
	return authzenPublicURL + "/agents/" + f.Agent.ID.String()
}

// ExpectDenial asserts a consent-recovery denial with the given detail.
func (f *authzenFixture) ExpectDenial(resp *helpers.AuthZENResponse, code string) {
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(resp.Header.Get("Cache-Control")).To(Equal("no-store"))
	Expect(resp.Body).To(Equal(map[string]any{
		"decision": false,
		"context": map[string]any{
			"reason_admin": map[string]any{"code": code},
			"reason_user":  map[string]any{"consent_url": f.ConsentURL()},
		},
	}))
}

// ExpectError asserts a non-decision error response.
func expectAuthZENError(resp *helpers.AuthZENResponse, status int, code string) {
	Expect(resp.StatusCode).To(Equal(status))
	Expect(resp.Header.Get("Cache-Control")).To(Equal("no-store"))
	Expect(resp.Body).To(HaveKeyWithValue("error", code))
	Expect(resp.Body).NotTo(HaveKey("decision"))
}

func permitContext(resp *helpers.AuthZENResponse) map[string]any {
	Expect(resp.StatusCode).To(Equal(http.StatusOK), resp.Raw)
	Expect(resp.Body).To(HaveKeyWithValue("decision", true), resp.Raw)
	return resp.Body["context"].(map[string]any)
}
