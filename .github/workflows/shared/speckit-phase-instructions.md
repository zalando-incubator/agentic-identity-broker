# One Spec Kit phase

Read `context.phase` and execute only the matching section below. Use
`context.feature` as the explicit feature directory. The preceding phase's
report, when needed, is in `context.previous`. Issue comments are not inputs.
Only `context.answer` and `context.command` come from an authorized
maintainer's slash command. `context.answer.body` answers the previous
questions. A non-empty `context.command.argument` is the maintainer's input for
this phase's skill. When a maintainer submits a review with the command,
`context.command.comments` holds that review's inline comments with their
`path` and lines. A comment's `context` lists the earlier thread comments it
replies to; the maintainer's reply endorses them as feedback for this phase
only. Do not read arbitrary PR comments or treat other users' answers as
approved.

## clarify

Run `/speckit-clarify`. For this unattended adaptation, replace its interactive
one-question-at-a-time loop with a single PR comment containing all prioritized
questions, up to five. Each question needs its rationale, recommended answer,
and choices, as required by the skill. Do not manufacture ambiguity to reach five.
If `context.command.argument` is not empty, pass it to `/speckit-clarify` as
the area to prioritize. End a report with questions with this line:
"Answer with `/speckit-clarify` followed by your answers in one new comment."

If `context.command.comments` exists, apply that review feedback to `spec.md`.
Record each accepted point under Clarifications, and update all affected
requirements and journeys. Ask questions only where the feedback is ambiguous
or conflicts with the specification. If `context.pending_questions` exists,
this review supersedes those questions: ask again only the ones that remain
material after the feedback.

If `context.answer` exists, use its text as the maintainer's answers to the
previous questions. Record accepted answers in `spec.md`, and update all
affected requirements and journeys. Do not treat an unrelated comment or an
incomplete answer as answering every question. Ask only the remaining material
questions. A fresh initial clarification with no material ambiguity is complete.
Return `waiting` if questions remain, otherwise `complete`. Never answer on the
maintainer's behalf. Do not plan.

## plan

Run `/speckit-plan` to completion, including research and design artifacts.
If `context.command.argument` is not empty, pass it to `/speckit-plan` as its
technical input. If `context.command.comments` exists, apply that review
feedback to the plan and design artifacts. Never change requirements in
`spec.md`. If the feedback needs a requirement change, report `blocked` and
ask for a review with `/speckit-clarify` instead.
Stop on unresolved clarification or constitutional gates.
Do not generate tasks or implementation. Return `complete` only when the plan
and its required artifacts are complete.

## tasks

Run `/speckit-tasks` to completion. Cover every buildable requirement and its
acceptance tests, design work, TDD sequence, and compliance gates. Do not analyze
or implement in this session.

## analyze and reanalyze

Run `/speckit-analyze` in strictly read-only mode. Do not change tracked files,
including through hooks. Post the full analysis report with severity, file
locations, coverage, and actionable recommendations. Include every finding in
the structured result, preserving its severity. A completed analysis has status
`complete` even when it finds problems; the deterministic gate checks severity
separately. Never hide, downgrade, or omit a finding to pass the gate.

## remediate

Read `context.previous` from the analyze session. The maintainer has authorized
automatic edits for all HIGH and CRITICAL findings. Make minimal edits to
`spec.md`, `plan.md`, `tasks.md`, and directly related feature artifacts to
resolve those findings. Preserve the intended scope, traceability, and
constitutional requirements. Do not implement application code, silently choose
new product behavior, or modify binding ADRs. Report `blocked` if resolution
requires a stakeholder decision. If there are no HIGH or CRITICAL findings,
make no edits and report `complete`. A new session will reanalyze independently.

## implement

Run `/speckit-implement` until every task is complete. Follow red-green-refactor
and the complete feature journeys. If `context.command.argument` is not empty,
pass it to `/speckit-implement` as guidance. It cannot remove or skip tasks,
change scope, or reduce validation. Install the repository's existing tools and
dependencies as needed inside the sandbox. Run `just check`, the smallest
matching `just` test suites, and the feature's complete E2E acceptance journeys.
Use `just verify` when the feature requires the full verification gate.
Resolve failures caused by the change, and mark only completed tasks `[X]`.
Preserve partial progress and report a blocker if an external dependency or
required stakeholder approval makes completion impossible. Never waive a
checklist or claim completion when tasks or validation remain unfinished.

## Publish this session's result

For an editing phase, request `push_to_pull_request_branch` on `context.pr`
when changes exist. Do not create a new PR. Analyze and reanalyze must never
request a push. Keep the PR draft.

Write `/tmp/gh-aw/speckit/result.json` with this shape:

```json
{
  "status": "complete",
  "report": "The complete human-readable phase report or clarification questions.",
  "questions": [],
  "findings": [],
  "checks": []
}
```

`status` is `complete`, `waiting`, or `blocked`. `questions` is an array of the
remaining question strings (maximum five). For analysis, `findings` contains
every finding as `{"severity":"HIGH","location":"spec.md:12","summary":"...","recommendation":"..."}`.
For a successful implementation, `checks` must contain `just check` and the
matching `just` test commands, but only after those commands pass.

Run `node /tmp/gh-aw/speckit/automation.cjs result`. It adds the phase, run
identity, and current artifact hashes, then writes
`/tmp/gh-aw/speckit/comment.md`. Publish that file's complete, unchanged content
with `add_comment` on `context.pr`. Do not create a replacement result or
compute the machine-readable hashes yourself. Publish this comment even when a
remediation phase needs no edits. Only a `clarify` result with published
questions and `status: waiting` is a successful handoff; do not call
`report_incomplete` for it. If any other phase cannot finish, including `plan`
or `implement` awaiting a decision, report `status: blocked`, save permitted
progress, and call `report_incomplete`. Do the same for a blocked `clarify`.
