/**
 * Modal Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Modal } from './Modal';
import { Button } from '@design-system/components/primitives/Button';

const meta = {
  title: 'Design System/Overlays/Modal',
  component: Modal,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    isOpen: {
      control: 'boolean',
      description: 'Whether the modal is open',
    },
    title: {
      control: 'text',
      description: 'Optional modal title',
    },
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
      description: 'Modal size variant',
    },
    scrollable: {
      control: 'boolean',
      description: 'Enable scrollable content area',
    },
    closeOnBackdropClick: {
      control: 'boolean',
      description: 'Whether clicking backdrop closes the modal',
    },
    children: {
      control: 'text',
      description: 'Modal content',
    },
  },
} satisfies Meta<typeof Modal>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default modal with title and basic content
 */
export const Default: Story = {
  render: (args) => {
    const [isOpen, setIsOpen] = useState(false);

    return (
      <>
        <Button onClick={() => setIsOpen(true)}>Open Modal</Button>
        <Modal {...args} isOpen={isOpen} onClose={() => setIsOpen(false)}>
          <p className="text-neutral-700">
            This is a basic modal with a title and content. Click the X button
            or press ESC to close.
          </p>
        </Modal>
      </>
    );
  },
  args: {
    title: 'Basic Modal',
    size: 'md',
    closeOnBackdropClick: true,
    scrollable: false,
  },
};

/**
 * Modal with footer actions (primary use case)
 */
export const WithFooterActions: Story = {
  render: () => {
    const [isOpen, setIsOpen] = useState(false);

    return (
      <>
        <Button onClick={() => setIsOpen(true)}>Open Modal with Actions</Button>
        <Modal
          isOpen={isOpen}
          onClose={() => setIsOpen(false)}
          title="Confirm Your Action"
          footer={
            <div className="flex gap-3 justify-end">
              <Button variant="outline" onClick={() => setIsOpen(false)}>
                Cancel
              </Button>
              <Button
                variant="primary"
                onClick={() => {
                  alert('Action confirmed!');
                  setIsOpen(false);
                }}
              >
                Confirm
              </Button>
            </div>
          }
        >
          <p className="text-neutral-700">
            Modals with footer actions are the most common use case. The footer
            is styled with a gray background to separate it from the main
            content.
          </p>
          <p className="text-neutral-600 mt-3 text-sm">
            Primary actions should be on the right, with cancel/secondary
            actions on the left.
          </p>
        </Modal>
      </>
    );
  },
  args: {
    title: 'Modal with Actions',
    size: 'md',
  },
};

/**
 * Different modal sizes: small, medium, large
 */
export const Sizes: Story = {
  render: () => {
    const [openModal, setOpenModal] = useState<'sm' | 'md' | 'lg' | null>(null);

    return (
      <div className="flex gap-3">
        <Button onClick={() => setOpenModal('sm')}>Small Modal</Button>
        <Button onClick={() => setOpenModal('md')}>Medium Modal</Button>
        <Button onClick={() => setOpenModal('lg')}>Large Modal</Button>

        <Modal
          isOpen={openModal === 'sm'}
          onClose={() => setOpenModal(null)}
          title="Small Modal"
          size="sm"
          footer={
            <div className="flex justify-end">
              <Button onClick={() => setOpenModal(null)}>Close</Button>
            </div>
          }
        >
          <p className="text-neutral-700">
            Small modals (400px max-width) are perfect for simple confirmations
            and short messages.
          </p>
        </Modal>

        <Modal
          isOpen={openModal === 'md'}
          onClose={() => setOpenModal(null)}
          title="Medium Modal"
          size="md"
          footer={
            <div className="flex justify-end">
              <Button onClick={() => setOpenModal(null)}>Close</Button>
            </div>
          }
        >
          <p className="text-neutral-700">
            Medium modals (600px max-width) are the default size, suitable for
            most forms and content.
          </p>
        </Modal>

        <Modal
          isOpen={openModal === 'lg'}
          onClose={() => setOpenModal(null)}
          title="Large Modal"
          size="lg"
          footer={
            <div className="flex justify-end">
              <Button onClick={() => setOpenModal(null)}>Close</Button>
            </div>
          }
        >
          <p className="text-neutral-700">
            Large modals (800px max-width) are ideal for complex forms, detailed
            content, or data tables.
          </p>
          <div className="mt-4 p-4 bg-neutral-50 rounded-lg border border-neutral-200">
            <h4 className="font-medium text-neutral-900 mb-2">
              Additional Content
            </h4>
            <p className="text-neutral-600 text-sm">
              Large modals provide more space for complex layouts and multiple
              sections.
            </p>
          </div>
        </Modal>
      </div>
    );
  },
  args: {
    title: 'Size Variants',
    size: 'md',
  },
};

