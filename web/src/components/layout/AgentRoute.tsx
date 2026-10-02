import { lazy } from 'react';
import { useSearchParams } from 'react-router-dom';
import { DecisionShell } from '@design-system/components/layout/DecisionShell/DecisionShell';
import { commonCopy, navigationCopy } from '@copy';

// Separate route chunks prevent console-only dependencies entering an authorization decision.
const AgentDecisionPage = lazy(() => import('../../pages/AgentDecisionPage'));
const AgentConsolePage = lazy(() => import('../../pages/AgentConsolePage'));
const ConsoleLayout = lazy(() => import('./ConsoleLayout'));

export default function AgentRoute() {
  const [parameters] = useSearchParams();
  if (parameters.has('session_token')) {
    return (
      <DecisionShell wordmarkLabel={commonCopy.brand} skipToMainLabel={navigationCopy.skipToContent} footerLabel={commonCopy.poweredBy}>
        <AgentDecisionPage />
      </DecisionShell>
    );
  }
  return <ConsoleLayout><AgentConsolePage /></ConsoleLayout>;
}
