# Research: Agent-Subject Impersonation

**Feature**: `specs/054-agent-subject-impersonation` | **Date**: 2026-10-27 | **Plan**: [plan.md](plan.md)

Each decision follows the confirmed answers in [spec.md](spec.md) (Q1–Q5) and the current code. Decisions marked **Needs confirmation** are proposals for the API and ADR review. Implementation must not start until they are confirmed (Principles II, IV, X).

## R1 — Governance: narrow superseding ADR for consent-free agent issuance

**Decision**: Draft [ADR 044](adr-044-draft.md) with **Status: Proposed**. It supersedes decisions 1 and 2 of [ADR 032](../../adrs/032-impersonation-requires-user-delegation.md) for agent-category requests only. Classification depends solely on verified signed raw `sub`/target-UUID equality, independently of rule eligibility. For eligible, CEL-authorized agents, delegation is skipped and the minted subject is the target UUID rather than necessarily the extracted subject principal. All principal extractions must still succeed; no new extraction-equality restriction is introduced. ADR 032 decisions 1–6 stay in force for eligible human-category requests; signed equality on human-only rules is intentionally ineligible even with active human delegation. ADR 031 and ADR 034 are unchanged.

The draft lives in the feature directory because the planning phase may only write feature artifacts. After acceptance, the Phase 2 governance task moves it to `adrs/044-agent-subject-impersonation.md` (or the next free number) and adds it to the AGENTS.md ADR index.

**Rationale**: ADR 032 is Accepted and merged. It requires delegation for every subject and has no opt-out. Under Principle II, deviating from it needs an Accepted superseding ADR. This phase drafts that ADR but does not accept it. A stakeholder must accept the precise agent-only supersession of both decisions 1 and 2 before any task that changes the delegation or minted-subject path.

**Alternatives considered**:
- *Amend ADR 032 in place*: forbidden for a merged Accepted ADR.
- *Satisfy ADR 032 with a synthetic grant for the agent*: violates FR-005 (no synthetic human grants or sessions).

**Needs confirmation**: Stakeholder acceptance of ADR 044's explicit agent-only supersession of ADR 032 decisions 1 and 2.

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

