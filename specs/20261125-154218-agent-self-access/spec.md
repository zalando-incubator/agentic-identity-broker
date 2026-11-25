# Feature Specification: Agents Acting on Their Own Behalf

**Feature Branch**: `speckit/issue-478`

**Created**: 2026-11-25

**Status**: Draft

**Input**: User description:

> Agents acting on their own behalf (issue #478, sub-issues #471–#477): service accounts and their
> kinds, agent grants with explicit service-account bindings, the machine branch in token exchange,
> and provider kinds through to keyless accounts — one spec for the whole design, phased per #478.

Issue #478 is the design of record. Its decisions are settled and are not reopened here. This
specification covers all seven delivery steps in one document and keeps them separable: every user
story, scenario, journey and requirement names the step it belongs to.

## Delivery Map

The feature is delivered in seven steps over three phases. A step is shippable when the steps it
depends on are merged. Step 3 is the first that lets an agent reach a company-built MCP.

| Step | Issue | Phase | Scope | Depends on | User story | Journey | Requirements |
|---|---|---|---|---|---|---|---|
| 1 | #471 | 1 | Service accounts, first kind `jwt_bearer` with the stored `signed_jwt` format | none | US1 | 1 | FR-001–FR-016 |
| 2 | #472 | 1 | Agent grants with explicit service-account bindings | Step 1 | US2 | 1, 2 | FR-020–FR-030 |
| 3 | #473 | 1 | Machine branch in token exchange, stored-JWT source, machine sessions, `subject_kind` at the gateway | Steps 1, 2 | US3, US4 | 1, 2 | FR-040–FR-062 |
| 4 | #474 | 2 | `bootstrapped_oauth` kind: one-time administrator sign-in | Step 3 | US5 | 3 | FR-070–FR-080 |
| 5 | #475 | 2 | `jwt_bearer` signing formats and the `client_credentials` kind | Step 3 | US6 | 4 | FR-090–FR-098 |
| 6 | #476 | 2 | `github_app` kind with observed ceilings | Step 3 | US7 | 5 | FR-100–FR-108 |
| 7 | #477 | 3 | Keyless service accounts through provider federation | Step 5 | US8 | 6 | FR-110–FR-115 |

Steps 4, 5 and 6 are independent of each other. Requirement numbers leave gaps between steps so a
step can gain a requirement without renumbering the others. Security, API, database and
configuration requirements name the step in which each first applies.

```mermaid
flowchart LR
    S1["Step 1 #471 service accounts"] --> S2["Step 2 #472 agent grants"]
    S1 --> S3["Step 3 #473 machine exchange"]
    S2 --> S3
    S3 --> S4["Step 4 #474 one-time sign-in"]
    S3 --> S5["Step 5 #475 signed JWT and client credentials"]
    S3 --> S6["Step 6 #476 GitHub App"]
    S5 --> S7["Step 7 #477 keyless"]
```

## User Scenarios & Testing *(mandatory)*

Scenario identifiers (`US<story>.<n>`) are stable references for 1:1 end-to-end coverage. Each
scenario maps to exactly one test. Actors:

- **Administrator**: a person who manages the broker through the admin API.
- **Agent**: registered software that holds a self token, a broker token in which the agent is its
  own subject.
- **Gateway**: the trusted component that exchanges a broker token for a service token on each
  call and applies tool-level authorization.
- **MCP author**: a developer of a company-built MCP server that receives the exchanged token.
- **User**: a person who consents to an agent today. Nothing changes for users.

### User Story 1 - Register a machine credential on a service (Priority: P1, Step 1)

An administrator attaches a service account to an existing third-party service. The first kind
holds a JWT that company infrastructure has already signed for one agent. The administrator pastes
it once, declares the ceiling (the most the account may ever obtain) and afterwards sees only what
is needed to recognise the credential: issuer, subject, key identifier, expiry and a fingerprint.

**Why this priority**: every later step attaches to a service account. Without it there is no
machine credential to bind or hand out.

**Independent Test**: create, read, list, rotate and delete a service account through the admin
API and confirm that the stored credential is never returned and that status follows the token's
stated expiry. Delivers a managed, encrypted store for machine credentials before any agent can
use them.

**Acceptance Scenarios**:

1. **US1.1** **Given** a registered service, **When** an administrator creates a `jwt_bearer`
   service account with a `signed_jwt` credential and a non-empty ceiling, **Then** the account is
   created with status `ready` and narrowing `none`, and the response shows a credential summary
   (issuer, subject, key identifier, expiry, fingerprint) and never the JWT.
2. **US1.2** **Given** an existing service account, **When** an administrator reads it or lists
   the service's accounts, **Then** both show the same representation with a version tag, and
   neither contains credential material.
3. **US1.3** **Given** a create request whose JWT is not a compact signed JWT, uses `alg: none`,
   exceeds 16 KiB, or states an expiry in the past, **When** it is submitted, **Then** it is
   refused with a validation error that names the failed check and nothing is stored.
4. **US1.4** **Given** a create request with an empty ceiling, an unknown kind, a `use` other than
   `access_token`, or a `canonical_id` already used within the same service, **When** it is
   submitted, **Then** it is refused and nothing is stored.
5. **US1.5** **Given** a `ready` account, **When** an administrator replaces its credential,
   **Then** the account keeps its identity, the summary shows the new fingerprint and expiry, the
   version tag changes, and a rotation audit record is written.
6. **US1.6** **Given** an account whose stored JWT states an expiry, **When** that time has
   passed, **Then** status reports `failed` with error class `credential_expired` until the
   credential is replaced, after which it reports `ready`.
7. **US1.7** **Given** an update that carries a version tag which is no longer current, **When**
   it is submitted, **Then** it is refused as a precondition failure; the same update without a
   version tag is applied.
8. **US1.8** **Given** a service that still has a service account, **When** an administrator
   deletes the service, **Then** the deletion is refused; after the account is deleted, the
   service can be deleted.
9. **US1.9** **Given** the same `canonical_id` on two different services, **When** both accounts
   are created, **Then** both succeed, and renaming one leaves every reference to it intact.

---

### User Story 2 - Delegate permission sets to an agent for its own use (Priority: P1, Step 2)

An administrator writes one agent grant per agent: the permission sets the agent may use for
itself and, for every service the agent should reach, the service account it uses there. The
binding is always explicit. The grant is refused when it asks for more than a bound account's
ceiling.

**Why this priority**: the agent grant is the machine counterpart of user consent. Without it the
exchange has nothing to authorize against and no account to mint from.

**Independent Test**: write, read and delete an agent grant through the admin API and confirm the
validation rules and that users never see it. Delivers auditable, administrator-owned delegation
before any token is issued.

**Acceptance Scenarios**:

1. **US2.1** **Given** an agent that declares a permission set and a `ready` account on the
   covered service, **When** an administrator writes the agent grant with that permission set and
   a binding to that account, **Then** the grant is stored with the agent as its principal and can
   be read back unchanged.
2. **US2.2** **Given** a service with exactly one service account, **When** an administrator
   writes an agent grant that covers the service and names no binding for it, **Then** the grant
   is stored, no account is chosen by default, and the agent can obtain no token for that service.
3. **US2.3** **Given** a binding for a service that none of the granted permission sets covers,
   **When** the grant is written, **Then** it is refused.
4. **US2.4** **Given** a binding that names an account of a different service or an unknown
   account, **When** the grant is written, **Then** it is refused.
5. **US2.5** **Given** granted permission sets whose combined access at a bound service exceeds
   the bound account's ceiling, **When** the grant is written, **Then** it is refused and the
   error names the service and the access beyond the ceiling.
6. **US2.6** **Given** a permission set the agent does not declare, **When** the grant lists it,
   **Then** the grant is refused.
7. **US2.7** **Given** an existing agent grant, **When** an administrator writes it again,
   **Then** it replaces the earlier grant as a whole: there is still exactly one grant for the
   agent.
8. **US2.8** **Given** an agent grant, **When** a user opens their consent overview or the
   agent's consent page, or lists their grants, **Then** the agent grant is never shown, including
   when a user's identifier text equals the agent's identifier.
9. **US2.9** **Given** an account that an agent grant binds, **When** an administrator deletes the
   account, **Then** the deletion is refused and names the binding grant; after the binding is
   removed, the deletion succeeds.
10. **US2.10** **Given** an agent grant, **When** an administrator deletes it, **Then** the agent
    obtains no further tokens and its cached machine sessions are no longer served.

---

### User Story 3 - An agent reaches a company-built MCP as itself (Priority: P1, Step 3)

An agent with a self token calls an internal MCP through the gateway. The gateway exchanges the
self token as it does for users. The broker recognises the agent as its own subject, finds the
agent grant and the bound account, and returns the token that the company's own issuer signed for
that agent. The MCP validates it as it validates every other token from that issuer.

**Why this priority**: this is the first end-to-end value: an unattended agent reaches a service
with a credential issued for it, under administrator delegation, without a fabricated human.

**Independent Test**: obtain a self token both ways, exchange it for a resource of the bound
service and compare the returned token with the stored one. Delivers the complete phase 1 journey
with no outbound call and no signing key in the broker.

**Acceptance Scenarios**:

1. **US3.1** **Given** an agent with a grant bound to a `ready` stored-JWT account, **When** the
   gateway exchanges a self token the agent obtained with its own broker secret, **Then** the
   response carries the account's stored JWT unchanged, a lifetime equal to the time left until
   the JWT's stated expiry, and the granted permission sets.
2. **US3.2** **Given** the same setup, **When** the self token was obtained through the
   impersonation flow by a workload an administrator's rule allows to act as the agent, **Then**
   the exchange returns the same stored JWT.
3. **US3.3** **Given** an agent with no agent grant, **When** its self token is exchanged,
   **Then** the exchange is denied with `access_denied` and no consent link.
4. **US3.4** **Given** a grant that covers the service and a service that has an account, but no
   binding for that service, **When** the self token is exchanged, **Then** the exchange is denied
   with `access_denied` and no consent link, and no account is used.
5. **US3.5** **Given** a grant whose combined access now exceeds the bound account's ceiling,
   **When** the self token is exchanged, **Then** the exchange is denied without a token, and the
   denial tells administrators that the service account's ceiling is the cause.
6. **US3.6** **Given** two agents bound to different accounts on one service, **When** each
   exchanges its self token, **Then** each receives the stored JWT of its own account and never
   the other's.
7. **US3.7** **Given** an agent that has already exchanged once, **When** an administrator rotates
   the bound account's credential and the agent exchanges again, **Then** the response carries the
   new JWT.
8. **US3.8** **Given** a bound account whose stored JWT has expired, **When** the self token is
   exchanged, **Then** the exchange is denied, the expired JWT is not returned, and the denial
   tells administrators that the service account has failed.
9. **US3.9** **Given** an agent whose grant is rebound from one account to another, **When** it
   exchanges again, **Then** it receives the token of the newly bound account.
10. **US3.10** **Given** an agent granted less than the stored JWT's ceiling, **When** it
    exchanges, **Then** it receives the whole stored JWT and the audit record lists the access
    beyond what was granted.
11. **US3.11** **Given** an agent grant with an end date in the past, **When** the self token is
    exchanged, **Then** the exchange is denied with `access_denied` and no consent link.
12. **US3.12** **Given** a successful machine exchange, **When** the gateway evaluates its policy
    for the call, **Then** the policy input says the subject is an agent, names the agent as the
    principal and lists the granted permission sets.

---

### User Story 4 - Humans are untouched and the machine path fails closed (Priority: P1, Step 3)

A user who consented to an agent keeps receiving their own service token. A service account is
never used for a human, an agent token never borrows a human's session, and an agent token still
cannot ask a human for approval.

**Why this priority**: opening a path that was closed on purpose is only acceptable if the
existing safety properties hold. This story is the regression and isolation guarantee for Step 3.

**Independent Test**: run the existing user exchange next to a machine exchange for the same agent
and service and confirm that each receives only its own token, and that detection failures and
approval requests are refused.

**Acceptance Scenarios**:

1. **US4.1** **Given** a user with consent and a session for an agent that also holds an agent
   grant, **When** the gateway exchanges the user's token, **Then** the response carries the
   user's own service token, the policy input says the subject is a human, and no service account
   is used.
2. **US4.2** **Given** a human principal whose identifier text equals an agent's UUID and who
   holds a grant and a session, **When** that agent's self token is exchanged without an agent
   grant, **Then** the exchange is denied and the human's grant and session are not used.
3. **US4.3** **Given** agent-subject detection cannot be evaluated, **When** any token is
   exchanged, **Then** the exchange fails with `server_error` and never continues as a human or
   as an agent.
4. **US4.4** **Given** an agent's self token, **When** it is presented to create or consume a
   tool approval, **Then** the request is refused as it is today.
5. **US4.5** **Given** a tool that the gateway's policy would send to a human for approval,
   **When** an agent calls it with a machine token, **Then** the call is denied and no approval
   is created.
6. **US4.6** **Given** machine sessions exist, **When** a user lists their sessions, **Then** no
   machine session is shown.
7. **US4.7** **Given** a machine session near expiry whose grant or binding has been removed,
   **When** background refresh runs, **Then** the machine session is dropped and nothing is
   minted.

---

### User Story 5 - One-time sign-in for a provider without machine credentials (Priority: P2, Step 4)

For a provider such as Linear, an administrator creates a service account that holds no secret
and completes it once in the browser by approving at the provider. The broker stores the resulting
tokens on the service account, not on a user session. Each access level is its own account,
because the provider cannot narrow.

**Why this priority**: it extends the machine path to providers that offer nothing else. It needs
a person once and builds on Step 3.

**Independent Test**: create an account in the waiting state, complete the sign-in against a
Linear-shaped test provider, and exchange an agent's self token for the stored provider token.

**Acceptance Scenarios**:

1. **US5.1** **Given** a service with its own client registration, **When** an administrator
   creates a `bootstrapped_oauth` account with a ceiling and no credential, **Then** it is created
   in status `needs_bootstrap` and shows the sign-in link; a create request that carries a
   credential is refused.
2. **US5.2** **Given** an account in `needs_bootstrap`, **When** an administrator opens the
   sign-in link and approves at the provider, and the provider grants exactly the declared
   ceiling, **Then** the account becomes `ready`, records who completed it and when, a
   `service_account.bootstrapped` audit record is written, the browser returns to the admin page,
   and no user session exists for the administrator.
3. **US5.3** **Given** a provider that grants a scope set different from the declared ceiling,
   **When** the sign-in returns, **Then** nothing is stored and the account is `failed` with
   reason `scope_mismatch`.
4. **US5.4** **Given** a provider that returns an error or a denied approval, **When** the
   sign-in returns, **Then** nothing is stored and the account is `failed` with the provider's
   error code.
5. **US5.5** **Given** a sign-in that was started before the account was changed, or that already
   completed once, **When** its return is replayed, **Then** it is refused and the account is
   unchanged.
6. **US5.6** **Given** a sign-in started by one administrator, **When** the return arrives in a
   browser recognised as a different person, **Then** it is refused and nothing is stored.
7. **US5.7** **Given** an agent bound to an account in `needs_bootstrap`, **When** its self token
   is exchanged, **Then** the exchange is denied and the denial tells administrators that the
   account still needs its sign-in.
8. **US5.8** **Given** a `ready` account and a bound agent, **When** the self token is exchanged,
   **Then** the agent receives the account's stored provider token; when the agent is granted
   less than the ceiling, the audit record lists the excess.
9. **US5.9** **Given** a `ready` account with bound agents, **When** an administrator repeats the
   sign-in, **Then** agents keep receiving the earlier token until the new one is stored and
   receive the new one afterwards, with no failed exchange in between.
10. **US5.10** **Given** a stored provider token near expiry and a stored refresh token, **When**
    background refresh runs, **Then** the account's token is refreshed and bound agents receive
    the refreshed token.

---

### User Story 6 - Narrowed tokens from a key the broker holds (Priority: P2, Step 5)

An administrator registers a Google service-account key, a generic private key, or a client
identifier and secret. Several agents share the account, and each receives a token that carries
exactly its granted access, minted per agent and re-minted before it expires.

**Why this priority**: it delivers least privilege at the token for providers that can narrow, and
the first Google use case. It builds on Step 3.

**Independent Test**: bind two agents with different permission sets to one account, exchange
both self tokens against a test token endpoint, and compare the scope each request asked for.

**Acceptance Scenarios**:

1. **US6.1** **Given** one Google-key account and two agents granted different permission sets
   bound to it, **When** each exchanges its self token, **Then** each receives a different
   provider token, and the provider was asked for exactly that agent's granted access.
2. **US6.2** **Given** a `private_key` account with issuer, subject, audience, token endpoint,
   algorithm and key identifier, **When** a bound agent exchanges, **Then** the provider receives
   a signed assertion with those values, a lifetime of at most one hour, and the agent's granted
   access as the requested scope.
3. **US6.3** **Given** a `client_credentials` account, **When** a bound agent exchanges, **Then**
   the service's token endpoint receives a client-credentials request with the agent's granted
   access as the requested scope, and the agent receives the returned token.
4. **US6.4** **Given** a create request with a key that is not RSA of at least 2048 bits or EC
   P-256, an unsupported algorithm, or a Google key file that fails the existing Google key
   checks, **When** it is submitted, **Then** it is refused and nothing is stored.
5. **US6.5** **Given** a cached machine session, **When** the same agent exchanges again with
   unchanged access, **Then** the cached token is returned and the provider is not called again.
6. **US6.6** **Given** an agent's granted access changes within the ceiling, **When** it
   exchanges again, **Then** a new token is minted for the new access and the earlier token is not
   served.
7. **US6.7** **Given** a machine session near expiry with an intact grant and binding, **When**
   background refresh runs, **Then** a new token is minted before the old one expires.
8. **US6.8** **Given** a provider that refuses or cannot be reached at mint, **When** an agent
   exchanges, **Then** the exchange fails as a dependency error without a token, the account's
   status shows the last mint error class, and provider error text is not returned.
9. **US6.9** **Given** a rotated key on the account, **When** a bound agent exchanges again,
   **Then** the token is minted with the new key and tokens minted with the old key are not
   served.

---

### User Story 7 - Per-agent permissions from one GitHub App installation (Priority: P2, Step 6)

An administrator registers a GitHub App installation as a service account. The broker reads the
installation's real permissions and treats them as the ceiling, so tightening the installation at
GitHub tightens every agent. Each agent receives an installation token that carries only its
granted permissions.

**Why this priority**: GitHub has no client-credentials grant, so this is the only machine path
for it. It builds on Step 3.

**Independent Test**: bind two agents with different permission sets to one GitHub App account,
exchange both against a test GitHub API, and compare the permissions each request asked for.

**Acceptance Scenarios**:

1. **US7.1** **Given** a `github_app` account with app identifier, installation identifier and
   key, **When** it is created, **Then** its summary shows the app and installation identifiers
   and a key fingerprint, and its status shows the ceiling observed at the installation.
2. **US7.2** **Given** two agents bound to one GitHub App account with different permission sets,
   **When** each exchanges, **Then** each receives an installation token requested with only its
   granted permissions.
3. **US7.3** **Given** an account with a repository restriction, **When** a bound agent
   exchanges, **Then** the installation token is requested for those repositories only.
4. **US7.4** **Given** a ceiling or permission set that uses a scope not in `permission:level`
   form, an unknown permission, or a level other than `read`, `write` or `admin`, **When** it is
   written for a GitHub App account, **Then** it is refused.
5. **US7.5** **Given** an installation whose permissions were reduced at GitHub, **When** the
   broker next observes it, **Then** status shows the reduced observed ceiling, and an agent whose
   granted access exceeds it is denied at exchange with a denial that points administrators to the
   account.
6. **US7.6** **Given** a grant asking for `issues:read` and a ceiling of `issues:write`, **When**
   the grant is written, **Then** it is accepted, because a higher level contains the lower ones.
7. **US7.7** **Given** a service that points at a GitHub Enterprise Server address, **When** a
   bound agent exchanges, **Then** the installation is observed and the token is minted at that
   address.

---

### User Story 8 - Keyless accounts through provider federation (Priority: P3, Step 7)

An administrator registers a service account that holds no secret at all. The provider is
configured to trust tokens the broker signs, and the broker obtains a narrowed provider token for
each agent by presenting such a token. Google Workload Identity Federation is the first provider.

**Why this priority**: it removes the most valuable long-lived secrets from the broker. It needs
configuration at the provider and builds on Step 5.

**Independent Test**: create a keyless account, exchange an agent's self token against test
federation endpoints, and confirm that no stored secret was used and the token is narrowed.

**Acceptance Scenarios**:

1. **US8.1** **Given** a `google_workload_identity` account that names the provider trust
   configuration and the Google service account to act as, **When** it is created, **Then** it is
   `ready`, holds no credential, and a create request that carries a credential is refused.
2. **US8.2** **Given** a bound agent, **When** it exchanges its self token, **Then** the broker
   presents a broker-signed token to the provider's federation endpoint, requests a provider token
   with exactly the agent's granted access, and returns it.
3. **US8.3** **Given** provider trust that allows only one agent to act as a given Google service
   account, **When** a different agent bound to the same account exchanges, **Then** the provider
   refuses, the exchange fails without a token, and the account's status shows the mint error
   class.
4. **US8.4** **Given** the provider's published trust configuration, **When** an operator follows
   the broker documentation, **Then** the provider can discover the broker's signing keys and
   validate the audience the broker uses.

---

### Edge Cases

- **Ceiling comparison is literal for flat scopes.** `drive.readonly` is not contained in a
  ceiling that lists only `drive`. Administrators list every scope a permission set may use.
  GitHub App levels are the only ordered grammar (US7.6).
- **A permission set changes after a grant was written.** The grant is not rewritten. The ceiling
  check on every exchange denies when the new combined access exceeds the bound ceiling (US3.5).
- **A ceiling is lowered below a bound grant's access.** The update is accepted. Affected agents
  are denied at exchange until the grant or the ceiling is corrected.
- **A stored JWT states no expiry.** The account stays `ready`, the exchange response omits the
  lifetime, and the token is handed out until it is replaced.
- **A stored JWT expires between two status reads.** Status is derived from the stated expiry at
  read and at exchange time, not only by a background job, so an expired JWT is never handed out.
- **An agent is deleted.** Its agent grant and machine sessions are removed with it; the bound
  accounts stay.
- **The permission set an agent grant lists is deleted or no longer declared by the agent.** The
  exchange treats the grant as not covering the service and denies without a consent link.
- **Concurrent exchanges for one agent and service with an empty cache.** At most one mint
  reaches the provider; the others receive its result.
- **Two replicas refresh the same machine session.** The same locking that protects user-session
  refresh applies; one mints, the other reuses.
- **A self token asks for a resource of a service the grant does not cover.** Denied without a
  consent link, before any account is read.
- **A user's token for an agent that has no user consent but has an agent grant.** Denied with
  the existing consent link. An agent grant never satisfies a human request.
- **The sign-in link is opened by someone who is not an administrator.** The admin side refuses
  it as it refuses every other admin request.
- **The sign-in link is opened twice in parallel.** The first completed return wins; the second
  fails the version check and changes nothing.
- **A provider returns a wider or narrower token than asked for at mint.** The broker never asks
  for more than the granted access. The machine session records the access that was asked for.
- **An override of the agent-subject detector no longer matches self tokens.** As today, such
  tokens continue through the human path and are denied there for lack of consent. They never
  reach a service account.

## Concrete Example Journeys *(mandatory)*

All journeys share one deployment. The HTTP exchanges for new admin resources show the contract
this feature proposes; they become binding when the admin API contract is updated and confirmed
(API-002, API-006). The token-exchange request and response use the existing end-user contract.
All identifiers, keys and tokens are synthetic. This feature changes no user interface: the admin
UI that will mirror these resources is a later feature, so no wireframes apply.

**Shared cast**

| Who or what | Value |
|---|---|
| Broker, user-facing | `https://broker.example.com` |
| Broker, admin | `https://broker-admin.example.com` |
| Administrator | Olivia, `olivia@example.com`, asserted as `X-Remote-User` on both sides |
| Gateway | `gateway-prod`, authenticates with a signed client assertion |
| Agent *triage-bot* | `550e8400-e29b-41d4-a716-446655440000` |
| Agent *planner-bot* | `660e8400-e29b-41d4-a716-446655440001` |
| Service *Company Platform* (`platform`) | `880e8400-e29b-41d4-a716-446655440010`, resources `https://mcp.example.com/tickets` and `https://mcp.example.com/planning` |
| Permission set *Platform reader* | `770e8400-e29b-41d4-a716-446655440001`, `read` at Company Platform |
| Permission set *Platform operator* | `770e8400-e29b-41d4-a716-446655440002`, `read write` at Company Platform |
| Company identity provider | `https://idp.example.com` |

All times are 2026-11-25 UTC.

### Journey 1 — Two agents reach the internal platform as themselves (User Stories 1–3, Scenarios US1.1, US1.2, US2.1, US3.1, US3.6, US3.12)

**Cast**: the shared cast. Both agents declare both permission sets. The company identity provider
has issued one JWT per agent, valid until 2026-12-25T00:00:00Z. No service accounts, agent grants
or machine sessions exist.

**Step 1 — Broker configuration (initial state; excerpt, existing keys only)**

```yaml
oauth2_authorization_server:
  mode: "local"                      # the broker mints self tokens
token_exchange:
  claim_extraction:
    principal_expression: subject_token.sub
    agent_id_expression: subject_token.agent_id
    # agent_subject_expression omitted: the default recognises sub == agent_id.
    # Before this feature a match was rejected; now it enters the machine path.
token_refresh:
  lookahead_duration: "5m"           # machine sessions are re-minted in this window
```

Phase 1 adds no configuration key.

**Step 2 — 09:00:00: Olivia registers triage-bot's token as a service account (US1.1)**

```http
POST /api/services/880e8400-e29b-41d4-a716-446655440010/service-accounts HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com
Content-Type: application/json

{
  "canonical_id": "platform-triage-bot",
  "kind": "jwt_bearer",
  "scopes": ["read"],
  "credential": {
    "format": "signed_jwt",
    "use": "access_token",
    "jwt": "SYNTHETIC_PLATFORM_TRIAGE_JWS"
  }
}

HTTP/1.1 201 Created
Content-Type: application/json
ETag: "sa-v1-7c9e"
Location: /api/services/880e8400-e29b-41d4-a716-446655440010/service-accounts/aa0e8400-e29b-41d4-a716-446655440100

{
  "id": "aa0e8400-e29b-41d4-a716-446655440100",
  "service_id": "880e8400-e29b-41d4-a716-446655440010",
  "canonical_id": "platform-triage-bot",
  "kind": "jwt_bearer",
  "scopes": ["read"],
  "narrowing": "none",
  "status": "ready",
  "credential_summary": {
    "format": "signed_jwt",
    "use": "access_token",
    "iss": "https://idp.example.com",
    "sub": "triage-bot",
    "kid": "2026-11",
    "exp": "2026-12-25T00:00:00Z",
    "fingerprint": "sha256:3b1f5c0e9a7d42c68e15b0a4f2d97c31"
  },
  "created_at": "2026-11-25T09:00:00Z",
  "updated_at": "2026-11-25T09:00:00Z"
}
```

Decoded `SYNTHETIC_PLATFORM_TRIAGE_JWS`, as the identity provider issued it. The broker reads
`iss`, `sub`, `kid` and `exp` for the summary and status only, and does not verify the signature:

```json
{
  "header": { "alg": "ES256", "kid": "2026-11" },
  "payload": {
    "iss": "https://idp.example.com",
    "sub": "triage-bot",
    "aud": "https://mcp.example.com",
    "scope": "read",
    "iat": 1795597200,
    "exp": 1798156800
  }
}
```

Olivia repeats the request for planner-bot with `canonical_id: platform-planner-bot`,
`scopes: ["read", "write"]` and `SYNTHETIC_PLATFORM_PLANNER_JWS` (`sub: planner-bot`,
`scope: "read write"`). The broker assigns `aa0e8400-e29b-41d4-a716-446655440101`.

**Step 3 — 09:02:00: Olivia grants triage-bot the reader set, bound to its own account (US2.1)**

```http
PUT /api/agents/550e8400-e29b-41d4-a716-446655440000/self-grant HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com
Content-Type: application/json

{
  "granted_permission_sets": {
    "770e8400-e29b-41d4-a716-446655440001": ["880e8400-e29b-41d4-a716-446655440010"]
  },
  "service_account_bindings": {
    "880e8400-e29b-41d4-a716-446655440010": "aa0e8400-e29b-41d4-a716-446655440100"
  }
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "990e8400-e29b-41d4-a716-446655440020",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "granted_permission_sets": {
    "770e8400-e29b-41d4-a716-446655440001": ["880e8400-e29b-41d4-a716-446655440010"]
  },
  "service_account_bindings": {
    "880e8400-e29b-41d4-a716-446655440010": "aa0e8400-e29b-41d4-a716-446655440100"
  },
  "created_by": "olivia@example.com",
  "created_at": "2026-11-25T09:02:00Z",
  "updated_at": "2026-11-25T09:02:00Z"
}
```

Combined access at Company Platform is `read`, within the bound ceiling `read`. Olivia writes
planner-bot's grant the same way with the operator set
(`770e8400-e29b-41d4-a716-446655440002`) bound to `aa0e8400-e29b-41d4-a716-446655440101`.

**Step 4 — 09:05:00: triage-bot obtains a self token with its own broker secret**

This step uses the existing client-credentials grant and is unchanged. Decoded
`SYNTHETIC_BROKER_TRIAGE_JWS`:

```json
{
  "header": { "alg": "ES256", "kid": "3f1c2b4a-8d7e-4f60-9a1b-2c3d4e5f6a7b" },
  "payload": {
    "iss": "https://broker.example.com",
    "sub": "550e8400-e29b-41d4-a716-446655440000",
    "agent_id": "550e8400-e29b-41d4-a716-446655440000",
    "iat": 1795597500,
    "exp": 1795601100
  }
}
```

`sub` equals `agent_id`: this is a self token. It names no audience, which is one reason the
gateway does not pass it on to the MCP.

**Step 5 — 09:05:10: the gateway exchanges the self token for the tickets MCP (US3.1)**

```http
POST /oauth2/token HTTP/1.1
Host: broker.example.com
Content-Type: application/x-www-form-urlencoded
Accept: application/json

grant_type=urn:ietf:params:oauth:grant-type:token-exchange
&resource=https%3A%2F%2Fmcp.example.com%2Ftickets
&subject_token_type=urn:ietf:params:oauth:token-type:access_token
&subject_token=SYNTHETIC_BROKER_TRIAGE_JWS
&client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer
&client_assertion=SYNTHETIC_GATEWAY_CLIENT_JWS

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: no-store
Pragma: no-cache

{
  "access_token": "SYNTHETIC_PLATFORM_TRIAGE_JWS",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "expires_in": 2559290,
  "granted_permission_sets": {
    "770e8400-e29b-41d4-a716-446655440001": ["880e8400-e29b-41d4-a716-446655440010"]
  },
  "principal": "550e8400-e29b-41d4-a716-446655440000"
}
```

Line breaks in the form body are for display only. The broker took the four steps of the exchange
decision: the subject is an agent; the agent holds a grant; the grant binds account
`aa0e8400-e29b-41d4-a716-446655440100`, which is `ready` and whose ceiling contains `read`; no
machine session existed, so the stored JWT became one. `expires_in` is the time from 09:05:10 to
the JWT's `exp`. The response shape is the one users' exchanges return.

The audit record of this exchange carries `subject_kind: agent`, the agent, the service,
`service_account_id: aa0e8400-e29b-41d4-a716-446655440100` and no token.

**Step 6 — 09:05:10: the gateway authorizes the tool call (US3.12)**

Authorization context in the gateway's policy input (excerpt):

```json
{
  "context": {
    "subject_kind": "agent",
    "principal": "550e8400-e29b-41d4-a716-446655440000",
    "granted_permission_sets_available": true,
    "granted_permission_sets": {
      "770e8400-e29b-41d4-a716-446655440001": ["880e8400-e29b-41d4-a716-446655440010"]
    }
  }
}
```

The policy allows the tickets MCP for *Platform reader* and forwards the call with
`Authorization: Bearer SYNTHETIC_PLATFORM_TRIAGE_JWS`. The MCP validates a token from
`https://idp.example.com` with `sub: triage-bot`, exactly as it validates a human's token.

**Step 7 — 09:06:00: planner-bot calls the planning MCP (US3.6)**

The same exchange with planner-bot's self token and
`resource=https://mcp.example.com/planning` returns `SYNTHETIC_PLATFORM_PLANNER_JWS`. Both
resources belong to the one Company Platform service, so each agent holds one token for the
service and the gateway tells the two MCPs apart by permission set.

**Observable result**: each agent reaches the internal MCPs with the token its own issuer signed
for it. The broker signed nothing for the MCPs and made no outbound call. The audit trail shows
which agent received which account's token.

### Journey 2 — Denials, rotation, expiry and a human behind the same agent (User Stories 2–4, Scenarios US2.2, US2.5, US2.9, US3.3, US3.4, US3.7, US3.8, US4.1)

**Cast**: the state at the end of Journey 1. In addition: agent *audit-bot*
(`660e8400-e29b-41d4-a716-446655440002`), which declares *Platform reader* and has no agent
grant; user Alice (`alice@example.com`), who consented to triage-bot for *Platform operator* and
has her own session at Company Platform.

**Step 1 — 10:00:00: audit-bot has no grant (US3.3)**

The gateway sends the Journey 1 Step 5 request with audit-bot's self token.

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{
  "error": "access_denied",
  "error_description": "agent is not authorized for this service"
}
```

There is no `error_uri`: nobody can consent for a machine.

**Step 2 — 10:01:00: Olivia grants audit-bot the reader set but names no account (US2.2, US3.4)**

```http
PUT /api/agents/660e8400-e29b-41d4-a716-446655440002/self-grant HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com
Content-Type: application/json

{
  "granted_permission_sets": {
    "770e8400-e29b-41d4-a716-446655440001": ["880e8400-e29b-41d4-a716-446655440010"]
  },
  "service_account_bindings": {}
}

HTTP/1.1 201 Created
```

The grant is stored. Company Platform has two accounts and the broker chooses neither. Repeating
Step 1 returns the same `403 access_denied` without `error_uri`.

**Step 3 — 10:02:00: Olivia tries to give triage-bot the operator set on its read-only account (US2.5)**

```http
PUT /api/agents/550e8400-e29b-41d4-a716-446655440000/self-grant HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com
Content-Type: application/json

{
  "granted_permission_sets": {
    "770e8400-e29b-41d4-a716-446655440002": ["880e8400-e29b-41d4-a716-446655440010"]
  },
  "service_account_bindings": {
    "880e8400-e29b-41d4-a716-446655440010": "aa0e8400-e29b-41d4-a716-446655440100"
  }
}

HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "error": "validation failed",
  "message": "granted access exceeds the ceiling of service account platform-triage-bot for service platform: write"
}
```

triage-bot's earlier grant is unchanged.

**Step 4 — 10:03:00: Olivia tries to delete an account that a grant binds (US2.9)**

```http
DELETE /api/services/880e8400-e29b-41d4-a716-446655440010/service-accounts/aa0e8400-e29b-41d4-a716-446655440100 HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com

