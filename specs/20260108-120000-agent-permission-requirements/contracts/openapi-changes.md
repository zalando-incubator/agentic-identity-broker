# API Contracts: Agent Permission Requirements

**Feature**: 20260108-120000-agent-permission-requirements  
**Date**: 2026-01-08  
**Status**: Phase 1 - API Contract Design

## Overview

This document describes the API contract changes for the agent permission requirements feature. It covers extensions to both Admin API and End-User API.

## OpenAPI Specification Changes

### Admin API Changes (`/api/admin/openapi.yaml`)

#### Schema Extensions

**Add ServiceRequirement Schema**:

```yaml
ServiceRequirement:
  type: object
  required:
    - service_id
    - requirement_type
    - required_scopes
  properties:
    service_id:
      type: string
      format: uuid
      description: UUID of the third-party OAuth2 service
      example: "550e8400-e29b-41d4-a716-446655440000"
    requirement_type:
      type: string
      enum: [mandatory, optional]
      description: Whether this service is required or optional for the agent
      example: "mandatory"
    required_scopes:
      type: array
      items:
        type: string
      minItems: 1
      description: OAuth2 scopes that must be granted for this service
      example: ["repo", "user:email"]
```

**Extend Agent Schema**:

```yaml
Agent:
  type: object
  required:
    - client_id
    - client_secret
    - name
    - redirect_uris
  properties:
    id:
      type: string
      format: uuid
      description: Agent unique identifier
      example: "a1b2c3d4-5678-90ab-cdef-1234567890ab"
    client_id:
      type: string
      description: OAuth2 client_id
      example: "my-agent-client"
    client_secret:
      type: string
      format: password
      description: OAuth2 client_secret (encrypted at rest)
      example: "secret123"
    name:
      type: string
      minLength: 1
      maxLength: 100
      description: Agent display name
      example: "GitHub Bot"
    description:
      type: string
      maxLength: 500
      description: Agent description
      example: "Automated GitHub workflow agent"
    redirect_uris:
      type: array
      items:
        type: string
        format: uri
      minItems: 1
      description: Valid OAuth2 redirect URIs
      example: ["https://example.com/callback"]
    service_requirements:  # NEW FIELD
      type: array
      items:
        $ref: '#/components/schemas/ServiceRequirement'
      description: Third-party services required by this agent (mandatory and optional)
      nullable: true
      example:
        - service_id: "550e8400-e29b-41d4-a716-446655440000"
          requirement_type: "mandatory"
          required_scopes: ["repo", "user:email"]
        - service_id: "7c9e6679-7425-40de-944b-e07fc1f90ae7"
          requirement_type: "optional"
          required_scopes: ["chat:write"]
    created_at:
      type: string
      format: date-time
      description: Creation timestamp
    updated_at:
      type: string
      format: date-time
      description: Last update timestamp
```

**Add ServiceRequirementWithMetadata Schema** (for GET responses):

```yaml
ServiceRequirementWithMetadata:
  allOf:
    - $ref: '#/components/schemas/ServiceRequirement'
    - type: object
      properties:
        service_name:
          type: string
          description: Display name of the referenced service (resolved from ThirdPartyOAuth2Service)
          example: "GitHub"
```

#### Endpoint Changes

**POST /api/agents** (extend existing):

