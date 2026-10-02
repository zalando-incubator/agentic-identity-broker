import { useId } from 'react';
import { RadioGroup, RadioGroupItem } from '@design-system/components/inputs/RadioGroup';
import { approvalCopy } from '@copy/approvals';
import type { ApprovalPersistence } from '../../types/approval';

export interface PersistenceSelectorProps {
  value: ApprovalPersistence;
  onChange: (value: ApprovalPersistence) => void;
  disabled?: boolean;
  name?: string;
  rememberOnly?: boolean;
}

const options = [
  { value: 'once', label: approvalCopy.once, description: approvalCopy.onceDescription },
  { value: 'session', label: approvalCopy.session, description: approvalCopy.sessionDescription },
  { value: 'permanent', label: approvalCopy.permanent, description: approvalCopy.permanentDescription },
] as const;

export function PersistenceSelector({ value, onChange, disabled = false, name, rememberOnly = false }: PersistenceSelectorProps) {
  const id = useId();
  return <fieldset className="space-y-3" disabled={disabled}>
    <legend className="mb-2 text-sm font-medium">{approvalCopy.persistenceLegend}</legend>
    <RadioGroup name={name ?? id} aria-label={approvalCopy.persistence} value={rememberOnly && value === 'once' ? '' : value} disabled={disabled} onValueChange={(next) => {
      if (next === 'once' || next === 'session' || next === 'permanent') onChange(next);
    }}>
      {options.filter((option) => !rememberOnly || option.value !== 'once').map((option) => <div key={option.value} className="flex items-start gap-3 rounded-md border border-border p-3">
        <RadioGroupItem id={`${id}-${option.value}`} value={option.value} aria-describedby={`${id}-${option.value}-description`} />
        <div className="min-w-0 space-y-1">
          <label htmlFor={`${id}-${option.value}`} className="block text-sm font-medium">{option.label}</label>
          <p id={`${id}-${option.value}-description`} className="text-sm text-muted-foreground">{option.description}</p>
        </div>
      </div>)}
    </RadioGroup>
    {value === 'permanent' && <p role="alert" className="rounded-md border border-border p-3 text-sm text-warning"><strong>{approvalCopy.warning}</strong> {approvalCopy.permanentWarning}</p>}
  </fieldset>;
}
