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
permissions:
  contents: read
  issues: read
  pull-requests: read
safe-outputs:
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
      - ".gitignore"
      - ".dockerignore"
    excluded-files:
      - "**/AGENTS.md"
    protected-files: request-review
    fallback-as-pull-request: false
  add-comment:
    target: ${{ inputs.pr }}
    discussions: false
    max: 1
---

Require `context.phase` to be `implement`. Do not run any other phase.
