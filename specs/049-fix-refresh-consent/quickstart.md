# Quickstart: Validate Consent-Bound Refresh Sessions

## Scope and status

This guide defines validation after implementation. The new journeys and migration do not exist merely because the plan exists. A focused run with zero executed scenarios is not a passing acceptance gate.

Contracts: [refresh grant](contracts/refresh.openapi.yaml), [lifecycle](contracts/lifecycle.md), [configuration](contracts/configuration.md), and [storage](contracts/storage.md). The [data model](data-model.md) defines deadline and recovery semantics.

## Prerequisites

- Go 1.27.1, `just`, Ginkgo, frontend build dependencies, and Playwright browsers.
- Docker or Podman for PostgreSQL acceptance, adapter, and migration tests.
- Helm for deployment configuration validation.
- Accept proposed [ADR 038](../../adrs/038-consent-bound-refresh-sessions.md) with its explicit supersession of ADR 008's encryption-subject restriction. Review canonical API and release changes before implementation.
- Supply a valid JWE key for the existing OAuth state flow and a working `EncryptionPort` backend for credentials and signing keys. Persisted retry results use `EncryptionPort`, not JWE. Durable replicas share PostgreSQL and can decrypt the same branch keys.
- Run commands from the repository root. Do not enable `GOEXPERIMENT=jsonv2` on Go 1.27.

## 1. Run the implementation gates

```bash
just check
just test
just test-integration
just test-integration-infra
just web-build
ginkgo -v --label-filter='!performance' --focus='Consent-Bound Refresh Sessions' ./tests/e2e/
ginkgo -v --tags=integration --label-filter='!performance' --focus='Consent-Bound Refresh Sessions' ./tests/e2e/
just test-e2e-frontend
```

The specification contains 47 acceptance scenarios: 9 for US1, 10 for US2, 11 for US3, 4 for US4, 5 for US5, and 8 for US6. Count unique identifiers across ordinary HTTP/CLI, tagged PostgreSQL, and browser runs. The tagged run also includes ordinary backend cases.

Map each scenario to one production-bootstrap journey in [plan.md](plan.md). US1-S2 uses the browser. US6-S8 recovers an original pair before memory restart rejection. Verify the documented storage limit separately. Do not assert documentation source text or count skipped cases.

Before runtime implementation, record acceptance-linked semantic failure for every primary scenario. Each primary It must compile and fail for its actual feature behavior. Keep unchanged baseline regressions separate from these 47 entries. Do not force failures with unrelated assertions or placeholders. After implementation, run all primary scenarios and existing regressions.

US2-S10 must recover the identical pair before replaying its authorization code. Then both refresh tokens must fail and an unrelated session must remain usable. The positive recovery control supplies its feature-specific semantic-red assertion. T050 completes this retry-dependent journey after US3 implementation.

Run T031's fresh capability/scope/promotion tests red before T032–T034. Run T043's retry and overlapping-failure tests red before T044–T047. With consent denied and capability removed, consent determines the error. With valid consent/lifetimes but capability removed, an older-token request returns the existing capability error without revocation. With capability allowed, prohibited reuse revokes before response-scope checks. A withdrawn scope on an otherwise eligible result rejects without mutation.

The infrastructure run must exercise migration apply/down/reapply, a rolling upgrade, two-instance behavior, restart, rollback, and one-connection-pool safety. Database faults must use real isolated schemas, not mock repository echoes.

Verify the updated Phase 2b Helm contract and canonical API guide before runtime work. Verify the rendered guide examples in docs/api/oauth2-refresh-sessions.md against final exercised outcomes.

## 2. Validate configuration delivery

After the three CLI flags and chart values exist, inspect the generated help and chart configuration:

```bash
./bin/agentic-identity-broker --help
helm template refresh-policy charts/agentic-identity-broker \
  --set broker.oauth2AuthorizationServer.mode=local \
  --set-string broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval=0s \
  --set-string broker.oauth2AuthorizationServer.local.absoluteSessionLifetime=168h \
  --set-string broker.oauth2AuthorizationServer.local.refreshTokenTtl=720h
```

Load the generated broker ConfigMap YAML with the production loader. Then exercise refresh with production bootstrap. This configuration enforces strict single use, a seven-day absolute limit, and a 30-day inactivity limit. Startup warns because the absolute limit is shorter than inactivity.

Exercise these source combinations:

