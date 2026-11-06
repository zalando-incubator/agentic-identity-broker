# Quickstart: Validate Incremental Service Authorization

**Feature**: [spec.md](./spec.md) | **Contracts**: [contracts/](./contracts/) | **Data model**: [data-model.md](./data-model.md)

This guide proves the feature end to end after implementation. It references contracts instead of
repeating them. All values are synthetic and follow the spec's shared cast.

## Implementation checkpoint — 2026-10-29

**Blocked; setup/design-precondition progress only. No feature behavior, migration or new acceptance test is implemented.**

### Supplied approval

All 11 items in the immutable plan match the supplied approval by **mjungsbluth** on
**2026-10-29**, [approval comment](https://github.com/zalando-incubator/agentic-identity-broker/pull/323#issuecomment-6509178189).
ADRs [045](../../adrs/045-service-session-recovery-return.md) and
[046](../../adrs/046-virtual-service-scopes.md) now record that approval in `adrs/`.
Their feature-local paths remain reference-only pointers. Approval is not evidence that the
feature is implemented. The original spec and plan are intentionally unchanged.

### Tooling and validation blocker

The installed executables report Go **1.27.3**, Node **24.22.1**, npm **11.19.1** and
just **1.58.0**. Migration number **046** is available after existing migration 045.
`ginkgo`, `golangci-lint` and `wireloom` are not on PATH. Tool versions have not changed in
this feature; no tool installation or permission workaround was attempted.

| Command | Actual outcome |
|---|---|
| `just check` | Exit 0. Formatting/vet pass; the recipe reports missing `golangci-lint` and falls back to vet. This is not a full lint run. |
| `just test` | Exit 0 on the existing application; not evidence of Feature 055 coverage. |
| `just test-integration` | Exit 0 on the existing self-contained suites; does not validate the new examples through dedicated Feature 055 assertions or replace real PostgreSQL tests. |
| `just test-e2e-backend` | Exit 1 after successful frontend build: `ginkgo is not installed`. No acceptance spec executed. |
| `just test-e2e-frontend` | Exit 127 after successful frontend build: `ginkgo: command not found`. No browser journey or capture executed. |
| `just test-integration-infra` | The attempt timed out after 30 seconds; no completed migration/repository validation. Container-backed validation is not available in this sandbox (Docker-in-Docker is unsupported). |
| `just verify` | Timed out after 90 seconds during security-tool dependency setup; the full gate did not complete. |

**Required maintainer action**: make the workflow's existing tool setup expose its pinned
Ginkgo, lint and Wireloom executables to the agent, and provide a supported runner for the
required real PostgreSQL/container suites. No failed/missing gate is waived. Once the harness
is available, finish the acceptance fixtures/tests and observe all 19 semantic-red scenarios
(T011–T018) before implementing foundations or user stories. Existing baseline tests passing
cannot substitute for that checkpoint.

### Remaining reanalysis contract boundaries

The supplied approval covers the 11 published items, not an unlisted exception or contract
revision that resolves the remaining findings. No unapproved behavior is implemented.

- **I2 (proxy-refresh validation)**: FR-002 requires rejecting malformed, unknown and ineligible
  broker scopes, but the approved proxy-refresh text only specifies stripping and preserving
  upstream outcomes. Recommended **Proposed API clarification**: validate syntax, resolve the
  request's agent through the existing client-resolution path, and check service existence and
  eligibility before forwarding; use `400 invalid_scope` with `requested service scope is not
  permitted` for the three invalid-scope categories and `500 server_error` for lookup failure.
  Ordinary-scope-only requests retain their existing path. Do not check provider usability or
  veto any upstream response. Rationale: satisfy FR-002 without losing rotated credentials.
  Main alternative: explicitly approve a proxy-refresh exception to FR-002. The newly specified
  pre-forward response contract requires explicit approval before code; T099 tracks it.
- **U4 (dot-segment IDs)**: existing canonical validation permits the complete identifiers `.`
  and `..`, while the approved emission/recovery contract always prefers an existing canonical
  ID. Browser path normalization prevents those URLs from reliably naming one service.
  Recommended **Proposed API/ADR revision**: emit UUID scope and recovery/page URLs only for
  those two complete identifiers, retain both existing canonical records and raw-scope parsing,
  and preserve exact granted strings/bindings and UUID rename durability. Do not tighten the
  global canonical-ID validator. Rationale: browser-safe recovery without invalidating existing
  records. Main alternative: explicitly approve a canonical-ID migration/restriction, which is
  broader and backward-incompatible. This fallback is not covered by the approved always-prefer-
  canonical text. T100 requires approval and regression coverage before implementation.

**Required maintainer action**: approve the proposed pre-forward API clarification and the
UUID-only emission/recovery exception for `.` and `..` (or supply a compatible approved
alternative). The immutable spec/plan must be reconciled in an authorized pre-implementation
run before those revisions are implemented. The existing approved ADR text is not silently
changed to imply that either revision was approved.

### Deferred work and resumption

| Tasks | Why unfinished / required next work |
|---|---|
| T003, T008, T099–T100 | Reconcile the two contract boundaries above under explicit approval, then finish the model review and complete OpenAPI 1.2.0 before behavior changes. |
| T011–T018 | Fixtures, 19 acceptance tests and semantic-red evidence remain unwritten/unexecuted. Expose Ginkgo, finish these tests and run the actual backend/browser suites. |
| T019–T038 | Foundations depend on T018. Keep migration SQL behind failing real-PostgreSQL tests; complete scope, usability, identity binding, both adapters and issuance in test-first pairs. |
| T039–T079 | All user-story tests, implementation and integrated documentation remain; they depend on the acceptance checkpoint and foundations. Continue the approved TDD sequence, including both reanalysis regression sets. |
| T080–T098 | Final compliance, complete lint, real database suites, all E2E journeys, four screenshots and `just verify` remain. Supply the supported tooling/runtime and rerun every required gate after implementation; do not treat baseline passes as feature completion. |

## Prerequisites

- `just install-tools` has completed, including Playwright browsers.
- Docker or Podman is available for infrastructure-backed integration tests and the full
  verification gate's container-backed suites. This feature's broker-plus-ExtProc challenge tests
  use `bootstrap.ExtProcEnvironment` and assert gRPC immediate responses without a gateway container.
- The maintainer has approved the items in [plan.md › Approval Items](./plan.md#approval-items).
  ADRs 045 and 046 have been moved to `adrs/` with an Accepted status.

## 1. Static checks and fast tests

```bash
just check
just test
```

Expected:

- The new `internal/domain/servicescope` table tests pass for the grammar in
  [contracts/virtual-service-scope.md](./contracts/virtual-service-scope.md).
- The `oauth2server` refresh tests show that `invalid_scope` leaves the refresh session unused.
- The ExtProc challenge tests cover `scope` and `resource_metadata` inclusion and omission.

## 2. Migration and repositories

```bash
just test-integration-infra
```

Expected: migration `046_add_service_scope_bindings` applies, rolls back, and reapplies. Bindings
survive the round trip on `authorization_codes` and `refresh_token_sessions`, and `NULL` rows stay
readable. Both adapters enforce exact successor binding inheritance and scope subset. Exercise code
issuance with full scopes → successful narrowed refresh → scope-less successor refresh: successor
`Scope` and response/JWT stay narrowed, every original binding survives and is rechecked, and neither
a scope-less refresh nor a retained binding can restore a removed scope. Include third-step service
failure and re-requested removed-scope refusal, both without token consumption.

## 3. Backend acceptance journeys (spec Journeys 1, 3, and 4)

```bash
just test-e2e
```

Run the suites below, or the whole E2E set:

| Suite | Proves |
|---|---|
| `tests/e2e/incremental_authorization_test.go` | Story 1, Scenarios 2, 3, 5, 6 and 7; Story 3, Scenarios 1 and 4; Story 4, Scenarios 2 and 3. Complete authorization/consent/code/refresh/exchange journeys cover non-consuming widening refusal, server-derived coverage, submission enforcement, exact visible scopes, local refresh rechecks, and rename/reuse identity continuity. |
| `tests/e2e/incremental_authorization_proxy_test.go` | Story 3, Scenarios 2 and 3. A mock upstream IdP records the received `scope`: virtual scopes never appear, and the rotated upstream refresh token is returned unchanged. |
| `tests/e2e/service_session_page_test.go` | Story 2, Scenarios 1, 3 and 4: both detail identifiers, `session: null`, `401` and `404`, principal isolation, provider login/callback to the same service page and successful exchange after direct recovery. |
| `tests/e2e/incremental_authorization_gateway_test.go` | Story 1, Scenario 1 and Story 4, Scenario 1. Each owns one complete broker-plus-ExtProc scenario, including persisted provider rejection, scope/action propagation and unchanged elicitation, with metadata-present/absent variants. |
| `tests/e2e/incremental_authorization_docs_test.go` | Story 4, Scenario 4: live discovery excludes virtual scopes and the operator guide's route/config example agrees with the contract. |
| `tests/e2e/frontend/incremental_authorization_test.go` | Story 1, Scenario 4 and Story 2, Scenario 2. Consent States C/D and service-page States A/B are exercised through the existing page objects, including login, Save/tool retry and refresh/termination. |

The [task mapping](./tasks.md#acceptance-scenario--task-mapping) assigns exactly one owning `It`
to each of the 19 scenarios; supporting unit/integration assertions do not duplicate acceptance
ownership. Run `just test-e2e-backend` for these broker-plus-ExtProc and proxy journeys, and
`just test-e2e-frontend` for the two browser journeys.

### Required regression variants

Keep these variants inside their existing owning scenarios, with semantic-red assertions first:

- US1 S4/S7: all-scopes and no-explicit-requirements agents, empty configured required scopes,
  selected Read Access needing `repo user:email`, but valid access with only `repo`. Selecting the
  failing candidate disables Save, deselecting it can enable Save; direct submission and refresh
  cannot claim usability until the effective union is present. Complete login/Save/issuance/exchange.
- US4 S2: Save successfully, deterministically reassign the canonical ID, then follow the returned
  authorize URL with its original JWE. Assert `invalid_scope`, no authorization code and no proxy
  redirect, even when the replacement service has a usable session and grant coverage.
- US3 S1: full → narrowed → scope-less refresh retains narrowed scopes and every assurance binding;
  test subsequent unusability and attempts to restore removed scopes without consuming the token.
- Supporting T024 unit cases: valid scope-sufficient, identity-matching access with absent refresh
  and with expired refresh is usable without provider calls; expired access with those same refresh
  states is unusable.

### Maintained UI captures

Normal verification does not capture screenshots. For maintained artifacts, use the existing
capture-enabled serial frontend harness:

```bash
E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend
```

The harness stages captures under `coverage/screenshots/` relative to the frontend suite working
directory. Retain the four plan-named images under `tests/e2e/screenshots/` using existing screenshot
handling; do not modify workflows. Verify consent C/D and session A/B against the spec's Wireloom
states. The design composition review below records T010; captures and runtime accessibility validation
remain unfinished.

### Design-system composition review (T010)

The review uses the design-system index, common-mistake guidance, decision trees and component
pairing guide, together with the existing `PermissionSetCard`, `SessionCard` and
`TerminationDialog` source. Both new compositions are application-specific, not universal
components; no design-system primitive or visual-direction change is needed.

| Wireloom state | Existing composition and behavior |
|---|---|
| Consent C — requested service needs login | Keep agent header, End Date and the existing permission-set/service switches. Use one application `RequestedServiceBanner` per service built from `Alert`/`Card`, text and `Badge` markers for every covering candidate. Use warning semantics plus explicit text, not color alone. Explain partial required-scope coverage and the selected candidate's usability separately; keep optional toggles editable and never auto-select them. |
| Consent D — ready to save | Change the banner to success with a textual ready explanation only when the baseline and every selected contribution are usable and the selected union covers required scopes. Unselected unusable candidates do not block Save. Save remains the single primary action for submission; separate the existing destructive revoke action visually. Preserve end date, unrelated toggles and protected request context through login. |
| Session A — missing/unusable | Use `Card` with service identity, scope URL, text-labelled warning state, the principal's dependent agents and a primary Login action. Distinguish an existing unusable row from no row. A loading skeleton, authenticated not-found state and retryable `Alert` are separate states, not fabricated empty sessions. |
| Session B — active | Reuse `SessionCard` content and UUID-based Refresh/Terminate actions, plus `TerminationDialog` confirmation. Show scopes, expiry, encryption and the principal-owned dependent agents. Successful refresh refetches detail; termination returns to State A. Login/management never mutates an agent grant. |

Use the existing design-system semantic tokens and variants (surface, trust, success, warning,
error and secondary text), not raw palette utilities, custom CSS, inline color values or external
assets. Use `Card padding="default"`, existing layout spacing and the existing switch keyboard
behavior/labels. Retain visible focus indicators. Requested-service markers must have readable
text as well as status color. Keep disabled-Save reasons outside the button, visible to keyboard
and screen-reader users, with stable IDs linked via `aria-describedby`; announce changing
satisfaction/retry status without moving focus on each toggle.

The existing light/dark tokens are the source of truth. Implementation must test both themes for
4.5:1 text and 3:1 UI contrast, keyboard focus/order, switch names/state and confirmation focus
restoration. Reusing an existing component does not prove accessibility or matching usability
semantics; T067/T071/T096 must cover any minimum adaptation needed by this feature without
unrelated restyling. Four maintained browser captures and those runtime checks are still required.

## 4. Manual spot check (optional)

1. Start the broker with an in-memory store and `oauth2_authorization_server.mode: "local"`, using
   the configuration from spec Journey 1, Step 1.
2. Reproduce spec Journey 1, Step 2 with the token-exchange request. The JSON error contains
   `"scope":"https://broker.example.com/sessions/github"`
   ([contracts/enduser-api.md §1](./contracts/enduser-api.md#1-post-oauth2token-rfc-8693-error-scope-member-api-exchange-challenge)).
3. Repeat spec Journey 1, Step 3 with the refresh request. The response is
   `400 invalid_scope`. Repeating the same refresh without `scope` then succeeds, which shows the
   token was not consumed.
4. Open `https://broker.example.com/sessions/github` while signed in. State A renders. Login
   returns to the same page in State B.
5. Rename the service's canonical ID to `github-production` through the admin API.
   - `/sessions/github` now shows not found.
   - A local refresh bound to the old scope returns `invalid_scope`.
   - A new exchange error emits `/sessions/github-production`.

## 5. Gateway metadata variant

Add `resource_metadata_url` to the agentgateway route's `metadataContext`, as in
[contracts/extproc-challenge.md §1](./contracts/extproc-challenge.md#1-optional-metadata-field-aibtokenexchangeresource_metadata_url-api-metadata-config).
Trigger an exchange error.

- With the field, the `WWW-Authenticate` header contains `resource_metadata="…"`.
- Without it, the header omits that attribute. The `scope` and `error_uri` are identical in both
  variants.

## 6. Rollback note

Rolling back requires reverting the code first, then migration 046. Rolling back the code alone is
not enough: the pre-feature broker treats a granted virtual scope as an ordinary scope string.
After the rollback, affected clients may need to authorize again. No user session or grant data is
lost. The discarded service-scope bindings cannot be recovered by reapplying migration 046;
affected clients need fresh interactive authorization to reestablish identity pinning.

## 7. Full completion gate

```bash
just verify
```

All tasks, including migration/repository validation and the complete acceptance journeys, remain
required. Unavailable PostgreSQL/container/browser infrastructure is a validation blocker, not a
passing or optional check. The implementation report must distinguish successful commands from
remaining validation and leave unfinished tasks unchecked.
