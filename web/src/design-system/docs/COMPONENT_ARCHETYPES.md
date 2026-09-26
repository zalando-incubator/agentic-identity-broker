# Component Aesthetic Archetypes

This guide documents the visual specifications for the four foundational components in the Refined Trust Architecture design system. These archetypes serve as the reference for all other component designs.

## Authority and example status

[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current direction and Principle XI process.
ADR 037 remains Proposed. These archetypes describe Refined Trust Architecture, not its proposed replacement.
The fixed colors, dimensions, shadows, and CSS blocks document existing visual examples, not constitutional aesthetic requirements.
Do not copy literal visual values or raw palette utilities into components. Express visual decisions through centralized semantic tokens.
Light and dark themes require accessible surfaces, not permanently white inputs or cards.
Self-host images and fonts. Verify every component story with the accessibility addon in both themes.
Align these archetypes with the implemented target in the single cutover after ADR acceptance.

---

## Primary Button

The most important visual element in the interface. Every interaction begins with the primary button, so it must communicate both action and trustworthiness.

### Visual Specifications

| Property              | Value                        | Notes                                     |
| --------------------- | ---------------------------- | ----------------------------------------- |
| **Background**        | Trust Deep (#0A2540)         | With subtle gradient overlay for depth    |
| **Text Color**        | Pure white (#ffffff)         | Maximum contrast and clarity              |
| **Font**              | Manrope Medium (500)         | Humanist sans-serif, weight 500           |
| **Height**            | 44px                         | Touch-friendly minimum for mobile         |
| **Padding**           | 12px 16px                    | Horizontal 16px, vertical 12px            |
| **Border Radius**     | 6px (md)                     | Subtle rounding, not pill-shaped          |
| **Shadow (default)**  | `0 2px 8px rgba(0,0,0,0.08)` | Subtle elevation                          |
| **Shadow (hover)**    | `0 8px 20px rgba(0,0,0,0.1)` | Increased elevation on hover              |
| **Transform (hover)** | `translateY(-1px)`           | Subtle lift on interaction                |
| **Transition**        | All properties 200ms         | Easing: cubic-bezier(0.34, 1.56, 0.64, 1) |

### States

#### Default State

```css
button {
  background: #0a2540; /* trust-deep */
  color: #ffffff;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  border-radius: 6px;
  padding: 12px 16px;
  font-family: 'Manrope', sans-serif;
  font-weight: 500;
  font-size: 1rem;
  cursor: pointer;
  border: none;
  transition: all 200ms cubic-bezier(0.34, 1.56, 0.64, 1);
}
```

#### Hover State

```css
button:hover {
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.1);
  transform: translateY(-1px);
}
```

#### Focus State

```css
button:focus {
  outline: 2px solid #1e4d6b;
  outline-offset: 2px;
}
```

#### Active State

```css
button:active {
  transform: translateY(0);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}
```

#### Disabled State

```css
button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  box-shadow: none;
}
```

#### Loading State

```css
button.is-loading {
  opacity: 0.85;
  pointer-events: none;
}

/* Spinner inside loading button should be white */
button.is-loading .spinner {
  color: #ffffff;
  animation: spin 1s linear infinite;
}
```

### Variants

| Variant       | Background              | Text    | Border            | Shadow           |
| ------------- | ----------------------- | ------- | ----------------- | ---------------- |
| **Primary**   | #0A2540 (trust-deep)    | white   | none              | md → lg on hover |
| **Secondary** | #f5f1ed (neutral-100)   | #0A2540 | 1px solid #e8e3de | sm → md on hover |
| **Outline**   | transparent             | #0A2540 | 2px solid #ddd8d1 | none             |
| **Ghost**     | transparent             | #0A2540 | none              | none             |
| **Danger**    | #DC2626 (error-primary) | white   | none              | md → lg on hover |

---

## Card Component

The workhorse of the UI. Cards organize content hierarchically and create visual separation between different sections of information.

### Visual Specifications

| Property             | Value                                                           | Notes                                       |
| -------------------- | --------------------------------------------------------------- | ------------------------------------------- |
| **Background**       | Pure white (#ffffff)                                            | With 10px blur backdrop-filter if supported |
| **Border**           | 1px solid rgba(240, 237, 232, 0.8)                              | Barely visible, containment only            |
| **Padding**          | 24px standard, 32px headers                                     | Generous whitespace                         |
| **Border Radius**    | 12px (xl)                                                       | Generous but not extreme                    |
| **Shadow (default)** | `0 2px 8px rgba(0,0,0,0.04), inset 0 1px 0 rgba(255,255,255,1)` | Subtle external + internal highlight        |
| **Shadow (hover)**   | `0 8px 20px rgba(0,0,0,0.1), inset 0 1px 0 rgba(255,255,255,1)` | Elevated on hover                           |
| **Transition**       | All properties 300ms                                            | Easing: cubic-bezier(0.34, 1.56, 0.64, 1)   |
| **Hover Transform**  | `translateY(-2px)`                                              | Subtle lift on hover                        |

### Base Styling

```css
.card {
  background: #ffffff;
  border: 1px solid rgba(240, 237, 232, 0.8);
  border-radius: 12px;
  padding: 24px;
  box-shadow:
    0 2px 8px rgba(0, 0, 0, 0.04),
    inset 0 1px 0 rgba(255, 255, 255, 1);
  backdrop-filter: blur(10px);
  transition: all 300ms cubic-bezier(0.34, 1.56, 0.64, 1);
}
```

### Interactive States

#### Hover State

```css
.card:hover {
  box-shadow:
    0 8px 20px rgba(0, 0, 0, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 1);
  transform: translateY(-2px);
}
```

#### Focus State (when interactive)

```css
.card:focus-within {
  outline: 2px solid #1e4d6b;
  outline-offset: 2px;
}
```

### Padding Variants

| Variant      | Padding | Use Case                     |
| ------------ | ------- | ---------------------------- |
| **Compact**  | 16px    | Dense lists, data tables     |
| **Default**  | 24px    | Standard content cards       |
| **Spacious** | 32px    | Important sections, emphasis |

### Card with Header

```tsx
<Card>
  <CardHeader padding="lg">
    <h2 className="text-2xl font-bold text-trust-deep">Card Title</h2>
  </CardHeader>
  <CardBody padding="lg">
    <p className="text-neutral-700">Card content here</p>
  </CardBody>
</Card>
```

### Card with Image

```tsx
<Card>
  <img
    src="image.jpg"
    alt="Card image"
    className="w-full h-48 object-cover rounded-t-xl"
  />
  <div className="p-6">
    <h3 className="text-lg font-semibold">Title</h3>
    <p className="text-neutral-600">Description</p>
  </div>
</Card>
```

---

## Form Input

Trust through clarity. Form inputs are where users enter sensitive data, so every detail must communicate confidence and clarity.

### Visual Specifications

| Property                | Value (Default)     | Value (Focus)                 | Value (Error)       | Notes                          |
| ----------------------- | ------------------- | ----------------------------- | ------------------- | ------------------------------ |
| **Border**              | 1.5px solid #ddd8d1 | 1.5px solid #1e4d6b           | 1.5px solid #dc2626 | Neutral-300 default            |
| **Height**              | 44px                | 44px                          | 44px                | Touch-friendly minimum         |
| **Padding**             | 12px 16px           | 12px 16px                     | 12px 16px           | Horizontal 16px, vertical 12px |
| **Background**          | #ffffff             | #ffffff                       | #ffffff             | Existing light appearance, not a dark-theme rule |
| **Border Radius**       | 6px (md)            | 6px (md)                      | 6px (md)            | Consistent with buttons        |
| **Focus Ring**          | none                | 2px solid #1e4d6b, 2px offset | none                | Clear, 2px offset              |
| **Disabled Background** | #f5f1ed             | —                             | —                   | Subtle background              |
| **Disabled Border**     | 1px solid #ddd8d1   | —                             | —                   | Lighter border                 |
| **Disabled Text**       | #9a9591             | —                             | —                   | Muted color                    |
| **Transition**          | —                   | All 150ms                     | —                   | Border and shadow              |

### Base Styling

```css
.input {
  height: 44px;
  padding: 12px 16px;
  border: 1.5px solid #ddd8d1; /* neutral-300 */
  border-radius: 6px;
  background: #ffffff;
  font-family: 'Manrope', sans-serif;
  font-size: 1rem;
  color: #0a2540; /* trust-deep */
  transition:
    border 150ms,
    box-shadow 150ms;
}

.input::placeholder {
  color: #c4bdb3; /* neutral-400 */
}
```

### States

#### Focus State

```css
.input:focus {
  outline: none;
  border-color: #1e4d6b;
  box-shadow: 0 0 0 2px #1e4d6b;
  box-shadow-offset: 2px;
}
```

#### Error State

```css
.input.is-error {
  border-color: #dc2626;
  box-shadow: 0 0 0 2px rgba(220, 38, 38, 0.1);
}

.input.is-error:focus {
  border-color: #dc2626;
  box-shadow: 0 0 0 2px #dc2626;
}
```

#### Disabled State

```css
.input:disabled {
  background: #f5f1ed;
  border-color: #ddd8d1;
  color: #9a9591;
  cursor: not-allowed;
}
```

### Label & Helper Text

```tsx
<div className="mb-4">
  <label
    htmlFor="email"
    className="block text-sm font-medium text-trust-deep mb-2"
  >
    Email Address
    <span className="text-error-primary">*</span>
  </label>
  <input
    id="email"
    type="email"
    className="w-full px-4 py-3 border border-neutral-300 rounded-md focus:border-trust focus:ring-2 focus:ring-trust"
    placeholder="you@example.com"
  />
  <p className="text-sm text-secondary mt-1">We'll never share your email</p>
</div>
```

### Error Message Display

```tsx
{
  errorMessage && (
    <div className="mt-2 flex items-center gap-2">
      <AlertCircleIcon className="w-4 h-4 text-error-primary" />
      <p className="text-sm text-error-primary">{errorMessage}</p>
    </div>
  );
}
```

---

## Modal

Command attention without aggression. Modals are critical for permission dialogs and important confirmations, so they must feel serious but not threatening.

### Visual Specifications

| Property                | Value                          | Notes                                     |
| ----------------------- | ------------------------------ | ----------------------------------------- |
| **Overlay Background**  | rgba(13, 24, 41, 0.5)          | Dark navy, 50% opacity                    |
| **Overlay Blur**        | 8px backdrop-blur              | Gaussian blur effect                      |
| **Modal Background**    | Pure white (#ffffff)           | Clean, elevated                           |
| **Modal Border Radius** | 16px (2xl)                     | Premium feel                              |
| **Modal Shadow**        | `0 20px 40px rgba(0,0,0,0.12)` | Strong separation                         |
| **Modal Padding**       | 24px standard, 32px spacious   | Generous whitespace                       |
| **Header Padding**      | 32px                           | Emphasis and hierarchy                    |
| **Overlay Animation**   | Fade in 200ms                  | Easing: ease-out                          |
| **Modal Animation**     | Slide up + scale 300ms         | Easing: cubic-bezier(0.34, 1.56, 0.64, 1) |
| **Animation Delay**     | 100ms                          | Stagger overlay and modal                 |

### Base Styling

```css
/* Overlay */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(13, 24, 41, 0.5);
  backdrop-filter: blur(8px);
  animation: fadeIn 200ms ease-out;
  z-index: 50;
}

/* Modal Body */
.modal {
  background: #ffffff;
  border-radius: 16px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.12);
  padding: 24px;
  max-width: 600px;
  max-height: 90vh;
  overflow-y: auto;
  animation: slideUpScale 300ms cubic-bezier(0.34, 1.56, 0.64, 1);
  animation-delay: 100ms;
}
```

### Animations

#### Overlay Fade In

```css
@keyframes fadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}
```

#### Modal Slide Up + Scale

```css
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

### Modal Sizes

| Size                 | Width  | Use Case                      |
| -------------------- | ------ | ----------------------------- |
| **Small (sm)**       | 400px  | Confirmations, simple dialogs |
| **Medium (md)**      | 600px  | Standard modals (default)     |
| **Large (lg)**       | 800px  | Forms, complex content        |
| **Extra Large (xl)** | 1000px | Full content modals           |
| **Full**             | 90vw   | Mobile-optimized, maximized   |

### Modal Structure

```tsx
<Modal isOpen={isOpen} onClose={onClose}>
  {/* Header - Optional */}
  <div className="mb-6 pb-6 border-b border-neutral-200">
    <h2 className="text-2xl font-bold text-trust-deep">Modal Title</h2>
  </div>

  {/* Body */}
  <div className="mb-8">
    <p className="text-neutral-700">Modal content here</p>
  </div>

  {/* Footer - Action Buttons */}
  <div className="flex justify-end gap-3">
    <Button variant="secondary" onClick={onClose}>
      Cancel
    </Button>
    <Button variant="primary" onClick={onConfirm}>
      Confirm
    </Button>
  </div>
</Modal>
```

### Close Button

```tsx
<button
  onClick={onClose}
  className="absolute top-6 right-6 w-11 h-11 flex items-center justify-center rounded-lg hover:bg-neutral-100 transition-colors"
  aria-label="Close dialog"
>
  <XIcon className="w-5 h-5 text-neutral-600" />
</button>
```

### Keyboard & Accessibility

- **ESC Key**: Close modal
- **Tab**: Focus trap within modal
- **Shift+Tab**: Reverse focus within modal
- **Enter**: Confirm (if applicable)
- **ARIA**: `role="dialog"`, `aria-labelledby="modal-title"`, `aria-modal="true"`

---

## Implementation Guidelines

### When to Use These Archetypes

1. **Primary Button**: Main call-to-action in any flow (submit, continue, confirm)
2. **Card**: Content grouping, service listings, permission displays
3. **Form Input**: User data entry (email, name, settings)
4. **Modal**: Critical confirmations, permission grants, destructive actions

### Existing visual conventions

- Primary buttons use trust-deep and shadow elevation in the current direction.
- Cards use an external shadow and an internal highlight.
- The input examples use a 44 px height. Exact height alone does not prove accessibility.
- Modal examples use a navy overlay and 8 px blur.

These descriptions are not constitutional aesthetic mandates.
Express each role through central tokens and verify accessibility in both themes.

### Customization

While these archetypes establish the foundation, variants exist for specific contexts:

- **Button variants**: secondary, outline, ghost, danger
- **Card padding**: compact, default, spacious
- **Input states**: default, focus, error, disabled
- **Modal sizes**: sm, md, lg, xl, full

Always maintain the core philosophy: **Trust through sophisticated simplicity.**

---

## Summary

These four component archetypes define the visual identity of the Refined Trust Architecture design system. Every other component is built upon these foundations, ensuring consistency and trustworthiness across the entire interface.

| Component          | Key Visual Feature                       | Emotional Signal                  |
| ------------------ | ---------------------------------------- | --------------------------------- |
| **Primary Button** | Trust-deep (#0A2540) + lifting animation | Authority with approachability    |
| **Card**           | Double shadow + white on warm neutrals   | Elevated content, premium quality |
| **Form Input**     | 44px + trust focus ring                  | Safety and touch-friendly         |
| **Modal**          | Trust-deep overlay + slide animation     | Important moment, clear focus     |
