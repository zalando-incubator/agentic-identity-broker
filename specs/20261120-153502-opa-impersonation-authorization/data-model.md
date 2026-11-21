# Data model: OPA policies for impersonation authorization

**Feature**: `specs/20261120-153502-opa-impersonation-authorization` | **Research**: [research.md](research.md)

No persisted entity, aggregate, repository, migration, or typed entity ID is added. Agent and
UserGrant are unchanged. All new concepts are configuration or request-local value objects.

## Value objects

### Impersonation Rule (extended)

| Field | Type | Rules |
| --- | --- | --- |
| `authorization.type` | `cel` \| `opa` | Exactly one engine; unknown fails startup |
| `authorization.cel` | CEL block | Required for `cel`; empty for `opa` |
| `authorization.opa` | OPA authorization | Required for `opa`; empty for `cel` |
| Binding requirements | derived set of policy roles | `subject_token` if `roles.subject.verification: none`; each role with `audience_requirement: absent` |

Ordering, first-match semantics, roles, trusted issuers, and `allowed_subject_categories` are
unchanged. Contract: [contracts/impersonation-opa-config.yaml](contracts/impersonation-opa-config.yaml).

### OPA Authorization (configuration)

| Field | Type | Default | Validation |
| --- | --- | --- | --- |
| `policy.config_file` | path | — | Required, readable, bootstrap subset (R3) |
| `policy.package` | Rego package path | — | Required, identifier segments |
| `policy.decision` | Rego identifier | `result` | Identifier |
| `evaluation_timeout` | duration | `100ms` | `10ms`–`5s` |
| `startup_timeout` | duration | `30s` | `1s`–`5m` |

### Impersonation Policy Input

Request-local document described in [contracts/policy-input-decision.md § 1](contracts/policy-input-decision.md#1-input-document).
Built from validated claim maps, `subject_unverified`, the CEL `request` map, the configured public
URL, and filtered transport metadata. Never persisted, logged, or exported.

### Impersonation Policy Decision

| Value | Effect |
| --- | --- |
| `allow` | Rule predicate satisfied |
| `deny` | Rule non-match, later rules evaluated |
| invalid (undefined, malformed, other action) | Terminal `500`, `opa_decision_invalid` |
| failure (error, timeout, cancellation) | Terminal `500`, `opa_evaluation_failed` |

### Policy/Data Revision

An immutable snapshot owned by one OPA rule's policy source.

| Field | Meaning |
| --- | --- |
| Prepared query | Compiled from the activation's modules with restricted capabilities |
| Data snapshot | Copy of the data document at that activation |
| Evidence | Role references and `sub`/`email` field bindings from the reachable decision graph |
| Label | `policy_revision`: sorted `<bundle>=<manifest revision>` list |
| Admitted at | Time of admission, for the age gauge |

A CEL rule has one static revision built from its compiled predicate. Its evidence uses the existing
`References` and `Binds` results.

### Binding Evidence

| Field | Meaning |
| --- | --- |
| References | Set of `client_assertion`, `actor_token`, `subject_token` referenced by the decision |
| Field bindings | Subset of `subject_token.sub`, `subject_token.email` (other fields are not recorded) |

## State transitions (one OPA rule)

```mermaid
stateDiagram-v2
    [*] --> Starting: NewService starts runtime
    Starting --> Ready: bundles activated once and a candidate admitted
    Starting --> Failed: startup_timeout elapsed or bootstrap invalid
    Failed --> [*]: broker exits, runtimes stopped
    Ready --> Ready: activation → candidate admitted → active revision swapped
    Ready --> Ready: activation → candidate rejected → previous revision stays
    Ready --> Ready: bundle fetch fails → previous revision stays
    Ready --> Stopped: broker shutdown
    Stopped --> [*]
```

Candidate rejection reasons: `compile_failed`, `prohibited_builtin`, `decision_undefined`,
`binding_missing`, `evidence_voided` (only when a requirement exists), `snapshot_failed`.

## Per-request use

```mermaid
sequenceDiagram
    participant H as Token handler
    participant S as impersonation.Service
    participant R as Rule revision (one read)
    H->>S: Impersonate(request + HTTP attributes, target)
    loop each rule in order
        S->>S: validate credentials, extract identities
        S->>R: current()
        S->>S: classify subject with R.evidence (sub binding)
        S->>S: check category eligibility and cross-agent rule
        S->>S: extract email if signed, or if R.evidence binds email
        S->>R: evaluate(input)
        alt allow
            S->>S: human delegation (human only), mint token
        else deny
            S->>S: next rule
        else invalid or failure
            S-->>H: 500 server_error (abort)
        end
    end
```

## Glossary additions (`ARCHITECTURE.md`)

| Term | Definition |
| --- | --- |
| **Impersonation Policy Engine** | The engine one impersonation rule selects for its authorization decision: `cel` or `opa`. |
| **Policy/Data Revision** | An immutable, admitted snapshot of an OPA rule's compiled policy, data, and binding evidence. Each rule evaluation uses one revision. |
| **Binding Evidence** | The token-map references and `sub`/`email` field bindings of a rule's authorization, computed from the compiled CEL predicate or the reachable Rego decision graph. Mandatory references gate startup and every OPA activation. |
| **Policy Bootstrap** | The OPA-native configuration file of an OPA rule, limited to `services`, `bundles`, `keys`, and `labels`. |
