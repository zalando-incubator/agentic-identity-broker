# Internal Contracts: Ports, Token Minting, Exchange Rejection, and Audit

**Feature**: `specs/054-agent-subject-impersonation` | **Research**: [../research.md](../research.md) | **Data model**: [../data-model.md](../data-model.md)

These contracts are internal. They define the seams that tests target. Their visible effects (the token claim and HTTP responses) are proposed in [openapi-token-agent-subject.yaml](openapi-token-agent-subject.yaml) and must be confirmed.

## 1. `ports.ImpersonationRuleConfig.AllowedSubjectCategories`

```go
// AllowedSubjectCategories lists the subject categories this rule accepts. Nil (omitted) means
// human only. Valid entries: SubjectCategoryHuman, SubjectCategoryAgent (CFG-001).
AllowedSubjectCategories []string `mapstructure:"allowed_subject_categories"`
```

## 2. `ports.ImpersonationMintInput.SubjectCategory`

```go
// SubjectCategory is "human" (or empty) for a human subject and "agent" for an agent acting for
// itself. An agent category mints the reserved subject_category claim; other values fail minting.
SubjectCategory string
```

`JWXAccessTokenStrategy.GenerateImpersonationToken` behaves as follows:

| Input | Effect |
| --- | --- |
| `""` or `"human"` | Behavior unchanged; no `subject_category` claim. |
| `"agent"` | Adds the claim `subject_category: "agent"` after the CEL claims. The claim name is in `baseClaims`, so CEL cannot override it. |
| anything else | Returns an error. The impersonation service maps it to the existing terminal `server_error`. |

For `"agent"`, the impersonation service passes `Subject = target UUID` and `Email = nil`.

## 3. `impersonation.Service` behavior

| Step | Human category | Agent category |
| --- | --- | --- |
| Rule does not allow the category | Fall-through `access_denied` (`subject_category_ineligible`) | Same |
| CEL false | Fall-through `access_denied` | Same |
| CEL error | Terminal `server_error` | Same |
| Delegation | `UserDelegationVerifier` called; a missing or expired delegation is a terminal consent-required denial | **Not called** |
| Mint | `Subject = extracted principal` | `Subject = target UUID`, `SubjectCategory = agent` |

Invariant (tested): no code path calls `VerifyUserDelegation` for an agent-category request, and no agent-category failure produces `error_uri`.

## 4. `tokenexchange.TokenExchangeService.Exchange`

The new step runs after `ExtractPrincipal` and `security.FinalizeCaptureHolder`, and before `ExtractAgentID`:

```text
if claims[SubjectCategoryClaim] is the string SubjectCategoryAgent:
    return access_denied("agent-subject tokens cannot be exchanged for third-party service tokens")
           with diagnostic (identity_resolution, subject_category_unsupported), and no error_uri
```

Invariant (tested with a recording fake or storage spy): for an agent-subject token, the exchange calls none of `consent.Service.VerifyAgentAccess`, `OAuth2SessionService.GetValidAccessToken`, or the provider lookup.

## 5. Audit event `impersonation_decision`

The new attribute is `subject_category` (`human` | `agent`). It is emitted on success and on every failure after classification. `subject_identity` is the represented identity. Allowed attribute names: `event`, `outcome`, `target_agent_id`, `rule`, `issuer_roles`, `privileged_client_identity`, `actor_identity`, `subject_identity`, `subject_category`, `oauth_error_code`, `request_id`. No other attributes are emitted, and no token values.

## 6. Diagnostic catalogue

`docs/reference/token-exchange.md` and `internal/domain/tokenexchange/diagnostic.go` gain:

| Detail | Stage | Kind |
| --- | --- | --- |
| `subject_category_ineligible` | `client_authorization` | `impersonation` |
| `subject_category_unsupported` | `identity_resolution` | `third_party` |
