# Design Decision Trees

Quick-reference decision trees to eliminate ambiguity when building with the Refined Trust Architecture design system. Use these flowcharts to make confident design choices without guesswork.

---

## Button Variant Selection

### Decision Flowchart

```
Is this a destructive action (delete, revoke, remove)?
├─ YES → Use `variant="danger"` (error-primary background)
└─ NO ↓

Is this the PRIMARY action on the page/section?
├─ YES → Use `variant="primary"` (trust-deep background)
└─ NO ↓

Does this action need visual prominence but isn't primary?
├─ YES → Use `variant="secondary"` (neutral-100 background, trust-deep text)
└─ NO ↓

Is this action supporting/tertiary?
├─ YES ↓
│   Does it need visible boundaries?
│   ├─ YES → Use `variant="outline"` (transparent bg, trust-deep border)
│   └─ NO → Use `variant="ghost"` (transparent bg, no border)
└─ NO → Default to `variant="primary"`
```

### Examples by Context

| Context             | Primary Action           | Secondary Action         | Tertiary Action     | Destructive               |
| ------------------- | ------------------------ | ------------------------ | ------------------- | ------------------------- |
| **Modal Dialog**    | "Confirm" (primary)      | "Cancel" (ghost)         | N/A                 | "Delete Forever" (danger) |
| **Form**            | "Submit" (primary)       | "Save Draft" (secondary) | "Preview" (outline) | "Reset Form" (ghost)      |
| **Card**            | "Grant Access" (primary) | "View Details" (outline) | "Copy Link" (ghost) | "Revoke" (danger)         |
| **Permission List** | "Approve All" (primary)  | "Review" (secondary)     | "Skip" (ghost)      | "Deny All" (danger)       |
| **Navigation**      | N/A                      | N/A                      | Links (ghost)       | N/A                       |

### Visual Hierarchy Rules

- **One primary button per section** - Multiple primaries create confusion
- **Danger buttons stand alone** - Never pair danger with primary (user must focus)
- **Ghost for repetitive actions** - Use when button appears many times (cards, lists)
- **Outline for equal-weight actions** - Multiple secondary actions of similar importance

---

## Text Color Hierarchy

### Decision Flowchart

```
What type of text is this?

Is it a HEADING (h1-h6)?
├─ YES ↓
│   Is it the primary page heading (h1) or major section (h2)?
│   ├─ YES → Use `text-trust-deep` (#111827)
│   └─ NO (h3-h6) → Use `text-trust` (#0a7a66)
└─ NO ↓

Is it BODY TEXT (paragraphs, descriptions)?
├─ YES ↓
│   Is it primary content?
│   ├─ YES → Use `text-neutral-700` (#364152)
│   └─ NO (supporting/secondary) → Use `text-secondary` (neutral-600, #4b5565)
└─ NO ↓

Is it METADATA or LABELS?
├─ YES → Use `text-tertiary` (neutral-500, #6b7487)
└─ NO ↓

Is it DISABLED or PLACEHOLDER?
├─ YES → Use `text-disabled` (neutral-400, #a9afbd)
└─ NO ↓

Is it a STATUS INDICATOR?
├─ Granted/Success → Use `text-success-dark` (#075e4f)
├─ Pending/Warning → Use `text-warning-dark` (#7a2e0e)
├─ Error/Denied → Use `text-error-primary` (#b42318)
└─ Info → Use `text-info-dark` (#1e3a8a)
```

### Typography Hierarchy Examples

```tsx
// Page Structure
<h1 className="text-4xl font-bold text-trust-deep">       {/* Primary page heading */}
  Consent Management Dashboard
</h1>

<h2 className="text-2xl font-bold text-trust-deep">       {/* Major section */}
  Active Permissions
</h2>

<h3 className="text-xl font-semibold text-trust">         {/* Subsection */}
  Healthcare Provider Access
</h3>

<p className="text-base text-neutral-700">                {/* Body content */}
  This permission allows your healthcare provider to access your medical records
  for treatment purposes. You can revoke this at any time.
</p>

<p className="text-sm text-secondary">                    {/* Supporting text */}
  Last updated: January 15, 2024
</p>

<span className="text-xs text-tertiary">                  {/* Metadata */}
  ID: grant_abc123xyz
</span>
```

### Common Mistakes to Avoid

❌ **DON'T** use `text-primary` for headings (confusing alias)
✅ **DO** use `text-trust-deep` for h1/h2, `text-trust` for h3-h6

❌ **DON'T** use `text-neutral-900` for headings
✅ **DO** use trust colors for brand consistency

