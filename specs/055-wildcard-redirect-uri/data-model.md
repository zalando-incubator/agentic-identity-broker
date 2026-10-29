# Data Model: Wildcard Redirect URI Hosts

## Existing Entity: Agent

The feature uses the existing Agent entity. It does not add an entity, identifier, repository, or state transition.

| Field | Type | Role | Persistence |
|---|---|---|---|
| `redirect_uris` | list of strings | Exact callback registrations and HTTPS host-wildcard registrations | Existing JSON/API field and existing PostgreSQL/memory representation |

### Registration Invariants

- A wildcard registration is an HTTPS URI with `*` only in one or more hostname labels.
- All non-host URI components remain fixed and follow existing redirect URI validation.
- Each `*` matches zero or more characters within one label. The resulting label must be valid; a wildcard never matches a dot.
- The effective registrable domain and its public suffix contain no wildcard labels.
- For wildcard registrations, the probe hostname must be classifiable through IDNA lookup conversion and the Public Suffix List. Reject wildcard patterns whose probe hostname is an IP literal, a single-label name, or otherwise unclassifiable. Exact HTTPS callback host validation remains unchanged.
- The original registration string is stored and returned unchanged. Normalized host data is used only while validating or matching.
- Exact HTTPS callbacks, loopback HTTP callbacks, and private-use callbacks retain their current rules.
- Agent `redirect_uris` do not make a CIMD document's web callbacks wildcard-capable. CIMD document callbacks remain exact.

## Authorization Request Value

The OAuth2 authorization request's `redirect_uri` is a concrete callback, not a pattern. It is parsed and validated under existing URI rules, matched against the selected registration source, and used verbatim as the redirect destination after a successful match. No authorization result is sent to the registered wildcard string.

## Relationships and Lifecycle

- An Agent owns zero or more registered callback strings.
- An opaque Agent authorization request is matched against that Agent's callback list.
- A CIMD authorization request is matched against the fetched document's callback list using exact matching; private-use callbacks also retain their Agent allowlist requirement.
- Registration validation runs on Agent create and update. Authorization matching also rejects malformed patterns defensively, including legacy stored values.

## Persistence Impact

No schema migration, new column, or data backfill is required. Existing exact callback values remain valid and unchanged.