HTTP/1.1 409 Conflict
Content-Type: application/json

{
  "error": "conflict",
  "message": "service account is bound by the agent grant of agent 550e8400-e29b-41d4-a716-446655440000"
}
```

**Step 5 — 10:10:00: the identity provider issues a new token for triage-bot and Olivia rotates (US1.5, US3.7)**

```http
PUT /api/services/880e8400-e29b-41d4-a716-446655440010/service-accounts/aa0e8400-e29b-41d4-a716-446655440100 HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com
If-Match: "sa-v1-7c9e"
Content-Type: application/json

{
  "canonical_id": "platform-triage-bot",
  "kind": "jwt_bearer",
  "scopes": ["read"],
  "credential": {
    "format": "signed_jwt",
    "use": "access_token",
    "jwt": "SYNTHETIC_PLATFORM_TRIAGE_JWS_2"
  }
}

HTTP/1.1 200 OK
Content-Type: application/json
ETag: "sa-v2-1d4a"
```

The response body shows the new fingerprint and `exp`. The next exchange for triage-bot returns
`SYNTHETIC_PLATFORM_TRIAGE_JWS_2`: the machine session that held the earlier JWT came from an
earlier version of the account and is not reused.

**Step 6 — 2026-12-25 00:00:01: planner-bot's stored token has expired (US1.6, US3.8)**

```http
GET /api/services/880e8400-e29b-41d4-a716-446655440010/service-accounts/aa0e8400-e29b-41d4-a716-446655440101/status HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com

