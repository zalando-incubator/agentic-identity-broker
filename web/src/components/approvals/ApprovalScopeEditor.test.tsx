import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ApprovalScopeEditor } from './ApprovalScopeEditor';
import type { ApprovalPersistence, ToolApprovalDetail } from '../../types/approval';
import { approvalApi } from '@services/api/approvals';

beforeEach(() => {
  vi.spyOn(approvalApi, 'previewApprovalScope').mockResolvedValue({
    tool_pattern: 'create_pull_request',
    params_pattern: { repo: 'acme/app', title: 'Fix bug' },
    preview: 'create_pull_request(repo=acme/app,title=Fix bug)',
  });
});
afterEach(() => vi.restoreAllMocks());

const approval: ToolApprovalDetail = {
  id: 'approval-1',
  principal: 'user@example.com',
  agent_id: 'agent-1',
  agent_display_name: 'Research Assistant',
  tool_name: 'create_pull_request',
  arguments: { repo: 'acme/app', title: 'Fix bug' },
  tool_pattern: 'create_pull_request',
  params_pattern: { repo: 'acme/app', title: 'Fix bug' },
  pattern_preview: 'create_pull_request(repo=acme/app,title=Fix bug)',
  status: 'pending',
  approval_url: 'https://broker.example.com/approvals/approval-1',
  created_at: '2026-03-29T00:00:00Z',
  expires_at: '2026-03-29T00:10:00Z',
};

function ScopeHarness({ detail }: { detail: ToolApprovalDetail }) {
  const [paramsPattern, setParamsPattern] = useState(detail.params_pattern);
  const [scopeValid, setScopeValid] = useState(false);

  return (
    <>
      <ApprovalScopeEditor
        approval={detail}
        paramsPattern={paramsPattern}
        onParamsPatternChange={setParamsPattern}
        persistence="permanent"
        onScopeValidationChange={setScopeValid}
      />
      <button type="button" disabled={!scopeValid}>
        Approve
      </button>
    </>
  );
}

function PersistenceHarness({ detail }: { detail: ToolApprovalDetail }) {
  const [persistence, setPersistence] = useState<ApprovalPersistence>('session');
  const [paramsPattern, setParamsPattern] = useState(detail.params_pattern);

  const changePersistence = (value: ApprovalPersistence) => {
    setPersistence(value);
    setParamsPattern(detail.params_pattern);
  };

  return (
    <>
      <button type="button" onClick={() => changePersistence('permanent')}>
        Always allow
      </button>
      <ApprovalScopeEditor
        approval={detail}
        paramsPattern={paramsPattern}
        onParamsPatternChange={setParamsPattern}
        persistence={persistence}
      />
    </>
  );
}

