# Specification Quality Checklist: Agent-Subject Impersonation

**Purpose**: Validate specification completeness and quality before proceeding to planning

**Created**: 2026-10-26

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

- Validation pass 1 checked the draft against the issue title/body, existing impersonation source, end-user contract, relevant ADRs, constitution, and active specification template. The third-party denial example was corrected to use explicit existing claim extraction and the existing missing-grant response, including its consent URL. Validation pass 2 reviewed the resulting specification and retained the incomplete items below intentionally.
- Protocol, configuration, CEL, and decoded claim examples describe observable domain behavior required by Constitution Principle XIV. No new implementation library, handler structure, persistence design, or executable code is prescribed; API references are contract dependencies, not an approved replacement API.
- **Status update (planning re-run 2026-10-29)**: Pass-1 notes about incomplete items are historical. Clarification sessions 2026-10-26 to 2026-10-28 resolved Q1 (exact signed `sub`/target-UUID equality, no request input), Q2 (self-agent only, option A), and Q3–Q5; the spec has no `NEEDS CLARIFICATION` markers, and Journeys 1–3 contain the concrete configuration examples. Governance (ADR 045 superseding ADR 032 decisions 1 and 2 for agents) and the API contracts are registered for sign-off in `plan.md` Approval Items.
- *(Historical)* Mandatory sections, clarification markers, provisional acceptance, and pending outcomes recorded in pass 1 were resolved by the clarification sessions above.
- **Governance dependency**: ADR 032 requires delegation for every impersonation subject. Consent-free agent issuance requires the superseding ADR 045 (Proposed) to be approved before implementation; OpenAPI and configuration changes likewise require explicit sign-off (`plan.md` Approval Items).
- The unsigned-subject exception is not broadened. Signed machine subjects are the documented bounded assumption; existing human controls remain intact.
- No issue comments were fetched or used. Only this new feature's specification and checklist belong in the PR. Planning, tests, implementation, optional context-update hooks, and optional commit hooks are deferred.

### Q1 — Subject category selection and binding

**Context**: FR-003 requires unambiguous classification and prohibits consent bypass by relabeling, while a rule may permit both categories.

**What we need to know**: How does a request select human versus agent, and how is that selection bound to the identity the rule authorizes? What are the exact operator setting and request/claim contract?

| Option | Answer | Implications |
| --- | --- | --- |
| A | Explicit request category, authorized against credential identity and per-rule eligibility | Requires a confirmed request parameter, safe default for legacy callers, and validation that prevents human relabeling. |
| B | Category derived from a trusted signed subject claim under configured policy | Requires a defined trusted claim/mapping contract and fail-closed handling for missing or ambiguous values. |
| C | Configured subject-category resolver evaluated within the rule | Requires a precise resolver contract, dual-category semantics, and safeguards against caller-controlled reclassification. |
| Custom | Provide another binding and selection mechanism | Describe legacy behavior, dual-category selection, exact configuration, and why human consent cannot be bypassed. |

**Your choice**: Custom (recorded in `spec.md` Clarifications, session 2026-10-26): agent category only when the verified signed `sub` exactly equals the audience-selected target UUID; no request category input or category claim.

### Q2 — Self-agent scope and identifier resolution

**Context**: FR-004 requires the resolved subject's UUID in `sub`. Existing routing independently resolves a registered target that supplies issued `agent_id`. The issue describes an agent acting for itself and asks for `subject = agent_id`.

**What we need to know**: Must subject and target always be the same registered agent, or may one agent be represented with a different target? Which identifiers can be supplied for the subject and how are they resolved?

| Option | Answer | Implications |
| --- | --- | --- |
| A | Self-agent only; subject credential identifies the target's exact broker UUID | Keeps `sub == agent_id`; mismatched subjects are denied and no additional alias resolution is needed. |
| B | Self-agent only; subject may use broker UUID or canonical identifier, resolved to the target UUID | Keeps `sub == agent_id`, but requires confirmed alias resolution and unknown/mismatch errors. |
| C | A different registered subject agent may be represented | Expands scope; requires independent subject resolution, policy binding, and explicit meanings for `sub` versus target-derived `agent_id`. |
| Custom | Provide another identity mapping | Specify accepted inputs, resolution, subject-target relationship, and mismatched/unknown behavior. |

**Your choice**: A (recorded in `spec.md` Clarifications, session 2026-10-26): self-agent only, exact broker UUID.
