# Common Mistakes & How to Fix Them

Learn from frequent pitfalls in the Refined Trust Architecture design system. Each mistake includes the reason it's wrong and the correct solution.

---

## Color Usage Mistakes

### ❌ Mistake 1: Using `text-primary` for Headings

```tsx
// DON'T
<h1 className="text-primary">Dashboard</h1>
```

**Why it's wrong**: `text-primary` is an alias that points to `trust-deep`, but it's confusing because "primary" could mean "primary text color" (for body text). The semantic intent is unclear.

**Correct solution**:

```tsx
// DO
<h1 className="text-trust-deep">Dashboard</h1>
```

**Rule**: Always use `text-trust-deep` or `text-trust` explicitly for headings to communicate brand authority.

---

### ❌ Mistake 2: Using `text-neutral-900` for Headings

```tsx
// DON'T
<h2 className="text-neutral-900">Section Title</h2>
```

**Why it's wrong**: Headings should use brand colors (trust family) to establish authority and visual hierarchy. Neutral colors are for body text and supporting content.

**Correct solution**:

```tsx
// DO
<h2 className="text-trust-deep">Section Title</h2>  // Major sections
<h3 className="text-trust">Subsection Title</h3>    // Minor sections
```

**Rule**: Reserve `neutral-900` only for body text when `neutral-700` doesn't provide enough contrast.

---

### ❌ Mistake 3: Using `gray-*` Classes

```tsx
// DON'T
<div className="bg-gray-100 text-gray-700 border-gray-300">Content</div>
```

**Why it's wrong**: Standard Tailwind grays have been replaced with warm neutrals throughout the design system. Gray classes don't exist in this system.

**Correct solution**:

```tsx
// DO
<div className="bg-neutral-100 text-neutral-700 border-neutral-300">
  Content
</div>
```

