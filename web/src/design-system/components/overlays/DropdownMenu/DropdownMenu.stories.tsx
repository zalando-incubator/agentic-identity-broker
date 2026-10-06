import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from './DropdownMenu';

function UserMenu({ disabled = false }: { disabled?: boolean }) {
  const [theme, setTheme] = useState('system');
  const [notifications, setNotifications] = useState(true);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild><Button variant="outline" disabled={disabled}>User menu</Button></DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>Appearance</DropdownMenuLabel>
        <DropdownMenuRadioGroup value={theme} onValueChange={setTheme} aria-label="Appearance">
          <DropdownMenuRadioItem value="light">Light</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="dark">Dark</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="system">System</DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator />
        <DropdownMenuCheckboxItem checked={notifications} onCheckedChange={setNotifications}>Notifications</DropdownMenuCheckboxItem>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>More actions</DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <DropdownMenuItem>View settings</DropdownMenuItem>
            <DropdownMenuItem variant="destructive">Remove access</DropdownMenuItem>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuItem disabled>Export unavailable</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

const meta = {
  title: 'Design System/Overlays/DropdownMenu',
  component: DropdownMenu,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
  tags: ['autodocs'],
  render: () => <UserMenu />,
} satisfies Meta<typeof DropdownMenu>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const OverlayOpen: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'User menu' }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('menu')).toBeVisible());
    await expect(within(canvasElement.ownerDocument.body).getByRole('menuitemradio', { name: 'System' })).toBeChecked();
  },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('button', { name: 'User menu' })).toHaveFocus();
    await userEvent.keyboard('{ArrowDown}');
    const body = within(canvasElement.ownerDocument.body);
    await waitFor(() => expect(body.getByRole('menu')).toBeVisible());
    await waitFor(() => expect(body.getByRole('menuitemradio', { name: 'Light' })).toHaveFocus());
  },
};
export const RadioSelection: Story = {
  play: async ({ canvasElement }) => {
    const trigger = within(canvasElement).getByRole('button', { name: 'User menu' });
    await userEvent.click(trigger);
    const body = within(canvasElement.ownerDocument.body);
    await waitFor(() => expect(body.getByRole('menu')).toBeVisible());
    await userEvent.click(body.getByRole('menuitemradio', { name: 'Dark' }));
    await waitFor(() => expect(canvasElement.ownerDocument.querySelector('[data-slot="dropdown-menu-content"]')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
    await userEvent.click(trigger);
    await waitFor(() => expect(body.getByRole('menu')).toBeVisible());
    await expect(body.getByRole('menuitemradio', { name: 'Dark' })).toBeChecked();
  },
};
export const SubmenuOpen: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('button', { name: 'User menu' })).toHaveFocus();
    await userEvent.keyboard('{ArrowDown}');
    const body = within(canvasElement.ownerDocument.body);
    await waitFor(() => expect(body.getByRole('menu')).toBeVisible());
    await waitFor(() => expect(body.getByRole('menuitemradio', { name: 'Light' })).toHaveFocus());
    await userEvent.keyboard('{End}');
    await waitFor(() => expect(body.getByRole('menuitem', { name: 'More actions' })).toHaveFocus());
    await userEvent.keyboard('{ArrowRight}');
    await waitFor(() => expect(body.getByRole('menuitem', { name: 'View settings' })).toHaveFocus());
  },
};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'User menu' }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('menu')).toBeVisible());
    await userEvent.hover(within(canvasElement.ownerDocument.body).getByRole('menuitemradio', { name: 'Dark' }));
  },
};
export const Disabled: Story = {
  render: () => <UserMenu disabled />,
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole('button', { name: 'User menu' })).toBeDisabled();
  },
};
