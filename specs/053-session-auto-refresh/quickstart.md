# Quickstart: Validate Automatic Third-Party Session Refresh

**Feature**: `053-session-auto-refresh` | **Spec**: [spec.md](spec.md) | **Contracts**: [contracts/](contracts/)

Run from the repository root.

## Prerequisites

- Go 1.27.1, `just`, `ginkgo` (`just install-tools`)
- `helm` 3 on `PATH`, plus `/bin/sh` and `curl` (the chart E2E suite runs the rendered script locally)
- Docker or Podman for PostgreSQL integration tests

## 1. Static checks and unit tests

```sh
just check
just test
```

Expected: new unit tests pass for `ClassifySessionHealth`, `SessionHealthMetrics`, the skipped counter, the sweep duration histogram, and the `token_refresh.health_metrics` validation.

## 2. Storage parity

```sh
go test ./internal/adapters/storage/memory/... -run TestUserSessionCountSessionsByExpiry
just test-integration-infra
```

Expected: scenarios H1 to H9 ([contracts/ports.md](contracts/ports.md#parity-scenarios-same-cases-in-both-adapter-test-files)) pass on both backends.

## 3. Chart render

```sh
just helm-lint
helm template broker ./charts/agentic-identity-broker | grep -c 'kind: CronJob'   # 0: disabled by default
helm template broker ./charts/agentic-identity-broker -n identity \
  --set sessionSweep.enabled=true \
  --show-only templates/cronjob-session-sweep.yaml
```

Expected:
- With only `enabled=true`, the second command fails with the identity message from [contracts/chart-values.md](contracts/chart-values.md#render-time-checks-fail).
- With `--set-string 'sessionSweep.headers.X-Request-Source=session-sweep'` added, it renders an hourly `Forbid` CronJob that targets `http://broker-agentic-identity-broker.identity.svc:14000`.

Also confirm that sweep values have no effect while the sweep is disabled:

```sh
helm template broker ./charts/agentic-identity-broker > /tmp/default.yaml
helm template broker ./charts/agentic-identity-broker \
  --set-string 'sessionSweep.headers.X-Request-Source=session-sweep' \
  --set sessionSweep.schedule='*/5 * * * *' > /tmp/disabled-with-values.yaml
diff /tmp/default.yaml /tmp/disabled-with-values.yaml   # no output (FR-001)
```

## 4. Default image check

```sh
docker run --rm --entrypoint /bin/sh curlimages/curl:8.16.0 -c 'curl --version | head -1'
```

Expected: a curl 8.16.0 version line. Config variables need curl 8.3 or later (research R1).

## 5. Acceptance checks

```sh
go test ./tests/integration/ -run 'TestHelmTemplate_SessionSweep' -v
ginkgo -v --focus-file session_metrics_test.go ./tests/e2e/
```

Expected: every scenario in the plan's mapping table passes. The chart render checks need `helm` on `PATH` and skip otherwise. The metrics suite reads `oauth2_sessions`, `session_sweep_duration_seconds`, and `session_sweep_skipped_total` from a ManualReader.

The run-time behavior of the rendered script is verified by hand. Render the CronJob (step 3), write a token to a file, and run the container command with the default image against a local echo server or the admin server:

```sh
helm template broker ./charts/agentic-identity-broker -n identity -f values.yaml \
  --show-only templates/cronjob-session-sweep.yaml | yq '.spec.jobTemplate.spec.template.spec.containers[0].command[2]' > /tmp/sweep.sh
docker run --rm -v /tmp/sweep.sh:/sweep.sh:ro -v /tmp/token:/var/run/secrets/session-sweep/token:ro \
  --read-only --user 1000 --network host curlimages/curl:8.16.0 /bin/sh -ec "$(cat /tmp/sweep.sh)"
```

Expected: the target sees `Authorization: Bearer <token>` with no quoting artifacts, the response body is printed, and a non-2xx status or missing file exits non-zero without printing the token.

## 6. Manual cluster check (optional)

On a test cluster with the chart installed and the sweep enabled:

```sh
kubectl -n <ns> create job --from=cronjob/<fullname>-session-sweep sweep-manual
kubectl -n <ns> logs job/sweep-manual
```

Expected: one JSON summary line and a `Complete` job. The broker logs one `session sweep` audit record with `outcome=completed` and your configured operator principal. Within one export cycle the backend shows `session_sweep_duration_seconds_count` +1 and `oauth2_sessions` series for each service.

## 7. Full gate

```sh
just verify
```
