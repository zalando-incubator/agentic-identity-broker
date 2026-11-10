/**
 * AppLayout Component
 *
 * Full-page application layout with optional header, sidebar, footer, and main content.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Flexible grid-based layout system
 * - Optional header, sidebar, footer sections
 * - Sidebar positioning (left/right)
 * - Sidebar width variants (sm, md, lg)
 * - Collapsible sidebar with toggle
 * - Sticky header and footer options
 * - Responsive design with mobile drawer pattern
 * - Fully accessible with semantic HTML
 */

import React, { useState, useEffect } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { ChevronLeft, ChevronRight, Menu, X } from 'lucide-react';

const appLayoutVariants = cva(
  // Base styles - full viewport height, overflow management
  'min-h-screen w-full flex flex-col',
  {
    variants: {
      // No variant needed for outer container
    },
    defaultVariants: {},
  },
);

const headerVariants = cva(
  // Base header styles
  'w-full bg-white border-b border-neutral-200 z-20',
  {
    variants: {
      sticky: {
        true: 'sticky top-0',
        false: 'relative',
      },
    },
    defaultVariants: {
      sticky: false,
    },
  },
);

const mainContainerVariants = cva(
  // Main container grows to fill available space
  'flex-1 flex w-full overflow-hidden',
  {
    variants: {},
    defaultVariants: {},
  },
);

const sidebarVariants = cva(
  // Base sidebar styles
  'bg-neutral-50 border-neutral-200 flex-shrink-0 transition-all duration-300 ease-in-out overflow-y-auto z-10',
  {
    variants: {
      position: {
        left: 'border-r',
        right: 'border-l',
      },
      width: {
        sm: '', // Will be set via style attr: 200px
        md: '', // 256px
        lg: '', // 320px
      },
      collapsed: {
        true: 'w-0 border-0',
        false: '',
      },
      mobile: {
        // Mobile drawer overlay pattern
        drawer: 'fixed inset-y-0 shadow-xl md:relative md:shadow-none',
        standard: 'hidden md:block',
      },
    },
    compoundVariants: [
      {
        position: 'left',
        mobile: 'drawer',
        className: 'left-0',
      },
      {
        position: 'right',
        mobile: 'drawer',
        className: 'right-0',
      },
    ],
    defaultVariants: {
      position: 'left',
      width: 'md',
      collapsed: false,
      mobile: 'standard',
    },
  },
);

const contentVariants = cva(
  // Main content area
  'flex-1 overflow-auto bg-white',
  {
    variants: {},
    defaultVariants: {},
  },
);

const footerVariants = cva(
  // Base footer styles
  'w-full bg-white border-t border-neutral-200 z-20',
  {
    variants: {
      sticky: {
        true: 'sticky bottom-0',
        false: 'relative',
      },
    },
    defaultVariants: {
      sticky: false,
    },
  },
);

const overlayVariants = cva(
  // Mobile overlay backdrop for drawer
  'fixed inset-0 bg-black/50 z-[5] md:hidden transition-opacity duration-300',
  {
    variants: {
      visible: {
        true: 'opacity-100',
        false: 'opacity-0 pointer-events-none',
      },
    },
    defaultVariants: {
      visible: false,
    },
  },
);

export interface AppLayoutProps
  extends
    React.ComponentPropsWithoutRef<'div'>,
    VariantProps<typeof appLayoutVariants> {
  /** Header content (optional) */
  header?: React.ReactNode;
  /** Sidebar content (optional) */
  sidebar?: React.ReactNode;
  /** Sidebar position */
  sidebarPosition?: 'left' | 'right';
  /** Sidebar width variant */
  sidebarWidth?: 'sm' | 'md' | 'lg';
  /** Enable collapsible sidebar */
  sidebarCollapsible?: boolean;
  /** Callback when sidebar is toggled */
  onSidebarToggle?: (collapsed: boolean) => void;
  /** Make header sticky at top */
  stickyHeader?: boolean;
  /** Make footer sticky at bottom */
  stickyFooter?: boolean;
  /** Footer content (optional) */
  footer?: React.ReactNode;
  /** Main content */
  children: React.ReactNode;
  /** Use drawer pattern on mobile (overlay instead of hidden) */
  mobileDrawer?: boolean;
  /** Controlled collapsed state (for external control) */
  collapsed?: boolean;
  /** Initial collapsed state (for uncontrolled) */
  defaultCollapsed?: boolean;
}

