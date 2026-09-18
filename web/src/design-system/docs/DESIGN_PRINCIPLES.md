# Design Principles

The Refined Trust Architecture design system is built on a set of core principles that guide component design, interaction patterns, and implementation decisions. These principles ensure consistency, accessibility, and developer experience across all applications using this system.

---

## Visual Design Direction: "Refined Trust Architecture"

### Design Concept

**Core Aesthetic**: Sophisticated legal-financial hybrid—the visual gravity of a bank vault combined with the approachability of modern SaaS. This is where users make critical decisions about AI agent permissions, so every pixel must communicate trustworthiness, clarity, and control.

**Tone**: Authoritative yet approachable. Think: premium law firm website meets modern fintech app. NOT corporate-bland, NOT startup-playful.

**The Unforgettable Element**: **Serif authority with warm humanity** - Using Crimson Pro (a refined, slightly warm serif) for all headings creates immediate trust and seriousness, while Manrope's humanist sans-serif for body text softens the experience. The contrast between these two typefaces defines the entire aesthetic.

### Visual Design Principles

#### 1. Visual Hierarchy Through Weight

Headlines use **Crimson Pro Bold (700)** at generous sizes (2.5rem+) to establish authority. Body text uses **Manrope** at comfortable reading sizes (1rem base, 1.5 line-height). The typographic contrast creates natural scanning patterns without needing bold colors.

**Implementation:**

- Headings: Crimson Pro, weight 700 (bold) or 600 (semibold), letter-spacing -0.02em
- Body text: Manrope, weight 400 (regular) or 500 (medium), letter-spacing -0.01em
- Monospace: JetBrains Mono for technical values (OAuth scopes, agent IDs), weight 500

#### 2. Color as Semantic Signal

| Color                   | Hex Values                                        | Usage                                                           |
| ----------------------- | ------------------------------------------------- | --------------------------------------------------------------- |
| **Navy (Trust)**        | #0A2540 (deep), #1E4D6B (medium), #E8F1F5 (light) | Primary actions, headings, critical UI chrome—conveys stability |
| **Emerald (Success)**   | #059669                                           | Success states, granted permissions—nature's "go ahead" signal  |
| **Amber (CTA/Warning)** | #D97706                                           | CTAs and warnings—attention without alarm                       |
| **Warm Neutrals**       | #faf9f7 (cream), #f5f1ed (sand), #e8e3de (taupe)  | Backgrounds create gentle, non-clinical environment             |
| **Pure White**          | #ffffff                                           | Elevated cards that "float" above warm background               |

#### 3. Elevation Through Shadow, Not Borders

- Cards use subtle multi-layer shadows: `0 2px 8px rgba(0,0,0,0.04), inset 0 1px 0 rgba(255,255,255,1)`
- Card hover states increase elevation: `0 8px 20px rgba(0,0,0,0.1), inset 0 1px 0 rgba(255,255,255,1)`
- Modal shadows: `0 20px 40px rgba(0,0,0,0.12)`
- Focus states use 2px navy ring at 2px offset (clear, not aggressive)
- Borders are used sparingly, only for visual containment (muted taupe/slate)

#### 4. Motion That Guides, Not Entertains

| Timing             | Use Case                                                     | Easing                            |
| ------------------ | ------------------------------------------------------------ | --------------------------------- |
| **Fast (150ms)**   | Hover color transitions, focus ring appearance               | cubic-bezier(0.4, 0, 0.2, 1)      |
| **Base (200ms)**   | Button state changes, dropdown open/close                    | cubic-bezier(0.34, 1.56, 0.64, 1) |
| **Slow (300ms)**   | Card elevation changes, modal overlays, component animations | cubic-bezier(0.34, 1.56, 0.64, 1) |
| **Slower (500ms)** | Page transitions, full-screen loading states                 | cubic-bezier(0.34, 1.56, 0.64, 1) |

