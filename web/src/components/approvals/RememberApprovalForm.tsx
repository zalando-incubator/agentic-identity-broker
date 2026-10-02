import { Button } from '@design-system/components/primitives/Button';
import { accessCopy, commonCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import { PersistenceSelector } from './PersistenceSelector';
import { ApprovalScopeEditor } from './ApprovalScopeEditor';
import type { ApprovalPersistence, ApproveRequest, ToolApprovalDetail } from '../../types/approval';

interface RememberApprovalFormProps {
  approval: ToolApprovalDetail;
  persistence: ApprovalPersistence;
  paramsPattern: Record<string, string>;
  submitting: boolean;
  scopeValid: boolean;
  onPersistenceChange: (value: ApprovalPersistence) => void;
  onParamsPatternChange: (value: Record<string, string>) => void;
  onScopeValidationChange: (valid: boolean) => void;
  onApprove: (request: ApproveRequest) => Promise<void>;
  onCancel: () => void;
}

export default function RememberApprovalForm({ approval, persistence, paramsPattern, submitting, scopeValid, onPersistenceChange, onParamsPatternChange, onScopeValidationChange, onApprove, onCancel }: RememberApprovalFormProps) {
  return <section id={`remember-${approval.id}`} className="space-y-4 rounded-lg border border-border p-4" aria-label={accessCopy.approveAndRemember}>
    <p className="text-sm text-muted-foreground">{approvalCopy.rememberDescription}</p>
    <PersistenceSelector value={persistence} onChange={onPersistenceChange} disabled={submitting} rememberOnly />
    <ApprovalScopeEditor approval={approval} paramsPattern={paramsPattern} onParamsPatternChange={onParamsPatternChange} persistence={persistence} disabled={submitting} onScopeValidationChange={onScopeValidationChange} />
    <div className="flex flex-wrap gap-3">
      <Button variant="secondary" disabled={submitting || persistence === 'once' || !scopeValid} onClick={() => {
        if (persistence !== 'once' && scopeValid) void onApprove({ persistence, params_pattern: paramsPattern });
      }}>{approvalCopy.confirmApproval}</Button>
      <Button variant="ghost" disabled={submitting} onClick={onCancel}>{commonCopy.cancel}</Button>
    </div>
  </section>;
}
