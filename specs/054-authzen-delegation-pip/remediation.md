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

## HIGH I2 — Evaluation audit identity seam (ADR decision accepted)

The maintainer accepted the ADR decision set in research R16. That set includes the extension
of ADR 033's token-only deferred-finalization seam to `POST /access/v1/evaluation`. The ADR
file is still drafted in T008, at the next unused number, and records this acceptance. The
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
predates these decisions, and its T056 still describes the superseded verbatim `delegation`
members. `/speckit-tasks` must regenerate it, and T049/T050/T052/T057/T058 must test the
decided freshness semantics and the typed context mapping. The MEDIUM C1 Docker-label finding
from the previous analysis is not resolved here.
