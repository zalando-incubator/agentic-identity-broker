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

The consent interface shows the requested **permission sets**. Each set is **mandatory**
or **optional**. The sets group provider scopes into business-readable capabilities.

Select the permission sets to grant. You can set an expiry. Then submit the request.

The consent interface shows the grant. The broker resumes the authorization request and
redirects you to the sample client. The sample client then holds broker-issued, scoped
access. It does not hold a third-party credential.

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

- **[Delegation and consent](../concepts/delegation-and-consent.md)** — Learn about
  principals, agents, permission sets, grants, and sessions.
- **[Manage agents and services](../guides/manage-agents-and-services.md)** — Register
  services, permission sets, and agents through the admin API.
- **[Deploy on Kubernetes](../guides/deploy-on-kubernetes.md)** — Deploy the broker beyond
  the local stack.
