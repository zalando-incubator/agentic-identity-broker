# Quickstart: Validating Agent-Subject Impersonation

**Feature**: `specs/054-agent-subject-impersonation` | **Plan**: [plan.md](plan.md)

The approved broker behavior, chart support and all 12 acceptance scenarios are implemented. **Delivery remains blocked by T072: the full `just verify` gate has security-scan termination and existing dependency findings.** Do not merge, mark ready, or deploy on the strength of the passing feature checks alone.

## Implementation record

### Supplied approval (T001)

The pipeline supplied approval by **mjungsbluth**, at **2026-10-29T13:35:38Z**, on [the approval comment](https://github.com/zalando-incubator/agentic-identity-broker/pull/293#issuecomment-6495558326), covering all ten items in the preceding reanalysis. This is the approval source, not an independent acceptance decision.

| Item | Approved scope recorded for implementation |
| --- | --- |
| A1 | ADR 045: agent-only exceptions to ADR 032 decisions 1/2, including target-derived subject despite mandatory extraction; C4 and major-version decision 8 |
| A2 | `allowed_subject_categories`, omission-only human default, strict validation and Helm schema |
| A3 | Optional guarded-default detector, replacing override, Helm setting and fail-closed evaluation; environment/CLI transports follow existing naming conventions (U1) |
| A4 | Token-endpoint self-agent behavior, category denial, matched third-party denial and detection-failure `server_error` |
| A5 | Credential-free `subject_category` audit and the two category diagnostic details |
| A6 | C4 full replacement: non-matching overrides remove categorical rejection and matching-human-state isolation |
| A7 | No unsigned-subject startup guard; unsigned subjects remain human |
| A8 | Signed exact-target rejection on human-only/omitted rules, even with active same-text delegation |
| A9 | Default equal-claim rejection for legacy, mapped/unsigned human and trusted external issuer tokens |
| A10 | End-User API 2.0.0 replacement on existing routes, collision migration and blocking version/behavior checks |

ADR relocation: `adr-045-draft.md` now resides at [adrs/045-agent-subject-impersonation.md](../../adrs/045-agent-subject-impersonation.md), with the exact approving login/date/URL in Status. This record resolves the implementation-frozen spec/plan's draft links. Existing ADRs were not amended. `spec.md` and `plan.md` remain unchanged.

### Delivered behavior

- Classification uses only a verified signed raw string `sub` and exact target UUID equality, after all principal extractions succeed. Eligibility is a separate rule gate. Omission remains human-only; signed collisions do not acquire a human fallback, even with delegation. Eligible agents skip delegation and credential-email extraction, mint the target UUID, and retain the verified actor.
- Human delegation failures remain terminal and retain the existing consent URI. Unsigned subjects remain human under ADR 031, including exact-text subjects and rules listing `agent`. ADR 034 audience guards, signed-role controls, scope checks and proxy/hybrid rejection are unchanged.
- YAML shape validation runs before Viper's weak decoding, retaining omission versus explicit null and rejecting non-string/list shapes. The typed YAML decoder also resolves aliases/merges; lookup follows Viper's case-insensitive key convention. Startup/domain validation enforces 1–2 distinct case-sensitive `human`/`agent` entries. No unsigned startup guard was added.
- The detector compiles once at startup over `subject_token`, accepts only static `bool`/`dyn`, enforces the existing evaluation timeout, and fails closed on errors/non-boolean output. An override replaces the guarded default entirely. Only third-party exchange invokes detection, after security-context finalization and before agent extraction, authorization, resource resolution or human state.
- U1 is resolved through real production Cobra registration and explicit loader bindings. The flag is `--token_exchange.claim_extraction.agent_subject_expression`; the environment variable is `IDENTITY_BROKER_TOKEN_EXCHANGE_CLAIM_EXTRACTION_AGENT_SUBJECT_EXPRESSION`. CLI > environment > YAML > evaluator default; unchanged flags do not shadow lower sources. Explicit empty CLI/environment selects the guarded evaluator default. Tests cover no YAML file/key and conflicting sources.
- Audit adds only `subject_category` after classification and uses the represented subject. Diagnostics distinguish category ineligibility and third-party unsupported subjects. T075 corrects the handler's sanitization mapping to the already-approved C2/C3 fixed descriptions, without exposing internal error text or changing nonmatching human descriptions.
- The mint strategy and its CEL context require no production change. Nil credential email yields no claim without an email policy; `principal.email` still produces `""`, and a literal policy email remains policy-owned. Reserved `sub`, `agent_id` and actor attribution cannot be overwritten by policy. Issuer, signing, TTL, scope and policy-owned audience remain unchanged.

### Semantic-red evidence before behavior

Run `41041966247/1` resumed at `1bed8c333d74cfa00f296d6a976abdad251d6a4f`. Go 1.27.3, Ginkgo, Helm and golangci-lint were available; the earlier missing-tool blockers are resolved. Existing ignores already cover build/test/dependency artifacts. The requirements checklist has 16 checked items and no unchecked items; its markers were not changed.

Before any chart or broker behavior change, only the two DTO fields necessary to compile acceptance tests were added. The complete feature suite compiled and ran **12 scenarios: 4 passed, 8 failed, zero pending**:

| Scenario | Initial observable outcome |
| --- | --- |
| US1.1, US1.2, US2.2 | Agent issuance incorrectly required human delegation instead of returning a signed token |
| US2.1, US2.4 | Signed-target collisions were accepted or acquired consent recovery instead of category denial |
| US2.5 | Direct startup incorrectly accepted an empty category list |
| US3.2 | Agent-only mismatches incorrectly offered human consent recovery |
| US3.4 | Completed sampled HTTP trace existed, but its positive human control had no required grant-boundary span |
| US1.3, US2.3, US3.1, US3.3 | Existing credential, delegation, policy and runtime-error protections already passed |

Helm tests separately compiled/executed with Helm installed and showed **seven semantic schema failures** before chart changes: all four explicit valid category lists and empty/guarded/replacing detector settings were rejected as unknown properties. After T009, Helm passed and the complete feature suite was rerun, still **4 passed / 8 failed**, before broker foundation work.

| Tasks | Further test-first evidence and green result |
| --- | --- |
| T019–T024 | YAML-shape, startup-value and direct-rule tables failed because invalid values were accepted. Layered validation turned them green. An additional case-insensitive-key regression failed before its lookup correction; YAML merge handling already passed. Loader/direct-startup acceptance also verifies omission and both dual orders. |
| T022/T025 | Detector tables failed on equality, replacing expressions, startup syntax/type rejection, runtime non-boolean/errors and timeout; diagnostic details were initially reduced to `internal_unclassified`. All pass after detector/catalogue implementation. |
| T026–T027 | Real consent/session services with recording repository fakes preserved outcomes but lacked the two required child spans on success/missing/error returns. Instrumentation turned these tests green; parent/context propagation, attribute/event absence, no-op tracing and cancellation are verified. |
| T028–T029/T032–T033 | Classification/eligibility and issuance tests failed on exact signed classification, delegation calls, target-derived extraction and skipped email evaluation. They now pass, including mapped-principal isolation and signed credential email input. |
| T030/T034 | Agent mint/normal-human compatibility characterization passed without changing `strategies.go`; no artificial failure or policy-email suppression was introduced. |
| T031/T035 | Classified success/denial/error log tables lacked `subject_category`; the minimal audit addition turned them green. Early-failure identity behavior and the attribute allowlist remain covered. |
| T037–T042/T045 | Matrix/rule-order, unsigned exact-text, extraction/security and existing no-match/terminal-denial protections characterize approved behavior; additional production corrections were unnecessary. Existing protections were not deliberately broken. |
| T043–T044 | Verified equal-claim subjects reached failing agent extraction instead of the required category rejection, and the dynamic detector returned the wrong failure detail. Detection now runs first, preserving finalized actor/calling-peer capture and zero grant/session calls. Equal-claim broker human-collision and trusted-external shapes are covered; the detector cannot distinguish their historical issuance path. |
| T073–T074 | The real root-command test first failed missing flag registration, then failed CLI/environment precedence and no-file cases, then failed explicit-empty environment behavior. All transport/default tables now pass. |
| T076 | Detector-only configuration silently bypassed the existing claim-extraction startup activation guard. A production Builder regression failed before adding the new field to that guard; it now requires the existing external client-assertion trust anchor instead of ignoring the detector. |
| T075 | Unit/HTTP checks exposed the handler's generic description in place of C2/C3. Fixed diagnostic-based mappings now return exactly `impersonation not permitted` and `agent-subject tokens cannot be exchanged for third-party service tokens`. |

Additional characterization tests expand coverage without changing acceptance expectations. Final test fixtures use a fresh concrete storage adapter and broker lifecycle for each US3.4 detector/state variant; assertion changes only strengthen startup round-trip and exact C2 checks. There are exactly 12 scenario-referenced `It()` blocks, no pending markers, vacuous assertions or red-phase annotations.

### Acceptance results

| Scenario | Final evidence |
| --- | --- |
| US1.1 | Signed output verified against published JWKS; `sub == agent_id == target UUID`, no category/email claim or consent URI; no target-human grants/sessions before or after |
| US1.2 | Distinct actor/client/target attribution, plus actor equal to represented target while retaining `act` |
| US1.3 | Invalid client and actor signatures independently rejected, no token/consent recovery |
| US2.1 | All six category/rule combinations, including same-text active human delegation; ineligible cases have exact C2 envelope |
| US2.2 | Dual rule issues for agent without delegation and human with delegation |
| US2.3 | Missing/expired human delegation remains denied with consent URI, including other-agent UUID/noncanonical/alias subjects |
| US2.4 | Omission keeps human behavior but rejects signed exact-target requests both without and with delegation |
| US2.5 | Every invalid shape/value fails before traffic through loader/direct production bootstrap; valid orders/omission round-trip; unsigned rule listing agent starts |
| US3.1 | CEL denial issues no token, irrespective of upstream authorization |
| US3.2 | Missing/invalid/noncanonical/alias/cross-agent subjects and unresolved targets issue no token |
| US3.3 | Runtime policy failure returns terminal `500 server_error`, no consent URI or human state |
| US3.4 | All four detector variants in both absent/same-text-human-state conditions pass the observation checkpoints below |

US3.4 captures sampled, uniquely request-correlated completed HTTP spans via production `BuildAppWithTracerProvider` and the concrete storage adapter. Positive human controls demonstrate a grant-boundary span for missing delegation and both grant/session spans for resource-covering delegation plus a usable session. Each default/compliant/`false` fixture runs controls; the dynamic-error fixture uses a sequential otherwise-identical default-detector control broker, then captures its own failing HTTP trace. Providers and propagators are restored and brokers/providers drained in cleanup. No E2E repository spies, private-field injection, reflection or custom DI were introduced.

- **Default and compliant override:** `403 access_denied`, exact C3 description/diagnostic, no `error_uri`, zero grant/session boundary spans and provider/refresh calls, with/without same-text human state.
- **Dynamic `subject_token.sub`:** `500 server_error`, no consent URI, zero human-state boundary spans/provider calls in both states. Unit tests separately cover evaluation errors/timeouts.
- **Replacing `false`:** no agent-subject description/diagnostic; absent human state produces the existing missing-grant error, consent URI and grant span. Matching usable human state produces both boundary spans. This is positive evidence of continuation, not a claim of categorical isolation or supported third-party agent identity.
- Human-state snapshots remain unchanged; zero-read claims rely on correlated spans and positive controls, not snapshots/provider counts alone.

### Executed validation

Commands ran with `TMPDIR=/tmp/gh-aw/agent` for permitted temporary storage. Focused runs used Ginkgo's `--no-color -v`; filtering excludes unrelated specs, not feature scenarios.

| Command | Actual final outcome |
| --- | --- |
| `just check` | Pass: formatting, `go vet` and actual golangci-lint, zero lint issues |
| `just test` | Pass: fast/package tests with race detection |
| `go test ./internal/domain/impersonation/... ./internal/domain/oauth2server/... ./internal/domain/tokenexchange/... ./internal/config/... ./internal/adapters/http/enduser/... -cover` | Pass: 89.3%, 86.3%, 96.5%, 88.5%, 88.2% respectively |
| `go test ./tests/integration/ -run TestHelm` | Pass with Helm installed and all feature schema/render cases executed, not skipped |
| `just test-integration` | Pass: default self-contained integration suites; this is not the infrastructure-backed/full gate |
| `ginkgo -v --focus 'Agent-Subject Impersonation' ./tests/e2e/` | Pass: 12/12, zero pending |
| `ginkgo -v --focus 'OAuth2 User Impersonation\|Local Mode Token Exchange' ./tests/e2e/` | Pass: 43/43 existing regression specs unchanged |
| `just test-e2e-backend` | Pass: frontend build and 923 functional backend specs, zero failures/pending; standard recipe excludes 10 performance-labelled specs |
| `just verify` | **Fail**: initial and final attempts exited 137 when govulncheck was killed (final gosec: zero issues). An intervening standalone run completed gosec/govulncheck, then OSV exited 1 with 20 known vulnerabilities across 11 packages (1 Critical, 4 High, 9 Medium, 3 Low, 3 Unknown). Later full-gate stages did not run. |

### Compliance and remaining blocker

- Existing End-User OpenAPI is `2.0.0`. Architecture, accepted/indexed ADR 045, migration guide, configuration/concept/reference documentation and complete synthetic examples describe the same approved contract. C1 remains withdrawn; no new request category or token claim was added. Existing Admin API/routes remain unchanged.
- No new dependency, persisted entity, repository, SQL or migration was added. Migration/apply/rollback/new-PostgreSQL-adapter work remains N/A. No frontend/design-system/Wireloom/Playwright change, third-party machine-identity support or unrelated refactor is included. Existing frontend build ran as part of the backend recipe.
- T072 is the only unchecked task. The security findings are in unchanged `go.mod`, `web/package-lock.json` and `assets/docusaurus/package-lock.json`, not new feature dependencies. They were not verified/exploited, suppressed or fixed through unrelated scope expansion. No issue was created.
- **Maintainer action:** provide sufficient runner resources for the unmodified govulncheck scan and resolve the dependency-security findings through a separately authorized dependency update, then rerun `just verify` in its supported environment, including its later infrastructure/ExtProc/frontend gates. This sandbox does not support Docker-in-Docker; missing later infrastructure must also remain a blocker, not a skipped success. Do not waive security or reduce validation. No new feature API/ADR/decision sign-off is requested.

## Reproduction

```bash
export TMPDIR=/tmp/gh-aw/agent
just check
just test
go test ./tests/integration/ -run TestHelm
ginkgo -v --focus 'Agent-Subject Impersonation' ./tests/e2e/
ginkgo -v --focus 'OAuth2 User Impersonation|Local Mode Token Exchange' ./tests/e2e/
just test-integration
just test-e2e-backend
just verify
```

The final command must pass before feature delivery. Configuration contracts are in [contracts/](contracts/); review the End-User API 2.0.0 consumer migration section in `docs/api/enduser-api.md` before rollout. Consumers that are not ready must defer deployment rather than weaken controls or use a non-matching detector as a compatibility fix.
