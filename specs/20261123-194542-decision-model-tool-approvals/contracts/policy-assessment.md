# Contract: Policy Input and Policy Result for an Assessment

**Feature**: [spec.md](../spec.md) | **Decisions**: [research.md](../research.md) D-01, D-04, D-05, D-16

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

The second evaluation occurs for each status, including `definition_missing` and `context_too_large`. Thus,
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
| `status` | `complete`, `incomplete`, `timeout`, `unavailable`, `context_too_large`, `definition_missing` | Always present. |
| `model` | string | The model that the response names. Absent if no response arrived. |
| `question_set` | fingerprint | Always present. |
| `evaluated` | boolean | True only if the evaluation record contains the model, the question set, and the tool context. |
| `tool.context` | fingerprint | See the table of keys for each status. |
| `context.*` | two lists and a number | See the table of keys for each status. If `context` is present, each of its three fields is present. An empty list is `[]`, and the count is a number. |
| `answers` | map with the question name as the key | Always present, with one entry for each question of the question set. If no response arrived, each entry has the status `missing`. |
| `answers.*.status` | `answered`, `refused`, `missing`, `invalid` | Always present in an entry. |
| `answers.*.probability` | number from 0 to 1 | Only for an answered `predicate`. |
| `answers.*.probabilities` | list of numbers | Only for an answered `score`. ExtProc puts each probability at the position of its level `value`. Index 0 is the level with the least risk. |
| `answers.*.score`, `answers.*.confidence` | number | Only for an answered `score`. Do not use them alone for automatic approval (R-17). |

A higher value always shows more risk (FR-027).

Keys for each status (FR-031, FR-033):

| Status | `model` | `tool` | `context` | `answers` |
|---|---|---|---|---|
| `definition_missing`, `context_too_large` | absent | absent | absent | each entry `missing` |
| `timeout`, `unavailable` | absent | present | present | each entry `missing` |
| `incomplete` | present if the response has a valid `model` value | present | present | the validated state of each entry |
| `complete` | present | present | present | each entry `answered` |

`status`, `question_set`, and `evaluated` are present for each status. A policy must read
`status` first. In Rego, a comparison with an absent key is undefined, and a comparison with `0`
or `[]` is not.

## 3. Result fields

The second evaluation can return each existing action. One field is new.

| Action | New field | Type | Rules |
|---|---|---|---|
| `allow` | `auto_approval.rule` | string, pattern `^[a-z][a-z0-9_]{0,63}$` | It names the rule for the audit record. |

`approval_context.description` and `approval_context.risk_level` keep their fields and their
rules. They are the only values of the result that the approver sees (D-16). A policy can derive
the two values from the answers. On an `allow` result, the system uses `approval_context` only if
the call waits for a person (D-05).

A policy author must use constant texts for `description`. A description must not contain an
argument value or an answer value (FR-039, SR-008). The system does not examine the text.

## 4. Rules that the system applies to the result

The system applies these rules in Go code. A policy cannot change them (FR-043).

| Result of the second evaluation | System behavior |
|---|---|
| `deny` | The system denies the call. |
| `approval_required` | The system creates a pending approval with the description and the risk level of this result. |
| `allow`, all conditions of FR-041 are true, and the local cache has no permanent denial of the tool | The call proceeds. The system writes the audit record first (D-13). |
| `allow`, and the local cache has a permanent denial of the tool | The system denies the call (D-15). |
| `allow`, and one condition of FR-041 is false | The system uses `approval_required`. The description and the risk level come from the `approval_context` of the `allow` result, or from the first evaluation if it has none. |
| `ciba_required` or an unknown action | The system denies the call, as before this feature. |
| An evaluation error or an evaluation timeout | The system uses the result of the first evaluation, with its description and its risk level. The call waits for a person (D-05). |
| An undefined result | The system denies the call, as before this feature. |

The authorizer reports the outcome of an evaluation apart from the action: a result, an
undefined result, an error, or a timeout. Before this feature, an error and a timeout have the
form of a `deny` action only. The first evaluation keeps that behavior: each of them denies the
call. The second evaluation reads the outcome, so that an error is not a policy denial and a
policy denial is not an error (D-05).

The permanent-denial lookup applies only to a result that lets the call proceed. A call that
waits for a person keeps the approval state of the last read from the broker.

The system sends the description and the risk level to the broker in the existing fields of the
create request, with the existing rules. It adds no other value of the assessment to that request
(FR-038). A policy result cannot stop the creation of a pending approval.

In the first evaluation, the system reads only the existing fields. It ignores `auto_approval`
there.

## 5. Example

The policy of [spec.md](../spec.md), Journey 1, step 3, is the reference example. On 2026-11-26,
the OPA library of the repository (`v1.21.4`) evaluated it against the first evaluation and
against the inputs of journeys 1 to 3. The results agree with the journeys.
