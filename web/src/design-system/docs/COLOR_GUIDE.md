# Color System Guide

The color system is a cool ink neutral ramp with one jade accent. Ink carries text, hairlines and surfaces. The accent marks only the primary action, links, the focus ring and the granted state. Every text token meets WCAG 2.1 AA on its intended ground. [ADR 048](../../../../adrs/048-consent-ui-design-language.md) sets this direction.

## Core Color Palette

### Accent (Jade): Actions, Links and the Granted State

One accent, used for three things only. If a surface is jade, it is either the action the screen is for or a grant that is active.

```
Semantic Tokens (Recommended):
--color-accent:       #0a7a66                   accent        (Primary action, links, focus ring; 5.4:1 on white)
--color-accent-hover: #086a59                   accent-hover  (Hover state)
--color-accent-ink:   #ffffff                   accent-ink    (Text on an accent surface)
--color-accent-soft:  rgba(10, 122, 102, 0.08)  accent-soft   (Tint behind an active row or chip)
```

**Usage:**

- Primary buttons: `bg-accent text-accent-ink hover:bg-accent-hover`
- Links and focus rings: `text-accent`, `focus:ring-accent`
- Toggles that are on: `bg-accent`
- An active row or selected chip: `bg-accent-soft border-accent/30`

### Trust (Ink): Headings and Primary Text

The `trust-*` family keeps its names so components keep working. `trust-deep` is ink and `trust` is the accent.

```
Semantic Tokens:
--color-trust-deep:  #111827   trust-deep   (Headings, primary text, ink surfaces)
--color-trust:       #0a7a66   trust        (Links and actions; same as accent)
--color-trust-hover: #086a59   trust-hover  (Same as accent-hover)
--color-trust-light: #e7f3f0   trust-light  (Active navigation, tints; opaque accent-soft)
```

**Examples:**

```tsx
<h1 className="text-trust-deep">Heading</h1>
<a className="text-trust hover:text-trust-hover">Documentation</a>
```

### Success: The Granted State

The granted state shares the accent on purpose: approving a grant and seeing it active are the same fact at two moments.

```
Semantic Tokens:
--color-success-primary: #0a7a66   success-primary  (Granted permissions, success state)
--color-success-hover:   #086a59   success-hover
--color-success-light:   #e7f3f0   success-light    (Success tint)
--color-success-dark:    #075e4f   success-dark     (Success text on a tint; 6.5:1 on white)
```

**Usage:**

- Granted badges: `bg-success-light text-success-dark` with a 30% accent hairline
- Granted rows: `bg-success-light text-success-dark`
- Checkmarks: `text-success-primary`

### Warning (Amber): Attention Without Alarm

```
Semantic Tokens:
--color-warning-primary: #b54708   warning-primary  (5.2:1 on white)
--color-warning-hover:   #93370d   warning-hover
--color-warning-light:   #fffaeb   warning-light
--color-warning-dark:    #7a2e0e   warning-dark

CTA aliases point at the accent, not at amber:
--color-cta:       #0a7a66   cta
--color-cta-hover: #086a59   cta-hover
--color-cta-light: #e7f3f0   cta-light
```

**Usage:**

- Pending states: `bg-warning-light text-warning-dark`
- Attention badges: `bg-warning-primary text-white`
- Warning alerts: `bg-warning-light border-warning-primary`

### Error (Red): Errors and Destructive Actions

```
Semantic Tokens:
--color-error-primary: #b42318   error-primary  (6.3:1 on white)
--color-error-hover:   #912018   error-hover
--color-error-light:   #fef3f2   error-light
--color-error-dark:    #7a271a   error-dark
```

**Usage:**

- Validation messages: `text-error-primary`
- Revoke and delete buttons: `bg-error-primary text-white hover:bg-error-hover`
- Error alerts: `bg-error-light border-error-primary text-error-dark`

### Info (Slate Blue): Informational Content

```
--color-info-primary: #3056d3   info-primary  (5.6:1 on white)
--color-info-hover:   #2447b3   info-hover
--color-info-light:   #eef2fc   info-light
--color-info-dark:    #1e3a8a   info-dark
```

### Cool Neutrals: Ground, Hairlines and Text

Replaces standard Tailwind grays with a cool ink ramp. Surfaces and hairlines also have direct names.

