# Quickstart: Validate Session Invalidation After Client-ID Changes

**Feature**: [spec.md](spec.md) | **Contracts**: [contracts/](contracts/) | **Data model**: [data-model.md](data-model.md)

This guide proves the feature end to end. It references the acceptance scenarios instead of restating them.

## Prerequisites

- `just install-tools` completed (Go, Ginkgo, Node, and Playwright browsers).
- Docker or Podman only for the PostgreSQL-backed suites (`just test-integration-infra`, `just test-e2e-docker`).
- Approved items API-001, API-002, ADR-001, and DEC-001 to DEC-005 (`plan.md` Approval Items).

## Deployment prerequisite — guarded cutover (Proposed DEC-005)

Before enabling identity changes, follow [ADR 047's guarded rollout](adr-047-client-identity-revisions.md#guarded-rollout-and-rollback-proposed-dec-005):

1. Freeze service/client-ID/source/canonical-ID/binding/file changes; pause Admin automation, discovery/DCR writers, external sweeps, and old-replica restart controllers.
2. Close both ingress planes and direct/internal callers. Drain requests and refresh work, stop every old broker/writer, and verify zero old replicas/jobs. A routing-only change is not enough.
3. Apply and validate migrations 047/048; start revision-aware binaries with coherent bindings/files while traffic/writes remain closed. Verify complete image/schema inventory and that old targets cannot restart. Failed checks keep the system closed.
4. Open traffic, then resume writers and identity changes only after every process enforces revisions. Pre-upgrade callbacks missing explicit identity/revision must restart sign-in; unchanged legacy stored sessions retain DEC-001.
5. After activation, rollback only to a revision-aware release with schema/evidence retained. An older binary remains offline even at revision 0; a guarded schema DOWN does not authorize restoring its traffic.

This maintenance outage and constrained rollback are Proposed decisions, not an optional enforcement switch. Required operator documentation records actual drain and image inventory checks; automated tests alone cannot prove a deployment is free of old processes.

## 1. Static checks and fast tests

```bash
just check
just test          # domain: revision recording, usability rule, enforcement points
just web-test      # sessions page and consent card states
```

Expected: everything passes. Observation concurrency tests pause a file read, commit filesystem→stored or canonical-ID/binding changes (including a same-revision source round trip), and assert the obsolete value cannot record evidence. Retry reloads service/source/version/binding and file rather than replaying it; a later same-ID update produces no false increment. Administrative metadata tests still assert no file reads; consent/session usability displays read only client-ID files and use persisted evidence on source failure. The domain tests include the AS-1.5 rollback and AS-1.6 legacy matrices against the in-memory store, plus FR-015's cross-product of identity validity, recorded refresh rejection, and access-token refreshability. Rejected sessions still block issuance with unexpired tokens; identity-valid expired tokens with stored, not-rejected refresh tokens remain refreshable.

## 2. Backend acceptance journeys (in-memory store)

```bash
ginkgo -v --label-filter='!performance && !docker' --focus "Client identity change" ./tests/e2e/
```

| Scenario group | Proves |
|---|---|
| AS-1.1, AS-1.7 | A filesystem publication and an Admin API `PUT` each turn an unexpired token into `400 invalid_grant` with ADR 044's `error_uri` for every user (Journey 1, Step 3). |
| AS-1.2–AS-1.6 | Per-user independence, refresh and sweep denial, in-flight callback rejection, rollback, and the legacy rollout variant. |
| AS-2.1–AS-2.5 | Automatic consent on the next agent authorization, consent-initiated and direct reconnection, failure and retry, and `requires_reauthentication`. |
| AS-3.1–AS-3.4 | Unrelated services and grants are untouched. Filesystem and Admin API secret rotations, metadata updates, and same-identity transitions invalidate nothing. AS-3.1 also retains ADR 046's rejection and refreshability outcomes for unchanged services and verifies `is_expired == requires_reauthentication`. |

Expected: one passing `It` per scenario, as mapped in `plan.md` Testing Strategy. AS-1.6's guarded-cutover matrix stops the old generation before new serving/writing resumes and preserves unchanged legacy sessions, then verifies post-cutover invalidation. AS-1.4 includes pre-upgrade callback fixtures missing explicit identity/revision (including public/CIMD/DCR/Google): reject before code exchange even at R=0, with no provider call or session replacement. No test may claim mixed-version serving is safe. AS-1.3 asserts changed Admin 3.0.0 sweep totals: one due identity-invalid candidate yields failed=1, refreshed=0 in real and dry runs; dry runs write no evidence.

## 3. Browser journeys

```bash
just test-e2e-frontend
```

Expected: consent shows Reports Production as needing sign-in and Calendar as connected (State A). Reconnecting returns with selections preserved (State B). The sessions page shows "Sign-in required" (State C), then connected (State D). Failed reconnection leaves sign-in and retry available.

Normal verification does not capture screenshots. For maintained review artifacts, run the existing capture path in serial mode:

```bash
E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend
```

The harness writes captures under its suite-relative `coverage/screenshots/` directory. Use the existing maintained-image flow to place the five named artifacts from `plan.md` into `tests/e2e/screenshots/`; no screenshot-workflow change is needed.

## 4. PostgreSQL persistence and migration (container suites)

```bash
just test-integration-infra
just test-e2e-docker
```

These commands run in the container-test environment on the pushed commit, not in the agent sandbox. Their absence locally does not block the non-container implementation or count as validation. Record their CI evidence separately.

Expected:

- Migrations 047 and 048 apply, roll back, and reapply.
- DOWN 047 refuses to run after any revision increment and leaves the schema unchanged.
- Concurrent observations across two connections produce exactly one increment. Source/version/canonical-ID/revision CAS fences delayed observations across filesystem→stored, canonical-ID/binding changes, and source round trips; stale values are discarded and later same-ID updates do not increment spuriously.
- A rejected or version-conflicting `PUT` records nothing.
- The invalidation survives a broker restart.

## 5. Manual smoke check (optional, local)

1. Start the broker with `examples/config/third-party-oauth2.yaml`, using a filesystem-bound service and the mock OAuth2 provider from `mocks/`.
2. Connect a session, then write a new client ID to the client-ID file. Request a token exchange. Expect `400 invalid_grant` and the `session.oauth2.client_identity_changed` log event with `channel=filesystem_observation`.
3. Restore the old client ID and repeat the request. Expect `invalid_grant` again (AS-1.5).
4. Rewrite only the secret file. Reconnect, then request a token. Expect success with no new event (AS-3.2).
