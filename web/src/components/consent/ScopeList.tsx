/**
 * ScopeList component displays OAuth scopes for a service.
 *
 * Features:
 * - Badge-style display
 * - Scope value and description
 * - Visual indicator for granted scopes
 * - Responsive layout
 */

import React from 'react';
import type { ServiceScope } from '../../types/consent';
import { CircleCheck } from 'lucide-react';

interface ScopeListProps {
  /** List of OAuth scopes */
  scopes: ServiceScope[];
  /** List of scope values that have been granted */
  grantedScopes?: string[];
  /** Whether the parent container is expanded */
  isExpanded: boolean;
}

/**
 * ScopeList displays a list of OAuth scopes with badge styling.
 * Highlights scopes that have been granted to the agent.
 */
export function ScopeList({
  scopes,
  grantedScopes = [],
  isExpanded,
}: ScopeListProps) {
  if (!isExpanded || scopes.length === 0) {
    return null;
  }

  const isGranted = (scopeValue: string) => grantedScopes.includes(scopeValue);

  return (
    <div className="divide-y divide-dotted divide-slate/10">
      {scopes.map((scope) => {
        const granted = isGranted(scope.value);

        return (
          <div
            key={scope.value}
            className="flex items-start gap-3 py-3 first:pt-0 last:pb-0"
          >
            {/* Status indicator */}
            <div className="flex-shrink-0 mt-0.5">
              {granted ? (
                <CircleCheck
                  className="w-5 h-5 text-success-primary"
                  aria-label="Granted"
                  strokeWidth={1.5}
                />
              ) : (
                <div
                  className="w-5 h-5 rounded-full border-2 border-neutral-300"
                  aria-label="Not granted"
                />
              )}
            </div>

            {/* Scope details */}
            <div className="flex-1 min-w-0">
              <div className="flex items-start justify-between gap-2">
                <code
                  className={`text-sm font-mono font-medium break-all ${
                    granted ? 'text-success-hover' : 'text-trust-deep'
                  }`}
                >
                  {scope.value}
                </code>
                {granted && (
                  <span className="flex-shrink-0 inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-success-light text-success-hover">
                    Granted
                  </span>
                )}
              </div>
              <p
                className={`mt-1 text-sm ${
                  granted ? 'text-success-primary' : 'text-neutral-600'
                }`}
              >
                {scope.description}
              </p>
            </div>
          </div>
        );
      })}
    </div>
  );
}

export default ScopeList;
