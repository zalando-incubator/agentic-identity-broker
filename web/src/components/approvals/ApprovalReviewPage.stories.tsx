import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import { approvalCopy } from '@copy/approvals';
import type { ScopePreview } from '../../types/approval';
import { ApprovalReviewPage } from './ApprovalReviewPage';
import { calendarRequest as approval, criticalRequest as criticalApproval, exactApprovalPreview, expiredRequest, longApproval } from '../../storybook/fixtures';
import { DecisionStoryShell } from '../../storybook/ScreenShell';

const meta = {
  title: 'Patterns/Approvals/Approval detail panel',
  component: ApprovalReviewPage,
  parameters: { layout: 'fullscreen', a11y: { test: 'error' } },
  decorators: [(Story) => <DecisionStoryShell path={`/approvals/${approval.id}`}><Story /></DecisionStoryShell>],
  args: {
    approval,
    actingPrincipal: approval.principal,
    submitting: false,
    submittingAction: null,
    errorCode: null,
    errorMessage: null,
    approveResult: null,
    denyResult: null,
    // Decision spies record intent only; no success state appears without an outcome response.
    onApprove: fn(async () => {}),
    onDeny: fn(async () => {}),
    onPreview: fn(exactApprovalPreview(approval)),
    onRetry: fn(),
  },
} satisfies Meta<typeof ApprovalReviewPage>;
export default meta;
type Story = StoryObj<typeof meta>;

const technicalApproval = {
  ...approval,
  id: 'approval-technical-inputs',
  arguments: {
    calendar: 'team-ops', title: 'Release safety review', start: '2026-10-06T14:00:00Z',
    attendees: ['alex@example.com', 'sam@example.com'],
    notes: { unsafe: '<script>alert("unsafe")</script>', lines: 'one\ntwo' }, dry_run: false,
  },
  agent_session_id: 'agent-session-technical', mcp_session_id: 'mcp-session-technical', tool_invocation_id: 'invocation-technical',
};

async function assertTechnicalDetails(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  const panel = canvas.getByTestId('approval-review-panel');
  const argumentsSection = canvas.getByRole('region', { name: 'Arguments' });
  const argumentsHeading = within(argumentsSection).getByRole('heading', { name: 'Arguments' });
  const json = within(argumentsSection).getByRole('button', { name: 'View JSON' });
  const session = canvas.getByRole('button', { name: 'Session context' });
  const scope = canvas.getByTestId('approval-scope-preview').parentElement!;
  const footer = panel.querySelector('footer')!;
  const body = footer.previousElementSibling as HTMLElement;
  const deny = canvas.getByRole('button', { name: 'Deny', exact: true });

  await expect(panel).toHaveAccessibleName(approvalCopy.title);
  await expect(canvas.queryByText('Selected request')).not.toBeInTheDocument();
  await expect(argumentsHeading).toBeVisible();
  await expect(parseFloat(getComputedStyle(json).fontSize)).toBeLessThan(parseFloat(getComputedStyle(deny).fontSize));
  await expect(canvas.getByText('Release safety review')).toBeVisible();
  await expect(canvas.getByText('notes')).toBeVisible();
  await expect(canvas.getByTestId('approval-scope-preview')).toHaveTextContent(technicalApproval.pattern_preview);
  await expect(panel.querySelector('script')).toBeNull();

  const sectionBounds = argumentsSection.getBoundingClientRect();
  const triggerBounds = json.getBoundingClientRect();
  await expect(triggerBounds.top).toBeGreaterThanOrEqual(sectionBounds.top);
  await expect(triggerBounds.bottom).toBeLessThanOrEqual(sectionBounds.bottom);
  await expect(triggerBounds.right).toBeLessThanOrEqual(sectionBounds.right);
  await expect(triggerBounds.bottom).toBeLessThanOrEqual(argumentsSection.querySelector('dl')!.getBoundingClientRect().top);
  await expect(scope.getBoundingClientRect().top - sectionBounds.bottom).toBeGreaterThanOrEqual(12);
  await expect(body.getBoundingClientRect().bottom).toBeLessThanOrEqual(footer.getBoundingClientRect().top);
  await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
  const panelBounds = panel.getBoundingClientRect();
  for (const name of ['Deny', 'Approve once']) {
    const action = canvas.getByRole('button', { name, exact: true }).getBoundingClientRect();
    await expect(action.left).toBeGreaterThanOrEqual(panelBounds.left);
    await expect(action.right).toBeLessThanOrEqual(panelBounds.right);
    await expect(action.top).toBeGreaterThanOrEqual(footer.getBoundingClientRect().top);
    await expect(action.bottom).toBeLessThanOrEqual(footer.getBoundingClientRect().bottom);
    await expect(action.right).toBeLessThanOrEqual(window.innerWidth);
  }
  json.scrollIntoView({ block: 'center' });
  await expect(json.getBoundingClientRect().top).toBeGreaterThanOrEqual(body.getBoundingClientRect().top);
  await expect(json.getBoundingClientRect().bottom).toBeLessThanOrEqual(footer.getBoundingClientRect().top);

  session.focus();
  await userEvent.keyboard('{Enter}');
  const context = await within(canvasElement.ownerDocument.body).findByRole('dialog', { name: 'Session context' });
  await waitFor(() => expect(context).toBeVisible());
  for (const id of ['agent-session-technical', 'mcp-session-technical', 'invocation-technical']) {
    await expect(within(context).getByText(id)).toBeVisible();
  }
  await userEvent.keyboard('{Escape}');
  await waitFor(() => expect(canvas.getByRole('button', { name: 'Session context' })).toHaveFocus());

  json.focus();
  await userEvent.keyboard('{Enter}');
  const dialog = await within(canvasElement.ownerDocument.body).findByRole('dialog', { name: 'Raw tool arguments' });
  await expect(within(dialog).getByTestId('approval-arguments').textContent).toBe(JSON.stringify(technicalApproval.arguments, null, 2));
  await expect(panel.querySelector('script')).toBeNull();
  await expect(dialog.querySelector('script')).toBeNull();
  await userEvent.keyboard('{Escape}');
  await waitFor(() => expect(canvas.getByRole('button', { name: 'View JSON' })).toHaveFocus());
  canvas.getByRole('button', { name: 'View JSON' }).blur();
  body.scrollTop = 0;
  window.scrollTo(0, 0);
}

