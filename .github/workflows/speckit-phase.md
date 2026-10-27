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
  threat-detection:
    continue-on-error: false
  push-to-pull-request-branch:
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
