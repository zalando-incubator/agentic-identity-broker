import { cleanup, render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it } from 'vitest';
import { SettingsLayout } from './SettingsLayout';

const appearance = { href: '/settings/appearance', label: 'Appearance' };
const secondCategory = { href: '/settings/other', label: 'Other' };

function renderLayout(categories: readonly (typeof appearance)[]) {
  return render(<MemoryRouter initialEntries={['/settings/appearance']}>
    <SettingsLayout categories={categories} currentHref={appearance.href} navigationLabel="Settings categories">
      <h2>Appearance options</h2>
    </SettingsLayout>
  </MemoryRouter>);
}

afterEach(cleanup);
describe('SettingsLayout', () => {
  it('does not render a category navigation for just Appearance', () => {
    renderLayout([appearance]);
    expect(screen.queryByRole('navigation', { name: 'Settings categories' })).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Appearance options' })).toBeVisible();
  });

  it('shows a linked category navigation when another category exists', () => {
    renderLayout([appearance, secondCategory]);
    const navigation = screen.getByRole('navigation', { name: 'Settings categories' });
    expect(within(navigation).getByRole('link', { name: 'Appearance' })).toHaveAttribute('aria-current', 'page');
    expect(within(navigation).getByRole('link', { name: 'Other' })).not.toHaveAttribute('aria-current');
    expect(within(navigation).getByRole('link', { name: 'Other' })).toHaveAttribute('href', secondCategory.href);
    expect(screen.getByRole('heading', { name: 'Appearance options' })).toBeVisible();
  });
});
