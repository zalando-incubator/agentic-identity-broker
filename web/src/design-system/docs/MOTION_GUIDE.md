# Motion Guide

## Authority and delivery status

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) became Accepted on 2026-09-27. The stakeholder approved its motion amendment on 2026-10-04.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current direction for [feature 047](../../../../specs/047-redesign-consent-console/spec.md).

Console routes can use owned animated Lucide icons and Motion. Consent and standalone approval review remain CSS-only.
Feedback uses 120–200 ms, and emphasis can last 320 ms. Reduced motion retains useful opacity and color fades without movement.

These are targets, not proof that runtime animation or validation is complete.
The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) track those results.
Principle XI keeps the WCAG 2.1 AA floor. Feature 047 targets WCAG 2.2 AA.

## Purpose

Motion provides feedback for a user's action or a local state change.
It must not delay a decision, imply authorization, or hide an error.
Content remains understandable without animation.

Use these rules:

- Acknowledge input immediately.
- Keep the same focus, selection, and authorization behavior with or without motion.
- Use shared motion tokens and console constants instead of per-component durations.
- Transition only properties that explain a change. Never shift a row or card under the pointer.
- Keep focus indicators immediately visible and unobscured.
- Remove movement and icon animation under reduced motion while preserving useful fades.

## Token contract

`web/src/design-system/tokens/theme.css` owns CSS duration and easing values:

| Token | Value | Use |
| --- | --- | --- |
| `--motion-feedback` | `120ms` | Hover color feedback |
| `--motion-control` | `160ms` | Control changes and disclosure |
| `--motion-overlay` | `200ms` | Dialog, Sheet, menu, popover, toast |
| `--motion-emphasis` | `320ms` | Success check, connection pulse, illustrative state |
| `--motion-ease` | `cubic-bezier(0.2, 0, 0, 1)` | CSS and Motion entrance easing |

Exit duration is 75% of its corresponding entrance. The shared `consoleMotion` constants module reads these durations and easing for Motion transitions.
CSS decision-route animations use the same values but never import Motion.
Tokens describe visual feedback, not a network timeout, polling interval, or notification reading time.
Do not add transition delays or stagger decision controls.

## Interaction patterns

| Interaction | Required presentation |
| --- | --- |
| Button or link hover | Semantic color feedback at 120 ms; primary and destructive buttons shift lightness 0.04 |
| Press | Button scale 0.98, except under reduced motion |
| Focus-visible | Immediate indicator without a delay |
| Checkbox, Switch, Select, or disclosure | Accessible state changes immediately; optional 160 ms feedback |
| Dialog, Sheet, menu, or popover | Opacity and small movement at 200 ms |
| Console card or row removal | Motion `AnimatePresence` and `layout` close gaps after departure without in-place expansion |
| Card-to-detail navigation | Browser View Transitions API as progressive enhancement; direct navigation otherwise |
| Animated icons | Three nav hover icons, three empty-state icons, a success check, and a new-connection plug |
| Consent and standalone approval review | Owned SVG/CSS success check and risk callout; no Motion import |
| Approval selection | Content cross-fades at 120 ms; shared-layout accent bar moves without shifting rows |
| Approval tab change | Shared header/tabs stay fixed; content enters with a 160 ms opacity fade, without hiding initial page content or retaining stale decisions |
| New pending request | Count bumps once on ID addition, not initial load, unchanged polling, or removal |
| Route or full-page content | No unrelated page-entry or staggered-row animation |

Source the console icons from lucide-animated as owned components, not runtime remote assets.
The decision check can draw with `stroke-dashoffset`; the risk callout can use a small transform. Neither delays focus or submission.
Empty-state icons play once on entry and on hover. Keep animation in meaningful state feedback and illustration locations, not every icon.
Use `transition-property` lists, never `transition: all`. Do not animate content height to reveal row controls.

## CSS and Motion integration

Components select tokens according to their interaction:

```css
.control-feedback {
  transition-property: background-color, border-color, color;
  transition-duration: var(--motion-feedback);
  transition-timing-function: var(--motion-ease);
}

.overlay-feedback {
  transition-property: opacity;
  transition-duration: var(--motion-overlay);
  transition-timing-function: var(--motion-ease);
}
```

These selectors illustrate composition, not public component APIs. Use semantic colors from [COLOR_GUIDE.md](COLOR_GUIDE.md).
`@starting-style` can enhance overlays in supported browsers. The visible result and focus path must not depend on support.
Browser View Transitions enhance card-to-detail navigation, but unsupported browsers navigate without animation.

Console routes can use `AnimatePresence` and `layout` for removed cards or decided rows, with durations and easing from `consoleMotion`.
Wrap the console with `<MotionConfig reducedMotion="user">`. Do not import Motion or its barrels into consent or standalone approval-review routes.
The existing decision bundle test must check that no decision route imports Motion. React controls the semantic result; no mutation waits for animation events.
Console icons finish one animation cycle. Ignore repeated pointer or focus triggers while that cycle runs; leaving the control does not reverse it. Reduced motion stops the cycle and restores the static icon.

## Reduced motion

Under `prefers-reduced-motion: reduce`:

- Keep opacity and semantic color transitions, including short overlay fades.
- Remove translation, scaling, shared-layout movement, icon keyframes, and smooth scrolling.
- Show the final position and status immediately. Keep focus, content, and announcements unchanged.
- Use the console's `MotionConfig reducedMotion="user"` to stop Motion movement.

Do not set all transitions to `none` or force every duration to `0ms`; that removes useful fades.
Remove movement at the component level. Do not reset every transform globally: overlay placement and a Switch's checked position are functional.
Decision-route CSS keyframes for icons and movement stop under this preference, but their status text stays visible.
The preference can change while the page remains open. Do not cache one JavaScript reading at mount.

## Overlay behavior

Dialog and Sheet must trap focus, offer a close control, and restore focus after dismissal.
DropdownMenu, Popover, and Tooltip must preserve their keyboard paths.
Opening or closing animation must not expose inactive content to interaction.
Closed tooltips become invisible immediately so their exit cannot cover the next focused control. Their positioning wrapper does not intercept pointer input; open tooltip content still supports hover.

Focus moves according to the component's accessible behavior, not an animation schedule.
The overlay inherits the resolved root theme.
Reduced motion must not change its location, accessible name, or dismissal behavior.

## Blocking evidence

Every component, composed pattern, and screen story needs applicable light and dark states.
Storybook accessibility uses `parameters.a11y.test = 'error'` in all six projects. Canonical light 1280 × 720 screenshot references remain separate evidence under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md). Story interactions use ordinary motion. Capture then normalizes final states without changing product animation behavior.
The screen stories include loading, empty, typical, dense, error, and stale states in both themes at 375, 768, and 1280 px.
Interaction tests must cover permission changes, duration, keyboard approval, and confirmed revocation.

Browser evidence must cover normal and reduced motion:

1. Trigger changed interactions in both themes and compare actual duration and easing with the token targets.
2. Enable reduced motion. Confirm that movement and icon animation stop while opacity and color feedback remain.
3. Navigate card to detail with and without View Transitions API support.
4. Verify that focus remains visible and unobscured during an exit or overlay change.
5. Verify that status announcements and authorization results do not wait for animation.
6. Check that the bundle test excludes Motion from every decision route.

Static screenshots do not prove timing or the absence of movement.
[ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md) defines remaining browser and assistive-technology requirements.
