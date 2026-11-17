# Historical test-first exception (D-6)

**Status**: Approved (approved by mjungsbluth on 2026-11-05: https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6661347255). The supplied approval time is `2026-11-05T16:38:49Z`; alternative-validation closure is recorded in `quickstart.md` §12.
**Scope**: Feature 054 implementation already present at `34f4f2b94dd78707777e9b22b0a9b58ccb734576`, only the missing historical sequencing obligations identified below.

## Constraint and evidence

Constitution VIII requires tests to compile and fail semantically before implementation;
Constitution XIII additionally requires acceptance tests before business behavior. The
“When Constraints Cannot Be Met” procedure requires a documented reason, alternative
validation and maintainer exception approval. This proposal invokes that procedure rather
than changing either principle or treating later tests as proof of compliance.

`quickstart.md` §7, “Red evidence”, explicitly records that domain, HTTP, ExtProc and
consumer/startup E2E tests were authored alongside implementation, with no separately
recorded red phase. Its historical task table records that the compile-only T029–T031 step
was not performed. These are actual sequencing deviations, not merely missing log files.
Tests cannot now be written before behavior that already exists. No authentic evidence
superseding those admissions is available in the supplied analysis or the feature's evidence
record. The historical 28-scenario broker red run remains evidence of those assertions'
sensitivity, not proof that all 44 tests preceded implementation. The newer T044/T047 red/green
record in `quickstart.md` §8 applies only to that correction, not the original work.

Affected historical obligations: T027, T029–T031, T036, T053, T065, T080, T094 and T097.
The 1:1 mapping of 44 scenarios remains required. No runtime guarantee, scenario, test,
security check, API approval or existing failure is waived by this proposal.

## Recommended decision (Proposed)

Approve a one-time exception to the unavailable original test-first/compile-only sequence
under VIII/XIII for the scoped existing implementation, conditional on completing the
alternative validation below. Preserve the original noncompliance in the evidence record.
Do not remove working behavior to insert retrospective stubs, rewrite history, fabricate red
runs, or count a later green run as historical test-first compliance. This exception does not
apply to any new behavior or correction: those must follow genuine test-first development.
It neither changes ADR 007's production-bootstrap pattern nor supersedes an architectural ADR.

Rationale: the historical order is irrecoverable, while the implementation and independent
acceptance criteria can still be reviewed and validated without disrupting working code.
Residual risk: tests authored alongside code may mirror its defects; passing them cannot
restore the design independence that test-first development would have provided.

Main alternative: decline the exception and retain the historical obligations as unresolved;
do not declare feature completion. A retrospective replay against scaffolding can help assess
test sensitivity, but cannot restore the original ordering and is not an alternative claim of
TDD compliance. Silent waiver or treating existing contract approval as exception approval
is not acceptable.

## Alternative validation and closure conditions

1. Review every existing feature unit/HTTP/ExtProc test against the requirements and contracts,
   independently of implementation details. Record concrete `file:line` coverage and gaps in
   `quickstart.md`. Preserve unchanged exchange regression assertions. For any gap or defect,
   first add realistic compiling assertions and observe semantic failure before correction;
   keep production-bootstrap integration and all fail-closed/security invariants.
2. Reconcile all 44 acceptance scenarios to separate `It()` blocks and real full journeys,
   checking assertion quality, fixtures, isolated production bootstrap and absence of skipped,
   pending or vacuous tests. Record which historical red evidence exists and which does not;
   do not infer test-first sequencing from coverage or current green results.
3. Complete the existing validation gates: `just check`, `just test`, `just extproc-test`,
   non-container backend and startup/consumer E2E journeys, docs checks and every required
   non-container `just verify` recipe (using the workflow-prepared security scan). Record
   commands, commit, results and any failures. Existing unrelated failures remain unresolved
   validation, not waived by D-6. New corrections require their own red/green evidence.
4. Obtain downstream results on the pushed commit for US2.19/`just test-e2e-docker`, applicable
   infrastructure integration and Agentgateway container journeys. Record them as deferred
   until results exist, never validated by a sandbox green run. Continue independent local
   work without waiting for containers.
5. Only implementation with supplied approval covering D-6 may record the approving maintainer,
   date and approval-comment URL in `quickstart.md`, together with the evidence for conditions
   1–4 and each affected task's disposition. Keep historical sequence requirements explicitly
   “not performed; covered by approved D-6” rather than certifying them as performed. Leave
   affected task boxes unchecked until both sign-off and applicable validation are recorded;
   approval alone closes no task. Keep any unrelated validation or work unfinished.

The sections above retain the approved proposal and its conditions. Implementation recorded
the supplied approval in `quickstart.md` §11 and the completed alternative validation in §12,
including downstream results on `277fec06b8b0ae735a6cfbd01cd7b8468c140d52`. The original
sequence was **not performed; covered by approved D-6**. Closure of this exception is not
retroactive historical TDD compliance, and no new correction is exempt from test-first work.
