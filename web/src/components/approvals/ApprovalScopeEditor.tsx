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
import { Radio } from '@design-system/components/inputs/Radio';
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

const NOT_COVERING = 'This pattern does not match the current value.';
const VALUE_PREVIEW_LIMIT = 120;

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

  const toolMode: 'exact' | 'custom' =
    toolCustom || toolPattern !== exactTool ? 'custom' : 'exact';

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

  const toolGroupName = `approval-${approval.id}-tool`;
  const toolLabel = humanizeParameterKey(approval.tool_name);

  const content = (
    <div className="space-y-4">
      <p className="text-sm text-neutral-700">
        Future calls must match this tool and these values. Change only what should be allowed
        to differ.
      </p>

      <div className="space-y-2 rounded-lg border border-neutral-200 p-3">
        <div className="flex flex-wrap items-baseline gap-2">
          <span className="text-sm font-medium text-neutral-900">Tool</span>
          <span className="text-sm text-neutral-700">{toolLabel}</span>
          {toolLabel !== approval.tool_name && (
            <code className="font-mono text-xs text-neutral-500 break-all">
              {approval.tool_name}
            </code>
          )}
        </div>

        <fieldset className="space-y-2" disabled={disabled}>
          <legend className="sr-only">Allowed tools</legend>
          <Radio
            id={`${toolGroupName}-exact`}
            name={toolGroupName}
            label="Exactly this tool"
            checked={toolMode === 'exact'}
            disabled={disabled}
            onChange={() => {
              setToolCustom(false);
              onToolPatternChange(exactTool);
            }}
          />
          <Radio
            id={`${toolGroupName}-custom`}
            name={toolGroupName}
            label="Custom match"
            description="Allow tool names that match a pattern you define."
            checked={toolMode === 'custom'}
            disabled={disabled}
            onChange={() => {
              setToolCustom(true);
              onToolPatternChange(`${exactTool}*`);
            }}
          />
        </fieldset>

        {toolMode === 'custom' && (
          <TextInput
            id={`${toolGroupName}-pattern`}
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
        <div className="space-y-3">
          <p className="text-sm font-semibold text-neutral-900">Request details</p>
          {keys.map((key) => {
            const mode = modeOf(key);
            const label = humanizeParameterKey(key);
            const currentValue = unescapeGlobLiteral(exactParamPattern(approval, key));
            const groupName = `approval-${approval.id}-param-${key}`;
            const issue = issues.params[key];
            const truncatable = currentValue.length > VALUE_PREVIEW_LIMIT;
            const expanded = expandedValues.has(key);

            return (
              <div
                key={key}
                data-testid={`approval-scope-param-${key}`}
                className="space-y-2 rounded-lg border border-neutral-200 p-3"
              >
                <div className="flex flex-wrap items-baseline gap-2">
                  <span className="text-sm font-medium text-neutral-900">{label}</span>
                  {label !== key && (
                    <code className="font-mono text-xs text-neutral-500 break-all">{key}</code>
                  )}
                </div>

                <div className="space-y-1">
                  <p className="text-xs text-neutral-500">Current value</p>
                  {currentValue === '' ? (
                    <p className="text-xs italic text-neutral-500">(empty)</p>
                  ) : (
                    <p
                      className={
                        truncatable && expanded
                          ? 'font-mono text-xs text-neutral-800 whitespace-pre-wrap break-all max-h-40 overflow-auto'
                          : 'font-mono text-xs text-neutral-800 break-all'
                      }
                    >
                      {truncatable && !expanded
                        ? `${currentValue.slice(0, VALUE_PREVIEW_LIMIT)}…`
                        : currentValue}
                    </p>
                  )}
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
                </div>

                <fieldset className="space-y-2" disabled={disabled}>
                  <legend className="sr-only">{`Allowed values for ${label}`}</legend>
                  <Radio
                    id={`${groupName}-exact`}
                    name={groupName}
                    label="This value"
                    checked={mode === 'exact'}
                    disabled={disabled}
                    onChange={() => selectMode(key, 'exact')}
                  />
                  <Radio
                    id={`${groupName}-any`}
                    name={groupName}
                    label="Any value"
                    description="The agent may use a different value here."
                    checked={mode === 'any'}
                    disabled={disabled}
                    onChange={() => selectMode(key, 'any')}
                  />
                  <Radio
                    id={`${groupName}-custom`}
                    name={groupName}
                    label="Custom match"
                    description="Allow values that match a pattern you define."
                    checked={mode === 'custom'}
                    disabled={disabled}
                    onChange={() => selectMode(key, 'custom')}
                  />
                </fieldset>

                {mode === 'custom' && (
                  <TextInput
                    id={`${groupName}-pattern`}
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

      <div className="space-y-1">
        <p className="text-sm font-semibold text-neutral-900">Applies to</p>
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

      <div className="space-y-1" aria-label="Approval pattern preview">
        <p className="text-sm font-semibold text-neutral-900">Technical rule</p>
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
