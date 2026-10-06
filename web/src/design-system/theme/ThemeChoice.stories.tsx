import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button/Button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@design-system/components/overlays/DropdownMenu/DropdownMenu';
import { ThemeChoice } from './ThemeChoice';

const labels = { light: 'Light', dark: 'Dark', system: 'System' };
const meta = {
  title: 'Design System/Theme/ThemeChoice',
  component: ThemeChoice,
  args: { labels, 'aria-label': 'Appearance' },
  parameters: { a11y: { test: 'error' } },
} satisfies Meta<typeof ThemeChoice>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Hover: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole('radio', { name: labels.system }));
  },
};

export const FocusVisible: Story = {
  play: async ({ canvasElement, globals }) => {
    const projectTheme = globals.theme === 'dark' ? 'dark' : 'light';
    const radio = within(canvasElement).getByRole('radio', { name: labels[projectTheme] });
    await userEvent.tab();
    await expect(radio).toHaveFocus();
  },
};

export const KeyboardSelection: Story = {
  play: async ({ canvasElement, globals }) => {
    const canvas = within(canvasElement);
    const root = canvasElement.ownerDocument.documentElement;
    const projectTheme = globals.theme === 'dark' ? 'dark' : 'light';
    await userEvent.click(canvas.getByRole('radio', { name: labels.light }));
    try {
      await userEvent.keyboard('{ArrowDown>}');
      await waitFor(async () => {
        await expect(canvas.getByRole('radio', { name: labels.dark })).toHaveFocus();
        await expect(canvas.getByRole('radio', { name: labels.dark })).toBeChecked();
      });
    } finally {
      await userEvent.keyboard('{/ArrowDown}');
    }
    await expect(canvas.getByRole('radio', { name: labels.dark })).toBeChecked();
    await expect(root).toHaveAttribute('data-theme', 'dark');
    await expect(root.style.colorScheme).toBe('dark');
    try {
      await userEvent.keyboard('{ArrowDown>}');
      await waitFor(async () => {
        await expect(canvas.getByRole('radio', { name: labels.system })).toHaveFocus();
        await expect(canvas.getByRole('radio', { name: labels.system })).toBeChecked();
      });
    } finally {
      await userEvent.keyboard('{/ArrowDown}');
    }
    await expect(canvas.getByRole('radio', { name: labels.system })).toBeChecked();
    await userEvent.click(canvas.getByRole('radio', { name: labels[projectTheme] }));
    await expect(root).toHaveAttribute('data-theme', projectTheme);
  },
};

export const MenuOpen: Story = {
  args: { presentation: 'menu' },
  render: (args) => (
    <DropdownMenu>
      <DropdownMenuTrigger asChild><Button variant="outline">Appearance menu</Button></DropdownMenuTrigger>
      <DropdownMenuContent><ThemeChoice {...args} /></DropdownMenuContent>
    </DropdownMenu>
  ),
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Appearance menu' }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('menu')).toBeVisible());
  },
};

export const MenuSelection: Story = {
  args: { presentation: 'menu' },
  render: (args) => (
    <DropdownMenu>
      <DropdownMenuTrigger asChild><Button variant="outline">Appearance menu</Button></DropdownMenuTrigger>
      <DropdownMenuContent><ThemeChoice {...args} /></DropdownMenuContent>
    </DropdownMenu>
  ),
  play: async ({ canvasElement, globals }) => {
    const canvas = within(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    const root = canvasElement.ownerDocument.documentElement;
    const projectTheme = globals.theme === 'dark' ? 'dark' : 'light';
    const alternateTheme = projectTheme === 'dark' ? 'light' : 'dark';
    const trigger = canvas.getByRole('button', { name: 'Appearance menu' });
    await userEvent.click(trigger);
    await waitFor(() => expect(body.getByRole('menu')).toBeVisible());
    await userEvent.click(body.getByRole('menuitemradio', { name: labels[alternateTheme] }));
    await expect(root).toHaveAttribute('data-theme', alternateTheme);
    await waitFor(() => expect(body.queryByRole('menu')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
    await userEvent.click(trigger);
    await waitFor(() => expect(body.getByRole('menu')).toBeVisible());
    await userEvent.click(body.getByRole('menuitemradio', { name: labels[projectTheme] }));
    await expect(root).toHaveAttribute('data-theme', projectTheme);
    await waitFor(() => expect(body.queryByRole('menu')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
    await userEvent.click(trigger);
    await waitFor(() => expect(body.getByRole('menu')).toBeVisible());
    await expect(body.getByRole('menuitemradio', { name: labels[projectTheme] })).toBeChecked();
  },
};
