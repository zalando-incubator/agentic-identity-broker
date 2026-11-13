# Architecture review evaluations

These cases check whether an architecture-review dimension finds the issues human
reviewers found by hand. The harness runs one dimension at a time, blind, against
the exact commits the humans reviewed.

## Cases: PR #293 (agent-subject impersonation)

jbspeakr and x418 found the following issues by hand. The full comments are on the
PR (`gh api repos/zalando-incubator/agentic-identity-broker/pulls/293/comments`).
The existing eight dimensions missed all of them. `behavioral-probe` was designed
for them.

| # | Comment | Location | Finding |
|---|---|---|---|
| G1 | 4466792705 | `tests/e2e/fixtures/impersonation_rules.go:43` | `HumanImpersonationRule`, `AgentImpersonationRule`, `DualImpersonationRule` have no callers. |
| G2 | 4466927172 | `internal/domain/impersonation/subject_category.go:52` | A token with `sub=A`, `agent_id=A` targeting agent B is classified human, so an agent identity gets human delegation and a consent link. |
| G3 | 4467989697 | `docs/configuration.md:1613` | The example predicate `subject_token.sub == request.agent_id` is always true for an agent request, so it limits nothing. An allowlist is needed (also ADR 045 decision 2a, US1.4). |
| G4 | 4467989731 | `adrs/045-agent-subject-impersonation.md:25` | Decision 2 still says another agent's UUID is human, which contradicts decision 2b and the code. The same text is in FR-003, the spec edge case, the Subject Category entry and the `AGENTS.md` glossary. Decision 8's list of denials misses 2b. |
| G5 | 4467989743 | `internal/domain/impersonation/service.go:231` | The cross-agent check uses the raw `sub`, but delegation and minting use the extracted principal. With `principal_expression` mapping to another agent's UUID, a token is still minted for that agent. |
| G6 | 4468451990 | `docs/configuration.md:1613` | The docs say principal mapping never selects the category, but a principal mapped to another agent's UUID now produces `agent`. The same sentence is in `docs/reference/token-exchange.md` and `ARCHITECTURE.md`. |
| G7 | 4468503625 | `internal/domain/tokenexchange/human_subject.go:34` | The new 403 without `error_uri` makes ExtProc (`exchangeErrorResponse`) return 500 `token_exchange_failed` and log an ERROR. Before the change, ExtProc returned a 403 consent challenge. Deferred to #416, so it is still present on the control commit. |
| G8 | 4469141987 | `adrs/045-agent-subject-impersonation.md:34` | Decision 8 says the new denials only affect identities that collide with an agent UUID. `client_credentials` tokens whose `client_id` is the agent's UUID also have `sub == agent_id`. |

Review each commit against the main commit it was based on. Each base is the
second parent of the latest merge on the commit's first-parent line:
`git rev-parse $(git log --merges --first-parent -1 --format=%H <commit>)^2`.

| Commit | Base | Expected |
|---|---|---|
| `d6d0cabdab6636761bf2bbab01bacaa6b24716d7` | `f63964b632bd14f7ccb002cd5b29a4ed9f140a00` | G1, G2 |
| `9d25952ca93ee7369bf29458c0b8f71e848bd935` | `f63964b632bd14f7ccb002cd5b29a4ed9f140a00` | G3, G4, G5 |
| `d177d859139bbac4bfece153c1044a9ca1d753b8` | `77b8206a4aa2c26d5f7a3d359ad3f0df4dae9d3f` | G6, G7 |
| `7f54db65dbee60b4684e07f1535d42a341da69d1` | `77b8206a4aa2c26d5f7a3d359ad3f0df4dae9d3f` | G8 |
| `b3e2335032bd9361980f72ae769bb9dd4cce5530` (control, approved head) | `77b8206a4aa2c26d5f7a3d359ad3f0df4dae9d3f` | none of the fixed items (G1–G6, G8). G7 is still open. |

## How to run

1. **Fetch the PR commits:**

   ```bash
   git fetch origin pull/293/head
   ```

2. **Build an isolated clone per commit.** The clone contains only the head and
   base ancestry and has no remote, so a run cannot see later fix commits or
   review comments:

   ```bash
   git init -q wt/<c>
   git -C wt/<c> fetch -q --no-tags --update-shallow "$(git rev-parse --show-toplevel)" \
     <commit>:refs/heads/pr-head <base>:refs/heads/base
   git -C wt/<c> checkout -q pr-head
   ```

3. **Write the brief.** It follows Step 1 of the workflow: the head and base SHAs,
   the changed files by area, ADR provenance, and the inspection commands. The PR
   body is left out.

   ```bash
   docs/evals/architecture-review/harness/make-brief.sh wt/<c> 293 "feat: support agent subjects in impersonation"
   ```

4. **Render the task.** This extracts the dimension section from
   `.github/workflows/architecture-review.md` into the clone and prints the
   sub-agent message:

   ```bash
   docs/evals/architecture-review/harness/render-task.sh behavioral-probe wt/<c> out/<c>-r1.md
   ```

5. **Run it blind.** Start a fresh sub-agent with no shared context (Claude Code
   `Agent`, `general-purpose`). Its only input is the printed message. Never give
   it the ground truth. Run each commit at least twice to see the variance.

6. **Judge the results yourself, not with the reviewer.**
   - **Hit:** the same underlying problem with a correct location or mechanism.
   - **Partial:** the right symptom with the wrong mechanism.
   - Classify every other finding as real (new), wrong, or noise (a duplicate).
     Read the code for any finding you count.
   - On the control commit, a fixed item that is raised again counts as a false
     positive. G7 is still open there, so raising it is correct.
   - Record the sub-agent's tokens and tool calls as the cost.

Change a dimension only with general review techniques, never with hints toward a
specific finding. To check for overfitting, remove repository-specific pointers
from the prompt, or tune on three commits and confirm on the fourth.
