# Goldfish Annotated Review Report — Round 2

Status: resolved

## Round 2 decisions (interactive)

- R2-D1: allow `time.now_ns` and `io.jwt.decode_verify`; the other 9 nondeterministic built-ins are listed and removed.
- R2-D2: kept (unsigned exact-target subjects are agent-category on OPA rules); the opt-in and echo alternatives are recorded as rejected.
- R2-D3: kept the terminal 500 for an undefined decision; added a risk row; admission uses an exact rule lookup (`GetRulesExact`).
- R2-D4: kept no https or signing requirement; rationale recorded.
- R2-D5: documented the fleet out-of-memory risk only.
- R2-D6: closed bootstrap allowlist, cloud credentials allowed in non-file forms.
- Applied: R2-S1 to R2-S8, R2-P1 to R2-P7, R2-T1 to R2-T7 (tasks.md resequenced to 85 tasks).
- Declined: R2-X1 to R2-X3.

**Targets** (all full file):
- `specs/20261120-153502-opa-impersonation-authorization/spec.md` (1–760)
- `specs/20261120-153502-opa-impersonation-authorization/plan.md` (1–367)
- `specs/20261120-153502-opa-impersonation-authorization/tasks.md` (1–336)

**Reviewers** (all completed):

| Reviewer | Target | Blockers | Majors | Nits |
|---|---|---|---|---|
| `design_critic` | spec | 0 | 6 | 4 |
| `structural` | spec | 0 | 4 | 9 |
| `implementation_readiness` | plan | 0 | 6 | 7 |
| `codebase_details` | plan | 1 | 4 | 5 |
| `implementation_readiness` | tasks | 1 | 7 | 7 |

**Round result: not clean.** Blockers and Majors are open, so the convergence counter restarts.

**Settled in round 1, not reopened:**
- Unlinked maintainer attributions (design M6, structural M3; S1-2 was skipped).
- H1 inside the quoted issue, and trailing blank lines in HTTP fences (structural N1 and N8; S1-20).
- Unnumbered API requirements and the Domain Model layout (structural N4 and N6; S1-19).
- Directory entries in the file tree (part of plan readiness M6; P1-18). The newly missing files in M6 are not settled.

Categories:
- **DECIDE:** a design choice for the user.
- **PROPOSE FIX:** a valid finding with a mechanical fix.
- **DECLINE:** a nit not worth the change.

## Design decisions (DECIDE)

| ID | Sev | Source | Finding | Options |
|---|---|---|---|---|
| R2-D1 | Blocker | `codebase_details`-2-1 | DEC-BUILTINS lists 7 removed built-ins. OPA v1.21.4 flags 11 nondeterministic (`v1/ast/builtins.go`), so the rule also removes `io.jwt.decode_verify`, `json.verify_schema` and `json.match_schema`. | List all of them and ban all except `time.now_ns`; or also allow `io.jwt.decode_verify`, which is time-dependent only, like `time.now_ns`. |
| R2-D2 | Major | `design_critic`-2-1 | On OPA rules, an unsigned exact-target subject is now agent-category with no check, so a client-only policy on an agent rule mints consent-free tokens for any registered agent. A runtime-binding alternative was not considered. | Keep it and record the rejected alternatives; or keep such subjects human unless the decision opts in (e.g. `"subject_category":"agent"`); or make the decision echo the approved `sub`/email for the broker to compare. |
| R2-D3 | Major | `design_critic`-2-2, plan readiness-2-M1 | An undefined decision at evaluation is a terminal 500. Reused Rego without a `default` then breaks every non-matching request and later rules. Admission's "decision exists" check is also undefined (exact rule, `data.json` value, nested refs). | Undefined = deny (non-match, as in ExtProc); or admission requires a `default` rule; or keep the 500 and add a risk row. In every case, define the admission algorithm. |
| R2-D4 | Major | `design_critic`-2-4 | Plain-HTTP and unsigned bundles are accepted by default, which conflicts with Principle I ("security features enabled by default"). The 2026-11-24 clarification chose "no broker requirement". | Keep it and record the rationale; require `https` unless explicitly opted out; also require signing unless opted out; refuse to send the bearer token over `http`. |
| R2-D5 | Major | `design_critic`-2-5 | A large bundle can make every replica run out of memory at once, and the replicas then crash-loop because readiness needs the same bundle. | Enforce a broker default cap (e.g. reject bundles without `size_limit_bytes`, or apply a default limit); or keep the recommendation and add a risk row. |
| R2-D6 | Major | plan readiness-2-M3 | The bootstrap credential policy is half allowlist, half denylist. Unsettled: `s3_signing`, `gcp_metadata`, `azure_managed_identity`, `oauth2.client_assertion_path`, `tls.ca_cert`, and which `bundles`/`keys` members are accepted. | Publish a closed allowlist for OPA v1.21.4 (proposed: `bearer.token` inline, `oauth2` client credentials inline, `plugin: aib_bearer_token_file`; every file-reading or cloud credential rejected; `bundles`: `service`, `resource`, `signing`, `polling`, `size_limit_bytes`, `trigger`; `keys`: `algorithm`, `key`, `scope`). |

## Proposed fixes (PROPOSE FIX)

### Spec

