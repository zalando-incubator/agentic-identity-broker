# Design: Agents Acting On Their Own Behalf

**Status**: Draft, design discussion
**Date**: 2026-10-23
**Related**: [issue 222](https://github.com/zalando-incubator/agentic-identity-broker/issues/222),
[claim variants for an agent subject](037-oauth2-user-impersonation/research-222-agent-subject-claims.md),
[permission sets and tool authorization](design-permission-sets-tool-authorization.md),
[ADR 032](../adrs/032-impersonation-requires-user-delegation.md),
[ADR 038](../adrs/038-proactive-token-refresh.md), [spec 018](018-oauth2-provider-flavors/spec.md)

## 1. Problem

Every path that turns a broker token into a third-party token assumes a human is behind it:

```
broker token (sub = user) ──▶ UserGrant(user, agent) ──▶ UserSession(user, service) ──▶ third-party token
```

An agent can already obtain a broker token for itself through `client_credentials`, and issue 222
adds a second way through impersonation with an external workload identity. Both produce a token
whose `sub` is the agent UUID. The token-exchange path then looks for a `UserGrant` and a
`UserSession` keyed by that UUID, finds neither, and returns `access_denied` with a consent URL that
no human will ever open.

Three things are missing:

1. **Who delegates to a machine.** A user consents through the consent UI. Nobody consents on behalf
   of an agent that acts as itself.
2. **Where the third-party credential comes from.** A user session holds tokens the user obtained
   interactively. A machine needs a credential the provider issued to a workload: a service-account
   key, an app private key, or an app token an administrator bootstrapped once.
3. **How different agents get different access from a shared credential.** Several agents may run
   as the same GitHub App installation or the same Google service account, yet a triage bot must not
   receive a token that can push to `main`.

Goals: give agents permissions through the same permission-set vocabulary users see, keep the
RFC 8693 exchange and the ExtProc contract unchanged for the gateway, and fail closed everywhere a
human-facing recovery path does not apply to a machine.

Non-goals: agents impersonating users without consent (ADR 032 stays), user-to-user delegation,
and replacing the OPA tool boundary at the gateway.

## 2. Current Building Blocks

| Block | Today | Reused as |
|---|---|---|
| `Agent.PermissionSets` | Declares which permission sets an agent may ask for | Unchanged: the ceiling of what any grant to the agent may contain |
| `PermissionSet.ServiceScopes` | Admin-defined bundle of scopes per service | Unchanged: the unit of delegation for users and machines alike |
| `UserGrant(principal, agent)` | User's delegation of permission sets to an agent | Same shape with `principal = agent UUID`: the **agent grant** |
| `UserSession(principal, service)` | Encrypted provider tokens obtained by the user | Same shape with `principal = agent UUID`: the **machine session**, a short-lived cache of minted tokens |
| `ThirdpartyOAuth2ProviderEntity` with `Flavor` | Provider config, `google` stores a service-account key, `github` fixes scope separators | Gains **service identities** (§3.2) |
| Token exchange `resolveEffectiveScopes` | Union of granted permission-set scopes per service | Unchanged |
| ExtProc `granted_permission_sets`, `principal`, `agent_id` | OPA inputs | Unchanged, plus `subject_kind` |
| `client_credentials` grant, issue 222 impersonation | Mint `sub = agent UUID` | The only two ways to obtain a self token |

## 3. Design

### 3.1 Self Principal

An agent acting as itself is identified by the principal equal to its own UUID. This is already
what `client_credentials` mints as `sub`, and the issue 222 research recommends the same for the
impersonation path. The broker adds one invariant so the convention cannot be spoofed: a token
whose `sub` is a registered agent UUID is a machine token **only if** `sub == agent_id`; any other
combination is rejected at exchange, and the authorization-code path refuses to mint a user token
whose `X-Remote-User` principal parses as a registered agent UUID.

Token exchange derives `subject_kind` from this check (`user` or `agent`) and branches once, right
after agent resolution. Everything upstream of that branch (client assertion, subject-token
validation, resource routing) and downstream of it (response shape, caching in ExtProc) stays as it
is.

### 3.2 Service Identity

A **service identity** is a provider-issued machine credential attached to a third-party service,
managed by administrators, encrypted at rest under the service's `service_id` encryption context
(ADR 008), and never returned by the API.

```
ServiceIdentity
  id, service_id, name, canonical_id
  kind:        jwt_bearer | github_app | client_credentials | bootstrapped_oauth
  credential:  Secret                  (kind-specific material, encrypted)
  ceiling:     []string                (maximum scopes this identity can ever mint)
  narrowing:   provider | none         (whether the provider accepts a smaller scope set at mint time)
  status:      ready | needs_bootstrap | failed
```

| Kind | Credential | How a token is minted | Narrowing |
|---|---|---|---|
| `jwt_bearer` | RSA/EC private key, issuer identity, token endpoint | RFC 7523 §2.1 assertion grant with `scope` set to the effective scopes | `provider` |
| `github_app` | App ID, private key, installation ID | RS256 app JWT, then installation access token with `permissions` and optional `repositories` | `provider` |
| `client_credentials` | client ID and secret, or private key for `private_key_jwt` | RFC 6749 §4.4 with `scope` | `provider` if the AS honours `scope`, else `none` |
| `bootstrapped_oauth` | Access and refresh token obtained once by an administrator through the authorization-code flow | Stored token, refreshed with the refresh token when present | `none` |

The existing `google` flavor credential (spec 018) becomes the first `jwt_bearer` identity: the
service-account JSON already carries the key, the issuer e-mail and the token URI. The `github`
flavor keeps its scope grammar for user sessions and gains `github_app` identities for machines.

A service may hold several identities. This is how a provider without narrowing still serves
different access levels: one identity per level, each bootstrapped with exactly its ceiling.

### 3.3 Agent Grant

An **agent grant** is an administrator's delegation of permission sets to an agent for its own
identity. It is stored as a `UserGrant` whose principal is the agent UUID, so `VerifyAgentAccess`,
`resolveEffectiveScopes`, and the ExtProc `granted_permission_sets` input work unchanged. It differs
from a user grant in who may create it and in one extra field per entry:

```
UserGrant
  principal = <agent UUID>
  agent_id  = <agent UUID>
  granted_permission_sets[]:
    permission_set_id
    included_service_ids
    identity_bindings: { service_id: service_identity_id }   (new, optional)
  valid_until
```

Rules:

- Created, replaced, and deleted only through the admin API. The consent UI never lists or edits
  grants whose principal is an agent UUID.
- Every permission set must be declared on the agent (same rule as user grants).
- For every included service, the effective scopes must be a subset of the bound identity's
  ceiling. Validated at write time and again at exchange, because ceilings and permission sets can
  change later.
- A service with exactly one identity needs no binding. A service with several requires one, so an
  administrator chooses the blast radius explicitly instead of the broker picking.

### 3.4 Machine Token Source

A new domain service in `internal/domain/oauth2session`, behind a `ports.MachineTokenSource` port
per identity kind, turns an identity plus effective scopes into a provider token. The token is
cached as a machine session keyed by `(agent UUID, service)`; the `Scope` column holds the scopes it
was minted with. Reuse is allowed only when the cached scopes equal the effective scopes, so a token
narrowed for one agent is never served to another. Expiry handling reuses ADR 038: the proactive
refresh worker calls the token source instead of a refresh endpoint for kinds that re-mint, and the
refresh token for `bootstrapped_oauth`.

### 3.5 Exchange Flow for a Machine Subject

```
ExtProc ──RFC 8693──▶ /oauth2/token
  subject_token: broker JWT { sub = A, agent_id = A, act? }
  resource:      https://api.github.com
  client_assertion: ExtProc identity
    │
    ├─ validate client assertion, subject token, resolve agent A, resolve service S by resource
    ├─ subject_kind = agent            (sub == agent_id, A registered)
    ├─ grant = UserGrant(A, A)         missing → access_denied, error_uri = admin agent page, not consent
    ├─ effective scopes for S          (unchanged resolver)
    ├─ identity = binding(grant, S) or the single identity of S
    │     effective ⊄ ceiling → access_denied (grant exceeds identity ceiling)
    │     status needs_bootstrap → invalid_grant, error_uri = admin identity page
    ├─ token = machine session (A, S) if cached scopes == effective, else MachineTokenSource.Mint(identity, effective)
    │     narrowing = none → token carries the ceiling; broker records the excess in the audit event
    └─ response: access_token, scope = token scopes, granted_permission_sets, principal = A, agent_id = A
```

ExtProc passes `subject_kind` into the OPA input. The existing Tier 1 boundary (tool allowed if any
granted permission set lists it) applies to machine subjects exactly as to users. Tier 2 runtime
approval has no human to ask; the policy must either auto-approve or deny for `subject_kind ==
"agent"`, and the broker rejects approval creation for a machine subject so a misconfigured policy
fails closed instead of queueing an approval nobody will answer.

### 3.6 Obtaining the Self Token

Two issuance paths, both minting the claim shape V1 from the issue 222 research:

- **`client_credentials`** with a broker-managed secret. Available today for locally registered
  agents.
- **Impersonation with an agent subject** (issue 222). A privileged client presents an external
  workload identity (SPIFFE, Kubernetes service account, cloud IdP) as actor token; a rule with
  `subject.kind: agent` mints `sub = agent_id` with `act` naming the workload, skips the user
  delegation check, and still runs the rule's CEL predicate. This requires an ADR superseding ADR 032
  for the agent-subject case only.

Neither path changes with this design. The design only gives the resulting token somewhere to go.

### 3.7 Administration

OpenAPI first (constitution Principle IV). New admin resources:

| Resource | Purpose |
|---|---|
| `POST/GET /api/services/{id}/identities` | Create and list identities. Credential material write-only. |
| `PUT/DELETE /api/services/{id}/identities/{iid}` | Rotate credential, change ceiling, remove. Removal fails while any agent grant binds it. |
| `POST /api/services/{id}/identities/{iid}/bootstrap` | For `bootstrapped_oauth`: returns the provider authorization URL; the callback stores the tokens on the identity, reusing the sealed-state callback machinery. |
| `GET /api/services/{id}/identities/{iid}/status` | `ready`, `needs_bootstrap`, `failed`, last mint error class, ceiling as last observed from the provider where discoverable (GitHub installation permissions). |
| `PUT/GET/DELETE /api/agents/{id}/self-grant` | The agent grant. One per agent. |

Audit events gain `subject_kind` and `service_identity_id`. Admin UI work mirrors the API; the
consent UI only needs the filter in §3.3.

## 4. Validation Against Three Providers

### 4.1 Google

Google issues machine tokens to service accounts through the RFC 7523 JWT bearer grant. The
assertion's `scope` claim selects the scopes, so narrowing is native.

- Identity: kind `jwt_bearer`, credential = the service-account JSON the `google` flavor already
  validates and stores. Ceiling = the scopes the administrator enables, for example
  `https://www.googleapis.com/auth/drive` and `.../spreadsheets.readonly`.
- Permission sets: `google-drive-readonly` → `drive.readonly`; `google-drive-editor` → `drive`.
- Two agents, same identity, different tokens:

```
POST https://oauth2.googleapis.com/token
grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer
assertion=<JWT iss=sa@project.iam.gserviceaccount.com, aud=https://oauth2.googleapis.com/token,
            scope="https://www.googleapis.com/auth/drive.readonly", exp=now+1h>
```

for `report-agent`, and `scope="https://www.googleapis.com/auth/drive"` for `filing-agent`. Each
is cached under its own `(agent, service)` machine session.

- Not used here: the assertion's `sub` claim (domain-wide delegation), which would make the token act
  as a user. That is impersonation of a human and stays under ADR 032.
- Later, keyless: Google Workload Identity Federation accepts an external OIDC token at its STS
  (RFC 8693) and lets it impersonate a service account. The broker already publishes a JWKS and mints
  `sub = agent UUID`, so a `google_workload_identity` kind could drop the stored private key. The
  identity and grant model are unchanged; only the token source differs.

Verdict: fits with no special case. The only new code is the `jwt_bearer` token source, which spec
018 explicitly deferred.

### 4.2 GitHub

GitHub has no `client_credentials`. Machine access goes through a GitHub App: a short app JWT
signed with the app's private key, then an installation access token. The installation token
request accepts `permissions` and `repositories`, so narrowing is native and finer than scopes.

- Identity: kind `github_app`, credential = app ID, private key PEM, installation ID. Ceiling = the
  permissions the installation was granted, which the broker reads from
  `GET /app/installations/{id}` and refreshes on a bounded interval so a tightened installation
  tightens the ceiling.
- Scope grammar for this kind: `permission:level`, for example `contents:read`, `issues:write`,
  `pull_requests:write`. Permission sets reuse the existing `scopes` field with that grammar; the
  `github_app` token source translates it into the `permissions` object.
- Example: `triage-bot` is granted `github-issues-rw` (`issues:write`, `metadata:read`);
  `release-bot` is granted `github-contents-rw` plus `github-issues-ro`. Both bind to the same
  installation identity.

```
POST https://api.github.com/app/installations/12345/access_tokens
Authorization: Bearer <app JWT, RS256, iss=<app id>, exp=now+10m>
{ "permissions": { "issues": "write", "metadata": "read" } }
```

for `triage-bot`, and `{ "contents": "write", "issues": "read", "metadata": "read" }` for
`release-bot`. Installation tokens live one hour and cannot be refreshed, so expiry means re-mint.

- Repository restriction: optional `repositories` list on the identity ceiling. A finer per-agent
  split needs a second identity (a second installation, or the same installation with a different
  repository list), bound per agent.
- The existing `github` flavor describes the OAuth App used for user sessions. Both can coexist on
  one service: users bring user-to-server tokens, agents bring installation tokens, and the
  permission set vocabulary differs between them. An administrator who wants one vocabulary registers
  two services.

Verdict: fits, and shows why the ceiling must be observed from the provider rather than typed in.

### 4.3 Linear

Linear has neither `client_credentials` nor a token endpoint that narrows scopes. An application
acts as itself by completing the authorization-code flow once with `actor=app`; the resulting token
represents the application, carries the scopes chosen at authorization time, and is long-lived.
Personal API keys exist but belong to a person and are out of scope.

- Identity: kind `bootstrapped_oauth`, narrowing `none`. An administrator triggers the bootstrap
  endpoint; the broker runs the ordinary authorization-code flow with `actor=app` in
  `authorization_params` (a field the service model already has) and stores the returned token on
  the identity instead of in a user session.
- Different access levels need different identities, because the token cannot be narrowed:
  `linear-read` bootstrapped with `read`, and `linear-write` bootstrapped with `read write
  issues:create comments:create`. `summary-agent` binds to `linear-read`; `planner-agent` binds to
  `linear-write`.
- An agent granted a permission set whose scopes exceed the bound identity's ceiling is rejected at
  grant time. An agent granted less than the ceiling receives the full ceiling token; the audit event
  records the excess and the OPA tool boundary enforces the difference. This is the same posture the
  broker already takes for user sessions that were consented more broadly than the grant.
- Rotation: a Linear app can hold several valid tokens, so re-running the bootstrap replaces the
  stored token without a gap; the old one is revoked explicitly.

Verdict: fits only because identities are first-class and bindable per agent. A design with a single
credential per service, or with narrowing assumed, would not work for Linear.

### 4.4 Summary

| Requirement | Google | GitHub | Linear |
|---|---|---|---|
| Machine credential kind | `jwt_bearer` (existing flavor credential) | `github_app` | `bootstrapped_oauth` |
| Provider narrows at mint | Yes, `scope` | Yes, `permissions` and `repositories` | No |
| Shared identity, different agent access | One identity, per-agent scopes | One installation, per-agent permissions | One identity per level, per-agent binding |
| Ceiling source | Admin-declared | Observed from installation | Fixed at bootstrap |
| Token lifetime, renewal | 1 h, re-mint | 1 h, re-mint | Long-lived, refresh if provided |
| Human needed | Never | Never | Once, at bootstrap |

## 5. Trade-offs

- **Reusing `user_grants` and `user_sessions` for agents versus new tables.** Reuse keeps token
  exchange, ExtProc, proactive refresh, and the session sweep untouched, and the self-principal
  convention makes the rows self-describing. The cost is that every list or UI that enumerates grants
  or sessions must filter by principal kind, and the table names become misleading. A view or a
  `principal_kind` column added by migration keeps queries honest without a second code path.
- **Where narrowing is enforced.** Narrowing at the provider gives least privilege in the token
  itself. Where the provider cannot narrow, the broker can either refuse (strict) or hand out the
  ceiling and rely on the gateway boundary (current posture for users). The design picks the second
  with an explicit audit trail, because strict mode would make Linear unusable with any shared
  identity. A per-identity `require_exact_fit` flag can give strict mode to administrators who want
  it.
- **Observed versus declared ceilings.** GitHub lets the broker read the real ceiling; Google and
  Linear do not expose one before minting. Declared ceilings can drift from reality in both
  directions. Minting with scopes the provider refuses surfaces as a mint failure; minting with a
  declared ceiling wider than the real one is harmless. Drift the other way (real wider than
  declared) is invisible and acceptable, since the broker never asks for more than declared.
- **Keys at rest versus federation.** `jwt_bearer` and `github_app` store long-lived private keys in
  the broker. They are encrypted under the existing envelope, but they remain the highest-value
  secrets the broker holds. Federation (Google Workload Identity, and any provider that accepts the
  broker's own JWT) removes them. It is a later phase because it needs provider-side trust
  configuration that administrators do not control uniformly.
- **Who delegates to a machine.** Administrators, through the admin API, with the same permission-set
  vocabulary as users. This keeps one consent model but moves the decision from the person who owns
  the data to the person who runs the platform. Audit events and `valid_until` are the compensating
  controls; an approval workflow for agent grants is out of scope.
- **Tier 2 approvals for machines.** Rejecting approval creation for machine subjects is the safe
  default. The alternative, routing approvals to an operator queue, is a product decision that can be
  added later without changing this design.
- **Permission-set vocabulary per credential kind.** GitHub App permissions and GitHub OAuth scopes
  are different languages. Allowing both on one service keeps the service list short but makes
  permission sets kind-aware. Requiring separate services is simpler to reason about and is the
  recommended convention, with validation that a permission set's scopes match the grammar of every
  identity it can be bound to.
- **Cache granularity.** Keying machine sessions by `(agent, service)` with a scope equality check
  is simple and never leaks a wider token to a narrower agent. It mints once per agent even when ten
  agents share identical effective scopes. Keying by `(identity, scope set)` would share tokens
  across agents and save mints, at the cost of blurring which agent a token belongs to in audit and
  revocation.

## 6. Phasing

1. **Self principal and machine exchange branch.** The `sub == agent_id` invariant, the user-token
   guard, `subject_kind` in exchange and ExtProc, agent grants through the admin API, and the machine
   session cache. Deliverable without any provider work: a `bootstrapped_oauth` identity validates
   the whole path against Linear.
2. **Provider token sources.** `jwt_bearer` on top of the stored Google credential, then
   `github_app` with observed ceilings.
3. **Issue 222.** Agent-subject impersonation with the superseding ADR, so workloads with an
   external identity obtain the self token without a broker-managed secret.
4. **Federation.** Keyless identities where providers accept the broker's own JWT.

Each phase needs its own spec with example journeys, OpenAPI changes before implementation, both
storage adapters, and E2E coverage of a complete agent journey from self token to third-party call.
