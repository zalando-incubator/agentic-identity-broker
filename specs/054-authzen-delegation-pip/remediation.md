# Remediation handoff: analysis run 40520766996/1

**Status: Freshness, audit-seam, and OPA input-shape decisions recorded.** The OPA input
shape was settled by the 2026-10-28 spec clarification and applied to the design by the
follow-up planning session. On 2026-10-28 maintainer `mjungsbluth` answered the open findings in a review submitted
with `/speckit-plan` (command 5688019903). This file records those answers and where the plan
artifacts apply them. It records no implementation evidence.

| Finding | Maintainer answer | Applied in |
|---|---|---|
| I1 | "Missing or zero shall mean fail closed." | research R13; ExtProc contract §2; data model §§3.3–3.4 |
| U1 | "Let's not overcomplicate this. We can live with the skew which is potentially a few minutes longer than required." | research R13; ExtProc contract §2; data model §3.4 |
| U2 | "Reject with 503" | research R14; ExtProc contract §3; data model §3.4; plan §6 |
| I2 | ADR accepted on research R16, which includes the ADR 033 seam extension | research R9 and R16; plan |
| R15 review | Spec clarification (spec Clarifications, Session 2026-10-28; command 6480426911) | research R15, R16, and R18; data model §§3.3.1 and 3.5; ExtProc contract §§2 and 4; plan §6 |

## HIGH I1 — Missing, malformed, or zero freshness (decided)

A permit is usable only if its `Cache-Control` header has exactly one `max-age` directive with a
positive integer value. A missing, zero, malformed, or duplicate directive, or a value that does
not parse as an integer, is a resolution error. ExtProc returns 503 `service_unavailable` with
`"delegation context unavailable"`. It does not use the permit for the current request, and it
does not cache the permit. The maintainer named the missing and zero cases. The plan treats
malformed values in the same way, because they also give no usable freshness (FR-015).
Large values need no separate rule. The usable lifetime is `min(max-age, max_staleness)`, so
`max_staleness` bounds them.

## HIGH U1 — End-to-end freshness deadline (decided: simple relative deadline)

ExtProc reads the clock once, just before it sends the evaluation request. It sets
`deadline = requestStart + min(max-age, max_staleness)` and computes this value once in the
client. The `Result`, the cache entry, the singleflight followers, and `requestState` all carry
the same deadline. A cache insertion, a cache hit, or a singleflight delivery never extends it.
ExtProc reads no `Age` header, does no transit accounting, and applies no separate
`grant.valid_until` cap. The broker already bounds `max-age` by the grant expiry (FR-009). The
broker computes `max-age` after ExtProc sends the request, so a deadline based on request start
never runs past the broker's window. The maintainer accepted the remaining skew.

## HIGH U2 — Expiry before policy use (decided: reject with 503)

ExtProc checks the stored deadline immediately before every OPA evaluation that receives
delegation input. This includes the header-only policy call, the body policy call, and each
decision in a batch. If `now >= deadline`, the request is rejected with the same 503
`service_unavailable` / `"delegation context unavailable"` response, and nothing is forwarded.
ExtProc does not re-resolve and does not retry. Expired context never reaches policy.

## HIGH I2 — Evaluation audit identity seam (ADR decision endorsed)

The maintainer endorsed the ADR decision set in research R16 ("ADR accepted"). That set includes
the extension of ADR 033's token-only deferred-finalization seam to `POST /access/v1/evaluation`.
The 2026-10-29 planning session drafted it in
[adr-045-authzen-delegation-pip.md](./adr-045-authzen-delegation-pip.md) with status Proposed and
registered it as approval item ADR-1 in the plan. T001 records the maintainer's 👍 on the
approval comment and moves the draft to `adrs/`. The
§3.1.4.2 reconciliation and the T044/T047 test-first work are still to be done. No runtime
proof is claimed.

## OPA input shape (decided by spec clarification)

The maintainer commented on research R15 that the `delegation` / `delegation_available`
members expose how ExtProc obtains the context. The `/speckit-clarify` session of 2026-10-28
revised FR-017, FR-018, US3.1, US3.2, US3.6, US3.8, US4.1, Journey 4, and SC-005. The policy
input now has these properties:

- `principal`, `agent`, `resource`, `grant`, and `permission_sets` appear directly in
  `input.context`. They come from ExtProc's own typed permit struct, never from a verbatim
  copy, and unknown permit fields are dropped.
- `granted_permission_sets*` are retained permanently and derived from the permit on both
  enabled paths.
- `granted_permission_sets_available` is the only availability signal.
- Disabled mode is unchanged byte-for-byte.

The planning session applied this in research R15 and in the artifacts listed in the table
above. A malformed permit context fails closed with 503, as other malformed responses do.

## Gate and scope

I1, U1, U2, the I2 architectural decision, and the OPA input shape are answered. `tasks.md`
was regenerated after these decisions: T056 implements the typed direct members, and
T049/T050/T052/T057/T058 test the decided freshness semantics and the typed context mapping.
Formal sign-off of the contracts and the ADR draft is tracked in the plan's Approval Items. The MEDIUM C1 Docker-label finding
from the previous analysis is addressed in the task and validation documents: T006/T023/T098 require the `docker` label on US2.19, `just test-e2e-docker` and a capable runner; `quickstart.md` distinguishes the 28 non-Docker broker scenarios from the full 29. Runtime verification remains implementation work.

## Analysis run 40902274709/1 — consistency remediation

These finding IDs belong to the latest analysis, not the earlier freshness/audit findings above.

| Finding | Resolution | Traceability |
|---|---|---|
| MEDIUM I1 — stale release baseline | Proposed end-user API `1.1.0` → `1.2.0`; keep exchange fields populated and later removal separately approved as `2.0.0`. Earlier review direction does not approve the corrected version. Alternative: reuse `1.1.0`, losing a distinct additive revision. | Contract §3; research R12; ADR §8; plan API-3; T013/T014/T070; spec Clarifications |
| MEDIUM I2 — permit schema/consumer mismatch | Add `permission_sets.minItems: 1`, `subject.id.minLength: 1`, and `minLength: 1` for every optional `canonical_id`. Keep optional omissions and empty scope/service arrays valid. Retain the existing typed consumer invariants rather than relax validation. | Contract §2.2; FR-007/FR-008; plan API-2; data model §3.3.1 (unchanged); T014/T033/T049 |
| LOW I3 — obsolete next-phase handoff | Record that R15 reconciliation and regeneration of the 101-task list are complete; retain sign-off, final-standard verification and implementation gates. | Spec Clarification handoff |

All three findings are resolved in feature documents. API-2, API-3 and ADR-1 remain Proposed
alongside the other approval items; no approval or implementation is recorded here.
