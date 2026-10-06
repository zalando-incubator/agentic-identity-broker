import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import type { CollectionView } from '@design-system/theme/collectionPreference';
import type { ThemePreference } from '@design-system/theme/themePreference';
import { commonCopy, settingsCopy } from '@copy';
import { ConsoleStoryShell } from '../storybook/ScreenShell';
import { SettingsView, type SettingsViewProps } from './SettingsView';

function InteractiveSettings(props: SettingsViewProps) {
  const [theme, setTheme] = useState<ThemePreference>(props.theme);
  const [defaultView, setDefaultView] = useState<CollectionView>(props.defaultView);
  return <SettingsView {...props} theme={theme} defaultView={defaultView}
    onThemeChange={next => { setTheme(next); props.onThemeChange(next); }}
    onDefaultViewChange={next => { setDefaultView(next); props.onDefaultViewChange(next); }} />;
}

const meta = {
  title: 'Screens/Settings/Appearance',
  component: SettingsView,
  parameters: { layout: 'fullscreen', a11y: { test: 'error' } },
  decorators: [(Story) => <ConsoleStoryShell path="/settings/appearance"><Story /></ConsoleStoryShell>],
  args: { theme: 'system', defaultView: 'grid', onThemeChange: fn(), onDefaultViewChange: fn() },
  render: (args) => <InteractiveSettings {...args} />,
} satisfies Meta<typeof SettingsView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const main = within(canvasElement).getByRole('main');
    await expect(within(main).getByRole('link', { name: commonCopy.returnToAgents })).toHaveAttribute('href', '/agents');
    await expect(within(main).getByRole('heading', { level: 2, name: 'Appearance' })).toBeVisible();
    await expect(within(main).queryByRole('navigation', { name: settingsCopy.categoriesLabel })).not.toBeInTheDocument();
    await expect(within(main).getByRole('radio', { name: 'System' })).toBeChecked();
    await expect(within(main).getByRole('button', { name: 'Grid view' })).toHaveAttribute('aria-pressed', 'true');
  },
};
export const LightSelected: Story = { args: { theme: 'light' } };
export const DarkSelected: Story = { args: { theme: 'dark' } };
export const ListSelected: Story = { args: { defaultView: 'list' } };
export const KeyboardChoices: Story = {
  play: async ({ canvasElement, args }) => {
    const main = within(canvasElement).getByRole('main');
    within(main).getByRole('radio', { name: 'System' }).focus();
    await userEvent.keyboard('{ArrowLeft>}');
    try {
      await waitFor(() => expect(within(main).getByRole('radio', { name: 'Dark' })).toBeChecked());
    } finally {
      await userEvent.keyboard('{/ArrowLeft}');
    }
    await expect(args.onThemeChange).toHaveBeenCalledWith('dark');
    const list = within(main).getByRole('button', { name: 'List view' });
    list.focus();
    await userEvent.keyboard('{Enter}');
    await expect(list).toHaveAttribute('aria-pressed', 'true');
    await expect(args.onDefaultViewChange).toHaveBeenCalledWith('list');
  },
};
