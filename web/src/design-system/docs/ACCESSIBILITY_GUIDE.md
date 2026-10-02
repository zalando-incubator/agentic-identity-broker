# Accessibility Guide

## Authority and evidence

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) became Accepted on 2026-09-27.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current visual direction for [feature 047](../../../../specs/047-redesign-consent-console/spec.md).

Constitution Principle XI keeps **WCAG 2.1 AA as the mandatory floor**.
Feature 047 targets **WCAG 2.2 AA**, including visible, unobscured focus and accessible target sizes.
Dynamic status changes must reach assistive technology without taking focus.

These are requirements, not a claim of measured compliance.
Component reuse, Radix behavior, token calculations, and automated accessibility results do not replace browser evidence.
The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) record implementation and verification separately.

## Semantic structure and names

Use semantic HTML before additional ARIA:

- Use a button for an action and a link for navigation.
- Keep one clear page heading and a logical heading hierarchy.
- Give navigation and main content their proper landmarks.
- Associate each input with a visible label.
- Give each icon-only control an accessible name.
- Hide decorative icons from assistive technology.
- Give informative images useful alternative text.
- Keep the local wordmark and compact mark accessible without SVG font dependencies.

Use owned design-system primitives rather than custom interactive containers.
Do not use a placeholder as the only label.
Tooltips supplement labels. They do not replace the name of a control.

Pages and application components take user-facing strings from `@copy`.
Design-system components receive labels, descriptions, and messages through required props or children.
Technical names and identifiers retain their exact spelling.

## Keyboard and focus

Every user flow must work without a pointer.
Focus order must match the reading order.
No control can depend on hover alone.

| Interaction | Required behavior |
| --- | --- |
| Tab and Shift+Tab | Move through the available controls in logical order |
| Enter and Space | Activate the focused control according to its semantics |
| Arrow keys | Navigate composite controls such as menus, radio groups, tabs, and command results |
| Home and End | Reach the first or last item where the control pattern supports them |
| Escape | Close the active dismissible overlay without submitting a decision |
| Overlay close | Restore focus to its trigger or a logical remaining control |

Use the semantic `ring` role for focus indicators.
Keep an immediately visible indicator with at least 3:1 contrast against adjacent surfaces.
Do not remove the outline without an equivalent focus-visible indicator.

Sticky headers and draft action bars must not obscure the focused element.
Keep focused content visible during scrolling and keyboard navigation.
A user must always have a keyboard exit from an overlay.
Do not use animation delays to postpone focus or interaction.

## Forms and selection

Every input needs a programmatically associated label.
Helper text and error text must connect through `aria-describedby` where appropriate.
Invalid fields expose `aria-invalid` and an actionable error message.
The error cannot rely on a colored border alone.

Required permissions and required services remain selected and locked.
Their text explains why they cannot change.
Optional permissions retain explicit, keyboard-operable controls.
A disabled presentation must not hide the control's meaning or state.

Use Input for text entry and RadioGroup for mutually exclusive choices.
Select, Switch, Checkbox, TextArea, and DatePicker follow the same label and keyboard requirements.
DatePicker must expose a valid date-entry path and the applicable minimum date.
ThemeChoice is a RadioGroup with caller-supplied Light, Dark, and System labels.

An invalid submission must identify the affected field without losing the user's selections.
A browser preference must never select or submit an authorization decision.

## Overlays and disclosure

Dialog and Sheet require an accessible name, appropriate initial focus, focus containment, and an available close control.
Inactive content behind a modal overlay must not receive interaction or assistive-technology focus.
Escape closes the active dismissible overlay. Closing it restores focus.

DropdownMenu uses menu keyboard behavior, not a custom clickable list.
Popover and Tooltip must work from keyboard focus as well as pointer interaction.
Tooltips remain supplementary and dismissible.

Accordion and expandable content expose their expanded state and the controlled content.
TruncatedText must provide an accessible expansion, not only a pointer-only tooltip.
Long names and descriptions use two-line truncation. Table cells use one line before expansion.
Expanded text must remain readable at narrow widths and zoom.

Portaled content must inherit the resolved root theme.
Do not override positioning transforms to disable motion. Remove only decorative movement.

## Tables and command search

Owned Table presentation must retain table semantics, headers, and an accessible caption or name.
TanStack Table manages state, not accessible markup or authorization.
Sorting controls expose their action and current sort state.
Repeated row actions need enough context to identify the affected record.

Responsive layouts retain labels, actions, and details without horizontal page scrolling.
The Agents list contains agent identity, expiry, View, and confirmed Revoke.
It contains no permission-set count column or per-agent count requests.

Command must support keyboard search, result navigation, selection, dismissal, and focus return.
Its accessible names must describe search and result groups.
Results contain only records available to the acting user.
Table and Command remain outside the initial decision bundle and its import barrels.

