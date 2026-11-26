# Data Model: Decision Model Assessment

**Feature**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md) | **Decisions**: [research.md](research.md)

The spec contains the entity diagram and the glossary terms. This document gives the fields, the
validation rules, and the location of each type. No type gets persistent storage. The broker has
no new type, and the feature adds no migration (DB-001).

## ExtProc types (`internal/extproc/assessment/`)

These types exist in memory only. They are not domain entities and need no typed identifier.

### Organization Context

The source is the ExtProc configuration: `authorization.assessment.organization` (D-17). ExtProc
reads it one time at startup.

| Field | Type | Rules |
|---|---|---|
| `name` | string | Required while assessment is enabled. Maximum 200 characters. |
| `description` | string | Required while assessment is enabled. Maximum 1,000 characters. |
| `internal_domains` | list of strings | Required while assessment is enabled. 1 to 50 entries. Each entry is a lowercase DNS name. |

### Disclosure

The source is the ExtProc configuration: `authorization.assessment.disclosure` (D-17).

| Field | Type | Rules |
|---|---|---|
| `arguments` | `all` or `none` | Required while assessment is enabled. No default. |
| `withhold` | list of name patterns | Optional. Maximum 100 entries. The shared matcher for approval patterns reads them. |

The configuration has no key for an individual server or tool. The operator cannot describe a
server, and cannot describe, correct, or replace a tool definition (FR-011, FR-013, FR-016).

### Question Set

The source is the ExtProc configuration: `authorization.assessment.questions` (D-17). The default
value of the key is the default question set of [spec.md](spec.md), Journey 1, step 2. ExtProc
reads the configuration one time at startup.

| Field | Type | Rules |
|---|---|---|
| `questions` | list of Assessment Question | 1 to 16 entries. Each name is unique. A list in the configuration replaces the full default list. |

The default question set obeys the rules of this document, and a test proves it (FR-028).

### Assessment Question

| Field | Type | Rules |
|---|---|---|
| `name` | string | Required. Pattern `^[a-z][a-z0-9_]{0,63}$`. |
| `type` | `predicate` or `score` | Required. |
| `instructions` | string | Required. Maximum 500 characters. No placeholder characters `{{` or `}}`. |
| `yes_when`, `no_when` | string | Required for `predicate`. Not permitted for `score`. Maximum 500 characters each. No placeholder characters `{{` or `}}`. |
| `levels` | list of `label` and `description` | Required for `score`. 2 to 10 entries. Not permitted for `predicate`. Each `label` has 1 to 120 characters. Each `description` has 1 to 500 characters. No placeholder characters `{{` or `}}`. |

A question has no text for the approver (FR-025).

### Tool Definition

The source is a `tools/list` result as the client sees it through the gateway (D-02).

| Field | Type | Rules |
|---|---|---|
| `target_server_name` | string | From the gateway metadata. Part of the key. |
| `name` | string | Required. Maximum 255 characters. Part of the key. The name as the client sees it. On a route with two or more MCP targets, the gateway adds the target name as a prefix, for example `drive_drive.share_file` (D-02). |
| `description` | string | Optional. If the listing has none, the `tool` section of the context has no description. |
| `input_schema` | JSON object | Optional. The system uses only the entries in `properties`. |
| `annotations` | JSON object | Optional. The system keeps only the four hint values of the MCP specification that the listing contains. It adds no default hint value. |
| `tool_context` | fingerprint | Calculated when the definition changes (D-06). |

The store keeps a maximum of 10,000 definitions. One definition has a maximum of 16 KiB of
canonical JSON. If the store is full, it removes the definition with the oldest listing time. A
larger definition replaces the stored entry with a marker. The status of a call to that tool is
then `context_too_large`. A listing that ExtProc cannot read removes all definitions of its target
server (D-02).

### Assessment Context

ExtProc builds this document for one call and discards it after the call (SR-007).

| Section | Fields |
|---|---|
| `organization` | `name`, `description`, `internal_domains` |
| `server` | `name` |
| `tool` | `name`, `description`, `annotations`, and `arguments` with the schema entry of each supplied and described argument |
| `call` | `arguments`, `withheld_arguments`, `oversized_arguments`, `undescribed_argument_count` |

Rules:

- The sections have the sequence of the table.
- One argument value has a maximum of 4 KiB of canonical JSON and a maximum of 8 levels. A larger value is oversized.
- The full context has a maximum of `max_context_bytes`.
- The names in `withheld_arguments` and `oversized_arguments` are names that the definition describes.
- In the model context, an empty `withheld_arguments` or `oversized_arguments` list is absent. `undescribed_argument_count` is always present. In the policy input, the key `context` is present only for the statuses that [contracts/policy-assessment.md](contracts/policy-assessment.md) section 2 lists. If it is present, each of the three fields is present.

### Assessment and Answer

| Field | Type | Rules |
|---|---|---|
| `status` | enumeration | One value of D-04. |
| `model` | string | The `model` value of the response. Absent in the policy input if no response arrived. |
| `question_set` | fingerprint | From the Organization Context, the Question Set, and the request framing (FR-029). |
| `tool.context` | fingerprint | Absent in the policy input for `definition_missing` and `context_too_large`. |
| `evaluated` | boolean | True if the evaluation record contains the model, the question set, and the tool context. |
| `context` | flags | `withheld_arguments`, `oversized_arguments`, `undescribed_argument_count`. Absent in the policy input for `definition_missing` and `context_too_large`. |
| `answers.<name>.status` | enumeration | `answered`, `refused`, `missing`, or `invalid`. Each question of the set has an entry for each assessment status. Without a response, the value is `missing`. |
| `answers.<name>.probability` | number | Only for an answered `predicate`. |
| `answers.<name>.probabilities`, `score`, `confidence` | list, number, number | Only for an answered `score`. The list has one value for each level. |

### Evaluation Record

The source is the evaluation run (D-08). ExtProc reads it one time at startup.

| Field | Type | Rules |
|---|---|---|
| `format` | integer | Required. The value is `1`. |
| `model` | string | Required. |
| `question_set` | fingerprint | Required. |
| `tool_contexts.<target server name>.<tool name>` | fingerprint | Required. A minimum of one entry. |

## Broker types

None. The existing `ToolApproval` keeps its fields. The create request carries the description
and the risk level of the policy result in the existing fields (FR-035, FR-038).

## State and lifetime

| Data | Lifetime |
|---|---|
| Organization Context, Disclosure, Question Set, Evaluation Record | From startup to stop of the ExtProc process |
| Tool Definition | From a listing to the next listing of that tool, or to removal from the full store |
| Assessment Context, Assessment, raw model response | One tool call |

The feature adds no state transition to `ToolApproval`.
