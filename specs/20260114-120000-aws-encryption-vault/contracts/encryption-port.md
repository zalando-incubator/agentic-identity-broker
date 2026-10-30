# Contract: EncryptionPort Interface

**Location**: `internal/ports/encryption.go`

**Purpose**: Defines the contract for encrypting and decrypting OAuth tokens with authenticated encryption context binding. This port allows the domain to remain independent of specific encryption implementations.

> **Implementation note (superseded configuration):**
> This document records the original single-field encryption proposal
> (`encryption.key` / `key_encryption_key` with `${ENCRYPTION_KEK}`).
> The shipped implementation uses a backend-explicit contract:
> `encryption.aws_kms` or `encryption.memory`.
> See `internal/ports/config.go` and `docs/configuration.md`
> for the live schema.

---

## Interface Definition

```go
type EncryptionPort interface {
    Encrypt(ctx context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error)
    Decrypt(ctx context.Context, ciphertext []byte, encryptionContext map[string]string) ([]byte, error)
}
```

---

## Methods

### Encrypt

**Signature**:

```go
Encrypt(ctx context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error)
```

**Purpose**: Encrypts plaintext data with authenticated encryption context binding.

**Parameters**:

- `ctx context.Context` - Cancellation and timeout context
- `plaintext []byte` - Token data to encrypt (e.g., OAuth access token)
- `encryptionContext map[string]string` - AAD (Authenticated Additional Data) binding encryption to service context

**Returns**:

- `[]byte` - Encrypted ciphertext (opaque format containing wrapped DEK + ciphertext + auth tag)
- `error` - Error if encryption fails (see Error Contract)

**Encryption Context Format**:

```go
encryptionContext := map[string]string{
    "service_id": "oauth2",  // OAuth service identifier
}
```

**Behavior**:

1. Generate fresh DEK (Data Encryption Key) using cryptographically secure randomness
2. Encrypt plaintext using DEK with AESGCMSIV (AES-256 in GCM-SIV mode)
3. Include `encryptionContext` as AAD (authenticated but not encrypted)
4. Wrap DEK using KEK (Key Encryption Key) with same `encryptionContext`
5. Return opaque ciphertext format: [wrapped_DEK || ciphertext || auth_tag]
6. Zero DEK and plaintext buffers in memory after encryption

**Error Cases**:

- `ErrorKindEncryptionFailed`: DEK generation or encryption operation failed
- `ErrorKindKEKUnavailable`: KEK (AWS KMS or env var) is not accessible
- `context.Canceled` or `context.DeadlineExceeded`: Context cancelled or timed out

**Performance Target**: <100ms (excluding AWS KMS network latency; local encryption <5ms)

---

### Decrypt

**Signature**:

```go
Decrypt(ctx context.Context, ciphertext []byte, encryptionContext map[string]string) ([]byte, error)
```

**Purpose**: Decrypts ciphertext using authenticated encryption context verification.

**Parameters**:

- `ctx context.Context` - Cancellation and timeout context
- `ciphertext []byte` - Encrypted token (opaque format from Encrypt)
- `encryptionContext map[string]string` - AAD must match context used during encryption

**Returns**:

- `[]byte` - Decrypted plaintext token
- `error` - Error if decryption fails (see Error Contract)

**Encryption Context Format** (must match Encrypt):

```go
encryptionContext := map[string]string{
    "service_id": "oauth2",  // Must match service_id from encryption
}
```

**Behavior**:

1. Extract wrapped DEK from ciphertext
2. Unwrap DEK using KEK with provided `encryptionContext` for authentication
3. Verify `encryptionContext` matches AAD bound during encryption (fails if context mismatch)
4. Decrypt ciphertext using unwrapped DEK with AESGCMSIV
5. Verify AESGCMSIV authentication tag (fails if tampered)
6. Zero DEK and ciphertext buffers in memory after decryption
7. Return plaintext token

**Error Cases**:

- `ErrorKindDecryptionFailed`: Decryption operation failed or auth tag verification failed
- `ErrorKindContextMismatch`: Provided `encryptionContext` doesn't match AAD bound during encryption
- `ErrorKindIntegrityViolation`: Ciphertext authentication tag verification failed (possible tampering)
- `ErrorKindKEKUnavailable`: KEK (AWS KMS or env var) is not accessible
- `context.Canceled` or `context.DeadlineExceeded`: Context cancelled or timed out

**Performance Target**: <100ms (excluding AWS KMS network latency; local decryption <5ms)

---

## Implementation Requirements

### Memory Protection