**Decision (Confirmed — maintainer review on PR #293, inline comment on this decision: "confirmed")**: A category-ineligible request uses the existing no-match envelope: `403 access_denied` with the same generic description as a policy denial. No new `error` code and no `error_uri` are added. The structured audit event and the diagnostic detail `subject_category_ineligible` tell the two cases apart. The human consent-required response (`403 access_denied` plus `error_uri`) is unchanged. It is never returned for an agent-category request.

**Rationale**: API-003 asks for a confirmed mapping and keeps the human responses. Reusing the established envelope does not reveal rule configuration to callers, and existing clients need no change.

**Alternatives considered**: A distinct `error_description` per cause. Rejected because descriptions are not stable identifiers (existing OpenAPI text) and would reveal rule contents.

## R5 — Agent-subject token detection: no category claim; configurable CEL detection on token exchange

**Decision (revised by the maintainer review on PR #293, inline comment on the former reserved-claim proposal)**: The broker adds **no** category claim to issued tokens. Following the spec ("no separate category claim or request parameter is required"), an agent-subject token is the ordinary local impersonation token whose reserved base claims `sub` and `agent_id` both equal the target UUID (FR-004). Human impersonation tokens and authorization-code tokens are unchanged.

Detection happens on the third-party token exchange through a new, optional CEL expression:

- **Key**: `token_exchange.claim_extraction.agent_subject_expression` (`ports.ClaimExtractionConfig.AgentSubjectExpression`, `mapstructure:"agent_subject_expression"`). It uses the existing configuration system and precedence and sits beside `principal_expression` and `agent_id_expression`, which already describe how `subject_token` claims are interpreted.
- **Input**: the validated `subject_token` claims, as the CEL variable `subject_token` (the same environment as the existing claim-extraction expressions). No new variable or function.
- **Output**: a boolean. `true` means the subject token represents an agent acting for itself, and the exchange is rejected (R6). `false` continues the existing flow.
- **Default** (`tokenexchange.DefaultAgentSubjectExpression`, applied by `NewCELEvaluator` when the key is empty or omitted), the guarded form of `sub == agent_id`:

  ```cel
  has(subject_token.sub) && has(subject_token.agent_id) && subject_token.sub == subject_token.agent_id
  ```

  The `has()` guards keep tokens without an `agent_id` claim, such as upstream IdP tokens that use `resolveAgentIdByClientId(subject_token.azp)`, on the existing path instead of raising a missing-key evaluation error. CEL heterogeneous equality returns `false` for values of different types.
- **Startup**: the expression is compiled with the other token-exchange expressions in `NewCELEvaluator` (FR-017 precedent). A syntax error, an undeclared variable, or a static output type other than `bool` or `dyn` fails startup.
- **Runtime**: evaluation uses the existing token-exchange evaluation timeout. It follows `ExtractPrincipal`'s error semantics: an evaluation error or timeout is a terminal `server_error` through `celEvaluationError` (`identity_resolution / cel_evaluation_failed`), and a non-boolean result is a terminal `server_error` with `identity_resolution / cel_configuration`. The broker never treats a failed detection as "not an agent" (Principle I).
- **Helm and examples**: `tokenExchange.claimExtraction.agentSubjectExpression` is an optional string in `values.schema.json` (the `claimExtraction` object is strict). `templates/configmap.yaml` renders `agent_subject_expression` only when the value is set, so the broker default applies otherwise. `values.yaml`, the chart `README.md`, and `examples/config/token-exchange.yaml` document the default and the override requirement.
- **Override semantics (Needs confirmation, C4)**: a configured expression **replaces** the default, as the review describes ("can default to"). The broker cannot statically prove that an arbitrary expression still matches its own agent-subject tokens. Documentation therefore requires every override to keep the `sub == agent_id` comparison as a disjunct, so that FR-011 remains guaranteed. The alternative is to always OR the configured expression with the built-in default, so that operators could only widen detection. The plan proposes replacement and asks for confirmation.

`ports.ImpersonationMintInput` is unchanged. The impersonation service passes `Subject = target UUID` and `Email = nil` for an agent-category request (R7). `baseClaims` is unchanged: `sub` and `agent_id` are already reserved, so `token_claims_expression` cannot break the `sub == agent_id` equality on an agent-subject token or create it on a human token.

**Rationale**: The spec requires no separate category claim, and the reviewer asked for a configurable CEL detector that defaults to `sub == agent_id`. With the default, every broker agent-subject token is detected by construction, because FR-004 sets both reserved claims to the target UUID and CEL cannot override them. The comparison is between two claims, not an inference from the subject text alone: a human token's `sub` is the human principal and its `agent_id` is the target agent. Making the detector configurable lets operators cover trusted external issuers that represent agent subjects differently, without a broker-specific claim.

**Collision analysis (fail closed)**:
- A broker human token can satisfy the default only when the minted human principal is the exact target UUID text. Under FR-003, a signed raw `sub` with that text is always agent-category, so this can only happen through an unsigned (ADR 031) subject or a `principal_expression` that maps another claim to the exact UUID. Such a token is rejected for third-party exchange. This is a fail-closed restriction, not a consent bypass, and is documented.
- A human token minted before this feature with `sub == agent_id` is also rejected after upgrade. Such collisions are not expected in practice.
- A trusted external issuer whose subject tokens carry equal `sub` and `agent_id` claims is rejected as well (R6).

**Alternatives considered**:
- *Reserved `subject_category: "agent"` claim* (the previous proposal): rejected by the review. The spec says no separate category claim is required, and a new claim would add a reserved name to every local token's policy surface.
- *Hard-coded `sub == agent_id` without configuration*: rejected by the review. Operators need to adapt detection for external issuers.
- *Emit `subject_category: human` on every token*: changes every existing token.
- *An RFC 9493 `sub_id` object*: too complex for a single flag, and no consumer uses it.

**Consequence**: Deployments that override `agent_subject_expression` own its correctness. `docs/configuration.md` and `docs/reference/token-exchange.md` document the default, the fail-closed collision cases, and the override requirement.

## R6 — Third-party exchange rejection (FR-011)

**Decision (Confirmed — maintainer review on PR #293, inline comment on this decision: "confirmed"; detection input revised by R5)**: In `TokenExchangeService.Exchange`, add a step after principal extraction and `FinalizeCaptureHolder` (so ADR 033's actor and calling-peer audit still applies). The step runs before agent-ID extraction, CEL authorization, and the grant and session lookups. It calls `CELEvaluator.IsAgentSubject(subjectTokenClaims)`, which evaluates `agent_subject_expression` (R5). If the result is `true`, return `403 access_denied` with description `agent-subject tokens cannot be exchanged for third-party service tokens`. The response has no `error_uri` and the diagnostic is `identity_resolution / subject_category_unsupported`. The check does not depend on the issuer. A trusted external issuer whose subject token matches the expression is also rejected; this fails closed and must be documented.

**Rationale**: The rejection must not depend on missing human state (FR-011). Because it happens before `VerifyAgentAccess` and `GetValidAccessToken`, no grant or session belonging to a human with the same text is read or used (SR-005). Consent cannot recover the request, so no consent URL is returned.

**Alternatives considered**: Rejecting only tokens from the broker's own issuer. This adds an issuer comparison but no security benefit; an operator who needs it can express it in `agent_subject_expression`.

## R7 — Agent token content: subject, email, and audit identity

**Decision**:
- `sub` and `agent_id` are both the target UUID (FR-004). The minted subject is `target.Agent.ID.String()`. This equals the raw verified `sub` by construction, but need not equal the successfully extracted subject principal. That agent-only departure from ADR 032 decision 2 requires the Proposed ADR 044's acceptance; humans retain the extracted-principal invariant.
- `act` is unchanged: the extracted actor and the verified actor issuer. The broker keeps it even when the actor and subject strings are equal (FR-007).
- `email_expression` is **not evaluated** for an agent-category request, and no `email` reaches local token CEL. An agent token is not a token for a human (Journey 1 shows no email). The impersonation service passes `Email = nil`; the mint input and strategy are unchanged.
- No category claim is minted (R5). The token is identifiable as an agent-subject token because `sub == agent_id`.
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
- **US3.4**: Boots one local-mode broker with both impersonation and `token_exchange` configured, as in `tests/e2e/local_token_exchange_e2e_test.go`. The subject-token trust is the broker's local issuer. The test exchanges the minted agent token, first without and then with a human `UserGrant` and a stored session for the identical UUID text. It leaves `agent_subject_expression` unset, so it proves that the default detects broker agent-subject tokens.
- **US2.5**: Covers direct-construction failures through `BuildApp`. It covers YAML-shape failures through the production loader: `bootstrap.LoadCredentialConfiguration` with a temporary YAML file.
- **Unit**: Table-driven tests for `classifySubject`, category eligibility, rule invariants, YAML shape validation, `validator.go`, the mint strategy (agent input yields `sub == agent_id` with no category claim; CEL cannot override either), `CELEvaluator.IsAgentSubject` (default and override expressions, missing claims, type mismatch, non-boolean result, evaluation error, startup compile failure), the token-exchange rejection, and handler audit fields.
- **Regression**: The existing `tests/e2e/impersonation_test.go` and `tests/e2e/local_token_exchange_e2e_test.go` must pass unchanged. This proves the human behavior is preserved (US2.4).

**Rationale**: Principle XIII, with existing harnesses only.

## R11 — Documentation and architecture

**Decision**: Implementation updates these files:
- `ARCHITECTURE.md`: §3.1.y, plus glossary terms *Subject Category*, *Agent-Subject Token*, and *Allowed Subject Categories*.
- The AGENTS.md glossary row and ADR index entry, after ADR 044 is accepted.
- `api/enduser/openapi.yaml`, from [contracts/openapi-token-agent-subject.yaml](contracts/openapi-token-agent-subject.yaml).
- `docs/configuration.md` and `docs/reference/token-exchange.md` (new diagnostic details, `token_exchange.claim_extraction.agent_subject_expression` with its default, failure semantics, fail-closed collision cases, and override requirement).
- `docs/concepts/token-exchange.md` (third-party limitation).
- `docs/api/enduser-api.md`: extend its existing `POST /oauth2/token` documentation with agent-subject requests, decoded token claims, the eligibility matrix and human-only signed collision rejection, and category/third-party denial examples. Keep these aligned with the confirmed OpenAPI contract and link the concept/reference/configuration guides.
- `examples/config/impersonation.yaml`, plus `examples/config/impersonation-agent-subject.yaml`, referenced from `examples/config/README.md`.
- The Helm chart files listed in R8 and R5, and `examples/config/token-exchange.yaml` (R5).

The existing `docs/api/enduser-api.md` token-endpoint page is extended rather than exempted: rendered end-user API examples are required by Constitution IV.

## Resolved unknowns

The spec contains no `NEEDS CLARIFICATION` markers. The technical unknowns (classification input, order, agent-token detection, YAML presence detection, error mapping, and E2E harness) are resolved above.

The PR #293 review confirmed R4 (C2) and R6 (C3), and replaced the reserved-claim proposal with the configurable detection expression in R5. Three items still need stakeholder confirmation before implementation: R1 (ADR 044 acceptance), the R5 override semantics (C4), and the R8 unsigned-subject guard.
