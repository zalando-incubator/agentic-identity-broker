# ADR 035: Shared Tool Pattern Matching

**Status**: Accepted
**Date**: 2026-09-04
**Feature**: approval-glob-patterns

---

## Context

The broker records user decisions for tool calls, and the stacked ExtProc approval-sync branch consumes
those decisions while matching later calls. Both processes must interpret tool-name globs, parameter
globs, canonical argument values, and single-pattern coverage identically.

## Decision

`internal/domain/approval/toolpattern` is a standard-library-only leaf owned by the approval bounded
context. Both `internal/domain/*` and `internal/extproc/*` may import this exact package; ExtProc may
not import any other `internal/domain/*` package. It owns grammar, canonicalization, validation,
single-pattern matching, formatting, and embedded match/canonicalization vectors.

Precedence selection belongs in `internal/extproc/approval`, beside its only consumer
(`Cache.Match`): it owns rank, comparison, candidate selection, and precedence vectors.

The shared package must not import `internal/ports`, `internal/adapters`, or `internal/extproc`.

## Rationale

The original top-level `internal/toolpattern` location sat outside every hexagonal ring while being
imported by domain code. Locating the pure package in the approval domain conforms to the domain
import rules while retaining one explicit, narrowly-scoped ExtProc exception. Keeping selection beside
the enforcing cache avoids prematurely exporting policy that no broker production caller uses.

## Consequences

**Positive**:
- Broker and ExtProc share one tested grammar, canonicalization, validation, and single-pattern matcher.
- `internal/domain/approval/toolpattern/vectors.json` supplies the shared match and canonicalization contract.
- ExtProc keeps candidate ranking and selection beside its enforcing cache.

**Negative**:
- `internal/extproc/*` has one explicit exception to its broker-internal import prohibition.
- Changes to shared pattern semantics require coordinated review because they affect both processes.

## Alternatives Considered

1. **Duplicate matching in ExtProc**: Rejected because duplicated canonicalization and glob semantics can drift.
2. **Use a third-party glob library**: Rejected because the required dialect has exactly one metacharacter and needs predictable escaping.
3. **Extract a public module**: Rejected as premature; both consumers remain in this repository.

## Implementation Notes

`internal/domain/approval/toolpattern` exposes `Canonical`, `EscapeLiteral`, `ExactParams`,
`ValidateToolPattern`, `ValidateParamsPattern`, `Matches`, and `Format`. Its `vectors.json` is
embedded and exposes match and canonicalization vectors. ExtProc owns precedence vectors and
selection inside `internal/extproc/approval`.
