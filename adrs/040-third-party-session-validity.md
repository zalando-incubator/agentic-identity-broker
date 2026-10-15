# ADR 040: Observed Third-Party Session Validity

**Status**: Accepted
**Confirmation**: The stakeholder approved the complete validity plan with “Plan approved.” On 2026-10-14, the stakeholder selected “Allow staged compatibility” in this conversation. This revision of the unmerged ADR replaces only its deployment and compatibility restrictions. API v2, immediate refusal after committed rejection, and the security policy remain approved.
**Release sequence**: This unmerged revision separates deployment into preparation, activation, and cleanup PRs. PR1 ships only migration 038 and prepared application code. PR2 supplies migration 039. PR3 supplies migration 040. Neither later migration is present in PR1.
**Supersedes**: Only ADR 038's availability consequence for terminal provider refresh rejection.

## Context

A provider token-set expiry describes its access token. The broker cannot predict the lifetime of an opaque third-party refresh token. Previously manufactured refresh-expiry dates and session-expired presentation misrepresented provider evidence. A terminal refresh rejection must stop subsequent broker issuance even while the stored access token remains unexpired.

## Decision

- Remove third-party refresh-expiry timestamps from current models, queries, APIs, and UI. Retain access-token expiry as an independent fact; a missing expiry means unknown, not permanent validity.
- Persist `refresh_token_rejected` only for HTTP 4xx OAuth `invalid_grant`, excluding 429. False means no observed terminal rejection, not proven validity. Retain the record, ciphertext, token type, scope, initiation time, and access expiry.
- Commit rejection through the existing locked update-only callback by returning `(true, nil)`, then return the captured credential-free provider error. Persistence failure is `persistence_failed`, not durable rejection or refresh success. Error-returning callbacks remain non-persistent.
- A rejected session cannot refresh or issue another broker token. Background and sweep work on an already-rejected row is an unchanged no-op; listing excludes it from sweep candidates and active consent selectors. Transient background failure and dropped work still permit an otherwise valid access token.
- `requires_reauthentication` is true after recorded provider rejection, or access expiry without stored refresh authority. Expired access with stored, not-known-rejected refresh credentials permits a refresh attempt. `has_refresh_token` means ciphertext presence even after rejection. `access_token_expired` remains independent.
- End-user OpenAPI version is `2.0.0`; endpoint paths remain unchanged. Replace `is_expired` with required `requires_reauthentication`; remove `refresh_token_expires_at`. Manual refresh returns 409 `refresh_rejected` with message `refresh token rejected by provider; reauthentication required`. Absent credentials retain 409 `refresh_unavailable`; non-terminal upstream failures retain 502 `refresh_failed`. Exchange retains 400 `invalid_grant` and the `/sessions` recovery URI.
- Successful authorization/callback upsert clears rejection and preserves existing identity/lifecycle semantics. Explicit termination deletes the retained record. The sessions UI offers Login for reauthentication and Terminate for every record, without predicted refresh dates or permanent-validity claims.
- Use online migrations 038 prepare, 039 expand, and 040 contract. Keep historical migrations and migration 037's access-expiry index unchanged. The temporary legacy column serves storage compatibility only. Its dates never indicate provider validity or rejection.

## Deployment and rollback

The PostgreSQL boundary is `user_session_state`, an explicit 13-field read view, plus two stable write functions. The view exposes `id`, `principal`, `service_id`, `encrypted_access_token`, `encrypted_refresh_token`, `token_type`, `access_token_expires_at`, `refresh_token_rejected`, `scope`, `encryption_context`, `initiated_at`, `created_at`, and `updated_at`. It never exposes legacy refresh expiry.

`upsert_user_session_state` takes those 13 typed arguments and preserves the existing conflict identity and lifecycle rules. `update_user_session_state` takes session ID, both ciphertexts, access expiry, rejection, and update time. It updates an existing row only. Both functions use `SECURITY INVOKER`. The PostgreSQL 15 view uses `security_invoker=true`. Locked reads through the view lock the underlying session row until the transaction ends. Runtime requests need no schema detection, JSON conversion, SQL retry, or fabricated expiry.

Upserts acquire the stable view's `ACCESS SHARE` lock inside their write transaction before PostgreSQL selects the phase-specific function body. The lock stays held through commit. This prevents a waiting callback from resuming a stale function after schema rollback removes its referenced column.

