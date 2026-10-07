package impersonation

import (
	"context"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type stubJWKSProvider struct{}

func (stubJWKSProvider) GetKeySet(context.Context) (jwk.Set, error) { return jwk.NewSet(), nil }
func (stubJWKSProvider) GetKey(context.Context, string) (jwk.Key, error) {
	return nil, assert.AnError
}

type stubIssuer struct {
	called bool
	calls  int
	input  ports.ImpersonationMintInput
}

func (s *stubIssuer) IssueImpersonationToken(_ context.Context, input ports.ImpersonationMintInput) (string, error) {
	s.called = true
	s.calls++
	s.input = input
	return "minted.jwt.token", nil
}

type allowDelegationVerifier struct{}

func (allowDelegationVerifier) VerifyUserDelegation(context.Context, id.Principal, id.AgentID) (ports.UserDelegationStatus, error) {
	return ports.UserDelegationActive, nil
}

type recordingDelegationVerifier struct {
	status    ports.UserDelegationStatus
	err       error
	calls     int
	principal id.Principal
	agentID   id.AgentID
}

func (v *recordingDelegationVerifier) VerifyUserDelegation(_ context.Context, principal id.Principal, agentID id.AgentID) (ports.UserDelegationStatus, error) {
	v.calls++
	v.principal = principal
	v.agentID = agentID
	return v.status, v.err
}

type stubAgentRepository struct {
	ports.AgentRepository
	get          func(context.Context, id.AgentID) (*storage.Agent, error)
	getCanonical func(context.Context, string) (*storage.Agent, error)
}

func (s stubAgentRepository) Get(ctx context.Context, agentID id.AgentID) (*storage.Agent, error) {
	if s.get == nil {
		return nil, assert.AnError
	}
	return s.get(ctx, agentID)
}

func (s stubAgentRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*storage.Agent, error) {
	if s.getCanonical == nil {
		return nil, assert.AnError
	}
	return s.getCanonical(ctx, canonicalID)
}

func testTarget() *Target {
	return &Target{Agent: &storage.Agent{ID: id.NewAgentID(), DisplayName: "Target", Description: "Target agent"}}
}

func testImpersonationConfig(ruleNames ...string) *ports.ImpersonationConfig {
	if len(ruleNames) == 0 {
		ruleNames = []string{"rule-1"}
	}
	rules := make([]ports.ImpersonationRuleConfig, 0, len(ruleNames))
	for _, name := range ruleNames {
		rules = append(rules, ports.ImpersonationRuleConfig{
			Name: name,
			Roles: map[string]ports.ImpersonationRoleConfig{
				"client_assertion": {ExpectedAudience: "aud", PrincipalExpression: "client_assertion.sub"},
				"actor":            {ExpectedAudience: "aud", PrincipalExpression: "actor_token.sub"},
				"subject":          {ExpectedAudience: "aud", PrincipalExpression: "subject_token.sub"},
			},
			TrustedIssuers: []ports.TrustedTokenIssuerConfig{{
				IssuerURI:         "https://idp.example.com",
				AllowedAlgorithms: []string{"RS256"},
				SignsRoles:        []string{"client_assertion", "actor", "subject"},
			}},
			Authorization: ports.AuthorizationConfig{
				Type: "cel",
				CEL:  ports.CELAuthorizationConfig{Expression: "true"},
			},
		})
	}
	return &ports.ImpersonationConfig{AudiencePrefix: "https://broker/impersonation", Rules: rules}
}

func newTestService(t *testing.T, cfg *ports.ImpersonationConfig) (*Service, *stubIssuer) {
	t.Helper()
	issuer := &stubIssuer{}
	factory := func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return stubJWKSProvider{}, nil
	}
	svc, err := NewService(cfg, factory, stubAgentRepository{}, issuer, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
	require.NoError(t, err)
	return svc, issuer
}

type uuidOnlyAgentRepository struct{ ports.AgentRepository }

func TestNewService_RequiresCanonicalIDResolution(t *testing.T) {
	_, err := NewService(testImpersonationConfig(), func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return stubJWKSProvider{}, nil
	}, uuidOnlyAgentRepository{}, &stubIssuer{}, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
	require.Error(t, err)
	assert.ErrorContains(t, err, "canonical ID resolution")
}

func TestNewService_CompilesRules(t *testing.T) {
	svc, _ := newTestService(t, testImpersonationConfig())
	assert.Equal(t, "https://broker/impersonation", svc.audiencePrefix)
}

func TestImpersonate_RequiresActiveUserDelegation(t *testing.T) {
	newService := func(t *testing.T, verifier ports.UserDelegationVerifier, expression string) (*Outcome, *stubIssuer, *Target, error) {
		t.Helper()
		signingKey, keySet := signedValidationKey(t, "delegation")
		cfg := testImpersonationConfig("first", "second")
		for i := range cfg.Rules {
			cfg.Rules[i].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
			cfg.Rules[i].Authorization.CEL.Expression = expression
		}
		issuer := &stubIssuer{}
		svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
			return configurableJWKSProvider{set: keySet}, nil
		}, stubAgentRepository{}, issuer, 0, nil, verifier, "https://broker.example.com/")
		require.NoError(t, err)
		target := testTarget()
		token := signedValidationTokenWithSubject(t, jwa.ES256(), signingKey, "https://idp.example.com", "aud", "subject-1", time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
		outcome, err := svc.Impersonate(context.Background(), &Request{
			ClientAssertion: token, ActorToken: token, SubjectToken: token, SubjectTokenType: JWTTokenType,
		}, target)
		return outcome, issuer, target, err
	}

	t.Run("active delegation mints", func(t *testing.T) {
		verifier := &recordingDelegationVerifier{status: ports.UserDelegationActive}
		outcome, issuer, target, err := newService(t, verifier, "true")
		require.NoError(t, err)
		require.NotNil(t, outcome.Response)
		assert.Equal(t, 1, verifier.calls)
		assert.Equal(t, id.Principal("subject-1"), verifier.principal)
		assert.Equal(t, target.Agent.ID, verifier.agentID)
		assert.True(t, issuer.called)
	})

	for _, tc := range []struct {
		name    string
		status  ports.UserDelegationStatus
		details string
		detail  tokenexchange.FailureDetail
	}{
		{name: "missing delegation", status: ports.UserDelegationMissing, details: "user_grant_missing", detail: tokenexchange.DetailGrantMissing},
		{name: "expired delegation", status: ports.UserDelegationExpired, details: "user_grant_expired", detail: tokenexchange.DetailGrantExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			verifier := &recordingDelegationVerifier{status: tc.status}
			outcome, issuer, target, err := newService(t, verifier, "true")
			var tokenErr *tokenexchange.TokenExchangeError
			require.ErrorAs(t, err, &tokenErr)
			assert.Equal(t, tokenexchange.AccessDeniedError, tokenErr.Code())
			assert.Equal(t, "user delegation is required before impersonation", tokenErr.Description())
			assert.Equal(t, "https://broker.example.com/agents/"+target.Agent.ID.String(), tokenErr.ErrorURI())
			assert.Equal(t, tc.details, tokenErr.Details())
			assert.Equal(t, tc.details, outcome.Audit.FailureCategory)
			assert.Equal(t, tokenexchange.StageGrantAuthorization, tokenErr.Diagnostic().Stage())
			assert.Equal(t, tc.detail, tokenErr.Diagnostic().Detail())
			assert.Equal(t, tokenexchange.RecoveryReconsent, tokenErr.Diagnostic().RecoveryAction())
			assert.Equal(t, tokenexchange.TargetConsent, tokenErr.Diagnostic().RecoveryTarget())
			assert.Equal(t, tokenexchange.ExchangeImpersonation, tokenErr.Diagnostic().ExchangeKind())
			assert.False(t, issuer.called)
			assert.Equal(t, 1, verifier.calls)
		})
	}

	t.Run("verifier error fails closed", func(t *testing.T) {
		verifier := &recordingDelegationVerifier{err: assert.AnError}
		outcome, issuer, _, err := newService(t, verifier, "true")
		var tokenErr *tokenexchange.TokenExchangeError
		require.ErrorAs(t, err, &tokenErr)
		assert.Equal(t, tokenexchange.ServerErrorCode, tokenErr.Code())
		assert.Empty(t, tokenErr.ErrorURI())
		assert.Equal(t, "user_grant_lookup_failed", outcome.Audit.FailureCategory)
		assert.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, tokenexchange.StageGrantAuthorization, tokenErr.Diagnostic().Stage())
		assert.Equal(t, tokenexchange.DetailGrantRepositoryUnavailable, tokenErr.Diagnostic().Detail())
		assert.Equal(t, tokenexchange.OutcomeInfrastructureError, tokenErr.Diagnostic().Outcome())
		assert.Equal(t, tokenexchange.RecoveryRetry, tokenErr.Diagnostic().RecoveryAction())
		assert.Equal(t, tokenexchange.ExchangeImpersonation, tokenErr.Diagnostic().ExchangeKind())
		assert.False(t, issuer.called)
		assert.Equal(t, 1, verifier.calls)
	})

	t.Run("predicate denial does not query delegation", func(t *testing.T) {
		verifier := &recordingDelegationVerifier{status: ports.UserDelegationActive}
		outcome, issuer, _, err := newService(t, verifier, "false")
		var tokenErr *tokenexchange.TokenExchangeError
		require.ErrorAs(t, err, &tokenErr)
		assert.Equal(t, tokenexchange.AccessDeniedError, tokenErr.Code())
		assert.Equal(t, "authorization_denied", outcome.Audit.FailureCategory)
		assert.Equal(t, tokenexchange.StageClientAuthorization, tokenErr.Diagnostic().Stage())
		assert.Equal(t, tokenexchange.DetailClientPolicyDenied, tokenErr.Diagnostic().Detail())
		assert.Equal(t, tokenexchange.RecoveryNone, tokenErr.Diagnostic().RecoveryAction())
		assert.False(t, issuer.called)
		assert.Zero(t, verifier.calls)
	})
}

