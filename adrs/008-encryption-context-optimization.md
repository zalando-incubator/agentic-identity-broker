# ADR 008: EncryptionContext Optimization - Service ID Only

**Status**: Accepted
**Date**: 2026-01-16
**Feature**: 012-aws-encryption-vault

**Partial supersession**: Accepted [ADR 038](038-consent-bound-refresh-sessions.md) adds `refresh_session_id` to the approved subject list on 2026-10-01. The original namespaces and exactly-one-subject rule remain unchanged. This approval does not claim runtime support before feature 049 implementation.

## Context

The AWS Encryption Vault feature requires encrypting OAuth tokens with envelope encryption using AWS KMS. Context binding is a critical security feature that prevents token reuse across different services or purposes.

The current `UserSession` domain model includes an `EncryptionContext` value object with 4 fields:
- `Principal` (user identifier)
- `ServiceID` (OAuth service identifier)
- `SessionID` (session identifier)
- `Purpose` (always "oauth2_token")

This 4-field context is passed to AWS KMS and AWS Encryption SDK as Authenticated Additional Data (AAD) during envelope encryption operations.

### Problem

**Performance Concern**: AWS KMS EncryptionContext is used to associate metadata with each KMS Encrypt/Decrypt API call. Each field in the context adds to the API request size and computational overhead.

With 4 context fields, each session encryption/decryption triggers KMS operations with verbose context. This impacts:
- Network latency: larger KMS request payload
- KMS computational cost: more AAD to verify
- Total operation latency: target is <300ms p99 (50ms local + 250ms KMS buffer)

**Key Management Concern**: Using Principal, SessionID, and Purpose as context fields creates tight coupling between cryptographic operations and deployment specifics. If session structure changes, context binding logic must change.

**Specification Alignment**: The feature specification (from clarification Q2) explicitly states:
> "One DEK per session. Sessions contain only access_token and refresh_token (no id_token). Both tokens share one DEK per session, optimized to single KMS call per session wrap operation."

This optimization only makes sense with minimal context.

## Decision

**Reduce EncryptionContext to contain ONLY `service_id`.**

### Rationale

1. **Performance Optimization**: Single context field minimizes KMS request size and computation. AWS recommends keeping AAD minimal for performance-critical paths.

2. **Sufficient Security**: Service ID alone provides meaningful security guarantees:
   - Tokens encrypted for "oauth2-github" cannot be decrypted for "oauth2-slack"
   - Cross-service token reuse attacks are prevented
   - DEK is still unique per session (AWS SDK generates fresh DEK per Encrypt call)
   - Principal and SessionID don't add cryptographic isolation (sessions already isolated by SessionID in DB)

3. **Simplicity**: Single context field is easier to reason about, audit, and test.

4. **AWS Best Practice**: AWS recommends context binding at the service/application level, not at the deployment/session ID level. From AWS Encryption SDK documentation: "Use EncryptionContext to associate a high-level meaning to your data, not low-level identifiers."

5. **Specification Alignment**: Feature spec clarification Q2 confirms this design: "service_id-only context binding (optimized from 4-field)."

### Implementation Changes

**UserSession.EncryptionContext** (in `internal/domain/storage/user_session.go`):

```go
// BEFORE:
type EncryptionContext struct {
    Principal string `json:"principal"`
    ServiceID string `json:"service_id"`
    SessionID string `json:"session_id"`
    Purpose   string `json:"purpose"` // Always "oauth2_token"
}

// AFTER:
type EncryptionContext struct {
    ServiceID string `json:"service_id"` // OAuth service identifier
}
```

**Database Migration**: JSONB column `encryption_context` will update from:
```json
{"principal": "user@example.com", "service_id": "oauth2-github", "session_id": "abc123", "purpose": "oauth2_token"}
```

To:
```json
{"service_id": "oauth2-github"}
```

**Migration Strategy**:
- Non-breaking for existing encrypted sessions (field extraction still works)
- New sessions use optimized single-field context
- Optional cleanup: future migration can normalize old sessions to new format (requires re-encryption)

## Consequences

### Positive

