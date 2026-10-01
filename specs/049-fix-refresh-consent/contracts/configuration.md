# Refresh Policy Configuration Contract

**Scope**: Local issuance in local and hybrid mode.  
**Configuration owner**: `internal/ports/config.go` and `internal/config/loader.go`.

## Parameters and delivery names

All YAML keys below have the prefix `oauth2_authorization_server.local.`. All Helm values have the prefix `broker.oauth2AuthorizationServer.local.`. Local and hybrid modes expose these three values through YAML, environment variables, CLI flags, and Helm. Proxy mode leaves local policy unused.

| Policy | YAML suffix | Default | Zero meaning | Helm suffix |
|--------|-------------|---------|--------------|-------------|
| Reuse interval | `refresh_token_reuse_interval` | `30s` | Disable retries of the consumed token | `refreshTokenReuseInterval` |
| Absolute lifetime | `absolute_session_lifetime` | `0s` | No absolute deadline | `absoluteSessionLifetime` |
| Inactivity lifetime | `refresh_token_ttl` | `720h` | Preserve existing 720h default | `refreshTokenTtl` |

| Parameter | Environment variable | CLI flag |
|-----------|----------------------|----------|
| Reuse | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL` | `--oauth2_authorization_server.local.refresh_token_reuse_interval` |
| Absolute | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME` | `--oauth2_authorization_server.local.absolute_session_lifetime` |
| Inactivity | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL` | `--oauth2_authorization_server.local.refresh_token_ttl` |

Existing precedence stays CLI flags, environment variables, YAML, then defaults. A higher-priority `0s` remains an explicit zero.

```yaml
oauth2_authorization_server:
  mode: local
  local:
    token_ttl: 1h
    refresh_token_reuse_interval: 30s
    absolute_session_lifetime: 0s
    refresh_token_ttl: 720h
