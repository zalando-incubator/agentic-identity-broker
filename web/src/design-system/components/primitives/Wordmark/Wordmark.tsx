import type { ComponentProps } from 'react';
import { cn } from '@design-system/utils';
import { useTheme } from '@design-system/theme/ThemeProvider';
import './Wordmark.css';

export interface WordmarkProps extends Omit<ComponentProps<'span'>, 'children'> {
  label: string;
  compact?: boolean;
}

export function Wordmark({ label, compact = false, className, ...props }: WordmarkProps) {
  const { resolvedTheme } = useTheme();
  return <span
    {...props}
    role="img"
    aria-label={label}
    data-testid="wordmark"
    data-variant={compact ? 'compact' : resolvedTheme === 'dark' ? 'white' : 'black'}
    className={cn('inline-block shrink-0 align-middle', compact ? 'size-8' : 'w-52 max-w-full', className)}
  >
    {compact ? <span className="aib-wordmark-compact" aria-hidden="true" /> : <>
      <img className="aib-wordmark-light" src="/brand/AIB_Wordmark_Black.svg" width="500" height="60" alt="" aria-hidden="true" />
      <img className="aib-wordmark-dark" src="/brand/AIB_Wordmark_White.svg" width="500" height="60" alt="" aria-hidden="true" />
    </>}
  </span>;
}