export const Default: Story = {};
export const TechnicalDetails: Story = {
  args: { approval: technicalApproval, onPreview: fn(exactApprovalPreview(technicalApproval)) },
  play: async ({ canvasElement }) => assertTechnicalDetails(canvasElement),
};
export const NarrowTechnicalDetails: Story = { ...TechnicalDetails, globals: { viewport: { value: 'narrow320', isRotated: false } } };
export const NarrowBusyTechnicalDetails: Story = {
  ...NarrowTechnicalDetails,
  args: { ...TechnicalDetails.args, submitting: true, submittingAction: 'approve' },
  play: async ({ canvasElement }) => {
    await assertTechnicalDetails(canvasElement);
    for (const name of ['Deny', 'Approve once']) {
      const action = within(canvasElement).getByRole('button', { name, exact: true });
      await expect(action).toBeVisible();
      await expect(action).toHaveTextContent(name);
      await expect(action).toBeDisabled();
    }
  },
};

export const CriticalRisk: Story = {
  args: { approval: criticalApproval, onPreview: fn(exactApprovalPreview(criticalApproval)) },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByText(/critical risk/i)).toBeVisible();
  },
};
export const HighRisk: Story = {
  args: { approval: { ...criticalApproval, id: 'approval-high-risk', risk_level: 'high' }, onPreview: fn(exactApprovalPreview(criticalApproval)) },
};
export const LowRisk: Story = { args: { approval: { ...approval, id: 'approval-low-risk', risk_level: 'low' } } };
export const UnratedRisk: Story = { args: { approval: { ...approval, id: 'approval-unrated', risk_level: undefined } } };
export const ExpiredRequest: Story = {
  args: { approval: expiredRequest },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole('alert')).toBeVisible();
    await expect(canvas.queryByRole('button', { name: /approve once/i })).not.toBeInTheDocument();
  },
};
export const ApprovedOutcome: Story = {
  args: {
    approval: { ...approval, status: 'approved', persistence: 'once', created_at: '2026-10-04T10:00:00Z', expires_at: '2026-10-04T10:15:00Z', approved_at: '2026-10-04T10:04:00Z' },
    approveResult: { id: approval.id, status: 'approved', persistence: 'once', approved_at: '2026-10-04T10:04:00Z' },
  },
};
export const DeniedOutcome: Story = {
  args: {
    approval: { ...approval, status: 'denied', persistence: 'permanent', created_at: '2026-10-04T10:00:00Z', expires_at: '2026-10-04T10:15:00Z', denied_at: '2026-10-04T10:04:00Z' },
    denyResult: { id: approval.id, status: 'denied', persistence: 'permanent', denied_at: '2026-10-04T10:04:00Z' },
  },
};
export const LongRealisticContent: Story = {
  args: { approval: longApproval, onPreview: fn(exactApprovalPreview(longApproval)) },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const panel = canvas.getByTestId('approval-review-panel');
    await expect(canvas.getByTestId('approval-tool-name')).toHaveTextContent(longApproval.tool_name);
    const panelBounds = panel.getBoundingClientRect();
    if (window.innerWidth === 1280) {
      expect(window.innerHeight).toBe(720);
      for (const name of ['Deny', 'Approve once']) {
        const action = canvas.getByRole('button', { name, exact: true }).getBoundingClientRect();
        await expect(action.left).toBeGreaterThanOrEqual(0);
        await expect(action.right).toBeLessThanOrEqual(window.innerWidth);
        await expect(action.top).toBeGreaterThanOrEqual(0);
        await expect(action.bottom).toBeLessThanOrEqual(window.innerHeight);
        await expect(action.left).toBeGreaterThanOrEqual(panelBounds.left);
        await expect(action.right).toBeLessThanOrEqual(panelBounds.right);
        await expect(action.top).toBeGreaterThanOrEqual(panelBounds.top);
        await expect(action.bottom).toBeLessThanOrEqual(panelBounds.bottom);
      }
    }
    const footer = panel.querySelector('footer')!;
    const footerBefore = footer.getBoundingClientRect();
    const body = footer.previousElementSibling as HTMLElement;
    // A tall viewport can fit this fixture; short viewports must still scroll the body, not the footer.
    const overflows = body.scrollHeight > body.clientHeight;
    if (window.innerWidth === 1280) await expect(body.scrollHeight).toBeGreaterThan(body.clientHeight);
    body.scrollTop = body.scrollHeight;
    if (overflows) await expect(body.scrollTop).toBeGreaterThan(0);
    else await expect(body.scrollTop).toBe(0);
    const footerAfter = footer.getBoundingClientRect();
    await expect([footerAfter.x, footerAfter.y, footerAfter.width, footerAfter.height]).toEqual([
      footerBefore.x, footerBefore.y, footerBefore.width, footerBefore.height,
    ]);
    await expect(footerAfter.top).toBeGreaterThanOrEqual(panel.getBoundingClientRect().top);
    await expect(footerAfter.bottom).toBeLessThanOrEqual(panel.getBoundingClientRect().bottom);

    // Leave the real raw-arguments dialog open for the snapshot after checking the pinned footer.
    await userEvent.click(canvas.getByRole('button', { name: 'View JSON' }));
    const dialog = await within(canvasElement.ownerDocument.body).findByRole('dialog', { name: 'Raw tool arguments' });
    await expect(await within(dialog).findByTestId('approval-arguments')).toHaveTextContent('SEC-2068');
  },
};
export const PermanentScopeEditor: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Approve options' }));
    // The decision menu mounts after a lazy import; await its accessible item before selecting it.
    await userEvent.click(await within(canvasElement.ownerDocument.body).findByRole('menuitem', { name: 'Always…' }));
    const confirm = await canvas.findByRole('button', { name: 'Always approve' });
    await waitFor(() => expect(confirm).toBeEnabled());
    await expect(args.onPreview).toHaveBeenCalled();
    await expect(canvas.getByTestId('approval-scope-preview')).toHaveTextContent(approval.pattern_preview);
    await userEvent.click(confirm);
    await expect(args.onApprove).toHaveBeenCalledWith({ persistence: 'permanent', params_pattern: approval.params_pattern });
  },
};
export const SessionScopeEditor: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Approve options' }));
    await userEvent.click(await within(canvasElement.ownerDocument.body).findByRole('menuitem', { name: 'For this session' }));
    await waitFor(() => expect(canvas.getByRole('button', { name: 'Approve for this session' })).toBeEnabled());
    await expect(args.onPreview).toHaveBeenCalled();
    await expect(canvas.getByTestId('approval-scope-preview')).toHaveTextContent(approval.pattern_preview);
  },
};
export const InvalidScope: Story = {
  args: {
    errorCode: 'INVALID_PATTERN',
    onPreview: fn(async (): Promise<ScopePreview> => { throw new Error('The broker rejected this scope preview.'); }),
  },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Approve options' }));
    await userEvent.click(await within(canvasElement.ownerDocument.body).findByRole('menuitem', { name: 'For this session' }));
    await waitFor(() => expect(args.onPreview).toHaveBeenCalled());
    await waitFor(() => expect(canvas.getByRole('button', { name: 'Approve for this session' })).toBeDisabled());
    await waitFor(() => expect(canvas.getByText(/could not validate this scope/i)).toBeVisible());
  },
};
export const ApprovalFromKeyboard: Story = {
  play: async ({ canvasElement, args }) => {
    const approve = within(canvasElement).getByRole('button', { name: /approve once/i });
    approve.focus();
    await userEvent.keyboard('{Enter}');
    await expect(args.onApprove).toHaveBeenCalledWith({ persistence: 'once' });
  },
};
export const PermanentDenialConfirmation: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: /deny options/i }));
    await userEvent.click(await within(canvasElement.ownerDocument.body).findByRole('menuitem', { name: /always deny/i }));
    await userEvent.click(await canvas.findByRole('button', { name: 'Always deny', exact: true }));
    const dialog = await within(canvasElement.ownerDocument.body).findByRole('dialog');
    await expect(args.onDeny).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole('button', { name: /confirm permanent denial/i }));
    await waitFor(() => expect(args.onDeny).toHaveBeenCalledWith(true));
  },
};
