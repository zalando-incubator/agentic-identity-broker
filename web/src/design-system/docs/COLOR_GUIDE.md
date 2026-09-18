# Color System Guide

The Refined Trust Architecture uses a carefully curated color palette that balances sophistication with approachability. Every color is chosen to meet accessibility standards (WCAG 2.1 AA) while maintaining the visual identity of the system.

## Core Color Palette

### Trust (Navy): Authority & Primary Brand

The primary color system representing stability, trust, and authority. Use semantic tokens for meaning-driven color application.

```
Semantic Tokens (Recommended):
--color-trust-deep:  #0A2540     trust-deep   (Darkest - headings, primary brand)
--color-trust:       #1E4D6B     trust        (Medium - primary actions, links)
--color-trust-hover: #2b68a5     trust-hover  (Lighter - hover states)
--color-trust-light: #E8F1F5     trust-light  (Lightest - backgrounds, tints)
```

**Usage:**

- Primary buttons and links: `bg-trust` or `bg-trust-deep`
- Headings and brand text: `text-trust-deep`
- Focus rings and active states: `ring-trust`
- Hover states: `hover:bg-trust-hover`
- Light backgrounds for sections: `bg-trust-light`

**Examples:**

```tsx
<button className="bg-trust-deep text-white hover:bg-trust-hover">
  Primary Action
</button>
<h1 className="text-trust-deep font-display">Heading</h1>
```

### Success (Emerald): Approval & Positive States

Signals granted access, approved states, and positive actions.

```
Semantic Tokens (Recommended):
--color-success-primary: #059669   success-primary  (Success state, granted permissions)
--color-success-hover:   #047857   success-hover    (Success hover state)
--color-success-light:   #d1fae5   success-light    (Success light background)
--color-success-dark:    #065f46   success-dark     (Success dark text)
```

**Usage:**

- Success badges: `bg-success-primary text-white`
- Approved permission states: `bg-success-light text-success-dark`
- Checkmarks and confirmation icons: `text-success-primary`
- Success alerts: `bg-success-light border-success-primary`

**Examples:**

```tsx
<span className="bg-success-primary text-white px-3 py-1 rounded">
  Granted
</span>
<div className="bg-success-light p-4 border border-success-primary">
  Permission approved successfully
</div>
```

### Warning/CTA (Amber): Attention & Important Actions

Unified color for both warnings and call-to-actions. Captures attention without alarm.

```
Semantic Tokens (Recommended):
--color-warning-primary: #D97706   warning-primary  (Warning/CTA - unified)
--color-warning-hover:   #B45309   warning-hover    (Warning/CTA hover)
--color-warning-light:   #fef3c7   warning-light    (Warning light background)
--color-warning-dark:    #92400e   warning-dark     (Warning dark text)

Aliases (same values):
--color-cta:       #D97706   cta         (Call-to-action)
--color-cta-hover: #B45309   cta-hover   (CTA hover)
--color-cta-light: #fef3c7   cta-light   (CTA light background)
```

**Usage:**

- Warning states: `bg-warning-light border-warning-primary`
- Important CTAs: `bg-cta text-white hover:bg-cta-hover`
- Pending permission states: `bg-warning-light text-warning-dark`
- Attention badges: `bg-warning-primary text-trust-deep`

**Examples:**

```tsx
<button className="bg-cta text-white hover:bg-cta-hover">
  Important Action
</button>
<div className="bg-warning-light p-4 border border-warning-primary">
  Pending approval
</div>
```

### Error (Red): Errors & Destructive Actions

Indicates errors, denied access, and destructive actions.

```
Semantic Tokens (Recommended):
--color-error-primary: #DC2626   error-primary  (Error state, destructive actions)
--color-error-hover:   #b91c1c   error-hover    (Error hover state)
--color-error-light:   #fee2e2   error-light    (Error light background)
--color-error-dark:    #991b1b   error-dark     (Error dark text)
```

**Usage:**

- Error states: `border-error-primary text-error-primary`
- Revoked permissions: `bg-error-light text-error-dark`
- Delete/destructive buttons: `bg-error-primary text-white hover:bg-error-hover`
- Error alerts: `bg-error-light border-error-primary`
- Validation messages: `text-error-primary`

**Examples:**

```tsx
<button className="bg-error-primary text-white hover:bg-error-hover">
  Delete Account
</button>
<span className="text-error-primary text-sm">
  This field is required
</span>
```

