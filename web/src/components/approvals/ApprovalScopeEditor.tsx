import { useEffect, useState } from 'react';
import { Accordion, AccordionItem, AccordionTrigger, AccordionContent } from '@design-system/components/advanced/Accordion';
import { Badge } from '@design-system/components/primitives/Badge';
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@design-system/components/inputs/Select';
import { Input } from '@design-system/components/inputs/Input';
import { Button } from '@design-system/components/primitives/Button';
import { approvalApi } from '@services/api/approvals';
import { approvalCopy } from '@copy/approvals';
import { humanizeParameterKey } from '@utils/humanize';
import type { ApprovalPersistence, ToolApprovalDetail } from '../../types/approval';

export interface ApprovalScopeEditorProps {
  approval: ToolApprovalDetail;
  paramsPattern: Record<string, string>;
  onParamsPatternChange: (value: Record<string, string>) => void;
  persistence: ApprovalPersistence;
  disabled?: boolean;
  onScopeValidationChange?: (valid: boolean) => void;
}

type ParameterMode = 'exact' | 'any' | 'custom';
const parameterModes = [
  { value: 'exact', label: approvalCopy.thisValue },
  { value: 'any', label: approvalCopy.anyValue },
  { value: 'custom', label: approvalCopy.customMatch },
] as const;
const valuePreviewLimit = 120;

export function ApprovalScopeEditor(props: ApprovalScopeEditorProps) {
  if (props.persistence === 'once') return null;
  return <ScopeEditor key={props.approval.id} {...props} />;
}

