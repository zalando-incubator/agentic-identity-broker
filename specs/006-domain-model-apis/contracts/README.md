# Domain Model and Consent API Contracts

This directory contains the complete REST API specification for the Domain Model and Consent APIs feature.

## Files

### [`openapi.yaml`](./openapi.yaml)
**OpenAPI 3.0.3 Specification**

Complete machine-readable API specification including:
- All endpoints with request/response schemas
- Authentication and security schemes
- Validation rules and constraints
- Error response formats
- Interactive documentation support

**Use cases**:
- Generate client SDKs (TypeScript, Python, Go, etc.)
- View interactive API documentation (Swagger UI, Redoc)
- Validate requests/responses in tests
- Contract testing between services
- API mocking for frontend development

**View the spec**:
```bash
# With Docker (Swagger UI)
docker run -p 8082:8080 -e SWAGGER_JSON=/openapi.yaml \
  -v $(pwd)/openapi.yaml:/openapi.yaml \
  swaggerapi/swagger-ui

# With npx (Redoc)
npx @redocly/cli preview-docs openapi.yaml

# Open http://localhost:8082
```

### [`API_DESIGN.md`](./API_DESIGN.md)
**Comprehensive Design Documentation**

In-depth design rationale and implementation guidance covering:
- RESTful architecture principles
- HTTP method semantics and status codes
- Security design (authentication, authorization, encryption)
- Error handling patterns with examples
- Database schema recommendations
- Hexagonal architecture integration
- Performance optimization strategies
- Monitoring and observability
- Versioning and deprecation policies

**Target audience**: Backend developers, architects, security reviewers

### [`QUICK_REFERENCE.md`](./QUICK_REFERENCE.md)
**Developer Quick Reference**

Practical implementation guide with:
- Endpoint overview table
- cURL request/response examples for all operations
- Error response examples
- Implementation checklist by phase
- Common code patterns
- Testing examples (unit, integration, E2E)
- Performance targets

**Target audience**: Developers implementing or consuming the APIs

### [`README.md`](./README.md) (this file)
**Navigation and Overview**

Guide to understanding and using the contract documentation.

## API Overview

### Admin APIs (Port 8081)

**Agent Management** (`/api/agents`):
- Full CRUD operations for AI agent registry
- OAuth2 client configuration
- Governance and documentation URLs
- Cascade deletion of associated grants

**OAuth2 Service Management** (`/api/third-party/oauth2/clients`):
- Full CRUD operations for third-party OAuth2 providers
- Automatic endpoint discovery via well-known metadata
- Client secret encryption and redaction
- Scope configuration with descriptions
- Deletion protection (blocks if grants reference service)

### User APIs (Port 8080)

**Consent Information** (`/api/consent/agent/{agent-id}`):
- Agent metadata for display to users
- All available third-party services and scopes
- Used to build consent interfaces

**Grant Management** (`/api/consent/agent/{agent-id}/grants`):
- View user's active grants
- Create/update grants (upsert semantics)
- Revoke grants (empty scopes array)
- User-selectable expiration dates
- Principal automatically derived from session

## Key Features

### Security
- Pre-authentication via reverse proxy (configurable header)
- Principal isolation (users only see their own grants)
- Client secrets encrypted at rest, redacted in responses
- URL validation to prevent injection attacks
- Comprehensive audit logging
- Rate limiting

