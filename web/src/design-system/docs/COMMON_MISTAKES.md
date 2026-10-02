# Common Mistakes and Corrections

## Authority and example status

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the accepted visual system for feature 047.
These examples describe the current component contract, not completed release validation.

Use [COLOR_GUIDE.md](COLOR_GUIDE.md) for exact light and dark values.
Use [TOKEN_GUIDE.md](TOKEN_GUIDE.md) for typography, spacing, radius, and motion tokens.
Do not copy obsolete runtime styling into new components.

## Using palette values instead of semantic roles

Raw palette utilities and component-local color literals bypass the theme contract.
Use semantic roles in every state, including hover, focus, error, and disabled presentation.

```tsx
<section className="border border-border bg-card text-card-foreground">
  <h2 className="font-display text-foreground">{title}</h2>
  <p className="text-muted-foreground">{description}</p>
</section>
```

In this primitive example, `title` and `description` are props.
Application components obtain user-facing strings from `@copy` and pass them into primitives.
Primitives do not import `@copy`.

Headings use `foreground`, not the primary action color.
Control boundaries use `border`. Decorative separators can use `border-soft`.
Status colors describe state and never introduce another primary action.

## Showing multiple primary actions

The limit is one accent action per view, not one per card or section.
Some views have no accent action.

```tsx
<div className="flex flex-wrap gap-3">
  <Button variant="secondary" onClick={onDeny}>{denyLabel}</Button>
  <Button variant="primary" onClick={onAllow}>{allowLabel}</Button>
</div>
```

The consent application supplies “Deny” and “Allow” from `@copy`.
Deny is not a destructive action. It creates nothing, revokes nothing, and constructs no redirect.

Use `secondary`, `outline`, or `ghost` for repeated row actions.
Use `destructive` for a destructive confirmation inside Dialog.
Do not put a resting destructive primary action beside the grant editor.
Put “Revoke all access” in its DropdownMenu instead.

See [DECISION_TREES.md](DECISION_TREES.md) for the route-specific action contract.

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

ConsoleShell owns console navigation. PageHeader supplies the title, one-line purpose, and an optional assigned action.
DecisionShell owns the focused decision layout and has no sidebar.
An invalid authorization session remains a decision error, not a console fallback.

Keep Table and Command in console modules.
Do not re-export them from a barrel that decision routes import.
Use concrete component paths to preserve the decision bundle boundary.

## Adding decorative elevation or motion

Use neutral surfaces and 1 px semantic borders instead of strong shadows, gradients, or hover lifts.
Use centralized CSS transitions at 120–200 ms with ease-out.
Do not add page-entry animation or a JavaScript animation library.
Under `prefers-reduced-motion`, remove movement without removing status feedback.

Use shared spacing tokens to separate labels, fields, sections, and actions.
Do not add generous card padding to a compact table merely for decoration.

## Loading remote identity assets

Use local outlined artwork through Wordmark.
Use Zalando Sans Variable for display text, Inter Variable for body text, and JetBrains Mono for technical values.
Fonts and licenses belong in `web/public/fonts/`. Artwork belongs in `web/public/brand/`.

If an agent image is off-origin, show the local Avatar fallback without requesting the image.
An external governance link or explicit OAuth2 redirect does not authorize automatic external asset loads.

## Hiding meaning from keyboard and screen-reader users

Use real buttons, links, headings, tables, and form labels.
Do not make a clickable `div` or nest a row action inside another interactive element.
Keep visible, unobscured focus and return focus after an overlay closes.

Connect Input errors with `aria-invalid` and `aria-describedby`.
Give icon-only controls accessible names through props.
Use status labels with colors. Hide redundant decorative icons from assistive technology.
Do not hide essential instructions in a Tooltip.

Use TruncatedText for escaped metadata with accessible expansion.
Allow two lines for names and descriptions, and one line in table cells.
Preserve every action at 320 px and 200% zoom.

## Presenting unsupported data or results

The Agents table shows agent, expiry, View, and confirmed Revoke.
`activeGrantCount` counts UserGrant records, not permission sets.
Do not display a count column or fetch per-agent counts.

Do not invent publisher, account, last-use, or missing-scope data.
Do not infer permission-group risk from scope names.
Only tool approvals use the server-provided risk level.

Require confirmation before optimistic revocation.
Restore only the affected record on failure, and announce success only after server acceptance.
Never optimistically grant or approve access.

## Treating examples as accessibility evidence

Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
Every component story needs light and dark accessibility checks and reviewed visual baselines.
Include default, hover, focus-visible, disabled, error, loading, and overlay-open states where applicable.

Rendered text needs 4.5:1 contrast. Controls need 3:1.
Token names alone do not prove contrast, keyboard behavior, or focus visibility.
See [ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md), [COMPONENT_PAIRING_GUIDE.md](COMPONENT_PAIRING_GUIDE.md), and [COMPOSITION_PATTERNS.md](COMPOSITION_PATTERNS.md).
