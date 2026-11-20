# Specification Quality Checklist: Business Event Ledger

**Purpose**: Validate specification completeness and quality before implementation
**Created**: 2026-11-12
**Feature**: [spec.md](../spec.md)

Checked items mean that requirements-quality validation passed. They do not mean that
implementation or maintainer approval is complete.

## Content Quality

- [ ] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [ ] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [ ] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [ ] No implementation details leak into specification

## Notes

- Validation iteration 1 (2026-11-12): reviewed the specification against every item.
- Four criteria remain unchecked. The specification targets operators and contributors and
  includes technical contracts, storage adapters, and file counts. These are documented
  deviations, not passing criteria or approval to proceed.
- The specification was written by hand from the re-planned design, not by `/speckit-specify`.
  Issue #188 is the input. Its presentation requirements (summaries, timelines, filters,
  accessibility) are out of scope here and belong to the activity feature.
- Journey 1 was checked against `api/enduser/openapi.yaml` (`DELETE
  /api/consent/agents/{agent-id}/grants` returns `204`) and `config.yaml` (`X-Remote-User`). The
  stdout record shows the target shape after Phase 3; on `main` today the record has
  `action=grant_revoked` and not `event`.
- All sign-off items are in `plan.md`, Approval Items. No approval is inferred.
- Validation iteration 2 (2026-11-12): the scope was reduced to the first stored event, and the
  ten findings of `/speckit-analyze` were resolved. Further events are out of scope. Retention
  stays in scope as an event-independent part of the foundation (maintainer decision).
- Validation iteration 3 (2026-11-18): clarified FR-001 and FR-008 for analysis finding I1.
  Eligible records use `Info` or above. An enabled ledger stores them independently of the
  operational sinks' `log.level` threshold, matching the existing handler contract and
  `20261118-125900-business-events-as-typed-log-records`. Existing checklist deviations and approval gates remain unchanged.

## Deviation Review — 2026-11-18

The review retains the technical scope and leaves all checkbox markers unchanged.

| Unchecked criterion | Evidence in the current specification |
|---|---|
| No implementation details (languages, frameworks, APIs) | Edge cases name `With(...)` and `slog.LogValuer`. FR-003 and FR-011 specify memory and PostgreSQL adapters. |
| Written for non-technical stakeholders | The intended readers are operators and contributors. The concrete journeys use HTTP, structured records, and storage terminology. |
| Success criteria are technology-agnostic (no implementation details) | SC-001 limits production files to the consent service package. SC-004 measures the PostgreSQL adapter. |
| No implementation details leak into specification | FR-005 specifies transaction boundaries. Database Requirements specify the table, JSON attributes, and index structure. |

Principle XIV requires concrete configuration and HTTP examples, so technical examples alone are not a reason to remove those journeys.
The specification also retains implementation prescriptions beyond those examples. The four criteria therefore remain unmet.

`/speckit-implement` must request explicit permission before proceeding with these unchecked criteria.
The notes and existing design approvals do not grant that permission. This review records no waiver.
