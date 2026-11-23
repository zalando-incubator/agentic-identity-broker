# Data Model: Service Icons

## Aggregate and value

The existing third-party service owns an optional **Service Icon**: an absent value or the exact validated SVG document. There is no icon ID, repository, shared mutable asset, user ownership, or independent lifecycle. Add this term and relationship to the `ARCHITECTURE.md` glossary before implementation. Existing typed service IDs remain unchanged.

| Model | Addition | Invariant |
|---|---|---|
| Third-party service | Optional SVG string | Absent or permitted source, never an empty string |
| Admin create/update input | Presence + nullable SVG string | Three distinct states: absent, null, value |
| Memory service record | Optional SVG string | Copy/round-trip with existing service metadata |
| PostgreSQL service row | Nullable `icon_svg` text | NULL means absent; decoded UTF-8 source is 1–65,536 bytes |
| Consent service/requirement projection | Optional `iconSvg` | Current source on existing `ServiceRequirementForUser` entries, reused by permission-set rows |
| Session summary/detail projection | Optional `service_icon_svg` | Current referenced service source, not a session snapshot |

`contracts/service-icons.md` defines named XML rejection checks and external JSON semantics. Database checks protect null/nonempty/size shape; application validation enforces the input contract. SQL `octet_length` measures bytes, not characters.

Validation uses the named XML checks in `contracts/service-icons.md`, not a complete SVG/CSS sanitizer. The service owns the exact accepted source, including inert references. Browser image mode is its only supported rendering context. No parser-derived sanitized copy, rendering certification, reference-graph state or renderer-budget fields are stored. Invalid input fails before any service mutation.

## Write transitions

| Input | New service | Existing service |
|---|---|---|
| Omitted | Absent | Preserve previous source |
| Null | Absent | Clear source |
| Valid document | Store exact document | Replace with exact document |
| Invalid value/type | Reject | Reject, retaining the entire previous service |

Parse field presence in the HTTP adapter; enforce the source invariant in the existing domain service. Reuse the repository's presence-aware update convention if available. Do not spread JSON parsing into storage or infer presence from a pointer alone. Construct an independently validated candidate before mutating stored state. In particular, do not mutate a pointer returned by memory storage and then validate it. Validation precedes external discovery, credential transformations with side effects, and writes. Existing optimistic concurrency and request preconditions continue to apply to icon edits.

Reuse `ServiceRequest.UnmarshalJSON`'s existing presence-check convention. Preserve/clear/replace must be applied in the actual provider `prepareUpdate`/`Update`, not only `ValidateUpdate`, which validates a copy. Clear transient request flags before memory persistence. The provider entity lives in `internal/domain/model/thirdparty_oauth2_provider.go`; the PostgreSQL table is `thirdparty_oauth2_services`. Existing client `If-Match` requirements for `protected_resources` replacement must not be extended to icon-only writes.

## Persistence design

Extend the current service table with a nullable text column and a check for nonempty sources within 65,536 UTF-8 bytes. Use an atomic `NNN_service_icon_svg.up.sql`/`.down.sql` pair; the inspected migration sequence ends at `048`, so `049` is the candidate only if still unused at implementation time. The up migration adds no backfill, timestamp update, identifier change, or default icon. The down migration drops only the new check and column. Losing configured icons on rollback is intentional; credentials and all other service data must survive.

Update both storage adapter record conversions, all explicit service INSERT/UPDATE/SELECT/RETURNING column lists, and any bulk/list projection that constructs service metadata. Keep repository interfaces unchanged unless a demonstrated existing operation cannot express the update. Session projections obtain the source through their current service join/lookup; no session schema migration is needed. Avoid adding one database query per session or consent occurrence.

Integration tests use the production migration chain and real PostgreSQL to prove upgrade of old rows, configured round-trips, reconnect/restart durability, down/up repetition, and preservation of unrelated data. Memory tests prove equivalent field semantics and that two services with identical documents remain independent.

## Presentation state

Presentation state is keyed by the accepted source: absent, loading, rendered, or failed. An absent or failed source shows the caller's original fallback. A different source creates a fresh load attempt; removal discards the old image and failure state. Image events never alter connection status, grants, session state, or stored source. Already-open views change only when their existing data-refresh mechanism supplies fresh metadata; there is no new push channel.
