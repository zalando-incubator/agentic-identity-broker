# Accessibility Guide

The Refined Trust Architecture design system is built with accessibility at its core. This guide explains WCAG 2.1 AA compliance and best practices for using the design system accessibly.

## Authority and evidence

[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current visual direction and Principle XI process.
ADR 037 remains Proposed. WCAG 2.2 AA is the proposed feature target, not a replacement for the binding WCAG 2.1 AA baseline.
Every component story must render and pass the accessibility addon in light and dark themes.
Use semantic tokens and self-hosted assets. Do not load fonts, scripts, or images from third-party origins.
Existing snippets illustrate patterns. They do not prove component, theme, or contrast compliance.
Verify actual keyboard behavior, accessible names, focus, reduced motion, and rendered contrast.
Align this guide with the implemented target in the single cutover after ADR acceptance.

## Overview

All design system components must meet **WCAG 2.1 Level AA** accessibility standards, which include:

- Perceivable: Information and UI are visible/perceivable to all
- Operable: All functionality accessible via keyboard
- Understandable: Text is clear and UI is intuitive
- Robust: Works with assistive technologies

## Core Accessibility Features

### Semantic HTML

Components use proper semantic HTML elements:

```tsx
// Button component renders actual <button>
<Button onClick={handleClick}>Click me</Button>
// → <button type="button">Click me</button>

// Link component renders actual <a>
<Link href="/page">Go to page</Link>
// → <a href="/page">Go to page</a>

// Navigation uses <nav>
<Breadcrumb items={items} />
// → <nav aria-label="Breadcrumb">...

// Headings maintain hierarchy
<h1>Main Title</h1>
<h2>Section</h2>
<h3>Subsection</h3>
```

### ARIA Attributes

Components include proper ARIA attributes:

```tsx
// Modal has proper ARIA
<Modal
  isOpen={true}
  title="Confirm Action"
>
  {/* Internally:
    role="dialog"
    aria-labelledby="modal-title"
    aria-modal="true"
  */}
</Modal>

// Accordion indicates state
<Accordion items={items} />
{/* Each item has:
  aria-expanded="true|false"
  aria-controls="panel-id"
*/}

// Table has proper structure
<Table columns={columns} data={data} />
{/* Uses <th>, <td>, proper headers, scope attributes */}

// Progress bar shows value
<Progress value={65} />
{/* Has:
  role="progressbar"
  aria-valuenow="65"
  aria-valuemin="0"
  aria-valuemax="100"
*/}
```

### Keyboard Navigation

All interactive components support full keyboard navigation:

```tsx
// Tab through buttons and links
<Button>Submit</Button>
<Button>Cancel</Button>
// Navigate with Tab/Shift+Tab

// Enter/Space to activate
// ESC to close dropdowns, modals
// Arrow keys in menus

// Focus always visible
// Proper focus styling on all interactive elements
```

### Color Contrast

Verify rendered colors in both themes against these requirements. The table records requirements, not measured results.

| Use Case | Minimum ratio | Requirement |
| --- | --- | --- |
| Text on background | 4.5:1 | Principle XI baseline |
| UI components | 3:1 | Principle XI baseline |
| Inactive controls | WCAG exception where applicable | Do not apply this exception to placeholder text |

## Component Accessibility

### Button Component

```tsx
// ✓ Good - Clear text label
<Button>Submit Form</Button>

// ✓ Good - Icon with aria-label
<Button icon={<DeleteIcon />} aria-label="Delete item" />

// ✓ Good - Disabled state is perceivable
<Button disabled>Submit</Button>

// ✗ Avoid - Icon-only without label
<Button icon={<DeleteIcon />} />

// ✗ Avoid - Unclear label
<Button>OK</Button>
```

### Link Component

```tsx
// ✓ Good - Descriptive link text
<Link href="/permissions">View all permissions</Link>

// ✓ Good - External link indicator
<Link href="https://example.com" external>
  External documentation
</Link>

// ✗ Avoid - "Click here" links
<Link href="/page">Click here</Link>

// ✗ Avoid - Vague link text
<Link href="/page">More</Link>
```

### Form Inputs

```tsx
// ✓ Good - Proper label association
<label htmlFor="email">Email address</label>
<TextInput id="email" type="email" />

// ✓ Good - Error messaging
<TextInput
  id="email"
  errorMessage="Invalid email format"
  aria-describedby="email-error"
/>

// ✓ Good - Required indicator
<TextInput
  id="name"
  label="Full Name"
  required
  aria-required="true"
/>

// ✗ Avoid - Missing label
<TextInput type="text" placeholder="Search..." />

// ✗ Avoid - Error color-only indication
<TextInput style={{ borderColor: 'red' }} />
```

### Modal Component

```tsx
// ✓ Good - Proper modal structure
<Modal isOpen={true} title="Confirm Deletion" onClose={handleClose}>
  <p>Are you sure?</p>
  <Button variant="danger">Delete</Button>
  <Button onClick={handleClose}>Cancel</Button>
</Modal>

// ✓ Features:
// - Focus trap (cannot tab out of modal)
// - ESC key closes modal
// - Dialog role and labeling
// - Backdrop prevents interaction behind modal
```

### Table Component

```tsx
// ✓ Good - Proper table semantics
<Table
  columns={[
    { key: 'name', header: 'Name' },
    { key: 'status', header: 'Status' },
  ]}
  data={data}
  caption="List of active permissions"
/>

// ✓ Includes:
// - <table>, <thead>, <tbody>, <tr>, <th>, <td>
// - Caption for context
// - Scope attributes on headers
// - Sortable indicators

// ✗ Avoid - HTML table structure as divs
<div className="table">
  <div className="row">
    <div>Data</div>
  </div>
</div>
```

### Accordion Component

```tsx
// ✓ Good - Semantic structure
<Accordion
  items={[
    {
      id: 'item1',
      title: 'What is a delegation?',
      content: <AnswerContent />,
    },
  ]}
/>

// ✓ Features:
// - Proper heading hierarchy
// - aria-expanded indicates state
// - aria-controls links header to content
// - Keyboard navigation (Arrow keys)
// - Enter/Space to toggle
```

## Testing for Accessibility

### Keyboard Navigation Testing

Test these keyboard interactions:

```
Tab          → Move focus forward
Shift+Tab    → Move focus backward
Enter/Space  → Activate buttons, checkboxes
Arrow Keys   → Navigate menus, tabs, sliders
ESC          → Close modals, dropdowns
Home/End     → Jump to start/end
```

Ensure:

- All interactive elements are reachable via keyboard
- Focus is always visible
- Tab order is logical (left-to-right, top-to-bottom)
- No keyboard traps (can always escape)

### Screen Reader Testing

Test with NVDA (Windows) or JAWS, or test using browser extensions:

```bash
# Browser DevTools accessibility tree
# Shows how screen readers see the page

# Key things to verify:
# - Page title/heading is clear
# - Navigation structure is logical
# - Form labels associated with inputs
# - Link text is descriptive
# - Dynamic content announcements
# - ARIA live regions working
```

### Contrast Testing

```bash
# Check color contrast with:
# - WebAIM Contrast Checker
# - Chrome DevTools > Elements > Accessibility
# - Stark plugin (Figma)
# - deque axe DevTools

# Minimum ratios:
# - Normal text: 4.5:1
# - Large text (18pt+): 3:1
# - UI components: 3:1
```

### Automated Testing

Run the Storybook accessibility addon for every component story in both themes.
The snippets that follow illustrate axe usage, not the configured test stack or evidence of passing checks.

```typescript
// Using axe-core in tests
import { axe } from 'jest-axe';

test('button is accessible', async () => {
  const { container } = render(<Button>Click me</Button>);
  const results = await axe(container);
  expect(results).toHaveNoViolations();
});

// Using Cypress accessibility plugin
cy.injectAxe();
cy.checkA11y();
```

## Accessibility Checklist

### Before Launch

- [ ] All page headings present and hierarchical
- [ ] Form labels associated with inputs (htmlFor)
- [ ] All buttons have text or aria-label
- [ ] Links have descriptive text (not "Click here")
- [ ] Color is not the only indicator
- [ ] Images have alt text
- [ ] Interactive elements keyboard accessible
- [ ] Focus indicators visible
- [ ] Page readable without CSS
- [ ] Text has sufficient contrast (4.5:1)
- [ ] Error messages clearly associated
- [ ] No keyboard traps
- [ ] ARIA used correctly (not overused)
- [ ] Page passes automated accessibility testing
- [ ] Every component story passes the accessibility addon in light and dark themes
- [ ] Contrast checks include rendered states, overlays, and focus indicators

### Ongoing

- [ ] Test with screen reader (monthly)
- [ ] Test keyboard navigation (monthly)
- [ ] Update alt text for new images
- [ ] Review ARIA usage in new components
- [ ] Check color contrast of new colors
- [ ] Test with actual users with disabilities
- [ ] Update accessibility documentation

## Common Issues & Fixes

### Issue: "Inputs don't have labels"

**Fix:**

```tsx
// ✗ Before
<TextInput placeholder="Email" />

// ✓ After
<label htmlFor="email">Email address</label>
<TextInput id="email" placeholder="you@example.com" />
```

### Issue: "Buttons don't have text"

**Fix:**

```tsx
// ✗ Before
<button className="icon-only"><TrashIcon /></button>

// ✓ After
<Button icon={<TrashIcon />} aria-label="Delete item" />
```

### Issue: "Images lack alt text"

**Fix:**

```tsx
// ✗ Before
<img src="profile.jpg" />

// ✓ After
<img src="profile.jpg" alt="Profile photo of John Doe" />
```

### Issue: "Links unclear"

**Fix:**

```tsx
// ✗ Before
<a href="/permissions">Click here to manage permissions</a>

// ✓ After
<a href="/permissions">Manage your permissions</a>
```

### Issue: "Focus not visible"

**Fix:**

```tsx
// ✗ Before (no focus indicator)
button { outline: none; }

// ✓ After
button:focus {
  outline: 2px solid navy-700;
  outline-offset: 2px;
}
```

## Responsive Accessibility

### Mobile Accessibility

```tsx
// ✓ Good - Touch targets large enough
<Button size="md" />     // At least 44x44px

// ✓ Good - Responsive font sizes
<p className="text-base md:text-lg" />

// ✗ Avoid - Too small touch targets
<button className="w-6 h-6" />

// ✗ Avoid - Unreadable on mobile
<p className="text-xs" />
```

### Reduced Motion

```tsx
// ✓ Good - Respects prefers-reduced-motion
<div className="motion-safe:animate-in motion-reduce:animate-none">
  {children}
</div>

// Tailwind automatically handles this
// Components with animations include motion-safe/motion-reduce
```

## Documentation & Labeling

### Page Structure

```tsx
// ✓ Good - Clear page structure
<main>
  <h1>Page Title</h1>
  <nav aria-label="main">...</nav>
  <article>
    <h2>Section</h2>
    <p>Content</p>
  </article>
</main>

// ✓ Good - Meaningful page title
<head>
  <title>Delegations - Manage your OAuth grants</title>
</head>
```

### ARIA Live Regions

```tsx
// ✓ Good - Dynamic content announcements
<div aria-live="polite" aria-atomic="true">
  {successMessage && <p>{successMessage}</p>}
</div>

// ✓ Good - Loading states
<div aria-live="polite">
  {loading && "Loading..."}
</div>
```

## Resources

### External References

- [WCAG 2.1 Guidelines](https://www.w3.org/WAI/WCAG21/quickref/)
- [WAI-ARIA Authoring Practices](https://www.w3.org/WAI/ARIA/apg/)
- [WebAIM](https://webaim.org/)
- [Deque axe DevTools](https://www.deque.com/axe/devtools/)

### Tools

- Chrome DevTools Accessibility Inspector
- NVDA Screen Reader (free)
- JAWS Screen Reader
- ColorSnack for contrast checking
- Lighthouse accessibility audit

## Summary

Accessibility requires implementation and verification for every component:

- **Semantic HTML** ensures proper structure
- **ARIA attributes** provide context to assistive tech
- **Keyboard navigation** works without mouse
- **Color contrast** must meet WCAG AA requirements
- **Focus indicators** must remain visible
- **Testing** must cover real user journeys in both themes

Component reuse does not prove accessibility. Verify each composed surface and retain the results.