**Rule**: Always use `neutral-*` scale instead of `gray-*`. Our warm neutrals (#fbfcfd, #f0f2f6) convey sophistication and trust.

---

### ❌ Mistake 4: Using Extended Color Palettes

```tsx
// DON'T
<button className="bg-navy-700 text-white">Click Me</button>
```

**Why it's wrong**: Extended palettes (navy-50 through navy-900, emerald-_, amber-_) were removed. Only semantic tokens are available.

**Correct solution**:

```tsx
// DO
<button className="bg-trust-deep text-white">Click Me</button>
```

**Rule**: Use semantic tokens: `trust-deep`, `trust`, `success-primary`, `error-primary`, `warning-primary`, `neutral-*`.

---

### ❌ Mistake 5: Inconsistent Text Color Usage

```tsx
// DON'T - mixing direct neutral colors with semantic aliases
<div>
  <p className="text-neutral-600">First paragraph</p>
  <p className="text-secondary">Second paragraph</p>
</div>
```

**Why it's wrong**: Inconsistency makes code harder to maintain. Pick one pattern and stick with it.

**Correct solution**:

```tsx
// DO - use semantic aliases consistently
<div>
  <p className="text-secondary">First paragraph</p>
  <p className="text-secondary">Second paragraph</p>
</div>
```

**Rule**: Prefer semantic aliases (`text-secondary`, `text-tertiary`) over direct neutral colors for consistency.

---

## Layout & Spacing Mistakes

### ❌ Mistake 6: Inconsistent Padding Props

```tsx
// DON'T - mixing 'md' and 'default'
<Card padding="default">
  <Button size="md">Action</Button>
</Card>

<Card padding="md">
  <Button size="default">Action</Button>
</Card>
```

**Why it's wrong**: Documentation uses "default" but common practice uses "md". This creates confusion about which is canonical.

**Correct solution**:

```tsx
// DO - stick with one convention
<Card padding="default">
  <Button size="md">Action</Button>
</Card>
```

**Rule**: Use `size="sm|md|lg"` for buttons and components, but `padding="compact|default|spacious"` for cards. Different scales serve different purposes.

---

### ❌ Mistake 7: Tight Spacing in Cards

```tsx
// DON'T
<Card className="p-2">
  <h3>Title</h3>
  <p>Content with cramped padding</p>
</Card>
```

**Why it's wrong**: Cards need generous padding to create the "luxury" feel. Cramped spacing looks cheap and rushed.

**Correct solution**:

```tsx
// DO
<Card padding="default" className="space-y-4">
  <h3>Title</h3>
  <p>Content with proper breathing room</p>
</Card>
```

**Rule**: Minimum card padding is 16px (`compact`). Default is 24px (`default`). Never go below 16px.

---

### ❌ Mistake 8: No Gap Between Elements

```tsx
// DON'T
<div>
  <h2>Title</h2>
  <p>Paragraph right after title</p>
  <Button>Action</Button>
</div>
```

**Why it's wrong**: Elements crush together visually without vertical spacing, making the interface feel dense and hard to scan.

**Correct solution**:

```tsx
// DO
<div className="space-y-4">
  <h2>Title</h2>
  <p>Paragraph with proper spacing</p>
  <Button>Action</Button>
</div>
```

**Rule**: Always use `space-y-*` utilities for vertical stacking. Default to `space-y-4` (16px) for related content.

---

## Animation Mistakes

### ❌ Mistake 9: Animating Width/Height

```tsx
// DON'T
<div className="transition-all duration-300 hover:w-64">Expanding div</div>
```

**Why it's wrong**: Animating layout properties (width, height, padding, margin) causes expensive reflows and janky animations.

**Correct solution**:

```tsx
// DO
<div className="transition-transform duration-300 hover:scale-105">
  Scaling div
</div>
```

**Rule**: Only animate transform properties (translate, scale, rotate) and opacity. Use `max-height` with overflow for height transitions if needed.

---

### ❌ Mistake 10: Wrong Animation Duration

```tsx
// DON'T
<Card className="transition-all duration-150 hover:shadow-lg">
  Card content
</Card>
```

**Why it's wrong**: Card elevation changes need **300ms**, not 150ms. The duration hierarchy exists for a reason.

**Correct solution**:

```tsx
// DO
<Card className="transition-all duration-300 hover:shadow-lg">
  Card content
</Card>
```

**Rule**:

- 150ms → Color/opacity changes
- 200ms → Button states
- 300ms → Card elevation
- 500ms → Page transitions

---

### ❌ Mistake 11: Not Respecting `prefers-reduced-motion`

```tsx
// DON'T
<div className="animate-bounce">Always bouncing</div>
```

**Why it's wrong**: Users with vestibular disorders can experience nausea from animations. Accessibility is mandatory.

**Correct solution**:

```tsx
// DO
<div className="motion-safe:animate-bounce">
  Bounces only if user allows motion
</div>
```

**Rule**: Always prefix animations with `motion-safe:` to respect user preferences.

---

## Component Usage Mistakes

### ❌ Mistake 12: Multiple Primary Buttons

```tsx
// DON'T
<div className="flex gap-3">
  <Button variant="primary">Save</Button>
  <Button variant="primary">Publish</Button>
  <Button variant="primary">Share</Button>
</div>
```

**Why it's wrong**: Multiple primary buttons create competing visual hierarchy. Users don't know which action is most important.

**Correct solution**:

```tsx
// DO
<div className="flex gap-3">
  <Button variant="primary">Publish</Button>
  <Button variant="secondary">Save Draft</Button>
  <Button variant="outline">Preview</Button>
</div>
```

**Rule**: One primary button per section. Use secondary/outline for supporting actions.

---

### ❌ Mistake 13: Danger Button with Primary Button

```tsx
// DON'T
<div className="flex gap-3">
  <Button variant="primary">Save</Button>
  <Button variant="danger">Delete</Button>
</div>
```

**Why it's wrong**: Combining primary and danger creates visual confusion. Both demand attention equally, making accidents likely.

**Correct solution**:

```tsx
// DO - separate dangerous actions
<div className="space-y-6">
  <div>
    <Button variant="primary" className="w-full">
      Save Changes
    </Button>
  </div>

  <div className="pt-6 border-t border-neutral-200">
    <p className="text-sm text-error-primary mb-2">Danger zone</p>
    <Button variant="danger">Delete Forever</Button>
  </div>
</div>
```

**Rule**: Separate destructive actions visually. Use dividers, spacing, or different sections.

---

### ❌ Mistake 14: Wrong Modal Size

```tsx
// DON'T - using xl for a simple confirmation
<Modal size="xl">
  <p>Are you sure you want to delete this?</p>
  <Button variant="danger">Delete</Button>
</Modal>
```

**Why it's wrong**: Modal size should match content complexity. Large modals for simple tasks waste screen space.

**Correct solution**:

```tsx
// DO
<Modal size="sm">
  <p>Are you sure you want to delete this?</p>
  <Button variant="danger">Delete</Button>
</Modal>
```

**Rule**:

- `sm` (400px) → Confirmations
- `md` (600px) → Forms with 3-5 fields
- `lg` (800px) → Complex forms/content
- `xl` (1000px) → Data tables

---

### ❌ Mistake 15: No Shadow on Cards

```tsx
// DON'T
<Card className="border border-neutral-200">Card without elevation</Card>
```

**Why it's wrong**: The design system uses **shadow-based elevation**, not borders. Cards need shadows to "float" above the page.

**Correct solution**:

```tsx
// DO
<Card className="shadow-sm hover:shadow-lg">Card with proper elevation</Card>
```

**Rule**: Cards use `shadow-sm` at rest, `shadow-lg` on hover. Borders are optional and subtle.

---

## Typography Mistakes

### ❌ Mistake 16: Using System Fonts

```tsx
// DON'T
<h1 className="font-sans">Title</h1>
```

**Why it's wrong**: `font-display` and `font-sans` both resolve to Geist today, but headings go through `font-display` so a later change to the display face lands in one token.

**Correct solution**:

```tsx
// DO
<h1 className="font-display">Title</h1>
```

**Rule**:

- Headings → `font-display` (Geist)
- Body → `font-sans` (Geist)
- Code → `font-mono` (Geist Mono)

---

### ❌ Mistake 17: Inconsistent Font Weights

```tsx
// DON'T
<h2 className="font-bold">Section</h2>
```

**Why it's wrong**: The direction uses weights 400 and 500 only. Hierarchy comes from size, color and spacing. Bold Geist shouts.

**Correct solution**:

```tsx
// DO
<h2 className="text-xl font-medium text-trust-deep">Section</h2>
<p className="text-sm text-neutral-600">Supporting text</p>
```

**Rule**: Medium (500) for headings, labels and names; regular (400) for everything else. Never `font-semibold` or `font-bold`.

---

### ❌ Mistake 18: Missing Letter Spacing

```tsx
// DON'T
<h1 className="text-4xl font-display">Cramped Display Heading</h1>
```

**Why it's wrong**: Geist headings tighten as they grow (-0.015em, -0.025em on h1). The base `h1`-`h6` rule applies this; a heading rendered with a `p` or `span` needs it explicitly.

**Correct solution**:

```tsx
// DO
<h1 className="text-4xl font-display tracking-tight">
  Properly Spaced Display Heading
</h1>
```

**Rule**: Use `tracking-tight` on display-sized text. Body text has default spacing.

---

## Accessibility Mistakes

### ❌ Mistake 19: Color-Only Status Indication

```tsx
// DON'T
<span className="text-success-primary">Active</span>
<span className="text-error-primary">Denied</span>
```

**Why it's wrong**: Colorblind users can't distinguish status by color alone. You need additional indicators.

**Correct solution**:

```tsx
// DO
<Badge variant="success">
  <CheckIcon className="w-3 h-3 mr-1" />
  Active
</Badge>
<Badge variant="error">
  <XIcon className="w-3 h-3 mr-1" />
  Denied
</Badge>
```

**Rule**: Always combine color with icons, text, or patterns for status communication.

---

### ❌ Mistake 20: Missing Focus States

```tsx
// DON'T
<button className="bg-trust text-white">No focus indicator</button>
```

**Why it's wrong**: Keyboard users can't see which element has focus. Violates WCAG AA.

**Correct solution**:

```tsx
// DO
<button className="bg-trust text-white focus:ring-2 focus:ring-trust focus:ring-offset-2">
  Proper focus indicator
</button>
```

**Rule**: All interactive elements need visible focus rings (2px, trust color, 2px offset).

---

### ❌ Mistake 21: Insufficient Color Contrast

```tsx
// DON'T
<p className="text-neutral-400 bg-white">Low contrast text</p>
```

**Why it's wrong**: `neutral-400` (#a9afbd) on white doesn't meet WCAG AA (4.5:1 minimum for text).

**Correct solution**:

```tsx
// DO
<p className="text-neutral-700 bg-white">High contrast text</p>
```

**Rule**: Use `neutral-700` (#364152) or darker for body text. `neutral-600` (#4b5565) for secondary text. `neutral-400` only for placeholders/disabled states.

---

## Performance Mistakes

### ❌ Mistake 22: Inline Styles for Theming

```tsx
// DON'T
<div style={{ backgroundColor: '#111827', color: '#ffffff' }}>
  Inline styled
</div>
```

**Why it's wrong**: Inline styles bypass the design token system and make theming impossible. Can't be purged by Tailwind.

**Correct solution**:

```tsx
// DO
<div className="bg-trust-deep text-white">Token-based styling</div>
```

**Rule**: Always use utility classes. Use CSS variables (`var(--color-trust-deep)`) only when dynamic theming is required.

---

### ❌ Mistake 23: Not Using Semantic HTML

```tsx
// DON'T
<div className="text-lg font-bold text-trust-deep" onClick={handleClick}>
  Clickable Heading
</div>
```

**Why it's wrong**: Screen readers can't identify headings. Buttons aren't keyboard-accessible. Hurts SEO and accessibility.

**Correct solution**:

```tsx
// DO
<h2 className="text-lg font-bold text-trust-deep">
  Actual Heading
</h2>
<button onClick={handleClick} className="text-trust hover:text-trust-hover">
  Clickable Element
</button>
```

**Rule**: Use semantic HTML (`<button>`, `<h1>`, `<nav>`) instead of styled `<div>` elements.

---

## Quick Reference: Most Common Mistakes

| Mistake                           | Fix                                        |
| --------------------------------- | ------------------------------------------ |
| Using `gray-*`                    | Use `neutral-*` instead                    |
| Using `navy-700`                  | Use `trust-deep` or `trust`                |
| Using `text-primary` for headings | Use `text-trust-deep` explicitly           |
| Multiple primary buttons          | One primary per section                    |
| No spacing between elements       | Use `space-y-4` for stacking               |
| Animating width/height            | Animate `transform` instead                |
| Wrong font utility on headings    | Use `font-display` for headings            |
| Missing focus rings               | Add `focus:ring-2` to interactive elements |
| Low contrast text                 | Use `text-neutral-700` minimum             |
| Color-only status                 | Add icons to colors                        |

---

## How to Avoid These Mistakes

1. **Read decision trees first** - Consult [DECISION_TREES.md](./DECISION_TREES.md) before making choices
2. **Use component pairing guide** - Reference [COMPONENT_PAIRING_GUIDE.md](./COMPONENT_PAIRING_GUIDE.md) for spacing patterns
3. **Check accessibility guide** - Verify compliance with [ACCESSIBILITY_GUIDE.md](./ACCESSIBILITY_GUIDE.md)
4. **Review component archetypes** - Study [COMPONENT_ARCHETYPES.md](./COMPONENT_ARCHETYPES.md) for visual specifications
5. **Test in Storybook** - View components in isolation before integrating

**Remember**: When in doubt, look at existing components in the design system. They follow these rules correctly and serve as working examples.
