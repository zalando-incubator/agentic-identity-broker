# Internal Contracts: Ports, Agent-Subject Detection, Exchange Rejection, and Audit

**Feature**: `specs/054-agent-subject-impersonation` | **Research**: [../research.md](../research.md) | **Data model**: [../data-model.md](../data-model.md)

These contracts are internal. They define the seams that tests target. Their visible effects (decoded token claims and HTTP responses) are documented in [openapi-token-agent-subject.yaml](openapi-token-agent-subject.yaml) and must be confirmed.

## 1. `ports.ImpersonationRuleConfig.AllowedSubjectCategories`

```go
// AllowedSubjectCategories lists the subject categories this rule accepts. Nil (omitted) means
// human only. Valid entries: SubjectCategoryHuman, SubjectCategoryAgent (CFG-001).
AllowedSubjectCategories []string `mapstructure:"allowed_subject_categories"`
```

## 2. `ports.ClaimExtractionConfig.AgentSubjectExpression` and `CELEvaluator.IsAgentSubject`

```go
// AgentSubjectExpression is an optional CEL boolean over subject_token that detects a subject token
// representing an agent acting for itself. Empty means tokenexchange.DefaultAgentSubjectExpression.
AgentSubjectExpression string `mapstructure:"agent_subject_expression"`
```

```go
// internal/domain/tokenexchange/constants.go
DefaultAgentSubjectExpression = "has(subject_token.sub) && has(subject_token.agent_id) && subject_token.sub == subject_token.agent_id"

// internal/domain/tokenexchange/cel_evaluator.go
func (e *CELEvaluator) IsAgentSubject(subjectTokenClaims map[string]interface{}) (bool, error)
```

`CELEvaluatorConfig` gains `AgentSubjectExpression`. `internal/app/builder.go` passes the configured value into the shared token-exchange `CELEvaluatorConfig` (`builder.go` ~L1269). Approval authentication shares that evaluator but never calls `IsAgentSubject`; only `TokenExchangeService.Exchange` evaluates it.

| Situation | Result |
| --- | --- |
| Expression empty or omitted | The default is compiled. |
| Compile error, or a static output type other than `bool`/`dyn` | `NewCELEvaluator` fails, and startup fails. |
| Result `true` | `(true, nil)` |
| Result `false` | `(false, nil)` |
| Evaluation error or timeout | `server_error` through `celEvaluationError` at `identity_resolution` (`cel_evaluation_failed`) |
| Non-boolean result | `server_error` at `identity_resolution` (`cel_configuration`) |

`ports.ImpersonationMintInput` and `JWXAccessTokenStrategy.GenerateImpersonationToken` are unchanged. For an agent-category request, the impersonation service passes `Subject = target UUID` and `Email = nil`. `sub` and `agent_id` are already reserved base claims, so the minted token always satisfies the default expression.

## 3. `impersonation.Service` behavior

| Step | Human category | Agent category |
| --- | --- | --- |
| Rule does not allow the category | Fall-through `access_denied` (`subject_category_ineligible`) | Same |
| CEL false | Fall-through `access_denied` | Same |
| CEL error | Terminal `server_error` | Same |
| Delegation | `UserDelegationVerifier` called; a missing or expired delegation is a terminal consent-required denial | **Not called** |
| Mint | `Subject = extracted principal` | `Subject = target UUID`, `Email = nil` |

Invariant (tested): no code path calls `VerifyUserDelegation` for an agent-category request, and no agent-category failure produces `error_uri`.

## 4. `tokenexchange.TokenExchangeService.Exchange`

The new step runs after `ExtractPrincipal` and `security.FinalizeCaptureHolder`, and before `ExtractAgentID`:

```text
isAgent, err := celEvaluator.IsAgentSubject(subjectTokenClaims)
if err != nil:
    return err   # terminal server_error; never treated as "not an agent"
if isAgent:
    return access_denied("agent-subject tokens cannot be exchanged for third-party service tokens")
           with diagnostic (identity_resolution, subject_category_unsupported), and no error_uri
```

Invariant (tested with a recording fake or storage spy): for a subject token that matches the expression, and for an evaluation failure, the exchange calls none of `consent.Service.VerifyAgentAccess`, `OAuth2SessionService.GetValidAccessToken`, or the provider lookup.

## 5. Audit event `impersonation_decision`

The new attribute is `subject_category` (`human` | `agent`). It is emitted on success and on every failure after classification. `subject_identity` is the represented identity. Allowed attribute names: `event`, `outcome`, `target_agent_id`, `rule`, `issuer_roles`, `privileged_client_identity`, `actor_identity`, `subject_identity`, `subject_category`, `oauth_error_code`, `request_id`. No other attributes are emitted, and no token values.

## 6. Diagnostic catalogue

`docs/reference/token-exchange.md` and `internal/domain/tokenexchange/diagnostic.go` gain:

| Detail | Stage | Kind |
| --- | --- | --- |
| `subject_category_ineligible` | `client_authorization` | `impersonation` |
| `subject_category_unsupported` | `identity_resolution` | `third_party` |
