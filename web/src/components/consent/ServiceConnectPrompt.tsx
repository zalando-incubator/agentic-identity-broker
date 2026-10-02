import { Avatar } from '@design-system/components/primitives/Avatar';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import { accessCopy, navigationCopy } from '@copy';
import type { ServiceRequirement } from '../../types/consent';
import type { ConsentDraft } from './consentDraft';

/** The authorize endpoint keeps the original callback and opaque authorization context. */
export function buildServiceLoginUrl(serviceId: string, draft: ConsentDraft, currentUrl: string): string {
  const callback = new URL(currentUrl);
  callback.searchParams.set('consent_state', draft.toConsentState());
  return `/api/third-party/${encodeURIComponent(serviceId)}/oauth2/authorize?redirect_uri=${encodeURIComponent(callback.toString())}`;
}

export function ServiceConnectPrompt({ services, draft, currentUrl, disabled }: { services: ServiceRequirement[]; draft: ConsentDraft; currentUrl: string; disabled?: boolean }) {
  if (services.length === 0) return null;
  return <section className="space-y-3" aria-label={navigationCopy.connections}>
    <h2 className="font-display text-lg font-semibold">{navigationCopy.connections}</h2>
    <ul className="divide-y divide-border-soft rounded-lg border border-border px-4">
      {services.map((service) => <li key={service.serviceId} data-testid="consent-service" className="flex flex-wrap items-center gap-3 py-3">
        <Avatar label={service.serviceName} src={service.logoUrl} />
        <span className="min-w-0 flex-1 break-words">{service.serviceName}</span>
        <Badge variant="outline">{accessCopy.noConnection}</Badge>
        <Button variant="outline" disabled={disabled} onClick={() => { window.location.href = buildServiceLoginUrl(service.serviceId, draft, currentUrl); }}>{accessCopy.connect}</Button>
      </li>)}
    </ul>
  </section>;
}
