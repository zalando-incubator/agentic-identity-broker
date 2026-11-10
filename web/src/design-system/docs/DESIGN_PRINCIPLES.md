# Design Principles

The Refined Trust Architecture design system is built on a set of core principles that guide component design, interaction patterns, and implementation decisions. These principles ensure consistency, accessibility, and developer experience across all applications using this system.

---

## Visual Design Direction

[ADR 048](../../../../adrs/048-consent-ui-design-language.md) sets this direction. It replaces the earlier serif, navy and warm-neutral look. The public docs site shares the same language, so a consent screen in the product and the consent screen drawn on the docs homepage look like the same thing.

### Design Concept

**Character**: Exact, sober, accountable. The consent UI is where a person decides what an agent may do in their name. The interface states facts and asks one question. It does not sell.

**World**: The grant register. Principals, agents, permission sets and `valid_until` drive the visuals. Identifiers, scopes and dates set in monospace are the signature detail.

**The one accent**: A single jade marks only three things: the action the screen is for, links, and the granted state. Everything else is ink on a cool neutral ground. Nothing glows.

### Visual Design Principles

#### 1. Hierarchy Through Size and Spacing, Not Weight

Headlines use **Geist 500** at generous sizes. Body text uses **Geist 400** at 1rem with a 1.6 line height. Weights above 500 do not appear. Hierarchy comes from size, color and whitespace.

**Implementation:**

- Headings: Geist, weight 500, letter-spacing -0.015em (h1: -0.025em)
- Body text: Geist, weight 400 (regular) or 500 (labels, names, emphasis), default letter-spacing
- Monospace: Geist Mono for technical values (OAuth scopes, agent IDs, dates), weight 400

#### 2. Color as Semantic Signal

| Role                   | Hex Values                                              | Usage                                                         |
| ---------------------- | ------------------------------------------------------- | ------------------------------------------------------------- |
| **Ink (text)**         | #111827 (primary), #4b5565 (secondary), #6b7487 (tertiary) | Headings, body, metadata                                   |
| **Ground (surfaces)**  | #fbfcfd (page), #ffffff (surface), #f0f2f6 (raised)     | Page background, cards and dialogs, chips and code            |
| **Hairlines**          | rgba(17, 24, 39, 0.08) and 0.14                         | Card borders, rules, input and outline-button borders         |
| **Accent (jade)**      | #0a7a66, hover #086a59, tint rgba(10, 122, 102, 0.08)   | Primary action, links, focus ring, the granted state          |
| **Error**              | #b42318                                                 | Errors, revocation and other destructive actions              |
| **Warning**            | #b54708                                                 | Attention without alarm                                       |

The granted state and the primary action share the accent on purpose: approving a grant and seeing it active are the same fact at two moments.

#### 3. Containment Through Hairlines, Not Shadows

- Cards and panels sit on the page behind a 1px hairline. They do not float and do not lift on hover; an interactive card darkens its hairline to the accent.
- One floating shadow, `shadow-float`, lifts overlays only: modals, dropdowns, popovers and toasts.
- Focus states use a 2px accent ring at 2px offset.
- Nothing uses a gradient, a glow, a blur or a texture.

#### 4. Motion That Guides, Not Entertains

| Timing             | Use Case                                                     | Easing                            |
| ------------------ | ------------------------------------------------------------ | --------------------------------- |
| **Fast (150ms)**   | Hover color transitions, focus ring appearance               | cubic-bezier(0.22, 1, 0.36, 1)    |
| **Base (200ms)**   | Toggle switches, dropdown open/close                         | cubic-bezier(0.22, 1, 0.36, 1)    |
| **Slow (300ms)**   | Modal overlays                                               | cubic-bezier(0.22, 1, 0.36, 1)    |
| **Entrance (480ms)** | Page content rising 8px on load                            | cubic-bezier(0.22, 1, 0.36, 1)    |

One easing curve, `--ease-out-quint`, serves every transition. Nothing bounces. All animations respect `prefers-reduced-motion` and fall back to instant state changes.

