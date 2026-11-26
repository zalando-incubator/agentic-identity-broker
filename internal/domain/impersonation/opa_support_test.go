package impersonation

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// fakeRevision is a domain-local ImpersonationPolicyRevision double.
type fakeRevision struct {
	label, admissionID string
	evaluate           func(ctx context.Context, rule string, input ports.ImpersonationPolicyInput) (ports.ImpersonationPolicyAction, error)

	mu     sync.Mutex
	inputs map[string][]ports.ImpersonationPolicyInput
}

func allowingRevision(label, admissionID string) *fakeRevision {
	return &fakeRevision{label: label, admissionID: admissionID, evaluate: func(context.Context, string, ports.ImpersonationPolicyInput) (ports.ImpersonationPolicyAction, error) {
		return ports.ImpersonationPolicyAllow, nil
	}}
}

func decidingRevision(label string, decide func(rule string) (ports.ImpersonationPolicyAction, error)) *fakeRevision {
	return &fakeRevision{label: label, admissionID: label + "-id", evaluate: func(_ context.Context, rule string, _ ports.ImpersonationPolicyInput) (ports.ImpersonationPolicyAction, error) {
		return decide(rule)
	}}
}

func (r *fakeRevision) Label() string       { return r.label }
func (r *fakeRevision) AdmissionID() string { return r.admissionID }

func (r *fakeRevision) Evaluate(ctx context.Context, rule string, input ports.ImpersonationPolicyInput) (ports.ImpersonationPolicyAction, error) {
	r.mu.Lock()
	if r.inputs == nil {
		r.inputs = map[string][]ports.ImpersonationPolicyInput{}
	}
	r.inputs[rule] = append(r.inputs[rule], input)
	r.mu.Unlock()
	return r.evaluate(ctx, rule, input)
}

func (r *fakeRevision) lastInput(rule string) ports.ImpersonationPolicyInput {
	r.mu.Lock()
	defer r.mu.Unlock()
	inputs := r.inputs[rule]
	if len(inputs) == 0 {
		return nil
	}
	return inputs[len(inputs)-1]
}

func (r *fakeRevision) evaluations(rule string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.inputs[rule])
}

// fakePolicySource is a domain-local ImpersonationPolicySource double.
type fakePolicySource struct {
	current  atomic.Pointer[ports.ImpersonationPolicyRevision]
	ready    func(ctx context.Context) error
	stopped  atomic.Int32
	afterGet func()
}

func newFakePolicySource() *fakePolicySource {
	s := &fakePolicySource{}
	s.set(allowingRevision("bundle=r1", "instance-1"))
	return s
}

func (s *fakePolicySource) set(rev ports.ImpersonationPolicyRevision) {
	s.current.Store(&rev)
}

func (s *fakePolicySource) AwaitReady(ctx context.Context) error {
	if s.ready != nil {
		return s.ready(ctx)
	}
	return nil
}

func (s *fakePolicySource) Current() ports.ImpersonationPolicyRevision {
	rev := s.current.Load()
	if s.afterGet != nil {
		s.afterGet()
	}
	if rev == nil {
		return nil
	}
	return *rev
}

func (s *fakePolicySource) Stop(context.Context) { s.stopped.Add(1) }

func sourceFactory(source ports.ImpersonationPolicySource) PolicySourceFactory {
	return func(ports.ImpersonationOPAEngineConfig, []ports.ImpersonationPolicyDecisionRef, ports.ImpersonationPolicyAdmission) (ports.ImpersonationPolicySource, error) {
		return source, nil
	}
}

func newTestServiceWithPolicies(t *testing.T, cfg *ports.ImpersonationConfig, policies PolicySourceFactory) (*Service, error) {
	t.Helper()
	jwks := func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return stubJWKSProvider{}, nil
	}
	return NewService(cfg, jwks, stubAgentRepository{}, &stubIssuer{}, 0, nil, allowDelegationVerifier{}, "https://broker.example.com", policies, "https://broker.example.com")
}

// opaHarness builds services with OPA and CEL rules over one ES256 issuer, a fake policy source,
// and an agent registry holding the target.
type opaHarness struct {
	t        *testing.T
	key      jwk.Key
	keyset   jwk.Set
	source   *fakePolicySource
	issuer   *stubIssuer
	verifier *recordingDelegationVerifier
	agents   map[id.AgentID]*storage.Agent
	target   *Target
}

const opaTestIssuer = "https://idp.example.com"

func newOPAHarness(t *testing.T) *opaHarness {
	t.Helper()
	key, keyset := signedValidationKey(t, "opa")
	canonical := "inventory-assistant"
	external := id.NewExternalID("inventory-agent")
	target := &Target{Agent: &storage.Agent{ID: id.NewAgentID(), DisplayName: "Target", Description: "Target agent", CanonicalID: &canonical, ExternalID: &external}}
	return &opaHarness{
		t: t, key: key, keyset: keyset,
		source:   newFakePolicySource(),
		issuer:   &stubIssuer{},
		verifier: &recordingDelegationVerifier{status: ports.UserDelegationActive},
		agents:   map[id.AgentID]*storage.Agent{target.Agent.ID: target.Agent},
		target:   target,
	}
}

