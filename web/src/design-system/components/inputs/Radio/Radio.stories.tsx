/**
 * Radio Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Radio } from './Radio';

const meta = {
  title: 'Design System/Inputs/Radio',
  component: Radio,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
} satisfies Meta<typeof Radio>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    label: 'Select this option',
    name: 'example',
    value: 'option1',
  },
};

export const Sizes: Story = {
  render: () => (
    <div className="space-y-4">
      <Radio size="sm" label="Small radio" name="size" value="sm" />
      <Radio
        size="md"
        label="Medium radio (default)"
        name="size"
        value="md"
        checked
      />
      <Radio size="lg" label="Large radio" name="size" value="lg" />
    </div>
  ),
};

export const States: Story = {
  render: () => (
    <div className="space-y-4">
      <Radio label="Unchecked" name="state" value="unchecked" />
      <Radio label="Checked" name="state" value="checked" checked />
      <Radio label="Disabled" name="state" value="disabled" disabled />
      <Radio
        label="Disabled checked"
        name="state"
        value="disabled-checked"
        checked
        disabled
      />
    </div>
  ),
};

export const WithDescription: Story = {
  render: () => (
    <div className="space-y-4 w-96">
      <Radio
        label="7 days"
        description="Grant access for one week"
        name="duration"
        value="7days"
      />
      <Radio
        label="30 days"
        description="Grant access for one month"
        name="duration"
        value="30days"
        checked={true}
      />
      <Radio
        label="No expiration"
        description="Grant permanent access"
        name="duration"
        value="none"
      />
    </div>
  ),
};

export const Group: Story = {
  render: () => (
    <fieldset className="space-y-3 p-4 bg-white border border-neutral-200 rounded-lg w-96">
      <legend className="text-sm font-medium text-neutral-900 mb-2">
        Grant Duration
      </legend>
      <Radio
        name="duration"
        value="24hours"
        label="24 hours"
        description="Shortest duration"
      />
      <Radio
        name="duration"
        value="7days"
        label="7 days"
        description="Standard duration"
        checked={true}
      />
      <Radio
        name="duration"
        value="30days"
        label="30 days"
        description="Extended duration"
      />
      <Radio
        name="duration"
        value="unlimited"
        label="Unlimited"
        description="No automatic expiration"
      />
    </fieldset>
  ),
};

export const ValidationStates: Story = {
  render: () => (
    <div className="space-y-4 w-96">
      <Radio
        label="Approve"
        name="action"
        value="approve"
        checked={true}
        variant="success"
        successMessage="Ready to proceed"
      />
      <Radio
        label="Reject"
        name="action"
        value="reject"
        variant="error"
        errorMessage="You must select an action"
      />
    </div>
  ),
};

export const Playground: Story = {
  args: {
    label: 'Choose this',
    size: 'md',
    name: 'playground',
    value: 'option',
    disabled: false,
    checked: false,
  },
};

export const Accessibility: Story = {
  render: () => (
    <fieldset className="space-y-3 w-96">
      <legend className="text-sm font-medium text-neutral-900 mb-3">
        Select an access level
      </legend>
      <Radio
        id="read-only"
        name="access"
        value="read"
        label="Read-only"
        description="View permissions only"
      />
      <Radio
        id="read-write"
        name="access"
        value="write"
        label="Read and Write"
        description="Can view and modify"
        checked={true}
      />
      <Radio
        id="admin"
        name="access"
        value="admin"
        label="Admin"
        description="Full permissions"
      />
    </fieldset>
  ),
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
