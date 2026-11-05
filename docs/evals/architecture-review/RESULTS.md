# Behavioral-probe dimension: evaluation results

This evaluation tests a ninth architecture-review dimension, `behavioral-probe`. It
targets the eight findings that jbspeakr and x418 raised by hand on PR #293
(agent-subject impersonation). The eight existing dimensions missed all of them.
The final dimension is [`behavioral-probe-dimension.md`](behavioral-probe-dimension.md).
It is also the ninth `## agent:` section in
[`.github/workflows/architecture-review.md`](../../../.github/workflows/architecture-review.md).

## Summary

| | v1 | v2 | v3 (final) |
|---|---|---|---|
| Strict recall: hits on the expected commit, 8 items × 2 runs | 6/16 (38%) | 7/16 (44%) + 1 partial | **10/16 (63%)** |
| Items hit by at least one of the 2 runs on the expected commit | 3/8 | 4/8 | **6/8** |
| Items found on any eval commit where the defect exists | 3/8 | 6/8 | **7/8** |
| Wrong findings reported (all commits) | 0 | 0 | 0 |
| Runs that wrongly claimed "no ExtProc regression" | 0 | 2/4 on `d177d85` | 1/2 on `d177d85` |
| Control (`b3e2335`): findings per run | 4, 5 | 10, 5 | 6, 6 |
| Control: fixed items re-raised (G1–G6, G8) | 0 | 0 | 0 |
| Avg tokens / tool calls per run | 162k / 32 | 173k / 32 | 174k / 32 |

Two items stay hard. G6 was missed in every one of the 31 runs. G8 was hit on its
own commit only by v1. v2 and v3 found the same defect on other commits:
`client_credentials` tokens are omitted from the lists of affected inputs.

## Method

- **Targets.** These are the reviewed commits and the bases they were reviewed
  against. Each base was verified with
  `git rev-parse $(git log --merges --first-parent -1 --format=%H <c>)^2`, and each
  diff touches 66 files.

  | Commit | Base | Expected |
  |---|---|---|
  | `d6d0cab` | `f63964b` | G1, G2 |
  | `9d25952` | `f63964b` | G3, G4, G5 |
  | `d177d85` | `77b8206` | G6, G7 |
  | `7f54db6` | `77b8206` | G8 |
  | `b3e2335` (control) | `77b8206` | none |

- **Blind runs.** Each commit got its own clone, built by
  `git init` + `git fetch --no-tags <repo> <head> <base>`, with no remote. Only the
  head and base ancestry is reachable, so later fix commits and review comments
  cannot leak in. [`harness/make-brief.sh`](harness/make-brief.sh) writes an
  orchestrator-style brief: the head and base SHAs, changed files by area with line
  counts, ADR provenance, and the inspection commands. It omits the PR body and the
  summary. [`harness/render-task.sh`](harness/render-task.sh) copies the dimension
  prompt into the clone's `.git/` and prints the task. Each run was a fresh
  `general-purpose` sub-agent with no shared context. It received only the task
  message, which names the prompt file, the brief and the clone. It was forbidden to
  read other checkouts or use the network. Every commit ran twice per version.
- **Judging.** I scored every run myself against the eight comment bodies, which I
  fetched from the PR. A **hit** names the same problem with a correct mechanism or
  location. A **partial** names the right symptom with the wrong mechanism. Extra
  findings are classed as real (new), wrong, or noise (a duplicate of another
  finding). For recurring claims I read the code myself:
  - unsigned docs vs `classifySubject`;
  - the per-rule fall-through of the cross-agent rejection;
  - the order of the agent lookup and CEL evaluation;
  - ExtProc's `exchangeErrorResponse` → `tokenExchangeErrorResponse` 500.

  Notes per run are in [`runs/judge-notes.md`](runs/judge-notes.md). Raw outputs are
  in `runs/<version>/<commit>-r<n>.md`. Costs are in [`runs/cost.tsv`](runs/cost.tsv)
  (version, commit, run, sub-agent tokens, tool calls, seconds).
