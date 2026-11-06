## 🏗️ Architectural Review

### Executive Summary

This is a follow-up review, as the maintainer asked. It checks the previous review (at `d49ef0af`) against remediation commit `6a4e14b8` and the `main` merge `27f3fd9e`. The `main` merge made `LOGGING.md` and `OPENTELEMETRY.md` binding.

**Resolved**

- **Previous F2 (audit record shape):** the evaluation record now has a constant message, an `event` and a typed `principal`. 4xx failures log at Warn and 5xx at Error. The ExtProc start-up record now carries the lifecycle event `extproc.delegation_context.enabled`.
- **Previous F7 (E2E revocation):** US2.2, US3.4 and US3.5 now revoke and re-consent through `DELETE`/`POST /api/consent/agents/{id}/grants`.

**Still open**

The remediation applied only the automatic items. No author or maintainer response was found for the question and the proposals, and the code they concern has not changed:

- previous F1: the policy's scopes and the forwarded token's scopes can disagree
- previous F3: telemetry naming and registration
- previous F4: overlapping OPA input fields
- previous F5: vocabulary
- previous F6: copied contract with no sync check

**New**

- **New finding:** ExtProc's delegation denied/unavailable records are authorization-failure security records with no `event`. `LOGGING.md` is now binding, so they need one (F6).
- **Naming note:** `6a4e14b8` names the new events `authzen.*`, but the domain logic lives in `tokenexchange`. This adds a fourth prefix to one operation. It is folded into F2.

The hexagonal layering is still clean, no 🔴 issue was found, and all eight dimensions were assessed.

### Top Findings (Require Author Response)

1. 🟡 **F1: Permit scopes and the cached forwarded token can still disagree after a grant is narrowed (previous F1, open)**: In enabled mode, OPA decides on the permit's `resource.scopes`, which are at most 60 s old. ExtProc still forwards the ADR 012 cached token, which can be up to about 1 h old and is reused when a refresh fails. ADR 047 does not say which of the two is authoritative for upstream scopes. Raised by data-authority, adr-compliance and coupling-cohesion. See [ADR 047](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/adrs/047-authzen-delegation-pip.md). Remediation: question
2. 🟡 **F2: AuthZEN telemetry identifiers do not follow `OPENTELEMETRY.md` and `LOGGING.md` §2.2 (previous F3, open and extended)**:
   
   - The span `authzen.access_evaluation` is started from the `tokenexchange` tracer and sets no span kind (§3.2 and §3.3).
   - The `access_evaluation.*` and `delegation.*` namespaces are not registered (§4.1).
   - The new `token_exchange.operation` and `failure_detail` values are not documented (§4.2).
   - `6a4e14b8` adds `authzen.access_evaluation.*` event IDs. `authzen` is the protocol, not the bounded context.

Raised by otel-parity, ubiquitous-language and maintainability. See [authzen\_evaluation.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/adapters/http/enduser/authzen_evaluation.go). Remediation: proposal

1. 🟡 **F3: OPA input still has two overlapping permission-set fields that depend on the mode (previous F4, open)**: In enabled mode, `input.context` carries both `granted_permission_sets` (map) and `permission_sets` (list), and ADR 047 §5 keeps both permanently. Both come from the same permit, so they cannot contradict each other within one request. However, `granted_permission_sets` means a different thing in each mode. Raised by coupling-cohesion, data-authority, adr-compliance and maintainability. See [input.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/extproc/authorization/input.go). Remediation: proposal
2. 🟡 **F4: Vocabulary drift (previous F5, open)**:
   
   - `tokenexchange.DelegationResolver` and ExtProc `server.DelegationResolver` are different components with the same name.
   - `Delegation` and "Access Evaluation" have no glossary entries.
   - `resource.id` is a URI in the request but a service UUID in the permit, which conflicts with ADR 030.

Raised by ubiquitous-language, adr-compliance, coupling-cohesion and maintainability. See [delegation.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/domain/tokenexchange/delegation.go). Remediation: proposal