HTTP/1.1 200 OK
Content-Type: application/json

{
  "status": "failed",
  "last_error_class": "credential_expired",
  "last_error_at": "2026-12-25T00:00:00Z"
}
```

An exchange for planner-bot now returns:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{
  "error": "access_denied",
  "error_description": "the service account for this agent is not ready; an administrator must repair it"
}
```

The expired JWT is not returned. The audit record names the account and the status.

**Step 7 — Alice uses triage-bot (US4.1)**

Alice's broker token has `sub: alice@example.com` and `agent_id` of triage-bot. The exchange
leaves the decision at its first step, takes today's path and returns the token from Alice's own
session. The gateway's policy input has `subject_kind: human` and `principal: alice@example.com`.
No service account is read.

**Observable result**: every refusal for a machine comes without a consent link; refusals caused
by an account tell administrators where to look; a rotation takes effect on the next exchange; an
expired stored token is never handed out; Alice's experience is unchanged.

### Journey 3 — One-time sign-in for Linear (User Story 5, Scenarios US5.1, US5.2, US5.3, US5.8)

**Cast**: the shared cast. Service *Linear* (`linear`,
`880e8400-e29b-41d4-a716-446655440011`) with its own client registration and resource
`https://mcp.linear.example`. Permission set *Linear contributor*
(`770e8400-e29b-41d4-a716-446655440003`): `read write issues:create comments:create` at Linear.
planner-bot declares it.

