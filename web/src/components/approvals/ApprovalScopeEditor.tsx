/**
 * ApprovalScopeEditor - Plain-language editor for what a session or permanent
 * approval covers.
 *
 * Collapsed by default: the summary states what future calls must match. Expanded,
 * each parameter offers "This value", "Any value" or a custom glob, and the raw
 * pattern stays visible as the technical rule.
 */

import { useEffect, useRef, useState } from 'react';
import { Accordion } from '@design-system/components/advanced/Accordion';
import { Badge } from '@design-system/components/primitives/Badge';
import { Select, type SelectOption } from '@design-system/components/inputs/Select';
import { TextInput } from '@design-system/components/inputs/TextInput';
import { Button } from '@components/ui/Button';
import { approvalApi } from '@services/api/approvals';
import { humanizeParameterKey } from '@utils/humanize';
import type { ApprovalPersistence, ToolApprovalDetail } from '../../types/approval';

interface ApprovalScopeEditorProps {
  approval: ToolApprovalDetail;
  paramsPattern: Record<string, string>;
  onParamsPatternChange: (value: Record<string, string>) => void;
  persistence: ApprovalPersistence;
  disabled?: boolean;
  onScopeValidationChange?: (valid: boolean) => void;
}

type ParameterMode = 'exact' | 'any' | 'custom';


const PARAMETER_MODE_OPTIONS: SelectOption[] = [
  { value: 'exact', label: 'This value' },
  { value: 'any', label: 'Any value' },
  { value: 'custom', label: 'Custom match' },
];

const VALUE_PREVIEW_LIMIT = 120;

function isParameterMode(value: string | number | (string | number)[] | null): value is ParameterMode {
  return value === 'exact' || value === 'any' || value === 'custom';
}


function exactParamPattern(approval: ToolApprovalDetail, key: string): string {
  return approval.params_pattern?.[key] ?? '';
}

function parameterKeys(approval: ToolApprovalDetail): string[] {
  const keys = new Set([
    ...Object.keys(approval.arguments ?? {}),
    ...Object.keys(approval.params_pattern ?? {}),
  ]);
  return Array.from(keys).sort();
}


