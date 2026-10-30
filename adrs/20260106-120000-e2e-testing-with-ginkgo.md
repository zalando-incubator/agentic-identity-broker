# ADR 20260106-120000: E2E Testing with Ginkgo and Production Bootstrap

**Status**: Accepted
**Date**: 2026-01-06
**Feature**: 20251222-120100-oauth2-auth-server

## Context

The OAuth2 Authorization Server feature (20251222-120100-oauth2-auth-server) requires comprehensive end-to-end (E2E) acceptance tests that validate the complete HTTP routing and configuration bootstrap. We need tests that:

1. Exercise the full production code path (dependency injection, routing, middleware, handlers)
2. Map directly to acceptance scenarios in the specification (Specification by Example)
3. Remain stable when internal architecture changes (refactoring-resistant)
4. Run fast without external dependencies (in-memory storage, no database containers)
5. Provide clear, readable test output that non-developers can understand

## Decision

We will use **Ginkgo v2** with **Gomega** for BDD-style E2E tests that exercise the complete production application stack via real HTTP servers. Tests will be organized into **stable test scenarios** (HTTP contract testing) and **volatile setup code** (production bootstrap wrappers).

### Key Architecture Patterns

1. **Use Production Code for Bootstrap**: Tests use `app.Builder`, `httpAdapter.Server`, and `storage.Adapter` from production code - no custom test implementations
2. **Separate Stable from Volatile**: Test scenarios focus on HTTP contracts (stable), setup code wraps production bootstrap (volatile, changes with refactoring)
3. **Real HTTP Testing**: Tests use `httptest.Server` with full routing, middleware, and handler stack
4. **Ginkgo BDD Structure**: Tests use `Describe`/`Context`/`It` blocks that map naturally to Given/When/Then
5. **Test Isolation**: Each test gets fresh storage and server via `BeforeEach` - no shared state

## Rationale

### Why Ginkgo + Gomega

1. **BDD Organization**: `Describe`/`Context`/`It` blocks map naturally to acceptance scenarios (Given/When/Then)
2. **Readable Output**: Test output is hierarchical and readable by non-developers
3. **Suite Lifecycle**: `BeforeEach`/`AfterEach` provide clean test isolation
4. **Focused Execution**: `--focus` flag allows running specific scenarios during development
5. **Watch Mode**: `ginkgo watch` enables TDD workflow with auto-rerun on changes
6. **Custom Matchers**: Gomega matchers improve readability and error messages

### Why Use Production Bootstrap

**Problem**: Custom test bootstrap code duplicates production DI logic and breaks when production changes.

**Solution**: Tests use production `app.Builder` and `httpAdapter.Server` via thin wrappers in `tests/e2e/bootstrap/`. When DI or routing changes in production, only the thin wrappers need updates - test scenarios remain unchanged.

**Benefits**:
- Tests exercise the **real** application bootstrap path
- No duplicate DI logic to maintain
- Tests survive refactoring (only wrappers need updates, not test scenarios)
- Production parity ensures tests validate actual deployment behavior

### Stability Through Separation

**Stable Layer** (test scenarios): Focus on HTTP contract (requests → responses), use abstraction methods like `server.AuthenticatedGET()`, no knowledge of internal routing/DI/bootstrap, rarely change during refactoring.

**Volatile Layer** (bootstrap wrappers): Thin wrappers around production code, update when production bootstrap changes, isolated from test scenarios.

**Result**: When refactoring routing or DI, update `bootstrap/` package only, not test scenarios.

### Alternatives Considered

1. **Standard `go test` with `httptest.ResponseRecorder`**: Rejected - doesn't map cleanly to acceptance scenarios from the specs
2. **testify/suite framework**: Rejected - flat structure, poor mapping to acceptance scenarios
3. **Custom test bootstrap (duplicate DI logic)**: Rejected - duplicates production code, high maintenance
4. **Docker Compose with real database**: Rejected - slow, complex CI setup, in-memory storage sufficient

## Consequences

### Positive

- **Refactoring-Resistant**: Tests survive routing/DI changes because they use production bootstrap
- **Fast Execution**: In-memory storage, no database containers (< 60 seconds for 62 tests)
- **Clear Mapping**: Each `It()` block maps to one acceptance scenario in spec.md
- **Readable Output**: Ginkgo output is hierarchical and understandable by non-developers
- **Test Isolation**: Fresh storage per test prevents flaky tests from shared state
- **Production Parity**: Tests exercise the same code path as production (DI, routing, middleware)
- **Maintainable**: Thin bootstrap wrappers are easy to update when production changes

### Negative

- **Additional Dependency**: Requires Ginkgo/Gomega (minimal learning curve, significant value)
- **Not Pure Unit Tests**: Tests exercise multiple layers (intentional for E2E validation)

### Risks and Mitigations

- **Risk**: Duplicate production bootstrap in tests → **Mitigation**: ADR mandates using production `app.Builder`
- **Risk**: Tests focus on implementation details → **Mitigation**: Test scenarios use HTTP abstraction methods only
- **Risk**: Shared state causes flakiness → **Mitigation**: Fresh storage per test via `BeforeEach`

## Implementation Details

**Comprehensive documentation**: All implementation details, test structure examples, running instructions, test organization patterns, anti-patterns, and developer guidance are documented in [tests/e2e/README.md](../tests/e2e/README.md).

**Quick Commands**:
```bash
just test-e2e-backend          # Run the backend E2E suite
just test-e2e-backend-coverage # Run the backend suite with coverage report
just test-e2e-backend-watch    # Watch the backend suite during TDD
just test-e2e                  # Run all backend, ExtProc, and frontend E2E suites
```

## References

- **Implementation Documentation**: [tests/e2e/README.md](../tests/e2e/README.md) - Comprehensive guide with examples, patterns, and anti-patterns
- **Test Implementation Plan**: .claude/plans/dapper-cuddling-shore.md
- **Feature Specification**: specs/20251222-120100-oauth2-auth-server/spec.md
- **Ginkgo v2**: https://onsi.github.io/ginkgo/
- **Gomega**: https://onsi.github.io/gomega/
- **Specification by Example**: https://gojko.net/books/specification-by-example/
