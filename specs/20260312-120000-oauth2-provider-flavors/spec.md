# Feature Specification: OAuth2 Provider Flavor Support

**Feature Branch**: `20260312-120000-oauth2-provider-flavors`
**Created**: 2026-03-12
**Status**: Draft
**Input**: User description: "Support Google OAuth in third-party services. I would like to extend and support non-vanilla flavors of OAuth2. Specifically Google OAuth is different in that it requires a private key instead of a client_secret. I want to be able to create a client using google specific mechanisms and also make this extensible for other cases where the third-party service is not fully OAuth2 spec compliant. To that end the API should support the OAuth2 Flavor, the client secret should be validated depending on the Flavor, for Google OAuth2, it needs to verify that the attribute contains the required JSON with the private key."

## Clarifications

### Session 2026-03-12

- Q: For the Google OAuth2 flavor, should `client_id` be required separately or extracted automatically from the service account JSON? → A: `client_id` is extracted automatically from the service account JSON (`client_id` field) to avoid duplication and prevent inconsistency; it is not required as a separate field when flavor is `google`
- Q: Does this feature cover executing the Google-specific OAuth2 flow (acquiring tokens using the service account private key), or only storing and validating the service account credential? → A: This feature covers configuration, storage, and validation only; the actual Google JWT Bearer token exchange flow is a separate follow-up feature
- Q: Should the system auto-detect the OAuth2 flavor by inspecting the credential content? → A: No. Auto-detection based on credential content is explicitly excluded — it would be brittle. The `oauth2_flavor` field is always required to be explicitly set by the administrator.
- Q: Should the API field name change from `client_secret` to something more generic for the `google` flavor? → A: Keep `client_secret` as the field name for backward compatibility. API documentation will clarify flavor-specific semantics. No renaming.
- Q: For `oauth2_flavor: google`, should `issuer_uri` be required, optional, or excluded? → A: Optional. If provided, it is validated for consistency against the `token_uri` in the service account JSON (scheme and host must match). If omitted, the `token_uri` from the service account JSON is the authoritative token endpoint.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Configure Third-Party Service with Explicit OAuth2 Flavor (Priority: P1)

An administrator needs to specify the OAuth2 flavor when registering or updating a third-party service, so the system knows how to interpret and validate the provided credentials. For standard OAuth2 providers that comply with the spec, the existing `client_secret` continues to work as before; the flavor defaults to `standard` if not specified.

**Why this priority**: The flavor field is the foundation for all other flavor-specific behavior. Without it, the system cannot determine how to validate credentials or which authentication mechanism to use. Existing services must continue to work unchanged.

**Independent Test**: Can be fully tested by creating and updating third-party service configurations with different flavor values, verifying the flavor is stored and returned in subsequent GET requests, and verifying that omitting the flavor defaults to `standard`.

**Acceptance Scenarios**:

1. **Given** an administrator creates a third-party service without specifying an OAuth2 flavor, **When** the service is saved, **Then** the system assigns the default flavor `standard` and the service behaves as before
2. **Given** an administrator creates a third-party service with `oauth2_flavor: standard`, **When** the service is saved, **Then** the flavor is stored as `standard` and credentials are validated as a plain secret string
3. **Given** an administrator creates a third-party service with `oauth2_flavor: google`, **When** the service is saved, **Then** the flavor is stored as `google` and credentials are validated as a Google service account JSON key
4. **Given** an administrator provides an unrecognized flavor value, **When** the service create/update request is submitted, **Then** the request is rejected with HTTP 400 and an error listing valid flavor values
5. **Given** an existing third-party service with flavor `standard`, **When** an administrator updates the service to flavor `google`, **Then** the system re-validates the credential as a Google service account JSON key

---

### User Story 2 - Configure Google OAuth2 Service with Service Account Credentials (Priority: P1)

An administrator needs to register a Google-hosted service (e.g., Google Workspace APIs, Google Drive, BigQuery) as a third-party service. Rather than a client secret, they upload the contents of a Google service account JSON key file as the credential.

**Why this priority**: This is the primary use case that motivates the feature. Administrators must be able to configure Google as a third-party service with its non-standard credential format.

**Independent Test**: Can be fully tested by submitting a POST/PUT request to the third-party service endpoint with `oauth2_flavor: google` and a valid Google service account JSON payload as the credential field. Success is verified by confirming the service is created, the credential is stored encrypted, and the `client_id` is extracted from the JSON automatically.

**Acceptance Scenarios**:

