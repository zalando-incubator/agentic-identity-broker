import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {
  ApprovalScopeEditor,
  hasScopeIssues,
  resolveScopeIssues,
} from './ApprovalScopeEditor';
import type { ApprovalPersistence, ToolApprovalDetail } from '../../types/approval';

const approval: ToolApprovalDetail = {
  id: 'approval-1',
  principal: 'user@example.com',
  agent_id: 'agent-1',
  agent_display_name: 'Research Assistant',
  tool_name: 'create_pull_request',
  arguments: { repo: 'acme/app', title: 'Fix bug' },
  tool_pattern: 'create_pull_request',
  params_pattern: { repo: 'acme/app', title: 'Fix bug' },
  status: 'pending',
  approval_url: 'https://broker.example.com/consent/approvals/approval-1',
  created_at: '2026-03-29T00:00:00Z',
  expires_at: '2026-03-29T00:10:00Z',
};

function ScopeHarness({ detail }: { detail: ToolApprovalDetail }) {
  const [toolPattern, setToolPattern] = useState(detail.tool_pattern);
  const [paramsPattern, setParamsPattern] = useState(detail.params_pattern);
  const issues = resolveScopeIssues(detail, toolPattern, paramsPattern);

  return (
    <>
      <ApprovalScopeEditor
        approval={detail}
        toolPattern={toolPattern}
        onToolPatternChange={setToolPattern}
        paramsPattern={paramsPattern}
        onParamsPatternChange={setParamsPattern}
        persistence="permanent"
      />
      <button type="button" disabled={hasScopeIssues(issues)}>
        Approve
      </button>
    </>
  );
}

function PersistenceHarness({ detail }: { detail: ToolApprovalDetail }) {
  const [persistence, setPersistence] = useState<ApprovalPersistence>('session');
  const [toolPattern, setToolPattern] = useState(detail.tool_pattern);
  const [paramsPattern, setParamsPattern] = useState(detail.params_pattern);

  const changePersistence = (value: ApprovalPersistence) => {
    setPersistence(value);
    setToolPattern(detail.tool_pattern);
    setParamsPattern(detail.params_pattern);
  };

  return (
    <>
      <button type="button" onClick={() => changePersistence('permanent')}>
        Always allow
      </button>
      <ApprovalScopeEditor
        approval={detail}
        toolPattern={toolPattern}
        onToolPatternChange={setToolPattern}
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
        toolPattern={approval.tool_pattern}
        onToolPatternChange={vi.fn()}
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
        toolPattern={approval.tool_pattern}
        onToolPatternChange={vi.fn()}
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
        toolPattern={approval.tool_pattern}
        onToolPatternChange={vi.fn()}
        paramsPattern={approval.params_pattern}
        onParamsPatternChange={onParamsPatternChange}
        persistence="permanent"
      />,
    );

    await user.click(screen.getByRole('button', { name: /approval scope/i }));
    const titleBlock = screen.getByTestId('approval-scope-param-title');
    expect(within(titleBlock).getByRole('button', { name: 'Title match mode' })).toHaveTextContent('This value');

    await user.click(within(titleBlock).getByRole('button', { name: 'Title match mode' }));
    await user.click(screen.getByRole('option', { name: 'Any value' }));

    expect(onParamsPatternChange).toHaveBeenCalledWith({ repo: 'acme/app' });
  });

  it('blocks approval while a custom pattern misses the current value', async () => {
    const user = userEvent.setup();
    render(<ScopeHarness detail={approval} />);

    await user.click(screen.getByRole('button', { name: /approval scope/i }));
    const repoBlock = screen.getByTestId('approval-scope-param-repo');
    await user.click(within(repoBlock).getByRole('button', { name: 'Repo match mode' }));
    await user.click(screen.getByRole('option', { name: 'Custom match' }));

    expect(within(repoBlock).getByRole('textbox')).toHaveValue('acme/app*');
    expect(screen.getByRole('button', { name: 'Approve' })).toBeEnabled();

    await user.clear(within(repoBlock).getByRole('textbox'));
    await user.type(within(repoBlock).getByRole('textbox'), 'other/*');

    expect(
      within(repoBlock).getByText('This pattern does not match the current value.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Approve' })).toBeDisabled();

    await user.clear(within(repoBlock).getByRole('textbox'));
    await user.type(within(repoBlock).getByRole('textbox'), 'acme/*');

    expect(within(repoBlock).getByText('Includes the current value.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Approve' })).toBeEnabled();
    expect(screen.getByText('create_pull_request(repo=acme/*,title=Fix bug)')).toBeInTheDocument();
  });

  it('renders canonical strings for non-string argument values', async () => {
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
      within(screen.getByTestId('approval-scope-param-filters')).getByText('{"a":1,"b":2}'),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId('approval-scope-param-note')).getByText('(empty)'),
    ).toBeInTheDocument();
    expect(screen.getByText('Dry run')).toBeInTheDocument();
  });

  it('drops customizations when persistence changes', async () => {
    const user = userEvent.setup();
    render(<PersistenceHarness detail={approval} />);

    await user.click(screen.getByRole('button', { name: /approval scope/i }));
    const repoBlock = screen.getByTestId('approval-scope-param-repo');
    await user.click(within(repoBlock).getByRole('button', { name: 'Repo match mode' }));
    await user.click(screen.getByRole('option', { name: 'Custom match' }));
    await user.clear(within(repoBlock).getByRole('textbox'));
    await user.type(within(repoBlock).getByRole('textbox'), 'acme/*');

    expect(screen.getByText('Custom rule')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Always allow' }));

    expect(screen.getByText('Exact request')).toBeInTheDocument();
    const restored = screen.getByTestId('approval-scope-param-repo');
    expect(within(restored).getByRole('button', { name: 'Repo match mode' })).toHaveTextContent('This value');
    expect(within(restored).queryByRole('textbox')).not.toBeInTheDocument();
  });
});