## Status, errors, and announcements

New approvals, changed pending counts, decision results, and toasts must announce meaningful changes without moving focus.
Use a polite status region for routine updates.
Use an alert only for an error that requires immediate attention.
Do not announce the same unchanged pending count after every refresh.

A loading region can expose `aria-busy` and a text status.
A decorative Skeleton must not produce repeated screen-reader content.
Toaster uses the owned Sonner presentation and must expose its messages accessibly.
Do not rely on a disappearing toast as the only explanation of a persistent error.

Revocation requires explicit confirmation.
The optimistic state says that the operation is pending, not that the server accepted it.
On failure, restore the affected record and announce the error.
Grant creation and approval must wait for the server result.

Color does not carry status or risk by itself.
Use a text label and an accessible explanation for server-provided tool risk.
An unrated tool uses “Risk not rated”. Permission groups have no risk indicator.
Agent Origin Labels describe domain metadata, not verified legal publisher identity.

## Contrast and themes

[COLOR_GUIDE.md](COLOR_GUIDE.md) defines every semantic color and its light/dark value.
Measure rendered colors after opacity, overlays, state styles, and focus styles apply.
Do not substitute raw palette utilities or component-local colors.

| Content | Minimum contrast |
| --- | --- |
| Text, including supporting text, placeholders, and filled status text | 4.5:1 |
| Required control boundaries and meaningful non-text indicators | 3:1 against adjacent surfaces |
| Focus indicators | 3:1 against adjacent surfaces |

Inactive controls have WCAG exceptions where applicable. Placeholder text is not exempt merely because it is a placeholder.
Decorative separators can use `border-soft`. They cannot establish a required control boundary.
The project text requirement remains 4.5:1 even where WCAG permits a lower large-text ratio.

Light, dark, and system preferences must preserve the same information and actions.
Native controls, scrollbars, and overlays use the resolved theme.
Forced-color mode must retain control boundaries, focus, status labels, and selection meaning.

## Size, zoom, and motion

At 320px width and 200% zoom, every route must work without horizontal page scrolling.
Do not hide actions or permission details to make the layout fit.
Target sizes meet WCAG 2.2 AA through sufficient size or its permitted spacing exceptions.
Use larger touch targets where space permits.

[MOTION_GUIDE.md](MOTION_GUIDE.md) requires CSS-only 120–200ms ease-out feedback.
Under `prefers-reduced-motion: reduce`, remove movement and delay.
Keep status text, focus, disclosure, and authorization behavior unchanged.
Do not add page-entry animation or Framer Motion.

## Blocking story checks

Every component and shared shell needs light and dark stories.
Each theme must cover every applicable state:

| State | Evidence |
| --- | --- |
| Default | Accessible structure, names, text, and boundaries |
| Hover | Feedback without information available only on hover |
| Focus-visible | Visible, unobscured focus and keyboard activation |
| Disabled | Meaningful label and state without an available action |
| Error | Associated message, state, and appropriate announcement |
| Loading | Accessible progress status without decorative noise |
| Overlay-open | Name, focus containment, dismissal, focus return, and portal theme |

Every story must pass the Storybook accessibility addon in both themes.
Use `parameters.a11y.test = 'error'` so violations fail the run.
A themes toolbar alone does not exercise both themes in CI.

Every story also needs a reviewed visual-regression baseline for each theme.
Accessibility failures, visual differences, missing baselines, and stale baselines must block CI according to the feature gates.
Automation uploads candidate images and diffs. It must not accept baseline changes automatically.

Story checks must include composed states, not only isolated default controls.
Passing automated checks does not establish complete WCAG compliance.

## Browser verification

Before release, collect evidence from the actual application:

1. Navigate every flow with the keyboard in both themes.
2. Open and close overlays, then verify focus return.
3. Verify visible focus around sticky headers and draft controls.
4. Use a screen reader to verify names, labels, state changes, and announcements.
5. Measure contrast on actual surfaces and interaction states.
6. Exercise every route at 320px and 200% zoom.
7. Enable reduced motion and verify that movement and delay disappear.
8. Exercise forced colors and native form controls.
9. Verify explicit-theme precedence, system changes, and first paint.
10. Verify that no page automatically requests third-party fonts, scripts, or images.

The route visual gate includes all six routes and both agent contexts in both themes.
Keep reviewed results separate from requirements and planning calculations.

## References

- [WCAG 2.1 quick reference](https://www.w3.org/WAI/WCAG21/quickref/)
- [WCAG 2.2](https://www.w3.org/TR/WCAG22/)
- [WAI-ARIA Authoring Practices](https://www.w3.org/WAI/ARIA/apg/)
- [Feature UI contract](../../../../specs/047-redesign-consent-console/contracts/ui-and-configuration.md)

These links are reading references, not frontend resource dependencies.
