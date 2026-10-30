# ADR 20260313-120000: Strongly Typed Entity IDs (`type XxxID uuid.UUID`)

**Status**: Accepted
**Date**: 2026-03-13

---

## Context

All entity IDs — `agents.id`, `thirdparty_oauth2_services.id`, `user_grants.id`, `user_sessions.id`, `users.id` — and non-UUID identity strings (`ClientID`, `ExternalID`, `Principal`) were originally plain `string` throughout the codebase.

This caused two classes of bugs:

1. **Silent cross-entity mixups**: Passing a `ServiceID` value where an `AgentID` is expected compiles and runs silently. The compiler cannot distinguish them because both are `string`. Runtime failures (e.g., "not found" errors or wrong data returned) are the only signal.
2. **Implicit UUID-validation gaps**: Code that accepts a raw `string` ID from an HTTP request must separately validate that it is a well-formed UUID before passing it to storage. This validation was inconsistently applied — some handlers used `uuid.MustParse` (which panics on invalid input) instead of `uuid.Parse` (which returns an error).

The codebase also contains ID-like strings that are intentionally *not* UUIDs: OAuth2 `client_id` values (`VARCHAR(255)`) and the `X-Remote-User` principal header value.

---

## Decision

We introduce a dedicated package `internal/domain/id` that defines one named type per entity:

### UUID-backed types (backed by `uuid.UUID = [16]byte`)

```go
type AgentID   uuid.UUID
type ServiceID  uuid.UUID
type GrantID   uuid.UUID
type SessionID  uuid.UUID
type UserID     uuid.UUID
```

### String-backed types (backed by `string`)

```go
type ClientID    string   // OAuth2 client_id, VARCHAR(255)
type ExternalID  string   // optional external governance ID, VARCHAR(255)
type Principal   string   // X-Remote-User header value
```

### Per-type API

Each UUID type implements the full serialization surface needed by JSON APIs, `sqlx` struct scanning, positional SQL args, and JSONB payloads:

```go
func NewXxxID() XxxID
func ParseXxxID(s string) (XxxID, error)      // safe; use in production
func MustParseXxxID(s string) XxxID           // panics; test fixtures ONLY

func (id XxxID) String() string
func (id XxxID) IsZero() bool
func (id XxxID) MarshalJSON() ([]byte, error)
func (id *XxxID) UnmarshalJSON(b []byte) error
func (id XxxID) Value() (driver.Value, error)
func (id *XxxID) Scan(src interface{}) error
func (id XxxID) MarshalText() ([]byte, error)
func (id *XxxID) UnmarshalText(b []byte) error
```

**Serialization implementation details**:
- `MarshalJSON`/`UnmarshalJSON` use explicit JSON string encoding (`json.Marshal`/`json.Unmarshal` into `string`, then `uuid.Parse`) to ensure correct quoted-string JSON output.
- `Scan`, `MarshalText`, and `UnmarshalText` delegate via the identity cast `(*uuid.UUID)(id)`, which is safe because `XxxID` has identical memory layout to `uuid.UUID` (`[16]byte`).

### Code generation

Five UUID types × ~10 methods = 50 one-liner methods. A `go:generate` template (`gen_ids.go`, build-ignored) eliminates repetition and keeps all types structurally identical. The generated output is committed as `uuid_ids_gen.go`.

---

## Rationale

### 1. Compile-time safety over runtime detection

Named types in Go make cross-entity assignments a compilation error. `AgentID` and `ServiceID` are distinct types; the compiler rejects passing one where the other is expected. This moves a class of bugs from runtime (test or production) to build time.

### 2. Zero-overhead — same wire/storage format

`uuid.UUID` is `[16]byte`. Casting between `AgentID` and `uuid.UUID` is a no-op at runtime. The `.String()` representation (`xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`) is identical to what the existing code stored in PostgreSQL and transmitted in JSON. No data migration required.

### 3. Transparent serialization

By implementing `json.Marshaler`/`json.Unmarshaler`, `driver.Valuer`/`sql.Scanner`, and `encoding.TextMarshaler`/`encoding.TextUnmarshaler`, the typed IDs work transparently with:
- `encoding/json` (HTTP request/response bodies)
- `sqlx` struct scanning (PostgreSQL UUID columns scan directly into `XxxID`)
- `database/sql` positional args (via `driver.Valuer`)
- JSONB columns (via `encoding.TextMarshaler`)

### 4. Eliminates panic risk in HTTP handlers

`MustParseXxxID` panics on malformed input. With typed IDs and `ParseXxxID` at every handler entry point, malformed UUID inputs produce HTTP 400 errors instead of process crashes. This is a direct security improvement.

### 5. Handler ordering constraint

Authentication (principal check) must occur *before* UUID format validation. Unauthenticated requests must receive HTTP 401, not HTTP 400. This ordering is enforced by code review and documented in `internal/domain/id/AGENTS.md`.

---

## Alternatives Considered

### A. Keep `string` but add linting rules

Rejected. Go linters cannot enforce "this `string` is an AgentID" semantics. The compile-time guarantee requires distinct types.

### B. Use `github.com/google/uuid.UUID` directly (no wrapper)

Rejected. `uuid.UUID` is a shared type — the compiler would still allow passing any `uuid.UUID` where an `AgentID` is expected. Named types are required for cross-entity safety.

### C. Generate with `stringer` or `mockery`

Rejected. The types have a fixed, stable shape (10 methods per UUID type) that a small custom template handles cleanly. External code generation tools would add a dependency for minimal benefit.

### D. Define types in each domain package

Rejected. IDs are shared across domain packages (`ServiceID` appears in `storage/`, `consent/`, `tokenexchange/`, `oauth2session/`). A dedicated `id` package avoids import cycles and provides a single canonical definition.

---

## Consequences

### Positive

- **Compile-time correctness**: Passing `ServiceID` where `AgentID` is expected is now a compile error.
- **No panics from malformed UUIDs**: All production parse paths use `ParseXxxID`, not `MustParseXxxID`.
- **Zero migration cost**: Wire and storage format unchanged (`uuid.UUID.String()` form, UUIDs in PostgreSQL).
- **Consistent serialization**: All IDs marshal/unmarshal the same way everywhere (JSON, SQL, text).
- **Single source of truth**: All entity ID types in one package, visible to all layers.

### Negative

- **Boilerplate at call sites**: Converting a path parameter string to an `AgentID` requires `id.ParseAgentID(chi.URLParam(r, "agent-id"))` instead of just reading the string. This is intentional friction that forces the developer to handle the error.
- **Generated file in version control**: `uuid_ids_gen.go` is committed. It must be regenerated after any change to `gen_ids.go`.

### Rules enforced by this ADR

1. No new entity ID fields may be plain `string` if the entity has a UUID primary key.
2. `MustParseXxxID` is forbidden in production code (handlers, domain services, adapters). Test fixtures are the only exception.
3. Handler entry points must check principal (authentication) before parsing UUIDs (input validation).
4. Every new domain entity with a UUID PK must add its type to `gen_ids.go` and document it in `internal/domain/id/AGENTS.md`.

---

## References

- Package: `internal/domain/id/`
- Package AGENTS: `internal/domain/id/AGENTS.md`
- Generator: `internal/domain/id/gen_ids.go`
- Generated file: `internal/domain/id/uuid_ids_gen.go`
- Domain layer rules: `internal/domain/AGENTS.md`
- Constitution Principle 5 (Domain-Driven Design): `.specify/memory/constitution.md`
