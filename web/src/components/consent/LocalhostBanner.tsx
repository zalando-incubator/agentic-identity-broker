import { Alert } from '@design-system/components/feedback/Alert';
import { consentCopy } from '@copy/consent';

export function LocalhostBanner({ agentName }: { agentName: string }) {
  return <Alert variant="warning" role="alert" data-testid="localhost-warning" className="[overflow-wrap:anywhere]">{consentCopy.localhostWarning(agentName)}</Alert>;
}
