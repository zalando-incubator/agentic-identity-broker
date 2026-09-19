# Protected Resource Subresources API

> **Implemented API — Feature 035.** The admin server implements this protected-resource
API and full-service update behavior. [`api/admin/openapi.yaml`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/api/admin/openapi.yaml)
is the canonical API definition.

Protected resources are absolute URIs for RFC 8693 token exchange. The API removes trailing
path slashes before it stores or matches a URI. A normalized URI belongs to one service only.

All endpoints use the administrator authentication and authorization for `/api/services`.
Replace `SERVICE_ID` with a service UUID. The examples use documentation-only URIs and no
credentials.

## Read the resource set

```
GET /api/services/{SERVICE_ID}/protected-resources
```

A successful response contains the normalized resource set and service strong `ETag`. Retain
this ETag for a later full-service resource-set replacement.

```http
GET /api/services/8e5aa5aa-8ec3-4c45-842b-55050a359bb4/protected-resources HTTP/1.1
```

```http
HTTP/1.1 200 OK
ETag: "7"
Content-Type: application/json

{
  "protected_resources": [
    "https://api.example.com/v1",
    "https://payments.example.net"
  ]
}
```

A missing service returns `404`.

## Add one resource

### Collection POST

Collection `POST` uses a URI in the request body:

```http
POST /api/services/8e5aa5aa-8ec3-4c45-842b-55050a359bb4/protected-resources HTTP/1.1
Content-Type: application/json

{
  "resource_uri": "https://api.example.com/v2/"
}
```

The server validates and normalizes the URI to `https://api.example.com/v2`. A new URI
returns `201 Created` and a new `ETag`. Repeating a URI already owned by the service returns
`200 OK` without a duplicate. A URI owned by another service returns `409 Conflict`. A
malformed, relative, empty, or whitespace-only URI returns `400 Bad Request` before state
changes.

### Member-addressed PUT

`PUT` is the idempotent add operation. It has no request body. The member URI is the final
path segment:

```sh
curl --path-as-is -X PUT \
  -H "X-Remote-User: admin@example.com" \
  'http://localhost:14000/api/services/8e5aa5aa-8ec3-4c45-842b-55050a359bb4/protected-resources/https%3A%2F%2Fapi.example.com%2Fv2'
```

Member `PUT` has the same status and ownership rules as collection `POST`. A new URI returns
`201`. A repeat returns `200`. A URI owned by another service returns `409`. An invalid URI
returns `400`. Each successful response contains the resource set and `ETag`.

## Address a member URI safely

A member URI is one RFC 3986 percent-encoded path segment. It is not a hierarchy of API
paths. Encode every reserved URI character. Use `%3A` for `:`, `%2F` for `/`, `%3F` for `?`,
and `%23` for `#`. For example, this source URI:

```
https://api.example.com/v2?region=eu#status
```

Use this encoded path segment:

```
https%3A%2F%2Fapi.example.com%2Fv2%3Fregion%3Deu%23status
```

The server gets one escaped segment. It percent-decodes the segment once. It then validates,
normalizes, and matches the URI. Do not send a raw URI containing `/`. Do not decode or
double-encode the value first. If a source URI contains `%`, encode it as `%25`. API
gateways must preserve `%2F` instead of decoding or rejecting it.

## Remove one resource

```
DELETE /api/services/{SERVICE_ID}/protected-resources/{ENCODED_RESOURCE_URI}
```

```http
DELETE /api/services/8e5aa5aa-8ec3-4c45-842b-55050a359bb4/protected-resources/https%3A%2F%2Fapi.example.com%2Fv2 HTTP/1.1
```

A successful removal returns `200 OK`, the removed normalized URI, the resource set, and the
strong `ETag`. A missing service or unowned URI returns `404`. An invalid member URI returns
`400`. Removal stops future token exchange for the URI. It does not revoke issued tokens.

## Rename one resource

`PATCH` uses the encoded path member as the source URI. It uses `to` as the target URI. The
operation is atomic. Clients do not need a delete-and-add round trip.

```http
PATCH /api/services/8e5aa5aa-8ec3-4c45-842b-55050a359bb4/protected-resources/https%3A%2F%2Fapi.example.com%2Fv1 HTTP/1.1
Content-Type: application/json

{
  "to": "https://api.example.com/v2"
}
```

A successful rename returns `200 OK`, the affected normalized URI, the resource set, and an
`ETag`. Renaming a URI to itself is a no-op. A missing source URI or service returns `404`.
An invalid source or target returns `400`. A target owned by a service returns `409`.

## Full-service update and ETag retry

The full-service endpoint can replace the complete resource set. This operation differs from
the single-resource operations:

```
PUT /api/services/{SERVICE_ID}
```

- If `protected_resources` is omitted or `null`, the API retains the current resource set.
  This update does not require `If-Match`.
- If `protected_resources` is present, including an empty array, it replaces the resource
  set. The request must contain the current strong ETag in `If-Match`.
- An empty array clears the resource set. It is not an omission.

For a replacement, first get the current resource set or service. Get its ETag. Include all
other fields required by the full-service update contract. The following fragment shows only
resource-set behavior:

```http
PUT /api/services/8e5aa5aa-8ec3-4c45-842b-55050a359bb4 HTTP/1.1
If-Match: "7"
Content-Type: application/json

{
  "protected_resources": [
    "https://api.example.com/v2",
    "https://payments.example.net"
  ]
}
```

If another request changes the service version, this request returns `412 Precondition
Failed`. It does not modify the resource set. Get the current service or resource set. Merge
the desired resource set with its returned state. Send a replacement with the new ETag. Do
not resend the old set. It can discard the change that `If-Match` protected.

A replacement without `If-Match` returns `428 Precondition Required`. A URI claimed by
another service returns `409 Conflict`. A successful replacement returns `200 OK` and a new
`ETag`.

For an update that does not intend to change protected resources, omit the field:

```http
PUT /api/services/8e5aa5aa-8ec3-4c45-842b-55050a359bb4 HTTP/1.1
Content-Type: application/json

{
  "display_name": "Example Payments Service"
}
```

Include each field required by the canonical full-service schema. An omitted
`protected_resources` value leaves the resource set unchanged.

## Status summary

| Operation | Success | Important failures |
|---|---|---|
| `GET` collection | `200` + set and `ETag` | `404` service missing |
| `POST` collection / member `PUT` | `201` new, `200` idempotent | `400` invalid URI, `404` service missing, `409` globally claimed |
| `PATCH` member | `200` + result and `ETag` | `400` invalid URI, `404` source or service missing, `409` claimed target |
| `DELETE` member | `200` + result and `ETag` | `400` invalid URI, `404` source or service missing |
| Full-service replacement | `200` + `ETag` | `409` globally claimed URI, `412` stale ETag, `428` missing `If-Match` |
