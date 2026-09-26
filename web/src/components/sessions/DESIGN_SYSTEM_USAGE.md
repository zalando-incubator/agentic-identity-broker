# Design System Usage for OAuth2 Session Components

**Feature**: Third-Party OAuth2 Session Management
**Component Scope**: SessionCard, TerminationDialog, ThirdPartySessionsPage
**Design System Version**: Refined Trust Architecture

## Authority and example status

[DESIGN_PRINCIPLES.md](../../design-system/docs/DESIGN_PRINCIPLES.md) defines the current direction and Principle XI process.
ADR 037 remains Proposed. These session examples do not implement or approve its target.
The snippets retain older component patterns and data shapes. Verify actual component props and service types before reuse.
Fixed aesthetic values describe existing examples, not constitutional requirements.
Do not copy raw palette utilities or literal visual values into components. Use centralized semantic tokens.
Self-host fonts and images. Dynamic service logos must resolve to repository-hosted assets or a local fallback.
Every component story must pass the accessibility addon in light and dark themes.
Align this file and `README.md` with the implemented target in the single cutover after ADR acceptance.
Do not use redesign phases, feature flags, or compatibility paths.

---

## Component Mapping

This document maps design system components to OAuth2 session management requirements (US1-US3).

---

## 1. SessionCard Component (US1)

**Purpose**: Display individual third-party OAuth2 session information with status, metadata, and actions.

**Design System Components Used**:

### Primary Container: Card

**Import**: `@design-system/components/data-display/Card`

```tsx
import { Card } from '@design-system/components/data-display/Card';

<Card
  padding="default" // 24px padding (spacious feel)
  border="subtle" // Ring-based border
  hover="lift" // Elevation on hover (interactive feel)
  backgroundColor="white" // Default white background
  header={/* Service name + logo */}
  footer={/* Action buttons */}
  divider={true} // Divider between sections
>
  {/* Session metadata */}
</Card>;
```

**Props Used**:

- `padding="default"` (24px) - Standard spacing for session cards
- `border="subtle"` - Soft ring border for clean appearance
- `hover="lift"` - Subtle elevation feedback when hovering over card
- `header` - Service name (h3) + service logo (Avatar)
- `footer` - Action buttons (Terminate, View Details)
- `divider={true}` - Visual separation between header/body/footer

---

### Service Logo: Avatar

**Import**: `@design-system/components/primitives/Avatar`

```tsx
import { Avatar } from '@design-system/components/primitives/Avatar';

<Avatar
  src={service.logo_url}
  alt={`${service.display_name} logo`}
  size="md" // 40px size for service logos
  fallback={service.display_name[0]}
/>;
```

**Props Used**:

- `src` - Service logo URL from API
- `alt` - Accessible label for screen readers
- `size="md"` (40px) - Appropriate size for card headers
- `fallback` - First letter of service name if logo fails to load

---

### Status Badge: Badge

**Import**: `@design-system/components/primitives/Badge`

```tsx
import { Badge } from '@design-system/components/primitives/Badge';

// Active session
<Badge variant="success" size="sm" shape="pill">
  Active
</Badge>

// Expiring soon
<Badge variant="warning" size="sm" shape="pill" showDot>
  Expiring Soon
</Badge>

// Expired session
<Badge variant="error" size="sm" shape="pill">
  Expired
</Badge>

// No session
<Badge variant="neutral" size="sm" shape="pill">
  No Session
</Badge>
```

**Props Used**:

- `variant` - Semantic variant based on session status
  - `success` - Active, valid session
  - `warning` - Expiring within 7 days
  - `error` - Expired session
  - `neutral` - No session established
- `size="sm"` - Compact size for inline status
- `shape="pill"` - Rounded pill shape (consistent with design system)
- `showDot` - Optional dot indicator for visual emphasis (expiring state)

---

### Metadata Display: StatusIndicator

**Import**: `@design-system/components/data-display/StatusIndicator`

```tsx
import { StatusIndicator } from '@design-system/components/data-display/StatusIndicator';

// Encryption status
<StatusIndicator
  icon={<LockIcon />}
  label="Encrypted"
  variant="default"
/>

// Dependent agent count
<StatusIndicator
  icon={<UsersIcon />}
  label={`${agentCount} agent${agentCount !== 1 ? 's' : ''}`}
  variant="default"
/>

// Initiation timestamp
<StatusIndicator
  icon={<ClockIcon />}
  label={formatRelativeTime(initiated_at)}
  variant="default"
/>
```

**Props Used**:

