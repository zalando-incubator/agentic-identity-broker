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

Before readiness, reconcile active roots against stored deadlines and current policy under the agent guard. Check elapsed stored deadlines before applying any longer policy. At each decision, effective inactivity expiry is `min(InactivityExpiresAt, LastFreshAt + refresh_token_ttl, current token ExpiresAt)`. A shorter policy persists this effective deadline on active roots during startup or maintenance. A later increase cannot extend that activity interval or the already-issued refresh token.

A fresh successful rotation alone advances `LastFreshAt`. It sets the successor's `ExpiresAt` and new `InactivityExpiresAt` from rotation time plus the current configured inactivity lifetime. Retries, denied requests, and resource access do not advance these clocks. A nonterminal absolute policy can be recomputed from the original `StartedAt`, but only after checking stored deadlines for elapsed expiry.

Startup and maintenance commit `ExpiredAt`, its reason, and live retry-ciphertext erasure in one scoped transaction. That transaction also invalidates every matching unconsumed legacy current or descendant mirror. A failed mirror update rolls back the entire transition. Failed startup reconciliation blocks readiness. Rejected refresh requests do not perform this maintenance. A longer policy cannot restore a terminal session.

Run the same policy and access to the same `EncryptionPort` key material on each durable instance. Retry results use credential encryption at rest, not the JWE `TokenService`. Keep encryption keys for still-live retry results available, or fail closed if they cannot be decrypted. OAuth-state JWE configuration remains separate.

Keep the legacy `refresh_token_sessions` schema during rolling upgrade. Nullable `session_id` and token-ancestry columns anchor new rows to refresh-session roots. New issuers never write unanchored tokens, but old instances can still write legacy rows.

Before a fresh refresh trusts unused new-table state, lock and re-read the matching anchored legacy current row. Hold that lock through rotation. If old-writer consumption lacks matching new-token and history evidence, the original token returns token-free `invalid_grant`. Its unsupported descendant also returns that error. Do not create a successor or change request state. An old-first commit is visible on re-read. A new-first rotation consumes the mirror before an old writer can use it. Recheck old-writer descendants during revocation.

Apply FR-025 to unsupported lineage. The old format alone cannot prove first issuance, so the broker cannot invent its start or retry result. An independently evidenced older family retains its original clocks only with proven ownership, grant, first token, full ancestry, and last fresh rotation.

The retry deadline does not slide. A larger interval affects the next rotation. A smaller current interval can shorten an existing retry window. Neither a shorter interval nor a shorter session lifetime permits a response past another deadline.

Retain consumed-token history for the whole live session. After terminal revocation or expiry, retain its history and root terminal state through the maximum recorded token expiry (`RetainUntil`). Erase live retry ciphertext when its window closes, its successor rotates, or its session ends, whichever occurs first. A zero reuse interval stores no retry result. Cleanup must not erase replay evidence from a still-refreshable session.

## Documentation and operational limits

Update `docs/configuration.md`, local/hybrid examples, their README, deployment documentation, and the changelog.

Document these distinctions:

- The inactivity lifetime measures fresh successful refresh, not browser or resource access.
- With absolute `0s`, total elapsed time alone never forces the user back to the identity provider. Consent and inactivity checks still apply.
- Reuse `0s` enables strict single use. Otherwise, only the immediately previous token can return its original result, at most three times.
- A lost commit acknowledgement returns token-free server_error without a rollback guarantee. Later refresh uses ordinary durable-state and retry rules. At zero reuse, committed consumption requires fresh authorization. A committed retry authorization consumes a count even if its response or acknowledgement is lost.
- Consent and session revocation always override recovery. An expired cached access token also blocks recovery without revoking a valid current refresh token.
- Existing access JWTs keep their own expiry after revocation, up to the existing `token_ttl`. A shorter `token_ttl` reduces this exposure.
- Upgrade, rollback, or an old writer during rollout can require fresh authorization when FR-025 lineage evidence is absent. Do not claim all old binaries are unable to write during rollout.
- A binary-only rollback without a down migration cannot restore terminal lifetime-expired sessions. Quiesce token traffic and old writers, then complete outgoing-policy expiry reconciliation before old binaries receive traffic. If reconciliation fails, do not admit the old binary. Down migration and reapplication also keep terminal legacy mirrors unusable.
- Non-durable storage supports one instance and rejects every old refresh token after restart.

CAUTION: Encrypted backups can retain retry ciphertext beyond its live-store deletion deadline. Protect and limit backup access and retention. Do not put plaintext access tokens, refresh tokens, or client secrets in backups or logs. A restore must recheck current consent, revocation, ancestry, retry count, and shared-time deadlines before returning a stored result. A stale backup cannot revive a revoked or expired session. If current authorization history cannot be established, require fresh authorization. Key loss or unknown ciphertext fails closed without a fallback token result.
