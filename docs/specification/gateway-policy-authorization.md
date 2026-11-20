---
title: "Gateway policy authorization"
description: Living specification of how a platform operator allows or denies a request at the gateway, such as one MCP tool call, with an Open Policy Agent policy. Every example on this page is checked against tests.
---

# Gateway policy authorization

An agent reaches a tool server through a gateway. The **ExtProc** service sits in that gateway and
exchanges the agent's token for one that the tool server accepts. A platform operator can make
ExtProc ask a policy first: may this request go through at all? This page describes that policy
check in the order an operator meets it.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the test that
proves it. `just living-docs` runs those tests and fails when the page and the gateway disagree.
The tests of this capability record outcomes, not HTTP exchanges, so this page has no compared
exchange blocks. The policy and the input document shown below are illustrations that the check
does not compare.
:::

## The cast

A platform operator runs the gateway in front of two MCP servers, `github-mcp` and
`salesforce-mcp`. The operator writes the policy in Rego, the language of Open Policy Agent (OPA).

Alice (`alice@example.com`) uses the agent *Research Assistant*. On the consent screen she granted
it the permission set `github-read`. The agent calls MCP tools such as `list_repositories`,
`create_issue`, and `delete_repository`.

## Turning the policy check on

Policy evaluation is off by default. The operator turns it on in the ExtProc configuration and
names one policy source: a local Rego file, or an OPA configuration file that lets OPA pull
policy bundles.

```yaml
authorization:
  enabled: true
  policy:
    path: "/etc/extproc/policy.rego"
    package: "aib.extproc.authz"
    decision: "result"
  default_decision: "deny"
  evaluation_timeout: 100ms
  max_body_size: 1048576
```

With a local Rego file, ExtProc compiles the policy when it starts and applies it to every
request. A read-only tool call is forwarded, and a destructive one is answered with `403` and the
reason that the policy gave. `example:gateway-policy-authorization.policy.allow-and-deny`

ExtProc refuses to start with a policy that it cannot use. A Rego file with a syntax error stops
the start, and the error names the file. `example:gateway-policy-authorization.configuration.invalid-policy`
A path that does not exist also stops the start. `example:gateway-policy-authorization.configuration.missing-policy-file`
An operator therefore learns about a broken policy at deployment, not from the first denied
request.

The two sources exclude each other. A configuration that sets both `policy.path` and
`policy.config_file` fails validation. `example:gateway-policy-authorization.configuration.one-source`

### Policy bundles

With `policy.config_file`, OPA loads the policy as its own configuration says, for example from
a bundle. ExtProc then decides requests with the bundle's policy. `example:gateway-policy-authorization.configuration.opa-config-file`

A bundle server can be unreachable when ExtProc starts. ExtProc still starts, and OPA keeps
trying. Until the bundle arrives there is no policy, so every request gets `403`. When the bundle
loads, the same request is forwarded. `example:gateway-policy-authorization.configuration.bundle-unavailable`

### When the check is off

With `authorization.enabled: false`, ExtProc does not ask for the request body. It exchanges the
token in the headers phase and the request goes on, exactly as it does in a deployment that never
configured a policy. `example:gateway-policy-authorization.configuration.disabled` The page
"Gateway token exchange" describes that exchange.

## What the policy sees

For each request, ExtProc builds one input document and gives it to the policy. The policy reads
it as `input`. The document has four parts:

- The HTTP request, in the layout of the OPA Envoy plugin: `input.attributes.request.http` with
  method, path, headers, and the raw body, plus `input.parsed_body`.
- `input.type`, which tells the policy what kind of request this is.
- `input.mcp`, the MCP part.
- `input.context`, what the token exchange said about the user's grant.

The gateway tells ExtProc which protocol a route carries. ExtProc does not guess it from the
body. For the protocol `mcp`, the kind of request decides the shape of `input.mcp`:

| Request | `input.type` | What `input.mcp` holds |
|---|---|---|
| `tools/call` | `mcp_tool_call` | `tool_name`, `arguments`, `target_server_name`, and `session_id` when the request has one |
| Another method, such as `initialize` | `mcp_method` | `method`, `params`, `target_server_name` |
| No body, such as the `GET` that opens a stream | `mcp_headers_only` | `target_server_name`, and `session_id` when the request has one |

