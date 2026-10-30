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
  report-failure-as-issue:
    - "!report_incomplete"
  threat-detection:
    continue-on-error: false
  push-to-pull-request-branch:
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
  add-comment:
    target: ${{ inputs.pr }}
    discussions: false
    max: 1
---

Require `context.phase` to be `apply-review`. Do not run any other phase.
Apply only the items that `context.review.items` lists; the maintainer's
ticks in the proposal are the sign-off for exactly those items.
