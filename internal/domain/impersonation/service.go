package impersonation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Service orchestrates impersonation rule selection, credential validation, authorization,
// target-agent context, and local token minting. It is safe for concurrent use.
type Service struct {
	audiencePrefix     string
	rules              []*compiledRule
	agents             ports.AgentRepository
	canonicalAgents    ports.AgentCanonicalIDRepository
	issuer             ports.ImpersonationTokenIssuer
	delegationVerifier ports.UserDelegationVerifier
	consentBaseURL     string
	logger             *slog.Logger
	ledger             *ledger.Service
}

// NewService compiles the impersonation rules at startup and validates direct construction.
func NewService(
	cfg *ports.ImpersonationConfig,
	factory JWKSProviderFactory,
	agents ports.AgentRepository,
	issuer ports.ImpersonationTokenIssuer,
	clockSkew time.Duration,
	logger *slog.Logger,
	delegationVerifier ports.UserDelegationVerifier,
	consentBaseURL string,
	recorder *ledger.Service,
) (*Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("impersonation config is nil")
	}
	if agents == nil {
		return nil, fmt.Errorf("impersonation agent repository is nil")
	}
	canonicalAgents, ok := agents.(ports.AgentCanonicalIDRepository)
	if !ok {
		return nil, fmt.Errorf("impersonation agent repository must support canonical ID resolution")
	}
	if issuer == nil {
		return nil, fmt.Errorf("impersonation token issuer is nil")
	}
	if delegationVerifier == nil {
		return nil, fmt.Errorf("impersonation user delegation verifier is nil")
	}
	if consentBaseURL == "" {
		return nil, fmt.Errorf("impersonation consent base URL is required")
	}
	if recorder == nil {
		return nil, fmt.Errorf("impersonation recorder is required")
	}
	if err := ValidateAudiencePrefix(cfg.AudiencePrefix); err != nil {
		return nil, err
	}
	if len(cfg.Rules) == 0 {
		return nil, fmt.Errorf("at least one impersonation rule is required")
	}
	seenNames := make(map[string]bool)
	rules := make([]*compiledRule, 0, len(cfg.Rules))
	for i, ruleCfg := range cfg.Rules {
		if ruleCfg.Name == "" {
			return nil, fmt.Errorf("oauth2_authorization_server.impersonation.rules[%d]: name is required", i)
		}
		if seenNames[ruleCfg.Name] {
			return nil, fmt.Errorf("oauth2_authorization_server.impersonation.rules[%d]: duplicate rule name %q", i, ruleCfg.Name)
		}
		seenNames[ruleCfg.Name] = true
		rule, err := compileRule(ruleCfg, factory, clockSkewOrDefault(clockSkew))
		if err != nil {
			return nil, fmt.Errorf("oauth2_authorization_server.impersonation.rules[%d] (%s): %w", i, ruleCfg.Name, err)
		}
		rules = append(rules, rule)
	}
	return &Service{audiencePrefix: cfg.AudiencePrefix, rules: rules, agents: agents, canonicalAgents: canonicalAgents, issuer: issuer, delegationVerifier: delegationVerifier, consentBaseURL: consentBaseURL, logger: logger, ledger: recorder}, nil
}

// AuditRecord is the credential-free decision record emitted for every impersonation attempt
// (FR-012). It never contains credential values, key material, or wholesale claims (SC-005).
type AuditRecord struct {
	Outcome                  string   // "success" or the OAuth2 error code
	TargetAgentID            string   // resolved target agent identity, when available
	SelectedRule             string   // rule name (matched rule, or furthest-evaluated on access_denied)
	IssuerRoles              []string // validated credential roles
	PrivilegedClientIdentity string   // extracted client identity, when available
	ActorIdentity            string   // extracted actor identity, when available
	SubjectIdentity          string   // extracted subject identity, when available
	OAuthErrorCode           string   // OAuth2 error code on failure
}

// Outcome carries the result of an impersonation attempt: a success response (nil on failure)
// and the audit record (always populated).
type Outcome struct {
	Response *tokenexchange.TokenExchangeResponse
	Audit    AuditRecord
}

