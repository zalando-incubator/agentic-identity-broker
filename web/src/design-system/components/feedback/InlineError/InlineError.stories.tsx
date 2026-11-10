/**
 * InlineError Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { InlineError } from './InlineError';

const meta = {
  title: 'Design System/Feedback/InlineError',
  component: InlineError,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
  argTypes: {
    message: {
      control: 'text',
      description: 'Error message text',
    },
    errors: {
      control: 'object',
      description: 'Array of error messages (for multiple errors)',
    },
    suggestions: {
      control: 'object',
      description: 'Array of suggestion texts to help fix the error',
    },
    helperText: {
      control: 'text',
      description: 'Optional helper text shown below error',
    },
    fieldLabel: {
      control: 'text',
      description: 'Associated field label for accessibility',
    },
    hideIcon: {
      control: 'boolean',
      description: 'Hide the default error icon',
    },
    size: {
      control: 'select',
      options: ['sm', 'md'],
      description: 'Size variant',
    },
  },
} satisfies Meta<typeof InlineError>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default inline error with basic message
 */
export const Default: Story = {
  args: {
    message: 'This field is required',
  },
};

/**
 * Error with custom icon
 */
export const WithIcon: Story = {
  render: () => {
    const CustomIcon = () => (
      <svg
        className="w-4 h-4"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
    );

    return (
      <div className="space-y-4 max-w-md">
        <InlineError message="Default error icon (X in circle)" />
        <InlineError
          message="Custom icon (info circle)"
          icon={<CustomIcon />}
        />
      </div>
    );
  },
  args: {
    message: 'Error with icon',
  },
};

/**
 * Error positioned below a form field
 */
export const FieldLevel: Story = {
  render: () => (
    <div className="max-w-md space-y-6">
      {/* Valid field */}
      <div>
        <label
          htmlFor="email-valid"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Email Address (Valid)
        </label>
        <input
          id="email-valid"
          type="email"
          placeholder="user@example.com"
          value="user@example.com"
          className="w-full px-3 py-2 border border-neutral-300 rounded-md focus:outline-none focus:ring-2 focus:ring-trust-deep focus:border-transparent"
          readOnly
        />
      </div>

      {/* Invalid field with error */}
      <div>
        <label
          htmlFor="email-invalid"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Email Address (Invalid)
        </label>
        <input
          id="email-invalid"
          type="email"
          placeholder="user@example.com"
          value="invalid-email"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="email-error"
        />
        <div id="email-error">
          <InlineError
            message="Please enter a valid email address"
            fieldLabel="Email Address"
          />
        </div>
      </div>

      {/* Required field with error */}
      <div>
        <label
          htmlFor="password-empty"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Password (Required)
        </label>
        <input
          id="password-empty"
          type="password"
          placeholder="Enter password"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="password-error"
        />
        <div id="password-error">
          <InlineError message="Password is required" fieldLabel="Password" />
        </div>
      </div>
    </div>
  ),
  args: {
    message: 'Field-level error',
  },
};

/**
 * Multiple validation errors for a single field
 */
export const MultipleErrors: Story = {
  render: () => (
    <div className="max-w-md space-y-6">
      <div>
        <label
          htmlFor="username"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Username
        </label>
        <input
          id="username"
          type="text"
          placeholder="Enter username"
          value="ab"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="username-errors"
        />
        <div id="username-errors">
          <InlineError
            errors={[
              'Username must be at least 3 characters',
              'Username can only contain letters and numbers',
              'Username is already taken',
            ]}
            message="Multiple validation errors"
            fieldLabel="Username"
          />
        </div>
      </div>

      <div>
        <label
          htmlFor="phone"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Phone Number
        </label>
        <input
          id="phone"
          type="tel"
          placeholder="(555) 123-4567"
          value="123"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="phone-errors"
        />
        <div id="phone-errors">
          <InlineError
            errors={[
              'Phone number must be 10 digits',
              'Phone number format is invalid',
            ]}
            message="Phone validation errors"
            fieldLabel="Phone Number"
          />
        </div>
      </div>
    </div>
  ),
  args: {
    errors: [
      'Username must be at least 3 characters',
      'Username can only contain letters and numbers',
    ],
    message: 'Multiple errors',
  },
};

