# Research: Inline and File-Based JWKS for Trust Anchors

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-10-16

This document resolves the design questions for feature 052. Each section gives the decision, the
rationale, and the alternatives that were considered. File references point at the code as of
`0e2e024` on `main`.

## Current-state findings

These facts constrain the design.

| Area | Finding | Source |
|---|---|---|
| Client-assertion config | `ClientAssertionTrustConfig` has `issuer_uri`, `jwks_uri`, `jwks_min_refresh`, `jwks_max_refresh`. Each key has an explicit `BindEnv`. | `internal/ports/config.go:781-786`, `internal/config/loader.go:281-285` |
| Client-assertion wiring | The builder reuses the shared upstream adapter only when all four fields are empty. Otherwise it discovers `jwks_uri` through RFC 8414 metadata and creates a dedicated `jwks.Adapter`. | `internal/app/builder.go:677-738` |
| Client-assertion rules in builder | The proxy/hybrid issuer fallback, the self-trust check, and the approval-auth issuer requirement run in the builder, not in the config validator. | `internal/app/builder.go:490-510` |
| Impersonation config | `TrustedTokenIssuerConfig` has the same four key fields plus `allowed_algorithms` and `signs_roles`. Rule lists have no environment override. | `internal/ports/config.go:760-773`, `internal/config/loader_test.go:264-302` |
| Impersonation wiring | `newImpersonationJWKSFactory` shares one adapter per `{issuer_uri, jwks_uri, min, max}` across rules. Its adapters are not added to `App.Shutdown`. | `internal/app/builder.go:1284-1335` |
| JWT pre-authentication | `JWTConfig` serves both `server.enduser` and `server.admin`. The jwtauth adapter uses its own `jwkfetch` cache with a 5-minute minimum and 15-minute maximum refresh, and fetches synchronously at startup. | `internal/ports/config.go:131-168`, `internal/adapters/jwtauth/jwx_authenticator.go:27-31,88-127` |
| Remote key-set adapter | `jwks.Adapter` caps responses at 1 MiB, parses with strict key-set parsing, serves cached keys until the next scheduled refresh after the last success, and then fails closed. | `internal/adapters/jwks/adapter.go:24-83,196-267` |
| Domain consumption | Token exchange and impersonation consume `tokenexchange.JWKSProvider` and call only `GetKeySet`. jwx `WithKeySet` requires a token `kid` and skips JWKs without `alg`. | `internal/domain/tokenexchange/jwt_validator.go:20-26,229-236` |
| Approved algorithms | Only impersonation has an allow-list: RS/PS/ES 256/384/512 and EdDSA. | `internal/domain/impersonation/constants.go:31-49` |
| Health | `/health` reports only `components.upstream_jwks`, and only in proxy and hybrid modes. Components never change the HTTP status. Trust-anchor adapters' `HealthState()` is not surfaced. | `internal/app/builder.go:108-114`, `internal/adapters/http/health.go:68-105` |
| Configuration errors | `ConfigError.Error()` echoes the rejected value: `invalid value %q for field %q`. | `internal/domain/config/errors.go:18-39` |
| Helm | One ConfigMap renders `config.yaml`. The chart has no generic volume mechanism. The values schema closes the client-assertion, trusted-issuer, and JWT objects with `additionalProperties: false`. | `charts/agentic-identity-broker/templates/configmap.yaml`, `values.schema.json:976-1018,1106,1538-1585` |
| File access | No broker code reads operator-named files other than `.env` and `config.yaml`. Nothing uses `os.Root` or `filepath.EvalSymlinks`. | `internal/config/loader.go:349-475` |

## R1. Where the key-source rules live

**Decision**: Add a new domain bounded context, `internal/domain/trustanchor/`. It owns:

- the `KeySetSource` value object and its resolution from configuration fields;
- parsing and invariant validation of a `LocalKeySet`;
- the broker-approved asymmetric algorithm set;
- key-identifier differences between two local key sets, which re-read logging uses.

