/**
 * PageTransition Component Stories
 *
 * Demonstrates all transition types, modes, and real-world usage examples.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { PageTransition } from './PageTransition';

const meta = {
  title: 'Layout/PageTransition',
  component: PageTransition,
  parameters: {
    layout: 'padded',
    docs: {
      description: {
        component:
          'PageTransition wraps content and animates transitions when the key prop changes. ' +
          'Perfect for multi-step forms, tabbed content, route transitions, and modal content switches. ' +
          'Supports multiple animation types, customizable timing, and respects prefers-reduced-motion.',
      },
    },
  },
  tags: ['autodocs'],
} satisfies Meta<typeof PageTransition>;

export default meta;
type Story = StoryObj<typeof meta>;

// Sample content components for demos
const ContentCard = ({
  title,
  description,
  color = 'navy',
}: {
  title: string;
  description: string;
  color?: string;
}) => (
  <div className={`rounded-lg p-8 bg-${color}-50 border border-${color}-200 `}>
    <h3 className="font-display text-2xl font-medium text-trust-deep mb-3">
      {title}
    </h3>
    <p className="text-secondary-700 leading-relaxed">{description}</p>
  </div>
);

// Interactive wrapper for transition demos
const TransitionDemo = ({
  type,
  duration = 300,
  mode = 'simultaneous',
  exitDelay = 0,
  easing = 'cubic-bezier(0.4, 0, 0.2, 1)',
}: {
  type: React.ComponentProps<typeof PageTransition>['type'];
  duration?: number;
  mode?: React.ComponentProps<typeof PageTransition>['mode'];
  exitDelay?: number;
  easing?: string;
}) => {
  const [view, setView] = useState<'view1' | 'view2'>('view1');

  return (
    <div className="space-y-4">
      <div className="flex gap-3">
        <button
          onClick={() => setView('view1')}
          className={`px-4 py-2 rounded-md font-medium transition-colors ${
            view === 'view1'
              ? 'bg-trust-hover text-white'
              : 'bg-neutral-100 text-trust hover:bg-neutral-200'
          }`}
        >
          View 1
        </button>
        <button
          onClick={() => setView('view2')}
          className={`px-4 py-2 rounded-md font-medium transition-colors ${
            view === 'view2'
              ? 'bg-trust-hover text-white'
              : 'bg-neutral-100 text-trust hover:bg-neutral-200'
          }`}
        >
          View 2
        </button>
      </div>

      <PageTransition
        key={view}
        type={type}
        duration={duration}
        mode={mode}
        exitDelay={exitDelay}
        easing={easing}
      >
        {view === 'view1' ? (
          <ContentCard
            title="View 1"
            description="This is the first view. Click 'View 2' to see the transition animation."
            color="navy"
          />
        ) : (
          <ContentCard
            title="View 2"
            description="This is the second view. Click 'View 1' to transition back."
            color="emerald"
          />
        )}
      </PageTransition>
    </div>
  );
};

/**
 * Basic fade transition - the most subtle and universal animation.
 * Content fades out and fades in smoothly.
 */
export const FadeTransition: Story = {
  render: () => <TransitionDemo type="fade" />,
};

/**
 * Slide left transition - simulates forward navigation.
 * Old content slides out left, new content slides in from right.
 * Perfect for multi-step flows and wizards.
 */
export const SlideLeftTransition: Story = {
  render: () => <TransitionDemo type="slideLeft" />,
};

/**
 * Slide right transition - simulates backward navigation.
 * Old content slides out right, new content slides in from left.
 * Ideal for "back" button interactions.
 */
export const SlideRightTransition: Story = {
  render: () => <TransitionDemo type="slideRight" />,
};

/**
 * Slide up transition - content rises from bottom.
 * Useful for progressive disclosure and bottom sheets.
 */
export const SlideUpTransition: Story = {
  render: () => <TransitionDemo type="slideUp" />,
};

/**
 * Slide down transition - content descends from top.
 * Good for notifications and dropdown content.
 */
export const SlideDownTransition: Story = {
  render: () => <TransitionDemo type="slideDown" />,
};

/**
 * Scale in transition - subtle zoom effect.
 * Content scales from 95% to 100% with fade.
 * Creates a smooth, polished appearance.
 */
export const ScaleInTransition: Story = {
  render: () => <TransitionDemo type="scaleIn" />,
};

/**
 * Zoom in transition - dramatic scale effect.
 * Content zooms from 90% to 110% with fade.
 * More energetic than scaleIn, use sparingly.
 */
export const ZoomInTransition: Story = {
  render: () => <TransitionDemo type="zoomIn" />,
};

/**
 * Sequential mode - exit animation completes before enter animation.
 * Content fades out completely, then new content fades in.
 * More deliberate pacing, clearer state changes.
 */
export const SequentialMode: Story = {
  render: () => <TransitionDemo type="fade" mode="sequential" duration={250} />,
};

/**
 * Simultaneous mode (default) - both animations happen at once.
 * Old content fades out while new content fades in.
 * Faster, more fluid transitions.
 */