/**
 * Error with helpful suggestions
 */
export const WithSuggestions: Story = {
  render: () => (
    <div className="max-w-md space-y-6">
      <div>
        <label
          htmlFor="password-weak"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Password
        </label>
        <input
          id="password-weak"
          type="password"
          placeholder="Enter password"
          value="abc123"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="password-suggestions"
        />
        <div id="password-suggestions">
          <InlineError
            message="Password is too weak"
            suggestions={[
              'Use at least 8 characters',
              'Include uppercase and lowercase letters',
              'Add numbers and special symbols',
              'Avoid common words or patterns',
            ]}
            fieldLabel="Password"
          />
        </div>
      </div>

      <div>
        <label
          htmlFor="upload"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Profile Photo
        </label>
        <input
          id="upload"
          type="file"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="upload-suggestions"
        />
        <div id="upload-suggestions">
          <InlineError
            message="File size is too large (5.2 MB)"
            suggestions={[
              'Maximum file size is 2 MB',
              'Try compressing your image',
              'Supported formats: JPG, PNG, GIF',
            ]}
            fieldLabel="Profile Photo"
          />
        </div>
      </div>
    </div>
  ),
  args: {
    message: 'Password is too weak',
    suggestions: [
      'Use at least 8 characters',
      'Include uppercase and lowercase letters',
      'Add numbers and special symbols',
    ],
  },
};

/**
 * Error message with actionable links
 */
export const WithLinks: Story = {
  render: () => (
    <div className="max-w-md space-y-6">
      <div>
        <label
          htmlFor="email-taken"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Email Address
        </label>
        <input
          id="email-taken"
          type="email"
          placeholder="user@example.com"
          value="existing@example.com"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="email-link-error"
        />
        <div id="email-link-error">
          <InlineError
            message={
              <>
                This email is already registered.{' '}
                <a
                  href="/login"
                  className="underline hover:text-error-primary font-medium"
                >
                  Sign in instead?
                </a>
              </>
            }
            fieldLabel="Email Address"
          />
        </div>
      </div>

      <div>
        <label
          htmlFor="code-invalid"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Verification Code
        </label>
        <input
          id="code-invalid"
          type="text"
          placeholder="Enter 6-digit code"
          value="123456"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="code-link-error"
        />
        <div id="code-link-error">
          <InlineError
            message={
              <>
                Invalid verification code.{' '}
                <button
                  type="button"
                  onClick={() => alert('Resending code...')}
                  className="underline hover:text-error-primary font-medium"
                >
                  Resend code
                </button>
              </>
            }
            fieldLabel="Verification Code"
          />
        </div>
      </div>
    </div>
  ),
  args: {
    message: 'Error with link',
  },
};

/**
 * Error with field label context
 */
export const WithFieldLabel: Story = {
  render: () => (
    <div className="max-w-md space-y-6">
      <div>
        <label
          htmlFor="billing-zip"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Billing ZIP Code
        </label>
        <input
          id="billing-zip"
          type="text"
          placeholder="12345"
          value="ABC"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="zip-error"
        />
        <div id="zip-error">
          <InlineError
            message="ZIP code must be 5 digits"
            fieldLabel="Billing ZIP Code"
            helperText="Example: 12345"
          />
        </div>
      </div>

      <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
        <p className="text-sm text-neutral-700">
          The <code>fieldLabel</code> prop adds screen reader context (hidden
          visually). It helps screen reader users understand which field has an
          error.
        </p>
      </div>
    </div>
  ),
  args: {
    message: 'ZIP code must be 5 digits',
    fieldLabel: 'Billing ZIP Code',
  },
};

/**
 * Error with helper text
 */
