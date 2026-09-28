package impersonation

import (
	"fmt"
	"reflect"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// JWKSProviderFactory builds a JWKS provider for a trusted issuer. It is supplied by the app
// builder so the domain compiles rules without creating adapters (Principle VI).
type JWKSProviderFactory func(issuer ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error)

// compiledRole holds the resolved per-role semantics of a rule (data-model §1).
type compiledRole struct {
	role                  ports.CredentialRole
	expectedAudience      string
	requireAbsentAudience bool // signed role only; true when audience_requirement is "absent" (CR-009)
	principal             *extractionProgram
	email                 *extractionProgram // subject role only; nil when no email extraction
	unverified            bool               // subject role only; true when verification is "none" (FR-003d)
}

// compiledIssuer pairs a trusted issuer with its signed-credential validator and the signed
// roles it may sign (CR-008).
type compiledIssuer struct {
	issuerURI  string
	signsRoles map[ports.CredentialRole]bool
	validator  *signedValidator
}

// compiledRule is a startup-compiled impersonation rule ready for first-match evaluation.
type compiledRule struct {
	name    string
	roles   map[ports.CredentialRole]*compiledRole
	issuers []*compiledIssuer
	authz   *authorizationProgram
}

// compileRule compiles a rule's authorization predicate, per-role extraction, and per-issuer
// validators at startup (fail-fast). It fails when required roles are missing or expressions
// are invalid.
func compileRule(cfg ports.ImpersonationRuleConfig, factory JWKSProviderFactory, clockSkew time.Duration) (*compiledRule, error) {
	if err := validateRuleInvariants(cfg); err != nil {
		return nil, err
	}

	timeout := cfg.Authorization.CEL.EvaluationTimeout
	authz, err := compileAuthorization(cfg.Authorization.CEL.Expression, timeout)
	if err != nil {
		return nil, fmt.Errorf("authorization: %w", err)
	}

	roles := make(map[ports.CredentialRole]*compiledRole, len(cfg.Roles))
	for name, roleCfg := range cfg.Roles {
		role := ports.CredentialRole(name)
		principal, err := compileExtraction(role, roleCfg.PrincipalExpression, timeout)
		if err != nil {
			return nil, fmt.Errorf("role %q principal_expression: %w", name, err)
		}
		if role == ports.CredentialRoleSubject {
			if roleCfg.Verification != "" &&
				roleCfg.Verification != ports.SubjectVerificationJWKS &&
				roleCfg.Verification != ports.SubjectVerificationNone {
				return nil, fmt.Errorf("rule %q: subject verification must be %q or %q, got %q", cfg.Name, ports.SubjectVerificationJWKS, ports.SubjectVerificationNone, roleCfg.Verification)
			}
		}
		compiled := &compiledRole{
			role:                  role,
			expectedAudience:      roleCfg.ExpectedAudience,
			requireAbsentAudience: roleCfg.AudienceRequirement == ports.ImpersonationAudienceRequirementAbsent,
			principal:             principal,
			unverified:            role == ports.CredentialRoleSubject && roleCfg.Verification == ports.SubjectVerificationNone,
		}
		if role == ports.CredentialRoleSubject && roleCfg.EmailExpression != "" {
			email, err := compileExtraction(ports.CredentialRoleSubject, roleCfg.EmailExpression, timeout)
			if err != nil {
				return nil, fmt.Errorf("role %q email_expression: %w", name, err)
			}
			compiled.email = email
		}
		roles[role] = compiled
	}

	for _, requiredRole := range []ports.CredentialRole{
		ports.CredentialRoleClientAssertion,
		ports.CredentialRoleActor,
		ports.CredentialRoleSubject,
	} {
		if roles[requiredRole] == nil {
			return nil, fmt.Errorf("rule %q is missing the %q role", cfg.Name, requiredRole)
		}
	}

	issuers := make([]*compiledIssuer, 0, len(cfg.TrustedIssuers))
	for _, issuerCfg := range cfg.TrustedIssuers {
		if len(issuerCfg.AllowedAlgorithms) == 0 {
			return nil, fmt.Errorf("issuer %q: allowed_algorithms must not be empty", issuerCfg.IssuerURI)
		}
		allowed := make(map[jwa.SignatureAlgorithm]struct{}, len(issuerCfg.AllowedAlgorithms))
		for _, name := range issuerCfg.AllowedAlgorithms {
			alg, ok := jwa.LookupSignatureAlgorithm(name)
			if !ok || !isApprovedAlgorithm(alg) {
				return nil, fmt.Errorf("issuer %q: allowed_algorithms contains a non-approved algorithm %q (asymmetric only; none and HS* are rejected)", issuerCfg.IssuerURI, name)
			}
			allowed[alg] = struct{}{}
		}
		if len(issuerCfg.SignsRoles) == 0 {
			return nil, fmt.Errorf("issuer %q: signs_roles must not be empty", issuerCfg.IssuerURI)
		}
		provider, err := factory(issuerCfg)
		if err != nil {
			return nil, fmt.Errorf("issuer %q: %w", issuerCfg.IssuerURI, err)
		}
		signs := make(map[ports.CredentialRole]bool, len(issuerCfg.SignsRoles))
		for _, r := range issuerCfg.SignsRoles {
			signs[ports.CredentialRole(r)] = true
		}
		issuers = append(issuers, &compiledIssuer{
			issuerURI:  issuerCfg.IssuerURI,
			signsRoles: signs,
			validator: &signedValidator{
				jwksProvider:      provider,
				issuerURI:         issuerCfg.IssuerURI,
				allowedAlgorithms: allowed,
				clockSkew:         clockSkew,
				// Value providers have no stable instance identity for verification reuse.
				cacheByProvider: reflect.TypeOf(provider).Kind() == reflect.Pointer,
			},
		})
	}

	// FR-007a / CR-005a: a rule that accepts the unverified subject mode MUST bind the subject in its
	// single authorization predicate; the compiled expression MUST reference subject_token. Enforced
	// at startup (fail-fast) from the checked AST so an unbound unverified rule can never start.
	if roles[ports.CredentialRoleSubject].unverified && !authz.References(celSubjectToken) {
		return nil, fmt.Errorf("rule %q: the unverified subject mode requires the authorization predicate to reference subject_token (FR-007a)", cfg.Name)
	}

	// CR-009: a role that drops the aud binding via audience_requirement: absent MUST be bound by the
	// rule's authorization predicate, so an audience-less credential can never be accepted unconstrained.
	for _, role := range []ports.CredentialRole{
		ports.CredentialRoleClientAssertion,
		ports.CredentialRoleActor,
		ports.CredentialRoleSubject,
	} {
		if roles[role].requireAbsentAudience && !authz.References(extractionVariable(role)) {
			return nil, fmt.Errorf("rule %q: role %q uses audience_requirement %q and requires the authorization predicate to reference %s (CR-009)",
				cfg.Name, role, ports.ImpersonationAudienceRequirementAbsent, extractionVariable(role))
		}
	}

	return &compiledRule{
		name:    cfg.Name,
		roles:   roles,
		issuers: issuers,
		authz:   authz,
	}, nil
}

// validateRuleInvariants enforces the structural rule invariants (CR-003, CR-005, CR-008) at
// startup, independently of internal/config.Validate, so a directly constructed config cannot
// start the broker with an invalid impersonation rule (fail closed regardless of the loader).
func validateRuleInvariants(cfg ports.ImpersonationRuleConfig) error {
	if cfg.Authorization.Type != "cel" {
		return fmt.Errorf("rule %q: authorization.type must be \"cel\"", cfg.Name)
	}
	if t := cfg.Authorization.CEL.EvaluationTimeout; t != 0 && (t < 10*time.Millisecond || t > 5*time.Second) {
		return fmt.Errorf("rule %q: authorization.cel.evaluation_timeout must be between 10ms and 5s", cfg.Name)
	}

	for name := range cfg.Roles {
		switch ports.CredentialRole(name) {
		case ports.CredentialRoleClientAssertion, ports.CredentialRoleActor, ports.CredentialRoleSubject:
		default:
			return fmt.Errorf("rule %q: unknown role %q (allowed: client_assertion, actor, subject)", cfg.Name, name)
		}
	}
	allRoles := []ports.CredentialRole{
		ports.CredentialRoleClientAssertion,
		ports.CredentialRoleActor,
		ports.CredentialRoleSubject,
	}
	for _, role := range allRoles {
		roleCfg, ok := cfg.Roles[string(role)]
		if !ok {
			return fmt.Errorf("rule %q: missing the %q role", cfg.Name, role)
		}
		if roleCfg.PrincipalExpression == "" {
			return fmt.Errorf("rule %q role %q: principal_expression is required", cfg.Name, role)
		}
		absentAudience := roleCfg.AudienceRequirement == ports.ImpersonationAudienceRequirementAbsent
		if roleCfg.AudienceRequirement != "" && !absentAudience {
			return fmt.Errorf("rule %q role %q: audience_requirement must be %q or unset", cfg.Name, role, ports.ImpersonationAudienceRequirementAbsent)
		}
		unverifiedSubject := role == ports.CredentialRoleSubject && roleCfg.Verification == ports.SubjectVerificationNone
		switch {
		case unverifiedSubject:
			if absentAudience {
				return fmt.Errorf("rule %q role %q: audience_requirement is not valid for the unverified subject mode", cfg.Name, role)
			}
			if roleCfg.ExpectedAudience != "" {
				return fmt.Errorf("rule %q role %q: expected_audience must be empty for the unverified subject mode", cfg.Name, role)
			}
		case absentAudience:
			if roleCfg.ExpectedAudience != "" {
				return fmt.Errorf("rule %q role %q: expected_audience must be empty when audience_requirement is %q", cfg.Name, role, ports.ImpersonationAudienceRequirementAbsent)
			}
		case roleCfg.ExpectedAudience == "":
			return fmt.Errorf("rule %q role %q: expected_audience is required for a signed role", cfg.Name, role)
		}
		if role != ports.CredentialRoleSubject {
			if roleCfg.EmailExpression != "" {
				return fmt.Errorf("rule %q role %q: email_expression is only valid on the subject role", cfg.Name, role)
			}
			if roleCfg.Verification != "" {
				return fmt.Errorf("rule %q role %q: verification is only valid on the subject role", cfg.Name, role)
			}
		}
	}

	if len(cfg.TrustedIssuers) == 0 {
		return fmt.Errorf("rule %q: at least one trusted_issuer is required", cfg.Name)
	}
	subjectUnverified := cfg.Roles[string(ports.CredentialRoleSubject)].Verification == ports.SubjectVerificationNone
	seenIssuer := make(map[string]bool)
	covered := make(map[ports.CredentialRole]bool)
	for _, issuer := range cfg.TrustedIssuers {
		if issuer.IssuerURI == "" {
			return fmt.Errorf("rule %q: trusted_issuer issuer_uri is required", cfg.Name)
		}
		if seenIssuer[issuer.IssuerURI] {
			return fmt.Errorf("rule %q: duplicate trusted_issuer issuer_uri %q", cfg.Name, issuer.IssuerURI)
		}
		seenIssuer[issuer.IssuerURI] = true
		for _, r := range issuer.SignsRoles {
			role := ports.CredentialRole(r)
			switch role {
			case ports.CredentialRoleClientAssertion, ports.CredentialRoleActor:
				covered[role] = true
			case ports.CredentialRoleSubject:
				// CR-008: an unverified subject role must never be signed by any trusted issuer.
				if subjectUnverified {
					return fmt.Errorf("rule %q: the unverified subject role (verification: none) must not appear in any signs_roles", cfg.Name)
				}
				covered[role] = true
			default:
				return fmt.Errorf("rule %q: issuer %q signs unknown role %q", cfg.Name, issuer.IssuerURI, r)
			}
		}
	}
	// Only signed roles must be covered by a trusted issuer; an unverified subject is issuer-less.
	signedRoles := []ports.CredentialRole{ports.CredentialRoleClientAssertion, ports.CredentialRoleActor}
	if !subjectUnverified {
		signedRoles = append(signedRoles, ports.CredentialRoleSubject)
	}
	for _, role := range signedRoles {
		if !covered[role] {
			return fmt.Errorf("rule %q: no trusted_issuer signs the %q role", cfg.Name, role)
		}
	}
	return nil
}

// issuerFor returns the unique trusted issuer that both matches iss and is authorized to sign
// role, or nil when none does (CR-008). Uniqueness of issuer_uri within a rule is enforced at
// startup, so at most one issuer matches.
func (r *compiledRule) issuerFor(role ports.CredentialRole, iss string) *compiledIssuer {
	for _, issuer := range r.issuers {
		if issuer.issuerURI == iss && issuer.signsRoles[role] {
			return issuer
		}
	}
	return nil
}

// unverifiedIssuer reads the iss claim from a compact JWT without verifying its signature. Used
// to select the trusted issuer before validation; the selected issuer's validator re-checks iss.
func unverifiedIssuer(tokenString string) (string, error) {
	token, err := jwt.ParseString(tokenString, jwt.WithVerify(false), jwt.WithValidate(false))
	if err != nil {
		return "", fmt.Errorf("not a parseable JWT: %w", err)
	}
	iss, _ := token.Issuer()
	return iss, nil
}

// clockSkewOrDefault returns the configured clock skew or the default.
func clockSkewOrDefault(skew time.Duration) time.Duration {
	if skew <= 0 {
		return defaultClockSkew
	}
	return skew
}
