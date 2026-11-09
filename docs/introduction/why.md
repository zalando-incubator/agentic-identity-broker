---
title: Why does this exist?
description: Why OAuth delegation alone is not enough when AI agents act for people, what is left after IT approves an agent, and where the problem shows up in a normal work day.
---

# Why does this exist?

This page explains the problem before the solution. It answers three questions that
people ask first:

- [Why is OAuth delegation not enough?](#why-is-oauth-delegation-not-enough)
- [If IT already approved the agent, what is left to solve?](#if-it-already-approved-the-agent-what-is-left-to-solve)
- [Is this only for enterprises?](#is-this-only-for-enterprises)

We start with an ordinary work day.

## A Monday with an AI assistant

Lena is a product manager. Her company gives every employee an AI assistant in Google Chat. Procurements reviewed the vendor and approved the assistant. The company also runs MCP servers for Linear, GitHub, and Gmail, so the assistant can use those tools.

At 9:00, Lena writes in chat:

> Summarize the open Linear tickets for my team, check which GitHub pull requests are
> still blocked, and send a status email to the launch stakeholders.

That is one sentence for Lena. For the systems behind it, it is a chain of delegated
access decisions:

| Step | The question that somebody must answer |
|---|---|
| The assistant reads Lena's Linear tickets | Is this the Linear access of *Lena*, and not of the last user or of a shared bot account? |
| The assistant reads GitHub pull requests | Does it get only the access it needs, or the broad `repo` scope that another agent on the same OAuth app asked for? |
| The assistant sends an email to people outside the company | Did Lena see and approve *this* email, or only "Gmail access" three months ago? |
| The request starts in Chat | Nobody is at a browser to click through an OAuth consent screen. Who proves that the request is from Lena? |
| Next month Lena changes teams | Can she remove the assistant's GitHub access, and keep the coding agent's GitHub access? |
| A security engineer investigates an email | Which agent sent it, for which person, and with which grant? |

Plain OAuth answers some of these questions. It does not answer all of them, and each
MCP server and each agent answers them in a different way.

## Why is OAuth delegation not enough?

The broker *is* OAuth. It uses authorization code with PKCE, refresh tokens, and RFC 8693
token exchange. The problem is not the protocol. The problem is where the OAuth decisions
happen and what they can express when the client is an AI agent.

**The OAuth client is not the agent.** A provider such as GitHub sees one OAuth app. One
app can sit behind many agents, many prompts, and many users. The provider cannot tell
the summarizing assistant from the coding agent. The broker records a separate grant for
each person and each agent.

**Provider scopes are coarse.** GitHub's `repo` scope gives read and write access to all
private repositories. Google scopes are similar for mail and files. When one OAuth app
serves many agents, it tends to ask for the union of what all of them need. In the
broker, an administrator defines
[permission sets](/docs/concepts/delegation-and-consent) in business terms, such as
*Read repositories*, and each agent requests only the sets it needs. The user approves
those sets, and a grant can expire, for example at the end of a project. A permission set
cannot be finer than the provider's scopes: if it contains `repo`, the provider token can
still write. When you need finer control, an optional
[OPA policy at the gateway](/docs/guides/token-exchange-gateway#optional-opa-gate) can
deny specific tool calls.

**Consent is scattered.** Without a broker, every MCP server runs its own authorization
server and its own token store. Lena consents three times for the assistant. When a team
adds a second agent, she consents three more times. No single page shows which agents can
act for her, and no single button removes one agent. The broker gives each user
[one consent interface](/docs/concepts/delegation-and-consent) for all agents and
services. It gives administrators one place to revoke an agent for all users. The gateway
can cache an exchanged token for a short, configurable time, so a revocation takes
effect when that cache entry ends. See the
[ExtProc token cache](/docs/reference/extproc-token-cache).

**OAuth consent is for a scope, not for an action.** After Lena approves Gmail access,
OAuth has no way to ask "send *this* email to *these* people?" A prompt injection in a
ticket can make the agent send a different email. Some agent products ask for
confirmation in their own interface. Other products do not, and you cannot enforce it
across products. The broker supports [tool approvals](/docs/api/approval-endpoints) at the
gateway. When an OPA policy marks a tool call as `approval_required`, the gateway holds
the call until the user approves it once, for the session, or permanently. This works
for every agent that goes through the gateway. You must
[enable approval gating](/docs/guides/token-exchange-gateway#enable-approval-gating)
together with OPA. Without that policy, the gateway does not ask for approval.

**Every MCP server must do the OAuth work.** An MCP server that calls a third-party
service must get, store, encrypt, and refresh that service's tokens for each user. With
many internal MCP servers, you operate many authorization flows, token stores, and
refresh loops. With the broker and a gateway, MCP servers only implement tools. The
gateway [exchanges the agent's token](/docs/concepts/token-exchange) for the correct
provider token and gives it to the MCP server. The provider token never reaches the agent.

## If IT already approved the agent, what is left to solve?

IT/ procurement approval is a one-time decision about software: *this vendor and this product are
allowed in our company.* It is important, and the broker does not replace it.

After that decision, the agent acts for thousands of people, thousands of times each day.
Each action has its own questions, and a vendor review cannot answer them:

- **For whom is the agent acting now?** An approved product does not tell a provider
  which employee started the request.
- **Which part of that person's access can it use?** Lena can merge pull requests. That
  does not mean that her assistant should merge pull requests.
- **Is this action acceptable?** An email to an external address, a shared document, or a
  deleted branch can need a human decision each time.
- **How do you take access back?** You want to remove one agent for one person, or one
  agent for everyone, without blocking the product for the company.
- **What happened afterward?** Audit needs the agent, the person, the grant, and the
  resource for each token exchange.

In short, procurement approval tells you *which agents may exist here*. The broker controls *what
each of those agents can do for each person, right now*.

## Is this only for enterprises?

The broker is for any organization that runs agents for many people. We use it in
production to connect hundreds of agents with dozens of internal and external MCP servers.
It also fits these cases:

- **Agent platforms and marketplaces.** Many agents from different teams act for many
  users. Users get one consent interface. Administrators get one place to revoke access.
- **Hosted agents.** The agent forwards one broker token through a gateway and does not
  collect a provider token for each MCP server.

If you are one person who connects one agent to one service, the OAuth flow of that
service is probably enough. The problems on this page grow with the number of agents,
users, and services. With three agents, ten services, and a thousand users, you have
thirty thousand possible delegations to govern.

## Where to go next

- **[Use cases](/docs/introduction/use-cases)** — more scenarios, each with the identity
  problem and how the broker solves it.
- **[Why not a traditional IdP?](/docs/introduction/why-not-idp)** — how the broker works
  with the identity provider that you already have.
- **[Get started](/docs/get-started)** — run the stack locally and follow one delegation
  from consent to token exchange.
