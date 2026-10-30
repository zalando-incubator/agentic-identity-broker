# Data Model: OAuth2 Provider Flavor Support

**Feature**: `20260312-120000-oauth2-provider-flavors`
**Date**: 2026-03-12

## New Value Objects

### `OAuth2Flavor`

**Location**: `internal/domain/model/oauth2_flavor.go`

**Type**: Named string enum

```go
type OAuth2Flavor string

const (
    // OAuth2FlavorStandard is the default flavor: plain client_secret string.
    // Preserves existing behavior for all currently configured services.
    OAuth2FlavorStandard OAuth2Flavor = "standard"

    // OAuth2FlavorGoogle indicates a Google service account JSON credential.
    // client_id is extracted automatically from the service account JSON.
    OAuth2FlavorGoogle OAuth2Flavor = "google"
)

// DefaultOAuth2Flavor is the zero-value flavor applied when the field is omitted.
const DefaultOAuth2Flavor = OAuth2FlavorStandard

// Validate returns an error if the flavor is not a recognized value.
// Returns nil for "standard" and "google". Returns error for any other value,
// including the empty string.
func (f OAuth2Flavor) Validate() error

// String returns the string representation of the flavor.
func (f OAuth2Flavor) String() string
```

**Validation rules**:
- Must be one of `"standard"`, `"google"` (extensible: add new constants here in future)
- Empty string is treated as invalid (zero value); callers should use `DefaultOAuth2Flavor` for defaulting

**Domain events**: See `ThirdPartyServiceFlavorChanged` below.

---

### `GoogleServiceAccountKey`

**Location**: `internal/domain/model/google_service_account.go`

**Type**: Value object (immutable once parsed)

```go
// GoogleServiceAccountKey is a parsed and validated Google service account JSON key.
// Required fields: type (must be "service_account"), private_key (non-empty),
// client_email (non-empty), token_uri (non-empty), client_id (non-empty).
//
// Note: private_key is stored as raw bytes from the JSON string value.
// Cryptographic validity of the key is NOT verified (per FR-014).
type GoogleServiceAccountKey struct {
    Type        string // always "service_account"
    ClientEmail string // from json:"client_email"
    PrivateKey  []byte // from json:"private_key", non-empty, not crypto-validated
    TokenURI    string // from json:"token_uri"
    ClientID    string // from json:"client_id"
}

// ParseGoogleServiceAccountKey parses and validates a Google service account JSON string.
//
// Validation steps (in order):
//  1. Size check: credential must be ≤ 32 KB (FR-012, SR-004)
//  2. JSON validity: json.Unmarshal must succeed (FR-008)
//  3. Type check: type must be "service_account" via google.JWTConfigFromJSON (FR-007)
//  4. Field presence: type, client_email, private_key, token_uri, client_id must be present (FR-005)
//  5. Non-empty: private_key, client_email, token_uri, client_id must be non-empty strings (FR-014)
//
// Returns a populated GoogleServiceAccountKey or a detailed error naming the failing field(s).
// The private key is stored as raw bytes; its cryptographic format is not validated (FR-014).
func ParseGoogleServiceAccountKey(credential string) (*GoogleServiceAccountKey, error)
```

**Key design notes**:
- Uses `golang.org/x/oauth2/google.JWTConfigFromJSON` for steps 2–3 (JSON parse + type validation)
- Uses auxiliary `struct { ClientID string \`json:"client_id"\` }` for `client_id` extraction (not exposed by the library)
- Validates non-emptiness of `PrivateKey` (from `jwt.Config.PrivateKey` length), `ClientEmail` (from `jwt.Config.Email`), `TokenURI` (from `jwt.Config.TokenURL`)
- Does NOT call any RSA or crypto parse functions on `PrivateKey` (satisfies FR-014)
- Error messages name the missing/empty field explicitly (satisfies API-004)
- Credential logging is never performed inside this function (satisfies SR-002, SR-003)

**Validation error format**: Plain English, field-specific. Examples:
- `"private_key is required (must be a non-empty string)"`
- `"client_id is required in Google service account JSON"`
- `"credential exceeds maximum size of 32 KB"`
- `"credential type must be \"service_account\" (got \"authorized_user\")"`

---

## Extended Entity

### `ThirdpartyOAuth2ProviderEntity` (extended)

**Location**: `internal/domain/model/thirdparty_oauth2_provider.go`

**New field**:
```go
Flavor OAuth2Flavor  // defaults to OAuth2FlavorStandard when zero
```

