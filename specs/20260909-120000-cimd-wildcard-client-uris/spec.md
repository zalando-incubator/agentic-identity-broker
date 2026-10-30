# Feature Specification: Wildcard CIMD Client URI Registration

**Feature Branch**: `feat/cimd-wildcard-client-uris`
**Created**: 2026-09-09
**Status**: Draft
**Extends**: [20260421-120000-cimd-support/spec.md](../20260421-120000-cimd-support/spec.md)

## Overview

Some agent platforms create a unique Client ID Metadata Document (CIMD) URL for each agent installation. Operators cannot register each future URL before use.

This feature lets an operator register a controlled URI pattern on one Agent. For example, `https://chatgpt.com/oauth/codex/*/client.json` matches `https://chatgpt.com/oauth/codex/dIwd44EtAHp-/client.json`.

This feature changes 028 FR-026. It supersedes 028's “Exact match only” clarification for `client_uris` registration lookup. All other CIMD requirements remain in force.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Register a hosted CIMD URI pattern (Priority: P1)

An operator registers one Agent with `https://chatgpt.com/oauth/codex/*/client.json`. A hosted agent sends a concrete URL with one installation identifier in that path position. The broker resolves the registered Agent and continues the authorization flow.

**Independent Test**: Register the pattern. Send an authorization request with `https://chatgpt.com/oauth/codex/dIwd44EtAHp-/client.json`. The request reaches consent.

**Acceptance Scenarios**:

1. **Given** an Agent has `https://chatgpt.com/oauth/codex/*/client.json` in `client_uris`, **When** the request uses `https://chatgpt.com/oauth/codex/dIwd44EtAHp-/client.json` as `client_id`, **Then** the broker resolves that Agent.
2. **Given** that pattern, **When** the request uses `https://chatgpt.com/oauth/codex/dIwd44EtAHp-/nested/client.json`, **Then** the broker rejects the request with `invalid_client`.
3. **Given** that pattern, **When** the request uses `https://chatgpt.com/oauth/other/dIwd44EtAHp-/client.json`, **Then** the broker rejects the request with `invalid_client`.

### User Story 2 — Preserve literal registration precedence (Priority: P1)

An operator registers a literal URL and a matching pattern on different Agents. The broker resolves the literal URL to the Agent with the literal registration.

**Independent Test**: Register both values on different Agents. Send the literal URL as `client_id`. The broker resolves the Agent with the literal registration.

**Acceptance Scenarios**:

1. **Given** one Agent has `https://chatgpt.com/oauth/codex/literal/client.json` and another has `https://chatgpt.com/oauth/codex/*/client.json` in `client_uris`, **When** the request uses `https://chatgpt.com/oauth/codex/literal/client.json` as `client_id`, **Then** the broker resolves the Agent with the literal registration.

### User Story 3 — Reject ambiguous pattern matches (Priority: P1)

Two Agents have different patterns that match the same concrete URL. The broker rejects the request instead of selecting an Agent.

**Independent Test**: Register two overlapping patterns on separate Agents. Send one matching concrete URL. The broker returns `invalid_client` and records an audit event.

**Acceptance Scenarios**:

1. **Given** one Agent has `https://chatgpt.com/oauth/*/foo/client.json` and another has `https://chatgpt.com/oauth/test/*/client.json` in `client_uris`, **When** the request uses `https://chatgpt.com/oauth/test/foo/client.json` as `client_id`, **Then** the broker rejects the request with `invalid_client` and records an audit event.

## Edge Cases

- A wildcard replaces one non-empty path segment. It never matches `/`.
- A wildcard cannot appear in the scheme, userinfo, host, port, query, or fragment.
- A client URI path that contains a literal `\`, `%2F`, or `%5C`, in any hexadecimal case for
  escapes, is rejected. This stops servers from decoding one matched segment into multiple routes.
- A client ID that contains `*` is rejected. Pattern text is never a valid incoming client ID.
- A stored value with a literal `*` is interpreted as a pattern. Such a value cannot work as a literal client ID.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The agent write API MUST accept a valid CIMD URI pattern in `client_uris`.
- **FR-002**: A `*` in a valid CIMD URI pattern MUST replace exactly one non-empty path segment.
- **FR-003**: A `*` MUST occupy an entire path segment. The system MUST reject partial segments that contain `*`.
- **FR-004**: The system MUST reject a `*` outside the path component.
- **FR-006**: The system MUST use an exact registered URI before it evaluates URI patterns.
- **FR-007**: If patterns registered to two or more distinct Agents match a concrete client ID, the system MUST reject the request with `invalid_client`.
- **FR-008**: The system MUST reject a literal `\`, `%2F`, and `%5C` in the path of both a registered client URI and an incoming client ID. The hexadecimal escape check is case-insensitive.
- **FR-009**: The system MUST reject an incoming client ID that contains `*`.
- **FR-010**: The system MUST preserve global uniqueness for the literal text of each registered `client_uri`.
- **FR-011**: The system MUST apply the existing CIMD URL, document, redirect URI, SSRF, and cache requirements to the concrete client ID URL.

### Domain Model

**CIMD Client URI Pattern**: A pre-registered CIMD URI that includes one or more `*` path segments. A pattern identifies a controlled set of concrete CIMD URLs for one Agent.

## Security Requirements

- **SR-001**: The system MUST compare literal URI components without normalization or case folding.
- **SR-002**: The system MUST reject literal and encoded path separators before wildcard matching.
- **SR-003**: Wildcards MUST NOT affect the scheme, host, or port. The consent screen therefore continues to show a literal verified domain.
- **SR-004**: An ambiguous pattern match MUST create a structured audit event without exposing the matching registrations to the client.
- **SR-005**: The fetched document `client_id` MUST still equal the concrete fetched URL byte-for-byte, as required by 028 SR-008.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A pattern for `https://chatgpt.com/oauth/codex/*/client.json` completes a CIMD authorization flow for a concrete installation URL.
- **SC-002**: A URI that differs from a pattern outside a wildcard segment is rejected with `invalid_client`.
- **SC-003**: A URI with an extra path segment is rejected with `invalid_client`.
- **SC-004**: A literal registration continues to take precedence over a matching pattern.
- **SC-005**: Two matching patterns on distinct Agents produce `invalid_client` and an audit event.
- **SC-006**: Literal `\`, encoded `/`, and encoded `\` in client URI paths are rejected for registration and authorization.
- **SC-007**: Existing exact-match CIMD authorization flows continue to pass.
## Assumptions

- Operators register patterns only for URL namespaces that the platform controls.
- This feature adds no configuration setting. Pattern registration is an explicit administrative action.
- This feature does not support host wildcards, `**`, or partial-segment wildcards.

## Clarifications

### Session 2026-09-09

- Q: Can a wildcard match a nested path? → A: No. `*` matches one complete non-empty path segment only.
- Q: What happens when an exact URI and a pattern both match? → A: The exact URI wins.
- Q: What happens when patterns on multiple Agents match? → A: The broker fails closed with `invalid_client` and records an audit event.
- Q: Are path separators safe to match as one segment? → A: No. The broker rejects literal `\`, `%2F`, and `%5C` in URI paths before matching.

## References

- [20260421-120000-cimd-support/spec.md](../20260421-120000-cimd-support/spec.md) — baseline specification
- [ADR 20260425-120000: CIMD Fetcher Architecture](../../adrs/20260425-120000-cimd-fetcher-architecture.md) — fetch and SSRF controls
