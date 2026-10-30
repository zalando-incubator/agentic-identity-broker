# Data Model: OAuth2 Authorization Server Proxy

**Feature**: 20251222-120100-oauth2-auth-server
**Date**: 2025-12-22

## Overview

This feature does not introduce new persistent entities. It leverages existing Agent and User Grant entities from feature 20251217-120000 (Domain Model APIs) and introduces transient request/response objects for OAuth2 protocol handling.

---

## Existing Persistent Entities (Reused)

### Agent
**Location**: `internal/domain/storage/agent.go` (existing)
**Purpose**: Represents AI agents registered in the identity broker. OAuth2 clients map to registered agents via `client_id`.

**Key Attributes**:
- `ID` (string): Unique agent identifier (UUID)
- **`ClientID` (string)**: OAuth2 client identifier (UNIQUE, indexed)
- `DisplayName` (string): Human-readable agent name
- `Description` (string): Agent description
- `CreatedAt` (time.Time): Registration timestamp
- `UpdatedAt` (time.Time): Last modification timestamp

**OAuth2 Usage**:
- Authorization endpoint validates `client_id` from OAuth2 request against `Agent.ClientID`
- Returns `invalid_client` error if no matching agent found

**Repository Interface**: `AgentRepository` in `internal/ports/storage.go` (existing)
```go
type AgentRepository interface {
    FindByClientID(ctx context.Context, clientID string) (*storage.Agent, error)
    // ... other methods
}
```

---

### User Grant
**Location**: `internal/domain/storage/grant.go` (existing)
**Purpose**: Records user consent for an agent to access third-party services on their behalf.

**Key Attributes**:
- `ID` (string): Unique grant identifier (UUID)
- `Principal` (string): User identity (extracted from context by principal middleware)
- `AgentID` (string): Foreign key to Agent.ID
- `ValidUntil` (*time.Time): Grant expiration timestamp (NULL = indefinite)
- `CreatedAt` (time.Time): Grant creation timestamp
- `UpdatedAt` (time.Time): Last modification timestamp

**OAuth2 Usage**:
- Authorization endpoint checks if active grant exists for (principal, agentID) pair
- Grant is active if `ValidUntil` is NULL or > current time
- No active grant → redirect to consent UI
- Active grant exists → redirect to upstream OAuth2 server

**Repository Interface**: `GrantRepository` in `internal/ports/storage.go` (existing)
```go
type GrantRepository interface {
    FindByPrincipalAndAgent(ctx context.Context, principal, agentID string) (*storage.Grant, error)
    // ... other methods
}
```

---

## Transient Request/Response Objects

### AuthorizationRequest
**Purpose**: Represents an OAuth2 authorization request (RFC 6749 Section 4.1.1)

**Attributes**:
- `ClientID` (string): OAuth2 client identifier (REQUIRED)
- `RedirectURI` (string): Client's callback URL (REQUIRED)
- `Scope` (string): Space-delimited requested scopes (OPTIONAL)
- `State` (string): Opaque CSRF protection value (RECOMMENDED)
- `ResponseType` (string): Must be "code" for authorization code flow (REQUIRED)
- `CodeChallenge` (string): PKCE code challenge (OPTIONAL, RFC 7636)
- `CodeChallengeMethod` (string): PKCE method "S256" or "plain" (OPTIONAL)

**Lifecycle**: Created from HTTP query parameters, validated, then either:
- Redirected to consent UI if no active grant
- Redirected to upstream OAuth2 server if active grant exists

**Validation Rules**:
- `ClientID` must match existing `Agent.ClientID`
- `RedirectURI` must be valid HTTPS URL (validated by upstream)
- `ResponseType` must be "code" (other types proxied to upstream for validation)
- All parameters preserved when redirecting to upstream

---

### TokenRequest
**Purpose**: Represents an OAuth2 token request (RFC 6749 Section 4.1.3)

**Attributes**:
- `GrantType` (string): "authorization_code" or "refresh_token" (REQUIRED)
- `Code` (string): Authorization code (REQUIRED for authorization_code grant)
- `RedirectURI` (string): Must match authorization request redirect_uri (REQUIRED for authorization_code)
- `ClientID` (string): OAuth2 client identifier (REQUIRED)
- `ClientSecret` (string): Client secret for authentication (REQUIRED)
- `RefreshToken` (string): Refresh token (REQUIRED for refresh_token grant)

**Lifecycle**: Parsed from HTTP POST body (form-urlencoded or JSON), proxied to upstream OAuth2 server as-is

**Security**: Broker does NOT validate these fields (validation delegated to upstream). Broker only proxies the request.

---

### TokenResponse
**Purpose**: OAuth2 token response (RFC 6749 Section 5.1)

**Attributes**:
- `AccessToken` (string): Bearer access token
- `TokenType` (string): "Bearer"
- `ExpiresIn` (int): Token lifetime in seconds
- `RefreshToken` (string): Refresh token (OPTIONAL)
- `Scope` (string): Granted scopes (OPTIONAL)

**Lifecycle**: Received from upstream OAuth2 server, proxied to client without modification

**Security**: Broker does NOT inspect, log, or store tokens. Tokens are opaque values.

---

### ErrorResponse
**Purpose**: OAuth2 error response (RFC 6749 Section 5.2)

**Attributes**:
- `Error` (string): OAuth2 error code (e.g., "invalid_client", "invalid_grant")
- `ErrorDescription` (string): Human-readable error description (OPTIONAL)
- `ErrorURI` (string): URL to error documentation (OPTIONAL)

