# Phase 1 Data Model: Public Client Support for Third-Party OAuth2 Services

**Feature**: `042-thirdparty-public-pkce` | **Date**: 2026-09-10
**Depends on**: [research.md](./research.md) (decisions D2, D3, D4, D6, D7)

This feature introduces **no new entity**. It extends one existing entity and one existing value
object, and adds one new value object. `id.ServiceID` already exists
(`internal/domain/id/uuid_ids_gen.go:66-74`), so no code generation in `internal/domain/id/` is
required (ADR 013).

---

## 1. Value object: `TokenEndpointAuthMethod` (new)

**File**: `internal/domain/model/token_endpoint_auth_method.go`

```go
type TokenEndpointAuthMethod string

const TokenEndpointAuthMethodNone TokenEndpointAuthMethod = "none"
```

| Value | Meaning |
|---|---|
| `""` | Absent. The service is a **confidential** client and authenticates upstream exactly as it does today. |
| `"none"` | The service is a **public** client. No credential is stored and none is transmitted upstream by any channel. |
| anything else | Invalid. |

**Behaviour**:

- `Validate() error` — accepts `""` and `"none"`; every other value returns an error naming `none`
  as the only accepted value (FR-002).
- `IsAbsent() bool` — reports `m == ""`.

**Invariants**:

- **No default.** Unlike `OAuth2Flavor`, which falls back to `DefaultOAuth2Flavor`
  (`internal/domain/model/oauth2_flavor.go:27`), this type has no default and none is ever inferred,
  assigned, or persisted on the operator's behalf (FR-003).
- Absence is a permanent state, not a placeholder awaiting a value (FR-023).

---

## 2. Value object: `Secret` (extended)

**File**: `internal/domain/model/secret.go`

Today `Secret` has two constructed states plus an unconstructed zero value that is *logically*
uninitialized but *reports* as plaintext (`secret.go:15-20`). This feature adds a third **explicitly
constructed** state.

```go
type Secret struct {
    plaintext  string
    ciphertext []byte
    absent     bool   // new
}

func NewAbsentSecret() Secret   // new — the only way to reach the absent state
```

| State | Constructed by | `IsAbsent` | `IsPlaintext` | `IsEncrypted` |
|---|---|---|---|---|
| Plaintext | `NewPlaintextSecret(s)` | false | true | false |
| Encrypted | `NewEncryptedSecret(b)` | false | false | true |
| Absent | `NewAbsentSecret()` | **true** | false | false |
| Zero value | *(none — a bug)* | false | true | false |

**Changed behaviour**:

- `IsPlaintext()` becomes `!s.absent && s.ciphertext == nil`.
- `IsEncrypted()` becomes `!s.absent && s.ciphertext != nil`.
- `GetPlaintext()` and `GetCiphertext()` on an absent secret return an error naming the absent
  state, so any consumer that assumes a credential exists fails loudly.
- `Redacted()` is unchanged and still returns `"REDACTED"`. Callers decide whether to call it; the
  admin read path does not, for public services (see §5).

**Invariant**: the zero value keeps its current meaning. Absence must be *constructed*, never
inherited, so an entity assembled without setting `Secret` still fails validation with today's
`client_secret is required` rather than silently becoming a public client (Constitution
Principle I, fail closed).

---

## 3. Entity: `ThirdpartyOAuth2ProviderEntity` (extended)

**File**: `internal/domain/model/thirdparty_oauth2_provider.go:74-92`

> **Naming note**: the specification calls this concept `ThirdpartyOAuth2Service`, matching the
> ARCHITECTURE.md glossary and the administrative API path `/api/services`. The Go type is
> `ThirdpartyOAuth2ProviderEntity`. This feature does not rename it; the pre-existing divergence is
> out of scope.

**Added field**:

```go
TokenEndpointAuthMethod TokenEndpointAuthMethod  // "" = confidential, "none" = public
```

**Added predicate**:

```go
func (e *ThirdpartyOAuth2ProviderEntity) IsPublicClient() bool {
    return e.TokenEndpointAuthMethod == TokenEndpointAuthMethodNone
}
```

This is the single predicate every consumer branches on, so the invariant has exactly one
definition.

**Changed field semantics**: `Secret` may now be in the absent state, and is absent **if and only
if** `IsPublicClient()` is true.

### Core invariant

