# Contract: Virtual Service Scope

**Status**: Proposed. Approval items API-CONSENT, API-LOCAL-SCOPES, DEC-001, and ADR-SCOPES (see
[plan.md](../plan.md#approval-items)).

## Grammar

```text
virtual-service-scope = public-url "/sessions/" identifier
public-url            = server.enduser.public_url with trailing "/" removed (configuration only)
identifier            = uuid-lower / canonical-id
uuid-lower            = 8HEXLOWER "-" 4HEXLOWER "-" 4HEXLOWER "-" 4HEXLOWER "-" 12HEXLOWER
canonical-id          = 1*128( ALPHA / DIGIT / "." / "_" / "-" )   ; never UUID-shaped
```

Classification of one space-delimited scope token `s`:

| Condition | Kind | Result at authorization and refresh |
|---|---|---|
| `s` does not start with the exact string `<public-url>/sessions/` | Ordinary | Existing ordinary-scope rules apply (`allowed_scopes`, reserved refresh scopes). The value never selects a broker service. |
| `s` has the prefix, and the remainder is `uuid-lower` or `canonical-id` | Virtual | It resolves to a service (see Resolution). |
| `s` has the prefix, but the remainder is empty or contains `/`, `?`, `#`, `%`, or uppercase hex in a UUID shape | Malformed | `invalid_scope` |

Examples with `public_url: https://broker.example.com`:

| Token | Kind |
|---|---|
| `https://broker.example.com/sessions/github` | Virtual (canonical) |
| `https://broker.example.com/sessions/880e8400-e29b-41d4-a716-446655440010` | Virtual (UUID) |
| `https://broker.example.com/sessions/880E8400-E29B-41D4-A716-446655440010` | Malformed |
| `https://broker.example.com/sessions/github/` | Malformed |
| `https://broker.example.com/sessions/` | Malformed |
| `https://other.example.com/sessions/github` | Ordinary |
| `https://BROKER.example.com/sessions/github` | Ordinary |
| `repo` | Ordinary |

## Resolution

1. A UUID identifier is looked up by primary key. A canonical identifier is looked up by its
   **current** canonical ID. No historical aliases are kept (DEC-001).
2. The service must be eligible for the agent:
   - when the agent declares `service_requirements`, the service must appear in them;
   - otherwise, one of the agent's declared permission sets must cover the service.
3. An unknown or ineligible service returns `invalid_scope`. A repository failure returns
   `server_error`.
4. Duplicate tokens for the same service form one Requested Service Set entry.
5. When the request carries an existing Service-Scope Binding, the resolved UUID must equal the
   bound UUID. This applies independently at grant submission, protected authorize re-entry after
   Save, code exchange, and refresh. The post-Save URL carries the same JWE as `session_token`; the
   authorize endpoint uses its sealed original request/bindings, not fresh unbound browser data.
   A mismatch returns `invalid_scope` before code issuance or proxy redirect (FR-019).

## Emission

The broker emits a virtual scope in token-exchange error `scope` members and in documentation
examples. It uses `<public-url>/sessions/<canonical_id>` when the service has a canonical ID, and
`<public-url>/sessions/<uuid>` otherwise. The broker never emits the scope:

- in `/.well-known/oauth-authorization-server` `scopes_supported` (FR-014);
- in an upstream authorization or refresh request (FR-013);
- as a provider scope at the third-party authorization endpoint.

## Granted scope and identity durability

- A granted virtual scope keeps the exact requested token. The broker does not rewrite it.
- The UUID form stays valid across canonical-ID renames.
- The canonical form is valid only while that canonical ID still names the bound service. After a
  rename, local refresh returns `invalid_scope` and does not consume the refresh token. The client
  obtains a fresh challenge and authorizes interactively again.
- Successful refresh narrowing persists the narrowed `Scope` and returns that same response/JWT
  scope. All original bindings remain inherited as continuing-service assurance, including removed
  virtual scopes. Later scope-less refresh uses the narrowed `Scope` and rechecks every binding;
  retained bindings cannot authorize widening back to removed scopes.
- Session usability checks the selected grant's effective provider-scope union with the same ceiling
  as exchange, not merely configured required scopes (research R4).
- Already-issued access tokens are not rewritten. They expire normally.
