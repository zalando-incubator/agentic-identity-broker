# Component Archetypes

## Authority and example status

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27.
These archetypes define the feature 047 component system.
The examples describe composition requirements, not completed release validation.

Owned shadcn/Radix source stays in `web/src/design-system/components/` under the existing categories.
CVA defines variants, and `cn()` merges classes.
Primitives receive user-facing copy through props. Applications supply that copy from `@copy`.

## Shared visual contract

| Role | Contract |
| --- | --- |
| Page | `bg-background text-foreground` |
| Contained content | `bg-card text-card-foreground` with a 1 px `border-border` boundary |
| Supporting text | `text-muted-foreground` |
| Primary action | `bg-primary text-primary-foreground` |
| Focus | `ring-ring`, visible and unobscured |
| Display text | `font-display`, self-hosted Zalando Sans Variable |
| Body text | `font-sans`, self-hosted Inter Variable |
| Technical values | `font-mono`, self-hosted JetBrains Mono |
| Motion | Central CSS tokens, 120–200 ms ease-out, and no movement under reduced motion |

[COLOR_GUIDE.md](COLOR_GUIDE.md) owns exact OKLCH values for both themes.
[TOKEN_GUIDE.md](TOKEN_GUIDE.md) owns the remaining visual tokens.
Do not add component-local palettes, decorative gradients, strong card shadows, or hover lifts.

## Button

Button represents an action. A navigation link retains link semantics through `asChild`.
The component exposes its variant through `data-variant`.
Its loading state uses Lucide `Loader2` and `aria-busy`.

| Variant | Use |
| --- | --- |
| `primary` | The view's single assigned accent action |
| `secondary` | A supporting action, including consent Deny |
| `outline` | A non-accent action with a visible boundary |
| `ghost` | A compact row action, menu trigger, or other non-accent control |
| `destructive` | Explicit destructive confirmation inside Dialog |

```tsx
<div className="flex flex-wrap gap-3">
  <Button variant="secondary" onClick={onCancel}>{cancelLabel}</Button>
  <Button variant="primary" type="submit" disabled={pending} aria-busy={pending}>
    {submitLabel}
  </Button>
</div>
```

The labels are props in this primitive composition.
Disabled and loading states must prevent duplicate actions without hiding the result or error.
Do not add another accent button in a nested card.

## Card

Card groups related content. It does not imply navigation, permission, or a security guarantee.
Use owned Card parts and central spacing tokens rather than old padding or hover APIs.

```tsx
<Card>
  <CardHeader>
    <CardTitle className="font-display">{title}</CardTitle>
    <CardDescription>{description}</CardDescription>
  </CardHeader>
  <CardContent>{children}</CardContent>
</Card>
```

Use explicit links and buttons within a card.
Do not turn the whole card into a button that contains other buttons.
Dense console collections use Table, not a grid of large cards.

## Input and selection controls

Input replaces the old text-entry wrapper. It uses a semantic surface, input boundary, and focus ring.
A placeholder does not replace a visible label.

```tsx
<div className="space-y-2">
  <label htmlFor={inputId} className="text-foreground">{label}</label>
  <Input
    id={inputId}
    value={value}
    onChange={onChange}
    aria-invalid={Boolean(error)}
    aria-describedby={error ? errorId : hintId}
  />
  {error ? (
    <p id={errorId} role="alert" className="text-destructive">{error}</p>
  ) : (
    <p id={hintId} className="text-muted-foreground">{hint}</p>
  )}
</div>
```

Use unique IDs for each instance.
Use RadioGroup for one choice, Checkbox for independent selections, and Switch for an immediate binary preference.
Use Select for a value selection, not DropdownMenu.
DatePicker preserves native date validation and the minimum-date constraint.
Required permission groups stay locked, with a visible explanation.

## Dialog and other overlays

Dialog provides a focused confirmation or short form.
The owned Radix parts provide its title, description, focus boundary, Escape handling, and focus return.
Dialog content must fit a narrow viewport and remain usable at 200% zoom.

```tsx
<Dialog open={open} onOpenChange={onOpenChange}>
  <DialogContent>
    <DialogHeader>
      <DialogTitle>{title}</DialogTitle>
      <DialogDescription>{description}</DialogDescription>
    </DialogHeader>
    {children}
    <DialogFooter>
      <Button variant="secondary" onClick={onCancel}>{cancelLabel}</Button>
      <Button variant="destructive" onClick={onConfirm} disabled={pending}>
        {confirmLabel}
      </Button>
    </DialogFooter>
  </DialogContent>
</Dialog>
```

The application supplies the named target and effect of revocation.
An open modal Dialog hides the background view from interaction and assistive technology.
Cancel never submits the destructive operation.
A full consent decision uses DecisionShell, not Dialog.

| Overlay | Purpose |
| --- | --- |
| DropdownMenu | Actions, including the agent detail overflow menu |
| Popover | Short contextual content that can contain controls |
| Tooltip | Supplementary help, never the only source of essential content |
| Sheet | Console navigation on narrow screens |

## Status, assets, and feedback

Badge variants are `neutral`, `outline`, `success`, `warning`, `danger`, and `info`.
Badge `danger` describes state. Button `destructive` confirms an operation.
Labels carry meaning without color. Permission groups do not receive risk badges.

Wordmark receives a required `label` prop and uses the local black or white artwork for the resolved theme.
Its compact variant uses the local mark.
Avatar requests only same-origin images and otherwise shows a local fallback.

TruncatedText escapes metadata and provides a labeled expand control.
Its `lines` prop is two for names and descriptions, and one in table cells.
Its required `expandLabel` and `collapseLabel` props come from the caller.

Skeleton preserves content structure during loading.
Alert and InlineError describe errors in context.
EmptyState explains missing data without suggesting an unsupported action.
The Sonner-backed Toaster announces server-confirmed outcomes and follows the resolved theme.

## Shells and dense collections

ConsoleShell and PageHeader compose the console. DecisionShell provides the focused column without console navigation.
Shells are presentational and fetch no data.
Table owns presentation. TanStack Table owns collection state.
Command uses cmdk and receives only acting-user records from its application caller.

Keep Table and Command out of barrels that decision routes import.
See [COMPOSITION_PATTERNS.md](COMPOSITION_PATTERNS.md) for shell and collection composition.

## Required story states

Every component story runs in light and dark themes with accessibility and visual-regression gates.
Cover default, hover, focus-visible, disabled, error, loading, and overlay-open states where applicable.

Principle XI sets the WCAG 2.1 AA floor. Feature 047 targets WCAG 2.2 AA.
Validate rendered contrast, keyboard operation, focus return, announcements, narrow layouts, and reduced motion.
These requirements do not claim that runtime migration or validation is complete.
