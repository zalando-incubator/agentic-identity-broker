# Data Model: Encryption Vault for OAuth Tokens

**Date**: 2026-01-15 | **Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

> **Implementation note (superseded configuration):**
> This document records the original single-field encryption proposal
> (`encryption.key` / `key_encryption_key` with `${ENCRYPTION_KEK}`).
> The shipped implementation uses a backend-explicit contract:
> `encryption.aws_kms` or `encryption.memory`.
> See `internal/ports/config.go` and `docs/configuration.md`
> for the live schema.

## Overview

This feature integrates envelope encryption into the oauth2session service layer. The service encrypts tokens before passing UserSession to the repository, which stores already-encrypted tokens transparently. Decryption happens in the service layer when retrieving sessions.

**Key Principle**: Encryption is a service-layer concern. Storage adapters handle encrypted tokens as opaque BYTEA; oauth2session service owns encryption/decryption logic.

---

## Existing Entities (Reused as-is)

### UserSession Aggregate

**Location**: `internal/domain/storage/user_session.go`

**Fields** (already present):

- `encrypted_access_token: []byte (BYTEA)` - Stores ciphertext + wrapped DEK (opaque to storage adapters)
- `encrypted_refresh_token: []byte (BYTEA)` - Stores ciphertext + wrapped DEK (opaque to storage adapters)
- `encryption_context: map[string]string (JSONB)` - Contains `{"service_id": "oauth2"}`

**Storage Adapter Role**: Accept UserSession as-is; store/retrieve encrypted_access_token and encrypted_refresh_token BYTEA fields unchanged (no encryption/decryption logic).

### EncryptionContext Value Object

**Location**: `internal/domain/storage/user_session.go`

**Existing** `map[string]string` with single key:

- `service_id` - OAuth service identifier

**Oauth2session Service Role**: Constructs EncryptionContext and passes to EncryptionPort methods as AAD (Authenticated Additional Data).

### EncryptionPort Interface

**Location**: `internal/ports/encryption.go`

**Existing interface**:

```go
type EncryptionPort interface {
    Encrypt(ctx context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error)
    Decrypt(ctx context.Context, ciphertext []byte, encryptionContext map[string]string) ([]byte, error)
}
```

**Service Layer Role**: Oauth2session service calls port methods to encrypt/decrypt tokens; storage adapters never call EncryptionPort.

---

## Service Layer Integration

### OAuth2Session Service

**Location**: `internal/services/oauth2session/service.go` (or similar)

**Create Flow**:

```
1. Service receives request to create session with plaintext tokens
2. Construct encryption context: {"service_id": "<service>"}
3. Call port.Encrypt(access_token, context) → encrypted_access_token
4. Call port.Encrypt(refresh_token, context) → encrypted_refresh_token
5. Create UserSession with encrypted fields:
   - encrypted_access_token = ciphertext + wrapped DEK
   - encrypted_refresh_token = ciphertext + wrapped DEK
   - encryption_context = {"service_id": "<service>"}
6. Call repository.Create(userSession) → stored as-is (storage adapter sees encrypted tokens)
7. Publish SessionEncrypted domain event
8. On error: Fail securely, publish SessionEncryptionFailed, return error (no fallback)
```

**Get Flow**:

```
1. Service receives request to retrieve session by ID
2. Call repository.Get(sessionID) → returns UserSession with encrypted fields
3. Call port.Decrypt(encrypted_access_token, encryption_context) → plaintext_access
4. Call port.Decrypt(encrypted_refresh_token, encryption_context) → plaintext_refresh
5. Return session with decrypted tokens available to caller
6. Publish SessionDecrypted domain event
7. On error: Fail securely, publish SessionDecryptionFailed, return error (no fallback)
```

**Key Invariants**:

- Plaintext tokens never reach storage adapters
- Storage adapters see only encrypted UserSession objects
- Service layer owns all encryption/decryption logic
- Service layer constructs and manages EncryptionContext

---

