# Feature Specification: Encryption Vault for OAuth Tokens (Envelope Encryption)

**Feature Branch**: `20260114-120000-aws-encryption-vault`
**Created**: 2026-01-14
**Status**: Draft
**Input**: User description: "Standard AWS Encryption Vault (PQC-Enabled by Go 1.24+)"

> **Implementation note (superseded configuration):**
> This document records the original single-field encryption proposal
> (`encryption.key` / `key_encryption_key` with `${ENCRYPTION_KEK}`).
> The shipped implementation uses a backend-explicit contract:
> `encryption.aws_kms` or `encryption.memory`.
> See `internal/ports/config.go` and `docs/configuration.md`
> for the live schema.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Encrypt OAuth Tokens Using Envelope Encryption (Priority: P1)

A platform operator deploying the system to production needs to encrypt sensitive OAuth tokens (access tokens, refresh tokens) stored in the sessions table using envelope encryption. Tokens should be encrypted at the application layer using a Data Encryption Key (DEK) before storage, with the DEK itself encrypted by a Key Encryption Key (KEK), providing two layers of protection. The encryption context (service_id) must be bound at the DEK level, ensuring tokens are tied to their specific service.

**Why this priority**: This is the core use case—protecting sensitive tokens at rest in the database using envelope encryption. Envelope encryption with DEK-level context binding provides defense in depth with the DEK and KEK managed separately.

**Independent Test**: Can be fully tested by encrypting an OAuth token with envelope encryption (DEK with context + wrapped KEK), storing it in the sessions table, retrieving it, decrypting it with the correct KEK and context, and verifying the token is correctly recovered.

**Acceptance Scenarios**:

1. **Given** a session with OAuth tokens and associated service_id context is stored, **When** tokens are saved to the sessions table, **Then** each token is encrypted using a DEK bound to its service context, the DEK is wrapped using the service-specific branch key (derived from KEK) with the same service context, and both the token ciphertext and wrapped DEK are stored
2. **Given** encrypted tokens and wrapped DEKs in the sessions table, **When** a session is retrieved and tokens are requested, **Then** the service-specific branch key (cached from KEK for the service_id) is used to unwrap the DEK (verifying context), the DEK is used to decrypt the token (verifying context), and the plaintext token is returned
3. **Given** an attacker gains direct database access and reads encrypted tokens with wrapped DEKs, **When** they attempt to use the encrypted tokens with different context, **Then** the tokens are invalid because context verification fails at both DEK and KEK layers
4. **Given** a session is loaded from the database, **When** context verification fails at any layer, **Then** the application fails securely and does not fall back to plaintext tokens

---

### User Story 2 - Secure KEK Storage Using Industry Best Practices (Priority: P1)

A security team needs the Key Encryption Key (KEK) to be securely stored using industry-standard key management practices. For cloud deployments, KEKs should be managed by a centralized, enterprise-grade key management service. For development, KEKs may be stored locally with appropriate protections. Under no circumstances should KEKs exist in plaintext in application memory or on disk.

**Why this priority**: KEK security is fundamental to envelope encryption. Following industry best practices for KEK storage ensures the security of the entire token encryption system.

**Independent Test**: Can be fully tested by configuring KEK storage, encrypting/decrypting tokens, and verifying that plaintext KEKs never appear in logs, memory, or on disk, and that all KEK operations are auditeable.

**Acceptance Scenarios**:

1. **Given** a production deployment with centralized key management configured, **When** a token is encrypted, **Then** the KEK is obtained from the key management service, used to wrap the DEK with context verification, and never stored in application memory or on disk
2. **Given** KEK access is configured with centralized key management, **When** tokens are decrypted, **Then** the key management service is called to unwrap the DEK (verifying context), and all KEK operations are logged by the key management service
3. **Given** access controls on the centralized key management service, **When** an unauthorized user's session attempts to decrypt tokens, **Then** the key management service denies KEK access and the application fails securely
4. **Given** keys are rotated in the centralized key management service, **When** tokens encrypted with a previous KEK version need decryption, **Then** the key management service can still decrypt them with the previous key version

