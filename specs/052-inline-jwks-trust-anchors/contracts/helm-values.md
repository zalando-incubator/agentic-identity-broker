# Contract Delta: Helm chart values

**Feature**: [../spec.md](../spec.md) | **Decision**: [../research.md](../research.md) R13

Chart: `charts/agentic-identity-broker/`. This delta satisfies FR-020 and the Principle VII Helm rule.

## New values

```yaml
broker:
  security:
    # -- Trust material mounted for jwks_file key sets.
    trustMaterial:
      # -- Absolute path of the trust directory (security.trust_material_dir). Empty disables jwks_file.
      directory: ""
      # -- Existing Secrets or ConfigMaps projected read-only into the trust directory.
      # Each entry is {secret: {name, items}} or {configMap: {name, items}}; items are {key, path}.
      sources: []
      # sources:
      #   - secret:
      #       name: agentgateway-jwks
      #       items:
      #         - key: jwks.json
      #           path: agentgateway-jwks.json
      #   - configMap:
      #       name: mesh-jwks
      #       items:
      #         - key: jwks.json
      #           path: mesh-jwks.json
  tokenExchange:
    clientAssertion:
      # -- Inline JWK Set JSON. Mutually exclusive with jwksUri and jwksFile.
      jwks: ""
      # -- Absolute key-set file path inside broker.security.trustMaterial.directory.
      jwksFile: ""
```

## Rendering

| Input | Output |
|---|---|
| `trustMaterial.directory` non-empty | `security.trust_material_dir: <directory>` in `config.yaml` |
| `trustMaterial.sources` non-empty | A `trust-material` volume of type `projected` with one `secret` or `configMap` source per entry, mounted read-only at `directory` **without `subPath`** |
| `sources` non-empty and `directory` empty | Template failure: `broker.security.trustMaterial.directory is required when sources are set` |
| `clientAssertion.jwks` | `token_exchange.client_assertion.jwks` as a YAML block scalar |
| `clientAssertion.jwksFile` | `token_exchange.client_assertion.jwks_file` |

Kubernetes does not update `subPath` mounts. The projected volume keeps the `..data` relative-link
layout, so a Secret or ConfigMap update reaches the broker through the file re-read.

## Schema (`values.schema.json`)

| Object | Change |
|---|---|
| `broker.security` | Add `trustMaterial` with `directory` (string; empty or an absolute path) and `sources` (array of `oneOf` `{secret}` / `{configMap}` objects, `additionalProperties: false`) |
| `broker.tokenExchange.clientAssertion` | Add `jwks` and `jwksFile` (strings) |
| Impersonation `trusted_issuers[]` items | Add `jwks` and `jwks_file` (strings) |
| `definitions.serverConfig.authentication.jwt` | Add `jwks` and `jwks_file` (strings). Valid on both the end-user and the admin server. |

## README

Add the four new values to the parameter table. Add a "Trust material for local key sets" section
with the Secret example, the no-`subPath` rule, and the rotation behaviour.

## Verification

- `helm lint charts/agentic-identity-broker`
- `tests/integration/helm_chart_test.go`: render with a Secret and a ConfigMap source and assert the
  projected volume, the read-only mount without `subPath`, and `security.trust_material_dir`.
- `tests/e2e/`: User Story 2 scenario 5 renders the chart and boots the broker from the rendered
  configuration ([../quickstart.md](../quickstart.md) Scenario 5).
