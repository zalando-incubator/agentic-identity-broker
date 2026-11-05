# Quickstart: Validate Session Invalidation After Client-ID Changes

**Feature**: [spec.md](spec.md) | **Contracts**: [contracts/](contracts/) | **Data model**: [data-model.md](data-model.md)

This guide proves the feature end to end. It references the acceptance scenarios instead of restating them.

## Prerequisites

- `just install-tools` completed (Go, Ginkgo, Node, and Playwright browsers).
- Docker or Podman only for the PostgreSQL-backed suites (`just test-integration-infra`, `just test-e2e-docker`).
- Approved items API-001, API-002, ADR-001, and DEC-001 to DEC-004 (`plan.md` Approval Items).

## 1. Static checks and fast tests

```bash
just check
just test          # domain: revision recording, usability rule, enforcement points
just web-test      # sessions page and consent card states
```

Expected: everything passes. The domain tests include the AS-1.5 rollback and AS-1.6 legacy matrices against the in-memory store, plus FR-015's cross-product of identity validity, recorded refresh rejection, and access-token refreshability. Rejected sessions still block issuance with unexpired tokens; identity-valid expired tokens with stored, not-rejected refresh tokens remain refreshable.

## 2. Backend acceptance journeys (in-memory store)

```bash
ginkgo -v --focus "Client identity change" ./tests/e2e/
```

| Scenario group | Proves |
|---|---|
| AS-1.1, AS-1.7 | A filesystem publication and an Admin API `PUT` each turn an unexpired token into `400 invalid_grant` with ADR 044's `error_uri` for every user (Journey 1, Step 3). |
| AS-1.2–AS-1.6 | Per-user independence, refresh and sweep denial, in-flight callback rejection, rollback, and the legacy rollout variant. |
| AS-2.1–AS-2.5 | Automatic consent on the next agent authorization, consent-initiated and direct reconnection, failure and retry, and `requires_reauthentication`. |
| AS-3.1–AS-3.4 | Unrelated services and grants are untouched. Filesystem and Admin API secret rotations, metadata updates, and same-identity transitions invalidate nothing. AS-3.1 also retains ADR 046's rejection and refreshability outcomes for unchanged services and verifies `is_expired == requires_reauthentication`. |

Expected: one passing `It` per scenario, as mapped in `plan.md` Testing Strategy.

## 3. Browser journeys

```bash
just test-e2e-frontend
```

Expected: consent shows Reports Production as needing sign-in and Calendar as connected (State A). Reconnecting returns with selections preserved (State B). The sessions page shows "Sign-in required" (State C), then connected (State D). Screenshots are written to `tests/e2e/screenshots/`, as named in `plan.md`.

## 4. PostgreSQL persistence and migration (container suites)

```bash
just test-integration-infra
just test-e2e-docker
```

Expected:

- Migrations 047 and 048 apply, roll back, and reapply.
- DOWN 047 refuses to run after any revision increment and leaves the schema unchanged.
- Concurrent observations across two connections produce exactly one increment.
- A rejected or version-conflicting `PUT` records nothing.
- The invalidation survives a broker restart.

## 5. Manual smoke check (optional, local)

1. Start the broker with `examples/config/third-party-oauth2.yaml`, using a filesystem-bound service and the mock OAuth2 provider from `mocks/`.
2. Connect a session, then write a new client ID to the client-ID file. Request a token exchange. Expect `400 invalid_grant` and the `session.oauth2.client_identity_changed` log event with `channel=filesystem_observation`.
3. Restore the old client ID and repeat the request. Expect `invalid_grant` again (AS-1.5).
4. Rewrite only the secret file. Reconnect, then request a token. Expect success with no new event (AS-3.2).
