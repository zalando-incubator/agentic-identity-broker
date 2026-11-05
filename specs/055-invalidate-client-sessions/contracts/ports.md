# Contract: Internal Ports and Domain Operations

**Status**: Internal design within existing patterns (no sign-off needed by itself). It implements the Proposed decisions in [ADR 045](../adr-045-client-identity-revisions.md).

## New port: `ports.ThirdpartyOAuth2ClientIdentityRepository`

File: `internal/ports/thirdparty_provider.go`. This is a separate focused interface, because `ThirdpartyOAuth2ProviderRepository` already exceeds the ISP method budget. Both the memory and postgres adapters implement it, and `app/builder.go` wires it.

```go
// ClientIdentityState is a service's recorded effective client identity.
type ClientIdentityState struct {
    Identity *id.ClientID // nil until first recorded
    Revision int64
}

// ThirdpartyOAuth2ClientIdentityRepository records filesystem client-identity observations.
type ThirdpartyOAuth2ClientIdentityRepository interface {
    // RecordObservedClientIdentity atomically records observed as the service's identity.
    // If the stored identity is nil, it sets it without changing the revision.
    // If it differs and the stored revision still equals expectedRevision, it sets it and
    // increments the revision by one; changed reports whether an increment happened.
    // If the revision moved, it returns the current state unchanged so the caller re-evaluates.
    // It never changes version, updated_at, or any administrative field.
    RecordObservedClientIdentity(ctx context.Context, serviceID id.ServiceID, observed id.ClientID, expectedRevision int64) (state ClientIdentityState, changed bool, err error)
}
```

Errors are domain `storage.StorageError` values: `NotFound` for a deleted service and `Unavailable` or `Timeout` for infrastructure. The read timeout applies to the read and the write timeout to the update (Principle IX).

## Changed repository behavior (existing ports)

| Port method | Change |
|---|---|
| `ThirdpartyOAuth2ProviderRepository.Create` | Initializes `ClientIdentity` (stored: `client_id`; filesystem: nil) and revision 0. |
| `ThirdpartyOAuth2ProviderRepository.Update` | Applies the stored-source recording rule ([data-model.md](../data-model.md)) in the same conditional `UPDATE ... RETURNING`, and returns the resulting identity and revision on the entity. A failed precondition records nothing. |
| `ThirdpartyOAuth2ProviderRepository.Get`, `GetByIDs`, `List`, `FindByProtectedResource` | Populate `ClientIdentity` and `ClientIdentityRevision` with explicit columns. |
| `UserSessionRepository.CreateIfAbsent` and `UserSessionRefreshRepository.WithLockedSession` | Persist and return `UpstreamClientRevision`. As with `UpstreamClientID`, only a new callback generation may replace it. Refresh writes preserve it. |
| `UserSessionRepository` reads (`FindByPrincipalAndService`, `ListSummariesByPrincipal`, `ListActiveByPrincipal`, `ListActiveServiceIDsByPrincipal`) and `UserSessionExpiryRepository.ListExpiringSessions` | Return `UpstreamClientID` and `UpstreamClientRevision` so that the domain can evaluate identity. Preserve their existing ADR 046 rejection and refreshability predicates, including sweep exclusion of rejected sessions; add identity filtering in the domain rather than replacing those predicates with expiry-only checks. |

## New domain operations (`internal/domain/oauth2session`)

```go
// resolveClientIdentity returns the service's current effective identity and revision,
// reading the client-ID file for filesystem services and recording any observed change.
func (s *OAuth2SessionService) resolveClientIdentity(ctx context.Context, service *model.ThirdpartyOAuth2ProviderEntity) (ports.ClientIdentityState, error)

// sessionIdentityStatus applies the usability rule; nil means identity-usable.
// A non-nil result is a *model.CredentialSourceError with reason identity_mismatch,
// identity_superseded, or identity_missing.
func sessionIdentityStatus(session *storage.UserSession, service *model.ThirdpartyOAuth2ProviderEntity, current ports.ClientIdentityState) error
```

Exported operations for other domains are exposed through one narrow interface in `internal/ports`, which `oauth2session` implements and the builder injects:

```go
// SessionUsabilityEvaluator reports which of a principal's sessions require reauthentication.
type SessionUsabilityEvaluator interface {
    // RequiresReauthentication combines ADR 046's existing predicate with unusable identity.
    // A live identity read failure on a gating path returns an error; display paths use the
    // persisted state instead (see research R5).
    RequiresReauthentication(ctx context.Context, sessions []*storage.UserSession, mode UsabilityMode) (map[id.ServiceID]bool, error)
}

type UsabilityMode int
const (
    UsabilityGate    UsabilityMode = iota // agent authorization, grant submission: fail closed on errors
    UsabilityDisplay                      // consent detail, session list/detail: persisted fallback
)
```

| Consumer | Mode | Effect |
|---|---|---|
| `oauth2.AuthorizationService.anyDelegatedSessionUnusable` | Gate | Adds reason `session_reauthentication_required`, which leads to the existing consent redirect. |
| `consent.Service.SubmitGrant` mandatory-service validation | Gate | A superseded session does not satisfy a mandatory service. |
| `consent.Service.GetAgentConsentDetail` | Display | `IsConnected = false` for sessions that require reauthentication. |
| `oauth2session.ListUserSessions`, `GetSessionWithAgents`, `ForceRefreshSession` summary | Display | Set `UserSessionSummary.RequiresReauthentication`. |

Token acquisition, refresh, callback, sweep, and initiation call the private operations directly and always fail closed.

## Credential-source reason

`model.CredentialSourceReasonIdentitySuperseded = "identity_superseded"` joins the closed reason set. `tokenexchange.sessionDiagnostic` maps it, like `identity_mismatch` and `identity_missing`, to `credential_identity_changed`, which gives a recoverable `invalid_grant` with ADR 044's `error_uri`. The manual-refresh handler maps these three identity reasons to `409 refresh_unavailable`. It preserves ADR 046's separate `409 refresh_rejected` outcome for a recorded or newly observed terminal refresh rejection.