---

### User Story 3 - DEK Generation and Context Binding (Priority: P1)

A security architect needs Data Encryption Keys (DEK) to be generated securely and bound to the token's service context. Each session generates fresh DEKs with cryptographically secure randomness, with the service_id context cryptographically bound during DEK encryption and wrapping. DEKs should be unique per service_id and tightly coupled with their encrypted tokens and service context. All tokens within a session (access_token and refresh_token) use DEKs wrapped with the same service-specific branch key, optimized for service-level key management.

**Why this priority**: DEK security and context binding are crucial for envelope encryption. Generating fresh, random DEKs per service_id context with cryptographic binding ensures that tokens cannot be reused in different contexts, and compromise of one service's DEKs doesn't affect other services.

**Independent Test**: Can be fully tested by encrypting multiple sessions for different service_id contexts, verifying that each service_id has DEKs wrapped with its service-specific branch key, and confirming that tokens encrypted in one service context cannot be decrypted with different service_id context values.

**Acceptance Scenarios**:

1. **Given** multiple sessions are created for different service_id contexts, **When** each session is encrypted, **Then** a fresh DEK is generated per encryption operation with cryptographically secure randomness, the DEK is wrapped with the service-specific branch key, and the context is bound at both DEK and branch key layers
2. **Given** two sessions with different service_id contexts (oauth2 vs github), **When** both are stored, **Then** each service_id has DEKs wrapped with its own service-specific branch key, and attempting to decrypt one service's DEK with another's service_id context fails at both DEK and KEK verification layers
3. **Given** DEKs are generated for service_id context encryption, **When** all encryption operations complete, **Then** DEKs and branch keys used for encryption are securely erased from memory and never appear in logs

---

### User Story 4 - Environment Variable KEK Injection for Development (Priority: P1)

A developer in a local development or test environment needs to inject KEK material via environment variables without requiring cloud key management services. This enables full-stack development in containers, local machines, and CI/CD environments using simple environment variable configuration.

**Why this priority**: Supporting environment variable KEK injection is essential for development flexibility. Developers and test environments cannot always access AWS services, so environment variable support via existing `${env_var}` interpolation enables full-stack development without cloud dependencies.

**Independent Test**: Can be fully tested by setting ENCRYPTION_KEK environment variable, configuring `encryption.key: ${ENCRYPTION_KEK}`, encrypting/decrypting tokens with context, and verifying tokens persist correctly across application restarts.

**Acceptance Scenarios**:

1. **Given** the encryption configuration is set to `encryption.key: ${ENCRYPTION_KEK}` and ENCRYPTION_KEK environment variable is set before application startup, **When** the application starts, **Then** KEK material is loaded from the environment variable via interpolation
2. **Given** KEK material is provided via environment variable, **When** tokens are encrypted and decrypted, **Then** the environment variable KEK is used correctly for both DEK wrapping and unwrapping
3. **Given** a token is encrypted and decrypted with environment variable KEK and context, **When** the application restarts with the same ENCRYPTION_KEK value, **Then** the same KEK can still decrypt previously encrypted tokens with the same context

---

### User Story 5 - Transparent Token Encryption/Decryption (Priority: P1)

A developer using the OAuth2SessionService needs token envelope encryption to be transparent and automatic. When storing sessions with tokens and their context, DEK generation and encryption should happen automatically in the service layer without manual intervention. When retrieving sessions, DEK unwrapping and token decryption should happen automatically in the service layer with context verification. The UserSessionRepository operates on opaque encrypted bytes and is unaware of encryption mechanics.

**Why this priority**: Transparency is critical for developer experience. OAuth2SessionService should transparently handle envelope encryption/decryption so developers don't need to manage encryption manually. UserSessionRepository operates on opaque encrypted bytes and is unaware of encryption mechanics.