export function ApprovalScopeEditor({
  approval,
  paramsPattern,
  onParamsPatternChange,
  persistence,
  disabled = false,
  onScopeValidationChange,
}: ApprovalScopeEditorProps) {
  const [customKeys, setCustomKeys] = useState<Set<string>>(new Set());
  const [expandedValues, setExpandedValues] = useState<Set<string>>(new Set());
  const [lastPersistence, setLastPersistence] = useState(persistence);
  const validationRequest = useRef(0);
  const [serverPreview, setServerPreview] = useState(approval.pattern_preview);

  // Call sites restore the exact patterns whenever persistence changes, so the sticky
  // custom-mode flags must drop with them.
  if (persistence !== lastPersistence) {
    setLastPersistence(persistence);
    setCustomKeys(new Set());
    setExpandedValues(new Set());
  }


  const keys = parameterKeys(approval);
  const constraints = paramsPattern ?? {};

  useEffect(() => {
    if (persistence === 'once') return;
    const request = ++validationRequest.current;
    onScopeValidationChange?.(false);
    const timer = window.setTimeout(() => {
      void approvalApi
        .previewApprovalScope(approval.id, {
          params_pattern: constraints,
        })
        .then((response) => {
          if (validationRequest.current === request) {
            setServerPreview(response.preview);
            onScopeValidationChange?.(true);
          }
        })
        .catch(() => {
          if (validationRequest.current === request) onScopeValidationChange?.(false);
        });
    }, 250);
    return () => window.clearTimeout(timer);
  }, [approval.id, constraints, onScopeValidationChange, persistence]);

  if (persistence === 'once') return null;


  const modeOf = (key: string): ParameterMode => {
    const value = constraints[key];
    if (value === undefined) return 'any';
    if (!customKeys.has(key) && value === exactParamPattern(approval, key)) return 'exact';
    return 'custom';
  };

  const withCustomKey = (key: string, custom: boolean) => {
    setCustomKeys((previous) => {
      if (previous.has(key) === custom) return previous;
      const next = new Set(previous);
      if (custom) next.add(key);
      else next.delete(key);
      return next;
    });
  };


  const setParamPattern = (key: string, value: string | undefined) => {
    const next = { ...constraints };
    if (value === undefined) delete next[key];
    else next[key] = value;
    onParamsPatternChange(next);
  };

  const selectMode = (key: string, mode: ParameterMode) => {
    withCustomKey(key, mode === 'custom');
    if (mode === 'any') setParamPattern(key, undefined);
    else if (mode === 'exact') setParamPattern(key, exactParamPattern(approval, key));
    else setParamPattern(key, `${exactParamPattern(approval, key)}*`);
  };


  const toggleExpanded = (key: string) => {
    setExpandedValues((previous) => {
      const next = new Set(previous);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const resetToRequest = () => {
    setCustomKeys(new Set());
    const exact: Record<string, string> = {};
    for (const key of keys) exact[key] = exactParamPattern(approval, key);
    onParamsPatternChange(exact);
  };

  const changedKeys = keys.filter((key) => modeOf(key) !== 'exact');
  const customized = changedKeys.length > 0;

  const changeSummaries = changedKeys.map(
    (key) =>
      `${humanizeParameterKey(key)} (${modeOf(key) === 'any' ? 'any value' : 'custom match'})`,
  );

  const description = customized
    ? `Future calls may vary: ${changeSummaries.slice(0, 3).join(', ')}${
        changeSummaries.length > 3 ? `, +${changeSummaries.length - 3} more` : ''
      }`
    : keys.length === 0
      ? 'Future calls must match this tool.'
      : `Future calls must match this tool and all ${keys.length} values from this request.`;


  const content = (
    <div className="space-y-6">
      <p className="border-l-2 border-trust/20 pl-3 text-sm text-secondary">
        Future calls must match this tool and these values. Change only what should be allowed
        to differ.
      </p>


      {keys.length > 0 && (
        <div className="space-y-0">
          <p className="pb-2 text-xs font-semibold uppercase tracking-wide text-trust">Request details</p>
          {keys.map((key) => {
            const mode = modeOf(key);
            const label = humanizeParameterKey(key);
            const currentValue = (typeof approval.arguments?.[key] === 'string'
              ? approval.arguments[key]
              : JSON.stringify(approval.arguments?.[key])) ?? '';
            const modeControlId = `approval-${approval.id}-param-${key}-mode`;
            const truncatable = currentValue.length > VALUE_PREVIEW_LIMIT;
            const expanded = expandedValues.has(key);

            return (
              <div
                key={key}
                data-testid={`approval-scope-param-${key}`}
                className="space-y-3 border-b border-neutral-200 py-5 first:pt-3 last:border-b-0 last:pb-0"
              >
                <div className="grid gap-x-3 gap-y-2 sm:grid-cols-[minmax(0,1fr)_auto_minmax(12rem,0.8fr)] sm:items-center">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                      <span className="text-sm font-semibold text-trust">{label}</span>
                      {label !== key && (
                        <code className="font-mono text-xs text-neutral-500 break-all">{key}</code>
                      )}
                    </div>
                    <p className="mt-1 text-xs text-tertiary">
                      Current value{' '}
                      {currentValue === '' ? (
                        <span className="italic">(empty)</span>
                      ) : (
                        <span
                          className={
                            truncatable && expanded
                              ? 'block font-mono text-neutral-800 whitespace-pre-wrap break-all max-h-40 overflow-auto'
                              : 'font-mono text-neutral-800 break-all'
                          }
                        >
                          {truncatable && !expanded
                            ? `${currentValue.slice(0, VALUE_PREVIEW_LIMIT)}…`
                            : currentValue}
                        </span>
                      )}
                    </p>
                  </div>
                  <span className="text-sm text-secondary">matches</span>
                  <Select
                    id={modeControlId}
                    aria-label={`${label} match mode`}
                    size="sm"
                    options={PARAMETER_MODE_OPTIONS}
                    value={mode}
                    disabled={disabled}
                    className="[&>div:last-child]:hidden"
                    onChange={(value) => {
                      if (isParameterMode(value)) selectMode(key, value);
                    }}
                  />
                </div>

                {truncatable && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => toggleExpanded(key)}
                  >
                    {expanded ? 'Hide full value' : 'Show full value'}
                  </Button>
                )}

                {mode === 'custom' && (
                  <TextInput
                    id={`${modeControlId}-pattern`}
                    size="sm"
                    label={`${label} custom match`}
                    value={constraints[key] ?? ''}
                    disabled={disabled}
                    onChange={(event) => {
                      withCustomKey(key, true);
                      setParamPattern(key, event.target.value);
                    }}
                    helperText="Use * for any characters. Example: acme/* matches values that start with acme/."
                    successMessage="Validated by the broker."
                  />
                )}
              </div>
            );
          })}
        </div>
      )}

      <Button type="button" variant="outline" size="sm" onClick={resetToRequest} disabled={disabled}>
        Reset to this request
      </Button>

      <div className="space-y-2 border-t border-neutral-200 pt-5">
        <p className="text-xs font-semibold uppercase tracking-wide text-trust">Applies to</p>
        <ul className="list-disc space-y-1 pl-5 text-sm text-neutral-700 break-words">
          <li>{`Only the tool ${approval.tool_name}`}</li>
          {keys.map((key) => {
            const mode = modeOf(key);
            const label = humanizeParameterKey(key);
            if (mode === 'any') return <li key={key}>{`${label}: any value`}</li>;
            if (mode === 'custom') {
              return <li key={key}>{`${label}: values matching ${constraints[key] ?? ''}`}</li>;
            }
            return (
              <li key={key}>{`${label}: exactly ${String(approval.arguments?.[key] ?? '')}`}</li>
            );
          })}
        </ul>
      </div>

      <div className="space-y-2 border-t border-neutral-200 pt-5" aria-label="Approval pattern preview">
        <p className="text-xs font-semibold uppercase tracking-wide text-trust">Technical rule</p>
        <code className="font-mono text-xs text-neutral-500 break-words">
          {serverPreview}
        </code>
      </div>
    </div>
  );

  return (
    <Accordion
      size="sm"
      items={[
        {
          id: `approval-${approval.id}-scope`,
          title: 'Approval scope',
          description,
          badge: customized ? (
            <Badge variant="warning" size="sm">
              Custom rule
            </Badge>
          ) : (
            <Badge variant="neutral" size="sm">
              Exact request
            </Badge>
          ),
          content,
        },
      ]}
    />
  );
}

export default ApprovalScopeEditor;
