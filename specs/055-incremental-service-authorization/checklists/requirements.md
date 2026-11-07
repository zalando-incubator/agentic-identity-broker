# Specification Quality Checklist: Incremental Service Authorization

**Purpose**: Validate specification completeness and quality before implementation preparation.
**Created**: 2026-10-29
**Feature**: [spec.md](../spec.md)

**Review ownership**: These existing checked items describe specification quality, not implementation completion or maintainer approval. The task phase reconciled stale handoff notes with the clarification and planning artifacts; no implementation validation is claimed.

## Content Quality

- [x] No implementation details beyond the public protocol examples and design-precondition sections required by the repository template
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders, with technical examples separated into journeys and contract requirements
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria at the specification level; implementation verification has not occurred
- [x] Required API, configuration, persistence and architecture design constraints are explicit and distinguished from application implementation

## Resolved Clarification Handoff

These are the recorded specification answers, not maintainer approvals. The earlier unanswered Q1–Q3 handoff is superseded by `spec.md` Clarifications and FR-019–FR-021.

| Question | Resolved answer | Approval item |
|---|---|---|
| Q1: Canonical-ID rename compatibility | Only UUID scopes are durable. No historical aliases. An old canonical refresh fails without consuming its otherwise valid token; a new challenge and interactive authorization restore access. Bindings prevent canonical-ID reuse from rebinding old grants. | DEC-001; DEC-004 for binding persistence |
| Q2: Visibility of virtual scopes in broker tokens | Local token-response scopes and signed access-token scopes include granted virtual scopes in their exact requested form. Refresh retains and rechecks the granted service set even when request scope is omitted. | API-LOCAL-SCOPES |
| Q3: First-release proxy refresh assurance | Validate syntax/existence/eligibility before forwarding broker-prefixed scopes, then strip valid virtual scopes and preserve upstream outcomes/rotation without a usability veto. Subsequent exchange and interactive authorization still check services. | DEC-002; API-PROXY-REFRESH-VALIDATION; ADR-SCOPES-REV1 |

## Current Validation Notes

- All 19 numbered acceptance scenarios have one owning E2E `It` assigned in [tasks.md](../tasks.md#acceptance-scenario--task-mapping), matching the plan. Historical implementation evidence is retained in quickstart; new US3 S3 variants remain unchecked. Generating tasks does not satisfy red/green or completion gates.
- Four editable Wireloom states and their rendered SVGs remain under `../wireframes/`: consent needs login/ready and service session absent/active. Playwright must capture the corresponding meaningful states using the existing harness and retain maintained images under `tests/e2e/screenshots/`.
- All 14 current items in [plan.md Approval Items](../plan.md#approval-items) remain Proposed and await maintainer sign-off in their current wording. ADRs 048/049 already moved to `adrs/` record historical supplied approval; feature-local files are reference pointers. ADR 049 Revision 1 remains Proposed. No approval is granted or extended by this checklist.
- ADR 048 records partial supersession of ADR 044's generic `/sessions` return and unchanged rejected-session clauses. Merged ADR 046's `RefreshRejectedAt` is the single observed-rejection marker, with no user-session schema change. ADR 049 documents virtual scope semantics, the single shared context and identity pinning. Merged ADR 044 is not edited; unrelated Proposed ADR 037 is not promoted.
- Planning selected explicit design details: additive session-detail API, route-local ExtProc metadata, consent coverage data, transient outcomes, userless-grant rejection and migration 048 for code/refresh bindings. These are design proposals, not evidence that application code implements them.
- Task generation reconciled quickstart test ownership and capture instructions with the plan and current harness. It clarified research R2's duplicate wording: identical tokens deduplicate, distinct canonical/UUID tokens remain granted in their requested form while selecting one requested service. This does not change FR-020 or the data model.
- Feature scope excludes MCP client implementation, a new Deny action, required metadata hosting, canonical alias history and unrelated refactoring. The `research.md` deferred finding remains outside the implementation backlog.
- Remediation resolves U1/U2/U3/I1: effective selected provider scopes and candidate contribution
  usability, the same JWE through post-Save authorize re-entry, narrowed successor `Scope` with all
  assurance bindings inherited, and valid access usable without refresh only when no rejection is
  recorded. The task-generation reconciliation applies binding ADR 046: rejection disqualifies valid
  access immediately and refresh expiry is never predicted. Current reconciled wording remains
  Proposed under the existing approval items; supporting tasks retain semantic-red regression
  requirements and the 19-scenario ownership mapping.
- Implementation must record only supplied approvals, establish compiling semantic-red tests before new business logic, complete all buildable requirements and compliance tasks, and preserve the unresolved historical browser-red gap. Unavailable local validation and unfinished work remain incomplete; container-backed suites are deferred to container tests on the pushed commit, never claimed as locally validated.
- Task generation adds mandatory Phase 2g and the feature-local observability contract for feature-owned security records and credential/principal safety tests. No metric, span namespace, exception or unrelated telemetry cleanup is proposed. Existing specification-quality checkbox markers remain unchanged.
