---
title: "Get started"
description: Run the local stack, grant an agent access, and call an MCP tool through Agentgateway without giving the agent a provider token.
---

# Get started

This tutorial starts with a fresh clone and ends with an agent tool call. You grant access
in the browser, inspect the stored grant, and send a broker token through Agentgateway.
The mock MCP server shows which provider claims it receives after token exchange.

Docker Compose seeds sample agents, services, and permission sets automatically.
The demo uses mock providers, not real GitHub or Google accounts.

## Before you start

You need Docker with Docker Compose v2, Git, and curl. The grant lookup also uses
[jq](https://jqlang.org/download/).

The project uses [just](https://github.com/casey/just) as a command runner.
Install it with `brew install just` or `cargo install just`, or use the plain Compose commands in Step 1.

```bash
git clone https://github.com/zalando-incubator/agentic-identity-broker.git
cd agentic-identity-broker
```

You do not need Go, Node.js, PostgreSQL, or cloud credentials on your host.
The stack uses an in-memory store and generates local encryption keys at startup.
This development stack trusts a simulated user identity. Do not expose it to other machines.

## Step 1: Start the stack

From the repository root:

```bash
just compose-up-detached
```

Without `just`, run:

```bash
test -f .env.compose || cp .env.compose.example .env.compose
IDENTITY_BROKER_JWE_SIGNING_KEY="$(./scripts/generate-jwe-key.sh)" \
IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY="$(./scripts/generate-jwe-key.sh)" \
  docker compose --env-file .env.compose -f docker-compose.yml up -d --build
```

The first run builds eight service images, including the Go backend and Vite frontend.
Startup depends on your machine, network, and build cache. A clean-machine cold-start time is not yet measured.
The command starts the containers; broker health alone does not mean that sample-data seeding has finished.
Wait for the agents in Step 2 and enabled login buttons on the sample client.

The `just` command prints these URLs after its health summary:

```text
Service URLs:
  - Browser UI (Vite proxy includes development identity): http://localhost:3000/
  - End-user broker (protected API requires upstream auth): http://localhost:8000/
  - Admin API: http://localhost:14000/api/agents
  - Admin health: http://localhost:14000/health
  - Sample OAuth2 client: http://localhost:9002/
  - Agentgateway MCP endpoint: http://localhost:4000/mcp
```

| Service | URL | Role |
|---|---|---|
| Consent UI | http://localhost:3000/ | Browser interface and development identity proxy |
| Broker — end-user API | http://localhost:8000/ | Consent, provider sessions, and OAuth2 endpoints |
| Broker — admin API | http://localhost:14000/ | Agent and service configuration |
| Sample agent | http://localhost:9002/ | OAuth2 client and broker-token display |
| Agentgateway | http://localhost:4000/mcp | JWT validation and transparent token exchange |
| Mock MCP server | http://localhost:9003/mcp | Tool backend; call it through the gateway |
| Mock OAuth2 providers | http://localhost:9000/, http://localhost:9001/ | Provider tokens and upstream authorization |

:::note
The Vite proxy adds `X-Remote-User: dev@example.com` to browser requests.
It simulates an authenticating reverse proxy. The broker trusts that asserted identity; it does not authenticate users itself.
:::

## Step 2: Review the seeded data

List the agents through the admin API:

```bash
curl --fail-with-body http://localhost:14000/api/agents
```

The response is a JSON array. Wait until it includes **Local Research Agent**.
If startup fails, inspect `docker compose --env-file .env.compose -f docker-compose.yml logs`.

## Step 3: Start the sample agent's authorization request

Open `http://localhost:9002/` in your browser. Select **Login as Local Client**.
This flow returns a broker-issued token. **Proxy Client** instead returns a token from the mock upstream authorization server.

The sample client redirects you to the broker's consent interface at `http://localhost:3000`.
The interface shows **Local Research Agent** and its requested permission sets.

## Step 4: Review and grant consent

Connect **Mock OAuth2 Service (Dev)** when the consent screen requests a provider session.
Complete the mock provider authorization in the browser. This stores the provider token in the broker, not the agent.

Review the mandatory and optional permission sets. Select the permissions to grant, then submit consent.
The broker redirects you to the sample client. Its **Access Token Claims** table describes the broker-issued token.

## Step 5: View the stored delegation

The admin API lists agent configuration, not user grants. Find the agent UUID there,
then read the grant from the authenticated end-user API through the development proxy:

```bash
AGENT_ID=$(curl --fail --silent http://localhost:14000/api/agents \
  | jq -er '.[] | select(.display_name == "Local Research Agent") | .id')
curl --fail-with-body "http://localhost:3000/api/consent/agents/$AGENT_ID/grants"
```

The response's `data` object contains the grant ID, `principal`, `agent_id`,
`granted_permission_sets`, and timestamps. The principal is `dev@example.com`.
A grant with an expiry also includes `valid_until`.
The response does not contain provider credentials.

## Step 6: Call a tool through the gateway

On the sample client page, select **Call MCP Tool: whoami (allowed)**.
The sample client sends your logged-in Local Client token through the configured gateway.
It handles MCP initialization and session headers automatically. You do not need to copy the token into a shell.

The result shows **Token Exchange Successful!**, the tool response, the gateway URL, and **JWT Claims**.
The tool response contains `auth_scheme=Bearer` and `jwt_claims=...` from the token visible to the MCP server.
Compare those provider claims with the **Access Token Claims** table on the same page.

| Claim | Broker token sent by the agent | Provider token received by the MCP server |
|---|---|---|
| `iss` | `http://localhost:3000` | `http://third-party-oauth2:9000` |
| `aud` | `token-exchange-broker` | `mcp-server` |
| `azp` | The Local Research Agent UUID | `mock-oauth2-client-dev` |

The gateway validates the broker JWT; ExtProc exchanges it for the stored provider token before forwarding the request.
The mock returns decoded claims for inspection, not the reusable provider token. Its claim decoder is not a signature verifier.

A 401 indicates a JWT validation problem, such as an expired token or the wrong client flow.
A session-recovery error means that the broker needs a usable provider session in addition to the grant.
Return to the consent UI to connect the mock provider, then retry.

## Step 7: Tear down

```bash
just compose-down
```

Without `just`:

```bash
docker compose --env-file .env.compose -f docker-compose.yml down
```

The in-memory store discards the seeded data, provider sessions, and grants when the broker stops.

## Where to go next

- **[Delegation and consent](/docs/concepts/delegation-and-consent)** — Principals, agents, permission sets, grants, and sessions.
- **[Manage agents and services](/docs/guides/manage-agents-and-services)** — Register your own services and agents.
- **[Gateway token exchange](/docs/guides/token-exchange-gateway)** — Configure the validation and exchange boundary.
- **[Deploy on Kubernetes](/docs/guides/deploy-on-kubernetes)** — Deploy beyond the local demo.
