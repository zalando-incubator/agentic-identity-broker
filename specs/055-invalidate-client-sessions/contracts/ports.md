# Contract: Internal Ports and Domain Operations

**Status**: Internal design within existing patterns (no sign-off needed by itself). It implements the Proposed decisions in [ADR 047](../adr-047-client-identity-revisions.md).

## New port: `ports.ThirdpartyOAuth2ClientIdentityRepository`

File: `internal/ports/storage.go` (interface and minimal DTOs), as required by Constitution IX. This is a separate focused interface, because `ThirdpartyOAuth2ProviderRepository` already exceeds the ISP method budget. Both the memory and postgres adapters implement it, expose it through the adapter factory/accessor, and `app/builder.go` wires it. No conditional re-export substitutes for the repository definition.

```go
// ClientIdentityState is a service's recorded effective client identity.
type ClientIdentityState struct {
    Identity *id.ClientID // nil until first recorded
    Revision int64
}

// ClientIdentityObservation binds a file read to its service configuration snapshot.
type ClientIdentityObservation struct {
    ServiceVersion int64
    CanonicalID    string
    Revision       int64
}

// ThirdpartyOAuth2ClientIdentityRepository records filesystem client-identity observations.
type ThirdpartyOAuth2ClientIdentityRepository interface {
    // RecordObservedClientIdentity requires source=filesystem and matching version,
    // canonical ID, and revision atomically, including for first/same-ID observations.
    // A matched null identity is initialized without an increment; a differing identity
    // increments once. applied distinguishes a matched no-op from a lost precondition.
    // A failed precondition writes nothing; changed is true only for an increment.
    // It never changes version, updated_at, or any administrative field.
    RecordObservedClientIdentity(ctx context.Context, serviceID id.ServiceID, observed id.ClientID, expected ClientIdentityObservation) (state ClientIdentityState, applied bool, changed bool, err error)
}
```

The existing service `version` is the administrative configuration generation: every accepted service update advances it, including source and canonical-ID changes and round trips. It is not the identity revision, and a version change alone invalidates only the pending observation, never a session. The source/version/canonical-ID/revision predicate is checked under one memory-store lock or in one conditional SQL update. On `applied == false`, the domain discards both the file value and expected snapshot, reloads the service/source/version/canonical ID/revision, reselects its binding from the immutable builder-injected configuration, and performs a new bounded file read if still filesystem; if now stored, it uses stored identity without an observation. Never replay a file value from a lost CAS, even if the revision is unchanged. Binding configuration changes require replica replacement under the guarded publication/cutover procedure; they are not live mutations outside the snapshot.

Errors are domain `storage.StorageError` values: `NotFound` for a deleted service and `Unavailable` or `Timeout` for infrastructure. The read timeout applies to the read and the write timeout to the update (Principle IX).

## Changed repository behavior (existing ports)

| Port method | Change |
|---|---|
| `ThirdpartyOAuth2ProviderRepository.Create` | Initializes `ClientIdentity` (stored: `client_id`; filesystem: nil) and revision 0. |
| `ThirdpartyOAuth2ProviderRepository.Update` | Applies the stored-source recording rule ([data-model.md](../data-model.md)) in the same conditional `UPDATE ... RETURNING`, populates the entity with this update's committed identity and revision, and returns `(identityChanged bool, err error)`. `identityChanged` is true only if this update itself committed a revision increment, derived from the predecessor captured under the update lock. A failed precondition records nothing and returns false with the error. |
| `ThirdpartyOAuth2ProviderRepository.Get`, `GetByIDs`, `List`, `FindByProtectedResource` | Populate `ClientIdentity` and `ClientIdentityRevision` with explicit columns. |
| `UserSessionRepository.CreateIfAbsent` and `UserSessionRefreshRepository.WithLockedSession` | Persist and return `UpstreamClientRevision`. As with `UpstreamClientID`, only a new callback generation may replace it. Refresh writes preserve it. |
| `UserSessionRepository` reads (`FindByPrincipalAndService`, `ListSummariesByPrincipal`, `ListActiveByPrincipal`) and `UserSessionExpiryRepository.ListExpiringSessions` | Return `UpstreamClientID` and `UpstreamClientRevision` so that the domain can evaluate identity. Preserve their existing ADR 046 rejection and refreshability predicates, including sweep exclusion of rejected sessions; add identity filtering in the domain rather than replacing those predicates with expiry-only checks. |

