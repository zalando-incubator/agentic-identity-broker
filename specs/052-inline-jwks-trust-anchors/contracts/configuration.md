# Contract: Trust-Anchor Key-Set Configuration

**Feature**: [../spec.md](../spec.md) | **Data model**: [../data-model.md](../data-model.md)

This feature adds no HTTP endpoint. Its operator-facing contract is configuration, startup
diagnostics, logs, and one `/health` component ([health-endpoint.md](./health-endpoint.md)). This
document is the source for `docs/configuration.md`, `examples/config/`, and the tests.

## 1. Settings

### 1.1 Broker-wide

| Key | Type | Default | Environment variable | Rules |
|---|---|---|---|---|
| `security.trust_material_dir` | string | unset | `IDENTITY_BROKER_SECURITY_TRUST_MATERIAL_DIR` | Absolute path. Must resolve, after symbolic links, to an existing directory other than `/`. Required by any `jwks_file`. Unused otherwise; the broker does not touch the filesystem when no `jwks_file` is configured. |

### 1.2 Per trust anchor

Each in-scope trust anchor accepts at most one of `jwks_uri` (existing), `jwks`, and `jwks_file`.

| Trust anchor | `jwks` | `jwks_file` | Environment override |
|---|---|---|---|
| `token_exchange.client_assertion` | Inline JWK Set JSON | Absolute path inside the trust directory | `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS`, `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_FILE` |
| `oauth2_authorization_server.impersonation.rules[].trusted_issuers[]` | Inline JWK Set JSON | Absolute path inside the trust directory | None (rule lists are YAML-only, as today) |
| `server.enduser.authentication.jwt` | Inline JWK Set JSON | Absolute path inside the trust directory | `${VAR}` expansion in YAML, as the whole value |
| `server.admin.authentication.jwt` | Inline JWK Set JSON | Absolute path inside the trust directory | `${VAR}` expansion in YAML, as the whole value |

The two JWT pre-authentication trust anchors are independent: each has its own source and adapter.

### 1.3 Refresh bounds

| Source | `jwks_min_refresh` / `jwks_max_refresh` |
|---|---|
| `jwks_uri` or discovery | Unchanged |
| `jwks_file` | Minimum: re-read interval. Maximum: how long after the last successful read the last valid set may still be served. Defaults: 15m and `max(min, 1h)`. JWT pre-authentication uses fixed 5m and 15m. |
| `jwks` | Rejected when either bound is set |

### 1.4 Inline value format

- JSON text of an RFC 7517 §5 JWK Set: an object with a `keys` array.
- A YAML block scalar (`jwks: |`) or a bound environment variable may span lines.
- A YAML value that contains a `${VAR}` reference must fit on one line and must not contain shell
  metacharacters, because the loader checks that value before it substitutes. Write the reference
  as the whole value (`jwks: ${MESH_JWKS}`); the substituted environment value may span lines.

### 1.5 Key-set document rules (inline and file)

| Rule | Category |
|---|---|
| At most 1 MiB | `too_large` |
| An object with a `keys` array that jwx parses with strict key-set parsing | `not_a_jwk_set` |
| At least one key | `empty_key_set` |
| No `kty: oct` key | `symmetric_key` |
| No private key members | `private_key` |
| `alg`, when present, is one of RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384, ES512, EdDSA | `unapproved_algorithm` |
| Every key has a non-empty `kid` | `missing_kid` |
| `kid` values are unique | `duplicate_kid` |

Only `alg` is optional for local keys. Token exchange and impersonation skip a key without `alg`, as they do for remote keys. Every trust anchor requires a token `kid`, so a local key without `kid` cannot match. Remote key-set acceptance does not change. Set `alg` on every local key for token exchange and impersonation.

### 1.6 Key-set file rules

| Rule | Category |
|---|---|
| Absolute path | `relative_path` |
| `security.trust_material_dir` is set | `missing_trust_directory` |
| The cleaned path lies under the cleaned trust directory | `outside_trust_directory` |
| Opening it beneath the trust directory does not leave the directory through `..` or a symbolic link; absolute symbolic links are refused | `outside_trust_directory` |
| It exists | `not_found` |
| It can be read | `unreadable` |
| It is a regular file after the broker follows symbolic links | `not_regular_file` |

## 2. Startup errors