/**
 * Scrollable content for long modal bodies
 */
export const ScrollableContent: Story = {
  render: () => {
    const [isOpen, setIsOpen] = useState(false);

    const longContent = Array.from({ length: 20 }, (_, i) => (
      <p key={i} className="text-neutral-700 mb-3">
        This is paragraph {i + 1} of long content. When content exceeds the
        viewport height, the modal body becomes scrollable while the header and
        footer remain fixed. This ensures important actions are always visible.
      </p>
    ));

    return (
      <>
        <Button onClick={() => setIsOpen(true)}>Open Scrollable Modal</Button>
        <Modal
          isOpen={isOpen}
          onClose={() => setIsOpen(false)}
          title="Terms and Conditions"
          size="md"
          scrollable
          footer={
            <div className="flex gap-3 justify-end">
              <Button variant="outline" onClick={() => setIsOpen(false)}>
                Decline
              </Button>
              <Button
                variant="primary"
                onClick={() => {
                  alert('Terms accepted!');
                  setIsOpen(false);
                }}
              >
                Accept Terms
              </Button>
            </div>
          }
        >
          <div className="prose prose-sm max-w-none">
            <p className="text-neutral-700 font-medium mb-4">
              Please read and accept the following terms and conditions to
              continue.
            </p>
            {longContent}
          </div>
        </Modal>
      </>
    );
  },
  args: {
    title: 'Scrollable Modal',
    scrollable: true,
  },
};

/**
 * Centered modal layout (default behavior)
 */
export const CenteredLayout: Story = {
  render: () => {
    const [isOpen, setIsOpen] = useState(false);

    return (
      <>
        <Button onClick={() => setIsOpen(true)}>Open Centered Modal</Button>
        <Modal
          isOpen={isOpen}
          onClose={() => setIsOpen(false)}
          title="Centered Modal"
          size="md"
        >
          <div className="text-center py-6">
            <div className="w-16 h-16 bg-trust-light rounded-full flex items-center justify-center mx-auto mb-4">
              <svg
                className="w-8 h-8 text-trust-deep"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <h3 className="text-lg font-medium text-neutral-900 mb-2">
              Action Successful
            </h3>
            <p className="text-neutral-600">
              Your changes have been saved successfully. All updates are now
              live.
            </p>
          </div>
        </Modal>
      </>
    );
  },
  args: {
    title: 'Centered Content',
    size: 'sm',
  },
};

/**
 * Confirmation modal pattern (common use case)
 */
export const Confirmation: Story = {
  render: () => {
    const [isOpen, setIsOpen] = useState(false);
    const [isDeleting, setIsDeleting] = useState(false);

    const handleDelete = () => {
      setIsDeleting(true);
      // Simulate async action
      setTimeout(() => {
        setIsDeleting(false);
        setIsOpen(false);
        alert('Item deleted successfully');
      }, 1500);
    };

    return (
      <>
        <Button variant="danger" onClick={() => setIsOpen(true)}>
          Delete Item
        </Button>
        <Modal
          isOpen={isOpen}
          onClose={() => setIsOpen(false)}
          title="Confirm Deletion"
          size="sm"
          closeOnBackdropClick={false}
          icon={
            <svg
              className="w-6 h-6 text-error-primary"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
              />
            </svg>
          }
          footer={
            <div className="flex gap-3 justify-end">
              <Button
                variant="outline"
                onClick={() => setIsOpen(false)}
                disabled={isDeleting}
              >
                Cancel
              </Button>
              <Button
                variant="danger"
                onClick={handleDelete}
                isLoading={isDeleting}
              >
                Delete
              </Button>
            </div>
          }
        >
          <p className="text-neutral-700">
            Are you sure you want to delete this item? This action cannot be
            undone.
          </p>
          <div className="mt-4 p-3 bg-error-light border border-error-primary/20 rounded-md">
            <p className="text-sm text-error-dark font-medium">
              Warning: This is a destructive action
            </p>
          </div>
        </Modal>
      </>
    );
  },
  args: {
    title: 'Confirmation',
    size: 'sm',
  },
};

