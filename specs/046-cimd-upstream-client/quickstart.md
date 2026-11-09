# Quickstart: Validate a CIMD Upstream Client

## Purpose

Use this guide after implementation. It proves the broker can authenticate to a third-party OAuth2 service with `private_key_jwt`.

See [data-model.md](./data-model.md) for the stored state. See [admin-api.md](./contracts/admin-api.md) and [enduser-api.md](./contracts/enduser-api.md) for approved deltas to the root OpenAPI contracts.

## Prerequisites

- Put Go 1.27.3 on `PATH`. Run `GOTOOLCHAIN=local go version` to check the local compiler; the security scan sets `GOTOOLCHAIN=local`.
- Install `just`, Ginkgo, and the repository development tools.
- Configure `server.enduser.public_url` as a stable HTTPS URL.
- Configure the admin reverse proxy to provide an operator principal.
- Use a third-party OAuth2 provider that accepts `private_key_jwt`.

Do not use an HTTP public URL for any CIMD service. The broker must reject that configuration.

## Run the focused automated validation

1. Run the 51 CIMD service, metadata, authentication, and rotation scenarios across both increments.

   ```sh
   ginkgo -v --label-filter="cimd-upstream-client && !performance" ./tests/e2e/
   ```

   The suite must run only the 51 scenarios that map to `spec.md`.

2. Run the SC-008 performance measurement.

   ```sh
   ginkgo -v --procs=1 --label-filter="cimd-upstream-client && performance" ./tests/e2e/
   ```

   The test warms each route once. It then sends 100 anonymous requests to each route with ten concurrent clients. Every request must return `200 OK`. Each route must have a p95 below one second.

3. Run the unit and storage validation.

   ```sh
   just test
   just test-integration
   ```

   The tests must prove all four manual service modes, assertion request shape, and in-memory persistence behavior.

4. Run the PostgreSQL migration and key-domain validation.

   ```sh
   just test-integration-infra
   ```

   The tests must apply and roll back migrations. A rollback must stop when CIMD services or keys exist.

5. Run static checks and documentation generation.

   ```sh
   just check
   just docs-build
   ```

   The checks must complete without formatting, vet, lint, or documentation errors.

## Validate CIMD service registration

1. Start the broker with an HTTPS end-user public URL.
2. When no CIMD confidential service exists, generate the first active CIMD key through `POST /api/cimd-client-keys`.
3. Create a service through `POST /api/services` with `token_endpoint_auth_method: private_key_jwt`.
4. Omit `client_id` and `client_secret` from this request.
5. Read the service through `GET /api/services/{service-id}`.

The response must include `private_key_jwt` and the broker-hosted HTTPS client ID URL. It must not include `client_secret`.

Repeat registration with a supplied client ID or a non-empty secret. The broker must return `400` before provider discovery.

## Validate public metadata and JWK documents

1. Fetch the `client_id` URL from the service response without an authentication header.
2. Read the response headers and JSON document.
3. Fetch the document `jwks_uri` without an authentication header.
4. Compare its `kid` values with `/oauth2/jwks.json`.

The metadata request must return `200`, `application/json`, and `Cache-Control: public, max-age=300`.

The metadata `client_id` must exactly equal the requested URL. It must include one callback URI, `private_key_jwt`, `ES256`, and the stable JWK URL.

The JWK response must return only ES256 public signing keys with `use: sig`. Neither response can contain a secret, private key, code, access token, refresh token, user data, or client assertion.

A static public, static confidential, deleted, unknown, or unavailable service identifier must return the JSON `404` contract. A CIMD public service follows the Increment 2 document contract.

## Validate a full connect and refresh journey

1. Start the conformant CIMD provider test double.
2. Start a user connect journey for the registered service.
3. Complete the authorization callback with a valid authorization code.
4. Trigger a refresh after the stored access token expires.

The provider must fetch the broker metadata and JWK Set. It must accept each ES256 assertion and reject shared-secret authentication.

Each assertion must use the service URL for `iss` and `sub`. Its sole `aud` must be the token endpoint. It must include a fresh `jti` and expire within five minutes.

The broker must store replacement tokens with the existing encryption behavior. It must not retry as a public client or as a static confidential client after an assertion error.

## Validate rotation and mode isolation

1. Generate a new CIMD key through `POST /api/cimd-client-keys`.
2. Fetch the service JWK Set before the key activation time.
3. Confirm that the new public key is present while the previous key signs assertions.
4. Promote an already published key with `PUT /api/cimd-client-keys/{kid}/current`.
5. Confirm that the promoted key signs the next assertion immediately.
6. Run the same route and public-document tests in `proxy`, `local`, and `hybrid` modes.

The metadata and JWK URLs must stay stable during rotation. The CIMD key routes must work in all modes. The token-signing key routes must remain unavailable in proxy mode.

The CIMD JWK Set and `/oauth2/jwks.json` must have no shared `kid` values.

## Increment 2 (issue #264): validate CIMD public clients

These procedures describe the intended behavior after implementation. The stakeholder confirmed D-A through D-D and accepted ADR 044 on 2026-10-22. Use a test authorization server that requires `none`, supports PKCE S256, and rejects client credentials. Set `ADMIN_URL` to the admin server's base URL behind the authenticated operator proxy. Set `PUBLIC_URL` to the stable HTTPS `server.enduser.public_url`. Replace the example issuer and endpoints with those of the test server. Do not put live tokens or secrets in a shell history.

### Apply and inspect the migration

1. Apply migrations 044 and 045 with the deployment's normal migration procedure before new broker replicas start.
2. Run the PostgreSQL migration integration suite:

   ```sh
   just test-integration-infra
   ```

