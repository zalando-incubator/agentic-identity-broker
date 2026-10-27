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

Require `context.phase` to be `plan`. Do not run any other phase.