- `icon` - Icon component (16px size, from Lucide React or similar)
- `label` - Text label for metadata
- `variant` - Color variant (default for neutral metadata)
- `interactive={true}` - Can be wrapped in Tooltip for additional context

---

### Action Buttons: Button

**Import**: `@design-system/components/primitives/Button`

```tsx
import { Button } from '@design-system/components/primitives/Button';

// Terminate session (destructive action)
<Button
  variant="danger"
  size="sm"
  onClick={handleTerminate}
  iconBefore={<TrashIcon />}
>
  Terminate
</Button>

// View details (secondary action)
<Button
  variant="outline"
  size="sm"
  onClick={handleViewDetails}
>
  View Details
</Button>

// Establish session (CTA)
<Button
  variant="primary"
  size="md"
  onClick={handleEstablish}
  iconBefore={<LinkIcon />}
>
  Establish Session
</Button>
```

**Props Used**:

- `variant` - Button variant based on action importance
  - `danger` - Destructive actions (terminate)
  - `outline` - Secondary actions (view details)
  - `primary` - Primary CTAs (establish session)
- `size` - Button size (`sm` for card footers, `md` for primary actions)
- `iconBefore` - Icon before button text
- `onClick` - Action handler
- `isLoading` - Loading state during async operations

---

### Layout: Stack

**Import**: `@design-system/components/layout/Stack`

```tsx
import { Stack } from '@design-system/components/layout/Stack';

// Vertical stack for metadata
<Stack gap="sm" direction="column">
  <StatusIndicator icon={<LockIcon />} label="Encrypted" />
  <StatusIndicator icon={<UsersIcon />} label="3 agents" />
  <StatusIndicator icon={<ClockIcon />} label="2 days ago" />
</Stack>

// Horizontal stack for action buttons
<Stack gap="sm" direction="row" justify="end">
  <Button variant="outline" size="sm">View Details</Button>
  <Button variant="danger" size="sm">Terminate</Button>
</Stack>
```

**Props Used**:

- `gap` - Spacing between items (`xs`, `sm`, `md`, `lg`)
- `direction` - Layout direction (`row`, `column`)
- `justify` - Horizontal alignment (`start`, `center`, `end`, `between`)
- `align` - Vertical alignment (`start`, `center`, `end`)

---

## 2. TerminationDialog Component (US3)

**Purpose**: Confirmation modal for terminating OAuth2 sessions with warning about affected agents.

**Design System Components Used**:

### Dialog Container: Modal

**Import**: `@design-system/components/overlays/Modal`

```tsx
import { Modal } from '@design-system/components/overlays/Modal';

<Modal
  isOpen={isTerminationDialogOpen}
  onClose={handleClose}
  title="Terminate OAuth2 Session"
  icon={<AlertTriangleIcon className="text-warning-primary" />}
  size="md" // 600px width for confirmation dialogs
  closeOnBackdropClick={false} // Prevent accidental dismissal
  footer={
    <div className="flex gap-3 justify-end">
      <Button variant="outline" onClick={handleClose}>
        Cancel
      </Button>
      <Button
        variant="danger"
        onClick={handleConfirmTerminate}
        isLoading={isTerminating}
      >
        Terminate Session
      </Button>
    </div>
  }
>
  <Stack gap="md" direction="column">
    <p className="text-neutral-700">
      Terminating this session will revoke access for the following agents:
    </p>
    <ul className="list-disc list-inside space-y-2 text-neutral-600">
      {affectedAgents.map((agent) => (
        <li key={agent.agent_id}>{agent.agent_name || agent.agent_id}</li>
      ))}
    </ul>
    <Alert variant="warning" size="sm">
      This action cannot be undone. Affected agents will need to be
      re-authorized.
    </Alert>
  </Stack>
</Modal>;
```

**Props Used**:

- `isOpen` - Boolean state controlling modal visibility
- `onClose` - Close handler (Cancel button or ESC key)
- `title` - Modal header title
- `icon` - Warning icon in header (AlertTriangle from Lucide)
- `size="md"` (600px) - Appropriate for confirmation dialogs
- `closeOnBackdropClick={false}` - Prevent accidental dismissal of destructive action
- `footer` - Cancel + Terminate buttons

---

### Warning Alert: Alert

**Import**: `@design-system/components/feedback/Alert`

```tsx
import { Alert } from '@design-system/components/feedback/Alert';

<Alert variant="warning" size="sm">
  This action cannot be undone. Affected agents will need to be re-authorized.
</Alert>;
```

**Props Used**:

- `variant="warning"` - Warning color scheme (amber background)
- `size="sm"` - Compact size for inline alerts

