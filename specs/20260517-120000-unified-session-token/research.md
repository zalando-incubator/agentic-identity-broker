# Research: Unified Session Token State Transport

**Feature**: 20260517-120000-unified-session-token
**Date**: 2026-05-18

## Research Questions

### 1. Can `AuthorizationSessionClaims` be reused for non-CIMD agents?

**Decision**: Yes — use existing struct with `CIMDMetadata` set to nil.

**Rationale**: The struct already has `CIMDMetadata *cimd.ClientIDMetadataDocument` as an `omitempty` JSON field. Setting it to nil for local/proxy agents produces a valid, minimal token containing only `agent_id`, `principal`, `original_url`, `iat`, `exp`.

**Alternatives considered**:
- Create a separate `NonCIMDSessionClaims` struct → rejected (code duplication, two code paths in consent handlers)
- Add a `Mode` field to distinguish agent types → rejected (unnecessary; CIMDMetadata presence already signals mode)

### 2. Impact of removing redirect_uri fallback

**Decision**: Atomic removal is safe.

**Rationale**: Session tokens have 10-min TTL. At deployment time, any in-flight consent sessions using the old `redirect_uri` format will be at most 10 minutes old. Since we remove the fallback handler simultaneously, worst case is a user mid-consent gets an error and must restart (re-initiates authorize → gets new session_token). This is acceptable given the short window.

**Alternatives considered**:
- Dual-mode transition period (accept both for N days) → rejected (spec explicitly requires no fallback; dual-mode extends attack surface)
- Feature flag → rejected (adds configuration complexity for a one-time transition)

### 3. ADR 20260428-120000 compatibility

**Decision**: No superseding ADR needed.

**Rationale**: ADR 20260428-120000 chose redirect_uri fallback for non-CIMD flows as a pragmatic decision ("no external-origin metadata to spoof"). However, the ADR's core decision is "use stateless JWE tokens for session state." Extending this to all modes strengthens the security posture without contradicting the ADR's architecture. The ADR's non-CIMD exception was pragmatic, not principled.

**Alternatives considered**:
- Write ADR 20260825-120000 superseding ADR 20260428-120000 → rejected (no contradiction; ADR 20260428-120000 supports this direction)

### 4. Error response format for rejected tokens

**Decision**: Reuse existing error responses from CIMD flow.

**Rationale**: The CIMD consent handlers already return structured error responses for expired/tampered/mismatched tokens. These same error paths will be triggered for non-CIMD agents. No new error types needed.

- Expired token → 400 with "session expired" message
- Principal mismatch → 403 with "forbidden" message
- Malformed JWE → 400 with "invalid session" message
- Agent ID mismatch → 400 with "invalid session" message

### 5. JWE key rotation during active sessions

**Decision**: No code change needed — handled operationally.

**Rationale**: Session tokens have a 10-minute TTL. Key rotation is a deploy-time event. Standard rolling deployment drains existing requests within the TTL window. The old key is only needed for at most 10 minutes after rotation begins. If immediate rotation is required (compromise), accepting that in-flight consent sessions (≤10 min old) will fail with a 400 error is the correct security trade-off (fail closed).

**Alternatives considered**:
- Support multiple decryption keys simultaneously → rejected (adds complexity for a scenario that is operationally preventable)
- Extend TTL to allow graceful rotation → rejected (longer TTL increases replay window)
