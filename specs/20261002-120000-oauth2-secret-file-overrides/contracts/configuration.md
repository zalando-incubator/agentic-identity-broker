# Contract: OAuth2 Credential-File Configuration

**Propagated**: 2026-10-07 — Updated from spec.md refinement

**Updated**: 2026-10-08 — Refreshed T006 for strict pair configuration, coherent acquisition, and irreversible source-transition evidence. These are design requirements, not runtime evidence.

This contract defines planned configuration behavior. Loader, CLI, chart, and operator-documentation changes remain implementation requirements.

## Parameter

| Interface | Name | Value |
|---|---|---|
| Broker YAML | `third_party_oauth2.credential_files` | Mapping of exact service canonical IDs to file-pair objects. |
| Environment | `IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES` | JSON object string. |
| CLI | `--third_party_oauth2.credential_files` | JSON object string. |
| Helm | `broker.thirdPartyOauth2.credentialFiles` | Object with the same keys and file pairs. |

Each binding contains two required absolute-path strings: `client_id_file` and `client_secret_file`.

~~`third_party_oauth2.client_secret_files`, `IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CLIENT_SECRET_FILES`, `--third_party_oauth2.client_secret_files`, and `broker.thirdPartyOauth2.clientSecretFiles` configure secret-only bindings.~~ Superseded because filesystem mode requires both credentials. No old-name alias is planned.

~~An omitted mapping and `{}` disable file overrides.~~ Superseded because the registered service selects its source independently.

The default is an empty mapping. Omission or `{}` supplies no bindings, but never changes a filesystem service to stored mode. No file discovery occurs.

## YAML

One binding:

```yaml
third_party_oauth2:
  credential_files:
    zalando-platform:
      client_id_file: /meta/credentials/employee-client-id
      client_secret_file: /meta/credentials/employee-client-secret
```

Multiple independent bindings:

```yaml
third_party_oauth2:
  credential_files:
    zalando-platform:
      client_id_file: /meta/credentials/employee-client-id
      client_secret_file: /meta/credentials/employee-client-secret
    GitHub-Prod:
      client_id_file: /run/broker-secrets/github-client-id
      client_secret_file: /run/broker-secrets/github-client-secret
    com.example.service:
      client_id_file: /run/broker-secrets/example-client-id
      client_secret_file: /run/broker-secrets/example-client-secret
```

Explicit empty mapping:

```yaml
third_party_oauth2:
  credential_files: {}
```

Keys retain their literal spelling. `GitHub-Prod` does not match `github-prod`. A dot is part of a canonical ID, not a nested configuration path.

## Environment and CLI

One binding as a JSON object string:

```bash
export IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES='{"zalando-platform":{"client_id_file":"/meta/credentials/employee-client-id","client_secret_file":"/meta/credentials/employee-client-secret"}}'
```

```bash
./bin/agentic-identity-broker \
  --third_party_oauth2.credential_files='{"zalando-platform":{"client_id_file":"/meta/credentials/employee-client-id","client_secret_file":"/meta/credentials/employee-client-secret"}}'
```

Multiple independent bindings as a JSON object string:

```bash
export IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES='{
  "zalando-platform": {
    "client_id_file": "/meta/credentials/employee-client-id",
    "client_secret_file": "/meta/credentials/employee-client-secret"
  },
  "GitHub-Prod": {
    "client_id_file": "/run/broker-secrets/github-client-id",
    "client_secret_file": "/run/broker-secrets/github-client-secret"
  },
  "com.example.service": {
    "client_id_file": "/run/broker-secrets/example-client-id",
    "client_secret_file": "/run/broker-secrets/example-client-secret"
  }
}'
```

The CLI flag accepts the same complete JSON object. JSON whitespace does not change key spelling or path values.

Explicit empty mapping:

```bash
export IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES='{}'
```

```bash
./bin/agentic-identity-broker --third_party_oauth2.credential_files='{}'
```

These examples configure the mapping only. Existing authentication, encryption, server-mode, and JWE configuration remain required.

## Source resolution

The winning source supplies the complete mapping:

1. An explicitly changed CLI flag
2. The environment variable supplied through the existing configuration loader
3. The YAML configuration field in the selected broker file
4. The empty default.

The existing `.env` loading flow remains available. Do not change unrelated configuration source resolution.

| YAML | Environment | CLI | Effective mapping |
|---|---|---|---|
| `{alpha: {client_id_file: /a/id, client_secret_file: /a/secret}}` | Absent | Unchanged flag | The complete `alpha` pair from YAML |
| `{alpha: {client_id_file: /a/id, client_secret_file: /a/secret}}` | `{"beta":{"client_id_file":"/b/id","client_secret_file":"/b/secret"}}` | Unchanged flag | The complete `beta` pair from environment |
| `{alpha: {client_id_file: /a/id, client_secret_file: /a/secret}}` | `{"beta":{"client_id_file":"/b/id","client_secret_file":"/b/secret"}}` | `{"gamma":{"client_id_file":"/c/id","client_secret_file":"/c/secret"}}` | The complete `gamma` pair from CLI |
| `{alpha: {client_id_file: /a/id, client_secret_file: /a/secret}}` | `{}` | Unchanged flag | Empty mapping, with service sources unchanged |
| `{alpha: {client_id_file: /a/id, client_secret_file: /a/secret}}` | `{"beta":{"client_id_file":"/b/id","client_secret_file":"/b/secret"}}` | `{}` | Empty mapping, with service sources unchanged |

