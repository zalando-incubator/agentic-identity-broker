import { lazy, Suspense, useCallback, useId, useLayoutEffect, useRef, useState } from 'react';
import type { ComponentProps } from 'react';
import { cva } from 'class-variance-authority';
import { Button } from '@design-system/components/primitives/Button';
import { cn } from '@design-system/utils/cn';

// Static loading would pull supplementary one-line tooltips into focused decision graphs.
const TruncatedTextTooltip = lazy(() => import('./TruncatedTextTooltip'));

const clampedText = cva('', {
  variants: { lines: { 1: 'truncate', 2: 'line-clamp-2', 3: 'line-clamp-3' } },
  defaultVariants: { lines: 2 },
});

export type TruncatedTextProps = Omit<ComponentProps<'div'>, 'children' | 'dangerouslySetInnerHTML'> & {
  text: string;
  as?: 'span' | 'p' | 'h1' | 'h2' | 'h3' | 'label';
  htmlFor?: string;
  /** Report clipping to an enclosing control that owns the tooltip. */
  onClippedChange?: (clipped: boolean) => void;
} & (
  | { interactive: false; lines?: 1 | 2 | 3; expandLabel?: string; collapseLabel?: string }
  | { interactive?: true; lines: 1; expandLabel?: string; collapseLabel?: string }
  | { interactive?: true; lines?: 2 | 3; expandLabel: string; collapseLabel: string }
);

export function TruncatedText({ text, as: Text = 'span', htmlFor, lines = 2, expandLabel, collapseLabel, interactive = true, onClippedChange, className, ...props }: TruncatedTextProps) {
  const contentId = useId();
  const measureRef = useRef<HTMLSpanElement>(null);
  const textRef = useRef<HTMLElement | null>(null);
  const restoreFocus = useRef(false);
  const setTextRef = useCallback((node: HTMLElement | null) => {
    if (!node) restoreFocus.current = document.activeElement === textRef.current;
    else if (restoreFocus.current) { node.focus({ preventScroll: true }); restoreFocus.current = false; }
    textRef.current = node;
  }, []);
  const [expanded, setExpanded] = useState(false);
  const [clipped, setClipped] = useState(false);

  useLayoutEffect(() => {
    const element = measureRef.current;
    if (!element) return;
    const measure = () => {
      const overflow = lines === 1
        ? element.scrollWidth > element.clientWidth
        : element.scrollHeight > element.clientHeight;
      setClipped(overflow);
      onClippedChange?.(overflow);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [text, lines, onClippedChange]);

  const content = <Text ref={setTextRef} id={contentId} htmlFor={Text === 'label' ? htmlFor : undefined} tabIndex={interactive && lines === 1 && clipped ? 0 : undefined} className={cn('block', lines !== 1 && 'whitespace-pre-wrap [overflow-wrap:anywhere]', (!expanded || lines === 1) && clampedText({ lines }))}>{text}</Text>;
  const Wrapper = Text === 'span' ? 'span' : 'div';
  return <Wrapper data-slot="truncated-text" className={cn('relative block min-w-0 max-w-full', className)} {...props}>
    {/* Measure the collapsed text even while its visible version is expanded. */}
    <span ref={measureRef} aria-hidden="true" data-measure-text={text} className={cn('pointer-events-none invisible absolute left-0 top-0 block w-full before:content-[attr(data-measure-text)]', lines !== 1 && 'whitespace-pre-wrap [overflow-wrap:anywhere]', clampedText({ lines }))} />
    {interactive && lines === 1 && clipped ? <Suspense fallback={content}><TruncatedTextTooltip text={text}>{content}</TruncatedTextTooltip></Suspense> : content}
    {interactive && lines !== 1 && clipped && <Button type="button" variant="ghost" size="sm" aria-controls={contentId} aria-expanded={expanded} onClick={() => setExpanded(value => !value)} className="mt-1 h-auto min-h-6 w-fit rounded-sm p-0 text-primary hover:bg-transparent hover:text-primary hover:underline">
      {expanded ? collapseLabel : expandLabel}
    </Button>}
  </Wrapper>;
}