1. **Given** a valid Google service account JSON key is provided as the credential with `oauth2_flavor: google`, **When** the administrator submits the create request, **Then** the system stores the service account JSON encrypted at rest and extracts `client_id` automatically from the JSON's `client_id` field
2. **Given** the service account JSON contains `client_email`, `private_key`, `token_uri`, and `type: service_account`, **When** the service is created, **Then** the system records the service and returns the service configuration with `client_id` populated and the credential redacted from the response
3. **Given** the Google service account JSON is valid but missing the `private_key` field, **When** the create request is submitted, **Then** the request is rejected with HTTP 400 and an error identifying `private_key` as required
4. **Given** the Google service account JSON has `type` set to a value other than `service_account`, **When** the create request is submitted, **Then** the request is rejected with HTTP 400 indicating the credential must be of type `service_account`
5. **Given** a valid Google service account is configured, **When** an administrator retrieves the service via GET, **Then** the response contains the `oauth2_flavor: google`, the `client_id`, and the credential is fully redacted (not returned in any form)
6. **Given** `oauth2_flavor: google` and an `issuer_uri` whose host differs from the `token_uri` host in the service account JSON, **When** the create request is submitted, **Then** the request is rejected with HTTP 400 identifying the inconsistency
7. **Given** `oauth2_flavor: google` and no `issuer_uri` is provided, **When** the service is created successfully, **Then** the `token_uri` from the service account JSON is used as the authoritative token endpoint

---

### User Story 3 - Validate Credentials According to OAuth2 Flavor (Priority: P2)

The system must apply the correct validation rules to the credential field based on the configured OAuth2 flavor, ensuring credentials are structurally correct before storing them.

**Why this priority**: Flavor-specific validation prevents misconfigured services from being created and catches errors at configuration time rather than during live OAuth2 flows.

**Independent Test**: Can be fully tested by submitting service configurations with various valid and invalid credential payloads for each supported flavor and verifying the correct acceptance or rejection responses.

**Acceptance Scenarios**:

1. **Given** `oauth2_flavor: standard` is set, **When** a non-empty string is provided as the credential, **Then** the credential is accepted
2. **Given** `oauth2_flavor: standard` is set, **When** an empty or whitespace-only string is provided as the credential, **Then** the request is rejected with HTTP 400
3. **Given** `oauth2_flavor: google` is set, **When** a valid JSON object containing all required Google service account fields is provided, **Then** the credential is accepted
4. **Given** `oauth2_flavor: google` is set, **When** the credential is not valid JSON, **Then** the request is rejected with HTTP 400 indicating the credential must be valid JSON
5. **Given** `oauth2_flavor: google` is set, **When** the credential JSON is missing one or more required fields (`type`, `private_key`, `client_email`, `token_uri`), **Then** the request is rejected with HTTP 400 and the response identifies each missing required field
6. **Given** `oauth2_flavor: google` is set, **When** a non-empty plain string (not JSON) is provided as the credential, **Then** the request is rejected with HTTP 400 indicating the credential must be a valid JSON service account key

---

### User Story 4 - List and Filter Third-Party Services by Flavor (Priority: P3)

An administrator reviewing the configured third-party services needs to see which flavor each service uses so they can understand the authentication mechanism at a glance.

**Why this priority**: Operational visibility into configured services is important but not blocking. Existing listing functionality still works; this story adds the flavor field to list responses.

**Independent Test**: Can be fully tested by creating services with different flavors and verifying that the GET list response includes the `oauth2_flavor` field for each service.

**Acceptance Scenarios**:

1. **Given** multiple third-party services are configured with different flavors, **When** an administrator requests the list of services via GET `/api/third-party/oauth2/clients`, **Then** each service in the response includes its `oauth2_flavor` value
2. **Given** services with both `standard` and `google` flavors exist, **When** an administrator retrieves a specific service, **Then** the GET response includes the `oauth2_flavor` field

---

### Edge Cases