| Input | Expected result |
|-------|-----------------|
| Omitted reuse interval in local/hybrid mode | 30-second bounded, idempotent retry |
| Explicit `0s` reuse through YAML, environment, CLI, or Helm | Strict single use |
| Higher-priority CLI or environment `0s` over lower-priority `30s` | Strict single use |
| Absolute `0s` | No absolute deadline, with consent and inactivity still enforced |
| Existing `refresh_token_ttl` omitted or `0s` | Existing `720h` inactivity default |
| Reuse at or above existing access `token_ttl` or inactivity `refresh_token_ttl` | Startup error naming the invalid policy |
| Finite absolute lifetime at or below reuse | Startup error naming the invalid policy |
| Finite absolute lifetime shorter than inactivity but longer than reuse | Startup warns. The earliest deadline still applies. |
| Malformed, bare numeric, negative, or overflowing duration | Startup error naming invalid configuration |
| Default proxy-mode chart deployment | No local-policy block that invalidates proxy startup |

Confirm CLI > environment > YAML > defaults with real startup and refresh outcomes. Observe startup warnings through the configured logger. Rendered text or copied configuration fields alone do not prove behavior.

## 3. Manual lost-response and consent smoke

Use an isolated local deployment. Do not reuse these development credentials in production. Disable shell tracing before setting or printing any token variables.

Keep generated keys stable for the duration of the exercise:

```bash
set +x
export IDENTITY_BROKER_JWE_SIGNING_KEY="$(openssl rand -base64 32)"
export IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY="$(openssl rand -base64 32)"
export IDENTITY_BROKER_SERVER_ENDUSER_PUBLIC_URL=http://127.0.0.1:8000
just build-all
./bin/agentic-identity-broker --config examples/config/oauth2-server-mode.yaml \
  --oauth2_authorization_server.local.refresh_token_reuse_interval=30s \
  --oauth2_authorization_server.local.absolute_session_lifetime=0s \
  --oauth2_authorization_server.local.refresh_token_ttl=720h
```

In another shell, use a registered local client and complete its authorization-code/PKCE consent flow with `offline_access`. Set `CLIENT_ID`, confidential `CLIENT_SECRET`, and `RT` from that real exchange. Public clients omit the secret field.

Keep token results in local shell variables, not logs or shared files. Do not echo tokens or enable `set -x`:

```bash
set +x
CLIENT_AUTH=(--data-urlencode "client_id=$CLIENT_ID")
if [ -n "${CLIENT_SECRET:-}" ]; then
  CLIENT_AUTH+=(--data-urlencode "client_secret=$CLIENT_SECRET")
fi
BROKER=http://127.0.0.1:8000
FIRST="$(curl --fail --silent --show-error "$BROKER/oauth2/token" \
  --data-urlencode grant_type=refresh_token \
  "${CLIENT_AUTH[@]}" \
  --data-urlencode "refresh_token=$RT")"
ACCESS="$(printf '%s' "$FIRST" | jq -er '.access_token | select(type == "string" and length > 0)')"
NEXT="$(printf '%s' "$FIRST" | jq -er '.refresh_token | select(type == "string" and length > 0)')"
RETRY="$(curl --fail --silent --show-error "$BROKER/oauth2/token" \
  --data-urlencode grant_type=refresh_token \
  "${CLIENT_AUTH[@]}" \
  --data-urlencode "refresh_token=$RT")"
test -n "$ACCESS" && test -n "$NEXT" && \
  test "$ACCESS" = "$(printf '%s' "$RETRY" | jq -er .access_token)" && \
  test "$NEXT" = "$(printf '%s' "$RETRY" | jq -er .refresh_token)"
```

Retry before 30 seconds elapse and before consuming NEXT. The token strings match. The second response's `expires_in` can decrease. It must not restart access-token validity.

Open the existing consent UI through the deployment's normal authentication flow. Revoke this agent and confirm the action. Then request renewal with NEXT:

```bash
curl --silent --show-error --include "$BROKER/oauth2/token" \
  --data-urlencode grant_type=refresh_token \
  "${CLIENT_AUTH[@]}" \
  --data-urlencode "refresh_token=$NEXT"
```

Expected result is HTTP 400 with `error: invalid_grant` and no access_token or refresh_token. Regranting the agent must not make RT or NEXT usable. Fresh authorization can create a new session.

The development pre-auth header is a deployment trust boundary, not a production authentication bypass. The browser test uses the real configured UI authentication and revoke action.

## 4. Prove lifetimes and direct-JWKS behavior

The future lifetime journeys use short configured durations or aged fixture state. They preserve the initial authorization journey and original clock metadata. PostgreSQL decisions use shared `clock_timestamp()` after the agent gate. Memory decisions use the coordinator's clock. Exercise before, at, and after each deadline without node-local time assumptions.

