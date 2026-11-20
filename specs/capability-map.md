# Capability Map

This map groups every feature spec under `specs/` by the capability it describes today. A feature
spec describes one change. A capability is a job that a person or system does with the broker, and
it is the unit of a living-specification page under `docs/specification/`.

Use this map to find the specs to read before you write or change a capability page. Read
`.agents/skills/living-docs/SKILL.md` for the procedure.

## How to read this map

- **Primary** specs define the capability. **Also read** specs change part of it or depend on it.
- Read the specs of a capability in directory order. A later spec overrides an earlier one. The
  "Outdated parts" column lists the overrides that were found.
- **Page and last check** names the living-specification page and the result of `just living-docs`
  on 2026-11-17, without Docker, Helm, or a browser. "Not run" citations need one of those.
- This map was made on 2026-11-16 from the spec headers, user stories, requirement headings, and
  the test references in `tests/`. Scenario counts are approximate. A test mapping marked
  "inferred" comes from a file name or a `Describe` text, not from a spec reference in the test.

## Capabilities

### For administrators

| Capability | The job | Primary specs | Also read | Outdated parts | Page and last check |
|---|---|---|---|---|---|
| **Agent registration** | Make an agent known to the broker and keep its definition current | `006-domain-model-apis` (agents), `054-agent-external-id-filter`, `055-agent-external-id-upsert` | `036-canonical-resource-ids`, `011-agent-permission-requirements`, `028c-cimd-wildcard-client-uris` | `054` FR-011 (several matches) is replaced by `055`. `006` paths and its "all services for all agents" rule are replaced by `011`. | `docs/specification/agent-registration.md`: 12 passed, 3 not run (Docker) |
| **Third-party service registration** | Describe an external OAuth2 provider, its scopes, and the resources it protects | `006-domain-model-apis` (services), `018-oauth2-provider-flavors`, `034-provider-auth-params`, `035-protected-resource-subresources`, `050-oauth2-protected-resource-discovery` | `036-canonical-resource-ids`, `013-token-exchange` (protected resources) | `006` service paths. `035` changes full-service `PUT` for protected resources (needs `If-Match`). | `docs/specification/third-party-service-registration.md`: 38 passed |
| **Broker credentials at a third-party service** | Decide how the broker proves its identity to a provider: secret, public client with PKCE, hosted CIMD client, or mounted files | `042-thirdparty-public-pkce`, `046-cimd-upstream-client`, `051-oauth2-secret-file-overrides` | `050-oauth2-protected-resource-discovery` (DCR), `032-aggregated-jwks` (key-domain separation) | The `051` directory name is outdated: the behavior is an explicit credential source per service, not a secret override. | `docs/specification/broker-credentials-at-third-party-services.md`: 51 passed |
| **Permission sets** | Bundle provider scopes into named permissions that a user can understand | `019-permission-sets` | `011-agent-permission-requirements`, `036-canonical-resource-ids`, `design-permission-sets-tool-authorization.md` (background) | `019` removes the per-scope grant model of `006` and `007`. | `docs/specification/permission-sets.md`: 22 passed |
| **Resource identifiers** | Address agents, services, and permission sets by UUID or by a readable canonical ID | `036-canonical-resource-ids` | `054-agent-external-id-filter`, `055-agent-external-id-upsert` | None found | `docs/specification/resource-identifiers.md`: 19 passed |

### For end users

