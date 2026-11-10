/**
 * Select Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Select } from './Select';
import type { SelectOption } from './Select';

const meta = {
  title: 'Design System/Inputs/Select',
  component: Select,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
} satisfies Meta<typeof Select>;

export default meta;
type Story = StoryObj<typeof meta>;

const basicOptions: SelectOption[] = [
  { value: 'option1', label: 'Option 1' },
  { value: 'option2', label: 'Option 2' },
  { value: 'option3', label: 'Option 3' },
  { value: 'option4', label: 'Option 4', disabled: true },
];

const accessLevelOptions: SelectOption[] = [
  { value: 'viewer', label: 'Viewer - Read-only access' },
  { value: 'editor', label: 'Editor - Can modify content' },
  { value: 'admin', label: 'Admin - Full permissions' },
  { value: 'owner', label: 'Owner - Ownership control' },
];

const countryOptions: SelectOption[] = [
  { value: 'us', label: 'United States' },
  { value: 'ca', label: 'Canada' },
  { value: 'uk', label: 'United Kingdom' },
  { value: 'au', label: 'Australia' },
  { value: 'de', label: 'Germany' },
  { value: 'fr', label: 'France' },
  { value: 'jp', label: 'Japan' },
  { value: 'sg', label: 'Singapore' },
];

// Helper for single-select stories to properly type the onChange
const handleSingleChange =
  (setter: (value: string | number | null) => void) =>
  (value: string | number | (string | number)[] | null) => {
    if (
      typeof value === 'string' ||
      typeof value === 'number' ||
      value === null
    ) {
      setter(value);
    }
  };

export const Default: Story = {
  render: () => {
    const [selected, setSelected] = useState<string | number | null>(null);

    return (
      <Select
        options={basicOptions}
        value={selected}
        onChange={handleSingleChange(setSelected)}
        label="Choose an option"
        placeholder="Select one..."
        helperText="This is a basic select dropdown"
      />
    );
  },
  args: {
    options: basicOptions,
    value: null,
    onChange: () => {},
  },
};

export const Sizes: Story = {
  render: () => {
    const [small, setSmall] = useState<string | number | null>(null);
    const [medium, setMedium] = useState<string | number | null>(null);
    const [large, setLarge] = useState<string | number | null>(null);

    return (
      <div className="space-y-6 w-96">
        <Select
          size="sm"
          options={basicOptions}
          value={small}
          onChange={handleSingleChange(setSmall)}
          label="Small Select"
          placeholder="Select..."
        />
        <Select
          size="md"
          options={basicOptions}
          value={medium}
          onChange={handleSingleChange(setMedium)}
          label="Medium Select (default)"
          placeholder="Select..."
        />
        <Select
          size="lg"
          options={basicOptions}
          value={large}
          onChange={handleSingleChange(setLarge)}
          label="Large Select"
          placeholder="Select..."
        />
      </div>
    );
  },
  args: {
    options: basicOptions,
    value: null,
    onChange: () => {},
  },
};

export const States: Story = {
  render: () => {
    const [normal, setNormal] = useState<string | number | null>(null);
    const [error, setError] = useState<string | number | null>(null);
    const [success, setSuccess] = useState<string | number | null>('option2');
    const [disabled] = useState<string | number | null>(null);

    return (
      <div className="space-y-6 w-96">
        <Select
          options={basicOptions}
          value={normal}
          onChange={handleSingleChange(setNormal)}
          label="Normal State"
          placeholder="Select an option..."
        />
        <Select
          options={basicOptions}
          value={error}
          onChange={handleSingleChange(setError)}
          label="Error State"
          placeholder="Select an option..."
          errorMessage="Please select a valid option"
        />
        <Select
          options={basicOptions}
          value={success}
          onChange={handleSingleChange(setSuccess)}
          label="Success State"
          placeholder="Select an option..."
          successMessage="Option selected successfully"
        />
        <Select
          options={basicOptions}
          value={disabled}
          onChange={() => {}}
          label="Disabled Select"
          placeholder="Select an option..."
          disabled
          helperText="This select is disabled"
        />
      </div>
    );
  },
  args: {
    options: basicOptions,
    value: null,
    onChange: () => {},
  },
};

export const MultiSelect: Story = {
  render: () => {
    const [selected, setSelected] = useState<(string | number)[]>([]);

    return (
      <div className="w-96">
        <Select
          multiselect
          options={accessLevelOptions}
          value={selected}
          onChange={(val) => setSelected(Array.isArray(val) ? val : [])}
          label="Select access levels"
          placeholder="Choose one or more..."
          helperText="You can select multiple options"
        />
        {selected.length > 0 && (
          <div className="mt-4 p-3 bg-blue-50 border border-blue-200 rounded">
            <p className="text-sm font-medium text-blue-900 mb-2">Selected:</p>
            <ul className="text-sm text-blue-800 space-y-1">
              {selected.map((val, idx) => {
                const opt = accessLevelOptions.find((o) => o.value === val);
                return <li key={`${val}-${idx}`}>• {opt?.label}</li>;
              })}
            </ul>
          </div>
        )}
      </div>
    );
  },
  args: {
    options: accessLevelOptions,
    value: [],
    onChange: () => {},
  },
};

export const Searchable: Story = {
  render: () => {
    const [selected, setSelected] = useState<string | number | null>(null);

    return (
      <Select
        options={countryOptions}
        value={selected}
        onChange={handleSingleChange(setSelected)}
        label="Select a country"
        placeholder="Search or select..."
        helperText="Type to filter the list"
        searchable
        className="w-96"
      />
    );
  },
  args: {
    options: countryOptions,
    value: null,
    onChange: () => {},
  },
};

export const CustomRendering: Story = {
  render: () => {
    const [selected, setSelected] = useState<string | number | null>(null);

    const customOptions: SelectOption[] = [
      { value: 'us', label: 'United States' },
      { value: 'ca', label: 'Canada' },
      { value: 'mx', label: 'Mexico' },
    ];

    return (
      <Select
        options={customOptions}
        value={selected}
        onChange={handleSingleChange(setSelected)}
        label="Select a country"
        placeholder="Choose..."
        renderOption={(option, isSelected) => (
          <div className="flex items-center justify-between w-full">
            <div className="flex flex-col">
              <span className="font-medium">{option.label}</span>
              <span className="text-xs text-neutral-500">
                {String(option.value).toUpperCase()}
              </span>
            </div>
            {isSelected && (
              <span className="text-trust-deep font-medium">✓</span>
            )}
          </div>
        )}
        className="w-96"
      />
    );
  },
  args: {
    options: [
      { value: 'us', label: 'United States' },
      { value: 'ca', label: 'Canada' },
      { value: 'mx', label: 'Mexico' },
    ],
    value: null,
    onChange: () => {},
  },
};

export const GroupedOptions: Story = {
  render: () => {
    const [selected, setSelected] = useState<string | number | null>(null);

    const groupedOptions = [
      {
        label: 'North America',
        options: [
          { value: 'us', label: 'United States' },
          { value: 'ca', label: 'Canada' },
          { value: 'mx', label: 'Mexico' },
        ],
      },
      {
        label: 'Europe',
        options: [
          { value: 'uk', label: 'United Kingdom' },
          { value: 'de', label: 'Germany' },
          { value: 'fr', label: 'France' },
        ],
      },
      {
        label: 'Asia Pacific',
        options: [
          { value: 'au', label: 'Australia' },
          { value: 'jp', label: 'Japan' },
          { value: 'sg', label: 'Singapore' },
        ],
      },
    ];

    return (
      <Select
        options={groupedOptions}
        value={selected}
        onChange={handleSingleChange(setSelected)}
        label="Select a region"
        placeholder="Choose a country..."
        className="w-96"
      />
    );
  },
  args: {
    options: [
      { value: 'us', label: 'United States' },
      { value: 'ca', label: 'Canada' },
    ],
    value: null,
    onChange: () => {},
  },
};

export const WithHelperText: Story = {
  render: () => {
    const [selected, setSelected] = useState<string | number | null>(null);

    const permissionOptions: SelectOption[] = [
      { value: 'read', label: 'Read - View documents' },
      { value: 'comment', label: 'Comment - Add comments' },
      { value: 'edit', label: 'Edit - Modify documents' },
    ];

    return (
      <div className="space-y-8 w-96">
        <Select
          options={permissionOptions}
          value={selected}
          onChange={handleSingleChange(setSelected)}
          label="Permission Level"
          placeholder="Select permission..."
          helperText="Higher permissions allow more actions on shared documents"
          required
        />
        <Select
          options={permissionOptions}
          value={selected}
          onChange={handleSingleChange(setSelected)}
          label="Grant Duration"
          placeholder="Select duration..."
          errorMessage="You must select a duration"
        />
        <Select
          options={permissionOptions}
          value={selected}
          onChange={handleSingleChange(setSelected)}
          label="Access Level"
          placeholder="Select level..."
          successMessage="Access level configured"
        />
      </div>
    );
  },
  args: {
    options: [
      { value: 'read', label: 'Read' },
      { value: 'edit', label: 'Edit' },
    ],
    value: null,
    onChange: () => {},
  },
};

export const RealWorldUseCases: Story = {
  render: () => {
    const [duration, setDuration] = useState<string | number | null>('7days');
    const [permission, setPermission] = useState<string | number | null>(
      'viewer',
    );

    const durationOptions: SelectOption[] = [
      { value: '24hours', label: '24 hours - Limited time access' },
      { value: '7days', label: '7 days - One week duration' },
      { value: '30days', label: '30 days - One month access' },
      { value: 'unlimited', label: 'Unlimited - No expiration' },
    ];

    const permissionOptions: SelectOption[] = [
      { value: 'viewer', label: 'Viewer' },
      { value: 'commenter', label: 'Commenter' },
      { value: 'editor', label: 'Editor' },
      { value: 'admin', label: 'Administrator' },
    ];

    return (
      <div className="space-y-8 w-96 p-4 bg-white border border-neutral-200 rounded-lg">
        <div>
          <h3 className="text-lg font-medium text-neutral-900 mb-4">
            Grant Temporary Access
          </h3>

          <div className="space-y-6">
            <Select
              options={durationOptions}
              value={duration}
              onChange={handleSingleChange(setDuration)}
              label="Access Duration"
              placeholder="Choose duration..."
              helperText="How long should access be granted?"
            />

            <Select
              options={permissionOptions}
              value={permission}
              onChange={handleSingleChange(setPermission)}
              label="Permission Level"
              placeholder="Choose level..."
              helperText="What can they do with this access?"
            />

            <button className="w-full px-4 py-2 bg-trust-deep text-white rounded-lg font-medium hover:bg-trust-hover transition-colors">
              Grant Access
            </button>
          </div>
        </div>
      </div>
    );
  },
  args: {
    options: [
      { value: '7days', label: '7 days - One week duration' },
      { value: '30days', label: '30 days - One month access' },
    ],
    value: '7days',
    onChange: () => {},
  },
};

export const Playground: Story = {
  render: (args: any) => {
    const [selected, setSelected] = useState<
      string | number | (string | number)[] | null
    >(null);

    return (
      <Select
        {...args}
        value={selected}
        onChange={setSelected}
        className="w-96"
      />
    );
  },
  args: {
    options: basicOptions,
    value: null,
    onChange: () => {},
    label: 'Select an option',
    placeholder: 'Choose...',
    size: 'md',
    disabled: false,
    searchable: false,
    multiselect: false,
    required: false,
  },
} satisfies Story;

export const Accessibility: Story = {
  render: () => {
    const [selected, setSelected] = useState<string | number | null>(null);

    const roleOptions: SelectOption[] = [
      { value: 'viewer', label: 'Viewer' },
      { value: 'editor', label: 'Editor' },
      { value: 'admin', label: 'Administrator' },
    ];

    return (
      <fieldset className="border border-neutral-200 rounded-lg p-6 w-96">
        <legend className="text-lg font-medium text-neutral-900 mb-4">
          User Role Assignment
        </legend>

        <Select
          id="user-role"
          options={roleOptions}
          value={selected}
          onChange={handleSingleChange(setSelected)}
          label="Assign role"
          placeholder="Select a role..."
          helperText="Select the role that best describes this user's responsibilities"
          required
        />

        <div className="mt-6 p-3 bg-neutral-50 rounded border border-neutral-200">
          <p className="text-sm font-medium text-neutral-900">
            Accessibility notes:
          </p>
          <ul className="text-xs text-neutral-700 space-y-1 mt-2">
            <li>• Use Tab to navigate to the select</li>
            <li>• Press Space or Enter to open options</li>
            <li>• Use arrow keys to navigate options</li>
            <li>• Press Enter to select option</li>
            <li>• Press Escape to close dropdown</li>
          </ul>
        </div>
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
          {
            id: 'label',
            enabled: true,
          },
        ],
      },
    },
  },
  args: {
    options: [
      { value: 'viewer', label: 'Viewer' },
      { value: 'editor', label: 'Editor' },
    ],
    value: null,
    onChange: () => {},
  },
};

export const LoadingState: Story = {
  render: () => {
    const [selected, setSelected] = useState<string | number | null>(null);

    return (
      <div className="space-y-4 w-96">
        <Select
          options={basicOptions}
          value={selected}
          onChange={handleSingleChange(setSelected)}
          label="Simulated Loading"
          placeholder="Options loaded..."
          helperText="In a real app, these would be fetched from an API"
        />
        <div className="p-3 bg-blue-50 border border-blue-200 rounded text-sm text-blue-800">
          <strong>Note:</strong> Loading states are typically handled by your
          application layer. You can set <code>disabled</code> and show a
          loading state externally while fetching options.
        </div>
      </div>
    );
  },
  args: {
    options: basicOptions,
    value: null,
    onChange: () => {},
  },
};