**Lifecycle**: Either generated by broker (e.g., "invalid_client" for unknown client_id) or proxied from upstream

**Broker-Generated Errors**:
- `invalid_client`: When client_id doesn't match registered Agent
- `server_error`: When upstream unreachable or TLS validation fails
- `temporarily_unavailable`: When upstream OAuth2 server temporarily down

---

### MetadataResponse
**Purpose**: OAuth2 Authorization Server Metadata (RFC 8414)

**Attributes**:
- `Issuer` (string): Identity broker's public base URL (REQUIRED)
- `AuthorizationEndpoint` (string): Full URL to `/oauth2/authorize` (REQUIRED)
- `TokenEndpoint` (string): Full URL to `/oauth2/token` (REQUIRED)
- `ResponseTypesSupported` ([]string): ["code"] (REQUIRED)
- `GrantTypesSupported` ([]string): ["authorization_code", "refresh_token"] (REQUIRED)
- `TokenEndpointAuthMethodsSupported` ([]string): ["client_secret_post", "client_secret_basic"] (OPTIONAL)

**Lifecycle**: Generated dynamically from configuration, cached in memory for performance

**Configuration Source**: Built from `server.enduser.public_url` config parameter

---

## Domain Services (New)

### OAuth2Service
**Location**: `internal/domain/oauth2/service.go` (NEW)
**Purpose**: Coordinates OAuth2 operations (authorization, token proxy, metadata)

**Port Interface**: `internal/ports/oauth2.go` (NEW)
```go
type OAuth2Service interface {
    // Authorization endpoint logic
    HandleAuthorization(ctx context.Context, req *AuthorizationRequest, principal string) (*AuthorizationDecision, error)

    // Token endpoint logic (validation only, actual proxy in adapter)
    ValidateTokenRequest(ctx context.Context, req *TokenRequest) error

    // Metadata endpoint logic
    GenerateMetadata(ctx context.Context) (*MetadataResponse, error)
}

type AuthorizationDecision struct {
    Action       string // "redirect_to_upstream", "redirect_to_consent", "error"
    RedirectURL  string // Target URL for redirect
    ErrorCode    string // OAuth2 error code (if Action == "error")
    ErrorDesc    string // Error description
}
```

**Dependencies**:
- `AgentRepository`: Validate client_id against registered agents
- `GrantRepository`: Check if active grant exists for (principal, agentID)
- Configuration: Upstream OAuth2 server URLs, public base URL

---

## Data Flow Diagrams

### Authorization Flow with Consent Check

```
Client → /oauth2/authorize?client_id=X&redirect_uri=Y&state=Z
          ↓
    [Extract client_id]
          ↓
    [AgentRepository.FindByClientID(client_id)]
          ↓
    Found? → No → Return OAuth2 error "invalid_client"
          ↓ Yes
    [GrantRepository.FindByPrincipalAndAgent(principal, agentID)]
          ↓
    Active grant? → No → HTTP 302 to /consent/agent/:agent-id?redirect_uri=<full original URL>
          ↓ Yes
    HTTP 302 to upstream authorize endpoint (preserve all params)
```

### Token Exchange Flow (Server-to-Server)

```
Client → POST /oauth2/token
         grant_type=authorization_code&code=ABC&redirect_uri=Y&client_id=X&client_secret=SECRET
          ↓
    [Validate Content-Type: application/x-www-form-urlencoded or application/json]
          ↓
    [Create HTTP POST request to upstream token endpoint]
          ↓
    [Copy all parameters and headers (except hop-by-hop)]
          ↓
    [Execute request with TLS validation]
          ↓
    [Stream response back to client (status code + body)]
```

---

## Persistence Considerations

**No New Database Tables**: This feature uses existing `agents` and `grants` tables from feature 20251217-120000.

**No Migrations Required**: Existing schema sufficient for OAuth2 proxy functionality.

**Session State**: OAuth2 state parameter provides CSRF protection; no server-side session storage required.

**Stateless Proxy**: Broker does not store authorization codes, access tokens, or refresh tokens. All tokens are opaque values managed by upstream OAuth2 server.

---

## Glossary Additions

The following terms should be added to `ARCHITECTURE.md` Glossary:

**OAuth2 Authorization Server**: RFC 6749 compliant server that issues access tokens to clients after successfully authenticating the resource owner and obtaining authorization.

**OAuth2 Client**: Application that requests access to protected resources on behalf of the resource owner. In this system, OAuth2 clients map to registered Agents.

**Authorization Code**: Short-lived, single-use code issued by authorization server to client after user grants consent. Exchanged for access token via token endpoint.

**Access Token**: Credential used by client to access protected resources. Issued by authorization server, validated by resource servers. Identity broker treats as opaque value (does not inspect or validate).

**Refresh Token**: Long-lived credential used to obtain new access tokens without re-prompting user for consent. Issued alongside access token, used via token endpoint.

**OAuth2 Metadata**: JSON document describing authorization server's capabilities and endpoint locations. Enables OAuth2 clients to auto-discover configuration.

**Upstream OAuth2 Server**: External RFC 6749 compliant authorization server to which the identity broker proxies authorization and token requests. Handles token issuance, validation, and management.

---

## References

- Feature 20251217-120000: Domain Model APIs (Agent and User Grant entities)
- RFC 6749: OAuth 2.0 Authorization Framework
- RFC 8414: OAuth 2.0 Authorization Server Metadata
- `internal/domain/storage/agent.go`: Agent entity definition
- `internal/domain/storage/grant.go`: User Grant entity definition
- `internal/ports/storage.go`: Repository interface definitions