**Step 1 — 11:00:00: Olivia creates the account for the read-and-write level (US5.1)**

```http
POST /api/services/880e8400-e29b-41d4-a716-446655440011/service-accounts HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com
Content-Type: application/json

{
  "canonical_id": "linear-write",
  "kind": "bootstrapped_oauth",
  "scopes": ["read", "write", "issues:create", "comments:create"],
  "authorization_params": { "actor": "app" }
}

HTTP/1.1 201 Created
Content-Type: application/json
ETag: "sa-v1-52be"

{
  "id": "aa0e8400-e29b-41d4-a716-446655440102",
  "service_id": "880e8400-e29b-41d4-a716-446655440011",
  "canonical_id": "linear-write",
  "kind": "bootstrapped_oauth",
  "scopes": ["read", "write", "issues:create", "comments:create"],
  "authorization_params": { "actor": "app" },
  "narrowing": "none",
  "status": "needs_bootstrap",
  "bootstrap_url": "https://broker-admin.example.com/api/services/880e8400-e29b-41d4-a716-446655440011/service-accounts/aa0e8400-e29b-41d4-a716-446655440102/oauth2/authorize",
  "created_at": "2026-11-25T11:00:00Z",
  "updated_at": "2026-11-25T11:00:00Z"
}
```

The link carries nothing secret and can be shown permanently.

**Step 2 — 11:01:00: Olivia opens the link in her browser**

```http
GET /api/services/880e8400-e29b-41d4-a716-446655440011/service-accounts/aa0e8400-e29b-41d4-a716-446655440102/oauth2/authorize HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com

HTTP/1.1 302 Found
Location: https://linear.example/oauth/authorize?response_type=code&client_id=...
    &redirect_uri=https%3A%2F%2Fbroker.example.com%2Fapi%2Fthird-party%2F880e8400-e29b-41d4-a716-446655440011%2Foauth2%2Fcallback
    &scope=read%20write%20issues%3Acreate%20comments%3Acreate&actor=app
    &code_challenge=...&code_challenge_method=S256&state=eyJ...
```

`state` is sealed and opaque. It records Olivia as the person who started the sign-in, the
service, the purpose (service-account sign-in), the account and its current version, and the admin
page to return to. `redirect_uri` is the callback users already return to, so nothing changes at
the provider's registration.

**Step 3 — 11:01:20: Olivia approves at Linear and the browser returns (US5.2)**

```http
GET /api/third-party/880e8400-e29b-41d4-a716-446655440011/oauth2/callback?code=SYNTHETIC_CODE&state=eyJ... HTTP/1.1
Host: broker.example.com
X-Remote-User: olivia@example.com

HTTP/1.1 302 Found
Location: https://broker-admin.example.com/services/880e8400-e29b-41d4-a716-446655440011/service-accounts/aa0e8400-e29b-41d4-a716-446655440102
```

Linear's token response states `scope: "read write issues:create comments:create"`, exactly the
declared ceiling. The broker stores the tokens on the service account and writes no user session
for Olivia. The account now reads:

```json
{
  "status": "ready",
  "bootstrapped_by": "olivia@example.com",
  "bootstrapped_at": "2026-11-25T11:01:20Z"
}
```

Variant (US5.3): had Olivia's approval at Linear produced `scope: "read"`, nothing would be
stored and status would read `{"status": "failed", "last_error_class": "scope_mismatch"}`.

**Step 4 — 11:05:00: planner-bot is granted and uses Linear (US5.8)**

Olivia adds *Linear contributor* to planner-bot's grant and binds Linear to
`aa0e8400-e29b-41d4-a716-446655440102`. The gateway's exchange for
`resource=https://mcp.linear.example` returns:

```json
{
  "access_token": "SYNTHETIC_LINEAR_APP_TOKEN",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "granted_permission_sets": {
    "770e8400-e29b-41d4-a716-446655440003": ["880e8400-e29b-41d4-a716-446655440011"]
  },
  "principal": "660e8400-e29b-41d4-a716-446655440001"
}
```

A read-only agent would be bound to a second account, `linear-read`, signed in with `read` only.

**Observable result**: a person was needed once. Agents receive the token of the account their
grant names; the token never appears in Olivia's session list.

### Journey 4 — Two agents share one Google service account at different levels (User Story 6, Scenarios US6.1, US6.5, US6.7)

**Cast**: the shared cast. Service *Google Workspace* (`google-workspace`,
`880e8400-e29b-41d4-a716-446655440012`) with resource `https://mcp.example.com/drive`.
Permission sets *Drive reader* (`770e8400-e29b-41d4-a716-446655440004`:
`https://www.googleapis.com/auth/drive.readonly`) and *Drive editor*
(`770e8400-e29b-41d4-a716-446655440005`: `https://www.googleapis.com/auth/drive` and
`https://www.googleapis.com/auth/documents`). triage-bot is granted the reader set and
planner-bot the editor set.

**Step 1 — 13:00:00: Olivia registers the Google key**

```http
POST /api/services/880e8400-e29b-41d4-a716-446655440012/service-accounts HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com
Content-Type: application/json

{
  "canonical_id": "drive-reporting",
  "kind": "jwt_bearer",
  "scopes": [
    "https://www.googleapis.com/auth/drive.readonly",
    "https://www.googleapis.com/auth/drive",
    "https://www.googleapis.com/auth/documents"
  ],
  "credential": {
    "format": "google_service_account_json",
    "service_account_json": "{ \"type\": \"service_account\", ... }"
  }
}

HTTP/1.1 201 Created
Content-Type: application/json
ETag: "sa-v1-90aa"

{
  "id": "aa0e8400-e29b-41d4-a716-446655440103",
  "canonical_id": "drive-reporting",
  "kind": "jwt_bearer",
  "narrowing": "per_request",
  "status": "ready",
  "credential_summary": {
    "format": "google_service_account_json",
    "client_email": "drive-reporting@acme-prod.iam.gserviceaccount.com",
    "key_id": "8f2c41d07b6e",
    "fingerprint": "sha256:a40c9d1e77f35b2086c4e9d0b1f37a52"
  },
  ...
}
```

The ceiling lists `drive.readonly` as well as `drive`, because containment is literal. Both
agents' grants bind Google Workspace to `aa0e8400-e29b-41d4-a716-446655440103`.

**Step 2 — 13:05:00: triage-bot's exchange mints a read-only token (US6.1)**

The broker signs an assertion and presents it at Google's token endpoint. Decoded assertion:

```json
{
  "header": { "alg": "RS256", "kid": "8f2c41d07b6e" },
  "payload": {
    "iss": "drive-reporting@acme-prod.iam.gserviceaccount.com",
    "scope": "https://www.googleapis.com/auth/drive.readonly",
    "aud": "https://oauth2.googleapis.com/token",
    "iat": 1795611900,
    "exp": 1795615500
  }
}
```

```http
POST /token HTTP/1.1
Host: oauth2.googleapis.com
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=SYNTHETIC_GOOGLE_ASSERTION_JWS

HTTP/1.1 200 OK
Content-Type: application/json

{ "access_token": "SYNTHETIC_GOOGLE_READ_TOKEN", "token_type": "Bearer", "expires_in": 3600 }
```

The gateway receives `SYNTHETIC_GOOGLE_READ_TOKEN` with `expires_in: 3600`.

**Step 3 — 13:06:00: planner-bot's exchange mints an editor token (US6.1)**

The same flow asks for
`scope: "https://www.googleapis.com/auth/documents https://www.googleapis.com/auth/drive"` and
returns `SYNTHETIC_GOOGLE_EDIT_TOKEN`. The two machine sessions are held separately per agent.

**Step 4 — 13:20:00 and 13:56:00: reuse and re-mint (US6.5, US6.7)**

At 13:20:00 triage-bot's next exchange returns the cached `SYNTHETIC_GOOGLE_READ_TOKEN` with
`expires_in: 2700` and Google is not called. At 13:56:00, inside the five-minute lookahead before
the 14:05:00 expiry, background refresh mints a new read-only token for triage-bot.

**Observable result**: one credential, two agents, two tokens that each carry exactly the granted
access. The service account only sees documents shared with it or in a Shared Drive it belongs
to, and what an agent creates must live in a Shared Drive; the user documentation says so.

### Journey 5 — Two agents share one GitHub App installation (User Story 7, Scenarios US7.1, US7.2, US7.3, US7.5)

**Cast**: the shared cast. Service *GitHub for agents* (`github-app`,
`880e8400-e29b-41d4-a716-446655440013`), separate from the GitHub service users sign in to, with
resource `https://mcp.example.com/github`. Permission sets *Issue triage*
(`770e8400-e29b-41d4-a716-446655440006`: `issues:write metadata:read`) and *Release*
(`770e8400-e29b-41d4-a716-446655440007`: `contents:write metadata:read`). triage-bot is granted
*Issue triage*, planner-bot *Release*.

**Step 1 — 14:00:00: Olivia registers the installation (US7.1)**

```http
POST /api/services/880e8400-e29b-41d4-a716-446655440013/service-accounts HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com
Content-Type: application/json

{
  "canonical_id": "release-automation",
  "kind": "github_app",
  "scopes": ["contents:write", "issues:write", "metadata:read"],
  "repositories": ["acme/app", "acme/infra"],
  "credential": {
    "app_id": "123456",
    "installation_id": "12345678",
    "private_key_pem": "-----BEGIN RSA PRIVATE KEY-----..."
  }
}

HTTP/1.1 201 Created
```

The account gets `aa0e8400-e29b-41d4-a716-446655440104`. Its status after the first observation:

```json
{
  "status": "ready",
  "observed_scopes": ["contents:write", "issues:write", "metadata:read", "pull_requests:write"],
  "observed_at": "2026-11-25T14:00:01Z"
}
```

The effective ceiling is what both the declared ceiling and the installation allow:
`contents:write issues:write metadata:read`.

**Step 2 — 14:05:00: triage-bot's exchange mints an issues-only token (US7.2, US7.3)**

```http
POST /app/installations/12345678/access_tokens HTTP/1.1
Host: api.github.com
Authorization: Bearer SYNTHETIC_GITHUB_APP_JWS
Accept: application/vnd.github+json

{
  "repositories": ["app", "infra"],
  "permissions": { "issues": "write", "metadata": "read" }
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "token": "SYNTHETIC_GITHUB_INSTALLATION_TOKEN_TRIAGE",
  "expires_at": "2026-11-25T15:05:00Z",
  "permissions": { "issues": "write", "metadata": "read" }
}
```

planner-bot's exchange asks for `{"contents": "write", "metadata": "read"}` and receives a
different token. triage-bot never holds a token that can push.

**Step 3 — 15:00:00: the installation loses `contents` at GitHub (US7.5)**

At the next observation status reads
`"observed_scopes": ["issues:write", "metadata:read", "pull_requests:write"]`. planner-bot's next
exchange is denied:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{
  "error": "access_denied",
  "error_description": "granted access exceeds the service account's ceiling; an administrator must repair it"
}
```

triage-bot is unaffected.

**Observable result**: one installation serves both agents with different permissions, and
tightening the installation at GitHub tightens every agent without a change in the broker.

### Journey 6 — A keyless Google account (User Story 8, Scenarios US8.1, US8.2)

**Cast**: the shared cast and the Google Workspace service of Journey 4. Google is configured
with a workload identity pool provider that trusts issuer `https://broker.example.com`, and with
a rule that lets the broker identity of triage-bot act as
`drive-reporting@acme-prod.iam.gserviceaccount.com`.

**Step 1 — 16:00:00: Olivia registers the keyless account (US8.1)**

```http
POST /api/services/880e8400-e29b-41d4-a716-446655440012/service-accounts HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: olivia@example.com
Content-Type: application/json

{
  "canonical_id": "drive-reporting-keyless",
  "kind": "google_workload_identity",
  "scopes": ["https://www.googleapis.com/auth/drive.readonly"],
  "federation": {
    "audience": "//iam.googleapis.com/projects/1234567890/locations/global/workloadIdentityPools/broker/providers/broker-oidc",
    "service_account_email": "drive-reporting@acme-prod.iam.gserviceaccount.com"
  }
}

HTTP/1.1 201 Created
```

The account gets `aa0e8400-e29b-41d4-a716-446655440105`, `status: ready`,
`narrowing: per_request` and no credential. Olivia rebinds triage-bot's grant to it.

**Step 2 — 16:05:00: triage-bot's exchange uses federation (US8.2)**

The broker presents a broker-signed token that identifies triage-bot to Google's federation
endpoint and receives a federated token:

```json
{
  "header": { "alg": "ES256", "kid": "3f1c2b4a-8d7e-4f60-9a1b-2c3d4e5f6a7b" },
  "payload": {
    "iss": "https://broker.example.com",
    "sub": "550e8400-e29b-41d4-a716-446655440000",
    "aud": "//iam.googleapis.com/projects/1234567890/locations/global/workloadIdentityPools/broker/providers/broker-oidc",
    "iat": 1795622700,
    "exp": 1795623000
  }
}
```

It then asks Google for an access token for
`drive-reporting@acme-prod.iam.gserviceaccount.com` with
`scope: ["https://www.googleapis.com/auth/drive.readonly"]` and returns it to the gateway. The
decoded token above shows the assumed shape; which broker-signed token is presented is an open
point (FR-112).

**Observable result**: the agent holds the same narrowed Google token as in Journey 4, and the
broker stores no Google secret.

## Requirements *(mandatory)*

### Functional Requirements

