# Service Icons in Browser Image Mode

**Status**: Accepted
**Date**: 2026-11-19

## Context

Operators need service artwork without separate image deployments. SVG source is untrusted content, even when an authorized operator supplies it.

The stakeholder approved the API drafts and all current design decisions. The approval record is [service-icons.md](../specs/20261118-224349-configure-service-icons/contracts/service-icons.md). This decision records that approval, not completed runtime verification.

## Decision

The existing third-party service owns an optional Service Icon. Its value is absent or the exact accepted SVG source. It has no independent ID, repository, shared asset lifecycle, or session snapshot.

Upload validation uses Go's standard XML and UTF-8 libraries. Source must contain 1–65,536 decoded UTF-8 bytes and one SVG-namespace `svg` root. The parser consumes the entire document. Malformed XML, duplicate attributes, and non-whitespace text outside the root fail validation.

The named rejection policy applies to root and descendants, independent of namespace aliases and ASCII letter case. It rejects `script`, `a`, and `foreignObject` elements, `on`-prefixed attributes, and decoded attribute values with leading `javascript:` or `vbscript:` schemes after ASCII control/space removal. Validation errors contain fixed descriptions, never source.

One initial UTF-8 XML declaration and one pre-root SVG DOCTYPE are permitted. A DOCTYPE can have a PUBLIC or SYSTEM identifier but no internal subset. Other directives, entity declarations, and processing instructions fail validation. No custom entity table or external resolver is installed. The broker never fetches a DTD or SVG reference.

Existing Admin JSON accepts `icon_svg`: omission preserves an existing icon, null clears it, and a permitted string replaces it. Responses omit absent source. Existing consent `data.services` entries carry `iconSvg`. Session summaries carry current `service_icon_svg`, not a creation-time copy. Existing principal and ownership checks govern end-user responses. Admin authorization remains the deployment proxy's responsibility.

The SPA URI-encodes exact source into `data:image/svg+xml;charset=utf-8,...` and uses only `<img alt="">`. It never inserts source as markup or uses object, embed, iframe, or document delivery. No navigable SVG route exists. Browser secure animated image mode, not exhaustive source analysis, prevents scripts, interaction, and external resource loads. Styles, effects, text, local reuse, embedded media, and declarative animation remain permitted.

The built SPA keeps DENY and nosniff headers and uses `frame-ancestors 'none'; object-src 'none'`. It adds no `default-src` or `img-src` restriction. Existing remote user avatars and agent images keep their present behavior. Parent CSP does not supply SVG image isolation.

Only image state is keyed by source. Callers retain their original fallback, visible name, and controls during loading, errors, replacement, and clearing. Existing five-minute consent and two-minute session caches remain unchanged.

## Security analysis

Authenticated JSON controls receipt of source bytes, not reuse of copied bytes. Accepted source is not certified safe for inline SVG or active-document embedding. Consumers must preserve the image-only boundary.

External references can remain in accepted source. The broker does not resolve them. Chromium acceptance tests must observe network and execution behavior for attribute, CSS, and animated references in actual image mode. Exact API/data-URI source and completed images with positive natural width are separate observables.

The policy rejects named constructs without pretending to be a CSS sanitizer, geometry validator, reference graph, or animation analyzer. Missing references, duplicate IDs, and ignored visual values remain browser rendering concerns.

Rendering-resource budgets and stress tests are explicitly out of scope. Expensive filters and continuing animations remain possible. The source byte limit is not a CPU, GPU, memory, or animation-lifetime guarantee.

## Consequences

There is no new dependency, configuration, feature flag, asset server, or builder wiring. Both storage adapters retain source with existing service metadata. PostgreSQL uses nullable text and an atomic shape-check migration.

Repeated source increases JSON payload size. One hundred maximum-sized occurrences total 6.25 MiB before escaping and compression. URI encoding also increases browser memory. This accepted cost does not change cache lifetimes or create a new performance guarantee.

Validation, HTTP contract, migration, storage, projection, image-state, and browser tests protect the separate boundaries. A finite Chromium smoke check cannot prove arbitrary future rendering safety, bounded resource use, or safety outside image mode.

## References

- [SVG secure animated mode](https://www.w3.org/TR/SVG2/conform.html#secure-animated-mode)
- [MDN: SVG as an image](https://developer.mozilla.org/en-US/docs/Web/SVG/Guides/SVG_as_an_image)
- [004-storage-layer-architecture](004-storage-layer-architecture.md)
- [035-root-mounted-spa](035-root-mounted-spa.md)
- [Feature contract](../specs/20261118-224349-configure-service-icons/contracts/service-icons.md)
