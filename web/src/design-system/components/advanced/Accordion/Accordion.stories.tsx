import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from './Accordion';

const meta = {
  title: 'Advanced/Accordion',
  component: Accordion,
  tags: ['autodocs'],
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  args: { type: 'single', collapsible: true },
  render: (args) => (
    <Accordion {...args} className="w-full max-w-xl">
      <AccordionItem value="permissions">
        <AccordionTrigger>Permission details</AccordionTrigger>
        <AccordionContent>Read calendar events from your connected account.</AccordionContent>
      </AccordionItem>
      <AccordionItem value="expiry">
        <AccordionTrigger>Access duration</AccordionTrigger>
        <AccordionContent>You can revoke access at any time.</AccordionContent>
      </AccordionItem>
    </Accordion>
  ),
} satisfies Meta<typeof Accordion>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Expanded: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole('button', { name: 'Permission details' });
    await userEvent.click(trigger);
    await expect(trigger).toHaveAttribute('aria-expanded', 'true');
    await expect(canvas.getByRole('region', { name: 'Permission details' })).toBeVisible();
  },
};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Permission details' }));
  },
};
export const Focus: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.tab();
    await expect(canvas.getByRole('button', { name: 'Permission details' })).toHaveFocus();
    await userEvent.keyboard('{ArrowDown}');
    await expect(canvas.getByRole('button', { name: 'Access duration' })).toHaveFocus();
  },
};
export const Disabled: Story = {
  args: { disabled: true },
  play: async ({ canvasElement }) => {
    const trigger = within(canvasElement).getByRole('button', { name: 'Permission details' });
    await expect(trigger).toBeDisabled();
    await userEvent.click(trigger);
    await expect(trigger).toHaveAttribute('aria-expanded', 'false');
  },
};
export const Multiple: Story = {
  args: { type: 'multiple', collapsible: undefined },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Permission details' }));
    await userEvent.click(canvas.getByRole('button', { name: 'Access duration' }));
    await expect(canvas.getByRole('region', { name: 'Permission details' })).toBeVisible();
    await expect(canvas.getByRole('region', { name: 'Access duration' })).toBeVisible();
  },
};
export const Loading: Story = {
  render: () => (
    <Accordion type="single" collapsible defaultValue="permissions" className="w-full max-w-xl">
      <AccordionItem value="permissions">
        <AccordionTrigger>Permission details</AccordionTrigger>
        <AccordionContent aria-busy="true"><p role="status">Loading permission details…</p></AccordionContent>
      </AccordionItem>
    </Accordion>
  ),
};
export const Error: Story = {
  render: () => (
    <Accordion type="single" collapsible defaultValue="permissions" className="w-full max-w-xl">
      <AccordionItem value="permissions">
        <AccordionTrigger>Permission details</AccordionTrigger>
        <AccordionContent><p role="alert" className="text-destructive">Permission details could not be loaded.</p></AccordionContent>
      </AccordionItem>
    </Accordion>
  ),
};