| Capability | The job | Primary specs | Also read | Outdated parts | Page and last check |
|---|---|---|---|---|---|
| **Consent and delegation** | Let an agent act for me, see what I allowed, and take it back | `006-domain-model-apis` (grants), `007-consent-frontend`, `011-agent-permission-requirements`, `019-permission-sets` (consent screen, grants), `022-revoke-agent-consent`, `031-unified-session-token` | `028-cimd-support` (client name and logo on the consent screen), `049-fix-refresh-consent`, `009-oauth2-auth-server` (consent gate) | `007` scope selection and edit mode are replaced by `011` and `019`. The `007` mount path `/consent` is replaced by the root-mounted SPA (`adrs/035-root-mounted-spa.md`). The raw `redirect_uri` on the consent URL is replaced by `031`. `022` writes `/consent/agent/`; the code uses `/consent/agents/`. | `docs/specification/consent-and-delegation.md`: 30 passed, 13 not run (browser) |
| **Third-party sessions** | Connect my account at a provider, keep it connected, and reconnect when the provider rejects it | `008-thirdparty-oauth2-sessions`, `029-proactive-token-refresh`, `053-session-auto-refresh` | `023-extproc-mcp-elicitation` (recovery URL), `034-provider-auth-params`, `042-thirdparty-public-pkce`, `adrs/046-observed-refresh-token-rejection.md` | Predicted session expiry in `008` is replaced by "requires reauthentication". The observability contract of `029` is replaced by `053`. | `docs/specification/third-party-sessions.md`: 37 passed, 8 not run (browser, Helm) |
| **Tool approvals** | Approve or deny one tool call of an agent, once or permanently | `024-approval-api-ui`, `026-extproc-approval-sync` | `020-extproc-opa-authorization` (`approval_required`), `design-permission-sets-tool-authorization.md` (background), `adrs/035-shared-tool-pattern-matching.md` | The delta sync wording in `024` is replaced by full snapshots in `026`. `020` FR-016 is replaced by `026` for `approval_required`. | `docs/specification/tool-approvals.md`: 46 passed, 6 not run (browser) |
| **End-user authentication** | Tell the broker who the user is, from a trusted proxy header or a JWT | `005-session-management`, `016-jwt-preauth` | `052-inline-jwks-trust-anchors`, `033-request-security-context` | `005` FR-009 (default header name) is outdated: the header name must be configured. The title of `005` says session management, but the spec covers principal extraction only. | `docs/specification/end-user-authentication.md`: 32 passed |

### For agents and clients

| Capability | The job | Primary specs | Also read | Outdated parts | Page and last check |
|---|---|---|---|---|---|
| **Authorization server modes** | Choose whether the broker forwards to an upstream server, issues its own tokens, or does both | `009-oauth2-auth-server`, `025-oauth2-server`, `030-hybrid-oauth-modes`, `032-aggregated-jwks` | `021-multi-agent-clientid` (proxy mode) | Mode names `delegate_upstream` (`009`) and `issue_token` (`025`) are replaced by `proxy`, `local`, and `hybrid` in `030`. `030` FR-012 and `025` User Story 6 Scenario 3 are replaced by `032`. The `client_id` rule of `009` is replaced by `021`. | `docs/specification/authorization-server-modes.md`: 37 passed |
| **Token issuance and refresh** | Get an access token, and a refresh token that stops working when consent ends | `025-oauth2-server` (grants, claims, signing keys), `033-offline-access-refresh`, `049-fix-refresh-consent` | `030-hybrid-oauth-modes` | None found | `docs/specification/token-issuance-and-refresh.md`: 35 passed |
| **Client registration and authentication** | Register how an agent identifies itself: issued credentials, a metadata document URL, redirect URIs, or a signed assertion | `025-oauth2-server` (credentials, redirect URIs), `028-cimd-support`, `028b-portless-registration`, `028c-cimd-wildcard-client-uris`, `051-private-key-jwt-auth`, `021-multi-agent-clientid` | `adrs/042-agent-allowlisted-private-use-redirect-uris.md`, `adrs/044-client-registration-independent-of-authentication.md` | `028` FR-000 (`issue_token` only) is replaced by `030`. `028` exact-port matching is replaced by `028b`. `028` FR-022 (public clients only) is replaced by `051-private-key-jwt-auth`. | `docs/specification/client-registration-and-authentication.md`: 44 passed, 1 not run (Docker) |
| **Token exchange** | A gateway swaps a user's token for that user's token at a third-party service | `013-token-exchange` | `019-permission-sets` (effective scopes), `023-extproc-mcp-elicitation` (`error_uri`), `029-proactive-token-refresh`, `026-extproc-approval-sync` (response fields), `052-inline-jwks-trust-anchors` | The session recovery target in `013` User Story 5 is replaced by `023` FR-001. | `docs/specification/token-exchange.md`: 31 passed |
| **Impersonation** | A privileged client obtains a broker token that represents a user or an agent | `037-oauth2-user-impersonation`, `054-agent-subject-impersonation` | `052-inline-jwks-trust-anchors`, `036-canonical-resource-ids` | `054-agent-subject-impersonation` tightens `037`: a signed `sub` equal to the target agent UUID is rejected on a human-only rule. | `docs/specification/impersonation.md`: 31 passed |
| **Trust anchors** | Supply the public keys that the broker trusts, from a URL, inline, or a file | `052-inline-jwks-trust-anchors` | `013-token-exchange`, `016-jwt-preauth`, `037-oauth2-user-impersonation`, `adrs/040-trust-anchor-local-key-sets.md` | None found | `docs/specification/trust-anchors.md`: 39 passed |

