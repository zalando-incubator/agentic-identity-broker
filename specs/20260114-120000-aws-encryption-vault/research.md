# Research: AWS Encryption Vault for OAuth Tokens (Phase 0)

**Date**: 2026-01-16 | **Feature**: 20260114-120000-aws-encryption-vault | **Status**: Complete

> **Implementation note (superseded configuration):**
> This document records the original single-field encryption proposal
> (`encryption.key` / `key_encryption_key` with `${ENCRYPTION_KEK}`).
> The shipped implementation uses a backend-explicit contract:
> `encryption.aws_kms` or `encryption.memory`.
> See `internal/ports/config.go` and `docs/configuration.md`
> for the live schema.

## Critical Finding: Official AWS Encryption SDK for Go Available

### Decision: Use Official AWS Encryption SDK for Go

**Rationale**:

- AWS maintains official AWS Encryption SDK for Go at: `github.com/aws/aws-encryption-sdk/releases/go`
- Provides battle-tested, production-grade envelope encryption implementation
- Supports AESGCMSIV (Encrypt-then-MAC with counter mode) authenticated encryption
- Integrated AWS KMS keyring support with context binding
- Post-quantum cryptography support ready for Go 1.24+
- Eliminates custom cryptography implementation risk

**Implementation Approach**:

1. AWS Encryption SDK handles DEK generation, encryption, and wrapping lifecycle
2. AWS KMS keyring for KEK management (wrapping/unwrapping DEK with context)
3. SDK enforces context verification at both DEK and KEK layers
4. Memory protection deferred to future memory hardening feature
5. Structured logging via project's existing slog with SecureLogger wrapper

**Advantages**:

- Official AWS implementation—no maintenance risk
- Context binding is native and auditable
- Fail-closed behavior built in
- Performance optimized by AWS team
- Aligns with AWS best practices and recommendations

**Risks Mitigated**:

- No custom cryptography
- No reliance on unmaintained community ports
- Context binding is explicit and verified by SDK

---

## Area 1: AWS Encryption SDK for Go - Envelope Encryption

### Research Question

How to implement production-grade envelope encryption using official AWS Encryption SDK for Go?

### Findings

**AWS Encryption SDK Architecture**:

- Uses keyring abstraction for key management
- AWS KMS keyring implements Key Encryption Key (KEK) operations
- Supports custom keyrings for environment variable KEK injection (development)
- AESGCMSIV algorithm: Encrypt-then-MAC with counter mode (built-in)
- Context binding via `EncryptionContext` map (service_id only per spec)

**Core API Pattern**:

```go
import "github.com/aws/aws-encryption-sdk-go/v3/sdk"

// Initialize client with AWS KMS keyring
keyring := aws.NewKeyring(cfg, keyID, encryptionContext)
client := sdk.NewClient(keyring)

// Encryption
result, err := client.Encrypt(ctx, plaintext, encryptionContext)
ciphertext := result.Result()

// Decryption with automatic context verification
result, err := client.Decrypt(ctx, ciphertext)
plaintext := result.Result()
```

**Envelope Structure**:

- SDK handles envelope serialization internally
- Serialized format: message format identifier + algorithm suite + encrypted data material (wrapped DEK + encrypted token)
- All handled by SDK—application doesn't construct envelope manually

**DEK Generation**:

- AWS SDK generates unique DEK per Encrypt() call
- 256 bits (32 bytes) of cryptographically secure randomness
- Per spec: DEKs per service_id context (all tokens for a service use DEKs wrapped with service-specific branch key)

**Context Binding**:

- `EncryptionContext` parameter is map[string]string: `{"service_id": "oauth2"}`
- Context is verified at both:
  1. DEK encryption layer (AESGCMSIV authenticated encryption)
  2. KEK wrapping layer (AWS KMS Encrypt with EncryptionContext)
- Mismatch at either layer causes decryption to fail immediately (fail-closed)

