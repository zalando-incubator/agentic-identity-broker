import { useEffect } from 'react';
import type { RefObject } from 'react';
import { Dialog, DialogContent, DialogTitle } from '@design-system/components/overlays/Dialog';
import { commonCopy } from '@copy';
import { consentCopy } from '@copy/consent';
import type { AgentDetail, CIMDMetadata } from '@app-types/consent';

interface TechnicalDetailsDialogProps {
  agent: AgentDetail;
  metadata?: CIMDMetadata;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: RefObject<HTMLButtonElement | null>;
  onReady: () => void;
}

export default function TechnicalDetailsDialog({ agent, metadata, open, onOpenChange, trigger, onReady }: TechnicalDetailsDialogProps) {
  const clientId = agent.clientId || metadata?.client_id_url;
  useEffect(() => { onReady(); }, [onReady]);

  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent closeLabel={commonCopy.close} onCloseAutoFocus={(event) => { event.preventDefault(); trigger.current?.focus(); }}>
      <DialogTitle>{consentCopy.technicalDetails}</DialogTitle>
      <dl className="space-y-3 text-sm">
        {clientId && <div><dt className="text-muted-foreground">{consentCopy.clientId}</dt><dd className="break-all font-mono">{clientId}</dd></div>}
        {metadata?.redirect_uri && <div><dt className="text-muted-foreground">{consentCopy.redirectUri}</dt><dd className="break-all font-mono">{metadata.redirect_uri}</dd></div>}
        {metadata && <div><dt className="text-muted-foreground">{consentCopy.requestedScopes}</dt><dd className="flex flex-wrap gap-2 font-mono">
          {metadata.requested_scopes.length ? metadata.requested_scopes.map((scope) => <span className="break-all" key={scope}>{scope}</span>) : consentCopy.noneRequested}
        </dd></div>}
      </dl>
    </DialogContent>
  </Dialog>;
}