**Independent Test**: Can be fully tested by calling OAuth2SessionService methods with context data, verifying that tokens are encrypted with envelope encryption before storage, and that retrieved sessions have plaintext tokens after decryption.

**Acceptance Scenarios**:

1. **Given** a session with tokens and service_id context is passed to OAuth2SessionService.storeSession(), **When** the session is stored, **Then** OAuth2SessionService encrypts tokens using envelope encryption with service_id context binding, UserSessionRepository stores encrypted bytes as opaque data, and plaintext tokens are not stored
2. **Given** a session is retrieved with OAuth2SessionService.Get(), **When** the session is returned, **Then** UserSessionRepository returns encrypted bytes, OAuth2SessionService automatically decrypts tokens using envelope encryption with context verification, and plaintext tokens are available to the caller
3. **Given** OAuth2SessionService is used normally, **When** envelope encryption/decryption happens, **Then** no manual encryption steps are required by calling code (transparent to HTTP handlers)

---

### User Story 6 - Multi-Service Isolation via Encryption Context (Priority: P2)

A platform operator managing multiple OAuth2 services (GitHub, Google, custom OAuth providers) needs tokens from different services to be isolated at the cryptographic level: tokens encrypted for service A cannot be used for service B, preventing cross-service token reuse attacks even if an attacker compromises the database and gains direct read access to encrypted tokens and wrapped DEKs.

**Why this priority**: Multi-service isolation validates that core envelope encryption (US1) prevents high-level cross-service attacks. US1 enforces context verification at DEK/KEK layers; US6 validates this mechanism prevents practical attack scenarios. P2 because context verification is mandatory (US1), but testing it across multiple services is validation/integration work.

**Dependencies**: Depends on User Story 1 (Envelope Encryption) for the context verification mechanism.

**Independent Test**: Can be fully tested by creating sessions for multiple services, extracting encrypted ciphertexts from the database, and verifying ciphertext from service A cannot be decrypted in service B's context through any attack vector.

**Acceptance Scenarios**:

1. **Given** an application managing sessions for services "oauth2", "github", and "google", **When** sessions are created for each service, **Then** each service_id has DEKs wrapped with its own service-specific branch key, providing cryptographic isolation between services (no shared branch key or DEK material across services)
2. **Given** ciphertext from service "oauth2" stored in database, **When** decryption is attempted with service_id "github", **Then** decryption fails at both DEK verification layer (AAD mismatch) AND KEK unwrap layer (context mismatch), preventing cross-service reuse
3. **Given** an attacker with database access extracts encrypted tokens and wrapped DEKs from service "oauth2" and attempts to decrypt them as service "github" tokens, **When** they call the application's decryption endpoint with wrong service_id context, **Then** both cryptographic layers reject the ciphertext with ErrorKindContextMismatch, and the attack is prevented with clear audit log entry

---

### User Story 7 - Post-Quantum Cryptography Readiness (Priority: P3)

A forward-thinking security team wants the encryption vault to support post-quantum cryptographic algorithms for both DEK and KEK when available. This future-proofs token encryption against potential quantum computing threats.

**Why this priority**: Post-quantum cryptography is future-oriented. While quantum threats are not immediate, supporting PQC algorithms now ensures long-term protection of tokens. This can be deferred to follow-up work if needed.

**Independent Test**: Can be fully tested by enabling post-quantum cryptography, encrypting tokens with post-quantum algorithms, and verifying encryption/decryption work correctly.

**Acceptance Scenarios**:

1. **Given** post-quantum cryptography is available and enabled, **When** tokens are encrypted with envelope encryption, **Then** both DEK encryption and KEK wrapping use post-quantum algorithms
2. **Given** tokens encrypted with post-quantum envelope encryption, **When** decryption is performed, **Then** decryption succeeds without requiring special algorithm handling
3. **Given** systems without post-quantum cryptography support, **When** the system runs with PQC disabled, **Then** tokens are encrypted with classical algorithms without errors

