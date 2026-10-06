import { lazy, Suspense, useCallback, useState } from 'react';
import type { ReactNode } from 'react';
import { Outlet } from 'react-router-dom';
import { Search } from 'lucide-react';
import { MotionConfig } from 'motion/react';
import { AnimatedBotIcon, AnimatedConnectionIcon, AnimatedShieldCheckIcon } from '@components/icons/AnimatedIcons';
import { ConsoleShell } from '@design-system/components/layout/ConsoleShell/ConsoleShell';
import { PendingApprovalsProvider, usePendingApprovals } from '@hooks/usePendingApprovals';
import { useCommandShortcut } from '@hooks/useCommandShortcut';
import { Button } from '@design-system/components/primitives/Button/Button';
import { Skeleton } from '@design-system/components/feedback/Skeleton/Skeleton';
import { Toaster } from '@design-system/components/feedback/Toaster/Toaster';
import { ApprovalArrivalAnnouncer } from '@components/approvals/ApprovalArrivalAnnouncer';
import { PendingApprovalCount, usePendingCountBump } from '@components/approvals/PendingApprovalCount';
import UserMenu from './UserMenu';
import { commonCopy, navigationCopy } from '@copy';

// Search is loaded only after an explicit console action, never with decision routes.
const CommandPalette = lazy(() => import('@components/command/CommandPalette'));

const navigation = [
  { href: '/agents', label: navigationCopy.agents, icon: <AnimatedBotIcon /> },
  { href: '/connections', label: navigationCopy.connections, icon: <AnimatedConnectionIcon /> },
  { href: '/approvals', label: navigationCopy.approvals, icon: <AnimatedShieldCheckIcon />, pendingApprovals: true },
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
  const countBump = usePendingCountBump(pending.data, pending.stale);
  const shortcutHint = /Mac/i.test(navigator.platform) ? navigationCopy.searchMacShortcut : navigationCopy.searchControlShortcut;
  return (
    <ConsoleShell
      navigation={navigation}
      mobileNavigationOpen={mobileNavigationOpen}
      onMobileNavigationOpenChange={setMobileNavigationOpen}
      search={<Button variant="ghost" className="w-full justify-start text-muted-foreground group-data-[state=collapsed]/sidebar:justify-center" aria-label={navigationCopy.search} aria-keyshortcuts="Control+K Meta+K" onClick={toggleCommand}>
        <Search aria-hidden="true" />
        <span className="group-data-[state=collapsed]/sidebar:hidden">{navigationCopy.search}</span>
        <kbd aria-hidden="true" className="ml-auto text-xs group-data-[state=collapsed]/sidebar:hidden">{shortcutHint}</kbd>
      </Button>}
      userMenu={<UserMenu />}
      announcement={<ApprovalArrivalAnnouncer />}
      pendingCount={pending.count}
      pendingCountDisplay={pending.count === undefined ? undefined : <PendingApprovalCount count={pending.count} bumpRevision={countBump} />}
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
      {children ?? <Suspense fallback={<Skeleton label={commonCopy.loading} count={3} />}><Outlet /></Suspense>}
      {commandMounted && <Suspense fallback={<Skeleton label={commonCopy.loading} />}>
        <CommandPalette open={commandOpen} onOpenChange={setCommandOpen} onNavigate={() => setMobileNavigationOpen(false)} />
      </Suspense>}
    </ConsoleShell>
  );
}

export default function ConsoleLayout({ children }: { children?: ReactNode }) {
  return (
    <MotionConfig reducedMotion="user">
    <PendingApprovalsProvider>
      <Toaster label={commonCopy.notifications} closeButtonLabel={commonCopy.close} />
      <ConsoleFrame>{children}</ConsoleFrame>
    </PendingApprovalsProvider>
    </MotionConfig>
  );
}
