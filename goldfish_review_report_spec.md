# Goldfish Annotated Review Report — Round 1

Status: resolved

## Round 1 decisions (interactive breakdown)

- Applied: S1-1, S1-3, S1-4, S1-5 (documented as an accepted risk), S1-6 (references the existing rejected-revision metric), S1-7, S1-8, S1-9 (also propagated to the input contract, internal contracts, research R8, data model, ADR draft, and plan API-INPUT), S1-11, S1-12, S1-13, S1-14, S1-15, S1-16, S1-17, S1-18.
- Settled without change: S1-2 (skipped by the user: maintainer links not added), S1-10 (DEC-CONST confirmation happens in PR review).
- Declined nits: S1-19, S1-20.

**Target:** `specs/20261120-153502-opa-impersonation-authorization/spec.md` (full file, lines 1–656)

**Reviewers:** `design_critic` (completed: 0 Blockers, 10 Majors, 5 Nits) and `structural` (completed: 0 Blockers, 8 Majors, 9 Nits), both against `feature-spec-standard`.

After removing duplicates, there are 20 findings: 14 Major and 6 Nit. Seven combine findings that both reviewers raised.

## Prioritized Findings & AI Suggestions

### [S1-1] [`design_critic`-1-1, `structural`-1-1] - Severity: Major
*   **Affected Scope:** [Lines 21-121](specs/20261120-153502-opa-impersonation-authorization/spec.md#L21-L121) (Clarifications) and the whole document
*   **Discrepancy Description:** The spec has no Alternatives Considered section. Alternatives are spread across the Clarifications. Several lack a rejection rationale: lines 27-28, 95, 112, 116 and 120. The basic choices are not compared at all:
    *   OPA embedded in the broker, a sidecar, or a remote decision service
    *   reusing the ExtProc OPA path
    *   one shared engine or one engine per rule
*   **Rationale:** The feature-spec standard's Alternatives Rule requires each alternative to have a concrete rejection reason. The ADR draft has such a list (adr-20261121-061324-…md:171-194), but the spec neither links it nor summarizes it.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid. The reasoning already exists in research.md (R1, R2, R11, R4) and in the ADR, so the spec only needs to summarize it and link to it.
*   ***Proposed Edit:***
    *   Location: a new `## Alternatives Considered` section, inserted before `## Requirements`
    *   Change: a table with one row per alternative and the reason it was rejected:
        *   **Remote OPA or sidecar decision service:** it adds a synchronous per-request network dependency, contrary to Q1, and the broker could not reject a revision or tie binding evidence to the revision that decided the request. See research R1/R2.
        *   **OPA in ExtProc in front of the token endpoint:** ExtProc sees the request before validation, so it has no verified claims, resolved target or provenance. ADR `028-opa-extproc-authorization` scopes OPA to ExtProc and keeps broker CEL. See research R1.
        *   **CEL with external data:** the issue rules this out; it cannot reuse company Rego.
        *   **One OPA runtime per rule:** first-match fallthrough could mix policy versions, and shared governance data would be loaded once per rule. See R11 option A.
        *   **Policy-time lookups (Q1 B/C):** these put an issuance dependency on the governance service and send claims to request-time destinations.
        *   **UUID-only governance input:** operators would have to keep the UUID-to-governance mapping in sync themselves, although the broker is authoritative for External ID.
        *   **Network-only built-in denylist:** it fails open when an OPA upgrade adds a built-in. See R4.

        The section ends with a link to the ADR's Alternatives Considered section. The "Main alternative" clauses at lines 95, 98, 103, 107, 112, 116 and 120 each get a short "rejected because …" reason taken from research.md.

### [S1-2] [`design_critic`-1-2, `structural`-1-2] - Severity: Major
*   **Affected Scope:** [Lines 30, 35-39, 53-54](specs/20261120-153502-opa-impersonation-authorization/spec.md#L30-L54) (Clarifications Q2; sessions of 2026-11-25 and 2026-11-24)
*   **Discrepancy Description:** These entries rely on maintainer decisions but have no link:
    *   Q2 "support all modes including unsigned subjects", which drops the signed-only option
    *   "maintainer directed" the Envoy envelope
    *   "maintainer choice" to omit the credential fields
    *   "maintainer chose one shared OPA engine"

    Only Q1 (line 25) links its PR comment.
*   **Rationale:** The standard forbids crediting a rejection to a maintainer without linking the issue, discussion or PR. Q2 widens use of the constitution's unsigned-subject exception, so its authority needs to be traceable.
*   ***AI Suggestion:*** PROPOSE FIX (needs input from the user)
*   ***Justification:*** The finding is valid. The only comment URL in the feature directory is Q1's (`pull/546#issuecomment-7046411159`), so I cannot provide the other links. Each entry also gets a technical rationale, so the rejection no longer rests on attribution alone.
*   ***Proposed Edit:***
    *   Location: lines 30, 35, 38 and 53
    *   Change: add the PR or issue comment URL for each maintainer decision (**the user must supply these**). Line 53 also gets the reason for rejecting per-rule engines, which line 59-60 already states.

### [S1-3] [`design_critic`-1-3, `structural`-1-3, `design_critic`-1-12] - Severity: Major
*   **Affected Scope:** Whole document. Risks are spread across lines 190-197, 461, 622-626 and 651.
*   **Discrepancy Description:** The spec has no Risks & Mitigations section and states no scalability limits. Three gaps stand out:
    *   Memory use: the data document is held twice.
    *   Rules are coupled at admission.
    *   The per-request worst case is N OPA rules × up to 5 s each.

    The ADR's Negative and Risks subsections (ADR:144-169) are not surfaced in the spec.
*   **Rationale:** The standard requires failure modes, security implications and scalability limits, each with a mitigation.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid. The new section also holds the fixes for S1-5, S1-6, S1-7 and S1-8.
*   ***Proposed Edit:***
    *   Location: a new `## Risks & Mitigations` section after Alternatives Considered
    *   Change: a table with the columns risk, impact and mitigation. Rows:
        1. A weak policy references a token map without constraining it. Mitigation: documentation and examples.
        2. Ordered rules can bypass a governance restriction placed on only one rule (Journey 3 step 2). Mitigation: documentation.
        3. Stale governance data with no maximum age. Mitigation: the revision-age gauge plus the rejected-candidate signal from S1-6.
        4. Rules are coupled at admission. Mitigation: the rejection log names the rule.
        5. Memory: two copies of the data plus a transient third, and a full candidate rebuild per activation. Mitigation: the per-bundle `size_limit_bytes` setting (see S1-8).
        6. Worst-case request latency is the sum of the selected OPA rules' `evaluation_timeout` values. Mitigation: per-rule bounds; document the aggregate.
        7. OPA's internal status disagrees with the broker. Mitigation: broker telemetry is authoritative.
        8. Startup availability (S1-5).
        9. Replica divergence (S1-7).

        The section links to the ADR's Consequences section.

### [S1-4] [`design_critic`-1-4, `structural`-1-4] - Severity: Major
*   **Affected Scope:** [Lines 576-591](specs/20261120-153502-opa-impersonation-authorization/spec.md#L576-L591) (Domain Model diagram)
*   **Discrepancy Description:** The only diagram is the per-request sequence. Three things are not drawn:
    *   that OPA is embedded in-process
    *   the bundle service and token file
    *   the revision lifecycle: candidate, then admitted or rejected, then active; the last revision is retained on failure; each request reads one snapshot

    The template asks for a state or flow diagram for non-trivial state transitions.
*   **Rationale:** The revision lifecycle is the feature's core risk surface, and the spec describes it only in prose spread across FR-003, FR-004, FR-012 and SR-005.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid. A new reader cannot tell from the spec whether OPA is a library, a sidecar or a remote service.
*   ***Proposed Edit:***
    *   Location: after the sequence diagram (line 591)
    *   Change: add two Mermaid diagrams.
        1. A `flowchart LR` component view:
            *   token endpoint → impersonation domain service → policy port → in-process OPA adapter (shared engine)
            *   bundle service → OPA bundle plugin, with the token file feeding `aib_bearer_token_file`
            *   adapter → domain admission
        2. A `stateDiagram-v2` revision lifecycle:
            *   Fetched or Loaded → Candidate
            *   Candidate → Rejected when it is incomplete, uses a prohibited built-in, or lacks a binding; Rejected → [*]
            *   Candidate → Admitted → Active, replacing the previous Active
            *   Active keeps serving when an update fails
            *   Startup with no Admitted revision within `startup_timeout` → NotReady

### [S1-5] [`design_critic`-1-5] - Severity: Major
*   **Affected Scope:** [Lines 26, 547, 556, 601](specs/20261120-153502-opa-impersonation-authorization/spec.md#L547) (Q1 rationale, FR-003, FR-012, startup_timeout)
*   **Discrepancy Description:** Readiness requires a complete admitted snapshot, and `persist` is rejected. So any pod start during a bundle-server outage fails, and that takes down the whole broker, not just the OPA rules. This conflicts with Q1's claim that bundles avoid "a new synchronous availability dependency".
*   **Rationale:** This is an availability risk that the spec neither states nor mitigates.
*   ***AI Suggestion:*** PROPOSE FIX (document the risk; the design is unchanged)
*   ***Justification:*** The finding is valid. Q1 removed the per-request dependency but not the startup dependency, and I1 deliberately rejected partial or fallback readiness (lines 96-99). The spec should state this trade-off rather than change the design. If you want an actual mitigation, such as OPA-scoped readiness or persisted bundles, that is a design decision for you.
*   ***Proposed Edit:***
    *   Location: the S1-3 Risks table, plus one sentence in the Q1 rationale at line 26
    *   Change: the Q1 rationale says "…no new *per-request* availability dependency; the bundle service is a startup dependency (see Risks & Mitigations)". The Risks row reads: "A pod that starts during a bundle-service outage does not become ready, and the whole broker is unavailable on that pod. Accepted because serving without a complete admitted revision would violate SR-002 and I1. Mitigations: a highly available bundle service; rollout surge settings that keep ready pods running; local bundle directory mode for deployments that value availability over live updates."

### [S1-6] [`design_critic`-1-6] - Severity: Major
*   **Affected Scope:** [Lines 58-60, 443, 556, 626](specs/20261120-153502-opa-impersonation-authorization/spec.md#L626) (admission coupling, SR-006)
*   **Discrepancy Description:** Admission is all-or-nothing and there is no maximum age. So an unrelated rule's binding failure can block a revocation revision indefinitely while the old allow keeps working. The spec only requires documentation. The revision-age gauge cannot tell "nothing new published" from "updates are being rejected".
*   **Rationale:** This is a security-relevant failure mode with no operator-visible signal required by the spec.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid. contracts/observability.md already defines a revisions counter with an outcome attribute, but the spec does not require operators to be able to alert on rejected candidates.
*   ***Proposed Edit:***
    *   Location: SR-004 (line 624), plus a row in the Risks table
    *   Change: add to SR-004: "Every rejected candidate revision MUST be observable without logs, as a metric distinguishable from successful admissions, so operators can alert when governance updates stop activating." The Risks row covers a blocked revocation caused by an unrelated rule's binding failure.

### [S1-7] [`design_critic`-1-7] - Severity: Major
*   **Affected Scope:** [Lines 548, 638](specs/20261120-153502-opa-impersonation-authorization/spec.md#L638) (FR-004, SC-003) and Journey 2
*   **Discrepancy Description:** Each replica polls and admits revisions on its own schedule. So identical requests can get different outcomes during a rollout, and SC-003 holds only per instance.
*   **Rationale:** The consistency model across replicas is never stated.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid. A sentence and a Risks row are enough to fix it.
*   ***Proposed Edit:***
    *   Location: FR-004, SC-003 and the Risks table
    *   Change:
        *   FR-004 gets: "Activation is per broker instance; replicas converge independently within their configured polling interval plus admission time."
        *   SC-003 becomes: "…on the instance that activated it…"
        *   Risks row: replicas can disagree during convergence, and a replica whose admission fails stays on its old revision. Detection: `policy_admission_id` in deciding audits (SR-004) plus the rejection metric from S1-6.

### [S1-8] [`design_critic`-1-8] - Severity: Major
*   **Affected Scope:** [Lines 547, 572, 626](specs/20261120-153502-opa-impersonation-authorization/spec.md#L547)
*   **Discrepancy Description:** Local mode is capped at 16 MiB and 1,000 files, but managed bundles have no stated size limit. Memory and CPU cost per activation is also not stated.
*   **Rationale:** The standard requires scalability limits to be stated.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid. OPA v1.21.4 already supports a per-bundle `size_limit_bytes`, defaulting to 1 GiB (`v1/plugins/bundle/config.go:154`, `v1/bundle/bundle.go:54`). The allowed `bundles` key accepts it, so the limit can be documented without a new broker setting.
*   ***Proposed Edit:***
    *   Location: SR-006 (line 626) and the Risks table
    *   Change: add to SR-006: "Documentation MUST state that the broker holds governance data about twice in memory and rebuilds each candidate in full, and recommend bounding managed bundles with OPA's `bundles.<name>.size_limit_bytes` (OPA default 1 GiB)."

### [S1-9] [`design_critic`-1-9] - Severity: Major
*   **Affected Scope:** [Lines 38-39, 549, 623](specs/20261120-153502-opa-impersonation-authorization/spec.md#L623) (2026-11-25 session, FR-005, SR-003)
*   **Discrepancy Description:** `parsed_body` omits only the three JWT fields. A caller-supplied `client_secret`, `refresh_token`, `code` or `code_verifier` would therefore reach the policy, as would credential-like headers other than `authorization`, `proxy-authorization` and `cookie`. Meanwhile SR-003 says "secrets MUST NOT be included in policy input".
*   **Rationale:** The requirements contradict each other. The token handler parses `client_secret`, `refresh_token`, `code` and `code_verifier` as known parameters (oauth2_token.go:89-92), and the impersonation path does not reject them.
*   ***AI Suggestion:*** PROPOSE FIX (affects approval item API-INPUT)
*   ***Justification:*** The finding is valid. The broker can exclude every form parameter it knows to be a credential. It cannot know which arbitrary custom headers carry secrets, so SR-003 should say exactly what is excluded rather than promise "secrets" in general. This changes the API-INPUT proposal, so the maintainer's sign-off covers it.
*   ***Proposed Edit:***
    *   Location: line 38, FR-005 (line 549), SR-003 (line 623) and contracts/policy-input-decision.md
    *   Change:
        *   `parsed_body` always omits the credential-bearing OAuth parameters the token endpoint defines: `client_assertion`, `actor_token`, `subject_token`, `client_secret`, `refresh_token`, `code` and `code_verifier`.
        *   SR-003 replaces "secrets" with that list, the removed headers and "broker secrets", and adds: "Other caller-supplied headers and form fields are unvalidated caller data passed to policy; documentation MUST warn that callers can place arbitrary values there."

### [S1-10] [`design_critic`-1-10] - Severity: Major
*   **Affected Scope:** [Lines 557, 655](specs/20261120-153502-opa-impersonation-authorization/spec.md#L557) (FR-013, Assumptions)
*   **Discrepancy Description:** FR-013 makes unsigned-subject support mandatory, but it never says this depends on DEC-CONST. DEC-CONST is the open decision on whether the OPA controls are equivalent under Principle I. The spec also doesn't say what happens if DEC-CONST is rejected.
*   **Rationale:** Constitution Principle I requires "all ADR 031 compensating controls". The plan's DEC-CONST item (plan.md:333) is the open gate.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid. The gate exists in the plan but is invisible in the spec.
*   ***Proposed Edit:***
    *   Location: the end of the Assumptions bullet at line 655
    *   Change: "OPA support for unsigned subjects and unverified agent subjects additionally depends on maintainer confirmation (plan approval item DEC-CONST) that the superseding ADR's controls satisfy Principle I's existing carve-out. If DEC-CONST is not confirmed, those modes cannot ship for OPA rules until a constitution amendment, and Q2's scope must be revisited."

### [S1-11] [`structural`-1-5, `design_critic`-1-14] - Severity: Major
*   **Affected Scope:** [Lines 23-121](specs/20261120-153502-opa-impersonation-authorization/spec.md#L23-L121) (Clarifications)
*   **Discrepancy Description:** The sessions are out of date order: 11-20, 11-25, 11-24, 11-24, 11-21, then an undated block. The remediation IDs (F1, I1, A1, A2/I3, F4, F5, F6) are never defined. The research IDs R3, R11, R15 and R16 are not linked.
*   **Rationale:** A reader cannot tell which statement is current. For example, line 79 is superseded by lines 56-57.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid.
*   ***Proposed Edit:***
    *   Location: the Clarifications section
    *   Change:
        *   Reorder the sessions chronologically: 11-20, 11-21 (planning), Remediation decisions, 11-24, 11-24 (configuration revision), 11-25.
        *   Introduce the remediation block with one sentence: "IDs refer to findings of the cross-artifact analysis (`/speckit-analyze`) of this feature."
        *   Link each Rnn to `research.md#…`.

### [S1-12] [`structural`-1-6] - Severity: Major
*   **Affected Scope:** [Lines 417-467](specs/20261120-153502-opa-impersonation-authorization/spec.md#L417-L467) (Journeys 2 and 3)
*   **Discrepancy Description:** Journeys 2 and 3 have no Cast block. They introduce new cast members partway through the steps:
    *   the second agent and the External ID `other-agent`
    *   revisions `r1` and `r2`
    *   tokens `T1` and `T2`

    Four configuration-driven variants show no configuration or diagnostic: J2 steps 4 and 8, and J3 steps 4 and 5.
*   **Rationale:** The template's journey structure and Principle XIV ("show configuration where it affects the journey") require them.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid.
*   ***Proposed Edit:***
    *   Location: the start of Journeys 2 and 3, and J3 step 4
    *   Change:
        *   Add a **Cast and initial state** paragraph to each journey. Journey 2 names `r1`, `r2`, the second agent `660e…0001` / `other-agent`, and token files `T1` and `T2`. Journey 3 names its two rules.
        *   Add a YAML fragment to J3 step 4 showing a rule with `authorization.opa.policy_url` set.
        *   Add an example startup diagnostic, credential-free and taken from the contracts/observability.md `startup_failed` fields, for J2 step 4 and J3 step 4.

### [S1-13] [`structural`-1-7] - Severity: Major
*   **Affected Scope:** [Lines 207-242, 249-262, 453-459, 476-493](specs/20261120-153502-opa-impersonation-authorization/spec.md#L207-L262)
*   **Discrepancy Description:** None of the four YAML blocks has an explanatory comment.
*   **Rationale:** Template line 100 says to "comment how each value changes the outcome".
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid and cheap to fix.
*   ***Proposed Edit:***
    *   Location: the four YAML blocks
    *   Change: add inline comments to the lines that drive the outcome. For example:
        *   `type: opa  # this rule uses OPA instead of CEL`
        *   `package`/`decision  # evaluates data.company.impersonation.result`
        *   `opa: policy: config_file  # shared engine used by every OPA rule`
        *   `credentials.plugin: aib_bearer_token_file  # token re-read before every download`
        *   `verification: none  # unsigned alg:none subject (explicit opt-in)`
        *   `signs_roles: [client_assertion, actor]  # subject deliberately not signed`
        *   `allowed_subject_categories: [human]  # human delegation required`

### [S1-14] [`structural`-1-8] - Severity: Major
*   **Affected Scope:** [Lines 185, 463, 546](specs/20261120-153502-opa-impersonation-authorization/spec.md#L185) (Scenario 4.3, J3 step 4, FR-002)
*   **Discrepancy Description:** Scenario 4.3 doesn't cover FR-002's "`impersonation.opa` without an OPA rule" case, and it promises only a "rule-specific" error. Separately, no journey shows the "OPA rule without `impersonation.opa`" case that Scenario 4.3 does list.
*   **Rationale:** E2E tests map 1:1 to scenarios, so one FR-002 behavior has no test anchor.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid.
*   ***Proposed Edit:***
    *   Location: Scenario 4.3 (line 185) and J3 step 4 (line 463)
    *   Change:
        *   Scenario 4.3 Given adds "…, or a configured `impersonation.opa` block with no OPA rule". Its Then becomes "…with a rule-specific or engine-field configuration error…".
        *   J3 step 4 adds "an OPA rule while `impersonation.opa` is absent".

### [S1-15] [`design_critic`-1-15] - Severity: Nit
*   **Affected Scope:** [Lines 553, 609, 640](specs/20261120-153502-opa-impersonation-authorization/spec.md#L553)
*   **Discrepancy Description:** Rejecting an explicit `token_exchange.authorization.type: opa` at startup breaks existing configurations that set it. That seems to conflict with SC-005's "without configuration changes".
*   **Rationale:** The current validator checks `type` only for impersonation rules (validator.go:1150). The ordinary-exchange `type: opa` is accepted and ignored today.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is accurate. SC-005 covers *valid* CEL-only configurations, and an explicit `type: opa` never worked, but the spec should state the migration impact.
*   ***Proposed Edit:***
    *   Location: FR-009 (line 553)
    *   Change: append "A deployment that currently sets this value, which is accepted and ignored today, must remove it or set `cel` before upgrading; documentation MUST call this out."

### [S1-16] [`design_critic`-1-13] - Severity: Nit
*   **Affected Scope:** [Line 436](specs/20261120-153502-opa-impersonation-authorization/spec.md#L436) (Journey 2 step 4)
*   **Discrepancy Description:** "with the rule and field identified" is wrong for an engine-level `config_file` failure, which has no rule. Scenario 3.3 is correct.
*   **Rationale:** The two statements are inconsistent.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The fix is trivial.
*   ***Proposed Edit:***
    *   Location: line 436
    *   Change: "…exits unsuccessfully with the engine field identified, and the rule as well when a rule's admission failed…"

### [S1-17] [`design_critic`-1-11, `structural`-1-N2] - Severity: Nit
*   **Affected Scope:** [Lines 645-655](specs/20261120-153502-opa-impersonation-authorization/spec.md#L645-L655)
*   **Discrepancy Description:** There is no Goals / Non-goals block. The non-goals are buried in Assumptions.
*   **Rationale:** The standard lists this as an expected area.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Both reviewers raised it, and the fix is cheap.
*   ***Proposed Edit:***
    *   Location: a short `### Non-goals` list at the top of Assumptions
    *   Change: list the non-goals: proxy/hybrid impersonation, ordinary third-party exchange via OPA, remote OPA decision servers, policy-time lookups, token revocation, new admin APIs, persistence or UI, and MCP parsing.

### [S1-18] [`structural`-1-N5] - Severity: Nit
*   **Affected Scope:** [Lines 597-607](specs/20261120-153502-opa-impersonation-authorization/spec.md#L597-L607)
*   **Discrepancy Description:** The parent row `impersonation.opa` (block) comes after its child keys.
*   **Rationale:** The table is harder to read in that order.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The fix is trivial.
*   ***Proposed Edit:***
    *   Location: the configuration table
    *   Change: move the row at line 602 above line 599.

### [S1-19] [`structural`-1-N3, N4, N6] - Severity: Nit
*   **Affected Scope:** FR-003, SR-005, API Requirements, Domain Model layout
*   **Discrepancy Description:** FR-003 and SR-005 bundle many obligations into one item. The API bullets are not numbered. The Domain Model layout differs from the template.
*   **Rationale:** These are template conformance and traceability points.
*   ***AI Suggestion:*** DECLINE
*   ***Justification:*** plan.md, tasks.md and the approval items cite FR-003 and SR-005 by ID, so splitting or renumbering them now would cause churn across artifacts for little gain.
*   ***Proposed Edit:*** None.

### [S1-20] [`structural`-1-N1, N7, N8, N9] - Severity: Nit
*   **Affected Scope:** Lines 13, 17, 268, 296, 329-331, 505, plus ADR slug references
*   **Discrepancy Description:** Four small layout and format points:
    *   An H1 appears inside the verbatim issue blockquote.
    *   ADR slugs are not linked.
    *   HTTP fences end with blank lines, and the claim-map excerpt has no `...`.
    *   The local variant paragraph appears before Step 2.
*   **Rationale:** These are cosmetic.
*   ***AI Suggestion:*** DECLINE
*   ***Justification:*** The issue text is a verbatim quote. AGENTS.md requires references by full ADR file stem, which the spec already uses. Line 299 already says the excerpt is relevant fields only.
*   ***Proposed Edit:*** None.
