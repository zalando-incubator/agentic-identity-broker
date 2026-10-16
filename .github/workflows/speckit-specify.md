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
      - ".specify/feature.json"
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

Set `GIT_BRANCH_NAME` to `context.branch` for the mandatory feature-branch hook.
Create exactly one feature. Preserve `.specify/feature.json` and the generated
specification and checklists.

Create a draft PR on `context.branch`. Its body must start with a short summary
of the feature to be built, not a summary of the automation. Include:

- `Related issue: https://github.com/<context.repository>/issues/<context.issue>`
- `Specification: https://github.com/<context.repository>/blob/<context.branch>/<feature_directory>/spec.md`
- The exact tracking marker `<!-- speckit-issue: N -->`, where N is the issue number.

Do not create another PR if the branch already has one. Leave clarification,
planning, and implementation to subsequent sessions.
