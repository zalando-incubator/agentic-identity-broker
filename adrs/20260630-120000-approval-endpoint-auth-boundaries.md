# ADR 20260630-120000: Approval Endpoint Authentication Boundaries

**Status**: Accepted
**Date**: 2026-06-30

---

## Context

The tool approval feature introduces a mixed set of endpoints under `/api/approvals` with materially different trust boundaries:

- Some endpoints are **gateway/control-plane** operations used by ExtProc or agentgateway.
- Some endpoints are **browser/user-facing** operations used by the consent UI.
- Some endpoints mutate **global or cross-user sync state**.
- Some endpoints mutate only a **single approval owned by one principal**.

Early planning applied a broad “subject token + client assertion” model to machine-facing approval endpoints. On review, that blanket approach proved too coarse:

- `GET /api/approvals` is a gateway cache synchronization channel, not a user operation.
- `POST /api/approvals/{id}/consume` is a per-approval, principal-scoped mutation with a narrower blast radius than approval creation.
- Browser-facing endpoints should remain aligned with the broker’s existing authenticated-principal model rather than require machine credentials.

At the same time, `POST /api/approvals` creates user-visible security state (an approval prompt) and therefore benefits from stronger caller guarantees than the rest of the approval API.

---

## Decision

Adopt **endpoint-specific authentication boundaries** for approval routes instead of a single blanket policy.

### 1. Create approval — dual auth

`POST /api/approvals` MUST require:

1. a **subject token** that binds the request to the user/agent context, and
2. a **client assertion** that proves the trusted gateway caller.

Rationale:
- this endpoint mints user-visible approval state
- the broker should independently validate both the user-scoped context and the trusted machine caller
- approval prompt creation is the highest-risk machine-facing operation in the approval API

### 2. Sync approvals — client assertion only

`GET /api/approvals` MUST require only a **client assertion**.

Rationale:
- this endpoint is a gateway control-plane synchronization channel
- it can expose grouped approval state across principals and agents
- the primary trust property is “is this an authorized gateway caller?” rather than “which user is this request about?”
- no subject token is required

### 3. Consume approval — subject token only

`POST /api/approvals/{id}/consume` MUST require only a **subject token**.

Rationale:
- consume is a per-approval, principal-scoped mutation
- it does not create approval prompts and does not expose cross-user state
- requiring the subject token is sufficient to bind the action to the approval owner
- dual auth here adds complexity with lower incremental security value than on create

### 4. Browser-facing endpoints — acting user principal only

The following endpoints MUST require only the authenticated **acting user principal** from the browser auth layer:

- `GET /api/approvals/{id}`
- `POST /api/approvals/{id}/approve`
- `POST /api/approvals/{id}/deny`
- `POST /api/approvals/{id}/revoke`
- `GET /api/approvals/permanent`
- `GET /api/approvals/pending`

Rationale:
- these are end-user/browser operations
- they should align with the broker’s existing principal-based auth model
- the service layer already enforces principal ownership checks on approval mutations and reads

In current pre-auth deployments this principal is typically propagated via `X-Remote-User`, but the architectural requirement is the authenticated acting user principal, not a specific transport mechanism.

---

## Consequences

### Positive

- Security complexity is applied only where it materially improves the trust model.
- `POST /api/approvals` receives the strongest protection because it creates approval prompts.
- `GET /api/approvals` stays aligned with its actual role as a gateway sync/control-plane endpoint.
- `POST /api/approvals/{id}/consume` remains simpler and easier to integrate while preserving principal binding.
- Browser-facing approval routes remain consistent with existing broker UX flows and middleware.

### Negative

- The approval API no longer has a single machine-facing auth rule; implementers must reason per endpoint.
- OpenAPI, contracts, spec text, and tests must all explicitly encode the auth split to avoid drift.
- Reusable middleware/helpers are needed to prevent auth logic from fragmenting across handlers.

---

## Alternatives Considered

### A. Dual auth for all machine-facing approval endpoints

Rejected.

Why:
- appropriate for `POST /api/approvals`
- excessive for `GET /api/approvals` and `POST /api/approvals/{id}/consume`
- adds implementation and test complexity where the security return is low

### B. Client assertion only for all machine-facing approval endpoints

Rejected as the primary model.

Why:
- plausible for `GET /api/approvals`
- potentially acceptable for `POST /api/approvals`
- but weaker than desired for approval creation because the broker would trust the caller as the sole source of user/agent context

### C. Subject token only for `POST /api/approvals`

Rejected.

Why:
- proves user/agent context
- does not prove that the caller is the trusted gateway authorized to mint approval prompts
- creates the wrong trust shape for approval creation

---

## Implementation Notes

- `POST /api/approvals` requires a reusable dual-auth path that validates both the subject token and the client assertion before handler logic runs.
- `GET /api/approvals` requires reusable client-assertion validation without a subject token requirement.
- `POST /api/approvals/{id}/consume` should continue to enforce approval ownership in the service layer even though it does not require a client assertion.
- OpenAPI security requirements must represent the dual-auth create endpoint as a single security requirement object containing both schemes (logical AND), not two separate alternative entries.