- What happens when updating a service from `google` to `standard` flavor but the stored credential is a JSON object? The system must reject the update unless a valid plain-text string credential is provided in the same request.
- What happens when the Google service account JSON credential exceeds a reasonable size limit? Reject with HTTP 400 to prevent oversized payloads (maximum 32 KB).
- How does the system handle a Google service account JSON where `client_id` is missing from the JSON? Reject with HTTP 400 indicating `client_id` is a required field in the service account JSON for `google` flavor.
- What happens when the `private_key` field in the Google JSON is present but empty or contains only whitespace? Reject with HTTP 400 indicating `private_key` must be a non-empty value.
- How does the system handle future flavors not yet implemented? Return HTTP 400 with a list of currently supported flavor values.
- What happens when an existing service's credential is updated without changing the flavor? Re-validate the new credential against the existing flavor's rules before storing.
- What happens when `issuer_uri` is provided for `google` flavor but its host differs from the `token_uri` host in the service account JSON? Reject with HTTP 400 indicating the inconsistency between `issuer_uri` and the `token_uri` in the credential.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support an `oauth2_flavor` field on third-party OAuth2 service configurations with a default value of `standard`
- **FR-002**: System MUST validate the `oauth2_flavor` value against the set of supported flavors; unsupported values MUST be rejected with HTTP 400
- **FR-003**: System MUST apply flavor-specific validation rules to the credential field when creating or updating a third-party service
- **FR-004**: For `oauth2_flavor: standard`, the credential MUST be validated as a non-empty string (existing behavior preserved)
- **FR-005**: For `oauth2_flavor: google`, the credential MUST be validated as a JSON object containing all of the following required fields: `type` (must equal `"service_account"`), `private_key` (non-empty string), `client_email` (non-empty string), `token_uri` (non-empty string), `client_id` (non-empty string)
- **FR-006**: For `oauth2_flavor: google`, the system MUST automatically extract `client_id` from the `client_id` field in the service account JSON and store it as the service's `client_id`; the `client_id` request field MUST be ignored or treated as optional for `google` flavor
- **FR-007**: System MUST reject Google service account JSON credentials where `type` is not `"service_account"` with HTTP 400
- **FR-008**: System MUST reject Google service account JSON credentials that are not valid JSON with HTTP 400
- **FR-009**: System MUST return the `oauth2_flavor` value in all GET responses for third-party service configurations
- **FR-010**: System MUST continue to redact the credential field from all API responses regardless of flavor
- **FR-011**: System MUST allow updating the `oauth2_flavor` of an existing service; when the flavor changes, the new credential must satisfy the new flavor's validation rules
- **FR-012**: System MUST reject credentials for the `google` flavor that exceed 32 KB in size with HTTP 400
- **FR-013**: System MUST be designed to support additional flavors in the future without requiring changes to the API contract; the `oauth2_flavor` field is an extensible named string enum
- **FR-014**: System MUST validate that required string fields in a Google service account JSON (`private_key`, `client_email`, `token_uri`, `client_id`) are non-empty; it MUST NOT validate the private key's cryptographic format
- **FR-015**: For `oauth2_flavor: google`, `issuer_uri` is optional; if provided, the system MUST validate that its scheme and host are consistent with the scheme and host of the `token_uri` field in the service account JSON; a mismatch MUST be rejected with HTTP 400
- **FR-016**: For `oauth2_flavor: google`, when `issuer_uri` is omitted, the `token_uri` extracted from the service account JSON serves as the authoritative token endpoint
- **FR-017**: For `oauth2_flavor: google`, the `authorize_endpoint` on the stored entity MUST be set to the standard Google OAuth2 authorization URL (`https://accounts.google.com/o/oauth2/auth`); this field is not meaningful for service account flows but must be populated for storage consistency. Standard HTTPS format validation for this derived endpoint is not re-applied since the value is a well-known constant.
- **FR-018**: For `oauth2_flavor: google`, endpoint fields (`token_endpoint`, `authorize_endpoint`, `issuer_uri`) derived from the service account JSON are not subject to the caller-supplied HTTPS format validation applied to `standard` flavor endpoints; they are accepted as-is from the validated Google credential.

### Domain Model *(if applicable)*

**Value Objects** (things without identity):

- **OAuth2Flavor**: An enumeration identifying the authentication mechanism variant of a third-party service. Current values: `standard` (plain client secret), `google` (Google service account JSON key). Designed for extension without breaking changes.

- **ClientCredential**: The value carried by the `client_secret` field. The internal structure varies by flavor. For `standard`: a plain secret string. For `google`: a serialized Google service account JSON string. Always encrypted at rest; never exposed in plaintext via the API. The field name `client_secret` is preserved for API backward compatibility regardless of flavor.

- **GoogleServiceAccountKey**: A structured value object representing the contents of a Google service account JSON key file. Required fields: `type` (must be `"service_account"`), `private_key`, `client_email`, `token_uri`, `client_id`. Provides validation of structural completeness independently of cryptographic key validation.

*All domain terms should be added to ARCHITECTURE.md Glossary (per Constitution Principle V)*

### API Requirements

- **API-001**: The `oauth2_flavor` field MUST be added to the `ThirdPartyOAuth2Service` schema in `/api/admin/openapi.yaml` as a string enum with default `standard`
- **API-002**: API documentation MUST list all accepted `oauth2_flavor` values and describe the credential validation rules for each
- **API-003**: For `oauth2_flavor: google`, the API documentation MUST note that `client_id` is derived from the credential JSON and need not be provided in the request body
- **API-004**: Error responses for flavor-specific validation failures MUST include a `detail` field identifying which required credential fields are missing or invalid
- **API-005**: The `client_secret` field name MUST be preserved in the API schema (Google service account JSON is passed as its value, serialized as a string); API documentation MUST clarify flavor-specific semantics for this field
- **API-006**: For `oauth2_flavor: google`, the API documentation MUST note that `issuer_uri` is optional; if provided, it must be consistent with the `token_uri` in the service account JSON
- **API-007**: All API changes MUST be confirmed before implementation begins per Constitution Principle X

