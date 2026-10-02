import { Badge, type BadgeProps } from '@design-system/components/primitives/Badge';
import { accessCopy } from '@copy';
import { connectionsCopy } from '@copy/connections';
import type { ConnectionState, ConnectionStatus } from './connectionState';

const presentation: Record<ConnectionStatus, { label: string; variant: BadgeProps['variant'] }> = {
  connected: { label: accessCopy.connected, variant: 'success' },
  'needs-reauthentication': { label: accessCopy.needsReauthentication, variant: 'warning' },
  expired: { label: accessCopy.expired, variant: 'warning' },
  'no-connection': { label: accessCopy.noConnection, variant: 'neutral' },
  error: { label: connectionsCopy.unavailable, variant: 'danger' },
};

export function ConnectionStateBadge({ state }: { state: ConnectionState }) {
  const { label, variant } = presentation[state.status];
  return <div className="space-y-1">
    <Badge variant={variant} data-testid="connection-state">{label}</Badge>
    {state.reason === 'access-expired' && <p className="text-xs text-muted-foreground">{connectionsCopy.refreshExplanation}</p>}
    {state.stale && <p className="text-xs text-muted-foreground">{connectionsCopy.stale}</p>}
  </div>;
}