```yaml
/api/agents:
  post:
    summary: Create a new agent
    tags:
      - Admin - Agents
    security:
      - bearerAuth: []
    requestBody:
      required: true
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Agent'
          examples:
            agentWithRequirements:
              summary: Agent with service requirements
              value:
                client_id: "my-agent-client"
                client_secret: "secret123"
                name: "GitHub Bot"
                description: "Automated GitHub workflow agent"
                redirect_uris: ["https://example.com/callback"]
                service_requirements:
                  - service_id: "550e8400-e29b-41d4-a716-446655440000"
                    requirement_type: "mandatory"
                    required_scopes: ["repo", "user:email"]
                  - service_id: "7c9e6679-7425-40de-944b-e07fc1f90ae7"
                    requirement_type: "optional"
                    required_scopes: ["chat:write"]
    responses:
      '201':
        description: Agent created successfully
        content:
          application/json:
            schema:
              type: object
              properties:
                id:
                  type: string
                  format: uuid
                client_id:
                  type: string
                name:
                  type: string
                description:
                  type: string
                redirect_uris:
                  type: array
                  items:
                    type: string
                service_requirements:
                  type: array
                  items:
                    $ref: '#/components/schemas/ServiceRequirementWithMetadata'
                created_at:
                  type: string
                  format: date-time
                updated_at:
                  type: string
                  format: date-time
      '400':
        description: Validation error
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
            examples:
              invalidServiceReference:
                summary: Non-existent service_id
                value:
                  error:
                    code: "INVALID_SERVICE_REFERENCE"
                    message: "Service requirement references non-existent service"
                    details:
                      service_id: "00000000-0000-0000-0000-000000000000"
                      requirement_index: 0
              invalidScopes:
                summary: Invalid scope names
                value:
                  error:
                    code: "INVALID_SCOPES"
                    message: "Service requirement contains scopes not defined in service"
                    details:
                      service_id: "550e8400-e29b-41d4-a716-446655440000"
                      service_name: "GitHub"
                      invalid_scopes: ["nonexistent:scope"]
                      available_scopes: ["repo", "user:email", "read:user"]
              duplicateService:
                summary: Duplicate service_id
                value:
                  error:
                    code: "DUPLICATE_SERVICE_REQUIREMENT"
                    message: "Service requirement contains duplicate service_id"
                    details:
                      service_id: "550e8400-e29b-41d4-a716-446655440000"
                      duplicate_indices: [0, 2]
      '401':
        description: Unauthorized (missing or invalid admin token)
```

**PUT /api/agents/{agent-id}** (extend existing):

```yaml
/api/agents/{agent-id}:
  put:
    summary: Update an existing agent
    tags:
      - Admin - Agents
    security:
      - bearerAuth: []
    parameters:
      - name: agent-id
        in: path
        required: true
        schema:
          type: string
          format: uuid
    requestBody:
      required: true
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Agent'
    responses:
      '200':
        description: Agent updated successfully
        content:
          application/json:
            schema:
              type: object
              properties:
                id:
                  type: string
                  format: uuid
                client_id:
                  type: string
                name:
                  type: string
                service_requirements:
                  type: array
                  items:
                    $ref: '#/components/schemas/ServiceRequirementWithMetadata'
                updated_at:
                  type: string
                  format: date-time
      '400':
        description: Validation error (same as POST)
      '404':
        description: Agent not found
```

**GET /api/agents/{agent-id}** (extend existing):

```yaml
/api/agents/{agent-id}:
  get:
    summary: Get agent details
    tags:
      - Admin - Agents
    security:
      - bearerAuth: []
    parameters:
      - name: agent-id
        in: path
        required: true
        schema:
          type: string
          format: uuid
    responses:
      '200':
        description: Agent details with resolved service names
        content:
          application/json:
            schema:
              type: object
              properties:
                id:
                  type: string
                  format: uuid
                client_id:
                  type: string
                name:
                  type: string
                description:
                  type: string
                redirect_uris:
                  type: array
                  items:
                    type: string
                service_requirements:
                  type: array
                  items:
                    $ref: '#/components/schemas/ServiceRequirementWithMetadata'
                created_at:
                  type: string
                  format: date-time
                updated_at:
                  type: string
                  format: date-time
      '404':
        description: Agent not found
```

### End-User API Changes (`/api/enduser/openapi.yaml`)

#### Schema Extensions

**Add ScopeWithDescription Schema**:

```yaml
ScopeWithDescription:
  type: object
  required:
    - name
  properties:
    name:
      type: string
      description: OAuth2 scope name
      example: "repo"
    description:
      type: string
      description: Human-readable description of what this scope allows
      nullable: true
      example: "Access to public and private repositories"
```

**Add ServiceRequirementForUser Schema**:

