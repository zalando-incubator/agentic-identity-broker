---
name: Spec Kit Apply Review
description: Apply the remediation items a maintainer selected in the proposal comment.
intent: Deliver the maintainer-approved remediation of architecture review findings, including contract and ADR changes.
on:
  workflow_call:
    inputs:
      phase:
        type: string
        default: apply-review
      pr:
        type: number
        required: true
      proposal:
        type: number
        required: true
imports:
  - shared/speckit-common.md
  - shared/speckit-review-instructions.md
timeout-minutes: 150
network:
  allowed:
    - local
    - containers
    - linux-distros
    - api.osv.dev
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
      - "internal/**"
      - "cmd/**"
      - "web/**"
      - "tests/**"
      - "infra/**"
      - "api/**"
      - "charts/**"
      - "migrations/**"
      - "mocks/**"
      - "examples/**"
      - "docs/**"
      - "coding-standards/**"
      - "adrs/**"
      - "specs/**"
      - "tools/**"
      - "build/**"
      - "go.mod"
      - "go.sum"
      - "justfile"
      - "Dockerfile*"
      - "ARCHITECTURE.md"
      - "AGENTS.md"
      - "**/AGENTS.md"
      - ".gitignore"
      - ".dockerignore"
    protected-files: request-review
    fallback-as-pull-request: false
  # Recommended remediations can defer a removal to a tracked follow-up.
  create-issue:
    title-prefix: "[PR follow-up] "
    labels: [automation, follow-up]
    deduplicate-by-title: true
    max: 3
  add-comment:
    target: ${{ inputs.pr }}
    discussions: false
    max: 1
---

Require `context.phase` to be `apply-review`. Do not run any other phase.
Apply only the items that `context.review.items` lists; the maintainer's
ticks in the proposal are the sign-off for exactly those items.
