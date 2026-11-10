/**
 * Switch Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Switch } from './Switch';

const meta = {
  title: 'Design System/Inputs/Switch',
  component: Switch,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component:
          'Toggle switch for binary on/off controls with labels and descriptions.',
      },
    },
  },
  tags: ['autodocs'],
} satisfies Meta<typeof Switch>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => {
    const [checked, setChecked] = useState(false);
    return (
      <Switch checked={checked} onChange={setChecked} label="Enable feature" />
    );
  },
};

export const Sizes: Story = {
  render: () => {
    const [sm, setSm] = useState(true);
    const [md, setMd] = useState(true);
    const [lg, setLg] = useState(true);

    return (
      <div className="space-y-6">
        <Switch size="sm" checked={sm} onChange={setSm} label="Small" />
        <Switch
          size="md"
          checked={md}
          onChange={setMd}
          label="Medium (default)"
        />
        <Switch size="lg" checked={lg} onChange={setLg} label="Large" />
      </div>
    );
  },
};

export const Variants: Story = {
  render: () => {
    const [primary, setPrimary] = useState(true);
    const [success, setSuccess] = useState(true);

    return (
      <div className="space-y-6">
        <Switch
          variant="primary"
          checked={primary}
          onChange={setPrimary}
          label="Primary (trust)"
        />
        <Switch
          variant="success"
          checked={success}
          onChange={setSuccess}
          label="Success (emerald)"
        />
      </div>
    );
  },
};

export const States: Story = {
  render: () => {
    const [checked, setChecked] = useState(false);
    const [checked2, setChecked2] = useState(true);

    return (
      <div className="space-y-4">
        <Switch checked={checked} onChange={setChecked} label="Off state" />
        <Switch checked={checked2} onChange={setChecked2} label="On state" />
        <Switch checked={false} onChange={() => {}} label="Disabled" disabled />
        <Switch
          checked={true}
          onChange={() => {}}
          label="Disabled checked"
          disabled
        />
      </div>
    );
  },
};

export const WithDescription: Story = {
  render: () => {
    const [grant, setGrant] = useState(false);
    const [notify, setNotify] = useState(true);

    return (
      <div className="space-y-6 w-96">
        <Switch
          checked={grant}
          onChange={setGrant}
          label="Grant access"
          description="Allow this agent to read your emails"
          size="lg"
        />
        <Switch
          checked={notify}
          onChange={setNotify}
          label="Email notifications"
          description="Receive updates about your permissions"
          size="lg"
        />
      </div>
    );
  },
};

export const RealWorldUseCases: Story = {
  render: () => {
    const [notifications, setNotifications] = useState(true);
    const [twoFactor, setTwoFactor] = useState(false);
    const [analytics, setAnalytics] = useState(true);

    return (
      <div className="space-y-6 p-6 bg-white border border-neutral-200 rounded-lg w-96">
        <h3 className="text-sm font-medium text-neutral-900">Settings</h3>

        <Switch
          checked={notifications}
          onChange={setNotifications}
          label="Notifications"
          description="Receive email updates"
          size="md"
        />

        <Switch
          checked={twoFactor}
          onChange={setTwoFactor}
          label="Two-factor authentication"
          description="Require 2FA for logins"
          size="md"
        />

        <Switch
          checked={analytics}
          onChange={setAnalytics}
          label="Analytics"
          description="Help us improve with usage data"
          size="md"
        />
      </div>
    );
  },
};

export const Interactive: Story = {
  render: () => {
    const [checked, setChecked] = useState(false);

    return (
      <div className="space-y-4">
        <Switch
          checked={checked}
          onChange={setChecked}
          label="Try toggling me"
          description="Click to see me change"
          size="lg"
        />
        <div className="text-sm text-neutral-600">
          Current state:{' '}
          <span className="font-medium">{checked ? 'ON' : 'OFF'}</span>
        </div>
      </div>
    );
  },
};

export const Accessibility: Story = {
  render: () => {
    const [grant1, setGrant1] = useState(true);
    const [grant2, setGrant2] = useState(false);
    const [grant3, setGrant3] = useState(true);

    return (
      <fieldset className="space-y-4 p-4 bg-white border border-neutral-200 rounded-lg w-96">
        <legend className="text-sm font-medium text-neutral-900 mb-2">
          Grant Permissions
        </legend>

        <Switch
          id="read-emails"
          checked={grant1}
          onChange={setGrant1}
          label="Read emails"
          description="Access to email content"
        />

        <Switch
          id="send-emails"
          checked={grant2}
          onChange={setGrant2}
          label="Send emails"
          description="Send emails on your behalf"
        />

        <Switch
          id="manage-labels"
          checked={grant3}
          onChange={setGrant3}
          label="Manage labels"
          description="Create and organize labels"
        />
      </fieldset>
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
