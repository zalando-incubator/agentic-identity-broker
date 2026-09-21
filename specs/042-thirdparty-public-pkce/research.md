# Phase 0 Research: Public Client Support for Third-Party OAuth2 Services

**Feature**: `042-thirdparty-public-pkce` | **Date**: 2026-09-10
**Input**: [spec.md](./spec.md), [checklists/requirements.md](./checklists/requirements.md)

All Technical Context unknowns are resolved below. Every decision is grounded in code read during
this phase; file and line references are current as of this date.

---

## Ground truth established before deciding

| Question | Answer | Evidence |
|---|---|---|
| Is PKCE already sent upstream? | Yes, unconditionally, for every third-party flow | `internal/domain/oauth2session/service.go:385` (`oauth2.GenerateVerifier`), `:421` (`cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))`), `:331` (`oauth2.VerifierOption(verifier)` on exchange) |
| How is the code exchange performed? | `golang.org/x/oauth2` `Config.Exchange` | `internal/domain/oauth2session/service.go:304-313` (config), `:335` (exchange) |
| Which AuthStyle is configured? | None — `Endpoint.AuthStyle` is left at the zero value `AuthStyleAutoDetect` | `internal/domain/oauth2session/service.go:309-312` |
| How is refresh performed? | Hand-rolled `POST` form; credential always in the body, never a header | `internal/domain/oauth2session/service.go:673-688` |
| Does RFC 8693 need separate work? | No — it calls `GetValidAccessToken`, which refreshes through the same path | `internal/domain/tokenexchange/service.go:308` |
| Can a `down` migration refuse to run? | Yes — golang-migrate/v4 runs each PostgreSQL migration in a transaction; `DO $$ … RAISE EXCEPTION` aborts it and the error surfaces | `tests/integration/migrations/framework.go:11-20`; precedent in `migrations/028_normalize_service_protected_resources.up.sql:12-38` |
| Highest existing migration | `029` | `migrations/029_add_canonical_ids.{up,down}.sql` |
| Next free ADR number | `036` | `adrs/` max is `035-root-mounted-spa.md` |
| Can the entity represent "no credential" today? | No. `Secret`'s zero value is *plaintext-uninitialized*; both storage adapters reject it | `internal/domain/model/secret.go:15-20`, `internal/adapters/storage/postgres/thirdparty_provider_record.go:128-131`, `internal/adapters/storage/memory/thirdparty_provider_record.go:26-28` |
| Does the entity name match the spec? | No. The spec says `ThirdpartyOAuth2Service`; the code type is `model.ThirdpartyOAuth2ProviderEntity` | `internal/domain/model/thirdparty_oauth2_provider.go:74` |

---

## D1 — Upstream client authentication for public clients

**Decision**: For a public service, build the same `oauth2.Config` with an empty `ClientSecret` and
pin `Endpoint.AuthStyle = oauth2.AuthStyleInParams`. For a confidential service, change nothing:
plaintext secret, `AuthStyle` left at the zero value.

**Rationale**: Verified against the module source at
`golang.org/x/oauth2@v0.36.0/internal/token.go:186-205`:

```go
if authStyle == AuthStyleInParams {
    v = cloneURLValues(v)
    if clientID != "" { v.Set("client_id", clientID) }
    if clientSecret != "" { v.Set("client_secret", clientSecret) }
}
req, err := http.NewRequest("POST", tokenURL, strings.NewReader(v.Encode()))
...
if authStyle == AuthStyleInHeader { req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(clientSecret)) }
```

With `AuthStyleInParams` and an empty secret the library emits `client_id` in the body, omits
`client_secret` entirely, and never calls `SetBasicAuth`. That is exactly FR-012 and FR-013.

Critically, `RetrieveToken` (`token.go:215-246`) probes *only* when the style is unknown:

```go
needsAuthStyleProbe := authStyle == AuthStyleUnknown
```

Pinning `InParams` sets `needsAuthStyleProbe = false`, so the credentialed `AuthStyleInHeader`
first attempt and the fallback second attempt cannot happen. This satisfies FR-015 (no probing, no
credentialed fallback) structurally rather than by convention.

**Alternatives rejected**:

- *Leave `AuthStyleAutoDetect` with an empty secret.* Rejected: the probe starts with
  `AuthStyleInHeader`, which calls `SetBasicAuth(clientID, "")` — an `Authorization` header with an
  empty password. That is precisely what FR-013 forbids, and a provider error would trigger a second
  request, violating FR-015.