func TestNewService_RejectsSymmetricAlgorithm(t *testing.T) {
	cfg := testImpersonationConfig()
	cfg.Rules[0].TrustedIssuers[0].AllowedAlgorithms = []string{"HS256"}
	factory := func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return stubJWKSProvider{}, nil
	}
	_, err := NewService(cfg, factory, stubAgentRepository{}, &stubIssuer{}, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allowed_algorithms")
}

// unverifiedSubjectConfig builds a config whose single rule accepts the unverified subject mode:
// verification: none, no expected_audience on the subject, the subject role absent from signs_roles,
// and a predicate that binds subject_token (FR-003d/007a).
func unverifiedSubjectConfig() *ports.ImpersonationConfig {
	return &ports.ImpersonationConfig{
		AudiencePrefix: "https://broker/impersonation",
		Rules: []ports.ImpersonationRuleConfig{{
			Name: "chat-bridge",
			Roles: map[string]ports.ImpersonationRoleConfig{
				"client_assertion": {ExpectedAudience: "aud", PrincipalExpression: "client_assertion.sub"},
				"actor":            {ExpectedAudience: "aud", PrincipalExpression: "actor_token.sub"},
				"subject": {
					Verification:        ports.SubjectVerificationNone,
					PrincipalExpression: "subject_token.sub",
					EmailExpression:     "subject_token.email",
				},
			},
			TrustedIssuers: []ports.TrustedTokenIssuerConfig{{
				IssuerURI:         "https://idp.example.com",
				AllowedAlgorithms: []string{"RS256"},
				SignsRoles:        []string{"client_assertion", "actor"},
			}},
			Authorization: ports.AuthorizationConfig{
				Type: "cel",
				CEL:  ports.CELAuthorizationConfig{Expression: `client_assertion.sub == "gw" && subject_token.sub != ""`},
			},
		}},
	}
}

