# Contract Delta: Admin API (`api/admin/openapi.yaml`)

**Status**: Proposed. Approval item API-002 in [plan.md](../plan.md).

**Version**: `info.version` stays `2.1.0`. Only documented behavior changes. No path, parameter, schema, status code, or error code changes. ADR 044 is the precedent for unversioned changes to broker-chosen behavior.

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
- Internal identity evidence is never accepted as input or returned.
```

The existing **Secret Rotation** bullets gain one bullet: `- Existing user sessions remain usable; rotation alone never requires users to reconnect`.

## `POST /api/services` and `PUT /api/services/{service-id}` request validation

The existing rejection of internal fields (`credential_source_transitioned`, credential-file paths) extends to `client_identity`, `client_identity_revision`, and `upstream_client_revision`. Each returns the existing `400` validation error. Service responses never contain these fields. This follows the existing internal-field rejection pattern and adds no new error code.

## `POST /api/sessions/sweep` — `SessionSweepResult.failed` description

Append: `Includes candidates whose service client ID changed after the session was established; they are not refreshed and require the user to reconnect.`

## Operator guidance (documentation, not contract)

`docs/guides/manage-agents-and-services.md` and `docs/configuration.md` (`third_party_oauth2.credential_files`) gain a short "Changing a service client ID" procedure:

1. Register the new client and its callback URI with the provider.
2. Publish the new credential pair coherently, or send one full-replacement `PUT`.
3. Let filesystem publication reach every broker replica before announcing the change. A replica that still reads the old file can record a further change.
4. Expect each user to reconnect the service on next use.

Secret-only rotation needs none of these steps.
