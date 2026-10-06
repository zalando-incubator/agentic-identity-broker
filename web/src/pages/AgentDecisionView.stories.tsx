import { useState } from 'react';
import type { FormEvent } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import { consentCopy } from '@copy/consent';
import { createConsentDraft } from '@components/consent/consentDraft';
import { consentDetail, consentDenseDetail, consentEmptyDetail, consentGrant, consentRiskDetail } from '../storybook/fixtures';
import { DecisionStoryShell } from '../storybook/ScreenShell';
import { AgentDecisionView, type AgentDecisionViewProps } from './AgentDecisionView';

const granted = createConsentDraft({ permissionSets: consentDetail.agent.permission_sets, serviceRequirements: consentDetail.agent.service_requirements, existingGrant: consentGrant, context: 'decision' });
const empty = createConsentDraft({ permissionSets: [], context: 'decision' });

function InteractiveDecision(args: AgentDecisionViewProps) {
  const [draft, setDraft] = useState(() => args.state === 'ready' ? args.draft : empty);
  const [outcome, setOutcome] = useState<'allowed' | 'denied'>();
  const [retrying, setRetrying] = useState(false);
  const [connected, setConnected] = useState<Set<string>>(() => new Set());
  if (outcome === 'denied') return <AgentDecisionView state="denied" user={args.user} />;
  if (retrying) return <AgentDecisionView state="loading" user={args.user} />;
  if (args.state === 'error') return <AgentDecisionView state="error" user={args.user} onRetry={() => { args.onRetry(); setRetrying(true); }} />;
  if (args.state !== 'ready') return <AgentDecisionView {...args} />;
  return <AgentDecisionView {...args} allowed={args.allowed || outcome === 'allowed'} draft={draft} missingServiceIds={args.missingServiceIds.filter((id) => !connected.has(id))}
    onChange={(next) => { setDraft(next); args.onChange(next); }}
    onAllow={(event) => { event.preventDefault(); args.onAllow(event); setOutcome('allowed'); }}
    onDeny={() => { args.onDeny(); setOutcome('denied'); }}
    onConnect={(id) => { args.onConnect(id); setConnected((before) => new Set(before).add(id)); }} />;
}

const ready = {
  state: 'ready', data: consentRiskDetail, user: { principal: 'alice', displayName: 'Alice' }, draft: granted, pending: false, missingServiceIds: [],
  onChange: fn(), onAllow: fn((event: FormEvent<HTMLFormElement>) => event.preventDefault()), onDeny: fn(), onConnect: fn(),
} satisfies Extract<AgentDecisionViewProps, { state: 'ready' }>;

const meta = {
  title: 'Screens/Consent decision', component: AgentDecisionView,
  tags: ['autodocs'], parameters: { layout: 'fullscreen', a11y: { test: 'error' } },
  decorators: [(Story) => <DecisionStoryShell compact path="/agents/research-agent?session_token=story"><Story /></DecisionStoryShell>],
  render: (args) => <InteractiveDecision {...args} />,
  args: ready,
} satisfies Meta<typeof AgentDecisionView>;
export default meta;
type Story = StoryObj<typeof AgentDecisionView>;