The package does no I/O. The builder calls it at startup for every trust anchor. The configuration
validator also calls its structural checks, so YAML errors appear before the builder runs.

**Rationale**:

- E2E tests build `*ports.Config` directly and call `app.Builder.Build()`. They bypass the loader
  validator. The builder path must therefore enforce every startup rule in FR-007, FR-009, FR-010,
  FR-012, FR-013, FR-022, and FR-023. One domain function serves both callers, which avoids
  duplicated rules.
- Constitution Principle VI forbids an anemic domain. Key-set invariants (non-empty, public
  asymmetric keys only, approved algorithms, unique key identifiers, size bound) are business rules,
  so they belong in the domain.
- Three trust anchors in three existing contexts (`tokenexchange`, `impersonation`, `jwtauth`) need
  the same rules. A separate context prevents one context from importing another for an unrelated
  concern.

**Alternatives considered**:

- *Validation only in `internal/config/validator.go`*: E2E startup tests would not exercise it, and
  `ConfigError` would echo inline key material.
- *Validation inside each adapter*: three copies of the same invariants, and domain rules would leak
  into infrastructure.
- *Extending `internal/domain/tokenexchange/`*: impersonation and JWT pre-authentication would then
  depend on the token-exchange context for trust-anchor rules.

## R2. How an inline key set is served

**Decision**: Add `jwks.StaticAdapter` in `internal/adapters/jwks/static_adapter.go`. It holds one
validated `jwk.Set` for the process lifetime. It implements `ports.JWKSPort`, so it also satisfies
`tokenexchange.JWKSProvider`. `HealthState()` always returns `healthy`. `GetKey` uses the same
`kid` lookup as `jwks.Adapter`.

**Rationale**: The domain already consumes the `JWKSProvider` contract from ADR 008. Supplying the
same contract keeps validation source-agnostic (FR-005). An inline set has no external dependency,
so it never reports degraded health (FR-019). The adapter performs no network I/O (FR-004).

**Alternatives considered**:

- *An `httprc` resource with a fake transport*: it adds refresh machinery that FR-012 forbids and
  that has nothing to refresh.
- *Passing a raw `jwk.Set` to the domain*: it would change the domain constructors for one source
  kind and break the adapter pattern.

## R3. File re-read cadence and staleness

**Decision**: Add `jwks.FileAdapter` in `internal/adapters/jwks/file_adapter.go`.

- At construction it reads and validates the file once. Any failure fails startup.
- A background goroutine re-reads the file every `jwks_min_refresh`. This satisfies both bounds in
  FR-011: reads are never closer than the minimum and never further apart than the maximum.
- A valid re-read replaces the whole set atomically and clears the failure state.
- A failed re-read keeps the last valid set and sets the adapter to `degraded`.
- The adapter serves the last valid set until `jwks_max_refresh` has elapsed since the last
  successful read. After that, `GetKeySet` fails closed until a valid re-read succeeds.
- `Shutdown` stops the goroutine. The builder chains it into `App.Shutdown`.

Effective bounds per trust anchor:

| Trust anchor | Re-read interval | Staleness window | Source |
|---|---|---|---|
| Client assertion | `jwks_min_refresh`, default 15m | `jwks_max_refresh`, default `max(min, 1h)` | ADR 029 defaults, `builder.go:707-718` |
| Impersonation trusted issuer | `jwks_min_refresh`, default 15m | `jwks_max_refresh`, default `max(min, 1h)` | `builder.go:1316-1327` |
| End-user JWT pre-authentication | 5m | 15m | `jwx_authenticator.go:27-31` |

The existing rule that raises a maximum below the effective minimum to the minimum still applies.

**Rationale**:

- The remote adapter never serves a key set later than its next scheduled refresh after the last
  success. That refresh is at most `jwks_max_refresh` later. Using `jwks_max_refresh` since the last
  successful read as the file staleness window applies the same upper bound (FR-011).
- Re-reading at the minimum interval gives a degraded-but-serving window of `max - min`, which is
  45 minutes by default. This matches the spec's state diagram: Current, Degraded, FailedClosed.
  When an operator sets the minimum equal to the maximum, the window is zero by their choice.
