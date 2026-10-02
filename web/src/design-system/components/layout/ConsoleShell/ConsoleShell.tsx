import { useCallback, useId, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from 'react';
import { NavLink } from 'react-router-dom';
import { AlertCircle, Menu, PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button/Button';
import { Wordmark } from '@design-system/components/primitives/Wordmark/Wordmark';
import { Sheet, SheetContent, SheetDescription, SheetTitle, SheetTrigger } from '@design-system/components/overlays/Sheet/Sheet';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@design-system/components/overlays/Tooltip/Tooltip';
import { parseSidebarCollapsed, SIDEBAR_STORAGE_KEY } from '@design-system/theme/themePreference';
import { cn } from '@design-system/utils/cn';

export interface ConsoleNavigationItem {
  href: string;
  label: string;
  icon: ReactNode;
  pendingApprovals?: boolean;
}
export interface ConsoleShellProps {
  children: ReactNode;
  navigation: readonly ConsoleNavigationItem[];
  search: ReactNode;
  userMenu: ReactNode;
  pendingCount: number | undefined;
  pendingStale?: boolean;
  announcement?: ReactNode;
  mobileNavigationOpen?: boolean;
  onMobileNavigationOpenChange?: (open: boolean) => void;
  labels: {
    wordmark: string;
    skipToMain: string;
    navigation: string;
    collapseSidebar: string;
    expandSidebar: string;
    openNavigation: string;
    closeNavigation: string;
    navigationDescription: string;
    pendingCount: (count: number) => string;
    pendingUnknown: string;
    pendingStale: string;
  };
}
export function ConsoleShell({
  children, navigation, search, userMenu, pendingCount, pendingStale = false, announcement, labels,
  mobileNavigationOpen, onMobileNavigationOpenChange,
}: ConsoleShellProps) {
  const mainId = useId();
  const sidebarId = useId();
  const countDescriptionId = useId();
  const mainRef = useRef<HTMLElement>(null);
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return parseSidebarCollapsed(window.localStorage.getItem(SIDEBAR_STORAGE_KEY));
    } catch {
      return false;
    }
  });
  const [internalMobileOpen, setInternalMobileOpen] = useState(false);
  const controlledNavigation = mobileNavigationOpen !== undefined;
  const mobileOpen = mobileNavigationOpen ?? internalMobileOpen;
  const setMobileOpen = useCallback((open: boolean) => {
    if (!controlledNavigation) setInternalMobileOpen(open);
    onMobileNavigationOpenChange?.(open);
  }, [controlledNavigation, onMobileNavigationOpenChange]);
  const mobileQuery = useMemo(() => window.matchMedia('(width < 48rem)'), []);
  const subscribe = useCallback((onChange: () => void) => {
    const onBreakpointChange = () => {
      setMobileOpen(false);
      onChange();
    };
    mobileQuery.addEventListener('change', onBreakpointChange);
    return () => mobileQuery.removeEventListener('change', onBreakpointChange);
  }, [mobileQuery, setMobileOpen]);
  const getMobile = useCallback(() => mobileQuery.matches, [mobileQuery]);
  const isMobile = useSyncExternalStore(subscribe, getMobile);
  const compact = collapsed && !isMobile;
  const pendingDescription = pendingCount === undefined
    ? labels.pendingUnknown
    : `${labels.pendingCount(pendingCount)}${pendingStale ? ` ${labels.pendingStale}` : ''}`;
  const pendingNotice = pendingCount === undefined ? labels.pendingUnknown : pendingStale ? labels.pendingStale : undefined;

  const navigationContent = (
    <>
      <div className="min-w-0">{search}</div>
      <TooltipProvider>
        <nav aria-label={labels.navigation}>
          <ul className="space-y-1">
            {navigation.map((item, index) => {
              const descriptionId = `${countDescriptionId}-${index}`;
              const link = (
                <NavLink
                  to={item.href}
                  aria-label={item.label}
                  aria-describedby={item.pendingApprovals ? descriptionId : undefined}
                  onClick={() => setMobileOpen(false)}
                  className={({ isActive }) => cn(
                    'relative flex min-h-10 min-w-0 items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors duration-(--motion-feedback) ease-(--motion-ease) hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:ring-offset-2 focus-visible:ring-offset-sidebar motion-reduce:transition-none',
                    isActive && 'bg-sidebar-accent text-sidebar-accent-foreground',
                    compact && 'justify-center px-2',
                  )}
                >
                  <span aria-hidden="true" className="shrink-0 [&_svg]:size-5">{item.icon}</span>
                  <span className={compact ? 'sr-only' : 'min-w-0 flex-1 break-words'}>{item.label}</span>
                  {item.pendingApprovals && (
                    <>
                      <span id={descriptionId} className="sr-only">{pendingDescription}</span>
                      {pendingCount !== undefined && (
                        <span
                          data-testid="pending-approval-count"
                          aria-hidden="true"
                          className={cn('min-w-5 rounded-full bg-muted px-1 text-center text-xs tabular-nums text-muted-foreground', compact && 'absolute right-0 top-0')}
                        >
                          {pendingCount}
                        </span>
                      )}
                      {pendingNotice && <AlertCircle aria-hidden="true" className={cn('size-4 shrink-0 text-muted-foreground', compact && 'absolute bottom-0 right-0')} />}
                    </>
                  )}
                </NavLink>
              );
              return (
                <li key={item.href}>
                  {compact ? (
                    <Tooltip>
                      <TooltipTrigger asChild>{link}</TooltipTrigger>
                      <TooltipContent side="right">
                        {item.label}
                        {item.pendingApprovals && <span className="block text-xs">{pendingDescription}</span>}
                      </TooltipContent>
                    </Tooltip>
                  ) : link}
                  {item.pendingApprovals && pendingNotice && !compact && (
                    <p aria-hidden="true" className="px-3 pt-1 text-xs text-muted-foreground">{pendingNotice}</p>
                  )}
                </li>
              );
            })}
          </ul>
        </nav>
      </TooltipProvider>
      <div className="mt-auto min-w-0 pt-4">{userMenu}</div>
    </>
  );

  return (
    <div className={cn('min-h-dvh min-w-0 bg-background text-foreground', !isMobile && 'flex')}>
      <a
        href={`#${mainId}`}
        onClick={(event) => {
          event.preventDefault();
          mainRef.current?.focus();
        }}
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-background focus:px-4 focus:py-2 focus:text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
      >
        {labels.skipToMain}
      </a>
      {isMobile ? (
        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <header className="flex min-w-0 items-center gap-3 border-b border-border px-4 py-3">
            <SheetTrigger asChild>
              <Button variant="ghost" size="icon" aria-label={labels.openNavigation}><Menu aria-hidden="true" /></Button>
            </SheetTrigger>
            <Wordmark label={labels.wordmark} className="min-w-0" />
          </header>
          <SheetContent side="left" closeLabel={labels.closeNavigation} className="bg-sidebar text-sidebar-foreground">
            <SheetTitle className="sr-only">{labels.navigation}</SheetTitle>
            <SheetDescription className="sr-only">{labels.navigationDescription}</SheetDescription>
            <div data-testid="console-sidebar" data-state="expanded" className="group/sidebar flex min-h-full min-w-0 flex-col gap-4">
              <div className="pr-5"><Wordmark label={labels.wordmark} /></div>
              {navigationContent}
            </div>
          </SheetContent>
        </Sheet>
      ) : (
        <aside
          id={sidebarId}
          data-testid="console-sidebar"
          data-state={collapsed ? 'collapsed' : 'expanded'}
          className={cn('group/sidebar sticky top-0 flex h-dvh shrink-0 flex-col gap-4 overflow-y-auto border-r border-sidebar-border bg-sidebar p-3 text-sidebar-foreground', collapsed ? 'w-20' : 'w-64')}
        >
          <div className={cn('flex min-h-10 items-center', compact && 'justify-center')}>
            <Wordmark label={labels.wordmark} compact={compact} />
          </div>
          <Button
            variant="ghost"
            size={compact ? 'icon' : 'default'}
            aria-label={collapsed ? labels.expandSidebar : labels.collapseSidebar}
            aria-expanded={!collapsed}
            aria-controls={sidebarId}
            className={compact ? 'self-center' : 'justify-start'}
            onClick={() => {
              const next = !collapsed;
              setCollapsed(next);
              try {
                window.localStorage.setItem(SIDEBAR_STORAGE_KEY, String(next));
              } catch {
                // The current view stays usable when browser storage is blocked.
              }
            }}
          >
            {collapsed ? <PanelLeftOpen aria-hidden="true" /> : <PanelLeftClose aria-hidden="true" />}
            {!compact && <span>{labels.collapseSidebar}</span>}
          </Button>
          {navigationContent}
        </aside>
      )}
      <main
        id={mainId}
        ref={mainRef}
        tabIndex={-1}
        className="min-w-0 flex-1 p-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring md:p-6"
      >
        {children}
      </main>
      <div data-testid="approval-announcement" aria-live="polite" aria-atomic="true" className="sr-only">{announcement}</div>
    </div>
  );
}