// FR-003d/007a: an unverified subject rule that binds subject_token compiles successfully.
func TestNewService_CompilesUnverifiedSubjectRule(t *testing.T) {
	svc, _ := newTestService(t, unverifiedSubjectConfig())
	assert.Equal(t, "https://broker/impersonation", svc.audiencePrefix)
}

// FR-007a/CR-005a: an unverified subject rule whose predicate does not reference subject_token
// MUST fail startup.
func TestNewService_RejectsUnverifiedWithoutSubjectBinding(t *testing.T) {
	cfg := unverifiedSubjectConfig()
	cfg.Rules[0].Authorization.CEL.Expression = `client_assertion.sub == "gw"`
	factory := func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return stubJWKSProvider{}, nil
	}
	_, err := NewService(cfg, factory, stubAgentRepository{}, &stubIssuer{}, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subject_token")
}

// A malformed client assertion cannot be resolved to a trusted issuer; the request falls closed
// with invalid_client and the audit record carries no credential value (FR-011, SC-005).
func TestImpersonate_MalformedClientAssertionFailsClosed(t *testing.T) {
	svc, issuer := newTestService(t, testImpersonationConfig())
	req := &Request{
		ClientAssertion:  "not-a-jwt",
		ActorToken:       "also.not.jwt",
		SubjectToken:     "still.not.jwt",
		SubjectTokenType: JWTTokenType,
	}
	outcome, err := svc.Impersonate(context.Background(), req, testTarget())
	require.Error(t, err)
	assert.False(t, issuer.called, "no token should be minted")
	assert.Nil(t, outcome.Response)
	assert.Equal(t, tokenexchange.InvalidClientError, codeOf(t, err))
	assert.Equal(t, tokenexchange.InvalidClientError, outcome.Audit.Outcome)
	// SC-005: the audit record must never carry credential values.
	assert.NotContains(t, outcome.Audit.FailureCategory, "not-a-jwt")
	assert.Empty(t, outcome.Audit.SubjectIdentity)
}
func TestImpersonate_DispatchesCredentialsByRole(t *testing.T) {
	type roleSigner struct {
		issuer   string
		audience string
		key      jwk.Key
		keySet   jwk.Set
	}

	clientKey, clientKeys := signedValidationKey(t, "client")
	actorKey, actorKeys := signedValidationKey(t, "actor")
	subjectKey, subjectKeys := signedValidationKey(t, "subject")
	signers := map[ports.CredentialRole]roleSigner{
		ports.CredentialRoleClientAssertion: {issuer: "https://client.example.com", audience: "client-audience", key: clientKey, keySet: clientKeys},
		ports.CredentialRoleActor:           {issuer: "https://actor.example.com", audience: "actor-audience", key: actorKey, keySet: actorKeys},
		ports.CredentialRoleSubject:         {issuer: "https://subject.example.com", audience: "subject-audience", key: subjectKey, keySet: subjectKeys},
	}
	roles := []ports.CredentialRole{
		ports.CredentialRoleClientAssertion,
		ports.CredentialRoleActor,
		ports.CredentialRoleSubject,
	}
	signersByIssuer := make(map[string]roleSigner, len(signers))
	for _, signer := range signers {
		signersByIssuer[signer.issuer] = signer
	}
	newService := func(t *testing.T) (*Service, *stubIssuer) {
		t.Helper()
		cfg := &ports.ImpersonationConfig{
			AudiencePrefix: "https://broker/impersonation",
			Rules: []ports.ImpersonationRuleConfig{{
				Name: "role-specific-issuers",
				Roles: map[string]ports.ImpersonationRoleConfig{
					"client_assertion": {ExpectedAudience: signers[ports.CredentialRoleClientAssertion].audience, PrincipalExpression: "client_assertion.sub"},
					"actor":            {ExpectedAudience: signers[ports.CredentialRoleActor].audience, PrincipalExpression: "actor_token.sub"},
					"subject":          {ExpectedAudience: signers[ports.CredentialRoleSubject].audience, PrincipalExpression: "subject_token.sub"},
				},
				Authorization: ports.AuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{Expression: "true"}},
			}},
		}
		for _, role := range roles {
			signer := signers[role]
			cfg.Rules[0].TrustedIssuers = append(cfg.Rules[0].TrustedIssuers, ports.TrustedTokenIssuerConfig{
				IssuerURI:         signer.issuer,
				AllowedAlgorithms: []string{"ES256"},
				SignsRoles:        []string{string(role)},
			})
		}
		issuer := &stubIssuer{}
		svc, err := NewService(cfg, func(issuer ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
			return configurableJWKSProvider{set: signersByIssuer[issuer.IssuerURI].keySet}, nil
		}, stubAgentRepository{}, issuer, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
		require.NoError(t, err)
		return svc, issuer
	}
	credential := func(role ports.CredentialRole, audience string) string {
		signer := signers[role]
		return signedValidationToken(t, jwa.ES256(), signer.key, signer.issuer, audience, time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
	}
	baseRequest := func() *Request {
		return &Request{
			ClientAssertion:  credential(ports.CredentialRoleClientAssertion, signers[ports.CredentialRoleClientAssertion].audience),
			ActorToken:       credential(ports.CredentialRoleActor, signers[ports.CredentialRoleActor].audience),
			SubjectToken:     credential(ports.CredentialRoleSubject, signers[ports.CredentialRoleSubject].audience),
			SubjectTokenType: JWTTokenType,
		}
	}

	tests := []struct {
		name     string
		mutate   func(*Request)
		wantCode string
		details  string
	}{
		{"accepts distinct issuers and audiences", nil, "", ""},
		{"rejects client assertion signed by actor issuer", func(req *Request) {
			req.ClientAssertion = credential(ports.CredentialRoleActor, signers[ports.CredentialRoleClientAssertion].audience)
		}, tokenexchange.InvalidClientError, "client_assertion_invalid"},
		{"rejects actor token signed by subject issuer", func(req *Request) {
			req.ActorToken = credential(ports.CredentialRoleSubject, signers[ports.CredentialRoleActor].audience)
		}, tokenexchange.InvalidRequestError, "actor_token_invalid"},
		{"rejects subject token signed by client issuer", func(req *Request) {
			req.SubjectToken = credential(ports.CredentialRoleClientAssertion, signers[ports.CredentialRoleSubject].audience)
		}, tokenexchange.InvalidRequestError, "subject_token_invalid"},
		{"rejects client assertion with actor audience", func(req *Request) {
			req.ClientAssertion = credential(ports.CredentialRoleClientAssertion, signers[ports.CredentialRoleActor].audience)
		}, tokenexchange.InvalidClientError, "client_assertion_invalid"},
		{"rejects actor token with subject audience", func(req *Request) {
			req.ActorToken = credential(ports.CredentialRoleActor, signers[ports.CredentialRoleSubject].audience)
		}, tokenexchange.InvalidRequestError, "actor_token_invalid"},
		{"rejects subject token with client audience", func(req *Request) {
			req.SubjectToken = credential(ports.CredentialRoleSubject, signers[ports.CredentialRoleClientAssertion].audience)
		}, tokenexchange.InvalidRequestError, "subject_token_invalid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, issuer := newService(t)
			req := baseRequest()
			if tc.mutate != nil {
				tc.mutate(req)
			}

			outcome, err := svc.Impersonate(context.Background(), req, testTarget())
			if tc.wantCode == "" {
				require.NoError(t, err)
				require.NotNil(t, outcome.Response)
				assert.Equal(t, 1, issuer.calls)
				assert.Equal(t, signers[ports.CredentialRoleActor].issuer, issuer.input.ActorIssuer)
				assert.Equal(t, []string{signers[ports.CredentialRoleClientAssertion].issuer, signers[ports.CredentialRoleActor].issuer, signers[ports.CredentialRoleSubject].issuer}, outcome.Audit.IssuerIdentifiers)
				assert.Equal(t, []string{"client_assertion", "actor", "subject"}, outcome.Audit.IssuerRoles)
				return
			}

			require.Error(t, err)
			assert.Equal(t, tc.wantCode, codeOf(t, err))
			assert.Nil(t, outcome.Response)
			assert.Equal(t, tc.details, outcome.Audit.FailureCategory)
			assert.Zero(t, issuer.calls)
		})
	}
}
func TestImpersonate_AttributesActorAndSubjectFromTheirRoles(t *testing.T) {
	signingKey, keySet := signedValidationKey(t, "es256")
	issuer := &stubIssuer{}
	cfg := testImpersonationConfig()
	cfg.Rules[0].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
	svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return configurableJWKSProvider{set: keySet}, nil
	}, stubAgentRepository{}, issuer, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
	require.NoError(t, err)

	token := func(subject string) string {
		return signedValidationTokenWithSubject(t, jwa.ES256(), signingKey, "https://idp.example.com", "aud", subject, time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
	}
	outcome, err := svc.Impersonate(context.Background(), &Request{
		ClientAssertion:  token("client-identity"),
		ActorToken:       token("actor-identity"),
		SubjectToken:     token("subject-identity"),
		SubjectTokenType: JWTTokenType,
	}, testTarget())

	require.NoError(t, err)
	require.NotNil(t, outcome.Response)
	assert.Equal(t, "actor-identity", issuer.input.Actor)
	assert.Equal(t, "subject-identity", issuer.input.Subject)
}
func TestImpersonate_TargetScopeValidation(t *testing.T) {
	t.Run("grants target allowed scopes", func(t *testing.T) {
		signingKey, keySet := signedValidationKey(t, "es256")
		issuer := &stubIssuer{}
		cfg := testImpersonationConfig()
		cfg.Rules[0].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
		svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
			return configurableJWKSProvider{set: keySet}, nil
		}, stubAgentRepository{}, issuer, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
		require.NoError(t, err)
		target := testTarget()
		target.Agent.AllowedScopes = []string{"read"}
		valid := signedValidationToken(t, jwa.ES256(), signingKey, "https://idp.example.com", "aud", time.Now().Add(time.Hour), time.Now().Add(-time.Minute))

		outcome, err := svc.Impersonate(context.Background(), &Request{
			ClientAssertion:  valid,
			ActorToken:       valid,
			SubjectToken:     valid,
			SubjectTokenType: JWTTokenType,
			Scope:            "read",
			Scopes:           []string{"read"},
		}, target)

		require.NoError(t, err)
		assert.Equal(t, "read", outcome.Response.Scope)
		assert.True(t, issuer.called)
		assert.Equal(t, []string{"read"}, issuer.input.Scopes)
		assert.Equal(t, "https://idp.example.com", issuer.input.ActorIssuer)
	})
	svc, issuer := newTestService(t, testImpersonationConfig())
	target := testTarget()
	target.Agent.AllowedScopes = []string{"read"}

	outcome, err := svc.Impersonate(context.Background(), &Request{Scope: "read admin", Scopes: []string{"read", "admin"}}, target)

	require.Error(t, err)
	var tokenErr *tokenexchange.TokenExchangeError
	require.ErrorAs(t, err, &tokenErr)
	assert.Equal(t, "invalid_scope", tokenErr.Code())
	assert.Equal(t, "requested scope is not permitted", tokenErr.Description())
	assert.Equal(t, "scope_not_permitted", tokenErr.Details())
	assert.False(t, issuer.called, "no token should be minted")
	assert.Nil(t, outcome.Response)
	assert.Equal(t, target.Agent.ID.String(), outcome.Audit.TargetAgentID)
	assert.Empty(t, outcome.Audit.SelectedRule)
	assert.Empty(t, outcome.Audit.IssuerIdentifiers)
	assert.NotContains(t, outcome.Audit.FailureCategory, "admin")
}