| Phase | Schema and runtime contract |
|---|---|
| PR1: 038 prepare (this release) | Keep the original table shape unchanged for legacy strict `SELECT *` readers and writes. Add the view and stable functions. The view reports rejection=false. True writes fail as `persistence_failed`, not durable rejection. Ordinary acquisition, refresh, and authorization work normally. The memory adapter already supports rejection. |
| PR1: prepared broker rollout | Roll prepared brokers across every replica at schema 38. They use the stable SQL boundary and contain rejection-aware logic. Unprepared binaries can coexist only before 39. They do not enforce the new rejection policy. |
| PR2: 039 activate (following PR) | Atomically add `refresh_token_rejected BOOLEAN NOT NULL DEFAULT FALSE`. Replace the view and functions with real rejection reads and writes. Keep the legacy column. Durable rejection activates without restart. Complete PR1 and drain every unprepared replica and in-flight operation before merging or deploying PR2. |
| PR3: 040 cleanup (following PR) | Remove only the unused legacy column after all legacy schema dependencies end. Upgrade the SPA and every external session-API consumer before merging or deploying PR3. Prepared and newer brokers keep the same SQL boundary. |

The separate migration runner remains the schema authority. This preparation release defaults and limits Helm `migration.targetVersion` to 38. A normal upgrade uses the guarded forward wrapper before rolling prepared brokers. Fresh installs run direct `up` through all available migrations, including with the default target. PostgreSQL upgrade rendering refuses explicit target 0. Positive upgrade targets inspect current version and dirty state. Dirty or unreadable metadata aborts without `Force`. An equal/newer clean schema is a non-destructive no-op. Only a lower clean version runs `goto`. Serialize database migration runners across deployments. Explicit DOWN remains a separate operator action, never a Helm stage side effect. Preparation and activation default `third_party_oauth2.session_api_compatibility` to true. This switch adds only `is_expired=requires_reauthentication`, without refresh-date aliases. Modern consumers can explicitly select false. Cleanup defaults compatibility to false after the SPA and external-consumer gate passes.

Online rollout requires at least two healthy replicas, ready surge capacity, `maxUnavailable=0`, and `maxSurge` of at least one. An HPA must retain at least two replicas. A disruption budget protects the required capacity during voluntary evictions. Readiness, load-balancer draining, and a termination grace longer than broker shutdown are prerequisites. The chart preserves operator strategy overrides. Unsafe overrides do not satisfy these prerequisites.

The existing hosting arrangement must retain immutable hashed SPA assets from running and replacement releases during rollout and for already-open clients. A shared origin, retained bundles, or verified routing can satisfy this prerequisite. SQL safety and rolling limits alone do not prove deployment-wide continuity. If asset continuity is absent, the rollout must not begin. The existing release keeps serving. This decision requires no new deployment stack.

Each schema step uses one atomic procedural statement with fail-fast locks. Expansion and contraction acquire `NOWAIT` locks on the view, then the table. Busy locks abort without schema or row changes while existing replicas continue to serve. Short metadata locks can affect latency. This design does not promise an absolute latency bound. A failed migration can dirty metadata. Inspect the actual schema and migration version before deliberate metadata repair or retry. Never run `Force` automatically.

Rollback depends on both the schema phase and the running binary:

PR1 supplies DOWN 038 only. DOWN 039 and DOWN 040 ship with their following PRs.

- DOWN 040 restores a nullable legacy column only. It retains every rejection flag and fabricates no dates. Preparation and newer brokers can continue to serve.
- DOWN 039 refuses while rejected rows remain. Resolve each row through successful reauthentication or explicit user termination. Never clear rejection or automatically delete rows to bypass the guard. Successful DOWN restores preparation view/functions and removes the flag atomically. Keep preparation brokers until schema 38 permits a rolling deployment of an old binary.
- DOWN 038 removes the stable view/functions. Drain every preparation or newer replica first. Matching old binaries can continue to serve against the version-37 table.

Helm image rollback alone does not establish schema compatibility. After expansion, automatic rollback to an unprepared binary is unsafe. A refused DOWN preserves the schema and rows but can dirty metadata. `Force` changes metadata only, not rejection state or schema. See the [runbook](../docs/operations/session-sweep.md#online-session-validity-rollout-migrations-038040) for the phase procedure.

## Consequences

This stops future broker issuance after rejection commits. It does not revoke already-issued access tokens or invalidate ExtProc caches. Existing recipients can retain tokens until access expiry. Users receive actionable reauthentication status. Operators retain rows until recovery or explicit termination. The approved staged rollout preserves availability with compatible binaries, temporary API compatibility, and fail-fast DDL. It does not claim that unchanged old binaries enforce rejection.

## References

- `api/enduser/openapi.yaml`
- `adrs/038-proactive-token-refresh.md`
- `specs/008-thirdparty-oauth2-sessions/spec.md`
- `specs/029-proactive-token-refresh/spec.md`
