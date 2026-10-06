import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { Tabs, TabsList, TabsTrigger, TabsContent } from './Tabs';

function Example({ manual = false }: { manual?: boolean }) {
  return (
    <Tabs defaultValue="active" activationMode={manual ? 'manual' : 'automatic'}>
      <TabsList aria-label="Access status">
        <TabsTrigger value="active">Active</TabsTrigger>
        <TabsTrigger value="pending" disabled>Pending</TabsTrigger>
        <TabsTrigger value="expired">Expired</TabsTrigger>
      </TabsList>
      <TabsContent value="active">Current access</TabsContent>
      <TabsContent value="pending">Waiting for access</TabsContent>
      <TabsContent value="expired">Past access</TabsContent>
    </Tabs>
  );
}

describe('Tabs', () => {
  it('skips disabled tabs with arrows and links the selected tab to its panel', async () => {
    const user = userEvent.setup();
    render(<Example />);
    await user.tab();
    expect(screen.getByRole('tab', { name: 'Active' })).toHaveFocus();
    await user.keyboard('{ArrowRight}');
    const expired = screen.getByRole('tab', { name: 'Expired' });
    expect(expired).toHaveFocus();
    expect(expired).toHaveAttribute('aria-selected', 'true');
    const panel = screen.getByRole('tabpanel', { name: 'Expired' });
    expect(panel).toHaveTextContent('Past access');
    expect(expired).toHaveAttribute('aria-controls', panel.id);
    expect(screen.queryByText('Current access')).not.toBeInTheDocument();
    await user.keyboard('{Home}');
    expect(screen.getByRole('tab', { name: 'Active' })).toHaveAttribute('aria-selected', 'true');
  });

  it('keeps manual activation unchanged until Enter and never submits a surrounding form', async () => {
    const user = userEvent.setup();
    let submissions = 0;
    render(<form onSubmit={(event) => { event.preventDefault(); submissions += 1; }}><Example manual /></form>);
    await user.tab();
    await user.keyboard('{End}');
    expect(screen.getByRole('tab', { name: 'Expired' })).toHaveFocus();
    expect(screen.getByRole('tabpanel', { name: 'Active' })).toBeVisible();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('tabpanel', { name: 'Expired' })).toBeVisible();
    expect(submissions).toBe(0);
  });
});
