# Contract: Granted Capabilities in the Delegation Permit and OPA Input

**Target files**: `api/enduser/openapi.yaml` (`1.3.0` → `1.4.0`), `docs/api/authzen-evaluation.md` | **Spec**: FR-014–FR-016, API-002, US3.1–US3.6 | **Research**: R7, R8, R12

This extends the ADR `048-authzen-delegation-pip` permit and enabled-mode OPA input by one member. Every existing member keeps its meaning. The new ADR `20261127-190555-capability-model` records the extension.

## 1. AuthZEN permit (`POST /access/v1/evaluation`)

`GrantContext` gains one required member:

```yaml
GrantContext:
  required: [subject, agent, resource, grant, permission_sets, granted_capabilities]
  properties:
    # subject, agent, resource, grant, permission_sets: unchanged
    granted_capabilities:
      type: array
      minItems: 1
      uniqueItems: true
      items: { type: string, minLength: 1, maxLength: 128 }
      description: >
        Canonical IDs of every capability the grant holds, across all granted permission sets,
        sorted by byte order and de-duplicated. The list does not identify which set granted a
        capability. Reflects the grant and capability definitions at evaluation time within the
        permit's max-age.
```

**Example** (Journey 1, abbreviated):

```json
{
  "decision": true,
  "context": {
    "subject": { "id": "alice@example.com" },
    "agent": { "id": "550e8400-e29b-41d4-a716-446655440000" },
    "resource": { "service_id": "880e8400-e29b-41d4-a716-446655440010", "resource_uri": "https://mcp.example.com/calendar", "scopes": ["https://www.googleapis.com/auth/calendar.events", "https://www.googleapis.com/auth/calendar.readonly"] },
    "grant": { "id": "aa0e8400-e29b-41d4-a716-446655440020", "updated_at": "2026-11-27T09:30:00Z" },
    "permission_sets": [ { "id": "770e8400-e29b-41d4-a716-446655440003", "canonical_id": "google-calendar", "services": [ { "id": "880e8400-e29b-41d4-a716-446655440010", "scopes": ["https://www.googleapis.com/auth/calendar.events", "https://www.googleapis.com/auth/calendar.readonly"] } ] } ],
    "granted_capabilities": ["google-calendar-edit", "google-calendar-view"]
  }
}
```

- **Member values**: `permission_sets[].services[]` is the union of the services of that set's granted capabilities. Their scopes are the union of those capabilities' scopes for the service (FR-015).
- **Denials**: Denials are unchanged. A stale capability yields the existing `grant_permission_set_missing` denial with the consent URL, and an undeclared one yields `grant_permission_set_undeclared` (US3.4).
- **Version**: End-user API `info.version` becomes `1.4.0`, an additive change.

## 2. Token-exchange response

`TokenExchangeResponse` is unchanged: it gains no capability field, and the deprecated `granted_permission_sets` keeps its meaning (FR-016).

## 3. ExtProc decode (`internal/extproc/grantcontext`)

- `wireContext` gains `GrantedCapabilities *[]string` (`json:"granted_capabilities"`).
- `parsePermitContext` rejects the permit when the member is absent, null, empty, or holds an empty string, a duplicate, or an unsorted list. The rejection uses the existing malformed-permit path: terminal 503, no cache entry.
- `Context` gains `GrantedCapabilities []string`, and consumers copy the slice.
- The permit cache key, singleflight key and `ExchangeResultBinding` are unchanged.

## 4. OPA input

```go
// internal/extproc/authorization/input.go — ContextInput
GrantedCapabilities []string `json:"granted_capabilities,omitempty"`
```

| Mode | `input.context.granted_capabilities` |
|---|---|
| `grant_context.enabled: true` (body, batch, header-only paths) | Copied from the permit (`contextInputFromGrantContext`) |
| `grant_context.enabled: false` | Absent: the struct field is never set. Disabled-mode JSON is unchanged byte for byte (US3.6) |

`granted_permission_sets`, `granted_permission_sets_available` and `permission_sets` keep their current derivation from the permit (FR-015, US3.5).

## 5. Sample policy (`examples/policies/capability-tool-map.rego`)

- **Package**: The policy uses the package and rule names that the ExtProc OPA engine expects. It copies them from the existing ExtProc fixtures (`tests/e2e/extproc/fixtures/policies/`).
- **Decisions**: It is deny by default and maps each tool name to one capability canonical ID (Journey 1). It allows `mcp_tool_call` only when the mapped ID is in `input.context.granted_capabilities`.
- **Other requests**: Non-tool-call MCP methods (`initialize`, `tools/list`) are allowed. This matches the existing ExtProc fixture policies, so the sample stays usable behind a real gateway.
- **Tests**: The ExtProc E2E suite and an `opa test` unit run load this exact file.

## 6. Compatibility and rollout

- **Old ExtProc with new broker**: The unknown member is ignored, and behavior is as today.
- **New ExtProc with old broker**: The permit is rejected as malformed (503) in enabled mode. Disabled mode is unaffected. The documented order is broker first, then ExtProc.
- **Policies**: Policies that read only `granted_permission_sets` or `permission_sets` decide identically (SC-002).

## 7. Contract tests

- **Broker**:
  - `internal/domain/tokenexchange` projection unit tests cover order, union and de-duplication.
  - `internal/adapters/http/enduser/authzen_evaluation_test.go` checks the response body.
  - The E2E tests `tests/e2e/authzen_delegation_test.go` and `grant_context_journey_test.go` gain assertions.
- **ExtProc**:
  - `grantcontext/client_test.go` covers the decode cases above.
  - `server/grant_context_test.go` covers enabled-mode input with JSONEq.
  - The existing disabled-mode exact test stays unchanged.
  - `tests/e2e/extproc/fixtures/policies/grant_context_disabled_input.rego` adds `granted_capabilities` to its forbidden-member list.
