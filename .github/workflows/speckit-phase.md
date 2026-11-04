---
name: Spec Kit Phase
description: Run one isolated specification, planning, or analysis phase.
intent: Produce a clarified and consistent feature design before implementation.
on:
  workflow_call:
    inputs:
      phase:
        type: string
        required: true
      pr:
        type: number
        required: true
      comment:
        type: number
        default: 0
      review:
        type: number
        default: 0
imports:
  - shared/speckit-common.md
  - shared/speckit-phase-instructions.md
timeout-minutes: 40
permissions:
  contents: read
  issues: read
  pull-requests: read
safe-outputs:
  # Pushes through this App start ordinary PR CI, which GITHUB_TOKEN pushes cannot.
  github-app:
    client-id: ${{ vars.AGENTIC_WORKFLOWS_APP_CLIENT_ID }}
    private-key: ${{ secrets.AGENTIC_WORKFLOWS_APP_PRIVATE_KEY }}
    owner: zalando-incubator
    repositories: [agentic-identity-broker]
  report-failure-as-issue:
    - "!report_incomplete"
  threat-detection:
    continue-on-error: false
  push-to-pull-request-branch:
    github-token-for-extra-empty-commit: none
    # Reading branch protection needs Administration, which the App does not hold; Spec Kit branches are unprotected.
    check-branch-protection: false
    target: ${{ inputs.pr }}
    required-title-prefix: "[Spec Kit] "
    base-branch: ${{ github.event.repository.default_branch }}
    allowed-files:
      - "specs/**"
    protected-files: request-review
    fallback-as-pull-request: false
  add-comment:
    target: ${{ inputs.pr }}
    discussions: false
    max: 1
---

Require `context.phase` to be `clarify`, `tasks`, or `remediate`.
This worker can only edit feature documents.
