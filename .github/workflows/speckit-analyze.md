---
name: Spec Kit Analyze
description: Analyze feature artifacts without permission to push changes.
intent: Identify design inconsistencies before implementation without modifying the design.
on:
  workflow_call:
    inputs:
      phase:
        type: string
        required: true
      pr:
        type: number
        required: true
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
  add-comment:
    target: ${{ inputs.pr }}
    discussions: false
    max: 1
---

Require `context.phase` to be `analyze` or `reanalyze`. Do not edit tracked
files or request branch updates. This worker has no push safe output.