**AWS KMS Keyring Integration**:

```go
// AWS KMS keyring uses KMS for DEK wrapping
keyring := aws.NewKeyring(kmsClient, keyARN, encryptionContext)

// KMS automatically:
// - Wraps DEK with current KEK version
// - Verifies context during unwrap
// - Maintains backward compatibility with old KEK versions
```

**Key Rotation Support**:

- AWS KMS handles key rotation automatically via key versioning
- Specify key by ARN or alias; KMS uses current version for encryption
- Old tokens encrypted with previous KEK version remain decryptable
- No custom rotation logic needed

### Best Practices

- ✅ Use AWS KMS ARN or alias for key specification
- ✅ Always bind context (service_id) via EncryptionContext parameter
- ✅ Let SDK handle fail-closed behavior on context mismatch
- ✅ Delegate KMS retry logic to AWS SDK (no custom retries)
- ✅ Performance: local ops <50ms, KMS latency 50-200ms typical (operator concern)
- ✅ Cache keyring instance for multiple operations (avoid recreating)

---

## Area 2: Memory Protection

### Research Question

How to handle sensitive data (plaintext tokens, DEKs) in memory securely?

### Findings

**Memory Protection Strategy**:
Memory protection for sensitive data (plaintext tokens, DEKs, KEK material) is deferred to a future memory hardening feature specification that will address:

- Protected buffers for keys (DEK, KEK) and plaintext tokens
- Automatic secure zeroing after use
- Memory locking (mlock) to prevent page swapping to disk
- Core dump exclusion to prevent post-mortem forensics
- Platform-specific memory protection mechanisms

**Current Approach**:

- AWS Encryption SDK provides baseline memory protection during operations
- Explicit zeroing of sensitive buffers where possible
- Secure handling patterns for key material
- Graceful operation without advanced memory protection features

**Future Memory Hardening Feature Will Address**:

- Protected memory enclaves for sensitive data
- Memory locking and core dump exclusion
- Platform-specific memory protection (Linux, macOS, Windows)
- Container compatibility and capability requirements
- Performance optimization for memory protection operations
- Testing and validation of memory protection behavior

### Best Practices

- ✅ Zero plaintext and key material after use where possible
- ✅ Rely on AWS SDK baseline memory protection during operations
- ✅ Plan for future memory hardening feature integration
- ✅ Document memory protection requirements for future implementation

---

## Area 3: Structured Logging for Encryption Operations

### Research Question

What structured logging patterns enable audit compliance while preventing sensitive data leakage?

### Findings

