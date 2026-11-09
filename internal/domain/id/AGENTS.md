# `internal/domain/id` — Strongly Typed Entity ID Package

Read `internal/domain/id/gen_ids.go` and `internal/domain/id/uuid_ids_gen.go` before changes to UUID-backed ID types.
Paths in this file are relative to the repository root. Run commands from the repository root.

Read `internal/domain/AGENTS.md` for domain-layer rules and canonical sources.

**ADR**: `adrs/013-strongly-typed-entity-ids.md` — binding decision for all typed ID usage.

## Purpose

This package defines ID types for each entity. The types prevent incompatible IDs at compile time.

## Type Catalogue

### UUID-backed types

`AgentID`, `ApprovalID`, `AuthorizationCodeID`, `BusinessEventID`, `CredentialID`, `GrantID`, `PermissionSetID`, `ServiceID`, `SessionID`, `SigningKeyID`, and `UserID` wrap `uuid.UUID`.

Read `internal/domain/id/uuid_ids_gen.go` for JSON, SQL, and text serialization methods.

`BusinessEventID` uses UUIDv7. Other UUID-backed constructors use UUIDv4.
Before you change these rules, read the [ledger data model](../../../specs/048-business-event-ledger/data-model.md).

### String-backed types

`ClientID`, `ExternalID`, `KeyID`, and `Principal` wrap `string`. They expose `String()`, `IsZero()`, and `New*()`.
Read `internal/domain/id/string_ids.go` for these types. Their constructors wrap strings without syntax validation.

## Code Generation

Do not edit `internal/domain/id/uuid_ids_gen.go` by hand.
The directive in `internal/domain/id/gen.go` runs the generator:

```bash
go generate ./internal/domain/id/
```

`internal/domain/id/gen_ids.go` uses the build tag `ignore`.
Its `uuidTypes` list and template define the UUID-backed types.
Read `internal/domain/id/id_test.go` for parse, zero-value, and serialization tests.

## Rules

1. Use `ParseXxxID` in production. Do not use `MustParseXxxID` in production handlers, services, or adapters.
2. Use `MustParseXxxID` only for test fixtures with constant valid UUIDs.
3. Authenticate before UUID syntax validation at handler entry points. Unauthenticated requests return 401, not 400.
4. Use typed IDs for entity values. If an external API requires strings, convert the IDs at that boundary.
5. Keep UUID-backed `SigningKeyID` separate from string-backed `KeyID`, which identifies the JWT `kid`.
6. Use `ServiceID.String()` for `service_id` encryption contexts. Use `KeyID` for `kid` contexts, including CIMD keys.
7. Supply only one context subject. Read `internal/domain/encryption/subject.go` for the constructors.
8. Log a `Principal` as the typed value, not `String()`. The principal policy in `coding-standards/LOGGING.md` section 5 depends on the type.
9. Set a `Principal` on a span only with `Principal.SpanAttribute(key)` under a `.principal` key. Read `coding-standards/OPENTELEMETRY.md` section 4.3. Until #356 adds `SpanAttribute`, do not add principal span attributes.

## Adding a New Entity ID

If you introduce a domain entity with a UUID primary key:

1. Add an entry to `uuidTypes` in `internal/domain/id/gen_ids.go`:

   ```go
   {"FooID", "foo"},
   ```

2. Run `go generate ./internal/domain/id/`. Commit the regenerated `internal/domain/id/uuid_ids_gen.go` with the generator change.
3. Add the new type to this file, as required by ADR 013.
4. Add tests to `internal/domain/id/id_test.go` for the new type and its serialization behavior.
5. Update the domain model and glossary in `ARCHITECTURE.md` for the new entity.
