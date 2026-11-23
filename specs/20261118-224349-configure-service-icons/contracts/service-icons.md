# Service Icon Contract

**Stakeholder approval:** The stakeholder explicitly approved all outstanding decisions in this conversation: “I approve all things that still need approval.” This includes the API drafts, SVG upload/rendering boundary, authenticated JSON delivery, payload cost, CSP and rendering-resource scope. Admin `icon_svg`, consent `ServiceRequirementForUser.iconSvg`, session `service_icon_svg`, and omission/null/value semantics are approved. The [consent scope correction](https://github.com/zalando-incubator/agentic-identity-broker/pull/510#discussion_r4632042688) remains part of the approved contract. No separate `available_services` response or `ThirdpartyService` extension is required.

No further stakeholder sign-off is required for this design. Publish the approved schemas in both canonical OpenAPI documents before behavior changes. Include endpoint examples and existing authentication/error definitions. Record the approved architecture in an ADR with status Accepted. Security analysis, resolution of blocking findings, ADR authoring and implementation checks remain unfinished work; this approval does not claim their completion. Additive icon fields do not authorize unrelated changes to existing fields or API versioning rules.

## Admin API

Existing `POST /api/services`, `PUT /api/services/{service-id}`, and their detail/list reads gain `icon_svg`. Create/update input is optional and nullable. A non-null value must satisfy the SVG profile below. Omission creates no icon or preserves an existing icon; null creates no icon or clears an existing icon. Admin responses omit the field when absent and return the accepted source exactly when present. A JSON number, object, array, empty string, whitespace-only string, URL, or path is invalid.

Successful writes retain existing 201/200 status, response envelope, authorization, and concurrency requirements. Invalid icon input returns existing Admin 400 validation errors, for example `{"error":"validation failed","message":"icon_svg contains prohibited content"}`. Size and malformed-document failures use the existing validation error envelope without echoing the document. No partial update, discovery fetch, or referenced-resource fetch occurs for an invalid icon. Existing authentication, not-found, and concurrency error behavior is unchanged.

## End-user API

| Existing schema | New property | Consumers |
|---|---|---|
| `ServiceRequirementForUser` | `iconSvg` | Existing `data.services` entries, reused by service cards, requirement rows and permission-set service rows |
| `UserSessionSummary` | `service_icon_svg` | Session list and inherited `SessionStatus` details |

Example consent service excerpt:

```json
{"serviceId":"880e8400-e29b-41d4-a716-446655440010","serviceName":"Atlas Files","iconSvg":"<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\"><path fill=\"currentColor\" d=\"M4 4h16v16H4z\"/></svg>","requirementType":"mandatory","requiredScopes":[{"name":"files.read","description":"Read files"}],"connectionStatus":"connected"}
```

Session list/detail use the same source under `service_icon_svg`. Consent entries use `iconSvg` beside their existing `serviceId` and `serviceName`. These extend existing authenticated consent and `/api/third-party/sessions` responses; no separate endpoint is added. Detail responses and any refresh response returning a session summary use the same projection.

The server omits absent source. Clients accept omission or null and replace their prior metadata rather than merging a cleared field back from stale state. A new source replaces the previous image; identical source is identical content. No revision token, image cache lifetime, or obsolete-resource status is needed. Existing JSON caching/authentication rules apply; do not introduce shared/public caching. Unauthenticated requests receive the existing route's authentication failure and no icon bytes.

Permission-set rows already obtain service names from `data.services`. Carry `iconSvg` through that same frontend mapping. Do not emit `available_services` or repeat SVG source on permission-set scope records. Preserve the existing service filtering and response scope.

The unused canonical `ThirdpartyService` schema, including `logoUrl`, remains unchanged. It needs no icon property for this feature. Do not turn `logoUrl` into an SVG alias or remote fallback.

## Upload validation and rendering boundary

Browser image mode is the primary rendering boundary. Upload validation checks well-formed input and named active constructs, not all SVG/CSS capabilities. Accepted source is not safe for arbitrary inline/document use.

1. Measure after JSON decoding: valid UTF-8, nonempty, at most 65,536 bytes. Parse strict XML using standard encoding/xml without resource resolution. Require one root `svg` in the SVG namespace, properly nested elements and no trailing non-whitespace content. Reject malformed XML and duplicate attributes; accept ordinary comments, harmless metadata, aliases and visual content.
2. Permit at most one SVG DOCTYPE before the root, with an optional external PUBLIC or SYSTEM identifier and no internal subset. This includes standard SVG 1.1 exports. Reject internal subsets, entity declarations, other XML directives and processing instructions except a single initial UTF-8 XML declaration. Never resolve or fetch a DTD, install an external resolver or set a custom entity table. Unknown entity references still fail strict XML parsing.
3. For every parsed element, reject local names `script`, `a` and `foreignObject`, compared ASCII case-insensitively regardless of namespace. Reject event-handler attributes whose local names start with ASCII-insensitive `on`, including namespaced forms. Check root and descendants equally.
4. For decoded attribute values, reject a leading `javascript:` or `vbscript:` scheme after removing ASCII control/space characters and comparing ASCII-insensitively. This explicit check does not parse CSS or certify arbitrary attribute/animated values. The browser remains the primary boundary for those capabilities.
5. Do not implement an SVG geometry/value vocabulary, CSS sanitizer, reference graph, animation analyzer, media decoder or rendering-cost model. Preserve accepted bytes exactly. No third-party artwork, hash or local download is required.

External references in otherwise valid source can be accepted. The broker never fetches them; browser SVG image mode prevents their loads. Styles, local reuse, media and animation follow browser image-mode semantics. Tests must pair explicit rejections with harmless visual positives and must observe network/execution behavior in actual supported browsers.

Rendering-resource budgets and stress tests are out of scope by explicit stakeholder instruction. Input bounds are not a claim about browser CPU/GPU, memory, filter work or animation lifetime.

## Browser contract

Use only `<img src="data:image/svg+xml;charset=utf-8,...">` with URI-encoded exact source and empty alt text. Never interpolate SVG markup or permit object/embed/iframe/document delivery. Do not expose a navigable SVG endpoint. JSON authentication controls receipt of bytes, not arbitrary reuse of bytes an authorized client copies.

Preserve existing security headers. The approved built-SPA policy is `frame-ancestors 'none'; object-src 'none'`. Record its analysis in the Accepted ADR; no further stakeholder sign-off is required. Add no `img-src` or `default-src` restriction: existing remote user avatars and agent images must remain available. No new script/style permission is needed. Parent CSP does not supply the SVG image-mode guarantees; verify the built image context in Chromium and retain remote-avatar coverage.

Source-key only the image load/error state. Keep visible service names, original caller fallbacks and working controls during failure, replacement and clearing. Existing data refresh/cache lifetimes remain unchanged.

## Required security and contract cases

Original visual examples must cover styles, text, effects, local reuse, embedded media and declarative animation. Include an SVG 1.1 DOCTYPE without an internal subset as a positive case. Named active constructs, malformed XML, internal subsets, entity declarations, other directives and over-limit source must fail atomically, including alongside credential/metadata changes. Namespace and character-reference variants exercise the named checks.

Accepted external references in attributes, CSS and animated values must remain inert in `<img>`. Verify network and execution observables in the existing Chromium suite. Verify exact source in API round trips and decoded data URIs, plus `complete` and `naturalWidth > 0`. Retain meaningful-state screenshots without pixel comparisons or synchronized animation checks. Storage integration tests and backend Docker acceptance cover persistence and restart; the browser suite needs no restart harness. Also cover authentication, omit/null/value transitions, independent services and fresh-source recovery. No rendering-resource stress suite or third-party asset is required.
