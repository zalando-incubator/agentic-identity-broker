import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ErrorBoundary } from './ErrorBoundary';
import { GlobalErrorBoundary } from '../GlobalErrorBoundary/GlobalErrorBoundary';

function Content({ broken }: { broken: boolean }) {
  if (broken) throw new Error('Private diagnostic');
  return <p>Recovered content</p>;
}

afterEach(() => vi.restoreAllMocks());

describe.each([
  ['local', ErrorBoundary],
  ['global', GlobalErrorBoundary],
] as const)('%s error boundary', (_name, Boundary) => {
  it('catches a descendant error and retries through the supplied recovery callback', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    let broken = true;
    const onReset = vi.fn(() => { broken = false; });
    const onError = vi.fn();
    const props = { title: 'Unable to display this view', description: 'Your changes were not submitted.', retryLabel: 'Reload view', onReset, onError };
    function Descendant() { return <Content broken={broken} />; }
    render(<Boundary {...props}><Descendant /></Boundary>);
    expect(screen.getByRole('alert')).toHaveTextContent(props.description);
    expect(screen.queryByText('Private diagnostic')).not.toBeInTheDocument();
    expect(onError).toHaveBeenCalledWith(expect.any(Error), expect.objectContaining({ componentStack: expect.any(String) }));
    await userEvent.click(screen.getByRole('button', { name: 'Reload view' }));
    expect(onReset).toHaveBeenCalledTimes(1);
    expect(screen.getByText('Recovered content')).toBeVisible();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('recovers on a changed reset key but not on an unrelated rerender', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const props = { title: 'View unavailable', retryLabel: 'Retry', resetKeys: ['first'] };
    const view = render(<Boundary {...props}><Content broken /></Boundary>);
    view.rerender(<Boundary {...props}><Content broken={false} /></Boundary>);
    expect(screen.queryByText('Recovered content')).not.toBeInTheDocument();
    view.rerender(<Boundary {...{ ...props, resetKeys: ['second'] }}><Content broken={false} /></Boundary>);
    expect(screen.getByText('Recovered content')).toBeVisible();
  });
});
