import { saveConsentDraft } from '@services/storage/session';
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
