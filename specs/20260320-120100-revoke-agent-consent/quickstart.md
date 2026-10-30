# Quickstart: Revoke Agent Consent (022)

**Branch**: `20260320-120100-revoke-agent-consent`

## What This Feature Adds

- `DELETE /api/consent/agent/{agent-id}/grants` — RESTful revocation endpoint
- "Revoke All Access" button on agent detail page (P1)
- Inline "Revoke" action on consent overview cards (P2)
- Shared `RevokeGrantDialog` confirmation modal
- Revocation enforced at token exchange (US3 — already satisfied by hard-delete + existing `VerifyAgentAccess`)

## Implementation Sequence

```
Phase 1: OpenAPI spec → stakeholder review
Phase 2: E2E tests (red phase) → backend → storage port → service → handler → wiring
Phase 3: Frontend → components → page updates → API client
Phase 4: E2E tests turn green → cleanup
```

## Backend Checklist (in order)

```
[ ] 1. Update /api/enduser/openapi.yaml — add DELETE path (from contracts/delete-grant.yaml)
[ ] 2. Add DeleteByPrincipalAndAgentID to UserGrantRepository port (internal/ports/storage.go)
[ ] 3. Add ErrGrantNotFound sentinel to consent service errors
[ ] 4. Add RevokeConsentForPrincipal method to ConsentService (+ add to ConsentService interface)
[ ] 5. Remove TODO comment from VerifyAgentAccess (replace with clarifying comment)
[ ] 6. Add logger to ConsentService (for audit logging) — check if already present
[ ] 7. Implement DeleteByPrincipalAndAgentID in memory adapter
[ ] 8. Implement DeleteByPrincipalAndAgentID in postgres adapter
[ ] 9. Write RevokeGrantHandler (internal/adapters/http/handlers/consent/revoke_grant_handler.go)
[ ] 10. Add RevokeGrant to EnduserHandlers (app/handlers.go)
[ ] 11. Wire RevokeGrantHandler in builder.go
[ ] 12. Register DELETE route in routing/enduser.go
[ ] 13. Add GrantRevoked term to ARCHITECTURE.md glossary
```

## Frontend Checklist (in order)

```
[ ] 1. Add deleteGrant() to useConsent.ts or new hook
[ ] 2. Create RevokeGrantDialog component
[ ] 3. Create RevokeGrantButton component
[ ] 4. Update AgentGrantDetailPage (P1): add RevokeGrantButton when grant exists
[ ] 5. Update DelegationCard: add optional onRevoke prop + "Revoke" button
[ ] 6. Update ConsentOverviewPage (P2): manage revoke state, pass handler to DelegationCard
```

## E2E Test Scenarios (Ginkgo)

File: `tests/e2e/revoke_grant_test.go`

```
Describe "Revoke Agent Grant" {
  Context "DELETE /api/consent/agent/{agent-id}/grants" {
    It "returns 204 when grant exists and is owned by principal"         [US1-S3, US2-S3]
    It "returns 404 when no grant exists for the principal+agent"        [Edge: not-found]
    It "returns 404 when grant exists for different principal"           [SR-001, Edge]
    It "returns 401 when no principal header is present"                 [SR-001]
    It "returns 400 when agent-id is not a valid UUID"                   [API validation]
    It "removes grant from list after successful revocation"             [US1-S4, US2-S3]
    It "token exchange is rejected after grant is revoked"               [US3-S1]
    It "immediate token exchange re-attempt is rejected"                 [US3-S2]
  }
}
```

## Key Files

| Layer | File | Change |
|---|---|---|
| API spec | `api/enduser/openapi.yaml` | Add DELETE /api/consent/agent/{agent-id}/grants |
| Port | `internal/ports/storage.go` | Add DeleteByPrincipalAndAgentID |
| Domain | `internal/domain/consent/service.go` | Add RevokeConsentForPrincipal, ErrGrantNotFound, remove TODO |
| Handler interface | `internal/adapters/http/handlers/consent/service_interface.go` | Add RevokeConsentForPrincipal |
| Handler | `internal/adapters/http/handlers/consent/revoke_grant_handler.go` | New file |
| Memory | `internal/adapters/storage/memory/user_grants.go` | Implement DeleteByPrincipalAndAgentID |
| Postgres | `internal/adapters/storage/postgres/user_grants.go` | Implement DeleteByPrincipalAndAgentID |
| DI | `internal/app/handlers.go` | Add RevokeGrant field |
| DI | `internal/app/builder.go` | Wire RevokeGrantHandler |
| Routing | `internal/adapters/http/routing/enduser.go` | Register DELETE route |
| Architecture | `ARCHITECTURE.md` | Add GrantRevoked to glossary |
| Frontend | `web/src/components/consent/RevokeGrantButton.tsx` | New file |
| Frontend | `web/src/components/consent/RevokeGrantDialog.tsx` | New file |
| Frontend | `web/src/pages/AgentGrantDetailPage.tsx` | Add P1 revoke section |
| Frontend | `web/src/components/consent/DelegationCard.tsx` | Add P2 revoke action |
| Frontend | `web/src/pages/ConsentOverviewPage.tsx` | Add P2 revoke state management |
| E2E | `tests/e2e/revoke_grant_test.go` | New file |

## Notes

- No database migration required.
- `RevokeConsent` (idempotent, used by POST path) is **unchanged**.
- `VerifyAgentAccess` already denies access for deleted grants — US3 works as-is after adding the DELETE endpoint.
- Audit log uses `slog.Info` with structured fields from the service method.
- All new storage errors must use `ports.ErrNotFound` (not raw `sql.ErrNoRows`).