### Atomic Admin-update outcome

`Update` computes `identityChanged` and the resulting identity/revision together under the same memory-store write lock or PostgreSQL row lock/transaction that applies the version/discovery preconditions and conditional update. PostgreSQL must capture the predecessor under that row lock, not through an unlocked pre-read or an unlocked CTE snapshot. Return the outcome only after successful commit; any error returns false and supplies no successful audit outcome. The entity's identity fields are populated from this update's committed result, never a later `Get`. For a resulting filesystem source, preserve the locked row's latest identity/revision rather than overwriting it with the caller's stale entity, and return false. For a resulting stored source, return true exactly when the recording rule increments the locked predecessor's revision; initialization and same-ID updates return false. This return value is operation-local metadata, not a persisted field or an HTTP input/output.

The third-party domain emits `channel=admin_update` only when `Update` succeeds with `identityChanged == true`, using the identity revision returned by that update. It must not infer ownership by comparing its initial read with the returned entity or with a follow-up read. Filesystem observations continue to use their atomic `applied`/`changed` result.

Required interleaving in both stores and domain audit tests: pause a filesystem metadata PUT after its initial service read; commit a differing filesystem observation, which increments the identity revision but leaves the administrative version unchanged; then complete the PUT successfully at that same administrative version. The PUT returns the observation's preserved identity/revision with `identityChanged == false`. Exactly one change event is emitted, by the observation with `channel=filesystem_observation`; the metadata PUT emits no identity-change event. Use deterministic test barriers through existing ports/wrappers, not sleeps or test-only production APIs.

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

Consent and session usability displays may read the current client-ID file (Proposed ADR 047 partially supersedes ADR 041's no-file-read rule for these surfaces only). They never read a secret file. Administrative service create/update/get/list metadata operations retain the no-file-read rule; an explicit session sweep is not a service-metadata operation.

Session list storage reads remain metadata-only: extend their internal projection with `UpstreamClientID` and `UpstreamClientRevision`, excluded from JSON, alongside the existing reauthentication/refreshability metadata. Do not fetch encrypted token blobs or decrypt secrets merely to adapt a summary to `RequiresReauthentication`'s full-session input. Share the private identity-status rule with a metadata helper inside `oauth2session` for summary evaluation; the cross-domain evaluator interface remains unchanged. This is an internal projection adaptation, not a new public field or persisted concept.

| Consumer | Mode | Effect |
|---|---|---|
| `oauth2.AuthorizationService.anyDelegatedSessionUnusable` | Gate | Adds reason `session_reauthentication_required`, which leads to the existing consent redirect. |
| `consent.Service.GrantConsent` mandatory-service validation (`ValidateSubmission`) | Gate | A superseded session does not satisfy a mandatory service. |
| `consent.Service.GetAgentConsentDetail` | Display | Reads sessions through `ListActiveByPrincipal` instead of the ID-only `ListActiveServiceIDsByPrincipal`, so identity can be evaluated. `IsConnected = false` for sessions that require reauthentication. |
| `oauth2session.ListUserSessions`, `GetSessionWithAgents`, `ForceRefreshSession` summary | Display | Set `UserSessionSummary.RequiresReauthentication`. |

Token acquisition, refresh, callback, sweep, and initiation call the private operations directly and always fail closed.

## Credential-source reason

`model.CredentialSourceReasonIdentitySuperseded = "identity_superseded"` joins the closed reason set. `tokenexchange.sessionDiagnostic` maps it, like `identity_mismatch` and `identity_missing`, to `credential_identity_changed`, which gives a recoverable `invalid_grant` with ADR 044's `error_uri`. The manual-refresh handler maps these three identity reasons to `409 refresh_unavailable`. It preserves ADR 046's separate `409 refresh_rejected` outcome for a recorded or newly observed terminal refresh rejection.
