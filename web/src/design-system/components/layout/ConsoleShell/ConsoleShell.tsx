import { useCallback, useEffect, useId, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from 'react';
import { NavLink } from 'react-router-dom';
import { AlertTriangle, Menu, PanelLeftClose, PanelLeftOpen } from 'lucide-react';
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
  pendingCountDisplay?: ReactNode;
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
  children, navigation, search, userMenu, pendingCount, pendingCountDisplay, pendingStale = false, announcement, labels,
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
  const mobileQuery = useMemo(() => window.matchMedia('(width < 63.25rem)'), []);
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
  const toggleCollapsed = useCallback(() => {
    const next = !collapsed;
    setCollapsed(next);
    try {
      window.localStorage.setItem(SIDEBAR_STORAGE_KEY, String(next));
    } catch {
      // Collapse remains available when browser storage is blocked.
    }
  }, [collapsed]);
  useEffect(() => {
    if (isMobile) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.repeat || event.key.toLowerCase() !== 'b' || !(event.metaKey || event.ctrlKey) || event.altKey || event.shiftKey) return;
      if (event.target instanceof Element && event.target.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="textbox"]')) return;
      event.preventDefault();
      toggleCollapsed();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [isMobile, toggleCollapsed]);
  const pendingDescription = pendingCount === undefined
    ? labels.pendingUnknown
    : `${labels.pendingCount(pendingCount)}${pendingStale ? ` ${labels.pendingStale}` : ''}`;
  const pendingNotice = pendingStale ? labels.pendingStale : undefined;

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
                  data-animated-icon-host
                  aria-label={item.label}
                  aria-describedby={item.pendingApprovals ? descriptionId : undefined}
                  onClick={() => setMobileOpen(false)}
                  className={({ isActive }) => cn(
                    'relative flex h-10 min-w-0 items-center gap-3 rounded-md px-2 text-sm font-medium transition-colors duration-(--motion-feedback) ease-(--motion-ease) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:ring-offset-2 focus-visible:ring-offset-sidebar',
                    isActive ? 'bg-primary-soft text-primary-soft-foreground hover:bg-primary-soft' : 'hover:bg-accent hover:text-accent-foreground',
                    compact && 'justify-center',
                    item.pendingApprovals && !compact && 'pr-9',
                  )}
                >
                  <span aria-hidden="true" className="shrink-0 [&_svg]:size-5">{item.icon}</span>
                  <span className={compact ? 'sr-only' : 'min-w-0 flex-1 truncate'}>{item.label}</span>
                  {item.pendingApprovals && (
                    <>
                      <span id={descriptionId} className="sr-only">{pendingDescription}</span>
                      {pendingCount !== undefined && pendingCount > 0 && (
                        <span
                          data-testid="pending-approval-count"
                          aria-hidden="true"
                          className={cn('inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded-md bg-primary-soft px-1 text-xs font-medium tabular-nums text-primary-soft-foreground', compact && 'absolute right-0 top-0')}
                        >
                          {pendingCountDisplay ?? pendingCount}
                        </span>
                      )}
                    </>
                  )}
                </NavLink>
              );
              return (
                <li key={item.href} className="relative">
                  {compact ? (
                    <Tooltip>
                      <TooltipTrigger asChild>{link}</TooltipTrigger>
                      <TooltipContent side="right">
                        {item.label}
                        {item.pendingApprovals && <span className="block text-xs">{pendingDescription}</span>}
                      </TooltipContent>
                    </Tooltip>
                  ) : link}
                  {item.pendingApprovals && pendingNotice && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button type="button" aria-label={pendingNotice} className={cn('absolute right-1 top-1/2 flex size-6 -translate-y-1/2 items-center justify-center rounded-md text-warning-foreground hover:bg-warning focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring', compact && 'bottom-0 right-0 top-auto size-4 translate-y-0')}>
                          <AlertTriangle aria-hidden="true" className="size-4" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side={compact ? 'right' : 'top'}>{pendingNotice}</TooltipContent>
                    </Tooltip>
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
        className="sr-only focus-visible:not-sr-only focus-visible:fixed focus-visible:left-4 focus-visible:top-4 focus-visible:z-100 focus-visible:rounded-md focus-visible:bg-background focus-visible:px-4 focus-visible:py-2 focus-visible:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {labels.skipToMain}
      </a>
      {isMobile ? (
        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <header className="flex min-w-0 items-center gap-3 border-b border-border-subtle bg-sidebar px-4 py-3">
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
          className={cn('group/sidebar shrink-0 border-r border-sidebar-border bg-sidebar text-sidebar-foreground', collapsed ? 'w-14' : 'w-60')}
        >
          <div className="sticky top-0 flex h-dvh flex-col gap-4 overflow-y-auto p-2">
          {compact ? (
            <div className="flex h-10 items-center justify-center">
              <Button variant="ghost" size="icon" aria-label={labels.expandSidebar} aria-keyshortcuts="Control+B Meta+B" aria-expanded={false} aria-controls={sidebarId} className="group size-7" onClick={toggleCollapsed}>
                <Wordmark label={labels.wordmark} compact className="h-4 group-hover:hidden group-focus-visible:hidden" />
                <PanelLeftOpen aria-hidden="true" className="hidden group-hover:block group-focus-visible:block" />
              </Button>
            </div>
          ) : (
            <div className="flex h-10 min-w-0 items-center justify-between pl-2">
              <Wordmark label={labels.wordmark} className="min-w-0 shrink" />
              <Button variant="ghost" size="icon" aria-label={labels.collapseSidebar} aria-keyshortcuts="Control+B Meta+B" aria-expanded={true} aria-controls={sidebarId} className="size-7" onClick={toggleCollapsed}>
                <PanelLeftClose aria-hidden="true" />
              </Button>
            </div>
          )}
          {navigationContent}
          </div>
        </aside>
      )}
      <main
        id={mainId}
        ref={mainRef}
        tabIndex={-1}
        className="min-w-0 flex-1 bg-console-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      >
        <div className="mx-auto w-full max-w-[1120px] px-4 py-6 md:px-6">{children}</div>
      </main>
      <div data-testid="approval-announcement" aria-live="polite" aria-atomic="true" className="sr-only">{announcement}</div>
    </div>
  );
}
