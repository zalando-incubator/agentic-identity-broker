# Design Token Guide

Design tokens are the visual design decisions encoded as data. This guide covers the tokens available in the Refined Trust Architecture design system and how to use them.

## Overview

Design tokens in this system are managed through:

- **Tailwind CSS v4**: CSS variables via `@theme` directive
- **CSS Custom Properties**: For runtime customization
- **TypeScript Type System**: For compile-time safety
- **Class Variance Authority (CVA)**: For component variants

## Color System

The color palette uses **semantic tokens** based on meaning, brand identity, and accessibility requirements. All colors meet WCAG 2.1 AA standards.

### Why Semantic Tokens?

Semantic tokens provide meaning-driven color naming that improves code readability and maintainability.

**✅ DO: Use semantic tokens**

```tsx
<button className="bg-trust text-white hover:bg-trust-hover">
  Primary Action
</button>
<div className="bg-neutral-50 text-neutral-700 border-neutral-200">
  Content
</div>
```

**❌ DON'T: Use extended palette classes**

### Semantic Color Tokens

```typescript
// PRIMARY - Trust & Authority (Navy)
trust-deep:  #111827     // Darkest - headings, primary brand
trust:       #0a7a66     // Medium - primary actions, links
trust-hover: #086a59     // Lighter - hover states
trust-light: #e7f3f0     // Lightest - backgrounds, tints

// ACTION - CTA (Unified with Warning)
cta:       #b54708       // Call-to-action and warnings
cta-hover: #93370d       // CTA hover state
cta-light: #fffaeb       // CTA light background

// SUCCESS - Emerald tones for positive actions
success-primary: #0a7a66 // Success state, granted permissions
success-hover:   #086a59 // Success hover state
success-light:   #e7f3f0 // Success light background
success-dark:    #075e4f // Success dark text

// ERROR - Red tones for destructive actions
error-primary: #b42318   // Error state, destructive actions
error-hover:   #912018   // Error hover state
error-light:   #fef3f2   // Error light background
error-dark:    #7a271a   // Error dark text

// WARNING - Unified with CTA (Amber)
warning-primary: #b54708 // Warning state (same as CTA)
warning-hover:   #93370d // Warning hover state
warning-light:   #fffaeb // Warning light background
warning-dark:    #7a2e0e // Warning dark text

// INFO - Blue tones for informational content
info-primary: #3056d3   // Info state
info-hover:   #2447b3   // Info hover state
info-light:   #eef2fc   // Info light background
info-dark:    #1e3a8a   // Info dark text

// WARM NEUTRALS - Sophisticated cream/sand/taupe (replaces gray)
neutral-50:  #fbfcfd    // Cream - page backgrounds
neutral-100: #f0f2f6    // Sand - section backgrounds
neutral-200: #e3e6ec    // Taupe - card borders
neutral-300: #d2d6de    // Light taupe - input borders
neutral-400: #a9afbd    // Medium taupe - placeholders
neutral-500: #6b7487    // Medium-dark - secondary icons
neutral-600: #4b5565    // Dark taupe - secondary text
neutral-700: #364152    // Darker taupe - body text
neutral-800: #1f2937    // Very dark - emphasized text
neutral-900: #111827    // Darkest - headings (or use trust-deep)

// SEMANTIC ALIASES - Contextual color names
text-primary:   #111827  // Primary text (trust-deep)
text-secondary: #4b5565  // Secondary text (neutral-600)
text-tertiary:  #6b7487  // Tertiary text (neutral-500)
text-disabled:  #a9afbd  // Disabled text (neutral-400)

bg-primary:   #fbfcfd    // Primary background (neutral-50)
bg-secondary: #f0f2f6    // Secondary background (neutral-100)
bg-elevated:  #ffffff    // Elevated cards (pure white)

border-primary:   #d2d6de // Primary borders (neutral-300)
border-secondary: #e3e6ec // Secondary borders (neutral-200)
border-focus:     #0a7a66 // Focus ring (trust)
```

### Color Usage

