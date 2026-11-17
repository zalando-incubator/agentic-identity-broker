package tokenexchange

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

type stubCanonicalIDs struct {
	ids   map[id.ServiceID]string
	err   error
	calls int
	asked []id.ServiceID
}

func (s *stubCanonicalIDs) CanonicalIDs(_ context.Context, ids []id.ServiceID) (map[id.ServiceID]string, error) {
	s.calls++
	s.asked = append([]id.ServiceID(nil), ids...)
	return s.ids, s.err
}

type evaluationFixture struct {
	privateKey  *rsa.PrivateKey
	keySet      jwk.Set
	agent       *storagedomain.Agent
	agentRepo   *singleAgentRepo
	grant       *storagedomain.UserGrant
	grantRepo   *MockGrantRepository
	psRepo      *MockPermissionSetRepository
	serviceRepo *MockServiceRepository
	service     *model.ThirdpartyOAuth2ProviderEntity
	otherSvcID  id.ServiceID
	readerPSID  id.PermissionSetID
	writerPSID  id.PermissionSetID
	canonical   *stubCanonicalIDs
	celExpr     string
	now         time.Time
}

const evaluationResource = "https://api.example.com/repos"

func newEvaluationFixture(t *testing.T) *evaluationFixture {
	t.Helper()
	privateKey, keySet := generateTestRSAKeySet(t)
	now := time.Now()
	agentID := id.NewAgentID()
	serviceID := id.NewServiceID()
	otherSvcID := id.NewServiceID()
	readerPSID := id.NewPermissionSetID()
	writerPSID := id.NewPermissionSetID()
	agent := &storagedomain.Agent{
		ID:          agentID,
		CanonicalID: ptr.To("data-assistant"),
		DisplayName: "Data Assistant",
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: readerPSID, RequirementType: storagedomain.RequirementTypeOptional},
			{PermissionSetID: writerPSID, RequirementType: storagedomain.RequirementTypeOptional},
		},
		ServiceRequirements: []storagedomain.ServiceRequirement{
			{ServiceID: serviceID, RequiredScopes: []string{"read:repo"}, RequirementType: storagedomain.RequirementTypeOptional},
			{ServiceID: otherSvcID, RequireAllScopes: true, RequirementType: storagedomain.RequirementTypeOptional},
		},
	}
	grant := &storagedomain.UserGrant{
		ID:        id.NewGrantID(),
		AgentID:   agentID,
		Principal: id.Principal("alice@example.com"),
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: writerPSID, IncludedServiceIDs: []id.ServiceID{otherSvcID, serviceID}},
			{PermissionSetID: readerPSID, IncludedServiceIDs: []id.ServiceID{serviceID}},
		},
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now.Add(-time.Minute),
	}
	service := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                 serviceID,
		CanonicalID:        ptr.To("repo-api"),
		DisplayName:        "Repo API",
		ClientID:           id.ClientID("repo-client"),
		Secret:             model.NewEncryptedSecret([]byte("placeholder")),
		ProtectedResources: []string{evaluationResource},
	}
	return &evaluationFixture{
		privateKey: privateKey,
		keySet:     keySet,
		agent:      agent,
		agentRepo:  &singleAgentRepo{agentID: agentID, agent: agent},
		grant:      grant,
		grantRepo:  &MockGrantRepository{grant: grant},
		psRepo: &MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
			readerPSID: {ID: readerPSID, CanonicalID: ptr.To("repo-reader"), Name: "Reader", ServiceScopes: []storagedomain.ServiceScope{
				{ServiceID: serviceID, Scopes: []string{"read:repo"}, RequirementType: storagedomain.RequirementTypeOptional},
			}},
			writerPSID: {ID: writerPSID, Name: "Writer", ServiceScopes: []storagedomain.ServiceScope{
				{ServiceID: serviceID, Scopes: []string{"write:repo", "read:repo"}, RequirementType: storagedomain.RequirementTypeOptional},
			}},
		}},
		serviceRepo: &MockServiceRepository{service: service},
		service:     service,
		otherSvcID:  otherSvcID,
		readerPSID:  readerPSID,
		writerPSID:  writerPSID,
		canonical:   &stubCanonicalIDs{ids: map[id.ServiceID]string{serviceID: "repo-api"}},
		celExpr:     "true",
		now:         now,
	}
}

