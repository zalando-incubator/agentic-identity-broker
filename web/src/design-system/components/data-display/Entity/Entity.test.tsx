import type { ReactNode } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import { EntityCard, EntityRow } from './Entity';

function Location() {
  return <output data-testid="location">{useLocation().pathname}</output>;
}

function mount(element: ReactNode) {
  return render(<MemoryRouter initialEntries={['/agents']}><Location />{element}</MemoryRouter>);
}

afterEach(() => vi.restoreAllMocks());

describe.each([['card', EntityCard], ['row', EntityRow]] as const)('Entity %s', (_, Component) => {
  it('keeps the whole surface navigable without nesting or activating an action', async () => {
    const user = userEvent.setup();
    const onRevoke = vi.fn();
    mount(<Component id="entity-1" data-testid="entity" name="Agent One" href="/agents/entity-1" supporting="Until revoked" meta="Last updated 3 Oct 2026"
      status={<Badge variant="success">Active</Badge>} actions={<Button size="sm" variant="destructive-outline" onClick={onRevoke}>Revoke</Button>} />);
    const entity = screen.getByTestId('entity');
    expect(entity).toHaveAttribute('id', 'entity-1');
    const link = screen.getByRole('link', { name: /Agent One/ });
    expect(link).toHaveAttribute('href', '/agents/entity-1');
    expect(link).toHaveAccessibleDescription('Active Until revoked Last updated 3 Oct 2026');
    expect(link).not.toContainElement(screen.getByRole('button', { name: 'Revoke' }));
    await user.click(screen.getByRole('button', { name: 'Revoke' }));
    expect(onRevoke).toHaveBeenCalledOnce();
    expect(screen.getByTestId('location')).toHaveTextContent(/^\/agents$/);
    await user.click(link);
    expect(screen.getByTestId('location')).toHaveTextContent('/agents/entity-1');
  });

  it('tabs to the true navigation control and then its independent action', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const onRevoke = vi.fn();
    mount(<Component id="agent-tab" name="Keyboard agent" href="/agents/agent-tab" onSelect={onSelect}
      actions={<Button onClick={onRevoke}>Revoke</Button>} />);
    const link = screen.getByRole('link', { name: 'Keyboard agent' });
    await user.tab();
    expect(link).toHaveFocus();
    expect(onSelect).not.toHaveBeenCalled();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Revoke' })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(onRevoke).toHaveBeenCalledOnce();
    expect(onSelect).not.toHaveBeenCalled();
    expect(screen.getByTestId('location')).toHaveTextContent(/^\/agents$/);
  });

  it('selects via a keyboard-accessible surface when no detail link exists', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    mount(<Component id="item-2" name="A long name" onSelect={onSelect} selected supporting="Finance agent" status={<Badge variant="risk-high">High risk</Badge>} meta="Expires in 2 minutes" />);
    expect(screen.getByRole('button', { name: 'A long name' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'A long name' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'A long name' })).toHaveAccessibleDescription('High risk Finance agent Expires in 2 minutes');
    expect(document.getElementById('item-2')).toHaveAttribute('data-selected', 'true');
    await user.tab();
    expect(screen.getByRole('button', { name: 'A long name' })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(onSelect).toHaveBeenCalledOnce();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  it('only reveals a clipped clickable name on hover and keyboard focus', async () => {
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(200);
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockReturnValue(400);
    const user = userEvent.setup();
    const name = 'An unusually long name that still occupies only one line in the collection';
    mount(<Component id="long-name" name={name} href="/agents/long-name" actions={<Button>Revoke</Button>} />);
    const link = screen.getByRole('link', { name });
    expect(within(screen.getByTestId('entity-name')).getByText(name)).not.toHaveAttribute('tabindex');
    await user.hover(link);
    expect(await screen.findByRole('tooltip')).toHaveTextContent(name);
    await user.unhover(link);
    await user.pointer({ target: document.body, coords: { clientX: 1000, clientY: 1000 } });
    await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
    await user.tab();
    expect(link).toHaveFocus();
    expect(await screen.findByRole('tooltip')).toHaveTextContent(name);
    await user.tab();
    expect(screen.getByRole('button', { name: 'Revoke' })).toHaveFocus();
  });

  it('does not show a name tooltip when the clickable name fits', async () => {
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(200);
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockReturnValue(200);
    const user = userEvent.setup();
    mount(<Component id="short-name" name="Agent One" onSelect={vi.fn()} actions={<Button>Revoke</Button>} />);
    const select = screen.getByRole('button', { name: 'Agent One' });
    await user.hover(select);
    await user.tab();
    expect(select).toHaveFocus();
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Revoke' })).toHaveFocus();
  });

  it('reveals a clipped passive name without wrapping the metadata or action controls', async () => {
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(200);
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockReturnValue(400);
    const user = userEvent.setup();
    const onScope = vi.fn();
    const onRevoke = vi.fn();
    const name = 'Noninteractive connection with a clipped display name';
    mount(<Component id="passive" name={name} status={<Badge variant="warning">Expired</Badge>}
      supporting="Your access expired. Reconnect to continue."
      meta={<button onClick={onScope}>View scope</button>}
      actions={<Button onClick={onRevoke}>Disconnect</Button>} />);
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(screen.getByText('Expired')).toBeVisible();
    expect(screen.getByText('Your access expired. Reconnect to continue.')).toBeVisible();
    await user.tab();
    expect(within(screen.getByTestId('entity-name')).getByText(name)).toHaveFocus();
    expect(await screen.findByRole('tooltip')).toHaveTextContent(name);
    await user.tab();
    expect(screen.getByRole('button', { name: 'View scope' })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(onScope).toHaveBeenCalledOnce();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Disconnect' })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(onRevoke).toHaveBeenCalledOnce();
  });

  it('keeps a fitting passive name out of the tab order', async () => {
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(200);
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockReturnValue(200);
    const user = userEvent.setup();
    mount(<Component id="fitting" name="Short name" meta={<button>View scope</button>} />);
    expect(screen.getByTestId('entity-name')).not.toHaveAttribute('tabindex');
    await user.tab();
    expect(screen.getByRole('button', { name: 'View scope' })).toHaveFocus();
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('makes busy content and actions noninteractive without hiding identity', async () => {
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(200);
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockReturnValue(400);
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const onRevoke = vi.fn();
    mount(<Component id="item-busy" name="Busy agent" href="/agents/item-busy" busy onSelect={onSelect} actions={<Button variant="destructive-outline" size="sm" onClick={onRevoke}>Revoke</Button>} />);
    expect(document.getElementById('item-busy')).toHaveAttribute('aria-busy', 'true');
    expect(screen.getByText('Busy agent')).toBeVisible();
    expect(screen.getByTestId('entity-name')).not.toHaveAttribute('tabindex');
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Revoke' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Revoke' }));
    expect(onRevoke).not.toHaveBeenCalled();
    expect(onSelect).not.toHaveBeenCalled();
  });
});
