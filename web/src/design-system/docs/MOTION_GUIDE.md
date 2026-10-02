# Motion Guide

## Authority and delivery status

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) became Accepted on 2026-09-27.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current visual direction for [feature 047](../../../../specs/047-redesign-consent-console/spec.md).

The accepted system uses **CSS-only transitions at 120–200ms with ease-out timing**.
Reduced motion removes movement and delay.
There is no spring, shared-layout, page-entry, or Framer Motion path.

This guide defines implementation requirements. It does not prove that motion changes or validation are complete.
The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) record those results separately.
Principle XI keeps the WCAG 2.1 AA floor. Feature 047 targets WCAG 2.2 AA.

## Purpose

Motion provides feedback for a user's action or a local state change.
It must not delay a decision, imply authorization, or hide an error.
Content remains understandable without animation.

Use these rules:

- Acknowledge input immediately.
- Keep the same interaction and focus behavior with or without motion.
- Use central motion tokens rather than component-specific durations.
- Transition only the properties that need feedback.
- Keep focus indicators immediately visible and unobscured.
- Remove decorative movement under reduced motion.

## Token contract

`web/src/design-system/tokens/theme.css` defines these values:

| Token | Value | Use |
| --- | --- | --- |
| `--motion-feedback` | `120ms` | Hover color feedback |
| `--motion-control` | `160ms` | Control state changes and disclosure indicators |
| `--motion-overlay` | `200ms` | Dialog, Sheet, DropdownMenu, Popover, and toast feedback |
| `--motion-ease` | `ease-out` | Every transition |

[TOKEN_GUIDE.md](TOKEN_GUIDE.md) routes components to this contract.
The duration applies to visual feedback, not a network timeout, polling interval, or notification reading time.
Do not add a transition delay or stagger related controls.

## Interaction patterns

| Interaction | Required presentation |
| --- | --- |
| Button or link hover | Semantic color feedback at 120ms, without lift or scale |
| Focus-visible | Immediate visible focus, without an entry delay |
| Checkbox, Switch, or RadioGroup change | State feedback at 160ms, with an accessible state change |
| Tabs or disclosure | Immediate content availability, with optional local feedback at 160ms |
| Dialog, Sheet, menu, or popover | CSS opacity or small decorative movement at 200ms |
| Toaster | Local feedback at no more than 200ms, with an accessible announcement |
| Skeleton | Stable semantic surface and an accessible loading status, without a decorative shimmer |
| Route or full-page content | No entry animation or staggered rows |
| Card | No lift, premium-shadow transition, or decorative gradient |

Do not use `transition: all`.
Select properties explicitly so layout, focus, and unrelated state changes remain immediate.
For movement, prefer a small transform on a decorative layer instead of animation of layout dimensions.
Do not animate table reordering, confirmation outcomes, or authorization state with shared-layout effects.

## CSS integration

Components select the central token for the interaction:

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

These selectors illustrate stylesheet composition, not public component APIs.
The actual styles belong to the owned design-system components.
Color changes use [semantic roles](COLOR_GUIDE.md), not literal palette values.

`@starting-style` can provide optional entry feedback for supported browsers.
The final state must remain visible and usable without that rule.
An unsupported entry effect must not block focus, dismissal, or access to content.

React controls semantic state. CSS controls visual transitions.
Do not add animation-library wrappers or JavaScript timing loops.
No behavior can depend on `transitionend` or an animation timer.

## Reduced motion

Under `prefers-reduced-motion: reduce`, apply these requirements:

- Set motion durations and delays to zero.
- Disable decorative keyframes and smooth scrolling.
- Remove decorative translation, scale, rotation, and entry offsets.
- Show the final state immediately.
- Preserve state announcements and visible focus.

The central duration override is:

```css
@media (prefers-reduced-motion: reduce) {
  :root {
    --motion-feedback: 0ms;
    --motion-control: 0ms;
    --motion-overlay: 0ms;
  }
}
```

Duration changes alone do not remove a hover transform or a keyframe.
Each component must also remove its decorative movement and delay.
Do not reset every transform globally. Overlay positioning and a Switch's checked position remain functional layout, not decorative animation.
A checked Switch still shows its checked position immediately.

The reduced-motion rule applies to portaled content and Toaster as well as normal page content.
The preference can change while the page remains open.
Do not cache it in a one-time JavaScript read.

## Overlay behavior

Dialog and Sheet must trap focus, offer a close control, and restore focus after dismissal.
DropdownMenu, Popover, and Tooltip must preserve their keyboard paths.
Opening or closing animation must not expose inactive content to interaction.

Focus moves according to the component's accessible behavior, not an animation schedule.
The overlay inherits the resolved root theme.
Reduced motion must not change its location, accessible name, or dismissal behavior.

## Blocking evidence

Every component story needs light and dark coverage for its applicable states:

- Default
- Hover and focus-visible
- Disabled
- Error
- Loading
- Overlay-open

Each story needs a reviewed visual-regression baseline for each theme.
Storybook accessibility uses `parameters.a11y.test = 'error'`.
Accessibility and visual-regression failures must block CI.
Automation cannot accept changed baselines.

Browser evidence must also exercise normal and reduced motion:

1. Trigger each changed interaction in both themes.
2. Verify that CSS feedback stays within 120–200ms and uses ease-out.
3. Enable reduced motion and trigger the interaction again.
4. Verify that decorative movement and delay disappear.
5. Verify that focus remains visible and unobscured.
6. Open and close each overlay through the keyboard.
7. Verify that status announcements and authorization behavior do not change.
8. Navigate between routes and verify that no page-entry animation occurs.

Static screenshots do not prove timing or the absence of movement.
[ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md) defines the remaining browser and assistive-technology requirements.
