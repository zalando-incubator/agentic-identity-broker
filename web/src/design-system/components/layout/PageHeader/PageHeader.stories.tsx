import type { Meta, StoryObj } from '@storybook/react';
import { Button } from '@design-system/components/primitives/Button/Button';
import { PageHeader } from './PageHeader';

const meta = {
  title: 'Design System/Layout/PageHeader', component: PageHeader, tags: ['autodocs'],
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  args: { title: 'Agents' },
} satisfies Meta<typeof PageHeader>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const WithCount: Story = { args: { count: 4 } };
export const ZeroCount: Story = { args: { count: 0 } };
export const WithActions: Story = { args: { title: 'Agent access', purpose: 'Choose what this agent can do.', actions: <Button>Save changes</Button> } };
export const Narrow: Story = { ...WithActions, globals: { viewport: { value: 'narrow320', isRotated: false } } };
