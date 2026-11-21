# Contract: OPA impersonation policy input and decision

**Feature**: `specs/20261120-153502-opa-impersonation-authorization` | **Sign-off**: plan.md Approval
Items API-INPUT (§ 1–2), API-DECISION (§ 3), DEC-BIND (§ 4), DEC-BUILTINS (§ 5) | **Research**: R4,
R5, R8, R9

The policy is evaluated at `data.<policy.package>.<policy.decision>` with the input below, once per
evaluated OPA rule, after credential validation, extraction, subject classification, and category
eligibility, and before human delegation (the existing CEL position in the rule pipeline).

## 1. Input document

```json
{
  "attributes": {
    "request": {
      "http": {
        "method": "POST",
        "path": "/oauth2/token",
        "host": "broker.example.com",
        "scheme": "https",
        "protocol": "HTTP/1.1",
        "headers": {
          "content-type": "application/x-www-form-urlencoded",
          "user-agent": "gateway/1.0"
        }
      }
    }
  },
  "parsed_path": ["oauth2", "token"],
  "parsed_query": {},
  "parsed_body": null,
  "truncated_body": false,
  "context": {
    "client_assertion": {"iss": "https://identity.example.com", "sub": "gateway-1", "aud": "https://broker.example.com/oauth2/token", "iat": 1795165140, "exp": 1795168800},
    "actor_token": {"iss": "https://identity.example.com", "sub": "automation-worker", "aud": "broker-impersonation", "iat": 1795165140, "exp": 1795168800},
    "subject_token": {"iss": "https://identity.example.com", "sub": "alice@example.com", "aud": "broker-impersonation", "iat": 1795165140, "exp": 1795168800},
    "subject_unverified": false,
    "request": {
      "resource": "",
      "grant_type": "urn:ietf:params:oauth:grant-type:token-exchange",
      "scope": "read",
      "agent_id": "550e8400-e29b-41d4-a716-446655440000"
    }
  }
}
```

| Path | Source | Rules |
| --- | --- | --- |
| `attributes.request.http.method` | Request method | Always `POST` on the token endpoint |
| `attributes.request.http.path` | Request URL path | No query string |
| `attributes.request.http.host`, `.scheme` | `server.enduser.public_url` | Trusted configuration, not the `Host` header or TLS state |
| `attributes.request.http.protocol` | Request protocol | `HTTP/1.1` or `HTTP/2.0` |
| `attributes.request.http.headers` | Request headers | Lower-case names; repeated values joined with `, `; `authorization`, `proxy-authorization`, `cookie`, and pseudo-headers removed |
| `parsed_path` | Path segments | Envoy plugin semantics |
| `parsed_query` | Query parameters | Object of string arrays; normally `{}` |
| `parsed_body`, `truncated_body` | Constants | `null` and `false`; the form body is never input |
| `context.client_assertion`, `.actor_token` | Validated, signature-verified claims | Same map as the CEL variable |
| `context.subject_token` | Signed and verified claims, or the caller-asserted claims of an unsigned subject | Same map as the CEL variable |
| `context.subject_unverified` | Rule's subject `verification` | `true` only for `verification: none` |
| `context.request` | Exchange parameters and resolved target | Same map as the CEL `request` variable; `resource` is always `""` |

Absent optional claims stay absent. Strings, numbers, booleans, arrays, and nested objects keep their
JSON types. Empty claim maps are `{}`. The input has no other members: no `body`, `raw_body`,
`source`, `destination`, `id`, raw JWT strings, or cookies. Caller headers cannot populate or
overwrite `context`, because `context` is built only from validated values.

Envoy compatibility covers the listed paths and their meanings. It does not guarantee that every
Envoy policy runs unchanged, for example policies that read bodies, source addresses, or
`authorization` headers.

## 2. Sensitivity

