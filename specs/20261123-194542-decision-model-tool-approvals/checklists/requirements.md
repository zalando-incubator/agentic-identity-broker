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
  and the approval page do not change. Third, the organization values and the
  questions are keys of the service configuration, and the product has a default question set.
  The spec has no catalog file. Fourth, the system always sends the arguments of a call as
  the agent sent them and the tool definition as the server listed it, and no setting withholds
  or filters them. Fifth, User Story 5 and each measurement are out of scope: the feature has no
  evaluation run, no case set, no evaluation record, and no fingerprint.
- **Language.** The prose uses the structural rules of Simplified Technical English in the
  pragmatic mode. Each clause of a Given/When/Then scenario counts as one sentence. Code,
  identifiers, quoted source text, and interface text did not change.
- **Research.** [decision-model-research.md](../decision-model-research.md) records the sources
  for the requirements that refer to `R-01` to `R-20`.

## Decisions to review first

These decisions change the scope or the security model:

- The model gets the tool definition that the MCP server lists. A Docker test of agentgateway `v1.5.0` and `v1.7.0` showed that ExtProc gets the `tools/list` response (research.md D-02).
- While assessment is enabled, the system sends the tool name and the full arguments of each assessed call to the decision endpoint, as the agent sent them. No setting withholds or filters them. Issue #206 asks for an explicit decision about argument disclosure, and the key `enabled` is that decision.
- The system does not send the static policy risk level to the model.
- An automatic approval creates no approval record and no separate audit record. The existing decision record of the service has the action and the answers. The decision log of the policy engine also has the full policy input, if the operator enables that log.
- The approver sees only the description and the risk level of the policy. The page does not show that a model contributed to them.
- The product has a default question set. No model measured the default questions. A release that changes a default question changes the answers, and the release notes must state it.
- One key selects one of four modes: `disabled` (default), `shadow`, `guidance`, and `auto_approve`.
- The feature measures no model, no question, and no policy. It has no evaluation run and no evaluation record. Automatic approval needs only the mode, an `allow` result of the policy after the assessment, and a complete assessment. The operator is responsible for the thresholds.
