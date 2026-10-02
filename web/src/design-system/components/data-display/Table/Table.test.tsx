import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow } from './Table';

 describe('Table', () => {
  it('keeps row headers, labeled values, and independent row actions available to keyboard users', async () => {
    const user = userEvent.setup();
    const revoked: string[] = [];
    render(<Table>
      <TableCaption>Connected agents</TableCaption>
      <TableHeader><TableRow><TableHead>Agent</TableHead><TableHead>Expiry</TableHead><TableHead>Actions</TableHead></TableRow></TableHeader>
      <TableBody>{['Planner', 'Reporter'].map((name) => <TableRow key={name}>
        <TableHead scope="row" label="Agent">{name}</TableHead>
        <TableCell label="Expiry">Tomorrow</TableCell>
        <TableCell label="Actions"><a href={`#${name}`}>View {name}</a><button type="button" onClick={() => revoked.push(name)}>Revoke {name}</button></TableCell>
      </TableRow>)}</TableBody>
    </Table>);
    const table = screen.getByRole('table', { name: 'Connected agents' });
    const planner = within(table).getByRole('rowheader', { name: 'Planner' }).closest('tr')!;
    expect(within(planner).getByRole('cell', { name: 'Tomorrow' })).toBeVisible();
    await user.tab();
    expect(screen.getByRole('link', { name: 'View Planner' })).toHaveFocus();
    await user.tab();
    await user.keyboard('{Enter}');
    expect(revoked).toEqual(['Planner']);
    expect(screen.getByRole('button', { name: 'Revoke Reporter' })).toBeEnabled();
    await user.tab();
    expect(screen.getByRole('link', { name: 'View Reporter' })).toHaveFocus();
  });
});
