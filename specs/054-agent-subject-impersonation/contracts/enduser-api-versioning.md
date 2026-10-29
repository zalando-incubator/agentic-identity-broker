# Proposed End-User API 2.0.0 compatibility contract

**Status**: Proposed; plan Approval Items A10. No maintainer approval is recorded.

## Release-status assessment

The checked-out `api/enduser/openapi.yaml:92` declares End-User API `1.0.0` and already documents `POST /oauth2/token`. The existing impersonation flow in `ARCHITECTURE.md` §3.1.y and Accepted ADR 032 requires delegation without the new category distinction. No supplied evidence establishes that this affected contract is unreleased or has never been deployed. Do not equate this new feature's unimplemented status with an unreleased baseline contract. Conservatively treat the baseline contract as released; delivery must meet Constitution IV/X's major-version requirement regardless of later release-status evidence.

## Recommended decision (Proposed A10)

Change `api/enduser/openapi.yaml` `info.version` from `1.0.0` to `2.0.0` before implementing the changed behavior. End-User API 2.0.0 replaces the previous contract on the existing routes, including `POST /oauth2/token`; there is no versioned route, selector, compatibility alias, or parallel legacy behavior. The OpenAPI major version is distinct from the broker release tag and from the Admin API version. This follows the major-contract-bump pattern already documented for Admin API 2.0.0 in `ARCHITECTURE.md` (feature 051), not an exemption from versioning.

The release includes both intentionally incompatible outcomes: (1) a verified signed raw `sub` exactly equal to the audience-selected target UUID is ineligible on human-only or omitted-category rules even with active same-text delegation (A8); (2) default third-party detection rejects any equal `sub`/`agent_id` token, including legacy, mapped-principal, unsigned-subject human outputs and trusted external issuer tokens (A9). Agent-category issuance, C2–C4 errors and the new configuration contracts are part of the same major contract. Nonmatching human flows, their existing errors and consent links, issuer/signing/TTL/scopes and policy-owned custom claims remain unchanged.

**Rationale**: a major contract bump meets Constitution IV/X while preserving the approved scope's single endpoint and requested fail-closed defaults. **Main alternative**: add a new versioned endpoint and maintain 1.0.0 behavior in parallel. That introduces discovery, routing, client selection and duplicated security-policy maintenance outside the requested extension; do not implement it without a revised approval.

## Required migration guidance

Before release, add an End-User API 2.0.0 migration section to `docs/api/enduser-api.md`, cross-linked from configuration and token-exchange concept/reference guides. It must state:

1. Inventory clients using local impersonation and third-party RFC 8693 exchange before upgrading. The URL, request fields and response envelopes are unchanged, but the two collision cases now deny; ordinary human delegation remains mandatory.
2. For intended self-agent issuance, configure a signed rule allowing `agent` and a narrowly authorizing CEL policy. Do not enable agent issuance as a workaround for a real human identity collision; same-text human grants never restore eligibility on a human-only rule.
3. For real human collisions, migrate the upstream human subject/mapping so the minted principal differs from the target UUID, and explicitly reestablish any needed human delegation/session under the intended human identity through existing flows. Do not automatically rewrite grants or sessions, fabricate consent, or add database migrations in this feature.
4. Review previously minted and trusted external tokens with equal `sub`/`agent_id`: they are rejected by the new default regardless of issuance date or issuer. Reissuance without fixing identity collisions does not restore access. Do not recommend `agent_subject_expression: false` as a compatibility fix; it removes categorical protection. Any operator override retains the documented C4 responsibility and should keep matching broker agent tokens.
5. Rollout requires consumer readiness and passing complete acceptance/regression checks. If consumers are not ready, defer deployment rather than weaken controls or claim 1.0.0 compatibility. No endpoint is removed or deprecated, so an endpoint-removal deprecation marker is not applicable.

## Blocking design and verification gates

- T001 verifies approval of A10 in addition to A1–A9; approval of A8/A9 alone is insufficient.
- T010 applies `info.version: 2.0.0` plus the behavioral OpenAPI delta, and documents this versioning/replacement strategy in `ARCHITECTURE.md` before behavioral code changes.
- T013–T017 write and run all 12 acceptance scenarios before any production behavior, including T009's Helm changes. T008 supplies the separate Helm semantic-red gate.
- T048 publishes the migration guide; T053, T058 and T059 block completion unless the actual OpenAPI version, architecture strategy, migration documentation, A8/A9 denials, and unchanged nonmatching human flows agree. T070–T072 execute the complete acceptance, regression and full verification gates. Record evidence in `quickstart.md`; no artifact here claims those checks have already passed.
