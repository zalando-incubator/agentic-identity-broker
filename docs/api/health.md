# Health API

The end-user server exposes an unauthenticated health endpoint for load balancers, Kubernetes
probes, and monitoring. The admin server's `/health` has the same lifecycle states and no
components.

The authoritative contract is `GET /health` in `api/enduser/openapi.yaml`.

## Endpoint

```
GET /health
```

No authentication. The response body is a `HealthResponse`.

| Status | Meaning |
|---|---|
| `200 OK` | The server is `healthy` and accepting requests. |
| `503 Service Unavailable` | The server is `starting`, `shutting_down`, or `unhealthy`. |

## Components

The optional `components` map reports the health of individual dependencies. Each value is
`healthy` or `degraded`. A component **never** changes the HTTP status or the top-level
`status`, so a degraded component does not fail a liveness or readiness probe. Alert on the
component value instead.

| Component | Present when | `degraded` means |
|---|---|---|
| `upstream_jwks` | OAuth2 mode is `proxy` or `hybrid` | The upstream key set that the broker republishes failed to refresh or became stale. |
| `trust_anchor_key_files` | At least one trust anchor uses `jwks_file`, in any OAuth2 mode | The latest re-read of at least one key-set file failed. |

`trust_anchor_key_files` covers every key-set file in the broker: the token-exchange client
assertion, impersonation trusted issuers, and JWT pre-authentication on both servers. Inline
`jwks` key sets never appear in it.

While a key-set file is degraded, the broker keeps verifying with the last valid key set until
that trust anchor's `jwks_max_refresh` has passed since the last successful read. After that,
verification for the trust anchor fails closed. The component returns to `healthy` after the
next valid re-read of every degraded file. See
[Trust-Anchor Key Sets](../configuration.md#trust-anchor-key-sets).

## Examples

A healthy server with every key-set file current:

```http
GET /health HTTP/1.1
Host: broker.example.com

HTTP/1.1 200 OK
Content-Type: application/json

{"status":"healthy","timestamp":"2026-10-16T10:00:00Z","uptime_seconds":4500,
 "components":{"trust_anchor_key_files":"healthy"}}
```

A key-set file that failed its latest re-read:

```http
GET /health HTTP/1.1
Host: broker.example.com

HTTP/1.1 200 OK
Content-Type: application/json

{"status":"healthy","timestamp":"2026-10-16T10:15:00Z","uptime_seconds":5400,
 "components":{"trust_anchor_key_files":"degraded"}}
```

## Operator Action for `degraded`

1. Find the `trust anchor key set re-read failed` log entry. Its `trust_anchor` and `path`
   fields name the affected trust anchor and file, and `error_kind` names the failure category.
   The log never contains file contents.
2. Fix the file, typically by correcting the Secret or ConfigMap that the deployment projects
   into `security.trust_material_dir`.
3. Expect a `trust anchor key set recovered` log entry and `"trust_anchor_key_files":"healthy"`
   after the next re-read, at most `jwks_min_refresh` later.
4. Act before the `fails_closed_at` time in the failure log. After it, credentials for that
   trust anchor are rejected until a valid re-read succeeds.
