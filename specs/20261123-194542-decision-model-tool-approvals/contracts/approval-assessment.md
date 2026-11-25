# Contract: Assessment Summary in the Approval API and on the Approval Page

**Feature**: [spec.md](../spec.md) | **Decisions**: [research.md](../research.md) D-09, D-10

**Approval status**: Draft. The stakeholder must approve the API change before implementation
(API-006, Principle X). The schemas are in [schemas.yaml](schemas.yaml).

## 1. End-user API

Two existing operations in `api/enduser/openapi.yaml` get one optional property. Paths, methods,
status codes, authentication, and the response envelope do not change. The Admin API does not
change.

| Operation | Schema | Change |
|---|---|---|
| `POST /api/approvals` | `CreateApprovalRequest` | New optional property `assessment` |
| `GET /api/approvals/{id}` | `ToolApprovalDetail` | New optional, nullable property `assessment` |

Other responses do not contain the summary. The sync response and the list responses do not
change.

### Rules for `POST /api/approvals`

- A request without `assessment` is valid and has the behavior of today.
- The broker rejects a summary that breaks a rule of [schemas.yaml](schemas.yaml) or of [data-model.md](../data-model.md). The response is the existing `400` validation error. The broker creates no approval.
- A summary with the status `not_available` must not contain a recommendation or findings.
- If an unexpired pending approval exists for the same principal, agent, tool, and arguments, the broker returns that approval. The stored summary does not change (FR-037).
- The existing authentication applies: the subject token and the client assertion (`adrs/018-approval-endpoint-auth-boundaries.md`). Only the gateway client can supply a summary.

Example request with a summary: [spec.md](../spec.md), Journey 1, step 8.

Example request without an available assessment:

```json
{
  "metadata": {"description": "Share a Drive file"},
  "tool_name": "drive.share_file",
  "arguments": {"file_id": "1Qm4RtY8cD2", "role": "reader", "grantee_domain": "example.com"},
  "risk_level": "medium",
  "assessment": {"status": "not_available"}
}
```

### Rules for `GET /api/approvals/{id}`

- The response contains the stored summary without a change.
- An approval without assessment information has no `assessment` property, or the value `null`.
- The existing authentication applies: the acting user principal.

Example excerpt:

```json
{
  "data": {
    "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "tool_name": "drive.share_file",
    "risk_level": "critical",
    "status": "pending",
    "assessment": {
      "status": "assessed",
      "recommendation": "attention",
      "findings": [
        {"question": "leaves_organization", "statement": "Gives access or data to a party outside the organization", "state": "indicated"}
      ],
      "model": "jev-1.14.0",
      "question_set": "qs-<64 hexadecimal characters>"
    }
  }
}
```

## 2. Approval page

The component `AssessmentSummary` is in `web/src/components/approvals/`. `ApprovalReviewPage`
shows it between the request summary and the technical details (FE-001). The type is in
`web/src/types/approval.ts`.

| Value of `assessment` | Content of the section |
|---|---|
| Absent or `null` | The page shows no section. |
| `status: not_available` | "No automated assessment was available for this request." and "Review the technical details before you decide." |
| `status: assessed`, `recommendation: attention` | "Automated checks found risk indicators. Review the details.", the findings, and the fixed notice |
| `status: assessed`, `recommendation: routine` | "Automated checks found no risk indicators.", the findings, and the fixed notice |
| `status: assessed`, no recommendation, one or more findings | The findings and the fixed notice |
| `status: assessed`, no recommendation, no finding | "The automated assessment has no details for this request." and "Review the technical details before you decide." |

The section heading is "Automated assessment". The fixed notice is "Automated checks can be
wrong. They do not replace your review."

Each finding is one row with a state label and the statement:

| `state` | Label |
|---|---|
| `indicated` | "Indicated" |
| `not_indicated` | "Not indicated" |
| `unclear` | "Unclear" |

Rules for the component:

- The page shows the statement as text. It does not interpret markup (SR-008).
- The state label is text. Color is not the only signal (FE-002).
- The `routine` text uses neutral styling, not success styling (FE-003).
- The section does not change the focus sequence or the sequence of the Approve and Deny actions.
- The page shows no model name, no fingerprint, and no number from an answer.

The page shows the section for each summary that is not absent or `null` (SC-001).

The wireframes are in [spec.md](../spec.md): Journey 1, step 9, Journey 2, step 4, and Journey 3,
step 3.