- A background read keeps health accurate when no traffic arrives. A lazy read would report a
  broken file as healthy until the next request.
- A replacement takes effect within `jwks_min_refresh`, which is inside SC-007's bound of one
  maximum refresh interval.

**Alternatives considered**:

- *Filesystem notifications (`fsnotify`)*: a new dependency, unreliable across Kubernetes symbolic
  link swaps, and outside the clarified behaviour (re-read within refresh bounds).
- *Lazy re-read on `GetKeySet`*: health would go stale without traffic, and FR-011 requires
  re-reads at least once per maximum interval.
- *Re-read every `jwks_max_refresh` and use the remote adapter's exact staleness test*: the first
  failed re-read would fail closed immediately. There would be no Degraded state.
- *Immediate retry after a failed read*: FR-011 forbids reads more often than the minimum interval.

## R4. Trust-directory containment

**Decision**: Use Go's `os.Root` (Go 1.24 and later; the project uses Go 1.27.1).

At startup:

1. Require `security.trust_material_dir` to be an absolute path. Resolve it with
   `filepath.EvalSymlinks`. It must be an existing directory and must not be `/` (FR-022).
2. Open it once with `os.OpenRoot`. Keep the `*os.Root` for the process lifetime and close it on
   shutdown.

For every `jwks_file`, at startup and on every re-read:

1. Require an absolute path (FR-010).
2. Apply the lexical check: `filepath.Clean(path)` must lie under `filepath.Clean(trust_material_dir)`.
   Compute the relative name and require `filepath.IsLocal`. On failure, read nothing (FR-023
   "Configured path").
3. Open the relative name through `root.Open`. `os.Root` follows symbolic links but refuses any
   component that resolves outside the root (FR-023 "Resolved file").
4. Call `Stat` on the opened handle and require a regular file (FR-010). The containment check,
   the regular-file check, and the read use the same file descriptor, as FR-023 requires.
5. Read at most 1 MiB plus one byte.

Probe result in this environment (Go 1.27.1, Linux):

| Name opened through `os.Root` | Result |
|---|---|
| `jwks.json` → `..data/jwks.json` → `..2026_10_12/jwks.json` (Kubernetes layout) | Opened, regular file |
| Relative link to `../outside.json` | `path escapes from parent` |
| Absolute link to a file outside | `path escapes from parent` |
| Absolute link to a file inside the directory | `path escapes from parent` |
| `../outside.json` | `path escapes from parent` |
| A subdirectory | Opened, not a regular file, so the broker rejects it |

**Consequence**: An *absolute* symbolic link is rejected even when its target is inside the trust
directory. Relative links, which Kubernetes Secret, ConfigMap, and projected volumes use, work
(User Story 2, scenario 9). This is stricter than a literal reading of the "Symlinked file" edge
case. It fails closed, and it is recorded in ADR 040 and the configuration reference.

**Rationale**:

- `os.Root` is the standard-library containment primitive (Principle III). It resolves each path
  component relative to the directory descriptor, so retargeting a link between a check and a read
  cannot escape the directory.
- Opening the relative configured name follows the `..data` link at open time. A resolve-then-open
  approach can race with a Kubernetes atomic swap: the swap deletes the old timestamped directory
  between the two steps, which causes a spurious failed re-read.

**Alternatives considered**:

- *`filepath.EvalSymlinks` plus a prefix check, then `os.Open`*: time-of-check to time-of-use gap,
  and the race with Kubernetes swaps described above.
- *`openat2` with `RESOLVE_BENEATH` through `golang.org/x/sys`*: Linux-only. Darwin is a supported
  development platform.
- *Lexical checks only (`filepath.IsLocal`)*: does not resolve symbolic links (FR-023 violation).
- *Resolve with `EvalSymlinks` to accept absolute in-directory links, then open the resolved
  relative name through `os.Root`*: keeps containment but reintroduces the swap race and adds a
  second code path for a layout no supported deployment package produces.

