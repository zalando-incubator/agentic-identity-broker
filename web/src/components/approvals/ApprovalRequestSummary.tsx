import { Avatar } from '@design-system/components/primitives/Avatar';
import type { ToolApprovalDetail } from '../../types/approval';
import { RiskBadge } from './RiskBadge';

interface ApprovalRequestSummaryProps {
  approval: ToolApprovalDetail;
  showExpiry?: boolean;
  showSessionContext?: boolean;
}

export function ApprovalRequestSummary({
  approval,
  showExpiry = false,
  showSessionContext = false,
}: ApprovalRequestSummaryProps) {
  const agentName = approval.agent_display_name?.trim() || approval.agent_id;
  const toolArguments = Object.entries(approval.arguments ?? {});
  const sessionEntries = [
    approval.agent_session_id
      ? { label: 'Agent session', value: approval.agent_session_id }
      : null,
    approval.mcp_session_id
      ? { label: 'MCP session', value: approval.mcp_session_id }
      : null,
    approval.tool_invocation_id
      ? { label: 'Tool invocation', value: approval.tool_invocation_id }
      : null,
  ].filter((entry): entry is { label: string; value: string } => entry !== null);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Avatar
            initials={agentName.charAt(0).toUpperCase()}
            shape="rounded"
            size="sm"
          />
          <div className="space-y-0.5">
            <p className="text-xs font-medium uppercase tracking-wide text-neutral-400">
              Requested by
            </p>
            <p className="text-sm font-semibold text-neutral-900">{agentName}</p>
          </div>
        </div>
        <RiskBadge level={approval.risk_level} />
      </div>

      <div className="space-y-1.5">
        <p className="text-xs font-medium uppercase tracking-wide text-neutral-400">
          Action
        </p>
        <div className="rounded-md border border-neutral-200 bg-neutral-50 px-3 py-2.5 text-sm font-semibold leading-snug text-neutral-900">
          {approval.description?.trim() || approval.tool_name}
        </div>
      </div>

      <div className="space-y-1.5">
        <p className="text-xs font-medium uppercase tracking-wide text-neutral-400">
          Technical details
        </p>
        <div className="space-y-1 rounded-md border border-neutral-200 bg-neutral-50 px-3 py-2.5 font-mono text-sm">
          <h3 className="font-semibold text-neutral-800">{approval.tool_name}</h3>
          {toolArguments.length === 0 ? (
            <p className="text-neutral-500">No parameters</p>
          ) : (
            toolArguments.map(([key, value]) => (
              <div key={key} className="flex gap-2">
                <span className="shrink-0 text-neutral-400">{key}:</span>
                <span className="break-all text-neutral-700">
                  {typeof value === 'string' ? value : JSON.stringify(value)}
                </span>
              </div>
            ))
          )}
        </div>
      </div>

      {showSessionContext && sessionEntries.length > 0 && (
        <div className="space-y-1.5">
          <p className="text-xs font-medium uppercase tracking-wide text-neutral-400">
            Session context
          </p>
          <div className="divide-y divide-neutral-100 rounded-md border border-neutral-200 bg-white">
            {sessionEntries.map((entry) => (
              <div
                key={entry.label}
                className="flex items-center justify-between gap-4 px-3 py-2 text-sm"
              >
                <span className="text-neutral-600">{entry.label}</span>
                <span className="break-all font-mono text-neutral-900">
                  {entry.value}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {approval.status !== 'pending' && (
        <div className="border-t border-neutral-100 pt-2">
          <p className="text-xs font-medium text-neutral-700">Covers</p>
          <code className="font-mono text-sm text-neutral-700">{approval.pattern_preview}</code>
        </div>
      )}

      {showExpiry && (
        <div className="border-t border-neutral-100 pt-2">
          <p className="text-xs text-neutral-400">
            Expires: {new Date(approval.expires_at).toLocaleString()}
          </p>
        </div>
      )}
    </div>
  );
}

export default ApprovalRequestSummary;
