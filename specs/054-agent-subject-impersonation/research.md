# Research: Agent-Subject Impersonation

**Feature**: `specs/054-agent-subject-impersonation` | **Date**: 2026-10-27 | **Plan**: [plan.md](plan.md)

Each decision follows the confirmed answers in [spec.md](spec.md) (Q1–Q5) and the current code. Decisions marked **Needs confirmation** are proposals for the API and ADR review. Implementation must not start until they are confirmed (Principles II, IV, X).

## R1 — Governance: narrow superseding ADR for consent-free agent issuance

**Decision**: Draft [ADR 044](adr-044-draft.md) with **Status: Proposed**. It supersedes only decision 1 of [ADR 032](../../adrs/032-impersonation-requires-user-delegation.md). The delegation check is no longer required for an **agent-category** request: a request whose verified signed `sub` exactly equals the target UUID, on a rule that explicitly allows `agent`. Decisions 2–6 of ADR 032 stay in force for every human-category request. ADR 031 and ADR 034 are unchanged.

The draft lives in the feature directory because the planning phase may only write feature artifacts. After acceptance, the Phase 2 governance task moves it to `adrs/044-agent-subject-impersonation.md` (or the next free number) and adds it to the AGENTS.md ADR index.

**Rationale**: ADR 032 is Accepted and merged. It requires delegation for every subject and has no opt-out. Under Principle II, deviating from it needs an Accepted superseding ADR. This phase drafts that ADR but does not accept it. A stakeholder must accept it before any task that changes the delegation path.

**Alternatives considered**:
- *Amend ADR 032 in place*: forbidden for a merged Accepted ADR.
- *Satisfy ADR 032 with a synthetic grant for the agent*: violates FR-005 (no synthetic human grants or sessions).

**Needs confirmation**: Stakeholder acceptance of ADR 044.

## R2 — Category classification: exact comparison of the verified `sub` claim

**Decision**: Add `classifySubject(subjectClaims, unverified, targetUUID) SubjectCategory` to `internal/domain/impersonation`. It returns `agent` only when all of these hold:

1. The rule's subject role is signed (`verification: jwks`) and the subject credential passed signature and claim validation.
2. The verified claims contain `sub` as a JSON string.
3. That string is byte-for-byte equal to `target.Agent.ID.String()`, the canonical lower-case UUID of the target resolved from the audience.

Any other request is `human`. The comparison reads the raw verified `sub` claim, not the rule's `principal_expression`. This follows the clarified wording "signed subject claim `sub`" and stops a policy expression from mapping another claim onto the UUID. No parsing, case folding, trimming, or alias resolution is done (FR-004). An unsigned subject is always `human` (SR-003).

**Rationale**: Q1 and Q2 make exact signed equality the only selector, and require no request parameter or category claim. `target.Agent.ID.String()` is already canonical: `ResolveTarget` rejects a non-canonical UUID suffix.

**Alternatives considered**: Comparing the extracted principal. Rejected: an operator expression such as `subject_token.email` could then produce the agent category from a non-`sub` claim.

## R3 — Evaluation order inside one rule

**Decision**: `evaluateRule` becomes:

1. Validate the client assertion, the actor, and the subject (unchanged).
2. Extract the client, actor, and subject principals (unchanged; every extraction must succeed, FR-008).
3. **Classify** the category (R2).
4. **Check category eligibility.** If the rule does not allow the category, record a fall-through `access_denied` failure with the new diagnostic detail `subject_category_ineligible` and go to the next rule. This failure is not terminal (FR-003, FR-009).
5. Evaluate the CEL authorization (unchanged; denial falls through, an evaluation error is a terminal `server_error`).
6. **Human:** verify delegation exactly as today (terminal consent-required denial). **Agent:** skip the delegation verifier entirely (FR-005).
7. Mint the token.

The eligibility check runs before CEL and before delegation (SR-001). A human request on an agent-only rule falls through, and so does an agent request on a human-only rule. A human delegation failure stays terminal, so the request cannot be retried as an agent (FR-009). An agent request never reaches the delegation verifier, so it cannot be retried as a human (FR-003).

**Rationale**: This keeps the existing first-match and no-match precedence (`access_denied` > `invalid_request` > `invalid_client`). It adds no new terminal error.

**Alternatives considered**: Checking the category after CEL. The outcome is the same, but the policy runs for requests that can never be eligible, and the audit would attribute the denial to the policy.

## R4 — Error mapping on `POST /oauth2/token` (impersonation)

**Decision (Needs confirmation)**: A category-ineligible request uses the existing no-match envelope: `403 access_denied` with the same generic description as a policy denial. No new `error` code and no `error_uri` are added. The structured audit event and the diagnostic detail `subject_category_ineligible` tell the two cases apart. The human consent-required response (`403 access_denied` plus `error_uri`) is unchanged. It is never returned for an agent-category request.

