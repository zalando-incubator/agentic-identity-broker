/**
 * AppLayout component wraps the design system AppLayout with a custom header.
 *
 * Includes:
 * - Sticky header with branding, navigation, and user info
 * - Main content area with responsive padding and premium styling
 */

import React, { ReactNode } from 'react';
import { useLocation, Link } from 'react-router-dom';
import { useConsent } from '@hooks/useConsent';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { AppLayout as DesignSystemAppLayout } from '@design-system/components/layout/AppLayout/AppLayout';

interface AppLayoutProps {
  children: ReactNode;
  header?: React.ComponentType<Record<string, never>> | null;
}

/**
 * Header component with the wordmark, navigation, and user info.
 * A quiet bar on the page background with a hairline underneath.
 */
function Header() {
  const { userInfo } = useConsent();
  const location = useLocation();

  const navLinks = [
    { label: 'Agent Delegations', href: '/delegations' },
    { label: 'Tool Authorizations', href: '/approvals' },
    { label: 'Third-Party Sessions', href: '/sessions' },
  ];


  return (
    <div className="bg-neutral-50 border-b border-line">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-16 gap-8">
          {/* Wordmark */}
          <Link
            to="/delegations"
            className="flex items-center flex-shrink-0 rounded-md focus:outline-none focus:ring-2 focus:ring-accent focus:ring-offset-2"
          >
            <h1 className="sr-only">Agentic Identity Broker</h1>
            <img
              src="/img/aib-wordmark.svg"
              alt="Agentic Identity Broker"
              className="h-5 w-auto"
              width={500}
              height={60}
            />
          </Link>

          {/* Navigation */}
          <nav className="flex items-center gap-6 ml-auto">
            {/* Main navigation links */}
            <div className="hidden md:flex items-center gap-1">
              {navLinks.map((link) => {
                const isActive = location.pathname === link.href;

                return (
                  <Link
                    key={link.href}
                    to={link.href}
                    className={`px-3 py-2 rounded-md text-sm font-medium transition-colors duration-150 ${
                      isActive
                        ? 'bg-neutral-100 text-trust-deep'
                        : 'text-neutral-600 hover:bg-neutral-100 hover:text-trust-deep'
                    }`}
                    aria-current={isActive ? 'page' : undefined}
                  >
                    {link.label}
                  </Link>
                );
              })}
            </div>

            {/* User info with avatar */}
            {userInfo && (
              <div className="flex items-center gap-3 pl-6 border-l border-line">
                <Avatar
                  src={userInfo.pictureUrl}
                  alt={userInfo.displayName}
                  initials={userInfo.displayName.charAt(0).toUpperCase()}
                  size="sm"
                  shape="circle"
                />
                <div className="flex flex-col min-w-0">
                  <p className="text-sm font-medium text-trust-deep truncate">
                    {userInfo.displayName}
                  </p>
                  <p className="text-xs text-neutral-500 truncate">
                    {userInfo.email || userInfo.principal}
                  </p>
                </div>
              </div>
            )}
          </nav>
        </div>
      </div>
    </div>
  );
}

/**
 * Main layout wrapper that extends the design system AppLayout with custom styling.
 * Provides consistent structure on the flat page background.
 */
export function AppLayout({ children, header }: AppLayoutProps) {
  const HeaderComponent = header || Header;

  return (
    <div className="bg-neutral-50">
      <DesignSystemAppLayout
        header={<HeaderComponent />}
        stickyHeader={true}
        className="bg-transparent"
      >
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
          <div className="animate-fade-in-up">{children}</div>
        </div>
      </DesignSystemAppLayout>
    </div>
  );
}

export default AppLayout;