/**
 * AppLayout component for full-page application layouts.
 * Provides flexible container with optional header, sidebar, content, and footer.
 *
 * @example
 * ```tsx
 * // Basic layout with all sections
 * <AppLayout
 *   header={<Header />}
 *   sidebar={<Navigation />}
 *   footer={<Footer />}
 * >
 *   <main>Content here</main>
 * </AppLayout>
 *
 * // Collapsible sidebar with sticky header
 * <AppLayout
 *   header={<Header />}
 *   sidebar={<Navigation />}
 *   sidebarCollapsible
 *   stickyHeader
 *   onSidebarToggle={(collapsed) => console.log('Sidebar:', collapsed)}
 * >
 *   <main>Content here</main>
 * </AppLayout>
 *
 * // Right sidebar with mobile drawer
 * <AppLayout
 *   sidebar={<Filters />}
 *   sidebarPosition="right"
 *   sidebarWidth="sm"
 *   mobileDrawer
 * >
 *   <main>Content here</main>
 * </AppLayout>
 * ```
 */
export const AppLayout = React.forwardRef<HTMLDivElement, AppLayoutProps>(
  (
    {
      header,
      sidebar,
      sidebarPosition = 'left',
      sidebarWidth = 'md',
      sidebarCollapsible = false,
      onSidebarToggle,
      stickyHeader = false,
      stickyFooter = false,
      footer,
      children,
      className,
      mobileDrawer = false,
      collapsed: controlledCollapsed,
      defaultCollapsed = false,
      ...props
    },
    ref,
  ) => {
    // Manage collapsed state (controlled or uncontrolled)
    const [internalCollapsed, setInternalCollapsed] =
      useState(defaultCollapsed);
    const isCollapsed =
      controlledCollapsed !== undefined
        ? controlledCollapsed
        : internalCollapsed;

    // Mobile drawer state
    const [mobileOpen, setMobileOpen] = useState(false);

    // Close mobile drawer when window resizes to desktop
    useEffect(() => {
      const handleResize = () => {
        if (window.innerWidth >= 768) {
          setMobileOpen(false);
        }
      };

      window.addEventListener('resize', handleResize);
      return () => window.removeEventListener('resize', handleResize);
    }, []);

    // Handle sidebar toggle
    const handleToggle = () => {
      const newCollapsed = !isCollapsed;
      setInternalCollapsed(newCollapsed);
      onSidebarToggle?.(newCollapsed);
    };

    // Handle mobile drawer toggle
    const handleMobileToggle = () => {
      setMobileOpen(!mobileOpen);
    };

    // Close mobile drawer on overlay click
    const handleOverlayClick = () => {
      setMobileOpen(false);
    };

    // Sidebar width mapping
    const sidebarWidthMap = {
      sm: 200,
      md: 256,
      lg: 320,
    };

    const sidebarWidthValue = sidebarWidthMap[sidebarWidth];

    // Determine mobile pattern
    const mobilePattern = mobileDrawer ? 'drawer' : 'standard';

    return (
      <div ref={ref} className={cn(appLayoutVariants(), className)} {...props}>
        {/* Header section */}
        {header && (
          <header
            className={cn(headerVariants({ sticky: stickyHeader }))}
            role="banner"
          >
            {header}
          </header>
        )}

        {/* Main content container with sidebar */}
        <div className={cn(mainContainerVariants())}>
          {/* Sidebar (order depends on position) */}
          {sidebar && sidebarPosition === 'left' && (
            <>
              {/* Mobile overlay backdrop */}
              {mobileDrawer && (
                <div
                  className={cn(overlayVariants({ visible: mobileOpen }))}
                  onClick={handleOverlayClick}
                  aria-hidden="true"
                />
              )}

              <aside
                className={cn(
                  sidebarVariants({
                    position: 'left',
                    width: sidebarWidth,
                    collapsed: isCollapsed && !mobileDrawer,
                    mobile: mobilePattern,
                  }),
                  // Mobile drawer: show when open, hide when closed
                  mobileDrawer &&
                    (mobileOpen
                      ? 'translate-x-0'
                      : '-translate-x-full md:translate-x-0'),
                )}
                style={{
                  width:
                    isCollapsed && !mobileDrawer
                      ? 0
                      : mobileDrawer && !mobileOpen
                        ? 0
                        : sidebarWidthValue,
                  maxWidth: sidebarWidthValue,
                }}
                aria-label="Sidebar navigation"
                aria-hidden={isCollapsed}
              >
                {sidebar}
              </aside>
            </>
          )}

          {/* Main content area */}
          <main className={cn(contentVariants())} role="main">
            {children}
          </main>

          {/* Sidebar on right */}
          {sidebar && sidebarPosition === 'right' && (
            <>
              {/* Mobile overlay backdrop */}
              {mobileDrawer && (
                <div
                  className={cn(overlayVariants({ visible: mobileOpen }))}
                  onClick={handleOverlayClick}
                  aria-hidden="true"
                />
              )}

              <aside
                className={cn(
                  sidebarVariants({
                    position: 'right',
                    width: sidebarWidth,
                    collapsed: isCollapsed && !mobileDrawer,
                    mobile: mobilePattern,
                  }),
                  // Mobile drawer: show when open, hide when closed
                  mobileDrawer &&
                    (mobileOpen
                      ? 'translate-x-0'
                      : 'translate-x-full md:translate-x-0'),
                )}
                style={{
                  width:
                    isCollapsed && !mobileDrawer
                      ? 0
                      : mobileDrawer && !mobileOpen
                        ? 0
                        : sidebarWidthValue,
                  maxWidth: sidebarWidthValue,
                }}
                aria-label="Sidebar"
                aria-hidden={isCollapsed}
              >
                {sidebar}
              </aside>
            </>
          )}
        </div>

        {/* Footer section */}
        {footer && (
          <footer
            className={cn(footerVariants({ sticky: stickyFooter }))}
            role="contentinfo"
          >
            {footer}
          </footer>
        )}

        {/* Floating toggle button for collapsible sidebar (optional enhancement) */}
        {sidebar && sidebarCollapsible && (
          <button
            onClick={handleToggle}
            className="hidden md:block fixed bottom-4 left-4 z-30 p-2 bg-trust-deep text-white rounded-full shadow-float hover:bg-neutral-800 transition-colors duration-150 focus:outline-none focus:ring-2 focus:ring-trust focus:ring-offset-2"
            aria-label={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            aria-expanded={!isCollapsed}
          >
            {sidebarPosition === 'left' ? (
              <ChevronLeft
                className="w-5 h-5 transition-transform duration-300"
                style={{
                  transform: isCollapsed ? 'rotate(0deg)' : 'rotate(180deg)',
                }}
                strokeWidth={1.5}
                aria-hidden="true"
              />
            ) : (
              <ChevronRight
                className="w-5 h-5 transition-transform duration-300"
                style={{
                  transform: isCollapsed ? 'rotate(0deg)' : 'rotate(180deg)',
                }}
                strokeWidth={1.5}
                aria-hidden="true"
              />
            )}
          </button>
        )}

        {/* Mobile hamburger toggle for drawer */}
        {sidebar && mobileDrawer && (
          <button
            onClick={handleMobileToggle}
            className="md:hidden fixed bottom-4 right-4 z-30 p-3 bg-trust-deep text-white rounded-full shadow-float hover:bg-neutral-800 transition-colors duration-150 focus:outline-none focus:ring-2 focus:ring-trust focus:ring-offset-2"
            aria-label={mobileOpen ? 'Close menu' : 'Open menu'}
            aria-expanded={mobileOpen}
          >
            {mobileOpen ? (
              <X className="w-6 h-6" strokeWidth={1.5} aria-hidden="true" />
            ) : (
              <Menu className="w-6 h-6" strokeWidth={1.5} aria-hidden="true" />
            )}
          </button>
        )}
      </div>
    );
  },
);

AppLayout.displayName = 'AppLayout';

export default AppLayout;
