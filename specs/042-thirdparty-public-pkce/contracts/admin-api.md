# Contract: Administrative API Delta

**Feature**: `042-thirdparty-public-pkce` | **Date**: 2026-09-10
**Canonical source**: [`api/admin/openapi.yaml`](../../../api/admin/openapi.yaml)
**Status**: **CONFIRMED 2026-09-10** (API-010; Constitution Principles IV and X)

This document is the exact delta to apply to `api/admin/openapi.yaml`. No other administrative
endpoint, schema, or error shape changes. There is no generated client or server code derived from
these files, so the OpenAPI document and the handwritten handler must be changed together.

---

## Summary of the delta

| Schema | Change |
|---|---|
| `Service` (read/list) | Add `token_endpoint_auth_method` as **required and nullable**; remove `client_secret` from `required` and omit it for public services |
| `ServiceCreateRequest` | Add `token_endpoint_auth_method` (nullable, `null` = confidential); remove `client_secret` from `required`; document the conditional requirement |
| `ServiceUpdateRequest` | Same as create, plus explicit full-replacement wording |
| `ErrorResponse` | **Unchanged** |
| Paths | Descriptions only |

---

## 1. `Service` — read and list representation

**Location**: `api/admin/openapi.yaml:2166-2290`

### 1.1 Adjust `required`

```yaml
    Service:
      type: object
      required:
        - id
        - display_name
        - client_id
        # - client_secret                  <- REMOVED (omitted for public services)
        - token_endpoint_auth_method       # <- ADDED (nullable, but always present)
        - oauth2_flavor
        - issuer_uri
        - discovery
        - endpoints
        - created_at
        - updated_at
```

`token_endpoint_auth_method` is `required` **and** `nullable: true`. In OpenAPI 3.0 that pair means
the property is always present and may carry `null` — exactly FR-024, which requires every read and
list entry to report `none` for a public service and `null` for a confidential one. A consumer can
therefore branch on the value without first testing for the key's existence (SC-007).

`client_secret` moves the other way: it is removed from `required` because it is **omitted** for
public services (FR-025). The two properties are deliberately asymmetric — the method answers a
question about every service, while the credential exists only for some.

### 1.2 Amend `client_secret` (currently lines 2207-2220)

Keep `type: string`, `readOnly: true`, and `example: REDACTED`. Replace the description's
`Always returns "REDACTED"` sentence with:

```yaml
            **SECURITY**: For a confidential service this always returns "REDACTED"; the stored
            secret is never transmitted. For a public service
            (`token_endpoint_auth_method: none`) this property is **omitted entirely**, because no
            credential exists — a redacted placeholder would falsely imply a stored secret.
```

### 1.3 Add `token_endpoint_auth_method`

```yaml
        token_endpoint_auth_method:
          type: string
          nullable: true
          enum: [none, null]
          readOnly: true
          description: |
            Client authentication method for this service's upstream token endpoint.

            - `none` — the service is a **public client**. It stores no credential and transmits
              none upstream; it authenticates the authorization-code exchange with the PKCE code
              verifier alone.
            - `null` — the service is a **confidential client**. It declares no method and
              authenticates upstream with its stored client secret.
          example: none
```

**Rationale**: FR-024 and SC-007 — an operator determines whether any service is public from a
single list read. `null` is reported rather than omitted so every entry answers the question.

---

## 2. `ServiceCreateRequest`

**Location**: `api/admin/openapi.yaml:2295-2390`

### 2.1 Remove `client_secret` from `required`

```yaml
    ServiceCreateRequest:
      type: object
      required:
        - display_name
        # - client_secret          <- REMOVED (now conditional; see description)
        - discovery
```

The requirement is conditional and cannot be expressed in a `required` list, so it is documented in
the property descriptions and enforced by the server (API-004).

### 2.2 Amend `client_secret` (currently lines 2339-2358)

Retain the existing flavor-specific content and `writeOnly: true`. Remove `minLength: 1`. Add
`nullable: true`. Append this description:

```yaml
            **Conditionally required**: If `token_endpoint_auth_method` is omitted or `null`,
            `client_secret` is REQUIRED and must contain a non-empty value.

            If `token_endpoint_auth_method` is `none`, this property can be omitted or set to
            `null` or `""`. A non-empty value returns 400. The broker never stores a secret for a
            public service.
```

### 2.3 Add `token_endpoint_auth_method`

```yaml
        token_endpoint_auth_method:
          type: string
          nullable: true
          enum: [none, null]
          description: |
            Declares that the upstream token endpoint expects **no client authentication**, making
            this service a public client (RFC 6749 §2.1, RFC 7591 `token_endpoint_auth_method`).

            Omit this property, or send `null`, for a confidential client; there is no default and
            no value is ever inferred or assigned on your behalf. `null` means exactly what omission
            means, so the `null` this property carries in a read response can be submitted straight
            back in an update without special handling.

            When `none`:
            - `client_secret` can be omitted, set to `null`, or set to `""`. It MUST NOT contain a non-empty value.
            - `client_id` is still REQUIRED; it is the only identity presented upstream.
            - `oauth2_flavor: google` is REJECTED, because the Google variant derives its client
              identifier from the credential document and a public service has none.
          example: none
```

**Note on `nullable`**: this property is `nullable: true` in the request schemas as well as the
read schema, and `null` is semantically identical to omission (API-003). That symmetry is what makes
a read-modify-write round trip work: a `GET` of a confidential service returns
`"token_endpoint_auth_method": null`, and that exact document can be `PUT` back with one field
changed. It remains absent from every `required` list, so omission is equally valid.