### For platform operators

| Capability | The job | Primary specs | Also read | Outdated parts | Page and last check |
|---|---|---|---|---|---|
| **Configuration** | Supply settings from files, environment, and flags, and fail early when they are wrong | `002-flexible-configuration` | `030-hybrid-oauth-modes`, `051-oauth2-secret-file-overrides` | None found | `docs/specification/configuration.md`: 16 passed |
| **Deployment** | Run the two servers in a container and on Kubernetes | `003-dual-port-server`, `010-docker-setup`, `014-helm-charts` | `053-session-auto-refresh` (CronJob) | The single image of `010` is replaced by separate runtime and migration images in `014`. | `docs/specification/deployment.md`: 7 passed, 6 not run (Helm) |
| **Storage and encryption at rest** | Choose a storage backend and keep stored provider tokens encrypted | `004-persistence-layer`, `012-aws-encryption-vault` | `055-agent-external-id-upsert` (migration runbook) | The configuration section of `012` is replaced by the shipped `encryption.aws_kms` and `encryption.memory` contract. | `docs/specification/storage-and-encryption.md`: 14 passed, 11 not run (Docker) |
| **Observability and audit** | Follow a request through traces, metrics, logs, and audit records | `017-opentelemetry-support`, `027-extproc-otel`, `033-request-security-context` | `053-session-auto-refresh` (metrics) | None found | `docs/specification/observability-and-audit.md`: 31 passed |
| **Gateway token exchange** | The gateway filter replaces the caller's token with the right third-party token on each request | `015-extproc-token-exchange`, `043-extproc-metadata-input`, `023-extproc-mcp-elicitation` | `013-token-exchange`, `adrs/012-extproc-in-memory-token-cache.md` | `015` FR-002 to FR-004, FR-009, and FR-013 (header input, pass-through) are replaced by `043`. `015` stays current for cache and circuit breaker. | `docs/specification/gateway-token-exchange.md`: 26 passed, 3 not run (Docker) |
| **Gateway policy authorization** | Allow or deny a request, such as one MCP tool call, with an OPA policy | `020-extproc-opa-authorization`, `044-extproc-opa-mcp-target-server` | `026-extproc-approval-sync`, `019-permission-sets` | `020` FR-016 is replaced by `026` for `approval_required`. `ciba_required` is still denied. | `docs/specification/gateway-policy-authorization.md`: 29 passed, 2 not run (Docker) |

### Not a capability

| Spec | Why |
|---|---|
| `001-end-user-docs` | Specifies the documentation set. The `docs/` layout has changed since. |
| `design-permission-sets-tool-authorization.md` | Design background for permission sets, gateway policy, and tool approvals. Its Tier 3 (CIBA) is not implemented. |

## Spec index

