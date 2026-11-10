# ADR 048: Consent UI Design Language

**Status**: Proposed
**Date**: 2026-11-09
**Supersedes**: The "Refined Trust Architecture" visual direction previously documented in `web/src/design-system/docs/DESIGN_PRINCIPLES.md`

## Context

Constitution Principle XI documents the consent UI's visual direction in
`web/src/design-system/docs/DESIGN_PRINCIPLES.md` and requires an ADR to change it. The direction in
force until now was "Refined Trust Architecture": Crimson Pro serif headings at weight 700, Manrope
body text, a navy brand color, an amber call to action, warm cream and taupe neutrals, warm
gradient page backgrounds, and cards that floated on multi-layer shadows and lifted on hover.

The public docs site (`assets/docusaurus/`) has since adopted its own design language: a cool ink
neutral ramp, one jade accent reserved for actions, links and the granted state, Geist and Geist
Mono self-hosted, hairlines instead of shadows, no gradients or glows, and one easing curve. Its
homepage draws the consent screen in code. That drawing and the real consent SPA no longer looked
like the same product.

The frontend already expresses color as semantic tokens in one `@theme` file and builds every
component from CVA variants, so the look can change without changing the stack, the components or
the page structure.

## Decision

1. **Direction.** The consent UI adopts the docs-site design language: exact, sober, accountable.
   The world is the grant register; principals, agents, permission sets and `valid_until` drive the
   visuals. `DESIGN_PRINCIPLES.md` documents the direction in full.
2. **Type.** Geist carries text and headings at weights 400 and 500. Geist Mono carries
   identifiers, scopes and dates. Weights above 500 do not appear. Both fonts are variable fonts
   self-hosted from `web/public/fonts/` under the SIL Open Font License 1.1, which keeps
   Principle XI's self-hosting rule.
3. **Color.** A cool ink ramp replaces the warm neutrals. One jade accent (`#0a7a66`, hover
   `#086a59`) marks only the primary action, links, the focus ring and the granted state. The
   legacy token names (`trust-*`, `cta-*`, `success-*`, `neutral-*`) stay and map onto the new
   ramp so components keep working; `trust-deep` is ink and `trust` is the accent. New tokens name
   the new roles directly: `accent`, `accent-hover`, `accent-ink`, `accent-soft`, `line`,
   `line-strong`, `surface`, `raised`, `page`. Error, warning and info keep their hues in a more
   sober register. Every text token meets WCAG 2.1 AA on its intended ground.
4. **Elevation and motion.** Cards sit behind a 1px hairline and do not lift. One floating shadow
   (`shadow-float`) serves overlays only. Gradients, glows, blurs and textures are not used. One
   easing curve, `cubic-bezier(0.22, 1, 0.36, 1)`, serves every transition, and reduced motion
   disables entrances.
5. **Scope.** This is a visual change. The React, Tailwind v4, CVA and Headless UI stack, the
   design-system components, the page structure, the routes and the API client do not change.
   Dark mode is not part of this decision; the SPA has no theme switch and hard-codes white
   surfaces in many components, so a dark theme is a separate structural change.

## Consequences

### Positive

The product and its documentation present one face. The consent screen a reviewer sees on the
docs homepage is the screen the user sees in the broker. A single accent makes the primary action
and the granted state unmistakable. Hairlines and flat ground remove visual noise from a screen
that exists to be read carefully. Font payload shrinks from three families to two.

### Negative

Screenshots in `tests/e2e/screenshots/` go stale until the screenshot workflow regenerates them.
The e2e render-stability check names font families and had to follow the font change. Components
that still carry hard-coded `bg-white` cannot take part in a future dark theme without a sweep.
Historic specification documents under `specs/007-consent-frontend/` describe the previous look
and stay as a record.

---

## References

- Supersedes the visual direction section of `web/src/design-system/docs/DESIGN_PRINCIPLES.md` (previous revision)
- Related: [ADR 006: Frontend Technology Stack](006-frontend-stack.md)
- Related: docs-site design language in `assets/docusaurus/src/css/custom.css`
- Constitution: `.specify/memory/constitution.md`, Principle XI
