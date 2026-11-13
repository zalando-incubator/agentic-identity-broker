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
import { Dialog } from '@headlessui/react';
import { Modal } from '@design-system/components/overlays/Modal';
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
  const agentCount = dependentAgents.length;
  const pluralize = agentCount !== 1 ? 's' : '';

  return (
    <Modal
      isOpen={isOpen}
      onClose={() => {
        if (!loading) onClose();
      }}
      title="Terminate Session?"
      size="sm"
      closeOnBackdropClick={!loading}
    >
      <Stack gap="md">
        {/* Dialog header */}
        <div>
          <Dialog.Description className="text-sm text-text-secondary">
            Are you sure you want to terminate your session with{' '}
            <strong>{serviceName}</strong>?
          </Dialog.Description>
        </div>

        {/* Warning about dependent agents */}
        {agentCount > 0 && (
          <Alert variant="warning" title="Dependent Agents" dismissible={false}>
            <Stack gap="sm">
              <p className="text-sm">
                This session is used by <strong>{agentCount}</strong> agent
                {pluralize}. Terminating will revoke their access.
              </p>

              {/* List of dependent agents */}
              <div className="mt-2">
                <p className="text-xs font-semibold text-neutral-700 mb-2">
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
          <Alert variant="info" title="No Dependent Agents" dismissible={false}>
            <p className="text-sm">
              No agents are currently using this session.
            </p>
          </Alert>
        )}

        {/* Termination error */}
        {error && (
          <Alert variant="error" title="Termination Failed" dismissible={false}>
            <p className="text-sm">{error}</p>
          </Alert>
        )}

        {/* Dialog actions */}
        <Stack direction="row" gap="md" justify="end">
          <Button
            variant="outline"
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
    </Modal>
  );
};

export default TerminationDialog;