| Use Case            | Token                                | Example                                    |
| ------------------- | ------------------------------------ | ------------------------------------------ |
| Primary buttons     | bg-trust-deep                        | `<Button variant="primary">`               |
| Success status      | bg-success-primary                   | `<Badge variant="success">Granted</Badge>` |
| Warning/Pending     | bg-warning-primary or bg-cta         | `<Badge variant="warning">Pending</Badge>` |
| Error/Denied        | bg-error-primary                     | `<Alert variant="error">`                  |
| Link hover          | hover:text-trust-hover               | Navigation links                           |
| Disabled state      | text-disabled or bg-neutral-50       | Inactive form inputs                       |
| Borders             | border-primary or border-neutral-300 | Card outlines                              |
| Backgrounds         | bg-primary or bg-neutral-50          | Page backgrounds                           |
| Section backgrounds | bg-secondary or bg-neutral-100       | Section containers                         |
| Body text           | text-neutral-700                     | Paragraph text                             |
| Headings (h1-h2)    | text-trust-deep                      | Major headings for authority               |
| Headings (h3-h6)    | text-trust                           | Minor headings, subsections                |

### ⚠️ Important: Semantic Token Naming Clarification

**Always use explicit color tokens for headings, not semantic aliases:**

```tsx
// ❌ DON'T - Ambiguous semantic alias
<h1 className="text-primary">Dashboard</h1>

// ✅ DO - Explicit brand color
<h1 className="text-trust-deep">Dashboard</h1>
<h3 className="text-trust">Subsection</h3>
```

**Why `text-primary` is confusing for headings:**