func (f *evaluationFixture) newService(t *testing.T) *AccessEvaluationService {
	t.Helper()
	jwtValidator, err := NewJWTValidator(&MockJWKSProvider{keySet: f.keySet}, "https://auth.example.com", "agentic-identity-broker", 60)
	require.NoError(t, err)
	celEvaluator, err := NewCELEvaluator(CELEvaluatorConfig{
		PrincipalExpression:     "subject_token.sub",
		AgentIDExpression:       "subject_token.azp",
		AuthorizationExpression: f.celExpr,
		EvaluationTimeout:       100 * time.Millisecond,
	})
	require.NoError(t, err)
	providerService := newTestProviderService(f.serviceRepo)
	psService := permissionset.NewPermissionSetService(f.psRepo, f.grantRepo, slog.Default())
	t.Cleanup(psService.Close)
	consentService := consent.NewService(f.agentRepo, providerService, f.grantRepo, nil, nil, nil, slog.Default())
	resolver, err := NewDelegationResolver(jwtValidator, celEvaluator, providerService, consentService, psService, f.agentRepo, "https://broker.example.com/")
	require.NoError(t, err)
	svc, err := NewAccessEvaluationService(resolver, f.canonical)
	require.NoError(t, err)
	svc.now = func() time.Time { return f.now }
	return svc
}

func (f *evaluationFixture) token(t *testing.T, overrides map[string]interface{}) string {
	t.Helper()
	claims := map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "alice@example.com",
		"azp": f.agent.ID.String(),
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	for k, v := range overrides {
		claims[k] = v
	}
	return signServiceTestJWT(t, f.privateKey, claims)
}

func (f *evaluationFixture) request(t *testing.T) AccessEvaluationRequest {
	return AccessEvaluationRequest{
		SubjectType:  AuthZENSubjectTypeJWT,
		SubjectID:    f.token(t, nil),
		ActionName:   AuthZENActionTokenExchange,
		ResourceType: AuthZENResourceTypeProtected,
		ResourceID:   evaluationResource,
	}
}

func TestAccessEvaluation_PermitProjectsCredentialFreeContext(t *testing.T) {
	t.Parallel()
	f := newEvaluationFixture(t)
	svc := f.newService(t)

	decision := svc.Evaluate(context.Background(), f.request(t), f.token(t, nil))

	require.Equal(t, DecisionPermit, decision.Kind)
	assert.Equal(t, 60, decision.MaxAge)
	assert.Equal(t, 1, f.canonical.calls, "canonical IDs must be read in one batch")
	assert.ElementsMatch(t, []id.ServiceID{f.otherSvcID, f.service.ID}, f.canonical.asked)

	raw, err := json.Marshal(decision.Context)
	require.NoError(t, err)
	expected := `{
		"subject": {"id": "alice@example.com"},
		"agent": {"id": "` + f.agent.ID.String() + `", "canonical_id": "data-assistant"},
		"resource": {"id": "` + f.service.ID.String() + `", "canonical_id": "repo-api",
			"resource_uri": "https://api.example.com/repos", "scopes": ["read:repo"]},
		"grant": {"id": "` + f.grant.ID.String() + `", "updated_at": "` + f.grant.UpdatedAt.UTC().Format(time.RFC3339) + `"},
		"permission_sets": [
			{"id": "` + f.writerPSID.String() + `", "services": [
				{"id": "` + f.otherSvcID.String() + `", "scopes": []},
				{"id": "` + f.service.ID.String() + `", "canonical_id": "repo-api", "scopes": ["read:repo", "write:repo"]}]},
			{"id": "` + f.readerPSID.String() + `", "canonical_id": "repo-reader", "services": [
				{"id": "` + f.service.ID.String() + `", "canonical_id": "repo-api", "scopes": ["read:repo"]}]}
		]}`
	assert.JSONEq(t, expected, string(raw))
	assert.NotContains(t, string(raw), "null")
	assert.Equal(t, SuccessDiagnostic(ExchangeThirdParty), decision.Diagnostic)
}

