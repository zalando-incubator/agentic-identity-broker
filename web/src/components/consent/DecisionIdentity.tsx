import { lazy, Suspense, useCallback, useRef, useState } from 'react';
import { CornerDownRight, Info, ShieldAlert, ShieldCheck, TriangleAlert } from 'lucide-react';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Button } from '@design-system/components/primitives/Button';
import { consentCopy } from '@copy/consent';
import type { AgentDetail, CIMDMetadata } from '@app-types/consent';
import { getAgentOrigin } from './origin';

const AboutAgentPopover = lazy(() => import('./AboutAgentPopover'));

export function DecisionIdentity({ agent, metadata }: { agent: AgentDetail; metadata?: CIMDMetadata }) {
  const [aboutOpen, setAboutOpen] = useState(false);
  const [aboutReady, setAboutReady] = useState(false);
  const aboutTrigger = useRef<HTMLButtonElement>(null);
  const onAboutReady = useCallback(() => setAboutReady(true), []);
  const origin = getAgentOrigin('decision', metadata);
  const links = [
    { href: agent.governanceUrl, label: consentCopy.governance },
    { href: agent.userDocumentationUrl, label: consentCopy.documentation },
    { href: agent.agentInterfaceUrl, label: consentCopy.agentInterface },
  ].filter((link): link is typeof link & { href: string } => {
    if (!link.href) return false;
    try {
      const url = new URL(link.href, window.location.origin);
      return url.protocol === 'https:' || url.protocol === 'http:';
    } catch {
      return false;
    }
  });

  return <div data-testid="agent-identity" className="space-y-2">
    <div className="flex min-w-0 items-center gap-3">
      <Avatar id={agent.agentId} label={agent.displayName} src={metadata?.logo_uri} className="size-10 shrink-0" />
      <h1 data-testid="agent-name-heading" className="min-w-0 flex-1 font-display leading-snug"><span data-testid="agent-name" title={agent.displayName} className="block truncate text-xl font-semibold">{agent.displayName}</span> <span className="block text-consent font-normal text-muted-foreground">{consentCopy.headingSuffix}</span></h1>
      {(agent.description || links.length > 0) && <>
        <Button ref={aboutTrigger} type="button" size="icon" variant="outline" className="size-8 shrink-0" aria-label={consentCopy.about}
          aria-haspopup="dialog" aria-expanded={aboutOpen} aria-busy={aboutOpen && !aboutReady || undefined}
          onClick={() => setAboutOpen((before) => !before)}
          onKeyDown={(event) => { if (event.key === 'Escape' && !aboutReady) setAboutOpen(false); }}><Info aria-hidden="true" /></Button>
        {(aboutOpen || aboutReady) && <Suspense fallback={null}>
          <AboutAgentPopover description={agent.description} links={links} trigger={aboutTrigger} open={aboutOpen} onOpenChange={setAboutOpen} onReady={onAboutReady} />
        </Suspense>}
      </>}
    </div>
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground [overflow-wrap:anywhere]" data-testid="agent-origin-label">
      <span className="inline-flex min-w-0 items-center gap-1.5">
        {origin.unverified ? <ShieldAlert aria-hidden="true" className="size-4 shrink-0" /> : metadata ? <ShieldCheck aria-hidden="true" className="size-4 shrink-0 text-success-foreground" /> : <Info aria-hidden="true" className="size-4 shrink-0" />}
        {origin.label}
      </span>
      {origin.redirectHost && <span className="inline-flex items-center gap-1.5"><CornerDownRight aria-hidden="true" className="size-4 shrink-0" />{consentCopy.redirectHost(origin.redirectHost)}</span>}
    </div>
    {(origin.localhost || origin.unverified) && <div role="alert" data-testid={origin.localhost ? 'localhost-warning' : 'consent-risk'} className="flex items-start gap-2 rounded-lg bg-warning px-3 py-2 text-xs text-warning-foreground">
      <TriangleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      <span>{origin.localhost ? origin.unverified ? consentCopy.unverifiedLocalWarning(agent.displayName) : consentCopy.connectionWarning(agent.displayName) : consentCopy.unverifiedWarning}</span>
    </div>}
  </div>;
}