**Step 1 (#471) — Service accounts and the stored-JWT kind**

- **FR-001**: Administrators MUST be able to create, list, read, update and delete service
  accounts attached to one existing third-party service, the same service record users hold
  sessions with. A service MAY hold several service accounts.
- **FR-002**: A service account MUST have a stable identifier, an administrator-chosen
  `canonical_id` that is unique within its service and can be renamed, a kind, a non-empty scope
  ceiling, and a status. Other records MUST refer to it by its stable identifier.
- **FR-003**: Credential material MUST be write-only. No response, list, status, log, trace,
  metric or audit record may contain it. Responses MUST carry a credential summary that lets an
  operator recognise a rotation.
- **FR-004**: The account representation MUST be the same in its read and in its list entry and
  MUST carry a strong version tag. An update MAY be made conditional on that tag; a stale tag MUST
  be refused; an unconditional update MUST be applied.
- **FR-005**: An update MUST be able to replace the credential in place (rotation), change the
  ceiling and rename the account, without changing its identifier. The kind MUST NOT change.
- **FR-006**: Narrowing MUST be derived from the kind and credential format, shown read-only, and
  never settable.
- **FR-007**: The `jwt_bearer` kind with format `signed_jwt` MUST accept a JWT that was signed
  outside the broker and a `use` value. `use` MUST accept only `access_token`, MUST default to it,
  and MUST be modelled so that a later `assertion` value with a token endpoint needs no new
  format.
- **FR-008**: A stored JWT MUST be opaque to the broker: stored encrypted, never verified, never
  rebuilt, never altered.
- **FR-009**: On write, a stored JWT MUST be refused unless it is a compact signed JWT, its
  algorithm is not `none`, it is at most 16 KiB, and any expiry it states lies in the future.
- **FR-010**: The summary of a stored JWT MUST show its `iss`, `sub`, `kid` and `exp` as read from
  the token without verification, and a fingerprint. These values MUST be used for display and
  status only and MUST NOT drive an authorization decision.
- **FR-011**: A stored-JWT account MUST be `ready` on create and after rotation, and MUST be
  `failed` with error class `credential_expired` once a stated expiry has passed, until the
  credential is replaced. Its narrowing MUST be `none`.
- **FR-012**: A status read MUST return the status, the last error class and time, and, where the
  provider exposes it, the ceiling as observed at the provider.
- **FR-013**: Deleting a service account MUST be refused while any agent grant binds it, and the
  refusal MUST identify a binding grant. This check takes effect with Step 2.
- **FR-014**: Deleting a service MUST be refused while it has service accounts.
- **FR-015**: Creating, rotating, updating and deleting a service account MUST each write an
  audit record with the acting administrator, the service, the account and no credential.
- **FR-016**: Renewal of a stored JWT is outside the broker. The broker MUST NOT attempt to renew
  it.

**Step 2 (#472) — Agent grants**

- **FR-020**: Administrators MUST be able to write, read and delete one agent grant per agent
  through the admin API only. Writing replaces the grant as a whole.
- **FR-021**: An agent grant MUST be stored and evaluated like a user's consent, with the agent's
  own identifier as the principal, so that grant verification, effective-access resolution and the
  gateway's permission-set input behave as for users.
- **FR-022**: An agent grant MUST list permission sets with their covered services and MAY carry
  an end date. Every listed permission set MUST be one the agent declares.
- **FR-023**: An agent grant MUST carry service-account bindings: at most one account per
  service. A binding MUST always be explicit. The broker MUST NOT choose a default account, also
  when the service has exactly one.
- **FR-024**: A binding MUST name an account of the service it is given for. A binding for a
  service that no granted permission set covers MUST be refused.
- **FR-025**: A service that the grant covers and does not bind MUST be accepted at write time
  and MUST give the agent no token.
- **FR-026**: For every bound service, the agent's effective access (the union over its granted
  permission sets at that service) MUST be contained in the bound account's ceiling when the grant
  is written. A violation MUST refuse the write and name the service and the excess.
- **FR-027**: Ceiling containment MUST be evaluated in the grammar of the account's kind: literal
  set containment for flat scopes, and level-ordered containment for `permission:level` scopes.
- **FR-028**: Agent grants and machine sessions MUST be distinguishable from user grants and user
  sessions by principal kind in every query, so that listings can filter honestly.
- **FR-029**: The consent screens and every user-facing listing of grants MUST NOT show agent
  grants. An agent grant MUST NOT be creatable, changeable or revocable through any user-facing
  path.
- **FR-030**: Writing and deleting an agent grant MUST write an audit record with the acting
  administrator, the agent, the permission sets and the bindings. Deleting an agent grant MUST
  make its machine sessions unusable at once.

**Step 3 (#473) — Machine branch in token exchange**

- **FR-040**: When agent-subject detection matches at third-party exchange, the request MUST enter
  the machine path instead of being rejected. Everything before that point (gateway
  authentication, subject-token validation, authorization policy, resource resolution) MUST be
  unchanged.
- **FR-041**: A detection failure MUST remain a terminal `server_error`. A non-match MUST continue
  on today's human path unchanged. The machine path MUST NOT read human grants or sessions, and
  the human path MUST NOT read agent grants, machine sessions or service accounts.
- **FR-042**: The machine path MUST decide in this order: the agent holds an active agent grant
  that covers the resolved service; the grant binds an account for that service; the effective
  access is within the account's ceiling and the account is `ready`; a reusable machine session
  exists, otherwise a token is obtained from the account's token source.
- **FR-043**: No grant, an expired grant, a grant that does not cover the service, or no binding
  for the service MUST yield `access_denied` without a consent link.
- **FR-044**: Access beyond the ceiling, or an account that is not `ready`, MUST yield a denial
  without a consent link whose description tells administrators that the service account is the
  cause. It MUST NOT reveal credential material or provider error text.
- **FR-045**: The ceiling check MUST run on every exchange, not only when the grant is written.
- **FR-046**: Each kind MUST supply a machine token source that receives the agent's effective
  access and returns a token and its lifetime. A source that can narrow MUST request exactly the
  effective access. A source that cannot MUST return the account's token as it is.
- **FR-047**: The stored-JWT source MUST return the account's stored JWT unchanged as the access
  token, with the lifetime taken from the JWT's stated expiry when present, and MUST make no
  outbound call, sign nothing and narrow nothing.
- **FR-048**: When a source that cannot narrow hands out more than the agent's effective access,
  the audit record of the exchange MUST list the excess.
- **FR-049**: A machine session MUST be held per agent and service. It MUST be reused only when
  it was obtained from the account the grant binds now, from the current version of that account's
  credential, and with access equal to the current effective access. Otherwise a new token MUST be
  obtained and the earlier one MUST NOT be served.
- **FR-050**: One agent's machine session MUST never be served to another agent, also when both
  have identical access.
- **FR-051**: A rotation MUST take effect on the next exchange.
- **FR-052**: An account in status `failed` MUST yield no token. An expired stored JWT MUST never
  be handed out.
- **FR-053**: The exchange response MUST keep its existing shape for machine exchanges, including
  the granted permission sets and the principal, which is the agent's identifier.
- **FR-054**: The gateway's authorization input MUST gain `subject_kind` with the values `human`
  and `agent`. All existing inputs MUST be unchanged.
- **FR-055**: Scopes MUST stay flat per service. Resources of one service share one machine
  session, and the gateway tells MCPs apart by permission set, as for users.
- **FR-056**: Background refresh MUST re-mint machine sessions through the token source for
  sources that can mint, MUST leave a stored JWT alone, and MUST drop machine sessions whose grant
  or binding is gone.
- **FR-057**: Approval endpoints MUST keep refusing agent-subject tokens. A gateway policy outcome
  that would ask a human for approval MUST deny an agent's call.
- **FR-058**: The self token routes (the agent's own broker secret, and the impersonation flow for
  a trusted workload) MUST be unchanged, and both MUST be exchangeable through the machine path.
- **FR-059**: Every machine exchange decision, successful or denied, MUST write an audit record
  with `subject_kind`, the agent, the service, the `service_account_id` where one was resolved,
  the outcome and, for impersonated self tokens, the acting workload.
- **FR-060**: A decision record that supersedes ADR `045-agent-subject-impersonation` decision 6
  for exchange MUST be accepted before this step merges. It MUST keep decision 6a and the
  fail-closed properties named in FR-041 and FR-043.
- **FR-061**: User documentation for MCP authors MUST state: the token is the one their own issuer
  signed for the agent, so validation is unchanged; one service account per agent is needed for
  the MCP to see which agent called; the broker adds no permission sets or workload attribution to
  the token; whether the agent identity travels beyond the MCP is between the MCP and the token's
  issuer.
- **FR-062**: The delegation decision endpoint that the gateway can consult before policy
  evaluation today rejects agent-subject tokens. [NEEDS CLARIFICATION: with grant-context
  resolution enabled at the gateway, must Step 3 also make that endpoint return a permit for an
  agent holding an agent grant, or does the machine path in phase 1 work only for gateways that
  take the permission sets from the exchange response?]

**Step 4 (#474) — `bootstrapped_oauth`**

- **FR-070**: The `bootstrapped_oauth` kind MUST hold no secret at creation, MUST refuse a
  credential, MUST be created in status `needs_bootstrap`, and MUST use the service's own client
  registration. It MAY carry authorization parameters that are merged over the service's.
- **FR-071**: The admin side MUST offer a browser-facing sign-in link per account that carries
  nothing secret. Opening it MUST capture the administrator's identity from the admin side's
  authentication, seal it with the purpose, the account and the account's current version, and
  redirect to the provider with proof-key protection and the existing callback address.
- **FR-072**: The existing callback MUST validate the returned state as it does for users,
  including that the browser is recognised as the same person, and MUST then branch on the
  purpose.
- **FR-073**: For a service-account sign-in the callback MUST check that the account's version
  still equals the sealed one and that the provider granted exactly the declared ceiling. On
  success it MUST store the tokens on the service account, set status `ready`, record who
  completed it and when, write a `service_account.bootstrapped` audit record, and return the
  browser to the sealed admin page.
- **FR-074**: A scope mismatch MUST store nothing and set status `failed` with reason
  `scope_mismatch`. A provider error MUST store nothing and set status `failed` with the
  provider's error code.
- **FR-075**: A service-account sign-in MUST NOT create or change a user session, and the result
  MUST NOT appear in any user's session list.
- **FR-076**: A sealed state MUST be unusable after a sign-in for the account succeeded or the
  account changed, in addition to its time limit.
- **FR-077**: Repeating the sign-in on a `ready` account MUST keep the earlier token in use until
  the new one is stored, and MUST then revoke the earlier one where the provider offers
  revocation.
- **FR-078**: The token source of this kind MUST return the stored token, MUST refresh it when a
  refresh token exists, and MUST NOT narrow.
- **FR-079**: An account in `needs_bootstrap` MUST yield a denial at exchange that tells
  administrators the account still needs its sign-in.
- **FR-080**: The account representation MUST show the sign-in link, who completed the last
  sign-in and when.

**Step 5 (#475) — Signing formats and `client_credentials`**

- **FR-090**: The `jwt_bearer` kind MUST gain the formats `google_service_account_json` and
  `private_key`. For both, the broker MUST sign an assertion itself and present it at the token
  endpoint as a JWT-bearer authorization grant (RFC 7523 §2.1).
- **FR-091**: The assertion MUST be signed with RS256 or ES256, carry the key identifier, live at
  most one hour, and request exactly the agent's effective access as its scope.
- **FR-092**: A `private_key` credential MUST carry issuer, subject, audience, token endpoint,
  algorithm, key identifier and the key. A key MUST be RSA of at least 2048 bits or EC P-256. A
  Google key file MUST pass the Google key checks the broker already applies.
- **FR-093**: The `client_credentials` kind MUST request a token at the service's token endpoint
  with the client-credentials grant (RFC 6749 §4.4) and the agent's effective access as scope. Its
  credential MUST be a client identifier with either a secret or a private key for key-based
  client authentication.
- **FR-094**: Both kinds MUST report narrowing `per_request`.
- **FR-095**: Summaries MUST show the key identifier and fingerprint, the Google account e-mail,
  or the client identifier, as applicable.
- **FR-096**: A failure to obtain a token from the provider MUST be classified as a dependency
  error, MUST fail the exchange without a token and without provider error text, and MUST be
  recorded as the account's last mint error class.
- **FR-097**: The `signed_jwt` format and its `use` field MUST be unchanged by this step.
- **FR-098**: User documentation MUST present Google Docs as the worked example, including that
  the service account only sees what is shared with it or lies in a Shared Drive it belongs to,
  and that what an agent creates must live in a Shared Drive.

**Step 6 (#476) — `github_app`**

- **FR-100**: The `github_app` kind MUST take an app identifier, an installation identifier and
  the app's key, and MAY take a repository restriction.
- **FR-101**: Scopes of this kind MUST use the `permission:level` grammar with known permission
  names and the levels `read`, `write` and `admin`. The same validation MUST apply to permission
  sets that can be bound to such an account.
- **FR-102**: The broker MUST read the installation's permissions from the provider on a bounded
  interval, show them as the observed ceiling in status, and treat the intersection of declared
  and observed ceiling as the effective ceiling at exchange.
- **FR-103**: The token source MUST request an installation token with exactly the agent's
  effective permissions and, when set, the account's repository restriction.
- **FR-104**: Installation tokens MUST be re-minted before they expire through the same
  background refresh as other machine sessions.
- **FR-105**: The kind MUST work against GitHub Enterprise Server by using the service's base
  address.
- **FR-106**: Narrowing MUST be `per_request`. Splitting repositories between agents requires a
  second account; the broker MUST NOT narrow repositories per agent.
- **FR-107**: The summary MUST show the app identifier, the installation identifier and a key
  fingerprint.
- **FR-108**: User documentation MUST state that a GitHub App for agents and a GitHub OAuth App
  for users use different vocabularies and are registered as two services.

**Step 7 (#477) — Keyless**

- **FR-110**: The `google_workload_identity` kind MUST hold no secret and MUST refuse a
  credential. It MUST name the provider trust configuration and the Google service account to act
  as.
- **FR-111**: The token source MUST exchange a broker-signed token at the provider's federation
  endpoint and then request a provider access token with exactly the agent's effective access.
- **FR-112**: The broker-signed token presented to the provider MUST identify the agent, so that
  provider-side rules can allow different agents to act as different Google service accounts.
  [NEEDS CLARIFICATION: is the presented token the agent's self token as received at exchange,
  which names no audience, or a short-lived token the broker mints for this purpose with the
  provider's audience? The second is assumed in Journey 6 and has to be reconciled with the
  decision that the broker signs no tokens for downstream services.]
- **FR-113**: User documentation MUST describe the broker's key discovery and audience policy as
  needed for the provider's trust configuration.
- **FR-114**: Narrowing MUST be `per_request`. A provider refusal MUST be handled as in FR-096.
- **FR-115**: The kind MUST be shaped so that other providers that accept the broker's tokens can
  be added as further kinds without changing agent grants or the exchange decision.

### Domain Model

**Domain Entity Diagram**:

```mermaid
erDiagram
    ThirdpartyOAuth2Service {
        uuid id PK
        string canonical_id
    }
    ServiceAccount {
        uuid id PK
        uuid service_id FK
        string canonical_id
        string kind
        string_list scopes
        string narrowing
        string status
        secret credential
        int credential_version
    }
    Agent {
        uuid id PK
    }
    PermissionSet {
        uuid id PK
        string name
    }
    AgentGrant {
        uuid id PK
        uuid agent_id FK
        string principal_kind
        datetime valid_until
    }
    ServiceAccountBinding {
        uuid grant_id FK
        uuid service_id FK
        uuid service_account_id FK
    }
    MachineSession {
        uuid agent_id FK
        uuid service_id FK
        uuid service_account_id FK
        int credential_version
        string_list minted_scopes
        datetime expires_at
    }
    ThirdpartyOAuth2Service ||--o{ ServiceAccount : "holds"
    Agent ||--o| AgentGrant : "is principal of"
    Agent }o--o{ PermissionSet : "declares"
    AgentGrant }o--o{ PermissionSet : "grants"
    PermissionSet }o--|{ ThirdpartyOAuth2Service : "scoped at"
    AgentGrant ||--o{ ServiceAccountBinding : "names"
    ServiceAccountBinding }o--|| ServiceAccount : "uses"
    Agent ||--o{ MachineSession : "holds"
    MachineSession }o--|| ServiceAccount : "obtained from"
```

`AgentGrant` and `MachineSession` are the existing user-grant and user-session records with the
agent as principal; they are drawn separately to show what the design adds.

**Exchange decision**:

```mermaid
flowchart TD
    A["Gateway authenticated, subject token valid, service resolved"] --> B{Agent subject?}
    B -- "detection fails" --> X["server_error"]
    B -- "no" --> H["Existing human path"]
    B -- "yes" --> C{Active agent grant covers service?}
    C -- "no" --> D1["access_denied, no consent link"]
    C -- "yes" --> E{Grant binds an account?}
    E -- "no" --> D1
    E -- "yes" --> F{Access within ceiling and account ready?}
    F -- "no" --> D2["Denied, points administrators to the account"]
    F -- "yes" --> G{Reusable machine session?}
    G -- "yes" --> R["Return cached token"]
    G -- "no" --> M["Obtain token from the account's source, cache, return"]
```

**Service account status**:

```mermaid
stateDiagram-v2
    [*] --> ready: created with a credential or keyless
    [*] --> needs_bootstrap: created as bootstrapped_oauth
    needs_bootstrap --> ready: sign-in completed with the declared ceiling
    needs_bootstrap --> failed: scope mismatch or provider error
    ready --> failed: stored JWT expired
    failed --> ready: credential replaced or sign-in repeated
    ready --> ready: rotated
    ready --> [*]: deleted when unbound
    failed --> [*]: deleted when unbound
    needs_bootstrap --> [*]: deleted when unbound
```

A mint failure records the last mint error class and does not change the status.

**Entities**:

- **ServiceAccount**: a machine credential at one third-party service. Identity is its stable
  identifier; `canonical_id` is unique within the service. Invariants: non-empty ceiling; kind
  never changes; credential never readable; narrowing derived.
- **AgentGrant**: a user grant whose principal is an agent. At most one per agent.
- **MachineSession**: a user session whose principal is an agent. At most one per agent and
  service.

**Aggregates**:

- **ThirdpartyOAuth2Service** is the consistency boundary for its service accounts: an account
  cannot exist without its service, and a service cannot be deleted while it has accounts.
- **AgentGrant** with its bindings: bindings are written and validated with the grant as one
  unit.

**Value Objects**:

- **Kind**: `jwt_bearer`, `bootstrapped_oauth`, `client_credentials`, `github_app`,
  `google_workload_identity`. Fixed for an account's lifetime.
- **Ceiling**: the non-empty scope set an account may ever obtain, in the grammar of its kind.
- **Narrowing**: `none` or `per_request`; a fact about the provider, derived from the kind.
- **Credential summary**: non-secret identifying values and a fingerprint.
- **Binding**: service and service account, meaningful only within one agent grant.
- **Subject kind**: `human` or `agent`.

**Domain Events**:

- Service account created, updated, rotated, deleted, bootstrapped, bootstrap failed.
- Agent grant written, deleted.
- Machine token issued, with excess recorded where the source cannot narrow; machine exchange
  denied.

*The terms Service Account, Kind, Ceiling, Narrowing, Agent Grant, Binding, Machine Session, Self
Token and Subject Kind are added to the `ARCHITECTURE.md` glossary in the step that introduces
them. "Subject Kind" is the exchange-time counterpart of the existing "Subject Category" of
impersonation; the glossary states the relation.*

### Configuration Requirements

- **CFG-001 (Steps 1–5)**: No new configuration key is required. The machine path uses the
  existing `token_exchange.claim_extraction.agent_subject_expression` for recognition and the
  existing `token_refresh.*` settings for background re-minting.
- **CFG-002 (Step 6)**: The interval at which observed ceilings are read MUST be configurable and
  bounded.

**Configuration Parameters**:

- **`service_accounts.observed_ceiling_refresh_interval`**: Go duration, how often the broker
  reads the ceiling from providers that expose it, default `5m`, allowed range `1m`–`1h`.

**Example YAML Configuration**:

```yaml
token_exchange:
  claim_extraction:
    # Unchanged. A match now selects the machine path instead of a refusal.
    # An override must keep matching broker self tokens.
    agent_subject_expression: >-
      has(subject_token.sub) && has(subject_token.agent_id) &&
      subject_token.sub == subject_token.agent_id
token_refresh:
  lookahead_duration: "5m"       # also the re-mint window for machine sessions
service_accounts:
  observed_ceiling_refresh_interval: "5m"   # Step 6: GitHub App installations
```

**Configuration Location**: the configuration reference, an example under `examples/config/` and
the Helm values are updated in the step that introduces a key.

### API Requirements

- **API-001**: The end-user contract in `api/enduser/openapi.yaml` MUST be updated before Step 3
  is implemented: the description of agent-subject tokens at third-party exchange, the new denial
  examples, and `subject_kind` wherever the gateway obtains it.
- **API-002**: The admin contract in `api/admin/openapi.yaml` MUST be updated before each step is
  implemented, with the resources below, examples per kind, error responses and authentication.

  | Resource | Step | Purpose |
  |---|---|---|
  | `POST`, `GET /api/services/{service-id}/service-accounts` | 1 | Create and list |
  | `GET`, `PUT`, `DELETE /api/services/{service-id}/service-accounts/{service-account-id}` | 1 | Read, rotate or change, remove |
  | `GET /api/services/{service-id}/service-accounts/{service-account-id}/status` | 1 | Status, last error class, observed ceiling |
  | `PUT`, `GET`, `DELETE /api/agents/{agent-id}/self-grant` | 2 | The agent grant |
  | `GET /api/services/{service-id}/service-accounts/{service-account-id}/oauth2/authorize` | 4 | Browser-facing sign-in start |

- **API-003**: The create and update request MUST be one schema discriminated by `kind`, with
  `credential` as a write-only alternative keyed by the same discriminator and, for `jwt_bearer`,
  by `format`. Each later step adds an alternative and MUST NOT change existing ones.
- **API-004**: Path parameters and identifiers MUST follow the existing admin conventions: a
  service, agent or service account can be addressed by its identifier, and services and agents
  also by canonical identifier where the contract already allows it.
- **API-005**: Error responses MUST use the existing admin error envelope: validation failures as
  a bad request that names the failed rule, unknown resources as not found, a duplicate
  `canonical_id` and a delete that a binding or an account blocks as a conflict, and a stale
  version tag as a precondition failure.
- **API-006**: Each step's contract change MUST be confirmed by the maintainer before
  implementation.
- **API-007**: The admin contract version MUST be raised compatibly: every change in this feature
  is additive.
- **API-008**: The token-exchange response shape MUST NOT change in a way existing gateways can
  observe.

### Database Requirements

- **DB-001**: Schema changes MUST be migrations in `migrations/` with the next free sequence
  numbers, one atomic migration per step that changes the schema, each with a tested reverse
  direction.
- **DB-002 (Step 1)**: Service accounts MUST be persisted with uniqueness of `canonical_id`
  within a service and with referential protection so that a service with accounts cannot be
  removed.
- **DB-003 (Step 2)**: Bindings MUST be persisted with referential protection so that a bound
  account cannot be removed, and with at most one binding per grant and service. Grants and
  sessions MUST expose the principal kind for filtering. Existing rows MUST read as `human`.
- **DB-004 (Step 3)**: A machine session MUST record the account, the credential version and the
  access it was minted with.
- **DB-005**: Every new repository MUST have an in-memory and a PostgreSQL implementation with
  the same behaviour, and PostgreSQL integration tests.
- **DB-006**: A down migration MUST refuse or remove machine data explicitly; it MUST NOT leave
  agent grants readable as user grants.

### Security Requirements

- **SR-001**: The machine path MUST fail closed at every step: no grant, no binding, an expired
  grant, access beyond the ceiling, an account that is not `ready`, a detection failure and a mint
  failure all yield no token.
- **SR-002**: The broker MUST never choose a service account on its own. The blast radius is
  always an administrator's explicit choice.
- **SR-003**: A service account MUST never be used for a human subject, and a human's grant or
  session MUST never satisfy an agent subject, including when identifier texts collide.
- **SR-004**: All credential material (stored JWTs, keys, client secrets, provider tokens) MUST
  be encrypted at rest under the owning service's encryption context, with the mechanisms the
  broker already uses, and MUST NOT have a plaintext fallback.
- **SR-005**: Credential material and provider tokens are credentials under
  `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`: never logged, never on a
  span or metric, never in an error response.
- **SR-006**: Values read from a stored JWT without verification MUST NOT influence
  authorization, routing or identity; they are display and expiry data only.
- **SR-007**: The broker MUST never ask a provider for more than the agent's effective access,
  and never for more than the declared ceiling.
- **SR-008**: Signing MUST use vetted libraries only. The broker MUST NOT accept `alg: none`
  anywhere in this feature.
- **SR-009**: Only administrators can create, change or delete service accounts and agent grants
  and start a sign-in. No user-facing path can.
- **SR-010**: The sign-in MUST be bound to the person who started it, to the account and to the
  account's version, MUST be single-use after success, and MUST verify the granted scope before
  anything is stored. A forged link can only start a sign-in for an account an administrator
  already configured.
- **SR-011**: Every security-relevant operation MUST write a structured audit record: account and
  grant lifecycle, sign-in results, each machine exchange decision with `subject_kind` and
  `service_account_id`, and each hand-out beyond the granted access.
- **SR-012**: Agents MUST NOT be able to obtain human approval. Approval requests from agent
  tokens stay refused.
- **SR-013**: Outbound calls to provider endpoints MUST use the broker's existing protections for
  outbound requests, with bounded time and response size.
- **SR-014**: The threat model and assurance case MUST be updated in Step 3 for the machine path
  and in each later step for the new credential class it introduces.

### Binding Design Dependencies

- **ADR `045-agent-subject-impersonation`**: decision 6 is superseded for exchange by a new
  accepted ADR in Step 3 (FR-060). Decision 6a and all other decisions stay in force.
- **ADR `038-proactive-token-refresh`**: machine sessions join the existing background refresh.
- **ADR `008-encryption-context-optimization`** and **ADR `009-envelope-encryption-design`**:
  credentials are encrypted under the service's context.
- **ADR `048-authzen-delegation-pip`**: touched only by the open point in FR-062.
- **ADR `036-extproc-approval-cache-and-gate`**: unchanged; the gate denies agents (FR-057).

### Key Entities

- **Service account**: a machine credential at a third-party service, owned by administrators,
  with a kind, a ceiling and a status.
- **Agent grant**: an administrator's delegation of permission sets to an agent for its own use,
  with one explicit binding per service the agent should reach.
- **Binding**: which service account an agent grant uses at a service.
- **Machine session**: the token the broker obtained for an agent at a service, cached and
  refreshed like a user session.
- **Self token**: a broker token in which the agent is its own subject.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After Step 3, an agent that holds a self token reaches a company-built MCP through
  the gateway with a token issued for that agent, without any human identity, consent record or
  user session being created.
- **SC-002**: An administrator sets up a new agent for an existing service, from registering the
  credential to the first successful call, in under 10 minutes using the documented requests.
- **SC-003**: In every tested combination of a missing grant, missing binding, expired grant,
  exceeded ceiling, account not ready, expired stored token and failed detection, the agent
  receives no token and no consent link.
- **SC-004**: For all existing user journeys, the exchange result and the gateway's inputs are
  identical before and after the feature, apart from the added subject kind.
- **SC-005**: Where the provider can narrow, every token handed to an agent carries exactly the
  agent's granted access. Where it cannot, every hand-out beyond the granted access is visible in
  the audit trail.
- **SC-006**: No response, log line, trace, metric or audit record contains credential material
  in any tested scenario.
- **SC-007**: A credential rotation takes effect on the next exchange, and repeating a one-time
  sign-in causes no failed exchange for bound agents.
- **SC-008**: With two agents on one shared account, neither ever receives a token minted for the
  other.
- **SC-009**: A repeated exchange with unchanged access is answered from the cached machine
  session without a call to the provider, and a token within the refresh window is replaced
  before it expires.
- **SC-010**: Tightening a GitHub App installation at GitHub is reflected in the broker's
  decisions within the configured observation interval, with no change in the broker.
- **SC-011**: After Step 7, an agent obtains a narrowed Google token while the broker stores no
  Google secret for that account.
- **SC-012**: Each of the seven steps can be released on its own once the steps it depends on are
  released, with every scenario of its user stories passing.

## Assumptions

**Scope**

- This one specification replaces the per-phase specifications that #478 foresaw. Planning and
  tasks may still be produced per step from the Delivery Map.
- No user interface changes. The admin UI that mirrors these resources is a later feature; the
  sign-in link is returned by the admin API and its return target is an admin page address that
  the later UI will serve. The existing consent screens only gain the guarantee that agent grants
  never appear.
- Out of scope, as decided in #478: agents impersonating users without consent, user-to-user
  delegation, an approval workflow for agent grants, an operator queue for agent approvals,
  per-MCP scopes, a stored JWT used as an assertion (`use: assertion`), Google domain-wide
  delegation, a strict mode that refuses instead of handing out the ceiling, and the broker
  signing tokens for downstream services.
- The automated comments on #471 about a `broker_issued` audience default are ignored; that kind
  is not part of the design.

**Behaviour not decided by the issues**

- An agent grant uses the same permission-set-to-services shape as user consent
  (`granted_permission_sets`), so that it is stored and evaluated identically, and it may carry
  an end date as user consent can. It may contain any subset of the agent's declared permission
  sets; the mandatory and optional marking is a consent-screen notion and does not apply.
- Ceiling containment is literal for flat scopes. The broker has no knowledge that one provider
  scope implies another.
- Lowering a ceiling below a bound grant's access is accepted at write time and takes effect as
  a denial at exchange. The alternative, refusing the update, would make a ceiling impossible to
  tighten quickly.
- Refusals at the admin API use the existing bad-request, not-found, conflict and
  precondition-failed responses. Denials at exchange are `access_denied` without `error_uri`;
  account-caused denials differ only in their description and audit record. A failure to mint is
  a `server_error`, as other provider failures at exchange are today.
- Status is evaluated when it is read and at exchange, so a stored JWT is treated as failed from
  the moment its stated expiry passes.
- The kind of an account cannot be changed; a different kind is a new account.
- Deleting an agent deletes its agent grant and machine sessions. Deleting a `bootstrapped_oauth`
  account revokes its token where the provider offers revocation.
- Audit event names follow the existing `<area>.<entity>.<action>` convention; the names written
  in this specification are the ones #478 uses and are aligned with the convention in planning.
- How the gateway learns the subject kind (from the exchange response or its own evaluation of
  the token) is a design decision for planning, bounded by API-008.
- The observed-ceiling interval defaults to five minutes.
- For keyless accounts, one account names one Google service account. Which agent may act as
  which Google service account is enforced by the provider's trust configuration on the agent
  identity in the presented token, in addition to the grant's binding.

**Environment**

- The admin and user-facing sides recognise the same person behind the browser, because the
  existing callback checks the returning browser against the person who started the sign-in. If a
  deployment's admin side has no browser identity, the callback moves to the admin side with its
  own registered return address; that variant is not specified here.
- Whoever issued a stored JWT replaces it through the admin API before it expires.
- The company identity provider issues one token per agent where an MCP must see which agent
  called.
- Provider facts are as described in #478: Google and GitHub can narrow, Linear and a stored JWT
  cannot; GitHub exposes the real ceiling, Google and Linear do not.
- Steps 4 to 7 are validated against test providers shaped like Linear, a JWT-bearer token
  endpoint, the GitHub API and Google's federation endpoints. Validation against the real
  providers is manual.

## Open Clarifications

| # | Where | Question |
|---|---|---|
| 1 | FR-062, Step 3 | Must the delegation decision endpoint permit agent subjects with an agent grant in phase 1, so that gateways with grant-context resolution enabled can use the machine path? |
| 2 | FR-112, Step 7 | Which broker-signed token is presented to the provider for keyless accounts: the agent's self token, or a purpose-minted token with the provider's audience? |
