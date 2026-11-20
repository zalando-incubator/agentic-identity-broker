# Research: Service Icon Delivery and SVG Security

## 1. Browser rendering boundary

**Decision:** Browser secure animated image mode is the primary boundary. Use bounded XML validation and named active-construct rejection at upload; retain accepted source exactly. Do not claim complete static detection of CSS, indirect-resource or animation capabilities.

**Rationale:** SVG 2 secure animated mode disables scripts, external references and interaction while permitting declarative animation. Same-document and data references are distinct from external references. This closely matches broad visual compatibility without implementing a browser parser.

**Evidence:** [SVG 2 processing modes and subresources](https://www.w3.org/TR/SVG2/conform.html#secure-animated-mode); [MDN SVG as an image](https://developer.mozilla.org/en-US/docs/Web/SVG/Guides/SVG_as_an_image).

**Alternatives considered:** A finite visual allowlist contradicts the request. Browser isolation alone would omit the requested explicit upload rejections. A full static capability analyzer would require substantially more XML/CSS/reference/animation semantics and has not been justified.

## 2. Library findings

**Decision:** Use standard Go XML/UTF-8 parsing and direct checks for the upload policy in [contracts/service-icons.md](contracts/service-icons.md). No SVG sanitizer dependency is needed. This is not a general-purpose sanitizer or security certification of source.

**Rationale:** Reviewed candidates do not satisfy the former full contract. [bluemonday](https://github.com/microcosm-cc/bluemonday) is an HTML allowlist sanitizer. [DOMPurify's threat model](https://github.com/cure53/DOMPurify/wiki/Security-Goals-%26-Threat-Model) excludes complete CSS and external-request prevention; its sanitized result, not inspection of removed items, is the security output. [svg-hush](https://github.com/cloudflare/svg-hush) rewrites a finite SVG vocabulary and allows same-origin paths. [PHP svg-sanitizer](https://github.com/darylldoyle/svg-sanitizer) documents best-effort CSS handling. These findings do not prove a complete validator is impossible; they do rule out an unsupported turnkey-library claim here.

**Alternatives considered:** Sanitized output violates exact-source retention. Comparing serialized sanitized output to original bytes rejects harmless formatting. Rasterization/resvg loses animation and changes the delivery contract. A server browser is unnecessary for the chosen image-mode design.

## 3. Input and rendering resource scope

**Decision:** Keep the existing 65,536 decoded UTF-8 byte maximum and basic non-resolving XML parsing. No filter-cost, frame-lifetime, reference-expansion or browser CPU/GPU/RAM budgets, stress suite or render-worker infrastructure are required.

**Rationale:** The stakeholder explicitly descoped renderer resource concerns for operator-supplied content. The byte limit is not a renderer quota. Expensive filters and ongoing animation remain possible; do not claim otherwise.

**Evidence:** [Filter Effects regions](https://drafts.csswg.org/filter-effects-1/#FilterEffectsRegion) depend on surfaces/transforms/resolution; [CSS infinite animation](https://www.w3.org/TR/css-animations-1/#animation-iteration-count) can continue indefinitely. Secure animated mode is a capability restriction, not a resource quota.

**Alternatives considered:** Fixed CSS image dimensions, async decoding, parser limits or an upload smoke render do not establish a portable lifetime resource bound. Hard runtime quotas or rendered derivatives would introduce different infrastructure and visual tradeoffs and are not selected.

## 4. Delivery and persistence

**Decision:** Embed exact source in existing authenticated JSON and encode it only into an img data source. No navigable SVG endpoint, public assets, provider discovery, remote fetch or operator image URL. The [contract](contracts/service-icons.md) records stakeholder approval of the API drafts and all current design decisions.

**Rationale:** Authentication already scopes metadata access. A source change changes the image input without a revision protocol. JSON text is not an SVG active document. Source is not safe for arbitrary downstream inline/document use; document this explicitly in API descriptions.

**Cost:** Repeated sources repeat payload bytes. One hundred maximum-sized occurrences total 6.25 MiB before JSON escaping/compression; URI encoding increases browser memory. No new performance promise or cache lifetime is introduced.

**Alternatives considered:** A protected endpoint adds direct-document security, authorization and cache lifecycle. Blob URLs require revocation ownership. Object storage adds an unnecessary asset subsystem.

## 5. Integration and governance

**Decision:** Existing service aggregate, presence-aware Admin handling, memory/PostgreSQL records, current consent/session projections and one service-specific frontend image composition. No new entity, repository, config, telemetry or infrastructure.

**Source evidence:** ServiceRequest.UnmarshalJSON retains request presence; provider prepareUpdate reads persisted state and applies existing field merges. SPA currently sets frame-ancestors only. Extend these paths, preserve row versions and add no icon-only mandatory If-Match. Permission-set rows derive names from existing data.services; carry iconSvg through that mapping without adding a catalog. AppLayout uses userInfo.pictureUrl, so a new same-origin-only img-src policy would break remote avatars.

**Rationale:** Session source must reflect the current service, never a session-creation snapshot. Key only image failure state by source so recovery does not remount actions. Existing consent/session caches remain authoritative; fresh-response observation is required.

**Remaining work:** Canonical API publication, security analysis and resolution of blocking findings, a timestamp-named ADR recording the approved design with status Accepted, glossary/architecture updates and test-first implementation. All stakeholder approvals are complete. No further sign-off is required; approval does not claim this work is complete.
