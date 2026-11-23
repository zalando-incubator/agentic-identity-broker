---
name: Spec Kit Specify
description: Open one draft feature PR from an issue, starting with the feature's specification.
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
  # The App opens the PR, so the pipeline acts under one identity and the new PR starts ordinary CI.
  github-app:
    client-id: ${{ vars.AGENTIC_WORKFLOWS_APP_CLIENT_ID }}
    private-key: ${{ secrets.AGENTIC_WORKFLOWS_APP_PRIVATE_KEY }}
    owner: ${{ github.repository_owner }}
    repositories: ["${{ github.repository }}"]
  report-failure-as-issue:
    - "!report_incomplete"
  threat-detection:
    continue-on-error: false
  create-pull-request:
    github-token-for-extra-empty-commit: none
    draft: true
    title-prefix: "[spec-driven] "
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
and publish the draft PR so the separate clarification session can resolve them.
Do not call `report_incomplete` solely because those questions remain.
Technical failures that prevent producing the specification still block the run.

Set `GIT_BRANCH_NAME` to `context.branch` for the mandatory feature-branch hook.
Create exactly one new feature for this issue.
Use `.specify/feature.json` only as ignored local session state.
Never force-add or commit that file. Include only the generated specification
and checklists in the PR.
Do not modify the previously selected feature's files.

When calling `create_pull_request`, omit `stack_position` and `stack_root`.
This is a standalone PR, not a stacked PR. Do not populate unused optional
fields with zero or empty-string placeholders.

Create a draft PR on `context.branch`. This PR delivers the whole feature: the
same PR later receives the plan, the implementation, and its review. Describe
the feature it delivers, never the documents of the current phase.

Title the PR with the feature's name, such as `Notify agents before an approval
expires`. The workflow adds the title prefix. Do not use the words
specification, spec, specify, or draft in the title.

Start the body with a `## Feature` section of two to four sentences. Say what
the feature lets its users do once it ships, who benefits, and its scope. Write
it as the target of this PR. Do not call the PR a specification, a spec-only
change, or a first step, and do not summarize the automation.

Do not add a stage, status, or command section. The pipeline adds the
`## Feature stage` and `## Maintainer commands` sections to the body and keeps
them current.

After the feature section, include:

- `Related issue: https://github.com/<context.repository>/issues/<context.issue>`
- `Specification: https://github.com/<context.repository>/blob/<context.branch>/<feature_directory>/spec.md`
- The exact tracking marker `<!-- speckit-issue: N -->`, where N is the issue number.
- The exact feature marker `<!-- speckit-feature: specs/NNN-feature-name -->`,
  using the new feature's actual relative directory.

If an earlier draft PR for this issue was closed without merging and its
branch deleted, create a fresh draft PR on `context.branch` from the current
default branch. Do not reuse the old PR or its artifacts. Do not create a
second PR while one is open or the branch still exists. Leave clarification,
planning, and implementation to subsequent sessions.
