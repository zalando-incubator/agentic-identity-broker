package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const grantDiagnosticPrivateClaim = "RAW_CLAIM_SENTINEL"

var grantDiagnosticPrivateFields = regexp.MustCompile(`(?i)"[^"]*(subject_token|client_assertion|access_token|refresh_token|id_token|credential|secret|unprojected_debug|resource|endpoint|recovery_uri|error|cause|exception|provider_response|response_body)[^"]*"\s*:`)

// grantDiagnosticsFixture uses the production memory adapters and dual HTTP stack.
// It starts with an active grant and deliberately no sessions.
type grantDiagnosticsFixture struct {
	Storage         *storageadapter.Adapter
	Enduser         *bootstrap.TestServer
	Admin           *bootstrap.TestServer
	Upstream        *helpers.MockUpstreamOAuth2Server
	Config          *ports.Config
	Logs            *bootstrap.BufferedLogCapture
	Recorder        *tracetest.SpanRecorder
	Principal       id.Principal
	Agent           *storagedomain.Agent
	PermissionSet   *storagedomain.PermissionSet
	Grant           *storagedomain.UserGrant
	S1              *model.ThirdpartyOAuth2ProviderEntity
	S2              *model.ThirdpartyOAuth2ProviderEntity
	S0              *model.ThirdpartyOAuth2ProviderEntity
	SubjectToken    string
	ClientAssertion string
}