1. 🟡 **F5: The Delegation Context shape and the nine denial codes are still copied with no check that keeps them in sync (previous F6, open)**: Copies exist in the domain, OpenAPI, the ExtProc allowlist and wire models, the OPA input and the docs. The only contract test covers the deprecated `/oauth2/token` field. Raised by maintainability, coupling-cohesion and data-authority. See [client.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/extproc/delegation/client.go). Remediation: proposal
2. 🟡 **F6: ExtProc delegation authorization-failure records have no `event` (new)**: `"delegation denied"`, `"delegation context unavailable"` and `"delegation permit expired before policy evaluation"` are security records for authorization failures. Under `LOGGING.md` §2.2 and §8 they need an `event`. The previous F2 fixed only the broker record and the ExtProc lifecycle record. See [server/delegation.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/extproc/server/delegation.go). Remediation: automatic

### Concept Map

Commit `6a4e14b8` adds no domain types, ports or API fields. The open term questions are summarized in F4.

| New Concept | Location | Potential Overlap | Question for Author |
| --- | --- | --- | --- |
| `tokenexchange.DelegationResolver` and `server.DelegationResolver` | [delegation.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/domain/tokenexchange/delegation.go), [server/delegation.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/extproc/server/delegation.go) | Each other (homonym) | "Rename one, or define both in the glossary?" |
| `Delegation` / Access Evaluation | [evaluation.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/domain/tokenexchange/evaluation.go) | **UserGrant**, **Delegation Context**, CEL Authorization, OPA Authorizer | "Is `Delegation` the resolved view of a UserGrant?" |
| Event `authzen.access_evaluation.*` (new) | [authzen\_evaluation.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/adapters/http/enduser/authzen_evaluation.go) | `token_exchange.*` bounded context | "Should it be `tokenexchange.access_evaluation.*`, renamed before it becomes a dashboard contract?" |
| Event `extproc.delegation_context.enabled` (new) | [root.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/cmd/extproc-token-exchange/root.go) | **Delegation Context** is a payload term | 🟢 "Is `extproc.` the intended family of ExtProc lifecycle events?" |

### Hexagonal Compliance

✅ No violations. The previous review's assessment still holds:

- The handler only parses input and maps results.
- `AccessEvaluationService` enforces the invariants.
- The wiring is in `builder.go`.
- ExtProc stays standalone, as ADR 011 requires.

`6a4e14b8` changes only log attributes and test fixtures.

### Pattern Consistency

| Existing Pattern | New Pattern (this PR) | Contradiction? |
| --- | --- | --- |
| ADR 012: an exchanged-token cache with a stale fallback | ADR 047: a separate permit cache of at most 60 s that never uses stale entries. It does not say which cache decides upstream scopes. | 🟡 F1 (open) |
| ADR 028: OPA `input.context.granted_permission_sets` | ADR 047 §5: `permission_sets` is added as well and both are kept permanently | 🟡 F3 (open) |
| ADR 030: a protected resource is identified by its normalized URI | The permit's `resource.id` is the service UUID | 🟡 F4 (open) |
| ADR 033: the audit principal comes from the finalized security context | `6a4e14b8` reads `security.FromContext`, with an `AnonymousActor` fallback, the same way as `oauth2_audit.go` | ✅ |
| Principle XIII: consent changes go through the consent API in E2E | `RevokeGrant()` and `ConsentToRepo()` call `/api/consent/agents/{id}/grants` | ✅ Previous F7 resolved |

### Data Authority

| Business Fact | Source A (existing) | Source B (new) | Can Contradict? |
| --- | --- | --- | --- |
| Effective delegated scopes | The ADR 012 cached exchanged token (up to about 1 h) | The permit's `resource.scopes` (at most 60 s) | 🟡 Yes, after a grant is narrowed (F1) |
| Granted permission sets in OPA input | `granted_permission_sets`: the exchange snapshot when disabled, the permit when enabled | `permission_sets` from the permit | 🟢 Not within one request; the meaning depends on the mode (F3) |

No new data-authority concern comes from `6a4e14b8`.

### Coupling &amp; Cohesion

- F3, F4 (the resolver name clash) and F5 are unchanged.
- 🟢 `6a4e14b8` reads the caller (`SecurityContext.Actor`, with an anonymous fallback) to set `principal`. `telemetry.ContextHandler` already adds the same value as `actor`, so the lookup now exists in two places. After ADR 033 finalization the actor is the delegating user, so `principal` is semantically correct on permits. On unauthenticated failures it logs the anonymous label instead of leaving the attribute out.
- The interfaces stay narrow and are defined by their consumers. `internal/ports/` does not change.

