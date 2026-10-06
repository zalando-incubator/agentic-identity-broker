import type { ReactNode } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { Bot, CheckCheck, Plug } from 'lucide-react';
import { ConsoleShell } from '@design-system/components/layout/ConsoleShell/ConsoleShell';
import { DecisionShell } from '@design-system/components/layout/DecisionShell/DecisionShell';
import { commonCopy, navigationCopy } from '@copy';

const navigation = [
  { href: '/agents', label: navigationCopy.agents, icon: <Bot aria-hidden="true" /> },
  { href: '/connections', label: navigationCopy.connections, icon: <Plug aria-hidden="true" /> },
  { href: '/approvals', label: navigationCopy.approvals, icon: <CheckCheck aria-hidden="true" />, pendingApprovals: true },
];

export function ConsoleStoryShell({ path, children }: { path: string; children: ReactNode }) {
  return <MemoryRouter initialEntries={[path]}>
    <ConsoleShell navigation={navigation} search={null} userMenu={null} pendingCount={undefined} labels={{
      wordmark: commonCopy.brand,
      skipToMain: navigationCopy.skipToContent,
      navigation: navigationCopy.mainNavigation,
      collapseSidebar: navigationCopy.collapseSidebar,
      expandSidebar: navigationCopy.expandSidebar,
      openNavigation: navigationCopy.openNavigation,
      closeNavigation: navigationCopy.closeNavigation,
      navigationDescription: navigationCopy.navigationDescription,
      pendingCount: navigationCopy.pendingCount,
      pendingUnknown: navigationCopy.pendingLoading,
      pendingStale: navigationCopy.pendingStale,
    }}>{children}</ConsoleShell>
  </MemoryRouter>;
}

export function DecisionStoryShell({ path, children, compact }: {
  path: string; children: ReactNode; compact?: boolean;
}) {
  return <MemoryRouter initialEntries={[path]}>
    <DecisionShell wordmarkLabel={commonCopy.brand} skipToMainLabel={navigationCopy.skipToContent} compact={compact}>
      {children}
    </DecisionShell>
  </MemoryRouter>;
}