- *Hand-roll the public code exchange with `net/http`.* Rejected: it duplicates response parsing and
  error handling that the vetted library already does (Constitution Principle III), and would create
  two divergent exchange implementations.
- *Pin `AuthStyleInParams` for confidential services too.* Rejected: it changes the wire format for
  every existing service that currently negotiates to `InHeader`, breaking FR-017 and SC-004.

**Consequence for the shared-provider edge case** (two services, same token endpoint, one public and
one confidential): `authStyleCache` is an unexported field on each `oauth2.Config`
(`oauth2.go:61-63`), and `buildOAuth2Config` constructs a fresh `Config` per operation, so no cache
is shared between services. Neither influences the other.

**Consequence for retries** (FR-016): `exchangeCodeWithRetry`
(`internal/domain/oauth2session/service.go:317-364`) receives an already-built `*oauth2.Config` and
reuses it on every attempt. The auth style is therefore fixed for the whole retry sequence with no
code change to the retry loop.

**Refresh** (`service.go:653-738`): omit the `data.Set("client_secret", …)` line for public services.
Everything else — `grant_type`, `refresh_token`, `client_id`, provider authorization params, the two
headers — is unchanged. There is no retry loop and no `Authorization` header on this path today, so
FR-014 and SR-003 are met by that single omission.

---

## D2 — Domain representation of an absent credential

**Decision**: Add an explicit third state to the `Secret` value object, constructed only by
`model.NewAbsentSecret()`. The zero value keeps its current meaning (plaintext-uninitialized).

```go
type Secret struct {
    plaintext  string
    ciphertext []byte
    absent     bool
}

func NewAbsentSecret() Secret          // the only way to construct the absent state
func (s Secret) IsAbsent() bool        // s.absent
func (s Secret) IsPlaintext() bool     // !s.absent && s.ciphertext == nil
func (s Secret) IsEncrypted() bool     // !s.absent && s.ciphertext != nil
```

**Rationale**: The spec's Domain Model calls for exactly this ("gains a third explicit state …
Absent must be an assertable state, not a validation failure"). Making absence *constructed* rather
than *inherited from the zero value* is the fail-closed choice required by Constitution Principle I:
an entity built by forgetting to set `Secret` still fails validation with today's
`client_secret is required`, instead of silently becoming a public client.

`GetPlaintext()` and `GetCiphertext()` on an absent secret return an error naming the absent state,
so every existing call site that assumes a credential fails loudly rather than producing `""`.

**Alternatives rejected**:

- *Make the entity field `*Secret`.* Rejected: it spreads nil checks across every consumer and makes
  the state unassertable from the value object itself.
- *Reinterpret the zero value as absent.* Rejected: fails open. A construction bug would produce a
  credential-free service that the broker happily uses.
- *Keep two states and encode absence as `NewEncryptedSecret(nil)`.* Rejected: `GetCiphertext()`
  already treats nil as the plaintext state (`secret.go:61-63`), so this overloads an existing
  meaning and cannot be distinguished downstream.

---

## D3 — Modelling the method itself

**Decision**: New value object in `internal/domain/model/token_endpoint_auth_method.go`:

```go
type TokenEndpointAuthMethod string
const TokenEndpointAuthMethodNone TokenEndpointAuthMethod = "none"
func (m TokenEndpointAuthMethod) Validate() error  // "" is valid (absent); only "none" otherwise
func (m TokenEndpointAuthMethod) IsAbsent() bool
```

The entity gains `TokenEndpointAuthMethod TokenEndpointAuthMethod` where the empty string means
absent/confidential, plus a derived predicate `func (e *…Entity) IsPublicClient() bool`.

**Rationale**: Mirrors the existing `OAuth2Flavor` string-enum pattern
(`internal/domain/model/oauth2_flavor.go:11-24`) rather than introducing a second convention. Unlike
`OAuth2Flavor`, it has **no default** (FR-003): `DefaultOAuth2Flavor` has no counterpart here. A
pointer type is unnecessary in the domain because the entity is never JSON-decoded — presence
detection is a wire concern handled in D5.

`IsPublicClient()` is the single predicate every consumer branches on, so the invariant
(`public ⟺ method == none ⟺ secret absent`) has exactly one definition.

---

## D4 — Where the invariant is enforced

**Decision**: Enforce in `ValidateForCreate` / `ValidateForUpdate` on the entity, before the flavor
switch, in this order:

