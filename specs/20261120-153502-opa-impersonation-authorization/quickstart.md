# Quickstart: validate OPA impersonation authorization

**Feature**: `specs/20261120-153502-opa-impersonation-authorization` | **Spec**: [spec.md](spec.md) |
**Contracts**: [contracts/](contracts/)

This guide describes how to validate the feature. It is not a record of any run.

## Prerequisites

- Go toolchain and tools from `just install-tools`.
- No container runtime: the acceptance suite uses an in-process bundle server and in-memory storage.
- Bundle tarballs are built in tests with OPA's `bundle.Write`; Rego fixtures live in
  `tests/e2e/fixtures/opa/impersonation/`.

## Automated validation

| Goal | Command | Expected outcome |
| --- | --- | --- |
| Static checks | `just check` | Passes |
| Unit tests (domain, adapter, config, handler) | `just test` | Passes, including `go test -race` for revision swaps under concurrent evaluation |
| Acceptance journeys 1–4 | `ginkgo -v --focus "OPA Impersonation Authorization" ./tests/e2e/` | One passing `It` per scenario 1.1–4.4 (19 in total) |
| Existing impersonation behavior | `ginkgo -v --focus "Impersonation" ./tests/e2e/` | Existing CEL suites pass unchanged (SC-005) |
| Helm contract | `just helm-lint` and `just helm-template` | Schema accepts the CEL and OPA rule shapes and rejects mixed blocks |

## Manual walk-through (local broker)

1. Create a bundle directory with `policy.rego` from
   [contracts/policy-input-decision.md § 3](contracts/policy-input-decision.md#3-decision-document)
   and `data.json` containing
   `{"governance":{"agents":{"550e8400-e29b-41d4-a716-446655440000":{"status":"approved","application":"inventory-api"}}}}`.
2. Write a bootstrap file:

   ```yaml
   bundles:
     impersonation:
       resource: file:///absolute/path/to/bundle-dir
   ```

3. Configure one impersonation rule as in
   [contracts/impersonation-opa-config.yaml](contracts/impersonation-opa-config.yaml) `example`, with
   `config_file` pointing to the bootstrap file, and start the broker in local mode.
4. Send Journey 1 Step 2 from [spec.md](spec.md) Journey 1
   with test-issued credentials. Expect `200` and the decoded claims of Journey 1 Step 4.
5. Change `status` to `unregistered` and rebuild the bundle (or serve it over HTTP with a new ETag).
   After the next poll, the same request returns `403 access_denied` (Journey 2 step 2), and the log
   contains `oauth2_server.impersonation_policy.revision_admitted`.
6. Add `allow if http.send({"method":"GET","url":"https://cmdb.example.com"})` to a module. The
   log shows `revision_rejected` with `reason=prohibited_builtin`, and step 5's outcome is unchanged.
7. Point `config_file` at a missing file, or add `decision_logs: {console: true}` to the bootstrap.
   Startup fails, naming the rule and `authorization.opa.policy.config_file`, and printing no file
   content.

## Expected observability

- `tokenexchange.impersonation.opa.revisions{outcome="admitted"}` increments after each admitted
  activation; `{outcome="rejected",reason=...}` after each rejected one.
- `tokenexchange.impersonation.opa.revision.age` grows while no new revision is admitted.
- `impersonation_decision` records contain `authorization_engine=opa` and `policy_revision`, and no
  claim values ([contracts/observability.md](contracts/observability.md)).
