<div align="center">
  <p>
    <img src="assets/docusaurus/static/img/logo.svg" alt="Agentic Identity Broker logo" width="128" height="128" />
  </p>
  <h1>Agentic Identity Broker</h1>
  <p><strong>Open-source OAuth2 broker for secure, user-controlled AI agent delegation.</strong></p>
  <p>
    <a href="https://github.com/zalando-incubator/agentic-identity-broker/actions/workflows/ci.yml">
      <img src="https://img.shields.io/github/actions/workflow/status/zalando-incubator/agentic-identity-broker/ci.yml?label=CI&style=flat-square" alt="CI status" />
    </a>
    <a href="LICENSE">
      <img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="MIT license" />
    </a>
  </p>
  <p>
    <a href="#what-it-does">What it does</a> ·
    <a href="#quick-start">Quick start</a> ·
    <a href="#architecture">Architecture</a> ·
    <a href="https://agenticidentitybroker.dev">Docs</a> ·
    <a href="#contributing">Contributing</a>
  </p>
</div>

---

The Agentic Identity Broker gives users a central way to grant AI agents scoped, revocable access to third-party services. It keeps provider credentials encrypted at rest and gives agents scoped access through OAuth2 token exchange.

## What it does


The broker is the trust boundary between users, agents, and third-party services. A user grants a permission set to a specific agent for a specific service. Grants can expire or be revoked.

The broker stores OAuth2 tokens and provider credentials. Agents do not receive these credentials. At request time, a gateway can exchange an agent token for the necessary service token with [RFC 8693 token exchange](docs/concepts/token-exchange.md).

### Why use a broker?

- **Least privilege** — Scope delegated access to the agent, service, and permission set.
- **User control** — Create, expire, and revoke grants from the consent interface.
- **Credential isolation** — Keep long-lived provider tokens out of agents and MCP servers.
- **Central enforcement** — Apply consent, policy, and audit controls in one place.
- **Standards-based integration** — Use OAuth2 authorization code flow, PKCE, discovery metadata, JWKS, and token exchange.

### What it is not

The broker does not authenticate people. A trusted reverse proxy authenticates the user and sends the principal in a configured header. Continue to use your existing identity provider for human login.

The broker is not a general secrets manager. It stores OAuth2 delegations and their related credentials.

## Status

The project is under active development and is used internally with MCP servers. We publish it to collect feedback on the delegation model, supported integrations, and operational requirements.

## Quick start

Run the complete local flow with Docker Compose. The stack uses an in-memory store, generates local encryption keys, and seeds sample data.

