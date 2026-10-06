import { lazy, Suspense, useCallback, useRef, useState } from 'react';
import { Button } from '@design-system/components/primitives/Button';
import { consentCopy } from '@copy/consent';
import type { AgentDetail, CIMDMetadata } from '@app-types/consent';

const TechnicalDetailsDialog = lazy(() => import('./TechnicalDetailsDialog'));

export function TechnicalDetails({ agent, metadata }: { agent: AgentDetail; metadata?: CIMDMetadata }) {
  const [open, setOpen] = useState(false);
  const [ready, setReady] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const onReady = useCallback(() => setReady(true), []);
  const clientId = agent.clientId || metadata?.client_id_url;
  if (!clientId && !metadata?.redirect_uri && !metadata?.requested_scopes.length) return null;

  return <>
    <Button ref={trigger} type="button" variant="ghost" size="sm" className="h-auto p-0 text-xs underline underline-offset-4"
      aria-haspopup="dialog" aria-expanded={open} aria-busy={open && !ready || undefined}
      onClick={() => setOpen(true)}
      onKeyDown={(event) => { if (event.key === 'Escape' && !ready) setOpen(false); }}>{consentCopy.technicalDetails}</Button>
    {(open || ready) && <Suspense fallback={null}>
      <TechnicalDetailsDialog agent={agent} metadata={metadata} open={open} onOpenChange={setOpen} trigger={trigger} onReady={onReady} />
    </Suspense>}
  </>;
}
