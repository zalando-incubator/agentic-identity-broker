# Data Model: Decision Model Assessment

**Feature**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md) | **Decisions**: [research.md](research.md)

The spec contains the entity diagram and the glossary terms. This document gives the fields, the
validation rules, and the location of each type. Only one type gets persistent storage: the
Assessment Summary on a tool approval.

## ExtProc types (`internal/extproc/assessment/`)

These types exist in memory only. They are not domain entities and need no typed identifier.

### Assessment Catalog

The source is the catalog file. ExtProc reads it one time at startup (D-11).

| Field | Type | Rules |
|---|---|---|
| `organization.name` | string | Required. Maximum 200 characters. |
| `organization.description` | string | Required. Maximum 1,000 characters. |
| `organization.internal_domains` | list of strings | Required. 1 to 50 entries. Each entry is a lowercase DNS name. |
| `questions` | list of Assessment Question | Required. 1 to 16 entries. Each name is unique. |
| `disclosure.arguments` | `all` or `none` | Required. No default. |
| `disclosure.withhold` | list of name patterns | Optional. Maximum 100 entries. The shared matcher for approval patterns reads them. |
| `servers` | map of Server Exception | Optional. The key is a target server name. |

### Assessment Question

| Field | Type | Rules |
|---|---|---|
| `name` | string | Required. Pattern `^[a-z][a-z0-9_]{0,63}$`. |
| `type` | `predicate` or `score` | Required. |
| `instructions` | string | Required. Maximum 500 characters. No placeholder characters `{{` or `}}`. |
| `yes_when`, `no_when` | string | Required for `predicate`. Not permitted for `score`. Maximum 500 characters each. No placeholder characters `{{` or `}}`. |
| `levels` | list of `label` and `description` | Required for `score`. 2 to 10 entries. Not permitted for `predicate`. Each `label` has 1 to 120 characters. Each `description` has 1 to 500 characters. No placeholder characters `{{` or `}}`. |
| `statement` | string | Required. 1 to 120 characters. Plain text in one line. No control characters. The rule is equal to the rule of `Finding.statement`, so that the broker accepts each catalog statement. |

### Server Exception

| Field | Type | Rules |
|---|---|---|
| `assess` | boolean | Optional. The default is `true`. |
| `arguments` | as in `disclosure` | Optional. It replaces the deployment-wide argument setting for this server. |
| `withhold` | as in `disclosure` | Optional. Its names apply in addition to the deployment-wide names. An exception cannot remove a deployment-wide name. An empty list changes nothing. |
| `description` | string | Optional. Maximum 500 characters. |
| `tools.<tool name>.description` | string | Optional. Maximum 2,000 characters. It replaces the description from the server. |

### Tool Definition

The source is a `tools/list` result as the client sees it through the gateway (D-02).

| Field | Type | Rules |
|---|---|---|
| `target_server_name` | string | From the gateway metadata. Part of the key. |
| `name` | string | Required. Maximum 255 characters. Part of the key. The name as the client sees it. On a route with two or more MCP targets, the gateway adds the target name as a prefix, for example `drive_drive.share_file` (D-02). |
| `description` | string | Optional. If the listing has none, the `tool` section of the context has no description. |
| `input_schema` | JSON object | Optional. The system uses only the entries in `properties`. |
| `annotations` | JSON object | Optional. The system keeps only the four hint values of the MCP specification that the listing contains. It adds no default hint value. |
| `written_by` | `server` or `operator` | `operator` if an exception replaces the description. |
| `tool_context` | fingerprint | Calculated when the definition or the catalog changes (D-06). |

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
| `server` | `name`, and `description` if an exception supplies it |
| `tool` | `written_by`, `name`, `description`, `annotations`, and `arguments` with the schema entry of each supplied and described argument |
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
| `question_set` | fingerprint | From the catalog. It does not include `statement` (FR-029). |
| `tool.written_by`, `tool.context` | string, fingerprint | Absent in the policy input for `excluded`, `definition_missing`, and `context_too_large`. |
| `evaluated` | boolean | True if the evaluation record contains the model, the question set, and the tool context. |
| `context` | flags | `withheld_arguments`, `oversized_arguments`, `undescribed_argument_count`. Absent in the policy input for `excluded`, `definition_missing`, and `context_too_large`. |
| `answers.<name>.status` | enumeration | `answered`, `refused`, `missing`, or `invalid`. Each catalog question has an entry for each assessment status. Without a response, the value is `missing`. |
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

### Assessment Summary (`internal/domain/storage/`)

A value object on `ToolApproval`. It is optional and does not change after creation (FR-037).

| Field | Type | Rules |
|---|---|---|
| `status` | `assessed` or `not_available` | Required. |
| `recommendation` | `routine` or `attention` | Optional. Permitted only with `assessed`. |
| `findings` | list of Finding | Optional. Maximum 16 entries. Permitted only with `assessed`. |
| `model` | string | Optional. 1 to 100 characters. No control characters. |
| `question_set` | string | Optional. Pattern `^qs-[0-9a-f]{64}$`. |

### Finding

| Field | Type | Rules |
|---|---|---|
| `question` | string | Required. Pattern `^[a-z][a-z0-9_]{0,63}$`. Unique in one summary. |
| `statement` | string | Required. 1 to 120 characters. One line. No control characters. |
| `state` | `indicated`, `not_indicated`, or `unclear` | Required. |

The domain service `approval.Service` validates these rules. A summary that breaks a rule causes
a validation error, and the service creates no approval.

### Storage

The next free migration number on `main` names the pair (D-09). This document uses `NNN`.
Migration `NNN_add_tool_approval_assessment` adds one column to `tool_approvals`:

| Column | Type | Rules |
|---|---|---|
| `assessment` | JSONB | Nullable. No default. `NULL` shows "no assessment information". |

The down migration removes the column only if no row has a summary. If one row has a value that
is not `NULL`, the down migration stops with an error and changes nothing, as the down migration
`039_add_protected_resource_discovery` does for its data. Thus, a rollback cannot remove
summaries without a decision of the operator, who must first set the values to `NULL` (Principle
IX). The two migrations are atomic. The in-memory repository
and the PostgreSQL repository store and return the value. The PostgreSQL repository adds the
column to each explicit column list.

## State and lifetime

| Data | Lifetime |
|---|---|
| Assessment Catalog, Evaluation Record | From startup to stop of the ExtProc process |
| Tool Definition | From a listing to the next listing of that tool, or to removal from the full store |
| Assessment Context, Assessment, raw model response | One tool call |
| Assessment Summary | The lifetime of the tool approval record |

The feature adds no state transition to `ToolApproval`.
