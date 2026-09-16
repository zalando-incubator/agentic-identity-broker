# ADR 012: Encryption Layer Separation (Domain vs. Storage Adapter)

## Status
Accepted

## Context
The project uses OAuth2 tokens and OAuth2 provider secrets that require encryption at rest.
Two approaches are possible:

1. **Adapter Encryption**: Repository adapter handles encryption/decryption
   - Adapter receives plaintext, returns plaintext
   - Repository has EncryptionPort dependency

2. **Domain Encryption**: Domain service handles encryption/decryption
   - Service encrypts before calling repository
   - Repository operates on encrypted bytes (opaque binary)
   - Repository has no crypto dependencies

## Decision
Adopt "Domain Encryption" as the standard for all entities. This decision is now fully
implemented across all sensitive data:

- **ThirdpartyOAuth2Provider** — `ThirdpartyOAuth2ProviderService` owns all encrypt/decrypt.
  The `Secret` value object (`internal/domain/model/secret.go`) enforces this at the type
  level: adapters call `Secret.GetCiphertext()` which errors if the secret was never
  encrypted, making plaintext persistence impossible.
- **UserSession** — `OAuth2SessionService` encrypts/decrypts access and refresh tokens.

Both repositories (`ThirdpartyOAuth2ProviderRepository`, `UserSessionRepository`) operate
on opaque encrypted bytes and have no knowledge of encryption mechanics.

## Secret Value Object
The `model.Secret` value object (`internal/domain/model/secret.go`) provides type-level
encryption safety:

- `NewPlaintextSecret(s)` — constructs a secret in plaintext state (before encryption)
- `NewEncryptedSecret(b)` — constructs a secret in ciphertext state (after encryption)
- `GetPlaintext() (string, error)` — errors if secret is encrypted (forces explicit decrypt)
- `GetCiphertext() ([]byte, error)` — errors if secret is plaintext (prevents storing before encrypt)
- `Redacted() string` — always returns `"REDACTED"` regardless of state (safe for logs/API)

This immutable two-state design makes it impossible to accidentally persist a plaintext secret
or accidentally expose ciphertext as a string.
>
> **Amended by ADR 036:** ADR 036 supersedes this two-state description. `Secret` now has an explicitly constructed absent state, reachable only through `NewAbsentSecret()`. Plaintext remains impossible to persist because `GetCiphertext()` errors for plaintext.

## Rationale

### Hexagonal Architecture Purity (++++)
- Domain service owns business logic (including data protection)
- Repository is pure persistence layer (no crypto concerns)
- Cleaner dependency graph: domain → port, adapter → domain+port

### Separation of Concerns (+++)
- Service layer: Business logic + encryption orchestration
- Repository layer: CRUD operations only
- Each layer has single responsibility

### Adapter Simplicity (++)
- Repositories: 50-100 lines (CRUD only)
- No encryption logic in adapters
- Easier to add new storage backends

### Testability (++)
- Repository tests: Test persistence without mocking encryption
- Service tests: Test encryption logic without storage
- Clear test boundaries

### Consistency (+)
- Same pattern across all sensitive data entities
- Reduced cognitive load for developers
- Clearer code review standards

## Consequences

### Benefits
✅ Domain logic clearly visible in service layer
✅ Repositories are infrastructure-agnostic
✅ Easy to swap storage backends
✅ Simpler error handling (no crypto logic in adapters)
✅ Type-level encryption safety via `Secret` value object
✅ Mandatory encryption at startup (no NoOp fallback)

### Constraints
- Service layer becomes more complex (coordinates encrypt/decrypt with repo calls)
- All consumer code must go through domain services, never raw repositories

## Related
- ADR 004: Storage Layer Architecture (hexagonal pattern)
- ADR 008: Encryption Context Optimization (service_id only)
- ADR 009: Envelope Encryption Design (three-layer key hierarchy)
- Feature 012: Token Vault (envelope encryption implementation)
