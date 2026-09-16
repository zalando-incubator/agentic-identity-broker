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
    <a href="#configuration">Configuration</a> ·
    <a href="#documentation">Documentation</a> ·
    <a href="#contributing">Contributing</a>
  </p>
</div>

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

### 3. Production Build

```bash
# Build optimized binary (30% smaller)
just build-release

# Or build both backend and frontend
just build-all
```

The binary at `./bin/agentic-identity-broker` is ready for deployment.

### 4. Docker Compose Development (Complete Stack)

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

### 5. Run Tests & Quality Checks

```bash
# Run the fast Go/package test loop
just test

# Run all integration suites
just test-integration

# Run all E2E acceptance suites
just test-e2e

# Run the dedicated SC-001 E2E performance measurement
just test-e2e-performance

# Run static quality checks (format, vet, lint)
just check

# Run the full verification gate with E2E last
just verify
```

## Configuration

The application supports multiple configuration sources with the following precedence (lowest to highest):

1. **Defaults**: Built-in default values
2. **.env Files**: Environment-specific files (`.env` → `.env.local` → `.env.{GO_ENV}` → `.env.{GO_ENV}.local`)
3. **YAML File**: `config.yaml` with `${VAR}` environment variable substitution
4. **CLI Flags**: Command-line flags override all other sources

### Configuration Files

Create a `.env` file for local development:
```env
IDENTITY_BROKER_LOG_LEVEL=debug
IDENTITY_BROKER_LOG_FORMAT=json
```

Or use a `config.yaml` file:
```yaml
log:
  level: debug
  format: json
```

See [examples/](examples/) directory for more configuration examples.

## Development Commands

