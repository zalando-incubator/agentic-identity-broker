import { Children, forwardRef, isValidElement, type ComponentProps, type ReactNode } from 'react';
import { AnimatePresence, motion, useIsPresent } from 'motion/react';
import { consoleMotion, useConsoleReducedMotion } from '@design-system/utils/consoleMotion';
import { cn } from '@design-system/utils/cn';

const AnimatedItem = forwardRef<HTMLDivElement, { children: ReactNode; reduced: boolean }>(function AnimatedItem({ children, reduced }, ref) {
  const present = useIsPresent();
  return <motion.div
    ref={ref}
    className="min-w-0"
    inert={!present}
    aria-hidden={!present || undefined}
    data-exiting={!present || undefined}
    layout={reduced ? false : 'position'}
    initial={false}
    animate={{ opacity: 1 }}
    exit={reduced ? { opacity: 0 } : { opacity: 0, y: -6 }}
    transition={{ duration: consoleMotion.overlay * consoleMotion.exitFactor, ease: consoleMotion.ease }}
  >{children}</motion.div>;
});

export function AnimatedCollection({ children, className, ...props }: ComponentProps<'div'>) {
  const reduced = useConsoleReducedMotion();
  return <div {...props} className={cn('relative', className)}>
    <AnimatePresence initial={false} mode="popLayout">
      {Children.toArray(children).map(child => isValidElement(child) ? <AnimatedItem key={child.key} reduced={reduced}>{child}</AnimatedItem> : child)}
    </AnimatePresence>
  </div>;
}