export const SimultaneousMode: Story = {
  render: () => (
    <TransitionDemo type="fade" mode="simultaneous" duration={300} />
  ),
};

/**
 * Fast duration (150ms) - snappy, responsive feel.
 * Best for simple transitions and frequent interactions.
 */
export const FastDuration: Story = {
  render: () => <TransitionDemo type="slideLeft" duration={150} />,
};

/**
 * Medium duration (300ms) - balanced, default timing.
 * Works well for most use cases.
 */
export const MediumDuration: Story = {
  render: () => <TransitionDemo type="slideLeft" duration={300} />,
};

/**
 * Slow duration (500ms) - deliberate, emphasized transitions.
 * Use for important state changes that deserve attention.
 */
export const SlowDuration: Story = {
  render: () => <TransitionDemo type="slideLeft" duration={500} />,
};

/**
 * Different easing functions demonstrate timing variations.
 */
export const EasingVariants: Story = {
  render: () => {
    const [easing, setEasing] = useState<string>(
      'cubic-bezier(0.4, 0, 0.2, 1)',
    );
    const [view, setView] = useState(0);

    const easings = [
      {
        name: 'Ease (default)',
        value: 'cubic-bezier(0.4, 0, 0.2, 1)',
        desc: 'Smooth, balanced',
      },
      {
        name: 'Ease In',
        value: 'cubic-bezier(0.4, 0, 1, 1)',
        desc: 'Slow start',
      },
      {
        name: 'Ease Out',
        value: 'cubic-bezier(0, 0, 0.2, 1)',
        desc: 'Slow end',
      },
      {
        name: 'Ease In Out',
        value: 'cubic-bezier(0.4, 0, 0.6, 1)',
        desc: 'Slow both ends',
      },
      {
        name: 'Bounce',
        value: 'cubic-bezier(0.68, -0.55, 0.265, 1.55)',
        desc: 'Playful overshoot',
      },
    ];

    return (
      <div className="space-y-4">
        <div className="flex flex-wrap gap-2">
          {easings.map((e, i) => (
            <button
              key={e.name}
              onClick={() => {
                setEasing(e.value);
                setView(i);
              }}
              className={`px-4 py-2 rounded-md font-medium transition-colors ${
                easing === e.value
                  ? 'bg-trust-hover text-white'
                  : 'bg-neutral-100 text-trust hover:bg-neutral-200'
              }`}
            >
              {e.name}
            </button>
          ))}
        </div>

        <PageTransition
          key={view}
          type="slideLeft"
          duration={400}
          easing={easing}
        >
          <ContentCard
            title={easings[view].name}
            description={`${easings[view].desc} - Easing: ${easings[view].value}`}
          />
        </PageTransition>
      </div>
    );
  },
};

/**
 * Real-world example: Multi-step consent flow.
 * Demonstrates how PageTransition works in a practical application.
 * Each step slides in from the right, creating forward progression.
 */