```yaml
ServiceRequirementForUser:
  type: object
  required:
    - service_id
    - service_name
    - requirement_type
    - required_scopes
    - user_has_active_session
  properties:
    service_id:
      type: string
      format: uuid
      description: UUID of the third-party OAuth2 service
    service_name:
      type: string
      description: Display name of the service
      example: "GitHub"
    requirement_type:
      type: string
      enum: [mandatory, optional]
      description: Whether this service is required or optional
      example: "mandatory"
    required_scopes:
      type: array
      items:
        $ref: '#/components/schemas/ScopeWithDescription'
      description: OAuth2 scopes with descriptions
    user_has_active_session:
      type: boolean
      description: Whether the current user has an active (non-expired) session for this service
      example: true
```

#### Endpoint Changes

**GET /api/consent/agent/{agent-id}** (extend existing):

```yaml
/api/consent/agent/{agent-id}:
  get:
    summary: Get agent information for consent screen
    tags:
      - Consent
    security:
      - cookieAuth: []
    parameters:
      - name: agent-id
        in: path
        required: true
        schema:
          type: string
          format: uuid
    responses:
      '200':
        description: Agent information with service requirements and user session status
        content:
          application/json:
            schema:
              type: object
              properties:
                agent:
                  type: object
                  properties:
                    id:
                      type: string
                      format: uuid
                    name:
                      type: string
                    description:
                      type: string
                service_requirements:  # NEW FIELD
                  type: array
                  items:
                    $ref: '#/components/schemas/ServiceRequirementForUser'
                  description: Service requirements with user's current session status
            example:
              agent:
                id: "a1b2c3d4-5678-90ab-cdef-1234567890ab"
                name: "GitHub Bot"
                description: "Automated GitHub workflow agent"
              service_requirements:
                - service_id: "550e8400-e29b-41d4-a716-446655440000"
                  service_name: "GitHub"
                  requirement_type: "mandatory"
                  required_scopes:
                    - name: "repo"
                      description: "Access to public and private repositories"
                    - name: "user:email"
                      description: "Read access to user email address"
                  user_has_active_session: true
                - service_id: "7c9e6679-7425-40de-944b-e07fc1f90ae7"
                  service_name: "Slack"
                  requirement_type: "optional"
                  required_scopes:
                    - name: "chat:write"
                      description: "Send messages to Slack channels"
                  user_has_active_session: false
      '404':
        description: Agent not found
```

**POST /api/consent/agent/{agent-id}/approve** (extend existing):

```yaml
/api/consent/agent/{agent-id}/approve:
  post:
    summary: Approve consent for agent
    description: |
      Approves consent for the agent. If redirect_uri query parameter is provided,
      the backend issues an HTTP redirect to that URL after successful approval.
      The redirect_uri MUST be same-origin for security (open redirect prevention).
    tags:
      - Consent
    security:
      - cookieAuth: []
    parameters:
      - name: agent-id
        in: path
        required: true
        schema:
          type: string
          format: uuid
      - name: redirect_uri
        in: query
        required: false
        schema:
          type: string
          format: uri
        description: |
          URL to redirect to after approval. MUST be same-origin or relative path.
          External domains are rejected with HTTP 400.
        example: "/oauth2/authorize?client_id=my-agent&response_type=code&state=xyz"
    requestBody:
      required: true
      content:
        application/json:
          schema:
            type: object
            properties:
              expiration_days:
                type: integer
                minimum: 1
                maximum: 365
                description: Days until grant expires
                example: 90
    responses:
      '200':
        description: Consent approved successfully (no redirect_uri provided)
        content:
          application/json:
            schema:
              type: object
              properties:
                message:
                  type: string
                  example: "Consent approved"
                grant_id:
                  type: string
                  format: uuid
      '302':
        description: Redirect to continue OAuth2 flow (redirect_uri provided)
        headers:
          Location:
            schema:
              type: string
            description: The redirect_uri from query parameter
            example: "/oauth2/authorize?client_id=my-agent&response_type=code&state=xyz"
      '400':
        description: Validation error or external redirect attempt
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Error'
            examples:
              externalRedirect:
                summary: External redirect attempt
                value:
                  error:
                    code: "INVALID_REDIRECT_URI"
                    message: "redirect_uri must be same-origin or relative path"
                    details:
                      provided_uri: "https://evil.com/phish"
      '404':
        description: Agent not found
```

