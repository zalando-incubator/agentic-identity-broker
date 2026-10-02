import { useState } from 'react';
import { Button } from '@design-system/components/primitives/Button';
import { useApprovalActions } from '@hooks/useApprovalReview';
import { accessCopy, commonCopy } from '@copy';
import { approvalQueueCopy as copy } from '@copy/approvalQueue';
import { approvalCopy } from '@copy/approvals';
import type { ApprovalPersistence, ToolApprovalDetail } from '../../types/approval';
import { ApprovalScopeEditor } from './ApprovalScopeEditor';
import { PersistenceSelector } from './PersistenceSelector';

export function InlineApprovalActions({ approval }: { approval: ToolApprovalDetail }) {
  const actions = useApprovalActions(approval.id);
  const [decision, setDecision] = useState<'approve' | 'deny' | null>(null);
  const [persistence, setPersistence] = useState<ApprovalPersistence>('once');
  const [paramsPattern, setParamsPattern] = useState(approval.params_pattern);
  const [scopeValid, setScopeValid] = useState(true);
  const expired = Date.parse(approval.expires_at) <= Date.now();
  const resolved = Boolean(actions.approveResult || actions.denyResult) || approval.status !== 'pending' || actions.errorCode === 'EXPIRED' || actions.errorCode === 'ALREADY_ACTIONED';
  const disabled = actions.submitting || expired || resolved;

  function changePersistence(value: ApprovalPersistence) {
    setPersistence(value);
    setParamsPattern(approval.params_pattern);
    setScopeValid(value === 'once');
  }

  function changePattern(value: Record<string, string>) {
    setScopeValid(false);
    setParamsPattern(value);
  }

  return <div className="min-w-0 space-y-3">
    <div className="flex flex-wrap gap-2">
      <Button variant="secondary" size="sm" disabled={disabled} aria-expanded={decision === 'approve'} onClick={() => setDecision('approve')}>{copy.approve}</Button>
      <Button variant="outline" size="sm" disabled={disabled} aria-expanded={decision === 'deny'} onClick={() => setDecision('deny')}>{accessCopy.deny}</Button>
    </div>
    {expired && <p className="text-sm text-muted-foreground">{copy.expired}</p>}
    {(actions.approveResult || actions.denyResult) && <p className="text-sm">{actions.approveResult ? approvalCopy.approved : approvalCopy.denied}. {approvalCopy.readonly}</p>}
    {decision !== 'deny' && <>
      <PersistenceSelector value={persistence} onChange={changePersistence} disabled={disabled} />
      {persistence === 'once' && <code data-testid="approval-scope-preview" className="block whitespace-pre-wrap break-all font-mono text-xs">{approval.pattern_preview}</code>}
      <ApprovalScopeEditor approval={approval} paramsPattern={paramsPattern} onParamsPatternChange={changePattern} persistence={persistence} disabled={disabled} onScopeValidationChange={setScopeValid} />
    </>}
    {decision === 'approve' && <div className="flex flex-wrap gap-2">
      <Button variant="secondary" size="sm" isLoading={actions.submitting} disabled={disabled || !scopeValid} onClick={() => void actions.approve({ persistence, ...(persistence === 'once' ? {} : { params_pattern: paramsPattern }) })}>{copy.confirmApprove}</Button>
      <Button variant="ghost" size="sm" disabled={actions.submitting} onClick={() => setDecision(null)}>{commonCopy.cancel}</Button>
    </div>}
    {decision === 'deny' && <>
      <p className="text-sm text-muted-foreground">{copy.denyDescription}</p>
      <code data-testid="approval-scope-preview" className="block whitespace-pre-wrap break-all font-mono text-xs">{approval.pattern_preview}</code>
      <p className="text-sm text-muted-foreground">{copy.permanentDenyWarning}</p>
      <div className="flex flex-wrap gap-2">
        <Button variant="secondary" size="sm" isLoading={actions.submitting} disabled={disabled} onClick={() => void actions.deny()}>{copy.denyOnce}</Button>
        <Button variant="outline" size="sm" disabled={disabled} onClick={() => void actions.deny(true)}>{copy.denyPermanently}</Button>
        <Button variant="ghost" size="sm" disabled={actions.submitting} onClick={() => setDecision(null)}>{commonCopy.cancel}</Button>
      </div>
    </>}
    {actions.errorCode && <p role="alert" className="text-sm text-destructive">{approvalCopy.errors[actions.errorCode].description}</p>}
  </div>;
}