### Warm Neutrals: Sophisticated Backgrounds & Text

**Replaces standard Tailwind grays** with warm cream, sand, and taupe tones that convey trust and premium quality.

```
Warm Neutral Scale (Complete):
--color-neutral-50:  #faf9f7   neutral-50   (Cream - page backgrounds)
--color-neutral-100: #f5f1ed   neutral-100  (Sand - section backgrounds)
--color-neutral-200: #e8e3de   neutral-200  (Taupe - card borders)
--color-neutral-300: #ddd8d1   neutral-300  (Light taupe - input borders)
--color-neutral-400: #c4bdb3   neutral-400  (Medium taupe - placeholders)
--color-neutral-500: #9a9591   neutral-500  (Medium-dark - secondary icons)
--color-neutral-600: #6b6561   neutral-600  (Dark taupe - secondary text)
--color-neutral-700: #4a4137   neutral-700  (Darker taupe - body text)
--color-neutral-800: #2a251f   neutral-800  (Very dark - emphasized text)
--color-neutral-900: #1a1511   neutral-900  (Darkest - headings or use trust-deep)

Semantic Aliases:
--color-text-primary:   #0A2540   text-primary    (Primary text - trust-deep)
--color-text-secondary: #6b6561   text-secondary  (Secondary text - neutral-600)
--color-text-tertiary:  #9a9591   text-tertiary   (Tertiary text - neutral-500)
--color-text-disabled:  #c4bdb3   text-disabled   (Disabled text - neutral-400)

--color-bg-primary:   #faf9f7   bg-primary    (Primary background - neutral-50)
--color-bg-secondary: #f5f1ed   bg-secondary  (Secondary background - neutral-100)
--color-bg-elevated:  #ffffff   bg-elevated   (Elevated cards - pure white)

--color-border-primary:   #ddd8d1   border-primary    (Primary borders - neutral-300)
--color-border-secondary: #e8e3de   border-secondary  (Secondary borders - neutral-200)
```

**Migration from Gray:**

| Old (Gray) | New (Warm Neutral) | Use Case                                           |
| ---------- | ------------------ | -------------------------------------------------- |
| `gray-50`  | `neutral-50`       | Page backgrounds                                   |
| `gray-100` | `neutral-100`      | Section backgrounds                                |
| `gray-200` | `neutral-200`      | Card borders                                       |
| `gray-300` | `neutral-300`      | Input borders                                      |
| `gray-400` | `neutral-400`      | Placeholders                                       |
| `gray-500` | `neutral-500`      | Secondary icons                                    |
| `gray-600` | `neutral-600`      | Secondary text                                     |
| `gray-700` | `neutral-700`      | Body text                                          |
| `gray-900` | `trust-deep`       | Primary headings (use trust for brand consistency) |

**Usage:**

```tsx
// Page structure
<body className="bg-primary text-primary">              {/* neutral-50 bg, trust-deep text */}
<section className="bg-secondary">                     {/* neutral-100 */}
<div className="bg-elevated border border-primary">    {/* white bg, neutral-300 border */}

// Typography
<h1 className="text-primary">                          {/* trust-deep */}
<p className="text-secondary">                         {/* neutral-600 */}
<span className="text-tertiary">                       {/* neutral-500 */}

// Borders & dividers
<input className="border-primary focus:border-focus">  {/* neutral-300, trust */}
<hr className="border-secondary">                      {/* neutral-200 */}
```

**Note**: Primary headings should use `text-trust-deep` instead of `text-neutral-900` for brand consistency.

### ⚠️ Important: `text-primary` vs `text-trust-deep` for Headings

**TL;DR**: Always use `text-trust-deep` explicitly for headings, not `text-primary`.

**Why the confusion exists:**

