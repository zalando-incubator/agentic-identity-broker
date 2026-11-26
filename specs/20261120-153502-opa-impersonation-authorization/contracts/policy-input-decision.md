# Contract: OPA impersonation policy input and decision

**Feature**: `specs/20261120-153502-opa-impersonation-authorization` | **Sign-off**: plan.md Approval
Items API-INPUT (§ 1–2), API-DECISION (§ 3), ADR-OPA and CONST-AMEND (§ 4), DEC-BUILTINS (§ 5) |
**Research**: R4, R5 (withdrawn), R8, R9

The policy is evaluated at `data.<package>.<decision>` of the rule's `authorization.opa` block
(defaults `aib.broker.impersonation` and `result`) with the input below, once per evaluated OPA rule.
All OPA rules share one engine, and every OPA rule evaluated for one request uses the same admitted
revision. Evaluation runs after credential validation, extraction, subject classification, and category
eligibility, and before human delegation (the existing CEL position in the rule pipeline).

The input shares one OPA vocabulary with ExtProc (ADR `028-opa-extproc-authorization`, amended by
ADR `048-authzen-delegation-pip`): the same Envoy envelope (including `version`, `parsed_query`,
and `parsed_body` with Envoy's parsing rules), an `input.type` value, and the same
`input.context.agent` object as ExtProc's AuthZEN-fed Grant Context. § 1.1 defines the shared
and the reserved members.

## 1. Input document

```json
{
  "type": "impersonation",
  "version": {"ext_authz": "v3", "encoding": "protojson"},
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
  "parsed_body": {
    "grant_type": ["urn:ietf:params:oauth:grant-type:token-exchange"],
    "audience": ["https://broker.example.com/impersonate/550e8400-e29b-41d4-a716-446655440000"],
    "scope": ["read"],
    "client_assertion_type": ["urn:ietf:params:oauth:client-assertion-type:jwt-bearer"],
    "actor_token_type": ["urn:ietf:params:oauth:token-type:jwt"],
    "subject_token_type": ["urn:ietf:params:oauth:token-type:jwt"]
  },
  "truncated_body": false,
  "context": {
    "client_assertion": {"iss": "https://identity.example.com", "sub": "gateway-1", "aud": "https://broker.example.com/oauth2/token", "iat": 1795165140, "exp": 1795168800},
    "actor_token": {"iss": "https://identity.example.com", "sub": "automation-worker", "aud": "broker-impersonation", "iat": 1795165140, "exp": 1795168800},
    "subject_token": {"iss": "https://identity.example.com", "sub": "alice@example.com", "aud": "broker-impersonation", "iat": 1795165140, "exp": 1795168800},
    "subject_unverified": false,
    "agent": {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "canonical_id": "inventory-assistant",
      "external_id": "inventory-agent"
    },
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
| `type` | Constant | Always `"impersonation"`; ExtProc's values are `mcp_tool_call`, `mcp_method`, `mcp_headers_only`, and `unknown` |
| `version` | Constant | `{"ext_authz": "v3", "encoding": "protojson"}`, the value the Envoy plugin sets and ExtProc passes through |
| `attributes.request.http.method` | Request method | Always `POST` on the token endpoint |
| `attributes.request.http.path` | Request URL path | No query string |
| `attributes.request.http.host`, `.scheme` | `server.enduser.public_url` | Trusted configuration, not the `Host` header or TLS state |
| `attributes.request.http.protocol` | Request protocol | `HTTP/1.1` or `HTTP/2.0` |
| `attributes.request.http.headers` | Request headers | Lower-case names; repeated values joined with `, `; `authorization`, `proxy-authorization`, `cookie`, and pseudo-headers removed |
| `parsed_path` | Path segments | Envoy plugin semantics |
| `parsed_query` | Query parameters | Object of string arrays (Envoy semantics); normally `{}`. Unvalidated caller data the token endpoint ignores; exchange parameters come from the body |
| `parsed_body` | Form body | Envoy semantics for `application/x-www-form-urlencoded`: `url.ParseQuery` result as an object of string arrays, every form field **except the credential-bearing OAuth parameters `client_assertion`, `actor_token`, `subject_token`, `client_secret`, `refresh_token`, `code`, and `code_verifier`**, which are always omitted. Unvalidated caller data: the validated, resolved values are in `context`. `null` for any other content type, as in Envoy |
| `truncated_body` | Constant | `false`; the token endpoint rejects bodies over its 256 KiB limit instead of truncating |
| `context.client_assertion`, `.actor_token` | Validated, signature-verified claims | Same map as the CEL variable |
| `context.subject_token` | Signed and verified claims, or the caller-asserted claims of an unsigned subject | Same map as the CEL variable |
| `context.subject_unverified` | Rule's subject `verification` | `true` only for `verification: none` |
| `context.request` | Exchange parameters and resolved target | Same map as the CEL `request` variable; `resource` is always `""` |
| `context.agent.id` | Resolved broker Agent | Target Agent UUID; equal to `context.request.agent_id`. Same member and meaning as ExtProc `input.context.agent.id` |
| `context.agent.canonical_id` | Resolved broker Agent | Canonical ID; **omitted** when unset, never `null` or `""`. Same member and meaning as ExtProc `input.context.agent.canonical_id` |
| `context.agent.external_id` | Resolved broker Agent | Literal External ID; **omitted** when unset, never `null` or `""`; broker-owned UUID mapping, never caller- or bundle-supplied; no UUID or Canonical ID fallback. Not yet in ExtProc's Grant Context (§ 1.1) |

Absent optional claims stay absent. Strings, numbers, booleans, arrays, and nested objects keep their
JSON types. Empty claim maps are `{}`. The input has no other members: no
`attributes.request.http.body`, `raw_body`,
`source`, `destination`, `id`, `mcp`, raw JWT strings, or cookies. Caller headers cannot populate or
overwrite `context`, because `context` is built only from validated values.

Envoy compatibility covers the listed paths and their meanings. It does not guarantee that every
Envoy policy runs unchanged, for example policies that read the raw body, source addresses,
`authorization` headers, or the omitted credential fields. Policies that read `parsed_query`
or `parsed_body` see unvalidated caller values; authorization decisions on identity and target
belong on `context`.

### 1.1 Alignment with the ExtProc Grant Context

ExtProc's enabled-mode `input.context` is the typed projection of the broker's AuthZEN permit
(`specs/054-authzen-delegation-pip/contracts/extproc-grant-context.md` § 4). An organization can
run one bundle for both engines, so a shared name MUST have one meaning, and a name with an ExtProc
meaning MUST NOT be reused with another meaning here.

| `input.context` member | ExtProc (grant context enabled) | Impersonation input | Reason |
| --- | --- | --- | --- |
| `agent.id`, `agent.canonical_id` | Permit agent | Resolved target Agent, same shape and omission rule | One agent object across engines |
| `agent.external_id` | Absent | Resolved target Agent | Governance key (F1). Adding it to the AuthZEN permit and ExtProc is a separate change to ADR `048-authzen-delegation-pip` |
| `principal` | Permit `subject.id`, backed by an active UserGrant | **Absent** | OPA runs before human delegation; the extracted principal may be unsigned. Policies read `subject_token` and `subject_unverified` instead |
| `resource`, `grant`, `permission_sets` | Permit members | **Absent** | No Grant Context exists before delegation, and impersonation has no protected resource |
| `granted_permission_sets`, `granted_permission_sets_available` | Compatibility members | **Absent** | Same as above. An absent member makes ExtProc-style checks undefined, so they fail closed |
| `agent_session_id` | MCP session header | **Absent** | No MCP session on the token endpoint |

Outside `context`, `parsed_body` keeps Envoy's meaning in both engines but holds different
documents: ExtProc's is the decoded MCP JSON-RPC message, the broker's is the parsed token form.
Shared bundles separate the two by package: each impersonation rule names its own
`authorization.opa.package`, which ExtProc's `authorization.policy.package` does not use.
| `client_assertion`, `actor_token`, `subject_token`, `subject_unverified`, `request` | Absent | Present | CEL parity (FR-005); ExtProc does not use these names |

The impersonation input never contains a reserved ExtProc member. A later feature that runs OPA
after delegation and supplies a Grant Context MUST use the ExtProc members and shapes above.

Comparing `context.subject_token.sub` with `context.agent.id` or `context.request.agent_id`
restricts nothing. Policies that admit unverified agent subjects must compare
`context.subject_token.sub` with the intended agents, for example from governance data (§ 4).

## 2. Sensitivity

The `context` maps are sensitive policy input. The broker never writes the input or any part of it
to logs, spans, metrics, error responses, or OPA decision logs (decision logs cannot be configured,
see [impersonation-opa-config.yaml](impersonation-opa-config.yaml) `bootstrap_subset`). Print
statements are disabled. The named input type implements `LogValue`, `String`, and `GoString`,
each returning only `[REDACTED]` for nil, empty, and populated values (LOGGING § 6.4). Full input is
retained only for evaluation; formatting and JSON/text slog tests must prove no claims leak.

## 3. Decision document

| Result | Meaning | Response | Diagnostic (`stage/detail`) |
| --- | --- | --- | --- |
| Exactly one value, an object with `"action": "allow"` | Rule predicate satisfied; delegation and minting continue | Normal flow | — |
| Exactly one value, an object with `"action": "deny"` | Non-match; later rules are evaluated | No-match precedence (`403 access_denied`, `token exchange is not authorized`) | `client_authorization/client_policy_denied` (existing) |
| Undefined, more than one result, non-object, missing or non-string `action`, any other `action` (including `approval_required` and `ciba_required`) | Invalid decision; terminal | `500 server_error`, `internal server error during token exchange` | `client_authorization/opa_decision_invalid` (new) |
| Evaluation error, strict built-in error, timeout, cancellation | Evaluation failure; terminal | `500 server_error`, `internal server error during token exchange` | `client_authorization/opa_evaluation_failed` (new) |

For caller-disconnected HTTP requests, cancellation maps internally to the same terminal
`server_error`, but no HTTP response is promised to the disconnected client. Domain tests assert
that mapping; Scenario 3.2 observes client cancellation, the correlated `canceled` evaluation span,
no issuance, and no later-rule authorization. Connected failure variants still assert HTTP 500.

Other members of the decision object (`reasons`, `approval_context`, response headers) are ignored.
They are never logged, recorded, or returned. A decision is never reused for another request.

**Divergence from ExtProc's parser** (`internal/extproc/authorization/decision.go`): the same
document has different effects. The broker treats a decision as a rule predicate, so anything other
than `allow` or `deny` is an authoring error and is terminal, not a deny:

| Decision value | ExtProc | Impersonation |
| --- | --- | --- |
| `allow` | Forward | Rule matches |
| `deny` (with optional `reasons`) | 403, reasons used | Non-match; reasons ignored |
| `approval_required` | Tool approval flow | Terminal 500 `opa_decision_invalid` |
| `ciba_required` | Deny ("not yet supported") | Terminal 500 `opa_decision_invalid` |
| Undefined, non-object, unknown action | Deny | Terminal 500 `opa_decision_invalid` |

A bundle shared with ExtProc MUST therefore put impersonation decisions in a package of their own,
named by the rule's `authorization.opa.package`, that returns only `allow` or `deny`. `input.type`
remains available but is not the separation mechanism.

Minimal policy (Journey 1; spec Journey 1 Step 3 shows the matching input):

```rego
package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	input.attributes.request.http.method == "POST"
	input.parsed_path == ["oauth2", "token"]
	input.context.client_assertion.sub == "gateway-1"
	input.context.actor_token.sub == "automation-worker"
	registration := data.governance.agents[input.context.agent.external_id]
	registration.status == "approved"
	registration.application == "inventory-api"
}
```

## 4. Exceptional credential modes (DEC-BIND withdrawn)

The static binding analysis (DEC-BIND) was withdrawn on 2026-11-25. The broker does not analyze
which token maps an OPA decision references, and there is no `with`-modifier rule. The operator's
policy is the only control on caller-asserted values in these modes:

| Rule configuration | Broker behavior for an OPA rule | The policy must constrain |
| --- | --- | --- |
| `roles.subject.verification: none` | `subject_unverified: true`; the extracted email is minted when `email_expression` yields a nonempty email and the decision allows; a raw `sub` exactly equal to the target UUID is agent category | `context.subject_token`, including `email` when email is minted and `sub` when the rule allows agents |
| `roles.<role>.audience_requirement: absent` | The role's signature, issuer, lifetime, and absence of `aud` are still verified | That role's token map (`context.client_assertion`, `context.actor_token`, or `context.subject_token`) |
| Signed subject, expected audiences | Signed classification and signed email unchanged | Nothing beyond the business rule |

A raw `sub` or extracted principal equal to another registered agent's UUID stays a rejected
cross-agent request. CEL rules keep their startup reference and field-binding checks. Documentation
and examples show a policy constraining each value above.

A candidate is admitted only if every OPA rule's decision rule exists. Otherwise the whole candidate
is rejected for all rules, the rejection names the first failing rule, and the previous admitted
revision stays active for every rule. Every admitted snapshot must itself contain all configured
bundles. At startup, a usable subset is not admitted; if the subsequent complete candidate is
rejected, `impersonation.opa.startup_timeout` fails startup even if every bundle activated
historically.

## 5. Built-ins (DEC-BUILTINS)

Revisions compile with this OPA version's built-ins minus every built-in marked nondeterministic,
except `time.now_ns` and `io.jwt.decode_verify`, which are nondeterministic only because they read
the clock. OPA v1.21.4 marks 11 built-ins nondeterministic (`v1/ast/builtins.go`), so the removed
built-ins are `http.send`, `net.lookup_ip_addr`, `opa.runtime`, `rand.intn`, `uuid.rfc4122`, `io.jwt.encode_sign`, `io.jwt.encode_sign_raw`, `json.verify_schema`, and `json.match_schema`. The schema built-ins can resolve remote `$ref`s. The
check covers every module in the revision, reachable or not. A revision that calls one is rejected
before activation, so the call never runs.
