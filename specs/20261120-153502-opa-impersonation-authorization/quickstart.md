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
| Acceptance journeys 1–4 | After `just web-build`: `ginkgo -v --label-filter="!performance && !docker" --focus "OPA Impersonation Authorization" ./tests/e2e/` | One passing `It` per scenario 1.1–4.3 (17 in total) |
| Existing impersonation behavior | `just test-e2e-backend` | Existing CEL suites pass unchanged (SC-005) |
| Container-free ExtProc regression subset | `ginkgo -v --procs=1 --skip='Agentgateway' ./tests/e2e/extproc/` | Runs direct ExtProc scenarios; excludes every Agentgateway container context, including Metadata Input via Agentgateway |
| Helm contract | `just helm-lint` and `just helm-template` | Schema accepts the `impersonation.opa` engine block and the CEL and OPA rule shapes, and rejects mixed blocks and engine keys on rules |

The full `just test-e2e-extproc` recipe includes Agentgateway testcontainers scenarios and runs
only in downstream container validation, together with other container-backed suites. The subset
above is not evidence that the full recipe passed; do not change tests or CI to make it container-free.

## Manual walk-through (local broker)

1. Create a bundle directory with `policy.rego` from
   [contracts/policy-input-decision.md § 3](contracts/policy-input-decision.md#3-decision-document)
   and `data.json` containing
   `{"governance":{"agents":{"inventory-agent":{"status":"approved","application":"inventory-api"}}}}`.
   The broker Agent with Journey 1's UUID must have literal External ID `inventory-agent`. An
   unset External ID is omitted from `input.context.agent`, so the lookup is undefined; unset or
   unknown External IDs deny the example policy, never falling back to UUID.
2. Package the modules and data as an OPA bundle tarball and serve it from a local test HTTP
   server at `http://127.0.0.1:8182/bundles/impersonation.tar.gz`, with a new ETag on each revision.
   Write the bootstrap as `/etc/broker/opa/bootstrap.yaml` (or in a temporary local directory under
   `/tmp/gh-aw/agent/`):

   ```yaml
   services:
     governance:
       url: http://127.0.0.1:8182
   bundles:
     impersonation:
       service: governance
       resource: bundles/impersonation.tar.gz
       polling: {min_delay_seconds: 1, max_delay_seconds: 1}
   ```

3. Configure the engine and one impersonation rule as in
   [contracts/impersonation-opa-config.yaml](contracts/impersonation-opa-config.yaml) `example`, with
   absolute `impersonation.opa.policy.config_file` pointing to the bootstrap. Use a regular file at
   most 1 MiB. For a bundle-service token, use inline credentials or mount the token file anywhere
   and select `credentials.plugin: aib_bearer_token_file` with its absolute path; OPA-native file references
   such as bearer `token_path` and TLS file paths fail startup. Start the broker in local mode.

   Shortcut without a bundle server: set `impersonation.opa.policy.path` to the step 1 directory instead
   of `config_file`. Steps 4 and 6 work after a restart;
   step 5 needs managed mode because a local directory is read only at startup.
4. Send Journey 1 Step 2 from [spec.md](spec.md) Journey 1
   with test-issued credentials. Expect `200` and the decoded claims of Journey 1 Step 4.
5. Change `status` to `unregistered`, rebuild the bundle, and publish it with a new ETag.
   After the next poll, the same request returns `403 access_denied` (Journey 2 step 2), and the log
   contains `oauth2_server.impersonation_policy.revision_admitted`.
6. Add `allow if http.send({"method":"GET","url":"https://cmdb.example.com"})` to a module. The
   log shows `revision_rejected` with `reason=prohibited_builtin`, and step 5's outcome is unchanged.
7. Point `config_file` at a missing file, or add `decision_logs: {console: true}` to the bootstrap.
   Startup fails, naming `impersonation.opa.policy.config_file`, and printing no file content.
8. Add `startup_timeout: 30s` or `policy_url: http://opa:8181` to the rule's `authorization.opa`
   block. Startup fails, naming the rule and the key.
9. Replace the token file's contents with a new token. The next bundle poll uses it without a
   restart; an empty file logs `oauth2_server.impersonation_policy.token_unavailable` and keeps the
   active revision.

## Expected observability

- `tokenexchange.impersonation.opa.revisions{outcome="admitted"}` increments after each admitted
  activation; `{outcome="rejected",reason=...}` after each rejected one.
- `tokenexchange.impersonation.opa.revision.age` grows while no new revision is admitted.
- `impersonation_decision` records contain `authorization_engine=opa`, `policy_revision`, and
  `policy_admission_id` (the deciding snapshot's process-instance UUID/engine sequence), and no
  claim values ([contracts/observability.md](contracts/observability.md)).
- Initial readiness requires one admitted all-configured-bundle snapshot; a usable first subset
  followed by a rejected complete candidate never becomes ready.
- Cancellation acceptance uses a correlated ended `canceled` evaluation span, no issuance and no
  later-rule authorization; a disconnected client is not expected to receive HTTP 500.
- Equal or empty manifest labels on two admitted snapshots must still yield different admission
  identifiers, matched to the snapshot used by each concurrent request.
