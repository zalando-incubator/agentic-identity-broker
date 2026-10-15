# Changelog

## Unreleased — end-user API 2.0.0

### Third-party session validity

This preparation PR ships migration 038 and the rejection-aware broker, API, and SPA. Migrations 039 and 040 belong to following PRs and are not present. The memory adapter supports the complete rejection lifecycle. PostgreSQL at 38 supports ordinary traffic but refuses true rejection writes as `persistence_failed`. The durable PostgreSQL behavior described here activates only with PR2's migration 039.

- Required `requires_reauthentication` replaces `is_expired` as the API v2 field. The temporary compatibility default also emits `is_expired` with the same value. Endpoint paths stay unchanged.
- Third-party `refresh_token_expires_at` is removed. Provider token-set expiry describes the access token only.
- HTTP 4xx refresh `invalid_grant`, except 429, persists known-invalid refresh authority and immediately stops subsequent broker token issuance.
- Rejected records remain visible. Ciphertext and access expiry remain unchanged. Successful Login through the real OAuth2 callback clears rejection.
- Manual rejection returns HTTP 409 `refresh_rejected`. Missing credentials retain 409 `refresh_unavailable`; non-terminal upstream failures retain 502 `refresh_failed`.
- Token exchange retains HTTP 400 `invalid_grant` and the `/sessions` recovery URI.
- Session cards offer Login for reauthentication and Terminate for every record, without predicted refresh dates or permanent-validity claims.
- Consent active selectors and sweep candidates exclude recorded rejection. A newly observed sweep rejection fails once; subsequent sweeps exclude it.

### Online deployment and rollback

The stakeholder selected staged compatibility on 2026-10-14. This release defaults to migration target 38, with maximum 38. A normal Helm upgrade prepares the schema and rolls the prepared broker in one operation. Fresh installs run direct UP-all through 38. Phase 38 preserves legacy table reads/writes and adds the stable SQL boundary. Before merging or deploying PR2, complete the PR1 rollout and drain every unprepared replica and in-flight operation. PR2 adds migration 039 and activates durable rejection. Before merging or deploying PR3, upgrade the embedded SPA and all external session-API consumers. PR3 adds migration 040 after legacy schema dependencies end. Unchanged old binaries do not enforce the new rejection policy.

The chart exposes `migration.targetVersion` and `broker.thirdPartyOauth2.sessionAPICompatibility`. Explicit target 0 fails PostgreSQL upgrade rendering when migrations are enabled. Upgrade stages inspect version and dirty state. Dirty or unreadable metadata aborts. Equal/newer clean schemas are non-destructive no-ops. Only a lower clean version runs `goto`. Serialize migration runners. Helm stages never request DOWN or automate `Force`. Compatibility defaults to true in preparation and activation. It adds only `is_expired=requires_reauthentication`. Modern consumers can explicitly select false. Cleanup defaults to false after its consumer gate passes. Legacy storage compatibility never supplies provider-validity evidence or refresh dates.

Online rollout requires at least two healthy replicas, `maxUnavailable=0`, ready surge capacity, a disruption budget, and sufficient traffic-drain and termination time. Fail-fast DDL aborts on lock contention while compatible replicas continue to serve. Short metadata locks can affect latency. There is no absolute latency guarantee.

The existing hosting arrangement must retain immutable hashed SPA assets from both releases during rollout and for already-open clients. SQL safety and rolling limits alone do not prove browser continuity. If this prerequisite is absent, keep the existing release serving and do not begin rollout.

This release supports DOWN 038 after all preparation/new replicas drain. Later activation and cleanup releases add phase-compatible rollback. DOWN 040 restores only a nullable legacy column and retains rejection. DOWN 039 refuses while rejected rows remain. Resolve them through successful reauthentication or explicit termination. Inspect actual schema and dirty metadata before deliberate repair. Never clear rejection, delete rows automatically, or automate `Force`.

This change does not revoke already-issued provider tokens or invalidate ExtProc caches. Broker-issued refresh lifetimes and provider-enforced lifetimes remain unchanged.

See [ADR 040](../adrs/040-third-party-session-validity.md) and [the online rollout procedure](operations/session-sweep.md#online-session-validity-rollout-migrations-038040).