#### 5. Whitespace as Structure

- Padding in cards: 24px standard, 16px for dense rows, 32px for important sections
- Vertical spacing between major sections: 48px to 64px
- Content max-width: 1280px (never full-bleed text)
- Hairline rules, not boxes, separate rows inside a card

### Distinctive Visual Details

#### The Grant Line

Facts about a grant appear in Geist Mono at 12px to 13px, in tertiary ink, separated by a middle dot: `one grant · per user · per agent`. The same treatment carries scopes, agent IDs and `valid_until` dates.

The agent detail grant line derives status from `valid_until`: past dates show `grant expired` with neutral text and a neutral dot. Future dates and grants without an end date show `grant active` with the jade accent.

#### Flat Ground

The page background is a flat `#fbfcfd`. There are no gradients, no micro-textures and no backdrop blur anywhere in the interface.

#### Border Radius Strategy

- Chips, inputs and small controls: 6px
- Buttons: 8px
- Cards and dropdowns: 12px
- Modals and the consent window: 16px
- Toggles, avatars and status dots: full

### Component Aesthetic Archetypes

#### Primary Button

- Background: accent (#0a7a66), text white, weight 500
- Height: 44px, padding 12px 16px, radius 8px
- Hover: accent hover (#086a59). No shadow, no lift.
- Focus: 2px accent ring at 2px offset
- Transition: colors 150ms

#### Outline Button (the quiet alternative, such as Decline or Cancel)

- Surface background, 1px strong hairline, ink text, weight 500
- Hover: raised background, darker hairline

#### Danger Button (Revoke, Delete)

- Surface background, 1px error-red hairline at 40%, error-red text, weight 500
- Hover: error tint background, full-strength hairline
- Red fills nothing until the action is confirmed in a dialog

#### Action Roles

Every action is a `Button`. Its variant follows from what the action does, never from where it sits:

| Role | Variant | Examples |
|---|---|---|
| Moves the task forward, one per view or card | `primary` | Approve, Save, Approve & Delegate, Login, Try Again on a crash screen |
| Neutral alternative or utility | `outline` | Deny, Deny this request, Go Back, View Details, Refresh, Retry |
| Removes access or blocks for good | `danger` | Revoke, Revoke All Access, Terminate, Disconnect, Deny permanently, Block permanently |
| Backs out of a step or dialog | `ghost` | Cancel |
| Toggle or chip inside content | `ghost` | Show more, date suggestions |

- Actions are never underlined text links, and never a `ghost` button with red text. Links are for navigation only.
- No colour overrides through `className` on a `Button`. If a role is missing, add a variant.
- `secondary` duplicates `primary` and is not used in the app.
- Buttons inside cards and rows use `size="sm"`. Page-level and dialog actions use the default size.

#### Action Order

Every button group follows one order, in dialogs, cards and pinned bars alike:

- The primary action is the last button and sits on the right: Approve, Save, Revoke All Access.
- The quiet alternative sits directly to its left: Deny, Cancel, Go Back.
- An extra destructive choice that is not the answer to the question, such as Deny permanently or Block permanently, sits on the far left, apart from the pair.
- On narrow screens a group may stack. It stacks with the primary action on top (`flex-col-reverse sm:flex-row`), so the DOM order and the tab order stay the same.

#### Card

- Surface background, 1px hairline, radius 12px, padding 24px
- Hover (interactive cards only): hairline takes the accent
- No shadow

#### Form Input

- Height 44px, padding 12px 16px, radius 6px
- Border: 1px strong hairline; accent on focus; error red on error
- Label: Geist 500, 14px, above the input

#### Toggle

- Off: raised background with a strong hairline, surface knob
- On: accent background, white knob
- Locked (mandatory permission set): on, at 55% opacity

#### Modal

- Overlay: ink at 50%
- Body: surface, radius 16px, 1px strong hairline, `shadow-float`
- Header: Geist 500, 20px to 24px

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
