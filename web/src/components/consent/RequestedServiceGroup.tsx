/**
 * RequestedServiceGroup
 *
 * Groups the permission sets that cover one service the client requested (Virtual Service
 * Scopes) under a header that states the request, its login and selection status, and the
 * single login action for that service. The nested rows hang off a rail whose colour repeats
 * the status: warning while a condition is unmet, success once both are met.
 */

import React, { useId } from 'react';
import { Button } from '@design-system/components/primitives/Button';
import type { RequestedServiceUsability } from '../../types/consent';
import type { RequestedServiceResult, RequestedServiceSelection } from './requestedServiceGate';

const loginLabel: Record<RequestedServiceUsability, string> = {
  usable: 'Connected',
  missing: 'Not connected',
  unusable: 'Needs reconnecting',
  unavailable: "Can't check right now",
};

const selectionLabel: Record<RequestedServiceSelection, string> = {
  none: 'No permission set on',
  incomplete: 'More permission sets needed',
  on: 'Permission set on',
};

export interface RequestedServiceGroupProps {
  agentName: string;
  result: RequestedServiceResult;
  /** False when no permission set offered to the agent covers the service */
  hasCoveringSets: boolean;
  /** Covering sets already shown under an earlier requested service */
  alsoAllowedBy: string[];
  onLogin: (serviceId: string) => void;
  onRetry: () => void;
  /** Covering permission set rows nested under the header */
  children?: React.ReactNode;
}

const StatusCheck: React.FC<{ met: boolean; label: string; srPrefix: string; testId: string }> = ({
  met,
  label,
  srPrefix,
  testId,
}) => (
  <li className={['inline-flex items-center gap-1.5', met ? 'text-success-dark' : 'text-warning-dark'].join(' ')}>
    <svg className="h-4 w-4 flex-shrink-0" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
      {met ? (
        <path
          fillRule="evenodd"
          d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
          clipRule="evenodd"
        />
      ) : (
        <path
          fillRule="evenodd"
          d="M10 18a8 8 0 100-16 8 8 0 000 16zm-1-11a1 1 0 112 0v3a1 1 0 11-2 0V7zm1 7a1 1 0 100-2 1 1 0 000 2z"
          clipRule="evenodd"
        />
      )}
    </svg>
    <span className="sr-only">{srPrefix}</span>
    <span data-testid={testId}>{label}</span>
  </li>
);

export const RequestedServiceGroup: React.FC<RequestedServiceGroupProps> = ({
  agentName,
  result,
  hasCoveringSets,
  alsoAllowedBy,
  onLogin,
  onRetry,
  children,
}) => {
  const titleId = useId();
  const rows = React.Children.toArray(children);
  const name = result.serviceName;
  const connected = result.login === 'usable';
  const selected = result.selection === 'on';
  const ready = hasCoveringSets && connected && selected;

  const tone = ready
    ? { rail: 'border-success-primary', header: 'bg-success-light', arrow: 'text-success-primary' }
    : { rail: 'border-warning-primary', header: 'bg-warning-light', arrow: 'text-warning-primary' };

  return (
    <div
      role="group"
      aria-labelledby={titleId}
      data-testid="requested-service-group"
      data-state={ready ? 'ready' : 'pending'}
      className={['border-l-4', tone.rail].join(' ')}
    >
      <div data-testid="requested-service-header" className={['px-6 py-4 space-y-2', tone.header].join(' ')}>
        <h3 id={titleId} className="text-base font-semibold text-trust-deep">
          {agentName} is asking for {name}
        </h3>

        {hasCoveringSets ? (
          <>
            {rows.length > 0 && (
              <p className="text-sm text-neutral-700">
                Turn on at least one of these permission sets to allow it.
              </p>
            )}
            {alsoAllowedBy.length > 0 && (
              <p className="text-sm text-neutral-700">
                {rows.length > 0 ? 'Also allowed' : 'Allowed'} by {alsoAllowedBy.join(', ')}, shown above.
              </p>
            )}
            <div className="flex flex-wrap items-center justify-between gap-3">
              <ul className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm font-medium">
                <StatusCheck
                  met={connected}
                  label={loginLabel[result.login]}
                  srPrefix={`${name} login: `}
                  testId="requested-service-status"
                />
                <StatusCheck
                  met={selected}
                  label={selectionLabel[result.selection]}
                  srPrefix="Selection: "
                  testId="requested-service-selection"
                />
              </ul>
              {(result.login === 'missing' || result.login === 'unusable') && (
                <Button variant="primary" size="sm" onClick={() => onLogin(result.serviceId)}>
                  Log in to {name}
                </Button>
              )}
              {result.login === 'unavailable' && (
                <Button variant="primary" size="sm" onClick={onRetry}>
                  Retry
                </Button>
              )}
            </div>
          </>
        ) : (
          <p className="text-sm text-neutral-700">
            None of this agent&apos;s permission sets include {name}, so you can&apos;t allow it here. An
            administrator can add one.
          </p>
        )}
      </div>

      {rows.length > 0 && (
        <div className={['ml-8 border-l-2', tone.rail].join(' ')}>
          {rows.map((row, index) => (
            <div key={index} data-testid="requested-service-row" className="relative">
              {index > 0 && <hr className="border-neutral-200" />}
              <svg
                className={['absolute left-0 top-10 h-3 w-5', tone.arrow].join(' ')}
                viewBox="0 0 20 12"
                fill="none"
                stroke="currentColor"
                strokeWidth={2}
                aria-hidden="true"
              >
                <path d="M0 6h17M12 1l5 5-5 5" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
              {row}
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default RequestedServiceGroup;