- **G7 on the control commit is not a false positive.** The author deferred G7 to
  #416, so ExtProc still returns 500 at `b3e2335`. A control run that reports it is
  correct.

## Per-commit results

Each cell shows run 1 / run 2. ✅ is a hit, ◐ a partial, ✗ a miss. "Extra" counts
findings outside the expected items (total findings in the run minus hits). Every extra was real unless marked otherwise.
"(dup)" marks duplicate rows of a claim already reported.

| Commit | Expected | v1 | v2 | v3 |
|---|---|---|---|---|
| `d6d0cab` | G1 | ✅ / ✅ | ✅ / ✅ | ✅ / ✅ |
| | G2 | ✗ / ✗ | ✗ / ✗ | **✅** / ✗ |
| | extra | 2 / 3 | 5 / 7 | 6 / 4 |
| `9d25952` | G3 | ✅ / ✅ | ✅ / ✅ | ✅ / ✅ |
| | G4 | ✗ / ✗ | ✅ / ✗ | ✅ / ✅ |
| | G5 | ✗ / ✗ | ✅ / ✅ | ✅ / ✅ |
| | extra | 4 / 3 | 7 / 6 | 3 / 4 |
| `d177d85` | G6 | ✗ / ✗ | ✗ / ✗ | ✗ / ✗ |
| | G7 | ✗ / ✗ | ◐ / ✗ (claimed no regression) | ✅ / ✗ (dismissed as documented) |
| | extra | 5 / 3 | 5 / 7 | 6 / 5 |
| `7f54db6` | G8 | ✅ / ✅ | ✗ / ✗ | ✗ / ✗ |
| | extra | 4 / 3 | 9 (5 dup) / 7 | 8 / 5 |
| `b3e2335` | none | 4 / 5 extra, 0 wrong | 10 (5 dup) / 5 extra, 0 wrong; G7 open 1/2 | 6 / 6 extra, 0 wrong; G7 open 2/2 |

Two ablation runs on `d177d85` used v2 with the ExtProc pointer removed
(`prompts/v2-noext.md`). G7 was ✅ in one and ✗ in the other, and the ✗ run wrongly
claimed that ExtProc maps the 403 to `authorization_denied`. G6 was ✗ in both.

### Extra findings that recur (real, not in the ground truth)

The dimension kept finding these. They are real gaps in the reviewed commits, and
most survive to the approved head:

1. **Docs say unsigned subjects are always human, but the code issues agent
   tokens.** These statements say listing `agent` on an unsigned rule "has no
   effect": `docs/configuration.md:1621`,
   `examples/config/impersonation-agent-subject.yaml:87`, `docs/api/enduser-api.md:44`
   and the OpenAPI "including every unsigned subject". Under ADR 045 decision 2a,
   `classifySubject` returns `agent` for an unsigned exact-target `sub` whose
   predicate binds `subject_token.sub`, and the broker mints a token without consent.
   I confirmed that the statements are still on `main` (`2dc18c0`). Every run on
   every commit found this.
2. **Signed-only wording.** ADR 045 decision 1, the ADR 032 supersession banner and
   several docs describe the delegation exemption as signed-only.
3. **The cross-agent rejection guards less than FR-003b states.** The agent-category
   branch skips the extracted-principal check (`service.go:230`). The rejection is a
   per-rule fall-through failure, so a later unbound unsigned rule falls back to
   human delegation with a consent link. Unbound unsigned subjects skip it entirely.
   Several runs confirmed these with temporary `go test` overlays.
4. **The cross-agent lookup runs before CEL authorization.** A client the policy
   denies can learn which UUIDs are registered agents from the different denial
   descriptions.
5. **Broken references and an incomplete list.** A migration guide is referenced
   after A14 removed it. Decision 8's list of affected inputs is still incomplete for
   the approval endpoints.

The single wrong-conclusion pattern was a missed G7 stated as "ExtProc shows no
regression". No run reported a finding that the code contradicts.