**Canonical Log Schema** (aligns with project's slog):

```json
{
  "operation": "encrypt|decrypt",
  "service_id": "oauth2",
  "success": true|false,
  "token_type": "access|refresh",
  "timestamp": "2026-01-16T10:30:45.123Z",
  "duration_ms": 45,
  "algorithm": "AESGCMSIV",
  "error_kind": "context_mismatch|kek_unavailable|decrypt_failed|invalid_ciphertext",
  "request_id": "correlation_id",
  "principal": "user@example.com"
}
```

**Error Classification**:

- `context_mismatch`: Context doesn't match at DEK or KMS layer
- `kek_unavailable`: KMS unreachable, retry exhausted
- `decrypt_failed`: Authentication tag verification failed (tampered data)
- `invalid_ciphertext`: Envelope parsing error

**What NOT to Log**:

- ❌ Plaintext OAuth tokens or fragments
- ❌ Encryption keys (DEK, KEK, nonces)
- ❌ Decrypted token content
- ❌ Raw SDK error messages (classify to error_kind)

**What to Log**:

- ✅ Operation type (encrypt/decrypt)
- ✅ Service context (service_id, request_id)
- ✅ Success/failure + error classification
- ✅ Performance metrics (duration_ms)
- ✅ Metadata (algorithm, key_version, ciphertext_size_bytes)

**Integration with Project**:

- Project uses Go's `log/slog` for structured JSON
- Extend existing audit patterns (OAuth2 middleware)
- SecureLogger wrapper for encryption operations
- 100% of failures logged (compliance)
- Sample of successes for performance monitoring (e.g., 1% sampling)

### Best Practices

- ✅ Use project's existing slog with SecureLogger wrapper
- ✅ Classify errors into safe categories
- ✅ Never expose raw crypto errors
- ✅ Log 100% of failures for compliance
- ✅ Redact patterns in error messages

---

## Area 4: Environment Variable KEK Injection for Development

### Research Question

How to support `${ENCRYPTION_KEK}` environment variable injection for dev while maintaining AWS KMS for production?

### Findings

**Configuration System** (leverages feature 20251214-120100-flexible-configuration):

- Single field: `encryption.key`
- Supports AWS KMS ARN format for production
- Supports `${ENCRYPTION_KEK}` env var reference for development
- Configuration loader resolves at startup

**Custom Keyring for Env Var KEK**:

- AWS Encryption SDK keyring abstraction allows custom implementations
- Create local keyring that uses environment variable KEK
- Use same EncryptionContext binding as AWS KMS keyring
- Fallback mechanism: if env var not set, fail fast

**AWS KMS vs. Env Var KEK**:

| Aspect | AWS KMS (Production) | Env Var (Development) |
|--------|---|---|
| Key Rotation | ✅ Automatic via versioning | ❌ Manual (ephemeral data) |
| Backward Compat | ✅ Old tokens decryptable | ❌ Old tokens fail on KEK change |
| Auditing | ✅ CloudTrail logs | ❌ Env var changes not audited |
| Performance | 50-200ms per operation | <5ms per operation |

**Configuration Examples**:

```yaml
# Production
encryption:
  key_encryption_key: arn:aws:kms:us-east-1:123456789:key/12345678

# Local development
encryption:
  key_encryption_key: ${ENCRYPTION_KEK}
```

### Best Practices

- ✅ Fail fast at startup if KEK missing or invalid
- ✅ No fallback to plaintext KEK—always fail closed
- ✅ Document that production MUST use AWS KMS
- ✅ Env var KEK is development/testing only
- ✅ Warn developers: changing ENCRYPTION_KEK breaks old sessions (acceptable in dev)

---

## Summary of Research Findings

| Area | Decision | Confidence | Risk Mitigation |
|------|----------|-----------|-----------------|
| AWS Encryption SDK | Use official AWS SDK for Go | High | Official AWS implementation; no maintenance risk |
| DEK/KEK Architecture | AWS SDK envelope, KMS keyring, context at both layers | High | SDK handles compliance; AWS native support |
| Memory Protection | Deferred to future memory hardening feature | High | AWS SDK baseline protection; advanced features deferred |
| Logging | Structured JSON with canonical schema; project's slog | High | Aligns with existing patterns; audit-ready |
| Env Var KEK | Development-only via `${ENCRYPTION_KEK}` interpolation | High | Ephemeral dev environments don't need rotation |
| Error Handling | AWS SDK retry delegation; fail-closed on context mismatch | High | Prevents custom retry bugs; security-first |
| Performance | Local <50ms achievable; KMS latency operator concern | High | AWS SDK optimized; KMS SLA documented |

---

## Recommendations for Phase 1 Design

1. **Implement EncryptionPort interface** (Encrypt/Decrypt methods)
2. **Create AWS KMS adapter** wrapping AWS Encryption SDK
3. **Create custom keyring adapter** for environment variable KEK
4. **Memory protection** deferred to future memory hardening feature
5. **Add SecureLogger wrapper** to project's slog
6. **Write unit tests first** (TDD): encryption/decryption, context binding, error cases
7. **Integration tests**: real AWS KMS via LocalStack testcontainers

**Next Step**: Phase 1 design artifacts (data-model.md, contracts/, quickstart.md)