---

## 3. ThirdPartySessionsPage Component (US1)

**Purpose**: Page layout displaying list of OAuth2 sessions.

**Design System Components Used**:

### Page Container: Container

**Import**: `@design-system/components/layout/Container`

```tsx
import { Container } from '@design-system/components/layout/Container';

<Container maxWidth="4xl" className="py-8">
  {/* Page content */}
</Container>;
```

**Props Used**:

- `maxWidth="4xl"` (896px) - Maximum width for page content
- `className="py-8"` - Vertical padding (32px)

---

### Grid Layout: Grid

**Import**: `@design-system/components/layout/Grid`

```tsx
import { Grid } from '@design-system/components/layout/Grid';

<Grid cols={1} md={2} lg={3} gap="md">
  {sessions.map((session) => (
    <SessionCard key={session.service.id} session={session} />
  ))}
</Grid>;
```

**Props Used**:

- `cols={1}` - 1 column on mobile
- `md={2}` - 2 columns on tablet
- `lg={3}` - 3 columns on desktop
- `gap="md"` (16px) - Spacing between cards

---

### Empty State: EmptyState

**Import**: `@design-system/components/feedback/EmptyState`

```tsx
import { EmptyState } from '@design-system/components/feedback/EmptyState';

<EmptyState
  title="No Sessions Yet"
  description="You haven't established any third-party OAuth2 sessions. Establish a session to allow agents to access third-party services on your behalf."
  icon={<LinkIcon />}
  action={
    <Button variant="primary" onClick={handleEstablishSession}>
      Establish Session
    </Button>
  }
/>;
```

**Props Used**:

- `title` - Empty state heading
- `description` - Explanatory text
- `icon` - Icon illustration
- `action` - Optional CTA button

---

### Loading State: Skeleton

**Import**: `@design-system/components/feedback/Skeleton`

```tsx
import { Skeleton } from '@design-system/components/feedback/Skeleton';

<Card padding="default" border="subtle">
  <Stack gap="md" direction="column">
    <Skeleton variant="text" width="60%" />
    <Skeleton variant="text" width="40%" />
    <Skeleton variant="rectangular" height="80px" />
  </Stack>
</Card>;
```

**Props Used**:

- `variant` - Skeleton type (`text`, `circular`, `rectangular`)
- `width` - Width (percentage or px)
- `height` - Height (required for `rectangular`)

---

## Accessibility Considerations

The composed session views must meet WCAG 2.1 AA. The following items are verification requirements, not recorded test results:

### Keyboard Navigation

- Card: Focusable when interactive (`clickable` or `onClick`)
- Button: Full keyboard support (Tab, Enter, Space)
- Modal: Focus trap, ESC to close
- Badge: Semantic markup (no interactive role)

### Screen Readers

- Card: Role and semantic HTML (`<article>`, `<section>`)
- Button: Proper ARIA labels for icon-only buttons
- Modal: ARIA attributes (role="dialog", aria-labelledby, aria-describedby)
- StatusIndicator: Icon marked as `aria-hidden="true"`, text label read

### Color Contrast

- Verify text contrast of at least 4.5:1 in both themes
- Verify UI contrast of at least 3:1 in both themes
- Verify each rendered badge variant, including success, warning, and error states

### Motion

- All components respect `prefers-reduced-motion`
- Card hover animation disabled if user prefers reduced motion
- Modal entrance animation disabled if user prefers reduced motion

---

## Color Token Usage

Use [COLOR_GUIDE.md](../../design-system/docs/COLOR_GUIDE.md) for current semantic role tokens and generated utility names.
The following status colors describe existing usage. They do not authorize raw palette utilities in new code.

**Quick Reference**:

- **Active session**: `success-primary` (#059669)
- **Expiring session**: `warning-primary` (#D97706)
- **Expired session**: `error-primary` (#DC2626)
- **No session**: Existing examples use `neutral-300`. Use a semantic role and verify contrast instead of copying that shade.

---

## Implementation Checklist

### US1: View Sessions (T038-T041)

- [ ] SessionCard uses Card component with proper props
- [ ] Status indicators use Badge component with correct variants
- [ ] Metadata uses StatusIndicator with icons
- [ ] Actions use Button component with correct variants
- [ ] Layout uses Stack for vertical/horizontal spacing
- [ ] Page uses Container + Grid for responsive layout
- [ ] Empty state uses EmptyState component
- [ ] Loading state uses Skeleton components

### US3: Terminate Session (T071-T072)

- [ ] TerminationDialog uses Modal component
- [ ] Warning alert uses Alert component
- [ ] Affected agents list uses semantic HTML (<ul>)
- [ ] Footer buttons use correct variants (outline + danger)
- [ ] Modal prevents backdrop click dismissal

### Design System Compliance

- [ ] No custom CSS beyond design system tokens
- [ ] All components imported from `@design-system/`
- [ ] Semantic token usage consistent
- [ ] Accessibility requirements met (WCAG 2.1 AA)
- [ ] Animation system respected (200ms interactions, 300ms elevations)
- [ ] Spacing system followed (24px padding, 16px gaps)

---

## Example Code Snippets

### Complete SessionCard Example

```tsx
import { Card } from '@design-system/components/data-display/Card';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import { StatusIndicator } from '@design-system/components/data-display/StatusIndicator';
import { Stack } from '@design-system/components/layout/Stack';
import { LockIcon, UsersIcon, ClockIcon, TrashIcon } from 'lucide-react';

export function SessionCard({ session }) {
  return (
    <Card
      padding="default"
      border="subtle"
      hover="lift"
      header={
        <Stack direction="row" align="center" justify="between">
          <Stack direction="row" align="center" gap="sm">
            <Avatar
              src={session.service.logo_url}
              alt={`${session.service.display_name} logo`}
              size="md"
              fallback={session.service.display_name[0]}
            />
            <h3 className="font-semibold text-neutral-900">
              {session.service.display_name}
            </h3>
          </Stack>
          <Badge variant="success" size="sm" shape="pill">
            Active
          </Badge>
        </Stack>
      }
      footer={
        <Stack direction="row" gap="sm" justify="end">
          <Button variant="outline" size="sm">
            View Details
          </Button>
          <Button
            variant="danger"
            size="sm"
            iconBefore={<TrashIcon size={16} />}
          >
            Terminate
          </Button>
        </Stack>
      }
      divider
    >
      <Stack gap="sm" direction="column">
        <StatusIndicator
          icon={<LockIcon size={16} />}
          label="Encrypted"
          variant="default"
        />
        <StatusIndicator
          icon={<UsersIcon size={16} />}
          label={`${session.dependent_agents} agents`}
          variant="default"
        />
        <StatusIndicator
          icon={<ClockIcon size={16} />}
          label={formatRelativeTime(session.initiated_at)}
          variant="default"
        />
      </Stack>
    </Card>
  );
}
```

### Complete TerminationDialog Example

```tsx
import { Modal } from '@design-system/components/overlays/Modal';
import { Button } from '@design-system/components/primitives/Button';
import { Alert } from '@design-system/components/feedback/Alert';
import { Stack } from '@design-system/components/layout/Stack';
import { AlertTriangleIcon } from 'lucide-react';

export function TerminationDialog({ isOpen, onClose, session, onConfirm }) {
  const [isTerminating, setIsTerminating] = useState(false);

  const handleConfirm = async () => {
    setIsTerminating(true);
    try {
      await onConfirm();
      onClose();
    } catch (error) {
      // Error handling
    } finally {
      setIsTerminating(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Terminate OAuth2 Session"
      icon={<AlertTriangleIcon className="text-warning-primary" />}
      size="md"
      closeOnBackdropClick={false}
      footer={
        <div className="flex gap-3 justify-end">
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="danger"
            onClick={handleConfirm}
            isLoading={isTerminating}
          >
            Terminate Session
          </Button>
        </div>
      }
    >
      <Stack gap="md" direction="column">
        <p className="text-neutral-700">
          Terminating this session will revoke access for the following agents:
        </p>
        <ul className="list-disc list-inside space-y-2 text-neutral-600">
          {session.affected_agents.map((agent) => (
            <li key={agent.agent_id}>{agent.agent_name || agent.agent_id}</li>
          ))}
        </ul>
        <Alert variant="warning" size="sm">
          This action cannot be undone. Affected agents will need to be
          re-authorized.
        </Alert>
      </Stack>
    </Modal>
  );
}
```

---

## References

- Design System Index: `web/src/design-system/docs/INDEX.md`
- Component Archetypes: `web/src/design-system/docs/COMPONENT_ARCHETYPES.md`
- Composition Patterns: `web/src/design-system/docs/COMPOSITION_PATTERNS.md`
- Color Guide: `web/src/design-system/docs/COLOR_GUIDE.md`
- Accessibility Guide: `web/src/design-system/docs/ACCESSIBILITY_GUIDE.md`
- Existing Pattern: `web/src/components/consent/ServiceCard.tsx`
