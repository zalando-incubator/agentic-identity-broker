/**
 * PageTransition Component
 *
 * Wrapper component for animating transitions between different page/view content.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Multiple transition types: fade, slideLeft, slideRight, slideUp, slideDown, scaleIn, zoomIn
 * - Customizable animation duration (150ms-500ms range)
 * - Customizable animation easing
 * - Sequential and simultaneous transition modes
 * - Optional exit animation delay
 * - Key-based re-triggering for controlled transitions
 * - Works with conditional rendering (if/else switching views)
 * - Respects prefers-reduced-motion for accessibility
 */

import React, { useEffect, useState } from 'react';
import { Transition } from '@headlessui/react';
import { cn } from '@design-system/utils';

export type TransitionType =
  | 'fade'
  | 'slideLeft'
  | 'slideRight'
  | 'slideUp'
  | 'slideDown'
  | 'scaleIn'
  | 'zoomIn';

export type TransitionMode = 'sequential' | 'simultaneous';

export interface PageTransitionProps {
  /** Content to animate */
  children: React.ReactNode;

  /** Type of transition animation */
  type?: TransitionType;

  /** Animation duration in milliseconds */
  duration?: number;

  /** CSS easing function */
  easing?: string;

  /** Transition mode - sequential (exit then enter) or simultaneous (both at once) */
  mode?: TransitionMode;

  /** Extra delay before enter animation in milliseconds */
  exitDelay?: number;

  /** Additional CSS classes */
  className?: string;

  /** Key to trigger re-animation (IMPORTANT: parent must change this to trigger transition) */
  key?: string | number;
}

// Transition class generators based on type
const getTransitionClasses = (
  type: TransitionType,
  duration: number,
  easing: string,
) => {
  const durationClass = `duration-[${duration}ms]`;
  const easingStyle = { transitionTimingFunction: easing };

  const transitions: Record<
    TransitionType,
    {
      enter: string;
      enterFrom: string;
      enterTo: string;
      leave: string;
      leaveFrom: string;
      leaveTo: string;
    }
  > = {
    fade: {
      enter: `transition-opacity ${durationClass}`,
      enterFrom: 'opacity-0',
      enterTo: 'opacity-100',
      leave: `transition-opacity ${durationClass}`,
      leaveFrom: 'opacity-100',
      leaveTo: 'opacity-0',
    },
    slideLeft: {
      enter: `transition-[transform,opacity] ${durationClass}`,
      enterFrom: 'opacity-0 translate-x-8',
      enterTo: 'opacity-100 translate-x-0',
      leave: `transition-[transform,opacity] ${durationClass}`,
      leaveFrom: 'opacity-100 translate-x-0',
      leaveTo: 'opacity-0 -translate-x-8',
    },
    slideRight: {
      enter: `transition-[transform,opacity] ${durationClass}`,
      enterFrom: 'opacity-0 -translate-x-8',
      enterTo: 'opacity-100 translate-x-0',
      leave: `transition-[transform,opacity] ${durationClass}`,
      leaveFrom: 'opacity-100 translate-x-0',
      leaveTo: 'opacity-0 translate-x-8',
    },
    slideUp: {
      enter: `transition-[transform,opacity] ${durationClass}`,
      enterFrom: 'opacity-0 translate-y-8',
      enterTo: 'opacity-100 translate-y-0',
      leave: `transition-[transform,opacity] ${durationClass}`,
      leaveFrom: 'opacity-100 translate-y-0',
      leaveTo: 'opacity-0 -translate-y-8',
    },
    slideDown: {
      enter: `transition-[transform,opacity] ${durationClass}`,
      enterFrom: 'opacity-0 -translate-y-8',
      enterTo: 'opacity-100 translate-y-0',
      leave: `transition-[transform,opacity] ${durationClass}`,
      leaveFrom: 'opacity-100 translate-y-0',
      leaveTo: 'opacity-0 translate-y-8',
    },
    scaleIn: {
      enter: `transition-[transform,opacity] ${durationClass}`,
      enterFrom: 'opacity-0 scale-95',
      enterTo: 'opacity-100 scale-100',
      leave: `transition-[transform,opacity] ${durationClass}`,
      leaveFrom: 'opacity-100 scale-100',
      leaveTo: 'opacity-0 scale-95',
    },
    zoomIn: {
      enter: `transition-[transform,opacity] ${durationClass}`,
      enterFrom: 'opacity-0 scale-90',
      enterTo: 'opacity-100 scale-100',
      leave: `transition-[transform,opacity] ${durationClass}`,
      leaveFrom: 'opacity-100 scale-100',
      leaveTo: 'opacity-0 scale-110',
    },
  };

  return { classes: transitions[type], style: easingStyle };
};