/**
 * Form modal with input fields
 */
export const FormModal: Story = {
  render: () => {
    const [isOpen, setIsOpen] = useState(false);
    const [isSubmitting, setIsSubmitting] = useState(false);

    const handleSubmit = (e: React.FormEvent) => {
      e.preventDefault();
      setIsSubmitting(true);
      // Simulate async submission
      setTimeout(() => {
        setIsSubmitting(false);
        setIsOpen(false);
        alert('Form submitted successfully!');
      }, 1500);
    };

    return (
      <>
        <Button onClick={() => setIsOpen(true)}>Create New User</Button>
        <Modal
          isOpen={isOpen}
          onClose={() => setIsOpen(false)}
          title="Create New User"
          size="md"
          footer={
            <div className="flex gap-3 justify-end">
              <Button
                variant="outline"
                onClick={() => setIsOpen(false)}
                disabled={isSubmitting}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                type="submit"
                form="user-form"
                isLoading={isSubmitting}
              >
                Create User
              </Button>
            </div>
          }
        >
          <form id="user-form" onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label
                htmlFor="name"
                className="block text-sm font-medium text-neutral-700 mb-1"
              >
                Full Name
              </label>
              <input
                type="text"
                id="name"
                name="name"
                required
                className="w-full px-3 py-2 border border-neutral-300 rounded-md focus:outline-none focus:ring-2 focus:ring-trust-deep focus:border-transparent"
                placeholder="John Doe"
              />
            </div>

            <div>
              <label
                htmlFor="email"
                className="block text-sm font-medium text-neutral-700 mb-1"
              >
                Email Address
              </label>
              <input
                type="email"
                id="email"
                name="email"
                required
                className="w-full px-3 py-2 border border-neutral-300 rounded-md focus:outline-none focus:ring-2 focus:ring-trust-deep focus:border-transparent"
                placeholder="john.doe@example.com"
              />
            </div>

            <div>
              <label
                htmlFor="role"
                className="block text-sm font-medium text-neutral-700 mb-1"
              >
                Role
              </label>
              <select
                id="role"
                name="role"
                required
                className="w-full px-3 py-2 border border-neutral-300 rounded-md focus:outline-none focus:ring-2 focus:ring-trust-deep focus:border-transparent"
              >
                <option value="">Select a role...</option>
                <option value="admin">Administrator</option>
                <option value="user">Standard User</option>
                <option value="viewer">Viewer</option>
              </select>
            </div>

            <div>
              <label
                htmlFor="bio"
                className="block text-sm font-medium text-neutral-700 mb-1"
              >
                Bio (Optional)
              </label>
              <textarea
                id="bio"
                name="bio"
                rows={3}
                className="w-full px-3 py-2 border border-neutral-300 rounded-md focus:outline-none focus:ring-2 focus:ring-trust-deep focus:border-transparent resize-none"
                placeholder="Tell us about yourself..."
              />
            </div>
          </form>
        </Modal>
      </>
    );
  },
  args: {
    title: 'Form Modal',
    size: 'md',
  },
};

/**
 * Backdrop interaction behavior
 */
