# Implementation Plan: Encryption Vault for OAuth Tokens (Envelope Encryption)

**Branch**: `20260114-120000-aws-encryption-vault` | **Date**: 2026-01-16 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/20260114-120000-aws-encryption-vault/spec.md`

> **Implementation note (superseded configuration):**
> This document records the original single-field encryption proposal
> (`encryption.key` / `key_encryption_key` with `${ENCRYPTION_KEK}`).
> The shipped implementation uses a backend-explicit contract:
> `encryption.aws_kms` or `encryption.memory`.
> See `internal/ports/config.go` and `docs/configuration.md`
> for the live schema.

## Summary

Implement application-layer envelope encryption for OAuth tokens stored in the sessions table using AWS Encryption SDK with AESGCMSIV authenticated encryption. Two-layer encryption: Data Encryption Keys (DEK) encrypt tokens with service_id context binding, Key Encryption Keys (KEK) wrap DEKs with same context. Support AWS KMS (production) and environment variable KEK injection (development). Implement transparent encryption/decryption in storage adapters via EncryptionPort interface. Memory protection deferred to future memory hardening feature. Enforce fail-closed behavior with no plaintext fallback.

## Technical Context

**Language/Version**: Go 1.24.0+ (post-quantum cryptography support)
**Primary Dependencies**:

- `github.com/aws/aws-encryption-sdk/releases/go` - Official AWS Encryption SDK with AESGCMSIV authenticated encryption
- `github.com/aws/aws-cryptographic-material-providers/releases/go` - Keyring management for KMS
- `github.com/aws/aws-sdk-go-v2/service/kms` - AWS KMS client

**Storage**: PostgreSQL 12+ (encrypted_access_token BYTEA, encrypted_refresh_token BYTEA, encryption_context JSONB)
**Testing**: Ginkgo/Gomega BDD (existing infrastructure), testcontainers for PostgreSQL, memory protection tests
**Target Platform**: Linux server (Docker/Kubernetes deployment)
**Project Type**: Single Go backend (extending agentic-identity-broker)
**Performance Goals**: Encrypt/decrypt typical session <100ms (excluding AWS KMS latency)
**Constraints**:

- DEK per service_id with branch key caching optimization (reduces KMS calls per service)
- service_id-only context binding (optimized from 4-field)
- Backward compatibility on KEK rotation required
- Fail-closed security (no plaintext fallback)
**Scale/Scope**: Single feature extending session management; transparent encryption/decryption in storage layer

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Phase 1 Readiness Assessment:**

- [x] **Domain Model**: UserSession (existing) extended with encryption; EncryptionContext VO (service_id only); EncryptionPort interface
- [x] **Domain Concepts**: Add "envelope encryption", "DEK", "KEK", "EncryptionContext", "EncryptionPort", "AAD" to ARCHITECTURE.md Glossary
- [x] **Configuration Design**: Single `encryption.key` field with environment variable interpolation support (AWS KMS ARN or base64-encoded AES key)
- [x] **Config Examples**: `encryption.key: arn:aws:kms:eu-central-1:123456789012:key/abcd` (prod with KMS) or `${ENCRYPTION_KEK}` resolving to base64 key (dev)
- [x] **API Design**: EncryptionPort interface (Encrypt/Decrypt) specified; internal port/adapter (no HTTP API)
- [x] **Database Design**: No migrations required (encrypted_access_token BYTEA, encrypted_refresh_token BYTEA, encryption_context JSONB columns already exist)
- [ ] **E2E Acceptance Tests**: Write tests for all 24 acceptance scenarios BEFORE implementation (red phase)
- [ ] **E2E Test Mapping**: Each scenario maps 1:1 to It() block in tests/e2e/encryption_vault_test.go
- [ ] **E2E Red Phase**: Verify tests fail initially
- [x] **Security-First**: Fail-closed on any encryption failure; memory protection deferred to future feature; context verification mandatory both layers
- [ ] **Architecture Docs**: Update ARCHITECTURE.md with encryption domain, glossary, port/adapter diagram
- [ ] **ADRs**: Create ADR for envelope encryption design (two-layer DEK/KEK vs. alternatives)
- [x] **Library-First Security**: AWS Encryption SDK (AESGCMSIV); no custom crypto; memory protection deferred
- [x] **Hexagonal Architecture**: EncryptionPort with AWS SDK adapter; storage adapters call port; DEK/KEK encapsulated
- [x] **Persistence Patterns**: Uses existing UserSession aggregate and repository; transparent encryption in storage adapter

**Phase 1 Gate**: ✅ All design preconditions met. Ready for Phase 1 design artifacts.

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/
├── ports/
│   └── encryption.go              # EncryptionPort interface (Encrypt/Decrypt)
├── adapters/
│   ├── encryption/
│   │   ├── aws/
│   │   │   ├── adapter.go          # AWS Encryption SDK adapter (AESGCMSIV)
│   │   │   └── adapter_test.go     # AWS KMS integration tests
│   │   └── context.go              # EncryptionContext utilities
│   └── storage/
│       ├── memory/
│       │   ├── adapter.go          # Updated with encryption integration
│       │   └── adapter_test.go
│       └── postgres/
│           ├── adapter.go          # Updated with encryption integration
│           └── adapter_test.go
├── domain/
│   ├── storage/
│   │   ├── user_session.go         # UserSession (EncryptedAccessToken, EncryptedRefreshToken BYTEA, EncryptionContext JSONB)
│   │   ├── errors.go               # Domain errors (TokenEncryptionFailed, TokenDecryptionFailed, ContextVerificationFailed)
│   │   └── *_test.go
│   └── encryption/
│       └── errors.go               # EncryptionError types (ErrorKindEncryptionFailed, ErrorKindDecryptionFailed, ErrorKindContextMismatch)

tests/e2e/
└── encryption_vault_test.go        # E2E acceptance tests (24 scenarios across 7 user stories)

adrs/
└── NNNN-envelope-encryption-design.md  # ADR: DEK per service_id, branch key caching, AWS Encryption SDK
```

