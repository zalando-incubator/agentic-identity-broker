# Component Pairing Guide

## Authority and example status

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27.
This guide describes the feature 047 composition rules.
The examples describe composition requirements, not completed release validation.
Read the owned source for exact props during implementation.

Use the existing design-system categories. Do not create another primitive library.
Applications obtain copy from `@copy`. Primitives receive labels and descriptions through props.
The `labels` values in these examples represent caller-supplied copy.

## Shell and page header

| Pair | Responsibility |
| --- | --- |
| ConsoleShell + PageHeader | Console navigation, page title, one-line purpose, and optional assigned action |
| ConsoleShell + Sheet | Mobile navigation without a second navigation implementation |
| ConsoleShell + Command | Acting-user search through the application-owned search slot |
| ConsoleShell + DropdownMenu + ThemeChoice | User menu and immediate browser appearance choices |
| DecisionShell + Wordmark | Focused decision column, local branding, and footer without a sidebar |

Shells are presentational. Application code supplies data, navigation content, search, and user-menu content.
DecisionShell limits the content column to 640 px.
PageHeader does not invent a primary action for list or settings pages.

Keep Table and Command outside decision-route imports.
No barrel that decision routes import can re-export either component.

## Table, search, and row actions

Table supplies presentation parts. TanStack Table supplies row and filter state.
Input supplies a labeled name-search field above the collection.
Rows use real links and buttons, not clickable containers with nested actions.

| Collection | Visible content | Actions |
| --- | --- | --- |
| Agents | Agent and expiry | View, confirmed Revoke |
| Connections | Provider, scope count, state, creation time | Reconnect, supported Refresh, confirmed Disconnect |
| Pending approvals | Request details, persistence choice, scope preview | Non-accent Approve and Deny |
| Standing decisions | Existing allow or deny scope | Confirmed Revoke |

The Agents list has no count, status, or Agent Origin Label column.
`activeGrantCount` counts UserGrant records, not permission sets. Do not request per-agent counts.

Use TruncatedText for escaped cell text, with one line and accessible expansion.
Below the small breakpoint, stack row labels and values while retaining every action.
Keep at least 12 agent rows visible at 1920×1080.

## Card and action group

Card groups one topic. It does not receive an accent button merely because it contains an action.
A consent decision has one primary Allow and a secondary Deny across the whole view.

```tsx
<Card>
  <CardHeader>
    <CardTitle className="font-display">{labels.title}</CardTitle>
    <CardDescription>{labels.description}</CardDescription>
  </CardHeader>
  <CardContent>{children}</CardContent>
  <CardFooter className="flex flex-wrap gap-3">
    <Button variant="secondary" onClick={onDeny}>{labels.deny}</Button>
    <Button variant="primary" onClick={onAllow}>{labels.allow}</Button>
  </CardFooter>
</Card>
```

Use neutral `background` and `card` surfaces with semantic borders.
Do not add gradients, strong shadows, or hover lifts.
Display text uses Zalando Sans Variable. Body text uses Inter Variable.
Technical values use JetBrains Mono.

## Input, label, and error

```tsx
<div className="space-y-2">
  <label htmlFor={id} className="text-foreground">{labels.label}</label>
  <Input
    id={id}
    value={value}
    onChange={onChange}
    aria-invalid={Boolean(error)}
    aria-describedby={error ? errorId : hintId}
  />
  {error ? (
    <p id={errorId} role="alert" className="text-destructive">{error}</p>
  ) : (
    <p id={hintId} className="text-muted-foreground">{labels.hint}</p>
  )}
</div>
```

Use unique IDs. Keep a visible label even when the input has a placeholder.
A form summary can link to invalid fields. It does not replace each field's error.
Do not move focus for background data updates.

RadioGroup represents one choice. Checkbox represents independent selections.
Tabs switch panels. Do not imitate either control with a group of styled buttons.

## DropdownMenu and confirmation Dialog

The agent detail overflow menu offers “Revoke all access”.
The menu item opens Dialog. It does not perform revocation directly.

```tsx
<DropdownMenu>
  <DropdownMenuTrigger asChild>
    <Button variant="ghost" aria-label={labels.actions}>
      <MoreHorizontal aria-hidden="true" />
    </Button>
  </DropdownMenuTrigger>
  <DropdownMenuContent>
    <DropdownMenuItem onSelect={onRequestRevoke}>
      {labels.revokeAllAccess}
    </DropdownMenuItem>
  </DropdownMenuContent>
</DropdownMenu>
```

Import the icon from Lucide.
Keep Dialog outside the menu content so menu closure does not remove the confirmation.
Manage focus explicitly when the trigger disappears after a successful operation.

Dialog pairs its title and description with a secondary Cancel and destructive confirmation.
The description names the target and explains the actual effect.
An open modal Dialog makes the background inert and returns focus on cancellation.

## Status, metadata, and feedback

Badge pairs a status label with semantic color. An icon can reinforce that label but cannot replace it.
Use `neutral`, `outline`, `success`, `warning`, `danger`, or `info` Badge variants.
Use `destructive` only as the Button variant for destructive confirmation.

Use a visible timestamp with `font-mono` when the response supplies that timestamp.
Do not invent last-use, publisher, account, or permission-risk fields.
Do not add raw scopes to permission groups.

Skeleton pairs with the future content structure during loading.
EmptyState explains an actual empty result, not a failed request.
Alert or InlineError reports failures next to the affected operation.
Toaster supplies polite, theme-aware notifications after server acceptance.

The Agents empty state uses Wordmark and a one-sentence delegation explanation.
Wordmark receives its accessible name through `label` and uses local outlined artwork.
An off-origin service image uses an Avatar fallback without an external request.

## Spacing, motion, and accessibility

Use shared spacing tokens for label-to-input, input-to-error, field, section, and action gaps.
Let owned components define internal padding and radius.
Use CSS feedback at 120–200 ms with ease-out. Under reduced motion, remove movement.

Every pairing must preserve visible, unobscured focus, keyboard access, and labels at 320 px and 200% zoom.
Every component story needs accessibility checks and reviewed visual baselines in both themes.
Principle XI sets WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.

## Related guides

- [Composition patterns](COMPOSITION_PATTERNS.md): Route and mutation recipes
- [Decision trees](DECISION_TREES.md): Components and action variants
- [Color guide](COLOR_GUIDE.md): Exact semantic OKLCH contract
- [Token guide](TOKEN_GUIDE.md): Shared visual values
- [Accessibility guide](ACCESSIBILITY_GUIDE.md): Focus, labels, contrast, and announcements