// No-match precedence is order independent across rules (FR-004a): a false predicate anywhere
// yields access_denied even alongside invalid_request/invalid_client failures.
func TestImpersonate_NoMatchPrecedence(t *testing.T) {
	signingKey, keySet := signedValidationKey(t, "es256")
	valid := signedValidationToken(t, jwa.ES256(), signingKey, "https://idp.example.com", "aud", time.Now().Add(time.Hour), time.Now().Add(-time.Minute))

	newRules := func() map[string]ports.ImpersonationRuleConfig {
		cfg := testImpersonationConfig("access-denied", "invalid-request", "invalid-client")
		for i := range cfg.Rules {
			cfg.Rules[i].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
		}
		cfg.Rules[0].Authorization.CEL.Expression = "false"
		actorRole := cfg.Rules[1].Roles["actor"]
		actorRole.ExpectedAudience = "wrong-audience"
		cfg.Rules[1].Roles["actor"] = actorRole
		clientRole := cfg.Rules[2].Roles["client_assertion"]
		clientRole.ExpectedAudience = "wrong-audience"
		cfg.Rules[2].Roles["client_assertion"] = clientRole
		return map[string]ports.ImpersonationRuleConfig{
			"access-denied":   cfg.Rules[0],
			"invalid-request": cfg.Rules[1],
			"invalid-client":  cfg.Rules[2],
		}
	}

	for _, tc := range []struct {
		name  string
		order []string
	}{
		{"access denied first", []string{"access-denied", "invalid-request", "invalid-client"}},
		{"access denied in the middle", []string{"invalid-client", "access-denied", "invalid-request"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rulesByName := newRules()
			cfg := &ports.ImpersonationConfig{AudiencePrefix: "https://broker/impersonation"}
			for _, name := range tc.order {
				cfg.Rules = append(cfg.Rules, rulesByName[name])
			}
			issuer := &stubIssuer{}
			svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
				return configurableJWKSProvider{set: keySet}, nil
			}, stubAgentRepository{}, issuer, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
			require.NoError(t, err)

			outcome, err := svc.Impersonate(context.Background(), &Request{
				ClientAssertion:  valid,
				ActorToken:       valid,
				SubjectToken:     valid,
				SubjectTokenType: JWTTokenType,
			}, testTarget())

			require.Error(t, err)
			assert.Equal(t, tokenexchange.AccessDeniedError, codeOf(t, err))
			assert.Nil(t, outcome.Response)
			assert.Equal(t, tokenexchange.AccessDeniedError, outcome.Audit.Outcome)
			assert.Equal(t, tokenexchange.AccessDeniedError, outcome.Audit.OAuthErrorCode)
			assert.Equal(t, "access-denied", outcome.Audit.SelectedRule)
			assert.Zero(t, issuer.calls)
		})
	}
}