Every error names the trust anchor's configuration path. No error includes inline content, file
contents, or key material. Key identifiers, file paths, and the trust directory may appear.

Configuration validator errors use the existing `ConfigError` format:

```text
configuration error: invalid value "<value>" for field "<path>" (expected: <expectation>)
```

For an inline key set, `<value>` is always `[inline JWK Set redacted]`.

Builder errors use this format:

```text
<path>: <category description>
```

| Category | Message (after `<path>: `) |
|---|---|
| `conflicting_key_sources` | `only one of jwks_uri, jwks, jwks_file may be set (found: jwks_uri, jwks)` |
| `refresh_bounds_with_inline` | `jwks_min_refresh and jwks_max_refresh are not allowed with jwks; an inline key set never refreshes` |
| `verification_disabled_with_key_source` | `verification 'none' and jwks are mutually exclusive` (or `jwks_file`, or the existing `jwks_uri` message) |
| `relative_path` | `jwks_file "<path>" must be an absolute path` |
| `missing_trust_directory` | `jwks_file requires security.trust_material_dir to be set` |
| `invalid_trust_directory` | `security.trust_material_dir "<dir>" must be an absolute path to an existing directory other than /` |
| `outside_trust_directory` | `jwks_file "<path>" is outside security.trust_material_dir "<dir>"` |
| `not_found` | `jwks_file "<path>" does not exist` |
| `unreadable` | `jwks_file "<path>" cannot be read` |
| `not_regular_file` | `jwks_file "<path>" is not a regular file` |
| `too_large` | `key set exceeds 1048576 bytes` |
| `not_a_jwk_set` | `key set is not a valid JWK Set document` |
| `empty_key_set` | `key set contains no keys` |
| `symmetric_key` | `key set contains a symmetric key; only public asymmetric keys are allowed` |
| `private_key` | `key set contains private key material; only public keys are allowed` |
| `unapproved_algorithm` | `key "<kid>" declares an algorithm outside the approved asymmetric set` |
| `missing_kid` | `key at index <zero-based index> requires a non-empty kid` |
| `duplicate_kid` | `key identifier "<kid>" appears more than once` |

For key-set rules, `<path>` is the source setting: `….jwks` or `….jwks_file`. A missing or empty key `kid` reports only its zero-based index, not key material. An unapproved-algorithm key without `kid` reports `unapproved_algorithm` first, using its index.

Unchanged startup errors also apply to every source: the self-trust rejection (FR-014), a missing
issuer for local-mode approval authentication, and JWT verification with no key source.

## 3. Log events

All events are structured `slog` records. None includes inline content, file contents, or key
material.

| Event message | Level | When | Fields |
|---|---|---|---|
| `trust anchor key set loaded` | Info | Startup, once per trust anchor with an inline or file source | `trust_anchor`, `key_source` (`inline` or `file`), `path` (file only), `key_count`, `key_ids` |
| `trust anchor key set changed` | Info | A file re-read that changes the key identifiers | `trust_anchor`, `key_source=file`, `path`, `key_count`, `added_key_ids`, `removed_key_ids` |
| `trust anchor key set re-read failed` | Warn | A failed file re-read | `trust_anchor`, `key_source=file`, `path`, `error_kind` (category), `last_success_at`, `fails_closed_at` |
| `trust anchor key set recovered` | Info | First valid re-read after a failure | `trust_anchor`, `key_source=file`, `path`, `key_count` |
| `trust anchor key set expired` | Warn | First `GetKeySet` call after the staleness window expires | `trust_anchor`, `key_source=file`, `path`, `error_kind=stale` |

A re-read with unchanged content logs nothing.

## 4. Behaviour invariants

| Invariant | Requirement |
|---|---|
| An inline or file source performs no discovery and no outbound request for that trust anchor | FR-004, SC-002 |
| Credentials verified against valid local and remote sets use the same key selection and claim policy | FR-005, SC-003 |
| A local source is used only by the trust anchor that declares it, is never published at `/oauth2/jwks.json`, and never validates subject tokens | FR-016 |
| Impersonation entries share an adapter only when every source attribute matches | FR-017 |
| Proxy or hybrid mode with no `issuer_uri` and a local source: the expected issuer is the upstream issuer; client assertions verify only against the local keys | FR-015 |
| The self-trust check applies to every source | FR-014, SR-005 |
