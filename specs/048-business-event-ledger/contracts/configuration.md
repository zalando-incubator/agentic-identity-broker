# Configuration and Deployment Contract

## Broker settings

Use `internal/ports/config.go`, `internal/config/loader.go`, `internal/config/validator.go`, and existing Cobra bindings. No ad-hoc environment reads and no new configuration loader.

| YAML key | Default | Environment | CLI flag | Helm value |
|---|---|---|---|---|
| business_events.retention | `2160h` (90 days) | IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION | `--business_events.retention` | broker.businessEvents.retention |
| business_events.telemetry_copy_enabled | `true` | IDENTITY_BROKER_BUSINESS_EVENTS_TELEMETRY_COPY_ENABLED | `--business_events.telemetry_copy_enabled` | broker.businessEvents.telemetryCopyEnabled |

Precedence follows the existing loader: explicitly supplied CLI flag, environment, configuration file, default. Bind all keys explicitly and add them to loader key enumeration. A false boolean in YAML/Helm must remain false, not be replaced by a templating `default true` expression.

```yaml
business_events:
  retention: 2160h
  telemetry_copy_enabled: true
```

Shorter retention and disabled additional copies:

```yaml
business_events:
  retention: 720h
  telemetry_copy_enabled: false
```

Retention is a standard Go duration, strictly positive. `90d` is not accepted Go duration syntax; use `2160h`. Reject malformed, zero, negative, overflowed values before storage/worker startup. PostgreSQL precision is microseconds: round positive fractions upward to one microsecond, never truncate a positive value to zero. Memory uses the same normalized duration. No minimum of a day or month is imposed. No ledger-persistence disable setting exists.

Effective telemetry copying requires all of `telemetry.enabled`, `telemetry.logs.enabled`, and `business_events.telemetry_copy_enabled`. Existing telemetry defaults and destination fields remain unchanged. Disabling the new switch suppresses only the additional ledger records. Disabled global telemetry is respected; it does not disable recording or retention. Temporary log exporter initialization/delivery errors leave eligible copies pending rather than converting configuration to disabled.

Configuration is startup-scoped, like existing broker configuration. All replicas sharing one ledger must use the same retention value. Change retention with a coordinated restart rather than running replicas with competing policies; the last successful validated startup policy write is authoritative, and each policy write serializes with maintenance. Document this rollout boundary rather than presenting per-process settings as independent retention policies.

## Helm contract

Add the `broker.businessEvents` values above, with the chart's existing `--` documentation style and environment-variable references. Render `business_events` into the broker ConfigMap. Update chart README and generated/example configuration. No ExtProc configuration is added.

Add two optional database-grant values for operational access, rendered only by the PostgreSQL grants ConfigMap:

| Helm value | Default | Effect |
|---|---|---|
| migration.grants.businessEvents.readerRole | `""` | When set, grant `SELECT` on the ledger parent tables to this existing role for operational investigation |
| migration.grants.businessEvents.erasureRole | `""` | When set, grant `EXECUTE` on `public.business_event_erase_subject(text)` to this existing role |

Empty values grant nothing, leaving investigation outside the broker and erasure to the migration owner. The chart never creates database roles and never grants either capability to the broker user. A custom `migration.grants.sql` replaces the default script, so it must apply the ledger restrictions and any role grants itself; the chart README states this.

For PostgreSQL, add a mandatory feature maintenance CronJob independent of telemetry enablement:

- Schedule `*/5 * * * *`; `concurrencyPolicy: Forbid`.
- Reuse `migration.grants.image` for the PostgreSQL client, the migration service account, Secret names/keys, trusted database endpoint and SSL settings.
- Execute `psql -v ON_ERROR_STOP=1 -c 'SELECT public.business_event_maintain_partitions();'`.
- Use the chart's established read-only filesystem/non-root/security context and bounded Job resource/retry settings.
- Do not render `spec.suspend`, so an operator's suspension of the CronJob persists across `helm upgrade` during a policy change.
- Never mount the migration Secret in the broker Deployment. The broker role gets DML only.
- Add `SELECT public.business_event_provision_partitions();` to pre-install/pre-upgrade migration completion so the current/future partitions exist before the broker starts. This step never drops partitions. A migration job remains required even when optional SQL-grants initialization is disabled.
- Revoke event UPDATE/DELETE and maintenance/provisioning/erasure function execution from the runtime role after existing broad DML grants. Maintain parent/child ownership and immutable UPDATE protection for future partitions.

The maintenance function reads the database policy written from validated broker config. The Job therefore receives database connectivity but no duplicate retention parser or telemetry switch. The migration seeds a 90-day policy for first installation; changed retention is applied by broker startup and respected by subsequent maintenance. Deployment of a changed policy must not run a destructive old-policy sweep before that policy is applied. A duration increase is the destructive direction: a sweep under the old, shorter policy drops history the new policy retains. The pre-upgrade migration job only provisions partitions, so it cannot sweep. For the CronJob: suspend it, upgrade so every new broker replica applies the policy at startup, confirm the stored `business_event_policy` value with migration credentials, then resume it. This is a policy-change rollout step, not normal retention maintenance.

For in-memory storage no CronJob is rendered; the builder starts the logical retention worker. For bare-process/container PostgreSQL deployments install an equivalent five-minute scheduler using operations credentials. Missing partition maintenance cannot be hidden behind an indefinite DEFAULT partition. Readiness fails when the current partition is absent and recovers after the scheduled catch-up succeeds.

## Implementation documentation targets

Before feature completion update `docs/configuration.md`, `examples/config/README.md`, a runnable ledger configuration example under `examples/config/`, chart README/values/templates, the database operations guide, and `ARCHITECTURE.md` glossary/design. Document:

- Mandatory recording and existing failure responses when the ledger is unavailable.
- Exact subject-based erasure and absence of system-wide erasure for external telemetry/backups.
- Scheduler installation, partition health, credential isolation, and retention-setting rollout.
- Event schema location, stable type names, recovery/duplicate semantics, and telemetry disablement precedence.

No new read/erase HTTP API, export API, external ingestion endpoint, or user interface is part of this contract. Existing HTTP success and error representations remain unchanged; any additional public contract change discovered during implementation requires separate stakeholder confirmation and OpenAPI updates.