---

### User Story 8 - Edge Cases & Error Handling (Priority: P1)

A reliability engineer needs the system to handle edge cases gracefully: large tokens that exceed encryption buffer limits, DEK generation failures, KEK unavailability during decryption, and context mismatch scenarios. Errors must fail fast and clearly rather than silently corrupting data or causing data loss.

**Why this priority**: Error handling is critical for production stability. Edge cases must fail fast with clear error messages to operators.

**Independent Test**: Can be fully tested by simulating each edge case and verifying application responses are safe, predictable, and fail-closed.

**Acceptance Scenarios**:

1. **Given** a token larger than 1MB (boundary test), **When** encryption is attempted, **Then** encryption fails with ErrorKindEncryptionFailed and error message includes max buffer size (no silent truncation)
2. **Given** DEK generation fails (e.g., insufficient randomness from OS), **When** session creation is attempted, **Then** application fails with ErrorKindEncryptionFailed and no session is created (fail-closed)
3. **Given** AWS KMS becomes unavailable during token decryption (network failure, service down), **When** GetSession is called, **Then** application fails with ErrorKindKEKUnavailable after AWS SDK timeout (default 10s) with clear error message for operator
4. **Given** encryption context (service_id) mismatches between encryption and decryption, **When** token decryption is attempted, **Then** failure occurs at both DEK verification AND KEK unwrap layers with ErrorKindContextMismatch error
5. **Given** ciphertext is tampered (bytes corrupted due to storage fault), **When** decryption is attempted, **Then** AESGCMSIV authentication tag verification fails with ErrorKindIntegrityViolation (no silent data corruption)
6. **Given** plaintext token is nil or empty, **When** encryption is attempted, **Then** encryption succeeds (empty tokens are valid; edge case handled correctly)

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST use hierarchical envelope encryption: a fresh Data Encryption Key (DEK) encrypts each token with context binding (service_id), the DEK is wrapped using a Branch Key derived per service_id from AWS KMS, and the KEK is managed by AWS KMS with DynamoDB branch key caching
- **FR-002**: System MUST generate a fresh DEK for each encryption operation per service_id context with cryptographically secure randomness (minimum 256 bits entropy), wrapped using the service-specific branch key
- **FR-003**: System MUST bind encryption context (service_id) to the DEK encryption as authenticated additional data
- **FR-004**: System MUST bind the same encryption context to the KEK wrapping operation (DEK wrapping with authenticated encryption)
- **FR-005**: System MUST wrap the DEK using the KEK with context verification and store the wrapped DEK alongside the encrypted token
- **FR-006**: System MUST decrypt tokens by first unwrapping the DEK using the KEK (verifying context), then using the DEK to decrypt the token (verifying context)
- **FR-007**: System MUST reject token decryption if context verification fails at either the DEK layer or the KEK layer
- **FR-008**: System MUST support configurable KEK storage via single `encryption.key` field: AWS KMS ARN for production or `${ENCRYPTION_KEK}` for environment variable injection
- **FR-009**: OAuth2SessionService MUST automatically decrypt OAuth tokens when retrieving sessions from UserSessionRepository, performing DEK unwrapping and token decryption transparently with context verification. UserSessionRepository operates on opaque encrypted bytes and is unaware of decryption mechanics
- **FR-010**: System MUST encode encrypted tokens in base64 for safe storage in database columns
- **FR-011**: System MUST validate KEK is accessible before the application starts (fail-fast on startup if KEK unavailable)
- **FR-012**: System MUST fail securely if KEK is missing, inaccessible, or unwrapping fails (no plaintext fallback)
- **FR-013**: System MUST never log plaintext OAuth tokens or encryption keys (DEK or KEK)
- **FR-014**: System MUST verify integrity of encrypted tokens during decryption; if integrity verification fails, decryption MUST fail
- **FR-015**: System MUST support token types: access tokens and refresh tokens (no id tokens)
- **FR-016**: System MUST support post-quantum cryptographic algorithms for both DEK encryption and KEK wrapping when available
- **FR-017**: System MUST protect against chosen-ciphertext attacks during envelope encryption and decryption