func TestAccessEvaluation_PermitSatisfiesPublishedSchemaConstraints(t *testing.T) {
	t.Parallel()
	f := newEvaluationFixture(t)
	f.agent.CanonicalID = nil
	f.service.CanonicalID = nil
	f.canonical.ids = map[id.ServiceID]string{}
	f.psRepo.psMap[f.readerPSID].CanonicalID = nil

	decision := f.newService(t).Evaluate(context.Background(), f.request(t), f.token(t, nil))

	require.Equal(t, DecisionPermit, decision.Kind)
	var generic map[string]any
	raw, err := json.Marshal(decision.Context)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &generic))
	assert.NotEmpty(t, generic["subject"].(map[string]any)["id"])
	assert.NotContains(t, generic["agent"], "canonical_id")
	assert.NotContains(t, generic["resource"], "canonical_id")
	assert.NotContains(t, generic["grant"], "valid_until")
	sets := generic["permission_sets"].([]any)
	require.NotEmpty(t, sets)
	for _, set := range sets {
		assert.NotContains(t, set, "canonical_id")
		for _, svc := range set.(map[string]any)["services"].([]any) {
			assert.NotContains(t, svc, "canonical_id")
			assert.IsType(t, []any{}, svc.(map[string]any)["scopes"])
		}
	}
}

func TestAccessEvaluation_MaxAgeBoundedByGrantExpiry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		remaining *time.Duration
		maxAge    int
		denied    bool
	}{
		{name: "no expiry", maxAge: 60},
		{name: "expiry beyond cap", remaining: ptr.To(2 * time.Hour), maxAge: 60},
		{name: "fifteen seconds", remaining: ptr.To(15*time.Second + 900*time.Millisecond), maxAge: 15},
		{name: "subsecond never rounds up", remaining: ptr.To(900 * time.Millisecond), maxAge: 0},
		{name: "expired at evaluation time", remaining: ptr.To(time.Duration(0)), denied: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newEvaluationFixture(t)
			// The grant repository check uses wall time; keep the grant valid there and move the
			// evaluation clock instead.
			validUntil := time.Now().Add(time.Hour)
			f.grant.ValidUntil = &validUntil
			if tt.remaining == nil {
				f.grant.ValidUntil = nil
			} else {
				f.now = validUntil.Add(-*tt.remaining)
			}

			decision := f.newService(t).Evaluate(context.Background(), f.request(t), f.token(t, nil))

			if tt.denied {
				require.Equal(t, DecisionDeny, decision.Kind)
				assert.Equal(t, DetailGrantExpired, decision.Detail)
				return
			}
			require.Equal(t, DecisionPermit, decision.Kind)
			assert.Equal(t, tt.maxAge, decision.MaxAge)
			if tt.remaining != nil {
				assert.Equal(t, validUntil.UTC().Format(time.RFC3339), decision.Context.Grant.ValidUntil)
			}
		})
	}
}

func TestAccessEvaluation_AgentAnchor(t *testing.T) {
	t.Parallel()

	t.Run("matching anchor permits with the same context", func(t *testing.T) {
		t.Parallel()
		f := newEvaluationFixture(t)
		svc := f.newService(t)
		without := svc.Evaluate(context.Background(), f.request(t), f.token(t, nil))
		req := f.request(t)
		req.AgentAnchor = ptr.To(f.agent.ID.String())
		with := svc.Evaluate(context.Background(), req, f.token(t, nil))
		require.Equal(t, DecisionPermit, with.Kind)
		assert.Equal(t, without.Context, with.Context)
	})

	t.Run("mismatched anchor fails before any grant read", func(t *testing.T) {
		t.Parallel()
		f := newEvaluationFixture(t)
		req := f.request(t)
		req.AgentAnchor = ptr.To(id.NewAgentID().String())
		decision := f.newService(t).Evaluate(context.Background(), req, f.token(t, nil))
		require.Equal(t, DecisionRequestFailure, decision.Kind)
		assert.Equal(t, 400, decision.Status)
		assert.Equal(t, DetailRequestMalformed, decision.Diagnostic.Detail())
		assert.Zero(t, f.grantRepo.findByPrincipalAndAgentCalls)
	})

	t.Run("non-UUID anchor fails without resolution", func(t *testing.T) {
		t.Parallel()
		f := newEvaluationFixture(t)
		req := f.request(t)
		req.AgentAnchor = ptr.To("data-assistant")
		decision := f.newService(t).Evaluate(context.Background(), req, f.token(t, nil))
		assert.Equal(t, 400, decision.Status)
		assert.Zero(t, f.grantRepo.findByPrincipalAndAgentCalls)
	})
}

