# Research: Hybrid OAuth Server Modes

**Feature**: 20260511-120000-hybrid-oauth-modes | **Date**: 2026-05-12

## R1: Mode Strategy — Domain vs Adapter Boundary

**Decision**: Split mode concerns into two layers:

1. **Domain layer** (`internal/domain/oauth2/`): `ModeStrategy` with `AcceptsClientMode(ClientMode) bool`. Three implementations (proxy, local, hybrid) — all pure domain logic. This determines whether a classified agent is permitted in the active mode.

2. **Adapter layer** (`internal/adapters/http/enduser/`): Dispatching proceed/grant strategies for hybrid mode. These wrap both proxy and local proceed/grant strategies and delegate based on client mode. This is HTTP adapter code, wired by the builder.

**Rationale**: Accept/reject is domain logic — it depends only on client mode, not HTTP. Dispatching between proxy and local HTTP strategies is adapter code — it selects which HTTP handler path to follow. Keeping these separate ensures domain never references HTTP adapters, satisfying hexagonal architecture rules.

Config validation happens in the configuration layer (`OAuth2AuthServerConfig.Validate()`). After validation, the builder selects the domain `ModeStrategy` and wires the appropriate adapter strategies:
- `proxy` mode → proxy proceed + proxy grant strategies (existing)
- `local` mode → local proceed + local grant strategies (existing, renamed from issueToken)
- `hybrid` mode → dispatching proceed + dispatching grant strategies (new, delegate based on `agent.Class()`)

**Alternatives considered**:
- Single `ModeStrategy` interface spanning both accept/reject and dispatch — rejected because it forces domain code to know about HTTP proceed/grant strategies
- Port interface in `internal/ports/` — rejected because nothing external implements it; all implementations are internal domain logic
- Single strategy with mode field + runtime branching in handlers — rejected per FR-014

## R2: Agent Classification Location

**Decision**: Classification is a method on the Agent entity: `agent.ClientMode() ClientMode`. The `ClientMode` type is defined in `internal/domain/storage/` alongside the Agent entity.

**Rationale**: Classification derives entirely from Agent's own fields (`ClientID`, `ClientURIs`). It's natural to ask an agent "what client mode are you?" rather than passing the agent to an external classifier. The Agent entity already lives in domain code (`internal/domain/storage/`), so this keeps the logic co-located with the data it inspects.

**Alternatives considered**:
- Free function `ClassifyAgent(agent)` in `internal/domain/oauth2/` — rejected because it separates logic from the data it operates on; classification is intrinsic to the entity
- Classification in HTTP handler — rejected because it's domain logic, not HTTP concern

## R3: Config Restructuring Approach

**Decision**: Restructure `OAuth2AuthServerConfig` to have nested `Proxy` and `Local` sections. Mode validation ensures only relevant sections are populated. The flat fields (`UpstreamIssuerURI`, `TokenTTL`, etc.) move into their respective subsections.

**Rationale**: The spec requires each mode to have a dedicated config section (FR-015). Hybrid mode requires both sections (FR-016). Nested sections make cross-mode field ambiguity impossible at the structural level.

**Alternatives considered**:
- Keep flat structure with mode-based validation only — rejected because FR-015 explicitly requires dedicated sections per mode
- Separate config types per mode (union type) — adds complexity without benefit since Viper maps to a single struct

**Migration path**: Existing configs using flat fields (`upstream_issuer_uri` at top level) need to be updated. The `issue_token` → `local` rename happens in Phase 0 as a pure refactor.

## R4: Hybrid Mode Builder Wiring

**Decision**: In hybrid mode, the builder creates both local and proxy proceed/grant strategies, then wraps each pair in a dispatching strategy. The dispatching strategies live in `internal/adapters/http/enduser/` (adapter code) and use `agent.ClientMode()` to delegate to the correct inner strategy per request.

**Rationale**: The builder already creates one strategy set or the other based on mode (`internal/app/builder.go:609-653`). For hybrid, both sets are created and wrapped. The dispatching strategies are adapter code because they select between HTTP handler paths — domain code is not involved in dispatch. Agent classification (`agent.ClientMode()`) is the only domain call, and it returns a value object.

**Alternatives considered**:
- Two separate routers/mux for proxy and local paths — overengineered, requires URL path splitting that doesn't exist in the spec
- Domain-level dispatch — violates hexagonal architecture by making domain code aware of HTTP strategies

## R5: CIMD Agent UUID Rejection (FR-005)

**Decision**: Enforce in the client resolver: after resolving an agent by UUID, check if `len(agent.ClientURIs) > 0`. If so, reject with error "CIMD agents must be addressed via their URL-format client_id".

**Rationale**: FR-005 is explicit: UUID-format `client_id` resolving to a CIMD agent must be rejected. This prevents bypassing CIMD validation. The check belongs in the resolver since that's where resolution format and agent properties are both available.

**Alternatives considered**:
- Check in mode strategy — too late, classification should not run on improperly addressed agents
- Check in handler — domain logic belongs in domain layer

## R6: Deprecation Handling for `issue_token`

**Decision**: Config validation returns a clear error: `"mode 'issue_token' has been renamed to 'local' — please update your configuration"`. No silent fallback (SR-003).

**Rationale**: Explicit error with remediation guidance. The spec is clear: no silent migration.

## R7: ADR Decision

**Decision**: No new ADR. The spec, plan, and research documents already capture all architectural decisions (mode strategy pattern, client mode classification, config restructuring, mode renaming). ADR 20260328-120100 remains the binding ADR for local token issuance via fosite; this feature extends its patterns without changing the architectural foundation.

**Rationale**: An ADR would duplicate content already present in the spec artifacts. The spec + plan serve the same purpose — documenting decisions, rationale, and alternatives considered.
