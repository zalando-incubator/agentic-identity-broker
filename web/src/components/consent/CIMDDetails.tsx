import { Accordion, AccordionItem, AccordionTrigger, AccordionContent } from '@design-system/components/advanced/Accordion';
import { consentCopy } from '@copy/consent';
import type { CIMDMetadata } from '../../types/consent';

export function CIMDDetails({ metadata }: { metadata: CIMDMetadata }) {
  return <Accordion type="single" collapsible><AccordionItem value="cimd">
    <AccordionTrigger>{consentCopy.advancedDetails}</AccordionTrigger>
    <AccordionContent><dl className="space-y-3">
      <div><dt className="text-muted-foreground">{consentCopy.clientId}</dt><dd className="break-all font-mono">{metadata.client_id_url}</dd></div>
      <div><dt className="text-muted-foreground">{consentCopy.redirectUri}</dt><dd className="break-all font-mono">{metadata.redirect_uri}</dd></div>
      <div><dt className="text-muted-foreground">{consentCopy.requestedScopes}</dt><dd className="flex flex-wrap gap-2 font-mono">
        {metadata.requested_scopes.length ? metadata.requested_scopes.map((scope) => <span className="break-all" key={scope}>{scope}</span>) : consentCopy.noneRequested}
      </dd></div>
    </dl></AccordionContent>
  </AccordionItem></Accordion>;
}