❌ **DON'T** use `text-neutral-600` directly
✅ **DO** use `text-secondary` semantic alias

---

## Animation Duration Selection

### Decision Flowchart

```
What is being animated?

Is it a COLOR or OPACITY change?
├─ YES → Use **150ms** (fast)
└─ NO ↓

Is it a small TRANSFORM (button press, focus ring)?
├─ YES → Use **200ms** (base)
└─ NO ↓

Is it a medium ELEVATION change (card hover, dropdown)?
├─ YES → Use **300ms** (slow)
└─ NO ↓

Is it a large TRANSITION (page change, full-screen modal)?
├─ YES → Use **500ms** (slower)
└─ NO → Default to **200ms**
```

### Duration by Element Type

| Element              | Duration | Use Case                       | Easing                              |
| -------------------- | -------- | ------------------------------ | ----------------------------------- |
| **Button hover**     | 150ms    | Background color change        | `cubic-bezier(0.4, 0, 0.2, 1)`      |
| **Button press**     | 200ms    | Scale down (active state)      | `cubic-bezier(0.34, 1.56, 0.64, 1)` |
| **Focus ring**       | 150ms    | Border color + ring appearance | `cubic-bezier(0.4, 0, 0.2, 1)`      |
| **Card hover**       | 300ms    | Elevation + translateY         | `cubic-bezier(0.34, 1.56, 0.64, 1)` |
| **Dropdown open**    | 200ms    | Scale + opacity                | `cubic-bezier(0.34, 1.56, 0.64, 1)` |
| **Modal enter**      | 300ms    | Scale + opacity (staggered)    | `cubic-bezier(0.34, 1.56, 0.64, 1)` |
| **Accordion expand** | 300ms    | Height transition              | `cubic-bezier(0.34, 1.56, 0.64, 1)` |
| **Page transition**  | 500ms    | Fade + slide                   | `cubic-bezier(0.34, 1.56, 0.64, 1)` |
| **Skeleton pulse**   | 1500ms   | Opacity shimmer (loop)         | `cubic-bezier(0.4, 0, 0.6, 1)`      |

### Edge Cases

**Multiple properties animating together?**

- Use the slowest duration for consistency
- Example: Card hover (bg-color + shadow + transform) → 300ms for all

**Hover on small elements (icons, badges)?**

- Use 150ms for snappy feel

**User-triggered vs automatic?**

- User-triggered: Use stated durations
- Automatic (toast dismiss): Add +100ms for user to notice

**Background color on Card hover?**

- 300ms (matches elevation change duration)

---

## Spacing & Padding Selection

### Decision Flowchart

```
What component needs spacing?

Is it a BUTTON?
├─ Size: sm → `px-3 py-2` (12px/8px)
├─ Size: md → `px-4 py-3` (16px/12px)
└─ Size: lg → `px-6 py-4` (24px/16px)

Is it a CARD?
├─ Compact → `p-4` (16px all sides)
├─ Default → `p-6` (24px all sides)
└─ Spacious → `p-8` (32px all sides)

Is it a FORM INPUT?
├─ Height → Always 44px (`h-11`)
└─ Padding → `px-4 py-3` (16px/12px)

Is it a MODAL?
├─ Body → `p-6` (24px)
└─ Header/Footer → `p-8` (32px)

Is it VERTICAL SPACING between elements?
├─ Related items → `gap-2` or `gap-3` (8px or 12px)
├─ Section items → `gap-4` or `gap-6` (16px or 24px)
└─ Major sections → `gap-16` (64px)
```

### Spacing Scale Reference

```tsx
// Component Internal Spacing
<Button size="sm" />          // px-3 py-2  (12px/8px)
<Button size="md" />          // px-4 py-3  (16px/12px) ← Default
<Button size="lg" />          // px-6 py-4  (24px/16px)

// Card Padding
<Card padding="compact" />    // p-4  (16px)
<Card padding="default" />    // p-6  (24px) ← Default
<Card padding="spacious" />   // p-8  (32px)

// Stack/Layout Gaps
<Stack gap="sm" />            // gap-2  (8px) - tight grouping
<Stack gap="md" />            // gap-4  (16px) ← Default
<Stack gap="lg" />            // gap-6  (24px) - section spacing
<Stack gap="xl" />            // gap-16 (64px) - major sections
```

### Context-Specific Guidelines

**Form Fields**

- Between label and input: `mb-2` (8px)
- Between input and helper text: `mt-1` (4px)
- Between form fields: `mb-4` (16px)
- Between form sections: `mb-8` (32px)

