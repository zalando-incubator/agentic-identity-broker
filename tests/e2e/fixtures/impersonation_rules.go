package fixtures

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const ImpersonationAudience = "https://broker.example.com/impersonation"

func SignedImpersonationRule(issuer, jwks string, categories []string) ports.ImpersonationRuleConfig {
	return ports.ImpersonationRuleConfig{
		Name:                     "subject-eligibility",
		AllowedSubjectCategories: categories,
		Roles: map[string]ports.ImpersonationRoleConfig{
			"client_assertion": {ExpectedAudience: ImpersonationAudience, PrincipalExpression: "client_assertion.sub"},
			"actor":            {ExpectedAudience: ImpersonationAudience, PrincipalExpression: "actor_token.sub"},
			"subject":          {Verification: ports.SubjectVerificationJWKS, ExpectedAudience: ImpersonationAudience, PrincipalExpression: "subject_token.sub"},
		},
		TrustedIssuers: []ports.TrustedTokenIssuerConfig{{
			IssuerURI: issuer, JWKSURI: jwks,
			JWKSMinRefresh: 15 * time.Minute, JWKSMaxRefresh: time.Hour,
			AllowedAlgorithms: []string{"RS256"}, SignsRoles: []string{"client_assertion", "actor", "subject"},
		}},
		Authorization: ports.ImpersonationAuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{
			Expression:        `client_assertion.sub == "gateway-prod" && actor_token.sub == "research-runner" && subject_token.sub != ""`,
			EvaluationTimeout: time.Second,
		}},
	}
}

func ImpersonationTarget(clientID string) *storage.Agent {
	agent := AgentWithClientID(clientID)
	canonical := clientID
	agent.CanonicalID = &canonical
	agent.AllowedScopes = []string{"read", "write"}
	return agent
}

// ImpersonationRuleSpec describes one impersonation rule's credential mode independently of its
// authorization engine, so CEL and OPA variants of the same rule are built from one definition.
type ImpersonationRuleSpec struct {
	Name       string
	IssuerURI  string
	JWKSURI    string
	Categories []string
	// UnverifiedSubject selects roles.subject.verification: none.
	UnverifiedSubject bool
	// AbsentAudience lists the signed roles with audience_requirement: absent.
	AbsentAudience []string
	// Email adds the subject email_expression.
	Email bool
}

// CEL builds the rule with a CEL predicate.
func (s ImpersonationRuleSpec) CEL(expression string) ports.ImpersonationRuleConfig {
	rule := s.rule()
	rule.Authorization = ports.ImpersonationAuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{
		Expression: expression, EvaluationTimeout: time.Second,
	}}
	return rule
}

// OPA builds the rule with a decision in the shared OPA engine. A zero timeout keeps the default.
func (s ImpersonationRuleSpec) OPA(pkg, decision string, timeout time.Duration) ports.ImpersonationRuleConfig {
	rule := s.rule()
	rule.Authorization = ports.ImpersonationAuthorizationConfig{Type: "opa", OPA: ports.ImpersonationRuleOPADecision{
		Package: pkg, Decision: decision, EvaluationTimeout: timeout,
	}}
	return rule
}

func (s ImpersonationRuleSpec) rule() ports.ImpersonationRuleConfig {
	absent := map[string]bool{}
	for _, role := range s.AbsentAudience {
		absent[role] = true
	}
	signed := func(role, principal string) ports.ImpersonationRoleConfig {
		if absent[role] {
			return ports.ImpersonationRoleConfig{AudienceRequirement: ports.ImpersonationAudienceRequirementAbsent, PrincipalExpression: principal}
		}
		return ports.ImpersonationRoleConfig{ExpectedAudience: ImpersonationAudience, PrincipalExpression: principal}
	}
	subject := signed("subject", "subject_token.sub")
	signsRoles := []string{"client_assertion", "actor", "subject"}
	if s.UnverifiedSubject {
		subject = ports.ImpersonationRoleConfig{Verification: ports.SubjectVerificationNone, PrincipalExpression: "subject_token.sub"}
		signsRoles = []string{"client_assertion", "actor"}
	}
	if s.Email {
		subject.EmailExpression = "has(subject_token.email) ? subject_token.email : ''"
	}
	return ports.ImpersonationRuleConfig{
		Name:                     s.Name,
		AllowedSubjectCategories: s.Categories,
		Roles: map[string]ports.ImpersonationRoleConfig{
			"client_assertion": signed("client_assertion", "client_assertion.sub"),
			"actor":            signed("actor", "actor_token.sub"),
			"subject":          subject,
		},
		TrustedIssuers: []ports.TrustedTokenIssuerConfig{{
			IssuerURI: s.IssuerURI, JWKSURI: s.JWKSURI,
			JWKSMinRefresh: 15 * time.Minute, JWKSMaxRefresh: time.Hour,
			AllowedAlgorithms: []string{"RS256"}, SignsRoles: signsRoles,
		}},
	}
}
