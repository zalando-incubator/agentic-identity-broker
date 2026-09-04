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
import { ApprovalScopeEditor, hasScopeIssues, resolveScopeIssues } from './ApprovalScopeEditor';
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

const INLINE_ERROR_CODES = new Set<ApprovalErrorCode>(['NETWORK_ERROR', 'INVALID_PATTERN']);

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

  if (errorCode && !INLINE_ERROR_CODES.has(errorCode)) {
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

  const scopeIssues = resolveScopeIssues(approval, toolPattern, paramsPattern);
  const scopeBlocked = persistence !== 'once' && hasScopeIssues(scopeIssues);

  return (
    <div className="max-w-2xl mx-auto px-6 pt-4 pb-28 space-y-4">
      <div>
        <h1 className="text-2xl font-bold text-neutral-900">
          Tool Approval Request
        </h1>
        <p className="text-sm text-neutral-500 mt-1">
          Review what the agent wants to do, then choose how long to allow it.
        </p>
      </div>

      {/* Tool call details */}
      <ToolCallCard approval={approval} />

      {errorCode && INLINE_ERROR_CODES.has(errorCode) && (
        <ApprovalErrorBanner
          errorCode={errorCode}
          message={errorMessage}
          onRetry={onRetry}
        />
      )}

      <div className="space-y-3">
        <PersistenceSelector
          value={persistence}
          onChange={handlePersistenceChange}
          disabled={submitting}
        />
        <ApprovalScopeEditor
          approval={approval}
          toolPattern={toolPattern}
          onToolPatternChange={setToolPattern}
          paramsPattern={paramsPattern}
          onParamsPatternChange={setParamsPattern}
          persistence={persistence}
          disabled={submitting}
        />
      </div>

      {/* Action buttons — pinned to the viewport so the decision stays reachable
          without scrolling past the review content. */}
      <div className="fixed inset-x-0 bottom-0 z-20 mb-0 border-t border-neutral-200 bg-white/95 px-4 py-2 backdrop-blur-sm">
        <div className="mx-auto flex max-w-2xl gap-3">
          <Button
            onClick={handleApprove}
            disabled={submitting || scopeBlocked}
            size="sm"
            className="flex-1"
          >
            {submitting ? 'Processing…' : 'Approve'}
          </Button>
          <Button
            onClick={handleDeny}
            disabled={submitting}
            variant="secondary"
            size="sm"
            className="flex-1"
          >
            Deny
          </Button>
        </div>
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