1. `TokenEndpointAuthMethod.Validate()` — unknown value rejected, naming `none` (FR-002).
2. Public + Google flavor → rejected, naming the variant (FR-007), *before* any attempt to derive
   identity from a credential that does not exist.
3. Public + secret not absent → rejected as a contradiction (FR-005).
4. Public + empty `ClientID` → rejected (FR-008).
5. Confidential + absent secret → rejected (FR-006) — the message stays `client_secret is required`
   so today's error text for today's mistake is unchanged.
6. Otherwise, today's checks run unchanged.

**Rationale**: Constitution Principle VI puts conditional logic in the domain, not the handler.
Placing the Google rejection before `enrichForGoogleFlavor` (`thirdparty_oauth2_provider.go:196`,
`:289`) is required: that function parses the credential document to derive `ClientID`, and a public
Google service has no document to parse.

`Validate()` (`thirdparty_oauth2_provider.go:96-133`) also needs the absent state added to its
secret-state check, since it is called on entities loaded from storage.

**Layer split**: the domain owns the invariant; the HTTP handler owns only the JSON-presence mapping
described in D5. The handler never re-implements a rule the domain enforces.

---

## D5 — Wire-format presence semantics

**Decision**: Both `CreateService` and `UpdateService` read the body once and decode it twice — into
the DTO and into `map[string]json.RawMessage` — then map presence to domain state:

| `token_endpoint_auth_method` | `client_secret` | Result |
|---|---|---|
| key absent | present | Confidential — today's path, byte-identical |
| key absent | absent | Confidential — rejected, `client_secret is required` (as today) |
| `"none"` | absent | Public — `Secret = NewAbsentSecret()` |
| `"none"` | present and non-empty | 400, contradiction |
| `"none"` | present but `""` or `null` | Public — an empty value is not a credential |
| `null` | present | Confidential — identical to omission (API-003) |
| `null` | absent | Confidential — rejected, `client_secret is required` |
| any other value | any | 400, naming `none` as the only accepted value |

**Rationale**: because `null` and absence both mean confidential (API-003, amended during this
planning phase), no raw-map presence detection is needed for either field. `encoding/json` collapses
absent and `null` into a nil pointer — normally a footgun, here exactly the required semantics:

```go
TokenEndpointAuthMethod *string `json:"token_endpoint_auth_method"`   // nil = confidential
ClientSecret            string  `json:"client_secret"`                // "" = not supplied
```

`ClientSecret` stays a plain string because "supplied" means non-empty (FR-005): `""`, `null`, and
absence are all "no credential", which is the only distinction the invariant needs.

**The `Secret` construction must branch on the method, not on the string being non-empty.** This is
the one place a plausible implementation silently breaks the approved contract:

```go
switch {
case req.TokenEndpointAuthMethod == nil:                       // absent or null
    entity.TokenEndpointAuthMethod = ""                        // confidential
    entity.Secret = model.NewPlaintextSecret(req.ClientSecret) // "" still rejected downstream
case *req.TokenEndpointAuthMethod == string(model.TokenEndpointAuthMethodNone):
    if req.ClientSecret != "" {
        // 400 — contradiction (FR-005)
    }
    entity.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
    entity.Secret = model.NewAbsentSecret()                    // NOT NewPlaintextSecret("")
default:
    entity.TokenEndpointAuthMethod = model.TokenEndpointAuthMethod(*req.TokenEndpointAuthMethod)
    // rejected by TokenEndpointAuthMethod.Validate() in the domain, naming `none`
}
```

The trap: today's handler builds the entity with an unconditional
`Secret: model.NewPlaintextSecret(req.ClientSecret)` (`services_handler.go:149`, `:326`). Left that
way, `{"token_endpoint_auth_method": "none", "client_secret": ""}` would produce
`NewPlaintextSecret("")` — the plaintext-empty state, *not* absent — and domain validation rule 3
("public and secret not absent → contradiction") would reject the very representation this contract
accepts. The handler must construct `NewAbsentSecret()` explicitly on the public branch. Both
handler and domain tests must cover the empty-string input, not only the omitted one, since the two
inputs reach different constructor calls.

This is the decisive simplification of accepting `null`. The rejecting alternative would have forced
`map[string]json.RawMessage` presence detection onto **both** the create and update paths — machinery
`UpdateService` has today only for `canonical_id` and `protected_resources`
(`services_handler.go:286-295`) and `CreateService` does not have at all.

