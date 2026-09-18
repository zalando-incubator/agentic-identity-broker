# Tool Approval API

The Tool Approval API supports human-in-the-loop authorization for agent tool calls. When an
AI agent calls a tool that requires approval, the ExtProc gateway creates a pending record. The
user approves or denies the request in the consent interface.

## Authentication

| Endpoint | Auth Method |
|---|---|
| `POST /api/approvals` | Subject token + client assertion (dual-auth) |
| `GET /api/approvals` (sync) | Client assertion (CEL) |
| `GET /api/approvals/permanent` | `X-Remote-User` header |
| `GET /api/approvals/{id}` | `X-Remote-User` header |
| `POST /api/approvals/{id}/approve` | `X-Remote-User` header |
| `POST /api/approvals/{id}/deny` | `X-Remote-User` header |
| `POST /api/approvals/{id}/consume` | Subject token (Bearer) |
| `POST /api/approvals/{id}/revoke` | `X-Remote-User` header |

## Approval Flow

```
ExtProc ──POST /api/approvals──▶ Broker ──(pending)──▶ User sees in UI
                                                          │
                                              ┌───────────┴───────────┐
                                              ▼                       ▼
                                   POST .../approve            POST .../deny
                                     (once|session|permanent)    (optional: permanent)
                                              │                       │
                                              ▼                       ▼
ExtProc ◀──GET /api/approvals (long-poll)── Broker updates sync state
                                              │
                                              ▼ (if once + approved)
                                   POST .../consume
```

## Endpoints

### Create Pending Approval

```
POST /api/approvals
```

Creates a pending approval record. Duplicate requests for one tool call return the existing
record with `200`. A new record returns `201`.

**Rate limit:** The API limits each principal and agent pair to `max_requests_per_minute`.
It returns `429` when the limit is exceeded.

### Sync Approval State (Long-Poll)

```
GET /api/approvals
```

Returns active approvals for each principal and agent pair. The endpoint supports long-poll
with these headers:

- `If-None-Match`: The version ETag from the previous response.
- `X-Long-Poll-Timeout`: The seconds to wait for a change. The range is 1–120. The default
  is 30.

The endpoint returns `304 Not Modified` when no change occurs before timeout. The `ETag`
header contains the current version.

**Query Parameters**:
- `principal` (optional): Filter results for one principal.

Each approval summary includes server-derived `tool_pattern` and `params_pattern`. The exact tool pattern matches only the approval's tool name. The params pattern maps constrained argument names to globs. Missing argument names are unconstrained.

Each approved summary includes `approved_at` as an RFC 3339 timestamp. Pending and denied summaries omit this field.

The token-exchange response can include `principal` and `agent_id`. They contain the broker-verified approval identity. ExtProc denies approval-required requests when either field is absent.


### Get Approval Detail

```
GET /api/approvals/{id}
```

Returns the full detail for one approval record. The acting principal must match the approval
principal.

### Approve

```
POST /api/approvals/{id}/approve
```

Changes a pending approval to approved. The request requires a `persistence` field:

- `once`: One use. The caller must consume the approval after tool use.
- `session`: Valid for the agent session duration.
- `permanent`: Persists until a user revokes it. The consent interface manages it.

The request can include an optional `params_pattern` field. Omit it for exact coverage of the reviewed arguments, or send `{}` to allow all arguments for the reviewed tool. A supplied `tool_pattern` returns `400 invalid_request` with `tool_pattern is not allowed`. Malformed or non-covering parameter patterns return `422 invalid_pattern`.

### Scope Preview

```
POST /api/approvals/{id}/scope-preview
```

The request accepts only `params_pattern` and does not change approval state. The response includes the server-derived exact `tool_pattern`, resolved `params_pattern`, and preview. A supplied `tool_pattern` returns `400 invalid_request` with `tool_pattern is not allowed`.

### Deny

```
POST /api/approvals/{id}/deny
```

Changes a pending approval to denied. The request can include `persistence: "permanent"` to
create a permanent denial. The request body is optional.

### Consume

```
POST /api/approvals/{id}/consume
```

Marks a `once` approval as consumed. A repeat consume returns `200`. The API returns `422`
when the approval is not `once` or is not approved.

### List Permanent Approvals

```
GET /api/approvals/permanent
```

Returns permanent approvals and denials for the authenticated user.

### Revoke Permanent Approval

```
POST /api/approvals/{id}/revoke
```

Revokes a permanent approval or denial and changes it to denied. The API returns `422` when
the approval is not permanent.

## Persistence Scopes

| Scope | Behavior |
|---|---|
| `once` | Single use. Must be consumed via `POST .../consume` after the tool executes. |
| `session` | Valid for the agent session (scoped by `agent_session_id`). |
| `permanent` | Persists indefinitely. Visible in the consent management UI. Revocable. |

## Error Codes

All error responses use the `ApprovalError` schema:

```json
{
  "error": "error_code",
  "message": "Human-readable description"
}
```

| Code | HTTP Status | Description |
|---|---|---|
| `unauthorized` | 401 | Missing or invalid authentication |
| `bad_request` | 400 | Invalid request body or parameters |
| `invalid_request` | 400 | Supplied `tool_pattern` or another invalid request field |
| `forbidden` | 403 | Principal does not match approval owner |
| `not_found` | 404 | Approval not found |
| `gone` | 410 | Approval has expired |
| `rate_limit_exceeded` | 429 | Rate limit exceeded |
| `not_consumable` | 422 | Approval cannot be consumed |
| `not_revocable` | 422 | Approval cannot be revoked |
| `invalid_pattern` | 422 | Pattern is malformed or does not cover the reviewed tool call |
| `internal_error` | 500 | Unexpected server error |

## OpenAPI Specification

See [`/api/enduser/openapi.yaml`](../../api/enduser/openapi.yaml) for the full OpenAPI 3.0
specification.
