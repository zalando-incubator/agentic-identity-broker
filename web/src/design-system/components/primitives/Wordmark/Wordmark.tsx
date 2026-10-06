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
    className={cn('inline-flex h-[18px] max-w-full shrink-0 align-middle', compact && 'aspect-[46.2/27.7]', className)}
  >
    {compact ? <span className="aib-wordmark-compact" aria-hidden="true" /> : <>
      <img className="aib-wordmark-light" src="/brand/AIB_Wordmark_Black.svg" width="407" height="39" alt="" aria-hidden="true" />
      <img className="aib-wordmark-dark" src="/brand/AIB_Wordmark_White.svg" width="407" height="39" alt="" aria-hidden="true" />
    </>}
  </span>;
}
