# ADR 048 revision draft: Canonical permission-set policy input

**Status**: Proposed (approval item ADR-3 in [plan.md](./plan.md#approval-items))
**Revises**: ADR 048 §5, same PR before merge; ADR 048 is not on the default branch.

## Context

The selected F3 recommendation in the 2026-11-06 remediation proposal calls for a canonical
permission-set field and a follow-up issue for compatibility removal. The prior contract
promises permanent retention. Naming a canonical field now must not silently remove that
compatibility or authorize future breaking changes.

## Decision (Proposed)

Designate `input.context.permission_sets` as the canonical enabled-mode field for new policies.
It includes typed IDs, optional canonical IDs and included-service contributions. Effective
resource scope ceilings still come from `input.context.resource.scopes`.

Retain `granted_permission_sets` and `granted_permission_sets_available` with their current
permit-derived values on both enabled paths, and keep disabled-mode serialization and
behavior unchanged. No OPA field is removed or deprecated by this feature. Replace §5's
"retained permanently" only with "retained in this increment; future removal requires a
separately approved, versioned policy-input contract and migration".

Before recording this implementation revision, the maintainer must provide the follow-up
issue URL required by T104. The issue covers both compatibility fields, migration guidance,
availability replacement, enabled/disabled behavior and versioned contract approval.
Creating an issue does not approve its future removal. FR-019's separate exchange-response
field deprecation and eventual API 2.0.0 approval remain unchanged.

After explicit supplied ADR-3 approval and the issue prerequisite, implementation records
that approval, incorporates this text in ADR 048 §5 and its Revision history, updates policy
guidance and removes this incorporated draft. Preserve ADR 048's existing approvals. This
phase records no acceptance and does not create an issue.

## Consequences

New enabled-mode policies have one recommended rich field; current policies remain compatible.
The permanent-retention commitment becomes revisable only through a future separately
approved contract change, not an authorization to remove now. The external issue prerequisite
can defer T104 but does not block independently approved token-binding work.

## Main alternative

Retain both representations permanently without canonical guidance. This avoids revising the
commitment but preserves duplicate policy surfaces and does not implement the selected F3
recommendation. Immediate removal is not the selected alternative and is out of scope.
