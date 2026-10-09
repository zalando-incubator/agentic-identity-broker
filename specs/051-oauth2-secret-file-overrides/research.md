# Phase 0 Research: OAuth2 Client-Secret File Overrides

**Propagated**: 2026-10-07 — Updated from spec.md refinement

**Updated**: 2026-10-08 — Aligned with FR-009 and FR-025. Defined coherent pair acquisition, immutable publication, and irreversible source-transition evidence.

## Scope and research questions

The refined specification selects credentials through the registered service's explicit `credential_source`, not binding presence. Filesystem bindings use exact canonical IDs, not UUIDs.

~~No provider SDK, database migration, or public API change is necessary.~~ Superseded because explicit source selection changes registration, administrative representation, and persistence.

Research covers source-specific registration, credential pairs, three operation boundaries, client-identity association, configuration decoding, file safety, errors, events, tests, and deployment generation. No provider SDK is necessary.

**Evidence status:** Feature behavior is implemented but unreleased under accepted ADR 038. Historical baseline citations, Viper probes, approvals, and semantic-red runs remain evidence of the design and test-first sequence. They are not current source snapshots. Current ownership resides in the third-party model/domain, session credential selection, reader port/adapter, loader decoder, both stores, and migration 036. Completed checks and delivery limits appear in the [quickstart](./quickstart.md#status-and-prerequisites) and [deployment contract](./contracts/deployment.md#deployment-repository-inputs-and-outputs). The full final gate remains pending.

## Decision 1: Keep selection inside the existing session domain

**Decision:** `OAuth2SessionService` in `internal/domain/oauth2session/` shares source selection across authorization initiation, code exchange, and refresh. `internal/app/builder.go` injects one credential-reader port.

~~A matching binding selects a file secret for an eligible service.~~ Superseded because the registered source alone selects stored or filesystem credentials.

An eligible service is neither public nor CIMD confidential and does not use the Google flavor. Standard, zero/default-standard, and GitHub confidential services remain eligible. Stored mode ignores bindings. Filesystem mode requires a canonical ID and its exact file-pair binding. A missing binding, including an empty mapping, never selects stored mode.

`model.CredentialSource` defines `stored` and `filesystem` values. The entity persists and exposes `CredentialSource` as `credential_source`. Creation omission selects stored. Update omission preserves the current source. Unknown values and explicit null are invalid. Null is not omission.

Filesystem registration requires a canonical ID and rejects either inline credential field, including empty or null input. Stored confidential validation remains unchanged. Excluded modes reject filesystem selection without file access. Filesystem services use `model.NewAbsentSecret()`, not the uninitialized `Secret` zero value. Source transitions remove obsolete stored credentials atomically or require explicit inline credentials. [data-model.md](./data-model.md) and the [outbound contract](./contracts/outbound-authentication.md) define these invariants.

Internal `CredentialSourceTransitioned bool` persists as `credential_source_transitioned`. Creation and legacy backfill set it to false. An actual source change atomically sets it to true with credential removal or replacement. No-op selections and metadata updates preserve it. A later source change never resets it.

**Rationale:** The existing entity is `model.ThirdpartyOAuth2ProviderEntity`, despite the specification's service terminology. It already contains `ID`, `CanonicalID`, `Secret`, `Flavor`, and authentication predicates. Both outbound token operations belong to the session domain.

**Alternatives considered:**

- Override `ThirdpartyOAuth2ProviderService.Get()`: rejected because administration and metadata also use that service.
- ~~Add a service field, repository, or reconciler: rejected because a binding is deployment configuration and must not persist.~~ Superseded for the service field. The service must persist its source. Paths remain configuration, and no new repository or reconciler is necessary.
- Add a credential-provider framework or separate domain package: rejected because this feature has one filesystem source and one existing consumer.

**Historical baseline evidence:** `internal/domain/model/thirdparty_oauth2_provider.go:116-153` defined the entity and authentication predicates before the source field. `internal/domain/oauth2session/service.go:312-347,669-775,794-935` and `internal/app/builder.go:617-628` established the session-domain and Builder boundaries. Current source selection resides in `internal/domain/oauth2session/credentials.go`.

Accepted [ADR 012](../../adrs/012-encryption-layer-separation.md) preserves domain-owned encryption. [ADR 036](../../adrs/036-public-client-token-endpoint-auth.md) defines secretless requests, PKCE, and confidential auth-style negotiation. [ADR 037](../../adrs/037-cimd-client-authentication-key-domain.md) separates assertion keys from token-signing keys. The user accepted [ADR 038](../../adrs/038-oauth2-client-secret-file-overrides.md) on 2026-10-08. It supersedes only ADR 036's confidential stored-secret invariant for selected eligible filesystem services. Public and CIMD protocol behavior stays unchanged.

## Decision 2: Read only at outbound authentication boundaries

~~Keep file access out of `buildOAuth2Config()` and authorization initiation. Resolve only the file secret before exchange and refresh.~~ Superseded because filesystem authorization initiation needs the current client ID.

**Decision:** Resolve only the client-ID file for filesystem authorization initiation. Never read the secret to construct an authorization redirect. Administration, consent, and session metadata remain file-independent.

Resolve both current files before each broker-owned `Config.Exchange()` attempt and each `RefreshAccessToken()` provider request. Code-exchange retries obtain a fresh pair. Existing OAuth2 auth-style negotiation within one library `Exchange()` call uses that attempt's pair. Preserve negotiation, retry policy, refresh locking, and singleflight.

For stored and excluded services, preserve existing credential behavior without file reads. Never assign file values to the service entity. Do not retain operation credentials, their configuration object, or descriptors between operations. Only the non-secret identity association can persist.

`OAuth2StateTokenClaims.UpstreamClientID *id.ClientID` uses sealed JWE JSON `upstream_client_id,omitempty`. New eligible authorizations, including stored mode, capture their effective client ID. Successful callback exchange copies the verified initiating identity to `storage.UserSession.UpstreamClientID *id.ClientID`. Both stores preserve this nullable association. PostgreSQL uses `upstream_client_id TEXT`. Session JSON uses `json:"-"`, and public session DTOs remain unchanged.

Filesystem callback and refresh require a non-empty association equal to the current effective client ID. Missing identity produces `identity_missing`. A different identity produces `identity_mismatch`. Both stop before provider authentication and require a new connection. Never infer a legacy identity from current files or service credentials.

A recorded eligible identity remains binding across both source transition directions. Compare it with the effective current client ID before callback exchange or refresh, including stored mode. Legacy missing identities remain usable only for stored services with `credential_source_transitioned: false`. Filesystem mode or a prior source transition requires a new connection, including after a return to stored. Excluded flows remain unchanged. After client ID A changes to B, new authorization uses B. A's pending codes and sessions must not authenticate as B.

**Rationale:** Baseline initiation constructed an OAuth2 configuration, and refresh used a separate manual form-post path. The refined source requires operation-specific resolution at all three boundaries. Identity association prevents code exchange or refresh under another client ID. Token encryption and its service-ID AAD stay unchanged.

**Alternatives considered:**

- Read once at startup or cache by service: rejected because rotation and source loss must affect the next operation.
- ~~Read during authorization initiation: rejected by FR-018.~~ Superseded by refined FR-005 and FR-018. Initiation reads the client ID only.
- Change authentication style, retry policy, or refresh concurrency: rejected because those controls are outside scope.

**Historical baseline evidence:** `internal/domain/oauth2session/service.go:398-464,494-568,794-876,1267-1344,1387-1419,1490-1516` and the RFC 8693 refresh path in `internal/domain/tokenexchange/service.go:331`. State/session types lacked the identity field during that review. Their current owners are `internal/domain/oauth2session/state_token.go` and `internal/domain/storage/user_session.go`. Existing JWE/session and encryption boundaries remain unchanged.

## Decision 3: Add one bounded filesystem adapter

~~Define `ClientSecretFileReader` in `internal/adapters/clientsecretfile/`.~~ Superseded because one reader must support either credential value, not only the secret.

**Decision:** `ports.CredentialFileReader` in `internal/ports/credential_file.go` defines two operations:

```go
ReadClientID(path string) (string, error)
ReadPair(clientIDPath, clientSecretPath string) (clientID, clientSecret string, err error)
```

`internal/adapters/credentialfile/reader.go` implements the port. Only validated operator configuration supplies paths. `ReadClientID` never accesses a secret path. `ReadPair` returns neither value on error.

The adapter follows symlinks when it opens the path anew. On Linux and Darwin, use a non-blocking read-only open. Inspect the opened descriptor with `Stat()` before reading. Reject non-regular targets and targets larger than 65,536 bytes. Bound the subsequent read to 65,537 bytes to detect growth after the size check. Close the descriptor on every exit.

Use the standard library's `strings.TrimSpace` to remove surrounding whitespace. Reject an empty result. Preserve internal characters. A file of exactly 65,536 bytes remains permitted before trimming.

`ReadPair` opens both targets before either bounded read. Both descriptors remain open while the adapter revalidates their configured paths with `os.SameFile`. A changed target returns `generation_changed` before provider authentication, without acquisition retries. An unavailable target returns its applicable safe source reason. Both descriptors close before return on every path.

This validation depends on the immutable-target publication contract in FR-009. Providers publish a complete pair through fresh immutable targets. They never modify published targets or republish retired targets during acquisition. Under that contract, successful target validation establishes a coherent pair without a provider-specific generation identifier.

Publication after validation can leave an in-flight operation using its validated pair. Later operations acquire current targets. Provider validity and established-identity checks still apply.

**Rationale:** A normal blocking open can wait on a FIFO before a descriptor type check. A path-only `Stat()` followed by a blocking open leaves a replacement race. Descriptor inspection and a bounded read protect the actual opened source. Normal projected-volume symlinks remain supported without retaining their resolved target.

**Alternatives considered:**

- `os.ReadFile()` alone: rejected because it neither prevents FIFO blocking nor bounds allocation.
- Resolve and retain a symlink target: rejected because projected-volume rotation replaces that target.
- Ban symlinks: rejected because Kubernetes projected credentials rely on them.
- Watches or retained descriptors: rejected by FR-009 and the scope boundaries.

**Dependencies:** Use Go `os`, `io`, and `strings`, plus the existing `golang.org/x/sys/unix` dependency for Unix flags. No new credential or cryptographic library is necessary.

## Decision 4: Decode the mapping within the existing configuration loader

~~Add `ClientSecretFiles map[string]string` for secret-only bindings.~~ Superseded because filesystem mode requires both credentials.

**Decision:** `ports.ThirdPartyOAuth2Config.CredentialFiles` maps canonical IDs to `ports.CredentialFileBinding`. Bindings contain `ClientIDFile` and `ClientSecretFile`, with YAML/JSON fields `client_id_file` and `client_secret_file`. The existing loader owns decoding through `internal/config/credentialdecoder.go`.

Use `third_party_oauth2.credential_files`, `IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES`, and `--third_party_oauth2.credential_files`. Do not retain old-name aliases.

Resolve the whole mapping in this order: changed Cobra flag, configured environment variable, YAML configuration field, default empty mapping. `.env` loading stays in the existing loader. A supplied empty string is invalid JSON. `{}` explicitly removes the winning bindings, but never changes a filesystem service to stored.

Exclude this field from generic case-folding map decoding. Read the original YAML mapping with the existing YAML library when YAML supplies the winning value. Preserve literal canonical-ID keys, including case and dots. Reuse the configuration file selected by the loader, not a guessed or separately selected path. Reuse the loader's path-value expansion behavior for YAML values where applicable.

For environment and CLI input, decode a JSON object strictly. Reject malformed input, `null`, arrays, scalars, non-object bindings, and missing pair fields. Reject every non-string path, including null, plus empty, whitespace-only, or relative paths. Do not echo raw input. Validate keys with `canonical.Validate()`. Do not access files or query registered services during validation.

**Rationale:** Earlier research exercised installed Viper v1.21.0 with a throwaway program. These historical results concern the original simple mapping, not the implemented pair decoder:

| Probe | Observed result |
|---|---|
| YAML key `GitHub-Prod` | Decoded as `github-prod` |
| YAML key `com.example.svc` | Remained a literal dotted key |
| Environment value containing a JSON object string | Failed generic map decoding without explicit JSON handling |
| Higher-precedence map `{b: /b}` over YAML `{a: /a}` | Replaced the mapping with `{b: /b}` |
| Higher-precedence empty map over non-empty YAML | Replaced the mapping with an empty map |
| YAML numeric path `123` | Weakly converted to a string |

Whole-map replacement matches CR-001. Case folding and weak scalar conversion violate the binding contract. The pair decoder must preserve exact keys and reject invalid field types. The historical probes do not prove that decoder's implementation.

**Alternatives considered:**

- Use Viper's generic `map[string]string` decoding without changes: rejected because it loses YAML key case and rejects JSON-string sources.
- Lowercase canonical IDs: rejected because canonical IDs are case-sensitive.
- Add a general JSON-map decode hook for all configuration: rejected because it changes unrelated configuration contracts.

**Historical baseline evidence:** `internal/config/loader.go:48-88,132-178,327-449,455-498,628-724`, `internal/config/validator.go:330-395`, `internal/domain/canonical/id.go:10-22`, and `cmd/agentic-identity-broker/root.go:435-452`. Current `internal/config/credentialdecoder.go` imports `go.yaml.in/yaml/v3`, which is a direct dependency.

## Decision 5: Classify source errors without changing HTTP contracts

**Decision:** `internal/domain/model/credential_source_error.go` defines `model.ErrCredentialSourceUnavailable` and `CredentialSourceError`. The closed reasons are `missing_binding`, `identity_missing`, `identity_mismatch`, `generation_changed`, `not_found`, `permission_denied`, `not_regular`, `too_large`, `empty`, and `read_failed`.

The adapter translates OS errors into safe file reasons. Selection and identity validation supply the other reasons. Printable errors expose no file contents, paths, provider payload, or raw OS error chain.

Source errors bypass `ErrRefreshFailed` wrapping in `refreshSessionTokens()` and misleading provider/PKCE classification during exchange. Preserve callback's generic `callback_failed` redirect, session refresh's `500 internal_error`, and token exchange's `500 server_error` without an `error_uri`. Initiation uses its existing generic `internal_error` response without file detail.

Source failures use typed diagnostic detail `credential_source_unavailable` with outcome `infrastructure_error` and no recovery URI. Provider rejection retains `invalid_grant` and `RefreshRejectedError`. Provider `invalid_grant` uses detail `refresh_rejected`, distinct from locally recorded `refresh_token_expired`. Source failures occur before a provider request.

**Rationale:** The baseline refresh wrapper classified all provider-access errors as upstream failures. A file error precedes an upstream request and must not use that category. The implemented path preserves the distinct source-error category.

**Alternatives considered:**

- Wrap source errors with `ErrRefreshFailed`: rejected because it produces the wrong session HTTP response.
- ~~Add an API error code or endpoint: rejected because the OpenAPI contracts do not change.~~ The operational error-code and endpoint rejection remains valid. The no-OpenAPI-change premise is superseded by required administrative source selection and representation.
- Fall back to stored or previous file credentials: rejected by FR-006 and SR-001.

**Evidence:** `internal/domain/oauth2session/errors.go:36-79`, `internal/domain/oauth2session/service.go:361-376,940-958,1408-1419`, `internal/adapters/http/oauth2_sessions/handler.go:345-354,598-638`, and `internal/domain/tokenexchange/service.go:331-380`. `api/enduser/openapi.yaml` already documents the session `500` response and token-exchange `server_error`.

## Decision 6: Emit bounded, credential-free events

**Decision:** Use the injected context-aware `slog.Logger`. Fix the event names as `session.oauth2.credential_files_used`, `session.oauth2.credential_source_failed`, and `session.oauth2.credential_provider_rejected`.

Each event includes `event`, UUID `service_id`, `operation`, and `outcome`. Failure events also include a closed non-secret `reason`. Operations are `authorization_initiation`, `code_exchange`, and `refresh`. Outcomes are `used`, `failed`, and `rejected`.

Emit `credential_files_used` only after every required read and identity validation succeeds. Initiation requires only the client ID. Exchange and refresh require both values. This event records selection, not provider acceptance.

Emit `credential_source_failed` for missing bindings, unusable required files, or invalid identity associations at the affected boundary. Emit `credential_provider_rejected` only after a usable pair reaches the provider during exchange or refresh. Its reason is `provider_rejected`. Transport and decoding errors retain their existing categories.

Never log file values, paths, provider-controlled descriptions, raw file errors, or credential-bearing requests. Configuration diagnostics must protect the renamed mapping.

**Rationale:** Existing session events use `event`, `service_id`, `reason`, and context-aware logging. Token exchange uses bounded `token_exchange.failure_stage`, `token_exchange.failure_detail`, and `token_exchange.outcome` attributes in logs and spans. The redactor recognizes the environment prefix and `CREDENTIAL`, as well as `SECRET`. Feature disclosure journeys cover safe handling of the renamed mapping.

**Alternatives considered:** A new audit sink, metric family, or credential diagnostic endpoint adds no required capability and is not necessary.

**Evidence:** `internal/domain/oauth2session/service.go:170-171,720-727,1410-1417,1436-1443`, `internal/adapters/http/enduser/oauth2_token.go:211-241`, and `internal/config/redactor.go:17-43`.

## Decision 7: Extend the chart without adding mount inputs

~~Add `broker.thirdPartyOauth2.clientSecretFiles` and render secret-only paths as `third_party_oauth2.client_secret_files`.~~ Superseded because the chart must expose both paths.

**Decision:** Add `broker.thirdPartyOauth2.credentialFiles`, default `{}`, to chart values and schema. Render non-empty pair objects under `third_party_oauth2.credential_files`. Preserve canonical-ID spelling. Defaults render no binding. The chart never selects a registered service's source.

Use the existing `broker.extraVolumes` and `broker.extraVolumeMounts`. Prefer a read-only directory mount without a credential-file `subPath`. Confirm that UID/GID 1000 can read the files. Do not introduce a chart-specific credential-discovery mechanism.

**Rationale:** The chart already mounts configuration and `/tmp`, accepts supplemental mounts, and runs as non-root. Its third-party OAuth2 values object forbids unknown properties, so schema, values, template, and README must change together.

**Alternatives considered:** New mount fields or duplicate configuration through `broker.extraConfig` are unnecessary. A generated-manifest-only change would not survive regeneration.

**Evidence:** `charts/agentic-identity-broker/values.yaml:58-64,333-335,406-418`, `values.schema.json:602-625`, `templates/configmap.yaml:77-79`, `templates/deployment.yaml:108-126`, and `README.md:110-123,151-152`.

## Decision 8: Deliver the explicit binding in the deployment repository

~~Set `zalando-platform: /meta/credentials/employee-client-secret` as a secret-only binding.~~ Superseded because filesystem mode supplies the client ID and secret.

**Decision:** Update `deploy/values/{test,prod,sandbox}.yaml` in the deployment repository with one `zalando-platform` pair. Its paths are `/meta/credentials/employee-client-id` and `/meta/credentials/employee-client-secret`. Explicitly select filesystem mode on the registered service without inline credentials. Update the deployment README and regenerate manifests with `scripts/update-k8s-manifests.sh` against feature-capable chart source.

The generator accepts `CHART_REPO_DIR` and records the chart's latest path commit in `scripts/chart_ref`. That output is neither a checkout pin nor a snapshot of uncommitted chart changes. Select a feature-capable broker image as well as feature-capable chart source.

**Rationale:** Prior deployment research found the existing read-only `agentic-platform-credentials` mount at `/meta/credentials` in all three environment inputs. The refined spec supplies both mounted filenames. The checked-in PlatformCredentialsSet names the `employee` client, but does not itself specify controller-generated filenames.

~~A stable client ID is a deployment prerequisite, and client-ID changes are outside scope.~~ Superseded because new authorizations can use a changed file client ID. Existing codes and sessions cannot migrate to that identity. Complete consistent pair publication and old/new provider validity overlap remain operator responsibilities, not verified continuity guarantees.

**Alternatives considered:** Filename inference, direct PlatformCredentialsSet integration, automatic identity migration, and guaranteed uninterrupted rotation remain outside scope.

**Historical evidence:** Earlier observations cited `deploy/values/test.yaml:42-50,144-145`, corresponding prod/sandbox inputs, and `scripts/update-k8s-manifests.sh:16-61`. They do not establish the current mount state or provider timing.

**Historical prerequisite review, 2026-10-08:** The three source inputs then omitted `broker.extraVolumes`, `broker.extraVolumeMounts`, and `credentialFiles`. T066 supplied the pair mapping and required read-only Secret directory mount through existing chart inputs. The existing script regenerated test, prod, and sandbox manifests from the current local chart. Validation covered the exact pair and credential/configuration/tmp mounts. `scripts/chart_ref` records historical chart path revision `ad2289121b459ea14d56efd7d227bbf71d452948`. The working chart includes uncommitted feature changes, so that revision alone does not reproduce the generated outputs.

CDP `DEP_BROKER_VERSION` placeholders remain. The local image was verified only. No registry publication or live deployment occurred.

## Decision 9: Preserve repository release-note conventions

**Decision:** Keep generated GitHub release notes and the implementation PR description. Add the required breaking-change record in [`docs/changelog.md`](../../docs/changelog.md). The approved Admin API `2.0.0` replaces `1.0.0` on existing `/api/services` routes without a compatibility endpoint or shim. Source-aware response schemas make filesystem omission explicit. Release notes must identify source selection, pair configuration, consumer migration, and fail-closed behavior.

~~Do not recreate an intentionally removed manual changelog.~~ Superseded as an absolute prohibition. Historical repository conventions cannot override required breaking-change documentation.

**Rationale:** Earlier research found that commit `746c8f4f` removed the manual changelog. The release workflow uses `gh release create --generate-notes`. Those conventions do not replace the required breaking-contract record for the released required-client-ID schema.

**Alternatives considered:** A new endpoint was permitted by FR-024 but not selected. The user approved a major contract bump on existing routes. Keeping `1.0.0`, inventing a filesystem client-ID placeholder, or refusing required breaking-change documentation would violate the approved contract.

**Evidence:** Commit `746c8f4f` and `.github/workflows/release.yml:306-338`.

**Release evidence, 2026-10-08:** GitHub reports published, non-prerelease `v0.1.81`, dated 2026-10-07. Its administrative OpenAPI declares version `1.0.0` and requires `Service.client_id`. This is a released contract, not an unreleased additive change. Sources: [release v0.1.81](https://github.com/zalando-incubator/agentic-identity-broker/releases/tag/v0.1.81) and `api/admin/openapi.yaml` at that tag.

**Approval record, 2026-10-08:** The user explicitly selected “Accept ADR 038” and “Major contract bump to 2.0.0”. ADR 038 is Accepted. T010 defines concrete OpenAPI 3.0 source variants and the breaking-change record before implementation. The contract version is separate from the broker release tag. Approval does not establish delivered runtime behavior.

## Decision 10: Exercise complete journeys with production bootstrap

~~Add 40 Ginkgo acceptance scenarios.~~ Superseded because the refined specification defines 50 numbered journeys.

**Decision:** Add all 50 Ginkgo acceptance scenarios under `tests/e2e/`, with one `It()` for each numbered specification scenario. Use dual-server production bootstrap, fresh storage, temporary credential pairs, and isolated provider doubles. Plan.md contains the complete US1-S1 through US7-S3 mapping.

Cover source-aware create/read/update, mixed-field presence rejection, explicit transitions, migration/backfill, and both storage adapters. Compare authorization redirects and provider credentials with the sealed/session identity. Exercise client ID A to B, missing legacy identity after a source transition, and recorded identity preservation across both source directions.

Reuse `MockUpstreamOAuth2Server.GetTokenRequests()` to observe HTTP Basic and form credentials. Reuse its error-response, refresh-token, and expiry controls. Use `NewBufferedJSONLogger()` for structured-event assertions. Use existing public-client and CIMD journey patterns. Add a real Google-flavor fixture that explicitly sets the flavor and routes its outbound journey to a local provider.

Configuration-source scenarios must run the real loader before application bootstrap. Builder-only configuration does not exercise startup validation. Use a production-binary wrapper for actual CLI startup, non-root readability, and permission-denied behavior. Build the binary once, outside per-scenario setup, through the established command runner.

For chart scenarios, render the real chart with portable deployment-input fixtures. Parse the resulting ConfigMap configuration and Deployment mounts, then exercise the loaded broker. Validate the actual external deployment generator separately against all three environment inputs. Do not make the default suite depend on a developer-specific external checkout.

**Rationale:** Existing broker E2E tests pass a configuration struct to `NewServerFactory()`. The loader owns validation, so Builder-only tests can miss invalid startup input. Existing chart integration tests provide a `helm template` precedent, not complete user-journey coverage. New registration, identity, and persistence assertions need semantic-red evidence before behavior changes. Stored and excluded compatibility controls must not manufacture failures.

**Boundary cases:** Permission-denied scenarios must use a non-root process. A root-run suite must drop the child process's privileges before authentication. Use observable readiness and request deadlines, not fixed sleeps or skipped scenarios. Provider request counts include the library's authentication-style probes, so fail-closed assertions compare a before/after request delta of zero.

**Alternatives considered:** Direct domain calls, config-struct-only source tests, root-only `chmod` tests, schema-only chart assertions, and file-derived legacy identity do not prove the required journeys.

**Evidence:** `tests/e2e/bootstrap/server_factory.go:91-121`, `bootstrap/logger.go:26-80`, `helpers/mock_upstream.go:265-277,405-414`, `thirdparty_public_client_test.go`, `thirdparty_cimd_authentication_test.go`, `token_exchange_test.go:242-275,1047-1080`, `tests/integration/config_test.go:180-275`, and `tests/integration/helm_chart_test.go:12-44`.

## Research completion

The refined spec resolves source-selection and credential-pair requirements. Accepted design identifiers and storage invariants are implemented. Provider validity overlap remains outside the continuity guarantee.

~~Planning gates pass because public APIs, storage, and authentication modes do not change.~~ Superseded because administrative contracts and persistence must change. Protocol and encryption boundaries remain binding.

ADR 038 and Admin API `2.0.0` are approved as recorded in Decision 9. `api/admin/openapi.yaml`, handlers, and repository consumers implement the source-aware contract. `docs/changelog.md` records the unreleased breaking change. ADR 038 narrowly supersedes the stored-secret invariant for selected filesystem services. Public/CIMD protocol requirements and CIMD key-domain separation remain unchanged. External administrative consumers must migrate before deployment of the cutover.

Existing migration 036 and both adapters persist source-specific nullable credentials, irreversible `credential_source_transitioned`, and nullable session `upstream_client_id`. Source-aware constraints preserve migration 035's valid stored, public, and CIMD branches. Backfill preserves authentication behavior and ciphertext. It neither infers legacy identity nor changes encryption AAD.

Guarded DOWN rejects filesystem records and every record with source-transition evidence. Removing that evidence can revive legacy missing-identity contexts, including after return to stored. Rollback neither coerces nor deletes records. Compatible DOWN discards the non-secret session identity association, as documented in the data model and changelog. These safeguards exist in migration 036.

All 50 acceptance journeys passed in individual story runs after historical semantic-red evidence. Selected unit/integration packages and 30 legacy public/CIMD journeys passed. Non-root/chart/deployment evidence includes a verified local linux/arm64 image and regenerated external manifests. The full final gate remains pending. No published feature release or live deployment is claimed.
