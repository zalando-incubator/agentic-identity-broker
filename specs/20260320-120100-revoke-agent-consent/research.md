# Research: Revoke Agent Consent (022)

**Date**: 2026-03-20 | **Branch**: `20260320-120100-revoke-agent-consent`

## 1. Existing Revocation Infrastructure

### Decision
`RevokeConsent(ctx, principal, agentID)` already exists in `domain/consent/service.go` (idempotent — returns nil if grant not found). Hard deletion is the chosen mechanism (no soft-delete). `VerifyAgentAccess` already enforces hard-deletion as "access denied": when a grant is deleted, `FindByPrincipalAndAgent` returns `ports.ErrNotFound`, which maps to `ErrAgentAccessDenied`. **US3 (FR-008) is already satisfied by the existing implementation** — no new domain logic needed for the authorization enforcement; only the DELETE endpoint and the removal of the TODO comment are required.

### Rationale
Using hard deletion means no schema migration (DB-001). The `UserGrantRepository` already has `Delete(ctx, id.GrantID)` and `FindByPrincipalAndAgent(ctx, principal, agentID)`. The service layer can compose these into a non-idempotent `RevokeConsentForPrincipal` method for the new DELETE endpoint, while retaining the idempotent `RevokeConsent` for the existing POST path (backward compatibility).

### Alternatives Considered
- Soft-delete with a `revoked_at` field: Rejected — adds schema migration, the TODO was speculative, and is explicitly excluded by spec assumption.
- Reuse existing `POST /grants` empty-array path: Not a clean REST API per API-003. A dedicated DELETE endpoint is required.

---

## 2. Storage Port: `DeleteByPrincipalAndAgentID` vs. Service-Layer Composition

### Decision
Add `DeleteByPrincipalAndAgentID(ctx, principal, agentID) error` to the `UserGrantRepository` port in `internal/ports/storage.go`. This method performs a single atomic lookup-and-delete, returning `ports.ErrNotFound` if no matching grant exists. The new service method `RevokeConsentForPrincipal` delegates to this single method.

### Rationale
Spec DB-002 explicitly requires this method. A single storage operation is safer than two-phase (find + delete) because it avoids a race window: if two concurrent revocations run, the second find returns nil even though the first delete succeeded. An atomic `DELETE WHERE principal=? AND agent_id=?` with rows-affected check eliminates the race.

### Alternatives Considered
- Two-phase: `FindByPrincipalAndAgent` + `Delete` in service layer: Rejected — race window exists (concurrent revocations), and spec explicitly requires the dedicated port method.

---

## 3. New Service Method vs. Modifying Existing `RevokeConsent`

### Decision
Add a new method `RevokeConsentForPrincipal(ctx, principal, agentID) error` that returns a new sentinel error `ErrGrantNotFound` if no grant exists. Keep `RevokeConsent` unchanged (idempotent, used by the POST empty-array path).

### Rationale
Changing `RevokeConsent` from idempotent to error-returning would break the existing `POST /api/consent/agent/{agent-id}/grants` handler logic (grants_handler.go line: "empty delegated_oauth2_tokens = call RevokeConsent, return 204"). The DELETE endpoint needs to distinguish "not found" (404) from success (204). A separate method maintains the contract per Principle X.

### Alternatives Considered
- Add a `bool` "require existence" parameter to `RevokeConsent`: Rejected — poor API design, conflates two behaviors in one function.
- Reuse `RevokeConsent` and add a pre-flight `GetAgentGrants` check: Rejected — race condition between check and delete.

---

## 4. API Design: `DELETE /api/consent/agent/{agent-id}/grants`