| Spec | Primary capability | Other capabilities |
|---|---|---|
| `001-end-user-docs` | Not a capability | |
| `002-flexible-configuration` | Configuration | |
| `003-dual-port-server` | Deployment | Configuration |
| `004-persistence-layer` | Storage and encryption at rest | |
| `005-session-management` | End-user authentication | |
| `006-domain-model-apis` | Agent registration | Third-party service registration, Consent and delegation |
| `007-consent-frontend` | Consent and delegation | |
| `008-thirdparty-oauth2-sessions` | Third-party sessions | Consent and delegation |
| `009-oauth2-auth-server` | Authorization server modes | Consent and delegation |
| `010-docker-setup` | Deployment | |
| `011-agent-permission-requirements` | Consent and delegation | Agent registration, Permission sets |
| `012-aws-encryption-vault` | Storage and encryption at rest | |
| `013-token-exchange` | Token exchange | Third-party service registration |
| `014-helm-charts` | Deployment | |
| `015-extproc-token-exchange` | Gateway token exchange | |
| `016-jwt-preauth` | End-user authentication | |
| `017-opentelemetry-support` | Observability and audit | |
| `018-oauth2-provider-flavors` | Third-party service registration | |
| `019-permission-sets` | Permission sets | Consent and delegation, Token exchange |
| `020-extproc-opa-authorization` | Gateway policy authorization | Tool approvals |
| `021-multi-agent-clientid` | Client registration and authentication | Authorization server modes |
| `022-revoke-agent-consent` | Consent and delegation | |
| `023-extproc-mcp-elicitation` | Gateway token exchange | Third-party sessions, Token exchange |
| `024-approval-api-ui` | Tool approvals | |
| `025-oauth2-server` | Token issuance and refresh | Authorization server modes, Client registration and authentication |
| `026-extproc-approval-sync` | Tool approvals | Gateway policy authorization, Token exchange |
| `027-extproc-otel` | Observability and audit | |
| `028-cimd-support` | Client registration and authentication | Consent and delegation |
| `028b-portless-registration` | Client registration and authentication | |
| `028c-cimd-wildcard-client-uris` | Client registration and authentication | Agent registration |
| `029-proactive-token-refresh` | Third-party sessions | Token exchange |
| `030-hybrid-oauth-modes` | Authorization server modes | Configuration |
| `031-unified-session-token` | Consent and delegation | |
| `032-aggregated-jwks` | Authorization server modes | Broker credentials at a third-party service |
| `033-offline-access-refresh` | Token issuance and refresh | |
| `033-request-security-context` | Observability and audit | End-user authentication |
| `034-provider-auth-params` | Third-party service registration | Third-party sessions |
| `035-protected-resource-subresources` | Third-party service registration | |
| `036-canonical-resource-ids` | Resource identifiers | Agent registration, Third-party service registration, Permission sets |
| `037-oauth2-user-impersonation` | Impersonation | |
| `042-thirdparty-public-pkce` | Broker credentials at a third-party service | Third-party sessions |
| `043-extproc-metadata-input` | Gateway token exchange | |
| `044-extproc-opa-mcp-target-server` | Gateway policy authorization | |
| `046-cimd-upstream-client` | Broker credentials at a third-party service | |
| `049-fix-refresh-consent` | Token issuance and refresh | Consent and delegation |
| `050-oauth2-protected-resource-discovery` | Third-party service registration | Broker credentials at a third-party service |
| `051-oauth2-secret-file-overrides` | Broker credentials at a third-party service | Configuration |
| `051-private-key-jwt-auth` | Client registration and authentication | |
| `052-inline-jwks-trust-anchors` | Trust anchors | Token exchange, Impersonation, End-user authentication |
| `053-session-auto-refresh` | Third-party sessions | Deployment, Observability and audit |
| `054-agent-external-id-filter` | Agent registration | |
| `054-agent-subject-impersonation` | Impersonation | |
| `055-agent-external-id-upsert` | Agent registration | Storage and encryption at rest |
| `design-permission-sets-tool-authorization.md` | Not a capability | |

## Findings

These points came up while the map was made. They are not fixed here.

- Almost every spec has the status "Draft", including specs that are implemented and tested. The
  status line does not show whether a spec is current. `054-agent-subject-impersonation` and
  `055-agent-external-id-upsert` say that approval is still required, but code and tests exist.
- Task lists are stale in places: `022-revoke-agent-consent` shows no completed task, and
  `019-permission-sets` shows about half, although both are implemented.
- Three prefixes are used twice: `033`, `051`, and `054`. Always use the full directory name.
- A GitHub provider flavor has tests (`tests/e2e/oauth2_provider_github_flavor_test.go`) but no
  spec text was found for it. `018-oauth2-provider-flavors` names `standard` and `google` only.
- No test references `006-domain-model-apis` or `010-docker-setup`. The tests for `001` to `009`
  do not name their specs, so their mapping is inferred.
- No test references `033-offline-access-refresh` by path. Its scenarios were found by the scope
  name `offline_access`.
