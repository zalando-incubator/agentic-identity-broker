import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow } from './Table';

function Connections({ disabled = false }: { disabled?: boolean }) {
  const [message, setMessage] = useState('');
  return <>
    <Table>
      <TableCaption>Connected agents</TableCaption>
      <TableHeader><TableRow><TableHead>Agent</TableHead><TableHead>Expiry</TableHead><TableHead>Actions</TableHead></TableRow></TableHeader>
      <TableBody>{['Research assistant with an exceptionally detailed display name', 'Planner'].map((name, index) => <TableRow key={name}>
        <TableHead scope="row" label="Agent">{name}</TableHead>
        <TableCell label="Expiry">September 30, 2026</TableCell>
        <TableCell label="Actions"><div className="flex flex-wrap gap-2">
          <Button variant="ghost" size="sm" disabled={disabled} aria-label={`View ${name}`} onClick={() => setMessage(`Viewing ${name}`)}>View</Button>
          <Button variant="outline" size="sm" disabled={disabled} aria-label={`Review revocation of ${name}`} onClick={() => setMessage(`Review revocation of agent ${index + 1}`)}>Revoke</Button>
        </div></TableCell>
      </TableRow>)}</TableBody>
    </Table>
    <p role="status" className="mt-2 text-sm text-muted-foreground">{message}</p>
  </>;
}
const meta = {
  title: 'Design System/Data Display/Table',
  component: Table,
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  render: () => <Connections />,
} satisfies Meta<typeof Table>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Hover: Story = {
  play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('button', { name: 'View Planner' })); },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    canvas.getByRole('button', { name: 'View Planner' }).focus();
    await userEvent.tab();
    await expect(canvas.getByRole('button', { name: 'Review revocation of Planner' })).toHaveFocus();
    await userEvent.keyboard('{Enter}');
    await expect(canvas.getByRole('status')).toHaveTextContent('Review revocation of agent 2');
  },
};
export const Disabled: Story = {
  render: () => <Connections disabled />,
  play: async ({ canvasElement }) => {
    for (const action of within(canvasElement).getAllByRole('button')) await expect(action).toBeDisabled();
  },
};
export const Loading: Story = {
  render: () => <Table aria-busy="true"><TableCaption>Connected agents</TableCaption><TableBody><TableRow><TableCell><p role="status">Loading connected agents…</p></TableCell></TableRow></TableBody></Table>,
};
export const Error: Story = {
  render: () => <Table><TableCaption>Connected agents</TableCaption><TableBody><TableRow><TableCell><p role="alert" className="text-destructive">Connected agents could not be loaded.</p></TableCell></TableRow></TableBody></Table>,
};
export const Empty: Story = {
  render: () => <Table><TableCaption>Connected agents</TableCaption><TableBody><TableRow><TableCell>No connected agents.</TableCell></TableRow></TableBody></Table>,
};
export const Narrow: Story = {
  globals: { viewport: { value: 'narrow320', isRotated: false } },
  parameters: { viewport: { options: { narrow320: { name: 'Narrow 320px', styles: { width: '320px', height: '800px' } } } } },
  play: async ({ canvasElement }) => {
    const document = canvasElement.ownerDocument;
    const viewport = document.defaultView!;
    await expect(viewport.innerWidth).toBe(320);
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(viewport.innerWidth);
    const actions = within(canvasElement).getAllByRole('button');
    await expect(actions).toHaveLength(4);
    for (const action of actions) {
      await expect(action).toBeVisible();
      const bounds = action.getBoundingClientRect();
      await expect(bounds.left).toBeGreaterThanOrEqual(0);
      await expect(bounds.right).toBeLessThanOrEqual(viewport.innerWidth);
      await userEvent.click(action);
    }
    await expect(within(canvasElement).getByRole('status')).toHaveTextContent('Review revocation of agent 2');
  },
};