All animations respect `prefers-reduced-motion` preference—fallback to instant state changes for users who prefer reduced motion.

#### 5. Whitespace as a Luxury Signal

- Generous padding in cards: 24px standard, 32px for important sections
- Vertical spacing between major sections: 64px
- Content max-width: 1280px (never full-bleed text)
- Components breathe—never cramped or overcrowded

### Distinctive Visual Details

#### Gradient Backgrounds

Not typical purple-to-pink. Our gradients are **warm neutrals with subtle shifts**:

```css
background: linear-gradient(
  135deg,
  #faf9f7 0%,
  #f5f1ed 50%,
  rgba(232, 227, 222, 0.3) 100%
);
```

This gradient creates a sophisticated, calm backdrop that doesn't compete with content.

#### Micro-Textures

Subtle grain overlay on cards (2% opacity noise) adds tactile quality without being distracting. This gives the interface a premium, printed feel while maintaining digital clarity.

#### Shadow Strategy

**Cards (default state)**

```css
box-shadow:
  0 2px 8px rgba(0, 0, 0, 0.04),
  inset 0 1px 0 rgba(255, 255, 255, 1);
```

Combines subtle external drop shadow with internal highlight to create depth.

**Cards (hover state)**

```css
box-shadow:
  0 8px 20px rgba(0, 0, 0, 0.1),
  inset 0 1px 0 rgba(255, 255, 255, 1);
```

Elevation increases on hover for interactive feedback.

**Modals**

```css
box-shadow: 0 20px 40px rgba(0, 0, 0, 0.12);
```

Strong separation from page—creates modal prominence.

#### Border Radius Strategy

| Component | Radius     | Pixel Value                      |
| --------- | ---------- | -------------------------------- |
| Buttons   | 6px (md)   | Subtle rounding, not pill-shaped |
| Cards     | 12px (xl)  | Generous but not extreme         |
| Modals    | 16px (2xl) | Premium feel                     |
| Inputs    | 6px (md)   | Consistency with buttons         |
| Badges    | 4px (base) | Compact, not rounded pills       |

### Component Aesthetic Archetypes

#### Primary Button

The most important visual element in the interface.

**Visual Specifications:**

