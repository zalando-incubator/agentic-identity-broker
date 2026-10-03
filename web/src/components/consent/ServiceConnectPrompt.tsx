import { useState } from 'react';
import { Alert } from '@design-system/components/feedback/Alert';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import { accessCopy, consentCopy, navigationCopy } from '@copy';
import { saveConsentDraft } from '@services/storage/session';
import type { ServiceRequirement } from '../../types/consent';
import type { ConsentDraft } from './consentDraft';

/** Start a provider login only after its callback context is safe in this tab. */
export function startServiceLogin(serviceId: string, draft: ConsentDraft, currentUrl: string): boolean {
  const callback = new URL(currentUrl);
  if (callback.origin !== window.location.origin) return false;
  for (const key of ['consent_state', 'consent_state_id', 'success', 'service_id']) {
    if (callback.searchParams.has(key)) callback.searchParams.delete(key);
  }
  const redirectURI = `${callback.origin}${callback.pathname}`;
  const action = `/api/third-party/${encodeURIComponent(serviceId)}/oauth2/authorize`;
  if (Object.keys(draft.selections).length === 0 && draft.duration === 'until-revoked' && !draft.customDate && !callback.search && !callback.hash) {
    window.location.href = `${action}?redirect_uri=${encodeURIComponent(redirectURI)}`;
    return true;
  }

  const stateID = saveConsentDraft(draft.snapshot(), serviceId, callback.toString());
  if (!stateID) return false;
  const form = document.createElement('form');
  form.method = 'post';
  form.action = action;
  form.hidden = true;
  for (const [name, value] of Object.entries({ redirect_uri: redirectURI, consent_state_id: stateID })) {
    const input = document.createElement('input');
    input.name = name;
    input.value = value;
    form.appendChild(input);
  }
  document.body.appendChild(form);
  form.submit();
  return true;
}

export function ServiceConnectPrompt({ services, draft, currentUrl, disabled }: { services: ServiceRequirement[]; draft: ConsentDraft; currentUrl: string; disabled?: boolean }) {
  const [storageError, setStorageError] = useState(false);
  if (services.length === 0) return null;
  return <section className="space-y-3" aria-label={navigationCopy.connections}>
    {storageError && <Alert variant="error">{consentCopy.connectionStorageError}</Alert>}
    <h2 className="font-display text-lg font-semibold">{navigationCopy.connections}</h2>
    <ul className="divide-y divide-border-soft rounded-lg border border-border px-4">
      {services.map((service) => <li key={service.serviceId} data-testid="consent-service" className="flex flex-wrap items-center gap-3 py-3">
        <Avatar label={service.serviceName} />
        <span className="min-w-0 flex-1 break-words">{service.serviceName}</span>
        <Badge variant="outline">{accessCopy.noConnection}</Badge>
        <Button variant="outline" disabled={disabled} onClick={() => { setStorageError(!startServiceLogin(service.serviceId, draft, currentUrl)); }}>{accessCopy.connect}</Button>
      </li>)}
    </ul>
  </section>;
}
