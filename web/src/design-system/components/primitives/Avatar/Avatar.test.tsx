import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Avatar } from './Avatar';

afterEach(() => vi.restoreAllMocks());

describe('Avatar', () => {
  it.each(['https://remote.example/avatar.png', '//remote.example/avatar.png', 'data:image/svg+xml,<svg/>', 'javascript:alert(1)', 'https://user:password@remote.example/avatar.png'])('never assigns an unsafe source %s and names its fallback', (src) => {
    const preloadSource = vi.spyOn(HTMLImageElement.prototype, 'src', 'set');
    const { container } = render(<Avatar src={src} label="Inventory agent" fallback="IA" />);
    expect(screen.getByRole('img', { name: 'Inventory agent' })).toHaveTextContent('IA');
    expect(container.querySelector('img[src]')).toBeNull();
    expect(preloadSource).not.toHaveBeenCalled();
  });

  it('falls back on a local load failure and retries a changed local source', () => {
    const { container, rerender } = render(<Avatar src="/first.png" label="Inventory agent" fallback="IA" />);
    const first = container.querySelector('img');
    expect(first).toHaveAttribute('src', `${window.location.origin}/first.png`);
    fireEvent.error(first!);
    expect(screen.getByRole('img', { name: 'Inventory agent' })).toHaveTextContent('IA');
    expect(container.querySelector('img')).toBeNull();
    rerender(<Avatar src="/second.png" label="Inventory agent" fallback="IA" />);
    expect(container.querySelector('img')).toHaveAttribute('src', `${window.location.origin}/second.png`);
    rerender(<Avatar src="https://remote.example/avatar.png" label="Inventory agent" fallback="IA" />);
    expect(container.querySelector('img')).toBeNull();
  });
});
