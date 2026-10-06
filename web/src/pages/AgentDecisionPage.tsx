import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useLocation, useParams } from 'react-router-dom';
import { isCancelledError } from '@tanstack/react-query';
import { consentCopy } from '@copy/consent';
import { useAgentDecision } from '@hooks/useAgentDecision';
import { useSaveGrant } from '@hooks/useAgentGrant';
import type { AgentDetailData } from '@services/api/consent';
import { usePrincipal } from '@services/query/QueryProvider';
import type { UserInfo } from '@app-types/consent';
import { isSafeRedirectUrl } from '@utils/validation';
import { createConsentDraft, InvalidGrantDateError, type ConsentDraftSnapshot } from '@components/consent/consentDraft';
import { consentErrorMessage, validateDraftSelection } from '@components/consent/consentValidation';
import { startServiceLogin } from '@components/consent/startServiceLogin';
import type { UserGrant } from '../types/consent';
import { AgentDecisionView } from './AgentDecisionView';

export function AgentDecisionPage({ currentUrl, restoredDraft, sessionToken: restoredSessionToken }: { currentUrl?: string; restoredDraft?: ConsentDraftSnapshot; sessionToken?: string }) {
  const { agentId = '' } = useParams<{ agentId: string }>();
  const location = useLocation();
  const sessionToken = restoredSessionToken ?? new URLSearchParams(location.search).get('session_token') ?? '';
  const user = usePrincipal();
  const query = useAgentDecision(agentId, sessionToken);
  const retry = () => { void query.refetch(); void query.grant.refetch(); };
  if (!sessionToken || !agentId) return <AgentDecisionView state="invalid" user={user} />;
  if (query.data && query.grant.data !== undefined) return <DecisionForm key={`${agentId}:${sessionToken}:${location.key}`} data={query.data} user={user} initialGrant={query.grant.data} sessionToken={sessionToken} restoredDraft={restoredDraft} currentUrl={currentUrl ?? new URL(location.pathname + location.search + location.hash, window.location.origin).toString()} authorizationError={query.error} onRetry={retry} />;
  if (query.error) {
    const invalid = typeof query.error === 'object' && query.error !== null && 'status' in query.error && [400, 401, 403, 404, 410].includes(Number(query.error.status));
    return invalid ? <AgentDecisionView state="invalid" user={user} /> : <AgentDecisionView state="error" user={user} onRetry={retry} />;
  }
  return <AgentDecisionView state="loading" user={user} />;
}

function DecisionForm({ data, user, initialGrant, sessionToken, restoredDraft, currentUrl, authorizationError, onRetry }: { data: AgentDetailData; user: UserInfo; initialGrant: UserGrant | null; sessionToken: string; restoredDraft?: ConsentDraftSnapshot; currentUrl: string; authorizationError: unknown; onRetry: () => void }) {
  const { agent, services } = data;
  const [draft, setDraft] = useState(() => createConsentDraft({ permissionSets: agent.permission_sets, serviceRequirements: agent.service_requirements, existingGrant: initialGrant, context: 'decision', sessionToken, restoredDraft }));
  const [outcome, setOutcome] = useState<'allowed' | 'denied' | null>(null);
  const [redirectUrl, setRedirectUrl] = useState<string>();
  const [error, setError] = useState<string>();
  const [selectionError, setSelectionError] = useState<string>();
  const [dateError, setDateError] = useState<string>();
  const errorRef = useRef<HTMLDivElement>(null);
  const submitting = useRef(false);
  const save = useSaveGrant(agent.agentId, { sessionToken });
  const assertCurrent = useRef(save.assertCurrent);
  useEffect(() => { assertCurrent.current = save.assertCurrent; });
  useEffect(() => { if (error) errorRef.current?.focus(); }, [error]);
  useEffect(() => {
    if (outcome !== 'allowed' || !redirectUrl) return;
    const timeout = window.setTimeout(() => {
      try {
        assertCurrent.current();
        // The server's continuation was checked before the success state appeared.
        window.location.href = redirectUrl;
      } catch (failure) {
        if (!isCancelledError(failure)) setError(consentErrorMessage(failure, consentCopy.saveError));
      }
    }, window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ? 0 : 320);
    return () => window.clearTimeout(timeout);
  }, [outcome, redirectUrl]);

  const selectedServices = new Set(Object.values(draft.selections).flat());
  const missingServiceIds = services.filter((service) => selectedServices.has(service.serviceId) && !agent.active_session_service_ids.includes(service.serviceId)).map((service) => service.serviceId);
  const errorGroupId = selectionError ? draft.groups.find((group) => group.selected && group.services.every((service) => !service.selected))?.id : undefined;

  async function allow(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current || outcome || missingServiceIds.length > 0 || !sessionToken) return;
    setError(undefined); setDateError(undefined); setSelectionError(undefined);
    const invalid = validateDraftSelection(draft);
    if (invalid) { setSelectionError(invalid); return; }
    submitting.current = true;
    try {
      const result = await save.mutateAsync(draft.toGrantRequest());
      save.assertCurrent();
      if (result.kind === 'redirect') {
        if (!isSafeRedirectUrl(result.redirectUrl)) { setError(consentCopy.invalidRedirect); return; }
        setRedirectUrl(result.redirectUrl);
      }
      setOutcome('allowed');
    } catch (failure) {
      if (isCancelledError(failure)) return;
      if (failure instanceof InvalidGrantDateError) setDateError(consentCopy.invalidDate);
      else setError(consentErrorMessage(failure, consentCopy.saveError));
    } finally {
      submitting.current = false;
    }
  }

  if (outcome === 'denied') return <AgentDecisionView state="denied" user={user} />;
  if (outcome !== 'allowed' && authorizationError && !save.isPending) {
    const invalid = typeof authorizationError === 'object' && authorizationError !== null && 'status' in authorizationError && [400, 401, 403, 404, 410].includes(Number(authorizationError.status));
    return invalid ? <AgentDecisionView state="invalid" user={user} /> : <AgentDecisionView state="error" user={user} onRetry={onRetry} />;
  }
  return <AgentDecisionView state="ready" data={data} user={user} draft={draft} allowed={outcome === 'allowed'} pending={save.isPending} missingServiceIds={missingServiceIds} error={error} errorRef={errorRef} selectionError={selectionError} errorGroupId={errorGroupId} dateError={dateError}
    onChange={(next) => { setDraft(next); setError(undefined); setDateError(undefined); setSelectionError(undefined); }}
    onAllow={allow}
    onDeny={() => { if (!submitting.current) setOutcome('denied'); }}
    onConnect={(serviceId) => { if (!submitting.current && !startServiceLogin(serviceId, draft, currentUrl)) setError(consentCopy.connectionStorageError); }} />;
}

export default AgentDecisionPage;