```
IsPublicClient()  ⟺  TokenEndpointAuthMethod == "none"  ⟺  Secret.IsAbsent()
```

Enforced in three independent layers, so no single defect can persist a contradictory service:

1. Entity validation (§3.1) — rejects contradictions before any side effect.
2. The database `CHECK` constraint (§6) — rejects contradictory rows outright.
3. The upstream request builders (§7) — branch on the same predicate, so a hypothetical
   contradictory entity cannot produce a half-authenticated request.

### 3.1 Validation rules

Applied in `ValidateForCreate` (`thirdparty_oauth2_provider.go:138`) and `ValidateForUpdate`
(`:226`), in this order, **before** the existing flavor switch:

| # | Condition | Result | Requirement |
|---|---|---|---|
| 1 | `TokenEndpointAuthMethod.Validate()` fails | Reject, naming `none` as the only accepted value | FR-002 |
| 2 | Public **and** `Flavor == google` | Reject, naming the `google` variant | FR-007 |
| 3 | Public **and** `!Secret.IsAbsent()` | Reject as a contradiction, naming both fields | FR-005 |
| 4 | Public **and** `ClientID` empty | Reject, `client_id is required` | FR-008 |
| 5 | Confidential **and** `Secret.IsAbsent()` | Reject, `client_secret is required` | FR-006 |
| 6 | otherwise | Today's checks run unchanged | FR-017 |

Rule 2 must precede any credential-derived work: `enrichForGoogleFlavor`
(`thirdparty_oauth2_provider.go:344`) parses the service-account document to derive `ClientID`,
`Endpoints`, and `IssuerURI`, and a public Google service has no document to parse.

For a public service, rules 1-4 replace the plaintext extraction at `:159-166` and `:250-259`; the
remaining flavor checks (issuer URI, HTTPS scheme, discovery-vs-endpoints, scopes, protected
resources, authorization params) run unchanged.

`Validate()` (`:96-133`), which runs on entities loaded from storage, adds the absent state to its
secret-state check.

### 3.2 State transitions

| Transition | Trigger | Effect |
|---|---|---|
| *(none)* → Confidential | Create with the method absent and a credential supplied | Credential encrypted and stored, as today |
| *(none)* → Public | Create with method `none` and no credential | Branch key provisioned (FR-028); no ciphertext stored |
| Confidential → Public | Update declaring `none` with no credential | Stored ciphertext **removed**; column set to `NULL` (FR-020, SR-006) |
| Public → Confidential | Update omitting the method, or sending `null`, and supplying a credential | Credential encrypted and stored; method column set to `NULL` (FR-021) |
| Public → Public | Update declaring `none` with no credential | Other fields replaced; still no credential (User Story 4, scenario 4) |
| Confidential → Confidential | Update omitting the method and supplying a credential | Today's behaviour, unchanged |
| Public → *(rejected)* | Update omitting the method (or sending `null`) **and** the credential | Rejected; the service keeps its previous configuration intact (FR-021) |

The update operation is **full replacement**: the stored method is never consulted when interpreting
the request (FR-019, clarification 2026-09-10). Omission — or an explicit `null`, which is
semantically identical (API-003) — means confidential regardless of what is stored, which is why the
last row is a rejection and not a no-op.

**No transition invalidates a stored `UserSession`.** Sessions are keyed by principal and service and
carry no reference to how the broker authenticated; the next upstream token request simply reads the
current setting (FR-022).

---

## 4. Entity: `UserSession` (unchanged)

No field, encryption context, or lifecycle change. Its tokens are encrypted under the same
service-scoped branch key subject used for the client secret, which is why branch-key provisioning
still runs for public services (FR-028; `internal/domain/thirdparty/service.go:121-134`).

---

## 5. Lifecycle: encryption and provisioning

**File**: `internal/domain/thirdparty/service.go`

| Step | Confidential | Public |
|---|---|---|
| Validate before side effects (`:108`, `:222`) | unchanged | unchanged |
| Generate `ServiceID` (`:117-119`) | unchanged | unchanged |
| `branchKeyManager.Create` (`:125`, `:236`) | unchanged | **unchanged — still runs** (FR-028) |
| `Secret.GetPlaintext()` + `encryption.Encrypt` (`:137-153`, `:247-261`) | unchanged | **skipped** |
| `repo.Create` / `repo.Update` (`:159`, `:266`) | unchanged | called with an absent secret |
| `decryptSecret` on read (`:451-468`) | unchanged | **skipped**; the entity is returned with the secret still absent |

