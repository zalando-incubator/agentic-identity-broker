# Additive Session State Contract

**Status**: Proposed. Stakeholder approval is required before implementation.

## Existing interfaces

`GET /api/third-party/sessions` returns `data[]` with service metadata and nullable `session`. `GET /api/third-party/{serviceId}/session` returns the session and dependent agents. Both use the existing acting-user authentication boundary.

The canonical schema is `SessionStatus` in `api/enduser/openapi.yaml`. The API also defines `UserSessionSummary`, a flat read model. Keep their existing field names and semantics unchanged.

## One new field

Add optional `required_scopes` to the existing session object. Use the `RequiredScopes` schema in [the OpenAPI proposal](enduser-additions.openapi.yaml). Apply the same field to the flat read model where used.

Example of the additive fragment:

```json
{
  "scope": ["repo"],
  "required_scopes": ["repo", "user:email"]
}
```

This session lacks a required scope. The field does not itself change authorization or initiate a provider request.

The domain computes a sorted, deduplicated union from active dependent grants owned by the acting principal. It respects selected permission sets and services and reuses existing requirement resolution. It excludes unselected optional scopes and other principals.

An empty array is authoritative only after successful computation. Absence on an older response means unknown coverage. The UI must not turn absence into a Connected claim.

## Why the existing response is insufficient

`service.scopes` is the available provider scope catalogue. `session.scope` is granted access. Neither provides required scope coverage for the acting user's dependent grants.

Per-agent detail reads expose requirements, but require request fan-out and selection reconciliation for the connections list. The proposed field provides that aggregate without new routes or a second state enum.

This proposal uses API-003's conditional allowance. Review its necessity and exact derivation before promoting it into the canonical OpenAPI file. No implementation begins on an unapproved field.

## State and errors

The [data model](../data-model.md) defines state precedence using existing expiry/refresh fields and the new requirement projection. No stored state column or new `state` response field is needed.

Keep existing 401, 404, and server-error behavior. If requirement resolution fails, return the existing server error instead of falsely returning an empty array. Do not expose another user's dependent agents in errors.

The change needs no database migration. Batch requirement reads in the existing domain/service path. Do not add database N+1 queries to eliminate browser N+1 requests.

## Compatibility acceptance

- Existing consumers continue to parse every current field unchanged.
- Existing authorize, callback, refresh, terminate, and affected-agent contracts remain unchanged.
- Two users with different grants receive different required-scope unions for the same provider.
- Unselected optional scopes and expired grants do not produce false Missing scopes states.
- A missing field remains unknown, not an empty requirement set.
- Session disconnection does not claim provider-side token revocation.

Before implementation, merge the approved field into `api/enduser/openapi.yaml`, update TypeScript models, and document it in `docs/api/`.
