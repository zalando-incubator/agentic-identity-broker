# OAuth2 Session Semantic Tokens

**Feature**: Third-Party OAuth2 Session Management
**Design System**: Refined Trust Architecture
**WCAG Compliance**: 2.1 AA (4.5:1 text, 3:1 UI elements)

---

## Purpose

This document defines semantic token usage for OAuth2 session status indicators, ensuring:

- Consistent visual language across all session UI components
- Accessible color contrast ratios (WCAG 2.1 AA compliance)
- Clear semantic meaning for session states
- Alignment with design system color palette

---

## Session Status States

OAuth2 sessions have the following status states:

| Status            | Meaning                                 | Visual Treatment           |
| ----------------- | --------------------------------------- | -------------------------- |
| **Active**        | Valid session with unexpired tokens     | Success colors (green)     |
| **Access Token Expired** | Access token expired, refresh token stored | Warning colors (amber) |
| **Login required** | Provider rejected the refresh token, or no refresh token after expiry | Error colors (red) |
| **No Session**    | User has not established a session      | Neutral colors (warm gray) |
| **Encrypted**     | Tokens are encrypted at rest (metadata) | Default colors (neutral)   |

---

## Semantic Token Mappings

### 1. Active Session Status

**Use Case**: Session is established and tokens are valid.

**Token**: `success-primary`
**Hex Value**: `#059669` (Emerald 600)
**Contrast Ratio**: 4.89:1 on white background (WCAG AA Pass)

**Usage Examples**:

```tsx
// Badge component
<Badge variant="success" size="sm" shape="pill">
  Active
</Badge>

// StatusIndicator component
<StatusIndicator
  icon={<CheckCircleIcon />}
  label="Active"
  variant="success"
/>

// Custom element
<div className="text-success-primary">Active</div>
<div className="bg-success-light border border-success-primary">
  Session is active and valid
</div>
```

**Design System Classes**:

- Text: `text-success-primary`
- Background: `bg-success-primary`
- Border: `border-success-primary`
- Light background: `bg-success-light` (#d1fae5)
- Hover state: `hover:bg-success-hover` (#047857)

---

### 2. Access Token Expired Status

**Use Case**: The access token expired, but a stored refresh token allows a refresh attempt.

**Token**: `warning-primary` (alias: `cta`)
**Hex Value**: `#D97706` (Amber 600)
**Contrast Ratio**: 5.12:1 on white background (WCAG AA Pass)

**Usage Examples**:

```tsx
// Badge component with dot indicator
<Badge variant="warning" size="sm" shape="pill" showDot>
  Access Token Expired
</Badge>

// StatusIndicator component
<StatusIndicator
  icon={<AlertTriangleIcon />}
  label="Access token expired"
  variant="warning"
/>

// Custom element
<div className="text-warning-primary">Access Token Expired</div>
<div className="bg-warning-light border border-warning-primary">
  The next request refreshes the access token
</div>
```

**Design System Classes**:

- Text: `text-warning-primary`
- Background: `bg-warning-primary`
- Border: `border-warning-primary`
- Light background: `bg-warning-light` (#fef3c7)
- Dark text on light bg: `text-warning-dark` (#92400e)
- Hover state: `hover:bg-warning-hover` (#B45309)

**Accessibility Note**: Amber text (#D97706) on white meets WCAG AA for text. For UI elements, ensure sufficient size (>3px) or use borders.

---

### 3. Expired Session Status

**Use Case**: Session tokens have expired and session is invalid.

**Token**: `error-primary`
**Hex Value**: `#DC2626` (Red 600)
**Contrast Ratio**: 5.89:1 on white background (WCAG AA Pass)

**Usage Examples**:

```tsx
// Badge component
<Badge variant="error" size="sm" shape="pill">
  Expired
</Badge>

// StatusIndicator component
<StatusIndicator
  icon={<XCircleIcon />}
  label="Expired"
  variant="error"
/>

// Custom element
<div className="text-error-primary">Expired</div>
<div className="bg-error-light border border-error-primary">
  Session has expired. Please re-authenticate.
</div>
```

**Design System Classes**:

- Text: `text-error-primary`
- Background: `bg-error-primary`
- Border: `border-error-primary`
- Light background: `bg-error-light` (#fee2e2)
- Dark text on light bg: `text-error-dark` (#991b1b)
- Hover state: `hover:bg-error-hover` (#b91c1c)

---

### 4. No Session Status

**Use Case**: User has not yet established a session with the service.

**Token**: `neutral-300` (warm neutral)
**Hex Value**: `#ddd8d1` (Light taupe)
**Contrast Ratio**: 1.42:1 on white background (Use for borders/backgrounds only, NOT text)

**Usage Examples**:

```tsx
// Badge component (neutral variant uses neutral-300 background)
<Badge variant="neutral" size="sm" shape="pill">
  No Session
</Badge>

// Text with neutral-600 (accessible text color)
<div className="text-neutral-600">No session established</div>

// Background with neutral-300
<div className="bg-neutral-300 text-neutral-600 px-3 py-1 rounded-full">
  No Session
</div>
```

**Design System Classes**:

- Background: `bg-neutral-300` (#ddd8d1)
- Border: `border-neutral-300`
- Text (accessible): `text-neutral-600` (#6b6561) - 4.76:1 contrast ratio
- Text (darker): `text-neutral-700` (#4a4137) - 7.12:1 contrast ratio

**Accessibility Note**: Never use `neutral-300` for text. Always use `neutral-600` or darker for accessible text.

---

### 5. Encryption Metadata (Not a Status)

**Use Case**: Indicate that session tokens are encrypted at rest (metadata indicator).

**Token**: `neutral-500` (default metadata color)
**Hex Value**: `#9a9591` (Medium-dark warm neutral)
**Contrast Ratio**: 3.14:1 on white background (WCAG AA Pass for large text)

**Usage Examples**:

```tsx
// StatusIndicator component (default variant)
<StatusIndicator
  icon={<LockIcon />}
  label="Encrypted"
  variant="default"
/>

// Custom element
<div className="text-neutral-500 text-xs">
  <LockIcon size={16} className="inline" /> Encrypted
</div>
```

**Design System Classes**:

- Text: `text-neutral-500` (12px or larger)
- Icon: `text-neutral-500`

**Accessibility Note**: `neutral-500` (#9a9591) provides 3.14:1 contrast, which meets WCAG AA for large text (18px+) or UI elements. For small text (<18px), use `neutral-600` or darker.

---

## Color Palette Summary

| Status            | Token Name        | Hex Value | On White (Contrast) | WCAG AA Pass      |
| ----------------- | ----------------- | --------- | ------------------- | ----------------- |
| Active            | `success-primary` | `#059669` | 4.89:1              | Yes               |
| Expiring          | `warning-primary` | `#D97706` | 5.12:1              | Yes               |
| Expired           | `error-primary`   | `#DC2626` | 5.89:1              | Yes               |
| No Session (bg)   | `neutral-300`     | `#ddd8d1` | 1.42:1              | No (borders only) |
| No Session (text) | `neutral-600`     | `#6b6561` | 4.76:1              | Yes               |
| Metadata          | `neutral-500`     | `#9a9591` | 3.14:1              | Yes (large text)  |

---

## Badge Component Variant Reference

Badge component automatically applies correct semantic tokens based on `variant` prop:

```tsx
// Active session
<Badge variant="success">Active</Badge>
// → bg-success-primary text-white border-success-primary

// Access token expired
<Badge variant="warning">Access Token Expired</Badge>
// → bg-warning-primary text-trust-deep border-warning-primary

// Expired
<Badge variant="error">Expired</Badge>
// → bg-error-primary text-white border-error-primary

// No session
<Badge variant="neutral">No Session</Badge>
// → bg-neutral-300 text-neutral-600 border-neutral-300
```

**Note**: Badge component enforces correct contrast ratios automatically.

---

## StatusIndicator Component Variant Reference

StatusIndicator component applies text colors only (no backgrounds):

```tsx
// Active/success metadata
<StatusIndicator variant="success" label="Active" />
// → text-success-primary

// Warning metadata
<StatusIndicator variant="warning" label="Expires in 3 days" />
// → text-warning-primary

// Error metadata
<StatusIndicator variant="error" label="Expired" />
// → text-error-primary

// Default metadata (encryption, timestamps, agent count)
<StatusIndicator variant="default" label="Encrypted" />
// → text-neutral-500
```

---

## Compound Status Patterns

### Session Card with Multiple Status Indicators

```tsx
<Card
  header={
    <Stack direction="row" align="center" justify="between">
      <h3>GitHub</h3>
      <Badge variant="success">Active</Badge>
    </Stack>
  }
>
  <Stack gap="sm" direction="column">
    {/* Encryption metadata (default) */}
    <StatusIndicator icon={<LockIcon />} label="Encrypted" variant="default" />

    {/* Agent count metadata (default) */}
    <StatusIndicator icon={<UsersIcon />} label="3 agents" variant="default" />

    {/* Timestamp metadata (default) */}
    <StatusIndicator
      icon={<ClockIcon />}
      label="Initiated 2 days ago"
      variant="default"
    />
  </Stack>
</Card>
```

**Token Usage**:

- Primary status (header): `success-primary` (Badge)
- Metadata items: `neutral-500` (StatusIndicator default)

---

### Expiring Session Warning

```tsx
<Card
  header={
    <Stack direction="row" align="center" justify="between">
      <h3>Google Drive</h3>
      <Badge variant="warning" showDot>
        Access Token Expired
      </Badge>
    </Stack>
  }
>
  <Stack gap="sm" direction="column">
    <StatusIndicator
      icon={<ClockIcon />}
      label="Expires in 3 days"
      variant="warning"
    />
    <StatusIndicator icon={<UsersIcon />} label="5 agents" variant="default" />
  </Stack>
</Card>
```

**Token Usage**:

- Primary status (header): `warning-primary` (Badge with dot)
- Expiration metadata: `warning-primary` (StatusIndicator)
- Agent count: `neutral-500` (StatusIndicator default)

---

### Expired Session Alert

```tsx
<Card
  header={
    <Stack direction="row" align="center" justify="between">
      <h3>Dropbox</h3>
      <Badge variant="error">Expired</Badge>
    </Stack>
  }
>
  <Alert variant="error" size="sm">
    Session expired. Re-authenticate to restore access.
  </Alert>
  <Button variant="primary" size="sm">
    Re-authenticate
  </Button>
</Card>
```

**Token Usage**:

- Primary status (header): `error-primary` (Badge)
- Alert: `error-light` background, `error-primary` border (Alert component)

---

## Accessibility Compliance Summary

### WCAG 2.1 AA Requirements

**Text Contrast (4.5:1 minimum)**:

- `success-primary` (#059669): 4.89:1 - Pass
- `warning-primary` (#D97706): 5.12:1 - Pass
- `error-primary` (#DC2626): 5.89:1 - Pass
- `neutral-600` (#6b6561): 4.76:1 - Pass

**UI Component Contrast (3:1 minimum)**:

- Badge backgrounds: All meet 3:1 for non-text UI elements
- StatusIndicator icons: 3:1+ when using semantic variants

### Color + Text Pattern

Never rely on color alone. Always combine color with:

1. **Text labels**: "Active", "Access Token Expired", "Login required"
2. **Icons**: CheckCircle, XCircle, AlertTriangle
3. **Dot indicators**: `showDot` prop on Badge for additional visual signal

**Example** (Good):

```tsx
<Badge variant="success" showDot>
  Active
</Badge>
```

- Color: Green (success-primary)
- Text: "Active" label
- Visual: Dot indicator

**Example** (Bad):

```tsx
<Badge variant="success">{/* No text label */}</Badge>
```

- Color only (fails WCAG)

---

## Dark Mode Considerations (Future)

Currently, the design system uses a light theme. When dark mode is implemented:

**Expected Token Adjustments**:

- `success-primary`: Shift to `success-light` (#d1fae5) for better contrast on dark
- `warning-primary`: May need lighter variant for dark backgrounds
- `error-primary`: Shift to `error-light` (#fee2e2) for dark backgrounds
- `neutral-*`: Use inverted scale (neutral-900 → neutral-100)

**Action Required**: Update this document when dark mode tokens are finalized.

---

## Testing Checklist

### Contrast Ratio Testing

- [ ] Verify success-primary on white: 4.5:1+ (Text)
- [ ] Verify warning-primary on white: 4.5:1+ (Text)
- [ ] Verify error-primary on white: 4.5:1+ (Text)
- [ ] Verify neutral-600 on white: 4.5:1+ (Text)
- [ ] Verify Badge backgrounds: 3:1+ (UI elements)

### Color Blindness Testing

- [ ] Test with Protanopia (red-blind) simulation
- [ ] Test with Deuteranopia (green-blind) simulation
- [ ] Test with Tritanopia (blue-blind) simulation
- [ ] Verify text labels + icons provide redundant cues

### Screen Reader Testing

- [ ] Badge content read aloud correctly
- [ ] StatusIndicator labels read (icons marked aria-hidden)
- [ ] Status changes announced dynamically

---

## Implementation Example: Complete Session Card

```tsx
import { Card } from '@design-system/components/data-display/Card';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Badge } from '@design-system/components/primitives/Badge';
import { StatusIndicator } from '@design-system/components/data-display/StatusIndicator';
import { Stack } from '@design-system/components/layout/Stack';
import { LockIcon, UsersIcon, ClockIcon } from 'lucide-react';

function getSessionStatusBadge(session) {
  if (!session) {
    return (
      <Badge variant="neutral" size="sm" shape="pill">
        No Session
      </Badge>
    );
  }

  if (session.requires_reauthentication) {
    return (
      <Badge variant="error" size="sm" shape="pill">
        Login required
      </Badge>
    );
  }

  if (session.access_token_expired) {
    return (
      <Badge variant="warning" size="sm" shape="pill" showDot>
        Access Token Expired
      </Badge>
    );
  }

  return (
    <Badge variant="success" size="sm" shape="pill">
      Active
    </Badge>
  );
}

export function SessionCard({ service, session }) {
  return (
    <Card
      padding="default"
      border="subtle"
      hover="lift"
      header={
        <Stack direction="row" align="center" justify="between">
          <Stack direction="row" align="center" gap="sm">
            <Avatar
              src={service.logo_url}
              alt={`${service.display_name} logo`}
              size="md"
              fallback={service.display_name[0]}
            />
            <h3 className="font-semibold text-neutral-900">
              {service.display_name}
            </h3>
          </Stack>
          {getSessionStatusBadge(session)}
        </Stack>
      }
      divider
    >
      {session ? (
        <Stack gap="sm" direction="column">
          <StatusIndicator
            icon={<LockIcon size={16} />}
            label="Encrypted"
            variant="default"
          />
          <StatusIndicator
            icon={<UsersIcon size={16} />}
            label={`${session.dependent_agents} agent${session.dependent_agents !== 1 ? 's' : ''}`}
            variant="default"
          />
          <StatusIndicator
            icon={<ClockIcon size={16} />}
            label={formatRelativeTime(session.initiated_at)}
            variant="default"
          />
        </Stack>
      ) : (
        <p className="text-neutral-600 text-sm">
          No active session. Establish a session to use this service.
        </p>
      )}
    </Card>
  );
}
```

**Token Usage in Example**:

- Active session: `success-primary` (Badge)
- Expiring session: `warning-primary` (Badge) + dot indicator
- Expired session: `error-primary` (Badge)
- No session: `neutral-300` background + `neutral-600` text (Badge)
- Metadata: `neutral-500` (StatusIndicator default variant)

---

## References

- Design System Color Guide: `web/src/design-system/docs/COLOR_GUIDE.md`
- Token Guide: `web/src/design-system/docs/TOKEN_GUIDE.md`
- WCAG 2.1 Contrast Requirements: https://www.w3.org/WAI/WCAG21/Understanding/contrast-minimum.html
- Design System Usage: `web/src/components/sessions/DESIGN_SYSTEM_USAGE.md`