1. **DEK Handling** (AWS Encryption SDK managed):
   - DEK is generated internally by AWS Encryption SDK
   - Not directly controllable by adapter, but AWS SDK uses secure random generation
   - After DEK wrapping (encryption) and unwrapping (decryption), DEK is discarded
   - Adapter handles plaintext token input/output securely

2. **KEK and Branch Key Handling** (depends on backend):

   **AWS KMS Backend with Hierarchical Keyring (Recommended)**:
   - KEK never exists as plaintext in application (stored only as ARN/key ID)
   - AWS KMS handles all KEK material server-side
   - **Branch Key** (intermediate cache): Generated from KEK, cached locally for TTL period
   - Branch Key memory protection deferred to future memory hardening feature:
     - Secure handling of cached Branch Keys
     - Memory protection for sensitive key material
     - Zero Branch Keys when cache evicts (TTL expiration or memory pressure)
   - TTL configuration balances security vs. performance (recommend 15 minutes)
   - Note: AWS SDK handles key material securely during wrapping/unwrapping operations

   **Direct KMS Calls (Alternative, Higher Security)**:
   - No Branch Key caching; no keys cached in memory between operations
   - Key material only exists during wrap/unwrap operations
   - AWS SDK handles operation-local key protection
   - Higher latency (50-200ms per operation) but lower memory attack surface

   **Environment Variable Backend** (local key material):
   - Load base64-encoded KEK material from environment at adapter initialization
   - Memory protection deferred to future memory hardening feature
   - Pass to AWS SDK's keyring initialization (AWS SDK handles it from there)
   - KEK material persists for adapter lifetime; zeroed on adapter shutdown
   - Note: AWS SDK handles key material securely after initialization

3. **Plaintext Handling**:
   - Secure handling of plaintext tokens deferred to future memory hardening feature
   - Zero plaintext after encryption completes
   - Calling code responsible for zeroing decrypted plaintext after use
   - Adapter does not manage caller's token buffers

4. **Ciphertext**:
   - Not secret; no special memory protection required

### Encryption Algorithm

- **Cipher**: AES-256-GCM-SIV (Serialized GCM-SIV authenticated encryption)
- **Library**: AWS Encryption SDK for Go (github.com/aws/aws-encryption-sdk-go/v3)
- **Key Size**: 256-bit (32 bytes) for DEK
- **IV/Nonce**: Generated per-operation by AESGCMSIV
- **AAD**: `encryptionContext` serialized and included in authentication

### KEK (Key Encryption Key) Support

**AWS KMS with Hierarchical Keyring (Recommended)**:

- KEK provided as ARN: `arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012`
- Adapter uses AWS Encryption SDK's Hierarchical Keyring with **Branch Key caching**
- **Three-layer architecture**:
  1. **KEK**: Stored in AWS KMS (never in plaintext in application)
  2. **Branch Key**: Cached locally for TTL period (~15 minutes, reduces KMS calls)
  3. **DEK**: Generated from Branch Key per service_id context (fresh per encryption operation)
- **Performance**:
  - First call per service_id: ~50-200ms (KMS roundtrip)
  - Subsequent calls during TTL: ~1-5ms (local cache hit)
- **Memory Profile**:
  - ~200 bytes per cached Branch Key
  - Typical overhead: <1MB for 100+ service contexts
- **Security**:
  - Configurable TTL (default 1 hour; recommend 15 minutes for higher security)
  - Context binding prevents cross-service key reuse
- **Automatic key rotation**: Backward compatibility (version byte in wrapped DEK)

**Alternative: Direct KMS Calls (No Caching)**:

- KEK provided as ARN (same format)
- No Branch Key caching; each operation calls AWS KMS
- **Performance**: 50-200ms per operation (consistent latency)
- **Security**: Minimal attack surface (no keys cached in memory)
- **Use case**: High-security environments, low-throughput systems, compliance requirements
- **Cost**: Higher AWS KMS charges (~10x vs. caching)

**Environment Variable (Development)**:

- KEK provided as base64-encoded key material via `${ENCRYPTION_KEK}` in config
- Adapter decodes and uses locally (no network calls)
- Latency: <5ms per operation
- No automatic rotation; manual key management required

### Context Binding

- `encryptionContext` is included as Additional Authenticated Data (AAD)
- Cryptographically bound to ciphertext (tampering detected)
- Provides service-level isolation (decrypt with wrong service_id fails)
- Single field: `service_id` (OAuth service identifier)

### Fail-Closed Behavior

- **No plaintext fallback** on any encryption/decryption failure
- **No degradation** if KEK unavailable
- Errors propagate immediately to caller
- Caller must handle encryption failures securely

---

## Adapter Implementation

**Production Adapter**: `internal/adapters/encryption/aws/adapter.go`

