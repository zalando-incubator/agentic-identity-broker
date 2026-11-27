# Capability Model for Permission Sets and Capability-Based Policy Input

**Status**: Proposed
**Date**: 2026-11-27
**Feature**: `20261127-140720-capability-model` (issue #579, part 1 of #285)
**Extends**: ADR `048-authzen-delegation-pip` §§ 4–5 (adds one permit member and one enabled-mode OPA input member; supersedes no decision)

## Context

Permission sets bundle OAuth2 scopes per service. A user grant records, for each set, which of the set's services the user included. Policy can therefore distinguish services but not what an agent does with a service. An operator cannot express "may read calendar events but not create them" in a grant, and ExtProc policy cannot tell which tool a grant covers.

Issue #285 asks for read-only mode and capability-level choice on the consent screen. Both need a unit that is finer than a service, that the operator names, and that policy can match. Feature 1 (#579) adds that unit without any user-visible change. Feature 2 (#580) adds the consent changes.

ADR `048-authzen-delegation-pip` makes the AuthZEN permit the source of enabled-mode policy context and deprecates `TokenExchangeResponse.granted_permission_sets`. ADR `029-token-exchange-client-assertion-trust-anchor` is Proposed, so the broker cannot yet prove that a token-exchange caller is the gateway.

## Decision

1. **Capability and Capability Group.**
   - A capability group is an aggregate root that owns capabilities.
   - A capability has a required, globally unique canonical ID (canonical-ID format), a label, a description, a `read_only` flag, and at least one service scope `{service_id, scopes[]}`.
   - Its canonical ID is the policy key and never replaces its UUID in storage or references.
2. **Composition.**
   - A permission set includes one or more capability entries `{capability_id, requirement_type}`, with no upper bound. These entries replace its service scopes.
   - An agent's permission-set entry may list a subset of the set's entries. It may tighten optional to mandatory but never relax mandatory to optional. A relaxing entry is rejected.
   - An agent entry without a list means every set entry.
   - A capability is mandatory for an agent when either entry says so.
3. **Referential protection.**
   - A capability that a set includes or an active grant references cannot be deleted (`409`).
   - A set update that would invalidate an agent entry is rejected (`409`).
   - A set update may drop a capability that grants hold. Token exchange and permit resolution for those grants then fail closed with the existing missing-permission-set response and consent URL.
   - A granted capability that the agent entry no longer lists fails closed with the existing undeclared-set response.
4. **Grants.**
   - Each granted set entry stores explicit granted capability IDs. A Full-intent marker is left to #580.
   - Until #580, the end-user grant request keeps its included-service shape. The broker stores every effective capability whose services are all included. It rejects the request when a mandatory capability needs an excluded service, or when an included service would yield no capability.
5. **Token exchange.** A service's scopes are the union of the granted capabilities' scopes for that service, capped by the agent's service-requirement ceiling as today.
6. **Policy input.**
   - The AuthZEN permit gains the required member `context.granted_capabilities`: the sorted, de-duplicated canonical IDs of all granted capabilities.
   - Enabled-mode ExtProc copies it to `input.context.granted_capabilities`.
   - The token-exchange response does not change.
   - Disabled-mode input stays unchanged byte for byte and carries no capability list.
   - Every existing ADR 048 member keeps its meaning.
   - An ExtProc that receives a permit without the member fails closed. The broker is upgraded first.
7. **Migration.**
   - Atomic migration `050` converts each permission set into a group of the same name, with one capability per former service scope. Each capability keeps its scopes and requirement type and gets `read_only: false`.
   - Each grant's included services are rewritten to the matching capability IDs, so no grant is broadened.
   - Canonical IDs are `<set key>-<service key>`, with a deterministic `cap-<md5>` fallback.
   - The down migration refuses data whose rollback would change what a grant allows.
8. **APIs.**
   - Admin API `4.0.0` adds `/api/capability-groups`, replaces permission-set `service_scopes` with `capabilities[]` (old requests return `400`) and adds agent-entry `capabilities[]`.
   - End-user API `1.4.0` adds the permit member.
9. **Enforcement boundary.**
   - Capabilities narrow OAuth2 scopes in the broker. Tool-level enforcement happens only in ExtProc policy.
   - Deployments that rely on capabilities MUST meet three controls:
     - enable `grant_context.enabled` and `authorization.enabled` with a deny-by-default policy;
     - register only the gateway's ExtProc client as the privileged token-exchange client;
     - keep agents away from the broker token endpoint and from upstream provider APIs by network policy.
   - The broker cannot verify these controls until an accepted successor to ADR `029-token-exchange-client-assertion-trust-anchor` exists.

## Consequences

- Operators can write Rego that maps each MCP tool to one capability and test it before users can narrow.
- Admin consumers must migrate to Admin API 4.0.0. Migrated permission sets keep working without changes, but their generated capabilities are named after services, not behaviors, until operators remodel them.
- Capability-based policy needs grant-context mode, and therefore an extra broker call per permit cache miss (ADR 048 freshness of 60 seconds or less).
- A grant can become unusable when an operator drops a granted capability from a set. The user is sent to the consent page, as for a deleted set.
- #580 can add capability selection and Full intent without another change to policy input.

## Alternatives considered

- **Inclusion mask or re-consent for legacy grants.** These were rejected in clarification. A mask keeps a second scope model alive, and re-consent interrupts every user.
- **Capability list in the token-exchange response.** Rejected: it extends a deprecated field's response and duplicates the ADR 048 source.
- **Optional permit member.** Rejected: a version mismatch would silently drop all capabilities.
- **New denial codes for stale capabilities.** Rejected: they change the AuthZEN denial enum without a user-visible need.
- **Unguarded down migration.** Rejected: merging capabilities back into services would broaden narrowed grants.