`Get` and `List` must not report an error, warning, or degraded state for a public service (FR-009).
The existing decryption-failure warning path (`:195-202`, `:270-296`) must not be reached by an
absent secret — absence is a normal state, not a decryption failure.

---

## 6. Persistence schema

**Table**: `thirdparty_oauth2_services` | **Migration**: `031_add_token_endpoint_auth_method`

| Column | Before | After |
|---|---|---|
| `client_secret_encrypted` | `BYTEA NOT NULL` | `BYTEA` (nullable) |
| `token_endpoint_auth_method` | *(does not exist)* | `VARCHAR(32)` nullable, **no default**, no backfill |

**Constraint** `chk_thirdparty_oauth2_services_client_auth`:

```sql
CHECK (
    (token_endpoint_auth_method IS NULL  AND client_secret_encrypted IS NOT NULL)
 OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none' AND client_secret_encrypted IS NULL)
)
```

This single constraint enforces both the XOR invariant (DB-005) and the closed value set, so no code
path — including a direct SQL write — can persist a contradictory or unrecognized row.

**Rollback guard**: the down migration aborts with `RAISE EXCEPTION` naming the blocking services
when any row has a non-null method, rather than deleting services or fabricating credentials
(DB-006).

### Adapter mapping

| Layer | Confidential | Public |
|---|---|---|
| `postgres` record (`thirdparty_provider_record.go:25-43`) | `SecretCiphertext []byte` populated; new `TokenEndpointAuthMethod *string` nil | `SecretCiphertext` nil; `TokenEndpointAuthMethod` → `"none"` |
| `entityToRecord` (`:123-161`) | `GetCiphertext()` as today | absent secret → nil ciphertext, no error |
| `recordToEntity` (`:167-219`) | `NewEncryptedSecret(bytes)` | nil ciphertext → `NewAbsentSecret()` |
| `memory` (`memory/thirdparty_provider_record.go:21-31`) | ciphertext required, as today | absent accepted; plaintext still rejected |

`[]byte` already scans SQL `NULL` as nil, so no custom scanner is required. `providerColumns`, the
`INSERT`, and the `UPDATE` column lists (`postgres/thirdparty_provider.go:14-20`, `:34-71`,
`:139-208`) all gain `token_endpoint_auth_method`.

The port contract in `internal/ports/thirdparty_provider.go:11-20` currently documents that stored
entities always carry an encrypted secret; it becomes "encrypted **or absent**". The method set is
otherwise unchanged.

---

## 7. Upstream request contract

Derived from the same `IsPublicClient()` predicate on all three request classes. Full wire detail is
in [contracts/upstream-token-requests.md](./contracts/upstream-token-requests.md).

| Request | Confidential | Public |
|---|---|---|
| Authorization redirect | `code_challenge` + `code_challenge_method=S256`, unchanged | identical (FR-010) |
| Code exchange | `oauth2.Config` with plaintext secret, `AuthStyle` left at `AutoDetect` | same config with an empty secret and `AuthStyle` pinned to `AuthStyleInParams` (FR-012, FR-013, FR-015) |
| Refresh | hand-rolled form with `client_secret` | same form, `client_secret` omitted (FR-014) |

Provider authorization parameters are applied to all three in both modes, unchanged (FR-018).

---

## 8. Glossary additions (ARCHITECTURE.md)

| Term | Definition |
|---|---|
| **TokenEndpointAuthMethod** | Optional attribute of a ThirdpartyOAuth2Service whose only accepted value is `none`. Absent means the service is a confidential client that authenticates upstream as it always has; `none` means it is a public client that stores no credential and transmits none. |
| **Public client** | A ThirdpartyOAuth2Service declaring `token_endpoint_auth_method: none`. It presents only its client identifier and the PKCE code verifier at the upstream token endpoint. |
| **Confidential client** | A ThirdpartyOAuth2Service declaring no token endpoint authentication method. It stores an encrypted client secret and transmits it upstream on the code exchange and refresh. |

The existing **Secret** glossary entry is amended: "Value object with exclusive plaintext, encrypted,
or absent state" — absent meaning the service is a public client and holds no credential.
