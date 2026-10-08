# Tasks

## Phase 1: Setup

- [x] Create the minimal branch from `origin/main`, leaving the reference branch untouched.
- [x] Read main governance and verify assumptions. Replace the disproved locking conclusion with the approved agent-first order.

## Phase 2: Design Preconditions

### Phase 2a: Domain Model & Glossary

- [x] Document nullable refresh grant identity and continuing consent in `ARCHITECTURE.md` (II, V).

### Phase 2b: Configuration Design

- [x] No configuration or Helm changes are required (VII).

### Phase 2c: API Design

- [x] Update the existing refresh response description, without changing its schema or grant API behavior (IV, X).

### Phase 2d: Database Design

- [x] Add reversible migration 036 with `lock_timeout`, no foreign key, and no backfill (IX).

### Phase 2e: Frontend/Design System Review

- [x] No frontend changes are required (XI).

### Phase 2f: E2E Acceptance Test Design

- [x] Write production-builder tests for S1–S6. Observe semantic failure for changed behavior and preserve existing agent-deletion behavior (VIII, XIII).

## Phase 3: Implementation and regression coverage

- [x] Write failing memory consent tests, including legacy timestamps and verifier errors (VIII).
- [x] Bind initial refresh issuance and successors to the active grant.
- [x] Reject ended or replaced consent and revoke denied chains. Preserve state on infrastructure errors.
- [x] Add principal-and-agent and agent-wide revocation to both repositories.
- [x] Lock PostgreSQL agents before refresh rows. Use `FOR KEY SHARE` for rotation and `FOR UPDATE` for revocation.
- [x] Call revocation after grant deletion, expired renewal, and credential deletion. Preserve existing grant API semantics.
- [x] Log consent-denial reasons and revocation trigger/count without secrets.
- [x] Cover matching-unused storage behavior, both rotation/revocation orders, and agent-deletion races.
- [x] Cover issuance and refresh in Europe/Berlin and populated migration up/down/up.

## Phase N: Constitution Compliance Verification

### Design Phase Verification

- [x] Keep main's constitution and accepted ADRs unchanged (II).
- [x] Verify spec-to-E2E mapping, API descriptions, and migration design (IV, VIII, IX, X, XIII).

### Implementation Phase Verification

- [x] Verify fail-closed consent and safe audit logs (I, III).
- [x] Verify domain/port/adapter boundaries and builder wiring (VI, XII).
- [x] Verify no configuration or frontend changes (VII, XI).
- [x] Run build, vet, lint, race unit tests, PostgreSQL integration, and backend E2E (VIII, IX, XIII).
- [x] Update continuing-consent documentation and record compatibility limits (II, IV).
- [x] Measure additions against both budgets and create small conventional commits.
