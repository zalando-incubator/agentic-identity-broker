# Specification Quality Checklist: Policy-Driven Tool Approvals with Decision Models

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-11-23
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- **Implementation details.** The functional requirements, user stories, and success criteria
  name no language, framework, or code structure. The example journeys contain configuration
  keys, HTTP exchanges, and a policy excerpt, because Constitution Principle XIV requires them.
  The spec identifies key names, wire encoding, and log field names as examples. The planning
  phase sets them (API-006).
- **Audience.** The users are the operator and the approver. The policy excerpt and the decision
  request are for reviewers of the security model. Each journey ends with an observable result
  in plain language.
- **Examples.** All YAML and JSON blocks parse. The OPA library of the repository evaluated the
  policy excerpt against the inputs of journeys 1 to 3. The results agree with the journeys. The
  spec has no wireframe, because the feature changes no page.
- **No clarification markers.** The assumptions record each decision that has more than one
  possible answer. The section "Decisions to review first" lists the most important decisions.
- **Revisions.** On 2026-11-23, three revisions changed the first draft. First, the model gets
  the tool definitions that the servers list, and the operator describes no tools. Second,
  disclosure became one deployment-wide setting, and entries for each server became optional
  exceptions. Third, all prose obeys the Simple English rules of the project skill.
- **Revisions of 2026-11-26.** The stakeholder narrowed the scope. First, the configuration has
  no entry for a server or a tool: the optional exceptions, the server description, and the
  replacement tool description are removed, because an operator cannot maintain metadata about
  externally managed servers. Second, the approver sees no new element: the policy derives the
  existing description and risk level from the answers, and the broker, its API, its database,
  and the approval page do not change. Third, the organization values and the disclosure
  settings are keys of the service configuration, and the product has a default question set.
  The spec has no catalog file.
- **Language.** The prose uses the structural rules of Simplified Technical English in the
  pragmatic mode. Each clause of a Given/When/Then scenario counts as one sentence. Code,
  identifiers, quoted source text, and interface text did not change.
- **Research.** [decision-model-research.md](../decision-model-research.md) records the sources
  for the requirements that refer to `R-01` to `R-20`.

## Decisions to review first

These decisions change the scope or the security model:

- The model gets the tool definition that the MCP server lists. A Docker test of agentgateway `v1.5.0` and `v1.7.0` showed that ExtProc gets the `tools/list` response (research.md D-02).
- If the deployment-wide setting permits it, the system sends argument values of all servers to the decision endpoint.
- The system does not send the static policy risk level to the model.
- An automatic approval creates no approval record. It is visible only in the audit log.
- The approver sees only the description and the risk level of the policy. The page does not show that a model contributed to them.
- The product has a default question set. A release that changes a default question stops automatic approval until a new evaluation run.
- Automatic approval requires the evaluation record of User Story 5.