export const WithHelperText: Story = {
  render: () => (
    <div className="max-w-md space-y-6">
      <div>
        <label
          htmlFor="amount"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Transfer Amount
        </label>
        <input
          id="amount"
          type="number"
          placeholder="0.00"
          value="25000"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="amount-error"
        />
        <div id="amount-error">
          <InlineError
            message="Transfer amount exceeds daily limit"
            helperText="Your daily transfer limit is $10,000. Contact support to increase your limit."
            fieldLabel="Transfer Amount"
          />
        </div>
      </div>

      <div>
        <label
          htmlFor="domain"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Custom Domain
        </label>
        <input
          id="domain"
          type="text"
          placeholder="example.com"
          value="invalid..domain"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="domain-error"
        />
        <div id="domain-error">
          <InlineError
            message="Invalid domain format"
            helperText="Domain must contain only letters, numbers, hyphens, and periods."
            fieldLabel="Custom Domain"
          />
        </div>
      </div>
    </div>
  ),
  args: {
    message: 'Transfer amount exceeds daily limit',
    helperText:
      'Your daily transfer limit is $10,000. Contact support to increase your limit.',
  },
};

/**
 * Different size variants
 */
export const Sizes: Story = {
  render: () => (
    <div className="max-w-md space-y-6">
      <div>
        <label
          htmlFor="field-sm"
          className="block text-xs font-medium text-neutral-700 mb-1"
        >
          Small Field
        </label>
        <input
          id="field-sm"
          type="text"
          className="w-full px-2 py-1 text-sm border-2 border-error-primary rounded-md"
          aria-invalid="true"
          aria-describedby="error-sm"
        />
        <div id="error-sm">
          <InlineError
            size="sm"
            message="Small size error message"
            suggestions={['Suggestion in small size']}
          />
        </div>
      </div>

      <div>
        <label
          htmlFor="field-md"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Medium Field (Default)
        </label>
        <input
          id="field-md"
          type="text"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md"
          aria-invalid="true"
          aria-describedby="error-md"
        />
        <div id="error-md">
          <InlineError
            size="md"
            message="Medium size error message"
            suggestions={['Suggestion in medium size']}
          />
        </div>
      </div>
    </div>
  ),
  args: {
    message: 'Size variants',
    size: 'md',
  },
};

/**
 * Error without icon
 */
export const WithoutIcon: Story = {
  render: () => (
    <div className="max-w-md space-y-6">
      <div>
        <label
          htmlFor="simple"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Simple Field
        </label>
        <input
          id="simple"
          type="text"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md"
          aria-invalid="true"
          aria-describedby="simple-error"
        />
        <div id="simple-error">
          <InlineError message="This field is required" hideIcon />
        </div>
      </div>

      <div>
        <label
          htmlFor="minimal"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Minimal Error
        </label>
        <input
          id="minimal"
          type="text"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md"
          aria-invalid="true"
          aria-describedby="minimal-error"
        />
        <div id="minimal-error">
          <InlineError
            message="Please enter a valid value"
            hideIcon
            helperText="No icon for cleaner appearance"
          />
        </div>
      </div>
    </div>
  ),
  args: {
    message: 'Error without icon',
    hideIcon: true,
  },
};

/**
 * Interactive playground with all controls
 */
export const Playground: Story = {
  render: (args) => (
    <div className="max-w-md">
      <label
        htmlFor="playground-field"
        className="block text-sm font-medium text-neutral-700 mb-1"
      >
        Form Field
      </label>
      <input
        id="playground-field"
        type="text"
        className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
        aria-invalid="true"
        aria-describedby="playground-error"
      />
      <div id="playground-error">
        <InlineError {...args} />
      </div>
    </div>
  ),
  args: {
    message: 'This is an error message',
    suggestions: ['Try this suggestion', 'Or this one'],
    helperText: 'Additional helper text',
    fieldLabel: 'Form Field',
    hideIcon: false,
    size: 'md',
  },
};

/**
 * Real-world consent form validation examples
 */