- `text-primary` points to `trust-deep` (#111827) technically, but semantically it's unclear
- "Primary" could mean "primary text" (body text) OR "primary brand color" (headings)
- This creates ambiguity that makes code harder to maintain

**Semantic Token Rules:**

- `text-trust-deep` → h1, h2 (major sections, authority)
- `text-trust` → h3-h6 (minor sections, subsections)
- `text-secondary` → Supporting text, metadata (neutral-600)
- `text-tertiary` → Labels, timestamps (neutral-500)
- `text-neutral-700` → Body paragraphs (explicit and clear)

**When to use semantic aliases:**

- ✅ `text-secondary` for supporting text (consistent, clear intent)
- ✅ `text-tertiary` for metadata (consistent, clear intent)
- ✅ `bg-primary` / `bg-secondary` for backgrounds (layouts, not brand)
- ❌ `text-primary` for headings (too ambiguous - use `text-trust-deep` instead)

See [COMMON_MISTAKES.md](./COMMON_MISTAKES.md) (Mistake #1) and [DECISION_TREES.md](./DECISION_TREES.md) (Text Color Hierarchy) for detailed guidance.

### Color Accessibility

All semantic colors meet WCAG AA contrast requirements:

- Text contrast: Minimum 4.5:1 (for body text)
- UI component contrast: Minimum 3:1 (for graphics)
- Use ColorSnack or WebAIM for verification

**When choosing colors:**

1. Prefer semantic tokens (trust, success, warning, error)
2. Use warm neutrals (neutral-\*) instead of standard grays
3. Check contrast with intended background (WCAG AA minimum 4.5:1 for text)
4. Consider colorblind accessibility (don't rely on color alone)
5. Test with accessibility tools (WebAIM, ColorSnack)

## Spacing System

Tailwind's spacing scale follows a consistent 4px base unit (0.25rem).

```typescript
// Core Spacing Scale
0; // 0px       - No space (useful for removing margins)
px; // 1px       - Divider lines
0.5; // 2px       - Very tight spacing
1; // 4px       - xs: Extra small spacing
2; // 8px       - Extra small
3; // 12px      - Small
4; // 16px      - md: Medium (default)
6; // 24px      - lg: Large
8; // 32px      - Extra large
10; // 40px      - XXL
12; // 48px      - XXXL
16; // 64px      - 2XL
```

### Named Spacing (in components)

Components use semantic names that map to this scale:

```typescript
size: 'xs' | 'sm' | 'md' | 'lg' | 'xl'

// Typical mapping
'xs' → 0.5  (2px)   or  2 (8px)    - Compact
'sm' → 3    (12px)                 - Small
'md' → 4    (16px)                 - Default
'lg' → 6    (24px)                 - Large
'xl' → 8    (32px)                 - Extra large
```

### Spacing Usage

```tsx
// Padding (internal spacing)
<Card padding="lg" />           // 24px internal padding
<Button size="sm" />            // Compact button

// Gaps (space between children)
<Stack gap="md" />              // 16px between items
<Grid gap="lg" />               // 24px between grid cells

// Margins (external spacing)
<div className="mb-4" />        // 16px margin bottom
<section className="my-6" />    // 24px margin top/bottom

// Margins within components
'mt-1' → 4px  (top)
'mt-2' → 8px  (top)
'mb-3' → 12px (bottom)
'mb-4' → 16px (bottom)
```

## Typography System

Typography is managed through semantic HTML and Tailwind's text utilities.

### Font Families

```typescript
// Display/Headings
'font-display' → Geist, ui-sans-serif, system-ui
  - Weight 500 (medium) for every heading; nothing is bold
  - Letter-spacing: -0.015em, -0.025em on h1
  - Usage: h1, h2, h3, page titles, modal headers

// Body/UI
'font-sans' → Geist, ui-sans-serif, system-ui
  - Weight 400 (regular) for body text
  - Weight 500 (medium) for labels, names and emphasis
  - Default letter-spacing
  - Usage: Body text, buttons, form labels, descriptions
  - Base size: 1rem (16px)
  - Line height: 1.6 for comfortable reading

// Monospace (Technical Values)
'font-mono' → Geist Mono, ui-monospace
  - Weight 400 for technical content
  - Usage: OAuth scopes, agent IDs, dates, API tokens, code blocks
  - Typically displayed on a raised background with a hairline
```

**Rationale:**

- One family at two optical roles keeps the consent UI exact and sober, and matches the public docs site (ADR 048)
- **Geist** reads cleanly at 400 and 500, so hierarchy comes from size and color rather than weight
- **Geist Mono** sets identifiers, scopes and dates apart as facts

### Font Sizes & Line Heights

```typescript
// Tailwind text-* scale
'text-xs'     → 12px  / 1rem      (overline text)
'text-sm'     → 14px  / 1.25rem   (small/secondary)
'text-base'   → 16px  / 1.5rem    (body text - default)
'text-lg'     → 18px  / 1.75rem   (heading 4)
'text-xl'     → 20px  / 1.75rem   (heading 3)
'text-2xl'    → 24px  / 2rem      (heading 2)
'text-3xl'    → 30px  / 2.25rem   (heading 1)
'text-4xl'    → 36px  / 2.25rem   (display)
```

### Font Weights

```typescript
'font-normal'   → 400   (regular text)
'font-medium'   → 500   (emphasized text)
'font-semibold' → 600   (strong emphasis)
'font-bold'     → 700   (headings)
```

### Typography Usage

```tsx
// Semantic HTML + Tailwind
<h1 className="text-4xl font-bold text-trust-deep">Main Title</h1>
<h2 className="text-2xl font-bold text-trust-deep">Section</h2>
<h3 className="text-xl font-semibold text-trust">Subsection</h3>
<p className="text-base font-normal text-neutral-700">Body text</p>
<p className="text-sm text-secondary">Secondary text (neutral-600)</p>
<span className="text-xs text-tertiary">Overline (neutral-500)</span>

// Component sizing
<Button size="sm" />    // Smaller text inside
<Badge size="md" />     // Default text sizing
<Heading level={2} />   // Semantic heading level
```

## Shadow System

Shadows provide depth and hierarchy in the interface.

Cards, panels and buttons do not cast shadows. They sit on the page behind a hairline (`border border-neutral-200` or `border-line`). One floating shadow lifts overlays.

```typescript
'shadow-float'  // 0 1px 2px rgba(17,24,39,.04), 0 12px 32px rgba(17,24,39,.08)
                // Modals, dropdowns, popovers, toasts. Nothing else.

// Shadow usage
<Card />                                  // Hairline, no shadow
<Modal className="shadow-float" />        // The one floating shadow
<Dropdown className="shadow-float" />
```

## Border Radius

Consistent rounded corners throughout the system.

```typescript
// Tailwind radius scale
'rounded-none'  → 0px       (sharp corners)
'rounded-sm'    → 0.125rem  (1px - very subtle)
'rounded'       → 0.25rem   (4px - default)
'rounded-md'    → 0.375rem  (6px)
'rounded-lg'    → 0.5rem    (8px - prominent)
'rounded-xl'    → 0.75rem   (12px)
'rounded-full'  → 9999px    (circular)

// Typical usage
<Card className="rounded" />           // 4px (default)
<Badge className="rounded-md" />       // 6px
<Avatar className="rounded-lg" />      // 8px
<Button className="rounded-lg" />      // 8px
<Checkbox className="rounded-sm" />    // 1px (checkbox)
```

## Transitions & Animations

Smooth, purposeful animations that enhance usability.

```typescript
// Transition durations
150ms   → Fast interactions (hover effects, small changes)
300ms   → Default (most component animations)
500ms   → Slow (page transitions, large changes)

// Easing functions
'cubic-bezier(0.4, 0, 0.2, 1)'  → Default (smooth)
'cubic-bezier(0.4, 0, 1, 1)'     → Ease-out (enter)
'cubic-bezier(0, 0, 0.2, 1)'     → Ease-in (exit)

// Usage
<Transition duration={300} easing="ease-out">
  <Modal />
</Transition>
```

## Z-Index Scale

Layering strategy for overlays and stacked elements.

```typescript
// Tailwind z-index scale
'z-0'   → 0       (default)
'z-10'  → 10      (tooltips, popovers)
'z-20'  → 20      (dropdowns)
'z-30'  → 30      (modals, important overlays)
'z-40'  → 40      (notification toasts)
'z-50'  → 50      (full-page overlays, critical modals)

// Component defaults
Tooltip  → z-10
Dropdown → z-50
Modal    → z-50
Toast    → z-40
Popover  → z-20
```

## Using Design Tokens in Code

### Via Tailwind Classes (Recommended)

```tsx
// Most common approach
<div className="bg-primary text-neutral-700 p-4 rounded-lg shadow">
  <h2 className="text-2xl font-bold text-trust-deep">Title</h2>
  <p className="mt-2 text-sm text-secondary">Description</p>
</div>
```

### Via CSS Variables

```tsx
// If needing dynamic theming
<div
  style={{
    backgroundColor: 'var(--color-bg-primary)',
    color: 'var(--color-text-primary)',
    padding: 'var(--spacing-4)',
    borderRadius: 'var(--radius-lg)',
  }}
>
  {children}
</div>
```

### Via Component Props

```tsx
// Most semantic approach
<Card padding="lg" hover="lift" backgroundColor="neutral-50">
  <heading>Title</heading>
  <p>Description</p>
</Card>

<Stack gap="md" direction="column" align="start">
  <Button variant="primary" size="lg" />
  <Button variant="secondary" size="lg" />
</Stack>
```

## Token Customization

### For Application-Specific Theming

1. **Token overrides** in `web/src/design-system/tokens/colors.css` (the `@theme` block is the single source of truth; `tailwind.config.ts` is not loaded by Tailwind v4 here):

```css
@theme {
  --color-accent: #0a7a66;
  --color-trust-deep: #111827;
}
```

2. **Dark counterpart** (defined by the docs site, not yet shipped in the SPA; see ADR 048):

```css
/* page #0b0d14, surface #11141c, raised #1a1e28, ink #eef0f5, accent #4fd1b5 */
```

### Respecting User Preferences

```typescript
// Dark mode support (automatic via Tailwind)
<div className="dark:bg-neutral-900 dark:text-white">
  {children}
</div>

// Reduced motion support
<div className="motion-safe:animate-in motion-reduce:animate-none">
  {children}
</div>

// High contrast support (automatic via semantic colors)
```

## Accessibility with Design Tokens

1. **Always verify color contrast** when using custom colors (WCAG AA: 4.5:1 for text, 3:1 for UI components)
2. **Use semantic tokens** (trust, success, warning, error) which are pre-verified for WCAG 2.1 AA
3. **Use the cool neutrals** (neutral-\*) for text hierarchy - pre-verified contrast ratios
4. **Avoid color-only encoding** - use icons, text, or patterns for status communication
5. **Test with ColorSnack** or WebAIM Contrast Checker for custom combinations
6. **Respect prefers-reduced-motion** in animations and transitions

## Token Maintenance

Design tokens are maintained in:

- `web/src/design-system/tokens/colors.css` - the `@theme` tokens (colors, fonts, shadow, easing)
- `web/src/design-system/tokens/*.ts` - TypeScript mirrors for programmatic access
- Component CVA files - Component-specific variants
- Storybook docs - Visual reference
- This guide - Documentation

When proposing new tokens:

1. Identify the design need
2. Check if existing token works
3. Verify accessibility compliance
4. Document in this guide
5. Update `colors.css` and its TypeScript mirror
6. Test across components

## Summary

Design tokens provide:

- **Consistency** across all applications
- **Accessibility** through verified color and sizing choices
- **Flexibility** to customize for brand or context
- **Maintainability** through centralized management
- **Performance** via efficient CSS generation

By using design tokens consistently, we ensure a cohesive, accessible, and maintainable user experience.