### Domain Model

**Entities** (things with unique identity):

- **EncryptedOAuthToken**: Represents an encrypted OAuth token with metadata: encrypted token data (ciphertext), wrapped DEK, encryption context, token type, algorithm identifier, timestamp
- **EncryptionConfig**: Represents envelope encryption configuration: KEK backend type, backend-specific parameters

**Aggregates** (consistency boundaries):

- **UserSession** (existing): Aggregate root extended to support envelope encryption. Storage adapters handle transparent encryption/decryption via EncryptionPort, with DEK generation, context binding, and KEK wrapping managed by the encryption adapter.

**Value Objects** (things without identity):

- **EncryptionContext**: Map containing exactly one field: service_id (OAuth service identifier). Immutable and bound to both DEK encryption and KEK wrapping as authenticated additional data (AAD). Optimizes key management operations while providing service-level separation of encrypted tokens.
- **WrappedEnvelope**: Represents the complete encrypted output: token ciphertext with DEK context verification, wrapped (encrypted) DEK with KEK context verification, and authentication metadata

**Domain Events** (state changes of business significance):

- **TokenEncrypted**: OAuth token encrypted with envelope encryption and context binding, ready for storage
- **TokenDecrypted**: OAuth token decrypted from envelope with context verified, ready for use
- **TokenEncryptionFailed**: Envelope encryption failed (DEK generation, context binding, or KEK wrapping); session storage should be rejected
- **TokenDecryptionFailed**: Envelope decryption failed (context verification or DEK unwrapping or token decryption); session retrieval should fail securely
- **ContextVerificationFailed**: Encryption context verification failed during encryption or decryption, indicating context mismatch or tampering

### Configuration Requirements

**Configuration Parameters**:

The system uses a single unified configuration parameter that supports both AWS KMS and environment variable KEK storage via the existing `${env_var}` interpolation:

- **`encryption.key`**: Single field accepting either an AWS KMS ARN or `${ENCRYPTION_KEK}` for environment variable interpolation
  - **AWS KMS (production)**: `arn:aws:kms:region:account-id:key/key-id` or `arn:aws:kms:region:account-id:alias/alias-name`
  - **Environment variable (development/containers)**: `${ENCRYPTION_KEK}` - resolves to the ENCRYPTION_KEK environment variable at runtime

The system automatically detects the KEK type by checking if the value is an AWS KMS ARN or an environment variable reference, and configures the appropriate AWS Encryption SDK keyring accordingly.

**Configuration Location**: Encryption configuration is integrated into the existing `.env` configuration approach from feature 20251214-120100-flexible-configuration, leveraging the platform's native `${env_var}` interpolation support.

### API Requirements

- **API-001**: Encryption port interface MUST define `Encrypt(ctx context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error)` performing envelope encryption with DEK (binding context) + KEK wrapping (binding context)
- **API-002**: Encryption port interface MUST define `Decrypt(ctx context.Context, ciphertext []byte, encryptionContext map[string]string) ([]byte, error)` performing envelope decryption with DEK unwrapping (verifying context) and token decryption (verifying context)
- **API-003**: EncryptionContext parameter MUST contain exactly one key: service_id (OAuth service identifier)
- **API-004**: Session repository MUST accept configured encryption backend and transparently perform envelope encryption/decryption on tokens with context binding from session data
- **API-005**: Configuration system MUST support encryption settings via `config.encryption.*` namespace
- **API-006**: All encryption/decryption operations MUST be context-aware and respect context deadlines

### Security Requirements

#### Cryptographic Requirements

