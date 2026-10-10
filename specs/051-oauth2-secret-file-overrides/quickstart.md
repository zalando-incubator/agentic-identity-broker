# Quickstart: Validate Explicit OAuth2 Credential Sources

**Propagated**: 2026-10-07 — Updated from spec.md refinement

**Updated**: 2026-10-08 — Aligned with implemented coherent pair acquisition, irreversible source history, completed story checks, and local delivery evidence.

## Status and prerequisites

This guide validates implemented, unreleased behavior described by [plan.md](./plan.md). The source selector, migration 036, reader, tests, chart value, and CLI flag exist. The evidence that follows distinguishes completed checks from the pending full final gate. No published broker release or live deployment is claimed.

~~The smoke setup requires a different stored placeholder, while registration, API, and storage remain unchanged.~~ Superseded by explicit filesystem registration without inline credentials.

**Design approval, 2026-10-08**: The user accepted ADR 038 and selected a major contract bump to Admin API `2.0.0`. Published `v0.1.81` contains the incompatible `1.0.0` required-client-ID schema. The implemented source-aware schema uses existing `/api/services` routes without a compatibility endpoint or shim. [`docs/changelog.md`](../../docs/changelog.md) records the unreleased breaking change and consumer migration. The route examples remain valid. Approval preceded implementation and did not itself prove runtime behavior.

Historical design and test-first evidence remains recorded here.

Required tools and inputs:

- Go 1.27.1, `just`, Ginkgo, and the existing frontend build toolchain for automated package and HTTP checks
- Helm for chart rendering, and `jq` for the manual smoke run
- A non-root Linux or Darwin account for the manual smoke run
- A feature-capable broker binary for the manual smoke run, and a chart checkout for deployment validation
- Synthetic credentials only
- The deployment checkout for the separate target-generation validation.
- Docker or Podman for the PostgreSQL migration and repository checks.

Run broker commands from the broker repository root. Run the deployment generator from the deployment repository root.

For exact semantics, see [configuration](./contracts/configuration.md), [outbound authentication](./contracts/outbound-authentication.md), and [deployment](./contracts/deployment.md).

`.specify/feature.json` selects this feature for local Spec Kit scripts. `SPECIFY_FEATURE_DIRECTORY` is an optional explicit override, not a required workaround.

```bash
env -u SPECIFY_FEATURE_DIRECTORY -u SPECIFY_FEATURE \
  bash .specify/scripts/bash/check-prerequisites.sh --paths-only --json
```

**T013 bootstrap smoke, 2026-10-08:** The shared real-loader/production-Builder fixture completed stored-service authorization and refresh through separate HTTP servers. Observed callback `302`, refresh `200`, two provider token requests, and exact synthetic credential matches. A real broker binary then ran under UID `501` and completed registration `201`, callback `302`, and refresh `200`. This proves the acceptance bootstrap, not filesystem feature behavior. The temporary smoke program was removed.

**T064 initial semantic red, 2026-10-08:** Before chart or broker behavior changes, the compiled feature-label run selected exactly 50 scenarios. Five stored/public/CIMD/Google controls passed, and 45 feature expectations failed. No selected feature scenario was pending or skipped. The other 646 suite scenarios were filtered out, not feature skips. Failures observed missing filesystem registration/source representation, missing CLI support and configuration validation, and absent chart support. Command: `ginkgo -v --procs=1 --label-filter='oauth2-secret-file-overrides' --json-report=tmp/oauth2-credential-initial-red.json ./tests/e2e/`.

The focused command `go test ./tests/integration -run TestHelmCredentialFiles -count=1` compiled and failed for explicit empty and populated pair inputs. The actual chart schema rejected `credentialFiles` as an unknown property. Typed default-omission and invalid-input controls also ran. Earlier fixture-only matcher/parser/configuration errors were corrected before this evidence run. No expectation was weakened or marked pending.

