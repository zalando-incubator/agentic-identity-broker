import { Button } from '@components/ui/Button';
import { TextInput } from '@design-system/components/inputs/TextInput';
import type { ApprovalPersistence, ToolApprovalDetail } from '../../types/approval';
import { formatToolPattern } from '@utils/toolPattern';

interface PatternEditorProps {
  approval: ToolApprovalDetail;
  toolPattern: string;
  onToolPatternChange: (value: string) => void;
  paramsPattern: Record<string, string>;
  onParamsPatternChange: (value: Record<string, string>) => void;
  persistence: ApprovalPersistence;
  disabled?: boolean;
}

export function PatternEditor({ approval, toolPattern, onToolPatternChange, paramsPattern, onParamsPatternChange, persistence, disabled = false }: PatternEditorProps) {
  if (persistence === 'once') return null;
  const updateParam = (key: string, value: string) => {
    const next = { ...(paramsPattern ?? {}) };
    if (value === '') delete next[key];
    else next[key] = value;
    onParamsPatternChange(next);
  };
  const pattern = formatToolPattern(toolPattern, paramsPattern ?? {});
  const agentName = approval.agent_display_name?.trim() || 'this agent';
  const preview = persistence === 'permanent'
    ? `Always allow ${agentName} to call ${pattern}.`
    : `Allow ${agentName} to call ${pattern} for the rest of this session.`;

  return (
    <section className="space-y-3" aria-label="Approval coverage">
      <fieldset className="space-y-3">
        <legend className="text-sm font-semibold text-neutral-700">What should this cover?</legend>
        <TextInput id={`approval-${approval.id}-tool-pattern`} label="Tool name" value={toolPattern} onChange={(event) => onToolPatternChange(event.target.value)} disabled={disabled} />
        {Object.keys(approval.params_pattern ?? {}).sort().map((key) => (
          <TextInput id={`approval-${approval.id}-param-pattern-${key}`} key={key} label={key} value={paramsPattern[key] ?? ''} onChange={(event) => updateParam(key, event.target.value)} disabled={disabled} />
        ))}
        <p className="text-sm text-neutral-700">Use <code className="font-mono">*</code> as a wildcard. Clear a field to allow any value.</p>
        <Button type="button" variant="outline" size="sm" onClick={() => onParamsPatternChange({})} disabled={disabled}>Allow any parameters</Button>
      </fieldset>
      <div className="space-y-1 text-sm text-neutral-700" aria-label="Approval pattern preview">
        <code className="font-mono">{pattern}</code>
        <p>{preview}</p>
      </div>
    </section>
  );
}