- **SR-001**: Encryption MUST use envelope encryption with two key layers: DEK for token encryption and KEK for DEK wrapping
- **SR-002**: DEK MUST be unique per service_id context, generated with cryptographically secure randomness (minimum 256 bits entropy), wrapped using the service-specific branch key
- **SR-003**: Encryption MUST use authenticated encryption (AEAD ciphers) ensuring both confidentiality and integrity of tokens
- **SR-004**: Encryption MUST use AWS Encryption SDK with AESGCMSIV (Encrypt-then-MAC with counter mode) for authenticated encryption
- **SR-005**: DEK encryption MUST bind encryption context (service_id) as authenticated additional data (AAD)
- **SR-006**: DEK wrapping MUST bind the same encryption context (service_id) as authenticated additional data to the KEK wrapping operation
- **SR-007**: Context verification MUST fail the entire decryption if context (service_id) does not match at either DEK or KEK layer
- **SR-008**: Encryption MUST prevent chosen-ciphertext attacks through authenticated encryption (AESGCMSIV authentication tag)
- **SR-009**: Post-quantum cryptography support via Go 1.24+ and AWS Encryption SDK MUST be available for future deployment

#### Key Management Requirements

- **SR-010**: KEK MUST be securely stored following industry best practices: AWS KMS for production or environment variable injection for development/containers
- **SR-011**: KEK MUST support key rotation with backward compatibility—tokens encrypted with previous KEK versions MUST remain decryptable after rotation
- **SR-012**: KEK MUST NEVER exist in plaintext in application memory during normal operation (only in use for wrapping/unwrapping operations)
- **SR-013**: KEK MUST NEVER be stored in plaintext in logs, configuration files (beyond `${ENCRYPTION_KEK}` variable reference), or debug output
- **SR-014**: AWS KMS key operations (when configured) MUST support audit logging of all KEK operations (wrapping and unwrapping) via AWS CloudTrail
- **SR-015**: Environment variable KEK injection (when configured) MUST NOT expose the KEK in error logs, stack traces, or application logs; AWS Encryption SDK MUST handle KEK securely from environment
- **SR-016**: System MUST validate KEK accessibility at startup and fail fast if KEK cannot be accessed

#### Buffer and Memory Requirements

- **SR-017**: Plaintext OAuth tokens MUST be securely cleared from memory after use via secure buffer handling (deferred to future feature spec: memory protection)
- **SR-018**: DEK material MUST be cleared from memory after use (after DEK wrapping during encryption, after DEK unwrapping during decryption) (deferred to future feature spec: memory protection)
- **SR-019**: Plaintext DEK MUST NOT persist in memory across multiple operations (single-use temporary buffers)
- **SR-020**: KEK and DEK material handling MUST follow industry best practices for sensitive key material (deferred to future feature spec: memory protection)

#### Operational Security Requirements

- **SR-023**: All token encryption/decryption operations MUST be auditable using structured JSON logging with canonical fields: operation (encrypt|decrypt), service_id, success (true|false), token_type (access|refresh), timestamp (ISO 8601), error_kind (for failures). No plaintext tokens or keys in logs. Follow existing agentic-identity-broker logging patterns.
- **SR-024**: OAuth tokens MUST NOT be logged as plaintext in any logs or error messages
- **SR-025**: Encryption keys (DEK and KEK) MUST NOT be logged in any logs or error messages
- **SR-026**: System MUST NOT silently fall back to plaintext token storage if envelope encryption fails
- **SR-027**: System MUST NOT silently fall back to plaintext tokens if decryption fails
- **SR-028**: Decryption failures MUST result in application-level failures, not silent acceptance of corrupted data
- **SR-029**: Context verification failures MUST cause immediate decryption failure with no fallback or retry
- **SR-030**: All DEK and KEK operations MUST include context verification (service_id) as integral part of cryptographic operations
- **SR-031**: If AWS KMS becomes unavailable during token decryption, delegate to AWS SDK retry behavior. If AWS SDK retries exhaust, fail the operation with SDK error. No custom retry logic. Total p99 latency target: <300ms (50ms local ops + 250ms KMS buffer).
- **SR-032**: Key rotation is supported via AWS KMS native key versioning for production deployments. Environment variable KEK (development-only) does not support rotation—old tokens become unrecoverable on KEK change; this is acceptable for ephemeral dev environments.

