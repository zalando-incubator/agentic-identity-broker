// Adapted from lucide-animated.com (bot, connect, shield-check); MIT, see LICENSE.
import { useCallback, useEffect, useRef } from 'react';
import { motion, useAnimation } from 'motion/react';
import { consoleMotion, useConsoleReducedMotion } from '@design-system/utils/consoleMotion';

interface AnimatedIconProps {
  size?: number;
  className?: string;
  playKey?: number;
}

function useAnimatedIcon(playKey?: number) {
  const ref = useRef<SVGSVGElement>(null);
  const controls = useAnimation();
  const reduced = useConsoleReducedMotion();
  const running = useRef<Promise<void> | null>(null);
  const start = useCallback(() => {
    if (running.current) return;
    const cycle = controls.start('animate');
    running.current = cycle;
    void cycle.then(() => {
      if (running.current === cycle) running.current = null;
    });
  }, [controls]);
  useEffect(() => {
    if (reduced) {
      controls.stop();
      controls.set('normal');
      running.current = null;
      return;
    }
    const target = ref.current?.closest('[data-animated-icon-host]') ?? ref.current;
    if (!target) return;
    target.addEventListener('pointerenter', start);
    const entryFrame = target.hasAttribute('data-empty-state-illustration') && playKey === undefined ? requestAnimationFrame(start) : undefined;
    target.addEventListener('focusin', start);
    return () => {
      target.removeEventListener('pointerenter', start);
      target.removeEventListener('focusin', start);
      if (entryFrame !== undefined) cancelAnimationFrame(entryFrame);
      controls.stop();
      running.current = null;
    };
  }, [controls, reduced, start, playKey]);
  useEffect(() => {
    if (playKey !== undefined && !reduced) start();
  }, [playKey, reduced, start]);
  return { ref, controls };
}

export function AnimatedBotIcon({ size = 20, className, playKey }: AnimatedIconProps) {
  const { ref, controls } = useAnimatedIcon(playKey);
  return <svg ref={ref} aria-hidden="true" width={size} height={size} className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={size >= 20 ? 1.5 : 1.75} strokeLinecap="round" strokeLinejoin="round">
    <path d="M12 8V4H8" /><rect height="12" rx="2" width="16" x="4" y="8" /><path d="M2 14h2M20 14h2" />
    {[9, 15].map(x => <motion.line key={x} x1={x} x2={x} initial="normal" animate={controls}
      variants={{ normal: { y1: 13, y2: 15 }, animate: { y1: [13, 14, 13], y2: [15, 14, 15] } }}
      transition={{ duration: consoleMotion.emphasis, ease: consoleMotion.ease }} />)}
  </svg>;
}

export function AnimatedConnectionIcon({ size = 20, className, playKey }: AnimatedIconProps) {
  const { ref, controls } = useAnimatedIcon(playKey);
  return <svg ref={ref} aria-hidden="true" width={size} height={size} className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={size >= 20 ? 1.5 : 1.75} strokeLinecap="round" strokeLinejoin="round">
    <path d="M19 5l3-3M2 22l3-3" />
    <motion.g initial="normal" animate={controls} variants={{ normal: { x: 0, y: 0 }, animate: { x: [0, 2, 0], y: [0, -2, 0] } }} transition={{ duration: consoleMotion.emphasis, ease: consoleMotion.ease }}>
      <path d="M6.3 20.3a2.4 2.4 0 0 0 3.4 0L12 18l-6-6-2.3 2.3a2.4 2.4 0 0 0 0 3.4ZM7.5 13.5l2.5-2.5M10.5 16.5l2.5-2.5" />
    </motion.g>
    <motion.path d="m12 6 6 6 2.3-2.3a2.4 2.4 0 0 0 0-3.4l-2.6-2.6a2.4 2.4 0 0 0-3.4 0Z" initial="normal" animate={controls}
      variants={{ normal: { x: 0, y: 0 }, animate: { x: [0, -2, 0], y: [0, 2, 0] } }}
      transition={{ duration: consoleMotion.emphasis, ease: consoleMotion.ease }} />
  </svg>;
}

export function AnimatedShieldCheckIcon({ size = 20, className, playKey }: AnimatedIconProps) {
  const { ref, controls } = useAnimatedIcon(playKey);
  return <svg ref={ref} aria-hidden="true" width={size} height={size} className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={size >= 20 ? 1.5 : 1.75} strokeLinecap="round" strokeLinejoin="round">
    <path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" />
    <motion.path d="m9 12 2 2 4-4" initial="normal" animate={controls}
      variants={{ normal: { pathLength: 1, opacity: 1 }, animate: { pathLength: [0, 1], opacity: [0.2, 1] } }}
      transition={{ duration: consoleMotion.emphasis, ease: consoleMotion.ease }} />
  </svg>;
}
