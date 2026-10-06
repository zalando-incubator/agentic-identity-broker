# ADR 035: Root-Mounted SPA

**Status**: Accepted
**Date**: 2026-09-04
**Extended by**: [Accepted ADR 038](038-design-system-rebuilt-on-shadcn-radix.md), amended 2026-10-04. Its amendment replaces the historical `/delegations` and `/sessions` view routes with `/agents` and `/connections`, adds `/approvals/remembered` and `/settings/appearance`, and routes `/` to `/agents`. The root-mounted SPA and protocol precedence in this ADR remain in force.

---

## Context

The consent SPA must provide a stable, directly addressable path for agent delegations while keeping other browser views as first-class root paths. The root URL remains a convenient browser entry point without duplicating the consent view's canonical URL. Protocol namespaces remain outside the browser route.

## Decision

Serve the SPA at `/*`. Configure React Router with `basename="/"`, Vite with `base: '/'`, and Vite output at `dist`.

The canonical browser view routes are:

- `/delegations` for agent delegations
- `/agents/:id` for agent details
- `/sessions` for third-party sessions
- `/approvals` for tool approval decisions
- `/approvals/:id` for approval review

React Router redirects `/` to `/delegations` with history replacement. The redirect is client-side so Vite development serving and broker production serving have the same entry behavior. The server continues to serve the SPA for `GET` and `HEAD` browser requests, including `/` and `/delegations`; it does not register an HTTP redirect for `/`.

`/api/*`, `/oauth2/*`, `/.well-known/*`, and `/health` take precedence over the SPA handler. The top-level agent delegations navigation links to `/delegations`. `/api/consent/*` remains unchanged.

The broker serves `web/dist` from its working directory. It has no `spa.*` runtime configuration.

## Consequences

### Positive

Direct navigation to `/delegations` renders the consent view. Each top-level browser view has its own canonical path, and root navigation leaves one history entry for the canonical delegation URL.

### Negative

Browser clients must load the SPA before the root URL changes to `/delegations`.

---

## References

- Supersedes the serving portion of [ADR 005: SPA Serving Pattern](005-spa-serving-pattern.md)
- Related: [ADR 006: Frontend Technology Stack](006-frontend-stack.md)
- Constitution: `.specify/memory/constitution.md` — Principles II, IV, and XIII