### OTel Parity

| Existing File (with spans) | New File (missing spans) | Gap |
| --- | --- | --- |
| `enduser/oauth2_token.go` | `enduser/authzen_evaluation.go` | ✅ Has a span and an audit record. The record now has `event`, `principal` and the correct level (previous F2 resolved). |
| `extproc` approval client | `extproc/delegation/{resolver,client}.go` | ✅ Has spans and an instrumented transport |

| Standard Rule | Location | Violation |
| --- | --- | --- |
| `OPENTELEMETRY.md` §3.2 and §3.3 | [authzen\_evaluation.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/adapters/http/enduser/authzen_evaluation.go) | The span `authzen.access_evaluation` comes from the `tokenexchange` tracer and has no span kind (F2) |
| `OPENTELEMETRY.md` §4.1 | [resolver.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/extproc/delegation/resolver.go) | The `access_evaluation.*` and `delegation.*` namespaces are not registered (F2) |
| `OPENTELEMETRY.md` §4.2 | [token-exchange.md](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/docs/reference/token-exchange.md) | `token_exchange.operation` still lists only `exchange`, `assertion_refresh` and `configuration` (F2) |
| `LOGGING.md` §2.2 and §8 | [server/delegation.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/internal/extproc/server/delegation.go) | The delegation denied, unavailable and expired records have no `event` (F6) |

🟢 The evaluation span (an Internal span) still sets `codes.Error` for 4xx request failures. The ExtProc resolver keeps denials off the error status. No credential or raw principal reaches a signal.

### E2E Test Assessment

