/**
 * Progress Component Stories
 *
 * Comprehensive showcase of all Progress component variants and use cases.
 * Demonstrates linear and circular progress with various states and configurations.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Progress } from './Progress';
import React, { useState, useEffect } from 'react';

const meta = {
  title: 'Advanced/Progress',
  component: Progress,
  parameters: {
    layout: 'padded',
    docs: {
      description: {
        component:
          'Visual progress indicator for displaying task/operation completion. Supports both linear and circular variants with determinate and indeterminate states.',
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    value: {
      control: { type: 'range', min: 0, max: 100, step: 1 },
      description:
        'Progress value (0-100). If omitted, shows indeterminate state',
    },
    variant: {
      control: 'select',
      options: ['default', 'success', 'warning', 'error'],
      description: 'Visual variant for semantic meaning',
    },
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
      description: 'Size variant',
    },
    type: {
      control: 'select',
      options: ['linear', 'circular'],
      description: 'Progress type: linear bar or circular',
    },
    label: {
      control: 'text',
      description: 'Optional label displayed above/before progress',
    },
    showValue: {
      control: 'boolean',
      description: 'Whether to show value text',
    },
    valueFormat: {
      control: 'select',
      options: ['percentage', 'custom'],
      description: 'Value format: percentage or custom',
    },
    customValue: {
      control: 'text',
      description: 'Custom value text (overrides percentage)',
    },
    indeterminate: {
      control: 'boolean',
      description: 'Force indeterminate state (animated)',
    },
    striped: {
      control: 'boolean',
      description: 'Show striped animation effect (linear only)',
    },
    height: {
      control: 'text',
      description: 'Custom height for linear progress (overrides size)',
    },
  },
} satisfies Meta<typeof Progress>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Basic determinate linear progress at 45%
 */
export const DeterminateLinear: Story = {
  args: {
    value: 45,
    variant: 'default',
    size: 'md',
    type: 'linear',
  },
};

/**
 * Indeterminate loading state with animated progress bar
 */
export const IndeterminateLinear: Story = {
  args: {
    indeterminate: true,
    variant: 'default',
    size: 'md',
    type: 'linear',
  },
};

/**
 * Progress with label above the bar
 */
export const WithLabel: Story = {
  args: {
    value: 65,
    label: 'Upload progress',
    variant: 'default',
    size: 'md',
    type: 'linear',
  },
};

/**
 * Progress with percentage value displayed
 */
export const WithValue: Story = {
  args: {
    value: 78,
    label: 'Processing',
    showValue: true,
    variant: 'default',
    size: 'md',
    type: 'linear',
  },
};

/**
 * All status variants demonstrating semantic colors
 */
export const StatusVariants: Story = {
  render: () => (
    <div className="space-y-6 max-w-lg">
      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">Default</h3>
        <Progress
          value={60}
          variant="default"
          label="Default progress"
          showValue
        />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">Success</h3>
        <Progress
          value={100}
          variant="success"
          label="Completed successfully"
          showValue
        />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">Warning</h3>
        <Progress
          value={45}
          variant="warning"
          label="Approaching limit"
          showValue
        />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">Error</h3>
        <Progress value={25} variant="error" label="Upload failed" showValue />
      </div>
    </div>
  ),
};

/**
 * Size variants: small, medium, large
 */
export const SizeVariants: Story = {
  render: () => (
    <div className="space-y-6 max-w-lg">
      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">Small</h3>
        <Progress value={60} size="sm" label="Small progress" showValue />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Medium (Default)
        </h3>
        <Progress value={60} size="md" label="Medium progress" showValue />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">Large</h3>
        <Progress value={60} size="lg" label="Large progress" showValue />
      </div>
    </div>
  ),
};

/**
 * Circular progress variant with determinate value
 */
export const CircularProgress: Story = {
  args: {
    value: 75,
    variant: 'default',
    size: 'md',
    type: 'circular',
    label: 'Loading',
    showValue: true,
  },
};

/**
 * Circular indeterminate loading spinner
 */
export const CircularIndeterminate: Story = {
  args: {
    indeterminate: true,
    variant: 'default',
    size: 'md',
    type: 'circular',
    label: 'Loading...',
  },
};

/**
 * Linear progress with striped animation effect
 */
export const StripedEffect: Story = {
  render: () => (
    <div className="space-y-6 max-w-lg">
      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Default Striped
        </h3>
        <Progress
          value={60}
          variant="default"
          striped
          label="Processing"
          showValue
        />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Success Striped
        </h3>
        <Progress
          value={85}
          variant="success"
          striped
          label="Almost done"
          showValue
        />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Warning Striped
        </h3>
        <Progress
          value={45}
          variant="warning"
          striped
          label="Caution"
          showValue
        />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Error Striped
        </h3>
        <Progress
          value={30}
          variant="error"
          striped
          label="Error state"
          showValue
        />
      </div>
    </div>
  ),
};

/**
 * Custom height examples for linear progress
 */
