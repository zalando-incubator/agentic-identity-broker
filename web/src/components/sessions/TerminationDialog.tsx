/**
 * TerminationDialog
 *
 * Modal dialog for confirming session termination.
 * Shows warning about affected agents before deletion.
 * Follows "Refined Trust Architecture" design system.
 *
 * Features:
 * - Warning about dependent agents
 * - List of affected agents
 * - Confirmation and cancel buttons
 * - Loading state during termination
 * - Error display if termination fails
 * - WCAG 2.1 AA compliant with focus management
 */

import React from 'react';
import { Alert } from '@design-system/components/feedback/Alert';
import { Button } from '@design-system/components/primitives/Button';
import { Stack } from '@design-system/components/layout/Stack';
import { Badge } from '@design-system/components/primitives/Badge';
import type { AgentInfo } from '@services/api/sessions';

interface TerminationDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => void;
  serviceName: string;
  dependentAgents: AgentInfo[];
  loading: boolean;
  error?: string | null;
}

/**
 * TerminationDialog displays a modal confirmation dialog for session termination.
 * Users must confirm before the session is deleted.
 *
 * @example
 * ```tsx
 * <TerminationDialog
 *   isOpen={showDialog}
 *   onClose={handleClose}
 *   onConfirm={handleConfirm}
 *   serviceName="Google"
 *   dependentAgents={["agent-1", "agent-2"]}
 *   loading={isDeleting}
 *   error={errorMessage}
 * />
 * ```
 */
export const TerminationDialog: React.FC<TerminationDialogProps> = ({
  isOpen,
  onClose,
  onConfirm,
  serviceName,
  dependentAgents,
  loading,
  error,
}) => {
  // Don't render if not open
  if (!isOpen) {
    return null;
  }

  const agentCount = dependentAgents.length;
  const pluralize = agentCount !== 1 ? 's' : '';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50">
      <button
        type="button"
        className="absolute inset-0 cursor-pointer"
        aria-label="Close dialog"
        onClick={onClose}
      />
      {/* Modal content card */}
      <div
        className="relative z-10 bg-white rounded-xl border border-line-strong shadow-float max-w-md w-full mx-4 p-6"
        role="dialog"
        aria-modal="true"
        aria-labelledby="termination-dialog-title"
        aria-describedby="termination-dialog-description"
      >
        <Stack gap="md">
          {/* Dialog header */}
          <div>
            <h2
              id="termination-dialog-title"
              className="text-xl font-medium text-neutral-900"
            >
              Terminate Session?
            </h2>
            <p
              id="termination-dialog-description"
              className="mt-2 text-sm text-neutral-600"
            >
              Are you sure you want to terminate your session with{' '}
              <strong>{serviceName}</strong>?
            </p>
          </div>

          {/* Warning about dependent agents */}
          {agentCount > 0 && (
            <Alert
              variant="warning"
              title="Dependent Agents"
              dismissible={false}
            >
              <Stack gap="sm">
                <p className="text-sm">
                  This session is used by <strong>{agentCount}</strong> agent
                  {pluralize}. Terminating will revoke their access.
                </p>

                {/* List of dependent agents */}
                <div className="mt-2">
                  <p className="text-xs font-medium text-neutral-700 mb-2">
                    Affected agents:
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {dependentAgents.map((agent) => (
                      <Badge key={agent.id} variant="warning" size="sm">
                        {agent.display_name}
                      </Badge>
                    ))}
                  </div>
                </div>
              </Stack>
            </Alert>
          )}

          {/* Info message when no dependent agents */}
          {agentCount === 0 && (
            <Alert
              variant="info"
              title="No Dependent Agents"
              dismissible={false}
            >
              <p className="text-sm">
                No agents are currently using this session.
              </p>
            </Alert>
          )}

          {/* Termination error */}
          {error && (
            <Alert
              variant="error"
              title="Termination Failed"
              dismissible={false}
            >
              <p className="text-sm">{error}</p>
            </Alert>
          )}

          {/* Dialog actions */}
          <Stack direction="row" gap="md" justify="end">
            <Button
              variant="ghost"
              size="md"
              onClick={onClose}
              disabled={loading}
              aria-label="Cancel session termination"
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              size="md"
              onClick={onConfirm}
              isLoading={loading}
              disabled={loading}
              aria-label={`Terminate session with ${serviceName}`}
            >
              Terminate Session
            </Button>
          </Stack>
        </Stack>
      </div>
    </div>
  );
};

export default TerminationDialog;