## Version changelog

Every change is a general review technique. None names an identifier, claim, or
wording from the ground truth.

| Version | Change | Why | Effect |
|---|---|---|---|
| v1 | Probes A–F: trace checks end to end, adversarial input tables, vacuous examples, consumers of new errors, stale statements, dead code. Severity scale and JSON contract as in the other dimensions. | Initial design from the brief. | G1, G3, G8. Misses: G2 (fallback), G5 (checked vs. used value), G4 and G6 (stale general rules), G7 (consumers). |
| v2 | A **value matrix**: list every identifier for the actor (raw, mapped, normalized, target) and which value each check and downstream step reads. Build the input where they differ. | Runs said "values agree" without comparing. | G5 went from 0/2 to 2/2. |
| v2 | **Default-branch probe**: inputs that *almost* match the narrow branch. Is the default treatment safe for them? | G2 class. | No change. Runs tested the input but accepted the documented outcome. |
| v2 | **Consumer table**: base vs. head response including optional fields. Read consumer code, not docs. Named the in-repo gateway as a consumer. | No v1 run opened ExtProc. | ExtProc traced in many runs. On `d177d85`, 2/4 stopped at an intermediate classification and concluded "no regression". |
| v2 | **Stale-statement sub-probes**: exception vs. general rule, absolutes and enumerations, internal consistency of new ADRs and specs. Report each statement separately. | G4/G6/G8 class. | G4 1/2. Duplicate rows rose (up to 9 per run), and G8 was crowded out (2/2 → 0/2). |
| v2-noext | Ablation: removed the `internal/extproc/` pointer. | Overfitting check. | G7 1/2 without the pointer, and 1 wrong "no regression" claim. The pointer is a repository fact that helps but is not decisive. |
| v3 | **Documented intent does not settle it.** Report as a question when the documented treatment is unsafe or makes no sense for the input: another kind's rules, a fallback that bypasses protection, or a recovery step the caller cannot complete. | v2 coverage notes showed G2 tested and accepted because the ADR said so. | G2 0/2 → 1/2. |
| v3 | Follow each consumer **to its final response**, not to an intermediate error type or label. | Wrong "no regression" conclusions. | 1 of 2 G7 misses on the expected commit remained. That run dismissed the 500 as documented gateway behavior, as the PR author did. |
| v3 | **One finding per contradicted claim**, listing all locations. | v2 duplication noise. | Findings per run fell from 7–10 to 5–7. Control dropped from 10/5 to 6/6. |
| v3 | **Read the PR's own commit log and probe the latest fix-ups first.** Added a "new paths into an outcome" sub-probe. | G4/G6 are statements made stale by late fixes. | G4 2/2. On `d177d85` the probe found a sibling stale ADR sentence ("looks the agent up only when `sub` is a canonical UUID"), but not G6 itself. |
| final | Removed the "for example a configured mapping …" illustration from the new-paths bullet. Allowed "consistently documented" behavior to be reported when probe B shows it is unsafe. Rewrapped lines. | Removes the one example close to G6's wording. Aligns the rules with the v3 change. | Not re-run. These are wording-only changes to v3. |

**Overfitting guard.** I ran the ExtProc-pointer ablation instead of a
leave-one-commit-out check. Each version's changes were motivated by misses spread
over all four target commits, so no commit was held out. G6 and G8 never improved,
and v3's gains are on items whose mechanisms the prompt describes generically.
Without the pointer, G7 is found about half the time.

## Overlap with the existing dimensions