export const Typical: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByTestId('localhost-warning')).toBeVisible();
    const card = canvas.getByTestId('consent-card');
    const account = canvas.getByTestId('consent-account');
    await expect(card).not.toContainElement(account);
    await expect(account).toHaveTextContent(consentCopy.signedInAs('Alice'));
    await expect(canvas.getAllByText(consentCopy.signedInAs('Alice'))).toHaveLength(1);
    await expect(canvas.getByRole('img', { name: 'Agentic Identity Broker' })).toBeVisible();
    const identity = within(card).getByTestId('agent-identity');
    await expect(within(identity).getByRole('heading', { name: /wants to act on your behalf/ }).parentElement).toContainElement(within(identity).getByRole('img', { name: consentRiskDetail.agent.displayName }));
    await expect(canvas.getAllByTestId('permission-group')).toHaveLength(3);
    if (window.innerWidth === 1280) {
      expect(window.innerHeight).toBe(720);
      const action = canvas.getByRole('button', { name: 'Allow' }).getBoundingClientRect();
      expect(action.top).toBeGreaterThanOrEqual(0);
      expect(action.bottom).toBeLessThanOrEqual(window.innerHeight);
    }
  },
};
export const IdentityFitsViewport: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByTestId('consent-account')).toBeVisible();
    await expect(canvasElement.ownerDocument.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
    await expect(canvas.getByRole('button', { name: 'Allow' }).getBoundingClientRect().bottom).toBeLessThanOrEqual(window.innerHeight);
  },
};
export const VerifiedOrigin: Story = { args: { data: consentDetail, draft: createConsentDraft({ permissionSets: consentDetail.agent.permission_sets, serviceRequirements: consentDetail.agent.service_requirements, context: 'decision' }) } };
export const RegisteredOrigin: Story = {
  args: { data: { ...consentDetail, cimd_metadata: undefined } },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByTestId('agent-origin-label')).toBeVisible();
    await expect(canvas.queryByText(/administrator/i)).not.toBeInTheDocument();
    await expect(canvas.queryByText(/Returns to/)).not.toBeInTheDocument();
  },
};
export const AlreadyGranted: Story = { args: { data: consentDetail } };
export const MissingConnection: Story = {
  args: { missingServiceIds: ['drive'] },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole('button', { name: 'Connect Drive to continue' })).toBeVisible();
    await userEvent.click(canvas.getByRole('button', { name: 'Connect Drive to continue' }));
    await expect(canvas.getByRole('button', { name: 'Allow' })).toBeVisible();
  },
};
export const Empty: Story = { args: { data: consentEmptyDetail, draft: empty } };
const fourGroupDetail = { ...consentDetail, agent: { ...consentDetail.agent, permission_sets: consentDenseDetail.agent.permission_sets.slice(0, 4) } };
export const FourGroups: Story = {
  args: { data: fourGroupDetail, draft: createConsentDraft({ context: 'decision', permissionSets: fourGroupDetail.agent.permission_sets, serviceRequirements: fourGroupDetail.agent.service_requirements }) },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const groups = canvas.getAllByTestId('permission-group');
    const scrollable = groups[0]!.parentElement!.parentElement!;
    const overflow = scrollable.scrollHeight > scrollable.clientHeight;
    if (overflow) {
      await waitFor(() => expect(canvas.getByTestId('permission-overflow-hint')).toHaveTextContent(/\d+ more/));
      const footerBefore = canvas.getByTestId('consent-footer').getBoundingClientRect().bottom;
      scrollable.scrollTop = scrollable.scrollHeight;
      await waitFor(() => expect(canvas.getByTestId('permission-overflow-hint')).toHaveTextContent(/^$/));
      await expect(canvas.getByTestId('consent-footer').getBoundingClientRect().bottom).toBe(footerBefore);
    } else {
      await expect(canvas.queryByTestId('permission-overflow-hint')).not.toBeInTheDocument();
      await expect(groups[3]!.getBoundingClientRect().bottom).toBeLessThanOrEqual(scrollable.getBoundingClientRect().bottom);
    }
    await expect(canvas.getByRole('button', { name: 'Allow' }).getBoundingClientRect().bottom).toBeLessThanOrEqual(window.innerHeight);
  },
};
const longDescriptionDetail = { ...fourGroupDetail, agent: { ...fourGroupDetail.agent, permission_sets: fourGroupDetail.agent.permission_sets.map((entry, index) => index === 1 ? { ...entry, permission_set: { ...entry.permission_set, description: 'Read private documents and shared files, send their contents to the agent for analysis, and organize your work across every connected service. '.repeat(3) } } : entry) } };
export const ExpandPermissionDescription: Story = {
  args: { data: longDescriptionDetail, draft: createConsentDraft({ context: 'decision', permissionSets: longDescriptionDetail.agent.permission_sets, serviceRequirements: longDescriptionDetail.agent.service_requirements }) },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const row = within(canvas.getAllByTestId('permission-group')[1]!);
    const permission = row.getByRole('checkbox', { name: 'Access group 2' });
    await userEvent.click(row.getByTestId('permission-group-name'));
    await expect(permission).toBeChecked();
    const more = await row.findByRole('button', { name: 'More' });
    more.focus();
    await userEvent.keyboard('{Enter}');
    await expect(row.getByRole('button', { name: 'Less' })).toHaveAttribute('aria-expanded', 'true');
    await expect(permission).toBeChecked();
    const description = row.getByText(longDescriptionDetail.agent.permission_sets[1]!.permission_set.description.trim());
    await expect(description.scrollHeight).toBeLessThanOrEqual(description.clientHeight);
    await userEvent.click(description);
    await expect(permission).not.toBeChecked();
    await userEvent.click(row.getByRole('button', { name: 'Less' }));
    await expect(row.getByRole('button', { name: 'More' })).toHaveAttribute('aria-expanded', 'false');
  },
};
export const Dense: Story = {
  args: {
    data: consentDenseDetail,
    draft: createConsentDraft({
      permissionSets: consentDenseDetail.agent.permission_sets,
      serviceRequirements: consentDenseDetail.agent.service_requirements,
      existingGrant: consentGrant,
      context: 'decision',
    }),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const groups = canvas.getAllByTestId('permission-group');
    await expect(groups).toHaveLength(8);
    const scrollable = groups[0]!.parentElement!.parentElement!;
    const overflows = scrollable.scrollHeight > scrollable.clientHeight;
    if (window.innerWidth === 1280) await expect(overflows).toBe(true);
    if (!overflows) await expect(groups[7]!.getBoundingClientRect().bottom).toBeLessThanOrEqual(scrollable.getBoundingClientRect().bottom);

    const card = canvas.getByTestId('consent-card').getBoundingClientRect();
    const allow = canvas.getByRole('button', { name: 'Allow' });
    const before = allow.getBoundingClientRect();
    const footer = canvas.getByTestId('consent-footer');
    const footerBefore = footer.getBoundingClientRect();
    if (window.innerWidth === 1280 || window.innerWidth < 1012) {
      if (window.innerWidth === 1280) expect(window.innerHeight).toBe(720);
      await expect(before.left).toBeGreaterThanOrEqual(0);
      await expect(before.right).toBeLessThanOrEqual(window.innerWidth);
      await expect(before.top).toBeGreaterThanOrEqual(0);
      await expect(before.bottom).toBeLessThanOrEqual(window.innerHeight);
    }
    if (window.innerWidth === 1280) {
      await expect(before.left).toBeGreaterThanOrEqual(card.left);
      await expect(before.right).toBeLessThanOrEqual(card.right);
      await expect(before.top).toBeGreaterThanOrEqual(card.top);
      await expect(before.bottom).toBeLessThanOrEqual(card.bottom);
    }

    if (overflows) {
      scrollable.scrollTop = scrollable.scrollHeight;
      await expect(scrollable.scrollTop).toBeGreaterThan(0);
    }
    const footerAfter = footer.getBoundingClientRect();
    await expect([footerAfter.x, footerAfter.y, footerAfter.width, footerAfter.height]).toEqual([
      footerBefore.x, footerBefore.y, footerBefore.width, footerBefore.height,
    ]);
    const after = allow.getBoundingClientRect();
    await expect([after.x, after.y, after.width, after.height]).toEqual([before.x, before.y, before.width, before.height]);
  },
};
export const Loading: Story = { args: { state: 'loading' } };
export const Error: Story = { args: { state: 'error', onRetry: fn() } };
export const StaleAuthorization: Story = {
  args: { state: 'invalid', message: consentCopy.invalidSession },
  parameters: { docs: { description: { story: 'An expired authorization cannot display cached consent data or accept a decision.' } } },
};
export const Denied: Story = { args: { state: 'denied' } };
export const InvalidSession: Story = { args: { state: 'invalid' } };
export const Allowed: Story = { args: { allowed: true } };
export const ValidationError: Story = {
  args: { draft: granted.setDuration('custom', '2020-01-01'), dateError: consentCopy.invalidDate },
  play: async ({ canvasElement }) => {
    const dialog = await within(canvasElement.ownerDocument.body).findByRole('dialog', { name: 'Custom date' });
    const input = within(dialog).getByLabelText('Custom date', { selector: 'input' });
    await waitFor(() => expect(input).toHaveFocus());
    await expect(input).toHaveValue('2020-01-01');
    await waitFor(() => expect(within(dialog).getByText(consentCopy.invalidDate)).toBeVisible());
  },
};
export const TogglePermissionAndDuration: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const optional = canvas.getByRole('checkbox', { name: 'Access group 2' });
    await userEvent.click(optional);
    await expect(optional).toBeChecked();
    await expect(args.onChange).toHaveBeenCalled();
    await userEvent.click(canvas.getByRole('combobox', { name: 'Access lasts' }));
    await userEvent.click(await within(canvasElement.ownerDocument.body).findByRole('option', { name: '30 days' }));
    await expect(canvas.getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('30 days');
    await expect(args.onChange).toHaveBeenCalledWith(expect.objectContaining({ duration: '30-days' }));
  },
};
export const AllowFromKeyboard: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    canvas.getByRole('button', { name: 'Allow' }).focus();
    await userEvent.keyboard('{Enter}');
    await expect(args.onAllow).toHaveBeenCalledOnce();
    await expect(canvas.getByTestId('consent-outcome')).toHaveTextContent('Access allowed.');
  },
};
