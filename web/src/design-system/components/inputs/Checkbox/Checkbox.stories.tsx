/**
 * Checkbox Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Checkbox } from './Checkbox';

const meta = {
  title: 'Design System/Inputs/Checkbox',
  component: Checkbox,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
} satisfies Meta<typeof Checkbox>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    label: 'Accept terms and conditions',
  },
};

export const Sizes: Story = {
  render: () => (
    <div className="space-y-4">
      <Checkbox size="sm" label="Small checkbox" />
      <Checkbox size="md" label="Medium checkbox (default)" />
      <Checkbox size="lg" label="Large checkbox" />
    </div>
  ),
};

export const States: Story = {
  render: () => (
    <div className="space-y-4">
      <Checkbox label="Unchecked" checked={false} />
      <Checkbox label="Checked" checked={true} />
      <Checkbox label="Disabled" disabled />
      <Checkbox label="Disabled checked" checked={true} disabled />
    </div>
  ),
};

export const WithDescription: Story = {
  render: () => (
    <div className="space-y-4 w-96">
      <Checkbox
        label="Email notifications"
        description="Receive updates about your grants"
      />
      <Checkbox
        label="Share analytics"
        description="Help us improve by sharing usage data"
        checked={true}
      />
    </div>
  ),
};

export const ValidationStates: Story = {
  render: () => (
    <div className="space-y-4 w-96">
      <Checkbox
        label="I agree"
        checked={true}
        variant="success"
        successMessage="Thank you!"
      />
      <Checkbox
        label="Confirm deletion"
        variant="error"
        errorMessage="You must confirm before deleting"
      />
    </div>
  ),
};

export const Group: Story = {
  render: () => (
    <div className="space-y-3 p-4 bg-white border border-neutral-200 rounded-lg w-96">
      <h4 className="text-sm font-medium text-neutral-900 mb-2">Permissions</h4>
      <Checkbox
        label="Read emails"
        description="View your email messages"
        checked={true}
      />
      <Checkbox
        label="Send emails"
        description="Send emails on your behalf"
        checked={true}
      />
      <Checkbox
        label="Delete emails"
        description="Delete your email messages"
      />
      <Checkbox
        label="Manage labels"
        description="Create and manage email labels"
        checked={true}
      />
    </div>
  ),
};

export const Playground: Story = {
  args: {
    label: 'Accept',
    size: 'md',
    disabled: false,
    checked: false,
  },
};

export const Accessibility: Story = {
  render: () => (
    <div className="space-y-4 w-96">
      <Checkbox
        id="terms-checkbox"
        label="I have read and agree to the terms of service"
        description="This is required to continue"
      />
      <Checkbox
        id="privacy-checkbox"
        label="I consent to the privacy policy"
        checked={true}
      />
    </div>
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
