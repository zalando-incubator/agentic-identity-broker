/**
 * ApprovalReviewPage - Main approval review component.
 *
 * Composes ToolCallCard + PersistenceSelector + action buttons.
 * Handles the complete approve/deny flow with transitions to confirmation.
 */

import { useState } from 'react';
import { Button } from '@components/ui/Button';
import { ToolCallCard } from './ToolCallCard';
import { PersistenceSelector } from './PersistenceSelector';
import { PatternEditor } from './PatternEditor';
import { ApprovalConfirmation } from './ApprovalConfirmation';
import { ApprovalErrorBanner } from './ApprovalErrorBanner';
import type {
  ToolApprovalDetail,
  ApprovalPersistence,
  ApprovalErrorCode,
  ApproveResponseData,
  DenyResponseData,
  ApproveRequest,
} from '../../types/approval';

interface ApprovalReviewPageProps {
  approval: ToolApprovalDetail;
  submitting: boolean;
  errorCode: ApprovalErrorCode | null;
  errorMessage: string | null;
  approveResult: ApproveResponseData | null;
  denyResult: DenyResponseData | null;
  onApprove: (request: ApproveRequest) => Promise<void>;
  onDeny: (permanent?: boolean) => Promise<void>;
  onRetry?: () => void;
}

export function ApprovalReviewPage({
  approval,
  submitting,
  errorCode,
  errorMessage,
  approveResult,
  denyResult,
  onApprove,
  onDeny,
  onRetry,
}: ApprovalReviewPageProps) {
  const [persistence, setPersistence] = useState<ApprovalPersistence>('once');
  const [toolPattern, setToolPattern] = useState(approval.tool_pattern ?? approval.tool_name);
  const [paramsPattern, setParamsPattern] = useState(approval.params_pattern ?? {});

  const handlePersistenceChange = (value: ApprovalPersistence) => {
    setPersistence(value);
    setToolPattern(approval.tool_pattern ?? approval.tool_name);
    setParamsPattern(approval.params_pattern ?? {});
  };

  if (approveResult || approval.status === 'approved') {
    return (
      <ApprovalConfirmation
        type="approved"
        persistence={approveResult?.persistence ?? approval.persistence}
        decidedAt={approveResult?.approved_at ?? approval.approved_at}
        toolName={approval.tool_name}
        agentName={approval.agent_display_name}
      />
    );
  }

  if (denyResult || approval.status === 'denied') {
    return (
      <ApprovalConfirmation
        type="denied"
        persistence={denyResult?.persistence ?? approval.persistence}
        decidedAt={denyResult?.denied_at ?? approval.denied_at}
        toolName={approval.tool_name}
        agentName={approval.agent_display_name}
      />
    );
  }

  if (errorCode && errorCode !== 'NETWORK_ERROR') {
    return (
      <div className="space-y-6">
        <ApprovalErrorBanner
          errorCode={errorCode}
          message={errorMessage}
          onRetry={onRetry}
        />
      </div>
    );
  }

  const handleApprove = async () => {
    await onApprove(persistence === 'once'
      ? { persistence }
      : { persistence, tool_pattern: toolPattern, params_pattern: paramsPattern });
  };

  const handleDeny = async () => {
    await onDeny();
  };

  const handleDenyPermanently = async () => {
    await onDeny(true);
  };

  return (
    <div className="max-w-2xl mx-auto p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-neutral-900">
          Tool Approval Request
        </h1>
        <p className="text-sm text-neutral-500 mt-1">
          An agent is requesting permission to execute a tool call. Review the
          details below and choose how to proceed.
        </p>
      </div>

      {/* Tool call details */}
      <ToolCallCard approval={approval} />

      {errorCode === 'NETWORK_ERROR' && (
        <ApprovalErrorBanner
          errorCode={errorCode}
          message={errorMessage}
          onRetry={onRetry}
        />
      )}

      <PersistenceSelector
        value={persistence}
        onChange={handlePersistenceChange}
        disabled={submitting}
      />
      <PatternEditor
        approval={approval}
        toolPattern={toolPattern}
        onToolPatternChange={setToolPattern}
        paramsPattern={paramsPattern}
        onParamsPatternChange={setParamsPattern}
        persistence={persistence}
        disabled={submitting}
      />

      {/* Action buttons */}
      <div className="flex gap-3 pt-2">
        <Button
          onClick={handleApprove}
          disabled={submitting}
          className="flex-1"
        >
          {submitting ? 'Processing…' : 'Approve'}
        </Button>
        <Button
          onClick={handleDeny}
          disabled={submitting}
          variant="secondary"
          className="flex-1"
        >
          Deny
        </Button>
      </div>

      {/* Deny permanently option */}
      <div className="border-t border-neutral-200 pt-3">
        <button
          type="button"
          onClick={handleDenyPermanently}
          disabled={submitting}
          className="text-sm text-error-primary hover:text-error-primary/80 underline disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Deny permanently — block this tool for this agent
        </button>
        <p className="text-xs text-neutral-500 mt-1">
          This permanently blocks the agent from requesting this tool. You can
          revoke this later from your approval settings.
        </p>
      </div>
    </div>
  );
}

export default ApprovalReviewPage;