func (s *Service) Impersonate(ctx context.Context, req *Request, target *Target) (outcome *Outcome, err error) {
	var targetID id.AgentID
	var selectedResult *ruleResult
	defer func() {
		if recordErr := s.completeImpersonation(ctx, targetID, selectedResult, err); recordErr != nil {
			failure := originError(ctx, serverError("failed to record impersonation outcome"), recordErr, tokenexchange.StageExchangeRouting, tokenexchange.DetailInternalUnclassified)
			err = failure
			outcome = &Outcome{Audit: failureAudit(targetID.String(), selectedResult, failure)}
		}
	}()
	if target == nil || target.Agent == nil || target.Agent.ID.IsZero() {
		err := serverError("impersonation target agent is required").
			WithDiagnostic(impersonationDiagnostic(tokenexchange.StageIdentityResolution, tokenexchange.DetailAgentInvalid))
		return &Outcome{Audit: failureAudit("", nil, err)}, err
	}
	targetID = target.Agent.ID
	request := requestContext{Scope: req.Scope, GrantType: GrantType, AgentID: targetID.String()}
	for _, scope := range req.Scopes {
		if !oauth2.IsScopeAllowed(target.Agent.AllowedScopes, scope) {
			err := invalidScope("requested scope is not permitted")
			return &Outcome{Audit: failureAudit(targetID.String(), nil, err)}, err
		}
	}

	results := make([]*ruleResult, 0, len(s.rules))
	validationCache := verifiedCredentialCache{}
	for _, rule := range s.rules {
		res := s.evaluateRule(ctx, rule, req, request, target.Agent, &validationCache)
		if res.matched {
			selectedResult = res
			return s.mintAuthorized(ctx, targetID, res)
		}
		if res.abort != nil {
			selectedResult = res
			return &Outcome{Audit: failureAudit(targetID.String(), res, res.abort)}, res.abort
		}
		results = append(results, res)
	}

	failures := make([]*tokenexchange.TokenExchangeError, 0, len(results))
	for _, res := range results {
		failures = append(failures, res.failure)
	}
	selected := selectNoMatchError(failures)
	selectedResult = resultForCode(results, selected.Code())
	return &Outcome{Audit: failureAudit(targetID.String(), selectedResult, selected)}, selected
}

func (s *Service) mintAuthorized(ctx context.Context, targetID id.AgentID, res *ruleResult) (*Outcome, error) {
	decisionData := map[string]any{"delegating_actor_id": res.actor}
	if err := s.recordFact(ctx, "impersonation-granted", impersonationFacts(targetID, res), decisionData); err != nil {
		failure := originError(ctx, serverError("failed to record impersonation permission"), err, tokenexchange.StageExchangeRouting, tokenexchange.DetailInternalUnclassified)
		return &Outcome{Audit: failureAudit(targetID.String(), res, failure)}, failure
	}
	token, err := s.issuer.IssueImpersonationToken(ctx, res.mintInput)
	if err != nil {
		failure := originError(ctx, serverError("failed to mint impersonation token"), err, tokenexchange.StageExchangeRouting, tokenexchange.DetailInternalUnclassified)
		return &Outcome{Audit: failureAudit(targetID.String(), res, failure)}, failure
	}
	response := tokenexchange.NewTokenExchangeResponse(token, BearerTokenType, AccessTokenType)
	response.Scope = strings.Join(res.mintInput.Scopes, " ")
	return &Outcome{Response: response, Audit: successAudit(targetID.String(), res)}, nil
}

// ruleResult captures the outcome of evaluating one rule, including audit-safe context.
type ruleResult struct {
	ruleName  string
	matched   bool
	mintInput ports.ImpersonationMintInput
	failure   *tokenexchange.TokenExchangeError // set when the rule did not match (fall-through)
	abort     *tokenexchange.TokenExchangeError // set on terminal fail-closed error

	client             string
	actor              string
	subject            string
	issuerRoles        []string
	subjectEstablished bool
	delegated          bool
	delegationMissing  bool
}

