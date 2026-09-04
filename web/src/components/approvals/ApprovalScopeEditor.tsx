/**
 * ApprovalScopeEditor - Plain-language editor for what a session or permanent
 * approval covers.
 *
 * Collapsed by default: the summary states what future calls must match. Expanded,
 * each parameter offers "This value", "Any value" or a custom glob, and the raw
 * pattern stays visible as the technical rule.
 */

import { useState } from 'react';
import { Accordion } from '@design-system/components/advanced/Accordion';
import { Badge } from '@design-system/components/primitives/Badge';
import { Select, type SelectOption } from '@design-system/components/inputs/Select';
import { TextInput } from '@design-system/components/inputs/TextInput';
import { Button } from '@components/ui/Button';
import {
  canonicalArgumentValue,
  escapeGlobLiteral,
  formatToolPattern,
  humanizeParameterKey,
  matchesGlob,
  unescapeGlobLiteral,
  validateParamGlob,
  validateToolGlob,
} from '@utils/toolPattern';
import type { ApprovalPersistence, ToolApprovalDetail } from '../../types/approval';

interface ApprovalScopeEditorProps {
  approval: ToolApprovalDetail;
  toolPattern: string;
  onToolPatternChange: (value: string) => void;
  paramsPattern: Record<string, string>;
  onParamsPatternChange: (value: Record<string, string>) => void;
  persistence: ApprovalPersistence;
  disabled?: boolean;
}

export interface ScopeIssues {
  tool: string | null;
  params: Record<string, string>;
}

type ParameterMode = 'exact' | 'any' | 'custom';
type ToolMode = 'exact' | 'custom';

const TOOL_MODE_OPTIONS: SelectOption[] = [
  { value: 'exact', label: 'Exactly this tool' },
  { value: 'custom', label: 'Custom match' },
];

const PARAMETER_MODE_OPTIONS: SelectOption[] = [
  { value: 'exact', label: 'This value' },
  { value: 'any', label: 'Any value' },
  { value: 'custom', label: 'Custom match' },
];

const NOT_COVERING = 'This pattern does not match the current value.';
const VALUE_PREVIEW_LIMIT = 120;

function isParameterMode(value: string | number | (string | number)[] | null): value is ParameterMode {
  return value === 'exact' || value === 'any' || value === 'custom';
}

function isToolMode(value: string | number | (string | number)[] | null): value is ToolMode {
  return value === 'exact' || value === 'custom';
}

function exactToolPattern(approval: ToolApprovalDetail): string {
  return approval.tool_pattern || approval.tool_name;
}

function exactParamPattern(approval: ToolApprovalDetail, key: string): string {
  return (
    approval.params_pattern?.[key] ??
    escapeGlobLiteral(canonicalArgumentValue(approval.arguments?.[key]))
  );
}

function parameterKeys(approval: ToolApprovalDetail): string[] {
  const keys = new Set([
    ...Object.keys(approval.arguments ?? {}),
    ...Object.keys(approval.params_pattern ?? {}),
  ]);
  return Array.from(keys).sort();
}

export function resolveScopeIssues(
  approval: ToolApprovalDetail,
  toolPattern: string,
  paramsPattern: Record<string, string>,
): ScopeIssues {
  const tool =
    validateToolGlob(toolPattern) ??
    (matchesGlob(toolPattern, approval.tool_name) ? null : NOT_COVERING);

  const params: Record<string, string> = {};
  const constraints = paramsPattern ?? {};
  for (const key of Object.keys(constraints)) {
    const pattern = constraints[key];
    const currentValue = unescapeGlobLiteral(exactParamPattern(approval, key));
    const issue =
      validateParamGlob(pattern) ?? (matchesGlob(pattern, currentValue) ? null : NOT_COVERING);
    if (issue) params[key] = issue;
  }

  return { tool, params };
}

export function hasScopeIssues(issues: ScopeIssues): boolean {
  return issues.tool !== null || Object.keys(issues.params).length > 0;
}

