# Quickstart: Validating `private_key_jwt` for CIMD Clients

Validation guide for feature 051. Contracts: [token endpoint](./contracts/token-endpoint.md), [metadata](./contracts/authorization-server-metadata.md). Data shapes: [data-model.md](./data-model.md).

## Prerequisites

- Go toolchain from `go.mod`; `just` installed.
- Docker or Podman for infra-backed integration tests.
- A broker config with `oauth2_authorization_server.mode` set to `local` or `hybrid` and `cimd.enabled: true` (see `examples/config/cimd.yaml`).

## 1. Fast checks

```fish
just check
just test
```

Expected: static checks and package tests pass. The verifier, key-set service, document parsing, metadata and cleanup tests are in this set.

## 2. Acceptance scenarios (E2E)

```fish
just test-e2e
```

Expected: every `It()` in the `tests/e2e/cimd_private_key_jwt_*_test.go` files passes (one per spec scenario), together with the baseline test showing a `none` CIMD client still exchanges a code without an assertion. The two-replica replay scenario carries `Label("docker")`: it is excluded from `just test-e2e-backend` and runs in `just test-e2e-docker`, which `just test-e2e` includes (Docker or Podman required).

## 3. Replay store and migrations (infra-backed)

```fish
just test-integration-infra
```

Expected:
- Both new migrations apply, roll back and re-apply without data loss.
- The replay repository accepts exactly one of N concurrent `Consume` calls for the same `(client_id, jti)`, keeps the record when the surrounding transaction rolls back, and sweeps only elapsed records.

## 4. Manual check against a ChatGPT-shaped client

1. Start the broker locally in `local` mode with CIMD enabled.
2. Pre-register a test client URL on an Agent through the admin API.
3. Serve a document at that URL with `token_endpoint_auth_method: private_key_jwt`, `token_endpoint_auth_signing_alg: RS256`, a `jwks_uri`, and `grant_types: [authorization_code, refresh_token]`. Serve the key set at the `jwks_uri`.
4. Fetch the broker's metadata and confirm `private_key_jwt` and `RS256` appear.
5. Run authorize and consent in a browser, then exchange the code with a freshly signed assertion (see the example in the token-endpoint contract).

Expected: `200` with access and refresh tokens. Repeat the same request with the same assertion: `401 invalid_client`. Send the exchange without an assertion: `401 invalid_client`, and the original code still works with a valid assertion. Make the test host unreachable and authorise again: `500 server_error` (not `invalid_client`).

6. Confirm the real content type served by ChatGPT's `jwks.json` is `application/json` or `application/jwk-set+json` (Risk R4).

Observed 2026-10-21: `https://chatgpt.com/oauth/jwks.json` returned `Content-Type: application/json; charset=utf-8`.

## 5. Observability

During step 4, confirm structured audit events (`client_authenticated`, `client_authentication_rejected`, `client_key_set_refreshed`) appear and that no log line contains an assertion, token, code or key material (SC-007).

## 6. Full gate

```fish
just verify
just test-integration-all
```

Expected: all green.