All development tasks are managed using [just](https://github.com/casey/just). Run `just --list` to see all available commands:

### Building & Running
- `just build` - Build the binary to ./bin/agentic-identity-broker
- `just build-release` - Build optimized binary for production (30% smaller)
- `just run` - Build and run the application
- `just dev` - Start hot-reload development server
- `just clean` - Remove build artifacts

### Testing & Quality
- `just test` - Run the fast Go/package test loop (no E2E or integration suites)
- `just test-coverage` - Generate HTML coverage report for the fast Go/package suite
- `just test-coverage-summary` - Display coverage summary for the fast Go/package suite
- `just test-integration` - Run the default self-contained integration suites
- `just test-integration-infra` - Run infra-backed integration suites only (Docker required)
- `just test-integration-all` - Run both integration layers
- `just test-e2e` - Run all backend, ExtProc, and frontend E2E suites
- `just test-e2e-performance` - Run only performance-labelled E2E measurements; normal E2E commands exclude them
- `just verify` - Run the full verification gate with E2E last
- `just check` - Run static quality checks only (fmt, vet, lint)

### Code Quality
- `just fmt` - Format code with gofmt
- `just vet` - Run go vet static analysis
- `just lint` - Run golangci-lint (with fallback to go vet)

### Dependencies & Tools
- `just deps` - Manage Go module dependencies
- `just install-tools` - Install Air and golangci-lint

### Documentation
- `just docs-serve` - Start documentation server locally
- `just docs-build` - Build documentation site
- `just docs-deploy` - Deploy documentation to GitHub Pages

## Testing

Run the fast Go/package test loop:
```bash
just test
```

Run all integration suites:
```bash
just test-integration
```

Run all E2E acceptance suites:
```bash
just test-e2e
```

Run the full verification gate:
```bash
just verify
```

Generate coverage for the fast Go/package suite:
```bash
just test-coverage
```

The coverage report will be generated at `coverage/coverage.html`.

## Code Quality

Format code:
```bash
just fmt
```

Run static analysis:
```bash
just vet
```

Run linter:
```bash
just lint
```

Run all quality checks:
```bash
just check
```

## Project Governance

This project uses **constitution-driven development** with binding governance principles enforced across all contributions:

- **API-First Development**: All APIs documented in OpenAPI before implementation
- **Security-First**: Security controls enabled by default, never optional, always fail-closed
- **Architecture Decision Records**: Decisions recorded as ADRs; accepted ADRs are binding
- **Domain-Driven Design**: Clear ubiquitous language and glossary maintenance
- **Design System Compliance**: Frontend components use the design system (React, Tailwind CSS)
- **Database Migrations**: All schema changes use go-migrate naming conventions
- **Constitution v1.4.0**: Full governance rules in [.specify/memory/constitution.md](.specify/memory/constitution.md)

See [Contributing](#contributing) for governance-compliant contribution guidelines.

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

## Project Structure

```
.
├── cmd/                         # Application entry points
│   └── agentic-identity-broker/ # Main application binary
├── internal/                    # Private application code (hexagonal architecture)
│   ├── adapters/                # Infrastructure adapters
│   │   ├── http/                # HTTP handlers for end-user & admin APIs
│   │   ├── config/              # Configuration loading & validation
│   │   └── storage/             # Storage implementations (PostgreSQL, in-memory)
│   ├── domain/                  # Domain models and business logic
│   │   ├── agent/               # Agent management domain
│   │   ├── consent/             # Consent management domain
│   │   ├── config/              # Configuration domain (LogLevel, etc)
│   │   ├── oauth2/              # OAuth2 service integration domain
│   │   ├── principal/           # User authentication domain
│   │   ├── server/              # Server configuration domain
│   │   └── storage/             # Storage domain models
│   └── ports/                   # Port interfaces (hexagonal boundaries)
│       ├── config.go            # Configuration port
│       ├── storage.go           # Storage port
│       └── [more ports]
├── web/                         # React consent frontend (SPA)
│   ├── src/
│   │   ├── components/          # React components
│   │   │   ├── consent/         # Consent-specific components
│   │   │   ├── layout/          # Layout components
│   │   │   ├── ui/              # Reusable UI components
│   │   │   └── design-system/   # Design system components & tokens
│   │   ├── pages/               # Application pages
│   │   ├── hooks/               # Custom React hooks
│   │   ├── services/            # API client and services
│   │   ├── types/               # TypeScript type definitions
│   │   └── utils/               # Utility functions
│   ├── dist/                    # Built frontend assets (served by Go)
│   └── package.json             # Dependencies and scripts
├── migrations/                  # Database schema migrations (go-migrate)
├── test/                        # Integration tests
├── specs/                       # Feature specifications (numbered directories)
│   ├── 001-end-user-docs/      # Documentation feature
│   ├── 002-flexible-configuration/
│   ├── 003-dual-port-server/
│   ├── 004-persistence-layer/
│   ├── 005-session-management/
│   ├── 006-domain-model-apis/
│   └── 007-consent-frontend/
├── .specify/                    # Project constitution & templates
│   ├── memory/
│   │   └── constitution.md      # Binding governance principles (v1.4.0)
│   ├── templates/               # Feature specification templates
│   └── scripts/                 # Automation scripts
├── docs/                        # User documentation
│   ├── api/                     # REST API documentation & examples
│   └── [guides, tutorials]
├── examples/                    # Configuration examples
├── ARCHITECTURE.md              # Detailed architecture documentation
├── README.md                    # This file
└── justfile                     # Task automation
```

## Security

The application implements security-first design principles:

- **Sensitive Value Redaction**: Automatic redaction of passwords, tokens, and secrets in logs
- **Command Injection Prevention**: Validation to prevent shell command injection
- **Circular Reference Detection**: Protection against infinite loops in configuration
- **Fail-Closed**: Graceful termination on configuration errors
- **Audit Logging**: Structured JSON audit logs for compliance

For security concerns, please see our security policy.

## Development Workflow

This project enforces governance-compliant development using the justfile and specification system:

### All Development Commands

Run `just --list` to see all available commands:

**Building & Running**:
- `just build` - Build binary to ./bin/agentic-identity-broker
- `just build-release` - Production-optimized build (30% smaller)
- `just run` - Build and run the application
- `just dev` - Start hot-reload development server (Air)
- `just clean` - Remove build artifacts

**Testing & Quality**:
- `just test` - Run the fast Go/package test loop (no E2E or integration suites)
- `just test-coverage` - Generate HTML coverage report for the fast Go/package suite (coverage/coverage.html)
- `just test-coverage-summary` - Display coverage summary for the fast Go/package suite
- `just test-integration` - Run the default self-contained integration suites
- `just test-integration-infra` - Run infra-backed integration suites only (Docker required)
- `just test-integration-all` - Run both integration layers
- `just test-e2e` - Run all backend, ExtProc, and frontend E2E suites
- `just test-e2e-performance` - Run only performance-labelled E2E measurements; normal E2E commands exclude them
- `just verify` - Run the full verification gate with E2E last
- `just check` - Run static quality checks only: fmt, vet, lint

**Code Quality**:
- `just fmt` - Format code with gofmt
- `just vet` - Run go vet static analysis
- `just lint` - Run golangci-lint

**Frontend**:
- `just web-install` - Install npm dependencies
- `just web-dev` - Start Vite dev server with HMR (port 3000)
- `just web-build` - Build production bundle to web/dist/
- `just build-all` - Build both backend and frontend

**Documentation**:
- `just docs-serve` - Start Docusaurus documentation server locally
- `just docs-build` - Build documentation site
- `just docs-deploy` - Deploy to GitHub Pages

**Dependencies**:
- `just deps` - Run go mod tidy and verify
- `just install-tools` - Install Air and golangci-lint

### Pre-Commit Checklist

Before committing any code, always run:

```bash
just check
just verify
```

This ensures:
1. Code is properly formatted (`gofmt`)
2. No obvious bugs are detected (`go vet`)
3. Code quality standards are met (`golangci-lint`)
4. The full verification gate passes, with E2E as the final guard layer

### Feature Development with Specifications

For new features, follow the specification-driven workflow:

1. Create a feature spec in `specs/NNN-feature-name/spec.md`
2. Document the design in `specs/NNN-feature-name/plan.md`
3. Define tasks in `specs/NNN-feature-name/tasks.md`
4. All API changes must have OpenAPI specifications before implementation
5. Ensure architecture decisions are reflected in [ARCHITECTURE.md](ARCHITECTURE.md)
6. Run full quality checks before PR submission

See `.specify/templates/` for specification templates.

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

## Consent Frontend Development

The consent frontend is a React-based Single Page Application (SPA) for managing OAuth2 delegations to AI agents.

### Prerequisites

- Node.js 24.0.0 or higher
- npm (comes with Node.js)

### Setup

1. Navigate to the web directory:
```bash
cd web
```

2. Install dependencies:
```bash
npm install
```

3. Configure environment (optional):
```bash
# Create .env file for local development
cp .env.example .env
```

### Development Workflow

There are several ways to develop the consent frontend depending on your needs.

#### Quick Start with Justfile (Recommended)

The easiest way to run both backend and frontend:

```bash
# Terminal 1: Start Go backend
just run

# Terminal 2: Start frontend dev server with hot reload
just web-dev
```

Access the frontend at http://localhost:3000 (Vite dev server with HMR)
API requests automatically proxy to http://localhost:8000 (Go backend)

**How the Vite Proxy Works:**

The Vite dev server is configured to proxy API requests from the frontend to the backend:

```
Browser Request: http://localhost:3000/api/consent/agents
       ↓
Vite Dev Server (port 3000)
       ↓ (proxy configuration in vite.config.ts)
       ↓ Automatically injects: X-Remote-User: dev@example.com
Go Backend (port 8000)
       ↓
API Handler returns JSON
       ↓
Vite proxies response back to browser
```

**Authentication During Development:**
The Vite proxy automatically adds the `X-Remote-User: dev@example.com` header to all API requests. This simulates the authentication header that would normally be set by an upstream proxy (oauth2-proxy, nginx, etc.) in production. All API requests appear as if they're coming from the authenticated user `dev@example.com`.

This setup provides:
- **Hot Module Replacement (HMR)**: Instant updates without page reload
- **Same-origin requests**: No CORS issues during development
- **API debugging**: See requests in Go backend logs
- **Frontend debugging**: Use React DevTools in browser
- **Simulated authentication**: X-Remote-User header automatically injected for local testing

**All Frontend Commands:**

```bash
just web-install          # Install npm dependencies
just web-dev              # Start Vite dev server (port 3000)
just web-build            # Build production bundle to web/dist/
just build-all            # Build both Go backend and frontend
```

#### Alternative: Integrated Build (No HMR)

Build frontend and run from Go backend:
```bash
cd web && npm run build && cd .. && just run
```

Access at http://localhost:8000/ (no hot reload, requires rebuild for changes)

#### Frontend-Only Development

**Start Development Server:**
```bash
cd web && npm run dev
# or from project root:
just web-dev
```
This starts the Vite dev server at http://localhost:3000 with hot module replacement.

**Run Tests:**
```bash
npm run test              # Run tests in watch mode
npm run test:coverage     # Generate coverage report
```

**Lint and Format:**
```bash
npm run lint              # Run ESLint
npm run format            # Format code with Prettier
```

**Build for Production:**
```bash
npm run build
# or from project root:
just web-build
```
This compiles TypeScript and bundles the app to `web/dist/` directory.

**Preview Production Build:**
```bash
npm run preview
```

### Integration with Go Backend

The frontend is served by the Go backend at `/`:

1. **Build the frontend**: `just web-build` (creates `web/dist/`)
2. **Start the backend**: `just run` (from project root)
3. **Access the app**: http://localhost:8000/

The backend serves static files from `web/dist/` and handles API requests at `/api/consent/*`.

### Environment Variables

Frontend configuration (optional `.env` file in `web/` directory):

```bash
# API base URL (defaults to /api)
VITE_API_BASE_URL=/api

# Development server port (defaults to 3000)
VITE_DEV_SERVER_PORT=3000
```

### Project Structure

```
web/
├── src/
│   ├── components/       # React components
│   │   ├── consent/      # Consent-specific components
│   │   ├── layout/       # Layout components
│   │   └── ui/           # Reusable UI components
│   ├── pages/            # Application pages
│   ├── hooks/            # Custom React hooks
│   ├── services/         # API client and services
│   ├── types/            # TypeScript type definitions
│   └── utils/            # Utility functions
├── dist/                 # Build output (served by Go backend)
├── package.json          # Dependencies and scripts
├── vite.config.ts        # Vite configuration
├── tsconfig.json         # TypeScript solution references
├── tsconfig.app.json     # App TypeScript configuration
├── tsconfig.test.json    # Test TypeScript configuration
├── tsconfig.build.json   # Build TypeScript configuration
└── tailwind.config.ts    # Tailwind CSS configuration
```

### Technology Stack

- **React 18.2+**: Frontend framework
- **TypeScript 5.3+**: Type-safe JavaScript
- **Vite 5.0+**: Fast build tool with HMR
- **Tailwind CSS v4.0**: Utility-first CSS framework
- **Headless UI**: Accessible UI components
- **Axios**: HTTP client
- **React Router DOM**: Client-side routing
- **Vitest**: Unit testing framework

### Testing

**Run Tests:**
```bash
npm run test
```

**Coverage Report:**
```bash
npm run test:coverage
open coverage/index.html
```

**Test Structure:**
- Unit tests: `*.test.tsx` or `*.test.ts`
- Integration tests: `*.integration.test.tsx`
- Test files located next to source files

### Deployment

**Build for Production:**
```bash
npm run build
```

Output: `web/dist/` directory with optimized static files.

**Deploy with Go Backend:**
1. Build frontend: `cd web && npm run build`
2. Build Go binary: `just build-release`
3. Deploy `bin/agentic-identity-broker` with `web/dist/`

The Go backend automatically serves the SPA from `web/dist/`.

### Troubleshooting

**Port Already in Use:**
```bash
# Change port in vite.config.ts or use environment variable
VITE_DEV_SERVER_PORT=3001 npm run dev
```

**API Connection Issues:**
- Verify Go backend is running on port 8000
- Check Vite proxy configuration in `vite.config.ts`
- Ensure CORS is configured correctly

**Build Errors:**
```bash
# Clear node_modules and reinstall
rm -rf node_modules package-lock.json
npm install
```

**TypeScript Errors:**
```bash
# Regenerate TypeScript types
npm run build
```

## Documentation

- [Architecture Overview](ARCHITECTURE.md) - System architecture and design decisions
- [User Documentation](docs/) - End-user guides and tutorials
- [API Documentation](docs/api/) - REST API reference
- [Documentation Site](assets/docusaurus/) - Full documentation website

Run the documentation server locally:
```bash
just docs-serve
```

## Resources

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - Detailed system architecture and design decisions
- **[Constitution v1.4.0](.specify/memory/constitution.md)** - Binding governance principles
- **[User Documentation](docs/)** - End-user guides, tutorials, and API documentation
- **[API Documentation](docs/api/)** - REST API reference with examples
- **[Feature Specifications](specs/)** - Numbered feature specs with design artifacts

## Support & Feedback

- **Issues**: Report bugs or request features at [GitHub Issues](https://github.com/agentic-identity-broker/agentic-identity-broker/issues)
- **Documentation**: Browse [documentation site](docs/) or [run locally](README.md#development-workflow)
- **Development**: Follow [Contributing Guidelines](#contributing)

## License

[Specify your license here]

## Contact

[Specify contact information or team here]
