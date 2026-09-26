# Design System Documentation Index

This index routes to design-system guidance. Consent UI v2 is a proposed replacement under feature 046. Runtime migration has not started.

---

## 📚 Documentation Files

### Core Design Documentation

#### [DESIGN_PRINCIPLES.md](./DESIGN_PRINCIPLES.md)

**Location**: `/web/src/design-system/docs/DESIGN_PRINCIPLES.md`
The [v2 design principles](DESIGN_PRINCIPLES.md) define focused decision views, a compact console, local typography and brand assets, restrained motion, and accessibility gates.

Use this guide for new v2 work. [ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) requires acceptance before implementation.

The remaining detailed guides describe legacy components until phase 3. They do not override the rewritten design principles or color contract.

---

#### [COLOR_GUIDE.md](./COLOR_GUIDE.md)

**Location**: `/web/src/design-system/docs/COLOR_GUIDE.md`
The [v2 color guide](COLOR_GUIDE.md) defines semantic OKLCH tokens, light/dark precedence, contrast requirements, and raw-palette lint enforcement.

It replaces the legacy trust, CTA, and warm-neutral palette instructions for v2. Runtime token changes belong to foundation implementation.

---

#### [TOKEN_GUIDE.md](./TOKEN_GUIDE.md)

**Location**: `/web/src/design-system/docs/TOKEN_GUIDE.md`
**Coverage**: 380+ lines

Design tokens system documentation for colors, typography, spacing, shadows, and animations.

**Sections**:

- **Color Tokens** (@theme CSS directives with semantic names)
- **Typography System**
  - ✅ **Font Families** (CORRECTED - now specifies exact fonts):
    - **Crimson Pro** (display/headings): weight 700/600, letter-spacing -0.02em
    - **Manrope** (body/UI): weight 400/500, letter-spacing -0.01em, base 1rem
    - **JetBrains Mono** (monospace): weight 500 for technical values
  - Font Sizes & Line Heights (12px to 48px scale)
  - Font Weights (400 to 700)
- **Spacing System** (4px base unit, 0 to 96px scale)
- **Shadow System** (subtle to premium elevation)
- **Border Radius** (0px to 9999px circular scale)
- **Transitions & Animations** (150ms-500ms with easing functions)
- **Z-Index Scale** (0 to 50 for layering strategy)
- **Using Design Tokens** (Tailwind classes, CSS variables, component props)
- **Token Customization** (application-specific theming)
- **Accessibility with Design Tokens** (contrast verification, responsive design)

**Typography Corrections**:

- ✅ Crimson Pro specified for display/headings
- ✅ Manrope specified for body/UI
- ✅ JetBrains Mono specified for monospace code
- ✅ Letter-spacing values included (-0.02em for headings, -0.01em for body)

---

#### [COMPONENT_ARCHETYPES.md](./COMPONENT_ARCHETYPES.md)

**Location**: `/web/src/design-system/docs/COMPONENT_ARCHETYPES.md`
**Coverage**: 500+ lines

Detailed visual specifications for the four foundational components that define the design system.

**Covered Components**:

1. **Primary Button**
   - Background: #0d1829 with gradient overlay
   - Height: 44px, Padding: 12px 16px
   - Shadow: 0 2px 8px (default), 0 8px 20px (hover)
   - Transform: translateY(-1px) on hover
   - Transition: 200ms cubic-bezier(0.34, 1.56, 0.64, 1)
   - States: Default, Hover, Focus, Active, Disabled, Loading
   - Variants: Primary, Secondary, Outline, Ghost, Danger

