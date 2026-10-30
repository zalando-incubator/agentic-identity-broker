# ADR 20260116-120200: Envelope Encryption Design for OAuth2 Token Vault

**Status**: Accepted
**Date**: 2026-01-16

> **Implementation note (superseded configuration):**
> This document records the original single-field encryption proposal
> (`encryption.key` / `key_encryption_key` with `${ENCRYPTION_KEK}`).
> The shipped implementation uses a backend-explicit contract:
> `encryption.aws_kms` or `encryption.memory`.
> See `internal/ports/config.go` and `docs/configuration.md`
> for the live schema.

---

## Context

The Agentic Identity Broker requires secure storage of OAuth2 access and refresh tokens for third-party services (GitHub, Google, Microsoft, etc.). These tokens are sensitive credentials that must be protected both at rest and in transit. The system needs to support:

1. **High Security**: Protection against database compromise, memory dumps, and insider threats
2. **Performance**: Sub-100ms token encryption/decryption for session operations
3. **Scalability**: Efficient encryption for thousands of concurrent sessions
4. **Compliance**: Enterprise-grade key management and audit trails
5. **Context Isolation**: Cryptographic guarantees that tokens encrypted for one service cannot be decrypted for another

Previous analysis in [ADR 20260116-120000: Encryption Context Optimization](20260116-120000-encryption-context-optimization.md) established the performance and security benefits of service_id-only context binding for OAuth2 user-session tokens.

---

## Decision

We will implement **envelope encryption with DEK per service_id context** using **AWS KMS Hierarchical Keyring with Branch Key caching** as the primary key management solution, with **context binding** for service isolation.

> This ADR's `service_id` context rules apply specifically to the **OAuth2 token vault** (user-session access and refresh tokens). The later amendment to [ADR 20260116-120000](20260116-120000-encryption-context-optimization.md#amendment-2026-06-01-single-subject-context-extension-for-non-session-assets) extends the same single-subject pattern to non-session assets such as broker signing-key private material, which use `kid` instead of `service_id`.

### Core Architecture

1. **Three-Layer Key Hierarchy**
   - **KEK** (Key Encryption Key): Stored in AWS KMS, never in plaintext in application
   - **Branch Key** (per service_id): Derived from KEK, cached locally for TTL period (~15 minutes)
   - **DEK** (Data Encryption Key): Generated per encryption operation, wrapped using service-specific branch key

2. **AWS KMS Hierarchical Keyring with Branch Key Caching**
   - Customer-managed KMS key (CMK) serves as the Key Encryption Key
   - Hierarchical keyring uses DynamoDB to cache branch keys per service_id (reduces KMS calls)
   - First call per service_id: ~50-200ms (KMS roundtrip to generate branch key)
   - Subsequent calls during TTL: ~1-5ms (cache hit, no KMS call needed)

3. **Service-ID Context Binding**
   - EncryptionContext contains `{"service_id": "oauth2"}` as Additional Authenticated Data (AAD)
   - AESGCMSIV authenticated encryption prevents cross-service token usage
   - Cryptographic isolation between different third-party services

---

## Rationale

### 1. DEK per Service_ID with Branch Key Caching

**Chosen**: Branch keys cached per service_id context, fresh DEK per encryption operation

**Alternative Considered 1**: Per-token direct KMS calls
**Rejected Because**: 2x KMS calls per session (access_token, refresh_token) creates 200ms+ latency and scales poorly with concurrent sessions

**Alternative Considered 2**: Per-session DEK (single KMS call per session)
**Rejected Because**: Cannot reuse branch keys across sessions; misses performance optimization of branch key caching

**Implemented Approach**: Hierarchical Keyring with DynamoDB branch key cache

- Branch key generated once per service_id, cached for ~15 minutes
- Fresh DEK generated per encryption operation, wrapped with cached branch key
- Single KMS call per service_id per TTL window (not per session or per token)

**Benefits**:

- **Performance**: Cached branch keys provide ~1-5ms encryption operations (vs 50-200ms per direct KMS call)
- **Cost Efficiency**: Dramatically fewer KMS API calls (1 per service_id per TTL vs multiple per session)
- **Scalability**: Supports 100+ service contexts with <1MB memory overhead
- **Service Isolation**: Each service_id gets cryptographically isolated branch key

### 2. Service-ID-Only Context Binding

**Chosen for OAuth2 user-session tokens**: `{"service_id": "oauth2"}` as encryption context

**Alternative Considered**: Multi-field context with principal, session_id, timestamp
**Analysis in**: [ADR 20260116-120000: Encryption Context Optimization](20260116-120000-encryption-context-optimization.md)
**Rejected Because**: Benchmarking showed 40% performance overhead with minimal security benefit

**Benefits**:

- **Performance**: Minimal AAD processing overhead (~8ms vs 12ms for multi-field)
- **Service Isolation**: Tokens encrypted for GitHub cannot be decrypted with Google's context
- **Simplicity**: Single field reduces complexity while maintaining core security property
- **Storage Efficiency**: Smaller encryption context reduces metadata overhead

### 3. AWS Encryption SDK Selection

**Chosen**: AWS Encryption SDK with customer-managed KMS keys

**Alternative Considered**: Custom envelope encryption implementation
**Rejected Because**:

- Higher development and audit overhead
- Potential for cryptographic implementation bugs
- No integration with AWS CloudTrail for key usage auditing

**Alternative Considered**: Database-level encryption (TDE)
**Rejected Because**:

- No application-level context binding
- Cannot prevent cross-service token access within same database
- Limited key rotation and access control

**Benefits**:

- **Battle-Tested Cryptography**: Production-hardened implementation used by AWS services
- **Automatic Key Rotation**: Support for automatic CMK rotation without application changes
- **Audit Integration**: CloudTrail logging of all key usage for compliance
- **Multi-Region Support**: Cross-region key replication for disaster recovery

---

## Consequences

### Positive

1. **High Security Posture**
   - Envelope encryption provides defense-in-depth
   - Context binding prevents cross-service token misuse
   - Memory protection reduces local attack surface
   - AWS KMS integration provides enterprise-grade key management

2. **Performance Efficiency**
   - Sub-50ms encryption/decryption operations (cached branch keys: ~1-5ms)
   - Branch key caching per service_id reduces KMS calls to 1 per TTL window vs multiple per session
   - AESGCMSIV provides fast authenticated encryption

3. **Operational Benefits**
   - CloudTrail audit logs for all key operations
   - Automatic key rotation without application downtime
   - Multi-region key replication for high availability
   - Integration with AWS IAM for fine-grained access control

4. **Development Velocity**
   - Well-documented AWS SDK reduces implementation complexity
   - Standardized error handling patterns from AWS SDK
   - Community support and extensive documentation

### Negative

1. **AWS Dependency**
   - Tight coupling to AWS ecosystem for key management
   - Requires AWS credentials and network connectivity
   - Regional outages could impact token encryption/decryption

2. **Additional Complexity**
   - Envelope encryption adds implementation complexity vs simple encryption
   - Error handling for KMS failures, network timeouts
   - Key rotation procedures require operational processes

3. **Cost Implications**
   - AWS KMS charges per API call (mitigated by branch key caching: 1 call per service_id per TTL)
   - CloudTrail logging has storage costs
   - Additional AWS services for audit and monitoring

### Mitigations

1. **AWS Dependency**: Environment variable KEK fallback for development/testing
2. **Complexity**: Comprehensive error handling in EncryptionPort interface
3. **Cost**: Branch key caching per service_id minimizes KMS API calls (10x reduction vs direct KMS calls)

---

## Implementation Notes

### Configuration Structure

```yaml
encryption:
  key_encryption_key: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"
  # Alternative for development:
  # key_encryption_key: "${ENCRYPTION_KEK}"
```

### Error Handling

All encryption failures are categorized per [specs/20260114-120000-aws-encryption-vault/contracts/error-contract.md](../specs/20260114-120000-aws-encryption-vault/contracts/error-contract.md):

- `ErrorKindEncryptionFailed`: DEK generation or encryption failure
- `ErrorKindDecryptionFailed`: KMS unwrap or decryption failure
- `ErrorKindContextMismatch`: Service isolation violation
- `ErrorKindIntegrityViolation`: AESGCMSIV authentication failure
- `ErrorKindKEKUnavailable`: AWS KMS service unreachable

### Security Considerations

1. **Key Material Protection**: DEKs and tokens handled with secure buffer practices (memory protection details deferred to future feature spec)
2. **Context Validation**: All decryption operations validate service_id context match
3. **Fail-Closed**: Any encryption error results in operation failure, no fallback to plaintext
4. **Audit Logging**: All key operations logged to CloudTrail for compliance

---

## Layer Separation: Domain Service vs. Storage Adapter Encryption

### Pattern B: Domain Service Encryption

**Overview**: OAuth tokens are encrypted at the domain service layer and stored as opaque
encrypted bytes in the repository. This maintains hexagonal architecture purity.

**Implementation**:

- Domain Service: Orchestrates encryption/decryption as business logic
- Storage Repository: Pure persistence of encrypted bytes (no crypto dependencies)
- HTTP Adapter: Never sees encrypted data (transparent to callers)

**Data Flow**:

1. OAuth2 Provider → tokens (plaintext)
2. OAuth2SessionService.storeSession(tokens, serviceID)
3. Service encrypts with context: encryption.Encrypt(token, {"service_id": serviceID})
4. Service creates UserSession{EncryptedAccessToken: ciphertext}
5. Repository stores ciphertext as BYTEA (opaque)
6. On retrieval: Repository returns encrypted bytes
7. Service decrypts: DecryptAccessToken(session) → plaintext
8. HTTP Handler receives plaintext (transparent)

**Hexagonal Architecture Benefits**:

- Domain owns business logic (including data protection)
- Repository is infrastructure-agnostic (easy to swap backends)
- Clear separation: domain logic ≠ persistence logic
- Repository tests don't need to mock encryption

**Example**: UserSessionRepository + OAuth2SessionService

### Pattern A: Storage Adapter Encryption (NOT RECOMMENDED)

**Why Pattern A is suboptimal**:

- Repository depends on EncryptionPort (ISP violation)
- Encryption logic in adapter layer (infrastructure concern)
- Memory adapter inconsistency (no encryption in memory, but encryption in postgres)
- Repository interface exposes crypto concerns

**Migration Strategy**: See ADR 20260223-120100 (Encryption Layer Separation)

---

## Related Documents

- [ADR 20260116-120000: Encryption Context Optimization](20260116-120000-encryption-context-optimization.md) - Context binding performance analysis for OAuth2 token encryption, plus the 2026-06-01 amendment extending the same single-subject rule to non-session assets (for example signing-key private material using `kid`)
- [ADR 20260223-120100: Encryption Layer Separation](20260223-120100-encryption-layer-separation.md) - Architectural pattern for domain vs. adapter encryption
- [AWS Encryption Vault Specification](../specs/20260114-120000-aws-encryption-vault/spec.md) - Functional requirements
- [Error Contract](../specs/20260114-120000-aws-encryption-vault/contracts/error-contract.md) - Error handling patterns
- [AWS KMS Developer Guide](https://docs.aws.amazon.com/kms/latest/developerguide/) - Implementation reference
