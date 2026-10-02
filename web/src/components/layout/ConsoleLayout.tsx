import { lazy, Suspense, useCallback, useState } from 'react';
import type { ReactNode } from 'react';
import { Outlet } from 'react-router-dom';
import { Bot, Plug, Search, Settings, ShieldCheck } from 'lucide-react';
import { ConsoleShell } from '@design-system/components/layout/ConsoleShell/ConsoleShell';
import { PendingApprovalsProvider, usePendingApprovals } from '@hooks/usePendingApprovals';
import { useCommandShortcut } from '@hooks/useCommandShortcut';
import { Button } from '@design-system/components/primitives/Button/Button';
import { Skeleton } from '@design-system/components/feedback/Skeleton/Skeleton';
import { Toaster } from '@design-system/components/feedback/Toaster/Toaster';
import { ApprovalArrivalAnnouncer } from '@components/approvals/ApprovalArrivalAnnouncer';
import UserMenu from './UserMenu';
import { commonCopy, navigationCopy } from '@copy';

// Search is loaded only after an explicit console action, never with decision routes.
const CommandPalette = lazy(() => import('@components/command/CommandPalette'));

const navigation = [
  { href: '/delegations', label: navigationCopy.agents, icon: <Bot aria-hidden="true" /> },
  { href: '/sessions', label: navigationCopy.connections, icon: <Plug aria-hidden="true" /> },
  { href: '/approvals', label: navigationCopy.approvals, icon: <ShieldCheck aria-hidden="true" />, pendingApprovals: true },
  { href: '/settings', label: navigationCopy.settings, icon: <Settings aria-hidden="true" /> },
];

function ConsoleFrame({ children }: { children?: ReactNode }) {
  const [commandOpen, setCommandOpen] = useState(false);
  const [commandMounted, setCommandMounted] = useState(false);
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false);
  const toggleCommand = useCallback(() => {
    setCommandMounted(true);
    setCommandOpen(open => !open);
  }, []);
  useCommandShortcut(toggleCommand);
  const pending = usePendingApprovals();
  return (
    <ConsoleShell
      navigation={navigation}
      mobileNavigationOpen={mobileNavigationOpen}
      onMobileNavigationOpenChange={setMobileNavigationOpen}
      search={<Button variant="outline" className="w-full justify-start group-data-[state=collapsed]/sidebar:justify-center" aria-label={navigationCopy.search} aria-keyshortcuts="Control+K Meta+K" onClick={toggleCommand}>
        <Search aria-hidden="true" />
        <span className="group-data-[state=collapsed]/sidebar:hidden">{navigationCopy.search}</span>
      </Button>}
      userMenu={<UserMenu />}
      announcement={<ApprovalArrivalAnnouncer />}
      pendingCount={pending.count}
      pendingStale={pending.stale}
      labels={{
        wordmark: commonCopy.brand,
        skipToMain: navigationCopy.skipToContent,
        navigation: navigationCopy.mainNavigation,
        collapseSidebar: navigationCopy.collapseSidebar,
        expandSidebar: navigationCopy.expandSidebar,
        openNavigation: navigationCopy.openNavigation,
        closeNavigation: navigationCopy.closeNavigation,
        navigationDescription: navigationCopy.navigationDescription,
        pendingCount: navigationCopy.pendingCount,
        pendingUnknown: pending.isError ? navigationCopy.pendingUnavailable : navigationCopy.pendingLoading,
        pendingStale: navigationCopy.pendingStale,
      }}
    >
      {children ?? <Outlet />}
      {commandMounted && <Suspense fallback={<Skeleton label={commonCopy.loading} />}>
        <CommandPalette open={commandOpen} onOpenChange={setCommandOpen} onNavigate={() => setMobileNavigationOpen(false)} />
      </Suspense>}
    </ConsoleShell>
  );
}

export default function ConsoleLayout({ children }: { children?: ReactNode }) {
  return (
    <PendingApprovalsProvider>
      <Toaster label={commonCopy.notifications} closeButtonLabel={commonCopy.close} />
      <ConsoleFrame>{children}</ConsoleFrame>
    </PendingApprovalsProvider>
  );
}
