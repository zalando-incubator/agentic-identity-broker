# Accessibility Guide

## Authority and evidence

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) became Accepted on 2026-09-27 and its UX amendment was approved on 2026-10-04.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current target for [feature 047](../../../../specs/047-redesign-consent-console/spec.md).

Constitution Principle XI keeps **WCAG 2.1 AA as the mandatory floor**.
Feature 047 targets **WCAG 2.2 AA**, including visible, unobscured focus and accessible target sizes.
Dynamic status changes must reach assistive technology without taking focus.

These are requirements, not a claim of measured compliance.
Component reuse, Radix behavior, token calculations, and automated accessibility results do not replace browser evidence.
The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) record implementation and verification separately.

## Semantic structure and names

Use semantic HTML before additional ARIA:

- Use a button for actions and a link for navigation, including a linked entity surface.
- Keep buttons outside a linked card's link layer. Do not nest interactive controls.
- Keep one clear page heading and a logical heading hierarchy.
- Give navigation and main content their proper landmarks.
- Associate each form control with a visible label.
- Give each icon-only control an accessible name and a supplementary tooltip.
- Hide decorative icons from assistive technology, but keep status words available.
- Give informative images useful alternative text; name the local wordmark without SVG fonts.
- Put one badge on the title baseline or in a fixed status slot, never in a wrapping row.
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

Use the semantic `ring` role for focus indicators, with at least 3:1 contrast against adjacent surfaces.
Use `focus-visible` on programmatically focused page headings or wrappers. Clicking empty page space must not draw a ring around an entire box.
Do not remove the outline without an equivalent focus-visible indicator.

Sticky consent and approval decision bars and grant-edit bars must not obscure a focused control.
Keep focused content visible during scrolling. Provide a keyboard exit and focus return for each overlay.
Do not delay focus, submission, or announcements until an animation finishes.

## Forms and selection

Every input needs a programmatically associated label.
Helper text and error text must connect through `aria-describedby` where appropriate.
Invalid fields expose `aria-invalid` and an actionable error message.
The error cannot rely on a colored border alone.

Required permissions and required services remain selected and locked.
Their text explains why they cannot change.
Optional permissions retain explicit, keyboard-operable controls.
A disabled presentation must not hide the control's meaning or state.

Use Input for text entry, Checkbox for independent selections, and a labeled Select for grant duration.
DurationSelect exposes Until I revoke it, 30 days, and Custom date; the custom date uses a Popover and DatePicker.
Agent detail shows the shared DurationSelect directly in its duration card. The selector and custom-date button support keyboard access, retain the chosen calendar date visibly, and announce validation errors. Changed duration has a saved-versus-pending expiry preview.
DatePicker must expose a valid keyboard date-entry path and the applicable minimum date.
The custom-date trigger matches the duration selector height. Its popover keeps a 16 px viewport gutter at narrow widths. Escape returns focus to the trigger; outside dismissal preserves the selected date.
DatePicker retains native block layout so the browser's calendar icon stays at the trailing edge rather than beside the date segments.
Consent keeps labeled triggers visible while its duration choices, custom date, About, service choices, and technical details load on first action. Their overlays open from that action and return focus to the trigger on Escape.
ThemeChoice and the Settings appearance tiles keep Light, Dark, and System keyboard-operable.
The stored collection view changes presentation only. It never selects or submits an authorization decision.

An invalid submission identifies the affected row and shows a summary in the decision footer without losing selections.

## Overlays and disclosure

Dialog and Sheet require an accessible name, appropriate initial focus, focus containment, and an available close control.
Inactive content behind a modal overlay must not receive interaction or assistive-technology focus.
Escape closes the active dismissible overlay. Closing it restores focus.

DropdownMenu uses menu keyboard behavior, not a custom clickable list.
Popover and Tooltip must work from keyboard focus as well as pointer interaction.
Tooltips remain supplementary and dismissible.

Consent service disclosure exposes its expanded state. Console service choices remain visible as pressed-state buttons; locked services have informational labels rather than redundant disabled controls.
One-line names use ellipsis and a keyboard-reachable Tooltip with full text, never a dead More toggle.
Multi-line descriptions show a keyboard-operable More/ Less control only after `ResizeObserver` measures clipping.
Keep expansion out of fixed-height collection rows. Full text must remain readable at narrow widths and zoom.

Portaled content must inherit the resolved root theme.
Do not override positioning transforms to disable motion. Remove only decorative movement.

## Collections, inbox, and command search

EntityCard and EntityRow preserve a single link target for detail and separate, labeled action buttons.
Their fixed height must not change when a decision or state changes. Long names have a tooltip with the full text.
When sorting changes, announce the active choice. Applied filters have accessible, removable chips.
The expanded CollectionToolbar search has a label. `/` focuses it, and Escape clears and collapses it.

