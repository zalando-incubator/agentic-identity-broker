import { useId, useState, type ComponentProps } from 'react';
import { cva } from 'class-variance-authority';
import { Button } from '@design-system/components/primitives/Button';
import { cn } from '@design-system/utils/cn';

const clampedText = cva('', {
  variants: { lines: { 1: 'line-clamp-1', 2: 'line-clamp-2' } },
  defaultVariants: { lines: 2 },
});

export interface TruncatedTextProps extends Omit<ComponentProps<'div'>, 'children' | 'dangerouslySetInnerHTML'> {
  text: string;
  as?: 'span' | 'p' | 'h1' | 'h2' | 'h3';
  lines?: 1 | 2;
  expandLabel: string;
  collapseLabel: string;
}

export function TruncatedText({ text, as: Text = 'span', lines = 2, expandLabel, collapseLabel, className, ...props }: TruncatedTextProps) {
  const contentId = useId();
  const [expanded, setExpanded] = useState(false);

  return <div data-slot="truncated-text" className={cn('min-w-0 max-w-full', className)} {...props}>
    <Text id={contentId} className={cn('block whitespace-pre-wrap [overflow-wrap:anywhere]', !expanded && clampedText({ lines }))}>{text}</Text>
    <Button type="button" variant="ghost" size="sm" aria-controls={contentId} aria-expanded={expanded} onClick={() => setExpanded((value) => !value)} className="mt-1 h-auto max-w-full whitespace-normal px-1 text-left [overflow-wrap:anywhere]">
      {expanded ? collapseLabel : expandLabel}
    </Button>
  </div>;
}