describe('ApprovalScopeEditor', () => {
  it('stays collapsed with an exact-request summary', () => {
    render(
      <ApprovalScopeEditor
        approval={approval}
        paramsPattern={approval.params_pattern}
        onParamsPatternChange={vi.fn()}
        persistence="session"
      />,
    );

    expect(screen.getByText('Exact request')).toBeInTheDocument();
    expect(
      screen.getByText('Future calls must match this tool and all 2 values from this request.'),
    ).toBeInTheDocument();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
  });

  it('renders nothing for a one-time approval', () => {
    const { container } = render(
      <ApprovalScopeEditor
        approval={approval}
        paramsPattern={approval.params_pattern}
        onParamsPatternChange={vi.fn()}
        persistence="once"
      />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it('unconstrains a single parameter when Any value is chosen', async () => {
    const user = userEvent.setup();
    const onParamsPatternChange = vi.fn();

    render(
      <ApprovalScopeEditor
        approval={approval}
        paramsPattern={approval.params_pattern}
        onParamsPatternChange={onParamsPatternChange}
        persistence="permanent"
      />,
    );

    await user.click(screen.getByRole('button', { name: /approval scope/i }));
    const titleBlock = screen.getByTestId('approval-scope-param-title');
    expect(within(titleBlock).getByRole('combobox', { name: 'Title match mode' })).toHaveTextContent('This value');

    await user.click(within(titleBlock).getByRole('combobox', { name: 'Title match mode' }));
    await user.click(screen.getByRole('option', { name: 'Any value' }));

    expect(onParamsPatternChange).toHaveBeenCalledWith({ repo: 'acme/app' });
  });


  it('renders JSON display values for non-string argument values', async () => {
    const user = userEvent.setup();
    const detail: ToolApprovalDetail = {
      ...approval,
      arguments: { dry_run: true, attempts: 3, filters: { b: 2, a: 1 }, note: '' },
      params_pattern: {},
    };

    render(<ScopeHarness detail={detail} />);

    await user.click(screen.getByRole('button', { name: /approval scope/i }));

    expect(
      within(screen.getByTestId('approval-scope-param-dry_run')).getByText('true'),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId('approval-scope-param-attempts')).getByText('3'),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId('approval-scope-param-filters')).getByText('{"b":2,"a":1}'),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId('approval-scope-param-note')).getByText('(empty)'),
    ).toBeInTheDocument();
    expect(screen.getByText('Dry Run')).toBeInTheDocument();
  });

  it('drops customizations when persistence changes', async () => {
    const user = userEvent.setup();
    render(<PersistenceHarness detail={approval} />);

    await user.click(screen.getByRole('button', { name: /approval scope/i }));
    const repoBlock = screen.getByTestId('approval-scope-param-repo');
    await user.click(within(repoBlock).getByRole('combobox', { name: 'Repo match mode' }));
    await user.click(screen.getByRole('option', { name: 'Custom match' }));
    await user.clear(within(repoBlock).getByRole('textbox'));
    await user.type(within(repoBlock).getByRole('textbox'), 'acme/*');

    expect(screen.getByText('Custom rule')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Always allow' }));

    expect(screen.getByText('Exact request')).toBeInTheDocument();
    const restored = screen.getByTestId('approval-scope-param-repo');
    expect(within(restored).getByRole('combobox', { name: 'Repo match mode' })).toHaveTextContent('This value');
    expect(within(restored).queryByRole('textbox')).not.toBeInTheDocument();
  });
});

it('labels custom inputs with their distinct targets', async () => {
  const user = userEvent.setup();
  render(<ScopeHarness detail={approval} />);
  await user.click(screen.getByRole('button', { name: /approval scope/i }));

  expect(screen.getByText('Only the tool create_pull_request')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Tool matching rule' })).not.toBeInTheDocument();

  const repoBlock = screen.getByTestId('approval-scope-param-repo');
  await user.click(within(repoBlock).getByRole('combobox', { name: 'Repo match mode' }));
  await user.click(screen.getByRole('option', { name: 'Custom match' }));
  expect(within(repoBlock).getByRole('textbox', { name: 'Repo custom match' })).toBeInTheDocument();
});

it('aborts superseded preview and ignores its late acceptance after a changed scope is rejected', async () => {
  const first = Promise.withResolvers<{ tool_pattern: string; params_pattern: Record<string, string>; preview: string }>();
  vi.mocked(approvalApi.previewApprovalScope)
    .mockReturnValueOnce(first.promise)
    .mockRejectedValueOnce({ status: 422, message: 'Scope does not match' });
  const onValidation = vi.fn();
  const editorProps = { approval, persistence: 'session' as const, onParamsPatternChange: vi.fn(), onScopeValidationChange: onValidation };
  const { rerender } = render(<ApprovalScopeEditor {...editorProps} paramsPattern={approval.params_pattern} />);
  await waitFor(() => expect(approvalApi.previewApprovalScope).toHaveBeenCalledTimes(1));
  const signal = vi.mocked(approvalApi.previewApprovalScope).mock.calls[0][2]?.signal;
  rerender(<ApprovalScopeEditor {...editorProps} paramsPattern={{ repo: 'other/*' }} />);
  await waitFor(() => expect(approvalApi.previewApprovalScope).toHaveBeenCalledTimes(2));
  expect(signal?.aborted).toBe(true);
  await act(async () => { first.resolve({ tool_pattern: 'create_pull_request', params_pattern: approval.params_pattern, preview: 'stale accepted scope' }); });
  expect(onValidation).toHaveBeenLastCalledWith(false);
  expect(screen.queryByText('stale accepted scope')).not.toBeInTheDocument();
});

it('preserves a literal __proto__ parameter when its constraint is restored', async () => {
  const user = userEvent.setup();
  const detail: ToolApprovalDetail = {
    ...approval,
    arguments: JSON.parse('{"__proto__":"sensitive"}') as Record<string, unknown>,
    params_pattern: JSON.parse('{"__proto__":"sensitive"}') as Record<string, string>,
  };
  const change = vi.fn();
  render(<ApprovalScopeEditor approval={detail} paramsPattern={{}} persistence="session" onParamsPatternChange={change} />);
  await user.click(screen.getByRole('button', { name: /approval scope/i }));
  await user.click(screen.getByRole('combobox'));
  await user.click(screen.getByRole('option', { name: 'This value' }));
  const pattern = change.mock.calls[0][0] as Record<string, string>;
  expect(Object.hasOwn(pattern, '__proto__')).toBe(true);
  expect(pattern['__proto__']).toBe('sensitive');
  expect(Object.getPrototypeOf(pattern)).toBe(Object.prototype);
});
