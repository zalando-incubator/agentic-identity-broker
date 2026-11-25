---
title: "Deployment"
description: Living specification of how a platform operator runs the broker in a container and on Kubernetes. Every example on this page is checked against tests.
---

# Deployment

A platform operator runs the broker as one process in a container, usually on Kubernetes. This
page describes what that process listens on, how it starts and stops, how an orchestrator learns
that it is alive, and what the Helm chart puts into a cluster.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the test that
proves it. `just living-docs` runs the tests and fails when the page and the broker disagree. The
tests behind this page are plain Go tests. They record no HTTP exchanges, so the configuration,
values, and commands shown here are illustrations, not compared output. The Helm tests need the
`helm` program and report "not run" where it is missing.
:::

## The cast

The **operator** deploys the broker for a company. Two groups reach it:

- Users such as Alice (`alice@example.com`), their browsers, and agents such as the *Research
  Assistant*. They use the consent screens and the OAuth2 endpoints.
- Administrators such as `admin@example.com` and automation. They register agents, third-party
  services such as *GitHub*, and permission sets such as `github-read`.

The operator wants the second group on a private network and the first group on a public one.

## One process, two servers

The broker runs two HTTP servers in one process: the **end-user server** and the **admin server**.
Each has its own port and its own bind address. The defaults are port `8000` and port `14000`,
both bound to `::`.

```yaml
# Illustration: the server section of a broker configuration file
server:
  enduser:
    port: 8000
    bind: "::"
  admin:
    port: 14000
    bind: 127.0.0.1
  shutdown:
    timeout: 30s
```

After the broker starts with two different ports, each server answers on its own port. The
end-user port serves the end-user API: Alice's request for her own profile succeeds there. The
admin port serves the admin API: the agent list is returned there. `example:deployment.servers.two-ports`

The split is the reason for the two servers. A network rule, a Service port, or an Ingress can
expose one server without the other. The page "Configuration" describes where these settings
come from.

### Bind addresses

The bind address selects the network interfaces of one server. A server bound to `::` accepts
connections over IPv4 (`127.0.0.1`), over IPv6 (`::1`) where the host has IPv6, and by the name
`localhost`. `example:deployment.servers.bind-address`

An operator who sets `bind: 127.0.0.1` for the admin server, as in the illustration, keeps that
server off every other interface. No test on this page proves that a restricted bind address
refuses connections from other interfaces.

## Startup is all or nothing

The broker never runs with only one server. It binds both ports first and serves traffic only
when both binds succeed.

Suppose another process already holds the admin port. The broker exits with the error
`admin server bind failed`, and the end-user port is free again afterwards. The same holds the
other way round: when the end-user port is taken, the broker reports
`enduser server bind failed` and leaves the admin port free. `example:deployment.startup.all-or-nothing`

A container orchestrator therefore sees a failed start as a failed container. It never sees a
broker that serves users while administrators cannot reach it.

## Graceful shutdown

Kubernetes stops a pod with the signal `SIGTERM`. On `SIGTERM` the broker stops both servers and
the process ends without an error. Both ports are free afterwards, so the next process can bind
them at once. `example:deployment.shutdown.sigterm`

During shutdown the broker lets requests that are in progress finish, up to
`server.shutdown.timeout` (default `30s`). No test on this page proves that an in-flight request
completes or that the timeout cuts it off.

## Health endpoint

Each server has its own `GET /health`. An orchestrator can probe either port, and each answer
describes only the server that was asked. With both servers running, both endpoints return
`200 OK` with the status `healthy`. `example:deployment.health.each-server`

The response is JSON with the content type `application/json`. It has three members that are
always present: `example:deployment.health.response`

```json
{
  "status": "healthy",
  "timestamp": "2026-10-30T09:00:00Z",
  "uptime_seconds": 42
}
```

