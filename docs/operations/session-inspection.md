# Inspect third-party sessions

Use these admin endpoints to see which users connected which third-party services. Use them also to find sessions that a sweep cannot refresh because of their stored state.

| Endpoint | Purpose |
|---|---|
| `GET /api/sessions` | List sessions across all users. |
| `GET /api/sessions/{session-id}` | Read one session by its `id`. |

Both endpoints are read-only. They never return token material, encrypted or plain. They return the user identifier, the service ID, scopes, timestamps, and expiry state.

## Before you start

CAUTION: These endpoints return the identifiers of all users. Send requests through your authenticated admin proxy. Do not expose the admin server directly or send identity headers from a client. The proxy must authenticate the operator and authorize access to user data.

Each request writes an audit log entry with the operator identity. This includes requests that the broker rejects as invalid. The broker rejects a request that has no operator identity with `500 server_misconfiguration`. If you get this error, ask the proxy administrator to check operator identity delivery.

NOTE: The `principal` filter is part of the query string. Proxies and access logs can record query strings. Treat those logs as personal data.

## List sessions

```sh
ADMIN_PROXY_URL=https://broker-admin.internal.example.com
curl --cert "$OPERATOR_CERT_FILE" --key "$OPERATOR_KEY_FILE" \
  --fail-with-body --silent --show-error \
  "$ADMIN_PROXY_URL/api/sessions?principal=alice%40example.com&limit=50"
```

| Parameter | Rule |
|---|---|
| `principal` | Optional. An exact, case-sensitive match on the stored user identifier. It is an email address or a login name, depending on your deployment. The broker does not support prefix or substring search. It must not be empty, longer than 200 characters, or contain NUL characters or invalid UTF-8. |
| `limit` | Optional. `1` through `1000`. The default is `100`. |
| `cursor` | Optional. The `next_cursor` value from the previous page. Do not change it. Use it only with the same `principal` filter as the request that returned it. |

The broker orders results by `principal`, then `service_id`. The order of `principal` follows the database text collation. Do not depend on a specific order between users.

The response has these fields:

```json
{
  "items": [
    {
      "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "principal": "alice@example.com",
      "service_id": "550e8400-e29b-41d4-a716-446655440000",
      "token_type": "Bearer",
      "scope": ["repo", "read:user"],
      "initiated_at": "2026-10-01T09:00:00Z",
      "access_token_expires_at": "2026-10-01T10:00:00Z",
      "refresh_token_expires_at": "2026-12-30T09:00:00Z",
      "is_expired": false,
      "access_token_expired": true,
      "has_refresh_token": true
    }
  ],
  "truncated": false
}
```

- `truncated`: `true` if more sessions matched than `limit` returned.
- `next_cursor`: Present only when `truncated` is `true`. Send it as `cursor` to read the next page. See [Read all pages](#read-all-pages).
- `is_expired`: The refresh-token expiry has passed. The user must reconnect.
- `access_token_expired`: The access-token expiry has passed. A refresh can still recover the session if `is_expired` is `false` and `has_refresh_token` is `true`.
- A session with no recorded expiry is not expired.

An empty `items` array means that no session matched.

### Read all pages

To read every session, repeat the request with the `next_cursor` of the last page until `truncated` is `false`.

```sh
PAGE=$(curl --cert "$OPERATOR_CERT_FILE" --key "$OPERATOR_KEY_FILE" \
  --fail-with-body --silent --show-error \
  "$ADMIN_PROXY_URL/api/sessions?limit=1000")
CURSOR=$(printf '%s' "$PAGE" | jq -r '.next_cursor // empty')
while [ -n "$CURSOR" ]; do
  PAGE=$(curl --cert "$OPERATOR_CERT_FILE" --key "$OPERATOR_KEY_FILE" \
    --fail-with-body --silent --show-error \
    "$ADMIN_PROXY_URL/api/sessions?limit=1000&cursor=$CURSOR")
  CURSOR=$(printf '%s' "$PAGE" | jq -r '.next_cursor // empty')
done
```

Each page is one request. Keep the pages if you need the full list.

The cursor is a position in the sorted list. It is not a snapshot. If a session is created or deleted while you read, no session repeats and no session in the remaining range is skipped. A session that someone creates at a position you already passed does not appear. A cursor that is malformed, or that you use with a different `principal` filter, returns `400`.

## Read one session

```sh
curl --cert "$OPERATOR_CERT_FILE" --key "$OPERATOR_KEY_FILE" \
  --fail-with-body --silent --show-error \
  "$ADMIN_PROXY_URL/api/sessions/7c9e6679-7425-40de-944b-e07fc1f90ae7"
```

The response is one item in the same format as the list. If the ID is not a UUID, you get `400`. If no session has that ID, you get `404`.

## Find sessions that a sweep cannot refresh

A sweep returns counts only. These endpoints show the stored state of each session. They do not show which sessions failed a refresh at the provider, because the broker does not store refresh failures.

The `failed` count includes two kinds of session (see [Refresh third-party sessions](session-sweep.md)):

- Sessions that have no usable refresh token. This endpoint can find them.
- Sessions where the provider rejected the refresh, for example with `invalid_grant`. The stored state of these sessions looks healthy, so this endpoint cannot find them. Use the refresh failure logs, which identify each session owner.

To find sessions that stored state shows a refresh cannot recover:

1. [Read all pages](#read-all-pages) of the list, or list with the `principal` filter for one user.
2. Find sessions where `is_expired` is `true`, or where `has_refresh_token` is `false` and `access_token_expired` is `true`. A refresh cannot recover these sessions. The user must reconnect.
3. Use your approved support process to contact the user.

Do not delete another user's session to clear a failed count.

## Errors

| Status | Cause |
|---|---|
| `400` | `principal` is empty, too long, or not valid text, `limit` is not an integer from `1` to `1000`, `cursor` is empty, malformed, or for a different `principal` filter, or the session ID is not a UUID. |
| `404` | No session has the requested ID. |
| `500` | The session store is unavailable, or the request has no operator identity. The log entry contains the cause. |