func (s *Service) evaluateRule(ctx context.Context, rule *compiledRule, req *Request, request requestContext, targetAgent *storage.Agent, validationCache *verifiedCredentialCache) *ruleResult {
	res := &ruleResult{ruleName: rule.name}

	clientClaims, _, err := s.validateCredential(ctx, rule, ports.CredentialRoleClientAssertion, req.ClientAssertion, validationCache)
	if err != nil {
		res.failure = credentialError(ctx, invalidClient("client assertion validation failed"), err, tokenexchange.StageClientValidation)
		return res
	}
	res.issuerRoles = append(res.issuerRoles, string(ports.CredentialRoleClientAssertion))

	actorClaims, actorIssuer, err := s.validateCredential(ctx, rule, ports.CredentialRoleActor, req.ActorToken, validationCache)
	if err != nil {
		res.failure = credentialError(ctx, invalidRequest("actor token validation failed"), err, tokenexchange.StageSubjectValidation)
		return res
	}
	res.issuerRoles = append(res.issuerRoles, string(ports.CredentialRoleActor))

	subjectRole := rule.roles[ports.CredentialRoleSubject]
	subjectUnverified := subjectRole.unverified
	// Signed and unverified subjects are both carried under the RFC 8693 JWT token type; the rule's
	// verification mode plus the token's own alg (checked below) select the path (FR-003a/003d).
	if req.SubjectTokenType != JWTTokenType {
		res.failure = invalidRequest("subject_token_type must be the RFC 8693 JWT token type")
		return res
	}
	var subjectClaims map[string]interface{}
	if subjectUnverified {
		// FR-003d: unsigned unverified subject; parseUnverifiedSubject requires alg:none, so a
		// signed JWS fails here and the rule falls through.
		claims, err := parseUnverifiedSubject(req.SubjectToken)
		if err != nil {
			res.failure = credentialError(ctx, invalidRequest("unverified subject token is invalid"), err, tokenexchange.StageSubjectValidation)
			return res
		}
		subjectClaims = claims
	} else {
		claims, _, err := s.validateCredential(ctx, rule, ports.CredentialRoleSubject, req.SubjectToken, validationCache)
		if err != nil {
			res.failure = credentialError(ctx, invalidRequest("subject token validation failed"), err, tokenexchange.StageSubjectValidation)
			return res
		}
		res.issuerRoles = append(res.issuerRoles, string(ports.CredentialRoleSubject))
		subjectClaims = claims
	}

	clientID, err := rule.roles[ports.CredentialRoleClientAssertion].principal.extract(ctx, clientClaims)
	if err != nil || clientID == "" {
		res.failure = originError(ctx, invalidRequest("could not extract privileged client identity"), err, tokenexchange.StageIdentityResolution, tokenexchange.DetailCELEvaluationFailed)
		return res
	}
	res.client = clientID

	actorID, err := rule.roles[ports.CredentialRoleActor].principal.extract(ctx, actorClaims)
	if err != nil || actorID == "" {
		res.failure = originError(ctx, invalidRequest("could not extract actor identity"), err, tokenexchange.StageIdentityResolution, tokenexchange.DetailCELEvaluationFailed)
		return res
	}
	res.actor = actorID

	subjectID, err := subjectRole.principal.extract(ctx, subjectClaims)
	if err != nil || subjectID == "" {
		res.failure = originError(ctx, invalidRequest("could not extract subject identity"), err, tokenexchange.StageIdentityResolution, tokenexchange.DetailCELEvaluationFailed)
		return res
	}
	res.subject = subjectID
	res.subjectEstablished = !subjectUnverified

	var email *string
	if extractor := subjectRole.email; extractor != nil {
		// FR-006a: a caller-controlled unverified-subject email is mintable only when the rule's
		// authorization predicate binds subject_token.email; a signed-subject email (from a verified
		// issuer) requires no such binding.
		if !subjectUnverified || rule.authz.Binds(celSubjectToken, "email") {
			value, err := extractor.extract(ctx, subjectClaims)
			if err != nil {
				res.failure = originError(ctx, invalidRequest("could not extract subject email"), err, tokenexchange.StageIdentityResolution, tokenexchange.DetailCELEvaluationFailed)
				return res
			}
			if value != "" {
				email = &value
			}
		}
	}

	authorized, err := rule.authz.evaluate(ctx, clientClaims, actorClaims, subjectClaims, subjectUnverified, request)
	if err != nil {
		res.abort = originError(ctx, serverError("authorization evaluation failed"), err, tokenexchange.StageClientAuthorization, tokenexchange.DetailCELEvaluationFailed)
		return res
	}
	if !authorized {
		res.failure = accessDenied("impersonation not authorized by policy")
		return res
	}

	delegationStatus, err := s.delegationVerifier.VerifyUserDelegation(ctx, id.Principal(subjectID), targetAgent.ID)
	if err != nil {
		res.abort = originError(ctx, serverError("user delegation verification failed"), err, tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantRepositoryUnavailable)
		return res
	}
	switch delegationStatus {
	case ports.UserDelegationActive:
		res.subjectEstablished = true
		res.delegated = true
	case ports.UserDelegationMissing:
		res.delegationMissing = true
		res.abort = consentRequired(strings.TrimRight(s.consentBaseURL, "/") + "/agents/" + targetAgent.ID.String()).
			WithDiagnostic(impersonationDiagnostic(tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantMissing))
		return res
	case ports.UserDelegationExpired:
		res.delegationMissing = true
		res.abort = consentRequired(strings.TrimRight(s.consentBaseURL, "/") + "/agents/" + targetAgent.ID.String()).
			WithDiagnostic(impersonationDiagnostic(tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantExpired))
		return res
	default:
		res.abort = originError(ctx, serverError("user delegation verification returned an unknown status"), nil, tokenexchange.StageGrantAuthorization, tokenexchange.DetailInternalUnclassified)
		return res
	}

	res.mintInput = ports.ImpersonationMintInput{
		Subject:     subjectID,
		Email:       email,
		Actor:       actorID,
		ActorIssuer: actorIssuer.issuerURI,
		TargetAgent: targetAgent,
		Scopes:      req.Scopes,
	}
	res.matched = true
	return res
}

