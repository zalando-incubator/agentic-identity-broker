import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from './Card';

const meta = {
  title: 'Design System/Data Display/Card',
  component: Card,
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  render: (args) => <Card {...args} className="max-w-lg">
    <CardHeader>
      <CardTitle>Connection details</CardTitle>
      <CardDescription>Review the access you have granted.</CardDescription>
      <CardAction><Button variant="outline" size="sm">Manage</Button></CardAction>
    </CardHeader>
    <CardContent><p>The connection expires tomorrow.</p></CardContent>
    <CardFooter><Button variant="outline">View connection</Button></CardFooter>
  </Card>,
} satisfies Meta<typeof Card>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Manage' }));
  },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    canvas.getByRole('button', { name: 'Manage' }).focus();
    await userEvent.tab();
    await expect(canvas.getByRole('button', { name: 'View connection' })).toHaveFocus();
  },
};
export const Loading: Story = {
  render: () => <Card aria-busy="true" className="max-w-lg">
    <CardHeader><CardTitle>Connection details</CardTitle></CardHeader>
    <CardContent><p role="status">Loading connection details…</p></CardContent>
    <CardFooter><Button variant="outline" disabled>View connection</Button></CardFooter>
  </Card>,
};
export const Error: Story = {
  render: () => <Card className="max-w-lg">
    <CardHeader><CardTitle>Connection details</CardTitle></CardHeader>
    <CardContent><p role="alert" className="text-status-danger-foreground">Connection details could not be loaded.</p></CardContent>
    <CardFooter><Button variant="outline">Try again</Button></CardFooter>
  </Card>,
};
