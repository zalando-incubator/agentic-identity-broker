import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useBeforeUnload, useBlocker, useLocation, useNavigate, useParams } from 'react-router-dom';
import { isCancelledError } from '@tanstack/react-query';
import { toast } from 'sonner';
import { consentCopy } from '@copy/consent';
import { Button } from '@design-system/components/primitives/Button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@design-system/components/overlays/Dialog';
import { commonCopy } from '@copy';
import { useAgentDetail } from '@hooks/useAgentDetail';
import { useConnections } from '@hooks/useConnections';
import { useSaveGrant } from '@hooks/useAgentGrant';
import { useRevokeGrant } from '@hooks/useRevokeGrant';
import { getAuthGeneration } from '@services/api/client';
import type { AgentDetailData } from '@services/api/consent';
import { isSafeRedirectUrl } from '@utils/validation';
import { deriveConnectionState } from '@components/sessions/connectionState';
import type { AgentConnection } from '@components/consent/AgentConnectionsRail';
import { startServiceLogin } from '@components/consent/startServiceLogin';
import { createConsentDraft, InvalidGrantDateError, type ConsentDraftSnapshot } from '@components/consent/consentDraft';
import { consentErrorMessage, validateDraftSelection } from '@components/consent/consentValidation';
import type { UserGrant } from '@app-types/consent';
import { AgentConsoleView } from './AgentConsoleView';

export function AgentConsolePage({ currentUrl, restoredDraft }: { currentUrl?: string; restoredDraft?: ConsentDraftSnapshot }) {
  const { agentId = '' } = useParams<{ agentId: string }>();
  const location = useLocation();
  const query = useAgentDetail(agentId);
  if (query.data && query.grant.data !== undefined) return <GrantEditor key={`${agentId}:${location.key}`}
    data={query.data} initialGrant={query.grant.data} restoredDraft={restoredDraft}
    stale={Boolean(query.error)} onRetry={() => { void query.refetch(); void query.grant.refetch(); }}
    currentUrl={currentUrl ?? new URL(location.pathname + location.search + location.hash, window.location.origin).toString()} />;
  if (query.error || !agentId) return <AgentConsoleView state="error" onRetry={agentId ? () => { void query.refetch(); void query.grant.refetch(); } : undefined} />;
  return <AgentConsoleView state="loading" agentId={agentId} />;
}