No lower-precedence entries or pair fields survive a winning mapping. A supplied empty string is invalid input, not an empty mapping. Malformed winning input stops startup instead of falling back.

The loader preserves canonical-ID case before it creates `ports.Config`. Generic Viper map decoding cannot define this field's semantics because it folds YAML key case.
If YAML supplies the winning mapping, decode its original mapping from the configuration file selected by the existing loader. Preserve literal keys and strict scalar types. Reuse the loader's path-value expansion behavior for YAML values where applicable. Do not select a separate configuration file.

## Startup validation

A valid canonical ID uses 1–128 ASCII letters, digits, `.`, `_`, or `-`. It is not UUID-shaped. Reuse `internal/domain/canonical.Validate()`.

A path is a non-empty string that is absolute on the broker platform. A whitespace-only or relative path is invalid. Do not trim, infer, or construct paths from service names or request data.

JSON input must be an object whose values are file-pair objects. YAML input must be a mapping with literal string keys and file-pair mappings. Each pair requires both `client_id_file` and `client_secret_file` as strings. No source permits weak scalar conversion or key case folding.

Reject these invalid inputs:

- Malformed input, arrays, scalar values, or `null` for the complete mapping
- Non-string YAML keys or canonical IDs that fail the stated validation
- A string, array, scalar value, or `null` instead of a file-pair object
- A missing `client_id_file` or `client_secret_file` field
- A numeric, boolean, collection, or `null` value for either path
- An empty, whitespace-only, or relative value for either path.

Validation errors identify `third_party_oauth2.credential_files` and the invalid rule. They do not echo raw input or credential contents.

Startup validation does not open credential files or query registered services. As a result, a missing file or unknown canonical ID does not block startup.

## Runtime selection

~~A matching binding selects the file-secret override for an eligible service.~~ Superseded because only the registered service's `credential_source` selects filesystem mode.

Administrators select `stored` or `filesystem` through the service administrative interface. Creation omission defaults to stored. Update omission preserves the current source. Unknown values and explicit null are invalid, not omission. Operator configuration never registers or modifies a service, its UUID, or its authentication mode.

A stored-source service ignores bindings and reads no credential files. Filesystem mode requires a canonical ID and its exact matching pair. Filesystem registration and updates reject either inline credential, including empty, null, or placeholder values. Administrative requests cannot supply file paths.

Stored confidential services retain existing non-empty inline client-ID and secret validation. Only an explicit administrative transition selects stored credentials for a filesystem service. That transition requires supplied inline credentials and never imports file values.

Public clients, CIMD confidential clients, and Google-flavor services ignore unused bindings and reject filesystem mode. Clearing a filesystem service's canonical ID is invalid. Changing it to another valid ID preserves filesystem mode and requires that ID's binding at use.

~~A service without a matching binding uses stored credentials.~~ Superseded because missing bindings cannot change the selected source. An empty mapping, removed binding, or unusable required file stops filesystem operations without stored or previous-value fallback.

Filesystem authorization initiation calls `ReadClientID(path string) (string, error)` only for its configured client-ID path. It never accesses the secret path. Each broker-owned exchange attempt or actual refresh attempt calls `ReadPair(clientIDPath, clientSecretPath string) (clientID, clientSecret string, err error)` exactly once. The reader opens both descriptors before either read and revalidates both configured paths after both bounded reads. A changed target produces `generation_changed`, without a partial pair or acquisition retry. [outbound-authentication.md](./outbound-authentication.md) defines descriptor validation and immutable-target publication.

Administrative, consent, and session metadata do not read either file. Filesystem administrative responses omit both inline credential fields. Stored confidential responses retain their stored client ID and existing `REDACTED` secret. Excluded-mode representations remain unchanged.

New eligible stored and filesystem authorizations capture their effective non-secret client ID. Successful exchange preserves that established identity in the session. Before exchange or refresh, both source modes compare the selected client ID with the established identity. A mismatch stops before provider authentication and requires a new connection. Source transitions never rewrite that identity.

Creation and legacy backfill initialize internal `credential_source_transitioned` to false. Each actual source transition sets it to true atomically with credential removal or replacement. The marker never resets or appears in administrative input or responses. No-op selections and unrelated updates preserve it.

Legacy identity absence remains usable only in stored mode with `credential_source_transitioned: false`. Filesystem mode or a prior transition produces `identity_missing` and requires a new connection. Returning to stored never restores the exception. Public, CIMD confidential, and Google-flavor contexts retain their existing behavior. Never derive legacy identity from current files or service credentials.

The mapping does not hot reload. File contents take effect independently on the next operation. Configuration changes use the normal restart workflow.

## Diagnostics and documentation

No configuration diagnostic reads credential contents. Implementation must redact the renamed key and its environment variable without relying on `SECRET` in the former name. Source audit metadata can identify the key and source without dumping the mapping value.

Implementation documentation belongs in `docs/configuration.md`, `examples/config/third-party-oauth2.yaml`, and `examples/config/README.md`. Explain all sources, whole-map precedence, literal key case, explicit service-source selection, and mandatory file pairs. Distinguish stored mode, an absent filesystem binding, and an unusable source.

For file and HTTP behavior, see [outbound-authentication.md](./outbound-authentication.md). For chart inputs, see [deployment.md](./deployment.md).