export const CustomHeight: Story = {
  render: () => (
    <div className="space-y-6 max-w-lg">
      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Height: 8px
        </h3>
        <Progress value={60} height={8} label="Thin progress" showValue />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Height: 16px
        </h3>
        <Progress value={60} height={16} label="Default progress" showValue />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Height: 24px
        </h3>
        <Progress value={60} height={24} label="Thick progress" showValue />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Height: 32px
        </h3>
        <Progress
          value={60}
          height={32}
          label="Extra thick progress"
          showValue
        />
      </div>
    </div>
  ),
};

/**
 * Custom value text instead of percentage
 */
export const CustomValue: Story = {
  render: () => (
    <div className="space-y-6 max-w-lg">
      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Items Processed
        </h3>
        <Progress
          value={30}
          label="Processing items"
          showValue
          customValue="3 of 10 items"
        />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Files Uploaded
        </h3>
        <Progress
          value={75}
          variant="success"
          label="Upload progress"
          showValue
          customValue="15/20 files"
        />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-3">
          Time Remaining
        </h3>
        <Progress
          value={66}
          variant="warning"
          label="Download"
          showValue
          customValue="2 min left"
        />
      </div>
    </div>
  ),
};

/**
 * Multi-step progress indicator
 */
export const MultiStep: Story = {
  render: () => {
    const steps = [
      { label: 'Account Details', completed: true },
      { label: 'Personal Info', completed: true },
      { label: 'Verification', completed: false },
      { label: 'Confirmation', completed: false },
    ];

    const completedSteps = steps.filter((s) => s.completed).length;
    const progress = (completedSteps / steps.length) * 100;

    return (
      <div className="max-w-lg space-y-4">
        <Progress
          value={progress}
          label="Registration Progress"
          showValue
          customValue={`Step ${completedSteps} of ${steps.length}`}
          variant={progress === 100 ? 'success' : 'default'}
        />

        <div className="grid grid-cols-4 gap-2 mt-6">
          {steps.map((step, index) => (
            <div key={index} className="text-center">
              <div
                className={`w-8 h-8 mx-auto rounded-full flex items-center justify-center text-sm font-medium ${
                  step.completed
                    ? 'bg-success-primary text-white'
                    : 'bg-neutral-200 text-neutral-500'
                }`}
              >
                {index + 1}
              </div>
              <div className="mt-2 text-xs text-neutral-600">{step.label}</div>
            </div>
          ))}
        </div>
      </div>
    );
  },
};

/**
 * Real-world file upload example with animation
 */
export const UploadProgress: Story = {
  render: () => {
    const [progress, setProgress] = useState(0);
    const [uploading, setUploading] = useState(false);

    const startUpload = () => {
      setProgress(0);
      setUploading(true);
    };

    useEffect(() => {
      if (!uploading) return;

      const interval = setInterval(() => {
        setProgress((prev) => {
          if (prev >= 100) {
            clearInterval(interval);
            setUploading(false);
            return 100;
          }
          return prev + Math.random() * 15;
        });
      }, 300);

      return () => clearInterval(interval);
    }, [uploading]);

    const variant = progress === 100 ? 'success' : 'default';

    return (
      <div className="max-w-lg space-y-4">
        <div className="border border-neutral-200 rounded-lg p-6 bg-white">
          <h3 className="text-sm font-medium text-neutral-900 mb-4">
            Upload Document
          </h3>

          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 bg-neutral-100 rounded flex items-center justify-center">
                  <svg
                    className="w-5 h-5 text-neutral-600"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"
                    />
                  </svg>
                </div>
                <div>
                  <div className="text-sm font-medium text-neutral-900">
                    document.pdf
                  </div>
                  <div className="text-xs text-neutral-500">2.4 MB</div>
                </div>
              </div>

              {progress === 100 && (
                <span className="text-sm font-medium text-success-primary">
                  Complete
                </span>
              )}
            </div>

            <Progress
              value={progress}
              variant={variant}
              showValue
              striped={uploading}
            />
          </div>

          {!uploading && progress < 100 && (
            <button
              onClick={startUpload}
              className="mt-4 w-full px-4 py-2 bg-trust-hover text-white rounded-lg hover:bg-trust transition-colors text-sm font-medium"
            >
              Start Upload
            </button>
          )}

          {progress === 100 && (
            <button
              onClick={startUpload}
              className="mt-4 w-full px-4 py-2 border border-neutral-300 text-neutral-700 rounded-lg hover:bg-neutral-50 transition-colors text-sm font-medium"
            >
              Upload Another
            </button>
          )}
        </div>
      </div>
    );
  },
};

/**
 * Animated loading states showing value changes
 */
