import type { Meta, StoryObj } from '@storybook/react';
import { CircleAlert } from 'lucide-react';
import { Badge } from './Badge';

const meta = {
  title: 'Design System/Primitives/Badge', component: Badge,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
  args: { children: 'Risk not rated' },
} satisfies Meta<typeof Badge>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Variants: Story = {
  render: () => <div className="flex flex-wrap gap-3">
    <Badge>Risk not rated</Badge>
    <Badge variant="success">Connected</Badge>
    <Badge variant="warning">Needs sign-in</Badge>
    <Badge variant="danger">Unavailable</Badge>
    <Badge variant="info">Required</Badge>
  </div>,
};
export const Risk: Story = {
  render: () => <div className="flex flex-wrap gap-3">
    <Badge variant="risk-low">Low risk</Badge>
    <Badge variant="risk-medium">Medium risk</Badge>
    <Badge variant="risk-high">High risk</Badge>
    <Badge variant="risk-high" icon={<CircleAlert />}>Critical</Badge>
  </div>,
};
export const Error: Story = { args: { variant: 'danger', children: 'Expired' } };