3. Inspect `thirdparty_oauth2_services.client_registration` after migration 045. The column is non-null and defaults to `static`.
4. Make sure that legacy `private_key_jwt` rows report `cimd`. All other legacy rows report `static`.
5. Make sure that the migration rejects invalid pairs and a legacy CIMD row with a nonconforming client ID.

The check `chk_thirdparty_oauth2_services_client_registration` remains separate from the existing authentication check. A rejected rollback with CIMD public rows leaves the migration state dirty. Convert or delete each CIMD public service. Then run `migrate force 44` and retry the down migration. Do not force past an unexamined failed migration. During a rolling upgrade, retry rejected old-replica CIMD writes after all replicas run the new version.

### Register and publish without a key

1. Start the broker with an empty CIMD key set and no CIMD confidential service.
2. Send the registration request through the authenticated admin proxy:

   ```sh
   curl -sS -i -X POST "$ADMIN_URL/api/services" -H 'Content-Type: application/json' -d '{
     "display_name":"CIMD public example",
     "client_registration":"cimd",
     "token_endpoint_auth_method":"none",
     "issuer_uri":"https://auth.example.com",
     "discovery":{"enable_discovery":false},
     "endpoints":{"authorize_endpoint":"https://auth.example.com/authorize","token_endpoint":"https://auth.example.com/token"}
   }'
   ```

3. Record the returned `id` as `SERVICE_ID` and the returned `client_id` as `CLIENT_ID`.
4. Fetch the service and document:

   ```sh
   curl -sS -i "$ADMIN_URL/api/services/$SERVICE_ID"
   curl -sS -i "$CLIENT_ID"
   curl -sS -i "$CLIENT_ID/jwks.json"
   curl -sS -i "$ADMIN_URL/api/cimd-client-keys"
   ```

The service response reports `client_registration: cimd`, method `none`, and no `client_secret`. The client ID equals `$PUBLIC_URL/.well-known/oauth-client/$SERVICE_ID`. The metadata response returns `200`, `application/json`, and `Cache-Control: public, max-age=300`. It contains exactly `client_id`, one callback `redirect_uris`, both grant types, `response_types: ["code"]`, and `token_endpoint_auth_method: none`. It has no signing algorithm or JWK URI. The JWK route returns the uniform JSON `404`. The key list remains empty.

### Connect and refresh without client authentication

1. Connect a user to the registered service through the normal broker end-user flow.
2. Complete the callback with a code from the `none`-only test server.
3. Inspect the test server's token request. It has the broker URL as `client_id`, the code, and the matching `code_verifier`.
4. Trigger a refresh after the access token expires.
5. Inspect the refresh request. It has the same `client_id` and the refresh token.
6. Inspect the encrypted stored session after both successful `200` token responses.

Neither request sends a client secret, client assertion, assertion type, or `Authorization` header. The broker signs no assertion. A provider rejection ends the operation without trying another authentication method or storing a new token. Session audit records use `public_client: true`.

### Replace modes and reject invalid requests

1. Send a full-replacement `PUT /api/services/$SERVICE_ID` with `client_registration: cimd` and `token_endpoint_auth_method: private_key_jwt`.
2. Make sure that the request fails without a usable CIMD key.
3. Generate a key through `POST /api/cimd-client-keys`, then repeat the replacement. The client identifier URL stays unchanged.
4. Send a full-replacement `PUT` with `client_registration: cimd` and method `none`. The URL stays unchanged and no key is required.
5. Send a full-replacement `PUT` with `client_registration: static`, method `none`, and an operator `client_id`.
6. Fetch the old document. It returns `404` after the static replacement.
7. Send a full-replacement `PUT` with `client_registration: cimd` and method `none`. The broker URL replaces the operator ID.

For each `PUT`, include the full service representation, including `display_name`, `issuer_uri`, `discovery`, and `endpoints`. A static confidential replacement also needs a non-empty `client_secret`. The broker removes a prior stored secret only after the complete replacement validates. Providers can cache the previous document for up to 300 seconds. If an old refresh token fails at the provider after an identity change, reconnect the user.

Send separate create or replacement requests for each invalid pair: `static` with `private_key_jwt`, and `cimd` with an absent or null method. Also try `client_registration` values `null`, `""`, and an unknown string. Try a caller `client_id` or non-empty `client_secret` with CIMD public. Try `cimd` with Google flavor or filesystem source. Each request returns the existing JSON `400` error before discovery or provider traffic and leaves the stored service unchanged. A discovery request with `discovery.resource_url` and a supplied registration also returns `400`. Discovery without that field reports `cimd` for selected hosted CIMD and `static` for DCR. It never selects CIMD public.

Omit `client_registration` on separate create and full-replacement requests. The broker selects `cimd` only for `private_key_jwt`, and `static` otherwise. Omission on a CIMD public replacement selects `static`. Without the required operator `client_id`, it returns `400`. Inspect read and list responses for each valid manual mode and the classified legacy services.

### Repeat in each OAuth2 server mode

Run the keyless registration, metadata, connect, and refresh steps separately in `proxy`, `local`, and `hybrid` modes. Start each broker with only CIMD public services and an empty CIMD key set. In `proxy` mode, confirm that the hosted client ID and `none` token request do not use proxy upstream credentials. In all three modes, confirm that startup creates no CIMD key and that the document remains available.

Run the new focused scenarios separately:

```sh
ginkgo -v --label-filter="cimd-public-client" ./tests/e2e/
```

Exactly 20 scenarios cover US6, US7, and US8. The combined focused command at the start of this guide selects 51 scenarios.
