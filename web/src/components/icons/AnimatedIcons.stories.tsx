import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { AnimatedBotIcon } from './AnimatedIcons';

const meta = {
  title: 'Patterns/Animated icons',
  parameters: { a11y: { test: 'error' } },
  render: () => <Button variant="outline" data-animated-icon-host><AnimatedBotIcon />Agent navigation</Button>,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const BusyPointer: Story = {
  play: async ({ canvasElement }) => {
    const trigger = within(canvasElement).getByRole('button', { name: 'Agent navigation' });
    await userEvent.hover(trigger);
    const peak = await new Promise<number>(resolve => {
      let frame = 0;
      let maximum = 13;
      const sample = () => {
        maximum = Math.max(maximum, Number(trigger.querySelector('line')!.getAttribute('y1')));
        trigger.dispatchEvent(new PointerEvent('pointerleave'));
        trigger.dispatchEvent(new PointerEvent('pointerenter'));
        if (++frame < 24) requestAnimationFrame(sample);
        else resolve(maximum);
      };
      requestAnimationFrame(sample);
    });
    expect(peak).toBeGreaterThan(13.9);
  },
};
