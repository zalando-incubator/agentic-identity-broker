import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useLocation, useParams } from 'react-router-dom';
import { isCancelledError } from '@tanstack/react-query';
import { Alert } from '@design-system/components/feedback/Alert';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@design-system/components/navigation/Tabs';
import { commonCopy, navigationCopy } from '@copy';
import { consentCopy } from '@copy/consent';
import { useAgentDetail } from '@hooks/useAgentDetail';
import { useSaveGrant } from '@hooks/useAgentGrant';
import type { AgentDetailData } from '@services/api/consent';
import { isSafeRedirectUrl } from '@utils/validation';
import { AgentIdentityHeader } from '@components/consent/AgentIdentityHeader';
import { PermissionGroupList } from '@components/consent/PermissionGroupList';
import { DurationChoice } from '@components/consent/DurationChoice';
import { GrantEditBar } from '@components/consent/GrantEditBar';
import { AgentOverflowMenu } from '@components/consent/AgentOverflowMenu';
import { AgentConnectionsTab } from '@components/consent/AgentConnectionsTab';
import { ServiceConnectPrompt } from '@components/consent/ServiceConnectPrompt';
import { createConsentDraft, InvalidGrantDateError, type ConsentDraftSnapshot } from '@components/consent/consentDraft';
import { consentErrorMessage, validateDraftSelection } from '@components/consent/consentValidation';
import type { UserGrant } from '../types/consent';

export function AgentConsolePage({ currentUrl, restoredDraft }: { currentUrl?: string; restoredDraft?: ConsentDraftSnapshot }) {
  const { agentId = '' } = useParams<{ agentId: string }>();
  const location = useLocation();
  const query = useAgentDetail(agentId);
  if (query.data && query.grant.data !== undefined) return <GrantEditor key={`${agentId}:${location.key}`} data={query.data} initialGrant={query.grant.data} restoredDraft={restoredDraft} currentUrl={currentUrl ?? new URL(location.pathname + location.search + location.hash, window.location.origin).toString()} />;
  if (query.error || !agentId) return <Alert variant="error" action={{ label: commonCopy.retry, onClick: () => { void query.refetch(); void query.grant.refetch(); } }}>{consentCopy.loadError}</Alert>;
  return <p role="status">{commonCopy.loading}</p>;
}

function GrantEditor({ data, initialGrant, restoredDraft, currentUrl }: { data: AgentDetailData; initialGrant: UserGrant | null; restoredDraft?: ConsentDraftSnapshot; currentUrl: string }) {
  const { agent, services } = data;
  const [savedGrant, setSavedGrant] = useState(initialGrant);
  const observedGrant = useRef(initialGrant);
  const [draft, setDraft] = useState(() => createConsentDraft({ permissionSets: agent.permission_sets, serviceRequirements: agent.service_requirements, existingGrant: initialGrant, context: 'console', restoredDraft }));
  const [error, setError] = useState<string>();
  const [dateError, setDateError] = useState<string>();
  const [status, setStatus] = useState<string>();
  const [tab, setTab] = useState('permissions');
  const errorRef = useRef<HTMLDivElement>(null);
  const headingRef = useRef<HTMLDivElement>(null);
  const submitting = useRef(false);
  const save = useSaveGrant(agent.agentId);
  const selectedServices = new Set(Object.values(draft.selections).flat());
  const missingServices = services.filter((service) => (draft.groups.length === 0 || selectedServices.has(service.serviceId)) && !agent.active_session_service_ids.includes(service.serviceId));
  const blocked = draft.groups.length > 0 && missingServices.length > 0;
  useEffect(() => { if (error) errorRef.current?.focus(); }, [error]);
  useEffect(() => {
    // A failed refetch retains its previous query data. It must not replace a successful POST result.
    if (observedGrant.current === initialGrant) return;
    observedGrant.current = initialGrant;
    setSavedGrant(initialGrant);
    if (!draft.dirty && !save.isPending) {
      setDraft(createConsentDraft({ permissionSets: agent.permission_sets, serviceRequirements: agent.service_requirements, existingGrant: initialGrant, context: 'console' }));
    }
  }, [initialGrant, agent.permission_sets, agent.service_requirements, draft.dirty, save.isPending]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current || !draft.dirty || blocked) return;
    setError(undefined); setDateError(undefined); setStatus(undefined);
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
        setSavedGrant(result.grant);
        setDraft(createConsentDraft({ permissionSets: agent.permission_sets, serviceRequirements: agent.service_requirements, existingGrant: result.grant, context: 'console' }));
        setStatus(consentCopy.grantSaved);
        headingRef.current?.focus();
      }
    } catch (failure) {
      if (isCancelledError(failure)) return;
      if (failure instanceof InvalidGrantDateError) { setTab('permissions'); setDateError(consentCopy.invalidDate); }
      else setError(consentErrorMessage(failure, consentCopy.saveError));
    } finally {
      submitting.current = false;
    }
  }

  return <form onSubmit={submit} className="space-y-6" onInvalid={(event) => { (event.target as HTMLElement).focus(); }}>
    <div ref={headingRef} tabIndex={-1} className="rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring">
      <AgentIdentityHeader agent={agent} actions={savedGrant && Object.keys(savedGrant.granted_permission_sets).length > 0 ? <AgentOverflowMenu agent={agent} disabled={save.isPending} /> : undefined} />
    </div>
    {error && <Alert ref={errorRef} tabIndex={-1} variant="error">{error}</Alert>}
    {status && <p role="status" className="text-sm text-success">{status}</p>}
    <Tabs value={tab} onValueChange={setTab}>
      <TabsList><TabsTrigger value="permissions">{consentCopy.permissions}</TabsTrigger><TabsTrigger value="connections">{navigationCopy.connections}</TabsTrigger></TabsList>
      <TabsContent value="permissions" className="space-y-6">
        <PermissionGroupList draft={draft} services={services} disabled={save.isPending} onChange={(next) => { setDraft(next); setError(undefined); setStatus(undefined); }} />
        <DurationChoice draft={draft} disabled={save.isPending} error={dateError} onChange={(next) => { setDraft(next); setDateError(undefined); setError(undefined); setStatus(undefined); }} />
        <ServiceConnectPrompt services={missingServices} draft={draft} currentUrl={currentUrl} disabled={save.isPending} />
      </TabsContent>
      <TabsContent value="connections"><AgentConnectionsTab services={services} draft={draft} currentUrl={currentUrl} /></TabsContent>
    </Tabs>
    {draft.dirty && <GrantEditBar pending={save.isPending} blocked={blocked} onCancel={() => { setDraft(createConsentDraft({ permissionSets: agent.permission_sets, serviceRequirements: agent.service_requirements, existingGrant: savedGrant, context: 'console' })); setError(undefined); setDateError(undefined); setStatus(undefined); headingRef.current?.focus(); }} />}
  </form>;
}

export default AgentConsolePage;