**Rationale**: API-003 asks for a confirmed mapping and keeps the human responses. Reusing the established envelope does not reveal rule configuration to callers, and existing clients need no change.

**Alternatives considered**: A distinct `error_description` per cause. Rejected because descriptions are not stable identifiers (existing OpenAPI text) and would reveal rule contents.

## R5 — Agent-subject token representation: reserved `subject_category` claim

**Decision (Needs confirmation)**: An agent-category impersonation token contains the private claim `"subject_category": "agent"`. Human impersonation tokens and authorization-code tokens are unchanged and do not contain the claim. The claim joins `baseClaims` in `internal/domain/oauth2server/strategies.go`, so `token_claims_expression` cannot add, change, or remove it. If the CEL expression returns the claim, the broker logs a warning and skips it, as it does for other reserved claims.

The constants `tokenexchange.SubjectCategoryClaim = "subject_category"` and `tokenexchange.SubjectCategoryAgent = "agent"` live in `internal/domain/tokenexchange`. Both the minting strategy and the exchange service import that package. `ports.ImpersonationMintInput` gains `SubjectCategory`. When the category is `agent`, the strategy also sets `Email = nil` (R7).

**Rationale**: The spec forbids deciding the category from the subject text alone. An explicit, signed, reserved claim keeps the decision independent of whether a human principal has the same text (FR-011, SR-005). Omitting the claim from human tokens keeps the existing token contract unchanged.

**Alternatives considered**:
- *Infer from `sub == agent_id` plus `act`*: explicitly forbidden. It would also misclassify human tokens issued before the feature.
- *Emit `subject_category: human` on every token*: changes every existing token. The feature does not need it.
- *An RFC 9493 `sub_id` object*: too complex for a single flag, and no consumer uses it.

**Consequence**: A deployment whose `token_claims_expression` already emits `subject_category` loses that claim; the broker logs a warning at mint time. This must be documented in `docs/configuration.md`.

## R6 — Third-party exchange rejection (FR-011)

