# ADR 20261116-182420: Living Documentation Checked Against Acceptance Evidence

**Status**: Proposed
**Date**: 2026-11-16

## Context

Feature specs under `specs/` describe one change each. The current behavior of a capability, such
as agent registration, is spread over several specs, ADRs, and guides. The example journeys that
Constitution Principle XIV requires are verified by hand on the day they are written. Nothing
detects when a later change makes them wrong.

Acceptance scenarios under `tests/e2e/` are the executable truth (Principle XIII), but stakeholders
cannot read them as a description of the product.

## Decision

- A living-documentation page under `docs/specification/` describes one capability as it behaves today,
  in free prose with concrete examples, in the manner of *Specification by Example*.
- A page cites an example by a stable identifier. The acceptance scenario that proves the example
  carries the same identifier through `example(...)`. No second fixture layer exists: pages
  do not drive execution.
- When evidence recording is on, the suite records the state of each such scenario and the HTTP
  exchanges of its test clients. `tools/livingdoc` compares the pages with that evidence. A
  citation without a passing scenario, and a shown exchange that the scenario did not perform,
  fail the check. `just living-docs` runs both steps.
- A language model can write and maintain the prose under `.agents/skills/living-docs/SKILL.md`.
  It has no part in the check. The check is deterministic.
- One page exists for each capability in `specs/capability-map.md`. Evidence comes from the
  backend, ExtProc, and browser acceptance suites and from plain Go tests bound with
  `evidence.ForTest`. Only the backend suite records HTTP exchanges; the other sources prove
  prose claims. A page shows a screen with a maintained screenshot of the browser suite.
- CI runs `just living-docs -strict -browser` as the job "Living specification" for code and
  documentation changes, and the CI gate requires it. The check is not part of `just verify`.

## Consequences

- A behavior change that contradicts a page fails `just living-docs` with a diff.
- Scenarios gain one decorator each. Normal test runs do not record and are unaffected.
- Examples bound to tests that need Docker, Helm, or a browser report "not run" where those are
  missing. A claim that only a unit test proves is marked the same way as one that an acceptance
  scenario proves.
- Recorded bodies are stored as sent. Pages and fixtures must keep to synthetic data.
- Open before acceptance: recorded evidence other than HTTP exchanges (decoded token claims,
  gateway requests, telemetry), and whether feature
  specs cite examples by the same identifiers.

## Alternatives

- **Pages that drive execution, as Concordion does**: needs a fixture layer beside the Ginkgo
  suites and competes with Principle XIII.
- **Pages generated from tests**: stays current but reads as a test report. It loses the free
  structure and the reasons.
- **A language model that judges whether pages and code agree**: not reproducible, and it cannot
  gate a merge.
