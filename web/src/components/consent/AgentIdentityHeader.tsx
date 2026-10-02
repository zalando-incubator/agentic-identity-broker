import type { ReactNode } from 'react';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { consentCopy } from '@copy/consent';
import type { AgentDetail } from '../../types/consent';

export function AgentIdentityHeader({ agent, logoUrl, originLabel, actions }: { agent: AgentDetail; logoUrl?: string; originLabel?: string | null; actions?: ReactNode }) {
  const links = [
    { href: agent.governanceUrl, label: consentCopy.governance },
    { href: agent.userDocumentationUrl, label: consentCopy.documentation },
    { href: agent.agentInterfaceUrl, label: consentCopy.agentInterface },
  ];
  return <header data-testid="agent-identity" className="space-y-4">
    <div className="flex items-start gap-3">
      <Avatar size="lg" label={agent.displayName} src={logoUrl ?? agent.logoUrl} fallback={agent.displayName.slice(0, 1).toUpperCase()} />
      <div className="min-w-0 flex-1 space-y-2">
        <TruncatedText as="h1" text={agent.displayName} lines={2} data-testid="agent-name-heading" expandLabel={consentCopy.expandAgentName} collapseLabel={consentCopy.collapseAgentName} className="font-display text-2xl font-semibold" />
        {originLabel && <Badge variant="outline" data-testid="agent-origin-label" className="max-w-full whitespace-normal break-words">{originLabel}</Badge>}
      </div>
      {actions}
    </div>
    {agent.description && <TruncatedText as="p" text={agent.description} lines={2} expandLabel={consentCopy.expandDescription} collapseLabel={consentCopy.collapseDescription} className="text-sm text-muted-foreground" />}
    <div className="flex flex-wrap gap-2">
      {links.map(({ href, label }) => {
        if (!href) return null;
        try {
          const url = new URL(href, window.location.origin);
          if (url.protocol !== 'https:' && url.protocol !== 'http:') return null;
        } catch { return null; }
        return <Button key={label} variant="outline" size="sm" asChild><a href={href} target="_blank" rel="noopener noreferrer">{label}</a></Button>;
      })}
    </div>
  </header>;
}