**Decision (Needs confirmation)**: In `TokenExchangeService.Exchange`, add a step after principal extraction and `FinalizeCaptureHolder` (so ADR 033's actor and calling-peer audit still applies). The step runs before agent-ID extraction, CEL authorization, and the grant and session lookups. If the validated subject token has `subject_category` equal to the string `agent`, return `403 access_denied` with description `agent-subject tokens cannot be exchanged for third-party service tokens`. The response has no `error_uri` and the diagnostic is `identity_resolution / subject_category_unsupported`. The check does not depend on the issuer. A trusted external issuer that uses the same claim and value is also rejected; this fails closed and must be documented.

**Rationale**: The rejection must not depend on missing human state (FR-011). Because it happens before `VerifyAgentAccess` and `GetValidAccessToken`, no grant or session belonging to a human with the same text is read or used (SR-005). Consent cannot recover the request, so no consent URL is returned.

**Alternatives considered**: Rejecting only tokens from the broker's own issuer. This adds an issuer comparison but no security benefit, because only the broker sets the reserved claim on its own tokens.

## R7 — Agent token content: subject, email, and audit identity

**Decision**:
- `sub` and `agent_id` are both the target UUID (FR-004). The minted subject is `target.Agent.ID.String()`. This equals the verified `sub` by construction.
- `act` is unchanged: the extracted actor and the verified actor issuer. The broker keeps it even when the actor and subject strings are equal (FR-007).
- `email_expression` is **not evaluated** for an agent-category request, and no `email` reaches local token CEL. An agent token is not a token for a human (Journey 1 shows no email).
- `AuditRecord` gains `SubjectCategory` (`human` | `agent`). `SubjectIdentity` holds the represented identity: the minted `sub` for both categories. The handler logs `subject_category`, plus the existing rule, target, client, and actor fields (SR-004).

**Rationale**: Each choice follows the clarified journeys. None needs a new port.

## R8 — Configuration: `allowed_subject_categories`

**Decision**:
- `ports.ImpersonationRuleConfig` gains `AllowedSubjectCategories []string \`mapstructure:"allowed_subject_categories"\``. Constants `ports.SubjectCategoryHuman = "human"` and `ports.SubjectCategoryAgent = "agent"` are added.
- **Shape validation (YAML source)**: Viper decodes with weakly typed input and string-to-slice hooks. It would turn a scalar `human` into `[human]`, `1` into `"1"`, and `null` into the omitted value. Rule lists are YAML-only (`docs/configuration.md`, "rule lists are YAML-only"). So `internal/config` adds `validateImpersonationRuleShapesYAML(data []byte)`, called from `loadYAML` before `ReadConfig`. It follows the existing `decodeCredentialFilesYAML` precedent. It parses `oauth2_authorization_server.impersonation.rules[*]` with `go.yaml.in/yaml/v3` nodes. When `allowed_subject_categories` is present, the value must be a sequence node whose entries are scalar `!!str` nodes. Startup fails for null, a scalar, a mapping, and non-string or null entries (CFG-001 table).
- **Value validation (all sources, and direct construction)**: `validateImpersonationRule` in `internal/config/validator.go` and `validateRuleInvariants` in the domain both run the same check. The list may be nil (omitted, default `[human]`) or a non-empty list of distinct case-sensitive entries from `{human, agent}`. An empty non-nil list, an unknown value, or a duplicate fails startup.
- **Unsigned-subject guard (Needs confirmation)**: A rule with `roles.subject.verification: none` whose list contains `agent` fails startup. Unsigned subjects are always human (SR-003), so the setting would have no effect, and an operator could wrongly believe agent issuance was enabled. The alternative is to accept and ignore the setting; the plan prefers failing closed.
- `compileRule` stores the categories as a two-flag set on `compiledRule` (`allowsHuman`, `allowsAgent`), with nil meaning human only.
- **Helm**: `values.schema.json` is strict (`additionalProperties: false` on each rule). It gains the property `allowed_subject_categories` as an array of `enum [human, agent]` with `minItems: 1`, `maxItems: 2`, and `uniqueItems: true`. `values.yaml` comments and `README.md` gain the key. The ConfigMap passes the subtree through unchanged.

**Rationale**: CFG-001 requires rejecting null and scalars, while omission defaults to `[human]`. That difference exists only before decoding. The domain check covers direct construction (E2E bootstrap and tests).

## R9 — CEL authorization context

**Decision**: Add no new CEL variable. A rule can already bind the represented agent with `subject_token.sub` and `request.agent_id` (Journey 1). A dual-category rule can tell the categories apart with `subject_token.sub == request.agent_id`.

**Rationale**: FR-008 is met by the existing context, and scope discipline rules out an extra policy surface.

## R10 — Testing approach

**Decision**:
- **E2E**: Add `tests/e2e/agent_subject_impersonation_test.go` with one `It()` per scenario: 12 in total, mapped in [plan.md](plan.md). It reuses `helpers.MockJWKSServer`, `fixtures.LocalConfig()`, and `fixtures.ActiveGrant`. The agent-only, human-only, and dual-category rule configurations come from a new fixture, `tests/e2e/fixtures/impersonation_rules.go`.
- **US3.4**: Boots one local-mode broker with both impersonation and `token_exchange` configured, as in `tests/e2e/local_token_exchange_e2e_test.go`. The subject-token trust is the broker's local issuer. The test exchanges the minted agent token, first without and then with a human `UserGrant` and a stored session for the identical UUID text.
- **US2.5**: Covers direct-construction failures through `BuildApp`. It covers YAML-shape failures through the production loader: `bootstrap.LoadCredentialConfiguration` with a temporary YAML file.
- **Unit**: Table-driven tests for `classifySubject`, category eligibility, rule invariants, YAML shape validation, `validator.go`, the mint strategy (claim present for agents only, reserved from CEL), the token-exchange rejection, and handler audit fields.
- **Regression**: The existing `tests/e2e/impersonation_test.go` and `tests/e2e/local_token_exchange_e2e_test.go` must pass unchanged. This proves the human behavior is preserved (US2.4).

**Rationale**: Principle XIII, with existing harnesses only.

## R11 — Documentation and architecture

**Decision**: Implementation updates these files:
- `ARCHITECTURE.md`: §3.1.y, plus glossary terms *Subject Category*, *Agent-Subject Token*, and *Allowed Subject Categories*.
- The AGENTS.md glossary row and ADR index entry, after ADR 044 is accepted.
- `api/enduser/openapi.yaml`, from [contracts/openapi-token-agent-subject.yaml](contracts/openapi-token-agent-subject.yaml).
- `docs/configuration.md` and `docs/reference/token-exchange.md` (new diagnostic details).
- `docs/concepts/token-exchange.md` (third-party limitation).
- `examples/config/impersonation.yaml`, plus `examples/config/impersonation-agent-subject.yaml`, referenced from `examples/config/README.md`.
- The Helm chart files listed in R8.

No `docs/api/` page covers the token endpoint today, so none is added (scope discipline).

## Resolved unknowns

The spec contains no `NEEDS CLARIFICATION` markers. The technical unknowns (classification input, order, token marker, YAML presence detection, error mapping, and E2E harness) are resolved above. Four items are proposals that stakeholders must confirm before implementation: R1 (ADR acceptance), R4, R5, and R6 (API and token contract), and the R8 unsigned-subject guard.
