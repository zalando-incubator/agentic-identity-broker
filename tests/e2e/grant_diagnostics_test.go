package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
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
const grantDiagnosticPrivateProfile = "LIFECYCLE_PROFILE_SENTINEL"

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

func (f *grantDiagnosticsFixture) Delete(server *bootstrap.TestServer, path string, status int) string {
	response, err := server.DirectRequest(http.MethodDelete, path, f.Principal.String(), nil, nil)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	Expect(response.StatusCode).To(Equal(status))
	if status == http.StatusNoContent {
		body, err := io.ReadAll(response.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(BeEmpty(), "successful DELETE must retain its empty 204 response")
	}
	return grantDiagnosticResponseTrace(response)
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
	return f.LastExchangeObservationFields()
}

func (f *grantDiagnosticsFixture) LastExchangeObservationFields() []map[string]any {
	records := f.ExchangeRecords()
	Expect(records).NotTo(BeEmpty())
	Eventually(func() int { return len(f.ExchangeSpans()) }).Should(Equal(len(records)))
	record := records[len(records)-1]
	span := f.ExchangeSpans()[len(records)-1]
	spanFields := make(map[string]any, len(span.Attributes()))
	for _, attr := range span.Attributes() {
		spanFields[string(attr.Key)] = attr.Value.AsInterface()
	}
	f.AssertPrivateObservation(record)
	f.AssertPrivateObservation(spanFields)
	f.AssertPrivateObservation(map[string]any{"span_status": span.Status().Description})
	for _, event := range span.Events() {
		eventFields := map[string]any{"span_event": event.Name}
		for _, attr := range event.Attributes {
			eventFields[string(attr.Key)] = attr.Value.AsInterface()
		}
		f.AssertPrivateObservation(eventFields)
	}
	Expect(record["actor"]).To(Equal(f.Principal.String()))
	Expect(record["calling_peer"]).To(Equal("grant-diagnostics-gateway"))
	Expect(span.SpanContext().IsValid()).To(BeTrue())
	Expect(record["trace_id"]).To(Equal(span.SpanContext().TraceID().String()))
	return []map[string]any{record, spanFields}
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
		grantDiagnosticPrivateProfile,
		f.Agent.DisplayName,
		f.Agent.Description,
		f.PermissionSet.Name,
		f.PermissionSet.Description,
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

func grantDiagnosticResponseTrace(response *http.Response) string {
	parts := strings.Split(response.Header.Get("traceresponse"), "-")
	Expect(parts).To(HaveLen(4), "the request trace must remain available through the existing response header")
	Expect(parts[1]).To(MatchRegexp(`^[0-9a-f]{32}$`))
	Expect(parts[1]).NotTo(Equal(strings.Repeat("0", 32)))
	return parts[1]
}

func grantDiagnosticJSONValue(value any) any {
	encoded, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())
	var decoded any
	Expect(json.Unmarshal(encoded, &decoded)).To(Succeed())
	return decoded
}

func (f *grantDiagnosticsFixture) LifecycleRecords() []map[string]any {
	records, err := f.Logs.Records()
	Expect(err).NotTo(HaveOccurred())
	messages := map[string]string{
		"grant_created":          "grant created",
		"grant_updated":          "grant updated",
		"grant_revoked":          "grant revoked",
		"agent_created":          "agent created",
		"agent_updated":          "agent updated",
		"agent_deleted":          "agent deleted",
		"permission_set_created": "PermissionSetCreated",
		"permission_set_updated": "PermissionSetUpdated",
		"permission_set_deleted": "PermissionSetDeleted",
	}
	fields := map[string]string{
		"grant_created":          "principal agent_id grant_id valid_until created_at updated_at granted_permission_sets",
		"grant_updated":          "principal agent_id grant_id valid_until created_at updated_at granted_permission_sets previous_observed_valid_until previous_observed_updated_at previous_observed_granted_permission_sets",
		"grant_revoked":          "principal agent_id grant_id valid_until updated_at granted_permission_sets revoked_at",
		"agent_created":          "agent_id permission_sets service_requirements",
		"agent_updated":          "agent_id permission_sets service_requirements previous_observed_permission_sets previous_observed_service_requirements previous_observed_updated_at",
		"agent_deleted":          "agent_id",
		"permission_set_created": "permission_set_id service_scopes",
		"permission_set_updated": "permission_set_id service_scopes previous_observed_service_scopes previous_observed_updated_at",
		"permission_set_deleted": "permission_set_id",
	}
	var audits []map[string]any
	for _, record := range records {
		action, _ := record["action"].(string)
		_, lifecycle := messages[action]
		for _, message := range messages {
			lifecycle = lifecycle || record["msg"] == message
		}
		lifecycle = lifecycle || record["msg"] == "permission set created" || record["msg"] == "permission set updated" || record["msg"] == "permission set deleted"
		if !lifecycle {
			continue
		}
		expectedMessage, implemented := messages[action]
		Expect(implemented).To(BeTrue(), "every lifecycle success must have a fixed domain-owned action; handler-only successes are duplicates")
		Expect(record["msg"]).To(Equal(expectedMessage))
		Expect(record["level"]).To(Equal("INFO"))
		allowed := strings.Fields("time level msg action actor trace_id calling_peer " + fields[action])
		for key := range record {
			Expect(allowed).To(ContainElement(key), "lifecycle records must contain only approved operational and typed definition fields")
		}
		f.AssertPrivateObservation(record)
		audits = append(audits, record)
	}
	return audits
}

func (f *grantDiagnosticsFixture) LifecycleActions() []string {
	var actions []string
	for _, record := range f.LifecycleRecords() {
		actions = append(actions, record["action"].(string))
	}
	return actions
}

func (f *grantDiagnosticsFixture) RequestJSON(server *bootstrap.TestServer, method, path, principal string, request any, status int) (map[string]any, string) {
	encoded, err := json.Marshal(request)
	Expect(err).NotTo(HaveOccurred())
	response, err := server.DirectRequest(method, path, principal, map[string]string{"Content-Type": "application/json"}, strings.NewReader(string(encoded)))
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	Expect(response.StatusCode).To(Equal(status), "lifecycle mutation must preserve its HTTP contract")
	var body map[string]any
	Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
	return body, grantDiagnosticResponseTrace(response)
}

func (f *grantDiagnosticsFixture) PostConsent(validUntil *time.Time, services []id.ServiceID) (*storagedomain.UserGrant, string) {
	serviceIDs := make([]string, len(services))
	for i, serviceID := range services {
		serviceIDs[i] = serviceID.String()
	}
	body, traceID := f.RequestJSON(f.Enduser, http.MethodPost, "/api/consent/agents/"+f.Agent.ID.String()+"/grants", f.Principal.String(), map[string]any{
		"valid_until":             validUntil,
		"granted_permission_sets": map[string][]string{f.PermissionSet.ID.String(): serviceIDs},
	}, http.StatusCreated)
	grant, err := f.Storage.UserGrants().FindByPrincipalAndAgent(context.Background(), f.Principal, f.Agent.ID)
	Expect(err).NotTo(HaveOccurred())
	Expect(grant).NotTo(BeNil())
	data, ok := body["data"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(data["id"]).To(Equal(grant.ID.String()))
	Expect(data["principal"]).To(Equal(f.Principal.String()))
	Expect(data["agent_id"]).To(Equal(f.Agent.ID.String()))
	for key, expected := range map[string]time.Time{"created_at": grant.CreatedAt, "updated_at": grant.UpdatedAt} {
		wireTime, ok := data[key].(string)
		Expect(ok).To(BeTrue())
		parsed, err := time.Parse(time.RFC3339, wireTime)
		Expect(err).NotTo(HaveOccurred())
		Expect(parsed.UTC().Format(time.RFC3339)).To(Equal(expected.UTC().Format(time.RFC3339)))
		Expect(parsed.Nanosecond()).To(BeZero(), "existing HTTP timestamps retain second precision")
	}
	Expect(data["granted_permission_sets"]).To(Equal(grantDiagnosticJSONValue(map[string][]string{f.PermissionSet.ID.String(): serviceIDs})))
	if validUntil != nil {
		Expect(data["valid_until"]).To(Equal(validUntil.UTC().Format(time.RFC3339Nano)))
	}
	return grant, traceID
}

func (f *grantDiagnosticsFixture) AssertAuditIdentity(record map[string]any, actor, traceID string) {
	Expect(record["actor"]).To(Equal(actor))
	Expect(record["trace_id"]).To(Equal(traceID))
	_, peerPresent := record["calling_peer"]
	Expect(peerPresent).To(BeFalse(), "preauthenticated consent/admin requests must not invent a calling peer")
}

func (f *grantDiagnosticsFixture) AssertGrantAudit(record map[string]any, grant *storagedomain.UserGrant) {
	Expect(record["principal"]).To(Equal(grant.Principal.String()))
	Expect(record["agent_id"]).To(Equal(grant.AgentID.String()))
	Expect(record["grant_id"]).To(Equal(grant.ID.String()))
	Expect(record["updated_at"]).To(Equal(grant.UpdatedAt.UTC().Format(time.RFC3339Nano)))
	value, present := record["valid_until"]
	Expect(present).To(BeTrue(), "indefinite expiry must be explicit JSON null")
	if grant.ValidUntil == nil {
		Expect(value).To(BeNil())
	} else {
		Expect(value).To(Equal(grant.ValidUntil.UTC().Format(time.RFC3339Nano)))
	}
	Expect(record["granted_permission_sets"]).To(Equal(grantDiagnosticJSONValue(grant.GrantedPermissionSets)))
	if record["action"] != "grant_revoked" {
		Expect(record["created_at"]).To(Equal(grant.CreatedAt.UTC().Format(time.RFC3339Nano)))
	}
}

func (f *grantDiagnosticsFixture) AssertLifecycleExchange(service *model.ThirdpartyOAuth2ProviderEntity, grant *storagedomain.UserGrant, detail string) map[string]any {
	before := len(f.ExchangeRecords())
	response := f.Exchange(service)
	defer func() { _ = response.Body.Close() }()
	var body map[string]any
	Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
	if detail == "" {
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		keys := make([]string, 0, len(body))
		for key := range body {
			keys = append(keys, key)
		}
		expectedKeys := []string{"access_token", "token_type", "issued_token_type", "expires_in", "principal", "agent_id", "granted_permission_sets"}
		if service.ID != f.S0.ID {
			expectedKeys = append(expectedKeys, "scope")
		}
		Expect(keys).To(ConsistOf(expectedKeys))
		accessToken, ok := body["access_token"].(string)
		Expect(ok && accessToken == "token-"+service.ID.String()).To(BeTrue(), "return the existing encrypted session token without printing it")
		Expect(body["principal"]).To(Equal(f.Principal.String()))
		Expect(body["agent_id"]).To(Equal(f.Agent.ID.String()))
		Expect(body["token_type"]).To(Equal("Bearer"))
		Expect(body["issued_token_type"]).To(Equal("urn:ietf:params:oauth:token-type:access_token"))
		Expect(body["expires_in"]).To(BeNumerically(">", 0))
		if service.ID == f.S0.ID {
			_, scopePresent := body["scope"]
			Expect(scopePresent).To(BeFalse(), "the established wire schema omits the empty scope")
		} else {
			Expect(body["scope"]).To(Equal("read"))
		}
		provenance := make(map[string][]string, len(grant.GrantedPermissionSets))
		for _, entry := range grant.GrantedPermissionSets {
			serviceIDs := make([]string, len(entry.IncludedServiceIDs))
			for i, includedServiceID := range entry.IncludedServiceIDs {
				serviceIDs[i] = includedServiceID.String()
			}
			provenance[entry.PermissionSetID.String()] = serviceIDs
		}
		Expect(body["granted_permission_sets"]).To(Equal(grantDiagnosticJSONValue(provenance)))
	} else {
		Expect(response.StatusCode).To(Equal(http.StatusForbidden))
		Expect(len(body)).To(Equal(3))
		Expect(body["error"]).To(Equal("access_denied"))
		Expect(body["error_description"]).To(Equal("User authorization is insufficient. Please re-consent."))
		Expect(body["error_uri"]).To(Equal(f.Config.Server.EndUser.PublicURL + "/agents/" + f.Agent.ID.String()))
	}
	Expect(len(f.ExchangeRecords())).To(Equal(before+1), "each HTTP exchange must emit exactly one observation")
	observations := f.LastExchangeObservationFields()
	for _, observation := range observations {
		Expect(observation["token_exchange.service.id"]).To(Equal(service.ID.String()))
		f.AssertAuthorizationContext(observation, f.Agent.ID, grant)
		if detail == "" {
			Expect(observation["token_exchange.outcome"]).To(Equal("success"))
			_, present := observation["token_exchange.failure_detail"]
			Expect(present).To(BeFalse())
		} else {
			Expect(observation["token_exchange.outcome"]).To(Equal("authorization_denied"))
			Expect(observation["token_exchange.failure_detail"]).To(Equal(detail))
			Expect(observation["token_exchange.failure_stage"]).To(Equal("grant_authorization"))
			Expect(observation["token_exchange.recovery_action"]).To(Equal("reconsent"))
			Expect(observation["token_exchange.recovery_target"]).To(Equal("consent"))
		}
	}
	Expect(observations[0]["trace_id"]).To(Equal(grantDiagnosticResponseTrace(response)))
	Expect(f.Upstream.GetTokenCalled()).To(BeFalse())
	Expect(f.Upstream.GetTokenRequests()).To(BeEmpty())
	return observations[0]
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

var _ = Describe("Grant Diagnostic Lifecycle", func() {
	var fixture *grantDiagnosticsFixture

	BeforeEach(func() {
		fixture = newGrantDiagnosticsFixture()
		fixture.SeedSession(fixture.S1, []string{"read"})
		fixture.SeedSession(fixture.S2, []string{"read"})
		fixture.SeedSession(fixture.S0, []string{})
	})

	// GD-S1 from specs/013-token-exchange/spec.md.
	It("[GD-S1] correlates missing consent, one creation, and a successful exchange with the stored grant", func() {
		Expect(fixture.Storage.UserGrants().Delete(context.Background(), fixture.Grant.ID)).To(Succeed())
		fixture.AssertLifecycleExchange(fixture.S1, nil, "grant_missing")
		Expect(fixture.LifecycleRecords()).To(BeEmpty())

		grant, traceID := fixture.PostConsent(nil, []id.ServiceID{fixture.S1.ID, fixture.S2.ID, fixture.S0.ID})
		Expect(fixture.LifecycleActions()).To(Equal([]string{"grant_created"}))
		record := fixture.LifecycleRecords()[0]
		fixture.AssertGrantAudit(record, grant)
		fixture.AssertAuditIdentity(record, fixture.Principal.String(), traceID)
		for key := range record {
			Expect(strings.HasPrefix(key, "previous_observed_")).To(BeFalse(), "a creation has no observed-before grant")
		}
		observation := fixture.AssertLifecycleExchange(fixture.S1, grant, "")
		Expect(observation["actor"]).To(Equal(record["actor"]))
		Expect(fixture.LifecycleActions()).To(Equal([]string{"grant_created"}), "exchange must not create a duplicate lifecycle event")
	})

	// GD-S2 from specs/013-token-exchange/spec.md.
	It("[GD-S2] records observed coverage changes but not identical consent or a rejected empty POST", func() {
		before, err := fixture.Storage.UserGrants().Get(context.Background(), fixture.Grant.ID)
		Expect(err).NotTo(HaveOccurred())
		services := []id.ServiceID{fixture.S1.ID, fixture.S0.ID}
		updated, traceID := fixture.PostConsent(nil, services)
		Expect(updated.ID).To(Equal(before.ID))
		Expect(updated.CreatedAt).To(Equal(before.CreatedAt))
		Expect(fixture.LifecycleActions()).To(Equal([]string{"grant_updated"}))
		record := fixture.LifecycleRecords()[0]
		fixture.AssertGrantAudit(record, updated)
		fixture.AssertAuditIdentity(record, fixture.Principal.String(), traceID)
		Expect(record["previous_observed_valid_until"]).To(Equal(before.ValidUntil.UTC().Format(time.RFC3339Nano)))
		Expect(record["previous_observed_updated_at"]).To(Equal(before.UpdatedAt.UTC().Format(time.RFC3339Nano)))
		Expect(record["previous_observed_granted_permission_sets"]).To(Equal(grantDiagnosticJSONValue(before.GrantedPermissionSets)))
		fixture.AssertLifecycleExchange(fixture.S2, updated, "grant_service_omitted")

		unchanged, _ := fixture.PostConsent(nil, services)
		Expect(unchanged).To(Equal(updated), "an identical POST must preserve all stored grant metadata and inclusions")
		Expect(fixture.LifecycleActions()).To(Equal([]string{"grant_updated"}))
		body, _ := fixture.RequestJSON(fixture.Enduser, http.MethodPost, "/api/consent/agents/"+fixture.Agent.ID.String()+"/grants", fixture.Principal.String(), map[string]any{
			"granted_permission_sets": map[string][]string{},
		}, http.StatusBadRequest)
		Expect(body["error"]).To(Equal("missing mandatory permission set"))
		stored, err := fixture.Storage.UserGrants().Get(context.Background(), updated.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(Equal(updated), "empty grant POST must not revoke or alter consent")
		Expect(fixture.LifecycleActions()).To(Equal([]string{"grant_updated"}))
		fixture.AssertLifecycleExchange(fixture.S1, stored, "")
	})

	// GD-S3 from specs/013-token-exchange/spec.md.
	It("[GD-S3] audits real admin definition changes and diagnoses their coverage limits without changing the user grant", func() {
		grant, err := fixture.Storage.UserGrants().Get(context.Background(), fixture.Grant.ID)
		Expect(err).NotTo(HaveOccurred())
		beforeAgent, err := fixture.Storage.Agents().Get(context.Background(), fixture.Agent.ID)
		Expect(err).NotTo(HaveOccurred())
		const operator = "grant-diagnostics-admin"
		createdScopes := []storagedomain.ServiceScope{{ServiceID: fixture.S1.ID, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional}}
		createdPSBody, createPSTrace := fixture.RequestJSON(fixture.Admin, http.MethodPost, "/api/permission-sets", operator, map[string]any{
			"name": grantDiagnosticPrivateProfile + "-created", "description": grantDiagnosticPrivateProfile, "service_scopes": createdScopes,
		}, http.StatusCreated)
		createdPSID, err := id.ParsePermissionSetID(createdPSBody["id"].(string))
		Expect(err).NotTo(HaveOccurred())
		Expect(fixture.LifecycleActions()).To(Equal([]string{"permission_set_created"}))
		createdRecord := fixture.LifecycleRecords()[0]
		fixture.AssertAuditIdentity(createdRecord, operator, createPSTrace)
		Expect(createdRecord["permission_set_id"]).To(Equal(createdPSID.String()))
		Expect(createdRecord["service_scopes"]).To(Equal(grantDiagnosticJSONValue(createdScopes)))
		fixture.Logs.Reset()
		createdPermissions := []storagedomain.AgentPermissionSetEntry{{PermissionSetID: createdPSID, RequirementType: storagedomain.RequirementTypeMandatory}}
		createdRequirements := []storagedomain.ServiceRequirement{{ServiceID: fixture.S1.ID, RequiredScopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional}}
		createdAgentBody, createAgentTrace := fixture.RequestJSON(fixture.Admin, http.MethodPost, "/api/agents", operator, map[string]any{
			"display_name": grantDiagnosticPrivateProfile, "description": grantDiagnosticPrivateProfile,
			"permission_sets": createdPermissions, "service_requirements": createdRequirements,
			"redirect_uris": beforeAgent.RedirectURIs,
		}, http.StatusCreated)
		createdAgentID, err := id.ParseAgentID(createdAgentBody["id"].(string))
		Expect(err).NotTo(HaveOccurred())
		Expect(fixture.LifecycleActions()).To(Equal([]string{"agent_created"}))
		createdRecord = fixture.LifecycleRecords()[0]
		fixture.AssertAuditIdentity(createdRecord, operator, createAgentTrace)
		Expect(createdRecord["agent_id"]).To(Equal(createdAgentID.String()))
		Expect(createdRecord["permission_sets"]).To(Equal(grantDiagnosticJSONValue(createdPermissions)))
		Expect(createdRecord["service_requirements"]).To(Equal(grantDiagnosticJSONValue(createdRequirements)))
		fixture.Logs.Reset()
		agentRequest := func(requirements []storagedomain.ServiceRequirement) map[string]any {
			return map[string]any{
				"display_name":         grantDiagnosticPrivateProfile,
				"description":          grantDiagnosticPrivateProfile,
				"permission_sets":      beforeAgent.PermissionSets,
				"service_requirements": requirements,
				"redirect_uris":        beforeAgent.RedirectURIs,
			}
		}
		agentPath := "/api/agents/" + fixture.Agent.ID.String()
		invalidRequirements := beforeAgent.Copy().ServiceRequirements
		invalidRequirements[0].RequirementType = storagedomain.RequirementType("invalid")
		body, _ := fixture.RequestJSON(fixture.Admin, http.MethodPut, agentPath, "", agentRequest(invalidRequirements), http.StatusBadRequest)
		Expect(body["error"]).To(Equal("invalid service requirements"))
		Expect(fixture.LifecycleRecords()).To(BeEmpty(), "rejected definition edits must not claim success")

		excluded := []storagedomain.ServiceRequirement{beforeAgent.ServiceRequirements[0], beforeAgent.ServiceRequirements[2]}
		_, excludeTrace := fixture.RequestJSON(fixture.Admin, http.MethodPut, agentPath, "", agentRequest(excluded), http.StatusOK)
		afterAgent, err := fixture.Storage.Agents().Get(context.Background(), fixture.Agent.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(afterAgent.ServiceRequirements).To(Equal(excluded))
		Expect(fixture.LifecycleActions()).To(Equal([]string{"agent_updated"}))
		record := fixture.LifecycleRecords()[0]
		fixture.AssertAuditIdentity(record, "anonymous", excludeTrace)
		Expect(record["agent_id"]).To(Equal(fixture.Agent.ID.String()))
		Expect(record["permission_sets"]).To(Equal(grantDiagnosticJSONValue(afterAgent.PermissionSets)))
		Expect(record["service_requirements"]).To(Equal(grantDiagnosticJSONValue(afterAgent.ServiceRequirements)))
		Expect(record["previous_observed_permission_sets"]).To(Equal(grantDiagnosticJSONValue(beforeAgent.PermissionSets)))
		Expect(record["previous_observed_service_requirements"]).To(Equal(grantDiagnosticJSONValue(beforeAgent.ServiceRequirements)))
		Expect(record["previous_observed_updated_at"]).To(Equal(beforeAgent.UpdatedAt.UTC().Format(time.RFC3339Nano)))
		fixture.AssertLifecycleExchange(fixture.S2, grant, "grant_service_requirement_excluded")

		_, restoreTrace := fixture.RequestJSON(fixture.Admin, http.MethodPut, agentPath, operator, agentRequest(beforeAgent.ServiceRequirements), http.StatusOK)
		Expect(fixture.LifecycleActions()).To(Equal([]string{"agent_updated", "agent_updated"}))
		restored, err := fixture.Storage.Agents().Get(context.Background(), fixture.Agent.ID)
		Expect(err).NotTo(HaveOccurred())
		record = fixture.LifecycleRecords()[1]
		fixture.AssertAuditIdentity(record, operator, restoreTrace)
		Expect(record["service_requirements"]).To(Equal(grantDiagnosticJSONValue(restored.ServiceRequirements)))
		Expect(record["permission_sets"]).To(Equal(grantDiagnosticJSONValue(restored.PermissionSets)))
		Expect(record["previous_observed_permission_sets"]).To(Equal(grantDiagnosticJSONValue(afterAgent.PermissionSets)))
		Expect(record["previous_observed_service_requirements"]).To(Equal(grantDiagnosticJSONValue(afterAgent.ServiceRequirements)))
		Expect(record["previous_observed_updated_at"]).To(Equal(afterAgent.UpdatedAt.UTC().Format(time.RFC3339Nano)))
		fixture.AssertLifecycleExchange(fixture.S2, grant, "")

		beforePS, err := fixture.Storage.PermissionSets().Get(context.Background(), fixture.PermissionSet.ID)
		Expect(err).NotTo(HaveOccurred())
		psPath := "/api/permission-sets/" + fixture.PermissionSet.ID.String()
		body, _ = fixture.RequestJSON(fixture.Admin, http.MethodPut, psPath, "", map[string]any{
			"name": "", "description": grantDiagnosticPrivateProfile, "service_scopes": beforePS.ServiceScopes,
		}, http.StatusBadRequest)
		Expect(body["error"]).To(Equal("validation failed"))
		Expect(fixture.LifecycleActions()).To(Equal([]string{"agent_updated", "agent_updated"}))
		newScopes := beforePS.Copy().ServiceScopes
		newScopes[1].Scopes = []string{"write"}
		_, psTrace := fixture.RequestJSON(fixture.Admin, http.MethodPut, psPath, "", map[string]any{
			"name": grantDiagnosticPrivateProfile, "description": grantDiagnosticPrivateProfile, "service_scopes": newScopes,
		}, http.StatusOK)
		afterPS, err := fixture.Storage.PermissionSets().Get(context.Background(), fixture.PermissionSet.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(afterPS.ServiceScopes).To(Equal(newScopes))
		Expect(fixture.LifecycleActions()).To(Equal([]string{"agent_updated", "agent_updated", "permission_set_updated"}))
		record = fixture.LifecycleRecords()[2]
		fixture.AssertAuditIdentity(record, "anonymous", psTrace)
		Expect(record["permission_set_id"]).To(Equal(fixture.PermissionSet.ID.String()))
		Expect(record["service_scopes"]).To(Equal(grantDiagnosticJSONValue(afterPS.ServiceScopes)))
		Expect(record["previous_observed_service_scopes"]).To(Equal(grantDiagnosticJSONValue(beforePS.ServiceScopes)))
		Expect(record["previous_observed_updated_at"]).To(Equal(beforePS.UpdatedAt.UTC().Format(time.RFC3339Nano)))
		fixture.AssertLifecycleExchange(fixture.S2, grant, "grant_scope_intersection_empty")
		_, authenticatedPSTrace := fixture.RequestJSON(fixture.Admin, http.MethodPut, psPath, operator, map[string]any{
			"name": grantDiagnosticPrivateProfile, "description": grantDiagnosticPrivateProfile, "service_scopes": newScopes,
		}, http.StatusOK)
		updatedPS, err := fixture.Storage.PermissionSets().Get(context.Background(), fixture.PermissionSet.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixture.LifecycleActions()).To(Equal([]string{"agent_updated", "agent_updated", "permission_set_updated", "permission_set_updated"}))
		record = fixture.LifecycleRecords()[3]
		fixture.AssertAuditIdentity(record, operator, authenticatedPSTrace)
		Expect(record["permission_set_id"]).To(Equal(fixture.PermissionSet.ID.String()))
		Expect(record["service_scopes"]).To(Equal(grantDiagnosticJSONValue(updatedPS.ServiceScopes)))
		Expect(record["previous_observed_service_scopes"]).To(Equal(grantDiagnosticJSONValue(afterPS.ServiceScopes)))
		Expect(record["previous_observed_updated_at"]).To(Equal(afterPS.UpdatedAt.UTC().Format(time.RFC3339Nano)))
		fixture.AssertLifecycleExchange(fixture.S0, grant, "")
		storedGrant, err := fixture.Storage.UserGrants().Get(context.Background(), grant.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(storedGrant).To(Equal(grant), "definition changes must not mutate the grant ID, inclusions, or metadata")
		Expect(fixture.LifecycleActions()).To(Equal([]string{"agent_updated", "agent_updated", "permission_set_updated", "permission_set_updated"}))
	})

	// GD-S4 from specs/013-token-exchange/spec.md.
	It("[GD-S4] preserves rejected empty consent, audits the deleted snapshot once, and omits absent-delete audits", func() {
		grant, err := fixture.Storage.UserGrants().Get(context.Background(), fixture.Grant.ID)
		Expect(err).NotTo(HaveOccurred())
		grantPath := "/api/consent/agents/" + fixture.Agent.ID.String() + "/grants"
		body, _ := fixture.RequestJSON(fixture.Enduser, http.MethodPost, grantPath, fixture.Principal.String(), map[string]any{
			"granted_permission_sets": map[string][]string{},
		}, http.StatusBadRequest)
		Expect(body["error"]).To(Equal("missing mandatory permission set"))
		stored, err := fixture.Storage.UserGrants().Get(context.Background(), grant.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(Equal(grant))
		Expect(fixture.LifecycleRecords()).To(BeEmpty())
		fixture.AssertLifecycleExchange(fixture.S1, stored, "")

		revokeStarted := time.Now().UTC()
		traceID := fixture.Delete(fixture.Enduser, grantPath, http.StatusNoContent)
		revokeFinished := time.Now().UTC()
		Expect(fixture.LifecycleActions()).To(Equal([]string{"grant_revoked"}))
		record := fixture.LifecycleRecords()[0]
		fixture.AssertGrantAudit(record, grant)
		fixture.AssertAuditIdentity(record, fixture.Principal.String(), traceID)
		revokedAt, ok := record["revoked_at"].(string)
		Expect(ok).To(BeTrue())
		parsed, err := time.Parse(time.RFC3339Nano, revokedAt)
		Expect(err).NotTo(HaveOccurred())
		Expect(revokedAt).To(Equal(parsed.UTC().Format(time.RFC3339Nano)))
		Expect(parsed.Before(revokeStarted) || parsed.After(revokeFinished)).To(BeFalse(), "revocation time must describe the successful request")
		_, err = fixture.Storage.UserGrants().Get(context.Background(), grant.ID)
		Expect(errors.Is(err, ports.ErrNotFound)).To(BeTrue())
		fixture.AssertLifecycleExchange(fixture.S1, nil, "grant_missing")
		fixture.Delete(fixture.Enduser, grantPath, http.StatusNotFound)
		Expect(fixture.LifecycleActions()).To(Equal([]string{"grant_revoked"}))

		agentPath := "/api/agents/" + fixture.Agent.ID.String()
		agentTrace := fixture.Delete(fixture.Admin, agentPath, http.StatusNoContent)
		fixture.Delete(fixture.Admin, agentPath, http.StatusNoContent)
		psPath := "/api/permission-sets/" + fixture.PermissionSet.ID.String()
		psTrace := fixture.Delete(fixture.Admin, psPath, http.StatusNoContent)
		fixture.Delete(fixture.Admin, psPath, http.StatusNoContent)
		Expect(fixture.LifecycleActions()).To(Equal([]string{"grant_revoked", "agent_deleted", "permission_set_deleted"}))
		records := fixture.LifecycleRecords()
		Expect(records[1]["agent_id"]).To(Equal(fixture.Agent.ID.String()))
		fixture.AssertAuditIdentity(records[1], fixture.Principal.String(), agentTrace)
		Expect(records[2]["permission_set_id"]).To(Equal(fixture.PermissionSet.ID.String()))
		fixture.AssertAuditIdentity(records[2], fixture.Principal.String(), psTrace)
	})
})