```

A finite example replaces `absolute_session_lifetime` with `168h`. It does not rename `refresh_token_ttl` or add a second inactivity parameter.

## Source presence and validation

- `LocalModeConfig.RefreshTokenReuseInterval` preserves presence (`*time.Duration`) until local/hybrid resolution. Omission defaults to `30s`. Explicit `0s` disables retry.
- Resolved `LocalOAuth2Config` and issuer policy use concrete durations. Directly constructed configuration preserves the same omitted-versus-explicit-zero behavior.
- `AbsoluteSessionLifetime` is non-negative. Its `0s` means no absolute deadline, not an exemption from consent or inactivity checks.
- `RefreshTokenTTL` retains the existing omission/`0s` default of `720h`. Negative values fail startup.
- Duration text must parse as a Go duration. Bare numeric values, negative durations, overflow, and malformed strings fail startup with the invalid setting identified.
- The reuse interval must be shorter than both the existing access `token_ttl` and `refresh_token_ttl`. A finite absolute lifetime must be longer than the reuse interval. Invalid combinations fail startup.
- Startup warns, but does not fail, when a finite absolute lifetime is shorter than `refresh_token_ttl`.
- The earliest grant, access-token, session, or reuse deadline still applies. Configuration cannot extend an issued access token or restore a terminal session.
- Proxy mode does not receive local defaults and retains rejection of explicitly supplied local-only broker configuration.

Future configuration checks must exercise startup and refresh behavior for each delivery source, including precedence and explicit zero.

## Deployment chart

Update the Helm contract in Phase 2b before runtime implementation. Add all three values and documentation to:

- `charts/agentic-identity-broker/values.yaml`
- `charts/agentic-identity-broker/values.schema.json`
- `charts/agentic-identity-broker/templates/configmap.yaml`
- `charts/agentic-identity-broker/README.md`

The chart values use strings `30s`, `0s`, and `720h`. Quote them in rendered configuration. Render the local policy block only for local or hybrid mode. Local defaults must not make the default proxy deployment fail broker validation.

Helm schema rejects wrong types and obvious invalid durations. The broker's parser and validator remain authoritative for duration syntax and cross-setting limits. Helm's `--set-string` preserves `0s`.

Configuration-parity journeys must render the chart, load its generated broker YAML through the production loader, and exercise policy behavior. CLI, environment, and YAML journeys must exercise real startup and refresh decisions. Source-text assertions do not prove precedence or explicit-zero behavior.

T025 owns core YAML/environment decoding, presence, defaults, relation validation, warnings, and local/hybrid resolution. US6 verifies that implementation and adds CLI delivery and runtime chart parity. It does not add chart values or implement core policy again.

## Startup, restart, and policy change

Use shared PostgreSQL for durable recovery across instances and restart. PostgreSQL decisions use `clock_timestamp()` under the ambient transaction executor after the agent guard is acquired. Memory uses its coordinator's clock, supports one instance, and loses sessions on process restart.

Before readiness, reconcile active roots against stored deadlines and current policy under the agent guard. Check elapsed stored deadlines before applying any longer policy. At each decision, effective inactivity expiry is `min(InactivityExpiresAt, LastFreshAt + refresh_token_ttl, current token ExpiresAt)`. A shorter policy persists this effective deadline on active roots during startup or maintenance. A later increase cannot extend that activity interval or the already-issued refresh token. Startup also persists shorter effective `RetryExpiresAt` values and clears overdue ciphertext before readiness.

A fresh successful rotation alone advances `LastFreshAt`. It sets the successor's `ExpiresAt` and new `InactivityExpiresAt` from rotation time plus the current configured inactivity lifetime. Retries, denied requests, and resource access do not advance these clocks. A nonterminal absolute policy can be recomputed from the original `StartedAt`, but only after checking stored deadlines for elapsed expiry.

Startup and maintenance commit `ExpiredAt`, its reason, and live retry-ciphertext erasure in one scoped transaction. That transaction also invalidates every matching unconsumed legacy current or descendant mirror. A failed mirror update rolls back the entire transition. Failed startup reconciliation blocks readiness. At zero reuse, startup erases all cached retry results before readiness. Rejected refresh requests do not perform this maintenance. A longer policy cannot restore a terminal session.

Run the same policy and access to the same `EncryptionPort` key material on each durable instance. Retry results use credential encryption at rest, not the JWE `TokenService`. Keep encryption keys for still-live retry results available, or fail closed if they cannot be decrypted. OAuth-state JWE configuration remains separate.

Keep the legacy `refresh_token_sessions` schema during rolling upgrade. Nullable `session_id` and token-ancestry columns anchor new rows to refresh-session roots. New issuers never write unanchored tokens, but old instances can still write legacy rows.

Before a fresh refresh trusts unused new-table state, lock and re-read the matching anchored legacy current row. Hold that lock through rotation. If old-writer consumption lacks matching new-token and history evidence, the original token returns token-free `invalid_grant`. Its unsupported descendant also returns that error. Do not create a successor or change request state. An old-first commit is visible on re-read. A new-first rotation consumes the mirror before an old writer can use it. Recheck old-writer descendants during revocation.

Apply FR-025 to unsupported lineage. Only anchored root/token records written by new issuers provide complete evidence. This evidence covers principal, agent, client, original active grant ID, and first issuance time and token. It also covers complete ancestry, last fresh rotation, and revocation history. Every pre-feature unanchored row and unanchored old-writer descendant requires fresh authorization. Anchored new-model sessions can continue across rollout or restart only with complete evidence and current authorization.

The original `ReuseUntil` never slides. Initialize effective `RetryExpiresAt` to the earliest reuse, original access-token, or session deadline. For the same predecessor, `RetryExpiresAt` cannot increase. Persist the minimum of its stored value, first consumption plus the current reuse interval, original access expiry, and effective session deadlines. A smaller policy can shorten this value without re-encrypting an unchanged result. The payload's `retry_expires_at` stays the original `min(ReuseUntil, RetryAccessExpiresAt)`. Before a stored-result return, require that original bound to match the payload. Both original and effective deadlines must be strictly future. The original bound must not precede effective `RetryExpiresAt`.

Retain consumed-token history for the whole live session. After terminal revocation or expiry, retain its history and root terminal state through the maximum recorded token expiry (`RetainUntil`). Deny retry at or after effective `RetryExpiresAt`, regardless of cleanup. With reachable storage, maintenance erases overdue live ciphertext within 1 second after that deadline. A missed bound or unavailable cleanup storage blocks readiness until overdue ciphertext is cleared. Rotation, revocation, and terminal expiry erase ciphertext transactionally. A zero reuse interval stores no new result and erases all existing results before readiness. Cleanup must not erase replay evidence from a still-refreshable session.

For a full database or refresh-state restore, stop token traffic and all writers before restoration. Run `agentic-identity-broker --config <file> refresh-sessions invalidate-restored` against the supported schema before starting brokers. Only this command writes during invalidation. Admit traffic only after acknowledged success and a final scan with zero active roots, retry ciphertext, and unused legacy rows. If the command fails or its commit is indeterminate, keep brokers offline. Rerun the idempotent command until it succeeds. See [Restore invalidation](../data-model.md#restore-invalidation) for the full procedure.

## Documentation and operational limits

Update `docs/configuration.md`, local/hybrid examples, their README, deployment documentation, and the changelog.

Document these distinctions:

- The inactivity lifetime measures fresh successful refresh, not browser or resource access.
- With absolute `0s`, total elapsed time alone never forces the user back to the identity provider. Consent and inactivity checks still apply.
- Reuse `0s` enables strict single use. Otherwise, only the immediately previous token can return its original result, at most three times.
- A lost commit acknowledgement returns token-free server_error without a rollback guarantee. Later refresh uses ordinary durable-state and retry rules. At zero reuse, committed consumption requires fresh authorization. A committed retry authorization consumes a count even if its response or acknowledgement is lost.
- Consent and session revocation always override recovery. An expired cached access token also blocks recovery without revoking a valid current refresh token.
- Existing access JWTs keep their own expiry after revocation, up to the existing `token_ttl`. A shorter `token_ttl` reduces this exposure.
- Upgrade, rollback, or an old writer during rollout requires fresh authorization for pre-feature unanchored rows and unsupported descendants. Anchored new-model sessions continue only with complete FR-025 evidence and current authorization. Old binaries can write during rollout.
- A binary-only rollback without a down migration cannot restore terminal lifetime-expired sessions. Quiesce token traffic and old writers, then complete outgoing-policy expiry reconciliation before old binaries receive traffic. If reconciliation fails, do not admit the old binary. Down migration and reapplication also keep terminal legacy mirrors unusable.
- Non-durable storage supports one instance and rejects every old refresh token after restart.

CAUTION: Encrypted backups can retain retry ciphertext after live-store erasure. Protect and limit backup access and retention. Do not put plaintext access tokens, refresh tokens, or client secrets in backups or logs. After any full database or refresh-state restore, keep token traffic and all other writers stopped. Run the supported [restore invalidation procedure](../data-model.md#restore-invalidation) before admission. A snapshot alone cannot prove post-snapshot authorization or revocation history. Key loss or unknown ciphertext fails closed without a fallback token result.
