import { lazy, useLayoutEffect, useMemo } from 'react';
import { useLocation } from 'react-router-dom';
import { DecisionShell } from '@design-system/components/layout/DecisionShell/DecisionShell';
import { commonCopy, navigationCopy } from '@copy';
import { loadConsentDraft } from '@services/storage/session';

// Separate route chunks prevent console-only dependencies entering an authorization decision.
const AgentDecisionPage = lazy(() => import('../../pages/AgentDecisionPage'));
const AgentConsolePage = lazy(() => import('../../pages/AgentConsolePage'));
const ConsoleLayout = lazy(() => import('./ConsoleLayout'));

export default function AgentRoute() {
  const location = useLocation();
  const callback = useMemo(() => {
    const params = new URLSearchParams(location.search);
    if (params.get('success') !== 'true') return undefined;
    const stateID = params.get('consent_state_id');
    const serviceID = params.get('service_id');
    if (!stateID || !serviceID) return undefined;
    const draft = loadConsentDraft(stateID, serviceID, location.pathname);
    if (!draft) return undefined;
    const returnURL = new URL(draft.returnURL);
    returnURL.searchParams.delete('consent_state_id');
    returnURL.searchParams.set('success', 'true');
    returnURL.searchParams.set('service_id', serviceID);
    return { draft, returnURL };
  }, [location.pathname, location.search]);

  useLayoutEffect(() => {
    if (callback) {
      const { pathname, search, hash } = callback.returnURL;
      window.history.replaceState(window.history.state, '', `${pathname}${search}${hash}`);
    }
  }, [callback]);

  const params = callback?.returnURL.searchParams ?? new URLSearchParams(location.search);
  const currentUrl = callback?.returnURL.toString()
    ?? new URL(`${location.pathname}${location.search}${location.hash}`, window.location.origin).toString();
  if (params.has('session_token')) {
    return (
      <DecisionShell wordmarkLabel={commonCopy.brand} skipToMainLabel={navigationCopy.skipToContent} footerLabel={commonCopy.poweredBy}>
        <AgentDecisionPage currentUrl={currentUrl} restoredDraft={callback?.draft} sessionToken={params.get('session_token') ?? ''} />
      </DecisionShell>
    );
  }
  return <ConsoleLayout><AgentConsolePage currentUrl={currentUrl} restoredDraft={callback?.draft} /></ConsoleLayout>;
}
