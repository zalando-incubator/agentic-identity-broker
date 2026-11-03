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
- `establishCurrency` in `internal/domain/oauth2session/`: cache hit, baseline, recognition, lost compare-and-set, source errors, and no write on failure
- Callback fencing in `internal/domain/oauth2session/`: stale sealed generation, stale reconnection, and a legacy zero claim
- Currency at `HandleAuthorization` admission in `internal/domain/oauth2/`
- Memory repository parity in `internal/adapters/storage/memory/`: atomic invalidation, compare-and-set, and cascade
- The fingerprinter adapter in `internal/adapters/credentialfile/`: encoding, mismatch reported as `false`, and concurrency bound

## 2. PostgreSQL repositories and migration (container-backed; validated by CI)

```bash
just test-integration-infra
```

Expected:

- Migration `046` (number provisional) applies, rolls back, and reapplies.
- The foreign key cascades on service deletion.
- The CHECKs reject a zero generation and a non-Argon2id fingerprint.
- `RecognizeChange` and `Update` with a `CredentialChange` invalidate exactly the service's sessions in one transaction.
- A concurrent `CreateIfAbsent` with a stale expected generation returns Conflict.
- A refresh holding the session lock commits before the invalidation and is then overwritten.

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
| Rejected PUT (`409` stale version) | Sessions stay usable. |
| Filesystem variants | Publish a new pair with `helpers.PublishCredentialPair`. The next token request by Alice is rejected without a provider call. Bob's request through a second broker sharing the storage is also rejected. The service `ETag` is unchanged. |
| Filesystem outage | The token request returns `500 server_error` without `error_uri`. Sessions stay usable after the files return unchanged. |
| Paused callback across a change | `/sessions?error=callback_failed`. No provider token request is made. |

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

## 6. Manual operator check (optional, local)

1. Start the broker with `examples/config/` filesystem credentials for one service.
2. Connect a user, then replace the bound secret file with a new value by publishing a fresh symlink generation.
3. Call the token exchange. Expect the recoverable `invalid_grant`, one `service.thirdparty.credentials_changed` log record with `trigger=filesystem_recognized`, and no secret or path in any log line.