export const BackdropInteraction: Story = {
  render: () => {
    const [dismissible, setDismissible] = useState(false);
    const [nonDismissible, setNonDismissible] = useState(false);

    return (
      <div className="flex gap-3">
        <Button onClick={() => setDismissible(true)}>
          Dismissible on Backdrop Click
        </Button>
        <Button onClick={() => setNonDismissible(true)}>
          Non-Dismissible on Backdrop Click
        </Button>

        <Modal
          isOpen={dismissible}
          onClose={() => setDismissible(false)}
          title="Dismissible Modal"
          size="sm"
          closeOnBackdropClick={true}
        >
          <p className="text-neutral-700 mb-3">
            This modal can be dismissed by clicking the backdrop (dark area
            outside the modal).
          </p>
          <p className="text-sm text-neutral-600">
            Try clicking outside this box or pressing ESC.
          </p>
        </Modal>

        <Modal
          isOpen={nonDismissible}
          onClose={() => setNonDismissible(false)}
          title="Non-Dismissible Modal"
          size="sm"
          closeOnBackdropClick={false}
          footer={
            <div className="flex justify-end">
              <Button onClick={() => setNonDismissible(false)}>Close</Button>
            </div>
          }
        >
          <p className="text-neutral-700 mb-3">
            This modal cannot be dismissed by clicking the backdrop. You must
            use the close button or press ESC.
          </p>
          <div className="p-3 bg-warning-light border border-warning-primary/20 rounded-md">
            <p className="text-sm text-warning-dark">
              Use this for critical actions that require explicit user
              acknowledgment.
            </p>
          </div>
        </Modal>
      </div>
    );
  },
  args: {
    title: 'Backdrop Interaction',
    closeOnBackdropClick: true,
  },
};

/**
 * Interactive playground with all controls
 */
export const Playground: Story = {
  render: (args) => {
    const [isOpen, setIsOpen] = useState(false);

    return (
      <>
        <Button onClick={() => setIsOpen(true)}>Open Playground Modal</Button>
        <Modal
          {...args}
          isOpen={isOpen}
          onClose={() => setIsOpen(false)}
          footer={
            args.footer || (
              <div className="flex gap-3 justify-end">
                <Button variant="outline" onClick={() => setIsOpen(false)}>
                  Cancel
                </Button>
                <Button variant="primary" onClick={() => setIsOpen(false)}>
                  Confirm
                </Button>
              </div>
            )
          }
        >
          {args.children || (
            <div>
              <p className="text-neutral-700 mb-3">
                Use the controls below to customize this modal's appearance and
                behavior.
              </p>
              <ul className="list-disc list-inside text-sm text-neutral-600 space-y-1">
                <li>Change the size (sm, md, lg)</li>
                <li>Toggle scrollable content</li>
                <li>Control backdrop click behavior</li>
                <li>Add or remove the title</li>
              </ul>
            </div>
          )}
        </Modal>
      </>
    );
  },
  args: {
    title: 'Playground Modal',
    size: 'md',
    scrollable: false,
    closeOnBackdropClick: true,
  },
};

/**
 * Real-world consent management examples
 */
