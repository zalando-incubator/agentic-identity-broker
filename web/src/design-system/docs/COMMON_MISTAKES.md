# Common Mistakes and Corrections

## Authority and example status

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27 and amended on 2026-10-04.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current direction for feature 047.
The examples are targets, not completed release validation.

Use [COLOR_GUIDE.md](COLOR_GUIDE.md) for exact light and dark values.
Use [TOKEN_GUIDE.md](TOKEN_GUIDE.md) for typography, spacing, radius, and motion tokens.
Do not copy obsolete runtime styling into new components.

## Using palette values instead of semantic roles

Raw palette utilities and component-local colors bypass the light/dark theme contract.
Use semantic roles in every state, including hover, focus, error, and disabled presentation:

```tsx
<section className="border border-border-subtle bg-card text-card-foreground">
  <h2 className="font-display text-foreground">{title}</h2>
  <p className="text-muted-foreground">{description}</p>
</section>
```

The caller supplies `title` and `description`; the application obtains those strings from `@copy`.
Use `--border-subtle` for card edges and dividers, `--border` for buttons, and `--border-control` for inputs, checkboxes, and radios.
Use `--primary-soft` for active navigation and selected rows. Soft status colors need a word or icon.

## Showing multiple primary actions or hiding the one needed

Use one accent action in each decision context, not one per row or section.
Consent uses Allow when ready. If a selected service is not connected, the primary action becomes “Connect {Service} to continue”.

```tsx
<div className="flex gap-3">
  <Button variant="outline" onClick={onDeny}>{denyLabel}</Button>
  <Button variant="primary" onClick={onAllow}>{allowLabel}</Button>
</div>
```

The application supplies labels from `@copy`. Deny creates nothing, revokes nothing, and constructs no redirect.
Consent actions remain visible without scrolling at 1280 × 720 with three permission groups and a risk callout.
The approval inbox puts Approve once and Deny in a pinned detail panel; its list rows never expand.
Use `outline` for repeated row actions. Use `destructive-quiet` with `ShieldOff` for agent revocation and `Unlink` for connection disconnect.
Use `destructive-outline` for the single Revoke button in the agent detail header.
Use `destructive` only for a confirmation in Dialog. Remove an overflow menu with just one action.

## Recreating controls or keeping obsolete APIs

Use the owned shadcn/Radix components in the existing design-system categories.
Do not create another component library under application components.

| Need | Accepted component |
| --- | --- |
| Confirmation | Dialog |
| Action menu | DropdownMenu |
| Text entry | Input |
| Exclusive choice | RadioGroup |
| Content separator | Separator |
| Mobile sidebar | Sheet |
| Result notification | Sonner-backed Toaster |

Read the owned source before you use a prop.
Do not preserve old size, padding, or event props through compatibility aliases.

## Replacing shells with custom page chrome

ConsoleShell owns the three-item navigation and centered main area. PageHeader supplies the title, optional muted count, and icon toolbar.
Move the purpose sentence to the empty state for Agents, Connections, and Approvals.
DecisionShell owns the focused consent card or approval review without a sidebar.
An invalid authorization session remains a decision error, never an editable console fallback.

Keep Command and Motion out of decision-route bundles. Use concrete imports to avoid bringing console-only code into decision routes.

## Adding decorative elevation or motion

Use tinted surfaces and semantic borders, not loud card outlines or strong card shadows. Floating layers use `shadow-overlay`; consent and the sticky save bar use `shadow-focal`. Only consent permits the static 3–4% top accent wash authorized by ADR 038's 2026-10-05 amendment.
Do not expand card or row controls under a pointer. Open detail in a panel, dialog, or reserved area.
Console routes can use `AnimatePresence` and `layout` for a removed card or decided row. Decision routes use CSS-only SVG animation.
Use shared 120, 160, 200, and 320 ms tokens and `cubic-bezier(0.2, 0, 0, 1)`.
Under reduced motion, remove movement and icon animation but preserve opacity and color fades.

Use 4–12 px spacing inside components, 16–24 px between components, and 32–48 px between sections.
Do not add generous card padding or show a toggle for text that is not clipped.

## Loading remote identity assets

Use local outlined artwork through Wordmark.
Use Zalando Sans Variable for display text, Inter Variable for body text, and JetBrains Mono for technical values.
Fonts and licenses belong in `web/public/fonts/`. Artwork belongs in `web/public/brand/`.

If an agent image is off-origin, show the local Avatar fallback without requesting the image.
An external governance link or explicit OAuth2 redirect does not authorize automatic external asset loads.

## Hiding meaning from keyboard and screen-reader users

Use real buttons, links, headings, and form labels. Use a linked card surface with separate buttons above the link layer.
Do not nest an action button inside the link or use a clickable `div`.
Keep focus visible and unobscured; use `focus-visible` on programmatically focused page wrappers.

Connect Input errors with `aria-invalid` and `aria-describedby`.
Give icon-only controls accessible names and tooltips. Use labeled status, not color alone.
One-line entity names truncate with CSS ellipsis. Show the full-text tooltip only when `TruncatedText` measures clipping.
Multi-line text gets a More toggle only when measured as clipped.
The toggle must support keyboard input. Preserve every action at 320 px and 200% zoom.

## Presenting unsupported data or results

Agents cards show the agent, grant expiry, and relative change time from `lastModifiedAt`. The name is more prominent than the quiet date metadata. Cards are 120 px tall with consistent 12 px outer insets. The linked surface opens detail; Revoke is a separate action. Do not add a duplicate Details link or keyboard stop.
`activeGrantCount` counts UserGrant records, not permission sets. Do not show a count column or fetch per-agent counts.

Do not invent publisher, account, last-use, or missing-scope data.
Do not infer permission-group risk from scope names. Only tool approvals use server-provided risk.
The consent origin line shows the available domain metadata. Do not claim that domain verification establishes legal publisher identity.

Confirm revocation before a request. Restore the affected record on failure and announce success only after server acceptance.
Never optimistically grant or approve access.

## Treating examples as accessibility evidence

Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
Primitives, patterns, and screens need both-theme accessibility checks. Reviewed pixel references cover eligible light 1280 × 720 cases only, under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md).
Screen stories also cover dense, long, error, stale, and decision-specific states at 375, 768, and 1280 px.
Rendered text needs 4.5:1 contrast; required form-control boundaries and focus indicators need 3:1.
Token targets and story examples do not prove contrast, keyboard behavior, layout stability, or focus visibility.
Measure the approval list bounds after decisions and consent Allow at 1280 × 720 before release.
See [ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md), [COMPONENT_PAIRING_GUIDE.md](COMPONENT_PAIRING_GUIDE.md), and [COMPOSITION_PATTERNS.md](COMPOSITION_PATTERNS.md).