function GrantEditor({ data, initialGrant, restoredDraft, currentUrl, stale, onRetry }: {
  data: AgentDetailData;
  initialGrant: UserGrant | null;
  restoredDraft?: ConsentDraftSnapshot;
  currentUrl: string;
  stale: boolean;
  onRetry: () => void;
}) {
  const { agent, services } = data;
  const navigate = useNavigate();
  const connections = useConnections();
  const [savedGrant, setSavedGrant] = useState(initialGrant);
  const observedGrant = useRef(initialGrant);
  const [draft, setDraft] = useState(() => createConsentDraft({ permissionSets: agent.permission_sets, serviceRequirements: agent.service_requirements, existingGrant: initialGrant, context: 'console', restoredDraft }));
  const [error, setError] = useState<string>();
  const [dateError, setDateError] = useState<string>();
  const [storageError, setStorageError] = useState(false);
  const errorRef = useRef<HTMLDivElement>(null);
  const headingRef = useRef<HTMLDivElement>(null);
  const revokeRef = useRef<HTMLButtonElement>(null);
  const submitting = useRef(false);
  const allowDeparture = useRef(false);
  const keepEditingRef = useRef<HTMLButtonElement>(null);
  const blocker = useBlocker(({ currentLocation, nextLocation }) => draft.dirty && !allowDeparture.current
    && (currentLocation.pathname !== nextLocation.pathname || currentLocation.search !== nextLocation.search || currentLocation.hash !== nextLocation.hash));
  useBeforeUnload((event) => {
    if (!draft.dirty || allowDeparture.current) return;
    event.preventDefault();
    event.returnValue = '';
  });
  const generation = useRef(getAuthGeneration());
  const mounted = useRef(false);
  const save = useSaveGrant(agent.agentId);
  const revoke = useRevokeGrant({ onSuccess: () => {
    if (mounted.current && generation.current === getAuthGeneration()) {
      toast.success(consentCopy.grantRevoked);
      allowDeparture.current = true;
      navigate('/agents');
    }
  } });
  const hasPermissionGroups = agent.permission_sets.length > 0;
  const selectedServices = new Set<string>();
  if (hasPermissionGroups) {
    for (const permissionId in draft.selections) {
      if (!Object.prototype.hasOwnProperty.call(draft.selections, permissionId)) continue;
      for (const serviceId of draft.selections[permissionId]!) selectedServices.add(serviceId);
    }
  }
  const missingServices = services.filter((service) => (!hasPermissionGroups || selectedServices.has(service.serviceId)) && !agent.active_session_service_ids.includes(service.serviceId));
  const blocked = hasPermissionGroups && missingServices.length > 0;
  const sessionsByService = new Map(connections.sessions.map(session => [session.service_id, session]));
  const rail: AgentConnection[] = services.map(service => {
    const session = sessionsByService.get(service.serviceId);
    return {
      service,
      state: session ? connections.getState(service.serviceId)
        : deriveConnectionState({ context: 'requirement', connectionStatus: service.connectionStatus, readFailed: Boolean(connections.error) }),
    };
  });

  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
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
    setError(undefined); setDateError(undefined);
    const selectionError = validateDraftSelection(draft);
    if (selectionError) { setError(selectionError); return; }
    submitting.current = true;
    try {
      const result = await save.mutateAsync(draft.toGrantRequest());
      save.assertCurrent();
      if (result.kind === 'redirect') {
        if (!isSafeRedirectUrl(result.redirectUrl)) { setError(consentCopy.invalidRedirect); return; }
        allowDeparture.current = true;
        window.location.href = result.redirectUrl;
      } else {
        setSavedGrant(result.grant);
        setDraft(createConsentDraft({ permissionSets: agent.permission_sets, serviceRequirements: agent.service_requirements, existingGrant: result.grant, context: 'console' }));
        toast.success(consentCopy.grantSaved);
        headingRef.current?.focus();
      }
    } catch (failure) {
      if (isCancelledError(failure)) return;
      if (failure instanceof InvalidGrantDateError) setDateError(consentCopy.invalidDate);
      else setError(consentErrorMessage(failure, consentCopy.saveError));
    } finally {
      submitting.current = false;
    }
  }

  return <><AgentConsoleView state="ready" data={data} draft={draft} savedGrant={savedGrant} stale={stale} onRetry={onRetry}
    error={error} dateError={dateError} pending={save.isPending} blocked={blocked} onSubmit={submit}
    onDraftChange={(next) => { setDraft(next); setError(undefined); }}
    onDurationChange={(next) => { setDraft(next); setDateError(undefined); setError(undefined); }}
    onCancel={() => { setDraft(createConsentDraft({ permissionSets: agent.permission_sets, serviceRequirements: agent.service_requirements, existingGrant: savedGrant, context: 'console' })); setError(undefined); setDateError(undefined); headingRef.current?.focus(); }}
    connections={rail} connectionLoading={connections.loading} connectionError={Boolean(connections.error)} connectionStorageError={storageError}
    onConnectionRetry={() => { void connections.refetch(); }}
    onConnect={(serviceId) => {
      allowDeparture.current = true;
      const started = startServiceLogin(serviceId, draft, currentUrl);
      if (!started) allowDeparture.current = false;
      setStorageError(!started);
    }}
    onRevoke={() => revoke.requestRevoke(agent)} revokeConfirmation={revoke.confirmation !== null} revokePending={revoke.isPending(agent.agentId)}
    revokeError={revoke.error ? consentErrorMessage(revoke.error, consentCopy.grantRevokeError) : undefined}
    onCancelRevoke={revoke.cancelRevoke} onConfirmRevoke={revoke.confirmRevoke} onReturnRevokeFocus={() => { if (revokeRef.current?.isConnected && !revokeRef.current.disabled) revokeRef.current.focus(); else headingRef.current?.focus(); }}
    headingRef={headingRef} errorRef={errorRef} revokeRef={revokeRef} />
    <Dialog open={blocker.state === 'blocked'} onOpenChange={(open) => { if (!open && blocker.state === 'blocked') blocker.reset(); }}>
      <DialogContent closeLabel={commonCopy.close} onOpenAutoFocus={(event) => { event.preventDefault(); keepEditingRef.current?.focus(); }}>
        <DialogHeader><DialogTitle>{consentCopy.discardTitle}</DialogTitle><DialogDescription>{consentCopy.discardDescription}</DialogDescription></DialogHeader>
        <DialogFooter>
          <Button ref={keepEditingRef} variant="secondary" onClick={() => { if (blocker.state === 'blocked') blocker.reset(); }}>{consentCopy.keepEditing}</Button>
          <Button variant="destructive" onClick={() => { if (blocker.state === 'blocked') blocker.proceed(); }}>{consentCopy.discardChanges}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </>;
}

export default AgentConsolePage;