### Decision
New endpoint `DELETE /api/consent/agent/{agent-id}/grants`:
- `204 No Content` on success
- `404 Not Found` if no active grant exists for the authenticated user + agent
- `401 Unauthorized` if no authenticated principal
- `400 Bad Request` if `{agent-id}` is not a valid UUID format
- `403 Forbidden` is enforced implicitly by scoping to the authenticated principal (cross-user delete returns 404 by design — another user's grant simply doesn't exist from the requester's perspective; 403 adds no additional security and leaks existence information)

Document in `/api/enduser/openapi.yaml` before implementation.

### Rationale
REST semantics for resource deletion. The `POST /grants` with empty array workaround may remain for backward compatibility. The DELETE endpoint provides a clean, semantic alternative per API-003 and API-004.

### Alternatives Considered
- `POST /api/consent/agent/{agent-id}/grants/revoke` action-based endpoint: Rejected — not RESTful per Zalando guidelines.
- 403 for cross-user: Rejected — existence leakage is worse than 404; the current principal-scoped query naturally returns not-found for another user's grants.

---

## 5. Frontend: Dialog Sharing Strategy

### Decision
A single `RevokeGrantDialog` component (React, `web/src/components/consent/`) wraps the design system `Modal` primitive and is used for both P1 (agent detail page) and P2 (consent overview page). Props: `agentName: string`, `agentId: string`, `isOpen: boolean`, `onConfirm: () => Promise<void>`, `onClose: () => void`. The `RevokeGrantButton` handles the open trigger and delegates to the dialog.

### Rationale
Spec requirement: "Full modal dialog — same `RevokeGrantDialog` component used for both P1 and P2." Deduplication also ensures consistent dialog copy, which mentions the OAuth2 sessions caveat. The `Modal` primitive from `web/src/design-system/components/overlays/` handles focus trap, `Escape` key dismiss, WCAG 2.1 AA requirements.

### Alternatives Considered
- Inline confirmation cards in the overview: Rejected by spec clarification (full modal for both P1 and P2).
- Separate dialogs per page: Rejected — code duplication, inconsistent UX.

---

## 6. E2E Test Strategy

### Decision
Single file `tests/e2e/revoke_grant_test.go` with Ginkgo/Gomega. Tests focus on HTTP API behavior; UI-specific scenarios (button visibility, dialog copy, focus trapping, keyboard navigation) are covered by React component tests (`.test.tsx`) rather than Ginkgo E2E. Scenario mapping:

| Spec Reference | E2E Test | Coverage |
|---|---|---|
| US1 Scenario 1 (button visible) | Frontend test | UI assertion |
| US1 Scenario 2 (dialog content) | Frontend test | UI assertion |
| US1 Scenario 3 (grant deleted, redirect) | E2E: `DELETE /grants` → 204, grant list empty | API behavior |
| US1 Scenario 4 (no active grant on overview) | E2E: GET grants after delete | API behavior |
| US1 Scenario 5 (cancel = no change) | E2E: grant still exists via GET | API behavior (no cancel needed, just verify grant exists) |
| US2 Scenario 1-3 (revoke from overview) | Same E2E tests as US1 3-4 | Same API |
| US2 Scenario 4 (cancel) | Same as US1 Scenario 5 | Same API |
| US3 Scenario 1 (token exchange denied) | E2E: revoke then POST /oauth2/token → 401/403 | API behavior |
| US3 Scenario 2 (immediate re-attempt denied) | E2E: immediate POST /oauth2/token after revoke | API behavior |
| Edge: non-existent grant | E2E: DELETE non-existent → 404 | API error |
| Edge: wrong principal | E2E: DELETE another user's grant → 404 | Security |
| Edge: no grant = button hidden | Frontend test | UI assertion |

### Rationale
Ginkgo E2E tests validate HTTP contracts and complete system integration. Frontend unit/integration tests (`vitest`) are more appropriate for UI assertions (button visibility, dialog text) because they can easily manipulate props and assert on rendered output without a full server.

---

## 7. `VerifyAgentAccess` TODO Cleanup

### Decision
Remove the TODO comment block from `domain/consent/service.go` lines 244-249 that suggests adding a `Revoked` field to `UserGrant`. Hard deletion is the definitive revocation mechanism (per spec Assumption 1). Replace with a clarifying comment that hard deletion enforces revocation (missing grant = access denied).

### Rationale
The TODO was written speculatively for a future soft-delete scenario. That scenario is explicitly excluded by spec Assumption 1: "Hard deletion of the `UserGrant` row is the revocation mechanism — no soft-delete or revoked status flag is introduced." Leaving it causes confusion and implies future work that won't happen.

---

## 8. Audit Logging Location

### Decision
Structured audit log entry (`slog.Info`) with fields `principal`, `agent_id`, `grant_id`, `action=grant_revoked`, `timestamp` is emitted from the `RevokeConsentForPrincipal` service method (domain service), not the HTTP handler.

### Rationale
Security-critical operations are logged at the domain layer (Principle I). This ensures the audit event fires regardless of which HTTP path triggers revocation (new DELETE endpoint, or future programmatic revocation). The existing logger infrastructure (`*slog.Logger`) is passed to the service at construction.

### Alternatives Considered
- Log from HTTP handler: Rejected — audit logging should be in domain service for completeness and to avoid logging-bypass via alternative code paths.