## Minimal New Elements (Feature 20260114-120000)

### Error Types

**New file**: `internal/domain/encryption/errors.go`

```go
package encryption

type ErrorKind string

const (
    ErrorKindEncryptionFailed   ErrorKind = "encryption_failed"
    ErrorKindDecryptionFailed   ErrorKind = "decryption_failed"
    ErrorKindContextMismatch    ErrorKind = "context_mismatch"
    ErrorKindIntegrityViolation ErrorKind = "integrity_violation"
    ErrorKindKEKUnavailable     ErrorKind = "kek_unavailable"
)

type EncryptionError struct {
    Kind    ErrorKind
    Message string // Sanitized (no key material, no full tokens)
}
```

**Service Layer Role**: Service catches EncryptionError from port; publishes domain events; returns fail-closed error to caller.

---

### Domain Events (Session-Level)

**Extend file**: `internal/domain/storage/events.go` (add to existing events)

```go
// SessionEncrypted: Tokens encrypted successfully (before storage)
type SessionEncrypted struct {
    SessionID UUID
    ServiceID string
    Timestamp time.Time
}

// SessionDecrypted: Tokens decrypted successfully (after retrieval)
type SessionDecrypted struct {
    SessionID UUID
    ServiceID string
    Timestamp time.Time
}

// SessionEncryptionFailed: Encryption failed (storage prevented)
type SessionEncryptionFailed struct {
    SessionID UUID
    ServiceID string
    ErrorKind string
    Timestamp time.Time
}

// SessionDecryptionFailed: Decryption failed (retrieval failed)
type SessionDecryptionFailed struct {
    SessionID UUID
    ServiceID string
    ErrorKind string
    Timestamp time.Time
}
```

---

## Storage Adapter Behavior (Unchanged)

### Memory and PostgreSQL Adapters

**No Changes to Core Logic**:

- `Create(session)`: Receive UserSession with encrypted tokens; store as-is (BYTEA + JSONB)
- `Get(id)`: Retrieve UserSession with encrypted tokens; return as-is

**Adapter Responsibilities**:

- Store/retrieve encrypted_access_token BYTEA unchanged
- Store/retrieve encrypted_refresh_token BYTEA unchanged
- Store/retrieve encryption_context JSONB unchanged
- No encryption/decryption logic in adapter layer

---

## Configuration Integration

**Location**: `.env` file (feature 20251214-120100-flexible-configuration)

**Parameter**: `encryption.key`

**Resolution** (in AWS adapter initialization):

1. Read value from config
2. If matches `arn:aws:kms:...`: Create AWS KMS keyring
3. If matches `${...}`: Resolve environment variable, use raw key material
4. Validate KEK accessible at startup (fail-fast)

---

## Memory Protection

**Implementation**: Memory protection deferred to future memory hardening feature

- DEK: Generated securely, zeroed after use
- KEK: Loaded securely, zeroed after use
- Plaintext tokens: Handled securely, zeroed after encryption
- Advanced memory protection features deferred to future specification

---

## Backward Compatibility

**KEK Rotation**: AWS KMS/Encryption SDK handles transparently

- Old tokens (wrapped with v1 KEK) remain decryptable with new KEK version
- Version byte in wrapped DEK enables future algorithm migration

---

## Testing Strategy

**E2E Acceptance Tests** (24 scenarios from spec):

- Service layer tests: Encrypt → store → retrieve → decrypt roundtrips
- Error paths: Context mismatch, tampered ciphertext, KEK unavailable
- Memory protection: DEK/token buffers verified zeroed
- Storage adapter tests: Unchanged (still just store/retrieve encrypted bytes)

**Unit Tests**:

- EncryptionContext validation
- AWS adapter error paths
- Domain event emissions (service layer)

**Integration Tests**:

- Service → port → storage roundtrip with encryption
- KEK startup validation
- Multiple services with different service_ids

---

**Version**: 1.0 | **Status**: Design Phase | **Last Updated**: 2026-01-15