- Implements EncryptionPort using AWS Encryption SDK
- Supports both AWS KMS and environment variable KEK
- Memory protection deferred to future memory hardening feature
- Validates KEK accessibility at startup (fail-fast)

**Testing Support**:

- Mock adapter for unit tests (in-memory encryption, deterministic)
- Real AWS KMS integration tests (testcontainers with a LocalStack-compatible AWS emulator)

---

## Usage Example (Service Layer)

```go
// Inject EncryptionPort into OAuth2Session service
type OAuth2SessionService struct {
    repository     UserSessionRepository
    encryptionPort ports.EncryptionPort
    eventPublisher EventPublisher
}

// Encrypt tokens during session creation
func (s *OAuth2SessionService) CreateSession(
    ctx context.Context,
    principal, serviceID, accessToken, refreshToken string,
    accessExpiry, refreshExpiry time.Time,
) (*UserSession, error) {
    encryptionContext := map[string]string{
        "service_id": serviceID,
    }

    // Encrypt access token
    encryptedAccess, err := s.encryptionPort.Encrypt(
        ctx,
        []byte(accessToken),
        encryptionContext,
    )
    if err != nil {
        s.eventPublisher.Publish(SessionEncryptionFailed{
            ServiceID: serviceID,
            ErrorKind: "encryption_failed",
        })
        return nil, fmt.Errorf("failed to encrypt access token: %w", err)
    }

    // Encrypt refresh token
    encryptedRefresh, err := s.encryptionPort.Encrypt(
        ctx,
        []byte(refreshToken),
        encryptionContext,
    )
    if err != nil {
        s.eventPublisher.Publish(SessionEncryptionFailed{
            ServiceID: serviceID,
            ErrorKind: "encryption_failed",
        })
        return nil, fmt.Errorf("failed to encrypt refresh token: %w", err)
    }

    // Create and store UserSession with encrypted tokens
    session := &UserSession{
        Principal:                principal,
        ServiceID:                serviceID,
        EncryptedAccessToken:     encryptedAccess,
        EncryptedRefreshToken:    encryptedRefresh,
        EncryptionContext:        EncryptionContext{ServiceID: serviceID},
        AccessTokenExpiry:        accessExpiry,
        RefreshTokenExpiry:       refreshExpiry,
    }

    if err := s.repository.Create(ctx, session); err != nil {
        return nil, fmt.Errorf("failed to store session: %w", err)
    }

    s.eventPublisher.Publish(SessionEncrypted{
        ServiceID: serviceID,
    })
    return session, nil
}

// Decrypt tokens during session retrieval
func (s *OAuth2SessionService) GetSession(
    ctx context.Context,
    sessionID string,
) (*UserSession, error) {
    encryptedSession, err := s.repository.Get(ctx, sessionID)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve session: %w", err)
    }

    encryptionContext := map[string]string{
        "service_id": encryptedSession.ServiceID,
    }

    // Decrypt access token
    plainAccessToken, err := s.encryptionPort.Decrypt(
        ctx,
        encryptedSession.EncryptedAccessToken,
        encryptionContext,
    )
    if err != nil {
        s.eventPublisher.Publish(SessionDecryptionFailed{
            ServiceID: encryptedSession.ServiceID,
            ErrorKind: "decryption_failed",
        })
        return nil, fmt.Errorf("failed to decrypt access token: %w", err)
    }

    // Decrypt refresh token
    plainRefreshToken, err := s.encryptionPort.Decrypt(
        ctx,
        encryptedSession.EncryptedRefreshToken,
        encryptionContext,
    )
    if err != nil {
        s.eventPublisher.Publish(SessionDecryptionFailed{
            ServiceID: encryptedSession.ServiceID,
            ErrorKind: "decryption_failed",
        })
        return nil, fmt.Errorf("failed to decrypt refresh token: %w", err)
    }

    s.eventPublisher.Publish(SessionDecrypted{
        ServiceID: encryptedSession.ServiceID,
    })

    // Return session with decrypted tokens to caller
    return &UserSession{
        ID:                   encryptedSession.ID,
        Principal:            encryptedSession.Principal,
        ServiceID:            encryptedSession.ServiceID,
        AccessToken:          string(plainAccessToken),      // Decrypted for caller
        RefreshToken:         string(plainRefreshToken),     // Decrypted for caller
        AccessTokenExpiry:    encryptedSession.AccessTokenExpiry,
        RefreshTokenExpiry:   encryptedSession.RefreshTokenExpiry,
        EncryptionContext:    encryptedSession.EncryptionContext,
    }, nil
}
```

---

**Version**: 1.0 | **Status**: Design Phase | **Last Updated**: 2026-01-16