func TestAccessEvaluation_RejectsUnsupportedRequestMembers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*AccessEvaluationRequest)
	}{
		{"identity subject", func(r *AccessEvaluationRequest) { r.SubjectType = "identity" }},
		{"empty subject id", func(r *AccessEvaluationRequest) { r.SubjectID = "" }},
		{"other action", func(r *AccessEvaluationRequest) { r.ActionName = "can_read" }},
		{"other resource type", func(r *AccessEvaluationRequest) { r.ResourceType = "account" }},
		{"relative resource", func(r *AccessEvaluationRequest) { r.ResourceID = "/repos" }},
		{"empty resource", func(r *AccessEvaluationRequest) { r.ResourceID = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newEvaluationFixture(t)
			req := f.request(t)
			tt.mutate(&req)
			decision := f.newService(t).Evaluate(context.Background(), req, f.token(t, nil))
			require.Equal(t, DecisionRequestFailure, decision.Kind)
			assert.Equal(t, 400, decision.Status)
			assert.Equal(t, StageRequestValidation, decision.Diagnostic.Stage())
			assert.Zero(t, f.grantRepo.findByPrincipalAndAgentCalls)
		})
	}
}

func TestAccessEvaluation_ClassifiesOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(*evaluationFixture)
		assertion func(*testing.T, *evaluationFixture) string
		subject   func(*testing.T, *evaluationFixture) string
		kind      DecisionKind
		status    int
		detail    FailureDetail
	}{
		{name: "missing grant denies", setup: func(f *evaluationFixture) { f.grantRepo.grant = nil },
			kind: DecisionDeny, detail: DetailGrantMissing},
		{name: "empty grant denies", setup: func(f *evaluationFixture) { f.grant.GrantedPermissionSets = nil },
			kind: DecisionDeny, detail: DetailGrantEmpty},
		{name: "undeclared permission set denies", setup: func(f *evaluationFixture) { f.agent.PermissionSets = f.agent.PermissionSets[:1] },
			kind: DecisionDeny, detail: DetailGrantPermissionSetUndeclared},
		{name: "missing permission set denies", setup: func(f *evaluationFixture) { delete(f.psRepo.psMap, f.readerPSID) },
			kind: DecisionDeny, detail: DetailGrantPermissionSetMissing},
		{name: "omitted service denies", setup: func(f *evaluationFixture) {
			f.grant.GrantedPermissionSets = f.grant.GrantedPermissionSets[1:]
			f.grant.GrantedPermissionSets[0].IncludedServiceIDs = []id.ServiceID{f.otherSvcID}
		}, kind: DecisionDeny, detail: DetailGrantServiceOmitted},
		{name: "missing service definition denies", setup: func(f *evaluationFixture) {
			f.psRepo.psMap[f.readerPSID].ServiceScopes = nil
			f.psRepo.psMap[f.writerPSID].ServiceScopes = nil
		}, kind: DecisionDeny, detail: DetailGrantServiceDefinitionMissing},
		{name: "excluded service requirement denies", setup: func(f *evaluationFixture) {
			f.agent.ServiceRequirements = f.agent.ServiceRequirements[1:]
		}, kind: DecisionDeny, detail: DetailGrantServiceRequirementExcluded},
		{name: "empty scope intersection denies", setup: func(f *evaluationFixture) {
			f.agent.ServiceRequirements[0].RequiredScopes = []string{"admin:repo"}
		}, kind: DecisionDeny, detail: DetailGrantScopeIntersectionEmpty},
		{name: "caller policy denial is 403", setup: func(f *evaluationFixture) { f.celExpr = "false" },
			kind: DecisionRequestFailure, status: 403, detail: DetailClientPolicyDenied},
		{name: "caller policy sees exchange grant type and empty scope", setup: func(f *evaluationFixture) {
			f.celExpr = `request.grant_type == "urn:ietf:params:oauth:grant-type:token-exchange" && request.scope == "" && request.resource == "https://api.example.com/repos"`
		}, kind: DecisionPermit},
		{name: "expired subject is 400", subject: func(t *testing.T, f *evaluationFixture) string {
			return f.token(t, map[string]interface{}{"exp": time.Now().Add(-time.Hour).Unix()})
		}, kind: DecisionRequestFailure, status: 400, detail: DetailSubjectExpired},
		{name: "invalid assertion is 401", assertion: func(*testing.T, *evaluationFixture) string { return "not-a-jwt" },
			kind: DecisionRequestFailure, status: 401, detail: DetailClientInvalid},
		{name: "unregistered resource is 400", setup: func(f *evaluationFixture) {
			f.serviceRepo.service = nil
			f.serviceRepo.err = NewResourceUnregisteredError()
		}, kind: DecisionRequestFailure, status: 400, detail: DetailResourceUnregistered},
		{name: "missing agent is 500", setup: func(f *evaluationFixture) { f.agentRepo.agent = nil },
			kind: DecisionRequestFailure, status: 500, detail: DetailAgentMissing},
		{name: "grant repository outage is 500, not consent", setup: func(f *evaluationFixture) { f.grantRepo.err = errors.New("connection refused") },
			kind: DecisionRequestFailure, status: 500, detail: DetailGrantRepositoryUnavailable},
		{name: "canonical lookup failure is 500", setup: func(f *evaluationFixture) { f.canonical.err = errors.New("timeout") },
			kind: DecisionRequestFailure, status: 500, detail: DetailResourceRepositoryUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newEvaluationFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}
			subject := f.token(t, nil)
			if tt.subject != nil {
				subject = tt.subject(t, f)
			}
			assertion := f.token(t, nil)
			if tt.assertion != nil {
				assertion = tt.assertion(t, f)
			}
			req := f.request(t)
			req.SubjectID = subject

			decision := f.newService(t).Evaluate(context.Background(), req, assertion)

			require.Equal(t, tt.kind, decision.Kind, "diagnostic %s", decision.Diagnostic.Detail())
			switch tt.kind {
			case DecisionDeny:
				assert.Equal(t, tt.detail, decision.Detail)
				assert.Equal(t, "https://broker.example.com/agents/"+f.agent.ID.String(), decision.ConsentURL)
				assert.Nil(t, decision.Context)
			case DecisionRequestFailure:
				assert.Equal(t, tt.status, decision.Status)
				assert.Equal(t, tt.detail, decision.Diagnostic.Detail())
				assert.Nil(t, decision.Context)
				assert.Empty(t, decision.ConsentURL)
			}
		})
	}
}

