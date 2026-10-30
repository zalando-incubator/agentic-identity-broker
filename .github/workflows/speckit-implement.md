---
name: Spec Kit Implement
description: Complete a reviewed feature on its existing draft PR.
intent: Deliver the approved feature with all required tasks and validation complete.
on:
  workflow_call:
    inputs:
      phase:
        type: string
        default: implement
      pr:
        type: number
        required: true
      comment:
        type: number
        default: 0
imports:
  - shared/speckit-common.md
  - shared/speckit-phase-instructions.md
timeout-minutes: 180
network:
  allowed:
    - local
    - containers
    - linux-distros
    - api.osv.dev
    - openid.net
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
      - "configs/**"
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

Require `context.phase` to be `implement`. Do not run any other phase.
Update `AGENTS.md` files when a task requires it, such as the ADR index,
glossary, or area guidance, and limit those edits to what the feature adds.