**Cards in Grid**

- Card-to-card gap: `gap-6` (24px) on desktop
- Card-to-card gap: `gap-4` (16px) on mobile

**Modal Dialog**

- Body padding: `p-6` (24px)
- Header padding: `px-6 py-5` (24px/20px)
- Between header and body: No gap (border handles separation)
- Between body and footer: No gap (border handles separation)

---

## Shadow Selection

### Decision Flowchart

```
What needs elevation?

Is it a CARD at rest?
├─ YES → Use `shadow-sm` (subtle hint of depth)
└─ NO ↓

Is it a CARD (rest or hover)?
├─ YES → NO shadow. Hairline `border border-neutral-200`; interactive cards use `hover:border-accent`
└─ NO ↓

Is it an OVERLAY (modal, dropdown, popover, toast)?
├─ YES → Use `shadow-float` with `border border-line-strong`
└─ NO ↓

Is it a BUTTON or INPUT?
├─ YES → NO shadow (background or hairline provides definition)
└─ NO → Default to no shadow
```

### Shadow Hierarchy

| Element            | Shadow              | When          | Purpose                    |
| ------------------ | ------------------- | ------------- | -------------------------- |
| **Cards (rest)**   | None, hairline      | Default state | Containment, no float      |
| **Cards (hover)**  | None, accent hairline | Hover state | Clear interactive feedback |
| **Dropdowns**      | `shadow-float`      | Open state    | Separate from page content |
| **Popovers**       | `shadow-float`      | Visible       | Floating element clarity   |
| **Modals**         | `shadow-float`      | Open state    | The one floating shadow    |
| **Tooltips**       | `shadow-float`      | Visible       | Floating element clarity   |
| **Buttons**        | None                | All states    | Background provides depth  |
| **Inputs**         | None                | Default       | Border provides definition |
| **Inputs (focus)** | `ring-2`            | Focus state   | Use ring, not shadow       |

### Custom Shadow Values

```tsx
// colors.css defines the one floating shadow
'shadow-float': '0 1px 2px rgba(17, 24, 39, 0.04), 0 12px 32px rgba(17, 24, 39, 0.08)'
```

---

## Border Radius Selection

### Decision Flowchart

```
What component needs border radius?

Is it a BUTTON or INPUT?
├─ YES → Use `rounded-md` (6px)
└─ NO ↓

Is it a CARD?
├─ YES → Use `rounded-xl` (12px)
└─ NO ↓

Is it a MODAL?
├─ YES → Use `rounded-2xl` (16px)
└─ NO ↓

Is it a BADGE or TAG?
├─ YES → Use `rounded` (4px)
└─ NO ↓

Is it an AVATAR or PROFILE IMAGE?
├─ YES → Use `rounded-lg` (8px) or `rounded-full` (circular)
└─ NO → Default to `rounded-lg`
```

### Component-Specific Radius

| Component      | Radius        | Pixels | Rationale                             |
| -------------- | ------------- | ------ | ------------------------------------- |
| **Buttons**    | `rounded-md`  | 6px    | Subtle rounding, professional         |
| **Inputs**     | `rounded-md`  | 6px    | Match button consistency              |
| **Cards**      | `rounded-xl`  | 12px   | Cards and dropdowns                   |
| **Modals**     | `rounded-2xl` | 16px   | Maximum refinement                    |
| **Badges**     | `rounded`     | 4px    | Compact, not pill-shaped              |
| **Dropdowns**  | `rounded-lg`  | 8px    | Balance between card and button       |
| **Avatars**    | `rounded-lg`  | 8px    | Soft square (not circular by default) |
| **Images**     | `rounded-xl`  | 12px   | Match card radius                     |
| **Checkboxes** | `rounded-sm`  | 2px    | Minimal rounding                      |

---

## Modal Size Selection

### Decision Flowchart

```
How much content does the modal contain?

Is it a simple confirmation (1-2 sentences)?
├─ YES → Use `size="sm"` (400px max-width)
└─ NO ↓

Is it a standard form or content (3-5 fields)?
├─ YES → Use `size="md"` (600px max-width) ← Default
└─ NO ↓

Is it a complex form or rich content (6-10 fields)?
├─ YES → Use `size="lg"` (800px max-width)
└─ NO ↓

Is it full content (data tables, extended forms)?
├─ YES → Use `size="xl"` (1000px max-width)
└─ NO ↓

Does it need maximum space?
└─ YES → Use `size="full"` (90vw max-width)
```

### Examples by Content Type

