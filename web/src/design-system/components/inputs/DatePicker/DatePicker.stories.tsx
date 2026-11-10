/**
 * DatePicker Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { DatePicker } from './DatePicker';
import { addDays, startOfToday } from 'date-fns';

const meta = {
  title: 'Design System/Inputs/DatePicker',
  component: DatePicker,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component:
          'Date input with validation states, min/max date support, and semantic styling.',
      },
    },
  },
  tags: ['autodocs'],
} satisfies Meta<typeof DatePicker>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => {
    const [date, setDate] = useState<Date | null>(null);
    return <DatePicker label="Select a date" value={date} onChange={setDate} />;
  },
};

export const Sizes: Story = {
  render: () => {
    const [sm, setSm] = useState<Date | null>(null);
    const [md, setMd] = useState<Date | null>(null);
    const [lg, setLg] = useState<Date | null>(null);

    return (
      <div className="w-96 space-y-4">
        <DatePicker size="sm" label="Small" value={sm} onChange={setSm} />
        <DatePicker
          size="md"
          label="Medium (default)"
          value={md}
          onChange={setMd}
        />
        <DatePicker size="lg" label="Large" value={lg} onChange={setLg} />
      </div>
    );
  },
};

export const States: Story = {
  render: () => {
    const [value, setValue] = useState<Date | null>(null);

    return (
      <div className="w-96 space-y-4">
        <DatePicker label="Default" value={value} onChange={setValue} />
        <DatePicker
          label="Error"
          value={new Date('2020-01-01')}
          onChange={() => {}}
          variant="error"
          errorMessage="Date cannot be in the past"
        />
        <DatePicker
          label="Success"
          value={new Date()}
          onChange={() => {}}
          variant="success"
          successMessage="Valid date selected"
        />
        <DatePicker
          label="Disabled"
          value={null}
          onChange={() => {}}
          disabled
        />
      </div>
    );
  },
};

export const WithMinDate: Story = {
  render: () => {
    const [date, setDate] = useState<Date | null>(null);
    const minDate = startOfToday();

    return (
      <DatePicker
        label="Grant expiration"
        value={date}
        onChange={setDate}
        minDate={minDate}
        helperText="Must be today or later"
      />
    );
  },
};

export const WithMaxDate: Story = {
  render: () => {
    const [date, setDate] = useState<Date | null>(null);
    const maxDate = addDays(startOfToday(), 30);

    return (
      <DatePicker
        label="Valid until"
        value={date}
        onChange={setDate}
        maxDate={maxDate}
        helperText="Must be within 30 days"
      />
    );
  },
};

export const DateRange: Story = {
  render: () => {
    const [startDate, setStartDate] = useState<Date | null>(null);
    const [endDate, setEndDate] = useState<Date | null>(null);

    return (
      <div className="w-96 space-y-4">
        <DatePicker
          label="Start date"
          value={startDate}
          onChange={setStartDate}
          required
          helperText="Select the start date"
        />
        <DatePicker
          label="End date"
          value={endDate}
          onChange={setEndDate}
          minDate={startDate || undefined}
          required
          helperText="Must be after start date"
        />
      </div>
    );
  },
};

export const RealWorldUseCases: Story = {
  render: () => {
    const [expiryDate, setExpiryDate] = useState<Date | null>(null);
    const minDate = startOfToday();

    return (
      <div className="w-96 space-y-6 p-6 bg-white border border-neutral-200 rounded-lg">
        <h3 className="text-sm font-medium text-neutral-900">
          Grant Configuration
        </h3>

        <DatePicker
          label="Grant expiration date"
          value={expiryDate}
          onChange={setExpiryDate}
          minDate={minDate}
          required
          helperText="When should this permission grant expire?"
        />

        <div className="text-xs text-neutral-600">
          {expiryDate ? (
            <p>
              Grant expires on:{' '}
              <span className="font-medium">
                {expiryDate.toLocaleDateString()}
              </span>
            </p>
          ) : (
            <p>No expiration date selected</p>
          )}
        </div>
      </div>
    );
  },
};

export const Playground: Story = {
  render: () => {
    const [date, setDate] = useState<Date | null>(null);

    return (
      <DatePicker
        label="Pick a date"
        value={date}
        onChange={setDate}
        size="md"
        required={false}
      />
    );
  },
};

export const Accessibility: Story = {
  render: () => {
    const [date, setDate] = useState<Date | null>(null);

    return (
      <div className="w-96">
        <DatePicker
          id="grant-expiry"
          label="Grant expiration date"
          value={date}
          onChange={setDate}
          required
          helperText="Select when this permission should expire"
          minDate={startOfToday()}
        />
      </div>
    );
  },
  parameters: {
    a11y: {
      config: {
        rules: [
          {
            id: 'color-contrast',
            enabled: true,
          },
        ],
      },
    },
  },
};
