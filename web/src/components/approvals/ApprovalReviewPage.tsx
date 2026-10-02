import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import { Button } from '@design-system/components/primitives/Button';
import { accessCopy, commonCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import { ToolCallCard } from './ToolCallCard';
import { ApprovalConfirmation } from './ApprovalConfirmation';
import { ApprovalErrorBanner } from './ApprovalErrorBanner';
import type { ToolApprovalDetail, ApprovalPersistence, ApprovalErrorCode, ApproveResponseData, DenyResponseData, ApproveRequest } from '../../types/approval';

const RememberApprovalForm = lazy(() => import('./RememberApprovalForm'));
const PermanentDenialDialog = lazy(() => import('./PermanentDenialDialog'));

export interface ApprovalReviewPageProps {
  approval: ToolApprovalDetail;
  actingPrincipal: string;
  submitting: boolean;
  errorCode: ApprovalErrorCode | null;
  errorMessage: string | null;
  approveResult: ApproveResponseData | null;
  denyResult: DenyResponseData | null;
  onApprove: (request: ApproveRequest) => Promise<void>;
  onDeny: (permanent?: boolean) => Promise<void>;
  onRetry?: () => void;
}

/** Key the entire draft, including disclosures, when an already cached route changes. */
export function ApprovalReviewPage(props: ApprovalReviewPageProps) {
  return <ApprovalReview key={props.approval.id} {...props} />;
}

function ApprovalReview({ approval, actingPrincipal, submitting, errorCode, errorMessage, approveResult, denyResult, onApprove, onDeny, onRetry }: ApprovalReviewPageProps) {
  const [persistence, setPersistence] = useState<ApprovalPersistence>('once');
  const [remember, setRemember] = useState(false);
  const [paramsPattern, setParamsPattern] = useState(approval.params_pattern);
  const [scopeValid, setScopeValid] = useState(false);
  const [denyPermanentOpen, setDenyPermanentOpen] = useState(false);
  const permanentDenyTrigger = useRef<HTMLButtonElement>(null);
  const [now, setNow] = useState(Date.now);
  const expiresAt = Date.parse(approval.expires_at);
  useEffect(() => {
    if (approval.status !== 'pending' || expiresAt <= now || !Number.isFinite(expiresAt)) return;
    const timer = window.setTimeout(() => setNow(Date.now()), Math.min(expiresAt - now, 2_147_483_647));
    return () => window.clearTimeout(timer);
  }, [approval.status, expiresAt, now]);

  const conflictNotice = errorCode === 'ALREADY_ACTIONED' || errorCode === 'EXPIRED'
    ? <ApprovalErrorBanner errorCode={errorCode} message={errorMessage} outcome={false} />
    : null;
  if (approveResult || approval.status === 'approved') return <div className="space-y-4">
    {conflictNotice}
    <ApprovalConfirmation type="approved" persistence={approveResult?.persistence ?? approval.persistence} decidedAt={approveResult?.approved_at ?? approval.approved_at} toolName={approval.tool_name} agentName={approval.agent_display_name ?? approval.agent_id} historical={!approveResult} consumed={approval.consumed} />
  </div>;
  if (denyResult || approval.status === 'denied') return <div className="space-y-4">
    {conflictNotice}
    <ApprovalConfirmation type="denied" persistence={denyResult?.persistence ?? approval.persistence} decidedAt={denyResult?.denied_at ?? approval.denied_at} toolName={approval.tool_name} agentName={approval.agent_display_name ?? approval.agent_id} historical={!denyResult} />
  </div>;
  if (expiresAt <= now || errorCode === 'EXPIRED') return <ApprovalErrorBanner errorCode="EXPIRED" />;
  if (errorCode === 'ALREADY_ACTIONED' || errorCode === 'FORBIDDEN' || errorCode === 'NOT_FOUND') return <ApprovalErrorBanner errorCode={errorCode} message={errorMessage} />;

  function changePersistence(value: ApprovalPersistence) {
    setPersistence(value);
    setParamsPattern(approval.params_pattern);
    setScopeValid(false);
  }

  return <div className="min-w-0 space-y-6">
    <header className="space-y-2">
      <h1 className="font-display text-2xl font-semibold">{approvalCopy.title}</h1>
      <p className="text-sm text-muted-foreground">{approvalCopy.description}</p>
    </header>
    <ToolCallCard approval={approval} actingPrincipal={actingPrincipal} showScope={persistence === 'once'} />
    {errorCode && <ApprovalErrorBanner errorCode={errorCode} message={errorMessage} onRetry={onRetry} outcome={false} />}
    <div className="flex flex-wrap gap-3">
      <Button variant="primary" disabled={submitting} isLoading={submitting} onClick={() => void onApprove({ persistence: 'once' })}>{accessCopy.approveOnce}</Button>
      <Button variant="secondary" disabled={submitting} aria-expanded={remember} aria-controls={`remember-${approval.id}`} onClick={() => {
        setRemember((previous) => !previous);
        changePersistence('once');
      }}>{accessCopy.approveAndRemember}</Button>
      <Button variant="secondary" disabled={submitting} onClick={() => void onDeny()}>{accessCopy.deny}</Button>
    </div>
    {remember && <Suspense fallback={<p role="status">{commonCopy.loading}</p>}>
      <RememberApprovalForm
        approval={approval}
        persistence={persistence}
        paramsPattern={paramsPattern}
        submitting={submitting}
        scopeValid={scopeValid}
        onPersistenceChange={changePersistence}
        onParamsPatternChange={setParamsPattern}
        onScopeValidationChange={setScopeValid}
        onApprove={onApprove}
        onCancel={() => { setRemember(false); changePersistence('once'); }}
      />
    </Suspense>}
    <Button ref={permanentDenyTrigger} variant="ghost" disabled={submitting} aria-haspopup="dialog" aria-expanded={denyPermanentOpen} className="h-auto max-w-full whitespace-normal text-left" onClick={() => setDenyPermanentOpen(true)}>{approvalCopy.denyPermanently}</Button>
    {denyPermanentOpen && <Suspense fallback={<p role="status">{commonCopy.loading}</p>}>
      <PermanentDenialDialog
        approval={approval}
        submitting={submitting}
        onCancel={() => setDenyPermanentOpen(false)}
        onConfirm={() => { setDenyPermanentOpen(false); void onDeny(true); }}
        onReturnFocus={() => permanentDenyTrigger.current?.focus()}
      />
    </Suspense>}
  </div>;
}
