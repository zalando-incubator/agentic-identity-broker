---
name: Spec Kit Specify
description: Create one draft feature specification PR from an issue.
intent: Give maintainers a reviewable specification derived only from the requested feature.
on:
  workflow_call:
    inputs:
      phase:
        type: string
        default: specify
      issue:
        type: number
        required: true
      comment:
        type: number
        default: 0
imports:
  - shared/speckit-common.md
timeout-minutes: 40
permissions:
  contents: read
  issues: read
  pull-requests: read
safe-outputs:
  threat-detection:
    continue-on-error: false
  create-pull-request:
    draft: true
    title-prefix: "[Spec Kit] "
    preserve-branch-name: true
    allowed-branches: ["speckit/issue-*"]
    base-branch: ${{ github.event.repository.default_branch }}
    allowed-files:
      - "specs/**"
    protected-files: request-review
    fallback-as-issue: false
    if-no-changes: error
    auto-close-issue: false
---

# Specify

Require `context.phase` to be `specify`. Run `/speckit-specify` with exactly
the issue title, a blank line, and the issue body as its feature-description
parameter. Do not fetch, read, include, or infer requirements from issue
comments. Do not follow the command's handoff to clarify in this session.

Unanswered product questions are an expected handoff, not a failed specify run.
Record them as NEEDS CLARIFICATION, keep the affected checklist items incomplete,
and publish the draft PR so the separate clarification session can ask them.
Do not call `report_incomplete` solely because those questions remain.
Technical failures that prevent producing the specification still block the run.

Set `GIT_BRANCH_NAME` to `context.branch` for the mandatory feature-branch hook.
Create exactly one new feature for this issue.
Use `.specify/feature.json` only as ignored local session state.
Never force-add or commit that file. Include only the generated specification
and checklists in the PR.
Do not modify the previously selected feature's files.

Create a draft PR on `context.branch`. Its body must start with a short summary
of the feature to be built, not a summary of the automation. Include:

- `Related issue: https://github.com/<context.repository>/issues/<context.issue>`
- `Specification: https://github.com/<context.repository>/blob/<context.branch>/<feature_directory>/spec.md`
- The exact tracking marker `<!-- speckit-issue: N -->`, where N is the issue number.
- The exact feature marker `<!-- speckit-feature: specs/NNN-feature-name -->`,
  using the new feature's actual relative directory.
- A short "Maintainer commands" section. List these commands, each with its
  one-line usage. A command must start a new PR comment.
  - `/speckit-clarify <answers>` answers the pending questions.
    `/speckit-clarify [focus]` starts a new clarification.
  - `/speckit-plan [technical guidance]` plans, creates tasks, and analyzes.
  - `/speckit-implement [guidance]` implements after a clean reanalysis.

If an earlier draft PR for this issue was closed without merging and its
branch deleted, create a fresh draft PR on `context.branch` from the current
default branch. Do not reuse the old PR or its artifacts. Do not create a
second PR while one is open or the branch still exists. Leave clarification,
planning, and implementation to subsequent sessions.
