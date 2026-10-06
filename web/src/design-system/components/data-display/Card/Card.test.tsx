import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from './Card';

describe('Card', () => {
  it('groups content without capturing keyboard interaction from its explicit actions', async () => {
    const user = userEvent.setup();
    let cancelled = false;
    render(<Card role="region" aria-labelledby="connection-title" aria-describedby="connection-description">
      <CardHeader><CardTitle id="connection-title">Connection</CardTitle><CardDescription id="connection-description">Access details</CardDescription></CardHeader>
      <CardContent><a href="#details">View details</a></CardContent>
      <CardFooter><button type="button" onClick={() => { cancelled = true; }}>Cancel</button></CardFooter>
    </Card>);
    expect(screen.getByRole('region', { name: 'Connection' })).toHaveAccessibleDescription('Access details');
    expect(screen.getByRole('heading', { name: 'Connection' })).toBeVisible();
    await user.tab();
    expect(screen.getByRole('link', { name: 'View details' })).toHaveFocus();
    await user.tab();
    await user.keyboard('{Enter}');
    expect(cancelled).toBe(true);
  });
});