| Content Type            | Modal Size | Max Width | Example                         |
| ----------------------- | ---------- | --------- | ------------------------------- |
| **Simple confirmation** | `sm`       | 400px     | "Delete this item?"             |
| **Permission grant**    | `md`       | 600px     | Grant form with scope selection |
| **Detailed form**       | `lg`       | 800px     | Multi-step agent registration   |
| **Data table**          | `xl`       | 1000px    | Permission audit log viewer     |
| **Full-screen editor**  | `full`     | 90vw      | JSON permission editor          |

---

## Status Color Selection

### Decision Flowchart

```
What status are you indicating?

POSITIVE states (approved, granted, active, success)?
├─ Background → `bg-success-light` (#e7f3f0)
├─ Text → `text-success-dark` (#075e4f)
└─ Border → `border-success-primary` (#0a7a66)

NEGATIVE states (denied, revoked, error, failed)?
├─ Background → `bg-error-light` (#fef3f2)
├─ Text → `text-error-primary` (#b42318)
└─ Border → `border-error-primary` (#b42318)

PENDING states (awaiting approval, processing)?
├─ Background → `bg-warning-light` (#fffaeb)
├─ Text → `text-warning-dark` (#7a2e0e)
└─ Border → `border-warning-primary` (#b54708)

NEUTRAL states (inactive, disabled)?
├─ Background → `bg-neutral-100` (#f0f2f6)
├─ Text → `text-neutral-600` (#4b5565)
└─ Border → `border-neutral-300` (#d2d6de)

INFO states (notice, tip, reference)?
├─ Background → `bg-info-light` (#eef2fc)
├─ Text → `text-info-dark` (#1e3a8a)
└─ Border → `border-info-primary` (#3056d3)
```

### Badge Examples

```tsx
// Success
<Badge variant="success">Granted</Badge>
// → bg-success-primary (#0a7a66), text-white

// Warning
<Badge variant="warning">Pending</Badge>
// → bg-warning-primary (#b54708), text-trust-deep

// Error
<Badge variant="error">Revoked</Badge>
// → bg-error-primary (#b42318), text-white

// Neutral
<Badge variant="neutral">Inactive</Badge>
// → bg-neutral-300 (#d2d6de), text-neutral-600 (#4b5565)
```

### Alert Examples

```tsx
// Success Alert
<Alert variant="success">
  Permission granted successfully
</Alert>
// → bg-success-light, border-success-primary, text-success-dark

// Error Alert
<Alert variant="error">
  Access denied: insufficient permissions
</Alert>
// → bg-error-light, border-error-primary, text-error-primary
```

---

## Icon Sizing

### Decision Flowchart

```
Where is the icon being used?

Inside a BUTTON?
├─ Size: sm → `w-4 h-4` (16px)
├─ Size: md → `w-5 h-5` (20px)
└─ Size: lg → `w-6 h-6` (24px)

Inside a BADGE or TAG?
└─ → `w-3 h-3` or `w-4 h-4` (12-16px)

As DECORATIVE ELEMENT in content?
├─ Small accent → `w-5 h-5` (20px)
├─ Medium emphasis → `w-6 h-6` (24px)
└─ Large feature → `w-8 h-8` or `w-12 h-12` (32-48px)

As EMPTY STATE illustration?
└─ → `w-16 h-16` to `w-24 h-24` (64-96px)
```

### Icon Size Reference

```tsx
// Buttons
<Button size="sm">
  <Icon className="w-4 h-4" />  {/* 16px */}
  Small Button
</Button>

<Button size="md">
  <Icon className="w-5 h-5" />  {/* 20px - Default */}
  Medium Button
</Button>

<Button size="lg">
  <Icon className="w-6 h-6" />  {/* 24px */}
  Large Button
</Button>

// Badges
<Badge>
  <Icon className="w-3 h-3" />  {/* 12px */}
  Status
</Badge>

// Empty States
<EmptyState>
  <Icon className="w-16 h-16 text-neutral-400" />  {/* 64px */}
  <p>No permissions granted yet</p>
</EmptyState>
```

---

## Summary

These decision trees eliminate ambiguity and enable confident, consistent design decisions. When in doubt:

1. **Start with the default** - Most decisions have a sensible default
2. **Consider the context** - User action importance guides variant choice
3. **Maintain hierarchy** - Visual weight should match importance
4. **Test the extremes** - If unclear, try the smallest and largest options
5. **Ask for review** - When truly uncertain, get a second opinion

Remember: **Consistency beats perfection**. It's better to choose decisively and be consistent than to agonize over every choice.
