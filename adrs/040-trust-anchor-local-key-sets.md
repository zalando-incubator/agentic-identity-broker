# ADR 040: Inline and File-Based Key Sets for Trust Anchors

**Status**: Proposed
**Date**: 2026-10-16
**Feature**: 052-inline-jwks-trust-anchors
**Extends**: [ADR 029](029-token-exchange-client-assertion-trust-anchor.md), [ADR 008](008-token-exchange-jwks-adapter-pattern.md)

---

## Context

The broker verifies signed credentials against three kinds of trust anchor:

- the token-exchange client-assertion trust anchor (ADR 029), which also authenticates machine-facing approval requests;
- every trusted issuer of every impersonation rule;
- end-user JWT pre-authentication.

Each trust anchor obtains its public verification keys from a remote JWKS endpoint, either an explicit `jwks_uri` or an endpoint discovered from issuer metadata. ADR 008 abstracts that fetch behind `JWKSPort`, and the domain consumes only `GetKeySet`.

A locally signing gateway or service mesh has public keys but no network-reachable service that publishes them. Today the operator must deploy and secure an extra key-publishing service for each such issuer. The service becomes a runtime dependency of credential verification and an outbound request path for the broker.

Operators need to supply the public key set directly, as configuration text or as a file that a cluster Secret or ConfigMap provides. A file path read by the broker is a new capability. Nothing in the broker today reads operator-named files other than `.env` and `config.yaml`. An unrestricted path would let a trust-anchor setting make the broker read credentials, configuration, or platform tokens from its runtime environment.

## Decision

### Three mutually exclusive key sources

Each in-scope trust anchor accepts at most one explicit key source:

- `jwks_uri`: the existing remote source, unchanged.
- `jwks`: an inline RFC 7517 §5 JWK Set document.
- `jwks_file`: the absolute path of a file that contains a JWK Set document.

There is no precedence. A trust anchor that sets more than one source fails startup with an error that names the anchor and the conflicting settings. A trust anchor that sets none keeps today's behavior: discovery, or ADR 029's reuse of the upstream key set.

The source is resolved once at startup, before any discovery or adapter creation, and is immutable afterward. A local source performs no discovery and no outbound request. A failed local source never falls back to discovery or to another source. Credential validation is identical for all three sources; only the origin of the public keys differs.

A local key set is used only by the trust anchor that declares it. It is never published at `/oauth2/jwks.json`, never validates subject tokens, and is never shared with another trust anchor. Impersonation entries share an adapter only when every source attribute matches.

ADR 029's self-trust protection applies to every source. In proxy and hybrid modes, a client-assertion anchor with a local source and no `issuer_uri` keeps the upstream issuer as the expected issuer and verifies client assertions against the local keys only.

### New domain bounded context

`internal/domain/trustanchor/` owns source resolution, Local Key Set validation, the approved asymmetric algorithm set, and key-identifier differences between two key sets. It performs no I/O. The config validator and the app builder both call it, so E2E tests that bypass the loader enforce the same rules. `impersonation.IsApprovedAlgorithm` delegates to the shared approved set.

A Local Key Set must satisfy these rules. Each failure maps to a fixed category error that never quotes document content or library error text:

- at most 1 MiB, the bound that remote key sets already use;
- an object with a `keys` array that jwx parses with strict key-set parsing;
- at least one key;
- no symmetric (`oct`) key and no private key material;
- `alg`, when present, in RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384, ES512, or EdDSA;
- unique non-empty `kid` values.

`alg` and `kid` stay optional, as for remote key sets. Key selection follows each trust anchor's existing rules. The configuration reference recommends both members on every key. jwx parses, classifies, and verifies keys (Constitution Principle III); the broker adds invariant checks only.

An inline set with explicit `jwks_min_refresh` or `jwks_max_refresh` fails startup, because an inline set never refreshes. For JWT pre-authentication, `verification: none` is mutually exclusive with every explicit key source.

### Adapters behind `JWKSPort`

- `jwks.StaticAdapter` serves one validated set for the process lifetime and always reports `healthy`.
- `jwks.FileAdapter` reads and validates the file at construction and re-reads it in the background.
- `jwtauth.JWXAuthenticatorConfig` gains an injected optional key-set port. The builder creates the adapter and injects it, which respects the cross-adapter import ban.

The domain consumers keep depending on the ADR 008 contract.

### Trust directory boundary

One broker-wide setting, `security.trust_material_dir`, has no default. It must be an absolute path that resolves, after symbolic links, to an existing directory other than `/`. Any `jwks_file` requires it. When no `jwks_file` is configured, the broker does not touch the filesystem.

Containment uses Go's `os.Root`, the standard-library primitive (Principle III):

1. The broker opens the resolved trust directory once and keeps the handle for the process lifetime.
2. Every `jwks_file` must be absolute. The cleaned path must lie under the cleaned trust directory. If it does not, nothing is read.
3. The broker opens the relative name through the root. `os.Root` refuses any component that resolves outside the root.
4. The regular-file check and the read use the same open file descriptor.
5. The read is bounded to the size limit plus one byte.

These checks run at startup and on every re-read. Retargeting a link after startup cannot make the broker read an outside file; the re-read fails and the last valid key set stays in effect.

**Consequence**: absolute symbolic links inside the trust directory are rejected; relative links (Kubernetes) are accepted. `os.Root` refuses an absolute symbolic link even when its target is inside the directory. Kubernetes Secret, ConfigMap, and projected volumes use relative links (`jwks.json` to `..data/jwks.json` to a timestamped directory), so supported deployments work. This is stricter than a literal reading of "follow symbolic links to a file inside the directory". It fails closed, and the configuration reference documents it.

