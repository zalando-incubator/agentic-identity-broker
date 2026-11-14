---
name: Spec Kit Plan
description: Create the feature plan in an isolated Opus session.
intent: Turn the clarified feature into an actionable and constitution-compliant design.
on:
  workflow_call:
    inputs:
      phase:
        type: string
        default: plan
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
model: copilot/claude-opus-5.5
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
    owner: ${{ github.repository_owner }}
    repositories: ["${{ github.repository }}"]
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

Require `context.phase` to be `plan`. Do not run any other phase.
