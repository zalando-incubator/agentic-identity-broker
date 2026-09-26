# Research: ExtProc Metadata Input

**Feature**: 043-extproc-metadata-input | **Date**: 2026-09-14

All Technical Context unknowns are resolved below. Each finding is grounded in the pinned
agentgateway release (`cr.agentgateway.dev/agentgateway:v1.5.0`, tag `v1.5.0` of
`github.com/agentgateway/agentgateway`) or in current repository source.

## Research Tasks & Findings

### R1: How does agentgateway serialize `metadataContext` into `ProcessingRequest`?

**Decision**: Read both inputs from `req.MetadataContext.FilterMetadata["aib.tokenexchange"].Fields`,
as `structpb.Value` entries keyed `subject_token` and `resource_uri`.

**Rationale**: `ExtProc.metadata_context` is declared as
`Option<HashMap<String, HashMap<String, Arc<cel::Expression>>>>` — a namespace-to-field map of CEL
expressions (`crates/agentgateway/src/http/ext_proc.rs`, `ExtProc` struct). At request time
`build_processing_metadata_context()` evaluates every namespace through `eval_to_struct()` and emits
`Metadata { filter_metadata: HashMap<namespace, Struct> }`. Namespace keys are plain map keys, so a
dotted namespace such as `aib.tokenexchange` is legal and is **not** split into nested structs. Each
field value passes through `eval_expression()` →
`res.json()` → `envoy_proto_common::json_to_prost_value()`, so a CEL string becomes a
`structpb.Value` of kind `StringValue`.

This is exactly the shape the service already consumes for `agentgateway.protocol`
(`internal/extproc/server/server.go:825-847`), so the new namespace reuses a proven access pattern.

**Alternatives considered**:
- *Request attributes* (`ExtProc.request_attributes`): agentgateway forces these into the reserved
  `envoy.filters.http.ext_proc` namespace (`build_request_attributes()`), which is an Envoy-owned
  namespace. Rejected — squatting on a reserved namespace.
- *gRPC initial metadata* (`agentgateway.dev.grpc_initial_metadata`): resolved once per stream, not
  per request, and carries no per-request token. Rejected.

### R2: What happens when a `metadataContext` CEL expression fails to evaluate?

**Decision**: Treat an absent field exactly like an invalid field — reject with the matching 503.
No separate "evaluation failed" signal exists or is needed.

**Rationale**: `eval_to_struct()` uses `filter_map`; an expression that errors is logged by the
gateway and **silently dropped from the struct** (`ext_proc.rs`, `eval_to_struct`). Consequently
`jwt.rawToken.unredacted()` evaluated on a request with no validated JWT produces an *absent*
`subject_token` field, not a null or empty one. A whole namespace whose struct fails to build is
dropped by `build_processing_metadata_context()`'s `.ok()` filter. The wire therefore offers only
three observable states per field — present-and-string, present-and-other-kind, absent — and FR-008
collapses the latter two into one rejection. This satisfies SR-002 (fail closed) without any
gateway-side error channel.

### R3: Is `jwt.rawToken.unredacted()` available in the pinned release, and what does it return?

**Decision**: Yes. Document `jwt.rawToken.unredacted()` as the required producer expression and
explicitly warn that bare `jwt.rawToken` is wrong.

**Rationale**: `crates/agentgateway/src/http/jwt.rs:520-537` exposes `rawToken` with the description
"The raw bearer token. Redacted by default; use `jwt.rawToken.unredacted()` to access the actual
value", implemented as `crate::cel::secret_string_to_value(&self.jwt)`. That helper wraps the token
in `Value::Dynamic(SecretStringValue)` (`crates/agentgateway/src/cel/types.rs:1774`), whose
`call_function` implements exactly one method, `unredacted`, returning
`Value::String(self.0.expose_secret())` (`types.rs:1758-1767`). Bare `jwt.rawToken` therefore does
not yield the credential; it yields a dynamic value whose JSON projection is a redaction
placeholder. The guide must say so, because the failure mode is a silent exchange rejection at the
broker rather than a configuration error at the gateway.

`rawToken` is populated only from the `Claims` the JWT policy inserts into request extensions
(`jwt.rs:631`), which is why FR-017/SR-005 require the JWT policy to precede the ExtProc policy.

### R4: Why can the subject token not simply be read from the Authorization header?

**Decision**: Record in ADR 036 that the raw header is not merely untrusted — with a strict JWT
policy it is **absent**.

**Rationale**: `Jwt::apply()` removes the validated credential from its location unless
`preserveToken` is set: `if !self.preserve_token { self.location.remove(req)... }`
(`jwt.rs:618-622`), and `preserve_token` defaults to `false`
(`LocalJwtSingleConfig`/`LocalJwtMultiConfig`, `#[serde(default)]`). So in the very configuration
the spec mandates — a `Strict` JWT policy in front of the ExtProc policy — the Authorization header
has already been stripped by the time ExtProc sees the request.