**Note on `enum: [none, null]`**: `api/admin/openapi.yaml` declares `openapi: 3.0.3`. Under OAS
3.0.x, `nullable: true` permits the null *type* but does **not** add `null` to an `enum` — a value
must satisfy both constraints, so `nullable: true` with `enum: [none]` rejects `null`. The listed
`null` is therefore required, and is the form the OpenAPI 3.0 documentation prescribes for a
nullable enum. This differs from OAS 3.1, where `nullable` does not exist and the equivalent is
`type: ["string", "null"]`; if this document is ever migrated to 3.1, both schemas need that
rewrite. The repository has no OpenAPI linter (no Spectral config, no `justfile` target), so this
is not enforced mechanically — the YAML is consumed by Docusaurus for rendering.

---

## 3. `ServiceUpdateRequest`

**Location**: `api/admin/openapi.yaml:2398-2487`

Apply the same three changes as §2, with this addition to the
`token_endpoint_auth_method` description:

```yaml
            **Full replacement**: this request replaces the service representation in full. The
            method is evaluated from the request alone — omitting it, or sending `null`, makes the
            updated service confidential regardless of its stored value, and the request is
            therefore rejected unless it also supplies a `client_secret`. This mirrors the existing
            rule that the client secret must be re-sent on every update. Updating a confidential
            service to `none` REMOVES the stored credential, leaving no dormant secret.
```

---

## 4. Path descriptions

| Location | Change |
|---|---|
| `POST /api/services` (`:526-547`) | Validation rules list: `client_secret: Required` → `Required unless token_endpoint_auth_method is none` |
| `GET /api/services` (`:454-470`) | Security note: `All client_secret fields are redacted as "REDACTED"` → `… for confidential services; the property is omitted for public services, which hold no credential` |
| `GET /api/services/{service-id}` (`:634-636`) | Same amendment as above |
| `PUT /api/services/{service-id}` (`:692-708`) | Add the full-replacement rule for the method and the credential-removal consequence |

---

## 5. Error responses

`ErrorResponse` (`api/admin/openapi.yaml:2782-2795`) is **unchanged**: `error` required, `message`
optional. There is no `field` property today and none is added.

API-007 requires validation failures to identify the offending field. The existing admin format
carries that in the free-text `message`, which is what every current validation failure does
(`internal/adapters/http/handlers/admin/services_handler.go:585-627`). Introducing a second error
shape for one feature would fragment the admin contract; the requirement is met by naming the field
in `message`.

All new validation failures return **400** with `error: "validation failed"` and one of:

| Condition | `message` |
|---|---|
| `token_endpoint_auth_method` is not `none` | `token_endpoint_auth_method: only "none" is accepted` |
| `none` with a non-empty `client_secret` | `client_secret must not have a non-empty value when token_endpoint_auth_method is "none"` |
| `none` with `oauth2_flavor: google` | `token_endpoint_auth_method "none" is not supported for the google flavor: the client identifier is derived from the credential document` |
| `none` with no `client_id` | `client_id is required` |
| method omitted, no `client_secret` | `client_secret is required` *(today's message, unchanged)* |

---

## 6. Backward compatibility

| Existing caller behaviour | Result |
|---|---|
| Create/update omitting the method and supplying a secret | Byte-identical to today (API-006) |
| `GET` a confidential service, change one field, `PUT` it back verbatim | Accepted — the returned `token_endpoint_auth_method: null` is valid input meaning confidential |
| Reading a confidential service | Identical: `client_secret: "REDACTED"`, plus the new `token_endpoint_auth_method: null` |
| Listing services | Identical for confidential entries; public entries omit `client_secret` |
| A client that requires `client_secret` in read responses | Still satisfied for every confidential service; only public services — which cannot exist before this change — omit it |

Relaxing `client_secret` from unconditionally required is additive: no request that is valid today
becomes invalid. Record the read-schema `required` relaxation in
[`docs/changelog.md`](../../../docs/changelog.md).

---

## 7. End-user API

`api/enduser/openapi.yaml` — the authorize (`:751-818`) and callback (`:819-921`) endpoints keep
their paths, parameters, and responses unchanged (API-008). Their descriptions gain one sentence
noting that the code exchange presents a client credential only for confidential services, while
PKCE applies to every service. No other end-user text mentions a third-party client secret.

---

## 8. Confirmation record (API-010)

Confirmed by the stakeholder on 2026-09-10:

- [x] `client_secret` may be omitted from `Service` read responses for public services, and is
      removed from the `required` list of all three schemas — **approved as proposed**
- [x] `token_endpoint_auth_method` accepts only `none` as a value; `null` and omission both mean
      confidential, so a read response can be submitted back unchanged — **approved, reversing the
      original API-003**, which had rejected explicit `null`
- [x] `token_endpoint_auth_method` is `required` and `nullable` on read, so every service reports
      either `none` or `null` in a single list call — consequence of FR-024 in the approved spec
- [x] Update keeps full-replacement semantics, so omitting the method — or sending `null` — on a
      public service is a rejected request rather than a preserved state — consequence of the
      2026-09-10 clarification
- [x] Validation failures identify the field in `message`, with no new error shape — preserves the
      existing admin contract; not a change

The first two are the substantive decisions; the remaining three follow from them and from
requirements already accepted in the specification.