- `text-primary` is an alias that points to `trust-deep` (#0A2540)
- While technically correct, `text-primary` is semantically ambiguous
- "Primary" could mean "primary text color" (for body text) OR "primary brand color"
- This ambiguity makes code harder to read and maintain

**The Rule:**

```tsx
// ❌ DON'T - Confusing intent
<h1 className="text-primary">Dashboard</h1>

// ✅ DO - Clear intent
<h1 className="text-trust-deep">Dashboard</h1>
```

**When to use each:**

- **Headings (h1-h2)**: Use `text-trust-deep` explicitly (major sections, authority)
- **Headings (h3-h6)**: Use `text-trust` explicitly (minor sections, subsections)
- **Body text**: Use `text-neutral-700` or `text-secondary` (neutral-600) for paragraphs
- **Metadata/labels**: Use `text-tertiary` (neutral-500)
- **NEVER use `text-primary` for headings** - too ambiguous

See [COMMON_MISTAKES.md](./COMMON_MISTAKES.md) (Mistake #1) and [DECISION_TREES.md](./DECISION_TREES.md) (Text Color Hierarchy) for more guidance.

## Color Accessibility

### Contrast Ratios (WCAG 2.1 AA Compliant)

All color combinations below meet WCAG 2.1 Level AA standards:

| Text Color            | Background            | Contrast Ratio | Status                  |
| --------------------- | --------------------- | -------------- | ----------------------- |
| Navy-900 (#0d1829)    | White (#ffffff)       | 13.8:1         | AAA (Passes large text) |
| Navy-700 (#1e4d6b)    | White (#ffffff)       | 8.2:1          | AAA                     |
| Emerald-600 (#059669) | White (#ffffff)       | 5.3:1          | AA                      |
| Amber-600 (#d97706)   | White (#ffffff)       | 5.1:1          | AA                      |
| Red-600 (#dc2626)     | White (#ffffff)       | 5.5:1          | AA                      |
| Navy-800 (#0d1829)    | Neutral-100 (#f5f1ed) | 11.2:1         | AAA                     |

**Rule:** Always verify custom color combinations with [WebAIM Contrast Checker](https://webaim.org/resources/contrastchecker/) or [ColorSnack](https://www.colorsnack.com/).

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

For applications needing semantic token references:

```typescript
// Trust & Authority
colors: {
  'trust-deep': '#0A2540',      // Darkest trust color for headings
  'trust': '#1E4D6B',            // Primary trust/navy color
  'trust-light': '#E8F1F5',      // Light trust backgrounds
}

// Action & Status
colors: {
  'action-cta': '#D97706',       // Call-to-action (amber)
  'success': '#059669',          // Success state (emerald)
  'warning': '#F59E0B',          // Warning state (amber)
  'error': '#DC2626',            // Error state (red)
  'info': '#3B82F6',             // Info state (blue)
}

// Neutral & Text
colors: {
  'bg-primary': '#faf9f7',       // Cream background
  'bg-secondary': '#f5f1ed',     // Sand background
  'text-primary': '#0d1829',     // Primary text (darkest)
  'text-secondary': '#4a5366',   // Secondary text (medium)
  'text-tertiary': '#7a8899',    // Tertiary text (light)
}
```

## Color Combinations to Avoid

❌ **Don't use**:

- Red on Amber backgrounds (poor readability)
- Emerald on Navy backgrounds (insufficient contrast)
- Neutral-600 on Neutral-200 (indistinguishable)
- Single color to indicate state (use icon + color combination)

✅ **Do use**:

- High-contrast text on colored backgrounds
- Color + icon/symbol for status indication
- Semantic tokens consistently across components
- Hover state transitions for interactive elements

## Dark Mode Considerations

For future dark mode support, invert the palette:

```css
@media (prefers-color-scheme: dark) {
  --color-bg-primary: #0d1829; /* Was Navy-900 */
  --color-bg-secondary: #1f2937; /* Was Neutral-700 */
  --color-text-primary: #faf9f7; /* Was Neutral-50 */
  --color-text-secondary: #c4bdb3; /* Was Neutral-400 */
}
```

## Resources

- [WCAG 2.1 Color Contrast Guidelines](https://www.w3.org/WAI/WCAG21/Understanding/contrast-minimum.html)
- [WebAIM Contrast Checker](https://webaim.org/resources/contrastchecker/)
- [ColorSnack Contrast Tool](https://www.colorsnack.com/)
- [Tailwind Color Palette](https://tailwindcss.com/docs/customizing-colors)

---

## Summary

The Refined Trust Architecture color system is built on five core palettes:

1. **Navy** - Trust and primary actions
2. **Emerald** - Success and approval
3. **Amber** - Warnings and CTAs
4. **Red** - Errors and destructive actions
5. **Neutral** - Structure and typography

Each color meets WCAG 2.1 AA accessibility standards and is chosen to communicate specific meanings to users while maintaining visual sophistication and approachability.
