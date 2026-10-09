package e2e_test

import (
	"context"
	"encoding/json"
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

			records := fixture.ExchangeRecords()
			Expect(len(records)).To(Equal(1), "one exchange observation must describe the denied HTTP request")
			Eventually(func() int { return len(fixture.ExchangeSpans()) }).Should(Equal(1))
			span := fixture.ExchangeSpans()[0]
			spanFields := make(map[string]any, len(span.Attributes()))
			for _, attr := range span.Attributes() {
				spanFields[string(attr.Key)] = attr.Value.AsInterface()
			}
			fixture.AssertPrivateObservation(records[0])
			fixture.AssertPrivateObservation(spanFields)
			fixture.AssertPrivateObservation(map[string]any{"span_status": span.Status().Description})
			for _, event := range span.Events() {
				eventFields := map[string]any{"span_event": event.Name}
				for _, attr := range event.Attributes {
					eventFields[string(attr.Key)] = attr.Value.AsInterface()
				}
				fixture.AssertPrivateObservation(eventFields)
			}
			for _, observation := range []map[string]any{records[0], spanFields} {
				Expect(observation["token_exchange.failure_detail"]).To(Equal(detail))
				Expect(observation["token_exchange.outcome"]).To(Equal("authorization_denied"))
				Expect(observation["token_exchange.failure_stage"]).To(Equal("grant_authorization"))
				Expect(observation["token_exchange.recovery_action"]).To(Equal("reconsent"))
				Expect(observation["token_exchange.recovery_target"]).To(Equal("consent"))
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