func credentialError(ctx context.Context, envelope *tokenexchange.TokenExchangeError, cause error, stage tokenexchange.FailureStage) *tokenexchange.TokenExchangeError {
	detail := tokenexchange.DetailSubjectInvalid
	if stage == tokenexchange.StageClientValidation {
		detail = tokenexchange.DetailClientInvalid
	}
	if errors.Is(cause, jwt.TokenExpiredError{}) {
		detail = tokenexchange.DetailSubjectExpired
		if stage == tokenexchange.StageClientValidation {
			detail = tokenexchange.DetailClientExpired
		}
	}
	var dependencyErr *tokenexchange.TokenExchangeError
	if errors.As(cause, &dependencyErr) && dependencyErr.Diagnostic().Detail() == tokenexchange.DetailJWKSUnavailable {
		detail = tokenexchange.DetailJWKSUnavailable
	}
	return originError(ctx, envelope, cause, stage, detail)
}

// validateCredential selects the trusted issuer matching the credential's iss and role, then
// validates signature, algorithm, issuer, audience, expiry, and not-before (FR-004).
func (s *Service) validateCredential(
	ctx context.Context,
	rule *compiledRule,
	role ports.CredentialRole,
	token string,
	validationCache *verifiedCredentialCache,
) (map[string]interface{}, *compiledIssuer, error) {
	iss, err := unverifiedIssuer(token)
	if err != nil {
		return nil, nil, err
	}
	issuer := rule.issuerFor(role, iss)
	if issuer == nil {
		return nil, nil, fmt.Errorf("no trusted issuer authorized to sign role %q for iss", role)
	}
	if err := issuer.validator.validateAlgorithm(token); err != nil {
		return nil, nil, err
	}
	verified, err := validationCache.verify(ctx, role, token, issuer)
	if err != nil {
		return nil, nil, err
	}

	credentialRole := rule.roles[role]
	claims, err := issuer.validator.validateClaims(verified, credentialRole.expectedAudience, credentialRole.requireAbsentAudience)
	if err != nil {
		return nil, nil, err
	}
	return claims, issuer, nil
}

