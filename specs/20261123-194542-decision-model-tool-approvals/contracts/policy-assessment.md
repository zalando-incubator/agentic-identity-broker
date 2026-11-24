# Contract: Policy Input and Policy Result for an Assessment

**Feature**: [spec.md](../spec.md) | **Decisions**: [research.md](../research.md) D-01, D-04, D-05

**Approval status**: Draft. The stakeholder must approve this contract before implementation
(API-006, Principle X).

This contract extends `specs/020-extproc-opa-authorization/contracts/opa-input-schema.md` and
`specs/026-extproc-approval-sync/contracts/opa-input-approval.md`. All existing input keys and
result fields keep their meaning.

## 1. Two evaluations

| Evaluation | Time | Input |
|---|---|---|
| First | For each request, as before this feature | The existing input. The key `assessment` is absent. |
| Second | Only after an assessment attempt (FR-001, FR-003) | The same input with the added key `assessment`. |

A policy author can use `not input.assessment` to find the first evaluation.

The second evaluation occurs for each status, including `excluded` and `definition_missing`. Thus,
a policy can deny a call or keep it for a person if no assessment is possible.

The input never contains approval records (FR-034).

## 2. Input key `assessment`

```json
{
  "assessment": {
    "status": "complete",
    "model": "jev-1.14.0",
    "question_set": "qs-<64 hexadecimal characters>",
    "evaluated": false,
    "tool": {
      "written_by": "server",
      "context": "tc-<64 hexadecimal characters>"
    },
    "context": {
      "withheld_arguments": ["file_id"],
      "oversized_arguments": [],
      "undescribed_argument_count": 0
    },
    "answers": {
      "leaves_organization": {"type": "predicate", "status": "answered", "probability": 0.99},
      "persistent_effect": {
        "type": "score",
        "status": "answered",
        "score": 1.2,
        "confidence": 0.6,
        "probabilities": [0.03, 0.74, 0.23]
      }
    }
  }
}
```

| Field | Values | Rules |
|---|---|---|
| `status` | `complete`, `incomplete`, `timeout`, `unavailable`, `context_too_large`, `definition_missing`, `excluded` | Always present. |
| `model` | string | The model that the response names. Absent if no response arrived. |
| `question_set` | fingerprint | Always present. |
| `evaluated` | boolean | True only if the evaluation record contains the model, the question set, and the tool context. |
| `tool.written_by` | `server`, `operator` | Absent if no definition is known. |
| `tool.context` | fingerprint | Absent if no definition is known. |
| `context.*` | lists and a number | Absent if the system built no context. |
| `answers` | map with the question name as the key | One entry for each question of the catalog. Empty if no response arrived. |
| `answers.*.status` | `answered`, `refused`, `missing`, `invalid` | Always present in an entry. |
| `answers.*.probability` | number from 0 to 1 | Only for an answered `predicate`. |
| `answers.*.probabilities` | list of numbers | Only for an answered `score`. Index 0 is the level with the least risk. |
| `answers.*.score`, `answers.*.confidence` | number | Only for an answered `score`. Do not use them alone for automatic approval (R-17). |

A higher value always shows more risk (FR-027).

## 3. Result fields

The second evaluation can return each existing action. Three fields are new.

| Action | New field | Type | Rules |
|---|---|---|---|
| `allow` | `auto_approval.rule` | string, pattern `^[a-z][a-z0-9_]{0,63}$` | It names the rule for the audit record. |
| `approval_required` | `approval_context.recommendation` | `routine` or `attention` | Optional. |
| `approval_required` | `approval_context.findings` | list of `{"question", "state"}` | Optional. `state` is `indicated`, `not_indicated`, or `unclear`. |

`approval_context.description` and `approval_context.risk_level` keep their rules. A policy can
derive `risk_level` from the answers.

## 4. Rules that the system applies to the result

The system applies these rules in Go code. A policy cannot change them (FR-043).

| Result of the second evaluation | System behavior |
|---|---|
| `deny` | The system denies the call. |
| `approval_required` | The system creates a pending approval. It accepts the recommendation and the findings only if the status is `complete` (FR-036). |
| `allow`, and all conditions of FR-041 are true | The call proceeds. The system writes the audit record first (D-13). |
| `allow`, and one condition of FR-041 is false | The system uses `approval_required`. The description and the risk level come from the first evaluation. |
| `ciba_required`, an unknown action, or an evaluation error | The system denies the call, as before this feature. |

More rules for an `approval_required` result:

- The system removes a finding with an unknown question name or an unknown state.
- The system removes an unknown recommendation value.
- The system adds the catalog `statement` to each finding. A policy cannot supply statement text.
- The system keeps a maximum of 16 findings.

In the first evaluation, the system reads only the existing fields. It ignores `auto_approval`,
`recommendation`, and `findings` there.

## 5. Example

The policy of [spec.md](../spec.md), Journey 1, step 3, is the reference example. The OPA library
of the repository evaluated it against the inputs of journeys 1 to 3.