| Journey | Deciding observation |
|---------|----------------------|
| Grant reaches `ValidUntil` | Renewal at/after expiry returns `invalid_grant`, including previous-token recovery |
| Grant renewed after expiry | Earlier sessions remain unusable. New authorization can create a new root. |
| Active grant extended before expiry | Existing sessions continue with original clocks and current permissions |
| Absolute lifetime ends despite frequent fresh refresh | Renewal fails from the original issuance deadline |
| Inactivity lifetime ends without fresh refresh | Renewal fails from the last fresh activity deadline |
| Resource calls without refresh | They do not advance the inactivity deadline |
| Duplicate previous-token recovery | It does not advance inactivity, absolute, or reuse deadlines |
| Original cached access token expires during grace | No expired result returns. The otherwise valid current refresh token remains usable. |
| Restart with shorter lifetime | Persist the shorter deadline from original clocks, even before that deadline elapses |
| Increase inactivity on an active session | Current token expiry and interval stay unchanged; only a fresh valid rotation gives its successor the longer lifetime |
| Increase after a stored shorter deadline | The old session stays unusable even if the broker was stopped when that deadline elapsed |

Do not poll successful refresh to wait for inactivity expiry. Each successful current-token rotation is activity. Poll the clock or fixture state, then issue the deciding request once.

For the direct-JWKS journey, validate the original broker JWT's signature, issuer, subject, agent, and expiry through the real published JWKS. Revoke consent and observe that refresh supplies no new JWT. The original JWT retains its existing expiry contract. Do not claim immediate downstream invalidation.

## 5. Prove lifecycle, restart, and race consistency

Run the tagged PostgreSQL journeys against cloned databases and production end-user/admin servers:

- Delete a grant through both existing paths. Only that principal/agent's sessions stop.
- Delete an agent. Every session for its principals stops.
- Replace credentials through the existing generation action. Old sessions continue with the new secret. The replaced secret cannot authenticate. First credential creation makes existing public sessions require the new credential.
- Explicitly DELETE credentials. Old sessions remain unusable after public fallback or later credential creation.
- Race refresh with each lifecycle action. After lifecycle revocation succeeds, no affected successor remains usable. Credential replacement must not terminate a session.
- Reject revocation writes with a test-owned database failure. The lifecycle action fails and authorization is not partially deleted.
- Recover a lost response through a second instance and after rebuilding an application over the same database and credential-encryption key material.
- Apply an additive migration that retains `refresh_token_sessions`. The current legacy format cannot prove first issuance, so unsupported tokens require fresh authorization. An evidenced older family retains original clocks. Old-writer descendants undergo the same FR-025 lineage check during a rolling upgrade. Revocation also blocks legacy current rows.
- Let an old writer consume an anchored current token before the new instance handles that same token. Expect invalid_grant without another successor or request mutation from the new instance. Its unsupported descendant must also require reauthorization.
- Exercise the opposite lock order. After the new instance rotates, the old writer's conditional MarkUsed must fail. Observe one successor, not independent branches.

- Present an expired consumed ancestor while its current successor and authorization remain valid. The older signature must still revoke the live family.
- For US3-S11, interrupt the PostgreSQL COMMIT acknowledgement through a test-owned protocol fault. Observe the actual durable rotation or retry-count outcome independently. Require token-free server_error without a rollback claim. Restore connectivity and retry under the ordinary consent, lifetime, predecessor, and count rules. Exercise confirmed rollback, committed work, unavailable resolution, and zero reuse. A committed retry count is never refunded. Zero reuse never returns a committed pair.
- End a session through absolute or shortened inactivity expiry. Verify ExpiredAt/reason, ciphertext erasure, and legacy current/descendant invalidation commit together. A mirror-write fault must roll back the whole transition and block startup readiness.
- Before binary-only rollback, quiesce token traffic and old writers. Run outgoing-policy expiry reconciliation. If it fails, do not admit the old binary. Keep the new schema and attempt legacy renewal after rollback. Ended sessions must return no tokens while unrelated eligible sessions remain usable.

Use observable database-lock interleavings and bounded waits, not arbitrary sleeps. A one-connection-pool journey must cover signing preparation and branch-key access without nested pool reads inside the transaction.

Memory runs prove in-process outcomes and staged rollback. A memory process restart discards sessions and must reject old tokens. It does not prove durable recovery.

For backup recovery, restore an encrypted snapshot in an isolated environment. Verify that live consent, current revocation history, token ancestry, retry count, and shared-time deadlines still gate the result. A stale snapshot cannot revive a revoked or expired session. Do not place plaintext credentials in backups or token strings in logs.

## 6. Browser evidence and completion

Run the maintained screenshot path in serial mode when capturing review artifacts:

```bash
E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend
```

Use the existing screenshot workflow to retain `refresh_consent_before_revoke.png` and `refresh_consent_after_revoke.png` under `tests/e2e/screenshots/`.

Before delivery, run the full existing gate:

```bash
just verify
```

Record executed scenario IDs, command outcomes, migration results, and the manual token/consent observation. Report missing infrastructure instead of claiming its journeys passed. Do not include tokens or secrets in the report.
