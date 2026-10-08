# Quickstart: Validate Protected Resource Discovery

## Purpose

Use this guide after implementation. It proves that the broker can create a third-party service from a protected resource URL. It also proves resource binding, client selection, failure isolation, and durable status.

See [data-model.md](./data-model.md) for stored state. See [admin-api.md](./contracts/admin-api.md) and [upstream-oauth.md](./contracts/upstream-oauth.md) for the API and wire contracts.

## Prerequisites

- Use Go 1.27.1, `just`, Ginkgo v2, and the repository development tools.
- Start Docker or Podman for PostgreSQL migration and restart validation.
- Set `server.enduser.public_url` to a stable public HTTPS URL for hosted CIMD and DCR callbacks.
- Configure the admin reverse proxy to send an operator principal.
- Set `third_party_oauth2.client_name` to a non-blank broker or platform name for DCR. CIMD and manual services do not need this setting.
- For manual validation, use a public HTTPS protected resource and authorization server.

Do not use private, loopback, or redirected URLs for production validation. The broker must reject them.

## Run the automated validation

1. Run the 41 functional acceptance scenarios.

   ```sh
   ginkgo -v --label-filter="protected-resource-discovery && !performance" ./tests/e2e/
   ```

   The command must select exactly 41 `It()` blocks. Each block must have one `USx-Sy from specs/050-oauth2-protected-resource-discovery/spec.md` reference.

2. Run the SC-006 performance measurement.

   ```sh
   ginkgo -v --procs=1 --label-filter="protected-resource-discovery && performance" ./tests/e2e/
   ```

   The test creates 20 independent services. Each provider response waits one second or less. At least 19 registrations must complete within five seconds.

3. Run the unit, adapter, and self-contained integration tests.

   ```sh
   just test
   just test-integration
   ```

   These tests must cover metadata order, challenge cardinality, identity checks, client selection, missing refresh tokens, expired credential rejection, safe transport, resource rules, and memory persistence.

4. Run PostgreSQL migration and repository validation.

   ```sh
   just test-integration-infra
   ```

   Migration 036 must apply, refuse unsafe rollback, roll back cleanly when allowed, and replay. The two restart scenarios in step 1 use the `docker` label. They create a new adapter for the same PostgreSQL database.

5. Run static checks and build documentation.

   ```sh
   just check
   just docs-build
   ```

   The commands must complete without formatting, vet, lint, or documentation errors.

## Validate hosted CIMD registration

1. Make sure that an active CIMD client-authentication key exists.
2. Configure the provider with one issuer, CIMD support, `private_key_jwt`, ES256, and a DCR endpoint.
3. Send `POST /api/services` with `discovery.enable_discovery: true` and `discovery.resource_url`.
4. Omit `issuer_uri`, endpoints, client fields, and `token_endpoint_auth_method`.
5. Read `GET /api/services/{service-id}` and `GET /api/services/{service-id}/discovery-status`.

The service must use the hosted client ID and `private_key_jwt`. Its `discovery.client_method` must be `cimd`. The provider must receive no DCR request. The status must be `ready`, with both timestamps set and no failure reason.

## Validate DCR selection

1. Configure the provider with no compatible CIMD capability.
2. Advertise both `client_secret_basic` and `none`.
3. Create a discovery-backed service.
4. Read the service and the provider registration log.

The broker must send one registration request for `client_secret_basic`. The service must show `discovery.client_method: dcr`, the returned `client_id`, and `client_secret_basic`. The response must not contain `client_secret`.
The DCR request must send the deployment-wide broker client name. It must not send the service display name. Without a non-blank name, DCR must fail before registration and leave no service; manual and CIMD modes remain available.

Repeat with only public DCR and `S256`. The service must use `none`, store no secret, and send no secret during token requests.

For renewal, configure the provider to issue a refresh token and accept the same registered credential after restart. If it omits the refresh token, renewal must fail without re-registration. If DCR returns any non-zero `client_secret_expires_at`, service creation must fail with `client_registration_invalid` and must not retry public DCR.

## Validate resource binding

1. Create a service without `authorization_params.resource`.
2. Start a user connection through `/api/third-party/{service-id}/oauth2/authorize`.
3. Complete the callback and then force a refresh.
4. Read the provider authorization query and both token forms.

Each request must contain exactly one `resource` value. The value must equal the service's `authorization_params.resource`. Authorization must contain PKCE S256. Code exchange must contain its verifier.

Configure the provider to reject `resource`. The connection or refresh must fail. The provider must receive no retry without `resource`, and the broker must store no replacement token.

## Validate failure isolation and restart

1. Create a ready DCR service on PostgreSQL.
2. Connect one user.
3. Make the provider return invalid authorization-server metadata.
4. Send a discovery-backed `PUT /api/services/{service-id}`.
5. Restart the broker with a new storage adapter for the same database.
6. Read the service and status. Then refresh the user session.

The update must fail with a safe error code. The service must keep its previous issuer, endpoints, client ID, secret, effective resource, and ETag. The status must be `failed`, with the new attempt time, prior success time, and safe reason. The refresh must use the previous client identity.

## Validate rejection paths

Run each request against a fresh service name:

- Protected resource metadata is absent, even when manual endpoint values are present.
- Metadata reports another resource or issuer.
- A discovered URL resolves to a private address or redirects.
- Metadata advertises two issuers and the request has no `issuer_uri`.
- A request has both `resource_url` and `metadata_url`.
- A selected CIMD identity is unavailable while DCR is advertised.
- Selected confidential DCR is rejected while public DCR is advertised.
- A same-issuer DCR client ID already exists.

Each create request must fail. The broker must create no service, status resource, or stored credential. The provider must receive no fallback registration after the selected method fails.

## Validate the not-applicable status

1. Create a manual service with explicit endpoints.
2. Read its discovery status.
3. Connect a user and refresh the session.

The status must be `not_applicable` with null discovery fields. The provider must receive no metadata or registration request. Existing manual connection and renewal behavior must not change.