export const LoadingStates: Story = {
  render: () => {
    const [progress1, setProgress1] = useState(0);
    const [progress2, setProgress2] = useState(0);
    const [progress3, setProgress3] = useState(0);

    useEffect(() => {
      const interval1 = setInterval(() => {
        setProgress1((prev) => (prev >= 100 ? 0 : prev + 10));
      }, 500);

      const interval2 = setInterval(() => {
        setProgress2((prev) => (prev >= 100 ? 0 : prev + 5));
      }, 300);

      const interval3 = setInterval(() => {
        setProgress3((prev) => (prev >= 100 ? 0 : prev + 8));
      }, 400);

      return () => {
        clearInterval(interval1);
        clearInterval(interval2);
        clearInterval(interval3);
      };
    }, []);

    return (
      <div className="space-y-8 max-w-lg">
        <div>
          <h3 className="text-sm font-medium text-neutral-700 mb-3">
            Linear Animation
          </h3>
          <Progress
            value={progress1}
            label="Processing data"
            showValue
            variant="default"
          />
        </div>

        <div>
          <h3 className="text-sm font-medium text-neutral-700 mb-3">
            Striped Animation
          </h3>
          <Progress
            value={progress2}
            label="Uploading files"
            showValue
            variant="success"
            striped
          />
        </div>

        <div className="flex items-center gap-6">
          <div>
            <h3 className="text-sm font-medium text-neutral-700 mb-3">
              Circular Animation
            </h3>
            <Progress
              value={progress3}
              type="circular"
              showValue
              variant="warning"
            />
          </div>
        </div>
      </div>
    );
  },
};

/**
 * Task completion real-world example
 */
export const TaskCompletion: Story = {
  render: () => {
    const [tasks, setTasks] = useState([
      { id: 1, name: 'Initialize project', completed: true },
      { id: 2, name: 'Install dependencies', completed: true },
      { id: 3, name: 'Configure database', completed: true },
      { id: 4, name: 'Setup authentication', completed: false },
      { id: 5, name: 'Create API routes', completed: false },
      { id: 6, name: 'Write tests', completed: false },
      { id: 7, name: 'Deploy application', completed: false },
    ]);

    const completedCount = tasks.filter((t) => t.completed).length;
    const totalCount = tasks.length;
    const progress = (completedCount / totalCount) * 100;

    const toggleTask = (id: number) => {
      setTasks((prev) =>
        prev.map((task) =>
          task.id === id ? { ...task, completed: !task.completed } : task,
        ),
      );
    };

    return (
      <div className="max-w-lg space-y-4">
        <div className="border border-neutral-200 rounded-lg p-6 bg-white">
          <h3 className="text-sm font-medium text-neutral-900 mb-4">
            Project Setup Checklist
          </h3>

          <Progress
            value={progress}
            label="Overall Progress"
            showValue
            customValue={`${completedCount}/${totalCount} tasks`}
            variant={progress === 100 ? 'success' : 'default'}
            striped={progress > 0 && progress < 100}
          />

          <div className="mt-6 space-y-2">
            {tasks.map((task) => (
              <label
                key={task.id}
                className="flex items-center gap-3 p-3 rounded-lg hover:bg-neutral-50 cursor-pointer transition-colors"
              >
                <input
                  type="checkbox"
                  checked={task.completed}
                  onChange={() => toggleTask(task.id)}
                  className="w-4 h-4 text-trust-hover border-neutral-300 rounded focus:ring-trust"
                />
                <span
                  className={`text-sm ${
                    task.completed
                      ? 'text-neutral-400 line-through'
                      : 'text-neutral-700'
                  }`}
                >
                  {task.name}
                </span>
              </label>
            ))}
          </div>
        </div>
      </div>
    );
  },
};

/**
 * All variants at 100% completion
 */
export const Variants100Percent: Story = {
  render: () => (
    <div className="space-y-8 max-w-lg">
      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-4">
          Linear - All Variants at 100%
        </h3>
        <div className="space-y-4">
          <Progress value={100} variant="default" label="Default" showValue />
          <Progress value={100} variant="success" label="Success" showValue />
          <Progress value={100} variant="warning" label="Warning" showValue />
          <Progress value={100} variant="error" label="Error" showValue />
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-4">
          Circular - All Variants at 100%
        </h3>
        <div className="flex items-end gap-6">
          <Progress
            value={100}
            type="circular"
            variant="default"
            label="Default"
            showValue
          />
          <Progress
            value={100}
            type="circular"
            variant="success"
            label="Success"
            showValue
          />
          <Progress
            value={100}
            type="circular"
            variant="warning"
            label="Warning"
            showValue
          />
          <Progress
            value={100}
            type="circular"
            variant="error"
            label="Error"
            showValue
          />
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-700 mb-4">
          Circular Sizes at 100%
        </h3>
        <div className="flex items-end gap-6">
          <Progress
            value={100}
            type="circular"
            size="sm"
            label="Small"
            showValue
          />
          <Progress
            value={100}
            type="circular"
            size="md"
            label="Medium"
            showValue
          />
          <Progress
            value={100}
            type="circular"
            size="lg"
            label="Large"
            showValue
          />
        </div>
      </div>
    </div>
  ),
};

/**
 * Playground story for interactive testing
 */
export const Playground: Story = {
  args: {
    value: 65,
    variant: 'default',
    size: 'md',
    type: 'linear',
    label: 'Progress',
    showValue: true,
    valueFormat: 'percentage',
    indeterminate: false,
    striped: false,
  },
};
