# Contract Delta: Admin API (`api/admin/openapi.yaml`)

**Status**: Proposed. Approval item API-002 in [plan.md](../plan.md).

**Version (Proposed API-002)**: `info.version` `2.1.1` → `2.1.2`. Only operation descriptions change. No path, parameter, schema, status code, or error code changes. The PUT request and response contract is unchanged; the client-ID change's consumer-visible effects are versioned on the End-User API (API-001). ADR 046's description-only Admin change (`2.1.0` → `2.1.1`) is the precedent. The main alternative is a minor or major bump, which would announce a request or response change that does not exist.

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

## `POST /api/sessions/sweep` — `SessionSweepResult.failed` description

Append: `Includes candidates whose service client ID changed after the session was established; they are not refreshed and require the user to reconnect. A dry run classifies them as failed without recording a client-ID file change.`

## Operator guidance (documentation, not contract)

`docs/guides/manage-agents-and-services.md` and `docs/configuration.md` (`third_party_oauth2.credential_files`) gain a short "Changing a service client ID" procedure:

1. Register the new client and its callback URI with the provider.
2. Publish the new credential pair coherently, or send one full-replacement `PUT`.
3. Let filesystem publication reach every broker replica before announcing the change. A replica that still reads the old file can record a further change.
4. Expect each user to reconnect the service on next use.

Secret-only rotation needs none of these steps.
