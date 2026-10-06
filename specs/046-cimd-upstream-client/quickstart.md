# Quickstart: Validate a CIMD Upstream Client

## Purpose

Use this guide after implementation. It proves the broker can authenticate to a third-party OAuth2 service with `private_key_jwt`.

See [data-model.md](./data-model.md) for the stored state. See [admin-api.md](./contracts/admin-api.md) and [enduser-api.md](./contracts/enduser-api.md) for approved deltas to the root OpenAPI contracts.

## Prerequisites

- Use Go 1.26.8.
- Set `GOEXPERIMENT=jsonv2` for Go 1.26 commands.
- Install `just`, Ginkgo, and the repository development tools.
- Configure `server.enduser.public_url` as a stable HTTPS URL.
- Configure the admin reverse proxy to provide an operator principal.
- Use a third-party OAuth2 provider that accepts `private_key_jwt`.

Do not use an HTTP public URL for a CIMD confidential service. The broker must reject that configuration.

## Run the focused automated validation

1. Run the 31 CIMD service, metadata, authentication, and rotation scenarios.

   ```sh
   ginkgo -v --label-filter="cimd-upstream-client && !performance" ./tests/e2e/
   ```

   The suite must run only the 31 scenarios that map to `spec.md`.

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

   The tests must prove the three service modes, assertion request shape, and in-memory persistence behavior.

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

A public, static, deleted, unknown, or unavailable service identifier must return the JSON `404` contract.

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