func TestImpersonate_SelectsFirstMatchingRule(t *testing.T) {
	signingKey, keySet := signedValidationKey(t, "es256")
	cfg := testImpersonationConfig("first", "second")
	for i := range cfg.Rules {
		cfg.Rules[i].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
	}
	secondSubject := cfg.Rules[1].Roles["subject"]
	secondSubject.PrincipalExpression = "subject_token.sub"
	cfg.Rules[1].Roles["subject"] = secondSubject
	issuer := &stubIssuer{}
	svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return configurableJWKSProvider{set: keySet}, nil
	}, stubAgentRepository{}, issuer, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
	require.NoError(t, err)

	valid := signedValidationToken(t, jwa.ES256(), signingKey, "https://idp.example.com", "aud", time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
	outcome, err := svc.Impersonate(context.Background(), &Request{
		ClientAssertion:  valid,
		ActorToken:       valid,
		SubjectToken:     valid,
		SubjectTokenType: JWTTokenType,
	}, testTarget())

	require.NoError(t, err)
	require.NotNil(t, outcome.Response)
	assert.Equal(t, "first", outcome.Audit.SelectedRule)
	assert.Equal(t, "subject", issuer.input.Subject)
	assert.Equal(t, 1, issuer.calls)
}

// NewService must fail closed on structurally invalid configs even when internal/config.Validate
// has not run (direct construction path), so startup guarantees do not depend on the loader.
func TestNewService_RejectsInvalidConfigs(t *testing.T) {
	factory := func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return stubJWKSProvider{}, nil
	}
	tests := []struct {
		name   string
		mutate func(c *ports.ImpersonationConfig)
	}{
		{"empty audience prefix", func(c *ports.ImpersonationConfig) { c.AudiencePrefix = "" }},
		{"no rules", func(c *ports.ImpersonationConfig) { c.Rules = nil }},
		{"duplicate rule name", func(c *ports.ImpersonationConfig) { c.Rules = append(c.Rules, c.Rules[0]) }},
		{"unknown role key", func(c *ports.ImpersonationConfig) {
			c.Rules[0].Roles["extra"] = ports.ImpersonationRoleConfig{PrincipalExpression: "x", ExpectedAudience: "aud"}
		}},
		{"missing expected_audience", func(c *ports.ImpersonationConfig) {
			r := c.Rules[0].Roles["actor"]
			r.ExpectedAudience = ""
			c.Rules[0].Roles["actor"] = r
		}},
		{"email on non-subject role", func(c *ports.ImpersonationConfig) {
			r := c.Rules[0].Roles["actor"]
			r.EmailExpression = "actor_token.email"
			c.Rules[0].Roles["actor"] = r
		}},
		{"non-cel authorization", func(c *ports.ImpersonationConfig) { c.Rules[0].Authorization.Type = "opa" }},
		{"cross-role principal expression", func(c *ports.ImpersonationConfig) {
			r := c.Rules[0].Roles["actor"]
			r.PrincipalExpression = "subject_token.sub"
			c.Rules[0].Roles["actor"] = r
		}},
		{"timeout too large", func(c *ports.ImpersonationConfig) {
			c.Rules[0].Authorization.CEL.EvaluationTimeout = 10 * time.Second
		}},
		{"duplicate issuer_uri", func(c *ports.ImpersonationConfig) {
			c.Rules[0].TrustedIssuers = append(c.Rules[0].TrustedIssuers, c.Rules[0].TrustedIssuers[0])
		}},
		{"uncovered signed role", func(c *ports.ImpersonationConfig) {
			c.Rules[0].TrustedIssuers[0].SignsRoles = []string{"client_assertion", "actor"}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testImpersonationConfig()
			tc.mutate(cfg)
			_, err := NewService(cfg, factory, stubAgentRepository{}, &stubIssuer{}, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
			require.Error(t, err)
		})
	}
}

