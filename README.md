<div align="center">
  <p>
    <img src="assets/docusaurus/static/img/logo.svg" alt="Agentic Identity Broker logo" width="128" height="128" />
  </p>
  <h1>Agentic Identity Broker</h1>
  <p>
    <a href="https://github.com/zalando-incubator/agentic-identity-broker/actions/workflows/ci.yml">
      <img src="https://img.shields.io/github/actions/workflow/status/zalando-incubator/agentic-identity-broker/ci.yml?label=CI&amp;style=flat-square" alt="CI status" />
    </a>
    <a href="LICENSE">
      <img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="MIT license" />
    </a>
  </p>
  <p>
    <a href="#overview">Overview</a> ·
    <a href="#status">Status</a> ·
    <a href="#use-cases">Use Cases</a> ·
    <a href="#quick-start">Quick Start</a> ·
    <a href="#architecture">Architecture</a> ·
    <a href="#documentation">Documentation</a> ·
    <a href="#contributing">Contributing</a>
  </p>
</div>

<br/>

<p align="center">
  <img src="assets/docusaurus/static/img/teaser-browser.webp" alt="Agentic Identity Broker consent interface" width="1000" />
</p>

<br/>

---

## Overview

This project captures delegation chains for on-behalf-of flows in agentic AI, brokers between different OAuth2 infrastructures, and implements a token vault. It is designed to be used by an infrastructure gateway (like [Agentgateway](https://agentgateway.dev)) in the call path between an agent and an MCP server or between agents.

It is designed to simplify both agent development and MCP server development and solve the hard authentication and authorization problems in agentic systems in one place.

From a user perspective, people can consent to delegating specific permissions in systems like Google, GitHub, Databricks, Linear, etc. to a given agent.

## Status

We have started to use this project in production internally for several MCP servers and have seen good potential. We are coding in the open to gauge our approach. If you find it useful, let us know. If you have ideas for other features, let us know via issues.

While this project was created by professionals with decades of experience in the identity space (including building multiple OIDC and OAuth2 providers), it was developed with a strong focus on agentic engineering and spec-driven development. We review every PR and prioritize sound engineering practices over convenience.

Major roadmap topics include: (a) centralized tool authorization with Open Policy Agent-based authorization policies, (b) support for on-behalf-of flows where no frontend user session exists (semi-autonomous cases with no user directly interacting with an agent), and (c) formalizing token exchange for agent-to-agent calls.

## Use Cases

### Internal MCP Servers

Frameworks like FastMCP can implement the whole OAuth2.1 ceremonies as mandated by the MCP spec but this means that you have to configure it for every MCP Server you deploy and maintain it. You also have to configure secure storage and basically run a multitude of OAuth2 authorization servers. With the identity broker and a gateway you can dumb down MCP server development to only provide tools on the target technologie you want. OAuth2 ceremonies, token vaulting and token translation are done transparently. Because token exchange is done centrally via a gateway, this allows agents to interact with hundreds of MCP servers via one channel.

### MCP Servers for SaaS

If you want to offer an MCP server to your customers as part of your SaaS, the identity broker can provide an additional consent surface that records which agents were actually used. With centralized tool authorization, you can keep an audit trail of approvals on your side regardless of which agent the customer runs.

### Hosted Agents

The agentic identity broker can simplify agents by requiring only a single user token that can be used with an arbitrary number of MCP servers. When paired with a portal for user interactions, token procurement can be front-loaded in the portal, and the agent's responsibility is to forward this token to upstream MCP servers via a gateway.

## Key Features

- **Secure Identity Management**: Agent registration, principal authentication, and session management
- **OAuth2 Delegation & Consent**: Manage OAuth2 scopes and user consent for third-party service integrations
- **Flexible Configuration**: Multi-source configuration (defaults, .env files, YAML, CLI flags) with environment-specific support
- **Security-First Design**: Sensitive value redaction, command injection prevention, cryptographic validation, and fail-closed architecture
- **Dual-Port Architecture**: Separate end-user (8000) and administrative (14000) ports with distinct API contracts
- **Persistence Layer**: PostgreSQL + in-memory storage with database migrations and schema versioning
- **React Consent Frontend**: Modern SPA for managing OAuth2 delegations with design system compliance
- **Comprehensive Audit Logging**: Structured JSON logging for compliance and security monitoring
- **Hexagonal Architecture**: Clear separation of domain logic, ports (interfaces), and adapters (implementations)

## Prerequisites

- **Go 1.26.8** or higher (required)
- **PostgreSQL 12+** (for persistence; in-memory storage available for development)
- **Node.js 24+** (for React frontend development, optional)
- **[just](https://github.com/casey/just)** command runner (recommended)
- **[Air](https://github.com/air-verse/air)** for hot-reload development (optional, recommended)
- **[golangci-lint](https://golangci-lint.run/)** for code quality checks (optional)

## Quick Start

### 1. Clone & Setup

```bash
git clone https://github.com/agentic-identity-broker/agentic-identity-broker.git
cd agentic-identity-broker

# Install Go dependencies
just deps

# Install optional dev tools (Air, golangci-lint)
just install-tools
```

### 2. Development Server (Recommended)

Start with hot-reload development:

```bash
# Terminal 1: Start backend with auto-rebuild
just dev

# Terminal 2 (optional): Start React frontend with HMR
just web-dev
```

- Backend: http://localhost:8000
- Frontend: http://localhost:3000 (with API proxy to backend)
- Admin API: http://localhost:14000

### 3. Docker Compose Development (Complete Stack)

For a fully orchestrated development environment with all services (broker, mock OAuth2 servers, mock agent, and frontend):

```bash
# Start all services with docker-compose (auto-seeds sample data)
just compose-up-detached

# View service logs
just compose-logs

# When ready, stop all services
just compose-down
```

This starts the full stack: broker (ports 8000/14000), frontend (3000), and three mock services (9000/9001/9002). All services auto-seed sample data on first startup, and the backend auto-rebuilds on code changes using Air. Seed data is automatically re-generated after each rebuild to test the flow end-to-end.

**Service URLs:**
- Broker (end-user): http://localhost:8000
- Broker (admin): http://localhost:14000
- Frontend (consent UI): http://localhost:3000
- Sample OAuth2 client: http://localhost:9002/oauth2/authorize

See [docs/docker-compose-setup.md](docs/docker-compose-setup.md) for advanced docker-compose usage and troubleshooting.

**Configuration:** Docker Compose automatically uses `config.docker.yaml` (with container DNS names), while native development (`just run`, `just dev`) uses `config.yaml` (with localhost addresses).

## Architecture

The project follows **hexagonal architecture** (ports and adapters pattern) with clear separation between:

- **Domain Logic** (`internal/domain/`): Core business logic independent of external concerns
- **Ports** (`internal/ports/`): Interfaces defining boundaries and contracts between layers
- **Adapters** (`internal/adapters/`): Infrastructure implementations (HTTP handlers, database queries, storage)
- **Configuration**: Security-first multi-source configuration management
- **Session Management**: Principal extraction middleware for authentication

### Key Architectural Components

1. **Flexible Configuration System**: Supports .env, YAML, CLI flags, and environment variable substitution with security validation
2. **Dual-Port Server**: End-user API (port 8000) and administrative API (port 14000)
3. **Persistence Layer**: PostgreSQL with migrations + in-memory storage fallback
4. **Domain Model**: Agent, OAuth2 service, grant, and consent management
5. **React Consent Frontend**: Managed via design system with accessibility compliance

For detailed documentation, see [ARCHITECTURE.md](ARCHITECTURE.md).

## Security

The application implements security-first design principles:

- **Sensitive Value Redaction**: Automatic redaction of passwords, tokens, and secrets in logs
- **Command Injection Prevention**: Validation to prevent shell command injection
- **Circular Reference Detection**: Protection against infinite loops in configuration
- **Fail-Closed**: Graceful termination on configuration errors
- **Audit Logging**: Structured JSON audit logs for compliance

For security concerns, please see our security policy.

## Contributing

We welcome contributions! Please follow these governance-compliant guidelines:

1. **Review Constitution**: Read [.specify/memory/constitution.md](.specify/memory/constitution.md) for binding principles
2. **Fork the repository**
3. **Create a feature branch** (`git checkout -b feature/amazing-feature`)
4. **Make your changes**:
   - For APIs: Document in OpenAPI first, get user confirmation for design changes
   - For backend: Follow hexagonal architecture patterns
   - For frontend: Use design system components from `web/src/design-system/`
5. **Run static checks and the verification gate** (`just check` and `just verify`) - both must pass
6. **Update architecture docs** if your changes affect system design ([ARCHITECTURE.md](ARCHITECTURE.md))
7. **Commit your changes** with clear, descriptive messages
8. **Push to your branch** (`git push origin feature/amazing-feature`)
9. **Open a Pull Request** with a clear description of changes and rationale

### Contribution Rules

- **Security-First**: Never disable security controls. If security is an issue, escalate rather than bypass.
- **API-First**: Document APIs in OpenAPI format before implementation
- **Tests Required**: New features must include tests. Run `just verify` before opening a PR.
- **No Architecture Deviations**: Follow established ADRs (Architecture Decision Records). Deviations require new ADRs.
- **Database Changes**: Use go-migrate naming conventions for all migrations
- **Design System**: All frontend components must use the design system (web/src/design-system/)

For security concerns, please refer to the project's security policy.

## Documentation

- [Architecture Overview](ARCHITECTURE.md) - System architecture and design decisions
- [User Documentation](docs/) - End-user guides and tutorials
- [API Documentation](docs/api/) - REST API reference
- [Documentation Site](assets/docusaurus/) - Full documentation website

Run the documentation server locally:
```bash
just docs-serve
```
