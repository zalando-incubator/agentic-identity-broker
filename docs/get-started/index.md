---
title: "Get started"
description: Run the Agentic Identity Broker stack locally with Docker Compose. Complete one delegation from the sample agent to the consent UI and stored grant.
---

# Get started

This tutorial starts with a fresh clone and ends with a working delegation. You run the
broker stack locally. Then you start a sample agent authorization request, give consent in
the browser, and view the stored grant through the admin API.

Docker Compose runs every service in one stack. The stack seeds sample data automatically.
You can complete the flow before you register agents or services.

## Before you start

You need:

- **Docker**, with Docker Compose v2 (`docker compose`). Docker Compose runs the local
  services.
- **[just](https://github.com/casey/just)**. The project uses this command runner for its
  workflows.
- **The repository**, cloned locally:

  ```bash
  git clone https://github.com/zalando-incubator/agentic-identity-broker.git
  cd agentic-identity-broker
  ```

You do not need Go, Node.js, PostgreSQL, or any cloud credentials for this tutorial. The
stack uses an in-memory store and a local encryption key generated for you at startup.

## Step 1: Start the stack

From the repository root, bring the whole stack up in the background:

```bash
just compose-up-detached
```

This command builds and starts every service for the flow. It seeds sample agents,
services, and permission sets after the broker reports healthy. The command then prints a
health summary and the service URLs:

```text
Service URLs:
  - Broker (end-user): http://localhost:8000
  - Broker (admin): http://localhost:14000
  - Frontend (consent UI): http://localhost:3000
  - Sample OAuth2 client: http://localhost:9002/oauth2/authorize
```

The stack is:

| Service | URL | Role |
|---|---|---|
| Broker — end-user API | http://localhost:8000 | Consent, third-party sessions, OAuth2 server surface |
| Broker — admin API | http://localhost:14000 | Agents, services, permission sets, credentials |
| Consent UI | http://localhost:3000 | The React screen where you approve delegations |
| Mock OAuth2 services | http://localhost:9000, `:9001`, `:9002` | A mock third-party provider, a mock upstream authorization server, and a sample agent/client |

The summary shows each broker port as healthy. If a broker port is in use, stop the
process that holds it. Then run the command again.

:::note
During local development, the consent UI dev server adds
`X-Remote-User: dev@example.com` to each proxied request. This simulates the
authenticating reverse proxy in production. The broker does not authenticate users. It
trusts the principal asserted by the proxy.
:::

## Step 2: Review the seeded data

The stack seeds sample agents and services. List them through the admin API:

```bash
curl http://localhost:14000/api/agents
```

The response shows a JSON array of sample agents, such as a weather assistant and a task
manager. You can grant these agents access without registering them first.

## Step 3: Start the sample agent's authorization request

Open the sample OAuth2 client in your browser:

```text
http://localhost:9002/oauth2/authorize
```

The sample client acts like an agent that requests access for you. It sends an OAuth2
authorization request to the broker on port 8000. The broker redirects you to the consent
interface because the agent does not yet have a grant.

The browser opens the consent interface at `http://localhost:3000`. It shows the agent
that requests access.

## Step 4: Review and grant consent

The focused consent view identifies the agent and its requested **permission sets**.
Required groups stay selected. Optional groups remain editable. Each group explains access with a name, description, and service names.

1. Select the optional groups that you want to grant.
2. If a selected service needs a connection, click **Connect** and complete its provider authorization.
3. Choose **Until revoked**, **30 days**, or a valid **Custom date**.
4. Click **Allow** to save the grant and resume the sample client's authorization request.

The provider callback preserves your selections and duration. The sample client receives broker-issued, scoped access, not a third-party credential.
**Deny** leaves existing grants unchanged and shows a local result without a redirect.

The console starts at `http://localhost:3000/delegations` in this development stack.
Agents shows unexpired grants. Connections (`/sessions`) shows stored provider sessions. Approvals (`/approvals`) shows pending requests before standing decisions.
The agent detail (`/agents/:id` without `session_token`) saves only deliberate edits. Settings (`/settings`) changes only this browser's theme.

## Step 5: View the stored delegation

You can list the agents again through the admin API. You can also view broker activity in
the logs:

```bash
just compose-logs
```

The streamed logs show the consent request and grant creation. Press `Ctrl+C` to stop the
log stream. The stack continues to run.

## Step 6: Tear down

When you are done, stop the whole stack:

```bash
just compose-down
```

All containers stop. This tutorial uses the in-memory store. The stack discards the seeded
data and your grant. The next `just compose-up-detached` starts with new data.

## What you did

You completed a full local delegation:

- You started the end-user broker on 8000 and admin broker on 14000. You also started the
  consent interface on 3000 and the mock OAuth2 services. The stack seeded sample data.
- You started an agent authorization request from the sample client.
- You reviewed permission sets and gave consent in the browser. The broker accepted the
  proxy-supplied `X-Remote-User` principal.
- You viewed the stored grant. Then you stopped the stack.

## Where to go next

- **[Delegation and consent](/docs/concepts/delegation-and-consent)** — Learn about
  principals, agents, permission sets, grants, and sessions.
- **[Manage agents and services](/docs/guides/manage-agents-and-services)** — Register
  services, permission sets, and agents through the admin API.
- **[Deploy on Kubernetes](/docs/guides/deploy-on-kubernetes)** — Deploy the broker beyond
  the local stack.
