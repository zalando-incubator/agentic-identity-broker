import { Link } from 'react-router-dom';
import { useConnections } from '@hooks/useConnections';
import { ConnectionStateBadge } from '@components/sessions/ConnectionStateBadge';
import { deriveConnectionState } from '@components/sessions/connectionState';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Button } from '@design-system/components/primitives/Button';
import { Alert } from '@design-system/components/feedback/Alert';
import { accessCopy, commonCopy } from '@copy';
import { consentCopy } from '@copy/consent';
import type { ServiceRequirement } from '../../types/consent';
import type { ConsentDraft } from './consentDraft';
import { buildServiceLoginUrl } from './ServiceConnectPrompt';

export function AgentConnectionsTab({ services, draft, currentUrl }: { services: ServiceRequirement[]; draft: ConsentDraft; currentUrl: string }) {
  const connections = useConnections();
  if (connections.loading) return <p role="status">{commonCopy.loading}</p>;
  return <div className="space-y-4">
    {Boolean(connections.error) && <Alert variant="error" action={{ label: commonCopy.retry, onClick: () => { void connections.refetch(); } }}>{consentCopy.connectionsLoadError}</Alert>}
    {services.length === 0 && <p className="text-muted-foreground">{consentCopy.noConnections}</p>}
    <ul className="divide-y divide-border-soft">
      {services.map((service) => {
        const session = connections.sessions.find((entry) => entry.service_id === service.serviceId);
        const state = service.connectionStatus === 'connected' && session ? connections.getState(service.serviceId)
          : deriveConnectionState({ context: 'requirement', connectionStatus: service.connectionStatus, session, readFailed: Boolean(connections.error) });
        return <li key={service.serviceId} data-testid="agent-connection-row" className="flex flex-wrap items-center gap-3 py-4">
          <Avatar label={service.serviceName} src={service.logoUrl} />
          <span data-testid="connection-provider" className="min-w-0 flex-1 [overflow-wrap:anywhere]">{service.serviceName}</span>
          <ConnectionStateBadge state={state} />
          {state.status === 'no-connection'
            ? <Button variant="outline" size="sm" asChild><a data-testid="connection-action" href={buildServiceLoginUrl(service.serviceId, draft, currentUrl)}>{accessCopy.connect}</a></Button>
            : <Button variant="outline" size="sm" asChild><Link data-testid="connection-action" to="/sessions">{consentCopy.manageConnections}</Link></Button>}
        </li>;
      })}
    </ul>
  </div>;
}