export const RealWorldExamples: Story = {
  render: () => (
    <div className="max-w-2xl space-y-8">
      <div>
        <h3 className="text-lg font-medium text-neutral-900 mb-4">
          Consent Form Validation
        </h3>

        <div className="space-y-6">
          {/* Email validation */}
          <div>
            <label
              htmlFor="consent-email"
              className="block text-sm font-medium text-neutral-700 mb-1"
            >
              Email Address *
            </label>
            <input
              id="consent-email"
              type="email"
              placeholder="user@example.com"
              value="invalid.email@"
              className="w-full px-3 py-2 border-2 border-error-primary rounded-md"
              aria-invalid="true"
              aria-describedby="consent-email-error"
            />
            <div id="consent-email-error">
              <InlineError
                message="Please enter a valid email address"
                suggestions={[
                  'Email must contain @ and a domain (e.g., user@example.com)',
                ]}
                fieldLabel="Email Address"
              />
            </div>
          </div>

          {/* Organization name */}
          <div>
            <label
              htmlFor="org-name"
              className="block text-sm font-medium text-neutral-700 mb-1"
            >
              Organization Name *
            </label>
            <input
              id="org-name"
              type="text"
              placeholder="Your organization"
              className="w-full px-3 py-2 border-2 border-error-primary rounded-md"
              aria-invalid="true"
              aria-describedby="org-error"
            />
            <div id="org-error">
              <InlineError
                message="Organization name is required"
                fieldLabel="Organization Name"
              />
            </div>
          </div>

          {/* Data retention period */}
          <div>
            <label
              htmlFor="retention"
              className="block text-sm font-medium text-neutral-700 mb-1"
            >
              Data Retention Period (days) *
            </label>
            <input
              id="retention"
              type="number"
              placeholder="90"
              value="500"
              className="w-full px-3 py-2 border-2 border-error-primary rounded-md"
              aria-invalid="true"
              aria-describedby="retention-error"
            />
            <div id="retention-error">
              <InlineError
                message="Data retention period exceeds maximum allowed"
                suggestions={[
                  'Maximum retention period is 365 days',
                  'For longer retention, contact compliance@example.com',
                ]}
                helperText="Must comply with GDPR and local privacy regulations"
                fieldLabel="Data Retention Period"
              />
            </div>
          </div>

          {/* Terms acceptance */}
          <div>
            <div className="flex items-start">
              <input
                id="terms"
                type="checkbox"
                className="mt-1 h-4 w-4 text-trust-deep border-2 border-error-primary rounded focus:ring-error-primary"
                aria-invalid="true"
                aria-describedby="terms-error"
              />
              <label htmlFor="terms" className="ml-2 text-sm text-neutral-700">
                I agree to the Terms of Service and Privacy Policy
              </label>
            </div>
            <div id="terms-error" className="ml-6">
              <InlineError
                message="You must accept the terms and conditions to continue"
                hideIcon
                size="sm"
                fieldLabel="Terms Acceptance"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  ),
  args: {
    message: 'Real-world validation example',
  },
};

/**
 * Accessibility features demonstration
 */
export const Accessibility: Story = {
  render: () => (
    <div className="space-y-6 max-w-3xl">
      <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
        <h4 className="text-sm font-medium text-neutral-900 mb-2">
          Accessibility Features
        </h4>
        <ul className="text-sm text-neutral-700 space-y-1">
          <li>
            • Uses <code>role="alert"</code> for immediate screen reader
            announcement
          </li>
          <li>
            • Uses <code>aria-live="polite"</code> to avoid interrupting users
          </li>
          <li>
            • Uses <code>aria-atomic="true"</code> for complete message reading
          </li>
          <li>
            • Icons marked with <code>aria-hidden="true"</code>
          </li>
          <li>
            • Field association via <code>aria-describedby</code> and{' '}
            <code>aria-invalid</code>
          </li>
          <li>
            • Optional <code>fieldLabel</code> for screen reader context
          </li>
          <li>• Error text color meets WCAG 2.1 AA contrast requirements</li>
          <li>• Keyboard accessible when containing links/buttons</li>
        </ul>
      </div>

      <div>
        <label
          htmlFor="accessible-field"
          className="block text-sm font-medium text-neutral-700 mb-1"
        >
          Accessible Form Field
        </label>
        <input
          id="accessible-field"
          type="text"
          className="w-full px-3 py-2 border-2 border-error-primary rounded-md focus:outline-none focus:ring-2 focus:ring-error-primary"
          aria-invalid="true"
          aria-describedby="accessible-error"
        />
        <div id="accessible-error">
          <InlineError
            message="This error will be announced to screen readers"
            fieldLabel="Accessible Form Field"
            helperText="Try navigating with a screen reader to hear the announcement"
          />
        </div>
      </div>
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
            id: 'aria-valid-attr',
            enabled: true,
          },
        ],
      },
    },
  },
  args: {
    message: 'Accessible error message',
  },
};
