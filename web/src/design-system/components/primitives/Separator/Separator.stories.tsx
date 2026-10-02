import type { Meta, StoryObj } from '@storybook/react';
import { Separator } from './Separator';

const meta = {
  title: 'Design System/Primitives/Separator', component: Separator,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
} satisfies Meta<typeof Separator>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {
  render: () => <div className="w-64 space-y-3 text-foreground"><p>Agent identity</p><Separator /><p>Permissions</p></div>,
};
export const Vertical: Story = {
  render: () => <div className="flex h-6 items-center gap-4 text-foreground"><span>Agents</span><Separator orientation="vertical" /><span>Connections</span></div>,
};
export const SemanticBoundary: Story = {
  render: () => <div className="w-64 space-y-3 text-foreground"><p>Required permissions</p><Separator decorative={false} /><p>Optional permissions</p></div>,
};
