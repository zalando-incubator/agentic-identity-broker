# Specification Quality Checklist: CIMD Client Authentication for Third-Party OAuth2 Services

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-17
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

### Validation iteration 4 — passed

- The specification separates the broker's outbound CIMD-client role from Feature 028's inbound authorization-server role and does not duplicate or supersede Feature 028.
- Feature 042 remains the baseline for public-client `none` behavior. This delta adds only the distinct `private_key_jwt` confidential mode requested for broker-hosted CIMD metadata.
- The `private_key_jwt` choice was validated against [CIMD draft §8.2](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-02#section-8.2): it is the specified confidential-client mechanism when the document publishes public verification keys and no shared secret can be established.
- The CIMD citation separates its two normative bases: [§8.2](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-02#section-8.2) supports `private_key_jwt` confidential authentication, while [§4.1](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-02#section-4.1) prohibits shared secrets and private key material in metadata.
- All 26 functional requirements, 7 API requirements, and 3 persistence requirements map to one or more observable acceptance scenarios or measurable outcomes.
- Mechanical validation confirms that `FR-001` through `FR-026`, `API-001` through `API-007`, and `DB-001` through `DB-003` are each unique and contiguous. Each of the five user stories has a contiguous acceptance-scenario sequence; no clarification markers remain.
- No clarification markers remain. The specification records the stable HTTPS public URL, existing signing-key lifecycle, and upstream-provider compatibility as explicit assumptions.
- Refresh validation confirms that all mandatory headings are present, both local baseline links resolve, the two CIMD normative references are present, all requirement identifiers and acceptance scenarios are contiguous, and no template or clarification placeholders remain.
- The API contract explicitly defines the generated metadata and public-key URL shapes, required response fields and headers, unavailable-service 404 behavior, and administrative fields; recorded stakeholder confirmation remains required before implementation.
- Post-annotation validation confirms 26 functional, 7 API, and 3 persistence requirement identifiers remain unique and contiguous; the five scenario sequences remain contiguous; mandatory headings and local baseline links resolve; and no template or clarification markers exist.

### Validation iteration 5 — passed

- Post-clarification validation confirms `FR-001` through `FR-028`, `API-001` through `API-007`, and `DB-001` through `DB-003` are unique and contiguous.
- The five user-story acceptance-scenario counts are 5, 6, 7, 3, and 3; each sequence is contiguous.
- The clarification session contains exactly three accepted decisions: all OAuth server modes, broker-global signing-key topology, and approval of the public and administrative API contract.
- The approved contract covers the per-service metadata URL, per-service JWK Set URL, response headers and 404 behavior, and service create, update, read, and list semantics.
- No unresolved clarification markers remain. All 16 checklist items remain passing.

### Validation iteration 6 — passed (in-session checkpoint)

- Recorded during the 2026-09-18 clarification session, which is still in progress; a closing iteration follows once the remaining rotation questions are answered.
- `FR-029` was added for the CIMD key-domain separation and ES256-only rule, so the identifiers are now `FR-001` through `FR-029`, `API-001` through `API-007`, and `DB-001` through `DB-003`, each unique and contiguous.
- Acceptance-scenario counts changed from 5, 6, 7, 3, 3 to 5, 7, 8, 3, 3 after adding US2 scenario 7 (JSON Web Key Set separation) and US3 scenario 8 (signing separation); each sequence remains contiguous.
- The clarification record now spans two sessions: three decisions on 2026-09-17 and the key-source decision on 2026-09-18.
- Every normative reference names the CIMD client-authentication key domain; the superseded "broker-global signing-key set" phrasing survives only in the historical 2026-09-17 bullet that the 2026-09-18 bullet refines.
- No unresolved clarification markers remain. All 16 checklist items remain passing; no checkbox state changed.

### Validation iteration 7 — passed with one open gate (2026-09-18 session not yet closed)

- Supersedes the iteration 6 checkpoint. Final identifiers are `FR-001` through `FR-031`, `API-001` through `API-009`, and `DB-001` through `DB-004`, each unique and contiguous.
- Acceptance-scenario counts are 5, 7, 8, 7, and 3; each sequence is contiguous.
- The clarification record holds seven decisions across two sessions: three on 2026-09-17 and four on 2026-09-18 covering the key source, activation policy, rotation trigger and surface, and route placement.
- New requirements from this session: `FR-029` key-domain separation with an ES256-only rule, `FR-030` three-path activation semantics, `FR-031` operator-triggered rotation with no scheduled rotation, `DB-004` CIMD key persistence, and `API-008`/`API-009` for the approved `/api/cimd-client-keys` surface.
- The `/api/cimd-client-keys` paths were corrected after an earlier draft both invented them and falsely recorded them as approved. They are now approved by an explicit 2026-09-18 decision, and are deliberately not nested under `/api/oauth2-server/` or `/api/services/`.
- All local baseline links resolve and no clarification markers remain.
- On "No implementation details": endpoint contracts remain in scope for this repository's specifications because Constitution Principles IV and X require API contracts to be designed, documented, and confirmed before implementation. The item stays checked on that basis.
- Open gate, deliberately not closed: `API-009` records stakeholder approval of the `/api/cimd-client-keys` paths and their four operations only. Request and response shapes, including the error representation, remain unconfirmed and must be approved and documented in the administrative OpenAPI specification before implementation. `API-008` was narrowed to behavioral constraints so it no longer fixes a response shape implicitly.
- All 16 checklist items pass; no checkbox state changed in this session.

### Validation iteration 8 — passed (closes the 2026-09-18 clarification session)

- The iteration 7 gate is now closed. The stakeholder approved the CIMD key request and response shapes, so `API-009` records paths, operations, and shapes together, with no open confirmation gate remaining.
- Final identifiers are `FR-001` through `FR-031`, `API-001` through `API-009`, and `DB-001` through `DB-004`, each unique and contiguous.
- Acceptance-scenario counts are 5, 7, 8, 8, and 3; each sequence is contiguous. US4 scenario 8 covers the non-`ES256` rejection.
- The approved generation contract defaults `algorithm` to `ES256` and rejects any other value, so the CIMD domain does not inherit the token-signing path's tolerance for other algorithms, consistent with `FR-029`.
- The clarification record holds eight decisions: three on 2026-09-17 and five on 2026-09-18.
- All 16 checklist items pass; no checkbox state changed across the session.