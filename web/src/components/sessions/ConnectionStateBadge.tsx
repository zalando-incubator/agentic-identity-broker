import { CircleCheck, CircleHelp, ClockAlert, KeyRound, TriangleAlert } from 'lucide-react';
import { Badge, type BadgeProps } from '@design-system/components/primitives/Badge';
import { accessCopy } from '@copy';
import { connectionsCopy } from '@copy/connections';
import type { ConnectionState, ConnectionStatus } from './connectionState';

const presentation: Record<ConnectionStatus, { label: string; variant: BadgeProps['variant']; icon: typeof CircleCheck }> = {
  connected: { label: accessCopy.connected, variant: 'success', icon: CircleCheck },
  'needs-reauthentication': { label: accessCopy.needsReauthentication, variant: 'warning', icon: KeyRound },
  expired: { label: accessCopy.expired, variant: 'warning', icon: ClockAlert },
  'no-connection': { label: accessCopy.noConnection, variant: 'neutral', icon: CircleHelp },
  error: { label: connectionsCopy.unavailable, variant: 'danger', icon: TriangleAlert },
};

export function ConnectionStateBadge({ state }: { state: ConnectionState }) {
  const { label, variant, icon: Icon } = presentation[state.stale ? 'error' : state.status];
  return <Badge variant={variant} icon={<Icon />} data-testid="connection-state">{label}</Badge>;
}