| ID | Sev | Source | Fix |
|---|---|---|---|
| R2-S1 | Major | `design_critic`-2-3 | Fetch outages are invisible to broker telemetry. Register an OPA bundle status listener. Add a gauge "seconds since last successful bundle download" and a failure counter per bundle (no claims). Rewrite the stale-data risk mitigation (affects API-DIAG). |
| R2-S2 | Major | `structural`-2-M1, `design_critic`-2-N10 | Add a `## Problem Statement & Goals` section after the Input. Move Non-goals into it. Remove the duplicate `### Assumptions` heading and the repeated non-goal bullets. |
| R2-S3 | Major | `structural`-2-M2 | In Journey 4 step 4, add a YAML fragment (`audience_requirement: absent` on one role) and the decoded header and claims of the audience-less signed JWT. |
| R2-S4 | Major | `structural`-2-M4 | Convert the Journey 2 and Journey 3 lists to bold `**Step N — …**` steps. Move Journey 1's local variant after Step 4. Make cross-journey step references explicit ("Journey 2 Step 5"). |
| R2-S5 | Nit | `design_critic`-2-N7 | Empty `impersonation.opa`: absent or `{}` is accepted; any populated key without an OPA rule fails. Align FR-002, the config table and the contract. |
| R2-S6 | Nit | `design_critic`-2-N8, `implementation_readiness`-plan-N2, `codebase_details`-2-9 | Governance wording: CONST-AMEND text is applied and takes effect when ADR-OPA is Accepted. Fix spec line 760, the plan's Approval Items preamble, and the CONST-AMEND quote ("under Accepted ADR"). |
| R2-S7 | Nit | `design_critic`-2-N9 | Extend the admission-coupling risk row: a rejected bundle stays in OPA's store, so later updates to other bundles are rejected until it is republished. |
| R2-S8 | Nit | `structural`-2-N3, N9 | Add the missing blank lines and `---` separators between user stories. State that composite scenarios map to one table-driven `It` each. |

### Plan

| ID | Sev | Source | Fix |
|---|---|---|---|
| R2-P1 | Major | plan readiness-2-M2, tasks readiness-2-M4 | Give the Go declaration of the `authorization` seam: `label()`, `engine()`, `evaluate(ctx, policyInputs)`, `bindsSubjectField(field) bool`. CEL returns the predicate binding; OPA's value follows R2-D2. Define the `policyInputs` struct. |
| R2-P2 | Major | plan readiness-2-M4, `codebase_details`-2-2, tasks readiness-2-M4 | Add an `internal/fscontain` contract covering: exported API (open root over a directory, read a regular file with a size bound, bounded directory walk); error type and reason classes; `_unix`/`_other` open flags and their tests; how jwks maps errors back to `trustanchor.Error` unchanged. |
| R2-P3 | Major | plan readiness-2-M5, `codebase_details`-2-4, tasks readiness-2-M5 | Migration list: add `internal/ports/config_test.go` and the `builder.go` `NewService` call (`nil, ""`). Mark `impersonation_validator_test.go` "non-cel authorization" and `service_test.go:622` as intent changes, not mechanical. Add the rule "every compile error from the change". |
| R2-P4 | Major | plan readiness-2-M6 | Complete the create/modify list to match tasks.md: test files, bootstrap wrappers, telemetry doc files, fscontain files, vector file names. |
| R2-P5 | Major | `codebase_details`-2-3 | Shared vectors: add a per-builder `path` expectation. ExtProc keeps the query in `path`; the broker strips it. |
| R2-P6 | Major | `codebase_details`-2-5, plan readiness-2-N3 | Use the real audit event name `impersonation.decision.recorded` in the plan, observability contract, research and quickstart. |
| R2-P7 | Nit | `codebase_details`-2-6, 7, 8, 9, 10, plan readiness-2-N1, N4, N5, N6, N7 | Mechanical nits: task refs T050→T058 and T033→T037; citation line ranges; name `defaultEvaluationTimeout` for extraction; remove the stale "dependency graph" mention; reuse the loader's decode-hook pattern or justify a raw-map pass; replace the `json.Number` claim with "same values as the CEL activation"; give the `Close(ctx) error` and `NewService` context; specify that the raw form is set on `req.HTTP` after `ParseRequest`; name the inline-only fallback for the credential plugin. |

### Tasks

| ID | Sev | Source | Fix |
|---|---|---|---|
| R2-T1 | Blocker | tasks readiness-2-B1 | Move local bundle loading (T047/T050 and the `local_mode` log) into US1, so the Scenario 1.1 local variant can pass at the US1 checkpoint. |
| R2-T2 | Major | tasks readiness-2-M1 | Add the following to US1: accept `type: opa` in `validateRuleInvariants`; apply the defaults at use; call `AwaitReady` in `NewService`; fail closed on a nil revision. |
| R2-T3 | Major | tasks readiness-2-M2 | Move all fixtures (T042) and variant assertions into T016/T017. Reword T043, T054 and T060 as "make green; fixture adjustments only". |
| R2-T4 | Major | tasks readiness-2-M3 | Move the telemetry-descriptions update and `just telemetry-doc` (T066) to immediately after T053, so `just test` passes. |
| R2-T5 | Major | tasks readiness-2-M6 | T008 also updates ARCHITECTURE.md §3.1.y (impersonation boundary) and the §10 ADR table row. |
| R2-T6 | Major | tasks readiness-2-M7 | T088 adds `just lint-plugin-test` and names `just security`. |
| R2-T7 | Nit | tasks readiness-2-N1 to N7 | Restore the Phase 2 constitution paragraph and checkpoints verbatim; name the bootstrap wrapper files and the span-processor owner; create `telemetry.go` before T050; fix the T030 scenario cite (3.3); drop `[P]` on T021; state interim US1 error behavior; fix stale task IDs in plan.md and data-model.md. Regenerate or patch tasks.md after the plan fixes. |

## Declined

| ID | Source | Reason |
|---|---|---|
| R2-X1 | `structural`-2-N2 | Clarifications format differences are historical record; restyling them adds churn. |
| R2-X2 | `structural`-2-N5 | Heading qualifiers: cosmetic. |
| R2-X3 | `structural`-2-N7 | Alternatives and Risks before Requirements: this order was accepted in round 1 (S1-1, S1-3). |