- CIBA (`ciba_required`) is designed but not implemented. The gateway denies it.

### Found while the pages were written

Each page was checked against the tests. Where a spec, a document, and a test disagreed, the page
follows the test. These are the disagreements and gaps that deserve a decision.

Behavior that may be a defect:

- An administrator request with a JWT that only the end-user key set can verify gets `500
  server_misconfiguration` on `POST /api/sessions/sweep`, not a `401`. The tests assert only "not 200".
- Creating a `cimd` service with `private_key_jwt` while no usable CIMD key exists returns `500
  internal server error`. The tests accept any status from 400.
- The `400` for an unresolved service reference shows an internal function name
  (`GetThirdpartyOAuth2ProviderByCanonicalID: provider not found`).
- An unregistered `redirect_uri` gives `invalid_redirect_uri` for an agent addressed by UUID and
  `invalid_request` for a CIMD client.
- The PostgreSQL adapter tells the operator to run `identity-broker migrate up`. The broker binary
  has no `migrate` command.
- The gateway filter serves a cached token up to 5 seconds after its expiry during a broker outage.
  `015-extproc-token-exchange` FR-015 and `adrs/012-extproc-in-memory-token-cache.md` require removal at expiry.

Specs and documents that no longer match the broker:

- `025-oauth2-server` and the `ARCHITECTURE.md` mode table say the mode defaults to `proxy`. The
  broker refuses to start without a mode.
- `025-oauth2-server` says a new signing key signs at once. It activates 600 seconds later, or on
  an explicit promotion.
- `018-oauth2-provider-flavors` says the default flavor is `standard`. An omitted flavor becomes
  `github` for a GitHub token endpoint. The glossary lists only `standard` and `google`.
- `002-flexible-configuration` says flags, then YAML, then `.env`. The tests show flags, then
  environment variables, then YAML, then defaults. `examples/config/config.minimal.yaml` cannot
  load by itself.
- `api/enduser/openapi.yaml` says an empty `granted_permission_sets` revokes a grant with `204`.
  The broker answers `400`; revocation is `DELETE`.
- `004-persistence-layer` FR-013 says the broker verifies that the schema is current. It checks
  only that the migrations table exists.
- `020-extproc-opa-authorization` lists a `default_decision` setting. An undefined policy result is
  denied even when the setting is `allow`.
- `coding-standards/LOGGING.md` says audit records use the key `event`. Grant audit records use
  `action`. Three session metric names end in `_total` or a unit, which
  `coding-standards/OPENTELEMETRY.md` forbids.
- `adrs/029-token-exchange-client-assertion-trust-anchor.md` and
  `adrs/033-request-security-context-propagation.md` have the status Proposed, but their behavior
  is implemented and tested.

Tests that prove less than their names say:

- Several tests in `tests/integration/oauth2_sessions_api_test.go` have empty bodies and pass.
- `agent_permission_requirements_test.go` US1 scenarios 6 and 7 stop at an earlier validation.
- `token_exchange_test.go` "grant revoked" seeds an expired grant. Its US2-S1 is a placeholder.
- The encryption scenarios named for KMS, key rotation, and KEK logging do one encrypt and decrypt
  round trip with a raw key.
- The ExtProc scenario "fresh exchange when cached token is expired" sends a zero lifetime, which
  is never cached.
- `TestOAuth2AuthServerConfig_Validate_EmptyModeDefaultsToProxy` asserts the opposite of its name.

Behavior with no acceptance scenario (Principle XIII gaps):

- Registering CIMD client URI patterns through the admin API.
- A token-exchange policy that denies a gateway, and the insufficient-scope denial.
- Decoded claims (`sub`, `agent_id`, `iss`) of a locally issued token.
- A grant saved with an end date through the API, and provider sessions that stay after revocation.
- The Terminate dialog on the sessions page, and the confirm step of revocation in the browser.
- Batch requests and the audit log in gateway policy authorization: unit tests only.
- The gateway circuit breaker, expiry margin, and outage fallback: unit tests only.
- The core of the Helm chart (Service, Ingresses, storage variants, migration Job) and the
  container images.
- Child spans, broker request metrics, and audit records other than grants.

