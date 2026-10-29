# Research: Wildcard Redirect URI Hosts

## Decision 1: Use the existing Public Suffix List implementation

**Decision**: Use `golang.org/x/net/publicsuffix` from the existing `golang.org/x/net v0.60.0` dependency to identify the public suffix and effective registrable domain of wildcard-host probes. Use `golang.org/x/net/idna` to convert DNS hostnames to lookup-form ASCII for classification and matching. Do not fetch a suffix list at runtime.

**Rationale**: A label-count heuristic cannot distinguish multi-label suffixes such as `co.uk` or private suffixes. The dependency is already present, and its Public Suffix List snapshot is bundled with the binary, so registration and authorization do not require network access. The security boundary is defined by PSL domain rules rather than a hard-coded suffix depth.

**Validation approach**:

1. Parse the registered URI and reject wildcard characters outside the HTTPS hostname.
2. Replace each `*` with a safe ASCII marker to create a probe hostname. Convert the probe with the IDNA lookup profile and validate its DNS labels.
3. Obtain the probe's public suffix and effective TLD plus one. Reject the wildcard registration if the boundary cannot be determined, the hostname is an IP literal or single-label name, or any wildcard appears in the effective registrable domain or its suffix.
4. Store and return the original registration string unchanged. Normalize the host only for matching; preserve exact matching of the URI's scheme, port, path, and query.
5. Match wildcard patterns label by label. Within a label, `*` matches zero or more characters, provided the resulting label is valid; it never matches a dot.

Include private PSL suffixes. Do not interpret `icann == false` as proof that no public suffix exists: the package also uses that result for private suffixes and unmanaged TLDs. A wildcard registration must fail closed if IDNA or PSL classification returns an error.

**Alternatives considered**:

- Hard-code the last one or two hostname labels: rejected because public suffixes can contain multiple labels and private suffixes.
- Download the PSL at runtime: rejected because redirect validation must not depend on network availability or mutable external state.
- Require only ICANN-managed suffixes: rejected because that would disallow private hosting suffixes even when their full boundary is fixed.
- Use regular expressions: rejected because a single-label wildcard matcher is simpler to constrain and audit.

## Decision 2: Keep wildcard matching scoped to opaque Agent callbacks

**Decision**: Add a matcher for HTTPS wildcard patterns in an Agent's `redirect_uris` and call it only for opaque Agent authorization. Keep the existing exact matcher for CIMD document callbacks and private-use callbacks. Continue to treat loopback HTTP registrations under their existing port-insensitive rule.

**Rationale**: The authorization service currently selects either Agent registrations or CIMD document registrations, then uses a shared matcher. Making that shared matcher wildcard-aware would unintentionally broaden a separate trust source. A source-specific matcher preserves the feature scope and existing ADR 042 guarantees.

**Alternatives considered**:

- Enable wildcard support in the shared `IsRegisteredRedirectURI` matcher: rejected because it would allow remote CIMD documents to declare wildcard callbacks.
- Change private-use matching: rejected because ADR 042 requires byte-for-byte matching and explicit Agent allowlisting.

## Decision 3: Keep registration and persistence contracts stable

**Decision**: Validate wildcard patterns through the existing Agent create/update validation path. Keep `redirect_uris` as the existing string list, persist and return the original string, and add no configuration, endpoint, or database field. Update the existing Agent and authorization OpenAPI descriptions.

**Rationale**: The feature changes which registered HTTPS strings are valid and how they match; it does not introduce a new resource or lifecycle. Validation at registration gives operators early feedback, while runtime revalidation protects authorization from invalid legacy stored values.

**Alternatives considered**:

- Add a separate pattern field or endpoint: rejected because the wildcard is a syntax in an existing callback registration.
- Add a database migration: rejected because the existing text representation stores the pattern unchanged.

## Decision 4: Record the security policy as an ADR

**Decision**: Add ADR 044 for the wildcard grammar, public-suffix boundary, source-specific matcher, and preserved exact-match cases. Update the `ARCHITECTURE.md` glossary for “Redirect URI Pattern”.

**Rationale**: Host wildcards change an authorization redirect trust boundary. The decision must be explicit, auditable, and consistent with ADR 042's exact private-use behavior.

## Sources

- [`golang.org/x/net/publicsuffix` v0.60.0 documentation](https://pkg.go.dev/golang.org/x/net@v0.60.0/publicsuffix)
- [`golang.org/x/net/idna` v0.60.0 documentation](https://pkg.go.dev/golang.org/x/net@v0.60.0/idna)
- `internal/domain/urivalidation/redirect.go` — current callback parsing and exact matching
- `internal/domain/urivalidation/cimd_pattern.go` — existing, separate CIMD path-pattern matcher
- `internal/domain/storage/agent.go` — Agent callback validation and unchanged persistence shape
- `internal/domain/oauth2/service.go` — current source selection for Agent and CIMD redirect registrations
- `adrs/042-agent-allowlisted-private-use-redirect-uris.md` — binding exact-match rules for private-use callbacks