### File re-read cadence and staleness

- The broker re-reads a file every `jwks_min_refresh`. Reads are never closer than the minimum and never further apart than the maximum.
- Every re-read passes the full Local Key Set validation. A valid re-read replaces the whole set atomically. A partially valid document is never applied.
- A failed re-read keeps the last valid set and marks the adapter degraded.
- The last valid set is served until `jwks_max_refresh` has elapsed since the last successful read. After that, verification for that trust anchor fails closed until a valid re-read succeeds. This applies the same upper bound as a remote key set, which is never served past its next scheduled refresh.
- Defaults match ADR 029: 15 minutes and `max(jwks_min_refresh, 1 hour)`. JWT pre-authentication uses its fixed 5 and 15 minutes.
- Inline key sets never reload. Changing one requires a restart.

A background read keeps health accurate when no traffic arrives. Filesystem notifications are not used: they add a dependency and are unreliable across Kubernetes link swaps.

### Health component

`GET /health` on the end-user server gains one optional component, `trust_anchor_key_files`, with values `healthy` and `degraded`. It appears when at least one trust anchor uses `jwks_file`, in any OAuth2 mode. It is `degraded` when the latest re-read of any key-set file failed, including after the staleness window expired. Like `upstream_jwks`, it never changes the HTTP status or the top-level `status`, so liveness and readiness probes do not restart pods over a temporary file problem. Inline key sets never contribute. The admin server's `/health` is unchanged.

This is an additive change to `api/enduser/openapi.yaml`.

### Scope of JWT pre-authentication

Local key sets apply to `server.enduser.authentication.jwt` only. `JWTConfig` is shared with the admin server, so the admin server's `jwks` and `jwks_file` fail startup with the category `unsupported_trust_anchor`. Silently ignoring them would mislead an operator and would not fail closed. Extending local key sets to the admin server is a scope change that needs a separate decision.

### Diagnostics

Startup errors name the trust anchor's configuration path. An error about an inline key set never includes the document, and a `ConfigError` for one reports the placeholder `[inline JWK Set redacted]`. File paths, the trust directory, and key identifiers are not secret and may appear. Structured logs record, per trust anchor, the key source kind, file path, key count, and key identifiers at startup. They record key identifier differences when a re-read changes a file, and failures and recoveries. No error, log, audit record, or health output includes key-set content or key material.

### Deployment package

The Helm chart gains a trust directory value and a read-only projected volume that mounts cluster Secret or ConfigMap keys at that directory without `subPath`. Kubernetes does not update `subPath` mounts, so a `subPath` mount would break rotation.

## Alternatives Considered

- **Precedence among sources**: a silent winner hides misconfiguration after a key rotation. Mutual exclusion with a startup error is explicit.
- **Validation only in the config validator**: E2E tests build `*ports.Config` and call the builder directly, and `ConfigError` echoes values. The builder must enforce every rule, so the rules live in one domain package.
- **`filepath.EvalSymlinks` plus a prefix check, then `os.Open`**: leaves a time-of-check to time-of-use gap, and races with Kubernetes atomic swaps that delete the old timestamped directory between the two steps.
- **`openat2` with `RESOLVE_BENEATH`**: Linux-only. Darwin is a supported development platform.
- **Resolve links to accept absolute in-directory links**: reintroduces the swap race and adds a second code path for a layout that no supported deployment package produces.
- **Re-read lazily on `GetKeySet`**: health goes stale without traffic.
- **Re-read every `jwks_max_refresh`**: the first failed re-read would fail closed immediately, with no degraded window.
- **Change the HTTP status when a file is degraded**: restarts pods during a temporary file problem and turns a degraded state into an outage.
- **One health component per trust anchor**: more contract surface for little gain. Logs identify the failing trust anchor and path.
- **Support local key sets on the admin server**: low cost, but outside the approved scope.
- **Allow any readable absolute path**: lets a configuration setting direct the broker to read credentials or platform tokens.

## Consequences

### Positive

- A locally signing gateway or mesh can be a trust anchor without any network-reachable key-publishing service.
- Local sources add no outbound request and no discovery dependency for the trust anchors that use them.
- The new capability cannot read arbitrary files. The trust directory is a single operator-designated boundary that the standard library enforces.
- Kubernetes Secret, ConfigMap, and projected volumes work with rotation and no restart.
- A file problem is visible in logs and `/health` before verification fails closed.
- Existing deployments that adopt no new setting behave identically.

### Negative

- Absolute symbolic links inside the trust directory are rejected, even when their targets are inside.
- Inline key sets require a restart to change.
- File rotation takes effect within `jwks_min_refresh`, not immediately.
- A degraded file keeps serving the last valid key set for up to `jwks_max_refresh` since the last successful read. A key removed from the file because it is compromised stays trusted until a valid re-read succeeds.
- The admin server cannot use local key sets.
- The `/health` response contract and the Helm values schema grow.

### Risks

- A trust directory that holds more than trust material widens what a configuration mistake can expose. Choosing a narrow directory is the operator's responsibility. The broker rejects `/`.
- A key set without `alg` or `kid` is valid but may not match credentials, because each trust anchor keeps its existing key selection. The configuration reference recommends both members on every key.
- Operators who rely on `/health` for alerting must watch `components`, because the component never changes the HTTP status.

### Out of Scope

- Inline or file key sets for the proxy-mode upstream authorization server.
- Combining several key sources for one trust anchor.
- Private keys or symmetric secrets through configuration.
- Changes to the standalone ExtProc service.
