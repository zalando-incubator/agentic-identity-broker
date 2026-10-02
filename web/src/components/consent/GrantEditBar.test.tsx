import { render, screen, fireEvent } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { GrantEditBar } from './GrantEditBar';

it('scrolls an obscured focused control into view and restores the document on unmount', () => {
  const previousPadding = document.documentElement.style.scrollPaddingBottom;
  const { unmount } = render(<form><button type="button">Edit permission</button><GrantEditBar pending={false} blocked={false} onCancel={() => {}} /></form>);
  const edit = screen.getByRole('button', { name: 'Edit permission' });
  const bar = screen.getByTestId('grant-save-bar');
  vi.spyOn(bar, 'getBoundingClientRect').mockReturnValue({ top: 500, bottom: 560, height: 60, left: 0, right: 320, width: 320, x: 0, y: 500, toJSON: () => ({}) });
  vi.spyOn(edit, 'getBoundingClientRect').mockReturnValue({ top: 490, bottom: 520, height: 30, left: 0, right: 80, width: 80, x: 0, y: 490, toJSON: () => ({}) });
  const scroll = vi.spyOn(edit, 'scrollIntoView');
  edit.focus();
  fireEvent(window, new Event('resize'));
  expect(scroll).toHaveBeenCalledWith({ block: 'center', behavior: 'instant' });
  expect(document.documentElement.style.scrollPaddingBottom).not.toBe(previousPadding);
  unmount();
  expect(document.documentElement.style.scrollPaddingBottom).toBe(previousPadding);
});