- Background: Deep navy (#0d1829) with subtle gradient overlay
- Text: Pure white (#ffffff) in Manrope Medium (500)
- Height: 44px (touch-friendly)
- Padding: 12px 16px
- Shadow: `0 2px 8px rgba(0,0,0,0.08)` (default), `0 8px 20px rgba(0,0,0,0.1)` (hover)
- Transform: `translateY(-1px)` on hover (subtle lift)
- Loading state: Spinning ring in white, button slightly desaturated
- Transition: All properties 200ms cubic-bezier(0.34, 1.56, 0.64, 1)

#### Card Component

The workhorse of the UI—used for content grouping and elevation.

**Visual Specifications:**

- Background: Pure white with 10px blur backdrop-filter (if supported)
- Border: 1px solid rgba(240, 237, 232, 0.8) (barely visible, just containment)
- Padding: 24px standard, 32px for headers
- Hover: Elevation increase + translateY(-2px) shift
- Shadow: `0 2px 8px rgba(0,0,0,0.04), inset 0 1px 0 rgba(255,255,255,1)` (default)
- Hover shadow: `0 8px 20px rgba(0,0,0,0.1), inset 0 1px 0 rgba(255,255,255,1)`
- Transitions: All properties 300ms cubic-bezier(0.34, 1.56, 0.64, 1)

#### Form Input

Trust through clarity—users need to feel confident entering sensitive data.

**Visual Specifications:**

- Border: 1.5px solid neutral-300 (default), navy-700 (focus), red-700 (error)
- Height: 44px (touch-friendly)
- Padding: 12px 16px
- Label: Manrope Medium (500), 14px, positioned above input (not floating)
- Helper text: 12px, muted secondary color
- Error message: 12px, error color (#DC2626), with small alert icon
- Focus ring: 2px solid navy-700 at 2px offset
- Disabled state: gray-400 background, gray-300 border

#### Modal

Command attention without aggression—critical for permission dialogs.

**Visual Specifications:**

- Overlay: rgba(13, 24, 41, 0.5) with 8px backdrop-blur (dark navy, semi-transparent)
- Modal body: Pure white, 16px border-radius, `0 20px 40px rgba(0,0,0,0.12)` shadow
- Animation: Overlay fades in 200ms, modal slides up and scales in 300ms with delay
- Close button: Ghost style in top-right, 44×44px minimum for accessibility
- Padding: 24px (standard), 32px (spacious sections)
- Header: Separate background (if multi-section), font-size 24px, Crimson Pro Bold

---

## Core Principles

### 1. Simplicity & Clarity

**Definition**: Design systems should reduce complexity and make interfaces predictable and easy to understand.

**Application:**

- Each component has a single, well-defined purpose
- Props interfaces are intuitive with sensible defaults
- Components are named clearly (e.g., `GrantStatusBadge` for grant statuses)
- Complex functionality is broken into smaller, composable pieces

**Examples:**

- `Badge` component only displays a tag/status - no interaction
- `Button` component has clear variants (primary, secondary, ghost, danger)
- Props like `variant`, `size`, `disabled` follow consistent naming across all components

### 2. Accessibility First (WCAG 2.1 AA)

**Definition**: All components must be usable by everyone, including people with disabilities.

**Implementation:**

- Semantic HTML (proper use of `<button>`, `<a>`, `<nav>`, `<main>`, etc.)
- ARIA attributes for screen readers (`aria-label`, `aria-expanded`, `aria-current`)
- Keyboard navigation support for all interactive elements
- Color contrast ratios meet WCAG AA standards (4.5:1 for text, 3:1 for graphics)
- Focus indicators visible and consistent across all components
- Proper heading hierarchy (h1 > h2 > h3, etc.)

**Examples:**

- Modal uses ARIA `role="dialog"` with `aria-labelledby` for title
- Accordion items have `aria-expanded` to indicate open/closed state
- Table headers use `<th>` with proper `scope` attributes
- All buttons have descriptive `aria-label` or visible text

### 3. Consistency

**Definition**: Similar functionality should look and behave the same across all components.

**Application:**

- Design tokens (colors, spacing, typography) are reused consistently
- CVA (class-variance-authority) manages variants programmatically
- Interaction patterns are standardized (e.g., all forms have similar patterns)
- Props naming conventions are consistent (`size`, `variant`, `disabled`)
- Animation timings and easing curves are unified

**Examples:**

- All components support `size` prop with same values: `sm`, `md`, `lg`
- All buttons use same color palette and hover/focus states
- All form inputs have consistent error display and labeling
- All overlays (Modal, Tooltip, Dropdown) use Headless UI for consistency

### 4. Flexibility

**Definition**: Components should be flexible enough to handle various use cases without creating new components.

**Implementation:**

- Props are composable and combine predictably
- Children support allows for custom content
- Slots/render props for advanced customization
- Support both controlled and uncontrolled patterns
- Responsive design built-in via Tailwind utilities

**Examples:**

- `Stack` component can be used for any flex layout (horizontal, vertical, with gaps)
- `Card` component accepts header, footer, and children for flexible layouts
- `Button` accepts icon slot, making it versatile across use cases
- `Accordion` supports rich content in items (not just text)

### 5. Performance

**Definition**: Components should be optimized for rendering performance and bundle size.

**Implementation:**

- Proper use of React hooks (`useMemo`, `useCallback`) to prevent unnecessary re-renders
- Memoization of expensive operations
- Lazy loading for heavy components
- CSS utilities instead of inline styles where possible
- Tree-shaking friendly exports

**Examples:**

- Dropdown groups items using `useMemo` to prevent infinite re-renders
- Card uses `React.forwardRef` for efficient ref handling
- SVG icons are inlined to avoid HTTP requests
- CSS animations instead of JavaScript where possible

### 6. Progressive Enhancement

**Definition**: Core functionality works without JavaScript; enhanced experiences layer on top.

**Application:**

- Base HTML semantic structure works without styling
- Form inputs work with native browser functionality
- Links navigate properly even if JavaScript fails
- Progressive enhancement of interactive features

**Examples:**

- Link components render as proper `<a>` tags with `href`
- Form inputs accept native HTML attributes
- Date pickers fall back to native date input
- Modals dismiss with ESC key for accessibility

## Design Values

### Trust & Transparency

The Agentic Identity Broker helps users make informed decisions about data sharing. This reflects in our design:

- Clear, honest communication about permissions and data usage
- Visual indicators for status and security level
- No hidden actions or surprises
- Explicit confirmation for important actions

**Components reflecting this value:**

- `GrantStatusBadge` - Clear permission status indicators
- `ScopeList` - Transparent permission breakdown
- `Alert` - Clear messaging for important information
- `Modal` - Confirmation dialogs for critical actions

### Efficiency

Users often need to manage many delegations and permissions. Design supports this:

- Quick actions and keyboard shortcuts
- Efficient layouts that show relevant information
- Smart defaults that reduce decision fatigue
- Batch operations where appropriate

**Components reflecting this value:**

- `Pagination` - Navigate large datasets
- `Table` - Display and sort many items
- `Tabs` - Organize content without changing pages
- `Breadcrumb` - Quick navigation context

### User Control

Users should always feel in control of their security and data:

- Clear options for enabling/disabling features
- Easy undo actions where possible
- Explicit confirmation before destructive actions
- Easy access to detailed information

**Components reflecting this value:**

- `Switch` - Clear on/off toggle
- `Checkbox` - Explicit selection control
- `GrantValidityControl` - Fine-grained permission control
- `AppLayout` - Customizable layout and sidebar

## Implementation Guidelines

### When to Create a New Component

Create a new component when:

1. The component has a distinct, well-defined purpose
2. It's reused across multiple applications
3. It has specific styling or interaction patterns
4. It doesn't fit naturally into existing components

Don't create a new component if:

1. It's application-specific logic wrapped in a component
2. It can be composed from existing components
3. It's used in only one place
4. It's a simple styled wrapper (use utilities instead)

### When to Extend an Existing Component

Extend a component when:

1. New functionality is closely related to the component's purpose
2. The component is already complex but benefits from the new behavior
3. The new functionality would be confusing on its own
4. Users expect this feature as part of the component

Examples of extension:

- Adding `expandable` prop to Accordion (similar interface expansion)
- Adding risk level colors to GrantStatusBadge (same purpose, more context)
- Adding sorting to Table (common data table feature)

### Component Maturity Levels

**Level 1: Experimental**

- New component, limited use
- Public API stability is not guaranteed
- Not recommended for production until Level 2

**Level 2: Stable**

- Used in production applications
- API is stable
- Comprehensive documentation and stories
- Accessibility verified

**Level 3: Mature**

- Well-established in the system
- Proven across multiple applications
- Rich ecosystem of examples
- Strong community adoption

All current design system components are at **Level 2: Stable** or higher.

## Design Evolution

The design system evolves based on:

1. User feedback and usage patterns
2. Accessibility improvements and compliance
3. Performance monitoring and optimization
4. Emerging design patterns and best practices
5. Framework and dependency updates

### API Evolution Policy

- Use a major version for incompatible public API changes
- Prefer additive public API changes
- Keep public APIs stable

## Summary

These principles ensure that the Refined Trust Architecture design system remains:

- **Accessible** to all users
- **Consistent** across applications
- **Performant** and efficient
- **Flexible** for different use cases
- **Trustworthy** and transparent
- **Maintainable** by development teams

By following these principles, we create interfaces that users trust and developers love to build with.