## R5. Local key-set parsing and validation

**Decision**: `trustanchor.ParseLocalKeySet(doc []byte) (LocalKeySet, error)` applies these steps in
order. Each failure returns a fixed category error that never wraps library error text.

1. Reject a document larger than `trustanchor.MaxKeySetBytes` (1 MiB). The remote adapter's
   constant then refers to this value, so there is one size bound.
2. Reject a document whose top-level JSON value is not an object with a `keys` array. This uses a
   structural `encoding/json` probe of the outer object only.
3. Parse with `jwk.Parse(doc, jwk.WithStrictKeySetParsing(true))`.
4. Reject an empty set.
5. For each key:
   - reject a symmetric key (`jwk.SymmetricKey`, `kty: oct`);
   - reject a private key (`jwk.IsPrivateKey`);
   - when `alg` is present, require it to be in the approved set (R6).
6. Reject a key with a missing or empty `kid`, reporting `missing_kid` and its zero-based index only. Then reject duplicate `kid` values, naming the duplicate identifier. Algorithm validation runs first: an unapproved `alg` on a key without `kid` reports `unapproved_algorithm` with its index.

Probe results with jwx v4.5.0:

| Input | jwx behaviour | Required broker behaviour |
|---|---|---|
| Bare JWK without `keys` | Parsed as a one-key set | Reject: not a JWK Set (FR-006, FR-007) |
| `{"keys":[]}` | Parsed, length 0 | Reject: no keys |
| Two keys with the same `kid` | Parsed, length 2 | Reject: duplicate identifier |
| `kty: oct` key | Parsed; `IsPrivateKey` returns an error | Reject: symmetric key |
| EC private key | Parsed; `IsPrivateKey` returns true | Reject: private key material |
| Unknown `kty` | Error text quotes the input value (`invalid key type from JSON (XYZ)`) | Reject with a fixed message |
| Malformed JSON | Error text quotes input characters | Reject with a fixed message |

**Rationale**:

- jwx does the key parsing and classification (Principle III, SR-003). The broker adds only
  invariant checks on parsed keys.
- `jwk.WithRejectDuplicateKID` is a *global* parse option. Setting it would change remote key-set
  parsing across the process, so the broker checks uniqueness itself.
- jwx error text can quote document content. FR-008 forbids any part of the content in errors and
  logs, so the broker maps parse failures to fixed categories.
- A key without `alg` stays valid at startup. Selection then follows each trust anchor's existing
  rules. Token exchange and impersonation skip a key without `alg`, exactly as they do for a remote
  key set. The configuration reference recommends `alg` on every key.
- A local key without `kid` cannot match a credential: existing verifiers require a token `kid`.
  Rejecting it at startup does not change remote key-set acceptance or verifier configuration.

**Alternatives considered**:

- *`jwk.PublicSetOf`*: it silently drops private parts and `oct` keys instead of rejecting them.
  SR-002 requires rejection.
- *Requiring `alg` on every key*: this would reject local keys that remote sources accept, even
  though `alg` remains optional in the local document contract. `kid` is required locally because
  a key without it cannot match any credential under the existing verifiers.

## R6. Broker-approved asymmetric algorithm set

**Decision**: Move the approved set (RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384, ES512,
EdDSA) to `trustanchor.IsApprovedAlgorithm`. `impersonation.IsApprovedAlgorithm` delegates to it, so
its callers and messages do not change.

**Rationale**: FR-007 refers to one broker-approved set. Moving it keeps a single definition (Code
Style: extend, do not duplicate) and keeps the dependency direction from `impersonation` to the new
context.

**Alternatives considered**: *`trustanchor` importing `impersonation`*: the general concept would
depend on an optional, local-mode-only feature.

## R7. Error and log hygiene

**Decision**:

- Startup errors name the full configuration path, for example
  `token_exchange.client_assertion.jwks`,
  `oauth2_authorization_server.impersonation.rules[0].trusted_issuers[1].jwks_file`, or
  `server.enduser.authentication.jwt.jwks`.
