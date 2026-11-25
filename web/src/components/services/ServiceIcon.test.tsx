import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { ServiceIcon } from './ServiceIcon';

const source = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><text fill="url(#paint)">Å &amp; 雲 #?%</text></svg>';
const replacement = '<svg xmlns="http://www.w3.org/2000/svg"><circle r="8"/></svg>';

function image(container: HTMLElement) {
  const element = container.querySelector('img');
  expect(element).not.toBeNull();
  return element as HTMLImageElement;
}

describe('ServiceIcon', () => {
  it('URI-encodes exact source only as a decorative image and retains fallback until load', () => {
    const { container } = render(
      <ServiceIcon iconSvg={source} fallback={<span>A</span>} />,
    );
    const icon = image(container);
    expect(icon).toHaveAttribute('src', `data:image/svg+xml;charset=utf-8,${encodeURIComponent(source)}`);
    expect(decodeURIComponent(icon.src.split(',')[1])).toBe(source);
    expect(icon).toHaveAttribute('alt', '');
    expect(icon).not.toHaveAttribute('tabindex');
    expect(icon).not.toBeVisible();
    expect(screen.getByText('A')).toBeVisible();
    expect(container.querySelector('svg, object, embed, iframe')).toBeNull();
    fireEvent.load(icon);
    expect(icon).toBeVisible();
    expect(screen.queryByText('A')).toBeNull();
    expect(screen.queryByRole('img')).toBeNull();
  });

  it.each([undefined, null])('uses the original fallback for absent source %s', (iconSvg) => {
    const { container } = render(<ServiceIcon iconSvg={iconSvg} fallback={<span>A</span>} />);
    expect(screen.getByText('A')).toBeVisible();
    expect(container.querySelector('img')).toBeNull();
  });

  it('keeps successful and failed unchanged sources stable, retries a changed source and clears it without remounting controls', () => {
    const onAction = vi.fn();
    const view = (iconSvg?: string | null) => (
      <div>
        <ServiceIcon iconSvg={iconSvg} fallback={<span>A</span>} />
        <button onClick={onAction}>Connect</button>
      </div>
    );
    const { container, rerender } = render(view(source));
    const action = screen.getByRole('button', { name: 'Connect' });
    const first = image(container);
    fireEvent.load(first);
    rerender(view(source));
    expect(image(container)).toBe(first);
    expect(first).toBeVisible();
    fireEvent.error(first);
    expect(container.querySelector('img')).toBeNull();
    expect(screen.getByText('A')).toBeVisible();
    rerender(view(source));
    expect(container.querySelector('img')).toBeNull();
    rerender(view(replacement));
    const next = image(container);
    expect(next).not.toBe(first);
    expect(next).not.toBeVisible();
    expect(screen.getByText('A')).toBeVisible();
    fireEvent.load(next);
    expect(next).toBeVisible();
    expect(screen.queryByText('A')).toBeNull();
    rerender(view(null));
    expect(container.querySelector('img')).toBeNull();
    expect(screen.getByText('A')).toBeVisible();
    rerender(view(undefined));
    expect(screen.getByRole('button', { name: 'Connect' })).toBe(action);
    fireEvent.click(action);
    expect(onAction).toHaveBeenCalledOnce();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('retains name-only callers without manufacturing an avatar on absence or error', () => {
    const { container, rerender } = render(<ServiceIcon />);
    expect(container).toBeEmptyDOMElement();
    rerender(<ServiceIcon iconSvg={source} />);
    fireEvent.error(image(container));
    expect(container).toBeEmptyDOMElement();
  });
});
