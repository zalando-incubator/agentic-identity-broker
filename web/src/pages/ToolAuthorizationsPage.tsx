/**
 * ToolAuthorizationsPage - Pending approvals queue and permanent tool authorizations.
 *
 * Sections:
 * 1. Pending Approvals — inline approve/deny with persistence choice.
 * 2. Permanent Authorizations — persisted allow/deny decisions with revoke actions.
 */

import { useState, useEffect, useCallback } from 'react';
import { AppLayout } from '@components/layout/AppLayout';
import { PageTransition } from '@components/ui/PageTransition';
import { InlineError } from '@components/ui/InlineError';
import { Button } from '@components/ui/Button';
import { Skeleton } from '@components/ui/Skeleton';
import { Card } from '@design-system/components/data-display/Card';
import { approvalApi } from '@services/api/approvals';
import { ApprovalRequestSummary } from '@components/approvals/ApprovalRequestSummary';
import { PersistenceSelector } from '@components/approvals/PersistenceSelector';
import { ApprovalScopeEditor, hasScopeIssues, resolveScopeIssues } from '@components/approvals/ApprovalScopeEditor';
import type { ToolApprovalDetail, ApprovalPersistence } from '../types/approval';


function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins} minute${mins === 1 ? '' : 's'} ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs} hour${hrs === 1 ? '' : 's'} ago`;
  const days = Math.floor(hrs / 24);
  return `${days} day${days === 1 ? '' : 's'} ago`;
}

function CardSkeleton() {
  return (
    <Card padding="default">
      <div className="space-y-6">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <Skeleton width="2rem" height="2rem" className="rounded-md flex-shrink-0" />
            <div className="space-y-1">
              <Skeleton width="72px" height="0.75rem" className="rounded-md" />
              <Skeleton width="120px" height="1rem" className="rounded-md" />
            </div>
          </div>
          <Skeleton width="80px" height="1.25rem" className="rounded-full" />
        </div>
        <div className="space-y-2">
          <Skeleton width="48px" height="0.75rem" className="rounded-md" />
          <Skeleton width="75%" height="1.25rem" className="rounded-md" />
        </div>
        <div className="rounded-md border border-neutral-200 p-3 space-y-1.5">
          <Skeleton width="110px" height="0.75rem" className="rounded-md" />
          <Skeleton width="40%" height="1rem" className="rounded-md" />
          <Skeleton width="60%" height="0.875rem" className="rounded-md" />
          <Skeleton width="50%" height="0.875rem" className="rounded-md" />
        </div>
        <div className="flex items-center justify-between border-t border-neutral-100 pt-3">
          <Skeleton width="100px" height="0.75rem" className="rounded-md" />
          <div className="flex gap-2">
            <Skeleton width="72px" height="2rem" className="rounded-md" />
            <Skeleton width="72px" height="2rem" className="rounded-md" />
          </div>
        </div>
      </div>
    </Card>
  );
}

// ── Pending Approval Card ─────────────────────────────────────────────────────

interface PendingApprovalCardProps {
  approval: ToolApprovalDetail;
  onResolved: (id: string, permanentAdded: boolean) => void;
}

type ActionState = null | 'approve' | 'deny';

function PendingApprovalCard({ approval, onResolved }: PendingApprovalCardProps) {
  const [action, setAction] = useState<ActionState>(null);
  const [persistence, setPersistence] = useState<ApprovalPersistence>('once');
  const [toolPattern, setToolPattern] = useState(approval.tool_pattern ?? approval.tool_name);
  const [paramsPattern, setParamsPattern] = useState(approval.params_pattern ?? {});

  const handlePersistenceChange = (value: ApprovalPersistence) => {
    setPersistence(value);
    setToolPattern(approval.tool_pattern ?? approval.tool_name);
    setParamsPattern(approval.params_pattern ?? {});
  };
  const [submitting, setSubmitting] = useState(false);
  const [errorText, setErrorText] = useState<string | null>(null);


  const handleApprove = async () => {
    setSubmitting(true);
    setErrorText(null);
    try {
      await approvalApi.approveApproval(approval.id, persistence === 'once'
        ? { persistence }
        : { persistence, tool_pattern: toolPattern, params_pattern: paramsPattern });
      onResolved(approval.id, persistence === 'permanent');
    } catch {
      setErrorText('Could not approve this request. Check the approval scope and try again.');
      setSubmitting(false);
    }
  };

  const handleDeny = async (permanent: boolean) => {
    setSubmitting(true);
    try {
      await approvalApi.denyApproval(approval.id, permanent ? { persistence: 'permanent' } : undefined);
      onResolved(approval.id, permanent);
    } catch {
      setSubmitting(false);
    }
  };

  const cancel = () => {
    setAction(null);
    setPersistence('once');
    setErrorText(null);
    setToolPattern(approval.tool_pattern ?? approval.tool_name);
    setParamsPattern(approval.params_pattern ?? {});
  };

  const scopeIssues = resolveScopeIssues(approval, toolPattern, paramsPattern);
  const scopeBlocked = persistence !== 'once' && hasScopeIssues(scopeIssues);

  return (
    <Card padding="default">
      <div className="space-y-6">
        <ApprovalRequestSummary approval={approval} />

        {action === 'approve' && (
          <div className="space-y-3 border-t border-neutral-100 pt-3">
            <PersistenceSelector
              value={persistence}
              onChange={handlePersistenceChange}
              disabled={submitting}
              name={`pending-approval-${approval.id}`}
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
            {errorText && (
              <InlineError error={errorText} onRetry={() => void handleApprove()} retryLabel="Try approving again" />
            )}
            <div className="flex items-center gap-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={handleApprove}
                isLoading={submitting}
                disabled={submitting || scopeBlocked}
              >
                Confirm Approve
              </Button>
              <Button variant="ghost" size="sm" onClick={cancel} disabled={submitting}>
                Cancel
              </Button>
            </div>
          </div>
        )}

        {action === 'deny' && (
          <div className="space-y-2 border-t border-neutral-100 pt-3">
            <p className="text-xs font-medium text-neutral-600">Deny scope</p>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                variant="danger"
                size="sm"
                onClick={() => handleDeny(false)}
                isLoading={submitting}
                disabled={submitting}
              >
                Deny this request
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleDeny(true)}
                disabled={submitting}
                className="text-red-700 border-red-300 hover:bg-red-50"
              >
                Block permanently
              </Button>
              <Button variant="ghost" size="sm" onClick={cancel} disabled={submitting}>
                Cancel
              </Button>
            </div>
          </div>
        )}

        {action === null && (
          <div className="flex items-center justify-between border-t border-neutral-100 pt-3">
            <p className="text-xs text-neutral-400">Requested {relativeTime(approval.created_at)}</p>
            <div className="flex items-center gap-2">
              <Button variant="danger" size="sm" onClick={() => setAction('deny')}>
                Deny
              </Button>
              <Button variant="secondary" size="sm" onClick={() => setAction('approve')}>
                Approve
              </Button>
            </div>
          </div>
        )}
      </div>
    </Card>
  );
}

// ── Permanent Authorization Card ──────────────────────────────────────────────

interface PermanentCardProps {
  approval: ToolApprovalDetail;
  onRevoke: (id: string) => void;
  revoking: boolean;
}

function PermanentAuthCard({ approval, onRevoke, revoking }: PermanentCardProps) {
  const isAllowed = approval.status === 'approved';
  const decidedAt = approval.approved_at ?? approval.denied_at;

  return (
    <Card padding="default">
      <div className="space-y-6">
        <ApprovalRequestSummary approval={approval} />

        <div className="flex items-center justify-between border-t border-neutral-100 pt-3">
          <div className="flex items-center gap-1.5 text-xs">
            {isAllowed ? (
              <span className="font-medium text-green-700">Permanently allowed</span>
            ) : (
              <span className="font-medium text-red-700">Permanently denied</span>
            )}
            {decidedAt && (
              <span className="text-neutral-400">
                · {new Date(decidedAt).toLocaleDateString(undefined, {
                  year: 'numeric',
                  month: 'short',
                  day: 'numeric',
                })}
              </span>
            )}
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onRevoke(approval.id)}
            disabled={revoking}
            className="text-red-600 hover:text-red-700"
          >
            {revoking ? 'Revoking…' : 'Revoke'}
          </Button>
        </div>
      </div>
    </Card>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export function ToolAuthorizationsPage() {
  const [pending, setPending] = useState<ToolApprovalDetail[]>([]);
  const [permanent, setPermanent] = useState<ToolApprovalDetail[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [revokingId, setRevokingId] = useState<string | null>(null);

  const fetchAll = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const [pendingData, permanentData] = await Promise.all([
        approvalApi.listPendingApprovals(),
        approvalApi.listPermanentApprovals(),
      ]);
      setPending(pendingData);
      setPermanent(permanentData);
    } catch {
      setError('Failed to load tool authorizations');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  // Called when a pending card is approved or denied.
  // If permanent, refresh the permanent list so the new entry appears immediately.
  const handleResolved = useCallback(async (approvalId: string, permanentAdded: boolean) => {
    setPending((prev) => prev.filter((a) => a.id !== approvalId));
    if (permanentAdded) {
      try {
        const permanentData = await approvalApi.listPermanentApprovals();
        setPermanent(permanentData);
      } catch { /* non-critical */ }
    }
  }, []);

  const handleRevoke = async (approvalId: string) => {
    try {
      setRevokingId(approvalId);
      await approvalApi.revokePermanentApproval(approvalId);
      setPermanent((prev) => prev.filter((a) => a.id !== approvalId));
    } catch {
      setError('Failed to revoke authorization');
    } finally {
      setRevokingId(null);
    }
  };

  const hasPending = pending.length > 0;
  const hasPermanent = permanent.length > 0;
  const isEmpty = !hasPending && !hasPermanent;

  return (
    <AppLayout>
      <PageTransition>
        <div className="space-y-8">
          {/* Page header */}
          <div>
            <h2 className="text-2xl font-semibold text-neutral-900">Tool Authorizations</h2>
            <p className="mt-2 text-neutral-600">
              Review pending requests and manage permanent tool permissions.
            </p>
          </div>

          {/* Loading skeletons */}
          {loading && (
            <div className="space-y-4">
              {[1, 2, 3].map((i) => <CardSkeleton key={i} />)}
            </div>
          )}

          {/* Error state */}
          {!loading && error && (
            <InlineError error={error} onRetry={fetchAll} />
          )}

          {/* Empty state */}
          {!loading && !error && isEmpty && (
            <div className="text-center py-12">
              <p className="text-neutral-500">No tool authorizations yet.</p>
              <p className="text-sm text-neutral-400 mt-1">
                Pending requests and permanent decisions will appear here.
              </p>
            </div>
          )}

          {/* ── Pending Approvals ─────────────────────────────────────── */}
          {!loading && !error && hasPending && (
            <section className="space-y-4">
              <div className="flex items-center gap-2">
                <h3 className="text-lg font-semibold text-neutral-900">Pending Approvals</h3>
                <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-amber-100 text-amber-800 text-xs font-bold">
                  {pending.length}
                </span>
              </div>
              <div className="space-y-3">
                {pending.map((a) => (
                  <PendingApprovalCard key={a.id} approval={a} onResolved={handleResolved} />
                ))}
              </div>
            </section>
          )}

          {/* ── Permanent Authorizations ──────────────────────────────── */}
          {!loading && !error && hasPermanent && (
            <section className="space-y-4">
              <h3 className="text-lg font-semibold text-neutral-900">Permanent Authorizations</h3>
              <div className="space-y-3">
                {permanent.map((a) => (
                  <PermanentAuthCard
                    key={a.id}
                    approval={a}
                    onRevoke={handleRevoke}
                    revoking={revokingId === a.id}
                  />
                ))}
              </div>
            </section>
          )}
        </div>
      </PageTransition>
    </AppLayout>
  );
}

export default ToolAuthorizationsPage;