- When a `ConfigError` concerns an inline key set, its `Value` is the fixed placeholder
  `[inline JWK Set redacted]`. A `jwks_file` path, the trust directory, and key identifiers are not
  secret and may appear (FR-018, User Story 2 scenarios 2, 6–8).
- Error categories: `conflicting_key_sources`, `not_a_jwk_set`, `empty_key_set`, `too_large`,
  `private_key`, `symmetric_key`, `unapproved_algorithm`, `duplicate_kid`,
  `refresh_bounds_with_inline`, `verification_disabled_with_key_source`,
  `relative_path`,
  `missing_trust_directory`, `invalid_trust_directory`, `outside_trust_directory`, `not_found`,
  `unreadable`, `not_regular_file`, and `stale` (runtime only). The exact messages are in
  [contracts/configuration.md](./contracts/configuration.md).
- Log events use the existing structured `slog` style with `trust_anchor`, `key_source`,
  `path` (file only), `key_count`, `key_ids`, `added_key_ids`, `removed_key_ids`, and `error_kind`.

**Rationale**: FR-008 and SR-004 forbid key material in errors, logs, audit, and health output. The
current `ConfigError` format echoes values, so inline values need an explicit placeholder.

## R8. Configuration surface and environment overrides

**Decision**:

| Key | Struct field | Environment variable |
|---|---|---|
| `token_exchange.client_assertion.jwks` | `ClientAssertionTrustConfig.JWKS` | `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS` (new `BindEnv`) |
| `token_exchange.client_assertion.jwks_file` | `ClientAssertionTrustConfig.JWKSFile` | `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_FILE` (new `BindEnv`) |
| `oauth2_authorization_server.impersonation.rules[].trusted_issuers[].jwks` | `TrustedTokenIssuerConfig.JWKS` | None. Rule lists have no environment override. |
| `oauth2_authorization_server.impersonation.rules[].trusted_issuers[].jwks_file` | `TrustedTokenIssuerConfig.JWKSFile` | None |
| `server.enduser.authentication.jwt.jwks` | `JWTConfig.JWKS` | None. `${VAR}` expansion in YAML applies. |
| `server.enduser.authentication.jwt.jwks_file` | `JWTConfig.JWKSFile` | None. `${VAR}` expansion in YAML applies. |
| `security.trust_material_dir` | `SecurityConfig.TrustMaterialDir` | `IDENTITY_BROKER_SECURITY_TRUST_MATERIAL_DIR` (new `BindEnv`) |

**Rationale**:

- FR-006 requires the *existing* override mechanism for each setting. Client assertion and the
  `security` section use explicit `BindEnv`. JWT pre-authentication has no bound keys. A bound key
  under the nil-able `jwt` pointer would also enable JWT authentication as a side effect of setting
  one variable, so it keeps `${VAR}` expansion.
- `${VAR}` expansion rejects newlines and shell metacharacters in the YAML value that holds the
  reference (`loader.go:612-627`); it does not check the substituted value. The configuration
  reference tells operators to write the reference as the whole value (`jwks: ${MESH_JWKS}`).
  Bound variables are not expanded and accept any valid JSON text.

## R9. JWT pre-authentication integration

**Decision**:

- Add an optional `KeySet ports.JWKSPort` field to `jwtauth.JWXAuthenticatorConfig`. When it is set,
  the authenticator skips the `jwkfetch` cache and the startup fetch. It verifies with
  `jwt.WithKeySet(set, jws.WithInferAlgorithmFromKey(true))`, the same options as the remote path.
- The builder creates the static or file adapter from `internal/adapters/jwks/` and injects it.
  This respects the cross-adapter import ban.
- Both `server.enduser.authentication.jwt` and `server.admin.authentication.jwt` accept `jwks` and
  `jwks_file`. The builder already constructs one authenticator per server in a loop, so each
  server resolves its own key source, gets its own adapter, and logs under its own configuration
  path. The two servers never share an adapter, even with identical sources.