### Performance Requirements

- **PR-001**: Local encrypt/decrypt operations (DEK generation, wrapping, context verification) SHOULD complete in under 50ms, excluding AWS KMS latency. AWS KMS latency is variable (typically 50-200ms per AWS SLA ~99.99%). Target total p99 latency: <300ms (50ms local + 250ms KMS buffer)
- **PR-002**: Decryption of a typical session (both access and refresh tokens) using envelope encryption with context verification and local operations SHOULD complete in under 50ms, excluding KMS unwrap latency
- **PR-003**: System MUST handle encryption and decryption operations without blocking the session repository under normal load
- **PR-004**: KMS latency variability is an operator/deployment concern; local performance targets assume KMS availability within AWS SLA

### Key Entities

- **EncryptedOAuthToken**: Encapsulates encrypted token envelope with wrapped DEK and service_id context
- **EncryptionConfig**: Manages KEK storage backend configuration
- **EncryptionPort**: Interface for envelope encryption/decryption, implemented by AWS SDK adapter

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Envelope encryption is implemented—100% of OAuth tokens are encrypted using DEK + wrapped KEK with context binding, 0% plaintext
- **SC-002**: DEK generation is cryptographically secure—each service_id context has DEKs generated with minimum 256 bits entropy, wrapped using service-specific branch keys
- **SC-003**: Context binding is enforced—encryption context (service_id) is bound to both DEK encryption and KEK wrapping
- **SC-004**: Context verification prevents cross-context reuse—tokens encrypted in one context cannot be decrypted in a different context
- **SC-005**: KEK security follows industry best practices—KEKs are securely stored (externalized or locally with 0600 permissions), never in plaintext
- **SC-006**: External key management works—tokens encrypted/decrypted via external KMS with zero plaintext KEKs on disk
- **SC-007**: Local development works—developers can encrypt/decrypt tokens with local KEK files in under 50ms per operation
- **SC-008**: Token integrity is verified—100% of tampered encrypted tokens are detected during decryption
- **SC-009**: No plaintext leakage—OAuth tokens, DEKs, and KEKs never appear in logs, errors, or debug output
- **SC-010**: Security fail-closed—if KEK is missing or context verification fails, application fails with clear error messages
- **SC-011**: Transparent encryption—session repository envelope encryption/decryption requires zero manual steps
- **SC-012**: Backward compatibility—tokens encrypted with previous algorithm versions can be decrypted after updates
- **SC-013**: Performance baseline—envelope encryption/decryption of typical tokens with context binding completes in under 100ms
- **SC-014**: Edge cases handled safely—large tokens, DEK generation failures, KEK unavailability, context mismatch, integrity violations all fail fast with clear errors

## Assumptions

- OAuth token size is typically 1-10 KB (standard OAuth2 token sizes with JWTs)
- Encryption context field (service_id) is available from session data at encryption and decryption time
- AWS KMS is available for production deployments, supports key versioning/rotation, and supports backward compatibility after rotation
- AWS Encryption SDK supports authenticated encryption with AESGCMSIV and includes post-quantum cryptography support via Go 1.24+
- Environment variable interpolation (`${env_var}`) is available in the configuration system for development/container KEK injection
- Session repository from feature 20251216-120000-session-management is available and can be extended with envelope encryption
- Operators manage KEKs (AWS KMS keys or environment variables) according to organizational security policies and best practices
- DEK context binding (service_id) is enforced as integral part of the encryption/decryption algorithm via AWS Encryption SDK, not as separate validation step
- Configuration system properly handles sensitive values and prevents KEK exposure in logs via existing security practices