func newGrantDiagnosticsFixture() *grantDiagnosticsFixture {
	f := &grantDiagnosticsFixture{Principal: id.Principal("grant-diagnostics-user")}
	logger, capture := bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
	f.Logs = capture
	f.Upstream = helpers.NewMockUpstreamOAuth2Server().
		WithSuccessfulTokenResponse().
		WithAccessToken("PROVIDER_ACCESS_SENTINEL").
		WithRefreshToken("PROVIDER_REFRESH_SENTINEL")
	DeferCleanup(f.Upstream.Close)

	storageFactory := bootstrap.NewStorageFactory(logger)
	var err error
	f.Storage, err = storageFactory.NewTestStorage()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(storageFactory.CloseStorage(f.Storage)).To(Succeed()) })

	previousProvider := otel.GetTracerProvider()
	provider, recorder := bootstrap.NewInMemoryTracerProvider()
	f.Recorder = recorder
	DeferCleanup(func() {
		defer otel.SetTracerProvider(previousProvider)
		Expect(provider.Shutdown(context.Background())).To(Succeed())
	})

	f.Config = fixtures.EnableTelemetryTracing(fixtures.OAuth2ConfigWithTokenExchange(f.Upstream.URL()))
	f.S1 = fixtures.ServiceWithID(id.NewServiceID().String())
	f.S2 = fixtures.ServiceWithID(id.NewServiceID().String())
	f.S0 = fixtures.ServiceWithID(id.NewServiceID().String())
	f.S0.Scopes = []model.OAuthScope{}
	ctx := context.Background()
	for i, service := range []*model.ThirdpartyOAuth2ProviderEntity{f.S1, f.S2, f.S0} {
		service.ProtectedResources = []string{"https://grant-diagnostics.example.com/" + []string{"s1", "s2", "s0"}[i]}
		service.Endpoints.TokenEndpoint = f.Upstream.URL() + "/oauth/token"
		service.Endpoints.AuthorizeEndpoint = f.Upstream.URL() + "/oauth/authorize"
		Expect(f.Storage.Services().Create(ctx, service)).To(Succeed())
	}

	now := time.Now().UTC()
	f.PermissionSet = &storagedomain.PermissionSet{
		ID:          id.NewPermissionSetID(),
		Name:        "Grant diagnostics",
		Description: "Optional services for grant denial classification",
		ServiceScopes: []storagedomain.ServiceScope{
			{ServiceID: f.S1.ID, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional},
			{ServiceID: f.S2.ID, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional},
			{ServiceID: f.S0.ID, Scopes: []string{}, RequirementType: storagedomain.RequirementTypeOptional},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	Expect(f.Storage.PermissionSets().Create(ctx, f.PermissionSet)).To(Succeed())
	f.Agent = fixtures.ValidAgent()
	f.Agent.PermissionSets = []storagedomain.AgentPermissionSetEntry{{
		PermissionSetID: f.PermissionSet.ID,
		RequirementType: storagedomain.RequirementTypeMandatory,
	}}
	f.Agent.ServiceRequirements = []storagedomain.ServiceRequirement{
		{ServiceID: f.S1.ID, RequiredScopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional, RequireAllScopes: false},
		{ServiceID: f.S2.ID, RequiredScopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional, RequireAllScopes: false},
		{ServiceID: f.S0.ID, RequiredScopes: []string{}, RequirementType: storagedomain.RequirementTypeOptional, RequireAllScopes: false},
	}
	Expect(f.Storage.Agents().Create(ctx, f.Agent)).To(Succeed())
	validUntil := now.Add(time.Hour)
	f.Grant = &storagedomain.UserGrant{
		ID:         id.NewGrantID(),
		Principal:  f.Principal,
		AgentID:    f.Agent.ID,
		ValidUntil: &validUntil,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{
			PermissionSetID:    f.PermissionSet.ID,
			IncludedServiceIDs: []id.ServiceID{f.S1.ID, f.S2.ID, f.S0.ID},
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	Expect(f.Storage.UserGrants().Create(ctx, f.Grant)).To(Succeed())

	f.SubjectToken, err = helpers.SignTestJWT(map[string]interface{}{
		"sub":               f.Principal.String(),
		"azp":               f.Agent.ID.String(),
		"iss":               f.Upstream.URL(),
		"aud":               "token-exchange-broker",
		"iat":               now.Unix(),
		"exp":               now.Add(time.Hour).Unix(),
		"unprojected_debug": grantDiagnosticPrivateClaim,
	}, f.Upstream.GetPrivateKeyPEM())
	Expect(err).NotTo(HaveOccurred())
	f.ClientAssertion, err = helpers.SignTestJWT(map[string]interface{}{
		"sub":               "grant-diagnostics-gateway",
		"iss":               f.Upstream.URL(),
		"aud":               "token-exchange-broker",
		"iat":               now.Unix(),
		"exp":               now.Add(time.Hour).Unix(),
		"unprojected_debug": grantDiagnosticPrivateClaim,
	}, f.Upstream.GetPrivateKeyPEM())
	Expect(err).NotTo(HaveOccurred())

	application, err := bootstrap.NewServerFactory(f.Config, logger).BuildAppWithTracerProvider(f.Storage, provider)
	Expect(err).NotTo(HaveOccurred())
	f.Enduser, err = bootstrap.NewEndUserTestServer(application, logger)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(f.Enduser.Close)
	f.Admin, err = bootstrap.NewAdminTestServer(application, logger)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(f.Admin.Close)
	f.Logs.Reset()
	return f
}

func (f *grantDiagnosticsFixture) Exchange(service *model.ThirdpartyOAuth2ProviderEntity) *http.Response {
	form := url.Values{
		"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":         {f.SubjectToken},
		"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
		"client_assertion":      {f.ClientAssertion},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"resource":              {service.ProtectedResources[0]},
	}
	response, err := f.Enduser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	Expect(err).NotTo(HaveOccurred())
	return response
}

func (f *grantDiagnosticsFixture) SeedSession(service *model.ThirdpartyOAuth2ProviderEntity, scopes []string) {
	session := fixtures.SessionForServiceWithScopes(f.Principal.String(), service.ID.String(), scopes)
	Expect(f.Storage.UserSessions().Create(context.Background(), session)).To(Succeed())
}

func (f *grantDiagnosticsFixture) ExchangeRecords() []map[string]any {
	records, err := f.Logs.Records()
	Expect(err).NotTo(HaveOccurred())
	var observations []map[string]any
	for _, record := range records {
		if _, ok := record["token_exchange.outcome"]; ok {
			observations = append(observations, record)
		}
	}
	return observations
}

func (f *grantDiagnosticsFixture) DeletionRecords() []map[string]any {
	records, err := f.Logs.Records()
	Expect(err).NotTo(HaveOccurred())
	var deletions []map[string]any
	for _, record := range records {
		switch record["msg"] {
		case "grant revoked", "agent deleted", "PermissionSetDeleted", "permission set deleted":
			f.AssertPrivateObservation(record)
			deletions = append(deletions, record)
		}
	}
	return deletions
}

func (f *grantDiagnosticsFixture) Delete(server *bootstrap.TestServer, path string, status int) {
	response, err := server.DirectRequest(http.MethodDelete, path, f.Principal.String(), nil, nil)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	Expect(response.StatusCode).To(Equal(status))
	if status == http.StatusNoContent {
		body, err := io.ReadAll(response.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(BeEmpty(), "successful DELETE must retain its empty 204 response")
	}
}

func (f *grantDiagnosticsFixture) ExchangeSpans() []sdktrace.ReadOnlySpan {
	var observations []sdktrace.ReadOnlySpan
	for _, span := range f.Recorder.Ended() {
		for _, attr := range span.Attributes() {
			if attr.Key == "token_exchange.outcome" {
				observations = append(observations, span)
				break
			}
		}
	}
	return observations
}

func (f *grantDiagnosticsFixture) ExchangeObservationFields() []map[string]any {
	records := f.ExchangeRecords()
	Expect(len(records)).To(Equal(1), "one exchange observation must describe the HTTP request")
	Eventually(func() int { return len(f.ExchangeSpans()) }).Should(Equal(1))
	span := f.ExchangeSpans()[0]
	spanFields := make(map[string]any, len(span.Attributes()))
	for _, attr := range span.Attributes() {
		spanFields[string(attr.Key)] = attr.Value.AsInterface()
	}
	f.AssertPrivateObservation(records[0])
	f.AssertPrivateObservation(spanFields)
	f.AssertPrivateObservation(map[string]any{"span_status": span.Status().Description})
	for _, event := range span.Events() {
		eventFields := map[string]any{"span_event": event.Name}
		for _, attr := range event.Attributes {
			eventFields[string(attr.Key)] = attr.Value.AsInterface()
		}
		f.AssertPrivateObservation(eventFields)
	}
	Expect(records[0]["actor"]).To(Equal(f.Principal.String()))
	Expect(records[0]["calling_peer"]).To(Equal("grant-diagnostics-gateway"))
	Expect(span.SpanContext().IsValid()).To(BeTrue())
	Expect(records[0]["trace_id"]).To(Equal(span.SpanContext().TraceID().String()))
	return []map[string]any{records[0], spanFields}
}

func (f *grantDiagnosticsFixture) AssertAuthorizationContext(observation map[string]any, agentID id.AgentID, grant *storagedomain.UserGrant) {
	if agentID.IsZero() {
		_, present := observation["token_exchange.agent.id"]
		Expect(present).To(BeFalse(), "an unregistered candidate must not become observed agent identity")
	} else {
		Expect(observation["token_exchange.agent.id"]).To(Equal(agentID.String()))
	}
	if grant == nil {
		for _, key := range []string{"token_exchange.grant.id", "token_exchange.grant.updated_at", "token_exchange.grant.valid_until"} {
			_, present := observation[key]
			Expect(present).To(BeFalse(), key+" must be omitted without a found grant")
		}
		return
	}
	Expect(observation["token_exchange.grant.id"]).To(Equal(grant.ID.String()))
	Expect(observation["token_exchange.grant.updated_at"]).To(Equal(grant.UpdatedAt.UTC().Format(time.RFC3339Nano)))
	if grant.ValidUntil == nil {
		_, present := observation["token_exchange.grant.valid_until"]
		Expect(present).To(BeFalse(), "an indefinite grant must omit expiry rather than inventing a sentinel")
	} else {
		Expect(observation["token_exchange.grant.valid_until"]).To(Equal(grant.ValidUntil.UTC().Format(time.RFC3339Nano)))
	}
}

func (f *grantDiagnosticsFixture) AssertPrivateObservation(observation map[string]any) {
	encoded, err := json.Marshal(observation)
	Expect(err).NotTo(HaveOccurred())
	Expect(grantDiagnosticPrivateFields.Match(encoded)).To(BeFalse(), "observation must not contain private fields")
	forbidden := []string{
		f.SubjectToken,
		f.ClientAssertion,
		f.Upstream.GetPrivateKeyPEM(),
		grantDiagnosticPrivateClaim,
		"PROVIDER_ACCESS_SENTINEL",
		"PROVIDER_REFRESH_SENTINEL",
		f.Config.Server.EndUser.PublicURL + "/agents/" + f.Agent.ID.String(),
	}
	for _, service := range []*model.ThirdpartyOAuth2ProviderEntity{f.S1, f.S2, f.S0} {
		forbidden = append(forbidden,
			service.ProtectedResources[0],
			service.Endpoints.TokenEndpoint,
			service.Endpoints.AuthorizeEndpoint,
			"test-secret-"+service.ID.String(),
			"token-"+service.ID.String(),
			"refresh-"+service.ID.String(),
		)
	}
	for _, value := range forbidden {
		Expect(strings.Contains(string(encoded), value)).To(BeFalse(), "observation must not contain private material")
	}
}

var _ = Describe("Grant Denial Classification", func() {
	var fixture *grantDiagnosticsFixture

	BeforeEach(func() {
		fixture = newGrantDiagnosticsFixture()
	})

	// GD-C1 from specs/013-token-exchange/spec.md.
	DescribeTable("[GD-C1] preserves the OAuth denial and classifies the authorization limit before session retrieval",
		func(configure func(*grantDiagnosticsFixture), detail string) {
			configure(fixture)
			sessions, err := fixture.Storage.UserSessions().ListByPrincipal(context.Background(), fixture.Principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(sessions)).To(BeZero(), "denial fixture deliberately has no token-vault sessions")

			response := fixture.Exchange(fixture.S2)
			defer func() { _ = response.Body.Close() }()
			Expect(response.StatusCode).To(Equal(http.StatusForbidden))
			var body map[string]any
			Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
			Expect(len(body)).To(Equal(3), "denial wire schema must contain only the established OAuth error fields")
			Expect(body["error"]).To(Equal("access_denied"))
			Expect(body["error_description"]).To(Equal("User authorization is insufficient. Please re-consent."))
			Expect(body["error_uri"]).To(Equal(fixture.Config.Server.EndUser.PublicURL + "/agents/" + fixture.Agent.ID.String()))
			Expect(fixture.Upstream.GetTokenCalled()).To(BeFalse(), "authorization denial must not contact the provider token endpoint")
			Expect(len(fixture.Upstream.GetTokenRequests())).To(BeZero())

			for _, observation := range fixture.ExchangeObservationFields() {
				Expect(observation["token_exchange.failure_detail"]).To(Equal(detail))
				Expect(observation["token_exchange.outcome"]).To(Equal("authorization_denied"))
				Expect(observation["token_exchange.failure_stage"]).To(Equal("grant_authorization"))
				Expect(observation["token_exchange.recovery_action"]).To(Equal("reconsent"))
				Expect(observation["token_exchange.recovery_target"]).To(Equal("consent"))
				Expect(observation["token_exchange.service.id"]).To(Equal(fixture.S2.ID.String()))
				fixture.AssertAuthorizationContext(observation, fixture.Agent.ID, fixture.Grant)
			}
		},
		Entry("when consent omits S2", func(f *grantDiagnosticsFixture) {
			f.Grant.GrantedPermissionSets[0].IncludedServiceIDs = []id.ServiceID{f.S1.ID, f.S0.ID}
			Expect(f.Storage.UserGrants().Update(context.Background(), f.Grant)).To(Succeed())
		}, "grant_service_omitted"),
		Entry("when nonempty agent requirements exclude included and defined S2", func(f *grantDiagnosticsFixture) {
			f.Agent.ServiceRequirements = []storagedomain.ServiceRequirement{f.Agent.ServiceRequirements[0], f.Agent.ServiceRequirements[2]}
			Expect(f.Storage.Agents().Update(context.Background(), f.Agent)).To(Succeed())
		}, "grant_service_requirement_excluded"),
		Entry("when S2's nonempty scope union has no scope below the explicit agent ceiling", func(f *grantDiagnosticsFixture) {
			f.PermissionSet.ServiceScopes[1].Scopes = []string{"write"}
			Expect(f.Storage.PermissionSets().Update(context.Background(), f.PermissionSet)).To(Succeed())
		}, "grant_scope_intersection_empty"),
	)
})

var _ = Describe("Grant Observation Context", func() {
	var fixture *grantDiagnosticsFixture

	BeforeEach(func() {
		fixture = newGrantDiagnosticsFixture()
	})

	// GD-O1 from specs/013-token-exchange/spec.md.
	DescribeTable("[GD-O1] observes only resolved agent and found-grant metadata without changing the wire response",
		func(configure func(*grantDiagnosticsFixture), status int, detail string, registeredAgent, foundGrant bool) {
			configure(fixture)
			var expectedAgentID id.AgentID
			if registeredAgent {
				expectedAgentID = fixture.Agent.ID
			}
			var storedGrant *storagedomain.UserGrant
			if foundGrant {
				var err error
				storedGrant, err = fixture.Storage.UserGrants().Get(context.Background(), fixture.Grant.ID)
				Expect(err).NotTo(HaveOccurred())
			}

			response := fixture.Exchange(fixture.S1)
			defer func() { _ = response.Body.Close() }()
			Expect(response.StatusCode).To(Equal(status))
			var body map[string]any
			Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
			outcome := "success"
			switch status {
			case http.StatusOK:
				keys := make([]string, 0, len(body))
				for key := range body {
					keys = append(keys, key)
				}
				Expect(keys).To(ConsistOf("access_token", "token_type", "issued_token_type", "expires_in", "scope", "principal", "agent_id", "granted_permission_sets"), "authorization context must not add fields to the established JSON response")
				accessToken, ok := body["access_token"].(string)
				Expect(ok && accessToken == "token-"+fixture.S1.ID.String()).To(BeTrue(), "the existing encrypted session token must be returned without printing it")
				Expect(body["token_type"]).To(Equal("Bearer"))
				Expect(body["issued_token_type"]).To(Equal("urn:ietf:params:oauth:token-type:access_token"))
				Expect(body["expires_in"]).To(BeNumerically(">", 0))
				Expect(body["scope"]).To(Equal("read"))
				Expect(body["principal"]).To(Equal(fixture.Principal.String()))
				Expect(body["agent_id"]).To(Equal(fixture.Agent.ID.String()))
				Expect(body["granted_permission_sets"]).To(Equal(map[string]any{
					fixture.PermissionSet.ID.String(): []any{fixture.S1.ID.String(), fixture.S2.ID.String(), fixture.S0.ID.String()},
				}))
			case http.StatusForbidden:
				outcome = "authorization_denied"
				Expect(len(body)).To(Equal(3), "denial JSON must retain only the established OAuth fields")
				Expect(body["error"]).To(Equal("access_denied"))
				Expect(body["error_description"]).To(Equal("User authorization is insufficient. Please re-consent."))
				Expect(body["error_uri"]).To(Equal(fixture.Config.Server.EndUser.PublicURL + "/agents/" + fixture.Agent.ID.String()))
			case http.StatusBadRequest:
				outcome = "reauth_required"
				Expect(len(body)).To(Equal(3), "missing-session JSON must retain only the established OAuth fields")
				Expect(body["error"]).To(Equal("invalid_grant"))
				Expect(body["error_description"]).To(Equal("User session is unavailable. Please re-authenticate."))
				Expect(body["error_uri"]).To(Equal(fixture.Config.Server.EndUser.PublicURL + "/sessions"))
			case http.StatusInternalServerError:
				outcome = "configuration_error"
				Expect(len(body)).To(Equal(2), "an unregistered candidate must not add identity or recovery fields to the OAuth error")
				Expect(body["error"]).To(Equal("server_error"))
			}
			Expect(fixture.Upstream.GetTokenCalled()).To(BeFalse(), "no scenario needs provider refresh or token issuance")
			Expect(len(fixture.Upstream.GetTokenRequests())).To(BeZero())

			for _, observation := range fixture.ExchangeObservationFields() {
				Expect(observation["token_exchange.outcome"]).To(Equal(outcome))
				Expect(observation["token_exchange.service.id"]).To(Equal(fixture.S1.ID.String()))
				if detail == "" {
					_, present := observation["token_exchange.failure_detail"]
					Expect(present).To(BeFalse(), "success must not acquire a failure detail")
				} else {
					Expect(observation["token_exchange.failure_detail"]).To(Equal(detail))
				}
				fixture.AssertAuthorizationContext(observation, expectedAgentID, storedGrant)
			}
		},
		Entry("missing grant retains registered agent and service but no grant metadata", func(f *grantDiagnosticsFixture) {
			Expect(f.Storage.UserGrants().Delete(context.Background(), f.Grant.ID)).To(Succeed())
		}, http.StatusForbidden, "grant_missing", true, false),
		Entry("successful exchange retains stored nanosecond timestamps and existing identity/provenance", func(f *grantDiagnosticsFixture) {
			zone := time.FixedZone("grant-fixture", 2*60*60)
			f.Grant.UpdatedAt = time.Date(2026, time.October, 7, 11, 12, 13, 123456789, zone)
			validUntil := f.Grant.ValidUntil.In(zone)
			f.Grant.ValidUntil = &validUntil
			Expect(f.Storage.UserGrants().Update(context.Background(), f.Grant)).To(Succeed())
			f.SeedSession(f.S1, []string{"read"})
		}, http.StatusOK, "", true, true),
		Entry("successful indefinite grant omits expiry rather than emitting a sentinel", func(f *grantDiagnosticsFixture) {
			f.Grant.ValidUntil = nil
			Expect(f.Storage.UserGrants().Update(context.Background(), f.Grant)).To(Succeed())
			f.SeedSession(f.S1, []string{"read"})
		}, http.StatusOK, "", true, true),
		Entry("expired grant denial retains metadata for the grant actually found", func(f *grantDiagnosticsFixture) {
			validUntil := time.Now().Add(-time.Hour).In(time.FixedZone("grant-fixture", -3*60*60))
			f.Grant.ValidUntil = &validUntil
			Expect(f.Storage.UserGrants().Update(context.Background(), f.Grant)).To(Succeed())
		}, http.StatusForbidden, "grant_expired", true, true),
		Entry("missing session retains all previously resolved grant context", func(f *grantDiagnosticsFixture) {
			sessions, err := f.Storage.UserSessions().ListByPrincipal(context.Background(), f.Principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(sessions)).To(BeZero())
		}, http.StatusBadRequest, "session_missing", true, true),
		Entry("unregistered subject-token candidate is not exported despite a stored grant", func(f *grantDiagnosticsFixture) {
			deleted, err := f.Storage.Agents().Delete(context.Background(), f.Agent.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(deleted).To(BeTrue())
			_, err = f.Storage.UserGrants().Get(context.Background(), f.Grant.ID)
			Expect(err).NotTo(HaveOccurred(), "the candidate must not expose even a still-stored grant")
		}, http.StatusInternalServerError, "agent_missing", false, false),
	)
})

var _ = Describe("Grant Deletion Outcomes", func() {
	var fixture *grantDiagnosticsFixture

	BeforeEach(func() {
		fixture = newGrantDiagnosticsFixture()
	})

	// GD-D1 from specs/013-token-exchange/spec.md.
	It("[GD-D1] returns 204 then 404 for grant revocation and records one minimal successful revoke", func() {
		path := "/api/consent/agents/" + fixture.Agent.ID.String() + "/grants"
		fixture.Delete(fixture.Enduser, path, http.StatusNoContent)
		fixture.Delete(fixture.Enduser, path, http.StatusNotFound)

		records := fixture.DeletionRecords()
		Expect(records).To(HaveLen(1), "only the actual revocation must produce a successful deletion record")
		Expect(records[0]["msg"]).To(Equal("grant revoked"))
		Expect(records[0]["action"]).To(Equal("grant_revoked"))
		Expect(records[0]["principal"]).To(Equal(fixture.Principal.String()))
		Expect(records[0]["agent_id"]).To(Equal(fixture.Agent.ID.String()))
		Expect(records[0]["grant_id"]).To(Equal(fixture.Grant.ID.String()))
	})

	// GD-D1 from specs/013-token-exchange/spec.md.
	It("[GD-D1] keeps existing and absent agent deletes at 204 with one domain deletion record", func() {
		fixture.Delete(fixture.Enduser, "/api/consent/agents/"+fixture.Agent.ID.String()+"/grants", http.StatusNoContent)
		fixture.Logs.Reset()

		path := "/api/agents/" + fixture.Agent.ID.String()
		fixture.Delete(fixture.Admin, path, http.StatusNoContent)
		fixture.Delete(fixture.Admin, path, http.StatusNoContent)

		records := fixture.DeletionRecords()
		Expect(records).To(HaveLen(1), "an absent delete and the handler must not add successful deletion records")
		Expect(records[0]["msg"]).To(Equal("agent deleted"))
		Expect(records[0]["agent_id"]).To(Equal(fixture.Agent.ID.String()))
	})

	// GD-D1 from specs/013-token-exchange/spec.md.
	It("[GD-D1] keeps existing and absent unused permission-set deletes at 204 with one domain deletion record", func() {
		fixture.Delete(fixture.Enduser, "/api/consent/agents/"+fixture.Agent.ID.String()+"/grants", http.StatusNoContent)
		fixture.Delete(fixture.Admin, "/api/agents/"+fixture.Agent.ID.String(), http.StatusNoContent)
		fixture.Logs.Reset()

		path := "/api/permission-sets/" + fixture.PermissionSet.ID.String()
		fixture.Delete(fixture.Admin, path, http.StatusNoContent)
		fixture.Delete(fixture.Admin, path, http.StatusNoContent)

		records := fixture.DeletionRecords()
		Expect(records).To(HaveLen(1), "an absent delete and the handler must not add successful deletion records")
		Expect(records[0]["msg"]).To(Equal("PermissionSetDeleted"))
		Expect(records[0]["action"]).To(Equal("permission_set_deleted"))
		Expect(records[0]["permission_set_id"]).To(Equal(fixture.PermissionSet.ID.String()))
	})
})