`status` is one of `starting`, `healthy`, `shutting_down`, and `unhealthy`, and
`uptime_seconds` grows while the server runs. The JSON above illustrates the shape; the test
checks the members and not a full body.
A server that is not `healthy` is meant to answer `503 Service Unavailable`, so that a probe
fails. No test on this page drives a real server into that state.

The end-user server also serves the single-page application at the root path. `/health` still
answers there and is not replaced by the application. `example:deployment.health.beside-spa`

## Container images

The repository builds two images for the broker:

| Image | Content | Used by |
|---|---|---|
| `agentic-identity-broker` | The broker binary and the built single-page application | The broker Deployment |
| `agentic-identity-broker-migrate` | The `migrate` tool and the SQL migrations | The migration Job |

The runtime image holds no migration tool, and the migration image holds no broker. Each image
runs as user `1000`, not as root. The runtime image declares ports `8000` and `14000` and starts
the broker binary. The migration image starts `migrate`.

The Dockerfiles are `build/docker/Dockerfile` and `build/docker/Dockerfile.migrate`. They copy a
binary that was built before, so the build of the image does not compile Go code. This section
is commentary: no test on this page builds or starts an image. The maintained build commands
are `just docker-build-broker` and `just docker-build-migrate`.

## The Helm chart

The chart `charts/agentic-identity-broker` deploys the broker to Kubernetes. With default values
it renders:

- one **Deployment** with one broker container;
- one **ConfigMap** that holds the broker configuration file;
- one **Service** with two named ports, `enduser` (`8000`) and `admin` (`14000`);
- one **ServiceAccount**.

The broker pod runs as a non-root user. The configuration file from the ConfigMap is mounted
read-only at `/app/config.yaml`, and `/tmp` is the only writable directory that the chart adds.
Secret values such as the JWE signing key reach the container as environment variables that
refer to a Kubernetes Secret, not as plain values in the Deployment. `example:deployment.chart.workload`

The liveness and readiness probes of the Deployment call `/health` on the `enduser` port.

### Two optional Ingresses

The chart can render two Ingress resources. Each one is off by default and has its own host,
class, annotations, and TLS settings. Both point at the same Service: the end-user Ingress at
the `enduser` port and the admin Ingress at the `admin` port.

```yaml
# Illustration: values that expose only the end-user server to the public
ingress:
  enduser:
    enabled: true
    hosts:
      - host: broker.example.com
        paths:
          - path: /
            pathType: Prefix
  admin:
    enabled: false
```

### Storage choice

`storage.type` selects the storage. The default `memory` keeps everything in the pod and needs
no database; it is for evaluation. `postgres` uses PostgreSQL, either an existing instance
(`postgresql.external`) or one that the Zalando PostgreSQL Operator creates
(`postgresql.operator`). The page "Storage and encryption at rest" describes the backends and
the encryption keys.

### The migration Job

With `storage.type: postgres`, the chart renders a **migration Job**. It is a Helm hook that
runs before every install and every upgrade, so the schema is current before a new broker pod
starts. When the Job fails, Helm stops the release and the running broker stays as it was.

The Job and the broker use different database users:

| Workload | Image | Database user | Rights |
|---|---|---|---|
| Migration Job | `agentic-identity-broker-migrate` | Migration user | Changes the schema |
| Broker Deployment | `agentic-identity-broker` | Broker user | Reads and writes rows only |

After the migrations, the Job grants the broker user `SELECT`, `INSERT`, `UPDATE`, and `DELETE`
on the tables and the use of the sequences. A broker that is compromised can therefore change
data but cannot change or drop tables. Each user's name and password come from its own
Kubernetes Secret.

No Go test renders the Service, the Ingresses, or the migration Job. This section and the two
before it describe the chart templates and are commentary. The script
`scripts/validate-helm-chart.sh` installs the chart into a local cluster, but it is not part of
the living-documentation check.

## Scheduled session sweep

The broker has an admin endpoint that refreshes third-party sessions that are about to expire,
`POST /api/sessions/sweep`. The page "Third-party sessions" describes what a sweep does. The
chart can call that endpoint on a schedule.

