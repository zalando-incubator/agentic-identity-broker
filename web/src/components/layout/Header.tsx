/**
 * Header Component
 *
 * Main application header with navigation links.
 * Uses design system components for consistent styling.
 *
 * Features:
 * - Navigation links to main pages
 * - Active link highlighting
 * - Responsive design
 * - Semantic HTML
 */

import React from 'react';
import { Link, useLocation } from 'react-router-dom';
import { cn } from '@design-system/utils';
import { Menu } from 'lucide-react';

interface NavLink {
  label: string;
  href: string;
  icon?: React.ReactNode;
}

const NAV_LINKS: NavLink[] = [
  {
    label: 'Agent Delegations',
    href: '/',
  },
  {
    label: 'Third-Party Sessions',
    href: '/sessions',
  },
];

export const Header: React.FC = () => {
  const location = useLocation();

  return (
    <header className="bg-white border-b border-neutral-200">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-16">
          {/* Logo/Brand */}
          <div className="flex-shrink-0">
            <h1 className="text-xl font-medium text-trust-deep">
              Identity Broker
            </h1>
          </div>

          {/* Navigation */}
          <nav className="hidden md:flex items-center space-x-1">
            {NAV_LINKS.map((link) => {
              const isActive =
                location.pathname === link.href ||
                (location.pathname === '/' && link.href === '/');

              return (
                <Link
                  key={link.href}
                  to={link.href}
                  className={cn(
                    'px-3 py-2 rounded-md text-sm font-medium transition-colors duration-200',
                    isActive
                      ? 'bg-trust-light text-trust-deep'
                      : 'text-neutral-700 hover:bg-neutral-100 hover:text-neutral-900',
                  )}
                  aria-current={isActive ? 'page' : undefined}
                >
                  {link.label}
                </Link>
              );
            })}
          </nav>

          {/* Mobile Navigation Menu Button (for future implementation) */}
          <div className="md:hidden">
            <button
              className="inline-flex items-center justify-center p-2 rounded-md text-neutral-700 hover:text-neutral-900 hover:bg-neutral-100 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-trust"
              aria-label="Open menu"
            >
              <Menu
                className="block h-6 w-6"
                aria-hidden="true"
                strokeWidth={1.5}
              />
            </button>
          </div>
        </div>
      </div>
    </header>
  );
};

export default Header;
