# OPA Authorization Mock

Demo OPA policy for ExtProc MCP authorization. Docker Compose enables authorization and approval
gating with this policy so the sample agent can exercise the complete approval journey.

## Policy

`bundle/aib/extproc/authz/main.rego` — package `aib.extproc.authz`

| Input                                                                                          | Decision |
|------------------------------------------------------------------------------------------------|----------|
| MCP lifecycle methods (`initialize`, `ping`, `notifications/initialized`)                      | allow    |
| MCP tool call in allowlist (`whoami`, `list_repositories`, `get_file_contents`, `search_code`) | allow    |
| MCP tool call `create_issue`                                                                   | approval_required |
| MCP tool call in denylist (`delete_repository`, `force_push`)                                  | deny     |
| Unknown type                                                                                   | deny     |

## Bundle structure

```
bundle/
└── aib/extproc/authz/
    └── main.rego      # OPA policy — edit this to change authorization rules
```

The directory structure mirrors the package path (`aib.extproc.authz`).

There are two independent workflows:

- **Local development** — `just opa-check` validates and builds the bundle into `bundles/` on the
  host so you can inspect it and verify the policy before running the stack.
- **Compose startup** — `opa-bundle-build` compiles the bundle into the `opa-bundles` named volume
  automatically. `opa-bundle-server` (nginx) serves it over HTTP for ExtProc's OPA client to poll.
  No manual steps needed.

## Modifying the policy

1. Edit `bundle/aib/extproc/authz/main.rego`
2. Validate and build locally:

```bash
just opa-check
```

This runs `opa check` (syntax + type check) and `opa build` (bundle compilation) via Docker — no
local OPA install needed.

3. Rebuild the bundle in the running stack:

```bash
just opa-reload
```

This rewrites `mcp-authz.tar.gz` in the nginx volume — nginx serves the new bundle immediately.
ExtProc's OPA client polls every 30–120 seconds and picks it up without an extproc restart.

## Testing the policy locally

The bundle server (nginx) exposes port `8080` on the host. You can verify the bundle is being
served and test the policy by running a temporary OPA container pointed at the local bundle:

**Verify the bundle is served:**
```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/mcp-authz.tar.gz
# Expected: 200
```

**Allow — read-only tool:**
```bash
echo '{"type":"mcp_tool_call","mcp":{"tool_name":"whoami"}}' | \
  docker run --rm -i -v "$(pwd)/mocks/opa/bundle:/bundle:ro" \
  openpolicyagent/opa:latest eval \
  --bundle /bundle --input /dev/stdin \
  'data.aib.extproc.authz.result'
```
Expected: `{"result": [{"expressions": [{"value": {"action": "allow"}, ...}]}]}`

**Deny — destructive tool:**
```bash
echo '{"type":"mcp_tool_call","mcp":{"tool_name":"delete_repository"}}' | \
  docker run --rm -i -v "$(pwd)/mocks/opa/bundle:/bundle:ro" \
  openpolicyagent/opa:latest eval \
  --bundle /bundle --input /dev/stdin \
  'data.aib.extproc.authz.result'
```
Expected: `{"result": [{"expressions": [{"value": {"action": "deny", "reasons": ["destructive operations are not permitted"]}, ...}]}]}`