The sweep is off by default. With `sessionSweep.enabled` unset, the chart renders no CronJob and
no other sweep resource, even when other `sessionSweep` values are set. `example:deployment.sweep.disabled`

```yaml
# Illustration: the smallest values that enable the sweep
broker:
  server:
    admin:
      publicUrl: https://broker-admin.example.test
sessionSweep:
  enabled: true
  headers:
    X-Operator: session-sweep
```

With these values in the namespace `identity`, the chart renders a **CronJob** that:

- runs every hour (`0 * * * *`) and never starts a run while the previous one is active
  (`concurrencyPolicy: Forbid`);
- sends the request to the admin port of the release's own Service,
  `http://broker-agentic-identity-broker.identity.svc:14000/api/sessions/sweep`;
- sets the `Host` header to the host of the admin public URL, `broker-admin.example.test`,
  because the admin API accepts only requests for that host;
- sends the configured header `X-Operator: session-sweep`;
- asks for a lookahead of two hours, `PT2H`, and not for a dry run;
- does not retry a failed run, mounts no service account token, and has a read-only root
  filesystem. `example:deployment.sweep.cronjob`

### The sweep needs a caller identity

The admin API must know who calls it. The chart only transports the request, so the operator
must supply the identity in one of three ways: literal headers (`sessionSweep.headers`), headers
read from mounted files (`sessionSweep.headersFromFile`), or a TLS client certificate
(`sessionSweep.tls`).

When the sweep is enabled without any of them, the chart refuses to render with the message
`sessionSweep.enabled requires an identity for the admin API`. The chart also refuses a request
timeout that is not shorter than the Job deadline, a client certificate without its key, and a
lookahead that is not a positive ISO 8601 duration. `example:deployment.sweep.identity-required`

Literal headers are written into the manifest, so they must not hold a secret. A header from a
file is different: the CronJob gets only the path of the file, such as
`/var/run/secrets/session-sweep/token` for an `Authorization` header with the prefix `Bearer `,
and the value is read when the job runs. `example:deployment.sweep.file-headers`

### The sweep pod is not a broker pod

The pod of the CronJob carries the component label `session-sweep` and never the label that the
broker Service and the PodDisruptionBudget select on, even when the operator sets that label in
`commonLabels` or `sessionSweep.podLabels`. The Service therefore never routes user traffic to a
sweep pod. `example:deployment.sweep.outside-service`

## Not yet on this page

- No test builds or runs the container images. The image content, the non-root user of the
  images, and the `just` commands are commentary.
- No Go test renders the Service, the two Ingresses, the storage variants, or the migration Job
  with its hook, its two database users, and its grants.
- In-flight requests during shutdown, the shutdown timeout, and the `503` answer of a server
  that is starting, shutting down, or unhealthy have no test at the level of a running broker.
- A restricted bind address, the fallback from `::` to `0.0.0.0` on a host without IPv6, and
  the independence of one server from a runtime failure of the other are not shown.
- The optional `components` member of the health response belongs to the page "Trust anchors".
- Autoscaling, the PodDisruptionBudget, IRSA, release verification, and the ExtProc service are
  not described here.

## Sources

The behavior on this page was decided in `specs/003-dual-port-server/spec.md`,
`specs/010-docker-setup/spec.md`, `specs/014-helm-charts/spec.md`, and the CronJob part of
`specs/053-session-auto-refresh/spec.md`, and in `adrs/004-dual-server-isolation.md`,
`adrs/009-separate-migration-docker-image.md`, and `adrs/038-proactive-token-refresh.md`. The
deployment contract is `charts/agentic-identity-broker/` and the Dockerfiles under
`build/docker/`. Guides are in `docs/deployment/` and `docs/guides/deploy-on-kubernetes.md`.
The tests are in `cmd/agentic-identity-broker/root_test.go`, `tests/integration/server_test.go`,
`tests/integration/health_test.go`, `tests/integration/helm_chart_test.go`, and
`internal/adapters/http/health_test.go`.