### Database Requirements

- **DB-001**: All database schema changes MUST be in `/migrations/` directory using go-migrate naming conventions
- **DB-002**: A new migration MUST add the `oauth2_flavor` column to the third-party OAuth2 services table with a default value of `standard`
- **DB-003**: The migration MUST NOT rename or modify the existing `client_secret` column; only the new `oauth2_flavor` column is added; an `up` and `down` migration MUST be provided
- **DB-004**: The migration MUST be tested: apply, rollback, and re-apply without data loss or errors; existing `standard` flavor services MUST be unaffected

### Security Requirements

- **SR-001**: Credentials for all flavors MUST be encrypted at rest using the existing encryption infrastructure (per Constitution Principle III and feature 20260114-120000)
- **SR-002**: Google service account private keys MUST NEVER appear in API responses, logs, audit trails, or error messages in any form
- **SR-003**: The system MUST NOT log the credential payload during validation — not even partial content or a prefix of the private key value
- **SR-004**: System MUST validate credential size limits (32 KB for Google JSON) to prevent memory exhaustion or denial-of-service attacks via oversized payloads
- **SR-005**: System MUST fail closed when credential validation fails — a service with an invalid or incomplete credential MUST NOT be persisted
- **SR-006**: When a flavor is changed on an existing service, the system MUST re-validate the full credential before committing the change; a request that changes the flavor without providing a valid credential for the new flavor MUST be rejected atomically

### Key Entities

- **ThirdPartyOAuth2Service** (extended): External OAuth2 provider configuration, extended with `oauth2_flavor` to identify the authentication variant. The credential field stores the flavor-appropriate authentication material (secret string for `standard`, service account JSON for `google`), always encrypted. The `client_id` is explicitly provided for `standard` flavor or automatically derived from the service account JSON for `google` flavor.

- **OAuth2Flavor**: Named enumeration value attached to a `ThirdPartyOAuth2Service` that determines credential validation rules and, in future, the token acquisition mechanism. Current values: `standard`, `google`.

*Domain concepts should be added to ARCHITECTURE.md Glossary (per Constitution Principle V)*

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Administrators can successfully configure a Google service account as a third-party service in under 3 minutes, including providing the service account JSON credential
- **SC-002**: Invalid Google service account JSON (missing required fields, wrong type, malformed JSON) is rejected with HTTP 400 and a clear error message identifying the specific validation failure 100% of the time
- **SC-003**: Existing third-party services configured without an explicit flavor continue to work without modification after the feature is deployed (zero regressions)
- **SC-004**: All supported flavor values and their credential validation rules are documented in the API specification before implementation begins
- **SC-005**: Adding a new OAuth2 flavor in the future requires changes only to the validation layer and documentation — no changes to the API contract or storage schema are required

## Assumptions

- The existing `client_secret` field in the `ThirdPartyOAuth2Service` domain model and API schema retains its name for backward compatibility; for the `google` flavor its value holds a Google service account JSON string rather than a plain secret
- Google service accounts are the only non-standard flavor required in this iteration; additional flavors are explicitly out of scope but the design accommodates them
- The actual execution of Google-specific OAuth2 token acquisition (using the private key to sign JWTs per RFC 7523) is deferred to a subsequent feature
- Credential encryption infrastructure from feature 20260114-120000 handles arbitrary-length string payloads sufficient for storing a service account JSON document (typically 2–4 KB)
- Existing third-party service integrations use the `standard` flavor and are unaffected by this change

## Out of Scope

- Executing OAuth2 token acquisition using Google service account credentials (JWT Bearer flow); this is a follow-up feature
- Validating the cryptographic validity or expiry of the private key in the Google service account JSON
- Support for Google Workspace domain-wide delegation configuration
- Support for other non-standard OAuth2 providers beyond Google in this iteration
- User-facing UI changes for selecting or configuring OAuth2 flavors

## Dependencies

- Encryption infrastructure (feature 20260114-120000) must support storing arbitrary-length string values including multi-KB JSON documents
- Admin API for third-party service management (feature 20251217-120000) must be extended; the existing API contract must be preserved for `standard` flavor services
- OpenAPI schema changes must be reviewed and approved before implementation begins (per Constitution Principle X)

## Future Enhancements

- Token acquisition support for Google flavor using JWT Bearer grant (RFC 7523) with service account private key signing
- Support for additional OAuth2 flavors: Azure AD workload identity, AWS Cognito app clients, Okta service principals
- Flavor-specific endpoint discovery behaviors (e.g., Google's `.well-known` endpoints differ from the standard OAuth2 metadata document)
