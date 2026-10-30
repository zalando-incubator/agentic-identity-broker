# API Architecture Diagram

Visual representation of the Domain Model and Consent APIs architecture.

## System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         External World                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐                           ┌─────────────┐     │
│  │   Admin     │                           │    User     │     │
│  │   Client    │                           │   Client    │     │
│  └──────┬──────┘                           └──────┬──────┘     │
│         │                                         │            │
└─────────┼─────────────────────────────────────────┼────────────┘
          │                                         │
          │ HTTPS                                   │ HTTPS
          │ X-Remote-User: admin@example.com        │ X-Remote-User: alice@example.com
          │                                         │
┌─────────▼─────────────────────────────────────────▼────────────┐
│                    Reverse Proxy / API Gateway                  │
│              (Authentication, Rate Limiting, TLS)               │
└─────────┬─────────────────────────────────────────┬────────────┘
          │                                         │
          │ Port 8081 (Admin)                       │ Port 8080 (User)
          │                                         │
┌─────────▼─────────────────┐         ┌─────────────▼─────────────┐
│   Admin Server            │         │   User Server             │
│   ┌──────────────────┐    │         │   ┌──────────────────┐    │
│   │  Agent API       │    │         │   │  Consent API     │    │
│   │  /api/agents     │    │         │   │  /api/consent    │    │
│   └──────────────────┘    │         │   └──────────────────┘    │
│   ┌──────────────────┐    │         │   ┌──────────────────┐    │
│   │  OAuth2 API      │    │         │   │  Grant API       │    │
│   │  /api/third-     │    │         │   │  /api/consent/   │    │
│   │  party/oauth2    │    │         │   │  agent/*/grants  │    │
│   └──────────────────┘    │         │   └──────────────────┘    │
└───────────┬───────────────┘         └───────────┬───────────────┘
            │                                     │
            │                                     │
┌───────────▼─────────────────────────────────────▼───────────────┐
│                     HTTP Adapter Layer                          │
│  ┌────────────────────────────────────────────────────────┐     │
│  │  Middleware Stack:                                     │     │
│  │  - RecoveryMiddleware (panic handling)                 │     │
│  │  - LoggingMiddleware (request/response logging)        │     │
│  │  - OptionalPrincipalMiddleware (extract principal)     │     │
│  │  - RequirePrincipalMiddleware (require principal)      │     │
│  │  - AdminAuthorizationMiddleware (check admin role)     │     │
│  │  - RateLimitMiddleware (rate limiting)                 │     │
│  └────────────────────────────────────────────────────────┘     │
│  ┌────────────────────────────────────────────────────────┐     │
│  │  Handlers:                                             │     │
│  │  - AgentHandler (CRUD for agents)                      │     │
│  │  - OAuth2ServiceHandler (CRUD for services)            │     │
│  │  - ConsentHandler (agent info + services)              │     │
│  │  - GrantHandler (grant CRUD)                           │     │
│  └────────────────────────────────────────────────────────┘     │
│  ┌────────────────────────────────────────────────────────┐     │
│  │  DTOs:                                                 │     │
│  │  - Request DTOs (validation, marshaling)               │     │
│  │  - Response DTOs (serialization, redaction)            │     │
│  │  - Error DTOs (structured error responses)             │     │
│  └────────────────────────────────────────────────────────┘     │
└───────────────────────────┬─────────────────────────────────────┘
                            │ Port Interfaces
┌───────────────────────────▼─────────────────────────────────────┐
│                        Domain Layer                             │
│  ┌────────────────────────────────────────────────────────┐     │
│  │  Entities:                                             │     │
│  │  - Agent (id, oauth2_client_id, display_name, ...)     │     │
│  │  - OAuth2Service (id, client_id, endpoints, scopes)    │     │
│  │  - UserGrant (id, principal, agent_id, tokens)         │     │
│  │  - DelegatedOAuth2Token (service_id, scopes)           │     │
│  └────────────────────────────────────────────────────────┘     │
│  ┌────────────────────────────────────────────────────────┐     │
│  │  Business Logic:                                       │     │
│  │  - Validation (fields, URLs, timestamps)               │     │
│  │  - Grant upsert logic (one per user-agent)             │     │
│  │  - Scope validation (against service config)           │     │
│  │  - Expiration filtering (valid_until)                  │     │
│  │  - Referential integrity checks                        │     │
│  └────────────────────────────────────────────────────────┘     │
│  ┌────────────────────────────────────────────────────────┐     │
│  │  Domain Services:                                      │     │
│  │  - OAuth2DiscoveryService (endpoint discovery)         │     │
│  │  - EncryptionService (client secret encryption)        │     │
│  │  - ValidationService (business rule validation)        │     │
│  └────────────────────────────────────────────────────────┘     │
└───────────────────────────┬─────────────────────────────────────┘
                            │ Repository Interfaces
┌───────────────────────────▼─────────────────────────────────────┐
│                    Persistence Adapter Layer                    │
│  ┌─────────────────────┐         ┌────────────────────────┐     │
│  │  In-Memory Adapter  │         │  PostgreSQL Adapter    │     │
│  │  - AgentRepo        │         │  - AgentRepo           │     │
│  │  - OAuth2ServiceRepo│         │  - OAuth2ServiceRepo   │     │
│  │  - GrantRepo        │         │  - GrantRepo           │     │
│  │  (sync.RWMutex)     │         │  (sqlx + pgx)          │     │
│  └─────────────────────┘         └────────────────────────┘     │
└───────────────────────────┬─────────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────────┐
│                        Data Store                               │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  PostgreSQL 12+                                         │    │
│  │  - agents (with oauth2_client_id index)                 │    │
│  │  - thirdparty_oauth2_services (with client_id index)    │    │
│  │  - oauth2_scopes (foreign key to services)              │    │
│  │  - user_grants (unique: principal + agent_id)           │    │
│  │  - delegated_oauth2_tokens (foreign keys)               │    │
│  │  - delegated_token_scopes (foreign keys)                │    │
│  │                                                          │    │
│  │  Constraints:                                           │    │
│  │  - CASCADE DELETE: agent → grants                       │    │
│  │  - RESTRICT DELETE: service (if grants exist)           │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

## Request Flow Diagrams

### Admin Flow: Create Agent

```
┌────────┐         ┌────────┐         ┌─────────┐         ┌─────────┐         ┌──────────┐
│ Admin  │         │ Reverse│         │  HTTP   │         │ Domain  │         │ Database │
│ Client │         │ Proxy  │         │ Handler │         │ Service │         │          │
└───┬────┘         └───┬────┘         └────┬────┘         └────┬────┘         └────┬─────┘
    │                  │                   │                   │                   │
    │ POST /api/agents │                   │                   │                   │
    ├─────────────────>│                   │                   │                   │
    │ + Agent JSON     │                   │                   │                   │
    │                  │                   │                   │                   │
    │                  │ Authenticate      │                   │                   │
    │                  │ Set X-Remote-User │                   │                   │
    │                  ├──────────────────>│                   │                   │
    │                  │                   │                   │                   │
    │                  │                   │ Extract Principal │                   │
    │                  │                   │ (Middleware)      │                   │
    │                  │                   │<─ ─ ─ ─ ─ ─ ─ ─ ─│                   │
    │                  │                   │                   │                   │
    │                  │                   │ Check Admin Role  │                   │
    │                  │                   │ (Middleware)      │                   │
    │                  │                   │<─ ─ ─ ─ ─ ─ ─ ─ ─│                   │
    │                  │                   │                   │                   │
    │                  │                   │ Decode + Validate │                   │
    │                  │                   │ Request           │                   │
    │                  │                   │<─ ─ ─ ─ ─ ─ ─ ─ ─│                   │
    │                  │                   │                   │                   │
    │                  │                   │ Create Agent      │                   │
    │                  │                   ├──────────────────>│                   │
    │                  │                   │                   │                   │
    │                  │                   │                   │ Validate Business │
    │                  │                   │                   │ Rules             │
    │                  │                   │                   │<─ ─ ─ ─ ─ ─ ─ ─ ─│
    │                  │                   │                   │                   │
    │                  │                   │                   │ Generate UUID     │
    │                  │                   │                   │<─ ─ ─ ─ ─ ─ ─ ─ ─│
    │                  │                   │                   │                   │
    │                  │                   │                   │ Insert Agent      │
    │                  │                   │                   ├──────────────────>│
    │                  │                   │                   │                   │
    │                  │                   │                   │                   │ BEGIN
    │                  │                   │                   │                   │ INSERT
    │                  │                   │                   │                   │ COMMIT
    │                  │                   │                   │<──────────────────┤
    │                  │                   │                   │ Agent Created     │
    │                  │                   │<──────────────────┤                   │
    │                  │                   │ Agent Entity      │                   │
    │                  │                   │                   │                   │
    │                  │                   │ Audit Log         │                   │
    │                  │                   │<─ ─ ─ ─ ─ ─ ─ ─ ─│                   │
    │                  │                   │                   │                   │
    │                  │                   │ Serialize Response│                   │
    │                  │                   │<─ ─ ─ ─ ─ ─ ─ ─ ─│                   │
    │                  │<──────────────────┤                   │                   │
    │<─────────────────┤ 201 Created       │                   │                   │
    │ Agent JSON       │                   │                   │                   │
```

### User Flow: Create Grant

```
┌────────┐         ┌────────┐         ┌─────────┐         ┌─────────┐         ┌──────────┐
│  User  │         │ Reverse│         │  HTTP   │         │ Domain  │         │ Database │
│ Client │         │ Proxy  │         │ Handler │         │ Service │         │          │
└───┬────┘         └───┬────┘         └────┬────┘         └────┬────┘         └────┬─────┘
    │                  │                   │                   │                   │
    │ 1. GET /api/     │                   │                   │                   │
    │    consent/agent │                   │                   │                   │
    │    /{id}         │                   │                   │                   │
    ├─────────────────>│                   │                   │                   │
    │                  ├──────────────────>│                   │                   │
    │                  │                   │ Get Agent + All   │                   │
    │                  │                   │ Services + Scopes │                   │
    │                  │                   ├──────────────────>│                   │
    │                  │                   │                   ├──────────────────>│
    │                  │                   │                   │<──────────────────┤
    │                  │                   │<──────────────────┤                   │
    │<─────────────────┤                   │                   │                   │
    │ Agent Info +     │                   │                   │                   │
    │ Services         │                   │                   │                   │
    │                  │                   │                   │                   │
    │ [User reviews    │                   │                   │                   │
    │  and selects     │                   │                   │                   │
    │  services/scopes]│                   │                   │                   │
    │                  │                   │                   │                   │
    │ 2. POST /api/    │                   │                   │                   │
    │    consent/agent │                   │                   │                   │
    │    /{id}/grants  │                   │                   │                   │
    ├─────────────────>│                   │                   │                   │
    │ + Grant Request  │ Set X-Remote-User │                   │                   │
    │                  ├──────────────────>│                   │                   │
    │                  │                   │ Extract Principal │                   │
    │                  │                   │ (alice@...)       │                   │
    │                  │                   │<─ ─ ─ ─ ─ ─ ─ ─ ─│                   │
    │                  │                   │                   │                   │
    │                  │                   │ Validate Request  │                   │
    │                  │                   │ - valid_until     │                   │
    │                  │                   │   in future       │                   │
    │                  │                   │ - scopes exist    │                   │
    │                  │                   │ - service exists  │                   │
    │                  │                   │<─ ─ ─ ─ ─ ─ ─ ─ ─│                   │
    │                  │                   │                   │                   │
    │                  │                   │ Check Existing    │                   │
    │                  │                   │ Grant (upsert)    │                   │
    │                  │                   ├──────────────────>│                   │
    │                  │                   │                   │ SELECT WHERE      │
    │                  │                   │                   │ principal=alice   │
    │                  │                   │                   │ AND agent_id={id} │
    │                  │                   │                   ├──────────────────>│
    │                  │                   │                   │<──────────────────┤
    │                  │                   │                   │ Grant exists?     │
    │                  │                   │<──────────────────┤                   │
    │                  │                   │                   │                   │
    │                  │                   │ Update/Create     │                   │
    │                  │                   │ Grant             │                   │
    │                  │                   ├──────────────────>│                   │
    │                  │                   │                   │ BEGIN             │
    │                  │                   │                   │ UPDATE/INSERT     │
    │                  │                   │                   │ DELETE old tokens │
    │                  │                   │                   │ INSERT new tokens │
    │                  │                   │                   │ COMMIT            │
    │                  │                   │                   ├──────────────────>│
    │                  │                   │                   │<──────────────────┤
    │                  │                   │<──────────────────┤                   │
    │                  │                   │                   │                   │
    │                  │                   │ Audit Log         │                   │
    │                  │                   │<─ ─ ─ ─ ─ ─ ─ ─ ─│                   │
    │                  │<──────────────────┤                   │                   │
    │<─────────────────┤ 201 Created       │                   │                   │
    │ Grant JSON       │ (or 200 Updated)  │                   │                   │
```

## Data Flow: OAuth2 Service Creation with Discovery

```
┌────────┐    ┌─────────┐    ┌──────────┐    ┌────────────┐    ┌──────────┐
│ Admin  │    │  HTTP   │    │ Discovery│    │ External   │    │ Database │
│ Client │    │ Handler │    │ Service  │    │ OAuth2     │    │          │
└───┬────┘    └────┬────┘    └────┬─────┘    │ Provider   │    └────┬─────┘
    │              │              │           └────┬───────┘         │
    │ POST         │              │                │                 │
    │ /api/third-  │              │                │                 │
    │ party/oauth2 │              │                │                 │
    │ /clients     │              │                │                 │
    ├─────────────>│              │                │                 │
    │ enable_      │              │                │                 │
    │ discovery:   │              │                │                 │
    │ true         │              │                │                 │
    │              │              │                │                 │
    │              │ Validate     │                │                 │
    │              │ Request      │                │                 │
    │              │<─ ─ ─ ─ ─ ─ ─│                │                 │
    │              │              │                │                 │
    │              │ Discover     │                │                 │
    │              │ Endpoints    │                │                 │
    │              ├─────────────>│                │                 │
    │              │              │                │                 │
    │              │              │ Construct URL  │                 │
    │              │              │ {issuer}/      │                 │
    │              │              │ .well-known/   │                 │
    │              │              │ oauth-...      │                 │
    │              │              │<─ ─ ─ ─ ─ ─ ─ ─│                 │
    │              │              │                │                 │
    │              │              │ HTTP GET       │                 │
    │              │              │ /.well-known/  │                 │
    │              │              │ oauth-         │                 │
    │              │              │ authorization- │                 │
    │              │              │ server         │                 │
    │              │              ├───────────────>│                 │
    │              │              │                │                 │
    │              │              │                │ Metadata JSON   │
    │              │              │<───────────────┤ (token_endpoint,│
    │              │              │                │ authorize_      │
    │              │              │                │ endpoint, ...)  │
    │              │              │                │                 │
    │              │              │ Parse & Validate│                │
    │              │              │<─ ─ ─ ─ ─ ─ ─ ─│                 │
    │              │<─────────────┤                │                 │
    │              │ Endpoints    │                │                 │
    │              │              │                │                 │
    │              │ Encrypt      │                │                 │
    │              │ client_secret│                │                 │
    │              │<─ ─ ─ ─ ─ ─ ─│                │                 │
    │              │              │                │                 │
    │              │ Insert       │                │                 │
    │              │ Service      │                │                 │
    │              ├─────────────────────────────────────────────────>│
    │              │              │                │                 │
    │              │              │                │         INSERT  │
    │              │              │                │         service,│
    │              │              │                │         scopes  │
    │              │<─────────────────────────────────────────────────┤
    │<─────────────┤              │                │                 │
    │ 201 Created  │              │                │                 │
    │ (client_     │              │                │                 │
    │ secret       │              │                │                 │
    │ redacted)    │              │                │                 │
```

## Security Boundaries

```
┌─────────────────────────────────────────────────────────────────┐
│                        Security Layers                          │
└─────────────────────────────────────────────────────────────────┘

Layer 1: Network Security
┌─────────────────────────────────────────────────────────────────┐
│  - TLS 1.3 encryption                                           │
│  - Reverse proxy (Nginx, Traefik, etc.)                         │
│  - Rate limiting (global + per-IP + per-principal)              │
└─────────────────────────────────────────────────────────────────┘

Layer 2: Authentication
┌─────────────────────────────────────────────────────────────────┐
│  - Pre-authentication via reverse proxy                         │
│  - Principal extraction from X-Remote-User header               │
│  - Principal validation (length, format)                        │
│  - Fail-closed on missing/invalid principal                     │
└─────────────────────────────────────────────────────────────────┘

Layer 3: Authorization
┌─────────────────────────────────────────────────────────────────┐
│  Admin APIs:                                                    │
│  - AdminAuthorizationMiddleware checks role/permission          │
│  - Separate port (8081) for additional network isolation        │
│                                                                 │
│  User APIs:                                                     │
│  - Grant queries filtered by principal (WHERE principal = ?)    │
│  - No cross-user access possible                                │
└─────────────────────────────────────────────────────────────────┘

Layer 4: Data Protection
┌─────────────────────────────────────────────────────────────────┐
│  - Client secrets encrypted at rest (AES-256)                   │
│  - Client secrets redacted in all responses ("***REDACTED***")  │
│  - URLs validated to prevent injection (scheme, characters)     │
│  - Input sanitization on all fields                             │
│  - Parameterized queries (SQL injection prevention)             │
└─────────────────────────────────────────────────────────────────┘

Layer 5: Audit & Monitoring
┌─────────────────────────────────────────────────────────────────┐
│  - Structured audit logs (who, what, when, where)               │
│  - Security event alerting (failed auth, rate limits)           │
│  - Anomaly detection (unusual grant patterns)                   │
│  - Regular security audits                                      │
└─────────────────────────────────────────────────────────────────┘
```

## Error Handling Flow

```
┌─────────────┐
│  Request    │
└──────┬──────┘
       │
       ▼
┌─────────────────────┐
│ Parse JSON          │
│ (json.Decoder)      │
└──────┬──────┬───────┘
       │      │
       │      └──────────┐
       │                 │
       ▼                 ▼
┌─────────────┐    ┌──────────────────┐
│ Valid JSON  │    │ Invalid JSON     │
└──────┬──────┘    │ → 400 Bad Request│
       │           │ {error: "..."    │
       │           │  message: "..."} │
       │           └──────────────────┘
       ▼
┌─────────────────────┐
│ Validate Schema     │
│ (struct tags,       │
│  custom validators) │
└──────┬──────┬───────┘
       │      │
       │      └──────────┐
       │                 │
       ▼                 ▼
┌─────────────┐    ┌──────────────────────────┐
│ Valid       │    │ Validation Errors        │
└──────┬──────┘    │ → 400 Bad Request        │
       │           │ {error: "..."            │
       │           │  validation_errors: [...]}│
       │           └──────────────────────────┘
       ▼
┌─────────────────────┐
│ Business Logic      │
│ (domain service)    │
└──────┬──────┬───────┘
       │      │
       │      └──────────┐
       │                 │
       ▼                 ▼
┌─────────────┐    ┌──────────────────────────┐
│ Success     │    │ Business Logic Error     │
└──────┬──────┘    │                          │
       │           │ - NotFoundError → 404    │
       │           │ - ConflictError → 409    │
       │           │ - ForbiddenError → 403   │
       │           └──────────────────────────┘
       ▼
┌─────────────────────┐
│ Database Operation  │
└──────┬──────┬───────┘
       │      │
       │      └──────────┐
       │                 │
       ▼                 ▼
┌─────────────┐    ┌──────────────────────────┐
│ Success     │    │ Database Error           │
└──────┬──────┘    │ → 500 Internal Error     │
       │           │ (error logged, details   │
       │           │  hidden from user)       │
       │           └──────────────────────────┘
       ▼
┌─────────────────────┐
│ Serialize Response  │
│ (json.Encoder)      │
└──────┬──────┬───────┘
       │      │
       │      └──────────┐
       │                 │
       ▼                 ▼
┌─────────────┐    ┌──────────────────────────┐
│ 200/201 OK  │    │ Serialization Error      │
│ {data...}   │    │ → 500 Internal Error     │
└─────────────┘    │ (logged, empty response) │
                   └──────────────────────────┘
```

## Database Relationships

```
┌─────────────────────────────────────────────────────────────────┐
│                         Database Schema                         │
└─────────────────────────────────────────────────────────────────┘

┌──────────────────────┐
│     agents           │
│──────────────────────│
│ id (PK, UUID)        │◄───────────┐
│ oauth2_client_id     │            │
│ external_id          │            │
│ display_name         │            │
│ description          │            │
│ governance_url       │            │
│ user_documentation_  │            │
│   url                │            │
│ agent_interface_url  │            │
│ created_at           │            │
│ updated_at           │            │
└──────────────────────┘            │ CASCADE DELETE
                                    │
                                    │
┌──────────────────────────────┐    │    ┌─────────────────────────┐
│ thirdparty_oauth2_services   │    │    │   user_grants           │
│──────────────────────────────│    │    │─────────────────────────│
│ id (PK, UUID)                │◄───┼────┤ id (PK, UUID)           │
│ display_name                 │    │    │ principal               │
│ client_id                    │    └────┤ agent_id (FK) ────────┐ │
│ client_secret_encrypted      │         │ valid_until             │ │
│ issuer_uri                   │         │ created_at              │ │
│ enable_discovery             │         │ updated_at              │ │
│ metadata_url                 │         │                         │ │
│ token_endpoint               │         │ UNIQUE(principal,       │ │
│ authorize_endpoint           │         │        agent_id)        │ │
│ created_at                   │         └─────────────────────────┘ │
│ updated_at                   │                     │                │
└──────────────────────────────┘                     │ CASCADE DELETE │
         │                                           │                │
         │                                           ▼                │
         │                          ┌─────────────────────────────┐  │
         │                          │ delegated_oauth2_tokens     │  │
         │                          │─────────────────────────────│  │
         │                          │ id (PK, UUID)               │  │
         │                          │ user_grant_id (FK) ────────┘  │
         └──────────────────────────┤ thirdparty_oauth2_service_  │
                                    │   id (FK)                   │
                                    │ created_at                  │
                                    │                             │
                                    │ UNIQUE(user_grant_id,       │
                                    │        thirdparty_oauth2_   │
                                    │        service_id)          │
                                    └─────────────────────────────┘
         │                                           │
         │ CASCADE DELETE                            │ CASCADE DELETE
         ▼                                           ▼
┌──────────────────────────┐    ┌─────────────────────────────────┐
│   oauth2_scopes          │    │ delegated_token_scopes          │
│──────────────────────────│    │─────────────────────────────────│
│ id (PK, UUID)            │    │ id (PK, UUID)                   │
│ thirdparty_oauth2_       │    │ delegated_oauth2_token_id (FK)  │
│   service_id (FK)        │    │ scope_value                     │
│ scope_value              │    │ created_at                      │
│ description              │    │                                 │
│ created_at               │    │ UNIQUE(delegated_oauth2_token_  │
│                          │    │        id, scope_value)         │
│ UNIQUE(service_id,       │    └─────────────────────────────────┘
│        scope_value)      │
└──────────────────────────┘

Key Constraints:
- CASCADE DELETE: agents → user_grants → delegated_oauth2_tokens → scopes
- RESTRICT DELETE: thirdparty_oauth2_services (blocked if grants exist)
- UNIQUE: (principal, agent_id) ensures one grant per user-agent pair
```

This architecture ensures:
1. Clear separation of concerns (hexagonal architecture)
2. Strong security boundaries
3. Referential integrity
4. Comprehensive error handling
5. Auditability and traceability