| Item | Existing dimension that should have caught it | Why it didn't |
|---|---|---|
| G1 dead fixtures | `maintainability`, `e2e-test-quality` (fixture reuse) | Neither asks for an unused-declaration check. A one-line addition to `maintainability` would cover it. |
| G4 ADR decision 2 vs. 2b, stale FR/glossary | `adr-compliance` (new ADRs reviewed critically), `ubiquitous-language` (glossary) | `adr-compliance` compares new ADRs with *pre-existing* ones, not with themselves. `ubiquitous-language` checks that terms exist, not that definitions are true. |
| G6, G8 stale claims | `maintainability` "documentation debt" | It checks whether docs were *updated*, not whether they are *true*. |
| G3 vacuous example | none | No dimension evaluates documented policies against code preconditions. |
| G2, G5 classification edge cases, checked vs. used value | `data-authority` (partly) | Its "can X contradict X" test covers stored facts, not request-time values. |
| G7 cross-component regression | `coupling-cohesion` (partly) | It looks at API surface and scattered logic, not at the consumer's runtime outcome. |

## Recommendation

**Add `behavioral-probe` as the ninth dimension.** Also make two small changes to
the existing dimensions:

1. **Why a new dimension.** Six of the eight items (G2, G3, G5, G6, G7, G8) need
   techniques that no existing dimension has:
   - executing the logic in your head against concrete inputs;
   - comparing checked and used values;
   - evaluating documented policies against code preconditions;
   - tracing consumers to their final responses.

   Folding these into structural dimensions would dilute both. The dimension costs
   about 170k tokens and 32 tool calls per run. That is comparable to a full
   structural dimension, and it finds real defects that survive human review
   (extra #1 above is still on `main`).
2. **Strengthen `adr-compliance`.** Add: "Compare each decision of a new or changed
   ADR with every other decision in it, and each requirement with the spec's edge
   cases and entity definitions." That makes G4 a structural catch as well.
3. **Strengthen `maintainability`.** Add a dead-code check for new declarations and
   test fixtures (G1).

Expect the dimension to report 5–7 findings per run even on an approved head. In
this evaluation they were real but outside the ground truth. Consolidation (Step 3)
should keep verifying them by their concrete inputs; the workflow now says so.

## Workflow integration

- `.github/workflows/architecture-review.md` changes:
  - description and intro: eight → nine dimensions;
  - Step 2 table: new row `behavioral-probe`;
  - Step 3: verify behavioral-probe findings by their concrete input, and keep the
    input when merging;
  - `result.json` example: new key `"behavioral-probe": "assessed"`;
  - Output Format: new `### Behavioral Probe` section and a 🔴 criterion for a
    concrete input reaching a forbidden outcome;
  - Important Guidelines: dimension 9 exception to "no individual lines";
  - the agent section itself.
- `.github/scripts/speckit-automation.cjs`: `reviewDimensions` gains
  `behavioral-probe`.
- `.github/scripts/architecture-review-automation.cjs`: `### Behavioral Probe` is
  now a required section.
- Tests now cover accepting a `behavioral-probe` finding, rejecting a result that
  omits the dimension, and rejecting a body that lacks the section.
  `node --test .github/scripts/*.test.cjs` passes (109/109). This is the same set
  as CI's "Automation guards" step.
- `CONTRIBUTING.md` lists nine dimensions.
- **`architecture-review.lock.yml` is not regenerated.** The prompt body is
  `runtime-import`ed. However, the lock file pins `frontmatter_hash` and
  `body_hash`, plus the description string, and CI runs `gh aw compile … --strict`
  and checks that every workflow is compiled. Installing the `gh-aw` extension was
  denied in this session, so run
  `gh extension install github/gh-aw --pin v1.0.1 && gh aw compile architecture-review`
  and commit the lock file before merging.

## Reproducing

```bash
# per commit: isolated clone + brief
git init -q wt/<c> && git -C wt/<c> fetch -q --no-tags --update-shallow <repo> <head>:refs/heads/pr-head <base>:refs/heads/base
git -C wt/<c> checkout -q pr-head
docs/evals/architecture-review/harness/make-brief.sh wt/<c> 293 "feat: support agent subjects in impersonation"
docs/evals/architecture-review/harness/render-task.sh docs/evals/architecture-review/behavioral-probe-dimension.md wt/<c> runs/<v>/<c>-r1.md
# start a fresh sub-agent with the printed task; score against the PR #293 review comments
```
