import type { ComponentProps, CSSProperties } from 'react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const skeletonVariants = cva('bg-muted overflow-hidden', {
  variants: {
    variant: { line: 'h-4 w-full rounded', circle: 'rounded-full', rectangle: 'h-24 w-full', rounded: 'h-24 w-full rounded-lg' },
  },
  defaultVariants: { variant: 'line' },
});

export interface SkeletonProps extends Omit<ComponentProps<'div'>, 'children'> {
  variant?: 'line' | 'circle' | 'rectangle' | 'rounded';
  width?: CSSProperties['width'];
  height?: CSSProperties['height'];
  count?: number;
  gap?: CSSProperties['gap'];
  /** Omit when a parent already announces loading. */
  label?: string;
}

export function Skeleton({ variant = 'line', width, height, count = 1, gap = '0.5rem', label, className, style, ...props }: SkeletonProps) {
  const dimensions: CSSProperties = {
    width: variant === 'circle' ? width ?? height ?? '2.5rem' : width,
    height: variant === 'circle' ? height ?? width ?? '2.5rem' : height,
  };
  return (
    <div {...props} role={label ? 'status' : undefined} aria-live={label ? 'polite' : undefined} aria-hidden={label ? undefined : true} className={cn(count === 1 ? skeletonVariants({ variant }) : 'flex flex-col', className)} style={{ ...(count === 1 ? dimensions : { gap }), ...style }}>
      {count > 1 && Array.from({ length: count }, (_, index) => <div key={index} aria-hidden="true" className={skeletonVariants({ variant })} style={dimensions} />)}
      {label && <span className="sr-only">{label}</span>}
    </div>
  );
}

export default Skeleton;