The one behaviour given up: `{"token_endpoint_auth_method": "none", "client_secret": ""}` is accepted
as a public registration rather than rejected as a contradiction. An empty string is not a credential,
nothing is persisted, and the invariant still holds at three layers. A **non-empty** secret alongside
`none` is still rejected, which is the case FR-005 exists to prevent.

Update keeps full-replacement semantics (FR-019, clarification 2026-09-10): the stored method is
never consulted when interpreting the request.

---

## D6 — Schema and rollback

**Decision**: One migration pair, `031_add_token_endpoint_auth_method.{up,down}.sql`.

Up:

```sql
ALTER TABLE thirdparty_oauth2_services ALTER COLUMN client_secret_encrypted DROP NOT NULL;
ALTER TABLE thirdparty_oauth2_services ADD COLUMN token_endpoint_auth_method VARCHAR(32);
ALTER TABLE thirdparty_oauth2_services ADD CONSTRAINT chk_thirdparty_oauth2_services_client_auth
    CHECK (
        (token_endpoint_auth_method IS NULL   AND client_secret_encrypted IS NOT NULL)
     OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none' AND client_secret_encrypted IS NULL)
    );
```

No `DEFAULT` and no backfill: every existing row keeps `NULL` and stays confidential (DB-004,
FR-023). The single `CHECK` enforces both the XOR (DB-005) and the closed value set, so no code path
can persist a contradictory or unrecognized row.

Down: a `DO $$ … RAISE EXCEPTION` block that aborts when any row has a non-null method, naming the
blocking services, then drops the constraint and column and restores `NOT NULL` (DB-006).

**Rationale**: Verified that the runner supports this — golang-migrate's PostgreSQL driver wraps
each migration in a transaction, and migration 028 already uses `RAISE EXCEPTION` for a guard
(`migrations/028_normalize_service_protected_resources.up.sql:12-38`). No migration in this
repository uses `CREATE INDEX CONCURRENTLY` or a no-transaction directive, so the transactional
assumption holds.

**Consequence — the refusal leaves migration state dirty.** golang-migrate writes
`SetVersion(target, dirty=true)` *before* executing a step and clears the flag only on success. A
refused rollback therefore rolls back the SQL transaction — the schema stays at `031`, nothing is
partially torn down — but leaves `schema_migrations.dirty = true`, and every subsequent `migrate`
call fails with `ErrDirty` until the flag is cleared. This is not a defect in the guard; it is how
the runner reports "a migration was attempted and did not complete".

Two things follow, and DB-007 is not executable without them:

1. **The test framework needs a recovery helper.** `tests/integration/migrations/framework.go`
   exposes `Up`, `UpAll`, `Down`, `DownAll`, and `Version` (which already returns `dirty`) but no
   way to clear the flag. Add `Force(t, version)` wrapping `m.Force(int(version))`, matching the
   existing helper style.
2. **The refusal is observable through the dirty flag.** The rollback-refusal test asserts the
   error message names the blocking services, that `Version()` reports `dirty == true`, and that
   `token_endpoint_auth_method` still exists — proving the guard aborted the whole step rather than
   dropping half the schema. It then calls `Force(31)` to return to a clean state before continuing.

Operators hitting the refusal in production follow the same two steps: remove or convert the named
public services, then `migrate force 31` before retrying the rollback. This belongs in the ADR's
consequences so it is not rediscovered during an incident.

**Alternatives rejected**:

- *Two separate constraints (one for nullability, one for the enum).* Rejected: the invariant is a
  single relationship; splitting it lets a future migration drop half of it.
- *Backfill `'client_secret_post'` for existing rows.* Rejected outright: the spec's whole design
  turns on absence being the confidential state (FR-003, FR-023). A backfill would change the
  requests existing services send upstream.
- *Silent down migration that nulls the column.* Rejected: it would leave services the broker treats
  as public with no credential and a `NOT NULL` column, i.e. data loss or a fabricated secret.

**Adapter consequences**:

- `postgres`: add `TokenEndpointAuthMethod *string \`db:"token_endpoint_auth_method"\`` to the record
  (`thirdparty_provider_record.go:25-43`), add the column to `providerColumns`, the `INSERT`, and the
  `UPDATE` (`thirdparty_provider.go:14-20`, `:34-71`, `:139-208`). `entityToRecord` must map an
  absent secret to a nil `SecretCiphertext` instead of failing at `GetCiphertext()`
  (`thirdparty_provider_record.go:128-131`); `recordToEntity` maps nil ciphertext to
  `NewAbsentSecret()`. `[]byte` already scans SQL `NULL` as nil, so no scanner change is needed.
