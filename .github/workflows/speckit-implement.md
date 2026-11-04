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
      validation:
        type: number
        default: 0
  # A repair run is started by the Container tests run that the App dispatched, so the App's bot is its actor.
  # speckit-assisted.yml routes a repair only for a base implementation that a maintainer started.
  bots: ["zalando-incubator-agent-workflows[bot]"]
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
  actions: read
  contents: read
  issues: read
  pull-requests: read
steps:
  # CI ran the pushed code without privileges; its test reports reach the sandbox only as data.
  - name: Download the failed CI run's test reports
    if: inputs.validation != 0
    uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2
    with:
      run-id: ${{ inputs.validation }}
      github-token: ${{ github.token }}
      pattern: "*-test-results"
      path: /tmp/gh-aw/speckit/validation
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