**T021 post-chart gate, 2026-10-08:** The same 50 compiled cases ran after T065: five stored/public/CIMD/Google controls passed, and 45 broker-feature expectations remained red. No selected case was skipped or pending. All three deployment journeys passed chart rendering and reached missing broker filesystem registration (`400` rather than expected `201`). Focused typed Helm tests and `just helm-lint` passed. Actual target rendering contained exactly both employee paths, a read-only credential directory without `subPath`, the existing configuration and `/tmp` mounts, and non-root pod settings. `just check` passed with zero issues.

Accepted ADR 038, approved Admin API 2.0.0, current supporting contracts, exact database design, indexed examples, evidenced frontend non-applicability, and initial red proof close the blocking design gate. Only compile-time source/session/state declarations accompanied the acceptance tests. No broker source selection, reader, persistence, or identity behavior was implemented before this gate.

**Foundation test-first evidence, 2026-10-08:** `go test ./internal/adapters/credentialfile -count=1` compiled and failed on real valid-file reads, acquisition stages, and specific source reasons while the minimum reader declarations returned safe `read_failed`. `go test ./internal/config ./cmd/agentic-identity-broker -run TestCredentialFiles -count=1` compiled and failed on case/dotted-key preservation, strict input validation, source precedence, and missing environment/CLI handling. These runs precede T025/T026 behavior. A temporary T024 smoke confirmed `errors.Is` unavailable classification, closed `read_failed` normalization for an unknown reason, and no raw-value exposure. The temporary program was removed.

**T025–T028 foundation green, 2026-10-08:** The unchanged reader suite and focused strict loader/validator suite passed. The real root-command tests passed after rebuilding the broker, and `--help` displayed the new JSON flag with default `{}`. Builder injects the stateless reader and the session constructor copies the binding map once. `just check` reported zero issues. Reader, model, app, and session package tests passed. The integrated smoke observed exact-case binding lookup, normalized pairs, current same-ID secret rotation, empty pair results on source loss, client-ID-only success during secret loss, and stored callback/refresh `302`/`200`. The temporary smoke program was removed. At that checkpoint, full filesystem authentication and runtime-source tests still depended on US1 core implementation.

**T001–T069 implementation evidence, 2026-10-08:** All 50 acceptance cases passed in individual story runs. Selected unit/integration packages and 30 legacy public/CIMD journeys passed. Source ownership, all three operation boundaries, nullable state/session identity, monotonic history, both stores, and migration 036 are implemented. This evidence does not claim a completed combined acceptance run or full final gate.

External test, prod, and sandbox manifests regenerated with the existing script from the current local chart, including uncommitted feature changes. Validation covered the exact pair and read-only credential/configuration/tmp mounts. `scripts/chart_ref` remains `ad2289121b459ea14d56efd7d227bbf71d452948`, a historical chart path revision rather than the full working-chart snapshot.

The local linux/arm64 image `agentic-identity-broker:oauth2-credential-files-051` was built and verified with UID 1000. Its local image ID is `sha256:f128f4a9237e272b32773d87682fd8ca06b6aea2247a9b984c9634b95d9a7ab0`. CDP `DEP_BROKER_VERSION` placeholders remain. No registry publication or live deployment occurred.

**Suite dependency corrections, 2026-10-09:** `just check`, race-enabled `just test`, focused configuration/session/reader package tests, and `just test-integration` passed. All 50 feature scenarios passed with four Ginkgo workers. They also passed through the current test executable with an empty tool PATH, without Helm. Real Helm rendering passed with `REQUIRE_HELM=1`. Missing Helm skipped optional rendering checks and failed required checks as expected. `actionlint .github/workflows/ci.yml` passed.

A temporary production-bootstrap smoke observed registration `201`, callback `302`, rotated refresh `200`, and identity-mismatch refresh `500`. Exactly two provider requests used client A and the expected secrets. A new authorization used B, and metadata remained available during a secret-file outage. The smoke program was removed. These checks do not claim a full repository verification gate or a new PostgreSQL infrastructure run.


## 1. Run the focused checks

```bash
just check
just test
just web-build
```

Run the focused package boundaries:

```bash
go test ./internal/config/... \
  ./internal/adapters/credentialfile/... \
  ./internal/domain/model/... \
  ./internal/domain/thirdparty/... \
  ./internal/adapters/http/handlers/admin/... \
  ./internal/adapters/storage/memory/... \
  ./internal/domain/oauth2session/... \
  ./internal/domain/tokenexchange/... \
  ./cmd/agentic-identity-broker/...
```

