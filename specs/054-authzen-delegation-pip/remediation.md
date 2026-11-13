# Remediation handoff: analysis run 40520766996/1

**Status: Blocked — stakeholder freshness decisions required.** This document records partial
remediation, not new product behavior, API approval, ADR acceptance, or implementation evidence.
The authorized input contains no maintainer answer to the findings. Existing clarification
answers in `spec.md` do not decide these freshness questions.

## HIGH I1 — Missing or invalid freshness (unresolved)

Research R13 treats missing/invalid permit `max-age` as an error (503); ExtProc contract §2,
data-model §3.4 and T049 allow current-request use without caching. Neither interpretation is
selected by this remediation. Do not implement either interpretation until the stakeholder
confirms one rule and all four artifacts agree.

**Decision required:** Should a permit with missing or malformed `max-age` fail closed with
503, or be usable only for the current request without caching? Recommended for review: reject
missing/malformed freshness because there is no trustworthy lifetime. Separately confirm
whether valid zero means current-request-only use or an already-expired permit, and whether an
excessive value is rejected or clamped to the fixed 60-second broker cap and local cap. These
are proposals, not accepted answers. Current-request-only use must also be reconciled with U2.

After confirmation, T049/T050 must distinguish missing, malformed, zero, positive, excessive,
duplicate/conflicting and overflowing values; assert the immediate request outcome and cache
reuse (or its absence), not just entry insertion. Update research R13, ExtProc contract §2,
data-model §§3.3–3.4, FR-015 and the linked Journey 4 together.

## HIGH U1 — End-to-end freshness deadline (unresolved)

The current relative `min(max-age, max_staleness)` is incomplete: starting it at receipt or
insertion can extend a permit beyond its original lifetime or `grant.valid_until`.

**Decision required:** Confirm a conservative time origin, HTTP response-age treatment and
absolute expiry calculation, including request/response transit, the fixed broker cap, local
`max_staleness`, and `grant.valid_until`. Recommended for review: anchor the usable duration
conservatively to request start, subtract any applicable response age without ignoring transit,
and cap the resulting absolute deadline at grant expiry. Review clock-skew assumptions and
missing/invalid age handling explicitly. This is not a selected formula or approved contract.

After confirmation, carry the single calculated deadline through Client `Result`, Resolver
entries and request state; cache hits, insertion and singleflight delivery must not restart it.
T049/T050 must test delayed responses, already-aged responses, near-grant-expiry responses,
exact expiry, local and fixed caps, and singleflight delivery after expiry. Each case must
assert both policy-use eligibility and reuse. Update FR-009/FR-015, SC-002/SC-003, Journey 4,
research R5/R13, the model and ExtProc contract together. A permit already expired on arrival
must not become fresh merely by insertion.

## HIGH U2 — Expiry before policy use (unresolved)

Resolving at headers does not establish freshness at body-policy time. Context can expire
while waiting for concurrent exchange, while buffering the body, or between batch decisions.
The existing no-stale requirement remains binding; the missing recovery rule is not permission
to feed expired context to policy.

**Decision required:** On expiry before policy use, reject the request with a fail-closed
unavailability response, or re-resolve under a bounded timeout and use only a fresh permit?
Recommended for review: reject with 503 rather than introduce additional calls/retry semantics;
this recommendation is not authorization. If re-resolution is chosen, confirm its bound,
precedence with exchange errors, denial/outage mapping, and behavior across batch decisions.

After confirmation, T052 must use synchronization barriers to expire the deadline while
awaiting exchange and while buffering a body, including revocation and outage variants; assert
no expired context reaches any policy invocation or downstream forwarding. Cover header-only
and each batch-policy use as well. T057/T058 must carry/check the unchanged deadline at the
approved use seams and implement only the confirmed recovery rule. Extend US3.1/US3.4/US3.6
and Journey 4 within their existing one-scenario/one-`It()` mapping, and reconcile research R14,
plan §6, model §3 and ExtProc contract §3. Testing only a later request after cache expiry is
insufficient.

## HIGH I2 — Evaluation audit identity seam (design correction recorded; approval pending)

The source `internal/adapters/http/middleware/security_context.go` defers only
`POST /oauth2/token`. R9's claim that the evaluation route already receives a capture holder
was incorrect. The design now explicitly requires a shared-perimeter deferred-finalization
seam for `POST /access/v1/evaluation`, rather than a second route-local capture mechanism.

T008 must include this extension of ADR 033's route/finalization description in the proposed
architectural decision and obtain stakeholder acceptance through the existing gate. ADR 033
is currently marked Proposed in this checkout, while `ARCHITECTURE.md` §3.1.4.2 describes the
existing token-only seam. Do not silently amend that ADR or imply that the extension is already
approved. T009 must reconcile §3.1.4.2 after approval.

T044 adds test-first middleware/handler/composed-perimeter coverage. T047 implements the
shared matcher extension and early-error fallback only after those tests fail semantically.
On permit and caller-policy denial after both credentials validate, assert the subject principal
as Actor, distinct assertion subject as CallingPeer, one finalization, and the same trace ID
in security context, audit/access records and `traceresponse`. Check equal-identity omission,
malformed/missing/invalid credential fallback without invented verified identities, and
unchanged ordinary HTTP, ordinary token-grant and delegated-exchange behavior. A post-handler
access-log fallback alone cannot stamp earlier in-handler audit events.

## Gate and scope

I1, U1 and U2 remain HIGH and unresolved. I2 has explicit design/test tasks, but neither an
accepted decision nor runtime proof is claimed. No task is marked complete. Implementation
must stop until the decisions above and the existing contract/ADR gates are satisfied.

The MEDIUM C1 Docker-label finding is preserved in the previous analysis; it is not part of
this session's HIGH/CRITICAL edit authorization and is not claimed resolved. No application,
workflow, skill, hook, constitution, binding ADR or approved API file is changed.