## Clarifications

### Session 2026-01-15

- Q: Should OAuthTokenVault aggregate exist or should storage adapters call EncryptionPort directly? → A: Remove OAuthTokenVault, storage adapters call EncryptionPort directly
- Q: Pre-commit to AWS Encryption SDK algorithms or keep algorithm-agnostic? → A: Pre-commit to AWS Encryption SDK (AESGCMSIV)
- Q: Is backward compatibility across KEK key rotations required? → A: Yes, support backward compatibility with old KEK versions
- Q: EncryptionContext fields optimization? → A: Reduce to service_id only (removes redundancy with principal, session_id, purpose)
- Q: Configuration design for KEK storage (separate backends vs. unified field)? → A: Single `encryption.key` field with `${env_var}` interpolation (leverages existing config system, AWS KMS ARN for production or `${ENCRYPTION_KEK}` for development)
- Q: Update all context references from 4-field to service_id only for consistency? → A: Yes, update all user stories, functional requirements, and success criteria to reflect service_id-only context

### Session 2026-01-16

- Q: Runtime KMS Failure Handling → A: Delegate to AWS SDK retry behavior. If KMS becomes unavailable during token decryption, the AWS SDK will handle transient retries according to its configured policy. If retries exhaust, the operation fails with the SDK error. No custom retry logic needed.
- Q: DEK-Per-Service Trade-off → A: DEKs are generated per service_id context. Sessions contain only access_token and refresh_token (no id_token). All tokens for a given service use DEKs wrapped with the same service-specific branch key, optimized to reduce KMS calls per service.
- Q: Observability & Audit Logging Format → A: Use structured JSON logging following existing project logging patterns. Canonical fields: operation, service_id, success, token_type, timestamp, error_kind. No plaintext tokens or keys. Align with current agentic-identity-broker logging conventions.
- Q: Backward Compatibility Scope for KEK Rotation → A: AWS KMS handles rotation natively with key versioning. Environment variable KEK (development-only) does not require rotation support; production must use AWS KMS for key rotation and backward compatibility.
- Q: Performance Target Interpretation → A: Separate performance budgets. Local encrypt/decrypt ops (DEK generation, wrapping, context verification): <50ms. KMS latency: variable (AWS SLA ~99.99%). Total p99 latency goal: <300ms. KMS latency is operator/deployment concern.

## Notes

This specification uses **Envelope Encryption** with two key layers and context binding at the DEK level: Data Encryption Keys (DEK) that encrypt individual tokens with encryption context (service_id) as authenticated additional data, and Key Encryption Keys (KEK) that wrap (encrypt) the DEKs with the same context as authenticated additional data. Each token encryption generates a fresh, random DEK, binds service_id context during encryption, and the DEK is wrapped by the KEK with context verification. Context verification happens at both layers: DEK decryption verifies context, and DEK unwrapping verifies context. Mismatch at either layer causes decryption to fail immediately.

The KEK is securely stored following industry best practices: externalized in a centralized key management service (for production) with audit logging and access controls, or locally with restricted file permissions (for development). The system explicitly forbids plaintext fallback and requires fail-closed behavior if KEK access or context verification fails at any point. KEK key rotation is supported with backward compatibility—tokens encrypted with previous KEK versions remain decryptable after rotation.

Encryption context consists of a single field: service_id (OAuth service identifier). This field binds each token to its service, providing defense-in-depth separation across services and optimizing key management operations. Memory protection for sensitive buffers is deferred to a future feature specification.

Implementation uses AWS Encryption SDK with AESGCMSIV (Encrypt-then-MAC with counter mode) for authenticated encryption, providing battle-tested cryptographic implementation and future post-quantum cryptography support via Go 1.24+.

---

**Version**: 1.4 | **Status**: Draft | **Last Updated**: 2026-01-16 | **Clarifications Applied**: 12 clarification questions resolved (7 from session 2026-01-15, 5 from session 2026-01-16)