func TestNewService_AudienceRequirementAbsent(t *testing.T) {
	newConfig := func() *ports.ImpersonationConfig {
		cfg := testImpersonationConfig()
		clientAssertion := cfg.Rules[0].Roles["client_assertion"]
		clientAssertion.ExpectedAudience = ""
		clientAssertion.AudienceRequirement = ports.ImpersonationAudienceRequirementAbsent
		cfg.Rules[0].Roles["client_assertion"] = clientAssertion
		cfg.Rules[0].Authorization.CEL.Expression = `client_assertion.sub == "subject"`
		cfg.Rules[0].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
		return cfg
	}

	t.Run("compiles an audience-less client assertion rule", func(t *testing.T) {
		_, err := NewService(newConfig(), func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
			return stubJWKSProvider{}, nil
		}, stubAgentRepository{}, &stubIssuer{}, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
		require.NoError(t, err)
	})

	t.Run("accepts an audience-less client assertion", func(t *testing.T) {
		signingKey, keySet := signedValidationKey(t, "es256")
		issuer := &stubIssuer{}
		svc, err := NewService(newConfig(), func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
			return configurableJWKSProvider{set: keySet}, nil
		}, stubAgentRepository{}, issuer, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
		require.NoError(t, err)

		validAudience := "aud"
		outcome, err := svc.Impersonate(context.Background(), &Request{
			ClientAssertion:  signedValidationTokenWithoutAudience(t, jwa.ES256(), signingKey, "https://idp.example.com", time.Now().Add(time.Hour), time.Now().Add(-time.Minute)),
			ActorToken:       signedValidationToken(t, jwa.ES256(), signingKey, "https://idp.example.com", validAudience, time.Now().Add(time.Hour), time.Now().Add(-time.Minute)),
			SubjectToken:     signedValidationToken(t, jwa.ES256(), signingKey, "https://idp.example.com", validAudience, time.Now().Add(time.Hour), time.Now().Add(-time.Minute)),
			SubjectTokenType: JWTTokenType,
		}, testTarget())
		require.NoError(t, err)
		require.NotNil(t, outcome.Response)
		assert.Equal(t, "success", outcome.Audit.Outcome)
		assert.Equal(t, 1, issuer.calls)
	})

	t.Run("rejects an unbound audience-less role", func(t *testing.T) {
		cfg := newConfig()
		cfg.Rules[0].Authorization.CEL.Expression = `actor_token.sub == "subject"`
		_, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
			return stubJWKSProvider{}, nil
		}, stubAgentRepository{}, &stubIssuer{}, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
		require.Error(t, err)
		assert.ErrorContains(t, err, "audience_requirement")
		assert.ErrorContains(t, err, "client_assertion")
	})
}