**Requirements:** [Docker](https://www.docker.com/) with Docker Compose v2 and [just](https://github.com/casey/just).

```bash
git clone https://github.com/zalando-incubator/agentic-identity-broker.git
cd agentic-identity-broker
just compose-up-detached
```

Open [the sample agent](http://localhost:9002/oauth2/authorize). It starts an authorization request and redirects to the consent interface at [localhost:3000](http://localhost:3000).

| Service | URL | Purpose |
| --- | --- | --- |
| Consent UI | http://localhost:3000 | Review and approve delegated access |
| End-user API | http://localhost:8000 | Consent, OAuth2, and token exchange |
| Admin API | http://localhost:14000 | Manage agents, services, and permission sets |
| Sample agent | http://localhost:9002/oauth2/authorize | Start the local authorization flow |

When you finish, stop the stack:

```bash
just compose-down
```

Read the [getting started guide](docs/get-started/index.md) for the complete delegation walkthrough.

## Features

- **Delegation and consent** — Users grant an agent defined permissions for a third-party service.
- **OAuth2 authorization server** — Supports authorization code flow, PKCE, discovery metadata, and JWKS.
- **Token exchange** — Issues scoped service access through RFC 8693 token exchange.
- **Encrypted token vault** — Encrypts third-party tokens and service credentials at rest.
- **Gateway integration** — Works with infrastructure gateways, such as [Agentgateway](https://agentgateway.dev), on the path between agents and MCP servers.
- **Dual server surfaces** — Separates the end-user API on port 8000 from the administrative API on port 14000.
- **Consent frontend** — Provides a React interface for grant review, approval, expiry, and revocation.
- **Policy and audit controls** — Centralizes authorization decisions and structured audit logging.

## Architecture

The codebase follows hexagonal architecture:

| Layer | Responsibility |
| --- | --- |
| `internal/domain/` | Business rules and domain models |
| `internal/ports/` | Interfaces at application boundaries |
| `internal/adapters/` | HTTP, storage, encryption, and external integrations |
| `internal/app/` | Application wiring through the builder |
| `web/` | React consent single-page application |
| `infra/` | AWS CDK and deployment infrastructure |

The end-user API manages consent, third-party sessions, and the OAuth2 authorization-server surface. The admin API manages agents, services, permission sets, and related credentials.

Read [the architecture overview](ARCHITECTURE.md) for the system design and [the architecture concept](docs/concepts/architecture.md) for the operator view.

## Develop from source

For native development, install the following tools:

- Go 1.26.8 or later
- Node.js 24 or later for the consent frontend
- PostgreSQL 12 or later when you use PostgreSQL storage
- [just](https://github.com/casey/just)
- [Air](https://github.com/air-verse/air) for backend hot reload (optional)

Install dependencies and development tools:

```bash
just deps
just install-tools
```

Run the backend and frontend in separate terminals:

```bash
# Terminal 1
just dev

# Terminal 2
just web-dev
```

`just dev` starts the backend with hot reload. `just web-dev` starts Vite with hot module replacement and proxies requests to the backend.

### Useful commands

| Command | Purpose |
| --- | --- |
| `just build` | Build the broker binary |
| `just build-all` | Build the broker and consent frontend |
| `just test` | Run the fast Go package tests |
| `just test-integration` | Run self-contained integration tests |
| `just test-e2e` | Run backend, ExtProc, and frontend acceptance tests |
| `just test-e2e-performance` | Run the separate performance-labelled E2E measurement |
| `just check` | Run formatting, vetting, and linting |
| `just verify` | Run the full local verification gate |
| `just --list` | Show all project commands |

## Configuration and deployment

The broker reads configuration in this precedence order:

1. Built-in defaults
2. `.env` files
3. YAML configuration
4. Command-line flags

Use [`config.yaml`](config.yaml) as the local-development reference. The configuration supports environment variable substitution. Do not store credentials in a configuration file committed to source control.

For production guidance, read the [configuration reference](docs/configuration.md), [Kubernetes deployment guide](docs/deployment/kubernetes.md), and [encryption integration guide](docs/ENCRYPTION_INTEGRATION_GUIDE.md).

## Documentation

- [Documentation site](https://agenticidentitybroker.dev) — Browse the published guides and reference material.
- [Get started](docs/get-started/index.md) — Run a local delegation from the sample agent to the consent UI.
- [Concepts](docs/concepts/index.md) — Learn about delegation, consent, OAuth2 modes, token exchange, encryption, and architecture.
- [Use cases](docs/introduction/use-cases.md) — Review common deployment scenarios.
- [Token exchange reference](docs/reference/token-exchange.md) — Integrate gateway token exchange.
- [End-user OpenAPI contract](api/enduser/openapi.yaml) — Consent and OAuth2 API.
- [Admin OpenAPI contract](api/admin/openapi.yaml) — Administrative API.
- [Configuration reference](docs/configuration.md) — Configure the broker and its integrations.
- [Documentation site source](assets/docusaurus/) — Build the full documentation site with `just docs-serve`.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) before you open a pull request. Contributions must follow the project's API-first, security-first, and specification-driven workflow.

Before you open a pull request, run:

```bash
just check
just verify
```

Read the [project constitution](.specify/memory/constitution.md) and [architecture decisions](adrs/) when a change affects an API, security boundary, or architecture.

## Security

Do not report security vulnerabilities in public issues. Follow the [security policy](SECURITY.md) to report them responsibly.

## License

This project is available under the [MIT License](LICENSE). Copyright © 2026 Zalando SE.