```
Surfaces and hairlines:
--color-page:        #fbfcfd                  page         (Page background)
--color-surface:     #ffffff                  surface      (Cards, dialogs, inputs)
--color-raised:      #f0f2f6                  raised       (Chips, code, table heads, inactive toggles)
--color-line:        rgba(17, 24, 39, 0.08)   line         (Card borders, rules)
--color-line-strong: rgba(17, 24, 39, 0.14)   line-strong  (Input and outline-button borders)

Neutral scale:
--color-neutral-50:  #fbfcfd   neutral-50   (Page background)
--color-neutral-100: #f0f2f6   neutral-100  (Raised surfaces)
--color-neutral-200: #e3e6ec   neutral-200  (Card borders, opaque hairline)
--color-neutral-300: #d2d6de   neutral-300  (Input borders, inactive toggles)
--color-neutral-400: #a9afbd   neutral-400  (Placeholders, disabled text)
--color-neutral-500: #6b7487   neutral-500  (Tertiary text, icons; 4.9:1 on white)
--color-neutral-600: #4b5565   neutral-600  (Secondary text; 7.2:1 on white)
--color-neutral-700: #364152   neutral-700  (Body text)
--color-neutral-800: #1f2937   neutral-800  (Emphasized text)
--color-neutral-900: #111827   neutral-900  (Headings; same as trust-deep)

Semantic Aliases:
--color-text-primary:   #111827   text-primary
--color-text-secondary: #4b5565   text-secondary
--color-text-tertiary:  #6b7487   text-tertiary
--color-text-disabled:  #a9afbd   text-disabled

--color-bg-primary:   #fbfcfd   bg-primary
--color-bg-secondary: #f0f2f6   bg-secondary
--color-bg-elevated:  #ffffff   bg-elevated

--color-border-primary:   #d2d6de   border-primary
--color-border-secondary: #e3e6ec   border-secondary
--color-border-focus:     #0a7a66   border-focus
```

**Migration from Gray:**

| Old (Gray) | New (Cool Neutral) | Use Case            |
| ---------- | ------------------ | ------------------- |
| `gray-50`  | `neutral-50`       | Page backgrounds    |
| `gray-100` | `neutral-100`      | Raised surfaces     |
| `gray-200` | `neutral-200`      | Card borders        |
| `gray-300` | `neutral-300`      | Input borders       |
| `gray-400` | `neutral-400`      | Placeholders        |
| `gray-500` | `neutral-500`      | Tertiary text, icons |
| `gray-600` | `neutral-600`      | Secondary text      |
| `gray-700` | `neutral-700`      | Body text           |
| `gray-900` | `trust-deep`       | Headings            |

### Headings: `text-trust-deep`, not `text-primary`

`text-primary` and `text-trust-deep` resolve to the same ink, but "primary" reads as "primary brand color" as often as "primary text". Use `text-trust-deep` on headings so the intent is clear, `text-neutral-700` or `text-secondary` on paragraphs, and `text-tertiary` on metadata. Do not use `text-trust` on headings: it is the accent, and the accent is for actions, links and the granted state.

## Color Accessibility

### Contrast Ratios (WCAG 2.1 AA Compliant)

