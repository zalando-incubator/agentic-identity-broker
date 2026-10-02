import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useLocation, useParams } from 'react-router-dom';
import { isCancelledError } from '@tanstack/react-query';
import { Alert } from '@design-system/components/feedback/Alert';
import { commonCopy } from '@copy';
import { consentCopy } from '@copy/consent';
import { useAgentDecision } from '@hooks/useAgentDecision';
import { useSaveGrant } from '@hooks/useAgentGrant';
import type { AgentDetailData } from '@services/api/consent';
import { isSafeRedirectUrl } from '@utils/validation';
import { AgentIdentityHeader } from '@components/consent/AgentIdentityHeader';
import { CIMDDetails } from '@components/consent/CIMDDetails';
import { LocalhostBanner } from '@components/consent/LocalhostBanner';
import { PermissionGroupList } from '@components/consent/PermissionGroupList';
import { DurationChoice } from '@components/consent/DurationChoice';
import { ConsentActions } from '@components/consent/ConsentActions';
import { ServiceConnectPrompt } from '@components/consent/ServiceConnectPrompt';
import { createConsentDraft, InvalidGrantDateError } from '@components/consent/consentDraft';
import { consentErrorMessage, validateDraftSelection } from '@components/consent/consentValidation';
import { getAgentOrigin } from '@components/consent/origin';
import type { UserGrant } from '../types/consent';

export function AgentDecisionPage() {
  const { agentId = '' } = useParams<{ agentId: string }>();
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const sessionToken = params.get('session_token') ?? '';
  const query = useAgentDecision(agentId, sessionToken);
  const retry = () => { void query.refetch(); void query.grant.refetch(); };
  if (!sessionToken || !agentId) return <Alert variant="error" data-testid="consent-error">{consentCopy.invalidSession}</Alert>;
  if (query.data && query.grant.data !== undefined) return <DecisionForm key={`${agentId}:${sessionToken}`} data={query.data} initialGrant={query.grant.data} sessionToken={sessionToken} consentState={params.get('consent_state')} currentUrl={new URL(location.pathname + location.search, window.location.origin).toString()} authorizationError={query.error} onRetry={retry} />;
  if (query.error) return <DecisionLoadError error={query.error} onRetry={retry} />;
  return <p role="status">{commonCopy.loading}</p>;
}

function DecisionLoadError({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const invalidSession = typeof error === 'object' && error !== null && 'status' in error && [400, 401, 403, 404, 410].includes(Number(error.status));
  return <Alert variant="error" data-testid="consent-error" action={invalidSession ? undefined : { label: commonCopy.retry, onClick: onRetry }}>
    {invalidSession ? consentCopy.invalidSession : consentCopy.loadError}
  </Alert>;
}

function DecisionForm({ data, initialGrant, sessionToken, consentState, currentUrl, authorizationError, onRetry }: { data: AgentDetailData; initialGrant: UserGrant | null; sessionToken: string; consentState: string | null; currentUrl: string; authorizationError: unknown; onRetry: () => void }) {
  const { agent, services, cimd_metadata: metadata } = data;
  const [draft, setDraft] = useState(() => createConsentDraft({ permissionSets: agent.permission_sets ?? [], serviceRequirements: agent.service_requirements, existingGrant: initialGrant, context: 'decision', sessionToken, consentState }));
  const [outcome, setOutcome] = useState<'allowed' | 'denied' | null>(null);
  const [error, setError] = useState<string>();
  const [dateError, setDateError] = useState<string>();
  const errorRef = useRef<HTMLDivElement>(null);
  const submitting = useRef(false);
  const save = useSaveGrant(agent.agentId, { sessionToken });
  const origin = getAgentOrigin('decision', metadata);
  const selectedServices = new Set(Object.values(draft.selections).flat());
  const missingServices = services.filter((service) => (draft.groups.length === 0 || selectedServices.has(service.serviceId)) && !agent.active_session_service_ids?.includes(service.serviceId));
  const blocked = draft.groups.length > 0 && missingServices.length > 0;
  useEffect(() => { if (error) errorRef.current?.focus(); }, [error]);

  async function allow(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current || outcome || blocked || authorizationError || !sessionToken) return;
    setError(undefined); setDateError(undefined);
    const selectionError = validateDraftSelection(draft);
    if (selectionError) { setError(selectionError); return; }
    submitting.current = true;
    try {
      const result = await save.mutateAsync(draft.toGrantRequest());
      save.assertCurrent();
      if (result.kind === 'redirect') {
        if (!isSafeRedirectUrl(result.redirectUrl)) { setError(consentCopy.invalidRedirect); return; }
        window.location.href = result.redirectUrl;
      } else {
        setOutcome('allowed');
      }
    } catch (failure) {
      if (isCancelledError(failure)) return;
      if (failure instanceof InvalidGrantDateError) setDateError(consentCopy.invalidDate);
      else setError(consentErrorMessage(failure, consentCopy.saveError));
    } finally {
      submitting.current = false;
    }
  }

  if (outcome) return <div className="space-y-6"><AgentIdentityHeader agent={agent} originLabel={origin.label} logoUrl={metadata?.logo_uri} /><Alert data-testid="consent-outcome" variant={outcome === 'allowed' ? 'success' : 'info'}>{outcome === 'allowed' ? consentCopy.allowed : consentCopy.denied}</Alert></div>;
  if (authorizationError && !save.isPending) return <DecisionLoadError error={authorizationError} onRetry={onRetry} />;
  return <form onSubmit={allow} className="space-y-6" onInvalid={(event) => { (event.target as HTMLElement).focus(); }}>
    <AgentIdentityHeader agent={agent} originLabel={origin.label} logoUrl={metadata?.logo_uri} />
    {origin.localhost && <LocalhostBanner agentName={agent.displayName} />}
    <p className="text-sm text-muted-foreground [overflow-wrap:anywhere]">{consentCopy.requestSummary(agent.displayName)}</p>
    {error && <Alert ref={errorRef} tabIndex={-1} variant="error" data-testid="consent-error">{error}</Alert>}
    <PermissionGroupList draft={draft} services={services} disabled={save.isPending} onChange={(next) => { setDraft(next); setError(undefined); }} />
    <DurationChoice draft={draft} disabled={save.isPending} error={dateError} onChange={(next) => { setDraft(next); setDateError(undefined); setError(undefined); }} />
    <ServiceConnectPrompt services={missingServices} draft={draft} currentUrl={currentUrl} disabled={save.isPending} />
    {metadata && <CIMDDetails metadata={metadata} />}
    <ConsentActions pending={save.isPending} blocked={blocked} onDeny={() => { if (!submitting.current) setOutcome('denied'); }} />
  </form>;
}

export default AgentDecisionPage;
