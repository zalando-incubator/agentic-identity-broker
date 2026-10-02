import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from './Accordion';

function Disclosures({ label }: { label: string }) {
  return (
    <Accordion type="single" collapsible aria-label={label}>
      <AccordionItem value="first">
        <AccordionTrigger>First section</AccordionTrigger>
        <AccordionContent>First details</AccordionContent>
      </AccordionItem>
      <AccordionItem value="unavailable" disabled>
        <AccordionTrigger>Unavailable section</AccordionTrigger>
        <AccordionContent>Unavailable details</AccordionContent>
      </AccordionItem>
      <AccordionItem value="last">
        <AccordionTrigger>Last section</AccordionTrigger>
        <AccordionContent>Last details</AccordionContent>
      </AccordionItem>
    </Accordion>
  );
}

describe('Accordion', () => {
  it('connects each disclosure to its named panel and collapses the previous single item', async () => {
    const user = userEvent.setup();
    render(<Disclosures label="Details" />);
    const first = screen.getByRole('button', { name: 'First section' });
    await user.click(first);
    const panel = screen.getByRole('region', { name: 'First section' });
    expect(first).toHaveAttribute('aria-controls', panel.id);
    expect(panel).toHaveAttribute('aria-labelledby', first.id);
    expect(first).toHaveAttribute('aria-expanded', 'true');
    await user.click(screen.getByRole('button', { name: 'Last section' }));
    expect(first).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('region', { name: 'First section' })).not.toBeInTheDocument();
    await user.keyboard('{Enter}');
    expect(screen.queryByRole('region', { name: 'Last section' })).not.toBeInTheDocument();
  });

  it('keeps arrow and boundary navigation within its instance and skips disabled items', async () => {
    const user = userEvent.setup();
    render(<><Disclosures label="One" /><Disclosures label="Two" /></>);
    const second = within(screen.getByLabelText('Two'));
    const first = second.getByRole('button', { name: 'First section' });
    const last = second.getByRole('button', { name: 'Last section' });
    await user.click(first);
    await user.keyboard('{ArrowDown}');
    expect(last).toHaveFocus();
    await user.keyboard('{Home}');
    expect(first).toHaveFocus();
    await user.keyboard('{End}');
    expect(last).toHaveFocus();
    await user.keyboard('{ArrowDown}');
    expect(first).toHaveFocus();
    expect(second.getByRole('button', { name: 'Unavailable section' })).toBeDisabled();
  });

  it('keeps independent panels expanded in multiple mode', async () => {
    const user = userEvent.setup();
    render(
      <Accordion type="multiple">
        <AccordionItem value="a"><AccordionTrigger>Alpha</AccordionTrigger><AccordionContent>Alpha details</AccordionContent></AccordionItem>
        <AccordionItem value="b"><AccordionTrigger>Beta</AccordionTrigger><AccordionContent>Beta details</AccordionContent></AccordionItem>
      </Accordion>,
    );
    await user.click(screen.getByRole('button', { name: 'Alpha' }));
    await user.click(screen.getByRole('button', { name: 'Beta' }));
    expect(screen.getByRole('region', { name: 'Alpha' })).toBeVisible();
    expect(screen.getByRole('region', { name: 'Beta' })).toBeVisible();
  });
});
