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
		Authorization: ports.AuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{
			Expression:        `client_assertion.sub == "gateway-prod" && actor_token.sub == "research-runner" && subject_token.sub != ""`,
			EvaluationTimeout: time.Second,
		}},
	}
}

func HumanImpersonationRule(issuer, jwks string) ports.ImpersonationRuleConfig {
	return SignedImpersonationRule(issuer, jwks, []string{"human"})
}

func AgentImpersonationRule(issuer, jwks string) ports.ImpersonationRuleConfig {
	return SignedImpersonationRule(issuer, jwks, []string{"agent"})
}

func DualImpersonationRule(issuer, jwks string) ports.ImpersonationRuleConfig {
	return SignedImpersonationRule(issuer, jwks, []string{"human", "agent"})
}

func ImpersonationTarget(clientID string) *storage.Agent {
	agent := AgentWithClientID(clientID)
	canonical := clientID
	agent.CanonicalID = &canonical
	agent.AllowedScopes = []string{"read", "write"}
	return agent
}