export function ApprovalScopeEditor({
  approval,
  toolPattern,
  onToolPatternChange,
  paramsPattern,
  onParamsPatternChange,
  persistence,
  disabled = false,
}: ApprovalScopeEditorProps) {
  const [customKeys, setCustomKeys] = useState<Set<string>>(new Set());
  const [toolCustom, setToolCustom] = useState(false);
  const [expandedValues, setExpandedValues] = useState<Set<string>>(new Set());
  const [lastPersistence, setLastPersistence] = useState(persistence);

  // Call sites restore the exact patterns whenever persistence changes, so the sticky
  // custom-mode flags must drop with them.
  if (persistence !== lastPersistence) {
    setLastPersistence(persistence);
    setCustomKeys(new Set());
    setToolCustom(false);
    setExpandedValues(new Set());
  }

  if (persistence === 'once') return null;

  const exactTool = exactToolPattern(approval);
  const keys = parameterKeys(approval);
  const constraints = paramsPattern ?? {};
  const issues = resolveScopeIssues(approval, toolPattern, constraints);

  const toolMode: ToolMode = toolCustom || toolPattern !== exactTool ? 'custom' : 'exact';

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

  const selectToolMode = (mode: ToolMode) => {
    setToolCustom(mode === 'custom');
    onToolPatternChange(mode === 'exact' ? exactTool : `${exactTool}*`);
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
    setToolCustom(false);
    onToolPatternChange(exactTool);
    const exact: Record<string, string> = {};
    for (const key of keys) exact[key] = exactParamPattern(approval, key);
    onParamsPatternChange(exact);
  };

  const changedKeys = keys.filter((key) => modeOf(key) !== 'exact');
  const customized = toolMode === 'custom' || changedKeys.length > 0;

  const changeSummaries = [
    ...(toolMode === 'custom' ? ['Tool (custom match)'] : []),
    ...changedKeys.map(
      (key) =>
        `${humanizeParameterKey(key)} (${modeOf(key) === 'any' ? 'any value' : 'custom match'})`,
    ),
  ];

  const description = customized
    ? `Future calls may vary: ${changeSummaries.slice(0, 3).join(', ')}${
        changeSummaries.length > 3 ? `, +${changeSummaries.length - 3} more` : ''
      }`
    : keys.length === 0
      ? 'Future calls must match this tool.'
      : `Future calls must match this tool and all ${keys.length} values from this request.`;

  const toolControlId = `approval-${approval.id}-tool-mode`;
  const toolLabel = humanizeParameterKey(approval.tool_name);

  const content = (
    <div className="space-y-6">
      <p className="border-l-2 border-trust/20 pl-3 text-sm text-secondary">
        Future calls must match this tool and these values. Change only what should be allowed
        to differ.
      </p>

      <div className="space-y-3 border-b border-neutral-200 pb-5">
        <div className="grid gap-x-3 gap-y-2 sm:grid-cols-[minmax(0,1fr)_auto_minmax(12rem,0.8fr)] sm:items-center">
          <div className="min-w-0">
            <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
              <span className="text-xs font-semibold uppercase tracking-wide text-trust">Tool</span>
              <span className="text-sm font-medium text-neutral-900">{toolLabel}</span>
              {toolLabel !== approval.tool_name && (
                <code className="font-mono text-xs text-neutral-500 break-all">
                  {approval.tool_name}
                </code>
              )}
            </div>
          </div>
          <span className="text-sm text-secondary">matches</span>
          <div>
            <Select
              id={toolControlId}
              aria-label="Tool matching rule"
              size="sm"
              options={TOOL_MODE_OPTIONS}
              value={toolMode}
              disabled={disabled}
              className="[&>div:last-child]:hidden"
              onChange={(value) => {
                if (isToolMode(value)) selectToolMode(value);
              }}
            />
          </div>
        </div>

        {toolMode === 'custom' && (
          <TextInput
            id={`${toolControlId}-pattern`}
            size="sm"
            label="Custom match"
            value={toolPattern}
            disabled={disabled}
            onChange={(event) => {
              setToolCustom(true);
              onToolPatternChange(event.target.value);
            }}
            helperText="Use * for any characters. Example: create_* matches tool names that start with create_."
            errorMessage={issues.tool ?? undefined}
          />
        )}
      </div>

      {keys.length > 0 && (
        <div className="space-y-0">
          <p className="pb-2 text-xs font-semibold uppercase tracking-wide text-trust">Request details</p>
          {keys.map((key) => {
            const mode = modeOf(key);
            const label = humanizeParameterKey(key);
            const currentValue = unescapeGlobLiteral(exactParamPattern(approval, key));
            const modeControlId = `approval-${approval.id}-param-${key}-mode`;
            const issue = issues.params[key];
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
                    label="Custom match"
                    value={constraints[key] ?? ''}
                    disabled={disabled}
                    onChange={(event) => {
                      withCustomKey(key, true);
                      setParamPattern(key, event.target.value);
                    }}
                    helperText="Use * for any characters. Example: acme/* matches values that start with acme/."
                    errorMessage={issue}
                    successMessage={issue ? undefined : 'Includes the current value.'}
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
          <li>
            {toolMode === 'custom'
              ? `Tools matching ${toolPattern}`
              : `Only the tool ${approval.tool_name}`}
          </li>
          {keys.map((key) => {
            const mode = modeOf(key);
            const label = humanizeParameterKey(key);
            if (mode === 'any') return <li key={key}>{`${label}: any value`}</li>;
            if (mode === 'custom') {
              return <li key={key}>{`${label}: values matching ${constraints[key] ?? ''}`}</li>;
            }
            return (
              <li key={key}>
                {`${label}: exactly ${unescapeGlobLiteral(exactParamPattern(approval, key))}`}
              </li>
            );
          })}
        </ul>
      </div>

      <div className="space-y-2 border-t border-neutral-200 pt-5" aria-label="Approval pattern preview">
        <p className="text-xs font-semibold uppercase tracking-wide text-trust">Technical rule</p>
        <code className="font-mono text-xs text-neutral-500 break-words">
          {formatToolPattern(toolPattern, constraints)}
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
