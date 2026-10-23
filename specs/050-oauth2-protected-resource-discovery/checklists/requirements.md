# Specification Quality Checklist: Protected Resource Discovery for Third-Party OAuth2 Services

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-10-01
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
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
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`

- **Iteration 1 — incomplete requirements and acceptance**: API-003 says "read-only discovery-status resource" but gives no path or precise response fields. FR-016 says status "distinguishes the last attempt from the last successful configuration" but does not require that a failed attempt survives a restart.
- **Iteration 1 — incomplete acceptance coverage**: Story 4 scenario 2 says "status separates the failure from the last success" without a restart. The draft has no acceptance case for durable failed-refresh status alongside the unchanged active configuration.
- These gaps leave "Requirements are testable and unambiguous," "All acceptance scenarios are defined," and "All functional requirements have clear acceptance criteria" incomplete. A consumer-visible Admin API path is a contract, not an implementation choice.
- **Iteration 2 — passed**: API-003 names `GET /api/services/{service-id}/discovery-status` and its public fields. DB-001 through DB-005 require durable identities and status while protecting the active configuration during failed refresh. Story 4 scenario 5 and SC-008 cover the restart boundary. The specification has 19 complete acceptance scenarios and no unresolved markers.
- **Planning gate**: API-006 requires written stakeholder confirmation of the proposed administrative contract before implementation. This is a future approval step, not a missing specification requirement.
- **Iteration 3 — open protocol gaps**: FR-005 says "including a supported OpenID Connect metadata form when applicable" without the order or paths of OAuth2 and OpenID Connect discovery. FR-011 says requests "MUST identify the selected resource" without requiring RFC 8707's `resource` parameter or defining rejection when the server does not accept it.
- These gaps reopen "Requirements are testable and unambiguous," "All acceptance scenarios are defined," and "All functional requirements have clear acceptance criteria" until explicit request behavior and acceptance scenarios are added.
- **Iteration 4 — passed**: FR-003 defines path-specific protected-resource metadata discovery. FR-005 names the exact path-issuer and root-issuer probe order and limits fallback to `404 Not Found`. FR-011 requires one RFC 8707 `resource` parameter on MCP authorization, code exchange, and refresh requests, with no retry without it. Stories 1–3 cover these behaviors in 26 numbered acceptance scenarios.
- **Explicit limit**: The broker can detect a rejected resource parameter, but metadata cannot prove the audience of an opaque token from a non-conforming server that silently ignores it. The specification records this assumption instead of claiming untestable audience validation.
