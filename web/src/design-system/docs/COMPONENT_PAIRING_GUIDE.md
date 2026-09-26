# Component Pairing Guide

These existing examples show component composition, spacing, and hierarchy for Refined Trust Architecture.

## Authority and example status

[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current direction and Principle XI process.
ADR 037 remains Proposed. These examples are not the proposed target or constitutional aesthetic requirements.
Raw palette classes and literal visual values in snippets are historical examples, not new-code instructions.
Use actual semantic role tokens and component APIs. Do not copy missing APIs or abbreviated utility names from snippets.
Self-host images and fonts. Dynamic image sources must resolve to repository-hosted assets or a local fallback.
Every component story must pass the accessibility addon in light and dark themes.
Align these examples with the implemented target in the single cutover after ADR acceptance.

---

## Button + Card Combinations

### Pattern 1: Card with Primary Action

**Use Case**: Single main action per card (grant permission, view details)

```tsx
<Card padding="default" hover="lift" className="max-w-md">
  <div className="space-y-4">
    {/* Card Header */}
    <div className="flex items-start justify-between">
      <div>
        <h3 className="text-lg font-semibold text-trust-deep">
          Healthcare Provider Access
        </h3>
        <p className="text-sm text-secondary mt-1">
          Dr. Sarah Chen, Primary Care
        </p>
      </div>
      <Badge variant="success">Active</Badge>
    </div>

    {/* Card Body */}
    <p className="text-neutral-700">
      Grants read access to medical records and treatment history for
      coordinated care between providers.
    </p>

    {/* Metadata */}
    <div className="flex items-center justify-between text-xs text-tertiary">
      <span>Granted: Jan 15, 2024</span>
      <span>Expires: Jan 15, 2025</span>
    </div>

    {/* Single Primary Action */}
    <Button variant="primary" className="w-full">
      View Permission Details
    </Button>
  </div>
</Card>
```

**Spacing Breakdown**:

- Card padding: 24px (`p-6`)
- Content vertical spacing: 16px (`space-y-4`)
- Heading to subtitle: 4px (`mt-1`)
- Button takes full width for emphasis

---

### Pattern 2: Card with Multiple Actions

**Use Case**: Primary + secondary actions (approve/deny, edit/delete)

```tsx
<Card padding="default" className="max-w-md">
  <div className="space-y-4">
    <div>
      <h3 className="text-lg font-semibold text-trust-deep">
        Agent Permission Request
      </h3>
      <p className="text-sm text-secondary mt-1">
        Research Assistant • Requested 2 hours ago
      </p>
    </div>

    <div className="p-3 bg-warning-light border border-warning-primary/20 rounded-md">
      <p className="text-sm text-warning-dark">
        This agent is requesting access to your calendar and email.
      </p>
    </div>

    {/* Action Button Group */}
    <div className="flex gap-3">
      <Button variant="primary" className="flex-1">
        Approve Access
      </Button>
      <Button variant="danger" className="flex-1">
        Deny Request
      </Button>
    </div>

    <Button variant="ghost" className="w-full">
      Review Permissions
    </Button>
  </div>
</Card>
```

**Spacing Breakdown**:

- Primary actions side-by-side: 12px gap (`gap-3`)
- Equal width for equal importance (`flex-1`)
- Tertiary action below with full width
- Warning callout: 12px padding (`p-3`)

---

### Pattern 3: Card Grid with Consistent Actions

**Use Case**: Multiple cards with identical action patterns

```tsx
<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
  {services.map((service) => (
    <Card key={service.id} padding="default" hover="lift">
      <div className="flex flex-col h-full">
        {/* Fixed-height header */}
        <div className="mb-4">
          <div className="flex items-center gap-3 mb-2">
            <Avatar src={service.icon} size="sm" />
            <h3 className="text-base font-semibold text-trust-deep">
              {service.name}
            </h3>
          </div>
          <p className="text-sm text-secondary line-clamp-2">
            {service.description}
          </p>
        </div>

        {/* Flexible body */}
        <div className="flex-1 space-y-2 mb-4">
          {service.scopes.slice(0, 3).map((scope) => (
            <div key={scope} className="flex items-center gap-2 text-xs">
              <CheckIcon className="w-3 h-3 text-success-primary" />
              <span className="text-neutral-700">{scope}</span>
            </div>
          ))}
        </div>

        {/* Fixed-position actions */}
        <div className="flex gap-2">
          <Button variant="outline" size="sm" className="flex-1">
            Configure
          </Button>
          <Button variant="primary" size="sm" className="flex-1">
            Connect
          </Button>
        </div>
      </div>
    </Card>
  ))}
</div>
```

**Layout Strategy**:

- Grid gap: 24px (`gap-6`) for breathing room
- Flex column ensures buttons stay at bottom
- Fixed header, flexible body, fixed footer pattern
- Consistent button sizes across all cards

---

## Typography Hierarchy in Cards

### Pattern: Information Card with Perfect Hierarchy

```tsx
<Card padding="spacious" className="max-w-2xl">
  {/* Primary Heading */}
  <h1 className="text-3xl font-bold text-trust-deep mb-2">
    Delegation Management
  </h1>

  {/* Subtitle */}
  <p className="text-lg text-secondary mb-6">
    Control which AI agents can access your data and perform actions on your
    behalf
  </p>

  {/* Section Heading */}
  <h2 className="text-xl font-semibold text-trust mb-4">Active Delegations</h2>

  {/* List of Items */}
  <div className="space-y-3 mb-6">
    {delegations.map((delegation) => (
      <div
        key={delegation.id}
        className="flex items-start gap-3 p-3 rounded-lg hover:bg-neutral-50"
      >
        {/* Icon */}
        <div className="w-10 h-10 bg-success-light rounded-lg flex items-center justify-center flex-shrink-0">
          <CheckIcon className="w-5 h-5 text-success-primary" />
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0">
          <h3 className="text-base font-medium text-trust-deep">
            {delegation.agentName}
          </h3>
          <p className="text-sm text-neutral-700 mt-0.5">
            {delegation.description}
          </p>
          <p className="text-xs text-tertiary mt-1">
            Granted {delegation.grantedDate} • {delegation.scopeCount}{' '}
            permissions
          </p>
        </div>

        {/* Status */}
        <Badge variant="success" size="sm">
          Active
        </Badge>
      </div>
    ))}
  </div>

  {/* Helper Text */}
  <p className="text-sm text-tertiary border-t border-neutral-200 pt-4">
    You can revoke access at any time from the delegation details page.
  </p>
</Card>
```

**Hierarchy Breakdown**:

1. **h1** (48px, trust-deep) - Page title, maximum visual weight
2. **Subtitle** (18px, secondary) - Context and purpose
3. **h2** (20px, trust) - Section divider
4. **h3** (16px, trust-deep) - Item titles
5. **Body text** (14px, neutral-700) - Descriptions
6. **Metadata** (12px, tertiary) - Timestamps, IDs
7. **Helper text** (14px, tertiary) - Footer notes

**Spacing Breakdown**:

- Title to subtitle: 8px (`mb-2`)
- Subtitle to section: 24px (`mb-6`)
- Section to content: 16px (`mb-4`)
- List item spacing: 12px (`space-y-3`)
- Footer separator: 16px padding (`pt-4`)

---

## Form Error States with Icon + Message

### Pattern: Input with Validation

```tsx
<div className="mb-4">
  {/* Label */}
  <label
    htmlFor="email"
    className="block text-sm font-medium text-trust-deep mb-2"
  >
    Email Address
    <span className="text-error-primary">*</span>
  </label>

  {/* Input */}
  <TextInput
    id="email"
    type="email"
    value={email}
    onChange={setEmail}
    error={!!error}
    placeholder="you@example.com"
    className="w-full"
  />

  {/* Error Message */}
  {error && (
    <div className="mt-2 flex items-start gap-2">
      <AlertCircleIcon className="w-4 h-4 text-error-primary flex-shrink-0 mt-0.5" />
      <p className="text-sm text-error-primary">{error}</p>
    </div>
  )}

  {/* Helper Text (when no error) */}
  {!error && (
    <p className="text-sm text-tertiary mt-1">
      We'll send a verification link to this address
    </p>
  )}
</div>
```

**Spacing Breakdown**:

- Label to input: 8px (`mb-2`)
- Input to error: 8px (`mt-2`)
- Input to helper: 4px (`mt-1`)
- Icon to text: 8px (`gap-2`)
- Icon vertical align: 2px offset (`mt-0.5`)

---

### Pattern: Form with Multiple Fields and Error Summary

```tsx
<form className="space-y-6">
  {/* Error Summary */}
  {errors.length > 0 && (
    <Alert variant="error">
      <div className="flex items-start gap-3">
        <AlertCircleIcon className="w-5 h-5 flex-shrink-0" />
        <div>
          <p className="font-medium mb-1">
            Please correct the following errors:
          </p>
          <ul className="list-disc list-inside space-y-1 text-sm">
            {errors.map((error, index) => (
              <li key={index}>{error}</li>
            ))}
          </ul>
        </div>
      </div>
    </Alert>
  )}

  {/* Form Fields */}
  <div className="space-y-4">
    <TextInput
      label="Agent Name"
      required
      error={fieldErrors.name}
      helperText="A descriptive name for this AI agent"
    />

    <TextArea
      label="Description"
      required
      error={fieldErrors.description}
      helperText="Explain what this agent will do (min 20 characters)"
      rows={4}
    />

    <Select
      label="Access Level"
      required
      error={fieldErrors.accessLevel}
      options={accessLevels}
    />
  </div>

  {/* Actions */}
  <div className="flex gap-3 pt-4 border-t border-neutral-200">
    <Button variant="ghost" onClick={onCancel} className="flex-1">
      Cancel
    </Button>
    <Button variant="primary" type="submit" className="flex-1">
      Create Agent
    </Button>
  </div>
</form>
```

**Spacing Breakdown**:

- Form sections: 24px (`space-y-6`)
- Form fields: 16px (`space-y-4`)
- Action separator: 16px padding top (`pt-4`)
- Error list items: 4px (`space-y-1`)

---

## Button Groups

### Pattern: Segmented Control (Equal Actions)

```tsx
<div className="inline-flex rounded-lg border border-neutral-300 p-1 bg-neutral-50">
  <button
    className={cn(
      'px-4 py-2 text-sm font-medium rounded-md transition-colors',
      active === 'grid'
        ? 'bg-white text-trust-deep shadow-sm'
        : 'text-neutral-700 hover:text-trust-deep',
    )}
    onClick={() => setActive('grid')}
  >
    <GridIcon className="w-4 h-4 inline mr-2" />
    Grid View
  </button>
  <button
    className={cn(
      'px-4 py-2 text-sm font-medium rounded-md transition-colors',
      active === 'list'
        ? 'bg-white text-trust-deep shadow-sm'
        : 'text-neutral-700 hover:text-trust-deep',
    )}
    onClick={() => setActive('list')}
  >
    <ListIcon className="w-4 h-4 inline mr-2" />
    List View
  </button>
</div>
```

**Styling Details**:

- Container padding: 4px (`p-1`)
- Button padding: 16px/8px (`px-4 py-2`)
- Icon spacing: 8px (`mr-2`)
- Active state: white bg + shadow
- Inactive state: transparent + hover

---

### Pattern: Action Group (Primary + Secondary)

```tsx
<div className="flex flex-wrap gap-3">
  {/* Primary Action */}
  <Button variant="primary" size="md">
    <SaveIcon className="w-5 h-5 mr-2" />
    Save Changes
  </Button>

  {/* Secondary Actions */}
  <Button variant="outline" size="md">
    <PreviewIcon className="w-5 h-5 mr-2" />
    Preview
  </Button>

  <Button variant="ghost" size="md">
    <HistoryIcon className="w-5 h-5 mr-2" />
    View History
  </Button>

  {/* Destructive Action (separated) */}
  <div className="ml-auto">
    <Button variant="danger" size="md">
      <TrashIcon className="w-5 h-5 mr-2" />
      Delete
    </Button>
  </div>
</div>
```

**Layout Strategy**:

- Gap between buttons: 12px (`gap-3`)
- Icon to text: 8px (`mr-2`)
- Danger button pushed right (`ml-auto`)
- Wraps on small screens (`flex-wrap`)

---

## Empty States with CTAs

### Pattern: Empty State with Action

```tsx
<div className="flex flex-col items-center justify-center py-16 px-6 text-center">
  {/* Illustration */}
  <div className="w-20 h-20 bg-neutral-100 rounded-full flex items-center justify-center mb-6">
    <InboxIcon className="w-10 h-10 text-neutral-400" />
  </div>

  {/* Heading */}
  <h3 className="text-xl font-semibold text-trust-deep mb-2">
    No active permissions
  </h3>

  {/* Description */}
  <p className="text-neutral-700 max-w-sm mb-6">
    You haven't granted any permissions yet. Start by connecting an AI agent to
    access your data securely.
  </p>

  {/* Primary CTA */}
  <Button variant="primary" size="lg">
    <PlusIcon className="w-5 h-5 mr-2" />
    Grant First Permission
  </Button>

  {/* Secondary Action */}
  <button className="mt-4 text-sm text-trust hover:text-trust-hover transition-colors">
    Learn about agent permissions →
  </button>
</div>
```

**Spacing Breakdown**:

- Container padding: 64px/24px (`py-16 px-6`)
- Icon to heading: 24px (`mb-6`)
- Heading to description: 8px (`mb-2`)
- Description to CTA: 24px (`mb-6`)
- Primary to secondary action: 16px (`mt-4`)
- Icon in button: 8px (`mr-2`)

---

## Status Indicators with Context

### Pattern: Status Badge + Timestamp + Action

```tsx
<div className="flex items-center justify-between p-4 bg-neutral-50 rounded-lg border border-neutral-200">
  {/* Left: Status + Info */}
  <div className="flex items-center gap-4">
    <Badge variant="success">Active</Badge>
    <div>
      <p className="text-sm font-medium text-trust-deep">Calendar Access</p>
      <p className="text-xs text-tertiary mt-0.5">Last used 2 hours ago</p>
    </div>
  </div>

  {/* Right: Action */}
  <Button variant="ghost" size="sm">
    Revoke
  </Button>
</div>
```

**Spacing Breakdown**:

- Container padding: 16px (`p-4`)
- Badge to text: 16px (`gap-4`)
- Text to timestamp: 2px (`mt-0.5`)

---

### Pattern: Multi-Status Timeline

```tsx
<div className="space-y-4">
  {events.map((event, index) => (
    <div key={event.id} className="relative pl-8">
      {/* Timeline Line */}
      {index < events.length - 1 && (
        <div className="absolute left-2 top-8 bottom-0 w-px bg-neutral-200" />
      )}

      {/* Status Dot */}
      <div
        className={cn(
          'absolute left-0 top-1 w-4 h-4 rounded-full border-2',
          event.status === 'success' &&
            'bg-success-primary border-success-light',
          event.status === 'pending' &&
            'bg-warning-primary border-warning-light',
          event.status === 'error' && 'bg-error-primary border-error-light',
        )}
      />

      {/* Content */}
      <div>
        <div className="flex items-center gap-2 mb-1">
          <span className="text-sm font-medium text-trust-deep">
            {event.title}
          </span>
          <Badge variant={event.status} size="sm">
            {event.statusLabel}
          </Badge>
        </div>
        <p className="text-sm text-neutral-700 mb-1">{event.description}</p>
        <p className="text-xs text-tertiary">{event.timestamp}</p>
      </div>
    </div>
  ))}
</div>
```

**Layout Details**:

- Timeline spacing: 16px (`space-y-4`)
- Content left padding: 32px (`pl-8`)
- Dot position: 8px left, 4px top
- Line position: 8px left, connects dots
- Title to badge: 8px (`gap-2`)

---

## Modal Content Patterns

### Pattern: Confirmation Modal

```tsx
<Modal isOpen={isOpen} onClose={onClose} size="sm">
  <div className="text-center">
    {/* Icon */}
    <div className="w-12 h-12 bg-error-light rounded-full flex items-center justify-center mx-auto mb-4">
      <AlertTriangleIcon className="w-6 h-6 text-error-primary" />
    </div>

    {/* Title */}
    <h2 className="text-xl font-bold text-trust-deep mb-2">
      Revoke Permission?
    </h2>

    {/* Description */}
    <p className="text-neutral-700 mb-6">
      This will immediately remove the agent's access to your calendar. This
      action cannot be undone.
    </p>

    {/* Actions */}
    <div className="flex gap-3">
      <Button variant="ghost" onClick={onClose} className="flex-1">
        Cancel
      </Button>
      <Button variant="danger" onClick={onConfirm} className="flex-1">
        Revoke Access
      </Button>
    </div>
  </div>
</Modal>
```

**Spacing Breakdown**:

- Icon to title: 16px (`mb-4`)
- Title to description: 8px (`mb-2`)
- Description to actions: 24px (`mb-6`)
- Action gap: 12px (`gap-3`)

---

### Pattern: Form Modal

```tsx
<Modal
  isOpen={isOpen}
  onClose={onClose}
  size="md"
  title="Edit Permission Scope"
>
  <form onSubmit={handleSubmit} className="space-y-6">
    {/* Section 1 */}
    <div>
      <h3 className="text-sm font-medium text-trust-deep mb-3">Access Level</h3>
      <RadioGroup value={accessLevel} onChange={setAccessLevel}>
        <div className="space-y-2">
          {accessLevels.map((level) => (
            <Radio key={level.value} value={level.value}>
              <div>
                <span className="font-medium">{level.label}</span>
                <span className="text-sm text-secondary block">
                  {level.description}
                </span>
              </div>
            </Radio>
          ))}
        </div>
      </RadioGroup>
    </div>

    {/* Section 2 */}
    <div>
      <h3 className="text-sm font-medium text-trust-deep mb-3">Expiration</h3>
      <DatePicker value={expirationDate} onChange={setExpirationDate} />
    </div>

    {/* Actions in Footer */}
    <div className="flex gap-3 pt-6 border-t border-neutral-200">
      <Button variant="ghost" onClick={onClose} className="flex-1">
        Cancel
      </Button>
      <Button variant="primary" type="submit" className="flex-1">
        Save Changes
      </Button>
    </div>
  </form>
</Modal>
```

**Spacing Breakdown**:

- Form sections: 24px (`space-y-6`)
- Section heading to content: 12px (`mb-3`)
- Radio options: 8px (`space-y-2`)
- Footer separator: 24px padding top (`pt-6`)

---

## Summary

### Spacing Cheat Sheet

| Context        | Gap/Margin  | Tailwind | Pixels |
| -------------- | ----------- | -------- | ------ |
| Label → Input  | `mb-2`      | 0.5rem   | 8px    |
| Input → Helper | `mt-1`      | 0.25rem  | 4px    |
| Form Fields    | `space-y-4` | 1rem     | 16px   |
| Form Sections  | `space-y-6` | 1.5rem   | 24px   |
| Card Content   | `space-y-4` | 1rem     | 16px   |
| Button Group   | `gap-3`     | 0.75rem  | 12px   |
| Grid Cards     | `gap-6`     | 1.5rem   | 24px   |
| List Items     | `space-y-3` | 0.75rem  | 12px   |
| Major Sections | `gap-16`    | 4rem     | 64px   |

### Icon Sizing Cheat Sheet

| Context            | Class       | Pixels |
| ------------------ | ----------- | ------ |
| Small button       | `w-4 h-4`   | 16px   |
| Medium button      | `w-5 h-5`   | 20px   |
| Large button       | `w-6 h-6`   | 24px   |
| Badge/Tag          | `w-3 h-3`   | 12px   |
| Content decoration | `w-5 h-5`   | 20px   |
| Empty state        | `w-16 h-16` | 64px   |

### Typography Hierarchy Cheat Sheet

| Element         | Size        | Weight          | Color              | Spacing |
| --------------- | ----------- | --------------- | ------------------ | ------- |
| Page title (h1) | `text-3xl`  | `font-bold`     | `text-trust-deep`  | `mb-2`  |
| Section (h2)    | `text-xl`   | `font-semibold` | `text-trust-deep`  | `mb-4`  |
| Subsection (h3) | `text-lg`   | `font-semibold` | `text-trust`       | `mb-3`  |
| Body text       | `text-base` | `font-normal`   | `text-neutral-700` | —       |
| Secondary text  | `text-sm`   | `font-normal`   | `text-secondary`   | —       |
| Metadata        | `text-xs`   | `font-normal`   | `text-tertiary`    | —       |

Use these patterns as starting points and adapt to your specific use case while maintaining the design system's visual consistency.
