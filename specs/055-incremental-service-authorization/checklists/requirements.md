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
| Q3: First-release proxy refresh assurance | Strip virtual scopes and preserve the upstream outcome, including refresh rotation, without a post-response usability veto. Subsequent exchange and interactive authorization still check services. | DEC-002 |

## Current Validation Notes

- All 19 numbered acceptance scenarios have one owning E2E `It` assigned in [tasks.md](../tasks.md#acceptance-scenario--task-mapping), matching the plan. Tests are future implementation work; generating tasks does not satisfy red/green or completion gates.
- Four editable Wireloom states and their rendered SVGs remain under `../wireframes/`: consent needs login/ready and service session absent/active. Playwright must capture the corresponding meaningful states using the existing harness and retain maintained images under `tests/e2e/screenshots/`.
- The 11 items in [plan.md Approval Items](../plan.md#approval-items) remain Proposed and await maintainer sign-off. Both feature-local ADRs remain Proposed. No API, ADR, schema or consequential decision is approved by this checklist.
- ADR 045 proposes partial supersession of ADR 044's generic `/sessions` return and unchanged rejected-session clauses. The existing rejection marker is reused without a user-session schema change. ADR 046 documents virtual scope semantics, the single shared context and identity pinning. Merged ADR 044 is not edited; unrelated Proposed ADR 037 is not promoted.
- Planning selected explicit design details: additive session-detail API, route-local ExtProc metadata, consent coverage data, transient outcomes, userless-grant rejection and migration 046 for code/refresh bindings. These are design proposals, not evidence that application code implements them.
- Task generation reconciled quickstart test ownership and capture instructions with the plan and current harness. It clarified research R2's duplicate wording: identical tokens deduplicate, distinct canonical/UUID tokens remain granted in their requested form while selecting one requested service. This does not change FR-020 or the data model.
- Feature scope excludes MCP client implementation, a new Deny action, required metadata hosting, canonical alias history and unrelated refactoring. The `research.md` deferred finding remains outside the implementation backlog.
- Implementation must record only supplied approvals, establish compiling semantic-red tests before business logic, complete all buildable requirements and compliance tasks, and report any unavailable validation or unchecked task as incomplete.
