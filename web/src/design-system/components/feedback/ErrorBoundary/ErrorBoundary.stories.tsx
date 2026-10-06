import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { ErrorBoundary } from './ErrorBoundary';

function Content({ broken }: { broken: boolean }) {
  if (broken) throw new globalThis.Error('Example descendant render failure');
  return <p>Connection details are available.</p>;
}
function RecoveryExample() {
  const [broken, setBroken] = useState(false);
  return <div className="space-y-4"><Button variant="outline" onClick={() => setBroken(true)}>Simulate render failure</Button><ErrorBoundary title="Details unavailable" description="Your permissions have not changed." retryLabel="Try details again" onReset={() => setBroken(false)}><Content broken={broken} /></ErrorBoundary></div>;
}
const meta = { title: 'Feedback/ErrorBoundary', component: ErrorBoundary, tags: ['autodocs'], args: { title: 'Details unavailable', description: 'Your permissions have not changed.', retryLabel: 'Try details again', children: <Content broken={false} /> } } satisfies Meta<typeof ErrorBoundary>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Dark: Story = { globals: { theme: 'dark' } };
export const Error: Story = { args: { children: <Content broken /> } };
export const Recovery: Story = {
  render: () => <RecoveryExample />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Simulate render failure' }));
    await expect(canvas.getByRole('alert')).toHaveTextContent('Your permissions have not changed.');
    await userEvent.click(canvas.getByRole('button', { name: 'Try details again' }));
    await expect(canvas.getByText('Connection details are available.')).toBeVisible();
  },
};
export const FocusVisible: Story = {
  args: Error.args,
  play: async ({ canvasElement }) => { await userEvent.tab(); await expect(within(canvasElement).getByRole('button', { name: 'Try details again' })).toHaveFocus(); },
};
export const Hover: Story = {
  args: Error.args,
  play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Try details again' })); },
};