**Does stripping break the producer expression?** No — and this is the question the design turns on.
`Claims` carries its own copy of the credential, `pub jwt: SecretString` (`jwt.rs:505-508`), and the
`rawToken` accessor reads that field rather than the request:
`"rawToken" => Some(crate::cel::secret_string_to_value(&self.jwt))` (`jwt.rs:535-540`). The ordering
inside `Jwt::apply()` confirms the independence: the header is removed at `jwt.rs:618-622`, and only
afterwards is `req.extensions_mut().insert(claims)` executed at `jwt.rs:631`. The claims — with the
raw token — are attached *after* the header is already gone.

The two facts are therefore complementary, not contradictory. Header stripping destroys the raw
attribute path; the `Claims`-held copy keeps `jwt.rawToken.unredacted()` resolvable for the ExtProc
`metadataContext` expressions evaluated later in the same request.

Stated precisely: the `Claims` extension is a surviving *in-process* carrier inside agentgateway,
but it does not cross the gRPC boundary — only what `metadataContext` projects into
`MetadataContext.FilterMetadata` does. Dynamic metadata is thus the only carrier of a
gateway-validated credential **exposed to ExtProc**. The `Claims` copy is what makes that projection
possible; it is not an alternative source ExtProc could read instead. This is the decisive argument
for the clean cutover and belongs in the ADR.

**Consequence for the E2E**: setting `preserveToken: false` explicitly (R8) makes the Docker
scenario prove exactly this — the upstream MCP server must observe the *exchanged* token, and a
request that reached ExtProc with raw extraction re-enabled would have nothing to extract.

### R5: Should validation happen before or after OPA evaluation in OPA mode?

**Decision**: Validate both metadata inputs first, before protocol-metadata checks, MCP transport
checks, and any OPA evaluation.

**Rationale**: FR-015 fixes the relative order of the two metadata validations
(`subject_token` before `resource_uri`), and SR-002 requires fail-closed behavior. Validating first
also means a misconfigured gateway is rejected without spending an OPA evaluation or a broker call,
and it keeps the two header paths (OPA and non-OPA) structurally identical up to the point where
they diverge. The consequence — currently-passing E2E case "missing protocol metadata returns 403"
now needs valid `aib.tokenexchange` metadata to reach its assertion — is a test fixture change, not
a behavior regression.

**Alternatives considered**: validating after OPA so that policy could deny first. Rejected: OPA
input does not depend on either token-exchange input, so evaluating first only adds latency on the
misconfiguration path and weakens the fail-closed guarantee.

### R6: What replaces the "no Bearer token → pass through" branch?

**Decision**: Remove it from both header paths. Absent subject-token metadata is a 503
`invalid_subject_token`, never a pass-through.

**Rationale**: The spec's Assumptions are explicit — "If either required metadata value is absent,
ExtProc rejects the request. This applies even when the request has no raw authorization
credential." Pass-through existed only because an unauthenticated request had no token to exchange;
under the metadata contract the gateway is the authority on whether a request is in scope, and a
gateway that routes a request through this ExtProc policy has asserted that it is. Keeping a
pass-through would reintroduce exactly the ambiguous precedence FR-011 forbids.

**Impact**: the telemetry `outcome` value `passthrough` (`server.go:433`) becomes unreachable and is
removed; `invalid_subject_token` is added to the outcome and `error.type` vocabularies.

### R7: How should the E2E suite supply a validated JWT to a Docker agentgateway container?

**Decision**: Mount a static JWKS **file** next to `config.yaml` in the testcontainer and configure
`jwtAuth.providers[0].jwks.file`. Mint the matching RS256 token inside the test. No JWKS HTTP server.


**Rationale**: `jwks` deserializes to `serdes::FileInlineOrRemote`, which accepts
`{ file: <path> }`, a bare inline string, or `{ url: ... }` (`crates/agentgateway/src/serdes.rs:30-41`).
The existing helpers already write `/config.yaml` into the container via
`testcontainers.ContainerFile` (`tests/e2e/extproc/agentgateway_e2e_test.go:396-407`), so adding a
second `ContainerFile` for the JWKS is a two-line change with no new network surface, no
`HostAccessPorts` entry, and no flakiness from container→host JWKS fetches. The `url` form used in
the spec's illustrative snippet stays in the operator documentation, where a real issuer applies.

**Scope discipline**: the real-gateway test is a *bounded* fixture. It exists to prove the
gateway-level contract — FR-017 policy ordering and the US1-S1/US2-S1 end-to-end path — and nothing
else. All validation-rule and precedence coverage stays in the fast direct-gRPC tests. Replacing the
JWT policy with a CEL header extraction, or omitting JWT validation to make the fixture cheaper,
would test a configuration the spec forbids and is not an acceptable simplification.