export const ConsentManagementExamples: Story = {
  render: () => {
    const [grantModal, setGrantModal] = useState(false);
    const [revokeModal, setRevokeModal] = useState(false);
    const [detailsModal, setDetailsModal] = useState(false);

    return (
      <div className="flex flex-col gap-3">
        <Button onClick={() => setGrantModal(true)}>
          Grant Consent Request
        </Button>
        <Button variant="danger" onClick={() => setRevokeModal(true)}>
          Revoke Consent
        </Button>
        <Button variant="outline" onClick={() => setDetailsModal(true)}>
          View Consent Details
        </Button>

        {/* Grant Consent Modal */}
        <Modal
          isOpen={grantModal}
          onClose={() => setGrantModal(false)}
          title="Grant Consent"
          size="md"
          icon={
            <svg
              className="w-6 h-6 text-trust-deep"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
              />
            </svg>
          }
          footer={
            <div className="flex gap-3 justify-end">
              <Button variant="outline" onClick={() => setGrantModal(false)}>
                Cancel
              </Button>
              <Button
                variant="primary"
                onClick={() => {
                  alert('Consent granted!');
                  setGrantModal(false);
                }}
              >
                Grant Access
              </Button>
            </div>
          }
        >
          <div className="space-y-4">
            <p className="text-neutral-700">
              <strong>Analytics Dashboard</strong> is requesting access to the
              following data:
            </p>
            <ul className="list-disc list-inside text-neutral-700 space-y-2 pl-2">
              <li>Basic profile information (name, email)</li>
              <li>Usage statistics and activity logs</li>
              <li>Preference settings</li>
            </ul>
            <div className="p-4 bg-info-light border border-info-primary/20 rounded-md">
              <p className="text-sm text-info-dark">
                This permission will be valid for 30 days and can be revoked at
                any time.
              </p>
            </div>
          </div>
        </Modal>

        {/* Revoke Consent Modal */}
        <Modal
          isOpen={revokeModal}
          onClose={() => setRevokeModal(false)}
          title="Revoke Consent"
          size="sm"
          closeOnBackdropClick={false}
          icon={
            <svg
              className="w-6 h-6 text-error-primary"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
              />
            </svg>
          }
          footer={
            <div className="flex gap-3 justify-end">
              <Button variant="outline" onClick={() => setRevokeModal(false)}>
                Keep Consent
              </Button>
              <Button
                variant="danger"
                onClick={() => {
                  alert('Consent revoked');
                  setRevokeModal(false);
                }}
              >
                Revoke Access
              </Button>
            </div>
          }
        >
          <p className="text-neutral-700 mb-3">
            Are you sure you want to revoke consent for{' '}
            <strong>Marketing Platform</strong>?
          </p>
          <p className="text-sm text-neutral-600">
            This application will immediately lose access to your data and may
            stop functioning properly.
          </p>
        </Modal>

        {/* Consent Details Modal */}
        <Modal
          isOpen={detailsModal}
          onClose={() => setDetailsModal(false)}
          title="Consent Details"
          size="lg"
          scrollable
          footer={
            <div className="flex justify-end">
              <Button onClick={() => setDetailsModal(false)}>Close</Button>
            </div>
          }
        >
          <div className="space-y-6">
            <div>
              <h4 className="text-sm font-medium text-neutral-900 mb-2">
                Application
              </h4>
              <p className="text-neutral-700">Analytics Dashboard v2.1</p>
            </div>

            <div>
              <h4 className="text-sm font-medium text-neutral-900 mb-2">
                Granted Permissions
              </h4>
              <ul className="list-disc list-inside text-neutral-700 space-y-1">
                <li>Read basic profile information</li>
                <li>Access usage statistics</li>
                <li>View activity logs (last 90 days)</li>
              </ul>
            </div>

            <div>
              <h4 className="text-sm font-medium text-neutral-900 mb-2">
                Consent Timeline
              </h4>
              <div className="space-y-3">
                <div className="flex items-start gap-3 p-3 bg-neutral-50 rounded-md">
                  <div className="text-xs text-neutral-500 mt-0.5">
                    Dec 1, 2025
                  </div>
                  <div className="flex-1">
                    <p className="text-sm text-neutral-900 font-medium">
                      Consent Granted
                    </p>
                    <p className="text-xs text-neutral-600">
                      Initial access granted for 30 days
                    </p>
                  </div>
                </div>
                <div className="flex items-start gap-3 p-3 bg-neutral-50 rounded-md">
                  <div className="text-xs text-neutral-500 mt-0.5">
                    Dec 15, 2025
                  </div>
                  <div className="flex-1">
                    <p className="text-sm text-neutral-900 font-medium">
                      Permissions Updated
                    </p>
                    <p className="text-xs text-neutral-600">
                      Added access to activity logs
                    </p>
                  </div>
                </div>
              </div>
            </div>

            <div className="p-4 bg-warning-light border border-warning-primary/20 rounded-md">
              <p className="text-sm text-warning-dark font-medium mb-1">
                Expires in 15 days
              </p>
              <p className="text-xs text-neutral-700">
                This consent will expire on December 31, 2025. You will need to
                renew access after this date.
              </p>
            </div>
          </div>
        </Modal>
      </div>
    );
  },
  args: {
    title: 'Consent Management',
  },
};
