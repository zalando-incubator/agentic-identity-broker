import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, within } from 'storybook/test';
import { consentCopy } from '@copy/consent';
import { createConsentDraft } from './consentDraft';
import { PermissionPanel, type PermissionPanelProps } from './PermissionPanel';
import { consentDetail, consentDenseDetail, consentGrant } from '../../storybook/fixtures';

const decisionDraft = createConsentDraft({ context: 'decision', permissionSets: consentDetail.agent.permission_sets, serviceRequirements: consentDetail.agent.service_requirements, existingGrant: consentGrant });
const consoleDraft = createConsentDraft({ context: 'console', permissionSets: consentDetail.agent.permission_sets, serviceRequirements: consentDetail.agent.service_requirements, existingGrant: consentGrant });

function InteractivePanel(args: PermissionPanelProps) {
  const [draft, setDraft] = useState(args.draft);
  const [connected, setConnected] = useState<Set<string>>(() => new Set());
  return <div className="w-full max-w-[432px]"><PermissionPanel {...args} draft={draft} missingServiceIds={args.missingServiceIds?.filter((id) => !connected.has(id))}
    onChange={(next) => { setDraft(next); args.onChange(next); }}
    onConnect={(id) => { args.onConnect?.(id); setConnected((before) => new Set(before).add(id)); }} /></div>;
}

const meta = {
  title: 'Patterns/Permission panel', component: PermissionPanel, tags: ['autodocs'],
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  render: (args) => <InteractivePanel {...args} />,
  args: { draft: decisionDraft, services: consentDetail.services, onChange: fn(), onConnect: fn() },
} satisfies Meta<typeof PermissionPanel>;
export default meta;
type Story = StoryObj<typeof PermissionPanel>;

export const Typical: Story = {};
export const Granted: Story = { args: { draft: decisionDraft } };
export const MissingConnection: Story = { args: { missingServiceIds: ['drive'] } };
export const ConsoleServices: Story = {
  args: { draft: consoleDraft, mode: 'console' },
  play: async ({ canvasElement }) => {
    const row = within(canvasElement).getAllByTestId('permission-group')[2]!;
    const drive = within(row).getByRole('button', { name: 'Drive' });
    await expect(drive).toBeVisible();
    await expect(drive).toHaveAttribute('aria-pressed', 'true');
    await expect(within(row).getAllByRole('checkbox')).toHaveLength(1);
    await expect(within(row).getByText('Mail')).toBeVisible();
    const bounds = row.getBoundingClientRect();
    await userEvent.click(drive);
    await expect(drive).toHaveAttribute('aria-pressed', 'false');
    await expect(row.getBoundingClientRect().height).toBe(bounds.height);
  },
};
export const ConsoleSingleService: Story = {
  args: { draft: consoleDraft, mode: 'console' },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const required = canvas.getAllByTestId('permission-group')[0]!;
    const optional = canvas.getAllByTestId('permission-group')[1]!;
    await expect(required).toHaveAttribute('data-read-only', 'true');
    await expect(within(required).queryByRole('checkbox')).not.toBeInTheDocument();
    await expect(within(required).getByText(consentCopy.required)).toBeVisible();
    const nameBounds = within(required).getByTestId('permission-group-name').getBoundingClientRect();
    const grantedBounds = within(required).getByTestId('permission-group-granted').getBoundingClientRect();
    expect(Math.abs(grantedBounds.top - nameBounds.top)).toBeLessThan(4);
    expect(grantedBounds.left).toBeGreaterThanOrEqual(nameBounds.right);
    await expect(within(required).getAllByText(consentCopy.granted)).toHaveLength(1);
    await expect(within(optional).getByText('Drive')).toBeVisible();
    await expect(within(optional).getAllByRole('checkbox')).toHaveLength(1);
    await userEvent.click(within(optional).getByTestId('permission-group-name'));
    await expect(within(optional).getByRole('checkbox', { name: 'Access group 2' })).toBeChecked();
    await expect(optional).toHaveAttribute('data-selected', 'true');
  },
};
export const Dense: Story = { args: { draft: createConsentDraft({ context: 'decision', permissionSets: consentDenseDetail.agent.permission_sets, serviceRequirements: consentDenseDetail.agent.service_requirements, existingGrant: consentGrant }) } };
export const Empty: Story = { args: { draft: createConsentDraft({ context: 'decision', permissionSets: [] }), services: [] } };
export const ValidationError: Story = { args: { error: consentCopy.selectPermission } };
export const OptionalSelection: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const panel = canvas.getByTestId('permission-groups');
    const height = panel.getBoundingClientRect().height;
    const optional = canvas.getByRole('checkbox', { name: 'Access group 2' });
    await userEvent.click(optional);
    await expect(optional).toBeChecked();
    await expect(panel.getBoundingClientRect().height).toBe(height);
  },
};
