import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { PermissionSetCard } from './PermissionSetCard';

const source = '<svg xmlns="http://www.w3.org/2000/svg"><path d="M1 1h6v6z"/></svg>';
const replacement = '<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>';
const permissionSet = {
  id: 'permissions', name: 'Files access', description: 'Read project files',
  service_scopes: [
    { service_id: 'required', requirement_type: 'mandatory' as const },
    { service_id: 'optional', requirement_type: 'optional' as const },
    { service_id: 'filtered', requirement_type: 'optional' as const },
  ],
};
const serviceRequirements = [
  { service_id: 'required', requirement_type: 'mandatory' as const },
  { service_id: 'optional', requirement_type: 'optional' as const },
];

describe('PermissionSetCard service icons', () => {
  it('uses current metadata on locked and optional rows without changing filtering or switches', () => {
    const onServiceToggle = vi.fn();
    const view = (iconSvg?: string | null) => (
      <PermissionSetCard
        permissionSet={permissionSet}
        requirementType="optional"
        isSelected
        onServiceToggle={onServiceToggle}
        serviceRequirements={serviceRequirements}
        availableServices={[
          { id: 'required', display_name: 'Atlas', iconSvg },
          { id: 'optional', display_name: 'Birch', iconSvg },
          { id: 'filtered', display_name: 'Excluded', iconSvg },
        ]}
      />
    );
    const { container, rerender } = render(view(source));
    const optional = screen.getByRole('switch', { name: 'Exclude Birch' });
    const permission = screen.getByRole('switch', { name: 'Toggle Files access' });
    expect(screen.getByLabelText('Atlas (required)')).toBeInTheDocument();
    expect(screen.queryByText('Excluded')).toBeNull();
    expect(container.querySelectorAll('img')).toHaveLength(2);
    container.querySelectorAll('img').forEach((icon) => {
      expect(icon).toHaveAttribute('alt', '');
      fireEvent.error(icon);
    });
    expect(screen.getByText('A')).toBeVisible();
    expect(screen.getByText('B')).toBeVisible();
    rerender(view(replacement));
    container.querySelectorAll('img').forEach((icon) => {
      expect(icon).toHaveAttribute('src', `data:image/svg+xml;charset=utf-8,${encodeURIComponent(replacement)}`);
      fireEvent.load(icon);
    });
    expect(screen.queryByText('A')).toBeNull();
    expect(screen.queryByText('B')).toBeNull();
    expect(screen.getByRole('switch', { name: 'Exclude Birch' })).toBe(optional);
    expect(screen.getByRole('switch', { name: 'Toggle Files access' })).toBe(permission);
    rerender(view(null));
    expect(container.querySelector('img')).toBeNull();
    expect(screen.getByText('A')).toBeVisible();
    expect(screen.getByText('B')).toBeVisible();
    fireEvent.click(optional);
    expect(onServiceToggle).toHaveBeenCalledWith('permissions', 'optional', false);
  });
});
