# Quickstart: Wildcard Redirect URI Hosts

This guide validates broker behavior. Applying the registration to an external deployment's existing and newly created test Agents is a separate rollout step.

## Prerequisites

- Go 1.27.3 and `just` are installed.
- Repository dependencies are available.
- The Go workspace is at the repository root.

## Validate Domain Rules

Run focused URI and Agent validation tests:

```fish
go test ./internal/domain/urivalidation ./internal/domain/storage ./internal/domain/oauth2
```

Expected results:

- The preview wildcard matches the two PR preview hosts, stable test host, sandbox host, and staging host.
- Wildcards cannot cross hostname labels or replace the registrable domain/public suffix.
- Scheme, port, path, and query mismatches do not match.
- Exact callbacks, loopback HTTP behavior, and private-use callback rules remain unchanged.
- Agent create/update validation rejects malformed or unsafe wildcard registrations.

## Validate End-to-End Behavior

Run only the feature's HTTP acceptance scenarios:

```fish
ginkgo -v --focus="Wildcard Redirect URI" ./tests/e2e/
```

Expected results: all nine scenarios in [spec.md](spec.md) pass. Matching callbacks receive the authorization result and their own `state`; out-of-scope callbacks receive `invalid_redirect_uri` without a redirect; invalid registration receives the existing validation error.

## Run Required Repository Gates

```fish
just check
just test
just test-e2e-backend
```

Expected results: formatting, static checks, Go tests, and backend E2E tests pass. No database or external-service setup is required for these scenarios.
