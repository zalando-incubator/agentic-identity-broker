import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { GlobalErrorBoundary } from './GlobalErrorBoundary';

function Content({ broken }: { broken: boolean }) {
  if (broken) throw new globalThis.Error('Example application render failure');
  return <p>Application content is available.</p>;
}
function RecoveryExample() {
  const [broken, setBroken] = useState(false);
  return <div className="space-y-4"><Button variant="outline" onClick={() => setBroken(true)}>Simulate application failure</Button><GlobalErrorBoundary title="Application unavailable" description="Reload this view to try again." retryLabel="Reload view" onReset={() => setBroken(false)}><Content broken={broken} /></GlobalErrorBoundary></div>;
}
const meta = { title: 'Feedback/GlobalErrorBoundary', component: GlobalErrorBoundary, tags: ['autodocs'], parameters: { layout: 'fullscreen' }, args: { title: 'Application unavailable', description: 'Reload this view to try again.', retryLabel: 'Reload view', children: <Content broken={false} /> } } satisfies Meta<typeof GlobalErrorBoundary>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Dark: Story = { args: { children: <Content broken /> }, globals: { theme: 'dark' } };
export const Error: Story = { args: { children: <Content broken /> } };
export const Recovery: Story = {
  render: () => <RecoveryExample />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Simulate application failure' }));
    await expect(canvas.getByRole('alert')).toHaveTextContent('Reload this view to try again.');
    await userEvent.click(canvas.getByRole('button', { name: 'Reload view' }));
    await expect(canvas.getByText('Application content is available.')).toBeVisible();
  },
};
export const FocusVisible: Story = {
  args: Error.args,
  play: async ({ canvasElement }) => { await userEvent.tab(); await expect(within(canvasElement).getByRole('button', { name: 'Reload view' })).toHaveFocus(); },
};
export const Hover: Story = {
  args: Error.args,
  play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Reload view' })); },
};