| Text Color                  | Background       | Contrast Ratio | Status |
| --------------------------- | ---------------- | -------------- | ------ |
| Ink (#111827)               | White (#ffffff)  | 17.4:1         | AAA    |
| Secondary ink (#4b5565)     | White (#ffffff)  | 7.2:1          | AAA    |
| Tertiary ink (#6b7487)      | White (#ffffff)  | 4.9:1          | AA     |
| Accent (#0a7a66)            | White (#ffffff)  | 5.4:1          | AA     |
| White (#ffffff)             | Accent (#0a7a66) | 5.4:1          | AA     |
| Error (#b42318)             | White (#ffffff)  | 6.3:1          | AA     |
| Warning (#b54708)           | White (#ffffff)  | 5.2:1          | AA     |
| Ink (#111827)               | Raised (#f0f2f6) | 15.6:1         | AAA    |

**Rule:** Verify custom combinations with the [WebAIM Contrast Checker](https://webaim.org/resources/contrastchecker/).

## Using Colors in Components

### In Tailwind Classes

**✅ DO: Use semantic tokens**

```tsx
// Background colors
className = 'bg-trust-deep'; // Primary brand
className = 'bg-success-primary'; // Success
className = 'bg-cta'; // Warning/CTA
className = 'bg-error-primary'; // Error
className = 'bg-secondary'; // Section background (neutral-100)

// Text colors
className = 'text-primary'; // Primary text (trust-deep)
className = 'text-secondary'; // Secondary text (neutral-600)
className = 'text-success-primary'; // Success text
className = 'text-error-primary'; // Error text
```

**❌ DON'T: Use extended palette classes**

### In CSS Custom Properties

```css
/* For dynamic theming or special cases */
color: var(--color-text-primary);
background-color: var(--color-primary-trust);
border-color: var(--color-neutral-200);
```

### Component Guidelines

#### Buttons

- **Primary**: `bg-trust-deep text-white hover:bg-trust-hover`
- **Secondary**: `bg-success-primary text-white hover:bg-success-hover`
- **Outline**: `border-neutral-300 text-trust-deep hover:bg-neutral-50`
- **Ghost**: `text-trust-deep hover:bg-neutral-100`
- **Danger**: `bg-error-primary text-white hover:bg-error-hover`

**Example:**

```tsx
<Button variant="primary" className="bg-trust-deep text-white">
  Primary Action
</Button>
```

#### Status Badges

- **Active/Approved**: `bg-success-primary text-white`
- **Pending**: `bg-warning-primary text-trust-deep`
- **Revoked/Error**: `bg-error-primary text-white`
- **Inactive**: `bg-neutral-300 text-neutral-600`

**Example:**

```tsx
<Badge variant="success">Granted</Badge>
<Badge variant="warning">Pending</Badge>
```

#### Form Inputs

- **Border (default)**: `border-neutral-300`
- **Border (focus)**: `focus:border-trust focus:ring-trust`
- **Border (error)**: `border-error-primary focus:ring-error-primary`
- **Background (disabled)**: `bg-neutral-50`
- **Text**: `text-neutral-900 placeholder-neutral-500`

**Example:**

```tsx
<input
  className="border-neutral-300 text-neutral-900 focus:border-trust focus:ring-1 focus:ring-trust"
  placeholder="Enter value"
/>
```

#### Cards

- **Background**: `bg-elevated` (white) or `bg-primary` (neutral-50)
- **Border**: `border-neutral-200`
- **Text**: `text-trust-deep` (headings), `text-neutral-700` (body)

**Example:**

```tsx
<div className="bg-elevated border border-neutral-200 rounded-xl p-6">
  <h3 className="text-trust-deep font-semibold mb-2">Card Title</h3>
  <p className="text-neutral-700">Card body content</p>
</div>
```

#### Alerts

- **Success**: `bg-success-light border-success-primary text-success-dark`
- **Warning**: `bg-warning-light border-warning-primary text-warning-dark`
- **Error**: `bg-error-light border-error-primary text-error-dark`
- **Info**: `bg-info-light border-info-primary text-info-dark`

**Example:**

```tsx
<Alert variant="success">Permission granted successfully</Alert>
<Alert variant="error">Access denied</Alert>
```

## Semantic Color Tokens

`web/src/design-system/tokens/colors.ts` mirrors the CSS tokens for programmatic access:

```typescript
semanticColors.accent.default   // var(--color-accent)
semanticColors.surface.line     // var(--color-line)
semanticColors.trust.deep       // var(--color-trust-deep)
semanticColors.success.primary  // var(--color-success-primary)
semanticColors.error.primary    // var(--color-error-primary)
```

## Color Combinations to Avoid

❌ **Don't use**:

- The accent on anything that is not an action, a link or a granted state
- Red on amber backgrounds (poor readability)
- Neutral-400 for text that must be read (it is for placeholders and disabled text)
- A single color to indicate state (use icon + color combination)
- Gradients, glows or textures anywhere

✅ **Do use**:

- Ink on the page or surface for everything that is read
- Color + icon/symbol for status indication
- Semantic tokens consistently across components
- Hairlines for containment, `shadow-float` for overlays only

## Dark Mode Considerations

The docs site defines the dark counterpart of this ramp (page #0b0d14, surface #11141c, raised #1a1e28, ink #eef0f5, accent #4fd1b5). The consent UI does not ship a dark theme yet: components still hard-code `bg-white` in many places, so a dark theme is a separate structural change (see ADR 048).

## Resources

- [WCAG 2.1 Color Contrast Guidelines](https://www.w3.org/WAI/WCAG21/Understanding/contrast-minimum.html)
- [WebAIM Contrast Checker](https://webaim.org/resources/contrastchecker/)
- [Tailwind Color Palette](https://tailwindcss.com/docs/customizing-colors)

---

## Summary

The color system has four parts:

1. **Ink** - text, hairlines and surfaces, in a cool neutral ramp
2. **Accent (jade)** - the primary action, links, focus and the granted state
3. **Amber and red** - attention and error, in a sober register
4. **Info** - slate blue for informational content

Each color meets WCAG 2.1 AA and the accent is used sparingly enough that it means something when it appears.
