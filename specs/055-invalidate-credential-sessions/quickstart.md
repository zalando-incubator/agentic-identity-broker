# Quickstart: Validating Credential-Change Session Invalidation

**Feature**: 055-invalidate-credential-sessions | **Plan**: [plan.md](plan.md)

This guide shows how to verify the feature after implementation. It refers to the [data model](data-model.md), the [contracts](contracts/), and the [ADR draft](adr-draft-credential-change-session-invalidation.md) instead of repeating them.

## Prerequisites

- Run `just install-tools`. It installs Go, ginkgo, Playwright browsers, and Wireloom.
- Run `just build-all`. Backend E2E suites build the SPA through `web-build`.
- Container-backed suites need Docker or Podman: `just test-integration-infra` and `just test-e2e-docker`. CI runs them on the pushed commit.

## 1. Static checks and fast tests

```bash
just check
just test
```

Expected: everything passes. The new unit tests cover these areas:

- Change detection in `internal/domain/thirdparty/`: unchanged values, metadata edits, secret-only and client-ID changes, transitions, and the decryption-failure rule
- `establishCurrency` in `internal/domain/oauth2session/`: revision-aware cache, conservative legacy initialization, higher-revision recognition, lost-CAS discard without reuse, stale/equal-mismatch source errors and no write on failure
- Callback fencing in `internal/domain/oauth2session/`: both stale-callback exchange timings, guarded reconnection, delayed old refresh acquisition after fresh login, every singleflight joiner's final guarded release ordering, and legacy zero claim
- Currency at `HandleAuthorization` admission in `internal/domain/oauth2/`
- Memory repository parity in `internal/adapters/storage/memory/`: atomic invalidation, compare-and-set, and cascade
- The fingerprinter adapter in `internal/adapters/credentialfile/`: coherent three-file publication/revision parsing, encoding, match/mismatch, concurrency bound and context-aware queued cancellation versus noninterruptible running Argon2

## 2. PostgreSQL repositories and migration (container-backed; validated by CI)

```bash
just test-integration-infra
```

Expected:

- Migration `046` (number provisional) applies, rolls back, and reapplies.
- The foreign key cascades on service deletion.
- CHECKs reject zero generation, nonpositive publication revision and non-Argon2id fingerprint. High-water marks persist across source transitions.
- `RecognizeChange` and `Update` with a `CredentialChange` invalidate exactly the service's sessions in one transaction.
- A concurrent `CreateIfAbsent` with a stale expected generation returns Conflict.
- A refresh holding service-share/session locks commits before invalidation and is then overwritten. An old acquisition admitted after rotation/new login is rejected before provider authentication.
- Final guarded release orders correctly with invalidation in both directions; preauthorized delivery is permitted, copied/shared results without a guard are not.

## 3. Backend acceptance journeys (User Story 1 and Scenario 2.4)

```bash
ginkgo -v --label-filter="credential-change-invalidation" ./tests/e2e/
```

Expected: one passing `It` per scenario 1.1–1.7 and 2.4 (see [plan.md](plan.md#scenario-mapping)). The setup follows spec Journey 1: Alice and Bob on GitHub Production, and Alice on Calendar.

| Check | Expected result |
|---|---|
| After a stored client-ID change (`PUT /api/services/{id}` with `report-client-v2`) | RFC 8693 for Alice returns `400 invalid_grant` with `error_uri=https://<public>/api/third-party/<github-uuid>/oauth2/authorize?redirect_uri=https%3A%2F%2F<public>%2Fsessions`, even though her access token expires at 10:00. Bob's refresh does not reach the provider. Calendar still returns a token. |
| `GET /api/third-party/sessions` as Alice | GitHub has `is_expired: true` and `has_refresh_token: false`. Calendar is unchanged. |
| Unchanged-value PUT, or a display-name PUT | Sessions stay usable. No `service.thirdparty.credentials_changed` event is emitted. |
| Rejected PUT (`412` stale If-Match) | Sessions stay usable. |
| Filesystem variants | Publish a coherent revision/pair with `helpers.PublishCredentialPublication`, advancing 41 to 42. The next token request by Alice is rejected without a provider call. Bob's request through a second broker sharing the storage is also rejected. The service `ETag` is unchanged. |
| Filesystem outage | The token request returns `500 server_error` without `error_uri`. Sessions stay usable after the files return unchanged. |
| Callback paused before exchange versus inside exchange | Both return `/sessions?error=callback_failed`. Before admission: zero provider calls. During admitted exchange: call allowed, stale persistence rejected. |
| Stale replica or delayed A/41 observation after B/42 | Source/state failure without token or state/session mutation; fresh B connection remains usable on a current replica. Intentional A restoration requires revision 43. |
| Legacy service with A sessions and no evidence first observes B | All sessions expire atomically; no unexpired A token is authorized. Empty service records evidence without generation increment. |
| Final release/invalidation race and singleflight joiners | Commit first rejects old release; decision first permits only the preauthorized snapshot's delayed delivery. Every joiner independently checks latest state/source/revision/session. |

The amended Feature 051 scenarios must pass under their existing label:

```bash
ginkgo -v --label-filter="oauth2-secret-file-overrides" ./tests/e2e/
```

## 4. Browser journeys (User Story 2)

```bash
E2E_CAPTURE_SCREENSHOTS=true just test-e2e-frontend
```

Expected, with screenshots in `tests/e2e/screenshots/`:

- `credential_change_consent_reconnect_required.png` (State A). The agent authorization flow with an active grant lands on `/agents/<agent>?session_token=...`. GitHub Production appears under **Connect Your Accounts** with **Login**, and **Save** is disabled.
- `credential_change_consent_reconnected.png` (State B). After the mock provider login, GitHub Production is connected and **Save** is enabled. **Save** resumes the original authorization, and the grant's permissions and expiry are unchanged.
- `credential_change_sessions_cancelled.png` (State C). After the user cancels at the provider, `/sessions` shows the access-denied alert and the GitHub Production card with **Expired**, "Log in to reconnect", and **Login**.

## 5. Full gate

```bash
just verify
```

Run every recipe that needs no container. CI covers `just test-integration-infra` and `just test-e2e-docker`.

## 6. Cutover and restoration

Follow [contracts/configuration.md](contracts/configuration.md): apply schema, publish revision material, exclude old replicas from filesystem traffic/background/sweeps and replace them before resuming. Unknown legacy sessions need one fresh login on first observation. A cold stale mount is independently rejected by the durable revision high-water mark. Preserve that mark through source transitions; returning to filesystem needs a higher revision. Before DOWN convert filesystem services to stored and drain contexts for 10 minutes under ADR 041 rollback prerequisites. Never reset live publication evidence.

## 7. Manual operator check (optional, local)

1. Start the broker with `examples/config/` filesystem credentials for one service.
2. Publish client ID, secret and revision 41 coherently. Connect a user, then publish a fresh immutable generation with the new secret and revision 42. Supply the required credential_revision_file binding; never independently overwrite files or reuse revision 41.
3. Call the token exchange. Expect the recoverable `invalid_grant`, one `service.thirdparty.credentials_changed` log record with `trigger=filesystem_recognized`, and no secret or path in any log line.