func opaRule(name string, categories ...string) ports.ImpersonationRuleConfig {
	rule := testImpersonationConfig(name).Rules[0]
	rule.AllowedSubjectCategories = categories
	rule.TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
	subject := rule.Roles["subject"]
	subject.EmailExpression = "has(subject_token.email) ? subject_token.email : ''"
	rule.Roles = map[string]ports.ImpersonationRoleConfig{
		"client_assertion": rule.Roles["client_assertion"], "actor": rule.Roles["actor"], "subject": subject,
	}
	rule.Authorization = ports.ImpersonationAuthorizationConfig{Type: "opa"}
	return rule
}

func celRule(name, expression string, categories ...string) ports.ImpersonationRuleConfig {
	rule := opaRule(name, categories...)
	rule.Authorization = ports.ImpersonationAuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{Expression: expression}}
	return rule
}

// unverified switches the rule to an unsigned subject.
func unverified(rule ports.ImpersonationRuleConfig) ports.ImpersonationRuleConfig {
	rule.Roles = map[string]ports.ImpersonationRoleConfig{
		"client_assertion": rule.Roles["client_assertion"],
		"actor":            rule.Roles["actor"],
		"subject":          {Verification: ports.SubjectVerificationNone, PrincipalExpression: "subject_token.sub", EmailExpression: rule.Roles["subject"].EmailExpression},
	}
	rule.TrustedIssuers[0].SignsRoles = []string{"client_assertion", "actor"}
	return rule
}

// audienceAbsent switches the listed roles to audience_requirement: absent.
func audienceAbsent(rule ports.ImpersonationRuleConfig, roles ...string) ports.ImpersonationRuleConfig {
	updated := map[string]ports.ImpersonationRoleConfig{}
	for name, role := range rule.Roles {
		updated[name] = role
	}
	for _, name := range roles {
		role := updated[name]
		role.ExpectedAudience = ""
		role.AudienceRequirement = ports.ImpersonationAudienceRequirementAbsent
		updated[name] = role
	}
	rule.Roles = updated
	return rule
}

func (h *opaHarness) service(rules ...ports.ImpersonationRuleConfig) (*Service, error) {
	h.t.Helper()
	cfg := &ports.ImpersonationConfig{AudiencePrefix: "https://broker/impersonation", Rules: rules}
	for _, rule := range rules {
		if rule.Authorization.Type == "opa" {
			cfg.OPA = ports.ImpersonationOPAEngineConfig{Policy: ports.ImpersonationOPAPolicySource{Path: "/policy"}}
		}
	}
	agents := stubAgentRepository{get: func(_ context.Context, agentID id.AgentID) (*storage.Agent, error) {
		if agent, ok := h.agents[agentID]; ok {
			return agent, nil
		}
		return nil, ports.ErrNotFound
	}}
	return NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return configurableJWKSProvider{set: h.keyset}, nil
	}, agents, h.issuer, 0, nil, h.verifier, "https://broker.example.com", sourceFactory(h.source), "https://broker.example.com")
}

func (h *opaHarness) mustService(rules ...ports.ImpersonationRuleConfig) *Service {
	h.t.Helper()
	svc, err := h.service(rules...)
	require.NoError(h.t, err)
	return svc
}

// sign issues an ES256 credential. A nil audience omits aud.
func (h *opaHarness) sign(sub string, audience *string, claims map[string]any) string {
	h.t.Helper()
	builder := jwt.NewBuilder().Issuer(opaTestIssuer).Subject(sub).Expiration(time.Now().Add(time.Hour)).NotBefore(time.Now().Add(-time.Minute))
	if audience != nil {
		builder = builder.Audience([]string{*audience})
	}
	for name, value := range claims {
		builder = builder.Claim(name, value)
	}
	token, err := builder.Build()
	require.NoError(h.t, err)
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), h.key))
	require.NoError(h.t, err)
	return string(signed)
}

func (h *opaHarness) signed(sub string, claims map[string]any) string {
	aud := "aud"
	return h.sign(sub, &aud, claims)
}

func (h *opaHarness) request(subjectToken string) *Request {
	return &Request{
		ClientAssertion:  h.signed("gw", nil),
		ActorToken:       h.signed("actor", nil),
		SubjectToken:     subjectToken,
		SubjectTokenType: JWTTokenType,
		Scope:            "read",
		Scopes:           []string{"read"},
		HTTP: RequestAttributes{
			Method: "POST", Path: "/oauth2/token", Protocol: "HTTP/1.1",
			ContentType: "application/x-www-form-urlencoded",
			Headers:     map[string][]string{"Content-Type": {"application/x-www-form-urlencoded"}},
			Form:        map[string][]string{"grant_type": {GrantType}, "scope": {"read"}, "subject_token": {subjectToken}},
		},
	}
}

func exchangeError(t *testing.T, err error) *tokenexchange.TokenExchangeError {
	t.Helper()
	var exchangeErr *tokenexchange.TokenExchangeError
	require.ErrorAs(t, err, &exchangeErr)
	return exchangeErr
}
