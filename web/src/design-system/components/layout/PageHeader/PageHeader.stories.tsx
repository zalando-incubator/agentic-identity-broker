import type { Meta, StoryObj } from '@storybook/react';
import { Button } from '@design-system/components/primitives/Button/Button';
import { PageHeader } from './PageHeader';

const meta = {
  title: 'Design System/Layout/PageHeader', component: PageHeader, tags: ['autodocs'],
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  args: { title: 'Agents', purpose: 'Manage the access you grant to agents.' },
} satisfies Meta<typeof PageHeader>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const WithAction: Story = { args: { title: 'Agent access', purpose: 'Choose what this agent can do.', action: <Button>Save changes</Button> } };
export const Narrow: Story = { ...WithAction, globals: { viewport: { value: 'narrow320', isRotated: false } } };