func TestNewService_RejectsInvalidAudienceRequirement(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ports.ImpersonationConfig)
	}{
		{"both expected audience and absent requirement", func(cfg *ports.ImpersonationConfig) {
			role := cfg.Rules[0].Roles["actor"]
			role.AudienceRequirement = ports.ImpersonationAudienceRequirementAbsent
			cfg.Rules[0].Roles["actor"] = role
		}},
		{"unknown audience requirement", func(cfg *ports.ImpersonationConfig) {
			role := cfg.Rules[0].Roles["actor"]
			role.ExpectedAudience = ""
			role.AudienceRequirement = "optional"
			cfg.Rules[0].Roles["actor"] = role
		}},
		{"unverified subject audience requirement", func(cfg *ports.ImpersonationConfig) {
			role := cfg.Rules[0].Roles["subject"]
			role.AudienceRequirement = ports.ImpersonationAudienceRequirementAbsent
			cfg.Rules[0].Roles["subject"] = role
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testImpersonationConfig()
			if tc.name == "unverified subject audience requirement" {
				cfg = unverifiedSubjectConfig()
			}
			tc.mutate(cfg)
			_, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
				return stubJWKSProvider{}, nil
			}, stubAgentRepository{}, &stubIssuer{}, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
			require.Error(t, err)
			assert.ErrorContains(t, err, "audience_requirement")
		})
	}
}

