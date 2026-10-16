# Quickstart: Validate Inline and File-Based JWKS for Trust Anchors

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

This guide lists the runnable checks that prove the feature works. The contracts are in
[contracts/](./contracts/), and the domain rules are in [data-model.md](./data-model.md). It does not
contain implementation code.

## Prerequisites

- Go 1.27.1, `just`, and the tools from `just install-tools` (Ginkgo v2.33.0).
- Helm 3 on `PATH`. User Story 2 scenario 5 renders the chart. The `e2e-backend` CI job installs it
  with `azure/setup-helm`.
- No Docker is needed. E2E tests use in-memory storage, in-memory keys, and per-test temporary
  trust directories.

## 1. Fast checks

```bash
just check
go test -race ./internal/domain/trustanchor/... ./internal/adapters/jwks/... \
  ./internal/adapters/jwtauth/... ./internal/config/... ./internal/app/...
```

Expected:

- `trustanchor` table tests cover every row of the resolution rules and Local Key Set invariants in
  [data-model.md](./data-model.md), including the jwx edge cases in [research.md](./research.md) R5.
- `jwks` adapter tests cover the file state machine (Current, Degraded, FailedClosed), containment
  through `os.Root`, and the static adapter's constant health.
- `config` tests cover the new keys, `BindEnv` overrides, and the `[inline JWK Set redacted]`
  placeholder.

## 2. Acceptance suite

```bash
ginkgo -v --label-filter="inline-jwks-trust-anchors" ./tests/e2e/
```

Expected: one passing `It()` per acceptance scenario, 29 in total. The mapping table is in
[plan.md](./plan.md). Each failure message names the scenario through its
`// USn-Sm from specs/052-inline-jwks-trust-anchors/spec.md` comment.

Key-source parity for SC-003 (five credentials × remote, inline, and file sources):

```bash
ginkgo -v --label-filter="inline-jwks-trust-anchors-parity" ./tests/e2e/
```

Regression guard for SC-005 (existing behaviour unchanged):

```bash
ginkgo -v --label-filter="!performance" \
  --focus="Token Exchange|Impersonation|JWT Pre-Authentication|Approval" ./tests/e2e/
```

## 3. Helm rendering

```bash
just helm-lint
go test -v -run 'TestHelmTemplate_TrustMaterial' ./tests/integration/
```

Expected: the rendered Deployment has a read-only `projected` volume at the trust directory without
`subPath`. The rendered `config.yaml` has `security.trust_material_dir`, and `jwks` or `jwks_file`
where set. See [contracts/helm-values.md](./contracts/helm-values.md).

## 4. Manual scenarios against a local binary

Build once with `just build`. Generate a P-256 key pair with any JOSE tool that prints a JWK, for
example `step crypto jwk create`. Keep the private JWK outside the trust directory. Use only the
public JWK Set in the steps below.

### Scenario A: Inline key set, no outbound key requests (User Story 1, SC-002)

1. Configure local mode and token exchange. Set `token_exchange.client_assertion.issuer_uri` to an
   unreachable host such as `https://agentgateway.invalid`. Set `token_exchange.client_assertion.jwks`
   to the public JWK Set.
2. Start `./bin/agentic-identity-broker --config <file>`.
3. Expected: startup succeeds. The log has one `trust anchor key set loaded` event with
   `key_source=inline`, `key_count=1`, and the key identifier. No log line contains the `x` or `y`
   coordinate. No DNS lookup or connection for `agentgateway.invalid` occurs.

### Scenario B: File rotation and degraded health (User Story 2, SC-007)

1. Create `/tmp/trust` and write the public set to `/tmp/trust/agentgateway-jwks.json`.
2. Set `security.trust_material_dir: /tmp/trust`, `jwks_file: /tmp/trust/agentgateway-jwks.json`,
   `jwks_min_refresh: 10s`, and `jwks_max_refresh: 2m`.
3. Start the broker. Expected: `GET /health` has no degraded component.
4. Replace the file with a set that holds a second key. Expected within 10 seconds: one
   `trust anchor key set changed` event with the added and removed key identifiers.
5. Write `{"keys":[` to the file. Expected within 10 seconds: a `trust anchor key set re-read failed`
   event with `error_kind=not_a_jwk_set`, and `"trust_anchor_key_files":"degraded"` in `/health`.
   Verification still uses the previous set.
6. Restore a valid file. Expected: `trust anchor key set recovered`, and the component returns to
   `healthy`.

### Scenario C: Fail-closed startup (User Story 3)

Each configuration must stop startup with the message in
[contracts/configuration.md](./contracts/configuration.md) §2, and no key material in the output.

| Configuration | Expected category |
|---|---|
| Both `jwks_uri` and `jwks` on one trust anchor | `conflicting_key_sources` |
| `jwks: '{"keys":[]}'` | `empty_key_set` |
| A JWK Set that contains the private `d` member | `private_key` |
| `jwks` plus `jwks_min_refresh: 5m` | `refresh_bounds_with_inline` |
| `jwks_file: /etc/passwd` with `trust_material_dir: /tmp/trust` | `outside_trust_directory` |
| `jwks_file` with no `trust_material_dir` | `missing_trust_directory` |
| `trust_material_dir: /` | `invalid_trust_directory` |

### Scenario D: Kubernetes mount layout (User Story 2, scenario 9)

1. In `/tmp/trust`, create `..2026_10_16/jwks.json`, a relative link `..data -> ..2026_10_16`, and a
   relative link `jwks.json -> ..data/jwks.json`.
2. Point `jwks_file` at `/tmp/trust/jwks.json`. Expected: startup succeeds.
3. Replace the link with `jwks.json -> /tmp/trust/..data/jwks.json` (absolute). Expected: startup
   fails with `outside_trust_directory`. This is the intended `os.Root` behaviour (research R4).

## 5. Deployment package (User Story 2, scenario 5)

```bash
kubectl create secret generic agentgateway-jwks --from-file=jwks.json=./public-jwks.json
helm template broker charts/agentic-identity-broker \
  --set broker.security.trustMaterial.directory=/etc/agentic-identity-broker/trust \
  --set 'broker.security.trustMaterial.sources[0].secret.name=agentgateway-jwks' \
  --set 'broker.security.trustMaterial.sources[0].secret.items[0].key=jwks.json' \
  --set 'broker.security.trustMaterial.sources[0].secret.items[0].path=agentgateway-jwks.json' \
  --set broker.tokenExchange.clientAssertion.jwksFile=/etc/agentic-identity-broker/trust/agentgateway-jwks.json
```

Expected: a `trust-material` projected volume from Secret `agentgateway-jwks`, mounted read-only at
`/etc/agentic-identity-broker/trust`, and the matching `jwks_file` and `trust_material_dir` in the
ConfigMap. After `helm install`, `kubectl create secret ... --dry-run=client -o yaml | kubectl apply -f -`
with a new key set reaches the broker within `jwks_min_refresh` without a pod restart.
