/**
 * TextInput Component Stories
 *
 * Demonstrates all TextInput variants, sizes, states, and configurations.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { TextInput } from './TextInput';

const meta = {
  title: 'Design System/Inputs/TextInput',
  component: TextInput,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component:
          'Flexible text input with validation states, icons, helper text, and character count support.',
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
    },
    variant: {
      control: 'select',
      options: ['default', 'error', 'success'],
    },
    label: {
      control: 'text',
    },
    placeholder: {
      control: 'text',
    },
    helperText: {
      control: 'text',
    },
    errorMessage: {
      control: 'text',
    },
    successMessage: {
      control: 'text',
    },
    required: {
      control: 'boolean',
    },
    disabled: {
      control: 'boolean',
    },
    showCharCount: {
      control: 'boolean',
    },
  },
} satisfies Meta<typeof TextInput>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default text input
 */
export const Default: Story = {
  args: {
    label: 'Email',
    placeholder: 'you@example.com',
  },
};

/**
 * All input sizes
 */
export const Sizes: Story = {
  render: () => (
    <div className="w-96 space-y-4">
      <TextInput size="sm" label="Small" placeholder="Small input" />
      <TextInput
        size="md"
        label="Medium (default)"
        placeholder="Medium input"
      />
      <TextInput size="lg" label="Large" placeholder="Large input" />
    </div>
  ),
};

/**
 * Input states: default, focus, error, success, disabled
 */
export const States: Story = {
  render: () => (
    <div className="w-96 space-y-4">
      <TextInput label="Default" placeholder="Default state" />
      <TextInput
        label="Error"
        placeholder="This field has an error"
        value="invalid@"
        variant="error"
        errorMessage="Invalid email format"
      />
      <TextInput
        label="Success"
        placeholder="This field is valid"
        value="user@example.com"
        variant="success"
        successMessage="Email looks good!"
      />
      <TextInput label="Disabled" placeholder="Disabled state" disabled />
    </div>
  ),
};

/**
 * With helper text
 */
export const WithHelperText: Story = {
  render: () => (
    <div className="w-96 space-y-6">
      <TextInput
        label="Email"
        placeholder="you@example.com"
        helperText="We'll never share your email with anyone else"
      />
      <TextInput
        label="Username"
        placeholder="choose-a-username"
        helperText="3-20 characters, letters and numbers only"
      />
    </div>
  ),
};

/**
 * With icons
 */
export const WithIcons: Story = {
  render: () => (
    <div className="w-96 space-y-4">
      <TextInput
        label="Search"
        placeholder="Search..."
        iconBefore={
          <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
        }
      />
      <TextInput
        label="Website"
        placeholder="example.com"
        iconBefore={
          <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.658 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"
            />
          </svg>
        }
      />
    </div>
  ),
};

/**
 * With character count
 */
export const WithCharCount: Story = {
  render: () => (
    <div className="w-96 space-y-4">
      <TextInput
        label="Bio"
        placeholder="Tell us about yourself"
        maxLength={100}
        showCharCount
        helperText="Make it interesting!"
      />
      <TextInput
        label="Short Title"
        placeholder="Keep it short"
        maxLength={30}
        showCharCount
      />
    </div>
  ),
};

/**
 * Form validation scenarios
 */
export const ValidationScenarios: Story = {
  render: () => (
    <div className="w-96 space-y-6 p-6 bg-white rounded-lg border border-neutral-200">
      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-4">
          Sign Up Form
        </h3>
      </div>

      <TextInput
        label="Email"
        type="email"
        placeholder="you@example.com"
        required
        helperText="Your email is required to create an account"
      />

      <TextInput
        label="Password"
        type="password"
        placeholder="••••••••"
        required
        errorMessage="Password must be at least 8 characters"
        variant="error"
      />

      <TextInput
        label="Confirm Password"
        type="password"
        placeholder="••••••••"
        required
        variant="success"
        successMessage="Passwords match!"
      />
    </div>
  ),
};

/**
 * Real-world use cases
 */
export const UseCases: Story = {
  render: () => (
    <div className="w-96 space-y-8">
      {/* Login field */}
      <div>
        <h4 className="text-xs font-medium text-neutral-700 mb-3">Login</h4>
        <TextInput
          label="Email or Username"
          placeholder="Enter your credentials"
          iconBefore={
            <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M16 12a4 4 0 10-8 0 4 4 0 008 0zm0 0v1.5a2.5 2.5 0 01-5 0V12m0 0V8.5A2.5 2.5 0 014 11m8-3h.01"
              />
            </svg>
          }
        />
      </div>

      {/* Search field */}
      <div>
        <h4 className="text-xs font-medium text-neutral-700 mb-3">Search</h4>
        <TextInput
          placeholder="Search agents, scopes..."
          iconBefore={
            <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
              />
            </svg>
          }
        />
      </div>

      {/* Required field */}
      <div>
        <h4 className="text-xs font-medium text-neutral-700 mb-3">
          Required Input
        </h4>
        <TextInput
          label="Grant Duration"
          type="number"
          placeholder="Days"
          required
          helperText="How long should this grant be valid?"
        />
      </div>
    </div>
  ),
};

/**
 * Interactive playground
 */
export const Playground: Story = {
  args: {
    label: 'Enter text',
    placeholder: 'Type something...',
    size: 'md',
    required: false,
    disabled: false,
  },
};

/**
 * Accessibility testing
 */
export const Accessibility: Story = {
  render: () => (
    <div className="w-96 space-y-6">
      <TextInput
        id="email-input"
        label="Email Address"
        type="email"
        placeholder="your.email@example.com"
        required
        helperText="Required for account creation"
        aria-describedby="email-helper"
      />

      <TextInput
        id="password-input"
        label="Password"
        type="password"
        placeholder="••••••••"
        required
        aria-describedby="password-requirements"
        helperText="Minimum 8 characters, one uppercase, one number"
      />

      <TextInput
        id="confirm-input"
        label="Confirm Password"
        type="password"
        placeholder="••••••••"
        required
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
          {
            id: 'label',
            enabled: true,
          },
        ],
      },
    },
  },
};