// resultForCode returns the first rule result whose failure carries the given OAuth2 code, or
// the last result as a fallback for audit context.
func resultForCode(results []*ruleResult, code string) *ruleResult {
	for _, res := range results {
		if res.failure != nil && res.failure.Code() == code {
			return res
		}
	}
	if len(results) > 0 {
		return results[len(results)-1]
	}
	return &ruleResult{}
}

func successAudit(targetAgentID string, res *ruleResult) AuditRecord {
	return AuditRecord{
		Outcome:                  "success",
		TargetAgentID:            targetAgentID,
		SelectedRule:             res.ruleName,
		IssuerRoles:              res.issuerRoles,
		PrivilegedClientIdentity: res.client,
		ActorIdentity:            res.actor,
		SubjectIdentity:          res.subject,
	}
}

func failureAudit(targetAgentID string, res *ruleResult, err *tokenexchange.TokenExchangeError) AuditRecord {
	record := AuditRecord{
		Outcome:        err.Code(),
		TargetAgentID:  targetAgentID,
		OAuthErrorCode: err.Code(),
	}
	if res != nil {
		record.SelectedRule = res.ruleName
		record.IssuerRoles = res.issuerRoles
		record.PrivilegedClientIdentity = res.client
		record.ActorIdentity = res.actor
		record.SubjectIdentity = res.subject
	}
	return record
}

func impersonationFacts(targetID id.AgentID, res *ruleResult) model.BusinessEvent {
	facts := model.BusinessEvent{AgentID: targetID, Actor: model.BusinessEventActor{Kind: "gateway"}}
	if res == nil {
		return facts
	}
	if res.client != "" {
		facts.Actor.ID = &res.client
		facts.GatewayClientID = id.ClientID(res.client)
	}
	if res.subjectEstablished {
		subject := id.Principal(res.subject)
		facts.Subject = &subject
	}
	if res.delegated {
		facts.Actor.OnBehalfOf = facts.Subject
	}
	return facts
}

func (s *Service) recordFact(ctx context.Context, eventType string, facts model.BusinessEvent, data map[string]any) error {
	facts.OccurredAt, facts.Data = time.Now().UTC(), data
	event, err := s.ledger.NewEvent(ctx, model.BusinessEventTypePrefix+eventType, facts)
	if err != nil {
		return err
	}
	return s.ledger.Record(ctx, event)
}

func (s *Service) completeImpersonation(ctx context.Context, targetID id.AgentID, res *ruleResult, requestErr error) error {
	facts := impersonationFacts(targetID, res)
	if requestErr == nil {
		return s.recordFact(ctx, "token-exchanged", facts, map[string]any{})
	}
	var failure *tokenexchange.TokenExchangeError
	if !errors.As(requestErr, &failure) {
		return s.recordFact(ctx, "token-request-failed", facts, map[string]any{"reason_code": "internal_failure"})
	}
	if res == nil && failure.Code() == tokenexchange.InvalidScopeError {
		return nil
	}
	reason := "authentication_failed"
	switch failure.Diagnostic().Outcome() {
	case tokenexchange.OutcomeAuthenticationFailed:
	case tokenexchange.OutcomeAuthorizationDenied:
		reason = "authorization_failed"
	case tokenexchange.OutcomeInvalidRequest:
		return s.recordFact(ctx, "token-request-failed", facts, map[string]any{"reason_code": "invalid_request"})
	default:
		return s.recordFact(ctx, "token-request-failed", facts, map[string]any{"reason_code": "internal_failure"})
	}
	decisionReason := reason
	if res != nil && res.delegationMissing {
		decisionReason = "delegation_missing"
	}
	decisionData := map[string]any{"reason_code": decisionReason}
	if res != nil && res.actor != "" {
		decisionData["delegating_actor_id"] = res.actor
	}
	hints := ports.StorageTransactionHintsFromContext(ctx)
	if facts.Subject != nil {
		hints.Subjects = append(hints.Subjects, ports.StorageSubjectGate{Principal: *facts.Subject})
	}
	return s.ledger.WithTransaction(ctx, hints, func(txCtx context.Context) error {
		if err := s.recordFact(txCtx, "impersonation-denied", facts, decisionData); err != nil {
			return err
		}
		return s.recordFact(txCtx, "token-exchange-denied", facts, map[string]any{"reason_code": reason})
	})
}