**`ValidateForCreate` changes** (flavor-dispatched):

```
Standard flavor (existing behavior preserved):
  - client_id: required, non-empty
  - credential: non-empty string
  - issuer_uri: required, HTTPS check
  - endpoints: required when discovery disabled

Google flavor (new behavior):
  - client_id: NOT required (derived from JSON; entity.ClientID must be set by handler before validate)
  - credential: parsed via ParseGoogleServiceAccountKey (all google validation rules)
  - issuer_uri: OPTIONAL
      - if empty: entity.IssuerURI is set to scheme+host of token_uri from JSON by handler
      - if provided: scheme+host must match scheme+host of token_uri in JSON
  - endpoints: NOT required (token_endpoint = token_uri from JSON; authorize_endpoint = Google default)
```

**`ValidateForUpdate` changes**: Same dispatch as `ValidateForCreate`.

**`Copy()` changes**: Copy `Flavor` field (string value, no allocation needed).

**`RedactedCopy()` changes**: Flavor is not sensitive; copy as-is.

---

## Domain Events (Conceptual)

### `ThirdPartyServiceFlavorChanged`

Emitted (conceptually) when a `ThirdpartyOAuth2ProviderEntity`'s `Flavor` changes on update. This triggers re-validation of the credential against the new flavor's rules.

**Implementation**: The event is implicit — when `UpdateService` receives a request with a different `oauth2_flavor` than the stored service, the handler constructs the entity with the new flavor and calls `ValidateForUpdate`. The repository `Update` then persists both the new credential and the new flavor atomically.

---

## Database Schema

### `thirdparty_oauth2_services` (extended)

**New column** (migration 007):

| Column | Type | Constraint | Default |
|---|---|---|---|
| `oauth2_flavor` | `VARCHAR(50)` | `NOT NULL` | `'standard'` |

**Migration 007 up** (`007_add_oauth2_flavor.up.sql`):
```sql
ALTER TABLE thirdparty_oauth2_services
    ADD COLUMN oauth2_flavor VARCHAR(50) NOT NULL DEFAULT 'standard';

COMMENT ON COLUMN thirdparty_oauth2_services.oauth2_flavor
    IS 'OAuth2 authentication variant: standard (plain client_secret) or google (service account JSON key)';
```

**Migration 007 down** (`007_add_oauth2_flavor.down.sql`):
```sql
ALTER TABLE thirdparty_oauth2_services
    DROP COLUMN oauth2_flavor;
```

**Backward compatibility**: Existing rows automatically get `oauth2_flavor = 'standard'` via the column DEFAULT. No data migration needed.

---

## ARCHITECTURE.md Glossary Additions

The following terms must be added to the Glossary section of `ARCHITECTURE.md` (per Constitution Principle V):

| Term | Definition |
|---|---|
| **OAuth2Flavor** | Named enumeration on `ThirdpartyOAuth2Service` identifying the credential format and future token acquisition mechanism. Current values: `standard` (plain client secret string), `google` (Google service account JSON key). Designed for extension. |
| **ClientCredential** | The authentication material stored in the `client_secret` field of a `ThirdpartyOAuth2Service`. Structure varies by `OAuth2Flavor`: a plain secret string for `standard`, a serialized Google service account JSON string for `google`. Always encrypted at rest. Field name preserved for API backward compatibility. |
| **GoogleServiceAccountKey** | Structured value object representing the parsed contents of a Google service account JSON key file. Required fields: `type` (must be `"service_account"`), `private_key`, `client_email`, `token_uri`, `client_id`. Validated structurally; cryptographic format of the private key is not verified at configuration time. |

---

## Relationships to Existing Types

```
ThirdpartyOAuth2ProviderEntity
  ├── Secret (ClientCredential): value object, 2 states (plaintext/encrypted)
  │   └── For google flavor, plaintext = Google service account JSON string
  ├── Flavor: OAuth2Flavor                 ← NEW
  └── (other fields unchanged)

OAuth2Flavor: "standard" | "google"        ← NEW enum
GoogleServiceAccountKey                    ← NEW value object, used in validation only
  └── parsed from Secret.plaintext when Flavor == "google"
```

The `GoogleServiceAccountKey` value object is used **only during validation** (in `ParseGoogleServiceAccountKey`). It is not stored as a separate entity or field — the raw JSON string remains in `Secret.plaintext` until encrypted by the domain service. After encryption, only the ciphertext is persisted; decryption returns the original JSON string.