A policy can match on the tool name and on single arguments of a tool call. `example:gateway-policy-authorization.input.tool-call`
It can match on the method and its parameters for other MCP methods. `example:gateway-policy-authorization.input.method`
A request without a body is evaluated too, in the headers phase, and it carries the target server
name. `example:gateway-policy-authorization.input.headers-only`

For any other protocol, `input.type` is `unknown` and there is no MCP part. The policy can still
decide on the raw body in `input.attributes.request.http.body`. `example:gateway-policy-authorization.input.unknown-protocol`

This is the input for Alice's agent calling `create_issue` on `github-mcp`, shortened. It is an
illustration built from the test fixtures; the check does not compare it.

```json
{
  "attributes": {
    "request": {
      "http": {
        "method": "POST",
        "headers": { "authorization": "Bearer …" },
        "body": "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"create_issue\",\"arguments\":{\"title\":\"Bug\",\"repo\":\"acme/app\"}}}"
      }
    }
  },
  "type": "mcp_tool_call",
  "mcp": {
    "jsonrpc": "2.0",
    "method": "tools/call",
    "id": 1,
    "tool_name": "create_issue",
    "arguments": { "title": "Bug", "repo": "acme/app" },
    "target_server_name": "github-mcp"
  },
  "context": {
    "granted_permission_sets_available": true,
    "granted_permission_sets": {
      "770e8400-e29b-41d4-a716-446655440001": ["22222222-2222-2222-2222-222222222222"]
    }
  }
}
```

### One policy for several MCP servers

`input.mcp.target_server_name` is the name that the gateway gives the MCP server of the route. It
is not part of the MCP message. With it, one policy can hold different rules for different
servers. Under a policy that allows tool calls only on `github-mcp`, a call of
`list_repositories` on `github-mcp` is forwarded. `example:gateway-policy-authorization.target-server.allowed`
The same call on `salesforce-mcp` gets `403`. `example:gateway-policy-authorization.target-server.denied`

### What the user granted

When ExtProc exchanges the token, the broker also returns the user's `granted_permission_sets`:
for each permission set that the user granted to this agent, the third-party services that the
grant includes. ExtProc copies this into `input.context`, so that a policy can allow a tool only
when the user granted the matching permission set.

`input.context.granted_permission_sets_available` says whether the broker supplied that
information. A policy must check it first, because "the user granted nothing" and "the broker did
not say" are different facts. The test policy shows the three cases. With a grant that includes
the required service, the tool is allowed. With an empty grant, it is denied as "required
permission set missing". Without the information, it is denied before the service check. `example:gateway-policy-authorization.permission-sets.policy`

Through the running service, a tool that the policy allows only with a grant for the service is
forwarded when the token exchange returned that grant. `example:gateway-policy-authorization.permission-sets.granted`
Through a real gateway, the same kind of tool is denied when the broker omits the grant. `example:gateway-policy-authorization.permission-sets.unavailable`

This policy is the test fixture behind several examples on this page. It is an illustration; the
check does not compare it.

```rego
package aib.extproc.authz

import rego.v1

default result := {"action": "deny", "reasons": ["tool is not permitted"]}

github_issues_service := "22222222-2222-2222-2222-222222222222"

granted(service_id) if {
	input.context.granted_permission_sets_available
	some permission_set_id
	service_id in input.context.granted_permission_sets[permission_set_id]
}

result := {"action": "allow"} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "list_repositories"
	granted(github_issues_service)
}

result := {"action": "approval_required", "approval_context": {"description": "Approval required for create_issue", "risk_level": "medium"}} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "create_issue"
	granted(github_issues_service)
}

result := {"action": "deny", "reasons": ["github-full permission set is required"]} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "delete_repository"
}

result := {"action": "ciba_required", "reasons": ["CIBA is deferred"]} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "elevated_action"
}
```

## What the policy answers

ExtProc reads one document from the policy, by default `result` in the package
`aib.extproc.authz`. Its `action` is one of four values.

| `action` | What the gateway does |
|---|---|
| `allow` | Forwards the request with the exchanged token. |
| `deny` | Answers `403`. The body carries the policy's `reasons`. |
| `approval_required` | Asks the user to approve this tool call. |
| `ciba_required` | Answers `403`. This action is not supported yet. |

Allow and deny are shown above. `example:gateway-policy-authorization.policy.allow-and-deny`

For `approval_required`, the gateway does not forward the call. It creates a pending
**ToolApproval** at the broker and answers the agent itself. `example:gateway-policy-authorization.decisions.approval-required`
The page "Tool approvals" describes that answer and the rest of the flow. It needs the tool approval settings of ExtProc.
Without them, `approval_required` is denied with `403` and the reason "approval_required is not
yet supported". `example:gateway-policy-authorization.decisions.approval-without-gate`

