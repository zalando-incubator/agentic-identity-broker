# API Contracts: Agent Permission Requirements

This directory contains the API contract documentation for the agent permission requirements feature (20260108-120000-agent-permission-requirements).

## Contents

- **openapi-changes.md**: Detailed OpenAPI specification changes for Admin API and End-User API
  - Schema extensions (ServiceRequirement, RequirementType)
  - Endpoint modifications (POST/PUT/GET agents, GET consent, POST approve)
  - Error codes and validation rules

## API Changes Summary

### Admin API (`/api/admin/openapi.yaml`)

**Extended Endpoints**:
- `POST /api/agents` - Accept optional `service_requirements` array
- `PUT /api/agents/{agent-id}` - Update service requirements
- `GET /api/agents/{agent-id}` - Return service requirements with resolved service names

**New Schemas**:
- `ServiceRequirement` - service_id, requirement_type, required_scopes
- `ServiceRequirementWithMetadata` - ServiceRequirement + resolved service_name

**New Error Codes**:
- `INVALID_SERVICE_REFERENCE` (400) - service_id doesn't exist
- `INVALID_SCOPES` (400) - scope names not in service
- `DUPLICATE_SERVICE_REQUIREMENT` (400) - duplicate service_id

### End-User API (`/api/enduser/openapi.yaml`)

**Extended Endpoints**:
- `GET /api/consent/agent/{agent-id}` - Return service requirements with user session status
- `POST /api/consent/agent/{agent-id}/approve` - Support redirect_uri query parameter
- `GET /oauth2/authorize` - Validate mandatory service requirements before proxying

**New Schemas**:
- `ServiceRequirementForUser` - Includes user_has_active_session boolean
- `ScopeWithDescription` - Scope name with optional description

**New Error Codes**:
- `INVALID_REDIRECT_URI` (400) - External redirect attempt (open redirect prevention)

## Validation Flow

### Admin API (Creation/Update)

```
Request → Structure Validation (Domain) → Referential Integrity (Application) → Storage (JSONB)
           ↓                              ↓
       UUID format                    service_id exists
       requirement_type enum          scopes exist in service
       scopes non-empty               no duplicate service_id
```

### Authorization Endpoint (Runtime)

```
OAuth2 Request → Load Agent → Check service_requirements?
                               ↓ YES
                   For each mandatory requirement:
                     - User has session for service_id?
                     - Session not expired?
                     - Session scopes ⊇ required_scopes?
                               ↓ ALL PASS
                   Proxy to upstream OAuth2 server
                               ↓ ANY FAIL
                   Redirect to consent screen
```

### Consent Approval (Redirect)

```
Approve Request → redirect_uri parameter present?
                  ↓ YES
              Validate same-origin or relative path
                  ↓ VALID
              HTTP 302 redirect to redirect_uri
                  ↓ INVALID
              HTTP 400 error (open redirect prevented)
```

## Implementation Checklist

- [ ] Update Admin API OpenAPI spec (`api/admin/openapi.yaml`)
- [ ] Update End-User API OpenAPI spec (`api/enduser/openapi.yaml`)
- [ ] Extend Agent entity with service_requirements field
- [ ] Create ServiceRequirement value object
- [ ] Add RequirementType enum
- [ ] Implement Admin API validation (referential integrity)
- [ ] Extend authorization endpoint with requirement checks
- [ ] Update consent endpoint to return service requirements with session status
- [ ] Implement redirect_uri validation (same-origin check)
- [ ] Add E2E tests for all 37 acceptance scenarios
- [ ] Update frontend to display service requirements

## Testing Coverage

- **Unit Tests**: Domain validation (15 test cases)
- **Integration Tests**: JSONB storage, migration testing (10 test cases)
- **E2E Tests**: All 37 acceptance scenarios (mandatory per Constitution Principle XIII)
- **Frontend Tests**: Component rendering (8 test suites)

## Security Considerations

1. **Service Requirement Injection**: Validated at domain and application layers (fail closed)
2. **Scope Confusion**: Case-sensitive scope matching per RFC 6749 (SR-006)
3. **Open Redirect**: Strict same-origin validation for redirect_uri (SR-001, SR-002)
4. **Authorization Bypass**: Mandatory requirements enforced before proxy (fail closed per FR-012)
5. **Audit Logging**: All validation failures logged for security audit (SR-004)

## Related Documentation

- [spec.md](../spec.md) - Feature specification with user stories
- [data-model.md](../data-model.md) - Entity and value object definitions
- [research.md](../research.md) - Technology decisions and rationale
- [quickstart.md](../quickstart.md) - Implementation guide (to be generated)