- `memory`: `providerEntityCopy` must accept an absent secret
  (`memory/thirdparty_provider_record.go:26-28`) while still rejecting plaintext.
- `internal/ports/thirdparty_provider.go:11-20` documents that stored entities always carry an
  encrypted secret; that contract text becomes "encrypted or absent".

---

## D7 — Branch key provisioning for public services

**Decision**: Keep `branchKeyManager.Create` unchanged for public services on both create and update.
Only the encrypt step is skipped.

**Rationale**: FR-028, confirmed in code. Provisioning precedes encryption
(`internal/domain/thirdparty/service.go:125` then `:143` for create; `:236` then `:252` for update),
and user session tokens for that service are encrypted under the same service-scoped subject
(`domainencryption.NewServiceBranchKeySubject(entity.ID)`). A public service whose branch key was
never provisioned would fail to encrypt the tokens of its first user session — the exact failure
Story 2 depends on not happening. Provisioning is idempotent, so update remains safe.

---

## D8 — Read representation of a public service

**Decision**: `ServiceResponse.ClientSecret` becomes `*string` with `json:"client_secret,omitempty"`.
Confidential services still report `"REDACTED"`; public services omit the field entirely. A new
`TokenEndpointAuthMethod *string \`json:"token_endpoint_auth_method"\`` reports `"none"` or `null`.

**Rationale**: FR-025 and API-002. `"REDACTED"` asserts that a secret exists; emitting it for a
public service would misinform an operator auditing which services hold credentials. `null` for the
method on a confidential service (rather than omission) is required by FR-024 so a single list read
answers SC-007 for every entry.

`RedactedCopy()` and `Secret.Redacted()` (`secret.go:86-88`) stay as they are; the handler chooses
whether to call `Redacted()` at all, based on `IsPublicClient()`.

---

## D9 — Audit logging

**Decision**: Add the `event` key to the third-party provider lifecycle logs and a `public_client`
boolean attribute to the lifecycle and session events that this feature touches:

| Path | Event value | Added attribute |
|---|---|---|
| `thirdparty.Service.Create` | `service.thirdparty.provider_created` | `public_client` |
| `thirdparty.Service.Update` | `service.thirdparty.provider_updated` | `public_client` |
| `InitiateOAuth2Flow` | `session.oauth2.flow_initiated` (existing) | `public_client` |
| `HandleCallback` | `session.oauth2.session_established` (existing) | `public_client` |
| `HandleCallback` (exchange failure) | `session.oauth2.pkce_validation_failed` (existing) | `public_client` |
| `refreshSessionTokens` | `session.oauth2.token_refreshed` / `refresh_failed` (existing) | `public_client` |

**Rationale**: FR-026 asks for the `event` key convention already used by third-party session
logging (`internal/domain/oauth2session/service.go:432`, `:624`, `:1177`, `:1198`). The provider
lifecycle logs currently use the event name as the *message* with no `event` key
(`internal/domain/thirdparty/service.go:163`), so they gain the key rather than a parallel second
log line.

FR-026 covers "exchanging a code for" a public service, which includes the **failed** exchange —
the case an operator most needs to see. `HandleCallback` logs `session.oauth2.pkce_validation_failed`
on *any* exchange error, not only a PKCE mismatch (`service.go:585-592`), so that path carries the
`public_client` attribute too. Without it, the most likely public-client failure — an upstream
that still demands client authentication — would be indistinguishable in the log from a
confidential PKCE failure.

That path must keep logging only the service ID, token endpoint, and a failure reason. The upstream
error body is not logged: a provider may echo submitted parameters back in an error description,
which for a confidential service could include the credential. No new value is logged on any path
in this table, satisfying FR-027 and SR-008 by construction — the credential, verifier, challenge,
code, and tokens are never passed to a logger on these paths today, and this feature adds no such
argument.

---

## D10 — E2E upstream test double

**Decision**: Extend `tests/e2e/helpers/mock_upstream.go` with an opt-in strict public-client mode,
default off.

- `/authorize` records `code_challenge` and `code_challenge_method` per `state`.
- `/token` in public mode returns `401 invalid_client` if an `Authorization` header is present or a
  `client_secret` form value is present, and `400 invalid_grant` if `code_verifier` does not
  `S256`-hash to the recorded challenge.
