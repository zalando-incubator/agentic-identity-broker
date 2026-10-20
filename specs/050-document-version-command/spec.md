# Feature Specification: Document the Broker Version Command

**Feature Branch**: `speckit/issue-248`

**Created**: 2026-10-20

**Status**: Draft — clarification required

**Input**: User description: "Smoke test: document the version command

Document how an operator displays the broker version. Include a concrete command and expected output. Do not change runtime behavior."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Identify the Broker Version (Priority: P1)

As an operator, I want documented instructions for displaying the broker version so that I can identify the software I am operating without changing its behavior.

**Why this priority**: A concrete command paired with its expected output is the entire requested operator-facing outcome.

**Independent Test**: An operator follows the documentation using the stated prerequisites, runs the documented command, and compares the result with the documented output without changing the broker.

**Acceptance Scenarios**:

1. **Given** an operator with the broker artifact described in the documentation and the stated prerequisites, **When** the operator follows the documented version-display command, **Then** the output identifies the broker version and matches the documented output format, allowing for explicitly identified version-specific values.
2. **Given** an operator whose broker version differs from the documentation's example, **When** the operator compares the command result with the example, **Then** the documentation makes clear which value identifies their version and does not imply that all installations must return the example version.

Each scenario retains the constitution's required 1:1 acceptance-test mapping for subsequent phases. This specification does not authorize a testing exception or runtime change.

### Edge Cases

- The installed version differs from the example: identify version-specific values explicitly rather than treating the sample as universal.
- The executable is not on the operator's command search path: state the location or prerequisite needed for the documented invocation.
- The checked-out broker does not expose the assumed version command: stop documentation delivery for clarification rather than document an invented command or add runtime functionality.

## Concrete Example Journeys *(mandatory)*

### Journey 1 — An operator identifies the broker version (User Story 1, Scenarios 1–2)

**Cast**: Operator Sam has the broker artifact covered by the documentation. The illustrative version is `1.2.3`; it is synthetic, not a claim about an existing release or observed command output.

**Step 1 — Find instructions**: Sam opens the operator documentation and finds the version-display instructions, including the required artifact and command location.

**Step 2 — Run the command**: Sam copies the documented command. The concrete invocation and exact expected output cannot yet be supplied truthfully: the reviewed broker CLI source exposes neither a version subcommand nor a version flag. The missing command/output pair is the clarification below, not an implied new runtime capability.

**Step 3 — Interpret the result**: Once an existing supported mechanism is confirmed, Sam compares its output with the documented example and identifies the version value. For an example artifact identified as `1.2.3`, that value must be `1.2.3`; another artifact may report a different value without indicating failure.

**Observable result**: Sam can name the broker version using one documented command and its matching output example. This journey is incomplete until the existing command and output are confirmed. No HTTP, browser UI, configuration change, or JWT interaction is introduced by this documentation request.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Operator documentation MUST explain how to display the broker version using an existing supported mechanism.
- **FR-002**: Documentation MUST include a concrete, copyable command, its prerequisites, and a matching expected-output example verified against that mechanism.
- **FR-003**: Documentation MUST identify the version-bearing value and distinguish example values from values that depend on the operator's artifact.
- **FR-004**: Delivery MUST NOT change runtime behavior, add a command or flag, alter version metadata generation, or modify API contracts, configuration, deployment behavior, or persistence.
- **FR-005**: Documentation MUST NOT present an unsupported invocation or invented output as existing broker behavior.
- **FR-006**: The existing supported version-display mechanism MUST be confirmed before the concrete journey and documentation are finalized. [NEEDS CLARIFICATION: The broker CLI source has no version command or flag. Which existing command and expected output should be documented while preserving the documentation-only scope?]

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can identify the broker version by following one documented command, with no source-code inspection or application changes.
- **SC-002**: The documentation contains at least one concrete command and matching expected-output example, with every artifact-dependent value explicitly explained.
- **SC-003**: Both acceptance scenarios can be completed using only the documentation and its stated prerequisites.
- **SC-004**: Delivery introduces zero runtime behavior changes.

## Assumptions

- The issue requests documentation of existing behavior, not implementation of a missing version capability.
- The operator has access to the broker artifact and a terminal; exact artifact prerequisites depend on the confirmed existing mechanism.
- Documentation belongs in the existing operator-facing documentation; selecting its precise page is left to planning.
- No new domain entities, configuration parameters, APIs, database changes, or frontend changes are needed.

## Dependencies and Open Questions

- **Evidence**: `cmd/agentic-identity-broker/root.go` defines the root command and its flags but has no version field, version subcommand, or version flag; `cmd/agentic-identity-broker/main.go` delegates to that command. No broker version-command registration was found under `cmd/`.
- **Required clarification**: Provide an existing supported command and its expected output, or explicitly reconsider the issue scope. A new runtime capability is not authorized by this issue.
- The concrete example journey and FR-002 cannot be finalized until that clarification is resolved. No command or output is invented here, and no stakeholder decision is presumed.
