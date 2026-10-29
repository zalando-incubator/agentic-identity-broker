# Specification Quality Checklist: Incremental Service Authorization

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-10-29
**Feature**: [spec.md](../spec.md)

**Review ownership**: The specify phase maintains this built-in checklist. Checked items describe requirements quality, not implementation completion or maintainer approval.

## Content Quality

- [x] No implementation details (languages, frameworks, APIs) beyond the public protocol examples and design-precondition sections required by the repository template
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
- [x] No implementation details leak into specification decisions: no stack, package wiring, schema or new configuration-key design is selected

## Validation Notes

- Reviewed the draft against the template and all checklist criteria. Three product questions remain, as expected for the separate clarification handoff. This draft is not ready for implementation or approval.
- Q1 / FR-019 asks whether old canonical scope URLs survive a rename. Alias compatibility changes release scope and acceptance expectations.
- Q2 / FR-020 asks whether broker-issued tokens expose virtual scopes. Journey 1 Step 8 explicitly labels visible scopes as the proposed answer, not a resolved contract. Story 3 Scenario 4 cannot be finalized until this is answered.
- Q3 / FR-021 asks whether proxy refresh includes a post-upstream-response check. Story 3 Scenario 3 and Journey 3 distinguish the two scopes of delivery without claiming an outcome for the unresolved branch.
- Accordingly, the clarity, scenario-completeness, scope and complete-acceptance-coverage items remain unchecked. Other scope boundaries are explicit: no MCP client implementation, new Deny action, mandatory metadata hosting or unrelated changes.
- The standard “no implementation details” criteria are evaluated against the installed project template: its API/configuration/database/security/frontend sections and constitutional protocol/UI examples are required product/design constraints, not implementation work. Proposed contract changes are clearly labeled rather than represented as existing APIs.
- Accepted ADR 044 currently requires a generic `/sessions` return and leaves provider-rejected stored sessions unchanged. The draft identifies both proposed deviations for later approved supersession; it neither changes that ADR nor records an approval. ADR 037 remains Proposed.
- Each numbered scenario has a stable story/scenario reference for later 1:1 E2E mapping. No tests or implementation are included in this specification phase.
- Four editable Wireloom states in the spec were rendered successfully to `../wireframes/` using `node tools/wireloom/render.mjs specs/055-incremental-service-authorization/spec.md`. The `just render-wireframes` wrapper could not use its runtime directory in this environment; the repository's documented renderer completed the generation directly. All three renderer tests passed via `node --test tools/wireloom/render.test.mjs`.
- Optional after-specify agent-context and git-commit hooks were skipped under unattended-run rules. The mandatory git-feature pre-hook completed on `speckit/issue-299`.
- Items marked incomplete require clarification and spec updates before planning can be considered complete. They do not prevent publishing this draft PR for the clarification session.
- 2026-10-29 (planning): The clarification session resolved Q1–Q3, which supersedes the open-question notes above. Planning aligned the Journey 2 and Journey 4 examples, Configuration Requirements, API-001 and Database Requirements with the designed contracts. It recorded each change under Clarifications › Session 2026-10-29 (planning). No requirement remains unresolved, and the approval items in `plan.md` await maintainer sign-off.

## Clarification Handoff

### Question 1: Canonical-ID rename compatibility

**Context**: “When a service canonical ID is renamed, must previously issued canonical-ID scopes keep resolving through aliases, or is only the UUID scope durable?” (Q1 / FR-019)

**What we need to know**: What happens to scope links and previously granted service scopes after a canonical ID rename?

| Option | Answer | Implications |
| --- | --- | --- |
| A | Only UUID scopes are durable; clients must re-challenge after a canonical rename. | No alias lifecycle in this release; old canonical links no longer resolve. |
| B | Preserve old canonical values as service aliases. | Scope matching and service pages must resolve aliases; alias uniqueness and lifetime need an explicit policy. |
| Custom | Provide another compatibility policy. | State what existing clients and links observe and when compatibility ends. |

**Your choice**: Awaiting the separate clarification session; no answer assumed.

### Question 2: Visibility of virtual scopes in broker tokens

**Context**: “Should broker-issued access tokens and token-response scope values include the granted virtual scopes as proposed, or hide them?” (Q2 / FR-020)

**What we need to know**: Which externally visible scope contract should local issuance guarantee?

| Option | Answer | Implications |
| --- | --- | --- |
| A | Include virtual scopes in both granted token-response scopes and broker-issued access-token scope claims, as proposed. | Client-visible granted scopes and token claims agree; local refresh remembers and rechecks them. |
| B | Hide virtual scopes from broker-issued access tokens and token responses. | Define how clients know the granted service set and how refresh retains and rechecks it without those visible claims. |
| Custom | Provide a precise alternative representation. | Specify response scopes, signed claims and refresh behavior together. |

**Your choice**: Awaiting the separate clarification session; Journey 1's visible-scope example is provisional.

### Question 3: First-release proxy refresh assurance

**Context**: “Does this release ship proxy refresh stripping plus recovery only, or also a post-upstream-response service-usability check that can refuse the refresh?” (Q3 / FR-021)

**What we need to know**: Is proxy refresh required to reject unusable requested services in this release?

| Option | Answer | Implications |
| --- | --- | --- |
| A | Strip virtual scopes and rely on subsequent service challenges/recovery for this release. | No proxy refresh usability guarantee; local issuance has stronger checks and documentation must make the difference explicit. |
| B | Also check service usability after upstream refresh and refuse success when it fails. | Requires an approved client-visible error and safe handling when upstream already rotated its refresh token. |
| Custom | Define a different release boundary. | State what reaches upstream, what clients receive and how an upstream rotation is preserved. |

**Your choice**: Awaiting the separate clarification session; no follow-on phase has been run.
