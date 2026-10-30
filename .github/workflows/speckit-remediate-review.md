---
name: Spec Kit Remediate Review
description: Apply the architecture review's unambiguous fixes to the implemented feature.
intent: Resolve review findings that need no maintainer decision without touching contracts, ADRs, schema, or the approved design.
on:
  workflow_call:
    inputs:
      phase:
        type: string
        default: remediate-review
      pr:
        type: number
        required: true
imports:
  - shared/speckit-common.md
  - shared/speckit-review-instructions.md
timeout-minutes: 120
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
      - "charts/**"
      - "mocks/**"
      - "examples/**"
      - "docs/**"
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

Require `context.phase` to be `remediate-review`. Do not run any other phase.
This worker cannot push changes under `api/`, `adrs/`, `migrations/`, or
`specs/`; a finding that needs them is reported, not resolved.
