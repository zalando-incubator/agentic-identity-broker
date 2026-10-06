import type { ComponentProps } from 'react';
import { cn } from '@design-system/utils/cn';
import './DecisionSuccessCheck.css';

export function DecisionSuccessCheck({ className, ...props }: ComponentProps<'svg'>) {
  return <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" className={cn('decision-success-check', className)} {...props}>
    <path pathLength="1" d="m5 12 4 4 10-10" />
  </svg>;
}
