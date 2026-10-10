# Breaking Changes

## Unreleased: Admin API 2.0.0 credential sources

**Design approved**: 2026-10-08. The user selected “Accept ADR 038” and “Major contract bump to 2.0.0”.

This entry records the implemented, unreleased source-aware contract. It does not announce a published broker release. Local verification does not establish registry publication or live deployment. The full final gate remains pending.

### Released baseline and version strategy

Published broker release [`v0.1.81`](https://github.com/zalando-incubator/agentic-identity-broker/releases/tag/v0.1.81) contains Admin API `1.0.0`. That contract requires `Service.client_id` in every service response.

Admin API `2.0.0` replaces that contract on the existing `/api/services` and `/api/services/{service-id}` routes. There is no compatibility endpoint, versioned route, shim, or alias. The OpenAPI contract version does not select a broker release tag. A client cannot negotiate the old service representation after the cutover.

The authoritative schema is [`api/admin/openapi.yaml`](../api/admin/openapi.yaml). The approval record is [ADR 038](../adrs/038-oauth2-client-secret-file-overrides.md).

### Breaking response changes

List, get, create, and update responses require `credential_source`, with exactly `stored` or `filesystem`.

| Source and authentication | `client_id` | `client_secret` |
|---|---|---|
| Stored shared-secret service, including Google | Always present, with existing stored/derived value | Existing `REDACTED` value |
| Stored public or CIMD confidential service | Always present, with existing stored/generated value | Absent |
| Filesystem service | Absent, not null or a placeholder | Absent, not null or `REDACTED` |

A filesystem service response no longer satisfies the released required-client-ID schema. Making that field universally required, nullable, or empty does not implement the new contract. Consumers must use the source-specific response variant.

Administrative metadata never resolves files. Responses contain no paths, file contents, or internal source-transition marker. Missing files do not make service metadata unavailable.

### Request and transition contract

Creation omission selects `stored`. Update omission preserves the current source. Explicit null and unknown source values are invalid.

Filesystem creation requires a valid canonical ID. Filesystem updates must retain a valid resulting canonical ID. Omission preserves the current ID, but null cannot clear it.

Filesystem requests reject either inline credential field whenever present. This includes null and empty strings. Public, CIMD confidential, and Google-flavor services cannot select filesystem mode. File paths belong only to operator configuration, not administrative requests.

An explicit stored-to-filesystem transition atomically removes both stored credentials. It also records irreversible internal transition evidence. The reverse requires explicitly supplied non-empty `client_id` and `client_secret`. The broker encrypts the supplied secret and never imports file contents. Rejected input or a version conflict preserves the previous state.

A source transition does not rewrite established authorization or session identity. A different identity requires a new connection. Missing legacy identity requires a new connection after a source transition, including a return to stored.

### Consumer migration

1. Regenerate or update administrative client models against Admin API `2.0.0`.
2. Branch on `credential_source` before reading inline credential properties.
3. For filesystem mode, remove both inline fields from create and PUT bodies, including null-valued serializer defaults.
4. Supply a valid canonical ID when you create a filesystem service.
5. Keep omitted-source PUT bodies consistent with the existing source.
6. For a reverse transition, send both explicit non-empty credentials instead of copying redacted output or reading files.
7. Retain full PUT bodies and existing ETag requirements for protected-resource replacement.

Repository admin DTOs/handlers, administrative integration/E2E callers, and the operator service-management guide use the source-aware contract. External administrative consumers need the same migration before deployment of the cutover.

### Unchanged contracts

Stored credential validation and applicable redaction remain unchanged. Public and CIMD secretless behavior remain unchanged. PUT remains a full replacement with existing documented omission exceptions.

Existing error envelopes, status codes, ETags, protected-resource preconditions, and endpoints remain unchanged. Other administrative schemas and the end-user API remain unchanged. No compatibility behavior preserves the obsolete service schema.

### Configuration, rotation, and persistence

Operator configuration supplies `third_party_oauth2.credential_files`. Each exact canonical-ID key maps to absolute `client_id_file` and `client_secret_file` paths. Environment and CLI inputs use strict JSON. Higher-precedence input replaces the whole mapping. An empty mapping does not change a filesystem service to stored mode.

Filesystem initiation reads only the current client ID. Each code-exchange or actual refresh attempt acquires one coherent current pair. Publish complete pairs through fresh immutable targets. Same-identity secret rotation requires no restart or service update. A client-ID change requires new connections for mismatched codes and sessions. Provider validity overlap remains an operator responsibility.

Missing bindings, unusable required files, changed targets during acquisition, and missing or mismatched required identity fail closed before provider authentication. No stored or last-known fallback applies. Source errors retain existing generic end-user responses and credential-free operational events.

Source failures use typed diagnostic detail `credential_source_unavailable` and outcome `infrastructure_error`, without a recovery URI. Outbound validation preserves shared provider state during concurrent refresh. Provider rejection remains distinct from source failure and locally recorded token expiry.

Existing migration 036 backfills services to stored mode without rewriting credentials or tokens. Both stores preserve nullable session identity and irreversible source-transition evidence. Guarded DOWN refuses filesystem services, true transition markers, and other incompatible records. Compatible DOWN discards non-secret session identity associations.

### Delivery status

All 50 acceptance cases passed in individual story runs. Selected unit/integration packages and 30 legacy public/CIMD journeys passed. These results do not claim a completed full final gate.

External test, prod, and sandbox manifests regenerated from the current local chart, including uncommitted feature changes. The exact pair and read-only credential/configuration/tmp mounts passed validation. `scripts/chart_ref` remains `ad2289121b459ea14d56efd7d227bbf71d452948`, a historical chart path revision, not a full snapshot of that working chart.

The local linux/arm64 image `agentic-identity-broker:oauth2-credential-files-051` was built and verified with UID 1000. Its local image ID is `sha256:f128f4a9237e272b32773d87682fd8ca06b6aea2247a9b984c9634b95d9a7ab0`. CDP `DEP_BROKER_VERSION` placeholders remain. No registry publication or live deployment occurred.
