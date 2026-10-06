import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import { ChevronDown } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button';
import { accessCopy, commonCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import { approvalQueueCopy } from '@copy/approvalQueue';
import { ToolCallCard } from './ToolCallCard';
import { RiskBadge } from './RiskBadge';
import { ApprovalConfirmation } from './ApprovalConfirmation';
import { ApprovalErrorBanner } from './ApprovalErrorBanner';
import type { ToolApprovalDetail, ApprovalPersistence, ApprovalErrorCode, ApproveResponseData, DenyResponseData, ApproveRequest, ScopePreview } from '../../types/approval';

const ApprovalScopeEditor = lazy(() => import('./ApprovalScopeEditor').then(({ ApprovalScopeEditor: component }) => ({ default: component })));
const PermanentDenialDialog = lazy(() => import('./PermanentDenialDialog'));
const ApprovalDecisionOptions = lazy(() => import('./ApprovalDecisionOptions'));
const approveLabels = {
  once: accessCopy.approveOnce,
  session: approvalQueueCopy.approveSession,
  permanent: approvalQueueCopy.approveAlways,
};

export interface ApprovalReviewPageProps {
  approval: ToolApprovalDetail;
  inboxSelected?: boolean;
  decisionEnabled?: boolean;
  actingPrincipal: string;
  submitting: boolean;
  submittingAction: 'approve' | 'deny' | null;
  errorCode: ApprovalErrorCode | null;
  errorMessage: string | null;
  approveResult: ApproveResponseData | null;
  denyResult: DenyResponseData | null;
  onApprove: (request: ApproveRequest) => Promise<void>;
  onDeny: (permanent?: boolean) => Promise<void>;
  onPreview: (paramsPattern: Record<string, string>, signal: AbortSignal) => Promise<ScopePreview>;
  onRetry?: () => void;
}

/** A changed request always gets a fresh scope draft and its own expiry deadline. */
export function ApprovalReviewPage(props: ApprovalReviewPageProps) {
  return <ApprovalReview key={props.approval.id} {...props} />;
}

function ApprovalReview({ approval, actingPrincipal, inboxSelected = false, decisionEnabled = true, submitting, submittingAction, errorCode, errorMessage, approveResult, denyResult, onApprove, onDeny, onPreview, onRetry }: ApprovalReviewPageProps) {
  const [persistence, setPersistence] = useState<Extract<ApprovalPersistence, 'session' | 'permanent'> | null>(null);
  const [approveChoice, setApproveChoice] = useState<ApprovalPersistence>('once');
  const [denyPermanent, setDenyPermanent] = useState(false);
  const [paramsPattern, setParamsPattern] = useState(approval.params_pattern);
  const [scopeValid, setScopeValid] = useState(false);
  const [denyPermanentOpen, setDenyPermanentOpen] = useState(false);
  const [denyMenuRequested, setDenyMenuRequested] = useState(false);
  const [approveMenuRequested, setApproveMenuRequested] = useState(false);
  const permanentDenyTrigger = useRef<HTMLButtonElement>(null);
  const [now, setNow] = useState(Date.now);
  const expiresAt = Date.parse(approval.expires_at);
  useEffect(() => {
    if (approval.status !== 'pending' || expiresAt <= now || !Number.isFinite(expiresAt)) return;
    const timer = window.setTimeout(() => setNow(Date.now()), Math.min(expiresAt - now, inboxSelected ? 60_000 : 2_147_483_647));
    return () => window.clearTimeout(timer);
  }, [approval.status, expiresAt, inboxSelected, now]);

  useEffect(() => {
    if (!decisionEnabled) {
      setDenyPermanentOpen(false);
      setDenyMenuRequested(false);
      setApproveMenuRequested(false);
    }
  }, [decisionEnabled]);

  const conflictNotice = errorCode === 'ALREADY_ACTIONED' || errorCode === 'EXPIRED'
    ? <ApprovalErrorBanner errorCode={errorCode} message={errorMessage} outcome={false} /> : null;
  if (approveResult || approval.status === 'approved') return <div className="space-y-4">
    {conflictNotice}
    <ApprovalConfirmation type="approved" persistence={approveResult?.persistence ?? approval.persistence} decidedAt={approveResult?.approved_at ?? approval.approved_at} toolName={approval.tool_name} agentName={approval.agent_display_name ?? approval.agent_id} historical={!approveResult} consumed={approval.consumed} />
  </div>;
  if (denyResult || approval.status === 'denied') return <div className="space-y-4">
    {conflictNotice}
    <ApprovalConfirmation type="denied" persistence={denyResult?.persistence ?? approval.persistence} decidedAt={denyResult?.denied_at ?? approval.denied_at} toolName={approval.tool_name} agentName={approval.agent_display_name ?? approval.agent_id} historical={!denyResult} />
  </div>;
  if (!Number.isFinite(expiresAt) || expiresAt <= Math.max(now, Date.now()) || errorCode === 'EXPIRED') return <ApprovalErrorBanner errorCode="EXPIRED" />;
  if (errorCode === 'ALREADY_ACTIONED' || errorCode === 'FORBIDDEN' || errorCode === 'NOT_FOUND') return <ApprovalErrorBanner errorCode={errorCode} message={errorMessage} />;

  function openEditor(value: 'session' | 'permanent') {
    setDenyMenuRequested(false);
    setApproveMenuRequested(false);
    setPersistence(value);
    setParamsPattern(approval.params_pattern);
    setScopeValid(false);
  }

  function approveSelected() {
    if (approveChoice === 'once') void onApprove({ persistence: 'once' });
    else openEditor(approveChoice);
  }

  function denySelected() {
    if (denyPermanent) setDenyPermanentOpen(true);
    else void onDeny();
  }

  const highRisk = approval.risk_level?.toLowerCase() === 'high' || approval.risk_level?.toLowerCase() === 'critical';
  const agentName = approval.agent_display_name?.trim() || approval.agent_id;
  const inboxDescription = approval.description?.trim();
  const inboxTitle = inboxDescription || approval.tool_name;
  const decisionDisabled = submitting || !decisionEnabled;
  return <section data-testid="approval-review-panel" data-editor-active={persistence ? 'true' : undefined}
    aria-label={inboxSelected ? `${approvalQueueCopy.selectedRequest}: ${approvalCopy.decisionContext(inboxTitle, agentName)}` : approvalCopy.title}
    tabIndex={-1} className={`flex h-[min(42rem,calc(100dvh-12rem))] min-h-80 min-w-0 flex-col overflow-hidden rounded-xl border border-border-subtle bg-card focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${inboxSelected ? 'border-l-4 border-l-primary' : ''}`}>
    <div role="region" aria-label={approvalCopy.requestDetails} tabIndex={inboxSelected ? 0 : undefined} className={`min-w-0 shrink-0 border-b border-border-subtle p-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring ${inboxSelected ? 'max-h-[50%] overflow-y-auto' : ''} ${highRisk ? 'bg-status-danger' : ''}`}>
      {inboxSelected ? <div data-testid="selected-approval-identity" className="min-w-0 space-y-2">
        <h2 data-testid={inboxDescription ? undefined : 'approval-tool-name'} className={`min-w-0 text-xl font-semibold [overflow-wrap:anywhere] ${inboxDescription ? 'font-display' : 'font-mono'}`}>{inboxTitle}</h2>
        {inboxDescription && <p data-testid="approval-tool-name" className="font-mono text-xs [overflow-wrap:anywhere]">{approval.tool_name}</p>}
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted-foreground">
          <span data-testid="approval-agent-name" className="min-w-0 [overflow-wrap:anywhere]">{agentName}</span>
          <RiskBadge level={approval.risk_level} />
          <time className="tabular-nums" dateTime={approval.expires_at} data-screenshot-dynamic>{approvalCopy.expiry(new Date(approval.expires_at).toLocaleString())}</time>
        </div>
      </div> : <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h1 className="font-display text-xl font-semibold">{approvalCopy.title}</h1>
          <p className="text-sm text-muted-foreground">{approvalCopy.description}</p>
        </div>
        <RiskBadge level={approval.risk_level} />
      </div>}
    </div>
    <div className="min-h-0 min-w-0 flex-1 overflow-y-auto p-4">
      {persistence ? <div className="space-y-4" id={`remember-${approval.id}`}>
        {!inboxSelected && <p className="break-words text-sm font-semibold"><span className="font-mono [overflow-wrap:anywhere]">{approval.tool_name}</span>{' '}{approvalCopy.forAgent(approval.agent_display_name ?? approval.agent_id)}</p>}
        <p className="text-sm text-muted-foreground">{approvalCopy.rememberDescription}</p>
        <h2 className="font-sans text-sm font-semibold">{persistence === 'session' ? approvalCopy.session : approvalCopy.permanent}</h2>
        {persistence === 'permanent' && <p role="alert" className="rounded-lg bg-warning p-3 text-sm text-warning-foreground">{approvalCopy.permanentWarning}</p>}
        <Suspense fallback={<p role="status">{commonCopy.loading}</p>}>
          <ApprovalScopeEditor approval={approval} persistence={persistence} paramsPattern={paramsPattern} onParamsPatternChange={setParamsPattern} disabled={decisionDisabled} onPreview={onPreview} onScopeValidationChange={setScopeValid} />
        </Suspense>
      </div> : <ToolCallCard approval={approval} actingPrincipal={inboxSelected ? undefined : actingPrincipal} inboxSelected={inboxSelected} showExpiry={!inboxSelected} />}
      {errorCode && <div className="mt-4"><ApprovalErrorBanner errorCode={errorCode} message={errorMessage} onRetry={onRetry} outcome={false} /></div>}
    </div>
    <footer className={`shrink-0 border-t border-border-subtle bg-card p-3 ${persistence ? 'flex items-center justify-between gap-2 sm:gap-3' : 'grid grid-cols-2 items-center gap-2 sm:flex sm:justify-between sm:gap-3'}`}>
      {persistence ? <>
        <Button variant="outline" disabled={decisionDisabled} onClick={() => { setPersistence(null); setScopeValid(false); }}>{approvalQueueCopy.back}</Button>
        <Button variant="primary" disabled={decisionDisabled || !scopeValid} isLoading={submittingAction === 'approve'} onClick={() => { if (scopeValid) void onApprove({ persistence, params_pattern: paramsPattern }); }}>{approvalCopy.confirmApproval}</Button>
      </> : <>
        <div className="flex min-w-0 items-stretch rounded-lg border border-border bg-background">
          <Button ref={permanentDenyTrigger} data-approval-action="deny" variant="outline" disabled={decisionDisabled} isLoading={submittingAction === 'deny'} className="min-w-0 flex-1 rounded-r-none border-0 gap-1 whitespace-normal px-1 py-0 sm:gap-2 sm:px-4" onClick={denySelected}><span className="min-w-0 whitespace-normal leading-4">{denyPermanent ? approvalQueueCopy.denyAlways : accessCopy.deny}</span>{inboxSelected && <kbd aria-hidden="true" className="shrink-0 rounded border border-current/25 px-1 font-mono text-[10px] leading-4">D</kbd>}</Button>
          {denyMenuRequested && decisionEnabled ? <Suspense fallback={<Button variant="outline" size="icon" aria-label={approvalQueueCopy.denyOptions} aria-haspopup="menu" aria-busy="true" disabled className="rounded-l-none border-0 border-l border-border"><ChevronDown aria-hidden="true" /></Button>}>
            <ApprovalDecisionOptions kind="deny" disabled={submitting} selected={denyPermanent} onSelect={setDenyPermanent} />
          </Suspense> : <Button variant="outline" size="icon" aria-label={approvalQueueCopy.denyOptions} aria-haspopup="menu" disabled={decisionDisabled} className="rounded-l-none border-0 border-l border-border" onClick={() => setDenyMenuRequested(true)}><ChevronDown aria-hidden="true" /></Button>}
        </div>
        <div className="flex min-w-0 items-stretch rounded-lg border border-primary bg-primary">
          <Button data-approval-action="approve" variant="primary" disabled={decisionDisabled} isLoading={submittingAction === 'approve'} className="min-w-0 flex-1 rounded-r-none border-0 gap-1 whitespace-normal px-1 py-0 sm:gap-2 sm:px-4" onClick={approveSelected}><span className="min-w-0 whitespace-normal leading-4">{approveLabels[approveChoice]}</span>{inboxSelected && <kbd aria-hidden="true" className="shrink-0 rounded border border-current/25 px-1 font-mono text-[10px] leading-4">A</kbd>}</Button>
          {approveMenuRequested && decisionEnabled ? <Suspense fallback={<Button variant="primary" size="icon" aria-label={approvalQueueCopy.approveOptions} aria-haspopup="menu" aria-busy="true" disabled className="rounded-l-none border-0 border-l border-primary-foreground/30"><ChevronDown aria-hidden="true" /></Button>}>
            <ApprovalDecisionOptions kind="approve" disabled={submitting} selected={approveChoice} onSelect={setApproveChoice} />
          </Suspense> : <Button variant="primary" size="icon" aria-label={approvalQueueCopy.approveOptions} aria-haspopup="menu" disabled={decisionDisabled} className="rounded-l-none border-0 border-l border-primary-foreground/30" onClick={() => setApproveMenuRequested(true)}><ChevronDown aria-hidden="true" /></Button>}
        </div>
      </>}
    </footer>
    {denyPermanentOpen && decisionEnabled && <Suspense fallback={<p role="status">{commonCopy.loading}</p>}>
      <PermanentDenialDialog approval={approval} submitting={submitting} submittingAction={submittingAction} onCancel={() => setDenyPermanentOpen(false)} onConfirm={() => { setDenyPermanentOpen(false); void onDeny(true); }} onReturnFocus={() => permanentDenyTrigger.current?.focus()} />
    </Suspense>}
  </section>;
}
