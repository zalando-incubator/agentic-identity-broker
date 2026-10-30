# Feature Specification: Canonical Resource IDs

**Feature Branch**: `20260813-120000-canonical-resource-ids`

**Created**: 2026-08-13

**Status**: Draft

**Input**: User description: "Change the admin APIs to support canonical IDs alongside UUIDs: all resources support optional non-UUID canonical IDs; resource paths and write references accept either ID; canonical IDs are unique; read representations prefer canonical IDs."

## Clarifications

### Session 2026-08-18

- Q: Which canonical-ID grammar should the API accept? → A: Case-sensitive URL-safe token: `A–Z`, `a–z`, `0–9`, `.`, `_`, `-`; 1–128 characters; UUID-shaped values remain invalid.
- Q: Which contract should request canonical nested-reference presentation and signal whether it was applied? → A: Request `Prefer: reference-id=canonical`; return `Preference-Applied: reference-id=canonical` only when honored.
- Q: For PUT updates, how should `canonical_id` behave when it is omitted, null, or a string? → A: Omitted preserves the existing canonical ID; explicit null removes it; a valid string assigns or replaces it.



## User Scenarios & Testing *(mandatory)*

### User Story 1 - Manage Resources by Canonical ID (Priority: P1)

An administrator assigns a memorable canonical ID while creating or updating a managed resource, then retrieves, changes, and deletes that resource using either its canonical ID or its UUID.

**Why this priority**: Human-readable stable identifiers remove the need for administrators to discover and copy opaque UUIDs before performing routine management.

**Independent Test**: Create a resource with a canonical ID, perform each supported single-resource action once using the canonical ID and once using the UUID, and verify both identify the same resource.

**Acceptance Scenarios**:

1. **Given** an administrator creates a managed resource with a new valid canonical ID, **When** the resource is saved, **Then** it retains both its UUID and canonical ID.
2. **Given** a managed resource has a canonical ID, **When** an administrator addresses it by either its canonical ID or UUID, **Then** the requested read, update, or delete action applies to that same resource.
3. **Given** an administrator creates a managed resource without a canonical ID, **When** it is saved, **Then** the resource remains manageable by UUID and does not receive an implicit canonical ID.
4. **Given** a managed resource has a canonical ID, **When** an administrator updates it without `canonical_id`, **Then** the existing canonical ID is retained; an explicit `canonical_id: null` removes it, and a valid string assigns or replaces it.


---

### User Story 2 - Reference Resources by Either Identifier (Priority: P1)

An administrator configures a resource that refers to another managed resource and can provide the referenced resource's canonical ID or UUID.

**Why this priority**: Canonical IDs only reduce configuration effort if they can be used consistently within resource definitions, not solely in direct lookups.

**Independent Test**: Configure every supported cross-resource reference once with the target's UUID and once with its canonical ID, then verify both configurations resolve to the same target.

**Acceptance Scenarios**:

1. **Given** a target resource has a canonical ID, **When** an administrator submits a supported reference using that canonical ID, **Then** the system links the referencing resource to the target.
2. **Given** a target resource has a canonical ID, **When** an administrator submits a supported reference using the target's UUID, **Then** the system links the referencing resource to the same target.
3. **Given** a submitted reference identifies no target, **When** the administrator saves the resource, **Then** the change is rejected and no partial relationship is persisted.

---

### User Story 3 - Receive Readable Resource Representations (Priority: P2)

An administrator reads or lists managed resources and, for each resource, receives its stable UUID `id` plus its `canonical_id` when one is assigned; by default nested references are returned as target UUIDs, and a resource's `id` never changes form between reads. A client MAY additionally request canonical presentation of nested references.

**Why this priority**: Readable output lets administrators reuse displayed identifiers directly in follow-up management and configuration tasks.

**Independent Test**: Read and list resources with and without canonical IDs, including resource relationships, and verify each resource's `id` is always its UUID, `canonical_id` is present only when assigned, every nested reference is the target's UUID by default, and requesting canonical presentation renders nested references as canonical where the target has one.

**Acceptance Scenarios**:

1. **Given** a resource has a canonical ID, **When** it is returned in a read representation, **Then** its `id` and every nested reference identifier are UUIDs, and its `canonical_id` attribute carries the canonical ID.
2. **Given** a resource has no canonical ID, **When** it is returned in a read representation, **Then** its `id` and every nested reference identifier are UUIDs and its `canonical_id` is null.
3. **Given** a resource whose references have canonical IDs, **When** it is read with `Prefer: reference-id=canonical` and the preference is honored, **Then** each nested reference identifier is the target's canonical ID, the resource's own `id` remains its UUID, and the response includes `Preference-Applied: reference-id=canonical`.


---

### Edge Cases

- A supplied canonical ID exactly matches the UUID syntax and is rejected, even if no resource currently uses that value.
- A supplied canonical ID duplicates one already assigned to the same canonical-ID-enabled managed-resource type and is rejected without altering existing data.
- A path identifier or write reference matches neither a UUID nor an existing canonical ID; the operation reports that the target cannot be found.
- Updating a resource with `canonical_id: null` removes its canonical ID, while omitting `canonical_id` preserves the existing value; removal leaves UUID-based addressing and references valid, subsequent reads return a null `canonical_id`, and the resource's `id` is unchanged.

- Deleting a resource through either identifier has the same relationship and validation behavior as deletion through the other identifier.
- A read request that omits `Prefer: reference-id=canonical`, sends an unrecognized preference, or is not honored returns UUID identifiers—the pre-feature default—and omits `Preference-Applied: reference-id=canonical`.


## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The administrative interface MUST allow every UUID-backed managed resource—agents, third-party OAuth2 services, and permission sets—to carry an optional `canonical_id` alongside its UUID.
- **FR-002**: A supplied canonical ID MUST be a case-sensitive URL-safe token of 1–128 characters using only `A–Z`, `a–z`, `0–9`, `.`, `_`, and `-`, and MUST NOT be UUID-shaped.

- **FR-003**: The system MUST enforce canonical-ID uniqueness within each canonical-ID-enabled managed-resource type.
- **FR-004**: The system MUST reject creating or updating a resource when its supplied canonical ID is UUID-shaped or already assigned to another resource of the same type, without changing existing resource data.
- **FR-005**: Every supported administrative resource path that currently accepts a UUID for an in-scope resource MUST accept either that resource's UUID or its canonical ID and resolve both forms to the same resource.
- **FR-006**: The system MUST accept either a target UUID or canonical ID in each supported cross-resource write reference: an agent's `service_requirements[].service_id`, an agent's `permission_sets[].permission_set_id`, and a permission set's `service_scopes[].service_id`.
- **FR-007**: The system MUST reject a write when any supplied resource reference cannot be resolved to an eligible target, and MUST not persist a partial change.
- **FR-008**: Read and list representations MUST return each in-scope resource's UUID as its stable `id` and include its optional `canonical_id` as a distinct, nullable attribute.
- **FR-009**: By default—absent an honored FR-011 presentation preference—identifier values in read representations MUST remain stable UUIDs: each in-scope resource's `id` and each supported nested reference—`service_requirements[].service_id`, `permission_sets[].permission_set_id`, and `service_scopes[].service_id`—MUST be the target's UUID whether or not that target has a canonical ID. A resource's own `id` MUST remain its UUID in every representation; a canonical ID is otherwise exposed through the owning resource's `canonical_id` attribute.
- **FR-010**: Resources without a canonical ID MUST remain fully manageable and referenceable by UUID, preserving existing UUID-based workflows.
- **FR-011**: A read or list request MAY send `Prefer: reference-id=canonical` to request canonical presentation of nested reference identifiers. When honored, each supported nested reference—`service_requirements[].service_id`, `permission_sets[].permission_set_id`, and `service_scopes[].service_id`—renders as the target's canonical ID where one exists and its UUID otherwise, every rendered canonical value remains valid input to the corresponding write reference, and the response MUST include `Preference-Applied: reference-id=canonical`. When the preference is absent, unrecognized, or not honored, nested references remain UUIDs per FR-009 and the response MUST omit that `Preference-Applied` value. A resource's own `id` stays its UUID in all cases; the top-level canonical ID remains available through `canonical_id`. Clients that send no preference are unaffected, and human-facing clients SHOULD request canonical presentation by default.
- **FR-012**: For an in-scope resource's `PUT` update, omitting optional `canonical_id` MUST preserve its existing value; an explicit `canonical_id: null` MUST remove it; and a valid canonical-ID string MUST assign or replace it.



### Key Entities

- **Canonical ID**: An optional, human-selected, per-resource-type unique non-UUID identifier assigned to a UUID-backed managed resource.
- **Managed Resource**: An administrator-managed agent, third-party OAuth2 service, or permission set that retains its UUID and may have a canonical ID.
- **Resource Reference**: An identifier within a writable managed-resource definition that selects another managed resource by UUID or canonical ID.
- **Stable Identifier**: A resource's UUID, returned unchanged as `id` in every read representation regardless of canonical-ID assignment; nested references likewise present the target's UUID.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Administrators can complete 100% of supported single-resource management actions for an in-scope resource by using either its UUID or its assigned canonical ID.
- **SC-002**: 100% of supported cross-resource write references resolve identically whether the target is supplied by UUID or canonical ID.
- **SC-003**: 100% of attempted duplicate or UUID-shaped canonical IDs are rejected without modifying an existing resource.
- **SC-004**: In the default representation (no canonical-presentation preference), 100% of read and list representations return the stable UUID as each in-scope resource's `id` and every nested reference identifier, and expose the `canonical_id` attribute for each resource that has one; a resource's `id` never changes form between reads.
- **SC-005**: Administrators can identify and reuse a displayed canonical ID for a follow-up management action without separately looking up the resource's UUID.
- **SC-006**: 100% of reads that request and are granted canonical presentation with `Prefer: reference-id=canonical` render each nested reference with a canonical target as its canonical ID and return `Preference-Applied: reference-id=canonical`, while 100% of reads that omit, send an unrecognized, or are denied that preference return the pre-feature UUID representation without that response header.

## Assumptions

- The scope is the current UUID-backed administrative resource types: agents, third-party OAuth2 services, and permission sets. Health checks, generated client credentials, and signing-key operations are excluded because they are not UUID-backed managed resources.
- Canonical IDs are case-sensitive opaque URL-safe identifiers of 1–128 characters using only `A–Z`, `a–z`, `0–9`, `.`, `_`, and `-`; no normalization beyond rejecting UUID-shaped values is implied by this feature.
- Canonical IDs are unique within each in-scope resource type; different types may reuse the same canonical ID.
- Existing resources and clients that use only UUIDs remain supported without migration or required canonical-ID assignment.
- In `PUT` updates, optional `canonical_id` follows the administrative API's established three-state field behavior: omitted preserves, explicit null clears, and a valid string assigns or replaces.
- The requesting user is the API stakeholder confirming this administrative API contract change.