1. **Performance**: Reduced KMS API payload and computation per operation
2. **Simplicity**: Easier to understand, audit, and test context binding logic
3. **Compliance with AWS Best Practices**: Aligns with AWS recommendations for EncryptionContext usage
4. **Specification Alignment**: Matches documented feature design

### Negative

1. **Backward Compatibility**: Existing sessions with 4-field context require migration (if full normalization is desired)
   - Mitigation: Fields are optional in JSON; extraction logic can handle both formats
2. **Lost Audit Detail**: Principal/SessionID not in cryptographic context (but still available in session metadata and logs)

### Neutral

1. **Testing**: Test fixtures may need updating to use optimized context format

## Alternatives Considered

### A1: Keep 4-Field Context
- **Pros**: Maximum audit detail at cryptographic layer
- **Cons**: Performance impact, violates AWS best practices, overly verbose for operational needs

### A2: Use Principal + ServiceID (3 Fields)
- **Pros**: Some audit detail, slightly better performance than 4 fields
- **Cons**: Still violates AWS best practices, SessionID is valuable for debugging

### A3: Service ID Only (Selected)
- **Pros**: Optimal performance, AWS best practice alignment, specification alignment
- **Cons**: Slightly less audit detail at crypto layer (but available elsewhere)

## Validation

- ✅ Specification approval: Clarification Q2 confirms service_id-only design
- ✅ AWS best practices: Aligns with AWS Encryption SDK documentation
- ✅ Security: Service-level isolation sufficient; session isolation handled by DB
- ✅ Performance: Minimal context = optimal KMS performance

## Amendment (2026-06-01): Single-Subject Context Extension for Non-Session Assets

The original decision in this ADR remains unchanged for `UserSession` token encryption: OAuth2 access and refresh tokens continue to use `{"service_id": "<service-id>"}` as their full `EncryptionContext`.

This amendment extends ADR 008 to cover encrypted assets that are not naturally scoped to a third-party OAuth2 service, starting with broker signing key private material.

### Extension

`EncryptionContext` remains constrained to **exactly one stable, non-secret subject identifier** per ciphertext namespace. The currently approved subject keys are:

- `{"service_id": "<service-id>"}` for OAuth2 user-session tokens and other service-scoped secrets
- `{"kid": "<key-id>"}` for broker signing key private material
- `{"refresh_session_id": "<session UUID>"}` for persisted refresh-session retry results, authorized by accepted ADR 038

These subject keys are mutually exclusive. A context MUST contain exactly one approved subject key. Decryption and branch-key routing MUST fail closed if zero or multiple subject keys are present.

A typed branch-key subject model is allowed to represent this invariant in code, provided it preserves the one-subject-only rule.

### Rationale

1. **Preserves ADR 008's performance goal**: each encryption/decryption operation still carries a single-key AAD map. This is an alternative subject, not an additive second field on the same ciphertext.
2. **Maintains semantic correctness**: broker signing keys are global broker assets, not third-party OAuth2 services. Reusing `service_id` for them would misrepresent the protected resource.
3. **Keeps existing service behavior stable**: service-backed secrets retain the original `service_id` subject and therefore keep their existing branch-key identity and isolation semantics.
4. **Keeps governance explicit**: accepted ADR 038 authorizes `refresh_session_id`. Every further subject key requires its own ADR amendment or superseding ADR before implementation.

### Impact

- The phrase "service_id-only" in this ADR applies to OAuth2 user-session token encryption, not to every encrypted asset in the broker.
- Existing OAuth2 token ciphertext and service branch-key IDs remain unchanged.
- Signing keys use their own single-field `kid` context and dedicated namespace while preserving the original minimal-context rule.

## Related ADRs

- [ADR 038: Refresh-Session Encryption and Transaction Ownership](038-consent-bound-refresh-sessions.md) — supersedes only the approved subject list.

## Follow-Up Tasks

1. **Update UserSession domain model** to use optimized EncryptionContext
2. **Update database schema** JSONB constraints (if any) to reflect new format
3. **Add migration guide** for operators managing existing encrypted sessions
4. **Update tests and fixtures** to use optimized context format
5. **Document in ARCHITECTURE.md** Glossary: why OAuth2 user sessions use `service_id`-only context and why non-session assets may use a different single-subject context

