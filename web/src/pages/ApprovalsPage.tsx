import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { usePendingApprovals } from '@hooks/usePendingApprovals';
import { useStandingApprovals } from '@hooks/useStandingApprovals';
import { useApprovalActions } from '@hooks/useApprovalReview';
import { usePrincipal } from '@services/query/QueryProvider';
import { approvalApi } from '@services/api/approvals';
import { approvalQueueCopy as copy } from '@copy/approvalQueue';
import { ApprovalsView } from './ApprovalsView';
import type { RememberedFilter } from './ApprovalsView';
import type { ApprovalReviewPageProps } from '@components/approvals/ApprovalReviewPage';
import type { ToolApprovalDetail } from '../types/approval';

export default function ApprovalsPage() {
  const { principal } = usePrincipal();
  const tab = useLocation().pathname === '/approvals/remembered' ? 'remembered' : 'pending';
  const [searchParams, setSearchParams] = useSearchParams();
  const search = searchParams.get('q') ?? '';
  const filterParam = searchParams.get('filter');
  const filter: RememberedFilter = filterParam === 'allowed' || filterParam === 'denied' ? filterParam : 'all';
  const updateParam = (key: 'q' | 'filter', value: string) => setSearchParams(previous => {
    const params = new URLSearchParams(previous);
    if (value && !(key === 'filter' && value === 'all')) params.set(key, value);
    else params.delete(key);
    return params;
  }, { replace: true });
  const pending = usePendingApprovals();
  const standing = useStandingApprovals({ onRevokeSuccess: () => toast.success(copy.revokeSuccess) });
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [compact, setCompact] = useState(() => window.matchMedia?.('(max-width: 1011px)').matches ?? false);
  const [minRows, setMinRows] = useState(0);
  const pageRef = useRef<HTMLDivElement>(null);
  const revokeTrigger = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    const media = window.matchMedia?.('(max-width: 1011px)');
    if (!media) return;
    const update = () => setCompact(media.matches);
    media.addEventListener('change', update);
    update();
    return () => media.removeEventListener('change', update);
  }, []);
  useEffect(() => {
    const count = pending.data?.length;
    if (count !== undefined) setMinRows(previous => count === 0 ? 0 : Math.max(previous, count));
  }, [pending.data?.length]);

  const approvals = useMemo(() => pending.data?.filter(approval => approval.principal === principal) ?? [], [pending.data, principal]);
  const selected = approvals.find(approval => approval.id === selectedId) ?? approvals[0] ?? null;
  const currentId = selected?.id ?? null;
  const nextAfterDecision = useRef<{ id: string; nextId: string | null } | null>(null);
  const onDecisionSuccess = useCallback((action: 'approve' | 'deny', id: string) => {
    const index = approvals.findIndex(approval => approval.id === id);
    const nextId = nextAfterDecision.current?.id === id ? nextAfterDecision.current.nextId : approvals[index + 1]?.id ?? approvals[index - 1]?.id ?? null;
    nextAfterDecision.current = null;
    setSelectedId(previous => previous === id || previous === null ? nextId : previous);
    toast.success(action === 'approve' ? copy.approvedToast : copy.deniedToast);
  }, [approvals]);
  const actions = useApprovalActions(currentId ?? '', { onDecisionSuccess });
  function captureNextRequest() {
    if (!selected) return;
    const index = approvals.findIndex(approval => approval.id === selected.id);
    nextAfterDecision.current = { id: selected.id, nextId: approvals[index + 1]?.id ?? approvals[index - 1]?.id ?? null };
  }
  const approve: ApprovalReviewPageProps['onApprove'] = request => { captureNextRequest(); return actions.approve(request); };
  const deny: ApprovalReviewPageProps['onDeny'] = permanent => { captureNextRequest(); return actions.deny(permanent); };
  const onPreview = useCallback((pattern: Record<string, string>, signal: AbortSignal) =>
    approvalApi.previewApprovalScope(currentId ?? '', { params_pattern: pattern }, { signal }), [currentId]);
  const review: ApprovalReviewPageProps | null = selected && tab === 'pending' ? {
    approval: selected,
    actingPrincipal: principal,
    submitting: actions.submitting,
    submittingAction: actions.submittingAction,
    errorCode: actions.errorCode,
    errorMessage: actions.errorMessage,
    approveResult: actions.approveResult,
    denyResult: actions.denyResult,
    onApprove: approve,
    onDeny: deny,
    onPreview,
    onRetry: () => { void pending.refetch(); },
  } : null;


  return <div ref={pageRef} tabIndex={-1} className="min-w-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring">
    <ApprovalsView tab={tab} compact={compact} minRows={minRows}
      pending={{ data: pending.data ? approvals : undefined, loading: pending.isPending, stale: pending.stale, onRetry: () => { void pending.refetch(); } }}
      standing={{ data: standing.data?.filter(approval => approval.principal === principal), loading: standing.isPending, stale: standing.stale, onRetry: () => { void standing.refetch(); } }}
      selectedId={currentId} onSelect={setSelectedId} review={review} search={search} onSearchChange={value => updateParam('q', value)} filter={filter}
      onApproveOnce={() => { void approve({ persistence: 'once' }); }}
      onDenyOnce={() => { void deny(); }}
      onFilterChange={next => updateParam('filter', next)}
      revoke={{
        confirmation: standing.revoke.confirmation,
        isPending: standing.revoke.isPending,
        error: standing.revoke.error !== null,
        request: (approval: ToolApprovalDetail, trigger: HTMLButtonElement) => {
          revokeTrigger.current = trigger;
          standing.revoke.requestRevoke(approval);
        },
        cancel: standing.revoke.cancelRevoke,
        confirm: () => { void standing.revoke.confirmRevoke(); },
        returnFocus: () => {
          const trigger = revokeTrigger.current;
          if (trigger?.isConnected && !trigger.disabled) trigger.focus(); else pageRef.current?.focus();
        },
      }} />
  </div>;
}