**Rationale**: The original draft rejected local sources on the admin server, because FR-002
covered only end-user JWT pre-authentication. On 2026-10-16 the stakeholder chose to support the
admin server instead, and FR-002 and User Story 5 scenario 4 were updated. `JWTConfig` is already
shared by both servers, and the per-server loop makes support nearly free.

**Alternatives considered**: *Reject on admin* (the original draft): fail-closed but leaves an
admin-API proxy that signs locally with no option. *Silently ignore on admin*: misleading and not
fail-closed.

## R10. Impersonation per-entry isolation

**Decision**: Extend the factory's sharing key in `newImpersonationJWKSFactory` to
`{kind, issuer_uri, jwks_uri, inline document, jwks_file, min, max}`. Entries share an adapter only
when every element matches. Each new static or file adapter is chained into `App.Shutdown`.

**Rationale**: FR-017 requires entries with the same issuer and different key sources to use only
their own keys. Identical sources yield identical keys, so sharing is safe and preserves the
verification-cache behaviour that relies on provider pointer identity.

## R11. Key-set health reporting

**Decision**: Add one end-user `/health` component, `trust_anchor_key_files`.

- It appears only when at least one trust anchor uses `jwks_file`.
- It is `degraded` when any file-backed key set's last re-read failed or its staleness window
  expired. Otherwise it is `healthy`.
- Like `upstream_jwks`, it never changes the HTTP status or the top-level `status`.
- Inline key sets never contribute (FR-019).
- `App.EnduserHealthComponents()` merges it with `upstream_jwks`, including in local mode.

This is an additive change to the `/health` response contract. It requires stakeholder confirmation
before implementation (Principles IV and X). [contracts/health-endpoint.md](./contracts/health-endpoint.md)
records the proposed contract.

**Rationale**:

- FR-019 requires degraded health "through the same health reporting that remote key sets use".
  For the broker, that is the `components` map with `healthy` or `degraded` values.
- User Story 2 scenario 4 and SC-007 need an observable signal. Logs alone do not meet "reports
  degraded key-set health".
- Helm liveness and readiness probes call `/health`. Changing the HTTP status would restart pods
  during a temporary file problem and turn a degraded state into an outage.
- One aggregate component keeps the contract small. Logs identify the failing trust anchor and
  path (FR-018).

**Alternatives considered**:

- *One component per trust anchor*: more contract surface for little operational gain.
- *Logs only*: does not satisfy FR-019.
- *Also reporting remote client-assertion and impersonation adapters*: a useful but separate
  improvement, outside this feature's scope (see Deferred findings).

## R12. Builder integration

**Decision**:

- Resolve every trust anchor's `KeySetSource` before any discovery or adapter creation.
- Client assertion: the switch at `builder.go:684-737` gains `inline` and `file` branches. They
  skip discovery (FR-004). "Configuration unset" now also requires empty `jwks` and `jwks_file`.
  With an empty `issuer_uri` in proxy or hybrid mode, the expected issuer stays the upstream issuer
  while the adapter is local (FR-015). Subject-token validation keeps the published adapter
  (FR-016).
- The self-trust checks at `builder.go:494-499` and `:1037-1048` stay ahead of key-source loading
  and apply to every source (FR-014).
- Impersonation: the factory dispatches by `KeySetSource` kind (R10).
- JWT pre-authentication: R9.
- After each local source loads, log one `trust anchor key set loaded` event (FR-018).
- Open the trust directory only when at least one `jwks_file` exists. Otherwise do not touch the
  filesystem.

**Rationale**: Ordering the checks this way guarantees that no outbound request or file read happens
for a misconfigured trust anchor, and no source is shared across trust anchors (FR-016).

## R13. Helm deployment package

**Decision**:

- Add `broker.security.trustMaterial.directory`, a string with no default. It renders
  `security.trust_material_dir`.
- Add `broker.security.trustMaterial.sources`, a list of `{secret | configMap: {name, items}}`.
  It renders a read-only `projected` volume mounted at the directory, **without** `subPath`.