**Alternatives considered**: an httptest JWKS server reachable through
`host.testcontainers.internal`. Rejected — strictly more moving parts for identical coverage.

### R8: Do the existing Docker agentgateway configurations need a JWT policy?

**Decision**: Yes — all three must gain a `jwtAuth` policy and an `aib.tokenexchange` producer, or every Docker-based ExtProc scenario fails closed. **Their JWKS provisioning is not the same mechanism.**


**Rationale**: None of the three active gateway configurations produces the new namespace today:
`tests/e2e/extproc/agentgateway_e2e_test.go:378-386` (no `jwtAuth`, no `metadataContext`), `tests/e2e/extproc/opa_agentgateway_e2e_test.go:839-845` (`metadataContext` with `agentgateway.protocol` only), and `mocks/agentgateway/config.yaml:22-27` (same). After the cutover, any request through these gateways is rejected with `invalid_subject_token`.

The first two are generated in Go and written into a container at runtime, so R7's `jwks.file`
approach applies directly. The third is different in kind: `mocks/agentgateway/config.yaml` is a
committed file bind-mounted read-only by Compose (`docker-compose.yml:250-252`), and the gateway
container's filesystem contains nothing the testcontainer helpers put there. Pointing it at a
testcontainer path would leave `just compose-extproc-up` with a gateway that fails at startup.

**Compose decision**: commit `mocks/agentgateway/jwks.json` and bind-mount it read-only in
`docker-compose.yml` beside the existing `config.yaml` mount; reference it with `jwks.file`. Commit
the matching RSA private key as `mocks/agentgateway/jwks-dev-key.pem`, clearly labelled as a
non-secret development fixture, so a developer can mint a token and actually drive traffic through
port 4000. This matches the mount pattern every other Compose service already uses
(`configs/config.extproc.docker.yaml`, `mocks/opa/opa-config.yaml`, `mocks/mcp-server/config.yaml`) and
keeps the gateway's boot contract self-contained: no cross-service startup ordering, no network
fetch, no YAML-embedded JSON blob.

**Alternatives considered**:
- *Inline JWKS string* in `config.yaml` (`FileInlineOrRemote::Inline`). Rejected — it embeds a JSON
  blob inside a YAML scalar in the one gateway file developers read to understand the stack, for no
  gain over a sibling mount.
- *Point `jwks.url` at a live issuer on the Compose network.* The broker already serves
  `http://identity-broker:8000/oauth2/jwks.json` (`internal/adapters/http/routing/enduser.go:162-164`)
  and is the OAuth2 authorization server that issues the agent's inbound token, so this is the most
  production-faithful shape and needs no new files. Rejected for the development stack because it
  couples gateway startup to broker readiness: `agentgateway` currently declares no `depends_on` for
  `identity-broker`, whose healthcheck allows a 4-minute `start_period`
  (`docker-compose.yml`). Making the gateway's boot depend on that trades a deterministic
  stack for a fragile one. The `url` form remains the documented shape for real deployments in
  `docs/guides/token-exchange-gateway.md`.

**Also decided**: set `preserveToken: false` explicitly in every gateway configuration. It is the
current upstream default (R4), but this feature's correctness argument rests on it, so the
configuration should assert it rather than inherit it.

### R9: Is any configuration, Helm, or API surface affected?

**Decision**: No. No new `EXTPROC_` key, no chart change, no OpenAPI change.

**Rationale**: Both inputs are per-request wire data, not service configuration. The sidecar schema
in `examples/config/extproc-token-exchange.yaml` contains no extraction settings to change, and
`charts/agentic-identity-broker/` deploys only the broker — it renders no ExtProc or gateway
configuration. The ExtProc gRPC surface is the Envoy `ExternalProcessor` service, which is not an
end-user or admin HTTP API, so Principles IV and X have no artifact to update. What *does* change is
a wire contract with the gateway, which is why this feature ships a contract document and an ADR
instead.

### R10: Is `validateResourceURI` reusable as-is?

**Decision**: Reuse the validation logic; reword its error strings away from `:path`.

**Rationale**: The rules FR-005 requires — non-empty, parses as an absolute request URI, `http` or
`https` scheme, non-empty host — are exactly what `validateResourceURI` already enforces
(`server.go:952-971`), and its SR-001 discipline of never echoing the raw URI into error text is
directly reusable for SR-003. Only the operator-facing wording is stale: messages such as
`"empty :path"` and `":path must have a non-empty host"` name a source that no longer exists. Its
white-box tests (`internal/extproc/server/security_test.go:21-60`) stay valid apart from those
message assertions.

`buildResourceURI` has no successor — URI assembly from `:scheme`/`:authority`/`:path` is precisely
the behavior FR-010 removes.
