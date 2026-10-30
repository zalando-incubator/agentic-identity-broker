# Specification Quality Checklist: Public Client Support for Third-Party OAuth2 Services

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-10
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

## Internal Consistency

- [x] Requirement IDs unique and contiguous: FR-001..029, API-001..010, DB-001..008, SR-001..009,
      SC-001..008 (verified mechanically; no duplicates, no gaps)
- [x] Cross-references resolve to their renumbered targets (FR-007, FR-014, FR-018, FR-026)
- [x] `none` versus absent is used consistently across stories, requirements, API, DB, and diagrams
- [x] All three Mermaid diagrams parse under the Mermaid parser

## Constitution Alignment

- [x] Principle I (Security-First): fails closed on contradictory configuration (FR-005, SR-004);
      PKCE non-disableable (FR-010, SR-001); no credentialed fallback (FR-015, FR-016, SR-005)
- [x] Principle IV / X (API-First): API-001..API-010 enumerate the contract changes; API-010 flags
      the required stakeholder confirmation before implementation
- [x] Principle V (DDD): entities and value objects documented; glossary additions called out
- [x] Principle IX (Persistence): migration numbering, nullability, constraint, rollback refusal,
      and both-adapter coverage specified (DB-001..DB-008)
- [x] Principle XIII (E2E traceability): every acceptance scenario is an observable Given/When/Then
      suitable for 1:1 mapping to an E2E test

## Notes

### Validation findings and resolutions

1. **Resolved — contradictory default.** The first draft defaulted an omitted method to
   `client_secret_post` while forbidding runtime negotiation. Against current behavior — the code
   exchange negotiates the credential channel, only refresh pins body parameters — that would
   silently break a provider accepting only header-based credentials whose sessions never refresh.
   The attribute now has **no default**; absence means "confidential, authenticated exactly as
   today" (FR-003, FR-017, FR-023, DB-004).

2. **Resolved — scope reduced to the request.** Once absence preserved today's behavior, the
   `client_secret_basic` and `client_secret_post` values had no regression left to prevent, so they
   were cut. The attribute now accepts exactly one value, `none`. The excluded values are recorded
   under Out of Scope with the reasoning, so the option stays documented without being built.

3. **Resolved — read representation contradiction.** API-002 stated the method reports as `null`
   when a service declares `none`, contradicting User Story 1 scenario 1 and FR-024. Corrected:
   `none` for public services, `null` for confidential ones.

4. **Resolved — undefined update semantics.** The update operation is full-replacement and already
   requires the client secret on every call, but the draft left an omitted method on update
   undefined while FR-021 referred to "clearing the method". Now explicit: the method is evaluated
   from the request alone, omission makes the service confidential and therefore requires a secret
   (FR-019, FR-021, API-005, User Story 4 scenarios 2-4). API-003's treatment of explicit `null` was
   later reversed — see finding 11.

5. **Resolved — unrequested event subsystem.** The draft listed `PublicServiceRegistered` and
   `ServiceAuthMethodChanged` as domain events. No domain event bus exists in this flow. Replaced
   with a note that observability is the existing structured audit log `event` key convention.
   FR-026, FR-027, and SR-008 remain, since Constitution Principle I requires security-critical
   operations to be auditable.

6. **Resolved — over-broad variant restriction.** The draft rejected `none` for both the Google and
   GitHub variants. Only Google is structurally incompatible, because it derives the client
   identifier from the credential document. The GitHub restriction was an outside business
   assumption, not an AIB invariant, and was removed (FR-007).

7. **Verified — PKCE is not new.** The broker already sends `code_challenge` with `S256` upstream
   and seals the verifier in the JWE state token. Stated in the Context section so planning does not
   re-implement existing behavior.

8. **Verified — no configuration or frontend surface.** Third-party services are registered through
   the administrative API only. The Configuration Requirements and Frontend/Design System
   Requirements template sections were removed rather than filled with "not applicable".

9. **Resolved by clarification — update semantics.** `/speckit-clarify` session 2026-09-10 asked
   whether omitting `token_endpoint_auth_method` when updating a public service should reject or
   preserve the stored status. Answer: full replacement — omission makes the service confidential
   and the request is rejected without a credential. This confirms FR-019, FR-021, and API-005 as a
   deliberate decision rather than an inference, and rules out presence-detection semantics.

10. **Resolved by evidence — branch-key provisioning and credential scope.** Two candidate
    ambiguities were investigated in code rather than asked. Branch-key provisioning already
    precedes secret encryption in the service lifecycle and session tokens use the same
    service-scoped key, so public services must still provision it (FR-028). Third-party credentials
    reach an upstream token endpoint from exactly two call sites, both in the OAuth2 session
    service; ExtProc's client credential targets its own authorization server, and no background
    refresh worker exists (FR-029). RFC 8693 token exchange refreshes through the same on-demand
    path and so inherits the behavior without separate work.

11. **Reversed during `/speckit-plan` — explicit `null` on write.** API-003 originally rejected an
    explicit `null` for `token_endpoint_auth_method`, on a "one representation per state" argument.
    That argument does real work for `client_secret`, where `""`, `null`, and absence could plausibly
    differ in meaning, but none for a closed two-state enum where `null` and absence have exactly one
    meaning between them. Rejecting `null` also broke the ordinary read-modify-write round trip,
    because the read representation reports `null` for every confidential service (FR-024), and it
    would have forced raw-map presence detection onto both the create and update handler paths.
    `null` is now accepted as equivalent to omission. Consequence: `"client_secret": ""` alongside
    `none` is accepted rather than rejected as a contradiction, since an empty string is not a
    credential; a non-empty secret alongside `none` is still rejected (FR-005). Acceptance scenario
    US4-S6 covers the round trip. Confirmed by the user on 2026-09-10.

### Items carried into `/speckit-plan` — both resolved

- **API-010 stakeholder confirmation** — cleared on 2026-09-10. The two substantive decisions were
  approved: `client_secret` omitted rather than redacted for public services, and explicit `null`
  accepted as equivalent to omission on write (finding 11). Recorded in
  [contracts/admin-api.md §8](../contracts/admin-api.md) and the plan's Constitution Check.
- **E2E upstream test double** — planned. The double inspects neither PKCE parameters nor client
  credentials, so [research.md §D10](../research.md) adds an opt-in strict public-client mode that
  rejects any request carrying a credential and verifies the `S256` code verifier. Default-off, so
  no existing suite changes behaviour.