Expected result: all checks pass. Configuration tests prove pair types, both required paths, case preservation, whole-map precedence, and explicit-empty behavior. Registration tests prove source-specific credentials, mixed-source rejection, and update omission.

Package and backend HTTP suites require no prebuilt broker binary. Normal HTTP journeys use the production loader and Builder in process. Privilege and blocking-file cases re-execute the current Go test executable. Exhaustive configuration tables remain in package tests; acceptance scenarios use representative startup cases for each source.

Helm rendering tests skip locally when Helm is unavailable. The existing Helm CI job requires them:

```bash
REQUIRE_HELM=1 go test -count=1 ./tests/integration -run '^TestHelmCredentialFiles'
```

Reader tests must prove the 65,536-byte boundary, real symlink reads, empty-source rejection, and prompt rejection of non-regular sources. Deterministic real-I/O interleavings must replace targets between pair opens, reads, and revalidation. Require `generation_changed`, neither partial value, closed descriptors, and current-pair recovery on the next acquisition.

Run the persistence layer with PostgreSQL available:

```bash
just test-integration
just test-integration-infra
```

Expected evidence: legacy stored backfill, both store contracts, atomic source transitions, and session identity persistence. The internal transition marker must remain true after a source round trip. Verify compatible UP/DOWN/reapply and guarded incompatible DOWN. A filesystem row or retained transition marker must prevent downgrade. Follow the rollback safeguards in [data-model.md](./data-model.md#postgresql-and-in-memory-persistence-design). Never force migration metadata without verifying the actual schema and data.


## 2. Run all mapped acceptance journeys

```bash
ginkgo -v --procs=1 \
  --label-filter='oauth2-secret-file-overrides' \
  ./tests/e2e/
```

~~Exactly 40 feature scenarios run.~~ Superseded by the refined acceptance mapping. Expected result: exactly 50 feature scenarios run, with no pending or skipped scenario. The mappings are in the plan's Testing Strategy.

Repeat with parallel workers:

```bash
ginkgo -v --procs=4 \
  --label-filter='oauth2-secret-file-overrides' \
  ./tests/e2e/
```

Expected result: the same 50 scenarios pass without shared file, provider, environment, port, or log state.

During test-first implementation, record semantic failure for the new behavior before production changes. Existing-behavior controls can already pass. Do not fabricate a failure to change that result.

## 3. Observe the rotation and source-loss journeys

```bash
E2E_VERBOSE=1 ginkgo -v --procs=1 \
  --label-filter='oauth2-secret-file-overrides' \
  --focus='US2-S1|US2-S3|US2-S5|US3-S6|US3-S8|US3-S9|US7-S2' \
  ./tests/e2e/
```

The mapped `It()` descriptions include these scenario IDs, so this focus selects seven scenarios.

Expected evidence:

| Journey | Result |
|---|---|
| Complete secret replacement for the same client ID | The next exchange and refresh use the current synthetic pair without restart or service writes. |
| Projected-volume generation replacement | The next operation uses a coherent pair. Detected acquisition races stop before provider authentication with `generation_changed`. |
| Client ID A becomes B | New authorization uses B. A's code and session stop before provider authentication as B and require a new connection. |
| Source loss after success | Exchange and refresh fail before a new provider token request. Client-ID loss also stops initiation. Secret loss does not. |
| Source restoration | The next operation succeeds without restart or registration change. |
| Source error versus provider rejection | Operator events and internal categories differ. Existing HTTP responses remain unchanged. |
| Non-root deployment | Connect before rotation and refresh afterward use the appropriate file values. |

The provider double records Basic and form credentials in memory. Production events remain credential-free. Compare provider requests rather than treating a selection event as proof of acceptance. The client-ID-change journey must exercise both a paused A authorization and an established A session.

## 4. Run an actual broker smoke scenario

Run this section after implementation and automated checks. Use a principal-aware local browser or HTTP client for complete authorization redirects. Send the principal header only to the broker origin, never the provider.

Publish replacement credentials as fresh immutable targets. Publish the pair together through an atomic projected-volume generation change. Never modify a published target or republish a retired target during acquisition.

A detected change stops the current attempt without an acquisition retry. Publication after validation can leave an in-flight operation using its validated pair. Provider validity and established-identity checks still apply.

Build the broker for this manual smoke run:

```bash
just build
```

Create a synthetic credential directory in terminal A:

```bash
id -u
SECRET_DIR="$(mktemp -d)"
printf '%s' 'mock-oauth2-client-dev' > "$SECRET_DIR/client-id"
printf '%s' 'mock-oauth2-secret-12345' > "$SECRET_DIR/client-secret"
printf 'SECRET_DIR=%s\n' "$SECRET_DIR"
export IDENTITY_BROKER_JWE_SIGNING_KEY="$(openssl rand -base64 32)"
export IDENTITY_BROKER_SERVER_ENDUSER_PUBLIC_URL='http://127.0.0.1:8000'
export IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES="$(printf '{"file-smoke":{"client_id_file":"%s/client-id","client_secret_file":"%s/client-secret"}}' "$SECRET_DIR" "$SECRET_DIR")"
./bin/agentic-identity-broker \
  --config examples/config/config.development.yaml \
  --server.enduser.port=8000 \
  --server.admin.port=14000
```

Expected result: `id -u` is nonzero and the broker serves both loopback ports. The development configuration uses `X-Custom-Principal`. Do not use this setup in production. In terminals B and C, set `SECRET_DIR` to the same printed absolute path.

In terminal B, start the existing provider with an isolated loopback configuration. Its checked-in configuration binds all interfaces, so do not use the unmodified start recipe for this smoke run.

```bash
mkdir "$SECRET_DIR/provider"
cat > "$SECRET_DIR/provider/config.yaml" <<'YAML'
server:
  port: 9000
  bind: "127.0.0.1"
oauth2:
  client_id: "mock-oauth2-client-dev"
  client_secret: "mock-oauth2-secret-12345"
  access_token_ttl: 3600s
  refresh_token_ttl: 604800s
  authorization_code_ttl: 300s
  scopes:
    - name: "profile"
      description: "Synthetic profile access"
mock_user:
  sub: "file-smoke-user"
  name: "Synthetic User"
  email: "file-smoke@example.invalid"
YAML
go -C mocks/third-party-service run ./cmd/mock-oauth2-server "$SECRET_DIR/provider"
```

The provider listens at `127.0.0.1:9000` with the same synthetic pair as the files. Its consent flow validates PKCE. See `mocks/third-party-service/README.md` and its configuration loader for runtime behavior.

In terminal C, register a filesystem service with neither inline credential:

```bash
cat > "$SECRET_DIR/register.json" <<'JSON'
{
  "canonical_id":"file-smoke",
  "display_name":"Filesystem credential smoke",
  "credential_source":"filesystem",
  "issuer_uri":"http://127.0.0.1:9000",
  "discovery":{"enable_discovery":false},
  "endpoints":{
    "authorize_endpoint":"http://127.0.0.1:9000/oauth/authorize",
    "token_endpoint":"http://127.0.0.1:9000/oauth/token"
  },
  "scopes":[{"scope_value":"profile","description":"Synthetic profile access"}]
}
JSON
curl --fail-with-body -sS \
  -H 'X-Custom-Principal: file-smoke-user' \
  -H 'Content-Type: application/json' \
  -D "$SECRET_DIR/service.headers" \
  -o "$SECRET_DIR/service.json" \
  -X POST http://127.0.0.1:14000/api/services \
  --data-binary "@$SECRET_DIR/register.json"
SERVICE_ID="$(jq -r '.id' "$SECRET_DIR/service.json")"
jq '{credential_source, has_client_id:has("client_id"), has_client_secret:has("client_secret")}' \
  "$SECRET_DIR/service.json"
```

Expected result: HTTP 201 and a service UUID. The source is `filesystem`. Both inline fields are absent, not `null` or `REDACTED`. The response contains neither file value nor path.

Request authorization with the configured broker origin as the return location:

```bash
curl -sS -D "$SECRET_DIR/authorize.headers" -o /dev/null \
  -H 'X-Custom-Principal: file-smoke-user' \
  --get --data-urlencode 'redirect_uri=http://127.0.0.1:8000/sessions' \
  "http://127.0.0.1:8000/api/third-party/${SERVICE_ID}/oauth2/authorize"
```

Expected result: HTTP 302. The provider authorization URL uses the file client ID. Through the principal-aware client, complete consent and the callback. Do not fabricate a code, change opaque state, or bypass PKCE. The broker creates a session using the configured pair without storing it on the service.

Refresh the session:

```bash
curl -sS -i -X POST \
  -H 'X-Custom-Principal: file-smoke-user' \
  "http://127.0.0.1:8000/api/third-party/${SERVICE_ID}/session/refresh"
```

Expected result: HTTP 200 and a credential-free `session.oauth2.credential_files_used` event with operation `refresh`.

Rename `client-secret` to `client-secret.saved` while the broker remains running. Repeat refresh and the authorization-initiation request.

Expected result: refresh returns HTTP 500 with `internal_error`, not `502 refresh_failed`. A safe source-failure event identifies the service UUID and operation. No provider token request occurs. Initiation still returns HTTP 302 because it reads only the usable client-ID file. Completing that new callback fails with generic `callback_failed` until the secret recovers.

Read admin and session metadata while the secret file is unavailable:

```bash
curl --fail-with-body -sS \
  -H 'X-Custom-Principal: file-smoke-user' \
  "http://127.0.0.1:14000/api/services/${SERVICE_ID}"
curl --fail-with-body -sS \
  -H 'X-Custom-Principal: file-smoke-user' \
  "http://127.0.0.1:8000/api/third-party/${SERVICE_ID}/session"
```

Expected result: both reads remain available. The admin response reports filesystem source and omits inline fields. Neither metadata surface resolves file values or exposes paths.

Restore `client-secret` and repeat refresh. Expected result: HTTP 200 without a restart. Then make only `client-id` unavailable. Initiation and refresh return generic internal errors before provider requests. Metadata still works. Restore the file and verify recovery.

For client-identity proof, pause a real authorization after the provider issues A's callback URL but before the client follows it. Replace `client-id` with synthetic B while preserving the secret. Follow A's callback with the correct broker principal. It must produce `callback_failed` without a token request. An existing A session must fail refresh with `identity_mismatch` before provider authentication. A new initiation must use B.

The fixed provider accepts only A, so it cannot prove a completed B connection or accepted secret rotation. Restore A for manual recovery. Use section 3's rotating-provider US2-S1/US2-S5 journeys for successful rotation and a complete B connection. Those scenarios must observe real broker operations and provider requests, not only domain mocks.

Verify source changes through the administrative API without reading files:

```bash
jq '. + {credential_source:"stored", client_id:"mock-oauth2-client-dev", client_secret:"mock-oauth2-secret-12345"}' \
  "$SECRET_DIR/register.json" > "$SECRET_DIR/stored.json"
curl --fail-with-body -sS -X PUT \
  -H 'X-Custom-Principal: file-smoke-user' -H 'Content-Type: application/json' \
  --data-binary "@$SECRET_DIR/stored.json" \
  "http://127.0.0.1:14000/api/services/${SERVICE_ID}"
jq 'del(.client_id,.client_secret) | .credential_source="filesystem"' \
  "$SECRET_DIR/stored.json" > "$SECRET_DIR/filesystem.json"
curl --fail-with-body -sS -X PUT \
  -H 'X-Custom-Principal: file-smoke-user' -H 'Content-Type: application/json' \
  --data-binary "@$SECRET_DIR/filesystem.json" \
  "http://127.0.0.1:14000/api/services/${SERVICE_ID}"
```

Expected result: the stored transition requires explicit credentials and returns the existing confidential redaction. The filesystem transition atomically removes both stored credentials and omits both fields. Inspect the storage integration evidence; an API projection alone does not prove erasure.

Repeat both transition directions with pending codes and established sessions. Matching recorded identities can continue. Mismatched or missing identities must stop before provider authentication. Also exercise a legacy missing-identity context through stored → filesystem → stored. The internal transition marker remains true, and returning to stored must not restore legacy reuse.

Exercise automatic refresh through the existing RFC 8693 flow after the synthetic provider expires the access token. Compare its provider request with explicit refresh. Repeat source loss and identity mismatch, requiring zero new token requests and generic `500 server_error`. Use the production-token-exchange fixtures and complete grant/agent setup, not a direct domain call.

While files are unavailable, submit an unrelated full PUT derived from `register.json` with `credential_source` omitted. Source remains filesystem and metadata stays available. Do not replace this with a partial PUT that changes unrelated semantics. Preserve existing ETag requirements when replacing protected resources.

Also submit filesystem requests containing either inline field. Expect validation rejection without record mutation or file reads. Removing the mapping and restarting must preserve filesystem mode and produce `missing_binding`, never stored fallback. Re-register the synthetic service after a memory-backend restart before checking this case; an absent record is not source-failure proof.

Stop the local broker and provider after the smoke run. Remove only the synthetic temporary directory. Never use real deployment credential files for this procedure.

## 5. Validate chart rendering

```bash
just helm-lint
just test-integration
just helm-template
just helm-template-values tests/e2e/fixtures/oauth2-secret-file-values.yaml
```

The existing fixture contains synthetic target inputs. Do not substitute production secrets into it.

Expected results:

- Default output omits `credential_files`.
- Target output contains exactly the `zalando-platform` pair with `client_id_file: /meta/credentials/employee-client-id` and `client_secret_file: /meta/credentials/employee-client-secret`.
- The credential mount remains read-only at `/meta/credentials` without a credential-file `subPath`.
- Application-configuration and `/tmp` mounts remain intact.
- The pod identity remains non-root.

The US7 acceptance tests must parse Kubernetes objects and load the rendered broker configuration. Render-only checks do not prove filesystem readability or rotation.

## 6. Validate the actual deployment generator

All three external environment inputs contain the [target value fragment](./contracts/deployment.md#existing-mount-inputs), and the deployment README is current. Registration of a live `zalando-platform` service remains separate from manifest generation. Select filesystem source without inline credentials and a feature-capable image through the deployment process. CDP image placeholders remain. No live registration or deployment is claimed.

In the deployment checkout, set `CHART_REPO_DIR` to the feature-capable broker checkout. Then run:

```bash
./scripts/update-k8s-manifests.sh
```

This script regenerates existing output directories. Do not edit the generated manifests directly.

Expected result: each generated broker ConfigMap contains exactly the target pair, not a secret-only path. Each Deployment preserves credential, application-configuration, and temporary-storage mounts. `scripts/chart_ref` records the latest chart path commit, not uncommitted working-chart contents. Current generation used the local chart with feature changes, while the file retains historical revision `ad2289121b459ea14d56efd7d227bbf71d452948`.

For runtime proof, use synthetic credentials in an isolated non-root deployment with explicit filesystem registration. Connect, publish a complete replacement pair, and refresh. Secret rotation retains identity; a client-ID change requires new connections. Observe the first post-publication operation within those continuity limits.

Record the broker image, chart revision, environment inputs, UID, file permissions, observed operation, and credential-free outcome. Do not record file contents.

## 7. Final delivery gate

```bash
just verify
```

**Status:** Combined feature runs and the standalone non-root smoke passed. `just verify` remains blocked at gosec under T080.

Required delivery evidence:

- All 50 mapped acceptance scenarios pass.
- New behavior has semantic-red evidence from before implementation.
- Initial T064 evidence covers all 50 compiled acceptance expectations before chart or broker behavior. T021 records the post-chart, pre-broker run. Existing controls can pass.
- Actual broker smoke covers filesystem registration/read/update, initiation, callback, refresh, source loss/recovery, and metadata independence.
- Rotation and non-root provider observations prove current credentials without disclosure. Client-ID changes block old codes/sessions before authentication under B.
- Source transitions atomically remove stored credentials or require explicit reverse credentials. Both directions enforce recorded identities. Legacy missing identities stay unusable after a round trip. Stored/excluded authentication and encryption remain unchanged.
- Authentication/rotation preserves service ciphertext, version, and timestamps. Normal session tokens and non-secret identity association still persist.
- PostgreSQL backfill, source constraints, both adapters, identity round-trips, and guarded rollback have matching integration evidence.
- Operator documentation, ADR, chart contract, and deployment inputs agree.
- Generated release notes include the feature and fail-closed configuration behavior.

~~No database migration or frontend screenshot is required because those surfaces do not change.~~ Superseded for persistence: migration and storage verification are required. No management UI or consent visual change is planned, so frontend screenshots remain inapplicable unless the consumer review identifies an actual UI change.

## Execution evidence: 2026-10-08

The combined feature label selected exactly 50 unique scenarios. Both one-process and four-process runs passed all 50 without selected failures or pending cases.
The full backend run passed 694 functional cases. Two performance cases were outside the functional label.
`just check`, the focused Go packages, full self-contained integration, changed PostgreSQL integration, and Helm checks passed.

The standalone smoke launched the actual broker as UID 501 with synthetic credentials and the production configuration loader.
Registration returned 201 without inline credential fields. Authorization and PKCE callback returned 302 with the expected provider credentials.
Explicit refresh, secret rotation, source recovery, and RFC 8693 automatic refresh returned 200 without a broker restart.
Source loss returned 500, while metadata returned 200 and client-ID-only initiation returned 302. Failed source operations added zero provider requests.
Changed identities blocked old codes and sessions. A new B connection succeeded. Both source transitions allowed matching identities and refused mismatches.
A legacy missing-identity code stayed unusable after a stored/filesystem/stored round trip. The smoke observed 11 provider requests and no credential values in diagnostics.
The throwaway executable and its synthetic process resources were removed after the run.

External generation parsed the exact pair and retained read-only credential-directory, configuration, and `/tmp` mounts in all three environments.
The local feature image is `agentic-identity-broker:oauth2-credential-files-051`, image ID `sha256:f128f4a9237e272b32773d87682fd8ca06b6aea2247a9b984c9634b95d9a7ab0`.
Its platform is Linux/arm64 and its configured UID/GID is 1000:1000. No image was published and no live deployment ran.
CDP image placeholders remain unchanged. The chart reference records the historical commit, not the uncommitted working-chart snapshot.


### Final security gate

`just verify` stopped at gosec. Explicit UID/GID bounds checks removed four G115 findings from the non-root process harness.
The repeated security scan reports 16 findings: three G101, one G204, four G304, five G302, and three G306.
They cover non-secret identifiers, trusted operator paths, the test executable path, and synthetic fixtures readable by a non-root UID.
The user selected manual review instead of scoped suppression. No scan rule was disabled and no new `#nosec` exception was added.
T080 remains unchecked. All non-security `just verify` stages passed when run independently.


Go/race, web unit, CDK, mock, and both integration layers passed. Backend, ExtProc, and frontend E2E passed 694, 112, and 63 cases.
All seven pinned `govulncheck` module scans passed with zero reachable vulnerabilities. The six secondary-module gosec scans reported zero issues.
The pinned OSV manifest scan reports 14 advisories affecting 13 packages, including documentation tooling, web development dependencies, and mock modules.
Those advisories are an additional security blocker. No unrelated dependency version was changed or advisory suppression added.

### Commit-preparation checks

The four-worker feature run initially passed 49 of 50 cases. US3-S11 observed the HTTP failure before subprocess audit output reached the capture. The event assertion now uses a bounded wait without changing its outcome, reason, or credential-safety checks. The same 50-case run then passed with zero failed or pending cases.

A root-run Linux container reproduced US3-S7's restart failure: the second launch collided with the existing exclusive executable copy. Each launch now copies into its own temporary directory. Root-run US3-S2, US3-S7, and US3-S11 passed against the real non-root broker process.

`just check`, race-enabled `just test`, both integration commands, the host/Linux broker builds, and the actual CLI help check passed. These commit-preparation checks do not resolve the security-gate findings above.
