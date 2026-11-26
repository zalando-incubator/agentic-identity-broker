# Research: Service Accounts on Third-Party Services (`jwt_bearer` with a Stored JWT)

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-11-25

The Technical Context has no open `NEEDS CLARIFICATION` items. Each section records one design
decision, its rationale and the main alternatives. Decisions marked **Proposed** are not fixed by
the spec, issue #471 or issue #478; they are registered in [plan.md](./plan.md#approval-items) and
need the maintainer's sign-off.

## R1 — Where the domain code lives

**Decision**: The entity and its value objects go in `internal/domain/model/` next to
`ThirdpartyOAuth2ProviderEntity`. A new `thirdparty.ServiceAccountService` in the existing
`internal/domain/thirdparty/` package holds the workflow. No new `domain/X/` package.

**Rationale**: A service account belongs to exactly one third-party service for its whole life
(FR-001) and is encrypted under that service's context (SR-001). It is part of the third-party
service bounded context, not an independent one, so Constitution Principle VI ("each new
`domain/X/` is a genuinely independent bounded context") argues against a new package. A separate
service type keeps the 1100-line `ThirdpartyOAuth2ProviderService` unchanged apart from the delete
guard (R9).

**Alternatives considered**:

- New `internal/domain/serviceaccount/` package: would need the provider service for existence
  checks and encryption subjects, so it is not independent.
- Methods on `ThirdpartyOAuth2ProviderService`: grows an already large service and mixes two
  repositories behind one type.

## R2 — Repository port

**Decision**: One new interface `ports.ServiceAccountRepository` in `internal/ports/storage.go`
with seven methods: `Create`, `Get`, `GetByCanonicalID`, `ListByService`, `Update`, `Delete`,
`CountByService`. Every read and write is scoped by `service_id`. `Update` and `Delete` take an
optional expected version. Two sentinels classify conflicts without message parsing:
`ports.ErrServiceAccountCanonicalIDConflict` and `ports.ErrServiceAccountVersionMismatch`, each
wrapped in a `storage.StorageError` of kind `conflict`. Signatures are in
[data-model.md](./data-model.md#repository-port).

**Rationale**: `adrs/004-storage-layer-architecture.md` and Principle IX require a focused
interface per entity with at most 5–7 methods, defined in `internal/ports/storage.go`. Scoping
every method by service makes FR-021 (an account addressed under another service is not found) a
property of the query, not of handler code. Sentinels follow `ports.ErrExternalIDConflict`; the
existing service handler tells a version conflict apart with `strings.Contains(message,
"version")`, which this feature does not copy.

**Alternatives considered**:

- Extending `ThirdpartyOAuth2ProviderRepository`: it already has 11 methods.
- A separate `ServiceAccountCanonicalIDRepository` like the agent and permission-set ones: those
  exist because canonical IDs were added later and are optional; here `canonical_id` is required
  and service-scoped from the start.

## R3 — What is encrypted, and under which context

**Decision**: The plaintext is a JSON credential document, discriminated by `format`:
`{"format":"signed_jwt","use":"access_token","jwt":"<compact JWS>"}`. The `jwt` member is the
submitted string, byte for byte. The document is encrypted with `ports.EncryptionPort.Encrypt`
under `encryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()` and stored in
`service_accounts.credential_encrypted` (`BYTEA`). The entity carries it as `model.Secret`
(plaintext on the way in, encrypted before the repository sees it). The repository never sees
plaintext, as for the service client secret.

**Rationale**: SR-001 and the #471 requirement name the owning service's `service_id` context,
which `adrs/008-encryption-context-optimization.md` already defines; the service's branch key
exists from service creation, so no new branch-key subject or provisioning step is needed. A
format-discriminated document lets `private_key`, `google_service_account_json`, `github_app` and
`client_credentials` (#475, #476) reuse the same column. `model.Secret` is already listed in
`credential-types` in `.golangci.yml`, so `telemetrylint` rejects it as a log or span attribute.

**Alternatives considered**:

- Encrypting only the raw JWT bytes: simplest now, but each later format would need its own
  column or an envelope migration.
- A new `service_account_id` encryption subject: contradicts SR-001 and
  `adrs/008-encryption-context-optimization.md` (exactly one subject key, `service_id` or `kid`).

## R4 — Form validation of the JWT

**Decision**: A pure function in `internal/domain/model` checks the submitted string in this
order and stops at the first failed rule:

1. Length at most 16384 bytes (checked before any decoding).
2. Exactly three segments separated by `.`, each non-empty and each valid unpadded base64url
   (`base64.RawURLEncoding`, strict).
3. Header and payload each decode to a JSON object (`encoding/json` with `UseNumber`).
4. Header member `alg` is a string, non-empty, and not `none` under `strings.EqualFold`.
5. If the payload has `exp`: it is a JSON number that parses as a finite value in
   `[0, 253402300799]` (year 9999), and it is after the request time. **Proposed** (the range
   bound; the spec only says "a number").

Each rule maps to one constant message and one enumerated reason
([contracts](./contracts/admin-service-accounts-openapi.yaml), [observability](./contracts/observability.md)).
The function uses only the Go standard library. It performs no signature operation and fetches
nothing (FR-008).

**Rationale**: The rules are FR-009 verbatim. Strict base64url makes the whitespace edge case
(leading or trailing whitespace is refused) and the JWE case (five segments) fall out of rule 2.
Decoding is bounded by rule 1 (SR-007). A structural decode is not cryptography, so Principle III
is not engaged. The upper bound on `exp` exists because a value such as `1e30` is a JSON number
but cannot be stored as a timestamp, and the broker "could not tell when the account fails" (spec
Edge Cases, malformed `exp`).

**Alternatives considered**:

- `jws.Parse` / `jwt.ParseInsecure` from `lestrrat-go/jwx/v4`: also accepts JSON serialization and
  has its own leniency rules; the spec's rules are stricter and simpler to state with the standard
  library.
- Refusing `iss`, `sub` or `kid` values that are not strings: the token is opaque, so only the
  form rules of FR-009 may refuse it. A non-string value is treated as not stated and omitted from
  the summary.

## R5 — Credential summary and fingerprint

**Decision**: The summary is computed once per write and stored as `credential_summary` (`JSONB`):
`format`, `use`, and `iss`, `sub`, `kid`, `exp` when the token states them. `exp` keeps the number
as the token states it. The fingerprint is `SHA256:` followed by the unpadded standard base64
encoding of the SHA-256 digest of the exact submitted compact serialization (43 characters after
the prefix), computed with `crypto/sha256`. The parsed expiry is also stored as
`credential_expires_at` so status needs no JSON access or decryption. **Proposed** (the textual
encoding; the spec leaves it to planning).

**Rationale**: FR-012, SR-003 and the spec Assumptions fix SHA-256 over the submitted
serialization. The `SHA256:<base64>` form is the one operators know from OpenSSH key
fingerprints. A signed JWT has a high-entropy signature, so the digest cannot be reversed or
guessed (DB-003). Storing the summary avoids decrypting on every list and status read.

**Alternatives considered**:

- Hex encoding: longer (64 characters) with no benefit.
- Keyed HMAC: needs a key and its rotation story; unnecessary for a high-entropy input.
- Recomputing the summary on read: needs a decrypt per listed account and makes list latency
  depend on KMS.

## R6 — Status is derived on read

**Decision**: `status` is not stored. `ServiceAccount.StatusAt(now)` returns `failed` with reason
`credential_expired` when `credential_expires_at` is set and `now` is not before it, and `ready`
otherwise. `ServiceAccountService` receives `now func() time.Time` through its constructor; the
builder passes `time.Now`. No background job, no stored transition, no audit record for expiry.

**Rationale**: FR-014 and SC-004 require the change "from the instant" the expiry passes, "on
every read", with no delay. A stored status would need a sweeper and would lag. A constructor
clock follows `consent.Service` and `oauth2server.NewClientAssertionVerifier` and lets unit tests
cover the boundary (`exp == now` is refused on write and `failed` on read; no skew allowance)
without a test-only API. Later status values (`needs_bootstrap`, last mint error class, observed
ceiling) are stored facts that #474–#476 add next to this derived rule; `ready` and `failed` keep
their meaning (FR-015).

**Alternatives considered**:

- Stored status flipped by a background sweep: observable delay, and a write path with no actor.
- Emitting an audit event when an account expires: nothing happens at that instant; an event would
  need a scheduler. Out of scope.

## R7 — Optimistic concurrency and no-op updates

**Decision**: `version BIGINT` starts at 1 and is the strong ETag (`"1"`), reusing the existing
`strongETag` / `parseStrongETag` helpers. `If-Match` is optional on `PUT` and `DELETE`:

- Present and well formed: the repository applies the write only where `version = expected`;
  otherwise it returns the version-mismatch sentinel and the handler answers `412`.
- Present and malformed (weak, unquoted, not a positive integer): `400`.
- Absent: the write is unconditional (last writer wins).

`ServiceAccountService.Update` loads the account, checks a supplied precondition, builds the
candidate, and returns the stored account with outcome `unchanged` when nothing differs: same
`canonical_id`, same ceiling as a set, and either no credential or a credential whose fingerprint,
format and use equal the stored summary. An unchanged update writes nothing, keeps the version and
`updated_at`, and emits no audit record (FR-019). The ceiling is compared as a set and stored
sorted and de-duplicated-by-validation, so reordering alone is not a change.

Request errors win over the precondition: a request that would answer `400` or `404` does so even
when `If-Match` is stale (RFC 9110 §13.2.1).

**Rationale**: This is the maintainer's 2026-11-25 answer (optional `If-Match`, `412` when stale).
Fingerprint equality is a sound "same JWT" test because SHA-256 is collision resistant, and it
needs no decryption. Enforcing the version in the repository statement makes the check atomic on
both adapters.

**Alternatives considered**:

- Requiring `If-Match`: rejected by the maintainer's answer.
- Comparing ciphertext: envelope encryption is randomized, so equal plaintexts differ.
- Decrypting the stored credential to compare: an extra KMS call on every update for no gain.

## R8 — Update request semantics

**Decision** (**Proposed**): `PUT` takes one body shape. `scopes` is required and replaces the
ceiling. `canonical_id` is optional: omitted keeps it, a different value renames. `kind` is
optional: omitted keeps it, a different value is refused with `400`. `credential` is optional:
omitted keeps the stored credential, present replaces it. `null` for any member is refused with
`400`.

**Rationale**: Every `PUT` in the spec journeys states `scopes`; FR-005 and FR-018 define the
behaviour of an omitted `canonical_id` and `credential` but say nothing about an omitted ceiling.
Requiring the ceiling on each update means an administrator always states the blast radius they
are confirming, and avoids a second "omitted means keep" rule on the security-relevant field.

**Alternatives considered**:

- Omitted `scopes` keeps the ceiling: makes User Story 3 Scenario 8 a one-member body, but turns
  `PUT` into a partial update for its most sensitive member.
- `PATCH` for rename: #478 lists `PUT/DELETE` for "rotate credential, change ceiling, rename".

## R9 — A service that holds accounts cannot be deleted

**Decision**: Three layers, the last one atomic:

1. `ThirdpartyOAuth2ProviderService.Delete` asks a narrow `ServiceAccountCounter`
   (`CountByService`) and returns a `conflict` `StorageError` with the message
   `cannot delete service: N service account(s) are attached to it`, next to the existing
   permission-set guard. The counter is injected in the builder with `WithServiceAccounts`.
2. PostgreSQL: `service_accounts.service_id REFERENCES thirdparty_oauth2_services(id) ON DELETE
   RESTRICT`. The provider repository's existing `23503` mapping already returns `conflict`.
3. Memory: the provider repository receives the same counter (`WithServiceAccountRepository`, as it
   receives the session repository today) and checks it under its write lock.

The memory service-account repository checks that the service exists on `Create` through an
injected existence function, mirroring the foreign key.

**Rationale**: FR-025 and DB-002. The domain check gives the operator a precise message; the
store-level check closes the race between a concurrent account creation and service deletion.

**Alternatives considered**:

- `ON DELETE CASCADE`: erases stored machine credentials as a side effect, which the maintainer
  ruled out.
- Domain check only: racy across broker instances.

## R10 — Delete conflict for bound accounts (FR-020)

**Decision**: `ServiceAccountService.Delete` consults a `ServiceAccountBindingChecker` port
(`IsBound(ctx, serviceAccountID) (bool, error)`) before deleting. This feature wires a
`thirdparty.NoServiceAccountBindings` implementation that always answers false; #472 replaces it
with the agent-grant repository. The `409` response and its message
(`service account is bound by an agent grant`) are in the OpenAPI contract now. A domain unit test
with a checker that answers true covers the refusal path; the E2E test for User Story 5
Scenario 3 arrives with #472, as the spec states.

**Rationale**: The spec asks for the rule and its response now and enforcement from #472. A port
with a null implementation keeps the refusal path tested and makes #472 a wiring change.

**Alternatives considered**:

- Leaving the check out until #472: the documented `409` would have no code or test behind it.

## R11 — HTTP adapter

**Decision**: A new `admin.ServiceAccountsHandler` with `Create`, `List`, `Get`, `Update`,
`Delete`, `Status`, registered in `SetupAdminRoutes` under
`/api/services/{service-id}/service-accounts`. Each method:

1. Requires the operator principal first and answers `401` (`unauthorized`, `authentication
   required`) without reading anything (SR-005), as `ServicesHandler.GetDiscoveryStatus` does. It
   reads the principal with the existing `operatorPrincipalFromContext` helper.
2. Resolves `{service-id}` with `ThirdpartyOAuth2ProviderService.ResolveID` (UUID or canonical ID).
3. Resolves `{service-account-id}`: a UUID-shaped value is an ID, anything else a `canonical_id`
   within the service. `canonical.Validate` already forbids UUID-shaped canonical IDs, so the two
   cannot collide.
4. Parses input only; all rules live in the domain service (Principle VI).

Request bodies are limited to 64 KiB with `http.MaxBytesReader`; an over-limit body answers `400`.
Unknown JSON members are refused (`DisallowUnknownFields`), so a mistyped `credential` member
cannot be silently ignored. The list is ordered by `canonical_id` in ascending byte order. The
representation's `service_id` is always the UUID; `Prefer: reference-id=canonical` is not offered
on these operations. **Proposed** (ordering, strict members, no `Prefer`).

**Rationale**: Mirrors `ProtectedResourcesHandler`, the existing sub-resource of a service. The
64 KiB limit matches `AgentsHandler` and leaves room for a 16 KiB JWT, so the domain size rule,
not the transport limit, produces the message for an oversized token up to that bound.

## R12 — Request schema shape

**Decision** (**Proposed**): One `ServiceAccountRequest` schema with `kind` (enum `jwt_bearer`),
`canonical_id`, `scopes` and `credential`. `credential` is a `oneOf` with a `format`
discriminator, whose only branch is `SignedJwtCredential`. `ServiceAccountCreateRequest` and
`ServiceAccountUpdateRequest` are `allOf` wrappers that only add the `required` list of each
operation. The schema description holds the kind-to-format table.

**Rationale**: API-002 asks for "one schema discriminated by `kind`", but the spec's own update
journeys omit `kind`, and an OpenAPI discriminator property must be present. Keeping `kind` an
enum member and discriminating the credential by `format` satisfies the journeys and FR-006: a new
format adds a `oneOf` branch, and a new kind adds an enum value plus, where its credential has no
`format`, a kind-level `oneOf` around the same branches. Existing `jwt_bearer` payloads stay valid
in both cases.

**Alternatives considered**:

- A kind-level `oneOf` with a required `kind` on update: contradicts Journey 1 Step 4 and
  Journey 2 Step 3.

## R13 — Audit records and telemetry

**Decision**: `ServiceAccountService` writes one `Info` audit record after each committed create,
update and delete, with `operator_principal` as `id.Principal`. An update writes one record whose
`event` is chosen by precedence (`credential_rotated`, then `ceiling_changed`, then `renamed`) and
whose `changes` attribute lists every changed aspect. A refused credential writes one `Warn`
record with an enumerated `reason`. The only new spans are the PostgreSQL repository's
`storage.*.service_account` spans through `startStorageSpan`. No new metric, no new span attribute
namespace. Details and the credential-absence test matrix are in
[contracts/observability.md](./contracts/observability.md).

`coding-standards/LOGGING.md` section 6.2 permits JWT metadata `kid`, `alg`, `iss`, `aud`, `jti`,
`exp` in logs; it does not list `sub`. FR-022 requires audit records to carry the credential
summary, and FR-023 permits `sub` in "audit summaries". The plan therefore amends section 6.2 in
the same PR (Principle XV allows in-place amendment with rationale) with one scoped row: the
credential summary of a service account, including `sub`, is permitted in
`service.service_account.*` audit records only. **Proposed**.

Claim values come from an unverified token, so they are untrusted input (LOGGING.md section 7).
The summary's `slog.LogValuer` caps each string at 256 bytes in log output; the API response
shows the full value.

**Rationale**: One record per committed write matches Journey 1 ("one audit record is written for
the rotation; it carries the previous and the new summary … and the previous and new ceiling").
The `changes` list keeps a combined rotation-and-rename attributable (SC-006) without three
records for one commit. The domain service is the layer that knows the write committed and what
changed, so it logs once (LOGGING.md section 3). Expiry needs no metric: the status endpoint is the
monitoring interface the spec defines (User Story 4).

**Alternatives considered**:

- Leaving `sub` out of logs: contradicts FR-022.
- Logging `sub` under a reserved principal key: it is not an `id.Principal`, and for a machine
  credential it names a workload.
- A gauge of failed accounts: useful, but no requirement asks for it (Feature Scope rule).

## R14 — Ceiling validation

**Decision**: The ceiling is a non-empty list of unique, non-empty strings. It is not checked
against the service's declared `scopes` catalogue and never against the token.

**Rationale**: FR-004 and the spec Assumptions ("the same per-value rules as service scopes
today", which is non-empty). The catalogue describes what users may be asked for; a machine
credential may legitimately carry scopes the consent catalogue does not list. Ceiling-versus-grant
validation belongs to #472.

## R15 — ADR

**Decision**: One new ADR, `adrs/20261125-154231-service-accounts-stored-jwt.md`, introduced and
accepted in this PR. It records: the service-account entity and its repository port (a new
boundary, `internal/ports/AGENTS.md`); storing a JWT without verification and why that is not a
Principle I exception (SR-004); encryption under the service's `service_id` context with a
format-discriminated credential document; status derived on read; optional `If-Match`; `RESTRICT`
on service deletion; and the atomic migration `050`. No accepted ADR is superseded:
`adrs/008-encryption-context-optimization.md`, `adrs/004-storage-layer-architecture.md`,
`adrs/013-strongly-typed-entity-ids.md` and `adrs/012-encryption-layer-separation.md` are followed.
The ADR that supersedes decision 6 of `adrs/045-agent-subject-impersonation.md` belongs to #473.

**Rationale**: The spec asks for an ADR "if planning finds that an accepted ADR or
`ARCHITECTURE.md` needs to change". `ARCHITECTURE.md` changes (glossary, data store, admin API),
a new port boundary appears, and the stored-JWT posture is a security decision reviewers will look
for.

## R16 — Storage parity (SC-008)

**Decision**: One shared repository contract (create, scoped reads, uniqueness, rename, version
preconditions, delete, count) lives in `tests/integration/storage/contract/`. It runs against the
memory adapter in the self-contained integration suite and against PostgreSQL under the
`integration` build tag. Adapter packages cannot share `_test.go` code, and `tests/` is where
AGENTS.md puts shared test helpers. The E2E scenarios whose outcome depends on store
semantics (User Story 1 Scenario 5, User Story 3 Scenarios 7 and 9, User Story 5 Scenario 5) keep
one `It()` each, labelled `docker`, with both store cases inside, as feature
`055-agent-external-id-upsert` did. All other scenarios run on the memory store.

**Rationale**: Principle XIII maps one `It()` to one scenario, so the suite cannot be duplicated
per store. The handler and domain code are store independent; the store-dependent behaviour is
exactly uniqueness, version checks and the foreign key.

## R17 — Time-dependent acceptance scenarios

**Decision**: The E2E tests for expiry (User Story 3 Scenario 3, User Story 4 Scenario 2,
Journey 2) register a JWT whose `exp` is a few seconds ahead and poll the status with
`Eventually`. No clock is injected into the broker under test.

**Rationale**: `tests/e2e/AGENTS.md` forbids fixed sleeps and asks for production bootstrap;
AGENTS.md forbids test-only production APIs. The exact boundary is covered by domain unit tests
with the constructor clock (R6).
