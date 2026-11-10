/**
 * TextArea Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { TextArea } from './TextArea';

const meta = {
  title: 'Design System/Inputs/TextArea',
  component: TextArea,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component:
          'Multi-line text input with validation states, character count, and optional auto-grow support.',
      },
    },
  },
  tags: ['autodocs'],
} satisfies Meta<typeof TextArea>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    label: 'Comments',
    placeholder: 'Enter your feedback...',
    rows: 4,
  },
};

export const Sizes: Story = {
  render: () => (
    <div className="w-96 space-y-4">
      <TextArea label="Small (2 rows)" placeholder="Compact" rows={2} />
      <TextArea label="Medium (4 rows)" placeholder="Default size" rows={4} />
      <TextArea label="Large (6 rows)" placeholder="Spacious" rows={6} />
    </div>
  ),
};

export const States: Story = {
  render: () => (
    <div className="w-96 space-y-4">
      <TextArea label="Default" placeholder="Default state" rows={3} />
      <TextArea
        label="Error"
        value="This message contains inappropriate..."
        variant="error"
        errorMessage="Please avoid using inappropriate language"
        rows={3}
      />
      <TextArea
        label="Success"
        value="Thank you for the detailed feedback!"
        variant="success"
        successMessage="Feedback received!"
        rows={3}
      />
      <TextArea
        label="Disabled"
        placeholder="Disabled state"
        rows={3}
        disabled
      />
    </div>
  ),
};

export const WithCharCount: Story = {
  render: () => (
    <div className="w-96 space-y-4">
      <TextArea
        label="Bio (200 chars)"
        placeholder="Tell us about yourself"
        maxLength={200}
        showCharCount
        rows={4}
      />
      <TextArea
        label="Comments (500 chars)"
        placeholder="Share your thoughts"
        maxLength={500}
        showCharCount
        helperText="Be constructive and respectful"
        rows={4}
      />
    </div>
  ),
};

export const AutoGrow: Story = {
  render: () => (
    <div className="w-96 space-y-4">
      <TextArea
        label="Auto-growing textarea"
        placeholder="Type more to see it grow..."
        autoGrow
        helperText="This textarea grows as you type"
      />
    </div>
  ),
};

export const RealWorldUseCases: Story = {
  render: () => (
    <div className="w-96 space-y-6 p-6 bg-white rounded-lg border border-neutral-200">
      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-4">
          Feedback Form
        </h3>
      </div>

      <TextArea
        label="What went wrong?"
        placeholder="Describe the issue you encountered"
        required
        helperText="Please be as detailed as possible"
        rows={4}
      />

      <TextArea
        label="How can we improve?"
        placeholder="Your suggestions"
        helperText="We value your input"
        rows={3}
      />

      <TextArea label="Additional comments" placeholder="Optional" rows={2} />
    </div>
  ),
};

export const Playground: Story = {
  args: {
    label: 'Your message',
    placeholder: 'Type something...',
    rows: 4,
    required: false,
    disabled: false,
  },
};

export const Accessibility: Story = {
  render: () => (
    <div className="w-96 space-y-6">
      <TextArea
        id="feedback-input"
        label="Feedback"
        placeholder="Share your feedback"
        required
        helperText="Required to submit"
      />

      <TextArea
        id="description-input"
        label="Description"
        placeholder="Detailed description"
        maxLength={1000}
        showCharCount
        helperText="Maximum 1000 characters"
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
