# Motion & Animation Guide

Motion in the Refined Trust Architecture is purposeful and restrained. Animations guide user attention, provide feedback, and create fluid transitions—but never entertain or distract. Every animation serves a functional purpose.

## Authority and example status

[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current direction and Principle XI process.
The 150–500 ms motion scale describes Refined Trust Architecture, not a constitutional duration mandate.
ADR 037 remains Proposed. Its 120–200 ms CSS-only target does not replace current motion guidance.
Literal CSS examples explain existing motion. Components must express motion through centralized semantic tokens and respect reduced motion.
Verify every component story with the accessibility addon in light and dark themes.
Resource links are reading references, not permission to load third-party scripts into the frontend.
Align this guide with the implemented target in the single cutover after ADR acceptance.

---

## Animation Principles

1. **Purposeful**: Every animation communicates something (feedback, state change, direction)
2. **Restrained**: Animations are subtle and quick—respect user time
3. **Responsive**: Animations acknowledge user input immediately
4. **Accessible**: Respect `prefers-reduced-motion` preference
5. **Consistent**: Use the same timings and easing across the system

---

## Timing Hierarchy

The animation timing system uses four tiers based on the perceived distance or importance of the transition:

### Fast (150ms)

**Easing**: `cubic-bezier(0.4, 0, 0.2, 1)` (ease-in-out, slightly snappy)

**Use Cases**:

- Hover color transitions on buttons and links
- Focus ring appearance
- Icon rotation (chevron opens/closes)
- Small state changes (checkbox check/uncheck)
- Opacity changes (subtle elements fading in/out)

**Example**:

```css
button {
  transition: background-color 150ms cubic-bezier(0.4, 0, 0.2, 1);
}

button:hover {
  background-color: #1e4d6b;
}
```

### Base (200ms)

**Easing**: `cubic-bezier(0.34, 1.56, 0.64, 1)` (spring easing—slightly bouncy)

**Use Cases**:

- Button state changes (press/release)
- Dropdown open/close
- Toggle switches
- Tab switching (content swap)
- Small overlays (tooltips appearing)

**Example**:

```css
.button-group {
  transition: all 200ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

.button-group:active {
  transform: scale(0.98);
}
```

### Slow (300ms)

**Easing**: `cubic-bezier(0.34, 1.56, 0.64, 1)` (spring easing)

**Use Cases**:

- Card elevation changes on hover
- Modal overlay fade-in
- Component animations (accordion expand/collapse)
- Drawer sliding open/close
- List item appearing/disappearing
- Loading state transitions

**Example**:

```css
.card {
  transition: all 300ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

.card:hover {
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}
```

### Slower (500ms)

**Easing**: `cubic-bezier(0.34, 1.56, 0.64, 1)` (spring easing)

**Use Cases**:

- Page transitions / route changes
- Full-page loading states
- Large content switches (multi-step forms)
- Staggered animations (list items entering one by one)
- Complex state changes

**Example**:

```css
.page {
  animation: slideIn 500ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

@keyframes slideIn {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
```

---

## Easing Functions

### Primary Easing: Spring Curve

```
cubic-bezier(0.34, 1.56, 0.64, 1)
```

This is the "Refined Trust" easing function. It provides a subtle spring feeling—starting smoothly but with a slight bounce at the end. This creates a friendly, responsive feel without being playful.

**Visual characteristics**:

- Smooth acceleration in the beginning
- Slight overshoot at the end (~56% overshoot)
- Creates a sense of "life" and responsiveness
- Used for: button presses, card hovers, state changes

### Secondary Easing: Smooth Curve

```
cubic-bezier(0.4, 0, 0.2, 1)
```

A smooth, balanced easing that's neutral and professional.

**Visual characteristics**:

- Even acceleration and deceleration
- No overshoot—controlled
- Used for: color transitions, small elements, hover states

---

## Common Animation Patterns

### Button Interaction (200ms)

```css
button {
  background-color: #0d1829;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  transform: translateY(0);
  transition: all 200ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

button:hover {
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.1);
  transform: translateY(-1px);
}

button:active {
  transform: translateY(0);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}
```

### Card Elevation (300ms)

```css
.card {
  box-shadow:
    0 2px 8px rgba(0, 0, 0, 0.04),
    inset 0 1px 0 rgba(255, 255, 255, 1);
  transform: translateY(0);
  transition: all 300ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

.card:hover {
  box-shadow:
    0 8px 20px rgba(0, 0, 0, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 1);
  transform: translateY(-2px);
}
```

### Modal Entrance (Staggered)

```css
/* Overlay fades in quickly */
.modal-overlay {
  animation: fadeIn 200ms ease-out;
}

@keyframes fadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

/* Modal slides up with delay */
.modal {
  animation: slideUpScale 300ms cubic-bezier(0.34, 1.56, 0.64, 1);
  animation-delay: 100ms;
}

@keyframes slideUpScale {
  from {
    opacity: 0;
    transform: translateY(32px) scale(0.95);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}
```

### Dropdown Menu Open (200ms)

```css
.dropdown-menu {
  opacity: 0;
  transform: scaleY(0.95) translateY(-4px);
  pointer-events: none;
  transform-origin: top center;
  transition: all 200ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

.dropdown-menu.is-open {
  opacity: 1;
  transform: scaleY(1) translateY(0);
  pointer-events: auto;
}
```

### Skeleton Loading (Pulse)

```css
.skeleton {
  background: linear-gradient(90deg, #f5f1ed 25%, #e8e3de 50%, #f5f1ed 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

@keyframes shimmer {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}
```

### Accordion Expand (300ms)

```css
.accordion-panel {
  max-height: 0;
  overflow: hidden;
  transition: max-height 300ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

.accordion-panel.is-open {
  max-height: 1000px; /* Large enough for content */
}

.accordion-header {
  transition: all 300ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

.accordion-header.is-open .chevron {
  transform: rotate(180deg);
}
```

### Toast Notification (Slide In)

```css
.toast {
  animation: slideInRight 300ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

@keyframes slideInRight {
  from {
    opacity: 0;
    transform: translateX(100%);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

/* Auto-dismiss animation */
.toast.is-dismissing {
  animation: slideOutRight 300ms cubic-bezier(0.34, 1.56, 0.64, 1) forwards;
}

@keyframes slideOutRight {
  from {
    opacity: 1;
    transform: translateX(0);
  }
  to {
    opacity: 0;
    transform: translateX(100%);
  }
}
```

---

## Accessibility: Respecting prefers-reduced-motion

Users who prefer reduced motion should still see state changes—just instantly:

```css
@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
```

Alternatively, use conditional animations in React:

```tsx
const prefersReducedMotion = window.matchMedia(
  '(prefers-reduced-motion: reduce)',
).matches;

const cardStyle = prefersReducedMotion
  ? {} // No animation
  : {
      transition: 'all 300ms cubic-bezier(0.34, 1.56, 0.64, 1)',
    };
```

---

## Animation Performance Tips

1. **Use `transform` and `opacity`**: These are GPU-accelerated
2. **Avoid animating `width` or `height`**: Use `max-height` or `scale` instead
3. **Prefer CSS animations for simple transitions**: Use JavaScript (Framer Motion) for complex interactions
4. **Keep animations under 500ms**: Users perceive anything longer as lag
5. **Stagger animations**: Avoid animating everything at once

### Good (GPU-accelerated)

```css
.element {
  transform: translateY(-2px);
  opacity: 0.8;
}
```

### Bad (CPU-intensive)

```css
.element {
  top: -2px;
  height: 100px; /* animating height causes layout thrashing */
}
```

---

## Animation Component Integration

### With Framer Motion

```tsx
import { motion } from 'framer-motion';

export const AnimatedCard = ({ children }) => (
  <motion.div
    initial={{ opacity: 0, y: 10 }}
    whileInView={{ opacity: 1, y: 0 }}
    whileHover={{ y: -8, boxShadow: '0 8px 20px rgba(0,0,0,0.1)' }}
    transition={{ type: 'spring', stiffness: 300, damping: 30 }}
  >
    {children}
  </motion.div>
);
```

### With CSS Transitions (Tailwind)

```tsx
export const Button = ({ children }) => (
  <button className="transition-all duration-200 hover:translate-y-[-1px] hover:shadow-lg active:translate-y-0">
    {children}
  </button>
);
```

---

## Animation Timing Reference Chart

| Interaction        | Duration | Easing      | Example              |
| ------------------ | -------- | ----------- | -------------------- |
| Hover color        | 150ms    | ease-in-out | Link color change    |
| Button press       | 200ms    | spring      | Button press/release |
| Dropdown open      | 200ms    | spring      | Menu appears         |
| Card hover         | 300ms    | spring      | Card elevation       |
| Modal enter        | 300ms    | spring      | Dialog appears       |
| Page transition    | 500ms    | spring      | Route change         |
| Disabled → enabled | 150ms    | ease-in-out | Form input state     |

---

## Summary

The Refined Trust Architecture uses purposeful, restrained motion that:

- **Communicates** state changes and user feedback
- **Guides** attention without distraction
- **Respects** user preferences (prefers-reduced-motion)
- **Performs** well on all devices
- **Feels** responsive and alive with subtle spring easing

Every animation should serve a purpose. If you're adding motion, ask: "What is this animation telling the user?"

---

## Resources

- [Easing Functions Cheat Sheet](https://easings.net/)
- [Cubic Bezier Generator](https://cubic-bezier.com/)
- [prefers-reduced-motion MDN](https://developer.mozilla.org/en-US/docs/Web/CSS/@media/prefers-reduced-motion)
- [Framer Motion Docs](https://www.framer.com/motion/)
- [Web Animation Performance](https://developer.chrome.com/blog/animation-performance/)