function ScopeEditor({ approval, paramsPattern, onParamsPatternChange, persistence, disabled = false, onScopeValidationChange }: ApprovalScopeEditorProps) {
  const [customKeys, setCustomKeys] = useState<Set<string>>(new Set());
  const [expandedValues, setExpandedValues] = useState<Set<string>>(new Set());
  const [lastPersistence, setLastPersistence] = useState(persistence);
  const [validation, setValidation] = useState<{ pattern: Record<string, string>; persistence: ApprovalPersistence; preview?: string; error: boolean } | null>(null);
  if (persistence !== lastPersistence) {
    setLastPersistence(persistence);
    setCustomKeys(new Set());
    setExpandedValues(new Set());
  }

  useEffect(() => {
    const controller = new AbortController();
    onScopeValidationChange?.(false);
    const timer = window.setTimeout(() => {
      void approvalApi.previewApprovalScope(approval.id, { params_pattern: paramsPattern }, { signal: controller.signal })
        .then((response) => {
          if (controller.signal.aborted) return;
          setValidation({ pattern: paramsPattern, persistence, preview: response.preview, error: false });
          onScopeValidationChange?.(true);
        })
        .catch(() => {
          if (controller.signal.aborted) return;
          setValidation({ pattern: paramsPattern, persistence, error: true });
          onScopeValidationChange?.(false);
        });
    }, 250);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [approval.id, paramsPattern, persistence, onScopeValidationChange]);

  const keys = Array.from(new Set([...Object.keys(approval.arguments), ...Object.keys(approval.params_pattern)])).sort();
  const exactPattern = (key: string) => Object.prototype.hasOwnProperty.call(approval.params_pattern, key) ? approval.params_pattern[key] : '';
  function modeOf(key: string): ParameterMode {
    if (!Object.prototype.hasOwnProperty.call(paramsPattern, key)) return 'any';
    return !customKeys.has(key) && paramsPattern[key] === exactPattern(key) ? 'exact' : 'custom';
  }
  function setParamPattern(key: string, value: string | undefined) {
    onScopeValidationChange?.(false);
    if (value === undefined) {
      const next = { ...paramsPattern };
      delete next[key];
      onParamsPatternChange(next);
    } else {
      // Computed properties preserve literal keys such as __proto__ from tool metadata.
      onParamsPatternChange({ ...paramsPattern, [key]: value });
    }
  }
  function selectMode(key: string, mode: string) {
    if (mode !== 'exact' && mode !== 'any' && mode !== 'custom') return;
    setCustomKeys((previous) => {
      const next = new Set(previous);
      if (mode === 'custom') next.add(key); else next.delete(key);
      return next;
    });
    setParamPattern(key, mode === 'any' ? undefined : mode === 'exact' ? exactPattern(key) : `${exactPattern(key)}*`);
  }
  const changedKeys = keys.filter((key) => modeOf(key) !== 'exact');
  const summary = changedKeys.length > 0
    ? approvalCopy.changedScope(changedKeys.slice(0, 3).map((key) => approvalCopy.changedParameter(humanizeParameterKey(key), modeOf(key) === 'any')), changedKeys.length)
    : approvalCopy.exactScope(keys.length);
  const currentValidation = validation?.pattern === paramsPattern && validation.persistence === persistence ? validation : null;
  const preview = currentValidation?.preview ?? (!currentValidation?.error && paramsPattern === approval.params_pattern ? approval.pattern_preview : undefined);

  return <div className="min-w-0 space-y-3">
    <Accordion type="single" collapsible>
      <AccordionItem value="scope">
        <AccordionTrigger aria-label={approvalCopy.scope}>
          <span>{approvalCopy.scope}</span>
          <Badge variant={changedKeys.length > 0 ? 'warning' : 'neutral'}>{changedKeys.length > 0 ? approvalCopy.customRule : approvalCopy.exactRequest}</Badge>
        </AccordionTrigger>
        <p className="px-2 pb-3 text-sm text-muted-foreground">{summary}</p>
        <AccordionContent>
          <div className="space-y-4">
            <p className="text-sm text-muted-foreground">{approvalCopy.scopeInstructions}</p>
            {keys.length > 0 && <div>
              <p className="text-xs font-semibold">{approvalCopy.requestDetails}</p>
              {keys.map((key) => {
                const mode = modeOf(key);
                const label = humanizeParameterKey(key);
                const argument = Object.prototype.hasOwnProperty.call(approval.arguments, key) ? approval.arguments[key] : undefined;
                const currentValue = (typeof argument === 'string' ? argument : JSON.stringify(argument)) ?? '';
                const controlId = `approval-${approval.id}-param-${key}-mode`;
                const expanded = expandedValues.has(key);
                const truncatable = currentValue.length > valuePreviewLimit;
                return <div key={key} data-testid={`approval-scope-param-${key}`} className="space-y-3 border-b border-border-soft py-4 last:border-b-0">
                  <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto_minmax(10rem,1fr)] sm:items-center">
                    <div className="min-w-0 space-y-1">
                      <p className="text-sm font-semibold">{label}</p>
                      {label !== key && <code className="break-all font-mono text-xs text-muted-foreground">{key}</code>}
                      <p className="text-xs text-muted-foreground">{approvalCopy.currentValue}{' '}
                        <span className="whitespace-pre-wrap break-all font-mono text-foreground">{currentValue === '' ? approvalCopy.empty : truncatable && !expanded ? `${currentValue.slice(0, valuePreviewLimit)}…` : currentValue}</span>
                      </p>
                    </div>
                    <span className="text-sm text-muted-foreground">{approvalCopy.matches}</span>
                    <Select value={mode} disabled={disabled} onValueChange={(value) => selectMode(key, value)}>
                      <SelectTrigger id={controlId} aria-label={approvalCopy.matchMode(label)} size="sm"><SelectValue /></SelectTrigger>
                      <SelectContent>{parameterModes.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}</SelectContent>
                    </Select>
                  </div>
                  {truncatable && <Button variant="ghost" size="sm" aria-expanded={expanded} onClick={() => setExpandedValues((previous) => {
                    const next = new Set(previous);
                    if (next.has(key)) next.delete(key); else next.add(key);
                    return next;
                  })}>{expanded ? approvalCopy.hideFullValue : approvalCopy.showFullValue}</Button>}
                  {mode === 'custom' && <Input id={`${controlId}-pattern`} label={approvalCopy.customMatchLabel(label)} value={paramsPattern[key] ?? ''} disabled={disabled} onChange={(event) => setParamPattern(key, event.target.value)} description={approvalCopy.customMatchDescription} />}
                </div>;
              })}
            </div>}
            <Button variant="outline" size="sm" disabled={disabled} onClick={() => {
              setCustomKeys(new Set());
              onScopeValidationChange?.(false);
              onParamsPatternChange({ ...approval.params_pattern });
            }}>{approvalCopy.resetScope}</Button>
            <div className="space-y-2 border-t border-border-soft pt-3">
              <p className="text-xs font-semibold">{approvalCopy.appliesTo}</p>
              <ul className="list-disc space-y-1 pl-5 text-sm [overflow-wrap:anywhere]">
                <li>{approvalCopy.onlyTool(approval.tool_name)}</li>
                {keys.map((key) => {
                  const label = humanizeParameterKey(key);
                  const mode = modeOf(key);
                  const argument = Object.prototype.hasOwnProperty.call(approval.arguments, key) ? approval.arguments[key] : undefined;
                  const value = (typeof argument === 'string' ? argument : JSON.stringify(argument)) ?? '';
                  return <li key={key}>{mode === 'any' ? approvalCopy.anyValueSummary(label) : mode === 'custom' ? approvalCopy.customValueSummary(label, paramsPattern[key] ?? '') : approvalCopy.exactValueSummary(label, value)}</li>;
                })}
              </ul>
            </div>
          </div>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
    <div aria-label={approvalCopy.patternPreview} className="space-y-2 rounded-md border border-border bg-muted p-3">
      <p className="text-xs font-semibold">{approvalCopy.technicalRule}</p>
      {preview && <code data-testid="approval-scope-preview" className="block whitespace-pre-wrap font-mono text-xs [overflow-wrap:anywhere]">{preview}</code>}
      {currentValidation?.error ? <p role="alert" className="text-sm text-destructive">{approvalCopy.invalidScope}</p> : !currentValidation && <p role="status" className="text-sm text-muted-foreground">{approvalCopy.validatingScope}</p>}
    </div>
  </div>;
}
