# Design Token Guide

Design tokens are the visual design decisions encoded as data. This guide covers the tokens available in the Refined Trust Architecture design system and how to use them.

## Authority and example status

[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current direction and Principle XI process.
ADR 037 remains Proposed. The OKLCH replacement is not the current token API.
This guide retains an inventory and older examples, not evidence of theme or accessibility compliance.
Raw palette utilities and literal visual values in examples are not requirements for new code.
Use centralized semantic tokens, self-hosted assets, and light/dark accessibility checks for every story.
Align this guide with the implemented target in the single cutover after ADR acceptance.

## Overview

Design tokens in this system are managed through:

- **Tailwind CSS v4**: CSS variables via `@theme` directive
- **CSS Custom Properties**: For runtime customization
- **TypeScript Type System**: For compile-time safety
- **Class Variance Authority (CVA)**: For component variants

## Color System

The source contains semantic roles and the existing palette. Verify rendered contrast in both themes. Token names do not prove WCAG compliance.

### Why Semantic Tokens?

Semantic tokens provide meaning-driven color naming that improves code readability and maintainability.

**Use existing semantic role tokens:**

```tsx
<div className="bg-bg-primary text-text-primary border-border-primary">
  Content
</div>
```

**❌ DON'T: Use extended palette classes**

### Existing color inventory — not a component palette API

```typescript
// PRIMARY - Trust & Authority (Navy)
trust-deep:  #0A2540     // Darkest - headings, primary brand
trust:       #1E4D6B     // Medium - primary actions, links
trust-hover: #2b68a5     // Lighter - hover states
trust-light: #E8F1F5     // Lightest - backgrounds, tints

// ACTION - CTA (Unified with Warning)
cta:       #D97706       // Call-to-action and warnings
cta-hover: #B45309       // CTA hover state
cta-light: #fef3c7       // CTA light background

// SUCCESS - Emerald tones for positive actions
success-primary: #059669 // Success state, granted permissions
success-hover:   #047857 // Success hover state
success-light:   #d1fae5 // Success light background
success-dark:    #065f46 // Success dark text

// ERROR - Red tones for destructive actions
error-primary: #DC2626   // Error state, destructive actions
error-hover:   #b91c1c   // Error hover state
error-light:   #fee2e2   // Error light background
error-dark:    #991b1b   // Error dark text

// WARNING - Unified with CTA (Amber)
warning-primary: #D97706 // Warning state (same as CTA)
warning-hover:   #B45309 // Warning hover state
warning-light:   #fef3c7 // Warning light background
warning-dark:    #92400e // Warning dark text

// INFO - Blue tones for informational content
info-primary: #3B82F6   // Info state
info-hover:   #2563eb   // Info hover state
info-light:   #dbeafe   // Info light background
info-dark:    #1e40af   // Info dark text

// WARM NEUTRALS - Sophisticated cream/sand/taupe (replaces gray)
neutral-50:  #faf9f7    // Cream - page backgrounds
neutral-100: #f5f1ed    // Sand - section backgrounds
neutral-200: #e8e3de    // Taupe - card borders
neutral-300: #ddd8d1    // Light taupe - input borders
neutral-400: #c4bdb3    // Medium taupe - placeholders
neutral-500: #9a9591    // Medium-dark - secondary icons
neutral-600: #6b6561    // Dark taupe - secondary text
neutral-700: #4a4137    // Darker taupe - body text
neutral-800: #2a251f    // Very dark - emphasized text
neutral-900: #1a1511    // Darkest - headings (or use trust-deep)

// SEMANTIC ALIASES - Contextual color names
text-primary:   #0A2540  // Primary text (trust-deep)
text-secondary: #6b6561  // Secondary text (neutral-600)
text-tertiary:  #9a9591  // Tertiary text (neutral-500)
text-disabled:  #c4bdb3  // Disabled text (neutral-400)

bg-primary:   #faf9f7    // Primary background (neutral-50)
bg-secondary: #f5f1ed    // Secondary background (neutral-100)
bg-elevated:  #ffffff    // Elevated cards (pure white)

border-primary:   #ddd8d1 // Primary borders (neutral-300)
border-secondary: #e8e3de // Secondary borders (neutral-200)
border-focus:     #1E4D6B // Focus ring (trust)
```

### Existing color usage examples — not new-code requirements

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

### Semantic token naming

Tailwind adds its utility prefix to the full token name.
For example, `--color-text-primary` generates `text-text-primary`, not `text-primary`.
`--color-bg-primary` generates `bg-bg-primary`.
See [COLOR_GUIDE.md](COLOR_GUIDE.md) for the current role inventory.

Use semantic roles according to purpose. Do not select numbered palette shades in components.
The older examples in this guide retain historical names and visual values.
They do not authorize raw palette use or establish missing APIs.
If a role is missing, define it in the token source before component use.

### Color Accessibility

Verify rendered semantic color pairs against these requirements:

- Text contrast: Minimum 4.5:1 (for body text)
- UI component contrast: Minimum 3:1 (for graphics)
- Use ColorSnack or WebAIM for verification

**When choosing colors:**

1. Prefer semantic tokens (trust, success, warning, error)
2. Use semantic roles instead of raw warm-neutral or standard palette utilities
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
// Display/Headings (Brand Primary)
'font-display' → Crimson Pro, serif
  - Weight 700 (bold) for main headings
  - Weight 600 (semibold) for subheadings
  - Letter-spacing: -0.02em for authority
  - Usage: h1, h2, h3, page titles, modal headers

// Body/UI (Humanist Sans-Serif)
'font-sans' → Manrope, sans-serif
  - Weight 400 (regular) for body text
  - Weight 500 (medium) for emphasized text
  - Letter-spacing: -0.01em for readability
  - Usage: Body text, buttons, form labels, descriptions
  - Base size: 1rem (16px)
  - Line height: 1.5 for comfortable reading

// Monospace (Technical Values)
'font-mono' → JetBrains Mono, monospace
  - Weight 500 (medium) for technical content
  - Usage: OAuth scopes, agent IDs, API tokens, code blocks
  - Typically displayed with subtle background highlight
```

**Rationale:**

- **Crimson Pro** conveys trust, authority, and seriousness—essential for consent UI
- **Manrope** provides excellent readability and humanist approachability
- **JetBrains Mono** offers clarity for technical values while maintaining visual consistency

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

```typescript
// Tailwind shadow scale
'shadow-sm'        // Subtle (cards, small elements)
'shadow'           // Default (most interactive elements)
'shadow-md'        // Medium (modals, dropdowns)
'shadow-lg'        // Large (overlays, prominent elements)
'shadow-lg-premium' // Custom premium shadow (design system specific)

// Shadow usage
<Card className="shadow" />           // Default card shadow
<Modal className="shadow-lg" />       // Prominent modal
<button className="hover:shadow-md" /> // Hover elevation
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

### Via Tailwind Classes

```tsx
// Most common approach
<div className="bg-bg-primary text-text-primary p-4 rounded-lg shadow">
  <h2 className="text-2xl font-bold text-text-primary">Title</h2>
  <p className="mt-2 text-sm text-text-secondary">Description</p>
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

### Older component-prop example — verify APIs before reuse

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

### Centralized token changes

Define semantic roles under `web/src/design-system/tokens/` and expose CSS roles through Tailwind v4 `@theme`.
Do not define a second palette in application components or `tailwind.config.ts`.
Component CVA variants select tokens. They do not own another source of visual values.

If a token changes the visual direction, obtain ADR acceptance before implementation.
Document its role, supported themes, and accessibility results.
The proposed feature 046 contract remains separate in [COLOR_GUIDE.md](COLOR_GUIDE.md).

### User preferences

Light and dark themes require implementation and verification. Tailwind utilities do not create a complete accessible theme automatically.
Do not use raw `dark:neutral-*` palette utilities as a substitute for semantic theme tokens.
Respect reduced motion and verify forced-color behavior.

## Accessibility with Design Tokens

1. **Always verify color contrast** when using custom colors (WCAG AA: 4.5:1 for text, 3:1 for UI components)
2. **Use semantic tokens** and verify actual rendered combinations in both themes
3. **Do not use raw palette utilities**, including numbered warm neutrals
4. **Avoid color-only encoding** - use icons, text, or patterns for status communication
5. **Test with ColorSnack** or WebAIM Contrast Checker for custom combinations
6. **Respect prefers-reduced-motion** in animations and transitions

## Token Maintenance

Design tokens are maintained in:

- `web/src/design-system/tokens/` - Central definitions for visual decisions
- Component CVA files - Variants that select those tokens
- Storybook stories and this guide - Examples and verification guidance, not duplicate token definitions

When proposing new tokens:

1. Identify the design need
2. Check if existing token works
3. Verify accessibility compliance
4. Document in this guide
5. Update the central token definitions and their Tailwind v4 mapping
6. Test across components

## Summary

Design tokens provide:

- **Consistency** across all applications
- **Accessibility** through verified color and sizing choices
- **Flexibility** to customize for brand or context
- **Maintainability** through centralized management
- **Performance** via efficient CSS generation

By using design tokens consistently, we ensure a cohesive, accessible, and maintainable user experience.
