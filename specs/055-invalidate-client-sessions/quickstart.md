# Quickstart: Validate Session Invalidation After Client-ID Changes

**Feature**: [spec.md](spec.md) | **Contracts**: [contracts/](contracts/) | **Data model**: [data-model.md](data-model.md)

This guide proves the feature end to end. It references the acceptance scenarios instead of restating them.

## Prerequisites

- `just install-tools` completed (Go, Ginkgo, Node, and Playwright browsers).
- Docker or Podman only for the PostgreSQL-backed suites (`just test-integration-infra`, `just test-e2e-docker`).
- Approved items API-001, API-002, ADR-001, and DEC-001 to DEC-005 (`plan.md` Approval Items).

## Deployment prerequisite — continuous rolling upgrade (Proposed DEC-005)

Before resuming identity changes, follow [ADR 047's guarded rollout](adr-047-client-identity-revisions.md#guarded-rollout-and-rollback-proposed-dec-005):

1. Record and freeze effective client IDs, credential sources, canonical IDs, and bindings across replicas. Pause identity-mutating automation/direct writers and DCR re-registration; defer new identity-assigning registrations. Keep both ingress planes and existing token/refresh/browser/session traffic open. Same-ID secret rotation and metadata-only writes may continue if frozen fields cannot change.
2. Apply and validate additive migrations 047/048 online before starting new replicas. Verify prior-release explicit-column reads, updates preserving unknown columns, and revision-0 legacy inserts against the expanded schema. Lock contention or validation failure aborts deployment, not healthy serving; retain the freeze and never force past failed validation.
3. Roll revision-aware replicas in incrementally with unchanged IDs/bindings. Old and new replicas may serve simultaneously; new replicas enforce immediately without a switch. Legacy callback state reaching a new replica must restart sign-in; unchanged eligible legacy stored sessions retain DEC-001.
4. Withdraw and drain old replicas/jobs and all their admitted callbacks, refreshes, sweeps, and writes individually while healthy new replicas serve. Verify the complete serving/writing image inventory, completed old work, and prevention of old-image restart before resuming identity mutations (activation). A failed check keeps healthy traffic and the freeze intact.
5. Before activation, a prior-binary abort requires verified unchanged identities, zero revision increments, drained new work, and retained schema/evidence; zero revisions alone are insufficient. After activation, rollback only between revision-aware releases with evidence retained, even at revision 0. If the freeze was breached, isolate unaware processes and restore a revision-aware fleet rather than claiming a safe abort.

No maintenance outage or global ingress closure is required. The temporary identity-mutation/new-registration restriction and rollback boundary remain Proposed decisions, not an optional enforcement switch. Required operator documentation records actual freeze, drain, and image inventory checks; automated tests alone cannot prove a deployment is safe to activate.

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

Expected: one passing `It` per scenario, as mapped in `plan.md` Testing Strategy. AS-1.6's rolling-upgrade matrix keeps token, refresh, browser/session, same-ID rotation, and metadata operations working under fixed identities, drains old admitted work while new replicas serve, preserves unchanged legacy sessions, and verifies invalidation only after inventory verification and resumption of identity mutations. Its failed-upgrade/abort variant retains healthy traffic and the freeze; an old-binary abort requires unchanged identities, zero revisions, drained new work, and retained evidence. AS-1.4 includes pre-upgrade callback fixtures missing explicit identity/revision (including public/CIMD/DCR/Google): reject before code exchange even at R=0, with no provider call or session replacement. The PostgreSQL variant uses the actual supported prior release to prove simultaneous old/new traffic against the expanded schema; never disable enforcement in new code to impersonate an old binary. No test may claim identity mutations are safe while unaware readers/writers remain. AS-1.3 asserts changed Admin 3.0.0 sweep totals: one due identity-invalid candidate yields failed=1, refreshed=0 in real and dry runs; dry runs write no evidence.

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

`just test-e2e-docker` pulls the pinned prior-release image for AS-1.6; set `CLIENT_IDENTITY_PRIOR_RELEASE_IMAGE` to use a mirror ([research.md](research.md) R12). These commands run in the container-test environment on the pushed commit, not in the agent sandbox. Their absence locally does not block the non-container implementation or count as validation. Record their CI evidence separately.

Expected:

- Migrations 047 and 048 apply, roll back, and reapply.
- DOWN 047 refuses to run after any revision increment and leaves the schema unchanged.
- Concurrent observations across two connections produce exactly one increment. Source/version/canonical-ID/revision CAS fences delayed observations across filesystem→stored, canonical-ID/binding changes, and source round trips; stale values are discarded and later same-ID updates do not increment spuriously.
- A rejected or version-conflicting `PUT` records nothing.
- The invalidation survives a broker restart.
- AS-1.6's actual-prior-release matrix serves old/new traffic concurrently under fixed identities on the expanded schema, preserves evidence across prior-release writes, and drains all old work before identity changes resume. Migrations remain bounded/abort on contention while healthy traffic continues.

## 5. Manual smoke check (optional, local)

1. Start the broker with `examples/config/third-party-oauth2.yaml`, using a filesystem-bound service and the mock OAuth2 provider from `mocks/`.
2. Connect a session, then write a new client ID to the client-ID file. Request a token exchange. Expect `400 invalid_grant` and the `session.oauth2.client_identity_changed` log event with `channel=filesystem_observation`.
3. Restore the old client ID and repeat the request. Expect `invalid_grant` again (AS-1.5).
4. Rewrite only the secret file. Reconnect, then request a token. Expect success with no new event (AS-3.2).
