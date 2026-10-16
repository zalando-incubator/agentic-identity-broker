# Contract Delta: `GET /health` key-set file component

**Feature**: [../spec.md](../spec.md) | **Decision**: [../research.md](../research.md) R11

**Status**: Proposed. This is an additive change to `api/enduser/openapi.yaml`. Constitution
Principles IV and X require stakeholder confirmation before implementation.

## Change

`HealthResponse.components` gains one optional key.

| Component | Values | Present when | Effect on status |
|---|---|---|---|
| `upstream_jwks` (existing) | `healthy`, `degraded` | Proxy and hybrid modes | None |
| `trust_anchor_key_files` (new) | `healthy`, `degraded` | At least one trust anchor uses `jwks_file`, in any OAuth2 mode | None |

`trust_anchor_key_files` is `degraded` when any file-based key set's last re-read failed, including
after its staleness window expired. It returns to `healthy` after the next valid re-read of every
degraded file. Inline key sets never appear in or affect this component. The admin server's
`/health` is unchanged.

## OpenAPI edits (`api/enduser/openapi.yaml`, `/health`)

Replace the description paragraph that starts "In proxy and hybrid OAuth2 modes" with:

```text
The response may also include component-level health in `components`. Components report
`healthy` or `degraded` and never change the HTTP status or the top-level `status`.

- `upstream_jwks` (proxy and hybrid modes): freshness of the upstream key set that the broker
  republishes.
- `trust_anchor_key_files` (any mode, when a trust anchor uses `jwks_file`): `degraded` when the
  latest re-read of any key-set file failed. The broker keeps verifying with the last valid key set
  until that trust anchor's maximum refresh interval expires.
```

Add a `200` example:

```yaml
healthyWithDegradedKeySetFile:
  summary: Healthy server with a key-set file that failed its latest re-read
  value:
    status: healthy
    timestamp: "2026-10-16T10:15:00Z"
    uptime_seconds: 5400
    components:
      trust_anchor_key_files: degraded
```

No schema change is needed: `HealthResponse.components` is already a string map.

## Example exchange

```http
GET /health HTTP/1.1
Host: broker.example.com

HTTP/1.1 200 OK
Content-Type: application/json

{"status":"healthy","timestamp":"2026-10-16T10:15:00Z","uptime_seconds":5400,
 "components":{"trust_anchor_key_files":"degraded"}}
```
