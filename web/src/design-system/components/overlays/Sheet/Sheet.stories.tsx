import { useRef } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { Sheet, SheetClose, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from './Sheet';

function Navigation({ disabled = false }: { disabled?: boolean }) {
  const firstLink = useRef<HTMLAnchorElement>(null);
  return (
    <Sheet>
      <SheetTrigger asChild><Button variant="outline" disabled={disabled}>Open navigation</Button></SheetTrigger>
      <SheetContent side="left" closeLabel="Close navigation" onOpenAutoFocus={(event) => {
        event.preventDefault();
        firstLink.current?.focus();
      }}>
        <SheetHeader>
          <SheetTitle>Console navigation</SheetTitle>
          <SheetDescription>Manage access and connections.</SheetDescription>
        </SheetHeader>
        <nav aria-label="Main navigation" className="flex flex-col gap-2">
          {['Agents', 'Connections', 'Approvals'].map((label, index) => (
            <SheetClose asChild key={label}>
              <Button variant="ghost" asChild><a ref={index === 0 ? firstLink : undefined} href={`#${label.toLowerCase()}`}>{label}</a></Button>
            </SheetClose>
          ))}
        </nav>
      </SheetContent>
    </Sheet>
  );
}

const meta = {
  title: 'Design System/Overlays/Sheet',
  component: Sheet,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
  tags: ['autodocs'],
  render: () => <Navigation />,
} satisfies Meta<typeof Sheet>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const OverlayOpen: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Open navigation' }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('dialog', { name: 'Console navigation' })).toBeVisible());
  },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await userEvent.keyboard('{Enter}');
    const body = within(canvasElement.ownerDocument.body);
    await waitFor(() => expect(body.getByRole('link', { name: 'Agents' })).toHaveFocus());
    await userEvent.tab({ shift: true });
    await expect(body.getByRole('button', { name: 'Close navigation' })).toHaveFocus();
  },
};
export const FocusReturn: Story = {
  play: async ({ canvasElement }) => {
    const trigger = within(canvasElement).getByRole('button', { name: 'Open navigation' });
    await userEvent.click(trigger);
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Open navigation' }));
  },
};
export const Disabled: Story = {
  render: () => <Navigation disabled />,
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole('button', { name: 'Open navigation' })).toBeDisabled();
  },
};
