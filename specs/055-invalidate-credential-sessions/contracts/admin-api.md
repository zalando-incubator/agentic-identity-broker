# Contract: Admin API 3.0.0 — credential changes invalidate user sessions

**Status**: Proposed (approval item API-ADMIN). Not approved. Implementation copies these deltas into `api/admin/openapi.yaml` only after approval.

**Scope**: Only documented behavior changes. Routes, methods, parameters, request and response schemas, status codes, and error bodies stay the same.

## `info.version`

```yaml
info:
  version: 3.0.0   # was 2.1.0
```

## `PUT /api/services/{service-id}`: added description section

Insert the following after the existing **Secret Rotation** block of the `updateService` description:

```yaml
        **Credential Changes and User Sessions**:
        - A credential change is a successful replacement that changes the service's effective
          operator-managed client credentials: the `client_id`, or the stored `client_secret`
          (including its removal or addition)
        - On a credential change, the same atomic update invalidates every user's established
          session for this service. Invalidated sessions report `is_expired: true` and
          `has_refresh_token: false` on the End-User API, and every user must log in to the
          service again. User grants are unchanged
        - Replacing a secret with the same value, unchanged credentials, display-name, scope, endpoint,
          or protected-resource edits, and discovery-backed refreshes that keep the client do not
          invalidate sessions
        - A `stored` to `filesystem` transition does not invalidate sessions by itself. The broker
          invalidates them when it first observes file credentials that differ from the previous
          stored pair
        - A `filesystem` to `stored` transition invalidates sessions unless the supplied pair equals
          the last pair observed from the files
        - Restoring former credential values is another change. It never revives sessions invalidated
          earlier
        - A rejected update (`400`, `404`, `409`, `412`, `428`, `500`, or `504`) leaves sessions unchanged
        - The broker does not revoke tokens already delivered to agents or held by the provider
        - For `filesystem` services, publishing a coherent higher-revision client ID/secret pair
          has the same effect. The broker recognizes it on the next authorization or token decision,
          before newly authorizing token release. Lower revisions and equal-revision/different-pair
          material fail closed without changing any session. Unknown legacy filesystem sessions
          are conservatively invalidated on first valid initialization; unchanged-value protection
          requires comparison evidence. Detection never changes this resource's `ETag` or
          `updated_at`
```

## `PUT` request example: annotation of the existing `rotate_secret` example

```yaml
              rotate_secret:
                summary: Rotate client secret (invalidates every user session for the service)
```

The example `value` is unchanged.

## `'200'` response: added sentence to the description

```yaml
        '200':
          description: |
            Service updated successfully. If the effective client credentials changed, every
            user session for the service was invalidated in the same update.
```

The required revision-file binding and cutover procedure are Proposed separately in [configuration.md](configuration.md). A response authorized by a guarded decision before commit can still be delivered afterward. Restoring former file values requires a higher publication revision, not replay of old material. Revision high-water marks survive source transitions and are never administrative inputs or outputs.

## Unchanged

`GET`, `POST`, and `DELETE /api/services...`, the protected-resource routes, the discovery-status route, `POST /api/sessions/sweep`, all schemas, and all error examples. Credential state, generations, and fingerprints are never administrative inputs or outputs.
