# Configuration and Deployment Contract

## Broker configuration

Use `internal/ports/config.go`, `internal/config/loader.go`, `internal/config/validator.go`, and the existing Cobra bindings.
Do not read environment variables ad hoc.
Do not add a configuration loader.

| YAML key | Default | Environment | CLI flag | Helm value |
|---|---|---|---|---|
| business_events.retention | `2160h` (90 days) | IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION | `--business_events.retention` | broker.businessEvents.retention |
| business_events.telemetry_copy_enabled | `true` | IDENTITY_BROKER_BUSINESS_EVENTS_TELEMETRY_COPY_ENABLED | `--business_events.telemetry_copy_enabled` | broker.businessEvents.telemetryCopyEnabled |

The existing loader applies this precedence: explicitly supplied CLI flag, environment, configuration file, default.

Bind all keys explicitly.
Add them to the loader key enumeration.
A false boolean in YAML/Helm must remain false.
Do not replace it with a templating `default true` expression.

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

Retention uses a standard Go duration that must be strictly positive.
Go duration syntax does not accept `90d`.
There is no minimum of a day or month.
There is no configuration to disable ledger persistence.

Use `2160h`.
Before storage/worker startup, reject malformed, zero, negative, and overflowed values.

PostgreSQL uses microsecond precision.
Round positive fractions upward to one microsecond.
Never truncate a positive value to zero.
Memory uses the same normalized duration.

Telemetry copying requires all of `telemetry.enabled`, `telemetry.logs.enabled`, and `business_events.telemetry_copy_enabled`.
Existing telemetry defaults and destination fields remain unchanged.
The new switch disables only the additional ledger records.
Global telemetry disablement does not disable recording or retention.

Temporary log exporter errors during initialization or delivery leave eligible copies pending.
These errors do not disable the configuration.

Configuration applies at startup, like existing broker configuration.
All replicas that share one ledger must use the same retention value.
The last successful validated startup policy write is authoritative.
Each policy write serializes with maintenance.

Change retention with a coordinated restart.
Do not run replicas with competing policies.
Document this rollout boundary.
Do not describe configuration for individual processes as independent retention policies.

## Helm contract

Add the `broker.businessEvents` values from the broker configuration table.
Use the chart's existing `--` documentation style and environment-variable references.
Render `business_events` into the broker ConfigMap.
Update the chart README and generated/example configuration.
Do not add ExtProc configuration.

Add two optional database-grant values for operational access:

| Helm value | Default | Effect |
|---|---|---|
| migration.grants.businessEvents.readerRole | `""` | When set, grants `SELECT` on the ledger parent tables to this existing role for operational investigation |
| migration.grants.businessEvents.erasureRole | `""` | When set, grants `EXECUTE` on `public.business_event_erase_subject(text)` to this existing role |

Only the PostgreSQL grants ConfigMap renders these values.
Empty values grant nothing.
With empty values, only the migration owner can investigate outside the broker or erase subjects.
The chart never creates database roles.
It never grants either capability to the broker user.

A custom `migration.grants.sql` replaces the default script.
The custom script must apply the ledger restrictions and role grants itself.
The chart README states this requirement.

For PostgreSQL, add a mandatory feature maintenance CronJob independent of telemetry enablement:

- Use schedule `*/5 * * * *` and `concurrencyPolicy: Forbid`.
- Reuse `migration.grants.image` for the PostgreSQL client. Reuse the migration service account, Secret names/keys, trusted database endpoint, and SSL configuration.
- Run `PGOPTIONS='-c lock_timeout=5s -c statement_timeout=30s' psql -v ON_ERROR_STOP=1 -c 'SELECT public.business_event_maintain_partitions();'`.
- Use the chart's established read-only filesystem/non-root/security context. Use its bounded Job resource/retry configuration.
- Do not render `spec.suspend`. Operator suspension then persists across `helm upgrade` during a policy change.
- Never mount the migration Secret in the broker Deployment. The broker role gets DML only.
- Add `SELECT public.business_event_provision_partitions();` to pre-install/pre-upgrade migration completion. This step creates current/future partitions before broker startup and never drops partitions. A migration job remains required even when optional SQL-grants initialization is disabled.
- After existing broad DML grants, revoke event UPDATE/DELETE and maintenance/provisioning/erasure function execution from the runtime role. Maintain parent/child ownership and immutable UPDATE protection for future partitions.

The maintenance function reads the database policy that validated broker configuration writes.
The Job receives database connectivity but no duplicate retention parser or telemetry switch.

The migration seeds a 90-day policy for first installation.
Broker startup applies changed retention.
Later maintenance uses that retention.
A deployment must not run a destructive old-policy sweep before it applies the changed policy.

A duration increase is the destructive direction.
A sweep under the old, shorter policy drops history that the new policy retains.
The pre-upgrade migration job only provisions partitions, so it cannot sweep.

For a policy change, suspend the CronJob.
Upgrade so every new broker replica applies the policy at startup.
Verify the stored `business_event_policy` value with migration credentials.
Then resume the CronJob.
These steps apply to a policy-change rollout, not normal retention maintenance.

For in-memory storage, the chart renders no CronJob.
The builder starts the logical retention worker.

For bare-process/container PostgreSQL deployments, install an equivalent five-minute scheduler with operations credentials.
Do not hide missing partition maintenance behind an indefinite DEFAULT partition.
When the current partition is absent, readiness fails.
After the scheduled maintenance succeeds, readiness recovers.

## Implementation documentation targets

Before feature completion, update these documentation targets:

- `docs/configuration.md`
- `examples/config/README.md`
- A runnable ledger configuration example under `examples/config/`
- The chart README/values/templates
- The database operations guide
- The `ARCHITECTURE.md` glossary/design.

Document these topics:

- Mandatory recording and existing failure responses when the ledger is unavailable.
- Exact subject-based erasure and absence of system-wide erasure for external telemetry/backups.
- Scheduler installation, partition health, credential isolation, and retention-configuration rollout.
- Event schema location, stable type names, recovery/duplicate semantics, and telemetry disablement precedence.

This contract adds no read/erase HTTP API, export API, external ingestion endpoint, or user interface.
Existing HTTP success and error representations remain unchanged.
Additional public contract changes discovered during implementation require separate stakeholder confirmation and OpenAPI updates.