export const ConsentFlowExample: Story = {
  render: () => {
    const [step, setStep] = useState(1);

    const ConsentStep1 = () => (
      <div className="bg-white rounded-lg p-8 shadow-float border border-slate">
        <div className="space-y-6">
          <div>
            <div className="inline-flex items-center justify-center w-12 h-12 rounded-full bg-trust-light text-trust font-display font-medium text-xl mb-4">
              1
            </div>
            <h2 className="font-display text-3xl font-medium text-trust-deep">
              Welcome to Identity Broker
            </h2>
            <p className="text-secondary-600 mt-2">Step 1 of 3: Introduction</p>
          </div>

          <div className="space-y-3">
            <p className="text-secondary-700 leading-relaxed">
              Welcome! Identity Broker helps you securely manage access to your
              applications. We need your consent to proceed with authentication.
            </p>
            <p className="text-secondary-700 leading-relaxed">
              In the next steps, you'll review the permissions being requested
              and decide whether to grant access.
            </p>
          </div>

          <div className="flex gap-3 pt-4">
            <button
              onClick={() => setStep(2)}
              className="px-6 py-3 bg-trust-hover text-white rounded-md font-medium hover:bg-trust transition-colors"
            >
              Continue
            </button>
          </div>
        </div>
      </div>
    );

    const ConsentStep2 = () => (
      <div className="bg-white rounded-lg p-8 shadow-float border border-slate">
        <div className="space-y-6">
          <div>
            <div className="inline-flex items-center justify-center w-12 h-12 rounded-full bg-trust-light text-trust font-display font-medium text-xl mb-4">
              2
            </div>
            <h2 className="font-display text-3xl font-medium text-trust-deep">
              Review Permissions
            </h2>
            <p className="text-secondary-600 mt-2">
              Step 2 of 3: Requested Access
            </p>
          </div>

          <div className="space-y-4">
            <div className="flex items-start gap-3 p-4 rounded-lg bg-success-light border border-success-light">
              <svg
                className="w-5 h-5 text-success-primary mt-0.5 flex-shrink-0"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fillRule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clipRule="evenodd"
                />
              </svg>
              <div>
                <h4 className="font-medium text-success-dark">
                  Read your profile
                </h4>
                <p className="text-sm text-success-hover">
                  Access your basic profile information
                </p>
              </div>
            </div>

            <div className="flex items-start gap-3 p-4 rounded-lg bg-success-light border border-success-light">
              <svg
                className="w-5 h-5 text-success-primary mt-0.5 flex-shrink-0"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fillRule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clipRule="evenodd"
                />
              </svg>
              <div>
                <h4 className="font-medium text-success-dark">
                  Manage your sessions
                </h4>
                <p className="text-sm text-success-hover">
                  Create and manage authentication sessions
                </p>
              </div>
            </div>
          </div>

          <div className="flex gap-3 pt-4">
            <button
              onClick={() => setStep(1)}
              className="px-6 py-3 bg-neutral-100 text-trust rounded-md font-medium hover:bg-neutral-200 transition-colors"
            >
              Back
            </button>
            <button
              onClick={() => setStep(3)}
              className="px-6 py-3 bg-trust-hover text-white rounded-md font-medium hover:bg-trust transition-colors"
            >
              Continue
            </button>
          </div>
        </div>
      </div>
    );

    const ConsentStep3 = () => (
      <div className="bg-white rounded-lg p-8 shadow-float border border-slate">
        <div className="space-y-6">
          <div>
            <div className="inline-flex items-center justify-center w-12 h-12 rounded-full bg-success-light text-success-hover font-display font-medium text-xl mb-4">
              3
            </div>
            <h2 className="font-display text-3xl font-medium text-trust-deep">
              Grant Consent
            </h2>
            <p className="text-secondary-600 mt-2">
              Step 3 of 3: Final confirmation
            </p>
          </div>

          <div className="space-y-4">
            <div className="p-4 rounded-lg bg-amber-50 border border-amber-200">
              <p className="text-sm text-amber-900 leading-relaxed">
                <strong>Important:</strong> By granting consent, you allow the
                application to access your profile and manage sessions on your
                behalf. You can revoke this access at any time from your account
                settings.
              </p>
            </div>

            <div className="flex items-start gap-3">
              <input
                type="checkbox"
                id="terms"
                className="mt-1 w-4 h-4 text-trust-hover rounded border-secondary-300 focus:ring-trust"
              />
              <label htmlFor="terms" className="text-sm text-secondary-700">
                I have read and understood the requested permissions and agree
                to grant access to this application.
              </label>
            </div>
          </div>

          <div className="flex gap-3 pt-4">
            <button
              onClick={() => setStep(2)}
              className="px-6 py-3 bg-neutral-100 text-trust rounded-md font-medium hover:bg-neutral-200 transition-colors"
            >
              Back
            </button>
            <button
              onClick={() => setStep(1)}
              className="px-6 py-3 bg-success-primary text-white rounded-md font-medium hover:bg-success-hover transition-colors"
            >
              Grant Consent
            </button>
          </div>
        </div>
      </div>
    );

    return (
      <div className="max-w-2xl mx-auto">
        <PageTransition key={step} type="slideLeft" duration={350}>
          {step === 1 && <ConsentStep1 />}
          {step === 2 && <ConsentStep2 />}
          {step === 3 && <ConsentStep3 />}
        </PageTransition>

        <div className="flex justify-center gap-2 mt-6">
          {[1, 2, 3].map((s) => (
            <button
              key={s}
              onClick={() => setStep(s)}
              className={`w-3 h-3 rounded-full transition-colors ${
                step === s ? 'bg-trust-hover' : 'bg-slate'
              }`}
              aria-label={`Go to step ${s}`}
            />
          ))}
        </div>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story:
          'A complete multi-step consent flow showing how PageTransition creates smooth navigation between steps. ' +
          'Each step slides in from the right, providing clear visual feedback of forward progression. ' +
          'The step indicators at the bottom allow quick navigation while maintaining smooth transitions.',
      },
    },
  },
};

/**
 * With exit delay - adds pause between exit and enter animations.
 * Useful when you want more distinct separation between views.
 */
export const WithExitDelay: Story = {
  render: () => (
    <TransitionDemo type="fade" mode="sequential" exitDelay={200} />
  ),
  parameters: {
    docs: {
      description: {
        story:
          'Adds a 200ms delay between the exit and enter animations in sequential mode. ' +
          'This creates a more pronounced pause, making state changes more noticeable.',
      },
    },
  },
};

/**
 * Accessibility: Respects prefers-reduced-motion.
 * When users have motion preferences set, transitions are instant.
 */
export const AccessibilityDemo: Story = {
  render: () => (
    <div className="space-y-4">
      <div className="p-4 rounded-lg bg-amber-50 border border-amber-200">
        <p className="text-sm text-amber-900">
          <strong>Accessibility Note:</strong> This component respects the
          prefers-reduced-motion media query. Users who have enabled reduced
          motion in their OS settings will see instant transitions without
          animation.
        </p>
        <p className="text-sm text-amber-900 mt-2">
          To test: Enable "Reduce motion" in your system accessibility settings.
        </p>
      </div>
      <TransitionDemo type="slideLeft" />
    </div>
  ),
};