| Feature Flow | True E2E Test? | Assessment |
| --- | --- | --- |
| US2.2: permit → revoke → `grant_missing` | Yes | Revocation now goes through the consent API ([denial test](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/tests/e2e/authzen_delegation_denial_test.go)). Previous F7 resolved. |
| US3.4 and US3.5: ExtProc revoke and re-consent, with no cached denial | Yes | Both use the API ([journey test](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/tests/e2e/delegation_context_journey_test.go)). Previous F7 resolved. |
| Evaluation audit record | Partial | 🟢 [authzen\_fixture\_test.go](https://github.com/zalando-incubator/agentic-identity-broker/blob/27f3fd9e457dbd4916174eb1c361e3ccb4a69d85/tests/e2e/authzen_fixture_test.go) checks that the `event` key exists but not its value. The level split is checked only in unit tests. |
| US1, US2.x, US3.x and US4 (unchanged) | Yes | 1:1 mapping kept. Writes go directly to storage only for setup states the API cannot create. |

### Maintainability Forecast

The remediation fixes the record shape cheaply by reusing the span-to-log pattern of `oauth2_token.go`. It does not lower the main long-term costs:

- **Shared contract (F5):** adding or renaming a reason code still means editing about 5 or 6 places, and nothing fails if one is missed.
- **OPA input (F3):** policy authors still need to know about two overlapping OPA fields.
- **Event IDs (F2):** the new event IDs are dashboard contracts that no document lists.

🟢 `ConsentToRepo()` defines a second grant in the E2E fixture. It is narrower than the seeded grant: Writer covers only Repo.

### Recommendation

**DISCUSS**: The automatic remediations for previous F2 and F7 are correctly applied and add no regressions. There are no 🔴 findings. Before merge:

- answer F1
- decide the open proposals F2 to F5; the event-name part of F2 is cheapest to settle before the identifiers ship
- apply the automatic fix F6, which adds `event` to the ExtProc delegation failure records

<details>
<summary>Review state</summary>

### Architecture review result

```json
{
  "run": "42087219381/1",
  "pr": 292,
  "head": "27f3fd9e457dbd4916174eb1c361e3ccb4a69d85",
  "recommendation": "DISCUSS",
  "summary": "DISCUSS: prev F2/F7 resolved; 6 🟡 remain (F1 scope authority, telemetry naming, OPA overlap, vocabulary, contract sync, ExtProc failure events).",
  "findings": {
    "RED": 0,
    "YELLOW": 6,
    "GREEN": 0
  },
  "failed": [],
  "items": [
    {
      "id": "F1",
      "severity": "YELLOW",
      "dimension": "data-authority",
      "bucket": "question",
      "title": "Permit scopes vs cached forwarded token after grant narrowing (prev. F1, open)",
      "question": "After a grant is narrowed but not revoked, which source is authoritative for upstream scopes: should ExtProc invalidate the ADR 012 token cache when the permit's scopes or grant.updated_at differ, or is the divergence accepted and to be documented in ADR 047?"
    },
    {
      "id": "F2",
      "severity": "YELLOW",
      "dimension": "otel-parity",
      "bucket": "proposed",
      "title": "AuthZEN telemetry identifiers off-standard (prev. F3, open + new event prefix)",
      "change": "Rename the span to tokenexchange.access_evaluation with an explicit kind, rename events to tokenexchange.access_evaluation.{permitted,denied,failed}, register access_evaluation.* and delegation.* in OPENTELEMETRY.md §4.1, and document the new operation/failure_detail values in docs/reference/token-exchange.md",
      "alternative": "Move the new attributes and events under the existing token_exchange namespace so coding-standards stays unchanged"
    },
    {
      "id": "F3",
      "severity": "YELLOW",
      "dimension": "coupling-cohesion",
      "bucket": "proposed",
      "title": "Overlapping mode-dependent permission-set fields in OPA input (prev. F4, open)",
      "change": "Name permission_sets as the canonical enabled-mode field in ADR 047 and policy docs, and track removal of granted_permission_sets from enabled-mode input in an issue",
      "alternative": "Omit granted_permission_sets from enabled-mode input now so each mode has exactly one permission-set field"
    },
    {
      "id": "F4",
      "severity": "YELLOW",
      "dimension": "ubiquitous-language",
      "bucket": "proposed",
      "title": "Vocabulary drift: DelegationResolver homonym, Delegation, resource.id (prev. F5, open)",
      "change": "Rename one DelegationResolver, add Delegation and Access Evaluation to the glossary, and give the permit's service identity a distinct member name from the request's resource URI",
      "alternative": "Keep the names and add glossary entries explaining the homonym and resource.id meaning in each direction"
    },
    {
      "id": "F5",
      "severity": "YELLOW",
      "dimension": "maintainability",
      "bucket": "proposed",
      "title": "Delegation Context shape and denial codes copied without sync check (prev. F6, open)",
      "change": "Add a conformance test under tests/ that checks the domain denial codes, the ExtProc allowlist and the context field set against the OpenAPI schema",
      "alternative": "Generate the ExtProc wire types and denial allowlist from the OpenAPI schema"
    },
    {
      "id": "F6",
      "severity": "YELLOW",
      "dimension": "otel-parity",
      "bucket": "auto",
      "title": "ExtProc delegation authorization-failure records lack event (new)",
      "change": "Add an event attribute to the ExtProc delegation denied, unavailable and permit-expired records, following the extproc.delegation_context.* family used by the lifecycle record"
    }
  ]
}
```

</details>

> Generated by [Architecture Review](https://github.com/zalando-incubator/agentic-identity-broker/actions/runs/42087219381) for #292 · pi · opus55 · \~420 AIC · ⌖ \~49 AIC · ⊞ 2.5K · [◷](https://github.com/search?q=repo%3Azalando-incubator%2Fagentic-identity-broker+%22gh-aw-workflow-call-id%3A+zalando-incubator%2Fagentic-identity-broker%2Farchitecture-review%22&type=issues)

<!-- gh-aw-workflow-call-id: zalando-incubator/agentic-identity-broker/architecture-review -->

<!-- gh-aw-agentic-workflow: Architecture Review, engine: pi, model: copilot/claude-opus-5.5, id: 42087219381, workflow_id: architecture-review, run: https://github.com/zalando-incubator/agentic-identity-broker/actions/runs/42087219381 -->

<!-- gh-aw-execution: {"version":1,"repo_id":"1295060661","repository":"zalando-incubator/agentic-identity-broker","aw_workflow_id":"architecture-review","run_id":"42087219381","run_attempt":"1","run_url":"https://github.com/zalando-incubator/agentic-identity-broker/actions/runs/42087219381","publishing_job":{"key":"safe_outputs","check_run_id":"127534487104"}} -->
