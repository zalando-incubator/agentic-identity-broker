# ADR 035: Shared Tool Pattern Matching

**Status**: Accepted
**Date**: 2026-09-04
**Feature**: approval-glob-patterns

---

## Context

The broker records user decisions for tool calls, and ExtProc consumes those decisions while matching later calls. Both processes must interpret tool-name globs, parameter globs, canonical argument values, and specificity identically. Separate implementations would permit a broker-approved decision to be interpreted differently by ExtProc.

## Decision

`internal/toolpattern` is a neutral, dependency-free leaf package that both `internal/domain/*` and `internal/extproc/*` may import. It owns the approval-pattern grammar, canonicalization, matching, formatting, precedence, and embedded language-neutral test vectors.

The package must not import `internal/domain`, `internal/ports`, `internal/adapters`, or `internal/extproc` packages.

## Rationale

A single implementation removes semantic drift from a security-sensitive authorization decision. Keeping the package under `internal/` preserves repository-local ownership while allowing the standalone ExtProc process to use the exact broker behavior. Standard-library-only implementation keeps the grammar small and auditable.

## Consequences

**Positive**:
- Broker and ExtProc share one tested matching and ranking implementation.
- Embedded vectors define the cross-service contract without working-directory-dependent fixtures.
- The dependency remains inward-neutral: neither service imports the other's domain or adapters.

**Negative**:
- `internal/extproc/*` has one explicit exception to its broker-internal import prohibition.
- Changes to tool-pattern semantics require coordinated review because they affect both processes.

## Alternatives Considered

1. **Duplicate matching in ExtProc**: Rejected because duplicated canonicalization and glob semantics can drift.
2. **Use a third-party glob library**: Rejected because the required dialect has exactly one metacharacter and needs predictable escaping.
3. **Extract a public module**: Rejected as premature; both consumers remain in this repository.

## Implementation Notes

`internal/toolpattern` exposes `Canonical`, `ExactParams`, `Matches`, `Format`, `Specificity`, `Compare`, and `SelectBest`. Its `vectors.json` is embedded and exposed through typed accessors so both broker and ExtProc tests consume the same fixture.
