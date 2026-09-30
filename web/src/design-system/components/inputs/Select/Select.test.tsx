import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Select, type SelectProps } from './Select';

const options = [
  { value: 'exact', label: 'This value' },
  { value: 'any', label: 'Any value' },
  { value: 'custom', label: 'Custom match', disabled: true },
];

function ControlledSelect() {
  const [value, setValue] = useState<SelectProps['value']>('exact');
  return (
    <Select
      aria-label="Parameter match mode"
      options={options}
      value={value}
      onChange={setValue}
    />
  );
}

describe('Select', () => {
  it('allows choosing an available approval mode with the keyboard', async () => {
    const user = userEvent.setup();
    render(<ControlledSelect />);

    await user.click(screen.getByRole('button', { name: 'Parameter match mode' }));
    await user.keyboard('{ArrowDown}{ArrowDown}{Enter}');

    expect(screen.getByRole('button', { name: 'Parameter match mode' })).toHaveTextContent('Any value');
  });

  it('prevents changes when approval editing is disabled', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Select
        aria-label="Parameter match mode"
        options={options}
        value="exact"
        onChange={onChange}
        disabled
      />,
    );

    const button = screen.getByRole('button', { name: 'Parameter match mode' });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(screen.queryByRole('option', { name: 'Any value' })).not.toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });
});
