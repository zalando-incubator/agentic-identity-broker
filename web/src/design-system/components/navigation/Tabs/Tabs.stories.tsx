import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Tabs, TabsContent, TabsList, TabsTrigger } from './Tabs';

const meta = {
  title: 'Design System/Navigation/Tabs',
  component: Tabs,
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  render: (args) => <Tabs defaultValue="active" {...args}>
    <TabsList aria-label="Access status">
      <TabsTrigger value="active">Active</TabsTrigger>
      <TabsTrigger value="expired">Expired</TabsTrigger>
      <TabsTrigger value="pending" disabled>Pending</TabsTrigger>
    </TabsList>
    <TabsContent value="active">Current connections appear here.</TabsContent>
    <TabsContent value="expired">Expired connections appear here.</TabsContent>
    <TabsContent value="pending">Pending connections appear here.</TabsContent>
  </Tabs>,
} satisfies Meta<typeof Tabs>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.hover(canvas.getByRole('tab', { name: 'Expired' }));
    await expect(canvas.getByRole('tab', { name: 'Active' })).toHaveAttribute('aria-selected', 'true');
  },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    canvas.getByRole('tab', { name: 'Active' }).focus();
    await userEvent.keyboard('{ArrowRight}');
    await expect(canvas.getByRole('tab', { name: 'Expired' })).toHaveFocus();
    await expect(canvas.getByRole('tabpanel', { name: 'Expired' })).toBeVisible();
  },
};
export const Disabled: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole('tab', { name: 'Pending' })).toBeDisabled();
    canvas.getByRole('tab', { name: 'Active' }).focus();
    await userEvent.keyboard('{End}');
    await expect(canvas.getByRole('tab', { name: 'Expired' })).toHaveFocus();
  },
};
export const ManualActivation: Story = {
  args: { activationMode: 'manual' },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    canvas.getByRole('tab', { name: 'Active' }).focus();
    await userEvent.keyboard('{ArrowRight}');
    await expect(canvas.getByRole('tabpanel', { name: 'Active' })).toBeVisible();
    await userEvent.keyboard('{Enter}');
    await expect(canvas.getByRole('tabpanel', { name: 'Expired' })).toBeVisible();
  },
};
export const Vertical: Story = { args: { orientation: 'vertical' } };