func TestImpersonate_VerificationReuseRespectsRuleTrust(t *testing.T) {
	signingKey, keySet := signedValidationKey(t, "shared")
	_, otherSet := signedValidationKey(t, "other")
	token := signedValidationToken(t, jwa.ES256(), signingKey, "https://idp.example.com", "aud", time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
	req := &Request{ClientAssertion: token, ActorToken: token, SubjectToken: token, SubjectTokenType: JWTTokenType}

	for _, tc := range []struct {
		name       string
		change     func(*ports.ImpersonationConfig)
		otherKeys  bool
		wantMinted bool
	}{
		{name: "per-rule audience still checked", change: func(cfg *ports.ImpersonationConfig) {
			role := cfg.Rules[0].Roles["client_assertion"]
			role.ExpectedAudience = "wrong"
			cfg.Rules[0].Roles["client_assertion"] = role
		}, wantMinted: true},
		{name: "different key source cannot reuse signature", change: func(cfg *ports.ImpersonationConfig) {
			cfg.Rules[1].TrustedIssuers[0].JWKSURI = "https://idp.example.com/other-jwks"
		}, otherKeys: true},
		{name: "algorithm policy cannot be inherited", change: func(cfg *ports.ImpersonationConfig) {
			cfg.Rules[1].TrustedIssuers[0].AllowedAlgorithms = []string{"RS256"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testImpersonationConfig("first", "second")
			for i := range cfg.Rules {
				cfg.Rules[i].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
			}
			cfg.Rules[0].Authorization.CEL.Expression = "false"
			tc.change(cfg)
			built := 0
			issuer := &stubIssuer{}
			svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
				built++
				set := keySet
				if tc.otherKeys && built == 2 {
					set = otherSet
				}
				return &configurableJWKSProvider{set: set}, nil
			}, stubAgentRepository{}, issuer, 0, nil, allowDelegationVerifier{}, "https://broker.example.com")
			require.NoError(t, err)
			outcome, err := svc.Impersonate(context.Background(), req, testTarget())
			if tc.wantMinted {
				require.NoError(t, err)
				require.NotNil(t, outcome.Response)
				assert.Equal(t, "second", outcome.Audit.SelectedRule)
				assert.Equal(t, 1, issuer.calls)
			} else {
				require.Error(t, err)
				assert.Nil(t, outcome.Response)
				assert.Zero(t, issuer.calls)
			}
		})
	}
}