func TestAccessEvaluation_OnlyAllowlistedGrantDetailsDeny(t *testing.T) {
	t.Parallel()
	f := newEvaluationFixture(t)
	svc := f.newService(t)
	arbitrary := NewAccessDeniedError("grant_unknown").WithDiagnostic(Diagnostic{
		outcome: OutcomeAuthorizationDenied, stage: StageGrantAuthorization, detail: "grant_unknown"})
	decision := svc.classify(arbitrary)
	assert.Equal(t, DecisionRequestFailure, decision.Kind)
	assert.Equal(t, 500, decision.Status)

	wrongStage := NewAccessDeniedError("denied").WithDiagnostic(Diagnostic{
		outcome: OutcomeAuthorizationDenied, stage: StageIdentityResolution, detail: DetailGrantMissing})
	assert.Equal(t, DecisionRequestFailure, svc.classify(wrongStage).Kind)
	assert.Equal(t, DecisionRequestFailure, svc.classify(errors.New("boom")).Kind)
}

func TestAccessEvaluation_CallerCancellationIsNotConsentDenial(t *testing.T) {
	t.Parallel()
	f := newEvaluationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision := f.newService(t).Evaluate(ctx, f.request(t), f.token(t, nil))
	assert.Equal(t, DecisionRequestFailure, decision.Kind)
	assert.Equal(t, 500, decision.Status)
}