### Data Integrity
- Cascade deletion (agent → grants)
- Referential integrity protection (services with grants can't be deleted)
- Automatic expired grant filtering
- One grant per user-agent pair (upsert semantics)

### Developer Experience
- Comprehensive error messages with field-level validation
- OpenAPI specification for code generation
- Interactive documentation
- Detailed design rationale and examples

## Getting Started

### For Backend Developers

1. **Read the specification**: Start with [`spec.md`](../spec.md) to understand requirements
2. **Review API design**: Read [`API_DESIGN.md`](./API_DESIGN.md) for architecture and patterns
3. **Follow implementation guide**: Use [`QUICK_REFERENCE.md`](./QUICK_REFERENCE.md) checklist
4. **Reference OpenAPI spec**: Use [`openapi.yaml`](./openapi.yaml) for schemas and validation

### For Frontend Developers

1. **Generate client SDK**: Use OpenAPI spec to generate TypeScript/JavaScript client
2. **Review request/response examples**: See [`QUICK_REFERENCE.md`](./QUICK_REFERENCE.md)
3. **Test with mock server**: Use OpenAPI spec with Prism or similar tools
4. **Implement consent flow**:
   - GET `/api/consent/agent/{id}` → Display agent info and services
   - User selects services/scopes
   - POST `/api/consent/agent/{id}/grants` → Create grant

### For API Consumers

1. **View interactive docs**: Run Swagger UI or Redoc with `openapi.yaml`
2. **Test endpoints**: Use cURL examples from [`QUICK_REFERENCE.md`](./QUICK_REFERENCE.md)
3. **Handle errors**: Refer to error response examples
4. **Integrate authentication**: Set `X-Remote-User` header (or configured alternative)

### For QA/Testing

1. **Review acceptance scenarios**: See [`spec.md`](../spec.md) user stories
2. **Use test examples**: Adapt from [`QUICK_REFERENCE.md`](./QUICK_REFERENCE.md)
3. **Validate contracts**: Use OpenAPI spec for contract testing
4. **Test edge cases**: Reference error scenarios in documentation

## Implementation Status

**Current Status**: Draft (API contracts defined, implementation pending)

**Implementation Phases**:
1. Database schema and persistence layer
2. Domain layer (entities, validation, repositories)
3. HTTP layer (handlers, middleware, routing)
4. Security (authorization, encryption, audit logging)
5. OAuth2 discovery service
6. Testing (unit, integration, E2E)
7. Documentation and deployment

See [`QUICK_REFERENCE.md`](./QUICK_REFERENCE.md) for detailed implementation checklist.

## Architecture

### Hexagonal Architecture

The APIs follow hexagonal architecture (ports and adapters):

```
┌─────────────────────────────────────────────────────────┐
│                    HTTP Adapter Layer                   │
│  (Handlers, Middleware, Request/Response DTOs)          │
└──────────────────────┬──────────────────────────────────┘
                       │ Port Interfaces
┌──────────────────────▼──────────────────────────────────┐
│                    Domain Layer                         │
│  (Entities, Validation, Business Logic)                 │
│  - Agent                                                │
│  - OAuth2Service                                        │
│  - UserGrant                                            │
└──────────────────────┬──────────────────────────────────┘
                       │ Repository Interfaces
┌──────────────────────▼──────────────────────────────────┐
│                 Persistence Adapters                    │
│  (In-Memory, PostgreSQL)                                │
└─────────────────────────────────────────────────────────┘
```

### Technology Stack

- **Language**: Go 1.23+
- **HTTP Router**: chi v5
- **Configuration**: Viper
- **Logging**: slog (structured logging)
- **Database**: PostgreSQL 12+ (with in-memory fallback)
- **Database Driver**: sqlx + pgx v5
- **Validation**: Custom validators + OpenAPI schema validation

## Compliance Mapping

The API design satisfies all functional and security requirements from [`spec.md`](../spec.md):

### Functional Requirements (FR)
- **FR-001 to FR-025**: All entity storage, CRUD operations, validation, and filtering requirements met
- **FR-006, FR-007**: Admin APIs for agents and OAuth2 services
- **FR-009 to FR-014**: User APIs for consent and grant management
- **FR-015 to FR-022**: Upsert semantics, expiration, validation, cascade/referential integrity

### Security Requirements (SR)
- **SR-001**: Admin endpoint protection
- **SR-002, SR-003**: Client secret encryption and redaction
- **SR-004, SR-010**: Principal validation and grant isolation
- **SR-005**: Scope validation against service configuration
- **SR-006**: Fail-closed on missing principal
- **SR-007, SR-008**: Audit logging for all operations
- **SR-009**: SSL certificate validation for discovery
- **SR-011**: URL validation
- **SR-012**: Rate limiting

## Related Documentation

- **Feature Specification**: [`../spec.md`](../spec.md) - Complete requirements and user stories
- **Architecture Overview**: [`docs/ARCHITECTURE.md`](../../../docs/ARCHITECTURE.md) - System architecture
- **Development Guide**: [`/AGENT.md`](/AGENT.md) - Build, test, and deployment workflows
- **Session Management**: [`/specs/005-session-management/`](/specs/005-session-management/) - Principal extraction

## Support and Feedback

For questions or feedback about the API design:

1. Review the documentation in this directory
2. Check the feature specification for requirements clarification
3. Refer to acceptance scenarios in [`spec.md`](../spec.md)
4. Create an issue with specific questions or concerns

## Changelog

### 2025-12-17 - Initial Draft
- Created OpenAPI 3.0.3 specification
- Documented API design rationale
- Provided implementation quick reference
- Defined all endpoints, schemas, and error responses
- Mapped to functional and security requirements

## License

This specification is part of the Agentic Identity Broker project and follows the same license as the main project.