- Existing capture (`GetLastRequest`, `GetLastBody`, `mock_upstream.go:168-182`) is unchanged.

**Rationale**: The spec lists this under Dependencies as blocking Stories 2 and 3. The existing
double accepts anything, so a green test would prove nothing about SC-002 or SC-003. Assertion of
*absence* is already possible today — `LastRequest` retains headers and `LastBody` is the parsed
form — but absence assertions alone cannot show the flow works against a provider that *rejects*
credentials, which is the actual acceptance criterion. Default-off keeps every existing E2E test on
its current behavior.

**Alternative rejected**: a second mock server type for public clients. Rejected: it duplicates the
authorize/token/metadata/JWKS surface and would drift from the primary double.

---

## D11 — Architectural record

**Decision**: Write `adrs/036-public-client-token-endpoint-auth.md` (next free number; `adrs/` maxes
at `035`). It is **required, not optional**, because this feature amends an accepted ADR.

**ADR 012 must be amended.** `adrs/012-encryption-layer-separation.md:32-43` documents the `Secret`
value object as a **two-state** design and states that "this immutable two-state design makes it
impossible to accidentally persist a plaintext secret". Adding the absent state changes an accepted
ADR's description of a security-relevant type. Constitution Principle II: "Deviation from accepted
ADRs is NOT permitted without creating a new superseding ADR." ADR 036 therefore amends ADR 012's
`Secret` section and must restate the guarantee for three states: plaintext is still impossible to
persist (`GetCiphertext()` still errors on it), and absence is reachable only through
`NewAbsentSecret()`, never through the zero value.

ADR 036 records four decisions:

1. Absence is the confidential state; the attribute has no default and none is ever inferred.
2. `none` is the only accepted value; `client_secret_basic` and `client_secret_post` are
   deliberately excluded because confidential services keep today's negotiated behavior.
3. `AuthStyleInParams` is pinned for public exchanges; `AutoDetect` is preserved for confidential
   ones.
4. `Secret` gains an explicitly constructed absent state (amending ADR 012).

**Rationale**: The auth-style pinning in particular is a non-obvious decision that a future
contributor could "simplify" back into `AutoDetect` — silently reintroducing the empty-password
`Authorization` header — unless it is recorded with its reasoning.

**Precedent — ADR 017 (Optional Agent `client_id`)** is the repository's existing pattern for an
optional, nullable attribute, and ADR 036 must address two points where this feature relates to it:

- *Representation.* ADR 017 uses a pointer (`Agent.ClientID *id.ClientID`, nil = absent) because
  `id.ClientID` has no meaningful zero. This feature uses a two-layer representation instead: the
  empty string in the domain (matching the existing `OAuth2Flavor` string-enum style and avoiding
  nil checks in every consumer) and `*string` in the PostgreSQL record, matching the adapter's
  existing treatment of nullable columns (`CanonicalID *string`, `MetadataURL *string`). The nullable
  column and the "no auto-generation fallback" rule are followed exactly.
- *Update semantics.* ADR 017 states that "on update, omitting `client_id` preserves the existing
  value". This feature does the opposite: omitting the method makes the service confidential
  (FR-019, FR-021), per the 2026-09-10 clarification. That divergence is deliberate — third-party
  service update is already full-replacement and already requires the client secret on every call —
  and must be stated in ADR 036 so a reviewer reads it as a decision rather than an inconsistency.

**Consequences to record**: the guarded rollback leaves migration state dirty (see D6), so the
operator runbook is: convert or remove the named public services, `migrate force 31`, then retry.

---

## Process gate — cleared

**API-010 / Constitution Principles IV and X**: the administrative API changes in
[contracts/admin-api.md](./contracts/admin-api.md) were confirmed by the stakeholder on 2026-09-10.

Two decisions were put to them and both were answered:

1. **`client_secret` omitted for public services** rather than reporting the `"REDACTED"`
   placeholder that implies a stored secret. Approved as proposed.
2. **Explicit `null` accepted on write** as equivalent to omission. This *reversed* the original
   API-003, which rejected it. The reversal is recorded in
   [spec.md Clarifications](./spec.md#clarifications) and
   [checklists/requirements.md finding 11](./checklists/requirements.md), and simplified D5: with
   `null` and absence both meaning confidential, no raw-map presence detection is needed on either
   handler path.