The Agents collection shows identity, grant expiry, a Last updated date, and confirmed Revoke, with no count column or per-agent count requests.
Connections show only stored services. A missing required service appears only in an agent or consent context.
Approval inbox shortcuts `J`, `K`, `A`, `D`, and Enter require focus inside the pending inbox or its review panel. They do nothing from the page body or unrelated controls and must not override text entry or an open overlay.
The `A` and `D` hints also work from the main decision buttons and activate their currently selected action. Menu choices only change that action; the user activates the main button before scope review or permanent-denial confirmation opens. Neither shortcut bypasses those checks.
Pending and Remembered tab labels include their loaded, principal-scoped totals, independent of remembered filters. Do not present an unavailable count as zero.
Approval-row text does not participate in native Shift-click range selection. Request details and full-scope popovers remain copyable; selected-row styling and visible keyboard focus are unchanged.
Selecting an approval must update and name its detail panel without changing the list's height.
Below 1012 px, a row opens the accessible standalone review route `/approvals/:id`.

Command supports keyboard search, result navigation, selection, dismissal, and focus return.
Its result groups contain only records available to the acting user.
Command and Motion remain outside decision-route bundles and imported barrels.

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

[COLOR_GUIDE.md](COLOR_GUIDE.md) defines semantic targets. Measure rendered colors after opacity, overlays, state styles, and focus styles apply.
Never substitute raw palette utilities or local component colors for the token roles.

| Content | Minimum contrast |
| --- | --- |
| Text, including supporting text, placeholders, and soft status text | 4.5:1 |
| Required form-control boundaries and meaningful non-text indicators | 3:1 against adjacent surfaces |
| Focus indicators | 3:1 against adjacent surfaces |

Decorative card and divider borders use `--border-subtle`; buttons and popovers use `--border`.
Input, checkbox, and radio boundaries use `--border-control` and must pass the 3:1 control requirement.
Inactive controls have WCAG exceptions where applicable. Placeholder text is not exempt merely because it is a placeholder.
The project text requirement remains 4.5:1 even when WCAG permits a lower ratio for large text.

Light, dark, and system preferences preserve the same information and actions.
Native controls, scrollbars, and overlays use the resolved theme.
Forced-color mode retains control boundaries, focus, status labels, and selection meaning.

## Size, zoom, and motion

At 320 px width and 200% zoom, every route works without horizontal page scrolling.
Do not hide an action or permission detail to make the layout fit.
Meet WCAG 2.2 AA target sizes through sufficient size or its permitted spacing exceptions.
Use larger touch targets where space permits.

At 1280 × 720 with three permission groups and a risk callout, consent Allow stays inside the viewport.
On smaller screens, fix the decision footer to the bottom and keep keyboard focus visible above it.
At 375, 768, and 1280 px in both themes, screen stories must exercise dense and long-content states.

[MOTION_GUIDE.md](MOTION_GUIDE.md) allows Motion on console routes but not decision routes.
Under reduced motion, remove movement and icon animation. Preserve opacity and color feedback, status text, and focus.

## Blocking story checks

Every primitive, composed pattern, and screen needs light and dark stories with applicable states:

| State | Evidence required |
| --- | --- |
| Default | Names, labels, text, and control boundaries |
| Hover | No information available only on hover |
| Focus-visible | Unobscured focus and keyboard activation |
| Disabled | Meaningful label and state, without a misleading action |
| Error | Associated message and appropriate announcement |
| Loading | Layout-matched Skeleton and accessible status |
| Overlay-open | Name, focus containment, dismissal, focus return, inherited theme |

Add screen states for empty, typical, dense (at least ten items), stale, and longest-realistic content.
Consent adds a risk callout, missing connection, already granted, denied result, and invalid session.
Approvals adds high-risk selection, expired request, and remembered-scope editor.
Exercise each screen at 375, 768, and 1280 px in both themes.

Use `parameters.a11y.test = 'error'`. The `storybook-light` and `storybook-dark` projects must fail on violations.
Add interaction tests for permission, duration, keyboard approval, and confirmed revocation.
Assert that the approval list bounding box stays fixed through a decision step.
Assert that consent Allow fits inside a 1280 × 720 viewport in its typical story.
Review canonical light 1280 × 720 pixel references by hand, under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md). Both-theme and responsive interaction/accessibility coverage remains required. Automation cannot accept references for release.
Automated checks do not establish complete WCAG compliance.

## Browser verification

Before release, collect evidence from the actual application:

1. Navigate each flow by keyboard in light and dark themes.
2. Open and close overlays and make sure that focus returns.
3. Keep focus visible around sticky decision and draft controls.
4. Use a screen reader to examine names, labels, state changes, and announcements.
5. Measure text, soft status, control-boundary, and focus contrast on rendered surfaces.
6. Exercise each route at 320 px and 200% zoom, plus consent at 1280 × 720.
7. Enable reduced motion and confirm that movement stops while useful fades remain.
8. Exercise forced colors and native controls.
9. Examine theme precedence, OS changes, and first paint.
10. Confirm that pages do not request third-party fonts, scripts, or images automatically.

Cover Agents, Connections, pending and remembered Approvals, Settings, consent, and standalone review.
Keep reviewed results separate from token targets and planning contrast calculations.

## References

- [WCAG 2.1 quick reference](https://www.w3.org/WAI/WCAG21/quickref/)
- [WCAG 2.2](https://www.w3.org/TR/WCAG22/)
- [WAI-ARIA Authoring Practices](https://www.w3.org/WAI/ARIA/apg/)
- [Feature UI contract](../../../../specs/047-redesign-consent-console/contracts/ui-and-configuration.md)

These links are reading references, not frontend resource dependencies.