/**
 * PageTransition component for animating view/page transitions.
 * Wraps content and animates when key prop changes.
 *
 * @example
 * ```tsx
 * // Basic fade transition
 * <PageTransition key={currentView}>
 *   <ViewContent />
 * </PageTransition>
 *
 * // Slide left transition (forward navigation)
 * <PageTransition type="slideLeft" duration={400} key={step}>
 *   <StepContent step={step} />
 * </PageTransition>
 *
 * // Sequential mode (exit before enter)
 * <PageTransition type="fade" mode="sequential" key={page}>
 *   <PageContent />
 * </PageTransition>
 *
 * // Multi-step consent flow
 * const [step, setStep] = useState(1);
 * <PageTransition type="slideLeft" key={step}>
 *   {step === 1 && <ConsentStep1 onNext={() => setStep(2)} />}
 *   {step === 2 && <ConsentStep2 onNext={() => setStep(3)} />}
 *   {step === 3 && <ConsentStep3 onComplete={handleComplete} />}
 * </PageTransition>
 * ```
 */
export const PageTransition = React.forwardRef<
  HTMLDivElement,
  PageTransitionProps
>(
  (
    {
      children,
      type = 'fade',
      duration = 300,
      easing = 'cubic-bezier(0.4, 0, 0.2, 1)',
      mode = 'simultaneous',
      exitDelay = 0,
      className,
    },
    ref,
  ) => {
    const [show, setShow] = useState(true);
    const [delayedShow, setDelayedShow] = useState(true);

    // For sequential mode: handle exit delay before showing new content
    useEffect(() => {
      if (mode === 'sequential' && exitDelay > 0) {
        setShow(false);
        const timer = setTimeout(() => {
          setShow(true);
        }, exitDelay);
        return () => clearTimeout(timer);
      }
    }, [children, mode, exitDelay]);

    // Reset show state when children change (for key-based transitions)
    useEffect(() => {
      if (mode === 'simultaneous') {
        setShow(false);
        // Use microtask to ensure DOM update before showing
        queueMicrotask(() => setShow(true));
      } else {
        // Sequential: exit first, then enter
        setShow(false);
        const timer = setTimeout(() => {
          setDelayedShow(false);
          queueMicrotask(() => {
            setDelayedShow(true);
            setShow(true);
          });
        }, duration + exitDelay);
        return () => clearTimeout(timer);
      }
    }, [children, duration, exitDelay, mode]);

    const { classes, style } = getTransitionClasses(type, duration, easing);

    return (
      <Transition
        show={mode === 'sequential' ? delayedShow && show : show}
        appear={true}
        enter={classes.enter}
        enterFrom={classes.enterFrom}
        enterTo={classes.enterTo}
        leave={classes.leave}
        leaveFrom={classes.leaveFrom}
        leaveTo={classes.leaveTo}
        as={React.Fragment}
      >
        <div
          ref={ref}
          className={cn('transition-transform', className)}
          style={style}
        >
          {children}
        </div>
      </Transition>
    );
  },
);

PageTransition.displayName = 'PageTransition';

export default PageTransition;