A policy can already return `ciba_required`, but the gateway denies it with `403` and creates no
approval. `example:gateway-policy-authorization.decisions.ciba-required` The reason is
"ciba_required is not yet supported", and ExtProc logs a warning that names the action. `example:gateway-policy-authorization.decisions.ciba-reason`

## When in doubt, the answer is no

The policy check never lets a request through because something went wrong.

- **No decision.** A policy that defines no `result` for a request denies it. `example:gateway-policy-authorization.fail-closed.undefined-result`
  Through a real gateway, a tool that no rule of the policy mentions gets `403`. `example:gateway-policy-authorization.policy.default-deny`
- **Timeout.** An evaluation that takes longer than `evaluation_timeout` denies the request with
  `403`, and the body says "evaluation timeout". `example:gateway-policy-authorization.fail-closed.timeout`
- **No protocol.** When the gateway does not say which protocol the route carries, ExtProc
  answers `403`. It neither evaluates the policy nor exchanges the token. `example:gateway-policy-authorization.fail-closed.missing-protocol`
- **No server name.** An MCP request without the gateway's server name gets `403` in the headers
  phase, before the body is read. `example:gateway-policy-authorization.fail-closed.missing-server-name`

The last two rules mean that a route which the operator forgot to configure fails visibly. It
cannot slip past a rule that is written for one server.

## Batch requests

An MCP body can be a JSON array of several JSON-RPC messages. ExtProc evaluates the policy once
for each message, with that message's own tool name and body. When every message is allowed, the
whole batch is forwarded unchanged. `example:gateway-policy-authorization.batch.all-allowed`

When the policy denies any message, the whole batch gets one `403`. Its body lists the reasons of
every denied message. `example:gateway-policy-authorization.batch.any-denied`

A batch cannot ask the user for approval. When a message in a batch evaluates to
`approval_required`, the batch gets `403` with a reason that tells the agent to send that tool
call as a standalone request. No approval is created. `example:gateway-policy-authorization.batch.approval-required`

## The audit log

ExtProc writes one structured log entry, "opa authorization decision", for each evaluation. The
entry belongs to the trace of the request, so an operator can find it from the trace ID. `example:gateway-policy-authorization.audit.decision-log`
The entry has the fields `action`, `reasons`, `tool_name`, `method`, `protocol`,
`target_server_name`, `duration_ms`, and `result_code`.

The log entry and the trace span describe the decision, not the request. They carry the tool name
and the target server name. They do not carry request headers, the request body, or free text
from the request. `example:gateway-policy-authorization.audit.no-request-content`

## Not yet on this page

- The remaining fields of the input document: `parsed_path`, `parsed_query`, `truncated_body`,
  `version`, and the MCP `session_id`. No test on this page reads them.
- The body size limit `max_body_size` and requests whose MCP members are ambiguous. Both are
  denied; the tests are in `internal/extproc/server/server_test.go`.
- The exact JSON of a `403` answer, the settings `policy.package` and `policy.decision` with
  other values than the defaults, and how OPA masks tokens in its own decision logs.
- Changing a local Rego file while ExtProc runs.
- The approval flow after `approval_required`: see "Tool approvals". The token exchange itself:
  see "Gateway token exchange".
- Two examples need Docker and a real gateway: the default deny for an unmatched tool and the
  denial when the broker omits the granted permission sets.

## Sources

The behavior on this page was decided in `specs/020-extproc-opa-authorization/spec.md`,
`specs/044-extproc-opa-mcp-target-server/spec.md`, `specs/026-extproc-approval-sync/spec.md`,
`specs/019-permission-sets/spec.md`, and `adrs/028-opa-extproc-authorization.md`. The
configuration example is `examples/config/extproc-opa-authorization.yaml`. The scenarios are in
`tests/e2e/extproc/opa_authorization_test.go`, `tests/e2e/extproc/approval_sync_test.go`, and
`tests/e2e/extproc/opa_agentgateway_e2e_test.go`. The tests are in
`internal/extproc/server/server_test.go`, `internal/extproc/authorization/authorizer_test.go`,
and `internal/extproc/authorization/authorizer_safety_test.go`. The policy fixtures are in
`tests/e2e/extproc/fixtures/policies/`.