The `context` maps are sensitive policy input. The broker never writes the input or any part of it
to logs, spans, metrics, error responses, or OPA decision logs (decision logs cannot be configured,
see [impersonation-opa-config.yaml](impersonation-opa-config.yaml) `bootstrap_subset`). Print
statements are disabled.

## 3. Decision document

| Result | Meaning | Response | Diagnostic (`stage/detail`) |
| --- | --- | --- | --- |
| Exactly one value, an object with `"action": "allow"` | Rule predicate satisfied; delegation and minting continue | Normal flow | — |
| Exactly one value, an object with `"action": "deny"` | Non-match; later rules are evaluated | No-match precedence (`403 access_denied`, `impersonation not permitted`) | `client_authorization/client_policy_denied` (existing) |
| Undefined, more than one result, non-object, missing or non-string `action`, any other `action` (including `approval_required` and `ciba_required`) | Invalid decision; terminal | `500 server_error`, `authorization evaluation failed` | `client_authorization/opa_decision_invalid` (new) |
| Evaluation error, strict built-in error, timeout, cancellation | Evaluation failure; terminal | `500 server_error`, `authorization evaluation failed` | `client_authorization/opa_evaluation_failed` (new) |

Other members of the decision object (`reasons`, `approval_context`, response headers) are ignored.
They are never logged, recorded, or returned. A decision is never reused for another request.

Minimal policy (Journey 1):

```rego
package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	input.attributes.request.http.method == "POST"
	input.parsed_path == ["oauth2", "token"]
	input.context.client_assertion.sub == "gateway-1"
	input.context.actor_token.sub == "automation-worker"
	registration := data.governance.agents[input.context.request.agent_id]
	registration.status == "approved"
	registration.application == "inventory-api"
}
```

## 4. Binding evidence (DEC-BIND)

Evidence is computed once per candidate revision from the rules reachable from the decision rule.

| Rego form in a reachable rule | Role reference | Field binding |
| --- | --- | --- |
| `input.context.subject_token.sub == "x"` | `subject_token` | `subject_token.sub` |
| `input.context.subject_token["email"] == e` | `subject_token` | `subject_token.email` |
| `import input.context.subject_token as s` then `s.email == e` | `subject_token` | `subject_token.email` |
| `s := input.context.subject_token` then `s.sub in approved` (same rule body) | `subject_token` | `subject_token.sub` |
| `helper` rule in another package that the decision references, containing any form above | Same as the form | Same as the form |
| `check(input.context.subject_token)` with `check(t) if t.email == e` | `subject_token` | none (not followed through arguments) |
| `input.context[k].sub` (non-ground key) | none | none |
| `input.context` or `input` alone | none | none |
| Any form in a rule the decision does not reach | none | none |
| Any reachable `... with input as x` or `with input.context... as x` | voids all evidence of the revision | voids all evidence |

Admission per rule configuration:

| Rule configuration | Required for admission | Used per request (same revision) |
| --- | --- | --- |
| `roles.subject.verification: none` | `subject_token` reference | `subject_token.email` binding → mint the extracted email; `subject_token.sub` binding → exact-target subject may be agent |
| `roles.<role>.audience_requirement: absent` | Reference to that role's map | — |
| Signed subject, expected audiences | Nothing | Signed classification and signed email unchanged |

A candidate without a decision rule, or failing a requirement, is rejected; the previous admitted
revision stays active. At startup, no admitted revision within `startup_timeout` fails startup.

## 5. Built-ins (DEC-BUILTINS)

Revisions compile with this OPA version's built-ins minus every built-in marked nondeterministic,
except `time.now_ns`. In OPA v1.21.4 the removed built-ins are `http.send`, `net.lookup_ip_addr`,
`opa.runtime`, `rand.intn`, `uuid.rfc4122`, `io.jwt.encode_sign`, and `io.jwt.encode_sign_raw`. The
check covers every module in the revision, reachable or not. A revision that calls one is rejected
before activation, so the call never runs.