2. **Card Component**
   - Background: White (#ffffff) with 10px blur backdrop-filter
   - Border: 1px solid rgba(240, 237, 232, 0.8)
   - Padding: 24px (default), 32px (headers)
   - Border Radius: 12px (xl)
   - Shadow: 0 2px 8px + inset highlight (default), 0 8px 20px (hover)
   - Transition: 300ms cubic-bezier(0.34, 1.56, 0.64, 1)
   - Hover transform: translateY(-2px)

3. **Form Input**
   - Height: 44px, Padding: 12px 16px
   - Border: 1.5px solid #ddd8d1 (default), #1e4d6b (focus), #dc2626 (error)
   - Border Radius: 6px (md)
   - Focus Ring: 2px solid #1e4d6b at 2px offset
   - Label: Manrope Medium (500), 14px
   - Error message: 12px, red-600, with icon

4. **Modal**
   - Overlay: rgba(13, 24, 41, 0.5) with 8px backdrop-blur
   - Modal body: White, 16px border-radius
   - Shadow: 0 20px 40px rgba(0,0,0,0.12)
   - Overlay animation: Fade in 200ms
   - Modal animation: Slide up + scale 300ms with 100ms delay
   - Close button: Ghost style, 44×44px minimum

---

#### [MOTION_GUIDE.md](./MOTION_GUIDE.md)

**Location**: `/web/src/design-system/docs/MOTION_GUIDE.md`
**Coverage**: 450+ lines

Animation and motion specifications for all interactions in the system.

**Sections**:

- **Animation Principles** - 5 core principles (purposeful, restrained, responsive, accessible, consistent)
- **Timing Hierarchy**
  - Fast (150ms): Hover colors, focus rings, icons
  - Base (200ms): Button presses, dropdowns, toggles
  - Slow (300ms): Card elevations, modals, accordion
  - Slower (500ms): Page transitions, full-page loading
- **Easing Functions**
  - Primary: cubic-bezier(0.34, 1.56, 0.64, 1) (spring with slight bounce)
  - Secondary: cubic-bezier(0.4, 0, 0.2, 1) (smooth curve)
- **Common Animation Patterns** (with code examples)
  - Button interaction (200ms spring)
  - Card elevation (300ms spring)
  - Modal entrance (staggered: 200ms + 300ms)
  - Dropdown open (200ms spring)
  - Skeleton loading (pulse shimmer)
  - Accordion expand (300ms spring)
  - Toast notification (slide-in/out)
- **Accessibility: prefers-reduced-motion** - Respecting user preferences
- **Animation Performance Tips** - GPU acceleration, CSS vs JavaScript
- **Animation Component Integration** - Framer Motion and Tailwind examples
- **Animation Timing Reference Chart** - Quick lookup table

---

### Additional Documentation

#### [ACCESSIBILITY_GUIDE.md](./ACCESSIBILITY_GUIDE.md)

**Location**: `/web/src/design-system/docs/ACCESSIBILITY_GUIDE.md`
**Coverage**: 490+ lines

WCAG 2.1 AA compliance guide for all components.

**Key Coverage**:

- Semantic HTML usage
- ARIA attributes for screen readers
- Keyboard navigation requirements
- Color contrast ratios (4.5:1 for text, 3:1 for graphics)
- Focus indicators on all interactive elements
- Component-specific accessibility patterns
- Testing methodologies (keyboard, screen reader, contrast)
- Accessibility checklist
- Common issues and fixes

---

#### [COMPOSITION_PATTERNS.md](./COMPOSITION_PATTERNS.md)

**Location**: `/web/src/design-system/docs/COMPOSITION_PATTERNS.md`
**Coverage**: 640+ lines

Real-world composition patterns for building with design system components.

**Pattern Categories**:

- Layout patterns (hero sections, two-column, grids, sidebars)
- Form patterns (simple, multi-step, validation)
- Data display patterns (tables, card grids, lists with badges)
- Navigation patterns (breadcrumbs, tabs, pagination)
- Feedback patterns (loading, empty state, error handling, toasts)
- Permission patterns (scope lists, grant display)
- Modal patterns (confirmations, dropdowns, popovers)
- Accordion patterns (FAQ, settings)
- Progress patterns (wizards, task progress)

---

#### [DECISION_TREES.md](./DECISION_TREES.md)

**Location**: `/web/src/design-system/docs/DECISION_TREES.md`
**Coverage**: 850+ lines

Flowchart-based decision guidance for eliminating ambiguity when building components.

**Decision Trees Covered**:

- Button Variant Selection (primary vs secondary vs outline vs ghost vs danger)
- Text Color Hierarchy (trust-deep vs trust vs neutral-700)
- Animation Duration Selection (150ms → 200ms → 300ms → 500ms)
- Spacing & Padding Selection (context-specific rules)
- Shadow Selection (sm → md → lg → xl hierarchy)
- Border Radius Selection (4px → 6px → 12px → 16px)
- Modal Size Selection (sm → md → lg → xl → full)
- Status Color Selection (success vs warning vs error vs info)
- Icon Sizing (context-based sizing rules)

**Use Case**: When you need to decide which variant to use or how to style a component, follow the decision flowcharts to make confident, consistent choices.

---

#### [COMPONENT_PAIRING_GUIDE.md](./COMPONENT_PAIRING_GUIDE.md)

**Location**: `/web/src/design-system/docs/COMPONENT_PAIRING_GUIDE.md`
**Coverage**: 650+ lines

Real-world examples of how components work together with exact spacing and hierarchy.

**Pairing Patterns Covered**:

- Button + Card combinations (primary actions, multiple actions, empty states)
- Typography hierarchy in cards (h1 → h2 → h3 → body → metadata)
- Form error states (icon + message patterns)
- Button groups (segmented controls, action groups)
- Empty states with CTAs (complete pattern with spacing breakdown)
- Status indicators with context (badges + timestamps)
- Modal content patterns (confirmation and form modals)
- Spacing cheat sheet (Tailwind classes → pixel values)
- Icon sizing cheat sheet (context-based sizes)
- Typography hierarchy cheat sheet (font sizes and weights)

**Use Case**: When composing multiple components together, reference these patterns for exact spacing, color, and hierarchy specifications.

---

#### [COMMON_MISTAKES.md](./COMMON_MISTAKES.md)

**Location**: `/web/src/design-system/docs/COMMON_MISTAKES.md`
**Coverage**: 650+ lines

Learn from frequent pitfalls in the design system. Each mistake includes the reason it's wrong and the correct solution.

**Mistake Categories** (23 total):

- **Color Usage** (5 mistakes): text-primary confusion, using gray-_, using navy-_, extended palettes, inconsistent usage
- **Layout & Spacing** (3 mistakes): inconsistent padding props, tight card spacing, no gaps between elements
- **Animation** (3 mistakes): animating width/height, wrong duration, not respecting prefers-reduced-motion
- **Component Usage** (4 mistakes): multiple primary buttons, danger + primary together, wrong modal size, no shadow on cards
- **Typography** (3 mistakes): using system fonts, inconsistent weights, missing letter spacing
- **Accessibility** (3 mistakes): color-only status, missing focus states, insufficient contrast
- **Performance** (2 mistakes): inline styles for theming, not using semantic HTML

**Use Case**: Before implementing a component, review common mistakes to avoid anti-patterns. Use the quick reference table for fast lookups.

---

## 📍 File Locations

All documentation is located in:

```
/web/src/design-system/docs/
├── INDEX.md                          (this file)
├── DESIGN_PRINCIPLES.md              ✅ Visual design direction + archetypes
├── COLOR_GUIDE.md                    ✅ Complete color palette with hex values
├── TOKEN_GUIDE.md                    ✅ Design tokens (typography corrected)
├── COMPONENT_ARCHETYPES.md           ✅ Detailed component specifications
├── MOTION_GUIDE.md                   ✅ Animation timings and easing
├── ACCESSIBILITY_GUIDE.md            ✅ WCAG 2.1 AA compliance
├── COMPOSITION_PATTERNS.md           ✅ Real-world usage patterns
├── DECISION_TREES.md                 ✅ Flowchart-based decision guidance
├── COMPONENT_PAIRING_GUIDE.md        ✅ Component composition with spacing
├── COMMON_MISTAKES.md                ✅ Anti-patterns and correct solutions
└── MIGRATION_GUIDE.md                ✅ Migration strategy
```

---

## 📖 How to Use This Documentation

### For Designers

1. Start with [DESIGN_PRINCIPLES.md](./DESIGN_PRINCIPLES.md) for overall aesthetic
2. Reference [COLOR_GUIDE.md](./COLOR_GUIDE.md) for color decisions
3. Check [COMPONENT_ARCHETYPES.md](./COMPONENT_ARCHETYPES.md) for specific component styling
4. Use [MOTION_GUIDE.md](./MOTION_GUIDE.md) for animation decisions

### For Developers

1. Read [TOKEN_GUIDE.md](./TOKEN_GUIDE.md) for implementation tokens
2. Review [COMPOSITION_PATTERNS.md](./COMPOSITION_PATTERNS.md) for common patterns
3. Verify [ACCESSIBILITY_GUIDE.md](./ACCESSIBILITY_GUIDE.md) for compliance
4. Check [MIGRATION_GUIDE.md](./MIGRATION_GUIDE.md) when refactoring existing components

### For AI Agents

1. Start with [DECISION_TREES.md](./DECISION_TREES.md) for all variant and styling decisions
2. Reference [COMPONENT_PAIRING_GUIDE.md](./COMPONENT_PAIRING_GUIDE.md) for exact spacing and composition patterns
3. Review [COMMON_MISTAKES.md](./COMMON_MISTAKES.md) to avoid anti-patterns
4. Use [COMPONENT_ARCHETYPES.md](./COMPONENT_ARCHETYPES.md) for detailed component specifications
5. Check [TOKEN_GUIDE.md](./TOKEN_GUIDE.md) for token names and values
6. Verify [ACCESSIBILITY_GUIDE.md](./ACCESSIBILITY_GUIDE.md) before finalizing components

---

## 🔍 Quick Reference

### Color Palette (Semantic Tokens)

- **Primary (Trust)**: trust-deep (#0A2540), trust (#1E4D6B), trust-light (#E8F1F5)
- **Success**: success-primary (#059669), success-hover (#047857), success-light (#d1fae5)
- **Warning/CTA**: warning-primary / cta (#D97706) - unified
- **Error**: error-primary (#DC2626), error-hover (#b91c1c), error-light (#fee2e2)
- **Warm Neutrals**: neutral-50 (#faf9f7), neutral-100 (#f5f1ed), neutral-200 (#e8e3de), neutral-300 (#ddd8d1), neutral-600 (#6b6561), neutral-700 (#4a4137)

### Using Colors

- **Semantic tokens**: Use `bg-trust`, `text-success-primary`, `border-neutral-300`
- **NO extended palettes**: `navy-700`, `emerald-600`, `gray-*` are removed
- **Warm neutrals**: Replace all `gray-*` with `neutral-*`

### Typography

- **Headings**: Crimson Pro, weight 700/600, -0.02em letter-spacing
- **Body**: Manrope, weight 400/500, -0.01em letter-spacing, 1rem base
- **Code**: JetBrains Mono, weight 500

### Spacing

- **Base unit**: 4px
- **Common**: 16px (md), 24px (lg), 32px (xl)
- **Sections**: 64px vertical

### Shadows

- **Cards**: `0 2px 8px rgba(0,0,0,0.04), inset 0 1px 0 rgba(255,255,255,1)`
- **Modals**: `0 20px 40px rgba(0,0,0,0.12)`

### Animation Timings

- **Fast**: 150ms (hover colors)
- **Base**: 200ms (button presses)
- **Slow**: 300ms (card hovers)
- **Slower**: 500ms (page transitions)

### Border Radius

- **Badges**: 4px
- **Buttons/Inputs**: 6px
- **Cards**: 12px
- **Modals**: 16px
