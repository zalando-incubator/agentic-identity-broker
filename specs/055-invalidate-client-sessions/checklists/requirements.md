# Specification Quality Checklist: Invalidate Sessions After Service Client-ID Changes

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-11-04
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

- Validation pass 1: 11 of 16 items pass. The five incomplete items depend on the two product questions recorded in the specification; they are not technical execution failures.
- Q1 / FR-013: “Legacy sessions with no recorded establishing client identity require a defined rollout policy.” The current ADR 041 exception and alternative rollout treatments have materially different user impact. Legacy acceptance scenarios and the exact guarantee for those sessions remain incomplete until clarification and any required approved superseding decision.
- Q2 / FR-014: “In addition to FR-007's next-authorization behavior, automatic consent entry requires a product boundary.” The minimum browser-authorization flow has scenarios; live-page or proactive notification scope and additional acceptance scenarios remain undefined until clarification.
- Content-quality checks mean that no implementation strategy, new API, storage field, framework, or crypto mechanism is selected. Existing-contract HTTP examples, configuration excerpts, conceptual domain diagrams, and Wireloom sources are included because Constitution Principle XIV requires reviewable concrete journeys. API and ADR changes remain proposals for later maintainer approval, not approval granted by this checklist.
- Sixteen named acceptance scenarios (AS-1.1–AS-1.7, AS-2.1–AS-2.5, AS-3.1–AS-3.4) cover invalidation, recovery, isolation, refresh paths, overlapping work, and rollback. Every scenario must receive its own end-to-end acceptance test before implementation; this phase creates no test or production code.
- “Feature meets measurable outcomes” records that the proposed scenarios cover SC-001 through SC-005, not that implementation has been tested or delivered.
- Wireloom sources and four generated SVGs accompany the journeys. Render validation: `just --tempdir /tmp/gh-aw/agent render-wireframes specs/055-invalidate-client-sessions/spec.md` passed. The explicit temporary directory was necessary because the default Just runtime directory was not writable.
- Items marked incomplete require specification updates in the separate `/speckit-clarify` phase before planning readiness. Publish this draft for that handoff; do not resolve product choices or run planning in this session.
- Planning update 2026-11-05: The clarification answers resolved Q1 and Q2. The maintainer's planning direction added AS-1.7 (Admin API change) and AS-3.4 (Admin API secret rotation and same-identity updates), and made AS-1.1 and AS-3.2 explicit filesystem scenarios. The Admin API journey variant uses the existing contract. The four wireframes are unchanged.
- Planning reconciliation 2026-11-05 (after merging `main`): `main` now owns ADRs 045 and 046 and migration 046, so the draft decision is ADR 047 and the migrations are 047/048. The Admin API baseline is 2.1.1, with a proposed description-only 2.1.2. No requirement changed.