- Add `jwks` and `jwksFile` to `broker.tokenExchange.clientAssertion` and the ConfigMap template.
- Add `jwks` and `jwks_file` to the schema for `trusted_issuers[]` items and
  `serverConfig.authentication.jwt`. Those blocks pass through with `toYaml`.
- Extend `values.schema.json`, `values.yaml` comments, and `README.md`.

**Rationale**: FR-020 requires mounting existing cluster objects without a custom image. Kubernetes
does not update `subPath` mounts, so a `subPath` mount would break file rotation (User Story 2,
scenario 3). A projected volume uses the `..data` relative-link layout, which `os.Root` accepts
(R4).

**Alternatives considered**: *Generic `extraVolumes`/`extraVolumeMounts`*: broader than FR-020
needs, and the constitution forbids bundling chart enhancements. *Reusing `extraConfig`*: it would
render a duplicate top-level `security:` key.

## R14. Test strategy constraints

**Decision**:

- E2E tests generate keys in memory with the existing helpers. File scenarios write files into a
  per-test `GinkgoT().TempDir()` trust directory. Scenario 9 reproduces the Kubernetes
  `..data` layout there. No key file is committed.
- Zero outbound requests (User Story 1 scenario 1, SC-002): point the issuer at an `httptest` server
  that counts every request, and assert the count stays zero after startup and after verification.
- Startup failures: `serverFactory.BuildApp(storage)` returns an error whose text names the setting
  and contains no key material. Assert the absence of the base64url `x`, `y`, `n`, and `d` member
  values.
- Logs: `bootstrap.NewBufferedJSONLogger`.
- Health: poll `/health` for `components.trust_anchor_key_files` with `Eventually`.
- Rotation and failure: configure `jwks_min_refresh: 200ms` and `jwks_max_refresh: 2s` so that
  `Eventually` observes re-reads.
- User Story 2 scenario 5: render the chart with `helm template`, project the referenced Secret key
  into a temp trust directory with the Kubernetes layout, load the rendered `config.yaml` through
  the production config loader, and boot the broker. The `e2e-backend` CI job gets
  `azure/setup-helm`, and `just install-tools` documents Helm as an E2E prerequisite. `Skip()` is
  forbidden.

## R15. Architecture decision record

**Decision**: Draft `adrs/040-trust-anchor-local-key-sets.md`. It extends ADR 029 and ADR 008
without superseding them. It records:

- the three mutually exclusive key sources, with no precedence;
- the trust-directory boundary and `os.Root`, including the absolute-symbolic-link consequence;
- the file re-read cadence and staleness window;
- the `trust_anchor_key_files` health component;
- the end-user and admin server scope for JWT pre-authentication.

The ADR must be Accepted before implementation tasks start (Principle II and the spec's
Dependencies section). Number 040 is the next free number after `039-concurrent-session-expiry-index.md`.

## Deferred findings (pending user decision)

Research found these issues outside the feature's required work. Per the constitution's
Feature Scope rules, they are not planned as tasks. The user decides whether to file GitHub issues.

1. **No algorithm allow-list for remote token-exchange and JWT pre-authentication key sets.** If a
   remote JWKS published a symmetric `oct` key with an `alg`, jwx would accept HS* signatures with
   it. `jwtauth` also infers the algorithm from the key (`jwx_authenticator.go:232-235`). Local key
   sets in this feature reject symmetric keys at startup, so the feature does not inherit the gap.
2. **Impersonation remote JWKS adapters are never shut down**, and several builder error paths
   return without calling `App.Shutdown` (`builder.go:1028-1053,1284-1335`). This feature chains
   shutdown for its own new adapters only.
3. **Remote trust-anchor key-set health is not surfaced in `/health`** for the client-assertion,
   impersonation, and JWT pre-authentication adapters.
4. **ADR 029 says discovery uses "OAuth2/OIDC metadata"**, but `DiscoverOAuth2Endpoints` reads only
   RFC 8414 `/.well-known/oauth-authorization-server` (`discovery.go:85`).