**Structure Decision**: Single Go backend extending agentic-identity-broker. Encryption layer implements hexagonal architecture: EncryptionPort interface abstraction with AWS SDK adapter implementation. Storage adapters (memory, postgres) transparently handle encryption/decryption via injected port.

## Testing Strategy

<!--
  Per Constitution Principle XIII (End-to-End Acceptance Testing & Spec Traceability):
  All features MUST have E2E acceptance tests mapped 1:1 to spec scenarios.

  This section documents HOW E2E tests will be structured and implemented for this feature.
-->

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/[feature]_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:

- **Top-level Describe**: Feature name (e.g., "OAuth2 Authorization Endpoint")
- **Nested Describe/Context**: Preconditions and scenarios (e.g., "when a valid request arrives" → "and no grant exists")
- **It blocks**: Individual acceptance scenarios (one It() per scenario from spec.md)

**E2E Acceptance Test Scenarios** (24 total, mapped 1:1 from spec):

| User Story | Scenario Count | Test Topics |
|------------|---|--|
| US1: Envelope Encryption | 4 | DEK with context binding, KEK wrapping with context, context verification failure, fail-closed behavior |
| US2: Secure KEK Storage | 4 | AWS KMS storage, KMS operations logged, access control, key rotation backward compatibility |
| US3: DEK Generation | 3 | Fresh DEK per service_id, service-specific branch key wrapping, DEK memory zeroization |
| US4: Env Var KEK Injection | 3 | Load KEK from ${ENCRYPTION_KEK}, use KEK for wrapping/unwrapping, persist across app restart |
| US5: Transparent Encryption | 3 | Repository Create() encrypts automatically, Get() decrypts automatically, no manual steps |
| US6: Cross-Service Prevention | 3 | Decrypt same service succeeds, different service_id fails both layers, ciphertext reuse fails |
| US7: Post-Quantum Ready | 4 | PQC available, encrypt/decrypt with PQC, disable PQC gracefully |

**Test Data Strategy**:

- Fixtures: test sessions with known principals, service_ids, token content for reproducibility
- KEK test data: AWS KMS key IDs for integration tests, environment variable KEK for unit tests
- Memory protection testing: helper functions to detect if memory was zeroed post-operation (via OS syscalls or heap inspection)

**Test Execution Flow** (TDD approach):

1. **Phase 2f (Design)**: Write all 24 E2E tests FIRST (red phase)
2. **Verify Red Phase**: Run `ginkgo -v ./tests/e2e/encryption_vault_test.go` - all tests FAIL (no implementation yet)
3. **Implementation**: Implement AWS Encryption SDK adapter, storage adapter integration incrementally
4. **Verify Green Phase**: Tests turn GREEN as each component satisfies acceptance criteria
5. **Minimal Changes**: Only fixture/test data adjustments during implementation (no test logic changes)

**Bootstrap Strategy**:

- Tests use production `app.Builder` via `tests/e2e/bootstrap/`
- Fresh app.Builder, fresh storage (memory/postgres), fresh encryption adapter per test (BeforeEach/AfterEach)
- KEK configuration injected via environment variables for tests (${ENCRYPTION_KEK} for unit; AWS KMS for integration)

**Helper Utilities**:

- Custom matchers: `HaveEncryptedToken()`, `HaveMatchingEncryptionContext()`, `FailWithContextMismatch()`
- Crypto helpers: Helpers to inspect encrypted payloads (ciphertext, wrapped DEK, AAD) for assertions
- Memory testing: Inspect memory after DEK operations to verify buffers were zeroed

### Unit & Integration Tests

**Unit Tests**:

- Location: `internal/adapters/encryption/aws/adapter_test.go`, `internal/adapters/encryption/context_test.go`
- Coverage: EncryptionContext validation, DEK generation randomness, AWS SDK wrapper behavior, error handling (encryption/decryption failures, context mismatches)
- Strategy: TDD - write tests FIRST, verify they FAIL, then implement

**Integration Tests**:

- Location: `internal/adapters/storage/memory/adapter_test.go`, `internal/adapters/storage/postgres/adapter_test.go`
- Coverage: Storage adapter encryption integration (session Create/Get with automatic encryption/decryption), memory protection behavior (DEK zeroization), AWS KMS integration (testcontainers for LocalStack or real AWS KMS in CI)
- Strategy: Real PostgreSQL via testcontainers, memory handling verification, AWS KMS mock/stub via LocalStack

**Test Coverage Goals**:

- Unit test coverage: Critical paths (happy path + error paths for encryption/decryption, context verification)
- Integration test coverage: Memory adapter (encrypt/decrypt/retrieval), PostgreSQL adapter (encrypt/decrypt/persistence, context JSONB handling)
- E2E test coverage: 100% of acceptance scenarios (24 scenarios, mandatory per Constitution Principle XIII)

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
