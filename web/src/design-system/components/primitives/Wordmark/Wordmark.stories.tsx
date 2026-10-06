import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { ThemeProvider, useTheme } from '@design-system/theme/ThemeProvider';
import { Button } from '@design-system/components/primitives/Button';
import { Wordmark } from './Wordmark';

const meta = {
  title: 'Design System/Primitives/Wordmark', component: Wordmark,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
  args: { label: 'Agent Identity Broker' },
  decorators: [(Story) => <ThemeProvider><Story /></ThemeProvider>],
} satisfies Meta<typeof Wordmark>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Compact: Story = { args: { compact: true } };
export const ConsentSize: Story = { args: { className: 'h-5' } };
function ThemeExample() {
  const { setTheme, resolvedTheme } = useTheme();
  return <div className="space-y-4"><Wordmark label="Agent Identity Broker" />
    <Button variant="outline" onClick={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}>Switch appearance</Button></div>;
}
export const ThemeChange: Story = {
  render: () => <ThemeExample />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const mark = canvas.getByRole('img', { name: 'Agent Identity Broker' });
    const initial = mark.getAttribute('data-variant');
    await userEvent.click(canvas.getByRole('button', { name: 'Switch appearance' }));
    await expect(mark).toHaveAttribute('data-variant', initial === 'black' ? 'white' : 'black');
    await userEvent.click(canvas.getByRole('button', { name: 'Switch appearance' }));
    await expect(mark).toHaveAttribute('data-variant', initial);
  },
};
