# Contract Delta: Admin API (`api/admin/openapi.yaml`)

**Status**: Proposed. Approval item API-002 in [plan.md](../plan.md).

**Version (Proposed API-002)**: `info.version` `2.1.1` → `3.0.0`, a major cutover on the existing routes. No path, parameter, schema, status code, or error code changes. The PUT request and response shape is unchanged, but sweep response classification changes observably: identity-invalid candidates previously counted as `refreshed` (including dry runs) now count as `failed`. This is not description-only. Constitution IV/X requires a major version or new endpoint for changed released behavior; no evidence establishes this baseline as unreleased. The main alternative is a new sweep operation retaining the old classification on the existing route; a major cutover avoids maintaining a second sweep contract. API-001 separately versions the End-User effects.

## `PUT /api/services/{service-id}` — add a section after **Secret Rotation**

```markdown
**Client Identity Changes**:
- An accepted update whose resulting stored `client_id` differs from the service's previous
  effective client ID is a client identity change. This includes a `static`↔`cimd` registration
  switch that replaces the client identifier. From the commit of the update, every existing user
  session for this service requires reauthentication. Token exchange returns `invalid_grant` with
  a service-specific `error_uri`, and refresh never uses the previous session's refresh token.
  Each user reconnects independently. Grants are unchanged.
- Restoring a previous `client_id` later does not restore sessions that a change invalidated.
- Rotating `client_secret` while keeping `client_id`, metadata-only changes, and source
  transitions that keep the effective client ID do not invalidate sessions.
- A filesystem service's client ID comes from its client-ID file. A different file value
  invalidates sessions the first time the broker reads it. Updates that select `filesystem` do
  not read files.
- Rejected, invalid, and version-conflicting (`409`/`412`) updates invalidate nothing.
- Internal identity evidence is never an administrative input or output. Request types do not
  bind it, and responses never contain it.
```

The existing **Secret Rotation** bullets gain one bullet: `- Existing user sessions remain usable; rotation alone never requires users to reconnect`.

## `POST /api/services` and `PUT /api/services/{service-id}` request handling

No request-validation change. The internal values `client_identity`, `client_identity_revision`, and `upstream_client_revision` are not fields of any request type, so they cannot be supplied, and service responses never contain them. Unlisted request properties keep today's handling. `ServiceCredentialMetadataBoundary` is unchanged. The main alternative extends that boundary's explicit rejection list, as ADR 041 did for `credential_source_transitioned`. That turns previously ignored input into a `400` contract change without a security benefit, because no request path can write the values.

## `POST /api/sessions/sweep` — changed result classification

For each otherwise eligible candidate, evaluate identity before refresh or dry-run success classification. A candidate with missing required identity, a different client ID, or a superseded revision increments `failed`, not `refreshed`; it submits no provider request. ADR 046's rejected-session exclusion and existing candidate selection remain unchanged. Other candidates retain their existing classifications.

A dry run performs the same comparison against the live identity and persisted revision but writes no observation or session state. A differing file value is sufficient to classify the candidate as failed without persisting an increment. An unreadable identity remains a failure, not proof of usability. Document both `failed` and `refreshed` descriptions and examples so consumers can adapt to changed totals. For one due, identity-invalid candidate and no others, both real and dry runs return `failed: 1`, `refreshed: 0`; the dry run leaves service revision and session tokens unchanged.

## Operator guidance (documentation, not contract)

`docs/guides/manage-agents-and-services.md` and `docs/configuration.md` (`third_party_oauth2.credential_files`) gain a short "Changing a service client ID" procedure:

Prerequisite: complete the continuous rolling upgrade in [ADR 047](../adr-047-client-identity-revisions.md#guarded-rollout-and-rollback-proposed-dec-005). Keep traffic open during old/new overlap, but freeze effective identity/source/canonical-ID/binding mutations, including identity-mutating automation and DCR registrations, until all old serving/writing processes and admitted work are drained and old-image restart is prevented. Same-ID secret rotation and metadata-only writes may continue if frozen fields cannot change. This is an operator procedure, not a new request rejection, parameter, or runtime bypass; new replicas enforce revisions immediately. Post-activation rollback uses revision-aware binaries only.

1. Register the new client and its callback URI with the provider.
2. Publish the new credential pair coherently, or send one full-replacement `PUT`.
3. Let filesystem publication reach every broker replica before announcing the change. A replica that still reads the old file can record a further change.
4. Expect each user to reconnect the service on next use.

Secret-only rotation needs none of these steps.