**GET /oauth2/authorize** (behavior change documentation):

```yaml
/oauth2/authorize:
  get:
    summary: OAuth2 authorization endpoint
    description: |
      Initiates OAuth2 authorization flow. Extended to validate agent service requirements.
      
      **NEW BEHAVIOR**: If the agent has mandatory service requirements that are not satisfied
      (user lacks active sessions or sessions lack required scopes), the endpoint redirects
      to the consent screen instead of proxying to the upstream OAuth2 server.
      
      The consent screen will display which requirements are missing and allow the user
      to connect to required services.
    tags:
      - OAuth2
    parameters:
      - name: client_id
        in: query
        required: true
        schema:
          type: string
        description: Agent's OAuth2 client_id
      - name: response_type
        in: query
        required: true
        schema:
          type: string
          enum: [code]
        description: OAuth2 response type
      - name: redirect_uri
        in: query
        required: true
        schema:
          type: string
          format: uri
        description: OAuth2 redirect URI
      - name: state
        in: query
        required: false
        schema:
          type: string
        description: OAuth2 state parameter
    responses:
      '302':
        description: |
          Redirect to consent screen (if requirements not met or no grant exists) OR
          redirect to upstream OAuth2 server (if all requirements satisfied)
        headers:
          Location:
            schema:
              type: string
            description: |
              - Consent screen: /consent/agent/{agent-id}?redirect_uri={original-request-uri}
              - Upstream server: (configured upstream OAuth2 authorization URL)
```

## Error Codes

| Error Code | HTTP Status | Description | Example Trigger |
|------------|-------------|-------------|-----------------|
| `INVALID_SERVICE_REFERENCE` | 400 | service_id does not reference existing ThirdPartyOAuth2Service | Admin creates agent with non-existent service_id |
| `INVALID_SCOPES` | 400 | required_scopes contains scope names not in service configuration | Admin specifies scope not defined in service |
| `DUPLICATE_SERVICE_REQUIREMENT` | 400 | Same service_id appears multiple times in service_requirements | Admin specifies GitHub twice |
| `INVALID_REDIRECT_URI` | 400 | redirect_uri is external domain (open redirect prevention) | User approval with redirect_uri=https://evil.com |

## Validation Rules Summary

### Admin API Validation (Request Time)

1. **Structure Validation** (Domain Layer):
   - service_id is valid UUID format
   - requirement_type is "mandatory" or "optional"
   - required_scopes is non-empty array
   - No duplicate service_id values in array

2. **Referential Integrity** (Application Layer):
   - service_id exists in ThirdPartyOAuth2Service table
   - All scopes in required_scopes exist in service's scope list
   - Scopes are case-sensitive

### Authorization Endpoint Validation (Runtime)

1. **Session Existence**:
   - User has ThirdPartyOAuth2Session for service_id
   - Session is not expired

2. **Scope Validation**:
   - Session scopes ⊇ required_scopes (superset check)
   - Case-sensitive comparison

3. **Mandatory vs Optional**:
   - Only "mandatory" requirements block authorization
   - "optional" requirements displayed in consent but don't block

### Redirect URI Validation (Approval Time)

1. **Same-Origin Check**:
   - redirect_uri scheme, host, port match request origin
   - OR redirect_uri is relative path (no scheme/host)
   - External domains rejected with HTTP 400

## Next Steps

- [x] Phase 0: Research completed
- [x] Phase 1: Data model completed
- [x] Phase 1: API contracts completed
- [ ] Phase 1: Generate quickstart.md implementation guide
- [ ] Phase 1: Update agent context via update-agent-context.sh
- [ ] Phase 1: Re-evaluate Constitution Check post-design
