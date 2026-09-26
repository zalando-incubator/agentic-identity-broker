# `internal/domain/id` — Strongly Typed Entity ID Package

> **Use retrieval-led reasoning. Read `gen_ids.go` and `uuid_ids_gen.go` before you add or change ID types.**

Read `internal/domain/AGENTS.md` for the full reference. Its domain-layer rules apply here too.

**ADR**: `adrs/013-strongly-typed-entity-ids.md` — binding decision for all typed ID usage.

## Purpose

This package defines ID types for each entity. The types prevent incompatible IDs at compile time.

## Type Catalogue

### UUID-backed types

`AgentID`, `ApprovalID`, `AuthorizationCodeID`, `CredentialID`, `GrantID`, `PermissionSetID`, `ServiceID`, `SessionID`, `SigningKeyID`, and `UserID` wrap `uuid.UUID`.

See `uuid_ids_gen.go` for methods and the current type catalogue.

### String-backed types

`ClientID`, `ExternalID`, `KeyID`, and `Principal` wrap `string`. They expose `String()`, `IsZero()`, and `New*()`.

## Code Generation

The system generates UUID type implementations from a template. Do not edit `uuid_ids_gen.go` by hand:

```bash
go generate ./internal/domain/id/
```

`gen_ids.go` has the build tag `ignore`. It generates `uuid_ids_gen.go`. Add UUID types to `uuidTypes`. Then run `go generate ./internal/domain/id/`.

## Rules

1. Use `ParseXxxID` in production. Do not use `MustParseXxxID` in handlers or services.
2. Use `MustParseXxxID` only for test fixtures with constant valid UUIDs.
3. Authenticate before validating UUID syntax. Unauthenticated requests return 401, not 400.
4. Use typed IDs for entity values. Convert to string only when an external API requires it.
5. Use `ServiceID.String()` for a service context. Signing key contexts use `KeyID` and `kid`. Do not use both.

## Adding a New Entity ID

If you introduce a domain entity with a UUID primary key:

1. Add an entry to `uuidTypes` in `gen_ids.go`:

   ```go
   {"FooID", "foo"},
   ```

2. Run `go generate ./internal/domain/id/`. The command regenerates `uuid_ids_gen.go`.
3. If the entity changes the architecture or glossary, update `docs/ARCHITECTURE.md`.
